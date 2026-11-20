package player

import "math"

type mixKey struct {
	wheel int // Camelot 1–12
	major bool
}

func skeyClass(index int) mixKey {
	major := [...]int{11, 6, 1, 8, 3, 10, 5, 12, 7, 2, 9, 4}
	minor := [...]int{10, 5, 12, 7, 2, 9, 4, 11, 6, 1, 8, 3}
	if index >= 0 && index < len(major) {
		return mixKey{major[index], true}
	}
	if index -= len(major); index >= 0 && index < len(minor) {
		return mixKey{minor[index], false}
	}
	return mixKey{}
}

// Only known clashes constrain the measured planner. Relative major/minor
// and adjacent keys in the same mode are compatible on the Camelot wheel.
func keysClash(a, b mixKey) bool {
	if a.wheel == 0 || b.wheel == 0 {
		return false
	}
	distance := max(a.wheel-b.wheel, b.wheel-a.wheel)
	if a.major != b.major {
		return distance != 0
	}
	return min(distance, 12-distance) > 1
}

type deviceMixProfile struct {
	beat     beatGrid
	voice    []float32 // FireRed speech/singing probabilities, one bin per 100 ms
	key      mixKey
	spectrum mixSpectrum
}

func (device deviceMixProfile) apply(a *trackAnalysis, duration int64) {
	applyDeviceBeatGrid(a, device.beat)
	var voice []float64
	if len(device.voice) > 0 {
		values := make([]float64, len(device.voice))
		for i, v := range device.voice {
			values[i] = float64(v)
		}
		voice = binMixSeries(values, false)
		a.vocals = deviceVocalWindows(voice, float64(duration)/outputRate, device.beat.bpm, 0, float64(duration)/outputRate, false)
		a.vocalsKnown = true
	}
	a.key = device.key
	length := float64(duration) / outputRate
	a.bands = buildMixBandProfile(device.spectrum, device.beat, length)
	a.deviceTiming = buildDeviceMixTiming(device.spectrum.rms, a.bands, device.beat, length, float64(len(a.tailPCM)/2)/outputRate)
	if a.deviceTiming != nil {
		a.deviceTiming.analyzeStructure(a.bands, voice, length)
	}
}

// The desktop builds its scoring and placement masks on the same 1800-bin
// timeline, with padding added before bridging only for the placement mask.
func deviceVocalWindows(probability []float64, duration, bpm, from, to float64, padded bool) []mixVocalWindow {
	if duration <= 0 || len(probability) == 0 {
		return nil
	}
	bin := duration / float64(len(probability))
	minBins := max(1, int(math.Ceil(math.Nextafter(.3/bin, math.Inf(-1)))))
	gap := .5
	if bpm > 0 {
		gap = min(1, max(gap, 60/bpm))
	}
	leftPadding, rightPadding := 0.0, 0.0
	if padded {
		leftPadding, rightPadding = .25, .75
	}
	type window struct{ start, end float64 }
	var windows []window
	for i := 0; i < len(probability); {
		if !(probability[i] >= .5) {
			i++
			continue
		}
		start := i
		var peak, sum float64
		for i < len(probability) && probability[i] >= .3 {
			peak = max(peak, probability[i])
			sum += probability[i]
			i++
		}
		if length := i - start; length < minBins || peak < .85 && sum/float64(length) < .65 {
			continue
		}
		w := window{max(from, float64(start)*bin-leftPadding), min(to, float64(i)*bin+rightPadding)}
		if w.start >= w.end {
			continue
		}
		if n := len(windows); n > 0 && w.start-windows[n-1].end <= gap {
			windows[n-1].end = w.end
		} else {
			windows = append(windows, w)
		}
	}
	result := make([]mixVocalWindow, 0, len(windows))
	for _, w := range windows {
		result = append(result, mixVocalWindow{int64(math.Round(w.start * outputRate)), int64(math.Round(w.end * outputRate))})
	}
	return result
}
