//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))

package player

/*
#cgo CFLAGS: -I${SRCDIR}/../../build/soxr-1.0.0/libsoxr/src -I${SRCDIR}/soxr -DSOXR_LIB
#include "soxr.h"
*/
import "C"

import (
	"context"
	"errors"
	"hash"
	"io"
	"os"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/audio"
)

// withAnalysisPCM uses the same decoded 48 kHz audio as the mixer and keeps
// the resampled track in a file, so feature windows need no full-track copy.
func withAnalysisPCM(ctx context.Context, dec audio.Decoder, tempDir string, rate int, use func([]float32, string, []int) error) error {
	return resampleAnalysisPCM(ctx, dec, tempDir, rate, nil, func(samples []float32, path string, ends []int, _ int64) error {
		return use(samples, path, ends)
	})
}

func resampleAnalysisPCM(ctx context.Context, dec audio.Decoder, tempDir string, rate int, identity hash.Hash, use func([]float32, string, []int, int64) error) error {
	if dec.SampleRate() != outputRate || dec.Channels() != 2 || dec.Len() < 8*outputRate || dec.Len() > 1200*outputRate {
		return errors.New("player: unsupported analysis PCM")
	}
	format := C.soxr_io_spec(C.SOXR_FLOAT32_I, C.SOXR_FLOAT32_I)
	quality := C.soxr_quality_spec(C.SOXR_HQ, 0)
	var reason C.soxr_error_t
	resampler := C.soxr_create(outputRate, C.double(rate), 1, &reason, &format, &quality, nil)
	if reason != nil {
		return errors.New(C.GoString(reason))
	}
	if resampler == nil {
		return errors.New("player: cannot create analysis resampler")
	}
	defer C.soxr_delete(resampler)
	pcm, err := os.CreateTemp(tempDir, "cloudmusic-analysis-*.f32")
	if err != nil {
		return err
	}
	defer os.Remove(pcm.Name())
	defer pcm.Close()
	const block = 4096
	const segment = 10 * outputRate // desktop S-KEY restarts one-second windows here
	var stereo [2 * block]float32
	var mono [block]float32
	var output [2 * block]float32
	sourceFrames, outputFrames := 0, 0
	var ends []int
	convert := func(input unsafe.Pointer, frames int) (int, error) {
		var used, written C.size_t
		if reason := C.soxr_process(resampler, C.soxr_in_t(input), C.size_t(frames), &used, C.soxr_out_t(unsafe.Pointer(&output[0])), C.size_t(len(output)), &written); reason != nil {
			return 0, errors.New(C.GoString(reason))
		}
		if used != C.size_t(frames) {
			return 0, errors.New("player: analysis resampler did not consume all input")
		}
		count := int(written)
		_, err := pcm.Write(unsafe.Slice((*byte)(unsafe.Pointer(&output[0])), count*4))
		outputFrames += count
		return count, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frames := min(block, segment-sourceFrames%segment)
		n, readErr := dec.Read(stereo[:frames*2])
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if n%2 != 0 || n == 0 && readErr == nil {
			return io.ErrUnexpectedEOF
		}
		sourceFrames += n / 2
		if sourceFrames > 1200*outputRate {
			return errors.New("player: analysis PCM exceeds duration limit")
		}
		for i := range n / 2 {
			mono[i] = (stereo[2*i] + stereo[2*i+1]) * .5
			if !finiteMixNumber(float64(mono[i])) {
				return errors.New("player: non-finite analysis PCM")
			}
		}
		if n > 0 {
			if identity != nil {
				// Hash the source before resampling, so the 22.05 and
				// 16 kHz models share the same decoded audio identity.
				identity.Write(unsafe.Slice((*byte)(unsafe.Pointer(&mono[0])), n*2))
			}
			if _, err := convert(unsafe.Pointer(&mono[0]), n/2); err != nil {
				return err
			}
			if sourceFrames%segment == 0 {
				ends = append(ends, outputFrames)
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	for {
		count, err := convert(nil, 0)
		if err != nil {
			return err
		}
		if count == 0 {
			break
		}
	}
	want := (sourceFrames*rate + outputRate/2) / outputRate
	if outputFrames < want-1 || outputFrames > want+1 || outputFrames == 0 {
		return io.ErrUnexpectedEOF
	}
	frames := min(outputFrames, want)
	if sourceFrames%segment == 0 {
		ends[len(ends)-1] = frames // desktop's last chunk includes the flush
	} else {
		ends = append(ends, frames)
	}
	mapped, err := syscall.Mmap(int(pcm.Fd()), 0, frames*4, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return err
	}
	defer syscall.Munmap(mapped)
	return use(unsafe.Slice((*float32)(unsafe.Pointer(&mapped[0])), frames), pcm.Name(), ends, int64(sourceFrames))
}
