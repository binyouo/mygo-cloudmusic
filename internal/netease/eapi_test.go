package netease

import (
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// An iPhone User-Agent paired with the encrypted Windows PC identity caused intermittent -460 stream rejections.
func TestEapiKeepsClientIdentityConsistent(t *testing.T) {
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	http.DefaultTransport = weapiTransport(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		sealed, err := hex.DecodeString(r.Form.Get("params"))
		if err != nil || len(sealed) == 0 || len(sealed)%aes.BlockSize != 0 {
			t.Fatal("invalid eapi body")
		}
		block, _ := aes.NewCipher([]byte(eapiKey))
		plain := make([]byte, len(sealed))
		for i := 0; i < len(sealed); i += aes.BlockSize {
			block.Decrypt(plain[i:i+aes.BlockSize], sealed[i:i+aes.BlockSize])
		}
		parts := strings.Split(string(plain), "-36cd479b6b5-")
		if len(parts) != 3 {
			t.Fatal("invalid eapi signature structure")
		}
		var data struct {
			Header map[string]string `json:"header"`
		}
		if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
			t.Fatal(err)
		}
		ua := r.UserAgent()
		consistent := data.Header["os"] == "pc" && strings.HasPrefix(data.Header["osver"], "Microsoft-Windows-") &&
			strings.Contains(ua, "Windows NT") && !strings.Contains(ua, "iPhone") &&
			strings.Contains(ua, "NeteaseMusicDesktop/"+data.Header["appver"])
		body := `{"code":-460}`
		if consistent {
			body = `{"code":200}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	var result struct {
		Code int `json:"code"`
	}
	if err := eapiPost("/api/song/enhance/player/url/v1", map[string]any{"ids": "[1]", "level": "hires", "encodeType": "flac"}, &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 200 {
		t.Fatalf("request declared conflicting client identities: code %d", result.Code)
	}
}
