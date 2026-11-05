package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1859245776, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://music.163.com/song?id=1859245776"},
	} {
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{
		{ID: 1, Name: "一张相片", Artists: []netease.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: 42}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://music.163.com/song?id=2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}

// A song's right-click menu queues it next without interrupting playback,
// including when it is already in the queue or is a local file.
func TestSongMenuPlaysNext(t *testing.T) {
	for _, local := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		tracks := []netease.Track{{ID: 1, Name: "一", Duration: 200000}, {ID: 2, Name: "二", Duration: 200000}, {ID: 3, Name: "三", Duration: 200000}}
		if local {
			tracks[2].LocalPath = "/music/三.mp3"
		}
		a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单", Tracks: tracks}
		a.route = route{page: pagePlaylist, id: 42}
		a.pl.Cue(tracks)
		a.pl.Hold(42 * time.Second)
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		for range 2 {
			if err := tt.RightClick("三"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("下一首播放"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			order, at := a.pl.PlayOrder()
			if a.pl.Queue()[order[at+1]].ID != 3 || len(a.pl.Queue()) != 3 || a.pl.Current().ID != 1 || a.pl.Position() != 42*time.Second || !a.pl.Playing() {
				t.Fatalf("local %v: current %+v, queue %+v, order %v at %d", local, a.pl.Current(), a.pl.Queue(), order, at)
			}
		}
	}
}
