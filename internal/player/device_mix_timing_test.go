package player

import (
	"math"
	"testing"
)

// When the kick ends before the rest of the music, the cue stays on the kick's final downbeat.
func TestDeviceTailCueKeepsLastKickDownbeat(t *testing.T) {
	const duration, buffer = 60, 45
	grid := beatGrid{meter: 4, bpm: 120}
	for i := range duration * 2 {
		at := .18 + float64(i)/2
		grid.beats = append(grid.beats, at)
		if i%grid.meter == 0 {
			grid.downbeats = append(grid.downbeats, at)
		}
	}
	s := mixSpectrum{rms: make([]float64, mixSpectrumBins)}
	for i := range s.bands {
		s.bands[i] = make([]float64, mixSpectrumBins)
	}
	for i := range s.rms {
		at := float64(i) * duration / mixSpectrumBins
		s.rms[i] = 1
		s.bands[mixMid][i] = .5
		if at < 52 {
			s.bands[mixLow][i] = .75
		}
	}
	p := buildMixBandProfile(s, grid, duration)
	timing := buildDeviceMixTiming(s.rms, p, grid, duration, buffer)
	want := float64(float32(50.18) - float32(duration-buffer))
	if timing == nil || timing.audioEnd != buffer || math.Abs(timing.anchor-want) > .000001 {
		t.Fatalf("tail timing = %+v, want audible end %d and final kick downbeat %g", timing, buffer, want)
	}
}

// Irregular bars before the outro prevent a beat mix even when the final few bars fit a fixed tempo.
func TestDeviceBeatMixUsesAnchoredGrid(t *testing.T) {
	const duration = 32
	grid := beatGrid{meter: 4, bpm: 120}
	for i := range duration * 2 {
		at := float64(i) / 2
		grid.beats = append(grid.beats, at)
		if i%grid.meter == 0 {
			grid.downbeats = append(grid.downbeats, at)
		}
	}
	rms := make([]float64, mixSpectrumBins)
	for i := range rms {
		rms[i] = 1
	}
	a := analyzedPCM(t, duration, func(float64) float64 { return .2 })
	b := analyzedPCM(t, duration, func(float64) float64 { return .2 })
	aa, bb := a.analysis.Load(), b.analysis.Load()
	profile := deviceMixProfile{beat: grid, spectrum: mixSpectrum{rms: rms}, key: skeyClass(3)}
	profile.apply(aa, duration*outputRate)
	profile.apply(bb, duration*outputRate)
	if plan, _, ok := chooseDeviceTransition(aa, bb); !ok || plan.Style != "blend" {
		t.Fatalf("regular grid should admit a blend: %s", plan.Style)
	}
	// The last four bars, which the local edge fitter uses, stay regular.
	for i := 1; i < 8; i += 2 {
		grid.downbeats[i] += .42
	}
	profile.apply(aa, duration*outputRate)
	if plan, tier, ok := chooseDeviceTransition(aa, bb); !ok || tier != deviceQuickFade || plan.Style == "blend" {
		t.Fatalf("irregular grid should hand off without a beat mix: %s", plan.Style)
	}
}
