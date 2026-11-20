package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A finger landing on a still-gliding list caught the fling, but stayed
// swallowed after it passed the touch slop: every second swipe of a
// playlist did nothing at all (MyGo's touchPan swallowed the gesture it
// should have handed to the pan after the slop).
func TestPlaylistCatchFlingThenScroll(t *testing.T) {
	a := newTestApp()
	a.phone = true
	tracks := make([]netease.Track, 400)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: "song", Duration: 200000}
	}
	a.playlists[7] = &netease.Playlist{ID: 7, Name: "big", Tracks: tracks, TrackOffset: 400}
	a.route = route{page: pagePlaylist, id: 7}
	tt := ui.NewTester(a.view, 390, 844)
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("no touch input")
	}
	tt.Frame()
	x, y := float32(195), float32(600)
	fling := func() (end float32) {
		touch.FingerDown(x, y)
		for step := 1; step <= 5; step++ {
			tt.Advance(10 * time.Millisecond)
			touch.FingerMove(x, y-float32(step)*100)
		}
		touch.FingerUp(x, y-500)
		tt.Advance(2 * time.Second)
		tt.Frame()
		return a.ps().Y
	}
	first := fling()
	if first < 1000 {
		t.Fatalf("the first fling travelled %.0f", first)
	}
	second := fling()
	if second <= first {
		t.Fatalf("the swipe that caught the fling was swallowed: %.0f then %.0f", first, second)
	}
}

// An edge pull draws the page it returns to under the one leaving: the
// pages' scroll states used to be shared, so the under page's fresh list
// wrote its own offset over the dragged page's and the dragged page
// jumped to its top mid-gesture.
func TestPhoneEdgePullKeepsPageScroll(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab = true, pageHome
	tracks := make([]netease.Track, 100)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: "song", Duration: 200000}
	}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单甲", Tracks: tracks, TrackOffset: 100}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	a.ls().ScrollTo(30, ui.Start)
	tt.Frame()
	top := a.ps().Y
	if top <= 0 {
		t.Fatal("the list did not scroll")
	}
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("no touch input")
	}
	touch.FingerDown(10, 400)
	touch.FingerMove(120, 400)
	tt.Frame()
	if a.ps().Y != top {
		t.Fatalf("the under page moved the dragged page's scroll: %.0f→%.0f", top, a.ps().Y)
	}
	touch.FingerUp(120, 400)
	tt.Advance(time.Second)
	tt.Frame()
	if a.route.page != pagePlaylist {
		t.Fatalf("a half pull popped to %v", a.route)
	}
}
