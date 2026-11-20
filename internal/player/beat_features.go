package player

import (
	"context"
	"errors"
	"math"
	"math/cmplx"
)

const (
	beatSampleRate = 22050
	beatFFT        = 1024
	beatHop        = beatSampleRate / beatFPS
	beatMelBins    = 128
	beatWindow     = 1500
	beatBorder     = 6
	beatDelay      = (beatFFT/2 + beatHop - 1) / beatHop
	beatTailPCM    = beatFFT + beatDelay*beatHop
)

type beatMelWeight struct {
	bin  int
	gain float32
}

type beatMelFilters struct {
	hann [beatFFT]float32
	bins [beatMelBins][]beatMelWeight
}

func makeBeatMelFilters() beatMelFilters {
	var filters beatMelFilters
	for i := range filters.hann {
		filters.hann[i] = float32(.5 - .5*math.Cos(2*math.Pi*float64(i)/beatFFT))
	}
	// Slaney mel is linear below 1 kHz and logarithmic above it.
	step := math.Log(6.4) / 27
	start, end := 30/(200.0/3), 15+math.Log(11000.0/1000)/step
	var hz [beatMelBins + 2]float32
	for i := range hz {
		mel := start + (end-start)*float64(i)/(beatMelBins+1)
		if mel < 15 {
			hz[i] = float32(mel * (200.0 / 3))
		} else {
			hz[i] = float32(1000 * math.Exp(step*(mel-15)))
		}
	}
	for i := range filters.bins {
		for bin := range beatFFT/2 + 1 {
			h := float32(bin) * beatSampleRate / beatFFT
			up := (h - hz[i]) / (hz[i+1] - hz[i])
			down := (hz[i+2] - h) / (hz[i+2] - hz[i+1])
			if gain := min(up, down); gain > 0 {
				filters.bins[i] = append(filters.bins[i], beatMelWeight{bin, gain})
			}
		}
	}
	return filters
}

func beatMelCenter(samples, frame int) int {
	// The desktop extractor flushes the delayed frames from its retained
	// PCM tail, whose first sample need not be on the global hop grid.
	if frame > (samples-1)/beatHop-beatDelay {
		return frame*beatHop + samples%beatHop - min(samples, beatTailPCM)%beatHop
	}
	return frame * beatHop
}

var beatMel = makeBeatMelFilters()

// beatMelFrame reproduces the centered, reflect-padded magnitude transform
// used by the desktop Beat This extractor. It consumes mono 22.05 kHz PCM.
func beatMelFrame(pcm []float32, center int, dst []float32) {
	var spectrum [beatFFT]complex128
	for i, gain := range beatMel.hann {
		at := center - beatFFT/2 + i
		if at < 0 {
			at = -at
		} else if at >= len(pcm) {
			at = 2*(len(pcm)-1) - at
		}
		spectrum[i] = complex(float64(pcm[at]*gain), 0)
	}
	fft(spectrum[:])
	var magnitude [beatFFT/2 + 1]float32
	for i := range magnitude {
		magnitude[i] = float32(cmplx.Abs(spectrum[i]) / 32) // sqrt(beatFFT)
	}
	for i, filter := range beatMel.bins {
		var sum float32
		for _, tap := range filter {
			sum += tap.gain * magnitude[tap.bin]
		}
		dst[i] = float32(math.Log1p(float64(1000 * sum)))
	}
}

// beatWindowModel runs a fixed-size mel window, where frames counts the
// meaningful rows (including the desktop model's six border frames).
type beatWindowModel func(ctx context.Context, mel []float32, frames int) (beat, downbeat []float32, err error)

func analyzeBeatGrid(ctx context.Context, pcm []float32, model beatWindowModel) (beatGrid, error) {
	if len(pcm) <= beatFFT/2 || len(pcm) > 1200*beatSampleRate {
		return beatGrid{}, errors.New("player: invalid Beat This PCM length")
	}
	const stride = beatWindow - 2*beatBorder
	frames := 1 + len(pcm)/beatHop
	beats, downbeats := make([]float32, frames), make([]float32, frames)
	window := make([]float32, beatWindow*beatMelBins)
	written := 0
	for start := -beatBorder; start < frames-beatBorder; start += stride {
		if err := ctx.Err(); err != nil {
			return beatGrid{}, err
		}
		if frames > stride && start+stride >= frames-beatBorder {
			start = frames - (beatWindow - beatBorder)
		}
		clear(window)
		for frame := max(0, start); frame < min(frames, start+beatWindow); frame++ {
			if frame%32 == 0 {
				if err := ctx.Err(); err != nil {
					return beatGrid{}, err
				}
			}
			at := (frame - start) * beatMelBins
			beatMelFrame(pcm, beatMelCenter(len(pcm), frame), window[at:at+beatMelBins])
		}
		valid := min(beatWindow, frames-start+beatBorder)
		b, d, err := model(ctx, window, valid)
		if err != nil {
			return beatGrid{}, err
		}
		if len(b) < valid || len(d) < valid {
			return beatGrid{}, errors.New("player: incomplete Beat This prediction")
		}
		end := start + valid - beatBorder
		// Keep the earlier window's prediction where the last pair overlaps.
		copy(beats[written:end], b[written-start:valid-beatBorder])
		copy(downbeats[written:end], d[written-start:valid-beatBorder])
		written = end
	}
	return decodeBeatGrid(ctx, beats, downbeats)
}
