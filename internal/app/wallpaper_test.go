package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
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

// Mouse movement, scrolling and a single click keep the wallpaper open; a double click on its background closes it.
func TestWallpaperMouseInteraction(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: -7, LocalPath: "test.mp3", Name: "播放测试", Duration: 200000}})
	a.pl.SetLyrics([]lyrics.Line{{At: 10 * time.Second, Text: "测试歌词"}})
	a.toggleWallpaper()
	tester := ui.NewTester(a.wallpaperView, 1180, 780)
	tester.Frame()
	tester.Move(30, 30)
	tester.Scroll(900, 400, 0, 30)
	tester.ClickAt(30, 30)
	tester.Frame()
	if !a.wallpaper.active {
		t.Fatal("mouse movement, scroll or a single click dismissed the wallpaper")
	}
	if err := tester.Click("测试歌词"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.pl.Position() != 10*time.Second {
		t.Fatalf("lyric click: active %v, position %v", a.wallpaper.active, a.pl.Position())
	}
	if err := tester.Click("测试歌词"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active {
		t.Fatal("double click on an interactive lyric dismissed the wallpaper")
	}
	tester.ClickAt(30, 30)
	tester.ClickAt(30, 30)
	tester.Frame()
	if a.wallpaper.active {
		t.Fatal("double click on the background did not close the wallpaper")
	}
}

// Playback controls change tracks and pause without leaving the wallpaper.
func TestWallpaperPlaybackControls(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{
		{ID: -1, Name: "第一首", LocalPath: "first.mp3", Duration: 200000},
		{ID: -2, Name: "第二首", LocalPath: "second.mp3", Duration: 200000},
	})
	a.pl.SetLyrics(nil)
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tester := ui.NewTester(a.wallpaperView, 1180, 780)
	tester.Frame()
	if err := tester.Click("下一首"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.pl.Current().ID != -2 {
		t.Fatalf("next: active %v, track %+v", a.wallpaper.active, a.pl.Current())
	}
	if err := tester.Click("播放/暂停"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.pl.Playing() {
		t.Fatal("pause did not keep the wallpaper open")
	}
	if err := tester.Click("上一首"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.pl.Current().ID != -1 {
		t.Fatal("previous did not play the first track")
	}
}

// Comments open for the current track and follow a track change without leaving the wallpaper.
func TestWallpaperCommentsFollowTrack(t *testing.T) {
	asked := make(chan string, 1)
	old := threadComments
	threadComments = func(thread string, offset, limit int) (*netease.Comments, error) {
		asked <- thread
		return &netease.Comments{}, nil
	}
	defer func() { threadComments = old }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "第一首", Duration: 200000}})
	a.pl.SetLyrics([]lyrics.Line{{Text: "歌词内容"}})
	a.comments = commentState{track: 7, thread: "R_SO_4_7", loaded: true, total: 1, latest: []netease.Comment{{ID: 1, Content: "评论内容"}}}
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tester := ui.NewTester(a.wallpaperView, 1180, 780)
	tester.Frame()
	if err := tester.Click("评论"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.npPane != paneComments || !tester.HasText("评论内容") {
		t.Fatal("comments did not open for the current track")
	}
	a.pl.Cue([]netease.Track{{ID: 8, Name: "第二首", Duration: 200000}})
	tester.Frame()
	if thread := wait(t, asked, "the next track's comments"); thread != "R_SO_4_8" || a.comments.track != 8 {
		t.Fatalf("asked for %s, comment track %d", thread, a.comments.track)
	}
	if err := tester.Click("歌词"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if !a.wallpaper.active || a.npPane != paneLyrics {
		t.Fatal("returning to lyrics closed the wallpaper")
	}
}

// Fullscreen keeps Now Playing's tabs and keyboard shortcuts instead of dismissing on a key press.
func TestWallpaperKeepsNowPlayingTabs(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "测试歌曲", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.comments = commentState{track: 7, thread: "R_SO_4_7", loaded: true}
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tester := ui.NewTester(a.wallpaperView, 1180, 780)
	tester.Move(500, 200)
	tester.Advance(time.Second)
	tester.Frame()
	for _, tab := range []struct {
		key  ui.Key
		pane int
	}{{ui.Key2, paneStage}, {ui.Key3, paneQueue}, {ui.Key4, paneWall}, {ui.Key5, paneComments}, {ui.Key1, paneLyrics}} {
		tester.Key(ui.Cmd, tab.key)
		tester.Frame()
		if !a.wallpaper.active || a.npPane != tab.pane {
			t.Fatalf("key %v: active %v, pane %d", tab.key, a.wallpaper.active, a.npPane)
		}
	}
	for _, tab := range []struct {
		label string
		pane  int
	}{{"律动", paneStage}, {"待播清单", paneQueue}, {"选歌", paneWall}, {"评论", paneComments}, {"歌词", paneLyrics}} {
		if err := tester.Click(tab.label); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
		if !a.wallpaper.active || a.npPane != tab.pane {
			t.Fatalf("clicked %s: active %v, pane %d", tab.label, a.wallpaper.active, a.npPane)
		}
	}
	tester.Key(0, ui.KeyA)
	tester.Frame()
	if !a.wallpaper.active {
		t.Fatal("ordinary key dismissed the fullscreen player")
	}
}

// The pane chosen in fullscreen remains selected when returning to Now Playing.
func TestWallpaperKeepsPaneAfterExit(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: -1, Name: "测试", LocalPath: "test.mp3", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	main := ui.NewTester(a.view, 1180, 780)
	main.Frame()
	main.Key(ui.Cmd, ui.KeyT)
	main.Frame()
	a.wallpaper.revealed = true
	full := ui.NewTester(a.wallpaperView, 1440, 900)
	full.Frame()
	full.Key(ui.Cmd, ui.Key2)
	full.Frame()
	main.Frame() // The covered window must not rebuild the same player controls.
	full.Frame()
	if !a.wallpaper.active || a.npPane != paneStage {
		t.Fatal("fullscreen did not show the selected pane")
	}
	full.Key(0, ui.KeyEscape)
	full.Frame()
	main.Frame()
	if a.wallpaper.active || !a.nowPlaying || a.npPane != paneStage {
		t.Fatal("leaving fullscreen lost the selected pane")
	}
}

// The main window must keep Now Playing covered during the fullscreen fade; it previously flashed the underlying page.
func TestWallpaperExitKeepsNowPlayingCovered(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: -1, Name: "测试", LocalPath: "test.mp3", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tester := ui.NewTester(a.view, 1200, 800)
	tester.SetDark(false)
	tester.Frame()
	tester.Advance(time.Second)
	tester.Frame()
	a.wallpaper.active = true
	tester.Frame()
	// The native window is fading out, but has not yet completed its close.
	a.wallpaper.active, a.wallpaper.closing = false, true
	for _, closing := range []bool{true, false} {
		a.wallpaper.closing = closing
		tester.Frame()
		img := tester.Image()
		x := img.Bounds().Dx() / 12
		y := img.Bounds().Dy() / 2
		r, g, b, _ := img.At(x, y).RGBA()
		if (r+g+b)/3 >= 0x9000 {
			t.Fatalf("the page and sidebar showed through: closing=%v", closing)
		}
	}
}

// Fullscreen opened while Now Playing is closing must return to the page without showing the player again.
func TestWallpaperExitKeepsPageDuringPlayerClose(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: -1, Name: "测试", LocalPath: "test.mp3", Duration: 200000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tester := ui.NewTester(a.view, 1200, 800)
	tester.Frame()
	tester.Advance(time.Second)
	tester.Frame()
	a.nowPlaying = false
	tester.Frame()
	a.wallpaper.active = true
	tester.Frame()
	a.wallpaper.active, a.wallpaper.closing = false, true
	tester.Frame()
	if tester.HasText("待播清单") {
		t.Fatal("the closing player appeared over the page during fullscreen exit")
	}
}

// Double-clicking empty space closes fullscreen from both the stage and the cover wall.
func TestWallpaperBackgroundDoubleClickInPanes(t *testing.T) {
	for _, pane := range []int{paneStage, paneWall} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.pl.Cue([]netease.Track{{ID: -1, Name: "测试", LocalPath: "test.mp3", Duration: 200000}})
		a.pl.SetLyrics(nil)
		a.npPane = pane
		a.toggleWallpaper()
		a.wallpaper.revealed = true
		tester := ui.NewTester(a.wallpaperView, 1180, 780)
		tester.Advance(time.Second)
		tester.Frame()
		tester.ClickAt(20, 500)
		tester.ClickAt(20, 500)
		tester.Frame()
		if a.wallpaper.active {
			t.Fatalf("double click did not close pane %d", pane)
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

// Physical input restarts idle entry without a UI frame; a fresh resume still gets its full wait.
func TestWallpaperSystemIdleDeadline(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name               string
		baseline, physical time.Duration
		want               time.Duration
	}{
		{"due without redraw", 3 * time.Minute, 3 * time.Minute, 0},
		{"recent physical input", 3 * time.Minute, 5 * time.Second, 115 * time.Second},
		{"fresh resume", 10 * time.Second, 3 * time.Minute, 110 * time.Second},
		{"unavailable native clock", 3 * time.Minute, -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wait, enabled := wallpaperIdleWait(now, now.Add(-tc.baseline), tc.physical, 120, true, true, false)
			if !enabled || wait != tc.want {
				t.Fatalf("wait=%v enabled=%v, want %v", wait, enabled, tc.want)
			}
		})
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
