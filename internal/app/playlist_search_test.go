package app

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

func playlistSearchApp(n int) (*app, *netease.Playlist) {
	a := newTestApp()
	a.settings = defaultSettings()
	p := &netease.Playlist{ID: 42, Name: "我的歌单"}
	for i := range n {
		p.Tracks = append(p.Tracks, netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌曲 %04d", i+1), Duration: 200000,
			LocalPath: "/missing/music.wav", Artists: []netease.Artist{{Name: "歌手"}}, Album: netease.Album{Name: "专辑"}})
	}
	a.playlists[p.ID], a.route = p, route{page: pagePlaylist, id: p.ID}
	return a, p
}

// A filtered row plays its original position and keeps the complete playlist as its queue.
func TestPlaylistSearchPlaysOriginalQueue(t *testing.T) {
	a, p := playlistSearchApp(25)
	p.Tracks[17].Name, p.Tracks[17].Alias = "宝船", []string{"Treasure"}
	tr := &p.Tracks[17]
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索歌单内歌曲"); err != nil {
		t.Fatal(err)
	}
	tt.Type("TREASURE 歌手")
	if got := a.playlistFilter.rows(p); !slices.Equal(got, []int{17}) {
		t.Fatalf("matched %v", got)
	}
	if err := tt.Click(tr.Name); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current() == nil || a.pl.Current().ID != tr.ID || len(a.pl.Queue()) != len(p.Tracks) {
		t.Fatalf("current %+v, queue length %d", a.pl.Current(), len(a.pl.Queue()))
	}
	if err := tt.Click("清除歌曲搜索"); err != nil {
		t.Fatal(err)
	}
	if got := a.playlistFilter.rows(p); got != nil {
		t.Fatalf("clearing kept %v", got)
	}
}

// Typing Space into the playlist's song search must edit the query, never toggle playback.
func TestPlaylistSearchSpaceAndShortcut(t *testing.T) {
	a, p := playlistSearchApp(5)
	a.pl.Cue(p.Tracks)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索歌单内歌曲"); err != nil {
		t.Fatal(err)
	}
	before := a.pl.Playing()
	tt.TypeKey(0, ui.KeySpace, " ")
	if a.pl.Playing() != before {
		t.Fatal("Space in the song search toggled playback")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	if !tt.Focused("搜索歌单内歌曲") {
		t.Fatal("Cmd+F did not focus the playlist search")
	}
}

// Cmd+F reaches the playlist's search, which is in its header, from far down a long list.
func TestPlaylistSearchShortcutFromScrolledList(t *testing.T) {
	a, _ := playlistSearchApp(2000)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	a.trackList.ScrollTo(1500, ui.Start)
	tt.Frame()
	if _, ok := tt.Find("搜索歌单内歌曲"); ok {
		t.Fatal("the search is still built far down the list")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	if !tt.Focused("搜索歌单内歌曲") {
		t.Fatal("Cmd+F did not focus the playlist search from a scrolled list")
	}
	tt.Type("0001")
	if a.playlistFilter.query != "0001" {
		t.Fatalf("query %q", a.playlistFilter.query)
	}
}

// A press anywhere in the playlist's search, its magnifier too, types in it, and clearing it leaves the typing there.
func TestPlaylistSearchFieldKeepsTyping(t *testing.T) {
	a, _ := playlistSearchApp(5)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("搜索歌单内歌曲")
	if !ok {
		t.Fatal("no playlist search")
	}
	// The magnifier, left of the text.
	tt.ClickAt(r.X-8, r.Y+r.H/2)
	tt.Frame()
	if !tt.Focused("搜索歌单内歌曲") {
		t.Fatal("pressing the magnifier did not focus the search")
	}
	tt.Type("0001")
	if err := tt.Click("清除歌曲搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Type("0002")
	if a.playlistFilter.query != "0002" {
		t.Fatalf("query %q after clearing and typing again", a.playlistFilter.query)
	}
}

// Missing song details must not shift the next batch's offset or duplicate requests; failed batches remain retryable.
func TestPlaylistRemainderBatchesAndRetry(t *testing.T) {
	old := playlistSongs
	defer func() { playlistSongs = old }()
	requests := make(chan []int64, 8)
	playlistSongs = func(ids []int64) ([]netease.Track, error) {
		requests <- slices.Clone(ids)
		return nil, errors.New("offline")
	}
	a, p := playlistSearchApp(1000)
	p.TrackOffset = 1000
	for i := range 1805 {
		p.TrackIDs = append(p.TrackIDs, struct {
			ID int64 `json:"id"`
		}{int64(i + 1)})
	}
	d := a.remainingPlaylist(p)
	a.morePlaylistSongs(d)
	a.morePlaylistSongs(d)
	ids := wait(t, requests, "first remaining batch")
	if len(ids) != 400 || ids[0] != 1001 || ids[399] != 1400 || len(requests) != 0 {
		t.Fatalf("batch %v", ids)
	}
	a.playlistSongsLoaded(d, 1400, nil, errors.New("offline"))
	a.morePlaylistSongs(d)
	if len(requests) != 0 || p.TrackOffset != 1000 {
		t.Fatal("failed batch advanced or retried automatically")
	}
	d.err = ""
	a.morePlaylistSongs(d)
	if got := wait(t, requests, "retry"); !slices.Equal(ids, got) {
		t.Fatal("retry skipped IDs")
	}
	// An unavailable song is omitted from the result, but its ID was resolved.
	a.playlistSongsLoaded(d, 1400, []netease.Track{{ID: 1001}}, nil)
	ids = wait(t, requests, "second remaining batch")
	if len(ids) != 400 || ids[0] != 1401 {
		t.Fatalf("second batch %v", ids)
	}
	a.playlistSongsLoaded(d, 1800, nil, nil)
	ids = wait(t, requests, "last remaining batch")
	if !slices.Equal(ids, []int64{1801, 1802, 1803, 1804, 1805}) {
		t.Fatalf("tail %v", ids)
	}
	a.playlistSongsLoaded(d, 1805, []netease.Track{{ID: 1805}}, nil)
	if p.TrackOffset != 1805 || p.Tracks[len(p.Tracks)-1].ID != 1805 {
		t.Fatal("tail missing from playlist")
	}
}

// Replies belonging to a refreshed playlist or a previous account cannot append songs into the new snapshot.
func TestPlaylistRemainderRejectsStaleReply(t *testing.T) {
	for _, account := range []bool{false, true} {
		a, p := playlistSearchApp(1)
		d := a.remainingPlaylist(p)
		if account {
			a.profile = &netease.Profile{UserID: 2}
		} else {
			a.playlists[p.ID] = &netease.Playlist{ID: p.ID}
		}
		a.playlistSongsLoaded(d, 400, []netease.Track{{ID: 2}}, nil)
		if len(p.Tracks) != 1 || p.TrackOffset != 0 {
			t.Fatal("stale reply was applied")
		}
	}
}

// Playing during loading waits for the whole queue, and a later choice cancels any earlier deferred play.
func TestPlaylistPlayWaitsForCompleteQueue(t *testing.T) {
	old := playlistSongs
	defer func() { playlistSongs = old }()
	requests := make(chan []int64, 4)
	playlistSongs = func(ids []int64) ([]netease.Track, error) { requests <- slices.Clone(ids); return nil, nil }
	a, p := playlistSearchApp(1)
	p.TrackOffset = 1
	p.TrackIDs = append(p.TrackIDs, struct {
		ID int64 `json:"id"`
	}{1}, struct {
		ID int64 `json:"id"`
	}{2})
	a.playPlaylist(p, 1, false)
	wait(t, requests, "remaining songs before play")
	if a.pl.Current() != nil {
		t.Fatal("started a truncated queue")
	}
	d := a.remainingPlaylist(p)
	a.playlistSongsLoaded(d, 2, []netease.Track{{ID: 2, Name: "二", LocalPath: "/missing/music.wav"}}, nil)
	if a.pl.Current() == nil || a.pl.Current().ID != 1 || len(a.pl.Queue()) != 2 {
		t.Fatal("did not start the complete queue")
	}
	// Another unfinished playlist should not win over a later click on this one.
	other := &netease.Playlist{ID: 43, TrackIDs: p.TrackIDs}
	a.playlists[43] = other
	a.playPlaylist(other, 2, false)
	wait(t, requests, "other playlist")
	a.playPlaylist(p, 1, false)
	a.playlistSongsLoaded(a.remainingPlaylist(other), 2, p.Tracks, nil)
	if a.pl.Current().ID != 1 {
		t.Fatal("an earlier deferred play overrode the later choice")
	}
}

// Matching includes names, artists, albums and aliases, and incorporates newly loaded tracks without copying a queue.
func TestPlaylistSongFilter(t *testing.T) {
	_, p := playlistSearchApp(2)
	p.Tracks[1].Alias = []string{"Treasure"}
	f := songFilter{query: "treasure 专辑"}
	if got := f.rows(p); !slices.Equal(got, []int{1}) {
		t.Fatalf("matched %v", got)
	}
	p.Tracks = append(p.Tracks, netease.Track{ID: 3, Name: "Treasure", Album: netease.Album{Name: "专辑"}})
	if got := f.rows(p); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("new tracks matched %v", got)
	}
	f.query = "nothing"
	if got := f.rows(p); got == nil || len(got) != 0 {
		t.Fatalf("no matches became all songs: %v", got)
	}
}

// With NM_FRAMES set, save the playlist's search and filtered songs for visual inspection.
func TestPlaylistSearchFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		a, p := playlistSearchApp(2000)
		a.mine, a.saved = []netease.Playlist{*p}, []netease.Playlist{{ID: 43, Name: "夜间音乐"}}
		a.pl.Cue(p.Tracks[19:])
		tt := ui.NewTester(a.view, 1200, 800)
		tt.SetDark(dark)
		tt.Frame()
		tt.Advance(time.Second)
		for _, state := range []string{"all", "filtered"} {
			if state == "filtered" {
				a.playlistFilter.query = "0020"
			}
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("playlist-search-%s-%t.png", state, dark)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func BenchmarkPlaylistSongSearch(b *testing.B) {
	_, p := playlistSearchApp(10000)
	f := songFilter{query: "歌手 0999"}
	f.rows(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.query = fmt.Sprintf("歌手 %04d", i%10000)
		f.rows(p)
	}
}

func BenchmarkPlaylistSongSearchCached(b *testing.B) {
	_, p := playlistSearchApp(10000)
	f := songFilter{query: "歌手 0999"}
	f.rows(p)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.rows(p)
	}
}

func BenchmarkPlaylistSongSearchCold(b *testing.B) {
	_, p := playlistSearchApp(10000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := songFilter{query: "歌手 0999"}
		f.rows(p)
	}
}
