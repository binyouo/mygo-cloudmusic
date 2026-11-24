package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The five bar patterns of Apple's WaveformPlayIndicator, decoded from
// MediaCoreUI's BouncyBars.caar: bounds.size.height keyframes on a 2.66 s
// loop of cubic segments, twelve keys each (0–100 of the view's height),
// every pattern closing on its first value.
const bouncyPeriod = 2.66 * float64(time.Second)

var bouncyBars = [5][12]float32{
	{49, 51, 42.9, 78.9, 65.5, 31, 47.3, 51, 53.2, 68.8, 70.9, 49},
	{77, 48, 77, 55.2, 45.1, 46.1, 88.1, 70, 90.7, 53.4, 51.9, 77},
	{60, 68, 58, 82.9, 68, 79, 61.4, 88, 57.1, 82.9, 62, 60},
	{33, 58.9, 68, 46.9, 93.7, 59.9, 71.7, 52, 68.8, 44, 73, 33},
	{40, 24, 48.9, 57, 42.1, 33.9, 25, 72, 29, 58.8, 50.1, 40},
}

// bouncyTimes are Apple's keyTimes over the loop.
var bouncyTimes = [12]float64{0, .091, .182, .273, .364, .455, .545, .636, .727, .818, .909, 1}

// bouncyAt samples pattern i at t seconds into the loop, interpolating
// the keys with a Catmull-Rom spline, as Core Animation's cubic mode does.
func bouncyAt(i int, t float64) float32 {
	p := &bouncyBars[i]
	pos := math.Mod(t, bouncyPeriod) / bouncyPeriod
	k := 0
	for k+1 < len(bouncyTimes)-1 && pos >= bouncyTimes[k+1] {
		k++
	}
	t0, t1 := bouncyTimes[k], bouncyTimes[k+1]
	u := (pos - t0) / (t1 - t0)
	y0, y1 := p[k], p[k+1]
	// Neighboring slopes, clamped at the loop's ends.
	prev, next := p[max(k-1, 0)], p[min(k+2, len(p)-1)]
	m0 := float64(y1-prev) * float64(t1-t0) / max(1e-6, t1-bouncyTimes[max(k-1, 0)])
	m1 := float64(next-y0) * float64(t1-t0) / max(1e-6, bouncyTimes[min(k+2, len(p)-1)]-t0)
	u2, u3 := u*u, u*u*u
	v := (2*u3-3*u2+1)*float64(y0) + (u3-2*u2+u)*m0 + (-2*u3+3*u2)*float64(y1) + (u3-u2)*m1
	return float32(math.Max(0, v))
}

// bouncyClock accumulates playing time, so the bars pause and resume
// with the music as the cover's animation does.
type bouncyClock struct {
	elapsed float64
	last    time.Time
	playing bool
}

func (b *bouncyClock) advance(now time.Time, playing bool) {
	if !b.last.IsZero() && b.playing && playing {
		b.elapsed += now.Sub(b.last).Seconds()
	}
	b.last, b.playing = now, playing
}

// drawBouncyBars paints the five dancing bars in r, Apple's fallback for
// tracks without a live spectrum (WaveformState.SamplingMode.custom).
func drawBouncyBars(p *ui.Painter, r ui.Rect, elapsed float64, color ui.Color) {
	const n = 5
	gap := r.W / n * 0.38
	w := (r.W - gap*(n-1)) / n
	for i := range n {
		h := r.H * (0.18 + 0.82*bouncyAt(i, elapsed)/100)
		x := r.X + float32(i)*(w+gap)
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: w, H: h}, color, w/2)
	}
}

// nowPlayingBars dance in the row of the track playing: the live
// spectrum when it flows, Apple's recorded patterns when it does not.
func (a *app) nowPlayingBars(c *ui.Context) {
	color := a.pal.accent
	ui.Box(c).Size(18, 14).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := a.pl.Levels()
		if live && a.pl.Playing() {
			a.rowBars.step(p.Now(), lv, live)
			drawBars(p, r, &a.rowBars, 5, color, 2.5)
			p.After(time.Second / 30)
			return
		}
		a.barsClock.advance(p.Now(), a.pl.Playing())
		drawBouncyBars(p, r, a.barsClock.elapsed, color)
		if a.pl.Playing() {
			p.After(time.Second / 30)
		}
	})
}
