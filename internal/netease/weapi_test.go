package netease

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"net/http"
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

// A scrobble names a song; there is nothing to post for a local file.
func TestScrobbleNeedsASong(t *testing.T) {
	if err := Scrobble(0, 0, time.Minute, ""); err == nil {
		t.Fatal("scrobbled a song without an ID")
	}
}
