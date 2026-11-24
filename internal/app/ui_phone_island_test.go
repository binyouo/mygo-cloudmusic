package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Tapping the island's capsule opens its mini controls, whose transport
// buttons drive playback and whose cover opens the full player; tapping
// the capsule's empty space closes it again.
func TestPhoneIsland(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	tr := netease.Track{ID: 7, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	insets, ok := any(tt).(interface {
		SetInsets(float32, float32, float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no mobile insets")
	}
	insets.SetInsets(59, 0, 34, 0)
	tt.Frame()

	capsule, found := tt.Find("播放灵动岛")
	if !found {
		t.Fatal("no island with a track playing")
	}
	if capsule.Y > 30 || capsule.Y+capsule.H < 40 || capsule.W < 150 || capsule.W > 260 {
		t.Fatalf("compact island misplaced: %+v", capsule)
	}
	if center := capsule.X + capsule.W/2; center < 180 || center > 210 {
		t.Fatalf("compact island off center: %+v", capsule)
	}

	tapPhone(t, tt, "播放灵动岛")
	tt.Frame()
	if !a.phoneIslandOpen {
		t.Fatal("tapping the capsule did not open it")
	}
	if _, found := tt.Find("灵动岛：打开正在播放"); !found {
		t.Fatal("the expanded island has no way into Now Playing")
	}
	was := a.pl.Playing()
	tapPhone(t, tt, "灵动岛播放/暂停")
	tt.Frame()
	if a.pl.Playing() == was {
		t.Fatal("the island's play button did not toggle playback")
	}

	// A tap on the capsule's padding, away from its controls, closes it.
	capsule, found = tt.Find("播放灵动岛")
	if !found {
		t.Fatal("the open island lost its capsule")
	}
	tt.ClickAt(capsule.X+capsule.W/2, capsule.Y+4)
	tt.Frame()
	if a.phoneIslandOpen {
		t.Fatal("tapping the capsule's empty space did not close it")
	}

	tapPhone(t, tt, "播放灵动岛")
	tt.Frame()
	tapPhone(t, tt, "灵动岛：打开正在播放")
	tt.Frame()
	if !a.nowPlaying || a.phoneIslandOpen {
		t.Fatal("the island's cover did not open Now Playing")
	}
}

// The capsule takes its place from the screen's cutout: grown from a
// Dynamic Island's pill or a notch's edge, floating under the bar when
// there is neither, and it keeps away in landscape.
func TestPhoneIslandPlacement(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		w, h                     float32
		top, right, bottom, left float32
		minY, maxY               float32
		hidden                   bool
	}{
		{"dynamic island", 390, 780, 59, 0, 34, 0, 0, 20, false},
		{"notch", 390, 780, 47, 0, 34, 0, 0, 12, false},
		{"no cutout", 390, 780, 20, 0, 0, 0, 40, 110, false},
		{"landscape", 780, 390, 0, 59, 0, 59, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp()
			a.phone = true
			a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
			tt := ui.NewTester(a.view, int(tc.w), int(tc.h))
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			insets, ok := any(tt).(interface {
				SetInsets(float32, float32, float32, float32)
			})
			if !ok {
				t.Skip("the desktop MyGo snapshot has no mobile insets")
			}
			insets.SetInsets(tc.top, tc.right, tc.bottom, tc.left)
			tt.Frame()
			box, found := tt.Find("播放灵动岛")
			if tc.hidden {
				if found {
					t.Fatalf("island showed in landscape: %+v", box)
				}
				return
			}
			if !found {
				t.Fatal("no island")
			}
			if box.Y < tc.minY || box.Y > tc.maxY {
				t.Fatalf("island at y=%v, want within %v..%v", box.Y, tc.minY, tc.maxY)
			}
			if box.W < 190 || box.W > 220 {
				t.Fatalf("compact island width %v, want about 204", box.W)
			}
		})
	}
}

// With nothing playing the capsule stays away.
func TestPhoneIslandHiddenWithoutTrack(t *testing.T) {
	a := newTestApp()
	a.phone = true
	tt := ui.NewTester(a.view, 390, 780)
	tt.Frame()
	if _, found := tt.Find("播放灵动岛"); found {
		t.Fatal("island showed with no track")
	}
}

// With NM_FRAMES set, saves the capsule resting on a Dynamic Island and
// opened into its mini controls, for visual review.
func TestPhoneIslandFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	tr := netease.Track{ID: 7, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000,
		Album: netease.Album{PicURL: "https://p1.music.126.net/fictional/600y600.jpg"}}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	insets, ok := any(tt).(interface {
		SetInsets(float32, float32, float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no mobile insets")
	}
	insets.SetInsets(59, 0, 34, 0)
	tt.Advance(time.Second)
	tt.Frame()
	for _, open := range []bool{false, true} {
		a.phoneIslandOpen = open
		tt.Frame()
		name := "phone-island-collapsed"
		if open {
			name = "phone-island-expanded"
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
