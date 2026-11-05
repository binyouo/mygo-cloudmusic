package player

import (
	"context"
	"errors"
	"io"
	"math"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/audio"
)

// smartAudio is immutable preparation work, not another audio output. It keeps
// each song separate so a live normalization change remains effective.
type smartAudio struct {
	outgoing, incoming                 []float32
	clock                              *smartClock
	sourceStart, pre, overlap, release int64
	levels                             *spectrum
	headroomOff, headroomOn            []float32
	resume                             *smartResume
	enter                              int64
}

type smartRenderJob struct {
	a, b   *pcmTrack
	cancel context.CancelFunc
	ready  atomic.Pointer[smartAudio]
	done   atomic.Bool
	mu     sync.Mutex
	closed bool
}

type smartPlayback struct {
	data               *smartAudio
	previous, incoming *pcmTrack
	start              int64
	promoted, invalid  bool
}

// smartClock maps the rendered tail back to the original song. Rubber Band's
// actual output length, including its delay compensation, is authoritative.
type smartClock struct{ source, output []float64 }
type smartWarp struct {
	clock              *smartClock
	sourceAt, outputAt int64
}

type smartLevelWindow struct {
	spectrum *spectrum
	from, to int64
}

// The incoming decoder advances sequentially on a worker. MP3 byte seeking
// is approximate and must not splice the rendered head to its original body.
type smartResume struct {
	stream  stream
	decoder audio.Decoder
}

func prepareSmartResume(ctx context.Context, p *pcmTrack, frames int64) (*smartResume, error) {
	dl, _ := p.raw.source()
	if dl == nil {
		return nil, errors.New("player: no shared incoming source")
	}
	s, err := openShared(dl, p.track.Duration)
	if err != nil {
		return nil, err
	}
	var dec audio.Decoder = s
	if s.SampleRate() != outputRate {
		dec = newResampler(s)
	}
	buf := make([]float32, 8192)
	for left := frames; left > 0; {
		if err = ctx.Err(); err != nil {
			s.Close()
			return nil, err
		}
		n, e := dec.Read(buf[:min(int64(len(buf)), left*2)])
		left -= int64(n / 2)
		if e != nil && (e != io.EOF || left > 0) || n == 0 {
			s.Close()
			return nil, io.ErrUnexpectedEOF
		}
	}
	return &smartResume{stream: s, decoder: dec}, nil
}

func (d *smartAudio) closeResume() {
	if d.resume != nil {
		d.resume.stream.Close()
		d.resume = nil
	}
}

func (j *smartRenderJob) stop() {
	j.cancel()
	j.mu.Lock()
	j.closed = true
	data := j.ready.Swap(nil)
	j.mu.Unlock()
	if data != nil {
		data.closeResume()
	}
}

func newSmartClock(frames, ramp int64, tempo float64, rendered int64) *smartClock {
	c := &smartClock{source: []float64{0}, output: []float64{0}}
	var output float64
	for at := int64(0); at < frames; {
		n := min(int64(256), frames-at)
		if at < ramp && at+n > ramp {
			n = ramp - at
		}
		output += float64(n) / stretchTempoAt(at+n/2, ramp, tempo)
		at += n
		c.source = append(c.source, float64(at))
		c.output = append(c.output, output)
	}
	if output > 0 {
		for i := range c.output {
			c.output[i] *= float64(rendered) / output
		}
	}
	return c
}

func smartInterpolate(from, to []float64, at float64) int64 {
	i := sort.SearchFloat64s(from, at)
	if i == 0 {
		return int64(math.Round(to[0]))
	}
	if i >= len(from) {
		return int64(math.Round(to[len(to)-1]))
	}
	k := (at - from[i-1]) / max(1e-9, from[i]-from[i-1])
	return int64(math.Round(to[i-1] + k*(to[i]-to[i-1])))
}
func (c *smartClock) sourceFrame(output int64) int64 {
	return smartInterpolate(c.output, c.source, float64(output))
}
func (c *smartClock) outputFrame(source int64) int64 {
	return smartInterpolate(c.source, c.output, float64(source))
}

func smartHeadroomEnvelope(out, in []float32, pre, release int64, ga, gb float32) []float32 {
	frames := int64(len(out)/2) + release
	sample := func(frame int64, ch int64) float32 {
		var value float32
		if frame < int64(len(out)/2) {
			value = out[frame*2+ch] * ga
		}
		if frame >= pre {
			value += in[(frame-pre)*2+ch] * gb
		}
		return value
	}
	full := make([]float32, frames*2)
	for f := int64(0); f < frames; f++ {
		for ch := int64(0); ch < 2; ch++ {
			full[f*2+ch] = sample(f, ch)
		}
	}
	smartMixHeadroom(full)
	envelope := make([]float32, frames)
	for f := int64(0); f < frames; f++ {
		g := float32(1)
		for ch := int64(0); ch < 2; ch++ {
			if v := sample(f, ch); math.Abs(float64(v)) > 1e-12 {
				g = min(g, full[f*2+ch]/v)
			}
		}
		envelope[f] = max(0, g)
	}
	return envelope
}

func prepareSmartAudio(ctx context.Context, a, b *pcmTrack, plan mixPlan) (*smartAudio, error) {
	if plan.full {
		return prepareFullSmartAudio(ctx, a, b, plan)
	}
	aa, bb := a.analysis.Load(), b.analysis.Load()
	if aa == nil || bb == nil {
		return nil, errors.New("player: missing smart-fade analysis")
	}
	from := plan.sourceStart - aa.tail.from
	length := plan.exit - plan.sourceStart
	if from < 0 || length <= 0 || (from+length)*2 > int64(len(aa.tailPCM)) {
		return nil, io.ErrUnexpectedEOF
	}
	source := append([]float32(nil), aa.tailPCM[from*2:(from+length)*2]...)
	estimate := max(int64(1), stretchedTailFrames(length, plan.tempo, plan.ramp)-stretchedTailFrames(plan.ramp, plan.tempo, plan.ramp))
	eqPlan := smartEQPlan{OutgoingStart: plan.ramp, OutgoingLength: plan.overlap, IncomingLength: estimate, BeatFrames: bb.head.beat}
	// Warm filters from real preceding samples, without emitting extra audio.
	outEQ := newSmartEQ(eqPlan)
	if from > 0 {
		warmFrom := max(int64(0), from-2048)
		warm := append([]float32(nil), aa.tailPCM[warmFrom*2:from*2]...)
		outEQ.ProcessOutgoing(warm, warmFrom-from)
	}
	outEQ.ProcessOutgoing(source, 0)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := stretchTailPCM(ctx, source, plan.tempo, plan.ramp)
	if err != nil {
		return nil, err
	}
	frames := int64(len(out) / 2)
	clock := newSmartClock(length, plan.ramp, plan.tempo, frames)
	pre := clock.outputFrame(plan.ramp)
	overlap := frames - pre
	const releaseFrames = outputRate / 4
	if overlap <= 0 || overlap+releaseFrames > int64(len(bb.headPCM)/2) {
		return nil, io.ErrUnexpectedEOF
	}
	in := append([]float32(nil), bb.headPCM[:(overlap+releaseFrames)*2]...)
	eqPlan.IncomingLength = overlap
	inEQ := newSmartEQ(eqPlan)
	inEQ.ProcessIncoming(in, 0)
	// Store the slowly varying crossfade envelopes, leaving both live gains
	// outside the render. No future Read needs to run a filter or allocate.
	for f := int64(0); f < overlap; f++ {
		gb, ga := math.Sincos(float64(f) / float64(overlap) * math.Pi / 2)
		for ch := int64(0); ch < 2; ch++ {
			out[(pre+f)*2+ch] *= float32(ga)
			in[f*2+ch] *= float32(gb)
		}
	}
	return finishSmartAudio(ctx, a, b, out, in, clock, plan.sourceStart, pre, overlap, releaseFrames, 0)
}

func finishSmartAudio(ctx context.Context, a, b *pcmTrack, out, in []float32, clock *smartClock, sourceStart, pre, overlap, releaseFrames, enter int64) (*smartAudio, error) {
	frames := int64(len(out) / 2)
	// Keep normalization modes independent; switching it off must also remove
	// any headroom reduction that was only needed by the normalized pair.
	headroomOff := smartHeadroomEnvelope(out, in, pre, releaseFrames, 1, 1)
	headroomOn := smartHeadroomEnvelope(out, in, pre, releaseFrames, float32(normGain(a.info)), float32(normGain(b.info)))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	combined := make([]float32, (frames+releaseFrames)*2)
	copy(combined, out)
	for i, v := range in {
		combined[pre*2+int64(i)] += v
	}
	levels := newSpectrum(outputRate, frames+releaseFrames)
	levels.feed(0, combined)
	resume, err := prepareSmartResume(ctx, b, enter+overlap+releaseFrames)
	if err != nil {
		return nil, err
	}
	return &smartAudio{outgoing: out, incoming: in, clock: clock, sourceStart: sourceStart, pre: pre, overlap: overlap, release: releaseFrames, levels: levels, headroomOff: headroomOff, headroomOn: headroomOn, resume: resume, enter: enter}, nil
}

// Called under sequence.mu; the worker retains only immutable profiles and
// never reads or seeks the foreground decoder.
func (s *sequence) startSmartRender(a, b *pcmTrack, plan mixPlan) {
	if s.render != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &smartRenderJob{a: a, b: b, cancel: cancel}
	s.render = job
	go func() {
		if data, err := prepareSmartAudio(ctx, a, b, plan); err == nil {
			job.mu.Lock()
			if !job.closed && ctx.Err() == nil {
				job.ready.Store(data)
			} else {
				data.closeResume()
			}
			job.mu.Unlock()
		}
		job.done.Store(true)
	}()
}

func (s *sequence) cancelSmartRender() {
	if s.render != nil {
		s.render.stop()
		s.render = nil
	}
}

func (s *sequence) beginSmart(part audioPart, data *smartAudio) {
	a, b := part.track, s.next
	if s.render != nil {
		s.render.ready.Swap(nil)
		s.render.stop()
	}
	a.retain()
	b.retain()
	s.smart = &smartPlayback{data: data, previous: a, incoming: b, start: s.pos}
	s.parts[len(s.parts)-1].warp = &smartWarp{clock: data.clock, sourceAt: data.sourceStart, outputAt: s.pos}
	s.parts[len(s.parts)-1].levels = &smartLevelWindow{spectrum: data.levels, from: s.pos, to: s.pos + data.pre + data.overlap + data.release}
	s.plan = nil
	s.render = nil
}

// readSmart runs only on the decoder thread, under mu. The incoming track
// becomes visible at its first audible frame, not at the start of pre-ramp.
func (s *sequence) readSmart(dst []float32) (n int, changed bool, err error) {
	pb := s.smart
	data := pb.data
	at := s.pos - pb.start
	if !pb.promoted && at >= data.pre {
		if data.resume == nil {
			return 0, false, errors.New("player: missing sequential continuation")
		}
		_, spec := pb.incoming.raw.source()
		switch resumed := data.resume.stream.(type) {
		case *mp3Stream:
			resumed.spec = spec
		case *flacStream:
			resumed.spec = spec
		}
		if pb.incoming.continuation != nil {
			pb.incoming.continuation.Close()
		}
		pb.incoming.continuation, pb.incoming.dec, pb.incoming.head = data.resume.stream, data.resume.decoder, nil
		data.resume = nil
		if s.next == pb.incoming {
			s.next = nil
		} else {
			pb.incoming.retain()
		}
		s.parts = append(s.parts, audioPart{track: pb.incoming, start: s.pos, offset: data.enter, overlap: data.overlap, revision: s.revision, levels: &smartLevelWindow{spectrum: data.levels, from: pb.start, to: pb.start + data.pre + data.overlap + data.release}})
		pb.promoted = true
		return 0, true, nil
	}
	end := data.pre + data.overlap + data.release
	limit := min(int64(len(dst)/2), end-at)
	if !pb.promoted {
		limit = min(limit, data.pre-at)
	}
	ga, gb := pb.previous.volume(), pb.incoming.volume()
	headroom := data.headroomOff
	if s.normalized {
		headroom = data.headroomOn
	}
	for f := int64(0); f < limit; f++ {
		for ch := int64(0); ch < 2; ch++ {
			var value float32
			if at+f < data.pre+data.overlap {
				value = data.outgoing[(at+f)*2+ch] * ga
			}
			if at+f >= data.pre {
				value += data.incoming[(at+f-data.pre)*2+ch] * gb
			}
			dst[f*2+ch] = value * headroom[at+f]
		}
	}
	s.pos += limit
	if s.pos >= pb.start+end {
		pb.previous.release()
		pb.incoming.release()
		s.smart = nil
	}
	return int(limit * 2), false, nil
}

func (s *sequence) smartLevels(start, frame int64) ([Bands]float32, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, part := range s.parts {
		if part.start == start && part.levels != nil && frame >= part.levels.from && frame < part.levels.to {
			return part.levels.spectrum.at(frame - part.levels.from)
		}
	}
	return [Bands]float32{}, false
}

func (s *sequence) songPosition(start, frame int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, part := range s.parts {
		if part.start == start {
			return part.sourcePosition(frame)
		}
	}
	return max(0, frame-start)
}
func (p audioPart) sourcePosition(frame int64) int64 {
	if p.warp != nil && frame >= p.warp.outputAt {
		return p.warp.sourceAt + p.warp.clock.sourceFrame(frame-p.warp.outputAt)
	}
	return max(0, frame-p.start+p.offset)
}
func (s *sequence) outputPosition(start, source int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, part := range s.parts {
		if part.start == start {
			if part.warp != nil && source >= part.warp.sourceAt {
				return part.warp.outputAt + part.warp.clock.outputFrame(source-part.warp.sourceAt)
			}
			return max(0, part.start+source-part.offset)
		}
	}
	return max(0, start+source)
}
