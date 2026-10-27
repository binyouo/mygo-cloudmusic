package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Cmd-T opens the screen saver without navigating the main player, and toggles it off again.
func TestWallpaperShortcutRestoresView(t *testing.T) {
	for _, wasOpen := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.pl.Cue([]netease.Track{{ID: -7, LocalPath: "test.mp3", Name: "播放测试", Duration: 200000}})
		a.pl.SetLyrics(nil)
		a.nowPlaying, a.npPane = wasOpen, paneQueue
		a.route = route{page: pageLocal}
		tester := ui.NewTester(a.view, 1180, 780)
		tester.Key(ui.Cmd, ui.KeyT)
		tester.Advance(time.Second)
		tester.Frame()
		if !a.wallpaper.active || a.nowPlaying != wasOpen || a.npPane != paneQueue {
			t.Fatal("screen saver changed the underlying player view")
		}
		tester.Key(ui.Cmd, ui.KeyT)
		tester.Advance(time.Second)
		tester.Frame()
		if a.wallpaper.active || a.nowPlaying != wasOpen || a.npPane != paneQueue || a.route.page != pageLocal {
			t.Fatalf("view did not restore: active=%v open=%v pane=%d route=%+v", a.wallpaper.active, a.nowPlaying, a.npPane, a.route)
		}
	}
}

// A waking key is consumed without pausing or seeking, while key releases do not dismiss a new screen saver.
func TestWallpaperWakingInput(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]netease.Track{{ID: -7, LocalPath: "test.mp3", Duration: 200000}})
	a.pl.Hold(30 * time.Second)
	a.toggleWallpaper()
	playing, position := a.pl.Playing(), a.pl.Position()
	if a.wakeWallpaper(false, false) || !a.wallpaper.active {
		t.Fatal("shortcut release dismissed the screen saver")
	}
	if !a.wakeWallpaper(true, false) || a.wallpaper.active {
		t.Fatal("waking key was not consumed")
	}
	if a.pl.Playing() != playing || a.pl.Position() != position {
		t.Fatal("waking input changed playback")
	}
}

// Pointer jitter during entry is ignored, but deliberate movement in any direction wakes the screen saver.
func TestWallpaperPointerThreshold(t *testing.T) {
	origin := mygo.Point{X: 100, Y: 100}
	for _, test := range []struct {
		point mygo.Point
		wake  bool
	}{
		{mygo.Point{X: 101, Y: 99}, false},
		{mygo.Point{X: 104, Y: 104}, false},
		{mygo.Point{X: 106, Y: 100}, true},
		{mygo.Point{X: 95, Y: 96}, true},
	} {
		if wallpaperPointerMoved(origin, test.point) != test.wake {
			t.Fatalf("wrong wake result at %+v", test.point)
		}
	}
}

// Escape exits the wallpaper and resets its idle countdown instead of immediately reopening it.
func TestWallpaperEscape(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: -7, LocalPath: "test.mp3", Duration: 200000}})
	a.pl.SetLyrics(nil)
	tester := ui.NewTester(a.view, 1180, 780)
	tester.Key(ui.Cmd, ui.KeyT)
	tester.Advance(time.Second)
	tester.Frame()
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if a.wallpaper.active || a.nowPlaying || a.wallpaper.lastInput.IsZero() {
		t.Fatal("Escape did not restore the page and reset idle")
	}
}

// Idle entry requires uninterrupted foreground playback and never runs while a dialog is open.
func TestWallpaperIdleDeadline(t *testing.T) {
	start := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name                      string
		elapsed                   time.Duration
		seconds                   int
		focused, playing, blocked bool
		want                      time.Duration
		enabled                   bool
	}{
		{"waiting", 25 * time.Second, 30, true, true, false, 5 * time.Second, true},
		{"due", 30 * time.Second, 30, true, true, false, 0, true},
		{"overdue", time.Minute, 30, true, true, false, 0, true},
		{"disabled", time.Minute, 0, true, true, false, 0, false},
		{"background", time.Minute, 30, false, true, false, 0, false},
		{"paused", time.Minute, 30, true, false, false, 0, false},
		{"dialog", time.Minute, 30, true, true, true, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			wait, enabled := wallpaperWait(start.Add(test.elapsed), start, test.seconds, test.focused, test.playing, test.blocked)
			if wait != test.want || enabled != test.enabled {
				t.Fatalf("got %v, %v; want %v, %v", wait, enabled, test.want, test.enabled)
			}
		})
	}
	if wait, enabled := wallpaperWait(start.Add(time.Minute), start.Add(55*time.Second), 30, true, true, false); !enabled || wait != 25*time.Second {
		t.Fatalf("input did not restart countdown: %v, %v", wait, enabled)
	}
}

// Requesting wallpaper without a current track leaves navigation and full screen untouched.
func TestWallpaperWithoutTrack(t *testing.T) {
	a := newTestApp()
	a.toggleWallpaper()
	if a.wallpaper.active || a.nowPlaying {
		t.Fatal("wallpaper opened without a track")
	}
}

// Continuous playback keeps the inactivity deadline across song transitions and other player updates.
func TestWallpaperPlaybackKeepsIdleDeadline(t *testing.T) {
	a := newTestApp()
	a.pl.Hold(time.Second)
	last := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	a.wallpaper.playing, a.wallpaper.lastInput = true, last
	a.wallpaperPlaybackChanged()
	if a.wallpaper.lastInput != last {
		t.Fatal("player update restarted the inactivity timer")
	}
	a.wallpaper.playing = false
	a.wallpaperPlaybackChanged()
	if !a.wallpaper.playing || a.wallpaper.lastInput == last {
		t.Fatal("resuming playback did not restart the idle countdown")
	}
}
