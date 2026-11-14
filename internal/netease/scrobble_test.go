package netease

import (
	"context"
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

// Playback start and heard time use the client log endpoint; the old web endpoint acknowledged but discarded plays.
func TestScrobblePostsClientLog(t *testing.T) {
	oldClient, oldTransport := httpClient, http.DefaultTransport
	defer func() { httpClient, http.DefaultTransport = oldClient, oldTransport }()
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "session", Path: "/"}})
	httpClient = &http.Client{Jar: jar}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	var actions []string
	code := 200
	http.DefaultTransport = weapiTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "clientlog.music.163.com" || r.URL.Path != "/eapi/feedback/weblog" {
			t.Fatalf("wrong endpoint: %s", r.URL)
		}
		if !strings.Contains(r.Header.Get("Cookie"), "MUSIC_U=session") || !strings.Contains(r.Header.Get("Cookie"), "os=osx") {
			t.Fatal("client log request lost the session or client identity")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		ciphertext, err := hex.DecodeString(r.Form.Get("params"))
		if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
			t.Fatal("invalid encrypted request")
		}
		block, _ := aes.NewCipher([]byte(eapiKey))
		plain := make([]byte, len(ciphertext))
		for i := 0; i < len(ciphertext); i += aes.BlockSize {
			block.Decrypt(plain[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
		}
		parts := strings.Split(string(plain), "-36cd479b6b5-")
		if len(parts) != 3 || parts[0] != "/api/feedback/weblog" {
			t.Fatal("invalid eapi payload")
		}
		var data struct {
			Logs   string            `json:"logs"`
			Header map[string]string `json:"header"`
		}
		if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
			t.Fatal(err)
		}
		if data.Header["os"] != "osx" {
			t.Fatal("encrypted identity is not osx")
		}
		var logs []struct {
			Action string         `json:"action"`
			JSON   map[string]any `json:"json"`
		}
		if err := json.Unmarshal([]byte(data.Logs), &logs); err != nil || len(logs) != 1 {
			t.Fatal("invalid log")
		}
		entry := logs[0]
		if entry.JSON["id"] != float64(42) || entry.JSON["content"] != "id=42" {
			t.Fatalf("wrong song: %v", entry.JSON)
		}
		if entry.Action == "play" && (entry.JSON["time"] != float64(185) || entry.JSON["sourceId"] != "42" || entry.JSON["end"] != "playend") {
			t.Fatalf("wrong play: %v", entry.JSON)
		}
		actions = append(actions, entry.Action)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"code":%d}`, code))), Header: make(http.Header)}, nil
	})
	if err := ScrobbleStart(42, 0); err != nil {
		t.Fatal(err)
	}
	if err := Scrobble(42, 0, 185*time.Second, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Join(actions, ",") != "startplay,play" {
		t.Fatalf("actions %v", actions)
	}
	code = 400
	if err := ScrobbleStart(42, 0); err == nil {
		t.Fatal("server error ignored")
	}
}

// A delayed report keeps its original account, orders start/play, and cannot change the new account's cookies.
func TestPlaybackReporterKeepsItsSession(t *testing.T) {
	oldClient, oldTransport := httpClient, http.DefaultTransport
	defer func() { httpClient, http.DefaultTransport = oldClient, oldTransport }()
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "first", Path: "/"}})
	httpClient = &http.Client{Jar: jar}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	report := PlaybackReporter(42)
	jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "second", Path: "/"}})
	calls := 0
	http.DefaultTransport = weapiTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if !strings.Contains(r.Header.Get("Cookie"), "MUSIC_U=first") {
			t.Fatal("playback was attributed to the new account")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":200}`)), Header: http.Header{"Set-Cookie": {"MUSIC_U=stale; Path=/"}}}, nil
	})
	if err := report(context.Background(), 42*time.Second); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("requests %d", calls)
	}
	for _, cookie := range jar.Cookies(siteURL) {
		if cookie.Name == "MUSIC_U" && cookie.Value != "second" {
			t.Fatal("old response overwrote current account")
		}
	}
}

// A rejected start is not followed by play, and a failed/ambiguous request is not blindly retried.
func TestPlaybackReporterStopsOnError(t *testing.T) {
	oldClient, oldTransport := httpClient, http.DefaultTransport
	defer func() { httpClient, http.DefaultTransport = oldClient, oldTransport }()
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(siteURL, []*http.Cookie{{Name: "MUSIC_U", Value: "first", Path: "/"}})
	httpClient = &http.Client{Jar: jar}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	for _, status := range []int{200, 429, 500} {
		calls := 0
		http.DefaultTransport = weapiTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"code":400}`)), Header: make(http.Header)}, nil
		})
		if err := PlaybackReporter(42)(context.Background(), 42*time.Second); err == nil {
			t.Fatal("error ignored")
		}
		if calls != 1 {
			t.Fatalf("retried or sent play after failure: %d", calls)
		}
	}
}
