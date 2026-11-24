package netease

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// The listener's session is NetEase's login cookie, MUSIC_U, which every
// request carries once signed in. The app keeps it in the system's
// keychain. Development builds keep it in a private file instead: each
// rebuild is signed anew, and the keychain would ask before every one of
// them read it.

const sessionSecret = "netease-session"

func sessionFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session"), nil
}

func readSession() ([]byte, error) {
	if !mygo.IsDev() {
		return getStoredSession()
	}
	path, err := sessionFile()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func writeSession(b []byte) error {
	if !mygo.IsDev() {
		return setStoredSession(b)
	}
	path, err := sessionFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func deleteSession() error {
	if !mygo.IsDev() {
		err := deleteStoredSession()
		if errors.Is(err, mygo.ErrSecretNotFound) {
			return nil
		}
		return err
	}
	path, err := sessionFile()
	if err != nil {
		return err
	}
	if err = os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

var siteURL, _ = url.Parse(apiBase)
var clientMu sync.RWMutex

func init() {
	httpClient.Jar = sessionJar(nil)
}

func currentHTTPClient() *http.Client {
	clientMu.RLock()
	client := httpClient
	clientMu.RUnlock()
	return client
}

// Each request retains the client it started with. Responses affecting the
// retired jar cannot replace cookies in the new session.
func replaceSessionJar(jar http.CookieJar) {
	clientMu.Lock()
	client := *httpClient
	client.Jar = jar
	httpClient = &client
	clientMu.Unlock()
}

func sessionJar(cookies []*http.Cookie) http.CookieJar {
	jar, _ := cookiejar.New(nil)
	jar.SetCookies(siteURL, cookies)
	return jar
}

// SavedAccount is a checked startup session. It remains separate from the
// active client until the app accepts it. A nil profile means it expired.
type SavedAccount struct {
	Profile *Profile
	jar     http.CookieJar
}

func LoadSavedAccount(ctx context.Context) (*SavedAccount, error) {
	return loadSavedAccount(ctx, readSession)
}

func loadSavedAccount(ctx context.Context, read func() ([]byte, error)) (*SavedAccount, error) {
	b, err := read()
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, mygo.ErrSecretNotFound) {
		return nil, nil
	}
	if err != nil || len(b) == 0 {
		return nil, err
	}
	var cookies []*http.Cookie
	for _, part := range strings.Split(string(b), ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && (name == "MUSIC_U" || name == "__csrf") {
			cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/"})
		}
	}
	jar := sessionJar(cookies)
	profile, err := accountWithJar(ctx, jar)
	if err != nil {
		return nil, err
	}
	return &SavedAccount{Profile: profile, jar: jar}, nil
}

func (s *SavedAccount) Activate() error {
	if s.Profile == nil {
		return deleteSession()
	}
	replaceSessionJar(s.jar)
	return nil
}

type Profile struct {
	UserID    int64  `json:"userId"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

func accountWithJar(ctx context.Context, jar http.CookieJar) (*Profile, error) {
	var res struct {
		Profile *Profile `json:"profile"`
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/api/nuser/account/get", nil)
	if err != nil {
		return nil, err
	}
	client := *currentHTTPClient()
	client.Jar = jar
	err = doWithClient(&client, req, &res)
	return res.Profile, err
}

// QRLogin owns the cookies of one scan until the caller accepts its account.
type QRLogin struct {
	key       string
	jar       http.CookieJar
	confirmed bool
}

func BeginQRLogin(ctx context.Context) (*QRLogin, error) {
	login := &QRLogin{jar: sessionJar(nil)}
	var res struct {
		Code   int    `json:"code"`
		Unikey string `json:"unikey"`
	}
	if err := login.post(ctx, eapiDomain, "/api/login/qrcode/unikey", map[string]any{"type": 3}, &res); err != nil {
		return nil, err
	}
	if res.Unikey == "" {
		return nil, fmt.Errorf("netease: no QR key (code %d)", res.Code)
	}
	login.key = res.Unikey
	return login, nil
}

func (l *QRLogin) URL() string { return "https://music.163.com/login?codekey=" + l.key }

func (l *QRLogin) post(ctx context.Context, domain, path string, data map[string]any, out any) error {
	return eapiPostWithJar(ctx, path, data, out, domain, eapiClient, eapiUA, l.jar.Cookies(siteURL), l.jar)
}

// QR login states, as NetEase reports them.
const (
	QRExpired   = 800
	QRWaiting   = 801
	QRScanned   = 802
	QRConfirmed = 803
)

type QRLoginStatus struct {
	Code      int      `json:"code"`
	Message   string   `json:"message"`
	Nickname  string   `json:"nickname"`
	AvatarURL string   `json:"avatarUrl"`
	Profile   *Profile `json:"-"`
}

// Check also looks up the account after confirmation. A retry after a failed
// profile lookup keeps the confirmed cookies instead of polling the QR again.
func (l *QRLogin) Check(ctx context.Context) (QRLoginStatus, error) {
	return l.check(ctx, eapiDomain)
}

func (l *QRLogin) check(ctx context.Context, domain string) (QRLoginStatus, error) {
	var result QRLoginStatus
	if !l.confirmed {
		if err := l.post(ctx, domain, "/api/login/qrcode/client/login", map[string]any{"key": l.key, "type": 3}, &result); err != nil || result.Code != QRConfirmed {
			return result, err
		}
		if _, _, err := sessionCookiesOf(l.jar); err != nil {
			return QRLoginStatus{Code: QRExpired, Message: "登录已失效，请刷新二维码"}, nil
		}
		l.confirmed = true
	}
	result.Code = QRConfirmed
	profile, err := accountWithJar(ctx, l.jar)
	if err != nil {
		return result, err
	}
	if profile == nil {
		return QRLoginStatus{Code: QRExpired, Message: "登录已失效，请刷新二维码"}, nil
	}
	result.Profile = profile
	return result, nil
}

func sessionCookiesOf(jar http.CookieJar) (cookies []*http.Cookie, values []byte, err error) {
	var parts []string
	hasUser := false
	for _, c := range jar.Cookies(siteURL) {
		if c.Name == "MUSIC_U" && c.Value != "" {
			hasUser = true
		}
		if (c.Name == "MUSIC_U" || c.Name == "__csrf") && c.Value != "" {
			cookies = append(cookies, &http.Cookie{Name: c.Name, Value: c.Value, Path: "/"})
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	if !hasUser {
		return nil, nil, errors.New("no session cookie")
	}
	return cookies, []byte(strings.Join(parts, "; ")), nil
}

// commitJar persists the accepted account before making it the active session.
func commitJar(jar http.CookieJar, save func([]byte) error) error {
	cookies, session, err := sessionCookiesOf(jar)
	if err != nil {
		return err
	}
	if err := save(session); err != nil {
		return err
	}
	replaceSessionJar(sessionJar(cookies))
	return nil
}

func (l *QRLogin) Commit() error {
	return l.commit(writeSession)
}

func (l *QRLogin) commit(save func([]byte) error) error {
	return commitJar(l.jar, save)
}

// PasswordLogin owns the cookies of one credential check until the caller
// accepts its account, as QRLogin does. The profile is who signed in.
type PasswordLogin struct {
	jar     http.CookieJar
	Profile *Profile
}

var mobileNumber = regexp.MustCompile(`^1\d{10}$`)

// LoginWithPassword signs in with a phone number or an email and a
// password, through the website's login, as NeteaseCloudMusicApi does.
// The session cookies stay in the attempt's jar until Commit.
func LoginWithPassword(ctx context.Context, account, password string) (*PasswordLogin, error) {
	if account = strings.TrimSpace(account); account == "" || password == "" {
		return nil, errors.New("netease: empty credentials")
	}
	digest := md5.Sum([]byte(password))
	data := map[string]any{
		"password":      hex.EncodeToString(digest[:]),
		"rememberLogin": true,
		"https":         true,
	}
	path := "/api/login"
	if mobileNumber.MatchString(account) {
		path = "/api/login/cellphone"
		data["phone"], data["countrycode"] = account, "86"
	} else {
		data["username"] = account
	}
	return credentialLogin(ctx, path, data)
}

// credentialLogin posts the credentials of one attempt and keeps the
// session the answer earns in its own jar until Commit.
func credentialLogin(ctx context.Context, path string, data map[string]any) (*PasswordLogin, error) {
	jar := sessionJar(nil)
	client := *currentHTTPClient()
	client.Jar = jar
	var res struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Msg     string   `json:"msg"`
		Profile *Profile `json:"profile"`
		// Cookie is the session the body carries as text, which the
		// cellphone login sends where Set-Cookie stays away.
		Cookie string `json:"cookie"`
	}
	if err := weapiPostAs(&client, ctx, path, data, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 || res.Profile == nil {
		return nil, errors.New(loginErrorMessage(res.Code, firstNonEmpty(res.Message, res.Msg)))
	}
	seedBodyCookies(jar, res.Cookie)
	return &PasswordLogin{jar: jar, Profile: res.Profile}, nil
}

// seedBodyCookies folds the cookie string of a login body into the jar:
// it is the session's Set-Cookie line sent as text, which the jar never
// saw.
func seedBodyCookies(jar http.CookieJar, cookie string) {
	if jar == nil {
		return
	}
	for _, line := range strings.Split(cookie, ";;") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		for _, c := range (&http.Response{Header: http.Header{"Set-Cookie": {line}}}).Cookies() {
			c.Domain, c.Path = "", "/"
			jar.SetCookies(siteURL, []*http.Cookie{c})
		}
	}
}

// Commit persists the accepted account before making it the active session.
func (l *PasswordLogin) Commit() error {
	return l.commit(writeSession)
}

func (l *PasswordLogin) commit(save func([]byte) error) error {
	return commitJar(l.jar, save)
}

// MobileNumber reports whether s reads as a Chinese mobile number, which
// is what the SMS code login accepts.
func MobileNumber(s string) bool {
	return mobileNumber.MatchString(strings.TrimSpace(s))
}

// SendSMSCode asks NetEase to text a login code to the mobile number.
func SendSMSCode(ctx context.Context, phone string) error {
	if !MobileNumber(phone) {
		return errors.New("netease: not a mobile number")
	}
	client := *currentHTTPClient()
	client.Jar = sessionJar(nil)
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	err := weapiPostAs(&client, ctx, "/api/sms/captcha/sent", map[string]any{
		"cellphone": strings.TrimSpace(phone),
		"ctcode":    "86",
	}, &res)
	if err != nil {
		return err
	}
	if res.Code != 200 {
		return errors.New(loginErrorMessage(res.Code, firstNonEmpty(res.Message, res.Msg)))
	}
	return nil
}

// LoginWithSMSCode signs in with a mobile number and the code SendSMSCode
// had texted to it: the code itself passes the risk check a bare
// password hits.
func LoginWithSMSCode(ctx context.Context, phone, code string) (*PasswordLogin, error) {
	phone, code = strings.TrimSpace(phone), strings.TrimSpace(code)
	if !mobileNumber.MatchString(phone) || code == "" {
		return nil, errors.New("netease: incomplete sms login")
	}
	return credentialLogin(ctx, "/api/login/cellphone", map[string]any{
		"phone":         phone,
		"captcha":       code,
		"countrycode":   "86",
		"rememberLogin": true,
		"https":         true,
	})
}

// loginErrorMessage says why the credentials failed, in the user's words.
func loginErrorMessage(code int, msg string) string {
	switch code {
	case 415:
		return "账号需要安全验证，请改用短信验证码或扫码登录"
	case 509:
		return "尝试次数过多，请稍后再试"
	}
	if msg == "" {
		return "登录失败，请稍后再试"
	}
	return msg
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// EndSession clears local authentication first. Its server request is tied
// to the old login and can safely run after the app has switched accounts.
func EndSession() (func(), error) {
	return endSession(deleteSession)
}

func endSession(erase func() error) (func(), error) {
	client := *currentHTTPClient()
	client.Jar = sessionJar(client.Jar.Cookies(siteURL))
	if err := erase(); err != nil {
		return nil, err
	}
	replaceSessionJar(sessionJar(nil))
	return func() {
		req, err := http.NewRequest("POST", apiBase+"/api/logout", nil)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		doWithClient(&client, req, &struct{}{})
	}, nil
}

// UserPlaylists returns the playlists a listener made, the liked songs
// first, and those they saved.
func UserPlaylists(uid int64) (made, saved []Playlist, err error) {
	var res struct {
		Playlist []Playlist `json:"playlist"`
	}
	q := url.Values{"uid": {strconv.FormatInt(uid, 10)}, "limit": {"200"}, "offset": {"0"}}
	if err := get("/api/user/playlist", q, &res); err != nil {
		return nil, nil, err
	}
	for _, p := range res.Playlist {
		if p.Subscribed {
			saved = append(saved, p)
		} else {
			made = append(made, p)
		}
	}
	return made, saved, nil
}

// RecommendedPlaylists returns the day's playlists picked for the listener.
func RecommendedPlaylists() ([]Playlist, error) {
	var res struct {
		Recommend []struct {
			Playlist
			Playcount float64 `json:"playcount"`
		} `json:"recommend"`
	}
	if err := get("/api/v1/discovery/recommend/resource", nil, &res); err != nil {
		return nil, err
	}
	out := make([]Playlist, len(res.Recommend))
	for i, r := range res.Recommend {
		out[i] = r.Playlist
		out[i].PlayCount = r.Playcount
	}
	return out, nil
}

// LikedIDs returns the songs a listener likes, in no order.
func LikedIDs(uid int64) ([]int64, error) {
	var res struct {
		Code int     `json:"code"`
		IDs  []int64 `json:"ids"`
	}
	if err := eapiPost("/api/song/like/get", map[string]any{"uid": strconv.FormatInt(uid, 10)}, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: liked songs: code %d", res.Code)
	}
	return res.IDs, nil
}

// Play is a song the listener played, on one of their devices.
type Play struct {
	Track Track
	At    time.Time
	// Device is the system NetEase names the device by: iphone, android,
	// osx, pc, web…
	Device string
}

// RecentPlays returns the songs the listener played last, on any device
// signed in as them, the latest first.
func RecentPlays(limit int) ([]Play, error) {
	var res struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Data     Track `json:"data"`
				PlayTime int64 `json:"playTime"` // milliseconds
				Terminal struct {
					OS string `json:"os"`
				} `json:"multiTerminalInfo"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := eapiPost("/api/play-record/song/list", map[string]any{"limit": limit}, &res); err != nil {
		return nil, err
	}
	if res.Code != 200 {
		return nil, fmt.Errorf("netease: recent plays: code %d", res.Code)
	}
	plays := make([]Play, len(res.Data.List))
	songs := make([]Track, len(plays))
	for i, l := range res.Data.List {
		songs[i] = l.Data
		plays[i] = Play{At: time.UnixMilli(l.PlayTime), Device: l.Terminal.OS}
	}
	// They come without privileges, as the daily songs do.
	detailPrivileges(songs)
	for i := range plays {
		plays[i].Track = songs[i]
	}
	return plays, nil
}

// Like likes a song, adding it to the listener's liked songs, or unlikes it.
func Like(id int64, like bool) error {
	var res struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	data := map[string]any{"alg": "itembased", "trackId": strconv.FormatInt(id, 10), "like": strconv.FormatBool(like), "time": "3"}
	if err := eapiPost("/api/radio/like", data, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("netease: like: code %d %s", res.Code, res.Msg)
	}
	return nil
}
