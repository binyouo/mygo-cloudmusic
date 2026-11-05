package player

import (
	"context"
	"io"
	"math"
	"slices"

	"github.com/egoist/mygo/audio"
)

const (
	mixHop  = outputRate / 50 // 20 ms; independent of decoder buffer sizes.
	mixEdge = 45 * outputRate
)

type mixEdgeAnalysis struct {
	from        int64
	rms, onset  []float64
	first, last int64 // audible bounds, in song frames
	level       float64
	beat        int64
	phase       int64
	confidence  float64
}

// mixVocalWindow is an optional, independently supplied activity interval on
// the original song timeline. Spectral energy is not a vocal classifier.
type mixVocalWindow struct{ start, end int64 }

// mixVocalHints is supplied separately from audio analysis. Known marks precise
// word-timed intervals; approximate line-timed hints may still shorten a blend.
type mixVocalHints struct {
	windows []mixVocalWindow
	known   bool
}

type trackAnalysis struct {
	head, tail       mixEdgeAnalysis
	headPCM, tailPCM []float32 // immutable 48 kHz stereo; each edge is at most 45 s.
	vocals           []mixVocalWindow
	vocalsKnown      bool
	neural           *neuralAnalysis
}

// startMixAnalysis owns a separate decoder: seeking to the outro never moves
// playback, and downloading/analysis never delays the prepared track's handoff.
func startMixAnalysis(p *pcmTrack) {
	dl, _ := p.raw.source()
	if dl == nil || p.track.ProgramID != 0 || p.dec.Len() < 8*outputRate || !p.analysisStarted.CompareAndSwap(false, true) || !dl.share() {
		return
	}
	ms := p.raw.Len() * 1000 / int64(p.raw.SampleRate())
	go func() {
		defer dl.release()
		s, err := openShared(dl, ms)
		if err != nil {
			return
		}
		defer s.Close()
		var dec audio.Decoder = s
		if s.SampleRate() != outputRate {
			dec = newResampler(s)
		}
		ctx := p.analysisContext
		if ctx == nil {
			ctx = context.Background()
		}
		if a, err := analyzeTrack(&mixAnalysisDecoder{Decoder: dec, ctx: ctx}); err == nil && ctx.Err() == nil {
			p.analysis.Store(a)
			// Heavy inference has its own sequential decoder, cancellation and
			// global concurrency budget; the basic profile stays usable meanwhile.
			if result, err := analyzeNeuralTrack(ctx, dl, ms); err == nil && ctx.Err() == nil {
				updated := *a
				updated.neural = result
				p.analysis.Store(&updated)
			}
		}
	}()
}

type mixAnalysisDecoder struct {
	audio.Decoder
	ctx context.Context
}

func (d *mixAnalysisDecoder) Read(pcm []float32) (int, error) {
	if err := d.ctx.Err(); err != nil {
		return 0, err
	}
	return d.Decoder.Read(pcm)
}
func (d *mixAnalysisDecoder) SetPosition(frame int64) error {
	if err := d.ctx.Err(); err != nil {
		return err
	}
	return d.Decoder.SetPosition(frame)
}

func analyzeTrack(d audio.Decoder) (*trackAnalysis, error) {
	length := d.Len()
	if length < 8*outputRate {
		return nil, io.ErrUnexpectedEOF
	}
	head, headPCM, err := readMixEdge(d, 0, min(length, mixEdge), true)
	if err != nil {
		return nil, err
	}
	from := max(0, length-mixEdge)
	if from == 0 {
		// The two edges are the same short song; retain one immutable buffer.
		return &trackAnalysis{head: head, tail: head, headPCM: headPCM, tailPCM: headPCM}, nil
	}
	// MP3 seeking is byte-approximate. Walk the independent decoder to the
	// precise outro sample so beat analysis and the rendered tail agree.
	position := min(length, int64(mixEdge))
	if from < position {
		if err = d.SetPosition(0); err != nil {
			return nil, err
		}
		position = 0
	}
	buf := make([]float32, 8192)
	for position < from {
		n, e := d.Read(buf[:min(int64(len(buf)), (from-position)*2)])
		position += int64(n / 2)
		if e != nil && (e != io.EOF || position < from) {
			return nil, e
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
	}
	tail, tailPCM, err := readMixEdge(d, from, length-from, true)
	if err != nil {
		return nil, err
	}
	return &trackAnalysis{head: head, tail: tail, headPCM: headPCM, tailPCM: tailPCM}, nil
}

// analyzeMixEdge measures stereo power without cancelling opposite-phase channels.
func analyzeMixEdge(d audio.Decoder, from, frames int64) (mixEdgeAnalysis, error) {
	e, _, err := readMixEdge(d, from, frames, false)
	return e, err
}

func readMixEdge(d audio.Decoder, from, frames int64, keepPCM bool) (mixEdgeAnalysis, []float32, error) {
	e := mixEdgeAnalysis{from: from}
	if frames < 0 || frames > mixEdge {
		return e, nil, io.ErrUnexpectedEOF
	}
	var pcm []float32
	if keepPCM {
		pcm = make([]float32, 0, frames*2)
	}
	buf := make([]float32, mixHop*2)
	var low [2]float64
	lowPole := 1 - math.Exp(-2*math.Pi*180/outputRate)
	var lowEnvelope []float64
	for read := int64(0); read < frames; {
		want := min(int64(mixHop), frames-read) * 2
		n := 0
		for int64(n) < want {
			got, err := d.Read(buf[n:want])
			n += got
			if err != nil && err != io.EOF {
				return e, nil, err
			}
			if int64(n) < want && (err != nil || got == 0) {
				return e, nil, io.ErrUnexpectedEOF
			}
		}
		if keepPCM {
			pcm = append(pcm, buf[:n]...)
		}
		var power, lowPower float64
		for i, v := range buf[:n] {
			power += float64(v) * float64(v)
			ch := i & 1
			low[ch] += lowPole * (float64(v) - low[ch])
			lowPower += low[ch] * low[ch]
		}
		e.rms = append(e.rms, math.Sqrt(power/float64(n)))
		lowEnvelope = append(lowEnvelope, math.Sqrt(lowPower/float64(n)))
		read += int64(n / 2)
	}
	if len(e.rms) == 0 {
		return e, pcm, nil
	}
	peak := slices.Max(e.rms)
	if peak < .0001 {
		return e, pcm, nil
	}
	threshold := max(.0001, peak*.012) // silence, never an ordinary quiet intro.
	first, last := -1, -1
	var audible []float64
	for i, v := range e.rms {
		if v >= threshold {
			if first < 0 {
				first = i
			}
			last = i
			audible = append(audible, v)
		}
		// A compressed full mix can hide drum attacks in its broadband RMS.
		// Stereo low-band flux retains their timing without calling it a vocal
		// classifier or cancelling opposite-phase channels.
		lowFlux := max(0, lowEnvelope[i]-mixEnergy(lowEnvelope, max(0, i-5), i))
		fullFlux := max(0, v-mixEnergy(e.rms, max(0, i-5), i))
		e.onset = append(e.onset, .7*lowFlux+.3*fullFlux)
	}
	if first < 0 {
		return e, pcm, nil
	}
	e.first = from + int64(first)*mixHop
	e.last = from + min(frames, int64(last+1)*mixHop)
	slices.Sort(audible)
	e.level = audible[len(audible)*3/4]
	e.findBeat()
	return e, pcm, nil
}

func mixEnergy(values []float64, from, to int) float64 {
	from, to = max(0, from), min(len(values), to)
	if to <= from {
		return 0
	}
	var sum float64
	for _, v := range values[from:to] {
		sum += v
	}
	return sum / float64(to-from)
}

// findBeat accepts a grid only when many attacks agree; sustained or irregular
// material must not acquire an invented beat from its loudness envelope.
func (e *mixEdgeAnalysis) findBeat() {
	var energy float64
	for _, v := range e.onset {
		energy += v * v
	}
	if energy < 1e-8 {
		return
	}
	best, lag := 0.0, 0
	scores := make([]float64, 62)
	for candidate := 15; candidate <= 60; candidate++ { // 50..200 BPM
		var dot, a, b float64
		for i := candidate; i < len(e.onset); i++ {
			x, y := e.onset[i], e.onset[i-candidate]
			dot += x * y
			a += x * x
			b += y * y
		}
		score := dot / max(1e-12, math.Sqrt(a*b))
		scores[candidate] = score
		if score > best+.015 {
			best, lag = score, candidate
		}
	}
	if best < .4 || lag == 0 {
		return
	}
	// Refine the period below the 20 ms envelope bin. Rounding every beat to
	// a whole bin would turn nearby tempos into the same or a wrong BPM, and
	// accumulated phase drift would defeat a genuine stretch-assisted blend.
	period := float64(lag)
	if lag > 15 && lag < 60 {
		left, center, right := scores[lag-1], scores[lag], scores[lag+1]
		if curve := left - 2*center + right; curve < -1e-9 {
			period += min(.5, max(-.5, .5*(left-right)/curve))
		}
	}
	phase, strength := 0, 0.0
	for candidate := range int(math.Ceil(period)) {
		var score float64
		for position := float64(candidate); position < float64(len(e.onset)); position += period {
			i := int(math.Round(position))
			if i < len(e.onset) {
				score += slices.Max(e.onset[max(0, i-1):min(len(e.onset), i+2)])
			}
		}
		if score > strength {
			phase, strength = candidate, score
		}
	}
	type beatPoint struct{ index, at float64 }
	var points []beatPoint
	threshold := slices.Max(e.onset) * .15
	span := max(1, int(period*.1))
	expected := 0
	for position := float64(phase); position < float64(len(e.onset)); position += period {
		i := int(math.Round(position))
		if i >= len(e.onset) {
			break
		}
		peakAt, peak := i, 0.0
		for at := max(0, i-span); at < min(len(e.onset), i+span+1); at++ {
			if e.onset[at] > peak {
				peakAt, peak = at, e.onset[at]
			}
		}
		if peak > threshold {
			points = append(points, beatPoint{float64(expected), float64(peakAt)})
		}
		expected++
	}
	if len(points) < 6 {
		return
	}
	var xMean, yMean float64
	for _, p := range points {
		xMean += p.index
		yMean += p.at
	}
	xMean /= float64(len(points))
	yMean /= float64(len(points))
	var covariance, variance float64
	for _, p := range points {
		covariance += (p.index - xMean) * (p.at - yMean)
		variance += (p.index - xMean) * (p.index - xMean)
	}
	if variance <= 0 {
		return
	}
	period = covariance / variance
	if period < 15 || period > 60 {
		return
	}
	intercept := yMean - period*xMean
	var squaredError float64
	for _, p := range points {
		delta := p.at - (intercept + period*p.index)
		squaredError += delta * delta
	}
	jitter := math.Sqrt(squaredError/float64(len(points))) / period
	coverage := float64(len(points)) / float64(max(1, expected))
	confidence := .5*best + .5*coverage*max(0, 1-jitter/.15)
	if confidence < .65 {
		return
	}
	e.beat = int64(math.Round(period * mixHop))
	phaseFrames := int64(math.Round(intercept * mixHop))
	phaseFrames = (phaseFrames%e.beat + e.beat) % e.beat
	e.phase, e.confidence = e.from+phaseFrames, confidence
}
