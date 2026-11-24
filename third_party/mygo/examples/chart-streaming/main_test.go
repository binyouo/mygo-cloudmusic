package main

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/chart"
)

func TestWaveformHistory(t *testing.T) {
	const samples = 15000
	s := newWaveform("raw", samples)
	start := time.Unix(1, 0)
	s.update(start)
	for _, elapsed := range []time.Duration{time.Millisecond, 17 * time.Millisecond, 16 * time.Second, 40*time.Second + time.Millisecond/2} {
		s.update(start.Add(elapsed))
		want := int64(samples) + elapsed.Milliseconds()
		if s.next != want {
			t.Fatal(s.next, want)
		}
		for k := range 3 {
			for i, x := range s.rawX {
				if s.ys[k][i] != value(int64(x), k) {
					t.Fatalf("bad ring at %v, %d/%d", elapsed, k, i)
				}
			}
		}
		if len(s.rawX) > 2*samples || s.xs[0] > 0 || s.xs[len(s.xs)-1] > 1 || s.xs[len(s.xs)-1] < .999 {
			t.Fatal("bad rolling range")
		}
	}
}

func TestWaveformIndex(t *testing.T) {
	for _, n := range []int{15000, 100000} {
		raw, indexed := newWaveform("polyline", n), newWaveform("index", n)
		start := time.Unix(1, 0)
		for _, elapsed := range []time.Duration{0, time.Millisecond, 17 * time.Millisecond, time.Duration(n)*time.Millisecond + time.Millisecond/2, 3 * time.Duration(n) * time.Millisecond} {
			now := start.Add(elapsed)
			raw.update(now)
			indexed.update(now)
			lo, hi := indexed.viewport(now)
			for k := range 3 {
				for _, width := range []int{1280, 1600, 2560} {
					wx, wy := chart.MinMax(nil, nil, raw.xs, raw.ys[k], width)
					dx, dy := indexed.index[k].Query(nil, nil, lo, hi, width)
					equal := func(a, b float32) bool { return a == b || math.IsNaN(float64(a)) && math.IsNaN(float64(b)) }
					if !slices.EqualFunc(dx, wx, equal) || !slices.EqualFunc(dy, wy, equal) {
						t.Fatalf("different modes at n=%d elapsed=%v series=%d width=%d", n, elapsed, k, width)
					}
				}
			}
		}
	}
}
