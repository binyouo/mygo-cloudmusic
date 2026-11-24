package player

import (
	"context"
	"errors"
	"math"
	"slices"
)

const (
	beatFPS         = 50
	beatMinInterval = 14 // round(60 * 50 / 215)
	beatMaxInterval = 55 // round(60 * 50 / 55)
	beatIntervals   = beatMaxInterval - beatMinInterval + 1
	beatStates      = (beatMinInterval + beatMaxInterval) * beatIntervals / 2
)

type beatGrid struct {
	beats, downbeats []float64 // seconds on the original song timeline
	meter            int
	bpm              float64
}

type beatActivation struct{ beat, downbeat float32 }

func beatProbability(logit float32) float32 {
	return float32(1/(1+math.Exp(-float64(logit))))*(1-1e-5) + .5e-5
}

// decodeBeatGrid is the desktop DBN downbeat tracker applied to Beat This
// logits. Only transitions between beats have a choice of predecessor.
func decodeBeatGrid(ctx context.Context, beatLogits, downbeatLogits []float32) (beatGrid, error) {
	if len(beatLogits) != len(downbeatLogits) || len(beatLogits) > 1200*beatFPS+1 {
		return beatGrid{}, errors.New("player: invalid Beat This frame count")
	}
	if len(beatLogits) == 0 {
		return beatGrid{}, nil
	}
	activations := make([]beatActivation, len(beatLogits))
	first, last := -1, 0
	for i, down := range downbeatLogits {
		if !finiteMixNumber(float64(down)) || !finiteMixNumber(float64(beatLogits[i])) {
			return beatGrid{}, errors.New("player: non-finite Beat This output")
		}
		b, d := beatProbability(beatLogits[i]), beatProbability(down)
		a := beatActivation{beat: max(b-d, .5e-5), downbeat: d}
		activations[i] = a
		if max(a.beat, a.downbeat) >= .05 {
			if first < 0 {
				first = i
			}
			last = i + 1
		}
	}
	if first >= 0 {
		activations = activations[first:last]
	} else {
		first = 0
	}

	// The three- and four-beat meter hypotheses use the same observations.
	observations := make([][3]float32, len(activations))
	for i, a := range activations {
		observations[i] = [3]float32{
			float32(math.Log(float64(max(1-a.beat-a.downbeat, 1e-7) / 15))),
			float32(math.Log(float64(max(a.beat, 1e-7)))),
			float32(math.Log(float64(max(a.downbeat, 1e-7)))),
		}
	}
	three, scoreThree, err := beatViterbi(ctx, observations, 3)
	if err != nil {
		return beatGrid{}, err
	}
	four, scoreFour, err := beatViterbi(ctx, observations, 4)
	if err != nil {
		return beatGrid{}, err
	}
	path, meter := three, 3
	if scoreFour > scoreThree {
		path, meter = four, 4
	}
	var grid beatGrid
	for at := 0; at < len(path); {
		if path[at] == 0 {
			at++
			continue
		}
		// Snap each DBN beat region to the strongest prediction in it.
		peak := at
		for ; at < len(path) && path[at] != 0; at++ {
			a, best := activations[at], activations[peak]
			if a.beat+a.downbeat > best.beat+best.downbeat {
				peak = at
			}
		}
		second := float64(peak+first) / beatFPS
		grid.beats = append(grid.beats, second)
		if path[peak] == 2 {
			grid.downbeats = append(grid.downbeats, second)
		}
	}
	if len(grid.beats) > 0 {
		grid.meter = meter
	}
	grid.bpm = beatGridBPM(grid.beats)
	return grid, nil
}

// For each beat length, precompute the probabilities of reaching it from
// each predecessor length. The desktop tracker drops log probabilities <= -50.
type beatTransition struct {
	source uint8
	log    float32
}

func beatTransitions() [beatIntervals][]beatTransition {
	var result [beatIntervals][]beatTransition
	for from := range beatIntervals {
		var row [beatIntervals]float64
		var total float64
		for to := range beatIntervals {
			ratio := float64(to+beatMinInterval) / float64(from+beatMinInterval)
			row[to] = -100 * math.Abs(ratio-1)
			total += math.Exp(row[to])
		}
		for to, unnormalized := range row {
			if logp := float32(unnormalized - math.Log(total)); logp > -50 {
				result[to] = append(result[to], beatTransition{uint8(from), logp})
			}
		}
	}
	return result
}

// beatViterbi returns the winning path's observation classes. Within a
// beat the phase always advances by one, so only its starting states need
// a back pointer. One byte per interval and beat bounds the storage even
// on a 20-minute track.
func beatViterbi(ctx context.Context, observations [][3]float32, meter int) ([]uint8, float32, error) {
	const classes = 3
	stateCount := meter * beatStates
	prev, current := make([]float32, stateCount), make([]float32, stateCount)
	var starts, ends [beatIntervals]int
	var within [beatStates]uint8
	var choose [beatStates]uint8
	for i, start := 0, 0; i < beatIntervals; i++ {
		starts[i], ends[i] = start, start+beatMinInterval+i-1
		choose[start] = uint8(i + 1)
		for phase := range beatMinInterval + i {
			if phase*16 < beatMinInterval+i {
				within[start+phase] = 1
			}
		}
		start = ends[i] + 1
	}
	for i := range prev {
		prev[i] = -float32(math.Log(float64(stateCount)))
	}
	transitions := beatTransitions()
	negInf := float32(math.Inf(-1))
	stride := meter * beatIntervals
	back := make([]uint8, len(observations)*stride)
	for frame, likelihood := range observations {
		if frame%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, err
			}
		}
		frameBack := back[frame*stride : (frame+1)*stride]
		for barBeat := range meter {
			base := barBeat * beatStates
			prevBase := (barBeat + meter - 1) % meter * beatStates
			class := 1
			if barBeat == 0 {
				class = classes - 1
			}
			for to, start := range starts {
				best, from := negInf, uint8(0)
				for _, step := range transitions[to] {
					score := prev[prevBase+ends[step.source]] + step.log
					if score > best {
						best, from = score, step.source
					}
				}
				current[base+start] = best + likelihood[class]
				frameBack[barBeat*beatIntervals+to] = from
				// The first 1/16 of each beat is the observation region.
				endBeatRegion := start + (beatMinInterval+to+15)/16
				for at := start + 1; at < endBeatRegion; at++ {
					current[base+at] = prev[base+at-1] + likelihood[class]
				}
				for at := endBeatRegion; at <= ends[to]; at++ {
					current[base+at] = prev[base+at-1] + likelihood[0]
				}
			}
		}
		prev, current = current, prev
	}
	best := 0
	for i, score := range prev {
		if score > prev[best] {
			best = i
		}
	}
	score := prev[best]
	path := make([]uint8, len(observations))
	for i := len(path) - 1; i >= 0; i-- {
		barBeat, phase := best/beatStates, best%beatStates
		path[i] = within[phase]
		if barBeat == 0 && path[i] == 1 {
			path[i] = 2
		}
		if start := choose[phase]; start != 0 {
			from := back[i*stride+barBeat*beatIntervals+int(start)-1]
			best = (barBeat+meter-1)%meter*beatStates + ends[from]
		} else {
			best--
		}
	}
	return path, score, nil
}

func beatGridBPM(beats []float64) float64 {
	const segments = 5
	if len(beats) < 2 {
		return 0
	}
	bpm := func(times []float64) float64 {
		return 60 * float64(len(times)-1) / (times[len(times)-1] - times[0])
	}
	if len(beats) < segments*2 {
		return bpm(beats)
	}
	var widths [segments]float64
	for i, at := 0, 0; i < segments; i++ {
		length := len(beats) / segments
		if i < len(beats)%segments {
			length++
		}
		widths[i] = bpm(beats[at : at+length])
		at += length
	}
	slices.Sort(widths[:])
	median := widths[segments/2]
	var all, matched float64
	var count int
	for _, rate := range widths {
		all += rate
		if math.Abs(rate-median) <= 3 {
			matched += rate
			count++
		}
	}
	if count >= 2 {
		return matched / float64(count)
	}
	return all / segments
}
