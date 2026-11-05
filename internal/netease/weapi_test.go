package netease

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"
)

type weapiTransport func(*http.Request) (*http.Response, error)

func (f weapiTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// unseal opens base64 of AES-128-CBC, PKCS#7, under key and the shared IV.
func unseal(t *testing.T, sealed, key string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		t.Fatalf("invalid sealed request %q: %v", sealed, err)
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, []byte(weapiIV)).CryptBlocks(out, raw)
	pad := int(out[len(out)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(out) {
		t.Fatalf("bad padding %d", pad)
	}
	return out[:len(out)-pad]
}

// The web API seals each request twice, the JSON under the preset key and
// that under a key of the request's own, which NetEase's public key seals:
// a wrong layer order, key or padding would not open, and the sealed key
// would not match the website's.
func TestWeapiSealsLikeTheWebsite(t *testing.T) {
	old := weapiSecret
	defer func() { weapiSecret = old }()
	weapiSecret = func() string { return "abcdefghijklmnop" }

	params, encSecKey, err := weapiEncrypt(map[string]any{"hello": "世界"})
	if err != nil || params == "" {
		t.Fatalf("sealed %q, %v", params, err)
	}
	const want = "d15a1683c992095d0c234c19966605c5c5964911268bbeda8cb8d08d834913e59d53b32358903a121b5fca784c1f5ae44951fd02524df58ecc98e52cc7cf8689b42c2e93ddf05b0592512d87f5960467e2f086c018849d76014d323500e30f13ef4cafbb0cf5a66731a3f1776c75ca35d0062dac70a3e33245afabcf47938487"
	if encSecKey != want {
		t.Fatalf("sealed key %q", encSecKey)
	}
	inner := string(unseal(t, params, "abcdefghijklmnop"))
	var got map[string]any
	if err := json.Unmarshal(unseal(t, inner, weapiPresetKey), &got); err != nil || got["hello"] != "世界" {
		t.Fatalf("opened %v, %v", got, err)
	}
}

// A scrobble is a play log posted to feedback/weblog: the song, how long it
// played, and the session's CSRF, in the shape NetEase records plays with.
func TestScrobblePostsAPlay(t *testing.T) {
	oldClient := httpClient
	defer func() { httpClient = oldClient }()
	oldSecret := weapiSecret
	defer func() { weapiSecret = oldSecret }()
	weapiSecret = func() string { return "abcdefghijklmnop" }
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })

	for _, tc := range []struct {
		code int
		fail bool
	}{{200, false}, {400, true}} {
		jar, _ := cookiejar.New(nil)
		jar.SetCookies(siteURL, []*http.Cookie{
			{Name: "MUSIC_U", Value: "session", Path: "/"},
			{Name: "__csrf", Value: "tok", Path: "/"},
		})
		var sent bool
		httpClient = &http.Client{Jar: jar, Transport: weapiTransport(func(r *http.Request) (*http.Response, error) {
			sent = true
			if r.Method != http.MethodPost || r.URL.Path != "/weapi/feedback/weblog" {
				t.Fatalf("requested %s %s", r.Method, r.URL.Path)
			}
			if !strings.Contains(r.Header.Get("Cookie"), "MUSIC_U=session") {
				t.Fatalf("no session cookie: %q", r.Header.Get("Cookie"))
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal(unseal(t, string(unseal(t, r.Form.Get("params"), "abcdefghijklmnop")), weapiPresetKey), &data); err != nil {
				t.Fatalf("opened request: %v", err)
			}
			if data["csrf_token"] != "tok" || data["e_r"] != false {
				t.Fatalf("csrf %v, e_r %v", data["csrf_token"], data["e_r"])
			}
			logs, ok := data["logs"].(string)
			if !ok {
				t.Fatalf("logs is %T", data["logs"])
			}
			var list []map[string]any
			if err := json.Unmarshal([]byte(logs), &list); err != nil || len(list) != 1 || list[0]["action"] != "play" {
				t.Fatalf("logs %q: %v", logs, err)
			}
			j, ok := list[0]["json"].(map[string]any)
			if !ok {
				t.Fatalf("log json is %T", list[0]["json"])
			}
			if j["id"] != float64(42) || j["sourceId"] != float64(42) || j["time"] != float64(185) ||
				j["end"] != "playend" || j["type"] != "song" || j["source"] != "list" || j["mainsite"] != float64(1) ||
				j["download"] != float64(0) || j["wifi"] != float64(0) || j["content"] != "" {
				t.Fatalf("play %v", j)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf("{\"code\":%d}", tc.code))), Header: make(http.Header)}, nil
		})}
		err := Scrobble(42, 0, 185*time.Second, "")
		if !sent || (err != nil) != tc.fail {
			t.Fatalf("code %d: scrobbled, err %v", tc.code, err)
		}
	}
}

// A scrobble names a song; there is nothing to post for a local file.
func TestScrobbleNeedsASong(t *testing.T) {
	if err := Scrobble(0, 0, time.Minute, ""); err == nil {
		t.Fatal("scrobbled a song without an ID")
	}
}
