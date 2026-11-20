package player

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
)

const deviceMixCacheVersion = 1
const deviceMixCacheLimit = 2 << 20

// A cache entry carries inputs to the planner. The audio decoder and the
// resulting transition stay per playback, since alignment depends on its tail.
type deviceMixCacheEntry struct {
	Version   int
	Frames    int64
	BPM       float64
	Meter     int
	Beats     []float64
	Downbeats []float64
	Voice     []float32
	Wheel     int
	Major     bool
	RMS       []float64
	Centroid  []float64
	Bands     [mixBandCount][]float64
}

func deviceMixCachePath(dir string, pcm, models [sha256.Size]byte) string {
	id := sha256.New()
	id.Write(pcm[:])
	id.Write(models[:])
	return filepath.Join(dir, hex.EncodeToString(id.Sum(nil))+".json")
}

func readDeviceMixCache(path string, frames int64) (deviceMixProfile, error) {
	f, err := os.Open(path)
	if err != nil {
		return deviceMixProfile{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, deviceMixCacheLimit+1))
	if err != nil {
		return deviceMixProfile{}, err
	}
	var entry deviceMixCacheEntry
	if len(data) > deviceMixCacheLimit || json.Unmarshal(data, &entry) != nil || entry.Frames != frames || !entry.valid() {
		return deviceMixProfile{}, errors.New("player: invalid device mix cache entry")
	}
	return deviceMixProfile{
		beat:     beatGrid{beats: entry.Beats, downbeats: entry.Downbeats, meter: entry.Meter, bpm: entry.BPM},
		key:      mixKey{wheel: entry.Wheel, major: entry.Major},
		voice:    entry.Voice,
		spectrum: mixSpectrum{rms: entry.RMS, centroid: entry.Centroid, bands: entry.Bands},
	}, nil
}

func writeDeviceMixCache(ctx context.Context, path string, frames int64, p deviceMixProfile) error {
	entry := deviceMixCacheEntry{
		Version: deviceMixCacheVersion, Frames: frames,
		BPM: p.beat.bpm, Meter: p.beat.meter, Beats: p.beat.beats, Downbeats: p.beat.downbeats,
		Voice: p.voice, Wheel: p.key.wheel, Major: p.key.major,
		RMS: p.spectrum.rms, Centroid: p.spectrum.centroid, Bands: p.spectrum.bands,
	}
	if !entry.valid() {
		return errors.New("player: incomplete device mix profile")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".analysis-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = json.NewEncoder(tmp).Encode(entry)
	closeErr := tmp.Close()
	if err = errors.Join(err, closeErr, ctx.Err()); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	pruneNeuralCache(dir)
	return nil
}

func (e *deviceMixCacheEntry) valid() bool {
	if e.Version != deviceMixCacheVersion || e.Frames < 8*outputRate || e.Frames > 1200*outputRate ||
		!finiteMixNumber(e.BPM) || e.BPM < 0 || e.BPM > 400 ||
		(e.Meter != 0 && e.Meter != 3 && e.Meter != 4) || e.Wheel < 0 || e.Wheel > 12 {
		return false
	}
	duration := float64(e.Frames) / outputRate
	// FireRed bins cover the audio resampled to 16 kHz. The conversion can
	// differ from the original media boundary by less than one such sample.
	if n := float64(len(e.Voice)); n < math.Ceil((duration-1.0/vocalSampleRate)*10) ||
		n > math.Ceil((duration+1.0/vocalSampleRate)*10) {
		return false
	}
	for _, value := range e.Voice {
		if !finiteMixNumber(float64(value)) || value < 0 || value > 1 {
			return false
		}
	}
	for _, grid := range [][]float64{e.Beats, e.Downbeats} {
		last := -1.0
		for _, time := range grid {
			if !finiteMixNumber(time) || time <= last || time < 0 || time > duration {
				return false
			}
			last = time
		}
	}
	series := append([][]float64{e.RMS, e.Centroid}, e.Bands[:]...)
	for _, values := range series {
		if len(values) != mixSpectrumBins {
			return false
		}
		for _, value := range values {
			if !finiteMixNumber(value) || value < 0 {
				return false
			}
		}
	}
	return true
}
