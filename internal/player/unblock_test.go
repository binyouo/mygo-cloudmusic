package player

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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
