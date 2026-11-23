package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The screensaver group gains a style choice and the standby card's small
// clock, kept in the settings file.
func TestStandbySettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	defer func(apply func(mygo.ThemeSource)) { setThemeSource = apply }(setThemeSource)
	setThemeSource = func(mygo.ThemeSource) {}
	tt := ui.NewTester(a.settingsView, 560, 720)
	tt.Frame()

	// The clock row is off while the style is not standby.
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, "显示小时钟"); !ok || !sw.Disabled {
		t.Fatalf("clock switch should be disabled while the style is not standby: %+v", sw)
	}

	if err := tt.Click("待机显示"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.WallpaperStyle != "standby" {
		t.Fatalf("style %q", a.settings.WallpaperStyle)
	}
	if err := tt.Click("显示小时钟"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.StandbyClock {
		t.Fatal("clock switch did not turn off")
	}

	b, err := json.Marshal(a.settings)
	if err != nil {
		t.Fatal(err)
	}
	back := parseSettings(b)
	if back.WallpaperStyle != "standby" || back.StandbyClock {
		t.Fatalf("round-trip: %+v", back)
	}
}

// The standby style is a single playback card: the lyric line seeks, the
// slim transport works, the clock follows its switch and there are no
// tabs; the default style keeps the Now Playing tabs.
func TestStandbySurface(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.WallpaperStyle = "standby"
	a.pl.Cue([]netease.Track{
		{ID: -1, Name: "第一首", LocalPath: "first.mp3", Duration: 200000},
		{ID: -2, Name: "第二首", LocalPath: "second.mp3", Duration: 200000},
	})
	a.pl.SetLyrics([]lyrics.Line{{At: 10 * time.Second, Text: "当前行"}, {At: 20 * time.Second, Text: "下一句"}})
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tt := ui.NewTester(a.wallpaperView, 1440, 900)
	tt.Frame()

	if _, ok := tt.Find("待机时钟"); !ok {
		t.Fatal("standby card has no clock")
	}
	if _, ok := tt.Find("评论"); ok {
		t.Fatal("standby card shows the Now Playing tabs")
	}
	if err := tt.Click("当前行"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.wallpaper.active || a.pl.Position() != 10*time.Second {
		t.Fatalf("lyric click: active %v, position %v", a.wallpaper.active, a.pl.Position())
	}
	if err := tt.Click("下一首"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.wallpaper.active || a.pl.Current().ID != -2 {
		t.Fatalf("next: active %v, track %+v", a.wallpaper.active, a.pl.Current())
	}
	if err := tt.Click("播放/暂停"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.wallpaper.active || a.pl.Playing() {
		t.Fatal("pause did not keep the standby card open")
	}
}

// The clock switch hides the small clock, the lyric block is omitted for
// programs, and the default style keeps the Now Playing tabs.
func TestStandbySurfaceVariants(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.WallpaperStyle = "standby"
	a.settings.StandbyClock = false
	a.pl.Cue([]netease.Track{{ID: -1, Name: "节目", LocalPath: "ep.mp3", Duration: 200000, ProgramID: 9}})
	a.pl.SetLyrics([]lyrics.Line{{At: 10 * time.Second, Text: "当前行"}})
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tt := ui.NewTester(a.wallpaperView, 1440, 900)
	tt.Frame()
	if _, ok := tt.Find("待机时钟"); ok {
		t.Fatal("clock shown while switched off")
	}
	if err := tt.Click("当前行"); err == nil {
		t.Fatal("program shows a lyric line")
	}

	b := newTestApp()
	b.settings = defaultSettings()
	b.pl.Cue([]netease.Track{{ID: -1, Name: "测试", Duration: 200000}})
	b.pl.SetLyrics(nil)
	b.comments = commentState{track: -1, loaded: true}
	b.toggleWallpaper()
	b.wallpaper.revealed = true
	bt := ui.NewTester(b.wallpaperView, 1440, 900)
	bt.Frame()
	if _, ok := bt.Find("评论"); !ok {
		t.Fatal("default style lost the Now Playing tabs")
	}
}

// NM_FRAMES saves the standby card for visual inspection.
func TestStandbyFrame(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the standby card")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.WallpaperStyle = "standby"
	a.pl.Cue([]netease.Track{{ID: -7, Name: "测试歌曲", LocalPath: "test.mp3", Duration: 200000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句歌词\n[00:10]第二句歌词\n[00:20]第三句歌词", "", ""))
	a.pl.Hold(10 * time.Second)
	a.toggleWallpaper()
	a.wallpaper.revealed = true
	tt := ui.NewTester(a.wallpaperView, 1440, 900)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	f, err := os.Create(filepath.Join(dir, "standby.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
