//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))

package player

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/egoist/mygo/audio"
)

type deviceMixModelBundle struct {
	dir      string
	identity [sha256.Size]byte
}

type deviceModelRequest struct {
	Package, Input string
	Frames         int
	Ends           []int `json:",omitempty"`
}

type deviceModelCall func(context.Context, string, deviceModelRequest, any) error

func deviceMixModels(dir string) deviceMixModelBundle {
	bundle := deviceMixModelBundle{dir: dir}
	digest := sha256.New()
	err := filepath.WalkDir(dir, func(path string, item fs.DirEntry, err error) error {
		if err != nil || item.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("player: unexpected mix model resource")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(digest, "%s\x00%d\x00", rel, info.Size())
		_, err = io.Copy(digest, f)
		return errors.Join(err, f.Close())
	})
	if err == nil {
		copy(bundle.identity[:], digest.Sum(nil))
	}
	return bundle
}

// Analyze the decoded audio used by playback. A cached result skips every
// model call, and the live decoder or queue can move independently.
func analyzeAppleMixProfile(ctx context.Context, dl *download, ms int64, temp, cache string, models deviceMixModelBundle, call deviceModelCall) (deviceMixProfile, error) {
	if _, err := os.Stat(filepath.Join(models.dir, "BeatThis.mlpackage", "Manifest.json")); err != nil {
		return deviceMixProfile{}, err
	}
	select {
	case neuralSlots <- struct{}{}:
	case <-ctx.Done():
		return deviceMixProfile{}, ctx.Err()
	}
	defer func() { <-neuralSlots }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var input *os.File
	defer func() {
		if input != nil {
			input.Close()
			os.Remove(input.Name())
		}
	}()
	predict := func(ctx context.Context, method string, features []float32, count int, reply any) error {
		if input == nil {
			var err error
			input, err = os.CreateTemp(temp, "cloudmusic-features-*.f32")
			if err != nil {
				return err
			}
		}
		if _, err := input.WriteAt(unsafe.Slice((*byte)(unsafe.Pointer(&features[0])), len(features)*4), 0); err != nil {
			return err
		}
		if err := input.Truncate(int64(len(features) * 4)); err != nil {
			return err
		}
		return call(ctx, method, deviceModelRequest{Package: models.dir, Input: input.Name(), Frames: count}, reply)
	}
	withDecoder := func(use func(audio.Decoder) error) error {
		s, err := openShared(dl, ms)
		if err != nil {
			return err
		}
		defer s.Close()
		var dec audio.Decoder = s
		if s.SampleRate() != outputRate {
			dec = newResampler(s)
		}
		return use(dec)
	}
	var profile deviceMixProfile
	var hit, keyOK bool
	var cachePath string
	var sourceFrames int64
	if err := withDecoder(func(dec audio.Decoder) error {
		digest := sha256.New()
		return resampleAnalysisPCM(ctx, dec, temp, beatSampleRate, digest, func(pcm []float32, path string, ends []int, frames int64) error {
			sourceFrames = frames
			if cache != "" && models.identity != [sha256.Size]byte{} {
				var pcmID [sha256.Size]byte
				copy(pcmID[:], digest.Sum(nil))
				cachePath = deviceMixCachePath(cache, pcmID, models.identity)
				if cached, err := readDeviceMixCache(cachePath, frames); err == nil {
					profile, hit = cached, true
					return nil
				}
			}
			grid, err := analyzeBeatGrid(ctx, pcm, func(ctx context.Context, features []float32, count int) ([]float32, []float32, error) {
				var prediction struct{ Beat, Downbeat []float32 }
				err := predict(ctx, "predict", features, count, &prediction)
				return prediction.Beat, prediction.Downbeat, err
			})
			if err != nil {
				return err
			}
			profile.beat = grid
			var key int
			request := deviceModelRequest{Package: models.dir, Input: path, Frames: len(pcm), Ends: ends}
			if err := call(ctx, "key", request, &key); err == nil {
				profile.key, keyOK = skeyClass(key), true
			}
			profile.spectrum, err = analyzeMixSpectrum(ctx, pcm, ends)
			return err
		})
	}); err != nil {
		return deviceMixProfile{}, err
	}
	if hit {
		return profile, ctx.Err()
	}
	data, err := os.ReadFile(filepath.Join(models.dir, "FireRedCMVN.json"))
	if err != nil {
		return profile, ctx.Err()
	}
	var cmvn vocalCMVN
	if err := json.Unmarshal(data, &cmvn); err != nil {
		return profile, ctx.Err()
	}
	err = withDecoder(func(dec audio.Decoder) error {
		return withAnalysisPCM(ctx, dec, temp, vocalSampleRate, func(pcm []float32, _ string, _ []int) error {
			var err error
			profile.voice, err = analyzeVocalActivity(ctx, pcm, cmvn, func(ctx context.Context, features []float32, count int) ([]float32, error) {
				var voice []float32
				err := predict(ctx, "vocal", features, count, &voice)
				return voice, err
			})
			return err
		})
	})
	if err == nil && keyOK && cachePath != "" {
		// Cache I/O is optional. Only a successful, complete inference can
		// replace a previous entry, and cancellation leaves the file alone.
		_ = writeDeviceMixCache(ctx, cachePath, sourceFrames, profile)
	}
	return profile, ctx.Err()
}
