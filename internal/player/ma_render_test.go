package player

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

type approximateMixSeek struct{ *samplesStream }

func (d *approximateMixSeek) SetPosition(frame int64) error {
	if frame > 0 {
		frame += 1200
	}
	return d.samplesStream.SetPosition(frame)
}

// Outro analysis must retain exact source frames; approximate MP3 byte seeks previously shifted its beat grid.
func TestMixAnalysisRetainsExactOutro(t *testing.T) {
	samples := make([]float32, 50*outputRate*2)
	for i := range samples {
		samples[i] = float32(i%100003) / 200006
	}
	d := &approximateMixSeek{&samplesStream{samples: samples, rate: outputRate}}
	profile, err := analyzeTrack(d)
	if err != nil {
		t.Fatal(err)
	}
	from := profile.tail.from * 2
	if !slices.Equal(profile.tailPCM, samples[from:]) {
		t.Fatal("outro PCM no longer matches its original timeline")
	}
}

// Invalid model schedules must be rejected before they can index or stretch audio buffers.
func TestMAPlanRejectsInvalidSchedules(t *testing.T) {
	valid := maTransitionPlan{Version: 1, Applicable: true, Style: "blend", FadeOutWindow: 20, CrossfadeDuration: 4, FadeOutCurve: "qsin", FadeInCurve: "qsin"}
	if err := valid.validate(20, 30); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*maTransitionPlan)
	}{
		{"unavailable", func(p *maTransitionPlan) { p.Applicable = false }},
		{"past outgoing", func(p *maTransitionPlan) { p.FadeOutWindow = 21 }},
		{"past incoming", func(p *maTransitionPlan) { p.Enter = 29 }},
		{"nonfinite", func(p *maTransitionPlan) { p.CrossfadeDuration = math.NaN() }},
		{"bad tempo", func(p *maTransitionPlan) { p.Tempo.Steps = [][2]float64{{0, 0}} }},
		{"backward tempo", func(p *maTransitionPlan) { p.Tempo.Steps = [][2]float64{{2, 1}, {1, 1.05}} }},
		{"bad echo", func(p *maTransitionPlan) { p.Echo = &maEcho{Cut: .1, Beat: .5} }},
		{"bad filter", func(p *maTransitionPlan) { p.Highpass = &maHighpass{Start: 1, End: 2, StartHz: 0, EndHz: 200} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := valid
			test.change(&p)
			if p.validate(20, 30) == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
}

// A changing tempo maps source and rendered clocks in both directions, including the corrected endpoint.
func TestMATempoClockPreservesSourceTimeline(t *testing.T) {
	steps := [][2]float64{{0, 1}, {1, 1.05}, {3, .98}}
	frames := int64(5 * outputRate)
	want := float64(outputRate) * (1 + 2/1.05 + 2/.98)
	c := scheduledTailClock(frames, steps, 0)
	if math.Abs(float64(c.outputFrame(frames))-want) > 1 {
		t.Fatal("tempo integration changed source duration")
	}
	c = scheduledTailClock(frames, steps, 240123)
	if c.sourceFrame(240123) != frames || c.outputFrame(frames) != 240123 {
		t.Fatal("actual Rubber Band endpoint was lost")
	}
	for source := int64(0); source < frames; source += 721 {
		if abs := math.Abs(float64(c.sourceFrame(c.outputFrame(source)) - source)); abs > 1 {
			t.Fatalf("round trip error %.0f", abs)
		}
	}
	if _, err := stretchScheduledPCM(context.Background(), []float32{0, 0}, [][2]float64{{0, math.NaN()}}); err == nil {
		t.Fatal("nonfinite schedule reached stretcher")
	}
}

// The outgoing echo repeats only the protected beat before the cut, never the discarded following phrase.
func TestMAEchoDoesNotRepeatPastCut(t *testing.T) {
	p := &maTransitionPlan{Echo: &maEcho{Cut: 1, Beat: .5}}
	a, b := make([]float32, 3*outputRate*2), make([]float32, 3*outputRate*2)
	for f := 0; f < 3*outputRate; f++ {
		v := float32(.2 * math.Sin(2*math.Pi*220*float64(f)/outputRate))
		for ch := range 2 {
			a[f*2+ch], b[f*2+ch] = v, v
			if f > outputRate+outputRate/50 {
				b[f*2+ch] = .9
			}
		}
	}
	applyMAEffects(a, p)
	applyMAEffects(b, p)
	if !slices.Equal(a[(outputRate+outputRate/50+1)*2:], b[(outputRate+outputRate/50+1)*2:]) {
		t.Fatal("echo leaked audio after the protected cut")
	}
	var power float64
	for _, v := range a[outputRate*3 : outputRate*4] {
		power += float64(v * v)
	}
	if power < .01 {
		t.Fatal("the protected beat did not echo")
	}
}

// A measured shelf changes bass energy while an absent schedule leaves original PCM untouched.
func TestMAMeasuredEQAttenuatesBass(t *testing.T) {
	pcm := make([]float32, outputRate*2)
	for f := 0; f < outputRate; f++ {
		v := float32(.3 * math.Sin(2*math.Pi*60*float64(f)/outputRate))
		pcm[f*2], pcm[f*2+1] = v, v
	}
	original := slices.Clone(pcm)
	applyMAShelf(pcm, nil)
	if !slices.Equal(pcm, original) {
		t.Fatal("bypassed EQ modified samples")
	}
	applyMAShelf(pcm, &maShelf{Type: "lowshelf", Frequency: 100, Width: .707, Steps: [][2]float64{{0, -26}}})
	var before, after float64
	for i := outputRate; i < len(pcm); i++ {
		before += float64(original[i] * original[i])
		after += float64(pcm[i] * pcm[i])
	}
	if after > before*.1 || !finiteMixNumber(after) {
		t.Fatalf("bass energy %v -> %v", before, after)
	}
}

// Incoming trim must follow the promoted song's clock; it previously always restarted its lyrics at zero.
func TestMASmartPromotionKeepsIncomingOffset(t *testing.T) {
	a, b := testPCM(t, outputRate, 100, .1), testPCM(t, outputRate, 100, .2)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	resume := &samplesStream{samples: make([]float32, 200), rate: outputRate}
	data := &smartAudio{outgoing: make([]float32, 40), incoming: make([]float32, 30), pre: 10, overlap: 10, release: 5, enter: 20, clock: newSmartClock(20, 0, 1, 20), headroomOff: make([]float32, 25), resume: &smartResume{stream: resume, decoder: resume}}
	s.beginSmart(s.parts[0], data)
	if _, err := s.Read(make([]float32, 22)); err != nil {
		t.Fatal(err)
	}
	part, _, _ := s.state(s.pos)
	if part.track != b || part.offset != 20 || part.sourcePosition(s.pos) != 21 {
		t.Fatalf("incoming timeline lost trim: %+v position %d", part, part.sourcePosition(s.pos))
	}
}

// A truncated or corrupt cache cannot become a trusted beat grid or vocal-activity mask.
func TestNeuralCacheRejectsCorruptTimelines(t *testing.T) {
	base := map[string]any{"version": 1, "duration": 40, "bpm": 120, "beats": []float64{0, .5, 1}, "downbeats": []float64{0}, "beats_per_bar": 4, "vocal_activity": make([]float64, 1800)}
	for _, name := range []string{"rms_energy", "spectral_centroid", "band_rms_low", "band_rms_low_mid", "band_rms_mid", "band_rms_high"} {
		base[name] = make([]float64, 1800)
	}
	path := filepath.Join(t.TempDir(), "analysis.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if _, err := readNeuralAnalysis(path, 40); err != nil {
		t.Fatal(err)
	}
	base["beats"] = []float64{0, 1, .5}
	write()
	if _, err := readNeuralAnalysis(path, 40); err == nil {
		t.Fatal("trusted backward beats")
	}
	base["beats"] = []float64{0, .5, 1}
	base["vocal_activity"] = []float64{.8}
	write()
	if _, err := readNeuralAnalysis(path, 40); err == nil {
		t.Fatal("trusted partial vocal mask")
	}
	base["vocal_activity"] = make([]float64, 1800)
	base["duration"] = 30
	write()
	if _, err := readNeuralAnalysis(path, 40); err == nil {
		t.Fatal("trusted a different rendition's duration")
	}
}
