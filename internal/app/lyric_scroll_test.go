package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// A line comes to rest in the middle of the lyrics pane within
// lyricScrollTime of being sung, rather than still gliding there when the
// next one starts: the glide's spring took most of a second to settle, so a
// song whose lines are a second or two apart showed the lyrics behind the
// words already heard.
func TestLyricsScrollToLineInTime(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	// Lines two seconds apart, each sung over its two seconds.
	names := []string{"第一句", "第二句", "第三句", "第四句", "第五句", "第六句", "第七句", "第八句", "第九句", "第十句"}
	var lrc string
	for i, name := range names {
		lrc += fmt.Sprintf("[00:%02d.00]%s\n", 2*i, name)
	}
	lines := lyrics.Build(lrc, "", "")
	for i := range lines {
		lines[i].End = lines[i].At + 2*time.Second
	}
	a.pl.SetLyrics(lines)
	a.nowPlaying, a.npPane = true, paneLyrics
	tt := ui.NewTester(a.view, 1200, 800)
	a.pl.Hold(0)
	tt.Frame()

	// By the time a line is sung, the lyrics have come to where they stay
	// for it: no part of the glide is left to happen over the words heard.
	// The frames are a sixth of a second apart, so a glide of 220 ms has
	// the last frame or two of it left.
	for i := 2; i < 8; i++ {
		want := names[i]
		a.pl.Seek(time.Duration(2*i)*time.Second + 100*time.Millisecond)
		tt.Frame() // the frame that sees the new line start the glide
		tt.Advance(lyricScrollTime)
		tt.Frame()
		early, ok := tt.Find(want)
		if !ok {
			t.Fatalf("%q is not shown", want)
		}
		tt.Advance(500 * time.Millisecond)
		tt.Frame()
		late, ok := tt.Find(want)
		if !ok {
			t.Fatalf("%q is not shown", want)
		}
		if early.Y-late.Y > 1 || late.Y-early.Y > 1 {
			t.Fatalf("%q moves from %v to %v after it is sung", want, early.Y, late.Y)
		}
	}
}
