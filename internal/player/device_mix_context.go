package player

import (
	"math"
	"slices"
	"sort"
)

const deviceMixHead = float64(mixEdge) / outputRate
const quietMixFraction = .3981071705534972 // −8 dB relative to sustained RMS

type deviceTransition struct {
	out, in                 *deviceMixTiming
	outBands, inBands       *mixBandProfile
	outKey, inKey           mixKey
	tier                    deviceMixTier
	crossMeter              bool
	bpmGap                  float64
	protective              []float64
	voiceOut, voiceIn       []mixSection // scoring windows, local to each buffer
	kickOut, kickIn         []mixSection
	kicklessOut, kicklessIn bool
	collisionReliable       bool
	quietTail, quietHead    float64
	quietTailSnapped        bool
}

func newDeviceTransition(a, b *trackAnalysis) deviceTransition {
	out, in := a.deviceTiming, b.deviceTiming
	c := deviceTransition{
		out: out, in: in, outBands: a.bands, inBands: b.bands, outKey: a.key, inKey: b.key,
		tier: chooseDeviceMixTier(out, in, a.key, b.key), crossMeter: out.meter != in.meter,
		bpmGap:     math.Abs(1-in.bpm/out.bpm) * 100,
		protective: extendDeviceDownbeats(out.tailDownbeats, out.audioEnd),
		voiceOut:   mixVocalSections(out.tail.scoring, out.bufferOffset), voiceIn: mixVocalSections(in.head.scoring, 0),
		collisionReliable: true,
	}
	if out.tail.known && in.head.known {
		start := int64(math.Round(out.bufferOffset * outputRate))
		end := int64(math.Round((out.bufferOffset + out.audioEnd) * outputRate))
		headEnd := int64(math.Round(min(deviceMixHead, in.duration) * outputRate))
		c.collisionReliable = vocalWindowDuty(out.tail.placement, start, end) < .9 ||
			vocalWindowDuty(in.head.placement, 0, headEnd) < .9
	}
	if a.bands != nil {
		end := out.bufferOffset + out.audioEnd
		c.kickOut = a.bands.kickRuns(out.bufferOffset, end, out.bufferOffset)
		c.kicklessOut = a.bands.bandDuty(mixLow, end-15, end) <= .125
	}
	if b.bands != nil {
		c.kickIn = b.bands.kickRuns(0, deviceMixHead, 0)
		c.kicklessIn = b.bands.bandDuty(mixLow, 0, 15) <= .125
	}
	endGrid := out.tailDownbeats[:mixBeatUpperBound(out.tailDownbeats, float64(float32(out.audioEnd)))]
	tail, snap := snapDeviceSegue(min(out.quietStart(), out.audioEnd), endGrid, out.bar, true)
	head, _ := snapDeviceSegue(in.quietEnd(), in.headDownbeats, in.bar, false)
	c.quietTail, c.quietHead, c.quietTailSnapped = out.audioEnd-tail, head, snap
	return c
}

func mixVocalSections(windows []mixVocalWindow, offset float64) []mixSection {
	spans := make([]mixSection, len(windows))
	for i, w := range windows {
		spans[i] = mixSection{float64(w.start)/outputRate - offset, float64(w.end)/outputRate - offset}
	}
	return spans
}

func extendDeviceDownbeats(grid []float64, end float64) []float64 {
	if len(grid) < 2 || grid[len(grid)-1] >= end-5 {
		return grid
	}
	intervals := make([]float64, len(grid)-1)
	for i := range intervals {
		intervals[i] = float64(float32(grid[i+1]) - float32(grid[i]))
	}
	slices.Sort(intervals)
	step := float64(float32(mixPercentile(intervals, .5)))
	// At two downbeats the desktop first tests against the estimated tempo,
	// then falls through to the same median-interval extrapolation.
	if step <= 0 || mixIntervalDeviation(grid) > .2 {
		return grid
	}
	last := grid[len(grid)-1]
	extended := slices.Clone(grid)
	for at := last + step; at < end && at-last <= 25; at += step {
		extended = append(extended, float64(float32(at)))
	}
	return extended
}

func (p *mixBandProfile) bandDuty(band int, start, end float64) float64 {
	first, last := sort.SearchFloat64s(p.starts, start), sort.SearchFloat64s(p.starts, end)
	floor := .5 * p.reference[band]
	count := 0
	for _, power := range p.power[first:last] {
		if power[band] >= floor {
			count++
		}
	}
	return float64(count) / float64(max(1, last-first))
}

func (p *mixBandProfile) kickRuns(start, end, offset float64) []mixSection {
	var segments []mixSection
	floor := .5 * p.reference[mixLow]
	for i, left := range p.starts {
		if left >= end {
			break
		}
		right := p.end
		if i+1 < len(p.starts) {
			right = p.starts[i+1]
		}
		left, right = max(left, start)-offset, min(right, end)-offset
		if right <= left || p.power[i][mixLow] < floor {
			continue
		}
		if n := len(segments); n > 0 && left <= segments[n-1].end {
			segments[n-1].end = right
		} else {
			segments = append(segments, mixSection{left, right})
		}
	}
	return segments
}

func (t *deviceMixTiming) quietStart() float64 {
	if t.sustained <= 0 {
		return t.buffer
	}
	bin := t.duration / float64(len(t.rms))
	width := max(1, int(math.RoundToEven(t.bar/bin)))
	return lastSmoothedMixBin(t.rms, int(t.bufferOffset/bin), width, bin, t.bufferOffset, t.buffer, quietMixFraction*t.sustained)
}

func (t *deviceMixTiming) quietEnd() float64 {
	if t.sustained <= 0 {
		return 0
	}
	bin := t.duration / float64(len(t.rms))
	window := make([]float64, max(1, int(math.RoundToEven(t.bar/bin))))
	floor := float64(float32(quietMixFraction * t.sustained))
	for i := 0; i < min(len(t.rms), max(1, int(deviceMixHead/bin))); i++ {
		if smoothedMixBin(t.rms, window, i) >= floor {
			return float64(i) * bin
		}
	}
	return min(deviceMixHead, t.duration)
}

func snapDeviceSegue(point float64, grid []float64, bar float64, later bool) (float64, bool) {
	end := mixBeatUpperBound(grid, float64(float32(point+.05)))
	if end < 4 || mixIntervalDeviation(grid[end-4:end]) >= .1 {
		return point, false
	}
	lo, hi := point-bar, point+.05
	if later {
		lo, hi = point-.05, point+bar
	}
	candidates := grid[sort.SearchFloat64s(grid, float64(float32(lo))):mixBeatUpperBound(grid, float64(float32(hi)))]
	if len(candidates) == 0 {
		return point, false
	}
	return nearestMixBeat(candidates, point), true
}

func nearestMixBeat(grid []float64, point float64) float64 {
	best, distance := grid[0], math.Inf(1)
	for _, at := range grid {
		if delta := math.Abs(at - point); delta < distance {
			best, distance = at, delta
		}
	}
	return best
}
