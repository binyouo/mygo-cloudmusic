package player

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"reflect"
	"testing"
)

// Replays reuse all planner inputs for matching audio and models, and an incomplete retry keeps the usable entry.
func TestDeviceMixCacheReusesCompleteProfile(t *testing.T) {
	const seconds = 14
	const frames = seconds * outputRate
	profile := deviceMixProfile{
		beat:     beatGrid{bpm: 120, meter: 4, beats: []float64{0, .5, 1}, downbeats: []float64{0}},
		key:      mixKey{wheel: 8, major: true},
		voice:    make([]float32, seconds*10),
		spectrum: mixSpectrum{rms: make([]float64, mixSpectrumBins), centroid: make([]float64, mixSpectrumBins)},
	}
	for i := range profile.spectrum.bands {
		profile.spectrum.bands[i] = make([]float64, mixSpectrumBins)
		profile.spectrum.bands[i][600] = (.13 + float64(i)) / 7
	}
	profile.voice[13] = .83
	profile.spectrum.rms[600], profile.spectrum.centroid[600] = .6, 1730.625
	ctx := context.Background()
	dir := t.TempDir()
	pcm := sha256.Sum256([]byte("decoded PCM"))
	models := sha256.Sum256([]byte("published models"))
	path := deviceMixCachePath(dir, pcm, models)
	if err := writeDeviceMixCache(ctx, path, frames, profile); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		deviceMixCachePath(dir, sha256.Sum256([]byte("different rendition")), models),
		deviceMixCachePath(dir, pcm, sha256.Sum256([]byte("updated models"))),
	} {
		if _, err := readDeviceMixCache(path, frames); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cache reused for different audio or models: %v", err)
		}
	}
	if _, err := readDeviceMixCache(path, frames+1); err == nil {
		t.Fatal("accepted cached features for a different number of source frames")
	}
	incomplete := profile
	incomplete.voice = nil
	if err := writeDeviceMixCache(ctx, path, frames, incomplete); err == nil {
		t.Fatal("cached an incomplete vocal analysis")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := writeDeviceMixCache(canceled, path, frames, profile); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled analysis replaced cache: %v", err)
	}
	got, err := readDeviceMixCache(path, frames)
	if err != nil || !reflect.DeepEqual(got, profile) {
		t.Fatalf("cached planner inputs changed: %v", err)
	}
}
