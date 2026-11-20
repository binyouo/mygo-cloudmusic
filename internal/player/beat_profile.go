package player

import (
	"math"
	"sort"
)

func applyDeviceBeatGrid(profile *trackAnalysis, grid beatGrid) {
	if (grid.meter != 3 && grid.meter != 4) || len(grid.downbeats) == 0 {
		return
	}
	fitBeatEdge(&profile.head, grid, false)
	fitBeatEdge(&profile.tail, grid, true)
}

// Fit only the bars nearest the handoff. A single whole-song BPM may be
// wrong for an intro or outro, and loose timing should use the natural mix.
func fitBeatEdge(edge *mixEdgeAnalysis, grid beatGrid, tail bool) {
	first := sort.SearchFloat64s(grid.beats, float64(edge.first)/outputRate)
	end := sort.SearchFloat64s(grid.beats, float64(edge.last)/outputRate)
	limit := 4 * grid.meter
	if tail {
		first = max(first, end-limit)
	} else {
		end = min(end, first+limit)
	}
	if end-first < 2*grid.meter {
		return
	}
	beats := grid.beats[first:end]
	downbeat := sort.SearchFloat64s(grid.downbeats, beats[0])
	if downbeat == len(grid.downbeats) || grid.downbeats[downbeat] > beats[len(beats)-1] {
		return
	}
	var y, xy, xx float64
	xMean := float64(len(beats)-1) / 2
	for i, time := range beats {
		x := float64(i) - xMean
		y += time
		xy += x * time
		xx += x * x
	}
	period := xy / xx
	intercept := y/float64(len(beats)) - period*xMean
	if period <= 0 || !finiteMixNumber(period) {
		return
	}
	for i, time := range beats {
		if math.Abs(time-(intercept+float64(i)*period)) > .06 {
			return
		}
	}
	beat := int64(math.Round(period * outputRate))
	if beat == 0 {
		return
	}
	anchor := math.Round((grid.downbeats[downbeat] - intercept) / period)
	phase := int64(math.Round((intercept+anchor*period)*outputRate)) - edge.from
	bar := beat * int64(grid.meter)
	edge.beat, edge.phase, edge.meter, edge.confidence = beat, edge.from+(phase%bar+bar)%bar, grid.meter, 1
}
