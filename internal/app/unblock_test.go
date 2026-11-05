package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// A grey song can be selected for an alternative stream: the row used to ignore its click.
func TestGreySongCanBeSelected(t *testing.T) {
	a := editTestApp()
	a.playlists[10].Tracks[1].Privilege = &netease.Privilege{ID: 2, St: -1, Pl: 0}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("宝船"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Current() == nil || a.pl.Current().ID != 2 {
		t.Fatalf("selected track: %+v", a.pl.Current())
	}
}

// Play All must try the first VIP song's alternative instead of skipping it before loading.
func TestPlayAllStartsWithVIPSong(t *testing.T) {
	a := editTestApp()
	a.playlists[10].Tracks[0].Fee = 1
	a.playlists[10].Tracks[0].Privilege = &netease.Privilege{ID: 1, Pl: 0}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("播放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tr := a.pl.Current(); tr == nil || tr.ID != 1 {
		t.Fatalf("selected track: %+v", tr)
	}
}

// Starting heart mode must use a liked VIP song as its seed instead of rejecting every liked song.
func TestHeartModeTriesVIPSeed(t *testing.T) {
	old := fetchHeartMode
	defer func() { fetchHeartMode = old }()
	requested := make(chan [3]int64, 1)
	fetchHeartMode = func(id, start, pid int64) ([]netease.Track, error) {
		requested <- [3]int64{id, start, pid}
		return nil, nil
	}
	a := editTestApp()
	a.playlists[5] = &netease.Playlist{ID: 5, Tracks: []netease.Track{
		{ID: 7, Name: "VIP song", Fee: 1, Privilege: &netease.Privilege{ID: 7, Pl: 0}},
	}}
	a.route = route{page: pageFM}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("开启心动模式"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requested, "heart mode recommendations"); got != [3]int64{7, 7, 5} {
		t.Fatalf("heart mode requested: %v", got)
	}
}
