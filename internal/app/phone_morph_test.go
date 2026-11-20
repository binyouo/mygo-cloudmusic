package app

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// morphCover is a colorful test cover: four quadrants, red, green, blue
// and yellow, that its blurred copy turns into a lively backdrop.
func morphCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			var c color.RGBA
			switch {
			case x < 48 && y < 48:
				c = color.RGBA{235, 60, 60, 255}
			case x >= 48 && y < 48:
				c = color.RGBA{60, 200, 90, 255}
			case x < 48:
				c = color.RGBA{70, 110, 235, 255}
			default:
				c = color.RGBA{240, 200, 50, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// morphTestApp is a light-themed phone with a playing song whose cover
// is a real bitmap, its blurred copy backing the living backdrop.
func morphTestApp() (*app, *ui.Tester) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	img := morphCover()
	tr := netease.Track{ID: 7, Name: "测试歌曲", Duration: 120000,
		Album:   netease.Album{PicURL: "morph-cover.jpg"},
		Artists: []netease.Artist{{Name: "测试歌手"}}}
	key := netease.Sized(tr.Album.PicURL, 600)
	art := &artwork{bmp: ui.NewBitmap(img), tint: dominantColor(img)}
	bd := *art
	bd.backdrop = ui.NewBitmap(blurImage(img))
	a.art.art[key], a.art.art[key+"#backdrop"] = art, &bd
	a.pl.Cue([]netease.Track{tr})
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetDark(false)
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// topBright is how much of the window's top strip is light: the page
// behind the sheet in the light theme, where the sheet is dark.
func topBright(t *testing.T, tt *ui.Tester) float64 {
	t.Helper()
	img := tt.Image()
	sx, sy := img.Bounds().Dx(), img.Bounds().Dy()
	lit, total := 0, 0
	for y := float32(2); y < 90; y += 12 {
		for x := float32(20); x < 390; x += 40 {
			r, g, b, _ := img.At(int(x*float32(sx)/390), int(y*float32(sy)/844)).RGBA()
			total++
			if float64(r+g+b)/3 > 40000 {
				lit++
			}
		}
	}
	return float64(lit) / float64(total)
}

// The player opens as a sheet growing from the mini player's capsule:
// mid-way the window's top still shows the page, and settled the sheet
// covers it all; closing shrinks it back.
func TestPhonePlayerMorphCoverage(t *testing.T) {
	a, tt := morphTestApp()
	tapPhone(t, tt, "打开正在播放")
	tt.Advance(120 * time.Millisecond)
	tt.Frame()
	if got := topBright(t, tt); got < 0.5 {
		t.Fatalf("mid-opening the top of the window is not the page: %.0f%% light", got*100)
	}
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	if got := topBright(t, tt); got > 0.2 {
		t.Fatalf("settled the sheet does not cover the window's top: %.0f%% light", got*100)
	}
	a.startPhoneClose()
	tt.Frame()
	tt.Advance(120 * time.Millisecond)
	tt.Frame()
	if got := topBright(t, tt); got < 0.5 {
		t.Fatalf("mid-closing the top of the window is not the page: %.0f%% light", got*100)
	}
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	if a.nowPlaying {
		t.Fatal("the player stayed open after the morph home")
	}
}

// The real mini player stays visible as the sheet begins to open and finishes shrinking back.
func TestPhonePlayerMorphRevealsMiniPlayer(t *testing.T) {
	a, tt := morphTestApp()
	mini := a.phonePlatter
	if mini.W <= 0 || mini.H <= 0 {
		t.Fatal("the mini player has no bounds")
	}
	sample := func() float64 {
		img := tt.Image()
		sx, sy := float32(img.Bounds().Dx())/390, float32(img.Bounds().Dy())/844
		// Use the space below the title, clear of the flying artwork and buttons.
		x := int((mini.X + mini.W*0.49) * sx)
		y := int((mini.Y + mini.H*0.82) * sy)
		r, g, b, _ := img.At(x, y).RGBA()
		return float64(r+g+b) / (3 * 65535)
	}
	rest := sample()
	if rest < 0.65 {
		t.Fatalf("expected a light mini player, sampled %.2f", rest)
	}
	tapPhone(t, tt, "打开正在播放")
	tt.Frame()
	if got := sample(); got < rest-0.1 {
		t.Errorf("the first opening frame darkened the mini player from %.2f to %.2f", rest, got)
	}
	tt.Advance(time.Second)
	tt.Frame()
	a.startPhoneClose()
	tt.Frame()
	for elapsed := 16 * time.Millisecond; elapsed <= 272*time.Millisecond; elapsed += 16 * time.Millisecond {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
		if elapsed >= 208*time.Millisecond {
			if got := sample(); got < rest-0.12 {
				t.Errorf("at %v into close, the sheet still darkens the mini player from %.2f to %.2f", elapsed, rest, got)
			}
		}
	}
}

// With NM_FRAMES set, saves the living backdrop, the sheet's open and
// close morphs, and the cover's shadow in both modes, to look at.
func TestPhoneMorphFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	save := func(name string, tt *ui.Tester) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// The living cover at three moments of its slow drift.
	for _, clock := range []float32{0, 8, 20} {
		clk := clock
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				art := &artwork{backdrop: ui.NewBitmap(blurImage(morphCover()))}
				drawLivingBackdrop(p, r, art, clk)
			})
		}, 390, 844)
		tt.Frame()
		save(fmt.Sprintf("living-%ds.png", int(clock)), tt)
	}
	a, tt := morphTestApp()
	tapPhone(t, tt, "打开正在播放")
	tt.Frame()
	save("open-0ms.png", tt)
	last := time.Duration(0)
	for _, ms := range []int64{50, 100, 160, 250, 500} {
		tt.Advance(time.Duration(ms)*time.Millisecond - last)
		last = time.Duration(ms) * time.Millisecond
		tt.Frame()
		save(fmt.Sprintf("open-%dms.png", ms), tt)
	}
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save("open-settled.png", tt)
	a.startPhoneClose()
	tt.Frame()
	save("close-0ms.png", tt)
	last = 0
	for _, ms := range []int64{50, 100, 160, 250, 500} {
		tt.Advance(time.Duration(ms)*time.Millisecond - last)
		last = time.Duration(ms) * time.Millisecond
		tt.Frame()
		save(fmt.Sprintf("close-%dms.png", ms), tt)
	}
	// The cover's shadow in cover mode, then the header's in lyrics mode.
	a, tt = morphTestApp()
	tapPhone(t, tt, "打开正在播放")
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	save("cover-mode.png", tt)
	a.phoneDetails = true
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save("lyrics-header.png", tt)
}
