package player

import (
	"testing"
	"time"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// Warmed lyrics must appear at the audible boundary, while another track's cached lyrics must never leak through.
func TestPreparedLyricsFollowTrack(t *testing.T) {
	old := fetchLyrics
	defer func() { fetchLyrics = old }()
	asked := make(chan int64, 1)
	fetchLyrics = func(id int64) (string, string, string, error) {
		asked <- id
		return "[00:00.00]新的一首", "", "", nil
	}
	posts := make(chan func(), 4)
	p := &Player{post: func(fn func()) { posts <- fn }}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	p.prefetchedLyrics.id, p.prefetchedLyrics.ready = 1, true
	p.prefetchedLyrics.lines = []lyrics.Line{{At: time.Second, Text: "已经准备好的歌词"}}
	p.loadMetadata(p.queue[0], 0)
	if !p.lyricsLoaded || len(p.lyrics) != 1 || p.lyrics[0].At != time.Second {
		t.Fatal("prepared lyrics were discarded")
	}
	p.loadMetadata(p.queue[1], 0)
	select {
	case id := <-asked:
		if id != 2 {
			t.Fatalf("fetched lyrics for %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("another track reused stale lyrics")
	}
	next(t, posts)
	if len(p.lyrics) != 1 || p.lyrics[0].Text != "新的一首" {
		t.Fatal("new lyrics were not published")
	}
}
