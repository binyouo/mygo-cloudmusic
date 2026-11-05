package app

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// Retired fixed-duration preferences migrate to AutoMix, while explicit gapless remains gapless.
func TestRetiredCrossfadePreferences(t *testing.T) {
	for _, tc := range []struct {
		name, saved string
		adaptive    bool
	}{
		{"four seconds", `{"adaptiveMix":false,"crossfadeSeconds":4}`, true},
		{"eight seconds", `{"adaptiveMix":false,"crossfadeSeconds":8}`, true},
		{"older fixed mix", `{"crossfadeSeconds":4}`, true},
		{"gapless", `{"adaptiveMix":false,"crossfadeSeconds":0}`, false},
		{"older gapless", `{"crossfadeSeconds":0}`, false},
		{"explicit AutoMix", `{"adaptiveMix":true,"crossfadeSeconds":0}`, true},
		{"current gapless", `{"adaptiveMix":false}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := parseSettings([]byte(tc.saved))
			if s.AdaptiveMix != tc.adaptive {
				t.Fatalf("AutoMix=%v, want %v", s.AdaptiveMix, tc.adaptive)
			}
			encoded, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]json.RawMessage
			if err = json.Unmarshal(encoded, &saved); err != nil {
				t.Fatal(err)
			}
			if _, exists := saved["crossfadeSeconds"]; exists {
				t.Fatal("retired fixed duration persisted again")
			}
		})
	}
}

// Changing the transition selector must reach the player and persist across a restart.
func TestSongTransitionSetting(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.AutoMix = func() bool { return a.settings.AdaptiveMix }
	tt := ui.NewTester(a.settingsView, 800, 2200)
	tt.Frame()
	if err := tt.Click("歌曲过渡方式"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.pl.AutoMix() {
		t.Fatal("gapless choice did not disable adaptive mixing")
	}
	if err := tt.Click("歌曲过渡方式"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !a.pl.AutoMix() {
		t.Fatal("adaptive choice did not reach the player")
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.AdaptiveMix {
		t.Fatal("transition was not saved")
	}
}

// NM_FRAMES renders both ready albums and the midpoint dissolve for visual inspection.
func TestMusicTransitionFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES, NM_COVER_A and NM_COVER_B for artwork review")
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.nowPlaying = true
	tracks := []netease.Track{
		{ID: 1, Name: "Cruel Summer", Artists: []netease.Artist{{Name: "Taylor Swift"}}, Duration: 178000, Album: netease.Album{PicURL: "cover-a"}},
		{ID: 2, Name: "Shape of You", Artists: []netease.Artist{{Name: "Ed Sheeran"}}, Duration: 233000, Album: netease.Album{PicURL: "cover-b"}},
	}
	for i, name := range []string{"NM_COVER_A", "NM_COVER_B"} {
		data, err := os.ReadFile(os.Getenv(name))
		if err != nil {
			t.Fatal(err)
		}
		bmp, img, err := decodeArt(data)
		if err != nil {
			t.Fatal(err)
		}
		a.art.art[netease.Sized(tracks[i].Album.PicURL, 600)+"#backdrop"] = &artwork{bmp: bmp, tint: dominantColor(img), palette: albumPalette(img), backdrop: ui.NewBitmap(blurImage(img))}
	}
	a.pl.Cue(tracks)
	a.pl.Hold(8 * time.Second)
	a.pl.SetLyrics(lyrics.Build("[00:00.00]音乐从这里继续\n[00:08.00]让每一个瞬间慢慢展开\n[00:16.00]旋律与画面一起流动\n[00:24.00]下一段旅程即将开始", "", ""))
	tt := ui.NewTester(a.view, 1100, 760)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	save := func(name string) {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	save("music-before")
	a.pl.Cue(tracks[1:])
	a.pl.Hold(8 * time.Second)
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	save("music-dissolve")
	tt.Advance(time.Second)
	tt.Frame()
	save("music-after")
}
