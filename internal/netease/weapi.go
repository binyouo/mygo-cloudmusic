package netease

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

// The web player's API, the "weapi", is what the website records plays
// with: its requests are sealed twice, and say which session sent them.
// The eapi, which the desktop client uses, carries the session for
// streaming; the weapi carries it for the listening record.

const (
	weapiIV        = "0102030405060708"
	weapiPresetKey = "0CoJUm6Qyw8W8jud"
	weapiAlphabet  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	weapiUA        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0"
)

// weapiPublicKey is NetEase's, which seals each request's own key.
const weapiPublicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDgtQn2JZ34ZC28NWYpAUd98iZ37BUrX/aKzmFbt7clFSs6sXqHauqKWqdtLkF2KexO40H1YTX8z2lSgBBOAxLsvaklV8k4cBFK9snQXE9/DDaFt6Rr7iVZMldczhC0JNgTz+SHXT6CBHuX3e9SdB1Ua44oncaTWz7OBGLbCiK45wIDAQAB
-----END PUBLIC KEY-----`

// weapiSecret makes the 16 characters each request seals with a key of
// its own; a test sets one to know what it sent.
var weapiSecret = func() string {
	b := make([]byte, 16)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(weapiAlphabet))))
		if err != nil {
			panic(err)
		}
		b[i] = weapiAlphabet[n.Int64()]
	}
	return string(b)
}

// weapiPost calls path (as "/api/feedback/weblog") through the weapi with
// data, and decodes the JSON it answers into out. The session's cookies,
// the jar's, go with it, as the website's own requests do.
func weapiPost(path string, data map[string]any, out any) error {
	data["csrf_token"] = csrfToken()
	data["e_r"] = false
	params, encSecKey, err := weapiEncrypt(data)
	if err != nil {
		return err
	}
	form := url.Values{"params": {params}, "encSecKey": {encSecKey}}
	req, err := http.NewRequest("POST", apiBase+"/weapi/"+strings.TrimPrefix(path, "/api/"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", weapiUA)
	req.Header.Set("Referer", apiBase+"/")
	// What the website's player says of itself, so a play shows as one from
	// this computer and not from nowhere. The jar adds the session's own.
	for k, v := range map[string]string{
		"os":          eapiClient["os"],
		"appver":      eapiClient["appver"],
		"osver":       eapiClient["osver"],
		"channel":     eapiClient["channel"],
		"versioncode": "140",
		"deviceId":    device(),
	} {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// Keep what it sets for the site, whichever host set it.
	if set := res.Cookies(); len(set) > 0 && httpClient.Jar != nil {
		for _, c := range set {
			c.Domain = ""
			c.Path = "/"
		}
		httpClient.Jar.SetCookies(siteURL, set)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// csrfToken is the CSRF cookie the session was given, sent as csrf_token.
func csrfToken() string {
	if httpClient.Jar == nil {
		return ""
	}
	for _, c := range httpClient.Jar.Cookies(siteURL) {
		if c.Name == "__csrf" {
			return c.Value
		}
	}
	return ""
}

// weapiEncrypt seals data as the website's player does: the JSON under the
// preset key, that under a key of the request's own, and that key itself
// under NetEase's public key.
func weapiEncrypt(data map[string]any) (params, encSecKey string, err error) {
	text, err := json.Marshal(data)
	if err != nil {
		return "", "", err
	}
	secret := weapiSecret()
	first, err := weapiAES(text, weapiPresetKey)
	if err != nil {
		return "", "", err
	}
	params, err = weapiAES([]byte(first), secret)
	if err != nil {
		return "", "", err
	}
	encSecKey, err = weapiRSA(secret)
	return params, encSecKey, err
}

// weapiAES seals text with key and the shared IV in AES-128-CBC, PKCS#7,
// in base64.
func weapiAES(text []byte, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(text)%aes.BlockSize
	plain := make([]byte, len(text)+pad)
	copy(plain, text)
	for i := len(text); i < len(plain); i++ {
		plain[i] = byte(pad)
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, []byte(weapiIV)).CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(out), nil
}

// weapiRSA seals the request's key, reversed, to NetEase's public key
// without padding, as the website does, in hex.
func weapiRSA(secret string) (string, error) {
	block, _ := pem.Decode([]byte(weapiPublicKey))
	if block == nil {
		return "", fmt.Errorf("netease: no public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("netease: not an RSA public key")
	}
	reversed := []byte(secret)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	m := new(big.Int).SetBytes(reversed)
	c := new(big.Int).Exp(m, big.NewInt(int64(key.E)), key.N)
	b := make([]byte, (key.N.BitLen()+7)/8)
	c.FillBytes(b)
	return hex.EncodeToString(b), nil
}
