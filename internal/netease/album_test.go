package netease

import (
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
)

type albumTransport func(*http.Request) (*http.Response, error)

func (f albumTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Album counts use the dynamic endpoint's subCount, not likedCount or
// the number of tracks; an API error must not masquerade as zero counts.
func TestAlbumDynamic(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	for _, tc := range []struct {
		body string
		fail bool
	}{
		{`{"code":200,"subCount":76019,"likedCount":0,"commentCount":1988,"isSub":true}`, false},
		{`{"code":405,"message":"unavailable"}`, true},
	} {
		httpClient = &http.Client{Transport: albumTransport(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/album/detail/dynamic" || r.URL.Query().Get("id") != "32311" {
				t.Fatalf("requested %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})}
		got, err := AlbumDynamic(32311)
		if tc.fail {
			if err == nil || got != nil {
				t.Fatal("API failure became a successful zero count")
			}
			continue
		}
		if err != nil || got.SubCount != 76019 || got.CommentCount != 1988 || !got.IsSub {
			t.Fatalf("counts %+v, %v", got, err)
		}
	}
}

// Collecting and uncollecting post the album ID to their matching routes;
// an API refusal must reach the caller so the optimistic count can roll back.
func TestSubscribeAlbum(t *testing.T) {
	old := httpClient
	defer func() { httpClient = old }()
	for _, tc := range []struct{ sub, fail bool }{{true, false}, {false, false}, {true, true}, {false, true}} {
		httpClient = &http.Client{Transport: albumTransport(func(r *http.Request) (*http.Response, error) {
			path := "/api/album/unsub"
			if tc.sub {
				path = "/api/album/sub"
			}
			if r.Method != http.MethodPost || r.URL.Path != path {
				t.Fatalf("requested %s %s", r.Method, r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("id") != "32311" {
				t.Fatal("did not send the album ID")
			}
			body := `{"code":200}`
			if tc.fail {
				body = `{"code":405,"message":"refused"}`
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		if err := SubscribeAlbum(32311, tc.sub); (err != nil) != tc.fail {
			t.Fatalf("subscribe %v, expected failure %v, got %v", tc.sub, tc.fail, err)
		}
	}
}

// Sending and replying encode the chosen album and parent comment into
// the real eapi request, and reject an API refusal instead of losing a draft.
func TestSendAlbumComment(t *testing.T) {
	oldClient, oldTransport := httpClient, http.DefaultTransport
	defer func() { httpClient, http.DefaultTransport = oldClient, oldTransport }()
	jar, _ := cookiejar.New(nil)
	httpClient = &http.Client{Jar: jar}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	for _, tc := range []struct {
		reply int64
		body  string
		fail  bool
	}{
		{0, `{"code":200,"comment":{"commentId":77,"content":"同感"}}`, false},
		{9, `{"code":200,"comment":{"commentId":78,"content":"同感"}}`, false},
		{9, `{"code":405,"message":"refused"}`, true},
	} {
		http.DefaultTransport = albumTransport(func(r *http.Request) (*http.Response, error) {
			action := "add"
			if tc.reply != 0 {
				action = "reply"
			}
			if r.URL.Path != "/eapi/resource/comments/"+action {
				t.Fatalf("requested %s", r.URL.Path)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			sealed, err := hex.DecodeString(r.Form.Get("params"))
			if err != nil || len(sealed)%aes.BlockSize != 0 {
				t.Fatal("invalid encrypted request")
			}
			block, _ := aes.NewCipher([]byte(eapiKey))
			plain := make([]byte, len(sealed))
			for i := 0; i < len(sealed); i += aes.BlockSize {
				block.Decrypt(plain[i:i+aes.BlockSize], sealed[i:i+aes.BlockSize])
			}
			parts := strings.Split(string(plain), "-36cd479b6b5-")
			if len(parts) != 3 || parts[0] != "/api/resource/comments/"+action {
				t.Fatal("wrong signed endpoint")
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(parts[1]), &data); err != nil {
				t.Fatal(err)
			}
			if data["threadId"] != "R_AL_3_8" || data["content"] != "同感" {
				t.Fatal("sent to the wrong thread or changed the content")
			}
			if tc.reply != 0 && data["commentId"] != "9" {
				t.Fatal("lost the reply target")
			}
			if tc.reply == 0 && data["commentId"] != nil {
				t.Fatal("turned a new comment into a reply")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
		})
		cm, err := SendComment("R_AL_3_8", "同感", tc.reply)
		if tc.fail {
			if err == nil || cm != nil {
				t.Fatal("a refused comment was accepted")
			}
			continue
		}
		if err != nil || cm.ID == 0 || cm.Content != "同感" {
			t.Fatalf("comment %+v, %v", cm, err)
		}
	}
}

// LIVE checks the actual album counts and comment thread without posting
// or liking anything on the listener's behalf.
func TestAlbumCommentsLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	activity, err := AlbumDynamic(32311)
	if err != nil || activity.SubCount <= 0 || activity.CommentCount <= 0 {
		t.Fatalf("activity %+v, %v", activity, err)
	}
	page, err := ThreadComments("R_AL_3_32311", 0, 2)
	if err != nil || len(page.Latest) == 0 || page.Latest[0].ID == 0 || page.Total <= 0 {
		t.Fatalf("comments %+v, %v", page, err)
	}
}
