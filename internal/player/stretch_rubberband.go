package player

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"unsafe"
)

var errStretchUnavailable = errors.New("player: pitch-preserving stretcher unavailable")

// rubberBandAPI uses the public C API (unsigned sample counts, planar float
// buffers). The library stays loaded because these function pointers are cached.
type rubberBandAPI struct {
	handle       uintptr
	new          func(uint32, uint32, int32, float64, float64) uintptr
	delete       func(uintptr)
	engine       func(uintptr) int32
	setTimeRatio func(uintptr, float64)
	startPad     func(uintptr) uint32
	startDelay   func(uintptr) uint32
	setMaxBlock  func(uintptr, uint32)
	process      func(uintptr, unsafe.Pointer, uint32, int32)
	available    func(uintptr) int32
	retrieve     func(uintptr, unsafe.Pointer, uint32) uint32
}

var loadTailStretcher = sync.OnceValues(func() (*rubberBandAPI, error) {
	var last error
	for _, path := range rubberBandLibraryPaths() {
		api, err := openRubberBandLibrary(path)
		if err == nil {
			return api, nil
		}
		last = err
	}
	return nil, fmt.Errorf("%w: %v", errStretchUnavailable, last)
})

// rubberBandLibraryPaths prefers the application bundle. Homebrew/system paths
// are development fallbacks; production packages must carry their own library.
func rubberBandLibraryPaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Frameworks"))
		switch runtime.GOOS {
		case "darwin":
			paths = append(paths, filepath.Join(dir, "librubberband.3.dylib"), filepath.Join(dir, "librubberband.2.dylib"), filepath.Join(dir, "librubberband.dylib"))
		case "linux":
			paths = append(paths, filepath.Join(dir, "librubberband.so.3"), filepath.Join(dir, "librubberband.so.2"), filepath.Join(dir, "librubberband.so"))
		}
	}
	switch runtime.GOOS {
	case "darwin":
		paths = append(paths,
			"/opt/homebrew/opt/rubberband/lib/librubberband.3.dylib",
			"/opt/homebrew/opt/rubberband/lib/librubberband.2.dylib",
			"/opt/homebrew/opt/rubberband/lib/librubberband.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.3.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.2.dylib",
			"/usr/local/opt/rubberband/lib/librubberband.dylib")
	case "linux":
		paths = append(paths, "librubberband.so.3", "librubberband.so.2", "librubberband.so")
	}
	return paths
}

// stretchTempoAt is the outgoing playback speed at a source frame: an S-curve
// reaches the incoming tempo before the overlap, then holds it during the mix.
// finalTempo is incoming BPM / outgoing BPM; pitch remains unchanged.
func stretchTempoAt(sourceFrame, rampFrames int64, finalTempo float64) float64 {
	if rampFrames <= 0 || sourceFrame >= rampFrames {
		return finalTempo
	}
	x := min(1, max(0, float64(sourceFrame)/float64(rampFrames)))
	return 1 + (finalTempo-1)*x*x*(3-2*x)
}

// stretchedTailFrames estimates the output timeline by integrating 1/tempo.
// The actual renderer's length is authoritative: no useful audio is discarded
// to force its output to this estimate.
func stretchedTailFrames(inputFrames int64, finalTempo float64, rampFrames int64) int64 {
	if inputFrames <= 0 || math.IsNaN(finalTempo) || math.IsInf(finalTempo, 0) || finalTempo < .5 || finalTempo > 2 {
		return 0
	}
	if finalTempo == 1 {
		return inputFrames
	}
	ramp := min(max(0, rampFrames), inputFrames)
	if ramp == 0 {
		return int64(math.Round(float64(inputFrames) / finalTempo))
	}
	// Simpson integration keeps the planner independent of decoder/block sizes.
	const intervals = 256
	sum := 0.0
	for i := 0; i <= intervals; i++ {
		x := float64(i) / intervals
		tempo := 1 + (finalTempo-1)*x*x*(3-2*x)
		weight := 2.0
		if i == 0 || i == intervals {
			weight = 1
		} else if i%2 != 0 {
			weight = 4
		}
		sum += weight / tempo
	}
	frames := float64(ramp)*sum/(3*intervals) + float64(inputFrames-ramp)/finalTempo
	return int64(math.Round(frames))
}

// stretchTailPCM runs on a preparation worker, never on the audio callback or
// UI thread. It consumes every interleaved 48 kHz stereo source frame, flushes
// Rubber Band, and returns all output after official startup-delay compensation.
// rampFrames names the source interval before the mix; afterward tempo is held.
func stretchTailPCM(ctx context.Context, input []float32, finalTempo float64, rampFrames int64) ([]float32, error) {
	return stretchPCM(ctx, input, finalTempo, rampFrames, nil)
}

func stretchScheduledPCM(ctx context.Context, input []float32, schedule [][2]float64) ([]float32, error) {
	last := -1.0
	for _, step := range schedule {
		if !finiteMixNumber(step[0]) || !finiteMixNumber(step[1]) || step[0] < 0 || step[0] < last || step[1] < .5 || step[1] > 2 {
			return nil, errors.New("player: invalid tempo schedule")
		}
		last = step[0]
	}
	return stretchPCM(ctx, input, 1, 0, schedule)
}

func stretchPCM(ctx context.Context, input []float32, finalTempo float64, rampFrames int64, schedule [][2]float64) ([]float32, error) {
	if ctx == nil {
		return nil, errors.New("player: nil stretch context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	frames := int64(len(input) / 2)
	if len(input)%2 != 0 || frames > 60*outputRate || rampFrames < 0 || rampFrames > frames ||
		math.IsNaN(finalTempo) || math.IsInf(finalTempo, 0) || finalTempo < .5 || finalTempo > 2 {
		return nil, errors.New("player: invalid tail stretch parameters")
	}
	if frames == 0 {
		return nil, nil
	}
	tempoAt := func(at int64) float64 { return stretchTempoAt(at, rampFrames, finalTempo) }
	unity := math.Abs(finalTempo-1) < 1e-9
	if len(schedule) > 0 {
		tempoAt = func(at int64) float64 { return scheduledTempo(schedule, float64(at)/outputRate) }
		for _, step := range schedule {
			if math.Abs(step[1]-1) >= 1e-9 {
				unity = false
			}
		}
	}
	if unity {
		return slices.Clone(input), nil
	}
	api, err := loadTailStretcher()
	if err != nil {
		return nil, err
	}
	// R3 finer engine, linked stereo, no internal worker: each process call is
	// synchronous, and no C worker retains pointers into Go's reusable buffers.
	const options = 0x00000001 | 0x00010000 | 0x10000000 | 0x20000000
	state := api.new(outputRate, 2, options, 1/tempoAt(0), 1)
	if state == 0 {
		return nil, errors.New("player: creating Rubber Band stretcher failed")
	}
	defer api.delete(state)
	if api.engine(state) != 3 {
		return nil, errors.New("player: Rubber Band R3 engine is required")
	}
	const block = 256
	api.setMaxBlock(state, block)
	pad, discard := int64(api.startPad(state)), int64(api.startDelay(state))
	if pad > 2*outputRate || discard > 2*outputRate {
		return nil, errors.New("player: excessive Rubber Band startup latency")
	}
	left, right := make([]float32, block), make([]float32, block)
	outputLeft, outputRight := make([]float32, block), make([]float32, block)
	inPlanes := [2]*float32{&left[0], &right[0]}
	outPlanes := [2]*float32{&outputLeft[0], &outputRight[0]}
	defer func() {
		runtime.KeepAlive(inPlanes)
		runtime.KeepAlive(outPlanes)
		runtime.KeepAlive(left)
		runtime.KeepAlive(right)
		runtime.KeepAlive(outputLeft)
		runtime.KeepAlive(outputRight)
	}()
	estimate := stretchedTailFrames(frames, finalTempo, rampFrames)
	if len(schedule) > 0 {
		estimate = scheduledTailClock(frames, schedule, 0).outputFrame(frames)
	}
	result := make([]float32, 0, int(estimate+block)*2)
	maxOutput := (frames*2 + outputRate) * 2
	drain := func() (int32, error) {
		for {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			available := api.available(state)
			if available <= 0 {
				return available, nil
			}
			count := api.retrieve(state, unsafe.Pointer(&outPlanes[0]), uint32(min(int32(block), available)))
			if count == 0 || count > block {
				return 0, errors.New("player: Rubber Band output made no progress")
			}
			skip := min(discard, int64(count))
			discard -= skip
			if int64(len(result))+2*(int64(count)-skip) > maxOutput {
				return 0, errors.New("player: Rubber Band output exceeded tail budget")
			}
			for i := int(skip); i < int(count); i++ {
				l, r := outputLeft[i], outputRight[i]
				if math.IsNaN(float64(l)) || math.IsInf(float64(l), 0) || math.IsNaN(float64(r)) || math.IsInf(float64(r), 0) {
					return 0, errors.New("player: invalid Rubber Band output")
				}
				result = append(result, l, r)
			}
		}
	}
	// The C API specifies start padding and a corresponding output delay for
	// realtime processing. Padding uses the same initial ratio as construction.
	for remaining := pad; remaining > 0; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := min(int64(block), remaining)
		api.process(state, unsafe.Pointer(&inPlanes[0]), uint32(n), 0)
		if _, err := drain(); err != nil {
			return nil, err
		}
		remaining -= n
	}
	for at := int64(0); at < frames; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n := min(int64(block), frames-at)
		if at < rampFrames && at+n > rampFrames {
			n = rampFrames - at
		}
		for _, step := range schedule {
			boundary := int64(math.Round(step[0] * outputRate))
			if boundary > at && boundary < at+n {
				n = boundary - at
			}
		}
		for i := int64(0); i < n; i++ {
			l, r := input[(at+i)*2], input[(at+i)*2+1]
			if math.IsNaN(float64(l)) || math.IsInf(float64(l), 0) || math.IsNaN(float64(r)) || math.IsInf(float64(r), 0) {
				return nil, errors.New("player: invalid tail PCM")
			}
			left[i], right[i] = l, r
		}
		api.setTimeRatio(state, 1/tempoAt(at+n/2))
		api.process(state, unsafe.Pointer(&inPlanes[0]), uint32(n), 0)
		if _, err := drain(); err != nil {
			return nil, err
		}
		at += n
	}
	// Signal the end separately so every source block is submitted exactly once.
	api.process(state, unsafe.Pointer(&inPlanes[0]), 0, 1)
	finished, err := drain()
	if err != nil {
		return nil, err
	}
	if finished != -1 || discard != 0 || len(result) == 0 {
		return nil, errors.New("player: Rubber Band did not finish the complete tail")
	}
	return result, nil
}
