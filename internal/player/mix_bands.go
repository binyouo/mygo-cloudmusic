package player

import (
	"math"
	"slices"
	"sort"
)

type mixBandProfile struct {
	starts    []float64
	power     [][mixBandCount]float64
	totals    []float64
	bar       float64
	end       float64
	reference [mixBandCount]float64
	level     float64 // median total power of active bars
	meanLevel float64 // mean total power of active bars
	lowShare  float64 // median fraction of total power across active bars
}

// buildMixBandProfile follows the desktop planner's power aggregation on the
// measured downbeats. Reference levels ignore bars below 1/16 of p95 power.
func buildMixBandProfile(s mixSpectrum, grid beatGrid, duration float64) *mixBandProfile {
	if len(grid.downbeats) < 8 || grid.meter <= 0 || grid.bpm <= 0 || duration <= 0 {
		return nil
	}
	for _, band := range s.bands {
		if len(band) != mixSpectrumBins {
			return nil
		}
	}
	starts := grid.downbeats
	p := &mixBandProfile{
		starts: starts,
		power:  make([][mixBandCount]float64, len(starts)),
		bar:    float64(grid.meter) * 60 / grid.bpm,
	}
	intervals := make([]float64, len(starts)-1)
	for i := range intervals {
		intervals[i] = starts[i+1] - starts[i]
	}
	slices.Sort(intervals)
	last := starts[len(starts)-1] + mixPercentile(intervals, .5)
	p.end = last
	totals := make([]float64, len(starts))
	p.totals = totals
	binWidth := duration / mixSpectrumBins
	bin := func(t float64) int { return min(mixSpectrumBins, max(0, int(t/binWidth))) }
	for band, values := range s.bands {
		var sum [mixSpectrumBins + 1]float64
		for i, v := range values {
			sum[i+1] = sum[i] + v*v
		}
		for i, start := range starts {
			end := last
			if i+1 < len(starts) {
				end = starts[i+1]
			}
			first, next := bin(start), bin(end)
			power := (sum[next] - sum[first]) / float64(max(1, next-first))
			p.power[i][band] = power
			totals[i] += power
		}
	}
	sorted := slices.Clone(totals)
	slices.Sort(sorted)
	if sorted[len(sorted)-1] <= 0 {
		return nil
	}
	floor := mixPercentile(sorted, .95) / 16
	active := make([]float64, 0, len(totals))
	for band := range p.reference {
		active = active[:0]
		for i, total := range totals {
			if total > 0 && total >= floor {
				active = append(active, p.power[i][band])
			}
		}
		slices.Sort(active)
		p.reference[band] = mixPercentile(active, .5)
	}
	active = active[:0]
	for i, total := range totals {
		if total > 0 && total >= floor {
			active = append(active, p.power[i][mixLow]/total)
		}
	}
	slices.Sort(active)
	p.lowShare = mixPercentile(active, .5)
	firstActive := sort.Search(len(sorted), func(i int) bool { return sorted[i] > 0 && sorted[i] >= floor })
	p.level = mixPercentile(sorted[firstActive:], .5)
	p.meanLevel = mixMean(sorted[firstActive:])
	return p
}

func mixPercentile(sorted []float64, q float64) float64 {
	at := float64(len(sorted)-1) * q
	i := int(at)
	return sorted[i] + (sorted[min(i+1, len(sorted)-1)]-sorted[i])*(at-float64(i))
}

func (p *mixBandProfile) window(start, end float64) (fraction, level [mixBandCount]float64) {
	lo, hi := sort.SearchFloat64s(p.starts, start), sort.SearchFloat64s(p.starts, end)
	var total float64
	for _, powers := range p.power[lo:hi] {
		for band, v := range powers {
			level[band] += v
			total += v
		}
	}
	if total > 0 {
		for band, v := range level {
			fraction[band] = v / total
			level[band] = v / float64(hi-lo)
		}
	}
	return fraction, level
}

// The eight-bar bass/high decision and audibility floor use the desktop
// planner's measured policy. The renderer supplies the shelf timing.
func measuredSmartEQ(outgoing, incoming *mixBandProfile, exit, entry float64) (out, in *smartEQDepths) {
	if outgoing == nil || incoming == nil {
		return nil, nil
	}
	a, aLevel := outgoing.window(exit-8*outgoing.bar, exit)
	b, bLevel := incoming.window(entry, entry+8*incoming.bar)
	out = &smartEQDepths{Low: -26 * mixBandGate(b[mixLow], .10, .25), High: -20 * mixBandGate(b[mixHigh], .02, .06)}
	in = &smartEQDepths{Low: -26 * mixBandGate(a[mixLow], .10, .25), High: -20 * mixBandGate(a[mixHigh], .02, .06)}
	// Keep the incoming deck above −6 dB predicted broadband attenuation.
	const floorDB = -6
	floor := math.Pow(10, floorDB/10.0)
	rest := 1 - b[mixLow]
	if rest < floor && b[mixLow]*math.Pow(10, in.Low/10)+rest < floor {
		in.Low = 10 * math.Log10((floor-rest)/b[mixLow])
	}
	for _, d := range []*smartEQDepths{out, in} {
		if d.Low > floorDB {
			d.Low = 0
		}
		if d.High > floorDB {
			d.High = 0
		}
	}
	if aLevel[mixHigh] < .25*outgoing.reference[mixHigh] {
		out.High = 0
	}
	if bLevel[mixHigh] < .25*incoming.reference[mixHigh] {
		in.High = 0
	}
	return out, in
}

func mixBandGate(x, lo, hi float64) float64 {
	t := min(1, max(0, (x-lo)/(hi-lo)))
	return t * t * (3 - 2*t)
}
