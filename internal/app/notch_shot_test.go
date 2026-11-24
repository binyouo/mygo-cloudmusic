package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func TestNotchExpandedShot(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := newTestApp()
	a.settings = defaultSettings()
	n := &notch{a: a, real: true, w0: 185, h0: 32, winW: notchOpenW + 60, winH: 292}
	a.notch = n
	a.pl.Cue([]netease.Track{{ID: 7, Name: "NAINAINAI", Artists: []netease.Artist{{Name: "ATARASHII GAKKO!"}}, Duration: 193877}})
	a.pl.Hold(61 * time.Second)
	main := ui.NewTester(a.view, 1200, 800)
	main.Frame()
	n.open = true
	tt := ui.NewTester(n.view, int(n.winW), int(n.winH))
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	f, _ := os.Create(filepath.Join(dir, "notch-expanded.png"))
	png.Encode(f, tt.Image())
	f.Close()
}
