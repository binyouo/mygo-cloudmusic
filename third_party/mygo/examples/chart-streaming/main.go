// Chart-streaming draws three rolling waveforms of 15,000 samples each,
// 1,000 new samples a second, reduced to a few points per pixel column.
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

type waveform struct {
	data             [3][]float32
	rawX             []float32
	index            [3]chart.MinMaxIndex
	samples          int
	next             int64
	xs               []float32
	ys               [3][]float32
	bucketX, bucketY [3][]float32
	paths            [3]ui.Path
	points           [3][][2]float32
	start            time.Time
	mode             string
}

func value(i int64, s int) float32 {
	x := float64(i) / 1000
	// Deterministic noise and isolated spikes, which MinMax keeps.
	n := uint64(i)*6364136223846793005 + uint64(s+1)*1442695040888963407
	v := .22*math.Sin(x*float64(s+1)*7) + .08*(float64(n>>48)/65535-.5)
	if i%997 == int64(s*31) {
		v += .22
	}
	return float32(.2 + float64(s)*.3 + v*.55)
}

func newWaveform(mode string, samples int) *waveform {
	s := &waveform{mode: mode, samples: samples, next: int64(samples), xs: make([]float32, samples)}
	s.rawX = make([]float32, samples, 2*samples)
	for i := range s.rawX {
		s.rawX[i] = float32(i)
	}
	for k := range 3 {
		s.data[k] = make([]float32, samples, 2*samples)
		for i := range samples {
			s.data[k][i] = value(int64(i), k)
		}
	}
	return s
}

// update retains at most two visible windows and indexes only new samples.
func (s *waveform) update(now time.Time) {
	samples := s.samples
	if s.start.IsZero() {
		s.start = now
	}
	elapsed := now.Sub(s.start)
	target := int64(samples) + elapsed.Milliseconds()
	// Bound retained history. Rebuild explicitly when old samples expire.
	if target-int64(s.rawX[0]) > int64(2*samples) {
		start := target - int64(samples)
		s.rawX = s.rawX[:0]
		for k := range 3 {
			s.index[k].Reset()
			s.data[k] = s.data[k][:0]
		}
		for i := start; i < target; i++ {
			s.rawX = append(s.rawX, float32(i))
			for k := range 3 {
				s.data[k] = append(s.data[k], value(i, k))
			}
		}
		s.next = target
	}
	for i := s.next; i < target; i++ {
		s.rawX = append(s.rawX, float32(i))
		for k := range 3 {
			s.data[k] = append(s.data[k], value(i, k))
		}
	}
	s.next = target
	if s.mode == "index" {
		for k := range 3 {
			s.index[k].Update(s.rawX, s.data[k])
		}
		return
	}
	lo, hi := s.viewport(now)
	s.xs = s.xs[:0]
	for _, x := range s.rawX {
		s.xs = append(s.xs, (x-lo)/(hi-lo))
	}
	for k := range 3 {
		s.ys[k] = s.data[k]
	}
}

func (s *waveform) viewport(now time.Time) (float32, float32) {
	phase := float32(now.Sub(s.start)%time.Millisecond) / float32(time.Millisecond)
	lo := float32(s.next-int64(s.samples)) + phase
	return lo, lo + float32(s.samples-1)
}

var colors = [3]ui.Color{ui.RGB(40, 140, 240), ui.RGB(240, 100, 70), ui.RGB(50, 190, 110)}

func (s *waveform) view(c *ui.Context) {
	s.update(c.Now())
	c.AnimationFrame()
	t := c.Theme()
	x := chart.NewLinear(-float64(s.samples)/1000, 0)
	y := chart.NewLinear(0, 1)
	chart.Plot(c, chart.Axes{Left: y.Ticks(5), Bottom: x.Ticks(5)}, func(area ui.Element) {
		area.Label("Three live waveforms").Description(fmt.Sprintf("%d samples at 1000 samples per second per series", s.samples)).Draw(func(p *ui.Painter, r ui.Rect) {
			for _, tick := range y.Ticks(5) {
				p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, t.Border)
			}
			p.Clip(r, 0, func() {
				for k := range 3 {
					if s.mode == "raw" {
						p.StrokePath(chart.Line(r, s.xs, s.ys[k], chart.CurveLinear), 2, colors[k])
						continue
					}
					// Keep the extremes of each pixel column, into slices
					// reused every frame.
					width := int(math.Ceil(float64(r.W * p.Scale())))
					if s.mode == "index" {
						lo, hi := s.viewport(c.Now())
						s.bucketX[k], s.bucketY[k] = s.index[k].Query(s.bucketX[k], s.bucketY[k], lo, hi, width)
					} else {
						s.bucketX[k], s.bucketY[k] = chart.MinMax(s.bucketX[k], s.bucketY[k], s.xs, s.ys[k], width)
					}
					xs, ys := s.bucketX[k], s.bucketY[k]
					if s.mode == "path" {
						p.StrokePath(chart.LineInto(&s.paths[k], r, xs, ys, chart.CurveLinear), 2, colors[k])
						continue
					}
					s.points[k] = s.points[k][:0]
					for i, x := range xs {
						s.points[k] = append(s.points[k], [2]float32{chart.X(r, x), chart.Y(r, ys[i])})
					}
					p.StrokePolyline(s.points[k], 2, colors[k])
				}
			})
		})
	}).Fill().Gap(8).Padding(8)
}

func main() {
	mode := flag.String("mode", "polyline", "how to draw: index (MinMaxIndex), polyline (MinMax), path (MinMax, StrokePath) or raw (every sample, StrokePath)")
	samples := flag.Int("samples", 15000, "samples visible per series (at least 2)")
	duration := flag.Duration("duration", 0, "quit after this duration; zero runs until closed")
	flag.Parse()
	if *mode != "polyline" && *mode != "path" && *mode != "raw" && *mode != "index" {
		log.Fatal("unknown mode ", *mode)
	}
	if *samples < 2 {
		log.Fatal("samples must be at least 2")
	}
	s := newWaveform(*mode, *samples)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Streaming chart", Width: 1360, Height: 800, Content: ui.View(s.view)})
		if *duration > 0 {
			time.AfterFunc(*duration, mygo.App.Quit)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
