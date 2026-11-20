package netease

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// An acknowledged scan retries a failed profile lookup using its own cookies, and only a persisted login becomes active.
func TestQRLoginIsolatedUntilCommitted(t *testing.T) {
	var polls atomic.Int32
	qrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		if r.URL.Path != "/eapi/login/qrcode/client/login" {
			t.Errorf("polled %s", r.URL.Path)
		}
		if _, err := r.Cookie("MUSIC_U"); !errors.Is(err, http.ErrNoCookie) {
			t.Error("the new scan inherited the active account")
		}
		http.SetCookie(w, &http.Cookie{Name: "MUSIC_U", Value: "new-test-session"})
		io.WriteString(w, `{"code":803}`)
	}))
	defer qrServer.Close()
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	active, _ := cookiejar.New(nil)
	active.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "old-test-session"}, {Name: "__csrf", Value: "old-test-csrf"}})
	profiles := 0
	old := httpClient
	httpClient = &http.Client{Jar: active, Transport: weapiTransport(func(r *http.Request) (*http.Response, error) {
		profiles++
		cookie, err := r.Cookie("MUSIC_U")
		if err != nil || cookie.Value != "new-test-session" {
			t.Fatal("profile lookup did not use the QR attempt's session")
		}
		if profiles == 1 {
			return nil, errors.New("temporary lookup failure")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":200,"profile":{"userId":42}}`))}, nil
	})}
	defer func() { httpClient = old }()
	isolated, _ := cookiejar.New(nil)
	attempt := &QRLogin{key: "test-key", jar: isolated}
	ctx := context.Background()
	if status, err := attempt.check(ctx, qrServer.URL); err == nil || status.Code != QRConfirmed {
		t.Fatalf("initial lookup = %+v, %v", status, err)
	}
	if status, err := attempt.check(ctx, qrServer.URL); err != nil || status.Profile == nil || status.Profile.UserID != 42 || polls.Load() != 1 {
		t.Fatalf("retry = %+v, %v, after %d QR checks", status, err, polls.Load())
	}
	assertSession := func(wantUser, wantCSRF string) {
		t.Helper()
		var user, csrf string
		for _, cookie := range currentHTTPClient().Jar.Cookies(siteURL) {
			switch cookie.Name {
			case "MUSIC_U":
				user = cookie.Value
			case "__csrf":
				csrf = cookie.Value
			}
		}
		if user != wantUser || csrf != wantCSRF {
			t.Fatal("active session changed at the wrong time or retained the previous CSRF cookie")
		}
	}
	assertSession("old-test-session", "old-test-csrf")
	refused := errors.New("keychain unavailable")
	if err := attempt.commit(func([]byte) error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("commit error = %v", err)
	}
	assertSession("old-test-session", "old-test-csrf")
	if err := attempt.commit(func(b []byte) error {
		if string(b) != "MUSIC_U=new-test-session" {
			t.Fatal("persisted a cookie from another session")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertSession("new-test-session", "")
}

// The active session stays in place while the reader checks another account's saved cookies.
func TestSavedAccountStaysSeparateUntilActivated(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	active := sessionJar([]*http.Cookie{{Name: "MUSIC_U", Value: "active-test-session"}})
	initial := &http.Client{Jar: active, Transport: weapiTransport(func(r *http.Request) (*http.Response, error) {
		cookie, err := r.Cookie("MUSIC_U")
		if err != nil || cookie.Value != "saved-test-session" || r.URL.Path != "/api/nuser/account/get" {
			t.Fatal("restoring did not validate its own stored login")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":200,"profile":{"userId":42}}`))}, nil
	})}
	httpClient = initial
	account, err := loadSavedAccount(context.Background(), func() ([]byte, error) { return []byte("MUSIC_U=saved-test-session"), nil })
	if err != nil || account == nil || account.Profile == nil || account.Profile.UserID != 42 || currentHTTPClient() != initial {
		t.Fatalf("saved account was activated before acceptance: %v, %v", account != nil, err)
	}
	if err := account.Activate(); err != nil {
		t.Fatal(err)
	}
	jar := currentHTTPClient().Jar
	cookies := jar.Cookies(siteURL)
	if len(cookies) != 1 || cookies[0].Value != "saved-test-session" || jar == active {
		t.Fatal("accepted session did not become the active request context")
	}
}

// Signing out removes the local login before sending a server request, which still belongs to the account that signed out.
func TestSignOutKeepsNewSessionWhenOldRequestReturns(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	requests := 0
	httpClient = &http.Client{
		Jar: sessionJar([]*http.Cookie{{Name: "MUSIC_U", Value: "old-test-session"}}),
		Transport: weapiTransport(func(r *http.Request) (*http.Response, error) {
			requests++
			cookie, err := r.Cookie("MUSIC_U")
			if err != nil || cookie.Value != "old-test-session" || r.Method != "POST" || r.URL.Path != "/api/logout" {
				t.Fatal("logout used the new session or hit the wrong endpoint")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Set-Cookie": {"MUSIC_U=; Path=/; Max-Age=0"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":200}`)),
			}, nil
		}),
	}
	refused := errors.New("keychain unavailable")
	if revoke, err := endSession(func() error { return refused }); !errors.Is(err, refused) || revoke != nil {
		t.Fatalf("failed local removal allowed server logout: %v", err)
	}
	if cookies := currentHTTPClient().Jar.Cookies(siteURL); len(cookies) != 1 {
		t.Fatal("failed local removal lost the active session")
	}
	revoke, err := endSession(func() error { return nil })
	if err != nil || revoke == nil || len(currentHTTPClient().Jar.Cookies(siteURL)) != 0 || requests != 0 {
		t.Fatalf("local logout did not finish before the request: %v, %d requests", err, requests)
	}
	replaceSessionJar(sessionJar([]*http.Cookie{{Name: "MUSIC_U", Value: "new-test-session"}}))
	revoke()
	if cookies := currentHTTPClient().Jar.Cookies(siteURL); requests != 1 || len(cookies) != 1 || cookies[0].Value != "new-test-session" {
		t.Fatal("the old logout removed the new login")
	}
}

// A catalog response that arrives after switching accounts can only write cookies to its original jar.
func TestCatalogResponseCannotChangeReplacementSession(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{
		Jar: sessionJar([]*http.Cookie{{Name: "MUSIC_U", Value: "old-test-session"}}),
		Transport: weapiTransport(func(r *http.Request) (*http.Response, error) {
			cookie, err := r.Cookie("MUSIC_U")
			if err != nil || cookie.Value != "old-test-session" {
				t.Fatal("catalog request did not start on the original account")
			}
			replaceSessionJar(sessionJar([]*http.Cookie{{Name: "MUSIC_U", Value: "new-test-session"}}))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Set-Cookie": {"MUSIC_U=old-response-session; Path=/"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":200}`)),
			}, nil
		}),
	}
	if err := get("/api/test-catalog", nil, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if cookies := currentHTTPClient().Jar.Cookies(siteURL); len(cookies) != 1 || cookies[0].Value != "new-test-session" {
		t.Fatal("response from a retired account overwrote the active session")
	}
}

// The cookie string of a login body seeds the attempt's jar where Set-Cookie stays away, so Commit finds MUSIC_U.
func TestBodyCookieSeedsLoginJar(t *testing.T) {
	jar := sessionJar(nil)
	body := "MUSIC_U=body-session; Max-Age=1296000; Expires=Wed, 01 Jan 2031 00:00:00 GMT; Path=/;; __csrf=body-csrf; Path=/"
	seedBodyCookies(jar, body)
	_, session, err := sessionCookiesOf(jar)
	if err != nil {
		t.Fatalf("the body cookie did not seed the jar: %v", err)
	}
	for _, part := range []string{"MUSIC_U=body-session", "__csrf=body-csrf"} {
		if !strings.Contains(string(session), part) {
			t.Fatalf("session %q is missing %s", session, part)
		}
	}
}
