package app

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"testing"
	"time"
)

// Animated WebP covers must retain every frame instead of silently flattening to a still image.
func TestAnimatedWebPPlayback(t *testing.T) {
	data, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	a := decodeCoverAnimation(data)
	if a == nil || len(a.frames) < 2 || a.memory > animationBudget {
		t.Fatalf("animation=%+v", a)
	}
	now := time.Unix(1, 0)
	a.step(now, true)
	index, _ := a.step(now.Add(a.ends[0]), true)
	if index != 1 {
		t.Fatalf("first frame never advanced: %d", index)
	}
}

// Partial GIF frames must clear disposed pixels and leave the rest of the previous canvas intact.
func TestAnimatedGIFDisposal(t *testing.T) {
	palette := color.Palette{color.Transparent, color.RGBA{R: 255, A: 255}, color.RGBA{B: 255, A: 255}}
	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	first.SetColorIndex(0, 0, 1)
	second := image.NewPaletted(image.Rect(1, 0, 2, 1), palette)
	second.SetColorIndex(1, 0, 2)
	g := &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 10}, Disposal: []byte{gif.DisposalBackground, gif.DisposalNone}, LoopCount: 0, Config: image.Config{ColorModel: palette, Width: 2, Height: 1}}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	a := decodeCoverAnimation(b.Bytes())
	if a == nil {
		t.Fatal("no GIF animation")
	}
	if a.frames[0].RGBAAt(0, 0).R != 255 || a.frames[1].RGBAAt(0, 0).A != 0 || a.frames[1].RGBAAt(1, 0).B != 255 {
		t.Fatal("partial frames were not composited with disposal")
	}
}

// Pausing an animated cover freezes its clock; resuming must not jump over time spent paused.
func TestAnimatedCoverPause(t *testing.T) {
	a := &coverAnimation{frames: []*image.RGBA{nil, nil}, ends: []time.Duration{time.Second, 2 * time.Second}}
	now := time.Unix(1, 0)
	a.step(now, true)
	a.step(now.Add(500*time.Millisecond), true)
	a.step(now.Add(500*time.Millisecond), false)
	a.step(now.Add(time.Hour), false)
	a.step(now.Add(2*time.Hour), true)
	if a.elapsed != 500*time.Millisecond {
		t.Fatalf("paused clock advanced: %v", a.elapsed)
	}
	index, _ := a.step(now.Add(2*time.Hour+600*time.Millisecond), true)
	if index != 1 {
		t.Fatal("cover did not resume from its frozen position")
	}
}

// Late artwork and rapid skips must keep the last ready cover, never flash an empty placeholder or an obsolete song.
func TestNowPlayingArtworkReadiness(t *testing.T) {
	var v nowPlayingVisual
	a, c := &artwork{}, &artwork{}
	now := time.Unix(1, 0)
	v.step(now, 1, a, false, 1, 0, false, true)
	v.step(now.Add(time.Second), 2, nil, false, 1, 0, false, true)
	if v.current != a {
		t.Fatal("discarded the cover before its replacement loaded")
	}
	v.step(now.Add(2*time.Second), 3, c, false, 1, 0, false, true)
	if v.current != c || v.before != a {
		t.Fatal("rapid skip lost its ready cover")
	}
	v.opacity(now.Add(3*time.Second), .4, true)
	if v.alpha != .4 || v.before != a {
		t.Fatal("artwork did not follow the heard mix progress")
	}
	v.opacity(now.Add(4*time.Second), 1, false)
	if v.before != nil {
		t.Fatal("retained a finished transition")
	}
}

// With the cover transition off, a new song's cover shows at once rather than dissolving in.
func TestNowPlayingArtworkCuts(t *testing.T) {
	var v nowPlayingVisual
	a, c := &artwork{}, &artwork{}
	now := time.Unix(1, 0)
	v.step(now, 1, a, false, 1, 0, false, true)
	v.step(now.Add(time.Second), 2, c, false, 1, 0, false, false)
	if v.current != c || v.before != nil || v.initial || v.alpha != 1 {
		t.Fatal("the cover did not cut to the new song")
	}
}
