package player

import (
	"context"
	"math"
	"math/cmplx"
)

const mixSpectrumBins = 1800

const (
	mixLow = iota
	mixLowMid
	mixMid
	mixHigh
	mixBandCount
)

type mixSpectrum struct {
	rms, centroid []float64
	bands         [mixBandCount][]float64
}

// mixDFT uses Bluestein's algorithm so the FFT bins stay on the desktop's
// exact 100 ms window. Zero-padding directly to a power of two moves bins
// across the band boundaries and changes the powers used by the planner.
type mixDFT struct {
	chirp, kernel, scratch []complex128
}

func newMixDFT(size int) *mixDFT {
	n := 1
	for n < 2*size-1 {
		n <<= 1
	}
	f := &mixDFT{make([]complex128, size), make([]complex128, n), make([]complex128, n)}
	for i := range f.chirp {
		f.chirp[i] = cmplx.Exp(complex(0, -math.Pi*float64(i*i)/float64(size)))
		f.kernel[i] = cmplx.Conj(f.chirp[i])
		if i > 0 {
			f.kernel[n-i] = f.kernel[i]
		}
	}
	fft(f.kernel)
	return f
}

func (f *mixDFT) transform(samples []float32) []complex128 {
	clear(f.scratch)
	for i, v := range samples {
		f.scratch[i] = complex(float64(v), 0) * f.chirp[i]
	}
	fft(f.scratch)
	for i, v := range f.scratch {
		f.scratch[i] = cmplx.Conj(v * f.kernel[i])
	}
	fft(f.scratch)
	// Callers use only magnitudes, so the final conjugate and phase factors
	// from the inverse transform can be left out.
	return f.scratch[:len(samples)/2+1]
}

func analyzeMixSpectrum(ctx context.Context, pcm []float32, ends []int) (mixSpectrum, error) {
	const energyWindow, centroidWindow, centroidHop = beatSampleRate / 10, 400, 512
	energyFFT, centroidFFT := newMixDFT(energyWindow), newMixDFT(centroidWindow)
	var hann [centroidWindow]float32
	var centroidInput [centroidWindow]float32
	for i := range hann {
		hann[i] = float32(.5 - .5*math.Cos(2*math.Pi*float64(i)/centroidWindow))
	}
	var rms, centroid []float64
	var bands [mixBandCount][]float64
	start := 0
	for _, end := range ends {
		if err := ctx.Err(); err != nil {
			return mixSpectrum{}, err
		}
		chunk := pcm[start:end]
		start = end
		for offset := 0; offset < len(chunk); offset += energyWindow {
			piece := chunk[offset:min(len(chunk), offset+energyWindow)]
			var total float64
			for _, sample := range piece {
				total += float64(sample * sample)
			}
			rms = append(rms, float64(float32(math.Sqrt(total/float64(len(piece))))))
			f := energyFFT
			if len(piece) != energyWindow {
				f = newMixDFT(len(piece))
			}
			power := mixBandPower(piece, f)
			for band, v := range power {
				bands[band] = append(bands[band], float64(float32(math.Sqrt(v))))
			}
		}
		if len(chunk) == 0 {
			continue
		}
		for center := 0; center <= len(chunk); center += centroidHop {
			for i, w := range hann {
				at := center - centroidWindow/2 + i
				if len(chunk) > 1 {
					reflect := 2 * (len(chunk) - 1)
					at = (at%reflect + reflect) % reflect
					at = min(at, reflect-at)
				} else {
					at = 0
				}
				centroidInput[i] = chunk[at] * w
			}
			var sum, weighted float64
			for bin, value := range centroidFFT.transform(centroidInput[:]) {
				magnitude := cmplx.Abs(value)
				sum += magnitude
				weighted += magnitude * (float64(bin) * beatSampleRate / centroidWindow)
			}
			hz := 0.0
			if sum > 0 {
				hz = weighted / sum
			}
			centroid = append(centroid, float64(float32(hz)))
		}
	}
	result := mixSpectrum{rms: binMixSeries(rms, true), centroid: binMixSeries(centroid, false)}
	peak := 0.0
	for _, v := range result.rms {
		peak = max(peak, v)
	}
	if peak == 0 {
		peak = 1
	}
	for i, v := range result.rms {
		result.rms[i] = float64(float32(v / peak))
		if result.rms[i] < .01 {
			result.centroid[i] = 0
		}
	}
	for band, values := range bands {
		result.bands[band] = binMixSeries(values, true)
		for i, v := range result.bands[band] {
			result.bands[band][i] = float64(float32(v / peak))
		}
	}
	return result, nil
}

func mixBandPower(pcm []float32, f *mixDFT) [mixBandCount]float64 {
	var power [mixBandCount]float64
	const low = 20
	limit := [3]float64{120, 400, 4000}
	stepHz := 1 / (float64(len(pcm)) * (1.0 / beatSampleRate))
	for i, v := range f.transform(pcm) {
		hz := float64(i) * stepHz
		if hz < low {
			continue
		}
		band := 0
		for band < len(limit) && hz >= limit[band] {
			band++
		}
		weight := 2.0
		if i*2 == len(pcm) {
			weight = 1 // Nyquist has no negative-frequency partner
		}
		power[band] += weight * (real(v)*real(v) + imag(v)*imag(v))
	}
	scale := float64(len(pcm)) * float64(len(f.scratch))
	for band := range power {
		power[band] /= scale * scale
	}
	return power
}

// binMixSeries averages exact fractional overlaps, including when a short
// source series is being expanded into the 1800-bin planner format.
func binMixSeries(values []float64, rms bool) []float64 {
	result := make([]float64, mixSpectrumBins)
	if len(values) == 0 {
		return result
	}
	accumulated := make([]float64, len(values)+1)
	for i, v := range values {
		if rms {
			v *= v
		}
		accumulated[i+1] = accumulated[i] + v
	}
	width := float64(len(values)) / mixSpectrumBins
	previous, left := 0.0, 0.0
	for i := range result {
		right := min(float64(len(values)), float64(i+1)*width)
		at := int(right)
		current := accumulated[at]
		if at < len(values) {
			current += (right - float64(at)) * (accumulated[at+1] - current)
		}
		value := max(0, (current-previous)/(right-left))
		if rms {
			value = math.Sqrt(value)
		}
		result[i] = float64(float32(value))
		previous, left = current, right
	}
	return result
}
