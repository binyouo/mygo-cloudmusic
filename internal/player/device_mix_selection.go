package player

import (
	"cmp"
	"math"
	"slices"
	"sort"
)

func (c *deviceTransition) tempoSteps(beats, downbeats []float64, end float64) [][2]float64 {
	if end <= 0 {
		return nil
	}
	lo := max(0, end-10)
	within := func(grid []float64) []float64 {
		return grid[sort.SearchFloat64s(grid, float64(float32(lo))):mixBeatUpperBound(grid, float64(float32(end)))]
	}
	points := within(downbeats)
	if c.bpmGap > 3 || len(points) < 2 {
		points = within(beats)
	}
	if len(points) < 2 {
		points = make([]float64, max(4, int((end-lo)/c.out.bar)))
		for i := range points {
			points[i] = lo + float64(float32((end-lo)*float64(i)/float64(len(points)-1)))
		}
	} else {
		points = slices.Clone(points)
		for i, at := range points {
			points[i] = lo + float64(float32(at)-float32(lo))
		}
	}
	ratio := c.in.bpm / c.out.bpm
	change := math.Abs(ratio - 1)
	count := min(len(points), max(1, int(math.Ceil(change/.005))))
	if count == 1 {
		return [][2]float64{{points[0], math.RoundToEven(ratio*1e6) / 1e6}}
	}
	sigmoid := func(k float64, i int) float64 {
		x := 2*float64(i)/float64(count-1) - 1
		a, b := 1/(1+math.Exp(k)), 1/(1+math.Exp(-k))
		return (1/(1+math.Exp(-k*x)) - a) / (b - a)
	}
	a, b := .1, 10.0
	for range 20 {
		k := (a + b) / 2
		var peak float64
		for i := 1; i < count; i++ {
			peak = max(peak, sigmoid(k, i)-sigmoid(k, i-1))
		}
		if peak*change <= .005 {
			a = k
		} else {
			b = k
		}
	}
	steps := make([][2]float64, count)
	for i := range steps {
		at := int(math.RoundToEven(float64(i) * float64(len(points)-1) / float64(count-1)))
		speed := 1 + (ratio-1)*sigmoid(a, i)
		steps[i] = [2]float64{points[at], math.RoundToEven(speed*1e6) / 1e6}
	}
	return steps
}

func mixSectionCoverage(windows []mixSection, start, end float64) float64 {
	var seconds float64
	for _, w := range windows {
		seconds += max(0, min(w.end, end)-max(w.start, start))
	}
	return seconds
}

func mixCollision(a, b []mixSection, duration float64) (float64, float64) {
	var overlaps []mixSection
	for _, left := range a {
		start, end := max(0, left.start), min(duration, left.end)
		if end <= start {
			continue
		}
		for _, right := range b {
			at, stop := max(start, right.start), min(end, right.end)
			if at < stop {
				overlaps = append(overlaps, mixSection{at, stop})
			}
		}
	}
	slices.SortFunc(overlaps, func(a, b mixSection) int { return cmp.Compare(a.start, b.start) })
	return mixCollisionTotals(overlaps, duration)
}

func mixCollisionTotals(sorted []mixSection, duration float64) (float64, float64) {
	var seconds, weighted float64
	primitive := func(t float64) float64 {
		p := t / duration
		return duration * (2*p*p - 4.0/3*p*p*p)
	}
	for i := 0; i < len(sorted); {
		w := sorted[i]
		i++
		for i < len(sorted) && sorted[i].start <= w.end {
			w.end = max(w.end, sorted[i].end)
			i++
		}
		seconds += w.end - w.start
		weighted += primitive(w.end) - primitive(w.start)
	}
	return seconds, weighted
}

func mixScheduleTime(p *maTransitionPlan, at float64) float64 {
	at = min(p.FadeOutWindow, max(0, at))
	previous, ratio, seconds := 0.0, scheduledTempo(p.Tempo.Steps, 0), 0.0
	for _, s := range p.Tempo.Steps {
		if s[0] >= at {
			break
		}
		seconds += (s[0] - previous) / ratio
		previous, ratio = s[0], s[1]
	}
	return seconds + (at-previous)/ratio
}

func (p *maTransitionPlan) dryEnd() float64 {
	if p.Echo != nil {
		return p.Echo.Cut
	}
	return p.FadeOutWindow
}

func renderedMixTail(p *maTransitionPlan, windows []mixSection, limit float64) []mixSection {
	fadeStart := mixScheduleTime(p, p.FadeOutWindow) - p.CrossfadeDuration
	out := make([]mixSection, 0, len(windows))
	for _, w := range windows {
		if w.start < limit {
			out = append(out, mixSection{mixScheduleTime(p, w.start) - fadeStart, mixScheduleTime(p, min(w.end, limit)) - fadeStart})
		}
	}
	return out
}

func renderedMixHead(p *maTransitionPlan, windows []mixSection) []mixSection {
	out := make([]mixSection, len(windows))
	for i, w := range windows {
		out[i] = mixSection{w.start - p.Enter, w.end - p.Enter}
	}
	return out
}

func (c *deviceTransition) voiceCollision(p *maTransitionPlan) (float64, float64) {
	out := renderedMixTail(p, c.voiceOut, p.dryEnd())
	if echo := p.Echo; echo != nil {
		start := echo.Cut - echo.Beat
		for _, w := range c.voiceOut {
			left, right := max(start, w.start), min(echo.Cut, w.end)
			if right > left {
				// The first two taps are audible enough to repeat a word.
				out = append(out, mixSection{left - start, right - start}, mixSection{left - start + echo.Beat, right - start + echo.Beat})
			}
		}
	}
	return mixCollision(out, renderedMixHead(p, c.voiceIn), p.CrossfadeDuration)
}

type scoredDeviceCandidate struct {
	deviceCandidate
	kickClash, penalty float64
}

func (c *deviceTransition) score(candidate deviceCandidate) (scoredDeviceCandidate, bool) {
	score := scoredDeviceCandidate{deviceCandidate: candidate}
	p, s := &candidate.plan, candidate.spec
	overlap := p.CrossfadeDuration
	trim := max(0, c.out.audioEnd-p.FadeOutWindow)
	if overlap <= 8 && trim > overlap || mixSectionCoverage(c.voiceOut, p.dryEnd(), c.out.audioEnd) > .25 {
		return score, false
	}
	score.penalty = trim
	if c.out.tail.known && c.in.head.known && c.collisionReliable {
		seconds, weighted := c.voiceCollision(p)
		if seconds >= 2 || weighted >= .35 {
			return score, false
		}
		score.penalty += mixSquare(weighted/.35) * 20
	}
	if p.Style != "blend" && c.outBands != nil && c.inBands != nil {
		end := p.dryEnd()
		if hp := p.Highpass; hp != nil {
			end = min(end, hp.Start+(hp.End-hp.Start)*math.Log(120/hp.StartHz)/math.Log(hp.EndHz/hp.StartHz))
		}
		_, weighted := mixCollision(renderedMixTail(p, c.kickOut, end), renderedMixHead(p, c.kickIn), overlap)
		score.kickClash = weighted / c.out.bar
		if p.Style != "cut" {
			if score.kickClash > 2 {
				return score, false
			}
			score.penalty += mixSquare(score.kickClash/2) * 20
		}
	}
	if gap := c.in.entry - p.Enter - overlap - 4; gap > 0 && c.out.hotAt(p.FadeOutWindow) {
		score.penalty += gap
	}
	if p.Style == "segue" {
		score.penalty += 4 * math.Log2(s.idealOverlap/overlap)
		if c.tier != deviceQuickFade {
			score.penalty += 15
		}
	} else {
		rung := func(bars int) int { return slices.Index(mixBarRungs[:], bars) }
		score.penalty += 10 * float64(rung(candidate.bars)-rung(s.idealBars))
		score.penalty += 15 * float64(max(0, int(c.tier)-int(candidate.tier)))
		if c.tier == deviceQuickFade {
			score.penalty += 15
		}
		if len(c.protective) == 0 || math.Abs(nearestMixBeat(c.protective, p.dryEnd())-p.dryEnd()) > .05 {
			score.penalty += 4
		}
		if s.entry >= 0 && !s.vocalOnset {
			score.penalty += 2
		}
	}
	return score, true
}

func (t *deviceMixTiming) hotAt(end float64) bool {
	bin := t.duration / float64(len(t.rms))
	anchor := t.bufferOffset + end
	first := min(len(t.rms)-1, max(0, int((anchor-8)/bin)))
	last := max(first+1, min(len(t.rms), int(anchor/bin)))
	return mixMean(t.rms[first:last]) >= .178*slices.Max(t.rms)
}

func mixSquare(x float64) float64 { return x * x }

func (c *deviceTransition) selectCandidate(specs []deviceCandidateSpec, rescue bool) (*scoredDeviceCandidate, int) {
	var survivors []scoredDeviceCandidate
	built := 0
	for _, spec := range specs {
		if candidate, ok := c.candidate(spec, true); ok {
			built++
			if score, ok := c.score(candidate); ok {
				survivors = append(survivors, score)
			}
		}
	}
	best := func(keep func(*scoredDeviceCandidate) bool) *scoredDeviceCandidate {
		var current *scoredDeviceCandidate
		for i := range survivors {
			m := &survivors[i]
			if keep(m) && (current == nil || m.penalty < current.penalty) {
				current = m
			}
		}
		return current
	}
	base := best(func(m *scoredDeviceCandidate) bool { return m.plan.Style == "cut" || m.plan.Style == "blend" })
	blend := best(func(m *scoredDeviceCandidate) bool { return m.plan.Style == "blend" })
	dressedStyle := "filter_out"
	if c.crossMeter || c.bpmGap > 20 {
		dressedStyle = "echo_out"
	}
	dressed := best(func(m *scoredDeviceCandidate) bool { return m.plan.Style == dressedStyle })
	if dressed == nil {
		dressed = best(func(m *scoredDeviceCandidate) bool { return m.plan.Echo != nil || m.plan.Highpass != nil })
	}
	if base == nil {
		if !rescue {
			return nil, built
		}
		if segue := best(func(m *scoredDeviceCandidate) bool { return m.plan.Style == "segue" }); segue != nil {
			return segue, built
		}
		return dressed, built
	}
	winner := base
	if blend == nil {
		winner = best(func(m *scoredDeviceCandidate) bool {
			return m.plan.Style == "cut" || m.plan.Style == "segue" && m.plan.CrossfadeDuration >= base.plan.CrossfadeDuration
		})
	}
	if dressed != nil && winner.plan.Style == "cut" && winner.kickClash*float64(c.out.meter) > 1 {
		return dressed, built
	}
	return winner, built
}

func (c *deviceTransition) kicksInBlend(p *maTransitionPlan) bool {
	ratio := 1.0
	if n := len(p.Tempo.Steps); n > 0 {
		ratio = p.Tempo.Steps[n-1][1]
	}
	out := c.outBands == nil || mixSectionCoverage(c.kickOut, p.FadeOutWindow-p.CrossfadeDuration*ratio, p.FadeOutWindow) >= 60/c.out.bpm
	in := c.inBands == nil || mixSectionCoverage(c.kickIn, p.Enter, p.Enter+p.CrossfadeDuration) >= 60/c.in.bpm
	return out && in
}

func (c *deviceTransition) fallback() maTransitionPlan {
	protect := func(fade float64) float64 {
		end := c.tailEnd(c.out.anchor)
		vocal := c.lastVocal() > end+1e-9
		excess := fade <= 8 && c.out.audioEnd-end > fade+1e-9
		if vocal || excess {
			target := max(end, c.lastVocal())
			if excess {
				target = max(target, c.out.audioEnd-fade)
			}
			end = c.tailEnd(c.protectiveAnchor(min(target, c.out.audioEnd), vocal))
		}
		return end
	}
	end := protect(min(8, c.tailEnd(c.out.anchor)))
	p := newDevicePlan("cut", end, min(8, end))
	seconds, weighted := c.voiceCollision(&p)
	if c.collisionReliable && (seconds >= 4 || weighted >= .70) {
		base, _ := c.candidate(deviceCandidateSpec{anchor: c.out.anchor, entry: c.in.entry, bars: 1, idealBars: 1}, true)
		end = protect(base.plan.CrossfadeDuration)
		onset := 0.0
		if w := c.in.head.placement; len(w) > 0 {
			onset = float64(w[0].start) / outputRate
		}
		p = newDevicePlan("cut", end, max(.4, min(1, onset-.1)))
		if seconds, weighted = c.voiceCollision(&p); seconds >= 2 || weighted >= .35 {
			p.CrossfadeDuration = .4
		}
		return p
	}
	if c.collisionReliable && weighted >= .35 {
		p.EQ.MidOut = &maShelf{Type: "equalizer", Frequency: 1200, Width: 2.5, Steps: append([][2]float64{{0, 0}}, mixDBRamp(end-p.CrossfadeDuration, .25*p.CrossfadeDuration, 0, -8)...)}
	}
	if c.keepMasterFade(&p) {
		p.FadeOutCurve = "nofade"
	}
	return p
}

func chooseDeviceTransition(a, b *trackAnalysis) (maTransitionPlan, deviceMixTier, bool) {
	c := newDeviceTransition(a, b)
	if c.out.audioEnd < minSmartTail || c.out.anchor < minSmartTail {
		return maTransitionPlan{}, c.tier, false
	}
	winner, built := c.selectCandidate(c.candidateSpecs(false), false)
	if built == 0 {
		return maTransitionPlan{}, c.tier, false
	}
	if winner == nil {
		winner, _ = c.selectCandidate(c.candidateSpecs(true), true)
	}
	if winner == nil {
		return c.fallback(), c.tier, true
	}
	p := winner.plan
	tier := winner.tier
	if p.Style == "blend" && len(p.Tempo.Steps) > 0 && !c.kicksInBlend(&p) {
		if unstretched, ok := c.candidate(winner.spec, false); ok && !c.kicksInBlend(&unstretched.plan) {
			if _, ok := c.score(unstretched); ok {
				p = unstretched.plan
				tier = unstretched.tier
			}
		}
	}
	if p.Style != "echo_out" && p.Style != "segue" && c.keepMasterFade(&p) {
		p.FadeOutCurve = "nofade"
	}
	c.planEQ(&p, tier)
	return p, tier, true
}

func mixDBRamp(start, duration, from, to float64) [][2]float64 {
	steps := max(2, int(duration/.1))
	result := make([][2]float64, steps+1)
	for i := range result {
		phase := float64(i) / float64(steps)
		result[i] = [2]float64{start + phase*duration, from + (to-from)*phase}
	}
	return result
}
