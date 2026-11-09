package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// NM_FRAMES saves search popups and playback transitions for visual inspection in each pane.
func TestNonpluginFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the ported UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, dark := range []bool{false, true} {
		a := newTestApp()
		a.settings.SearchHistory = []string{"春日影", "MyGO!!!!!", "晚风"}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.SetDark(dark)
		tt.Frame()
		if err := tt.Click("搜索"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		tt.Frame()
		save(tt, fmt.Sprintf("search-history-dark%v", dark))
	}
	for _, pane := range []int{paneLyrics, paneStage, paneWall} {
		a := newTestApp()
		a.settings = defaultSettings()
		tr := netease.Track{ID: 7, Name: "春日影", Artists: []netease.Artist{{Name: "MyGO!!!!!"}}, Album: netease.Album{PicURL: "qa-cover"}, Duration: 200000}
		a.pl.Cue([]netease.Track{tr})
		a.pl.SetLyrics(nil)
		a.npPane = pane
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 600)+"#backdrop"] = art
		a.art.art["qa-cover"] = art
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		a.nowPlaying = true
		tt.Frame()
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		save(tt, fmt.Sprintf("playing-pane%d-opening", pane))
		tt.Advance(time.Second)
		tt.Frame()
		save(tt, fmt.Sprintf("playing-pane%d-open", pane))
		a.nowPlaying = false
		tt.Frame()
		tt.Advance(180 * time.Millisecond)
		tt.Frame()
		save(tt, fmt.Sprintf("playing-pane%d-closing", pane))
	}
}
