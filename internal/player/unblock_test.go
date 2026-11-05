package player

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// A song with no NetEase stream opens its alternative; network errors and podcasts do not trigger it.
func TestOpenAlternative(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	oldSong, oldAlt := resolveSong, resolveAlternative
	defer func() { resolveSong, resolveAlternative = oldSong, oldAlt }()
	calls := 0
	resolveSong = func(int64, string) (*netease.Stream, error) { return nil, netease.ErrUnavailable }
	resolveAlternative = func(netease.Track) (*netease.Stream, error) {
		calls++
		return &netease.Stream{URL: srv.URL, Size: int64(len(b)), Level: "exhigh", Type: "mp3"}, nil
	}
	p := &Player{}
	info, stream, err := p.open(netease.Track{ID: 1, Duration: 5000}, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if calls != 1 || info.URL != srv.URL {
		t.Fatalf("calls=%d stream=%+v", calls, info)
	}
	_, _, err = p.open(netease.Track{ID: 2, ProgramID: 2}, "exhigh")
	if !errors.Is(err, netease.ErrUnavailable) || calls != 1 {
		t.Fatalf("podcast: calls=%d err=%v", calls, err)
	}
	networkErr := errors.New("network failed")
	resolveSong = func(int64, string) (*netease.Stream, error) { return nil, networkErr }
	_, _, err = p.open(netease.Track{ID: 3}, "exhigh")
	if !errors.Is(err, networkErr) || calls != 1 {
		t.Fatalf("network: calls=%d err=%v", calls, err)
	}
}

// VIP and unavailable collaborative songs must decode fully through the real stream and fallback path.
func TestVIPPlaybackLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to call NetEase and the alternative music source")
	}
	for _, track := range []netease.Track{
		{ID: 1901371647, Name: "孤勇者", Artists: []netease.Artist{{Name: "陈奕迅"}}, Duration: 256000, Fee: 1},
		{ID: 531051217, Name: "等你下课 (with 杨瑞代)", Artists: []netease.Artist{{Name: "周杰伦"}, {Name: "杨瑞代"}}, Duration: 270000},
	} {
		t.Run(track.Name, func(t *testing.T) { checkFullPlayback(t, track) })
	}
}

func checkFullPlayback(t *testing.T, track netease.Track) {
	t.Helper()
	old := resolveAlternative
	defer func() { resolveAlternative = old }()
	alternative := false
	resolveAlternative = func(track netease.Track) (*netease.Stream, error) {
		alternative = true
		return old(track)
	}
	p := &Player{}
	info, decoded, err := p.open(track, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	buf := make([]float32, 32<<10)
	var samples int64
	for {
		n, err := decoded.Read(buf)
		samples += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	duration := time.Duration(samples) * time.Second / time.Duration(decoded.SampleRate()*decoded.Channels())
	if delta := duration - track.Length(); delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("decoded %v, expected about %v", duration, track.Length())
	}
	t.Logf("%s: alternative=%v format=%s decoded=%v", track.Name, alternative, info.Type, duration)
}
