package player

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/audio"
)

// Immutable model output, cached by decoded audio rather than song ID, so a
// replacement file or another service's rendition cannot reuse stale timing.
type neuralAnalysis struct {
	Version          int      `json:"version"`
	Duration         float64  `json:"duration"`
	BPM              *float64 `json:"bpm"`
	Beats, Downbeats []float64
	Meter            int       `json:"beats_per_bar"`
	VocalActivity    []float64 `json:"vocal_activity"`
	RMS              []float64 `json:"rms_energy"`
	Centroid         []float64 `json:"spectral_centroid"`
	Low              []float64 `json:"band_rms_low"`
	LowMid           []float64 `json:"band_rms_low_mid"`
	Mid              []float64 `json:"band_rms_mid"`
	High             []float64 `json:"band_rms_high"`
	path             string
	payload          []byte
}

var neuralSlots = make(chan struct{}, 1)

func neuralRuntimePaths() (python, runner string) {
	if exe, err := os.Executable(); err == nil {
		home := filepath.Join(filepath.Dir(exe), "..", "Resources", "smart-fades")
		python, runner = filepath.Join(home, "python", "bin", "python3"), filepath.Join(home, "runner.py")
		if regularFile(python) && regularFile(runner) {
			return
		}
	}
	if os.Getenv("MYGO_SMART_FADES_DEV") != "1" {
		return "", ""
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			runner = filepath.Join(dir, "scripts", "smart_fades", "runner.py")
			python = filepath.Join(dir, "build", "smart-fades-python", "cpython-3.12.14-macos-aarch64-none", "bin", "python3")
			if regularFile(runner) && regularFile(python) {
				return python, runner
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", ""
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func runNeural(ctx context.Context, args ...string) error {
	python, runner := neuralRuntimePaths()
	if python == "" {
		return errors.New("player: neural analysis runtime unavailable")
	}
	cmd := exec.CommandContext(ctx, python, append([]string{"-B", runner}, args...)...)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !slices.Contains([]string{"PYTHONHOME", "PYTHONPATH", "VIRTUAL_ENV", "OMP_NUM_THREADS", "VECLIB_MAXIMUM_THREADS", "PYTHONNOUSERSITE", "PYTHONDONTWRITEBYTECODE"}, key) {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "OMP_NUM_THREADS=2", "VECLIB_MAXIMUM_THREADS=2", "PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1")
	var log limitedAnalysisLog
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("player: Smart Fades: %w: %s", err, log.String())
	}
	return nil
}

type limitedAnalysisLog struct{ bytes.Buffer }

func (b *limitedAnalysisLog) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 4096 {
		b.Buffer.Write(p[:min(len(p), 4096-b.Len())])
	}
	return n, nil
}

func readNeuralAnalysis(path string, duration float64) (*neuralAnalysis, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, 2<<20))
	if err != nil {
		return nil, err
	}
	var result neuralAnalysis
	if err = json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	if result.Version != 1 || !finiteMixNumber(result.Duration) || result.Duration < 8 || result.Duration > 1200 || math.Abs(result.Duration-duration) > .1 ||
		result.BPM != nil && (!finiteMixNumber(*result.BPM) || *result.BPM <= 0 || *result.BPM > 400) ||
		result.Meter != 0 && result.Meter != 3 && result.Meter != 4 || len(result.VocalActivity) != 1800 {
		return nil, errors.New("player: invalid neural analysis metadata")
	}
	for _, grid := range [][]float64{result.Beats, result.Downbeats} {
		last := -1.0
		for _, at := range grid {
			if !finiteMixNumber(at) || at <= last || at < 0 || at > result.Duration {
				return nil, errors.New("player: invalid neural beat grid")
			}
			last = at
		}
	}
	for _, v := range result.VocalActivity {
		if !finiteMixNumber(v) || v < 0 || v > 1 {
			return nil, errors.New("player: invalid neural vocal mask")
		}
	}
	for _, series := range [][]float64{result.RMS, result.Centroid, result.Low, result.LowMid, result.Mid, result.High} {
		if len(series) != 1800 {
			return nil, errors.New("player: incomplete neural spectral profile")
		}
		for _, v := range series {
			if !finiteMixNumber(v) || v < 0 {
				return nil, errors.New("player: invalid neural spectral profile")
			}
		}
	}
	result.path = path
	result.payload = payload
	return &result, nil
}

func analyzeNeuralTrack(ctx context.Context, dl *download, ms int64) (*neuralAnalysis, error) {
	if python, _ := neuralRuntimePaths(); python == "" {
		return nil, errors.New("player: neural runtime unavailable")
	}
	select {
	case neuralSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-neuralSlots }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	s, err := openShared(dl, ms)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	var dec audio.Decoder = s
	if s.SampleRate() != outputRate {
		dec = newResampler(s)
	}
	if dec.Len() < 8*outputRate || dec.Len() > 1200*outputRate {
		return nil, errors.New("player: neural analysis duration out of bounds")
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "mygo-cloudmusic", "smart-fades-v1")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	pcm, err := os.CreateTemp(dir, ".pcm-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(pcm.Name())
	defer pcm.Close()
	hash := sha256.New()
	io.WriteString(hash, "smart-fades-ma-e16a468-v1-f32-mono-48000\n")
	writer := io.MultiWriter(pcm, hash)
	buf := make([]float32, 8192)
	encoded := make([]byte, len(buf)*2)
	var frames int64
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := dec.Read(buf)
		if n%2 != 0 {
			return nil, io.ErrUnexpectedEOF
		}
		for i := 0; i < n; i += 2 {
			v := (buf[i] + buf[i+1]) * .5
			if !finiteMixNumber(float64(v)) {
				return nil, errors.New("player: non-finite analysis PCM")
			}
			binary.LittleEndian.PutUint32(encoded[i*2:], math.Float32bits(v))
		}
		if _, err = writer.Write(encoded[:n*2]); err != nil {
			return nil, err
		}
		frames += int64(n / 2)
		if frames > 1200*outputRate {
			return nil, errors.New("player: neural PCM budget exceeded")
		}
		if readErr != nil {
			if readErr != io.EOF {
				return nil, readErr
			}
			break
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if math.Abs(float64(frames-dec.Len())) > float64(outputRate)/10 {
		return nil, io.ErrUnexpectedEOF
	}
	if err = pcm.Close(); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, hex.EncodeToString(hash.Sum(nil))+".json")
	duration := float64(frames) / outputRate
	if cached, err := readNeuralAnalysis(path, duration); err == nil {
		return cached, nil
	}
	output, err := os.CreateTemp(dir, ".analysis-*")
	if err != nil {
		return nil, err
	}
	output.Close()
	defer os.Remove(output.Name())
	if err = runNeural(ctx, "analyze", pcm.Name(), output.Name()); err != nil {
		return nil, err
	}
	if _, err = readNeuralAnalysis(output.Name(), duration); err != nil {
		return nil, err
	}
	if err = os.Rename(output.Name(), path); err != nil {
		return nil, err
	}
	pruneNeuralCache(dir)
	return readNeuralAnalysis(path, duration)
}

func pruneNeuralCache(dir string) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) <= 256 {
		return
	}
	type entry struct {
		path string
		at   time.Time
	}
	entries := make([]entry, 0, len(files))
	for _, path := range files {
		if info, err := os.Stat(path); err == nil {
			entries = append(entries, entry{path, info.ModTime()})
		}
	}
	slices.SortFunc(entries, func(a, b entry) int { return a.at.Compare(b.at) })
	for _, item := range entries[:max(0, len(entries)-256)] {
		os.Remove(item.path)
	}
}

func neuralPlan(ctx context.Context, a, b *neuralAnalysis, buffer float64) (*maTransitionPlan, error) {
	dir, err := os.MkdirTemp("", "mygo-smart-plan-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string][]byte{"outgoing.json": a.payload, "incoming.json": b.payload} {
		if err = os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return nil, err
		}
	}
	output := filepath.Join(dir, "plan.json")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err = runNeural(ctx, "plan", filepath.Join(dir, "outgoing.json"), filepath.Join(dir, "incoming.json"), strconv.FormatFloat(buffer, 'f', 6, 64), output); err != nil {
		return nil, err
	}
	f, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var result maTransitionPlan
	if err = json.NewDecoder(io.LimitReader(f, 2<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if err = result.validate(buffer, b.Duration); err != nil {
		return nil, err
	}
	return &result, nil
}
