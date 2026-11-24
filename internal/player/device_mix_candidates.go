package player

import (
	"math"
	"slices"
	"sort"
)

const automaticMixEntry = -1.0

type deviceCandidateSpec struct {
	style                 string // empty selects cut or blend from the tier at the candidate's anchor
	anchor, entry         float64
	bars, idealBars       int
	overlap, idealOverlap float64 // segue only
	vocalOnset            bool
}

type deviceCandidate struct {
	spec deviceCandidateSpec
	plan maTransitionPlan
	tier deviceMixTier
	bars int
}

var mixBarRungs = [...]int{16, 8, 4, 2, 1}

func (c *deviceTransition) ladder(tier deviceMixTier) []int {
	ideal := 8
	switch tier {
	case deviceQuickFade:
		ideal = 1
		if c.bpmGap <= 12 {
			ideal = 4
		} else if c.bpmGap <= 20 {
			ideal = 2
		}
		if c.crossMeter {
			ideal = min(ideal, 2)
		}
	case deviceFullBlend:
		if c.out.tail.known && c.in.head.known &&
			mixSectionCoverage(c.voiceOut, 0, c.out.audioEnd)/max(.001, c.out.audioEnd) <= .05 &&
			mixSectionCoverage(c.voiceIn, 0, deviceMixHead)/deviceMixHead <= .05 {
			ideal = 16
		}
	}
	return mixBarRungs[slices.Index(mixBarRungs[:], ideal):]
}

func (c *deviceTransition) tierAt(anchor float64) deviceMixTier {
	return chooseDeviceMixTierAt(c.out, c.in, c.outKey, c.inKey, mixGridBlendable(c.out.tailDownbeats, anchor))
}

func (c *deviceTransition) lastVocal() float64 {
	if v := c.out.tail.placement; len(v) > 0 {
		return min(c.out.audioEnd, float64(v[len(v)-1].end)/outputRate-c.out.bufferOffset)
	}
	return 0
}

func (c *deviceTransition) protectiveAnchor(target float64, earliest bool) float64 {
	grid := c.protective[sort.SearchFloat64s(c.protective, target):]
	grid = grid[:mixBeatUpperBound(grid, c.out.audioEnd)]
	if len(grid) == 0 {
		return min(target, c.out.audioEnd)
	}
	if earliest {
		return grid[0]
	}
	return grid[len(grid)-1]
}

func (c *deviceTransition) fadePin() float64 {
	if c.out.fadeOnset < 0 {
		return c.out.anchor
	}
	at := max(c.out.fadeOnset-c.out.bufferOffset, c.lastVocal())
	if at < minSmartTail {
		return c.out.anchor
	}
	return min(c.out.anchor, at)
}

func (c *deviceTransition) entryOptions(bars int) []float64 {
	entries := make([]float64, 0, 3)
	deep := c.in.entry - float64(bars)*c.in.bar
	if deep > 0 && len(c.in.headDownbeats) > 0 {
		at := nearestMixBeat(c.in.headDownbeats, deep)
		if at >= 0 && !mixPointInVoice(c.in.head.placement, at) {
			entries = append(entries, at)
		}
	}
	if bars <= 2 && c.in.entry > 0 {
		entries = appendUniqueEntry(entries, 0)
	}
	return appendUniqueEntry(entries, c.in.entry)
}

func appendUniqueEntry(values []float64, at float64) []float64 {
	if !slices.Contains(values, at) {
		return append(values, at)
	}
	return values
}

func mixPointInVoice(windows []mixVocalWindow, at float64) bool {
	for _, w := range windows {
		if float64(w.start)/outputRate <= at && float64(w.end)/outputRate > at {
			return true
		}
	}
	return false
}

func (c *deviceTransition) candidateSpecs(rescue bool) []deviceCandidateSpec {
	ladder := c.ladder(c.tier)
	var specs []deviceCandidateSpec
	add := func(anchor float64, bars, ideal int, entries []float64, style string) {
		for _, entry := range entries {
			specs = append(specs, deviceCandidateSpec{anchor: anchor, entry: entry, bars: bars, idealBars: ideal, style: style})
		}
	}
	automatic := []float64{automaticMixEntry}
	withAuto := func(bars int) []float64 {
		return append([]float64{automaticMixEntry}, c.entryOptions(bars)...)
	}
	late := func() {
		ladder := c.ladder(c.tierAt(c.out.audioEnd))
		for _, bars := range ladder {
			add(c.out.audioEnd, bars, ladder[0], automatic, "")
		}
	}
	if rescue {
		late()
		for _, bars := range ladder {
			if bars <= 2 {
				target := min(c.out.audioEnd, max(c.out.audioEnd-float64(bars)*c.out.bar, c.lastVocal()))
				add(c.protectiveAnchor(target, false), bars, ladder[0], withAuto(bars), "")
			}
		}
	} else {
		for _, bars := range ladder {
			add(c.out.anchor, bars, ladder[0], automatic, "")
		}
		pin := c.fadePin()
		if pin < c.out.anchor {
			for _, bars := range ladder {
				add(pin, bars, ladder[0], c.entryOptions(bars), "")
			}
		}
		if zone := c.out.coda; zone != nil && c.out.tail.known && c.outBands != nil {
			ends := mixBeatUpperBound(c.outBands.starts, zone.end)
			if ends > 0 && c.outBands.starts[ends-1] >= zone.start {
				anchor := c.outBands.starts[ends-1] - c.out.bufferOffset
				available := int((zone.end - zone.start) / c.out.bar)
				for _, top := range mixBarRungs[:4] {
					if top <= available && anchor >= minSmartTail {
						add(anchor, top, top, c.entryOptions(top), "")
						if top != 2 {
							add(anchor, 2, top, c.entryOptions(2), "")
						}
						break
					}
				}
			}
		}
		if len(c.out.tail.placement) > 0 {
			anchor := c.protectiveAnchor(c.lastVocal(), true)
			for _, bars := range ladder {
				add(anchor, bars, ladder[0], withAuto(bars), "")
				if target := c.out.audioEnd - float64(bars)*c.out.bar; target > anchor {
					if other := c.protectiveAnchor(target, false); other != anchor {
						add(other, bars, ladder[0], withAuto(bars), "")
					}
				}
			}
		}
		if voice := c.in.head.placement; len(voice) > 0 && vocalWindowDuty(voice, 0, mixEdge) < .9 {
			ideal := ladder[0]
			entry := float64(voice[0].start)/outputRate - float64(ideal)*c.in.bar
			if entry >= 0 && !mixPointInVoice(voice, entry) {
				specs = append(specs, deviceCandidateSpec{anchor: pin, entry: entry, bars: ideal, idealBars: ideal, vocalOnset: true})
				if len(c.out.tail.placement) > 0 {
					protect := c.protectiveAnchor(c.lastVocal(), true)
					if protect != pin && protect != c.out.anchor {
						specs = append(specs, deviceCandidateSpec{anchor: protect, entry: entry, bars: ideal, idealBars: ideal, vocalOnset: true})
					}
				}
			}
		}
	}
	specs = append(specs, c.segueSpecs(rescue)...)
	if !rescue && c.out.audioEnd-c.out.anchor >= 8 {
		late()
	}
	if c.tier == deviceQuickFade {
		energy := c.out.anchor
		if len(c.protective) > 0 {
			energy = nearestMixBeat(c.protective, energy)
		}
		anchors := func(length, margin float64) []float64 {
			positions := []float64{energy}
			if len(c.out.tail.placement) > 0 {
				positions = appendUniqueEntry(positions, c.protectiveAnchor(c.lastVocal()+margin, true))
			}
			return appendUniqueEntry(positions, c.protectiveAnchor(c.out.audioEnd-length, false))
		}
		filterBars := []int{4, 2}
		if c.crossMeter {
			filterBars = filterBars[1:]
		}
		for _, bars := range filterBars {
			for _, anchor := range anchors(float64(bars)*c.out.bar, 0) {
				entries := automatic
				if anchor != energy {
					entries = withAuto(bars)
				}
				add(anchor, bars, filterBars[0], entries, "filter_out")
			}
		}
		length := 4 * 60 / c.out.bpm
		for _, anchor := range anchors(length, length) {
			add(anchor, 1, 1, automatic, "echo_out")
		}
	}
	return specs
}

func (c *deviceTransition) segueSpecs(rescue bool) []deviceCandidateSpec {
	if !c.collisionReliable || c.tier != deviceQuickFade && !rescue {
		return nil
	}
	floor := max(2, float64(c.ladder(deviceQuickFade)[0])*c.out.bar)
	quiet := min(c.quietTail+c.quietHead, 15, c.out.audioEnd)
	ideal := quiet
	end := c.tailEnd(c.out.audioEnd)
	span := min(15, end)
	if (c.kicklessOut || c.kicklessIn) && c.out.tail.known && c.in.head.known && span > 0 &&
		mixSectionCoverage(c.voiceOut, end-span, end)/span <= .1 && mixSectionCoverage(c.voiceIn, 0, span)/span <= .1 {
		ideal = min(15, c.out.audioEnd)
	}
	if ideal < floor {
		return nil
	}
	step := 2.0
	if c.quietTailSnapped {
		step = 2 * c.out.bar
	}
	var lengths []float64
	for n := ideal; n > floor && len(lengths) < 6; n -= step {
		lengths = append(lengths, n)
	}
	lengths = append(lengths, floor)
	if ideal > quiet && quiet >= floor {
		lengths = appendUniqueEntry(lengths, quiet)
		slices.Sort(lengths)
		slices.Reverse(lengths)
	}
	specs := make([]deviceCandidateSpec, 0, len(lengths))
	for _, length := range lengths {
		specs = append(specs, deviceCandidateSpec{style: "segue", anchor: c.out.audioEnd, bars: 1, idealBars: 1, overlap: length, idealOverlap: ideal})
	}
	return specs
}

func (c *deviceTransition) tailEnd(anchor float64) float64 {
	if anchor >= c.out.buffer-.5 {
		return c.out.buffer
	}
	return anchor
}

func newDevicePlan(style string, end, overlap float64) maTransitionPlan {
	return maTransitionPlan{Version: 1, Applicable: true, Style: style, FadeOutWindow: end, CrossfadeDuration: overlap, FadeOutCurve: "qsin", FadeInCurve: "qsin"}
}

func (c *deviceTransition) candidate(s deviceCandidateSpec, stretch bool) (deviceCandidate, bool) {
	end := c.tailEnd(min(s.anchor, c.out.audioEnd))
	result := deviceCandidate{spec: s, tier: c.tier, bars: s.bars}
	if s.style == "segue" {
		p := newDevicePlan(s.style, end, s.overlap)
		within := s.overlap <= c.quietTail+c.quietHead+1e-6
		if (within && c.out.quietOver(c.out.bufferOffset+end-c.out.bar, c.out.bufferOffset+end)) || c.keepMasterFade(&p) {
			p.FadeOutCurve = "nofade"
		}
		if within && c.in.quietOver(0, c.in.bar) {
			p.FadeInCurve = "nofade"
		}
		result.plan = p
		return result, true
	}
	if s.style == "echo_out" {
		beat := 60 / c.out.bpm
		first, last := sort.SearchFloat64s(c.protective, beat), mixBeatUpperBound(c.protective, c.out.buffer-4*beat)
		if last <= first || len(c.in.headDownbeats) == 0 {
			return result, false
		}
		cut := nearestMixBeat(c.protective[first:last], s.anchor-4*beat)
		end = c.tailEnd(cut + 4*beat)
		enter := c.in.headDownbeats[0]
		if enter > end-cut+2 {
			return result, false
		}
		p := newDevicePlan(s.style, end, end-cut)
		p.Enter, p.FadeOutCurve, p.FadeInCurve = enter, "nofade", "nofade"
		p.Echo = &maEcho{Cut: cut, Beat: beat}
		result.plan = p
		return result, true
	}
	if s.style != "filter_out" {
		result.tier = c.tierAt(end)
		result.bars = min(s.bars, c.ladder(result.tier)[0])
	}
	bars := result.bars
	beats := c.out.tailBeats[:mixBeatUpperBound(c.out.tailBeats, float64(float32(end)))]
	downbeats := c.out.tailDownbeats[:mixBeatUpperBound(c.out.tailDownbeats, float64(float32(end)))]
	protect := c.protective[:mixBeatUpperBound(c.protective, float64(float32(end)))]
	entry := s.entry
	if entry < 0 {
		needed := bars * c.in.meter
		if len(protect) >= bars && len(c.in.headDownbeats) >= bars {
			entry = c.in.headDownbeats[0]
		} else if len(beats) >= needed && len(c.in.headBeats) >= needed {
			entry = c.in.headBeats[0]
		} else if bars > 1 {
			return result, false
		}
	}
	duration := float64(bars) * c.in.bar
	if n := len(downbeats); n > bars && end-downbeats[n-1] < c.out.bar {
		duration = float64(float32(end) - float32(downbeats[n-1-bars]))
	}
	duration = min(duration, end)
	ratio, minBeat := 1.0, 0.0
	var steps [][2]float64
	if stretch && result.tier != deviceQuickFade && c.bpmGap > .1 && c.bpmGap <= 8 {
		steps = c.tempoSteps(beats, protect, end-duration)
		if len(steps) > 0 {
			ratio, minBeat = c.in.bpm/c.out.bpm, end-duration
		}
	}
	if len(protect) > 0 && entry >= 0 {
		start := end - duration
		first := sort.SearchFloat64s(protect, float64(float32(minBeat)))
		after := mixBeatUpperBound(protect, float64(float32(start)))
		for _, i := range []int{after - 1, after} {
			if i >= first && i < len(protect) {
				span := float64(float32(end) - float32(protect[i]))
				if entry+span/ratio <= deviceMixHead {
					duration = span
					break
				}
			}
		}
	}
	duration /= ratio
	if entry+duration > deviceMixHead {
		if entry >= 0 && bars > 1 {
			return result, false
		}
		entry = automaticMixEntry
	}
	if result.tier == deviceFullBlend && s.entry < 0 && c.in.entry-max(0, entry) > duration {
		trim, ok := c.rollingEntry(duration)
		if ok {
			entry = trim
		} else if bars > 1 {
			return result, false
		}
	}
	if entry > duration+2 {
		entry, result.spec.entry = automaticMixEntry, automaticMixEntry
	}
	style := s.style
	if style == "" {
		style = "blend"
		if result.tier == deviceQuickFade {
			style = "cut"
		}
	}
	p := newDevicePlan(style, end, duration)
	p.Enter, p.Tempo.Steps = max(0, entry), steps
	if style == "filter_out" {
		p.Highpass = &maHighpass{Start: max(0, end-duration), End: end, StartHz: 20, EndHz: 600}
	}
	result.plan = p
	return result, duration > 0
}

func (c *deviceTransition) rollingEntry(duration float64) (float64, bool) {
	entry := c.in.entry - duration
	if len(c.in.headDownbeats) > 0 {
		entry = nearestMixBeat(c.in.headDownbeats, entry)
	}
	p := c.inBands
	if p == nil {
		return entry, true
	}
	protected := func(i int) bool {
		v := p.power[i]
		return v[mixLow] < .25*p.reference[mixLow] && (v[mixLowMid] >= .4*p.reference[mixLowMid] || v[mixMid] >= .4*p.reference[mixMid])
	}
	first := sort.SearchFloat64s(p.starts, entry-1e-6)
	if first >= len(p.starts) || !protected(first) {
		return entry, true
	}
	for i := first + 1; i < len(p.starts); i++ {
		if !protected(i) {
			return p.starts[i], p.starts[i]+duration <= deviceMixHead
		}
	}
	return 0, false
}

func (c *deviceTransition) keepMasterFade(p *maTransitionPlan) bool {
	return c.out.fadeOnset >= 0 && p.FadeOutWindow-p.CrossfadeDuration >= c.out.fadeOnset-c.out.bufferOffset &&
		p.FadeOutWindow >= c.out.audioEnd-c.out.bar
}

func (t *deviceMixTiming) quietOver(start, end float64) bool {
	bin := t.duration / float64(len(t.rms))
	first := max(0, int(start/bin))
	last := min(len(t.rms), max(first+1, int(math.Ceil(end/bin))))
	if last <= first {
		return false
	}
	window := slices.Clone(t.rms[first:last])
	slices.Sort(window)
	return float64(float32(mixPercentile(window, .5))) < quietMixFraction*t.sustained
}
