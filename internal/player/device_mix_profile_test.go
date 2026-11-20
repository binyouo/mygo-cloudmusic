package player

import "testing"

// TestDeviceProfileGuidesMix shortens a harmonic clash and avoids a long overlap when both tracks have sustained vocals.
func TestDeviceProfileGuidesMix(t *testing.T) {
	const duration = 32
	grid := beatGrid{meter: 4, bpm: 120}
	for i := range duration * 2 {
		grid.beats = append(grid.beats, float64(i)/2)
		if i%grid.meter == 0 {
			grid.downbeats = append(grid.downbeats, float64(i)/2)
		}
	}
	a := analyzedPCM(t, duration, func(float64) float64 { return .2 })
	b := analyzedPCM(t, duration, func(float64) float64 { return .2 })
	aa, bb := a.analysis.Load(), b.analysis.Load()
	out := deviceMixProfile{beat: grid, key: skeyClass(3)} // C major
	in := deviceMixProfile{beat: grid, key: skeyClass(22)} // A minor
	out.apply(aa, duration*outputRate)
	in.apply(bb, duration*outputRate)
	matched := planBeatMix(a, b, aa, bb)
	if matched == nil || matched.overlap < 4*outputRate {
		t.Fatalf("relative keys did not blend: %+v", matched)
	}
	in.key = skeyClass(9) // F-sharp major
	in.apply(bb, duration*outputRate)
	clash := planBeatMix(a, b, aa, bb)
	if clash == nil || clash.overlap > 2*outputRate || clash.overlap >= matched.overlap {
		t.Fatalf("clashing keys did not shorten the blend: %+v", clash)
	}
	in.key = skeyClass(22)
	out.voice, in.voice = make([]float32, duration*10), make([]float32, duration*10)
	for i := range 100 {
		out.voice[len(out.voice)-1-i], in.voice[i] = .9, .9
	}
	out.apply(aa, duration*outputRate)
	in.apply(bb, duration*outputRate)
	if plan := planBeatMix(a, b, aa, bb); plan != nil {
		t.Fatalf("overlapping vocal phrases still planned a beat mix: %+v", plan)
	}
}
