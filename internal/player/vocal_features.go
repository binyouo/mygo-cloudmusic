package player

import (
	"context"
	"errors"
	"math"
)

const (
	vocalSampleRate = 16000
	vocalWindow     = 400 // 25 ms, snip edges
	vocalHop        = 160 // 10 ms
	vocalFFT        = 512
	vocalBands      = 80
	vocalContext    = 160 // covers the FireRed model's receptive field
	vocalBatch      = 3000
)

type vocalCMVN struct {
	Means      []float64 `json:"means"`
	InverseStd []float64 `json:"inverseStd"`
}

type vocalFilterbank struct {
	window [vocalWindow]float32
	bins   [vocalBands][]beatMelWeight
}

func makeVocalFilterbank() vocalFilterbank {
	var f vocalFilterbank
	for i := range f.window {
		f.window[i] = float32(math.Pow(.5-.5*math.Cos(2*math.Pi*float64(i)/(vocalWindow-1)), .85))
	}
	mel := func(hz float32) float32 { return 1127 * float32(math.Log(float64(1+hz/700))) }
	low := mel(20)
	step := (mel(vocalSampleRate/2) - low) / (vocalBands + 1)
	for i := range f.bins {
		left, middle, right := low+float32(i)*step, low+float32(i+1)*step, low+float32(i+2)*step
		for bin := range vocalFFT / 2 {
			at := mel(float32(bin) * (vocalSampleRate / float32(vocalFFT)))
			if weight := min((at-left)/(middle-left), (right-at)/(right-middle)); weight > 0 {
				f.bins[i] = append(f.bins[i], beatMelWeight{bin, weight})
			}
		}
	}
	return f
}

var vocalFilter = makeVocalFilterbank()

func vocalFbankFrame(pcm []float32, cmvn vocalCMVN, result []float32) {
	var frame [vocalWindow]float32
	var sum float32
	for i, sample := range pcm[:vocalWindow] {
		frame[i] = float32(min(32767, max(-32768, math.RoundToEven(float64(sample)*32768))))
		sum += frame[i]
	}
	mean := sum / vocalWindow
	var spectrum [vocalFFT]complex128
	prev := frame[0] - mean
	for i, window := range vocalFilter.window {
		centered := frame[i] - mean
		spectrum[i] = complex(float64((centered-.97*prev)*window), 0)
		prev = centered
	}
	fft(spectrum[:])
	var power [vocalFFT / 2]float32
	for i := range power {
		v := spectrum[i]
		power[i] = float32(real(v)*real(v) + imag(v)*imag(v))
	}
	for band, taps := range vocalFilter.bins {
		var e float32
		for _, tap := range taps {
			e += power[tap.bin] * tap.gain
		}
		energy := float32(math.Log(float64(max(e, 1.0/(1<<23)))))
		result[band] = float32((float64(energy) - cmvn.Means[band]) * cmvn.InverseStd[band])
	}
}

type vocalWindowModel func(context.Context, []float32, int) ([]float32, error)

// analyzeVocalActivity builds the same 100 ms speech/singing series as desktop.
// The context on both sides keeps inference boundaries out of the emitted core.
func analyzeVocalActivity(ctx context.Context, pcm []float32, stats vocalCMVN, model vocalWindowModel) ([]float32, error) {
	if len(pcm) < vocalWindow || len(stats.Means) != vocalBands || len(stats.InverseStd) != vocalBands {
		return nil, errors.New("player: invalid FireRed input")
	}
	n := 1 + (len(pcm)-vocalWindow)/vocalHop
	features := make([]float32, (vocalBatch+2*vocalContext)*vocalBands)
	voice := make([]float32, 0, (len(pcm)+1599)/1600)
	var sum float64
	var count int
	for start := 0; start < n; start += vocalBatch {
		end := min(n, start+vocalBatch)
		left, right := max(0, start-vocalContext), min(n, end+vocalContext)
		for row := left; row < right; row++ {
			if row%64 == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			vocalFbankFrame(pcm[row*vocalHop:], stats, features[(row-left)*vocalBands:])
		}
		result, err := model(ctx, features[:(right-left)*vocalBands], right-left)
		if err != nil {
			return nil, err
		}
		if len(result) != right-left {
			return nil, errors.New("player: incomplete FireRed output")
		}
		for _, v := range result[start-left : end-left] {
			if !finiteMixNumber(float64(v)) {
				return nil, errors.New("player: non-finite FireRed output")
			}
			sum += float64(min(1, max(0, v)))
			count++
			if count == 10 {
				voice = append(voice, float32(sum/10))
				count, sum = 0, 0
			}
		}
	}
	if count > 0 {
		voice = append(voice, float32(sum/float64(count)))
	}
	for len(voice) < (len(pcm)+1599)/1600 {
		voice = append(voice, voice[len(voice)-1])
	}
	return voice, nil
}
