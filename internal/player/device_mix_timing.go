package player

import (
	"math"
	"slices"
	"sort"
)

const minSmartTail = 8.0 // seconds of material required by the desktop planner

type deviceMixTiming struct {
	bpm                              float64
	meter                            int
	entry                            float64 // incoming groove cue in media time
	bufferOffset                     float64 // start of the decoded tail in media time
	audioEnd                         float64 // RMS-audible boundary in buffer-local time
	anchor                           float64 // default, kick-aware buffer-local cue
	blendable                        bool    // regular downbeats up to the anchor
	fadeOnset                        float64 // mastered fade onset in media time; -1 when absent
	coda                             *mixSection
	head, tail                       deviceVocalMask
	duration, buffer, bar, sustained float64
	rms                              []float64
	tailBeats, tailDownbeats         []float64 // buffer-local
	headBeats, headDownbeats         []float64 // media time
}

type mixSection struct{ start, end float64 }

type deviceVocalMask struct {
	known              bool
	placement, scoring []mixVocalWindow // media frames
}

type deviceMixTier uint8

const (
	deviceQuickFade deviceMixTier = iota
	deviceTempoBlend
	deviceFullBlend
)

func chooseDeviceMixTier(out, in *deviceMixTiming, a, b mixKey) deviceMixTier {
	return chooseDeviceMixTierAt(out, in, a, b, out.blendable)
}

func chooseDeviceMixTierAt(out, in *deviceMixTiming, a, b mixKey, blendable bool) deviceMixTier {
	if out.meter != in.meter || math.Abs(1-in.bpm/out.bpm)*100 > 8 || !blendable {
		return deviceQuickFade
	}
	if out.meter == 4 && a.wheel != 0 && b.wheel != 0 && !keysClash(a, b) {
		return deviceFullBlend
	}
	return deviceTempoBlend
}

// buildDeviceMixTiming carries the desktop's RMS and kick anchors into the
// device planner. The outgoing origin follows the available decoded buffer.
func buildDeviceMixTiming(rms []float64, bands *mixBandProfile, grid beatGrid, duration, buffer float64) *deviceMixTiming {
	if len(rms) != mixSpectrumBins || grid.bpm <= 0 || len(grid.beats) == 0 || duration <= 0 || buffer <= 0 {
		return nil
	}
	meter := grid.meter
	if meter == 0 {
		meter = 4
	}
	bar := float64(meter) * 60 / grid.bpm
	bin := duration / float64(len(rms))
	offset := max(0, duration-buffer)
	first := min(len(rms), int(offset/bin))
	sustained := mixSustainedEnergy(rms)
	audioEnd := lastMixEnergyBin(rms, first, bin, offset, buffer, max(.02, .05*sustained))
	var anchor float64
	tailBeats, headBeats := mixPlannerGrids(grid.beats, offset)
	downbeats, headDownbeats := mixPlannerGrids(grid.downbeats, offset)
	if audioEnd >= minSmartTail {
		mixOut := buffer
		if sustained > 0 {
			width := max(1, int(math.RoundToEven(bar/bin)))
			mixOut = lastSmoothedMixBin(rms, first, width, bin, offset, buffer, .70*sustained)
		}
		if bands != nil && bands.lowShare >= .10 {
			kick := bands.lastKick() - offset
			if kick >= minSmartTail && math.Abs(kick-mixOut)/bar <= 16 {
				mixOut = kick
			}
		}
		anchor = audioEnd
		if mixOut >= minSmartTail {
			anchor = min(anchor, mixOut)
		}
		if anchor >= buffer-.5 {
			anchor = buffer
		} else if i := mixBeatUpperBound(downbeats, float64(float32(anchor))); i > 0 && anchor-downbeats[i-1] < 2*bar {
			anchor = downbeats[i-1]
		}
	}
	return &deviceMixTiming{
		bpm: grid.bpm, meter: meter, entry: deviceGrooveEntry(rms, bin, sustained, bands, grid.downbeats),
		bufferOffset: offset, audioEnd: audioEnd, anchor: anchor, blendable: anchor >= minSmartTail && mixGridBlendable(downbeats, anchor), fadeOnset: -1,
		duration: duration, buffer: buffer, bar: bar, rms: rms, sustained: sustained,
		tailBeats: tailBeats, tailDownbeats: downbeats, headBeats: headBeats, headDownbeats: headDownbeats,
	}
}

func mixPlannerGrids(grid []float64, offset float64) (tail, head []float64) {
	for _, value := range grid {
		// Match the desktop's float32 grid and scalar operations.
		at := float32(value)
		if at <= mixEdge/outputRate {
			head = append(head, float64(at))
		}
		if local := at - float32(offset); local >= 0 {
			tail = append(tail, float64(local))
		}
	}
	return tail, head
}

func mixGridBlendable(grid []float64, anchor float64) bool {
	anchored := grid[:mixBeatUpperBound(grid, float64(float32(anchor)))]
	return len(anchored) >= 8 && mixIntervalDeviation(anchored) < .1
}
func mixBeatUpperBound(beats []float64, end float64) int {
	return sort.Search(len(beats), func(i int) bool { return beats[i] > end })
}

func mixSustainedEnergy(rms []float64) float64 {
	active := make([]float64, 0, len(rms))
	for _, v := range rms {
		if v > .01 {
			active = append(active, v)
		}
	}
	if len(active) == 0 {
		return 0
	}
	slices.Sort(active)
	return float64(float32(mixPercentile(active, .5)))
}

func lastMixEnergyBin(rms []float64, first int, bin, offset, buffer, floor float64) float64 {
	floor = float64(float32(floor))
	for i := len(rms) - 1; i >= first; i-- {
		if rms[i] > floor {
			return min(float64(i+1)*bin-offset, buffer)
		}
	}
	return 0
}

func lastSmoothedMixBin(rms []float64, first, width int, bin, offset, buffer, floor float64) float64 {
	window := make([]float64, width)
	for i := len(rms) - 1; i >= first; i-- {
		if smoothedMixBin(rms, window, i) >= float64(float32(floor)) {
			return min(float64(i+1)*bin-offset, buffer)
		}
	}
	return 0
}

func smoothedMixBin(rms, window []float64, at int) float64 {
	for j := range window {
		window[j] = rms[min(len(rms)-1, max(0, at-len(window)/2+j))]
	}
	slices.Sort(window)
	return float64(float32(mixPercentile(window, .5)))
}

func (p *mixBandProfile) lastKick() float64 {
	threshold := .5 * p.reference[mixLow]
	for i := len(p.starts) - 1; i >= 0; i-- {
		if p.power[i][mixLow] >= threshold {
			return p.starts[i]
		}
	}
	return -1
}

func deviceGrooveEntry(rms []float64, bin, sustained float64, bands *mixBandProfile, beats []float64) float64 {
	const head = float64(mixEdge) / outputRate
	if bands != nil && bands.lowShare >= .10 {
		threshold := .5 * bands.reference[mixLow]
		for i := 4; i+1 < len(bands.starts) && bands.starts[i] < head; i++ {
			var precedingSum float64
			for _, powers := range bands.power[i-4 : i] {
				precedingSum += powers[mixLow]
			}
			level := bands.power[i][mixLow]
			if level >= threshold && level >= precedingSum && bands.power[i+1][mixLow] >= threshold {
				return bands.starts[i]
			}
		}
	}
	beats = beats[:mixBeatUpperBound(beats, head)]
	if len(beats) < 12 {
		return 0
	}
	levels := make([]float64, len(beats)-1)
	i := 0
	for bar := range levels {
		start, end := float64(float32(beats[bar])), float64(float32(beats[bar+1]))
		for i < len(rms) && (float64(i)+.5)*bin < start {
			i++
		}
		first := i
		var sum float32
		for i < len(rms) && (float64(i)+.5)*bin < end {
			sum += float32(rms[i])
			i++
		}
		levels[bar] = float64(sum / float32(max(1, i-first)))
	}
	sigma := mixIntervalDeviation(levels)
	if sigma == 0 {
		sigma = 1e-6
	}
	for i := 1; i < len(levels)-4; i++ {
		pre := mixMean(levels[max(0, i-4):i])
		post := mixMean(levels[i : i+4])
		if levels[i]-pre > 1.5*sigma && post > max(pre*1.5, .5*sustained) {
			return float64(float32(beats[i]))
		}
	}
	return 0
}

func mixMean(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func mixIntervalDeviation(values []float64) float64 {
	mean := (values[len(values)-1] - values[0]) / float64(len(values)-1)
	var squared float64
	for i := 1; i < len(values); i++ {
		delta := values[i] - values[i-1] - mean
		squared += delta * delta
	}
	return math.Sqrt(squared / float64(len(values)-1))
}
