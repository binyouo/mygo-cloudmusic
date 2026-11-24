package player

import (
	"math"
	"sort"
)

func (t *deviceMixTiming) analyzeStructure(p *mixBandProfile, voice []float64, duration float64) {
	const head = float64(mixEdge) / outputRate
	if len(voice) > 0 {
		t.head = deviceVocalMask{
			known:     true,
			placement: deviceVocalWindows(voice, duration, t.bpm, 0, head, true),
			scoring:   deviceVocalWindows(voice, duration, t.bpm, 0, head, false),
		}
		end := int64(math.Round((t.bufferOffset + t.audioEnd) * outputRate))
		t.tail = deviceVocalMask{
			known:     true,
			placement: clipVocalEnd(deviceVocalWindows(voice, duration, t.bpm, t.bufferOffset, duration, true), end),
			scoring:   clipVocalEnd(deviceVocalWindows(voice, duration, t.bpm, t.bufferOffset, duration, false), end),
		}
	}
	if p == nil {
		return
	}
	first, end := sort.SearchFloat64s(p.starts, t.bufferOffset), sort.SearchFloat64s(p.starts, duration)
	t.fadeOnset = p.detectMasteredFade(first, end)
	tailStart := int64(math.Round(t.bufferOffset * outputRate))
	length := int64(math.Round(duration * outputRate))
	if vocalWindowDuty(t.tail.placement, tailStart, length) >= .9 {
		return
	}
	earliest := t.bufferOffset + t.anchor
	if p.lowShare >= .10 {
		if kick := p.lastKick(); kick >= 0 {
			earliest = kick + p.bar
		}
	}
	if n := len(t.tail.placement); n > 0 {
		earliest = max(earliest, float64(t.tail.placement[n-1].end)/outputRate)
	}
	t.coda = p.detectCoda(first, end, earliest, t.fadeOnset, duration, t.tail.placement)
}

func clipVocalEnd(windows []mixVocalWindow, end int64) []mixVocalWindow {
	for i, w := range windows {
		if w.start >= end {
			return windows[:i]
		}
		windows[i].end = min(w.end, end)
	}
	return windows
}

// A mastering fade loses power over consecutive bars while retaining the
// same distribution across frequency bands. This is distinct from the kick
// dropping out or the arrangement thinning ahead of a coda.
func (p *mixBandProfile) detectMasteredFade(first, end int) float64 {
	const minBars, drop, jitter = 4, 10, .5
	if end-first < minBars || p.level < .0001 {
		return -1
	}
	floor := .01 * p.level
	last := end - 1
	for last >= first && p.totals[last] < floor {
		last--
	}
	if last < first {
		return -1
	}
	start := last
	for start > first && p.totals[start-1] >= floor {
		start--
	}
	magnitude := make([]float64, last-start+1)
	fractions := make([][mixBandCount]float64, len(magnitude))
	for i := range magnitude {
		bar := start + i
		magnitude[i] = 10 * math.Log10(p.totals[bar]/p.level)
		for band, v := range p.power[bar] {
			fractions[i][band] = v / p.totals[bar]
		}
	}
	n := len(magnitude) - 1
	for i := 0; i+minBars-1 <= n; i++ {
		if magnitude[i]-magnitude[n] < drop {
			continue
		}
		var nonRising int
		for j := i + 1; j <= n; j++ {
			if magnitude[j]-magnitude[j-1] <= jitter {
				nonRising++
			}
		}
		if float64(nonRising) < .8*float64(n-i) ||
			!mixBandSharesStable(fractions[i:], p.totals[start+i:last+1], .1*p.level) {
			continue
		}
		onset := i
		for onset+minBars <= n && magnitude[onset]-magnitude[onset+1] < jitter && magnitude[onset+1]-magnitude[n] >= drop {
			onset++
		}
		if onset > i && onset+minBars <= n && magnitude[onset+1]-magnitude[n] >= drop {
			onset++
		}
		return p.starts[start+onset]
	}
	return -1
}

func mixBandSharesStable(shares [][mixBandCount]float64, totals []float64, floor float64) bool {
	var reference [mixBandCount]float64
	prefix := shares[:min(3, len(shares))]
	for _, row := range prefix {
		for band, v := range row {
			reference[band] += v / float64(len(prefix))
		}
	}
	for i, row := range shares {
		if totals[i] < floor {
			continue
		}
		for band, v := range row {
			if math.Abs(v-reference[band]) > .15 {
				return false
			}
		}
	}
	return true
}

func (p *mixBandProfile) detectCoda(first, end int, earliest, fade, duration float64, voice []mixVocalWindow) *mixSection {
	if p.level < .0001 {
		return nil
	}
	lastKick := -1
	if floor := p.reference[mixLow]; floor >= .0001 {
		for i := end - 1; i >= first; i-- {
			if p.power[i][mixLow] >= .5*floor {
				lastKick = i
				break
			}
		}
	}
	barEnd := func(i int) float64 {
		if i+1 == len(p.starts) {
			return p.end
		}
		return p.starts[i+1]
	}
	qualifies := func(i int) bool {
		start, end := p.starts[i], barEnd(i)
		return start >= earliest && (fade < 0 || end <= fade) && p.totals[i] >= .15*p.level &&
			!mixVocalIntersects(voice, start, end, .25*(end-start))
	}
	var coda *mixSection
	for i := first; i < end; {
		if !qualifies(i) {
			i++
			continue
		}
		left := i
		for i < end && qualifies(i) {
			i++
		}
		stop := barEnd(i - 1)
		if i-left < 2 || stop-p.starts[left] < 4 || lastKick >= i ||
			mixVocalIntersects(voice, stop, duration, .25*(stop-p.starts[i-1])) {
			continue
		}
		half := left + (i-left)/2
		if mixMean(p.totals[half:i]) >= .5*mixMean(p.totals[left:half]) {
			coda = &mixSection{p.starts[left], stop}
		}
	}
	return coda
}

func mixVocalIntersects(voice []mixVocalWindow, start, end, required float64) bool {
	for _, w := range voice {
		if min(end, float64(w.end)/outputRate)-max(start, float64(w.start)/outputRate) >= required {
			return true
		}
	}
	return false
}
