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
