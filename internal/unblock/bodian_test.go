package unblock

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/go-mp3"
	"netease-music-mygo/internal/netease"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A cover or a different version must not replace the requested original.
func TestMatch(t *testing.T) {
	track := netease.Track{Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Duration: 269000}
	good := candidate{ID: "MUSIC_228908", Name: "晴天", Artist: "周杰伦", Duration: "269"}
	if !match(track, good) {
		t.Fatal("original did not match")
	}
	for _, c := range []candidate{
		{ID: good.ID, Name: good.Name, Artist: "陈星晨", Duration: good.Duration},
		{ID: good.ID, Name: "晴天 (Live)", Artist: good.Artist, Duration: good.Duration},
		{ID: good.ID, Name: good.Name, Artist: good.Artist, Duration: "200"},
	} {
		if match(track, c) {
			t.Fatalf("wrong recording matched: %+v", c)
		}
	}
}

// An unavailable song resolves through its matching source and preserves its stream format.
func TestSong(t *testing.T) {
	old := client
	defer func() { client = old }()
	client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := `{"content":[{"musicpage":{"abslist":[{"MUSICRID":"MUSIC_1","SONGNAME":"晴天","ARTIST":"陈星晨","DURATION":"269"},{"MUSICRID":"MUSIC_228908","SONGNAME":"晴天","ARTIST":"周杰伦","DURATION":"269"}]}}]}`
		if r.URL.Path == "/api/play/music/v2/audioUrl" {
			if r.URL.Query().Get("musicId") != "228908" || r.URL.Query().Get("sign") == "" {
				t.Fatalf("bad request: %s", r.URL)
			}
			body = `{"code":200,"data":{"audioUrl":"https://example.com/song.mp3"}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	track := netease.Track{Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Duration: 269000}
	stream, err := Song(track)
	if err != nil || stream.URL != "https://example.com/song.mp3" || stream.Type != "mp3" {
		t.Fatalf("stream=%+v err=%v", stream, err)
	}
}

// A live match resolves the original Jay Chou recording of Sunny Day.
func TestSongLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to call the music source")
	}
	track := netease.Track{ID: 186016, Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Duration: 269000}
	stream, err := Song(track)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("matched format=%s level=%s", stream.Type, stream.Level)
	req, err := http.NewRequest("GET", stream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		t.Fatalf("audio: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil || len(b) < 1024 {
		t.Fatalf("audio bytes=%d err=%v", len(b), err)
	}
	decoded, err := mp3.NewDecoder(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	seconds := decoded.Length() / int64(decoded.SampleRate()*4)
	if seconds < 264 || seconds > 274 {
		t.Fatalf("audio duration %ds, expected about 269s", seconds)
	}
	t.Logf("downloaded %d bytes, decoded duration %ds", len(b), seconds)
}
