package player

import (
	"context"
	"math"
	"testing"
)

// A bass-light outro leaves the incoming bass intact, and a bass-heavy pair keeps the incoming deck audible.
func TestMeasuredSmartEQShelvesFollowTrackBands(t *testing.T) {
	const seconds = 8
	grid := beatGrid{downbeats: []float64{0, 1, 2, 3, 4, 5, 6, 7}, meter: 3, bpm: 180}
	profile := func(bass, mid, high float64) *mixBandProfile {
		t.Helper()
		pcm := make([]float32, seconds*beatSampleRate)
		for i := range pcm {
			phase := 2 * math.Pi * float64(i) / beatSampleRate
			pcm[i] = float32(bass*math.Sin(80*phase) + mid*math.Sin(1000*phase) + high*math.Sin(5000*phase))
		}
		spectrum, err := analyzeMixSpectrum(context.Background(), pcm, []int{len(pcm)})
		if err != nil {
			t.Fatal(err)
		}
		p := buildMixBandProfile(spectrum, grid, seconds)
		if p == nil {
			t.Fatal("missing profile for an audible track with eight downbeats")
		}
		return p
	}
	light := profile(.10, .40, .08)
	heavy := profile(.70, .20, .09)

	out, in := measuredSmartEQ(light, heavy, seconds, 0)
	if out.Low != -26 || out.High != 0 || in.Low != 0 || in.High >= -6 {
		t.Fatalf("reciprocal shelves = outgoing %+v, incoming %+v", out, in)
	}
	out, in = measuredSmartEQ(heavy, heavy, seconds, 0)
	if out.Low != -26 || in.Low <= out.Low || in.Low > -6 {
		t.Fatalf("bass-heavy shelves = outgoing %g dB, incoming %g dB", out.Low, in.Low)
	}
	bassFraction := .70 * .70 / (.70*.70 + .20*.20 + .09*.09)
	fullBandGain := (1 - bassFraction) + bassFraction*math.Pow(10, in.Low/10)
	if attenuation := 10 * math.Log10(fullBandGain); math.Abs(attenuation+6) > .001 {
		t.Fatalf("incoming broadband attenuation = %g dB, want -6 dB", attenuation)
	}
}
