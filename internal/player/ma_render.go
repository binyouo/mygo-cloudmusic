package player

import (
	"context"
	"errors"
	"io"
	"math"
	"slices"
	"sort"
)

// These are the serializable fields of the pinned MA TransitionPlan. Planning
// stays on a worker; the real-time sequence sees only finished immutable PCM.
type maTransitionPlan struct {
	Version           int     `json:"version"`
	Applicable        bool    `json:"applicable"`
	Style             string  `json:"style"`
	FadeOutWindow     float64 `json:"fade_out_window"`
	CrossfadeDuration float64 `json:"crossfade_duration"`
	Enter             float64 `json:"fadein_trim_start"`
	FadeOutCurve      string  `json:"fadeout_curve"`
	FadeInCurve       string  `json:"fadein_curve"`
	Tempo             struct {
		Steps [][2]float64 `json:"steps"`
	} `json:"tempo_plan"`
	EQ struct {
		LowOut  *maShelf `json:"low_out"`
		LowIn   *maShelf `json:"low_in"`
		HighOut *maShelf `json:"high_out"`
		HighIn  *maShelf `json:"high_in"`
		MidOut  *maShelf `json:"mid_out"`
		MidIn   *maShelf `json:"mid_in"`
	} `json:"eq_plan"`
	Highpass *maHighpass `json:"highpass"`
	Echo     *maEcho     `json:"echo"`
}
type maShelf struct {
	Type      string       `json:"shelf_type"`
	Frequency float64      `json:"frequency"`
	Width     float64      `json:"width_oct"`
	Steps     [][2]float64 `json:"steps"`
}
type maHighpass struct {
	Start   float64 `json:"start_s"`
	End     float64 `json:"end_s"`
	StartHz float64 `json:"start_hz"`
	EndHz   float64 `json:"end_hz"`
}
type maEcho struct {
	Cut  float64 `json:"cut_s"`
	Beat float64 `json:"beat_s"`
}

func (p *maTransitionPlan) validate(buffer, incoming float64) error {
	bad := errors.New("player: invalid MA transition plan")
	if p.Version != 1 || !p.Applicable {
		return bad
	}
	if !slices.Contains([]string{"blend", "segue", "cut", "filter_out", "echo_out"}, p.Style) ||
		!finiteMixNumber(p.FadeOutWindow) || p.FadeOutWindow <= 0 || p.FadeOutWindow > buffer+.001 ||
		!finiteMixNumber(p.CrossfadeDuration) || p.CrossfadeDuration <= 0 || p.CrossfadeDuration > buffer*2 ||
		!finiteMixNumber(p.Enter) || p.Enter < 0 || p.Enter+p.CrossfadeDuration > incoming ||
		!slices.Contains([]string{"qsin", "nofade"}, p.FadeOutCurve) || !slices.Contains([]string{"qsin", "nofade"}, p.FadeInCurve) {
		return bad
	}
	last := -1.0
	for _, step := range p.Tempo.Steps {
		if !finiteMixNumber(step[0]) || !finiteMixNumber(step[1]) || step[0] < 0 || step[0] < last || step[0] > buffer || step[1] < .5 || step[1] > 2 {
			return bad
		}
		last = step[0]
	}
	for _, shelf := range []*maShelf{p.EQ.LowOut, p.EQ.LowIn, p.EQ.HighOut, p.EQ.HighIn, p.EQ.MidOut, p.EQ.MidIn} {
		if shelf == nil {
			continue
		}
		if !slices.Contains([]string{"lowshelf", "highshelf", "equalizer"}, shelf.Type) || !finiteMixNumber(shelf.Frequency) || shelf.Frequency < 20 || shelf.Frequency > 20000 || !finiteMixNumber(shelf.Width) || shelf.Width <= 0 || shelf.Width > 8 || len(shelf.Steps) == 0 || len(shelf.Steps) > 4096 {
			return bad
		}
		last = -1
		for _, step := range shelf.Steps {
			if !finiteMixNumber(step[0]) || !finiteMixNumber(step[1]) || step[0] < 0 || step[0] < last || step[0] > buffer*2 || step[1] < -30 || step[1] > 0 {
				return bad
			}
			last = step[0]
		}
	}
	if hp := p.Highpass; hp != nil {
		if !finiteMixNumber(hp.Start) || !finiteMixNumber(hp.End) || !finiteMixNumber(hp.StartHz) || !finiteMixNumber(hp.EndHz) || hp.Start < 0 || hp.End <= hp.Start || hp.End > buffer || hp.StartHz < 20 || hp.EndHz < hp.StartHz || hp.EndHz > 20000 {
			return bad
		}
	}
	if echo := p.Echo; echo != nil {
		if !finiteMixNumber(echo.Cut) || !finiteMixNumber(echo.Beat) || echo.Beat <= 0 || echo.Beat > 2 || echo.Cut < echo.Beat || echo.Cut > buffer {
			return bad
		}
	}
	return nil
}

func scheduledTempo(steps [][2]float64, seconds float64) float64 {
	if len(steps) == 0 {
		return 1
	}
	i := sort.Search(len(steps), func(i int) bool { return steps[i][0] > seconds })
	return steps[max(0, i-1)][1]
}

func scheduledTailClock(frames int64, steps [][2]float64, rendered int64) *smartClock {
	c := &smartClock{source: []float64{0}, output: []float64{0}}
	var output float64
	for at := int64(0); at < frames; {
		n := min(int64(256), frames-at)
		for _, step := range steps {
			boundary := int64(math.Round(step[0] * outputRate))
			if boundary > at && boundary < at+n {
				n = boundary - at
			}
		}
		output += float64(n) / scheduledTempo(steps, float64(at+n/2)/outputRate)
		at += n
		c.source = append(c.source, float64(at))
		c.output = append(c.output, output)
	}
	if rendered > 0 && output > 0 {
		for i := range c.output {
			c.output[i] *= float64(rendered) / output
		}
	}
	return c
}

func maGainAt(steps [][2]float64, at float64) float64 {
	if at <= steps[0][0] {
		return steps[0][1]
	}
	i := sort.Search(len(steps), func(i int) bool { return steps[i][0] >= at })
	if i == len(steps) {
		return steps[i-1][1]
	}
	a, b := steps[i-1], steps[i]
	return a[1] + (b[1]-a[1])*(at-a[0])/max(1e-9, b[0]-a[0])
}

func applyMAShelf(pcm []float32, schedule *maShelf) {
	if schedule == nil {
		return
	}
	var filter smartBiquad
	for f := 0; f < len(pcm)/2; f++ {
		if f%64 == 0 {
			db := maGainAt(schedule.Steps, float64(f)/outputRate)
			if schedule.Type == "equalizer" {
				filter.peak(schedule.Frequency, schedule.Width, db)
			} else {
				filter.shelf(schedule.Frequency, db, schedule.Type == "highshelf")
			}
		}
		for ch := range 2 {
			pcm[f*2+ch] = float32(filter.sample(float64(pcm[f*2+ch]), ch))
		}
	}
}

func applyMAEffects(pcm []float32, p *maTransitionPlan) {
	if hp := p.Highpass; hp != nil {
		var filter smartBiquad
		for f := int64(math.Round(hp.Start * outputRate)); f < int64(len(pcm)/2); f++ {
			if f == int64(math.Round(hp.Start*outputRate)) || f%64 == 0 {
				phase := min(1, max(0, (float64(f)/outputRate-hp.Start)/(hp.End-hp.Start)))
				hz := hp.StartHz * math.Pow(hp.EndHz/hp.StartHz, phase)
				sin, cos := math.Sincos(2 * math.Pi * hz / outputRate)
				alpha := sin / math.Sqrt2
				filter.set((1+cos)/2, -1-cos, (1+cos)/2, 1+alpha, -2*cos, 1-alpha)
			}
			for ch := range 2 {
				pcm[f*2+int64(ch)] = float32(filter.sample(float64(pcm[f*2+int64(ch)]), ch))
			}
		}
	}
	if echo := p.Echo; echo != nil {
		original := slices.Clone(pcm)
		cut, beat := int64(math.Round(echo.Cut*outputRate)), int64(math.Round(echo.Beat*outputRate))
		declick := float64(outputRate) * .015
		for f := int64(0); f < int64(len(pcm)/2); f++ {
			dry := min(1, max(0, 1-float64(f-cut)/declick))
			for ch := int64(0); ch < 2; ch++ {
				value := float64(original[f*2+ch]) * dry
				for tap, gain := range []float64{.5, .25, .12, .06} {
					source := f - int64(tap+1)*beat
					if source < max(0, cut-beat) || source >= cut {
						continue
					}
					edge := min(1, float64(source-(cut-beat))/declick, float64(cut-source)/declick)
					value += float64(original[source*2+ch]) * gain * max(0, edge)
				}
				pcm[f*2+ch] = float32(value)
			}
		}
	}
}

func prepareFullSmartAudio(ctx context.Context, a, b *pcmTrack, initial mixPlan) (*smartAudio, error) {
	aa, bb := initial.outAnalysis, initial.inAnalysis
	if aa == nil || bb == nil || aa.neural == nil || bb.neural == nil {
		return nil, errors.New("player: missing MA analysis")
	}
	p, err := neuralPlan(ctx, aa.neural, bb.neural, float64(len(aa.tailPCM)/2)/outputRate)
	if err != nil {
		return nil, err
	}
	length := int64(math.Round(p.FadeOutWindow * outputRate))
	if length <= 0 || length*2 > int64(len(aa.tailPCM)) {
		return nil, io.ErrUnexpectedEOF
	}
	out := slices.Clone(aa.tailPCM[:length*2])
	for _, shelf := range []*maShelf{p.EQ.LowOut, p.EQ.HighOut, p.EQ.MidOut} {
		applyMAShelf(out, shelf)
	}
	out, err = stretchScheduledPCM(ctx, out, p.Tempo.Steps)
	if err != nil {
		return nil, err
	}
	applyMAEffects(out, p)
	frames := int64(len(out) / 2)
	enter := int64(math.Round(p.Enter * outputRate))
	const releaseFrames = outputRate / 4
	overlap := min(int64(math.Round(p.CrossfadeDuration*outputRate)), frames, int64(len(bb.headPCM)/2)-enter-releaseFrames)
	if overlap <= 0 || enter < 0 {
		return nil, io.ErrUnexpectedEOF
	}
	pre := frames - overlap
	in := slices.Clone(bb.headPCM[enter*2 : (enter+overlap+releaseFrames)*2])
	for _, shelf := range []*maShelf{p.EQ.LowIn, p.EQ.HighIn, p.EQ.MidIn} {
		applyMAShelf(in, shelf)
	}
	for f := int64(0); f < overlap; f++ {
		phase := float64(f) / float64(overlap)
		gb, ga := math.Sincos(phase * math.Pi / 2)
		if p.FadeOutCurve == "nofade" {
			ga = 1
		}
		if p.FadeInCurve == "nofade" {
			gb = 1
		}
		for ch := int64(0); ch < 2; ch++ {
			out[(pre+f)*2+ch] *= float32(ga)
			in[f*2+ch] *= float32(gb)
		}
	}
	clock := scheduledTailClock(length, p.Tempo.Steps, frames)
	return finishSmartAudio(ctx, a, b, out, in, clock, initial.sourceStart, pre, overlap, releaseFrames, enter)
}
