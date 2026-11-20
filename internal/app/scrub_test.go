package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Dragging the progress bar seeks once, where it is let go: not as the
// pointer moves, even off the thin bar, and not to the end, which would
// play the next song, before it is let go.
func TestScrubSeeksOnRelease(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 7, Name: "宝船", Duration: 200000}
	a.pl.Cue([]netease.Track{tr, {ID: 8, Name: "下一首"}})
	a.pl.SetLyrics(nil)
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress bar")
	}
	y := bar.Y + bar.H/2
	at := func(f float32) float32 { return bar.X + bar.W*f }

	tt.Press(at(0.25), y)
	tt.Frame()
	tt.Move(at(0.6), y-40) // off the bar, above it
	tt.Frame()
	tt.Move(at(1)+30, y+30) // past its end, below it
	tt.Frame()
	if pos := a.pl.Position(); pos > 10*time.Second {
		t.Fatalf("seeked to %v while dragging", pos)
	}
	if a.pl.Current().ID != 7 {
		t.Fatal("went to the next song while dragging")
	}
	tt.Move(at(0.6), y-40)
	tt.Frame()
	tt.Release(at(0.6), y-40)
	tt.Frame()
	tt.Frame()
	if pos := a.pl.Position(); pos < 110*time.Second || pos > 130*time.Second {
		t.Errorf("let go at 60%%, at %v", pos)
	}
}

// TestPhoneScrubCancel leaves the position unchanged when iOS takes a finger drag away.
func TestPhoneScrubCancel(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.Hold(20 * time.Second)
	tt := ui.NewTester(a.view, 390, 780)
	finger, ok := any(tt).(interface {
		FingerDown(x, y float32)
		FingerMove(x, y float32)
		FingerUp(x, y float32)
		FingerCancel()
	})
	if !ok {
		t.Skip("desktop MyGo snapshot has no touch tester")
	}
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("no progress bar")
	}
	y := bar.Y + bar.H/2
	at := func(relative float32) float32 { return bar.X + bar.W*relative }

	finger.FingerDown(at(0.1), y)
	finger.FingerMove(at(0.6), y)
	if pos := a.pl.Position(); pos != 20*time.Second {
		t.Fatalf("position during touch drag = %v; want 20s", pos)
	}
	finger.FingerCancel()
	if pos := a.pl.Position(); pos != 20*time.Second {
		t.Fatalf("cancelled touch moved to %v; want 20s", pos)
	}

	finger.FingerDown(at(0.1), y)
	finger.FingerMove(at(0.6), y)
	finger.FingerUp(at(0.6), y)
	if pos := a.pl.Position(); pos < 110*time.Second || pos > 130*time.Second {
		t.Fatalf("released touch moved to %v; want about 120s", pos)
	}
}

// Moving up from the progress bar keeps the sheet open and makes each horizontal step seek by less.
func TestPhoneScrubFineAdjustment(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.Hold(20 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	finger, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("desktop MyGo snapshot has no touch tester")
	}
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("the progress slider is missing")
	}
	y := bar.Y + bar.H/2
	x := func(f float32) float32 { return bar.X + bar.W*f }

	finger.FingerDown(x(0.2), y)
	finger.FingerMove(x(0.3), y)
	finger.FingerMove(x(0.4), y-50)
	finger.FingerMove(x(0.5), y-95)
	finger.FingerMove(x(0.6), y-140)
	if a.pl.Position() != 20*time.Second || a.phonePull.Y != 0 {
		t.Fatalf("preview moved playback to %v or pulled the sheet to %v", a.pl.Position(), a.phonePull)
	}
	finger.FingerUp(x(0.6), y-140)
	if pos := a.pl.Position(); pos < 77*time.Second || pos > 78*time.Second || !a.nowPlaying {
		t.Fatalf("fine scrub ended at %v with player open=%v, want 77.5s and an open player", pos, a.nowPlaying)
	}
}
