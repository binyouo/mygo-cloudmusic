package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// Clicking the search icon or field edges focuses typing; these areas previously lost the click.
func TestSearchFieldWholeClickFocuses(t *testing.T) {
	for _, at := range []struct{ x, dy float32 }{{24, 0}, {sidebarWidth - 18, 0}, {sidebarWidth / 2, -13}, {sidebarWidth / 2, 13}} {
		a := newTestApp()
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		r, ok := tt.Find("搜索")
		if !ok {
			t.Fatal("no search field")
		}
		tt.ClickAt(at.x, r.Y+r.H/2+at.dy)
		tt.Type("中文")
		if a.query != "中文" {
			t.Errorf("click %+v: query %q", at, a.query)
		}
		a.dismissSuggestions()
	}
}

// Clearing the search returns its results page to Home while keeping an unrelated page and the search history.
func TestClearSearchButton(t *testing.T) {
	for _, onResults := range []bool{false, true} {
		a := newTestApp()
		r := route{page: pageSearch, query: "歌曲", kind: netease.SearchSongs}
		a.query, a.searched = "歌曲", r
		a.searches[r] = &searchData{page: paged{loaded: true}}
		a.settings.SearchHistory = []string{"歌曲"}
		if onResults {
			a.route = r
		} else {
			a.route = route{page: pageLocal}
		}
		before := a.route
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		if err := tt.Click("清除搜索"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		want := before
		if onResults {
			want = route{page: pageHome}
		}
		if a.query != "" || a.searched.query != "" || a.route != want || len(a.settings.SearchHistory) != 1 {
			t.Fatalf("clear: query %q, last %+v, page %+v, history %v", a.query, a.searched, a.route, a.settings.SearchHistory)
		}
	}
}

// Clearing search history from its popup forgets the entries without submitting a search.
func TestClearRecentSearchesMenu(t *testing.T) {
	a := newTestApp()
	a.settings.SearchHistory = []string{"甲", "乙"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("清空搜索历史"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.settings.SearchHistory) != 0 || a.route.page == pageSearch {
		t.Fatal("clearing history submitted or retained a search")
	}
}

// A song's download menu opens the save chooser for that song and leaves playback alone when cancelled.
func TestSongMenuDownload(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	dir := t.TempDir()
	a.pl.Cache = player.NewCache(dir, 1<<20)
	data := []byte("cached song")
	meta, _ := json.Marshal(netease.Stream{Type: "flac", Level: "lossless", Size: int64(len(data))})
	os.WriteFile(filepath.Join(dir, "42-lossless.flac"), data, 0600)
	os.WriteFile(filepath.Join(dir, "42-lossless.json"), meta, 0600)
	tr := netease.Track{ID: 42, Name: "歌曲", Artists: []netease.Artist{{Name: "歌手"}}}
	a.playlists[1] = &netease.Playlist{ID: 1, Tracks: []netease.Track{tr}}
	a.route = route{page: pagePlaylist, id: 1}
	a.pl.Cue([]netease.Track{{ID: 7}})
	chosen := make(chan string, 1)
	real := saveSongAs
	saveSongAs = func(_ *app, name string) string { chosen <- name; return "" }
	defer func() { saveSongAs = real }()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("歌曲"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("下载"); err != nil {
		t.Fatal(err)
	}
	if name := wait(t, chosen, "save chooser"); name != "歌手 - 歌曲.flac" {
		t.Fatalf("filename %q", name)
	}
	if a.pl.Current().ID != 7 {
		t.Fatal("download changed the current song")
	}
}

// Pulling Now Playing far down closes it while a short stationary pull returns it without changing the song.
func TestNowPlayingPullToClose(t *testing.T) {
	for _, distance := range []float32{30, 220} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.pl.Cue([]netease.Track{{ID: 7, Name: "歌曲", Duration: 200000}})
		a.pl.SetLyrics(nil)
		a.nowPlaying = true
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		x, y := float32(1100), float32(150)
		tt.Press(x, y)
		for i := 1; i <= 8; i++ {
			tt.Advance(20 * time.Millisecond)
			tt.Move(x, y+distance*float32(i)/8)
		}
		tt.Advance(200 * time.Millisecond)
		tt.Release(x, y+distance)
		tt.Advance(time.Second)
		tt.Frame()
		if a.nowPlaying != (distance < 160) || a.pl.Current().ID != 7 {
			t.Fatalf("pull %v: open %v, song %d", distance, a.nowPlaying, a.pl.Current().ID)
		}
	}
}
