package player

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
)

// TestBeatGridTracksMeter checks that the decoder snaps to syncopated peaks and keeps the correct meter at both ends of a track.
func TestBeatGridTracksMeter(t *testing.T) {
	for _, tc := range []struct {
		meter, period, onset, count int
	}{{3, 31, 13, 20}, {4, 27, 11, 28}} {
		length := tc.onset + (tc.count+1)*tc.period
		beat, down := make([]float32, length), make([]float32, length)
		for i := range beat {
			beat[i], down[i] = -10, -10
		}
		var wantBeats, wantDownbeats []float64
		drift := [3]int{0, 1, -1}
		for i := range tc.count {
			frame := tc.onset + tc.period*i + drift[i%3]
			beat[frame] = 10
			time := float64(frame) / beatFPS
			wantBeats = append(wantBeats, time)
			if i%tc.meter == 0 {
				down[frame] = 8
				wantDownbeats = append(wantDownbeats, time)
			}
		}
		got, err := decodeBeatGrid(t.Context(), beat, down)
		if err != nil {
			t.Fatal(err)
		}
		if got.meter != tc.meter || !slices.Equal(got.beats, wantBeats) || !slices.Equal(got.downbeats, wantDownbeats) {
			t.Fatalf("%d/4 predicted meter %d, beats %v, downbeats %v", tc.meter, got.meter, got.beats, got.downbeats)
		}
		wantBPM := (60 * beatFPS) / float64(tc.period)
		if math.Abs(got.bpm-wantBPM) > 1 {
			t.Fatalf("%d/4 BPM = %.3f, want about %.3f", tc.meter, got.bpm, wantBPM)
		}
	}
}

// TestBeatGridCancels checks that an obsolete analysis stops before decoding its model output.
func TestBeatGridCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := decodeBeatGrid(ctx, []float32{0}, []float32{0}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Beat This analysis = %v", err)
	}
}

// TestBeatModelWindowsPreserveTimeline recovers the same grid across two overlapping windows and a short stand-alone window.
func TestBeatModelWindowsPreserveTimeline(t *testing.T) {
	const stride = beatWindow - 2*beatBorder
	for _, frames := range []int{483, stride, stride + 1, 2*stride + 1} {
		pcm := make([]float32, (frames-1)*beatHop)
		referenceBeat, referenceDown := make([]float32, frames), make([]float32, frames)
		for i := range referenceBeat {
			referenceBeat[i], referenceDown[i] = -8, -8
			if i > 0 && i%25 == 0 {
				referenceBeat[i] = 7
				if i%100 == 0 {
					referenceDown[i] = 6
				}
			}
		}
		want, err := decodeBeatGrid(t.Context(), referenceBeat, referenceDown)
		if err != nil {
			t.Fatal(err)
		}
		start := -beatBorder
		covered := 0
		got, err := analyzeBeatGrid(t.Context(), pcm, func(_ context.Context, _ []float32, count int) ([]float32, []float32, error) {
			if start != -beatBorder && start+stride >= frames-beatBorder {
				start = frames - (beatWindow - beatBorder)
			}
			beat, down := make([]float32, count), make([]float32, count)
			for i := range beat {
				at := i + start
				if at >= 0 && at < frames {
					beat[i], down[i] = referenceBeat[at], referenceDown[at]
					if at < covered {
						// The later window disagrees on its overlapping region.
						beat[i], down[i] = -10, -10
					}
				}
			}
			covered = start + count - beatBorder
			start += stride
			return beat, down, nil
		})
		if err != nil || got.meter != want.meter || !slices.Equal(got.beats, want.beats) || !slices.Equal(got.downbeats, want.downbeats) {
			t.Fatalf("frame count %d changed beat alignment: %v, %v", frames, got, err)
		}
	}
}

// TestDeviceGrooveUsesDownbeat aligns the incoming downbeat with an outgoing downbeat in a three-beat transition.
func TestDeviceGrooveUsesDownbeat(t *testing.T) {
	grid := func(period, phase float64) beatGrid {
		result := beatGrid{meter: 3}
		for i, at := 0, phase; at < 32; i, at = i+1, at+period {
			result.beats = append(result.beats, at)
			if i%result.meter == 0 {
				result.downbeats = append(result.downbeats, at)
			}
		}
		return result
	}
	a := analyzedPCM(t, 32, func(float64) float64 { return .2 })
	b := analyzedPCM(t, 32, func(float64) float64 { return .2 })
	aa, bb := a.analysis.Load(), b.analysis.Load()
	applyDeviceBeatGrid(aa, grid(.52, .11))
	applyDeviceBeatGrid(bb, grid(.50, .23))
	plan := planBeatMix(a, b, aa, bb)
	if plan == nil || !plan.smart || math.Abs(plan.tempo-1.04) > .001 || plan.ramp == 0 {
		t.Fatalf("modeled beat grid did not plan a variable-tempo transition: %+v", plan)
	}
	incomingDownbeat := float64(plan.exit-plan.overlap) + float64(bb.head.phase)*plan.tempo
	bars := (incomingDownbeat - float64(aa.tail.phase)) / (3 * float64(aa.tail.beat))
	if math.Abs(bars-math.Round(bars)) > .001 {
		t.Fatalf("transition aligns %.3f bars from the outgoing downbeat", bars)
	}
}
