package player

import (
	"math"
	"slices"
)

type mixPlan struct {
	exit, enter, overlap    int64 // original song frames; overlap is A's source span.
	smart                   bool
	tempo                   float64 // A's playback speed; B always remains at 1.
	ramp                    int64   // A's source frames before the overlap.
	sourceStart             int64   // A's original source start, including its ramp.
	full                    bool
	outAnalysis, inAnalysis *trackAnalysis
}

func keepBoundary(a, b *pcmTrack) bool {
	return a.track.ProgramID != 0 || b.track.ProgramID != 0 ||
		a.track.ID != 0 && a.track.ID == b.track.ID ||
		a.track.Album.ID != 0 && a.track.Album.ID == b.track.Album.ID
}

// planMix uses the measured outro and intro, rather than subtracting a fixed
// number of seconds from every song. Missing analysis preserves gapless audio.
func planMix(a, b *pcmTrack) (selected *mixPlan) {
	if keepBoundary(a, b) {
		return nil
	}
	aa, bb := a.analysis.Load(), b.analysis.Load()
	if aa == nil || bb == nil || aa.tail.last == 0 || bb.head.last == 0 {
		return nil
	}
	defer func() {
		if selected != nil {
			selected.outAnalysis, selected.inAnalysis = aa, bb
		}
	}()
	if aa.neural != nil && bb.neural != nil {
		// A worker runs MA's complete candidate planner. Reserve the earliest
		// possible tail start now; the returned audio owns its actual overlap.
		return &mixPlan{exit: a.dec.Len(), tempo: 1, sourceStart: aa.tail.from, smart: true, full: true}
	}
	out, in := &aa.tail, &bb.head
	if plan := planBeatMix(a, b, aa, bb); plan != nil {
		return plan
	}
	// Energy thresholds describe the mix, never what audio may be deleted.
	// Soft intros, tails and deliberate pauses keep all their original frames.
	exit, enter := a.dec.Len(), int64(0)
	// Find a sustained final decay, ignoring a single dip between drum hits.
	last := int((out.last - out.from) / mixHop)
	decay := last
	for i := last - 1; i >= 0; i-- {
		if mixEnergy(out.rms, max(0, i-5), i+1) > out.level*.7 {
			break
		}
		decay = i
	}
	natural := max(0, out.last-(out.from+int64(decay)*mixHop))
	var overlap int64
	if natural >= outputRate/2 {
		overlap = natural
		// A loud entrance needs a shorter handoff than a soft instrumental intro.
		headLevel := mixEnergy(in.rms, int(enter/mixHop), int(enter/mixHop)+25)
		if headLevel > in.level*.8 {
			overlap = overlap * 2 / 3
		}
	} else {
		// Hard endings and incompatible beats get a brief handoff at the final
		// release, not several seconds of competing full-level music.
		for i := last - 1; i >= max(0, last-75); i-- {
			if out.onset[i] > out.level*.12 {
				overlap = out.last - (out.from + int64(i)*mixHop)
				break
			}
		}
		overlap = min(overlap, outputRate*3/4)
	}
	// These are safety bounds, not a selected transition duration.
	overlap = min(overlap, 12*outputRate, (exit - out.from), (b.dec.Len()-enter)/2, a.dec.Len()/4)
	if overlap < outputRate/10 {
		return nil
	}
	// Long boundary pauses are part of the song. With neither cue trimming
	// nor time stretching, forcing a mix here would only create a volume hole.
	if in.first > overlap/2 || exit-out.last > overlap/2 {
		return nil
	}
	// Even a natural decay is not a reason to put two clearly sung phrases
	// on top of each other. Missing vocal data never invents an activity mask.
	for !vocalMixSafe(a, b, aa, bb, exit-overlap, overlap, 1) && overlap > outputRate*2/5 {
		overlap = max(outputRate*2/5, overlap/2)
	}
	return &mixPlan{exit: exit, enter: enter, overlap: overlap, tempo: 1, sourceStart: exit - overlap}
}

// planBeatMix follows Smart Fades' separation of planning and rendering:
// choose a musical source window, reach B's tempo BEFORE the overlap, then
// leave B untouched. The renderer reconciles the actual stretched PCM length.
// We assume four-beat phrases here; this is not a learned downbeat detector.
func planBeatMix(a, b *pcmTrack, aa, bb *trackAnalysis) *mixPlan {
	out, in := &aa.tail, &bb.head
	if out.beat <= 0 || in.beat <= 0 || out.confidence < .65 || in.confidence < .65 {
		return nil
	}
	tempo := float64(out.beat) / float64(in.beat)
	if !finiteMixNumber(tempo) || math.Abs(tempo-1) > .08 {
		return nil
	}
	if math.Abs(tempo-1) <= .001 {
		tempo = 1
	}
	availableTail := int64(len(aa.tailPCM) / 2)
	availableHead := int64(len(bb.headPCM) / 2)
	if availableTail == 0 || availableHead == 0 {
		return nil
	}
	exit := a.dec.Len()
	// Never transform more than half a short song, and never retain more PCM
	// than the analyzed edge. Every source frame is still played, including
	// a deliberate quiet opening or tail.
	sourceBudget := min(availableTail, exit-out.from, exit/2, int64(mixEdge))
	maxOverlap := min(exit/4, int64(float64(min(availableHead, b.dec.Len()/2))*tempo))
	outWindows, outKnown := mixVocalWindows(a, aa)
	inWindows, inKnown := mixVocalWindows(b, bb)
	bars := []int64{4, 2, 1}
	if outKnown && inKnown {
		bars = []int64{8, 4, 2, 1}
	}
	for _, count := range bars {
		overlap := count * 4 * out.beat
		// At the final speed, A's source clock advances tempo frames for each
		// output frame. Align B's own grid by shifting A's overlap start;
		// unlike a cue seek, this does not discard B's opening.
		start := exit - overlap
		nearest := out.phase + int64(math.Round((float64(start-out.phase)+float64(in.phase)*tempo)/float64(out.beat)))*out.beat
		aligned := nearest - int64(math.Round(float64(in.phase)*tempo))
		overlap = exit - aligned
		if overlap < out.beat*2 || overlap > maxOverlap {
			continue
		}
		ramp := int64(0)
		if tempo != 1 {
			beats := int64(4)
			if math.Abs(tempo-1) > .03 {
				beats = 8
			}
			ramp = min(beats*out.beat, 10*outputRate, sourceBudget-overlap)
			if ramp < out.beat*2 {
				continue
			}
		}
		if overlap+ramp > sourceBudget || aligned-ramp < out.from {
			continue
		}
		outputOverlap := int64(math.Ceil(float64(overlap) / tempo))
		if in.first > outputOverlap/2 || exit-out.last > overlap/2 {
			continue
		}
		if !vocalWindowsSafe(outWindows, inWindows, outKnown && inKnown, aligned, overlap, tempo) {
			continue
		}
		return &mixPlan{
			exit: exit, enter: 0, overlap: overlap, smart: true, tempo: tempo,
			ramp: ramp, sourceStart: aligned - ramp,
		}
	}
	return nil
}

func finiteMixNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func mixVocalWindows(p *pcmTrack, a *trackAnalysis) ([]mixVocalWindow, bool) {
	if hints := p.vocals.Load(); hints != nil && (hints.known || len(hints.windows) > 0) {
		return hints.windows, hints.known
	}
	return a.vocals, a.vocalsKnown
}

func vocalMixSafe(a, b *pcmTrack, aa, bb *trackAnalysis, start, overlap int64, tempo float64) bool {
	out, outKnown := mixVocalWindows(a, aa)
	in, inKnown := mixVocalWindows(b, bb)
	return vocalWindowsSafe(out, in, outKnown && inKnown, start, overlap, tempo)
}

func vocalWindowsSafe(out, in []mixVocalWindow, precise bool, start, overlap int64, tempo float64) bool {
	if len(out) == 0 || len(in) == 0 || overlap <= 0 || tempo <= 0 {
		return true
	}
	type interval struct{ left, right float64 }
	duration := float64(overlap) / tempo / outputRate
	var collisions []interval
	for _, a := range out {
		if a.end <= start || a.start >= start+overlap || a.end <= a.start {
			continue
		}
		leftA := float64(a.start-start) / tempo / outputRate
		rightA := float64(a.end-start) / tempo / outputRate
		for _, b := range in {
			if b.end <= 0 || b.start >= int64(duration*outputRate) || b.end <= b.start {
				continue
			}
			left := max(0, leftA, float64(b.start)/outputRate)
			right := min(duration, rightA, float64(b.end)/outputRate)
			if right > left {
				collisions = append(collisions, interval{left, right})
			}
		}
	}
	slices.SortFunc(collisions, func(a, b interval) int {
		if a.left < b.left {
			return -1
		}
		if a.left > b.left {
			return 1
		}
		return 0
	})
	var raw, weighted float64
	primitive := func(t float64) float64 {
		phase := t / duration
		return duration * (2*phase*phase - 4.0/3*phase*phase*phase)
	}
	for i := 0; i < len(collisions); {
		left, right := collisions[i].left, collisions[i].right
		i++
		for i < len(collisions) && collisions[i].left <= right {
			right = max(right, collisions[i].right)
			i++
		}
		raw += right - left
		weighted += primitive(right) - primitive(left)
	}
	// Word-timed data can enforce MA's collision ceilings. Approximate LRC
	// hints only ask for a more conservative window, with extra tolerance.
	factor := 1.5
	if precise {
		factor = 1
	}
	return raw < 2*factor && weighted < .35*factor
}
