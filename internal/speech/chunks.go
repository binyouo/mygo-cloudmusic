package speech

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"
)

// transcribeChunks feeds bounded audio files to the system recognizer.
// It reports the duration of silent chunks too, so resuming skips them.
func transcribeChunks(ctx context.Context, temp string, src Source, from, until time.Duration, recognize func(context.Context, string) ([]Segment, error), said func(Segment), heard func(time.Duration)) error {
	rate, channels := src.SampleRate(), src.Channels()
	if rate <= 0 || channels <= 0 {
		return errors.New("speech: invalid source audio format")
	}
	dir, err := os.MkdirTemp(temp, "cloudmusic-speech-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "speech.wav")
	const chunkSeconds = 16
	samples := make([]float32, chunkSeconds*rate*channels)
	wave := make([]byte, 44+2*chunkSeconds*rate)
	limit := int64(-1)
	if until > from {
		d := until - from
		limit = int64(d/time.Second)*int64(rate) + int64((d%time.Second)*time.Duration(rate)/time.Second)
	}
	at := func(frame int64) time.Duration {
		r := int64(rate)
		return from + time.Duration(frame/r)*time.Second + time.Duration(frame%r)*time.Second/time.Duration(rate)
	}
	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frames := chunkSeconds * rate
		if limit >= 0 {
			frames = int(min(int64(frames), limit-offset))
			if frames <= 0 {
				return nil
			}
		}
		n, err := readSpeechChunk(ctx, src, samples[:frames*channels])
		if err != nil && err != io.EOF {
			return err
		}
		frames = n / channels
		if frames == 0 {
			return nil
		}
		start := at(offset)
		offset += int64(frames)
		end := at(offset)
		if writeSpeechWave(wave, samples[:frames*channels], rate, channels) {
			if err := os.WriteFile(path, wave[:44+2*frames], 0600); err != nil {
				return err
			}
			segments, err := recognize(ctx, path)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, s := range segments {
				s.Start = max(start, min(end, start+s.Start))
				s.End = max(s.Start, min(end, start+s.End))
				if s.Text != "" && s.End > s.Start {
					said(s)
				}
			}
		}
		heard(end)
		if err == io.EOF {
			return nil
		}
	}
}

func readSpeechChunk(ctx context.Context, src Source, samples []float32) (int, error) {
	var n int
	for n < len(samples) {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		read, err := src.Read(samples[n:])
		n += read
		if err != nil {
			return n, err
		}
		if read == 0 {
			return n, io.ErrNoProgress
		}
	}
	return n, nil
}

// writeSpeechWave encodes interleaved audio as a mono 16-bit WAV and
// reports whether there is any nonzero audio for recognition.
func writeSpeechWave(dst []byte, samples []float32, rate, channels int) bool {
	frames := len(samples) / channels
	dataSize := uint32(2 * frames)
	le := binary.LittleEndian
	copy(dst[:4], "RIFF")
	le.PutUint32(dst[4:8], dataSize+36)
	copy(dst[8:16], "WAVEfmt ")
	le.PutUint32(dst[16:20], 16)
	le.PutUint16(dst[20:22], 1) // PCM
	le.PutUint16(dst[22:24], 1) // mono
	le.PutUint32(dst[24:28], uint32(rate))
	le.PutUint32(dst[28:32], uint32(rate*2))
	le.PutUint16(dst[32:34], 2)
	le.PutUint16(dst[34:36], 16)
	copy(dst[36:40], "data")
	le.PutUint32(dst[40:44], dataSize)
	audible := false
	for i := range frames {
		var sum float64
		for _, v := range samples[i*channels : (i+1)*channels] {
			sum += float64(v)
		}
		v := sum / float64(channels)
		if math.IsNaN(v) {
			v = 0
		}
		sample := int16(math.Round(max(-1, min(1, v)) * 32767))
		le.PutUint16(dst[44+2*i:], uint16(sample))
		audible = audible || sample != 0
	}
	return audible
}
