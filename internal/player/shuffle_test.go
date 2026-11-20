package player

import (
	"errors"
	"netease-music-mygo/internal/netease"
	"testing"
)

// Shuffling a list may start anywhere; explicitly selecting a song still pins that song first.
func TestPlayShuffledStartsAnywhere(t *testing.T) {
	old := openTrack
	openTrack = func(*Player, netease.Track, string) (*netease.Stream, stream, error) {
		return nil, nil, errors.New("offline test")
	}
	defer func() { openTrack = old }()
	p := New(func(func()) {})
	defer p.stop()
	tracks := make([]netease.Track, 10)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), LocalPath: "/missing/shuffle.mp3"}
	}
	heads := map[int64]bool{}
	for range 128 {
		p.PlayShuffled(tracks)
		heads[p.Current().ID] = true
		if !p.Shuffle() || len(p.order) != len(tracks) {
			t.Fatal("shuffle did not replace queue")
		}
		seen := map[int]bool{}
		for _, i := range p.order {
			if seen[i] {
				t.Fatal("duplicate shuffled song")
			}
			seen[i] = true
		}
	}
	if len(heads) < 2 {
		t.Fatalf("shuffle always starts on %v", heads)
	}
	p.PlayQueue(tracks, 3)
	if p.Current().ID != tracks[3].ID {
		t.Fatal("shuffle displaced explicitly selected song")
	}
	p.PlayShuffled(nil)
	if p.Current().ID != tracks[3].ID {
		t.Fatal("empty shuffle replaced queue")
	}
}
