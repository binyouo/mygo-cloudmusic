package player

import "math"

// The on-device candidate planner implements Music Assistant Smart Fades at
// e16a4688130e9779dbce091c62ed8625c86ee64b (Apache-2.0).
// See scripts/smart_fades/SOURCE.json for the pinned desktop source.
func (c *deviceTransition) planEQ(p *maTransitionPlan, tier deviceMixTier) {
	if p.Style != "blend" && (p.Style != "segue" || p.FadeOutCurve == "nofade" || p.FadeInCurve == "nofade") {
		return
	}
	duration := p.CrossfadeDuration
	swap := c.in.entry - p.Enter
	if swap <= 0 || swap > duration {
		swap = .6 * duration
	}
	nearest, distance := swap, math.Inf(1)
	for _, beat := range c.in.headDownbeats {
		at := float64(float32(beat) - float32(p.Enter))
		if d := math.Abs(at - swap); at > 0 && at < duration && d < distance {
			nearest, distance = at, d
		}
	}
	width := min(max(.5*duration, 2*c.in.bar), 8*c.in.bar, duration)
	swap = min(nearest, duration-width/2)
	ratio := 1.0
	if n := len(p.Tempo.Steps); n > 0 {
		ratio = p.Tempo.Steps[n-1][1]
	}
	crossStart := p.FadeOutWindow - duration*ratio
	startIn := max(0, swap-width/2)
	startOut := max(crossStart, crossStart+(swap-width/2)*ratio)
	ease := .25 * duration

	exit := c.out.bufferOffset + p.FadeOutWindow
	entry := c.in.entry
	out, in := measuredSmartEQ(c.outBands, c.inBands, exit, entry)
	if out == nil {
		out, in = &smartEQDepths{Low: -26, High: -20}, &smartEQDepths{Low: -26, High: -20}
	}
	var aFrac, bFrac [mixBandCount]float64
	mid := 0.0
	wash := false
	if a, b := c.outBands, c.inBands; a != nil && b != nil {
		aStart, bEnd := exit-8*a.bar, entry+8*b.bar
		var aLevel, bLevel [mixBandCount]float64
		aFrac, aLevel = a.window(aStart, exit)
		bFrac, bLevel = b.window(entry, bEnd)
		depth := -6.0
		if tier == deviceFullBlend {
			depth = -8
		}
		midA := mixBandGate(aFrac[mixMid], .18, .30) * mixBandGate(a.bandDuty(mixMid, aStart, exit), .60, .85)
		midB := mixBandGate(bFrac[mixMid], .18, .30) * mixBandGate(b.bandDuty(mixMid, entry, bEnd), .60, .85)
		mid = depth * min(midA, midB)
		if duration >= 8*c.out.bar && aFrac[mixHigh] >= .06 && bFrac[mixHigh] >= .06 &&
			a.bandDuty(mixHigh, aStart, exit) >= .8 && b.bandDuty(mixHigh, entry, bEnd) >= .8 &&
			aLevel[mixHigh] > 0 && bLevel[mixHigh] > 0 {
			gap := 10 * math.Log10((aLevel[mixHigh]/a.meanLevel)/(bLevel[mixHigh]/b.meanLevel))
			wash = math.Abs(gap) <= 6
		}
	}
	lowRamp := func(w float64) {
		startIn := max(0, swap-w/2)
		startOut := max(crossStart, crossStart+(swap-w/2)*ratio)
		p.EQ.LowOut = mixShelf("lowshelf", 100, .707, startOut, w*ratio, 0, out.Low, 6)
		p.EQ.LowIn = mixShelf("lowshelf", 100, .707, startIn, w, in.Low, 0, 6)
	}
	lowRamp(width)
	highOut := startOut + width*ratio
	if wash {
		out.High = -26
		highOut = crossStart + max(0, startIn-ease)*ratio
	}
	p.EQ.HighOut = mixShelf("highshelf", 13000, .707, highOut, ease*ratio, 0, out.High, 6)
	p.EQ.HighIn = mixShelf("highshelf", 13000, .707, max(0, startIn-ease), ease, in.High, 0, 6)
	midRamp := func(depth float64) {
		p.EQ.MidOut = mixShelf("equalizer", 1200, 2.5, startOut, width*ratio, 0, depth, 1)
		p.EQ.MidIn = mixShelf("equalizer", 1200, 2.5, startIn, width, depth, 0, 1)
	}
	midRamp(mid)
	if c.outBands == nil || c.inBands == nil {
		return
	}
	notch := mixSection{-1, -1}
	if p.EQ.LowOut != nil || p.EQ.LowIn != nil {
		notch = mixSection{startIn, startIn + width}
	}
	dip := func() float64 {
		return mixPredictedDip(p, crossStart, ratio, aFrac, bFrac, notch)
	}
	if dip() <= 3 {
		return
	}
	for _, s := range []float64{.75, .5, .25, 0} {
		midRamp(mid * s)
		if dip() <= 3 {
			return
		}
	}
	if p.EQ.LowOut != nil || p.EQ.LowIn != nil {
		lowRamp(2 * c.in.bar)
	}
}

func mixShelf(kind string, hz, width, start, duration, from, to, minimum float64) *maShelf {
	if math.Abs(from-to) < minimum {
		return nil
	}
	steps := append([][2]float64{{0, from}}, mixDBRamp(max(0, start), duration, from, to)...)
	return &maShelf{Type: kind, Frequency: hz, Width: width, Steps: steps}
}

func mixPredictedDip(p *maTransitionPlan, start, ratio float64, a, b [mixBandCount]float64, notch mixSection) float64 {
	out := [mixBandCount]*maShelf{mixLow: p.EQ.LowOut, mixMid: p.EQ.MidOut, mixHigh: p.EQ.HighOut}
	in := [mixBandCount]*maShelf{mixLow: p.EQ.LowIn, mixMid: p.EQ.MidIn, mixHigh: p.EQ.HighIn}
	power := func(time float64, schedules [mixBandCount]*maShelf, parts [mixBandCount]float64) float64 {
		var total float64
		for band, shelf := range schedules {
			gain := 1.0
			if shelf != nil {
				gain = math.Pow(10, maGainAt(shelf.Steps, time)/10)
			}
			total += parts[band] * gain
		}
		return total
	}
	var peak, drop float64
	for i := range 65 {
		phase := float64(i) / 64
		at := p.CrossfadeDuration * phase
		if at >= notch.start && at <= notch.end {
			continue
		}
		gb, ga := math.Sincos(.5 * math.Pi * phase)
		combined := ga*ga*power(start+at*ratio, out, a) + gb*gb*power(at, in, b)
		peak = max(peak, combined)
		if combined > 0 && peak > 0 {
			drop = max(drop, 10*math.Log10(peak/combined))
		}
	}
	return drop
}
