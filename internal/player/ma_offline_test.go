package player

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

// The optional offline check runs models, MA planning and sequential PCM handoff without opening an audio output.
func TestMAOfflinePipeline(t *testing.T) {
	path := os.Getenv("MYGO_SMART_FADES_OFFLINE")
	if path == "" {
		t.Skip("set MYGO_SMART_FADES_OFFLINE to a synthetic 8-1200 second FLAC or MP3")
	}
	t.Setenv("MYGO_SMART_FADES_DEV", "1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dl := downloaded(data)
	defer dl.release()
	makeTrack := func(id int64) *pcmTrack {
		s, err := openShared(dl, 0)
		if err != nil {
			t.Fatal(err)
		}
		p, err := preparePCM(s, netease.Track{ID: id}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.release)
		profileSource, err := openShared(dl, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer profileSource.Close()
		var dec audio.Decoder = profileSource
		if dec.SampleRate() != outputRate {
			dec = newResampler(dec)
		}
		profile, err := analyzeTrack(dec)
		if err != nil {
			t.Fatal(err)
		}
		profile.neural, err = analyzeNeuralTrack(context.Background(), dl, 0)
		if err != nil {
			t.Fatal(err)
		}
		p.analysis.Store(profile)
		return p
	}
	a, b := makeTrack(1), makeTrack(2)
	plan := planMix(a, b)
	if plan == nil || !plan.full {
		t.Fatal("complete neural analysis did not select MA planner")
	}
	rendered, err := prepareSmartAudio(context.Background(), a, b, *plan)
	if err != nil {
		t.Fatal(err)
	}
	defer rendered.closeResume()
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.pos = plan.sourceStart
	s.beginSmart(s.parts[0], rendered)
	buf := make([]float32, 8192)
	var frames int64
	for {
		n, readErr := s.Read(buf)
		frames += int64(n / 2)
		for _, v := range buf[:n] {
			if !finiteMixNumber(float64(v)) || v < -1.001 || v > 1.001 {
				t.Fatal("render exceeded finite full-scale audio")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		if n == 0 {
			t.Fatal("handoff stalled")
		}
	}
	want := rendered.pre + b.dec.Len() - rendered.enter
	if frames != want {
		t.Fatalf("sequential handoff duplicated/lost frames: got %d want %d", frames, want)
	}
	t.Logf("offline models + MA planner + render + continuation: %d frames; overlap %.3fs", frames, float64(rendered.overlap)/outputRate)
}
