package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The clarity button above the title toggles and saves lyric blur, supports keyboard input, and leaves lyric seeking usable.
func TestLyricBlurSwitch(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]netease.Track{{ID: 1, Name: "歌曲", Duration: 200000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句\n[00:10]第二句\n[00:20]第三句", "", ""))
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Move(1000, 160)
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	for _, want := range []bool{false, true} {
		if !want {
			if err := tt.Click("歌词模糊"); err != nil {
				t.Fatal(err)
			}
		} else {
			playing := a.pl.Playing()
			tt.Key(0, ui.KeySpace)
			if a.pl.Playing() != playing {
				t.Fatal("Space on the switch toggled playback")
			}
		}
		tt.Frame()
		data, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		if a.settings.LyricsBlur != want || parseSettings(data).LyricsBlur != want {
			t.Fatalf("blur did not persist as %v", want)
		}
	}
	if err := tt.Click("第二句"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Position() != 10*time.Second {
		t.Fatalf("lyric click sought to %v", a.pl.Position())
	}
}

// NM_FRAMES saves the lyric blur control and its effect in both positions for visual inspection.
func TestLyricBlurFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect lyric blur")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "Good Day Will Arrive", Artists: []netease.Artist{{Name: "ear"}}, Duration: 200000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]I'm moving out\n[00:10]Before the first\n[00:20]And when we learn to walk\n[00:30]Without stepping on each other\n[00:40]And I'm looking up\n[00:50]These fluorescent lights", "", ""))
	a.pl.Hold(15 * time.Second)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Move(1000, 160)
	for _, on := range []bool{true, false} {
		a.settings.LyricsBlur = on
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("lyric-blur-%v.png", on)))
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
