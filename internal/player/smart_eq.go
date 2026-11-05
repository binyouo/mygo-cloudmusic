package player

import "math"

// The handoff follows Music Assistant's Smart Fades assembly/renderer design:
// https://github.com/music-assistant/server/tree/dev/music_assistant/controllers/streams/smart_fades
// This is an independent Go implementation: 100 Hz bass exchange centered at
// the incoming downbeat (60% fallback), an earlier high-frequency entrance,
// and a later outgoing high-frequency retreat. Source-time outgoing EQ belongs
// before time stretching. The incoming stream remains on its original clock.

type smartEQDepths struct {
	Low, Mid, High float64 // attenuation in dB; zero bypasses that band.
}

type smartEQPlan struct {
	// OutgoingStart/Length describe the overlap in the outgoing tail's source
	// frames. IncomingLength is its rendered/output length. Positions supplied
	// to ProcessOutgoing are relative to that complete tail, including pre-roll.
	OutgoingStart, OutgoingLength int64
	IncomingLength                int64
	BeatFrames, SwapFrame         int64 // incoming clock; SwapFrame<=0 uses 60%.
	// Nil profiles use conservative shelves (-12 dB bass, -6 dB highs) and
	// bypass mids. A vocal/mid cut requires an explicit measured plan.
	Outgoing, Incoming *smartEQDepths
}

type smartEQ struct {
	plan                     smartEQPlan
	outgoing, incoming       smartSideEQ
	outDepth, inDepth        smartEQDepths
	swapStart, swapEnd       float64
	highInStart, highInEnd   float64
	highOutStart, highOutEnd float64
}

func newSmartEQ(plan smartEQPlan) *smartEQ {
	plan.IncomingLength = max(1, plan.IncomingLength)
	if plan.OutgoingLength <= 0 {
		plan.OutgoingLength = plan.IncomingLength
	}
	m := &smartEQ{plan: plan}
	m.outDepth, m.inDepth = smartEQDepths{Low: -12, High: -6}, smartEQDepths{Low: -12, High: -6}
	if plan.Outgoing != nil {
		m.outDepth = *plan.Outgoing
	}
	if plan.Incoming != nil {
		m.inDepth = *plan.Incoming
	}
	m.outDepth, m.inDepth = boundedSmartDepth(m.outDepth), boundedSmartDepth(m.inDepth)
	length := float64(plan.IncomingLength)
	swapLength := length * .5
	if plan.BeatFrames > 0 {
		// Music Assistant bounds the bass exchange between two and eight bars.
		beat := float64(plan.BeatFrames)
		swapLength = min(max(swapLength, 8*beat), 32*beat, length)
	}
	center := float64(plan.SwapFrame)
	if plan.SwapFrame <= 0 {
		center = length * .6
	}
	center = min(max(center, swapLength/2), length-swapLength/2)
	m.swapStart, m.swapEnd = (center-swapLength/2)/length, (center+swapLength/2)/length
	m.highInStart = max(0, m.swapStart-.25)
	m.highInEnd = min(1, m.highInStart+.25)
	m.highOutStart, m.highOutEnd = m.swapEnd, min(1, m.swapEnd+.25)
	return m
}

func boundedSmartDepth(d smartEQDepths) smartEQDepths {
	d.Low = min(0, max(-26, d.Low))
	d.Mid = min(0, max(-8, d.Mid))
	d.High = min(0, max(-26, d.High))
	return d
}

// ProcessOutgoing applies EQ in original source time, before the tempo ramp.
// Calls for one tail must be contiguous; discontinuities reset filter history.
func (m *smartEQ) ProcessOutgoing(samples []float32, sourceFrame int64) {
	m.process(&m.outgoing, samples, sourceFrame, true)
}

// ProcessIncoming applies EQ on the incoming song's unchanged sample clock.
func (m *smartEQ) ProcessIncoming(samples []float32, frame int64) {
	m.process(&m.incoming, samples, frame, false)
}

func (m *smartEQ) process(side *smartSideEQ, samples []float32, first int64, outgoing bool) {
	if !side.ready || first != side.nextFrame {
		*side = smartSideEQ{}
	}
	for i := 0; i+1 < len(samples); i += 2 {
		frame := first + int64(i/2)
		if !side.ready || frame >= side.updateFrame {
			var phase, low, mid, high float64
			if outgoing {
				phase = float64(frame-m.plan.OutgoingStart) / float64(m.plan.OutgoingLength)
				swap := smartRamp(phase, m.swapStart, m.swapEnd)
				low, mid = m.outDepth.Low*swap, m.outDepth.Mid*swap
				high = m.outDepth.High * smartRamp(phase, m.highOutStart, m.highOutEnd)
			} else {
				phase = float64(frame) / float64(m.plan.IncomingLength)
				swap := smartRamp(phase, m.swapStart, m.swapEnd)
				low, mid = m.inDepth.Low*(1-swap), m.inDepth.Mid*(1-swap)
				high = m.inDepth.High * (1 - smartRamp(phase, m.highInStart, m.highInEnd))
			}
			// Per-64-frame coefficient updates avoid the upstream 100 ms command
			// steps without doing trigonometry for every sample or allocating.
			side.filters[0].shelf(100, low, false)
			side.filters[1].shelf(13000, high, true)
			side.filters[2].peak(1200, 2.5, mid)
			side.ready, side.updateFrame = true, frame+64
		}
		for ch := range 2 {
			v := float64(samples[i+ch])
			for band := range side.filters {
				v = side.filters[band].sample(v, ch)
			}
			samples[i+ch] = float32(v)
		}
	}
	side.nextFrame = first + int64(len(samples)/2)
}

// Mix combines already-EQ'd, already-stretched blocks. It neither clips nor
// normalizes individual sample peaks; offline smartMixHeadroom handles the
// completed render using a linked, anticipatory gain envelope.
func (m *smartEQ) Mix(dst, outgoing, incoming []float32, first int64, outGain, inGain float32) {
	for i := 0; i+1 < len(dst); i += 2 {
		phase := min(1, max(0, float64(first+int64(i/2))/float64(m.plan.IncomingLength)))
		sin, cos := math.Sincos(phase * math.Pi / 2)
		ga, gb := float32(cos)*outGain, float32(sin)*inGain
		for ch := range 2 {
			var a, b float32
			if i+ch < len(outgoing) {
				a = outgoing[i+ch]
			}
			if i+ch < len(incoming) {
				b = incoming[i+ch]
			}
			dst[i+ch] = a*ga + b*gb
		}
	}
}

// smartMixHeadroom protects a complete offline render without flattening each
// waveform peak. Both channels share one gain; forward/backward envelopes add
// a 5 ms anticipatory attack and 50 ms release. It never boosts quiet material.
// Supply any post-overlap rendered audio too, so the release can finish there.
// This is a worker-only pass; Mix itself performs no allocations.
func smartMixHeadroom(samples []float32) {
	frames := len(samples) / 2
	var maximum float32
	for _, v := range samples {
		maximum = max(maximum, float32(math.Abs(float64(v))))
	}
	if maximum <= 1 || frames == 0 {
		return
	}
	gain := make([]float32, frames)
	const release = float32(1.0 / (.05 * outputRate))
	const attack = float32(1.0 / (.005 * outputRate))
	for i := range frames {
		peak := max(float32(math.Abs(float64(samples[i*2]))), float32(math.Abs(float64(samples[i*2+1]))))
		gain[i] = 1
		if peak > 1 {
			gain[i] = .98 / peak
		}
		if i > 0 {
			gain[i] = min(gain[i], gain[i-1]+release)
		}
	}
	for i := frames - 2; i >= 0; i-- {
		gain[i] = min(gain[i], gain[i+1]+attack)
	}
	for i, g := range gain {
		samples[i*2] *= g
		samples[i*2+1] *= g
	}
}

func smartRamp(at, from, to float64) float64 {
	if to <= from {
		if at > from {
			return 1
		}
		return 0
	}
	return min(1, max(0, (at-from)/(to-from)))
}

type smartSideEQ struct {
	filters                [3]smartBiquad
	nextFrame, updateFrame int64
	ready                  bool
}

// RBJ audio-EQ cookbook biquads: each stereo channel has independent history.
type smartBiquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             [2]float64
}

func (b *smartBiquad) set(b0, b1, b2, a0, a1, a2 float64) {
	b.b0, b.b1, b.b2, b.a1, b.a2 = b0/a0, b1/a0, b2/a0, a1/a0, a2/a0
}

func (b *smartBiquad) sample(x float64, ch int) float64 {
	y := b.b0*x + b.z1[ch]
	b.z1[ch] = b.b1*x - b.a1*y + b.z2[ch]
	b.z2[ch] = b.b2*x - b.a2*y
	return y
}

func (b *smartBiquad) shelf(frequency, db float64, high bool) {
	a := math.Pow(10, db/40)
	sin, cos := math.Sincos(2 * math.Pi * frequency / outputRate)
	alpha := sin / math.Sqrt2 // Q = 1/sqrt(2), as in the MA shelf tool.
	beta := 2 * math.Sqrt(a) * alpha
	if high {
		b.set(a*((a+1)+(a-1)*cos+beta), -2*a*((a-1)+(a+1)*cos), a*((a+1)+(a-1)*cos-beta),
			(a+1)-(a-1)*cos+beta, 2*((a-1)-(a+1)*cos), (a+1)-(a-1)*cos-beta)
	} else {
		b.set(a*((a+1)-(a-1)*cos+beta), 2*a*((a-1)-(a+1)*cos), a*((a+1)-(a-1)*cos-beta),
			(a+1)+(a-1)*cos+beta, -2*((a-1)+(a+1)*cos), (a+1)+(a-1)*cos-beta)
	}
}

func (b *smartBiquad) peak(frequency, widthOct, db float64) {
	a := math.Pow(10, db/40)
	w := 2 * math.Pi * frequency / outputRate
	sin, cos := math.Sincos(w)
	alpha := sin * math.Sinh(math.Ln2/2*widthOct*w/sin)
	b.set(1+alpha*a, -2*cos, 1-alpha*a, 1+alpha/a, -2*cos, 1-alpha/a)
}
