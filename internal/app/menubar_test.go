package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// Dragging the width slider changes the native lyric viewport and saves
// the choice, instead of only resizing the control in the settings view.
func TestMenuBarWidthSlider(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS menu bar setting")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.MenuBarLyrics = true
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 720)
	r, ok := tt.Find("歌词展示宽度")
	if !ok {
		t.Fatal("missing width slider")
	}
	tt.Press(r.X+r.W/2, r.Y+r.H/2)
	tt.Move(r.X+r.W*0.8, r.Y+r.H/2)
	tt.Release(r.X+r.W*0.8, r.Y+r.H/2)
	tt.Frame()
	s, _, _ := a.menuBarSnapshot()
	if s.width == 180 || s.width < 100 || s.width > 300 {
		t.Fatalf("viewport width: %v", s.width)
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.MenuBarWidth != s.width {
		t.Fatalf("saved width %v, viewport %v", saved.MenuBarWidth, s.width)
	}
}

// A long line reads to its end within the time it is sung, resting at each
// end for what is left, as NetEase's marquee does, so that the next line
// comes in with the text already at its end. Fitting lines never scroll.
// Times are supplied explicitly, independent of wall clocks.
func TestMenuBarScroll(t *testing.T) {
	const overflow, textWidth = 56.0, 200.0
	// Half the line is hidden: the scroll takes half of its 4 s, and the
	// two ends share the other half.
	const line = 4 * time.Second
	for _, tc := range []struct {
		at   time.Duration
		want float64
	}{
		{0, 0}, {time.Second, 0}, {2 * time.Second, 28},
		{3 * time.Second, 56}, {line, 56}, {line + time.Second, 56},
	} {
		if got := menuBarScrollOffset(overflow, textWidth, tc.at, line); got != tc.want {
			t.Errorf("at %v: offset %v, want %v", tc.at, got, tc.want)
		}
	}
	if got := menuBarScrollOffset(0, textWidth, 2*time.Second, line); got != 0 {
		t.Fatalf("fitting text scrolled %v", got)
	}
}

// Resuming or seeking into a long line must animate only what remains;
// offsetting a full-duration native animation made it wrap to the start.
func TestMenuBarScrollResume(t *testing.T) {
	for _, tc := range []struct {
		elapsed, wait, moving time.Duration
		offset                float64
	}{
		{-time.Second, time.Second, 2 * time.Second, 0},
		{0, time.Second, 2 * time.Second, 0},
		{time.Second, 0, 2 * time.Second, 0},
		{2 * time.Second, 0, time.Second, 50},
		{3 * time.Second, 0, 0, 100},
		{5 * time.Second, 0, 0, 100},
	} {
		wait, moving := menuBarScrollRemaining(100, 200, tc.elapsed, 4*time.Second)
		offset := menuBarScrollOffset(100, 200, tc.elapsed, 4*time.Second)
		if wait != tc.wait || moving != tc.moving || offset != tc.offset {
			t.Errorf("at %v: wait %v, moving %v, offset %v; want %v, %v, %v", tc.elapsed, wait, moving, offset, tc.wait, tc.moving, tc.offset)
		}
	}
}

// The lyric viewport keeps the width set whatever the line: sizing it to
// the line made the status item resize with every lyric, which shifted the
// items beside it along the menu bar.
func TestMenuBarWidthIsFixed(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.MenuBarLyrics, a.settings.MenuBarWidth = true, 200
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌名"}})
	lines := []lyrics.Line{
		{At: 0, Text: "短"},
		{At: time.Second, Text: "一句很长很长很长很长很长很长很长很长很长的歌词"},
		{At: 2 * time.Second, Text: ""},
	}
	a.pl.SetLyrics(lines)
	for _, pos := range []time.Duration{0, time.Second, 2 * time.Second} {
		a.pl.Hold(pos)
		if s, _, _ := a.menuBarSnapshot(); s.width != 200 {
			t.Fatalf("at %v: width %v, want the setting 200", pos, s.width)
		}
	}
}

// Switching menu bar lyrics on and off persists the listener's choice.
func TestMenuBarSettingsToggle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS menu bar setting")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 560, 720)
	for _, want := range []bool{true, false} {
		if err := tt.Click("显示菜单栏歌词"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved settings
		if err := json.Unmarshal(b, &saved); err != nil {
			t.Fatal(err)
		}
		if a.settings.MenuBarLyrics != want || saved.MenuBarLyrics != want {
			t.Fatalf("switch to %v: app %v, disk %v", want, a.settings.MenuBarLyrics, saved.MenuBarLyrics)
		}
	}
}

// The bar follows exact lyric boundaries and backwards seeks, and only
// schedules another update while playback has a future line to reach.
func TestMenuBarLyricsFollowPosition(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌名"}})
	a.pl.SetLyrics([]lyrics.Line{
		{At: 2 * time.Second, Text: "第一句"},
		{At: 5 * time.Second, Text: "第二句"},
		{At: 9 * time.Second, Text: ""},
	})
	for _, tc := range []struct {
		pos, next time.Duration
		text      string
	}{
		{0, 2 * time.Second, "歌名"},
		{2 * time.Second, 3 * time.Second, "第一句"},
		{5 * time.Second, 4 * time.Second, "第二句"},
		{3 * time.Second, 2 * time.Second, "第一句"},
		{10 * time.Second, 0, "歌名"},
	} {
		a.pl.Hold(tc.pos)
		s, _, next := a.menuBarSnapshot()
		if s.text != tc.text || next != tc.next {
			t.Errorf("at %v: text %q, next %v; want %q, %v", tc.pos, s.text, next, tc.text, tc.next)
		}
	}
}

// Seeking while paused changes the displayed line without starting a timer.
func TestMenuBarPausedSeek(t *testing.T) {
	a := newTestApp()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌名"}})
	a.pl.SetLyrics([]lyrics.Line{{At: 0, Text: "第一句"}, {At: time.Second, Text: "第二句"}})
	for _, pos := range []time.Duration{time.Second, 0} {
		a.pl.Seek(pos)
		s, _, next := a.menuBarSnapshot()
		want := "第一句"
		if pos > 0 {
			want = "第二句"
		}
		if s.text != want || s.playing || next != 0 {
			t.Fatalf("paused at %v: %+v, next %v", pos, s, next)
		}
	}
}

// A pending lyric load keeps the track title; the lyric response and
// liked-song updates replace it without depending on a playback event.
func TestMenuBarLoadedLyricsAndLikes(t *testing.T) {
	a := newTestApp()
	if s, _, next := a.menuBarSnapshot(); s.hasTrack || next != 0 {
		t.Fatalf("empty player: %+v, next %v", s, next)
	}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌名"}})
	if s, _, _ := a.menuBarSnapshot(); s.text != "歌名" || !s.hasTrack {
		t.Fatalf("loading lyrics: %+v", s)
	}
	a.pl.SetLyrics(nil)
	if s, _, next := a.menuBarSnapshot(); s.text == "歌名" || next != 0 {
		t.Fatalf("no lyrics: %+v, next %v", s, next)
	}
	a.pl.SetLyrics([]lyrics.Line{{Text: " 一行\n歌词 "}})
	a.liked = map[int64]bool{7: true}
	if s, _, _ := a.menuBarSnapshot(); s.text != "一行 歌词" || !s.liked {
		t.Fatalf("loaded: %+v", s)
	}
	a.setLiked(7, false)
	if s, _, _ := a.menuBarSnapshot(); s.liked {
		t.Fatal("heart did not follow unlike")
	}
}
