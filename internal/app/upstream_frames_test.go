package app

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// NM_FRAMES saves retro players in playback and pause, plus radio and tall-window layouts for visual review.
func TestUpstreamPlayerFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES for visual review")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	readyRetro()
	for _, scenario := range []struct {
		style          string
		playing, radio bool
		w, h           int
	}{
		{"vinyl", true, false, 1200, 800}, {"vinyl", false, false, 1200, 800},
		{"cassette", true, false, 1200, 800}, {"cassette", false, false, 1200, 800},
		{"", true, true, 900, 650}, {"", true, false, 900, 1400},
	} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.PlayerStyle = scenario.style
		tr := netease.Track{ID: 7, Name: "春日影", Artists: []netease.Artist{{Name: "MyGO!!!!!"}}, Duration: 180000}
		a.pl.Cue([]netease.Track{tr})
		var lines []lyrics.Line
		for i := range 30 {
			lines = append(lines, lyrics.Line{At: time.Duration(i) * 5 * time.Second, End: time.Duration(i+1) * 5 * time.Second, Text: fmt.Sprintf("第 %02d 句 悴んだ心 ふるえる眼差し", i)})
		}
		a.pl.SetLyrics(lines)
		a.pl.Seek(60 * time.Second)
		if scenario.playing {
			a.pl.Hold(60 * time.Second)
		}
		if scenario.radio {
			a.pl.AttachRadio("私人 FM", func(func([]netease.Track)) {})
		}
		a.nowPlaying = true
		tt := ui.NewTester(a.view, scenario.w, scenario.h)
		tt.SetDark(true)
		settleRetro(tt)
		tt.Move(float32(scenario.w)/2, 70)
		settleRetro(tt)
		path := filepath.Join(dir, fmt.Sprintf("%s-playing%t-radio%t-%dx%d.png", scenario.style, scenario.playing, scenario.radio, scenario.w, scenario.h))
		f, err := os.Create(path)
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
