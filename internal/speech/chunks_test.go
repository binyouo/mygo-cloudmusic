package speech

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type chunkTestAudio struct {
	samples []float32
}

func (s *chunkTestAudio) SampleRate() int { return 4 }
func (s *chunkTestAudio) Channels() int   { return 2 }

func (s *chunkTestAudio) Read(dst []float32) (int, error) {
	n := copy(dst[:min(6, len(dst))], s.samples)
	s.samples = s.samples[n:]
	if len(s.samples) == 0 {
		return n, io.EOF
	}
	return n, nil
}

// The chunk timeline keeps offsets across silence, caps the last chunk, and removes temporary audio from the chosen directory.
func TestSpeechChunkTimeline(t *testing.T) {
	const second = time.Second
	audio := chunkTestAudio{samples: make([]float32, 38*4*2)}
	audio.samples[0] = 1
	audio.samples[32*4*2] = 0.5

	var byteSizes []int
	var heard []time.Duration
	var said []Segment
	temp := t.TempDir()
	recognize := func(_ context.Context, path string) ([]Segment, error) {
		if filepath.Dir(filepath.Dir(path)) != temp {
			t.Fatalf("temporary WAV %q is outside %q", path, temp)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		le := binary.LittleEndian
		if len(b) < 46 || le.Uint16(b[22:24]) != 1 || le.Uint32(b[24:28]) != 4 ||
			le.Uint32(b[40:44]) != uint32(len(b)-44) || le.Uint16(b[44:46]) == 0 {
			t.Fatalf("unexpected WAV: %x", b)
		}
		byteSizes = append(byteSizes, len(b)-44)
		return []Segment{{Start: second / 2, End: 2 * second, Text: "词"}}, nil
	}
	from := 90 * second
	until := from + 33*second
	err := transcribeChunks(t.Context(), temp, &audio, from, until, recognize,
		func(s Segment) { said = append(said, s) },
		func(at time.Duration) { heard = append(heard, at) })
	if err != nil {
		t.Fatal(err)
	}
	if files, err := os.ReadDir(temp); err != nil || len(files) != 0 {
		t.Fatalf("temporary audio was not cleaned up: %v, %v", files, err)
	}
	if want := []int{128, 8}; !slices.Equal(byteSizes, want) {
		t.Fatalf("recognized chunk sizes = %v, want %v", byteSizes, want)
	}
	if want := []time.Duration{106 * second, 122 * second, 123 * second}; !slices.Equal(heard, want) {
		t.Fatalf("heard = %v, want %v", heard, want)
	}
	want := []Segment{
		{Start: 90*second + second/2, End: 92 * second, Text: "词"},
		{Start: 122*second + second/2, End: 123 * second, Text: "词"},
	}
	if !slices.Equal(said, want) {
		t.Fatalf("said = %v, want %v", said, want)
	}
}

// An audible interlude with no recognized words advances the cursor and leaves the following speech at its original time.
func TestSpeechChunkContinuesAfterUnrecognizedAudio(t *testing.T) {
	audio := &chunkTestAudio{samples: make([]float32, 20*4*2)}
	audio.samples[0] = 0.5
	audio.samples[16*4*2] = 0.5
	var heard []time.Duration
	var spoken []Segment
	calls := 0
	recognize := func(context.Context, string) ([]Segment, error) {
		calls++
		if calls == 1 {
			return nil, nil // iOS completes a music-only chunk without text.
		}
		return []Segment{{Start: time.Second, End: 2 * time.Second, Text: "后续人声"}}, nil
	}
	if err := transcribeChunks(t.Context(), t.TempDir(), audio, time.Minute, 0, recognize,
		func(s Segment) { spoken = append(spoken, s) },
		func(at time.Duration) { heard = append(heard, at) }); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !slices.Equal(heard, []time.Duration{76 * time.Second, 80 * time.Second}) {
		t.Fatalf("calls = %d, heard = %v", calls, heard)
	}
	if len(spoken) != 1 || spoken[0].Start != 77*time.Second || spoken[0].End != 78*time.Second {
		t.Fatalf("following speech = %+v", spoken)
	}
}
