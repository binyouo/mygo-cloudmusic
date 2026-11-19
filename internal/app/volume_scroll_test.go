package app

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Scrolling either volume control changes the app's volume and clamps it;
// wheel distances from Windows used to turn it by a quarter or more.
func TestVolumeControlsScroll(t *testing.T) {
	for _, control := range []string{"button", "slider"} {
		t.Run(control, func(t *testing.T) {
			a := newTestApp()
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Center().Children(func() {
					if control == "button" {
						a.volumeButton(c, white)
					} else {
						a.nativeVolumeSlider(c, popoverColors(false))
					}
				})
			}, 300, 300)
			for _, tc := range []struct {
				name    string
				start   float64
				dx, dy  float32
				precise bool
				want    float64
			}{
				{"wheel down 3 lines", 0.5, 0, 99, false, 0.47},
				{"wheel down 4 lines", 0.5, 0, 132, false, 0.47},
				{"wheel up", 0.5, 0, -132, false, 0.53},
				{"horizontal wheel", 0.5, 40, 0, false, 0.5},
				{"touchpad", 0.5, 0, 40, true, 0.4},
				{"horizontal touchpad", 0.5, 40, 0, true, 0.5},
				{"lower bound", 0.01, 0, 99, false, 0},
				{"upper bound", 0.99, 0, -99, false, 1},
			} {
				a.volume = tc.start
				tt.Frame()
				r, ok := tt.Find("音量")
				if !ok {
					t.Fatal("no volume control")
				}
				x, y := r.X+r.W/2, r.Y+r.H/2
				if tc.precise {
					tt.Scroll(x, y, tc.dx, tc.dy)
				} else {
					tt.Wheel(x, y, tc.dx, tc.dy)
				}
				if math.Abs(a.volume-tc.want) > 1e-9 {
					t.Errorf("%s: volume = %v, want %v", tc.name, a.volume, tc.want)
				}
			}
		})
	}
}
