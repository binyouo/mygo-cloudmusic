package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// coverApp is Now Playing showing the second of three songs.
func coverApp(t *testing.T) (*app, *ui.Tester, ui.Rect) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}, {ID: 3, Name: "三"}})
	a.pl.Next()
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 860, 560)
	// Without the tilt and the swing, which the CPU draws slowly in depth:
	// the gestures do as they would.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	r, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("no cover")
	}
	return a, tt, r
}

// drag drags from the middle of r by dx, over a few frames, and lets go
// after hold, still.
func drag(tt *ui.Tester, r ui.Rect, dx float32, hold time.Duration) {
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	for i := 1; i <= 6; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Move(x+dx*float32(i)/6, y)
	}
	if hold > 0 {
		tt.Advance(hold)
	}
	tt.Release(x+dx, y)
	tt.Advance(time.Second)
	tt.Frame()
}

// Flung far to the left, the cover flies off for the next song; to the
// right, for the one before, however far this one has played.
func TestFlingCover(t *testing.T) {
	a, tt, r := coverApp(t)
	if a.pl.Current().ID != 2 {
		t.Fatalf("playing %d", a.pl.Current().ID)
	}
	drag(tt, r, -r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 3 {
		t.Fatalf("flung left: playing %d", id)
	}
	if r2, ok := tt.Find("专辑封面"); !ok || r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the next cover rests at %v, not %v", r2, r)
	}
	drag(tt, r, r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 2 {
		t.Errorf("flung right: playing %d", id)
	}
}

// A drag too short, let go still, springs back, the song playing on.
func TestCoverSpringsBack(t *testing.T) {
	a, tt, r := coverApp(t)
	drag(tt, r, -r.W*0.3, 300*time.Millisecond)
	if id := a.pl.Current().ID; id != 2 {
		t.Fatalf("a short drag changed the song to %d", id)
	}
	if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the cover rests at %v, not %v", r2, r)
	}
}

// Two fingers swiping across the trackpad fling it too.
func TestSwipeCover(t *testing.T) {
	a, tt, r := coverApp(t)
	x, y := r.X+r.W/2, r.Y+r.H/2
	for range 10 {
		tt.Scroll(x, y, r.W*0.06, 0)
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if id := a.pl.Current().ID; id != 3 {
		t.Errorf("swiped left: playing %d", id)
	}
}

// The switch on the cover's corner turns the dissolve between songs' covers on and off, keeps the choice, and leaves Space to itself.
func TestCoverTransitionSwitch(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]netease.Track{{ID: 1, Name: "歌曲", Duration: 200000}})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	for _, want := range []bool{false, true} {
		if !want {
			if err := tt.Click("封面过渡"); err != nil {
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
		if a.settings.CoverTransition != want || parseSettings(data).CoverTransition != want {
			t.Fatalf("cover transition did not persist as %v", want)
		}
	}
}

// Button and keyboard changes keep the cover in place while its images dissolve.
func TestCoverTargetStaysWithTheSong(t *testing.T) {
	a, tt, r := coverApp(t)
	tt.SetPreferences(ui.Preferences{})
	for _, step := range []struct {
		name string
		play func()
		id   int64
	}{{"next", a.pl.Next, 3}, {"prev", a.pl.Prev, 2}} {
		step.play()
		tt.Frame()
		tt.Advance(60 * time.Millisecond)
		tt.Frame()
		if id := a.pl.Current().ID; id != step.id {
			t.Fatalf("%s: playing %d", step.name, id)
		}
		if r2, _ := tt.Find("专辑封面"); r2.X != r.X || r2.Y != r.Y || r2.W != r.W {
			t.Errorf("%s: cover moved from %v to %v", step.name, r, r2)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
			t.Errorf("%s: the cover rests at %v, not %v", step.name, r2, r)
		}
	}
}
