//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))

package player

import (
	"math"
	"os"
	"testing"
)

// TestBundledRubberBand prepares a variable-tempo crossfade without retuning the outgoing song or skipping incoming audio.
func TestBundledRubberBand(t *testing.T) {
	const frames = 4 * outputRate
	const ramp = outputRate * 3 / 4
	const tempo = 1.12
	frequencies := [2]float64{440, 659}
	input := make([]float32, frames*2)
	for frame := range frames {
		for ch, frequency := range frequencies {
			input[frame*2+ch] = float32(0.4 * math.Sin(2*math.Pi*frequency*float64(frame)/outputRate))
		}
	}
	data, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := openStream(downloaded(data), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	decoder := newResampler(stream)
	head, headPCM, err := readMixEdge(decoder, 0, decoder.Len(), true)
	if err != nil {
		t.Fatal(err)
	}
	outgoing, incoming := &pcmTrack{}, &pcmTrack{raw: stream}
	outgoing.analysis.Store(&trackAnalysis{tailPCM: input})
	incoming.analysis.Store(&trackAnalysis{head: head, headPCM: headPCM})
	rendered, err := prepareSmartAudio(t.Context(), outgoing, incoming, mixPlan{
		exit: frames, ramp: ramp, overlap: frames - ramp, tempo: tempo,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rendered.closeResume()
	output := rendered.outgoing
	want := stretchedTailFrames(frames, tempo, ramp)
	if got := int64(len(output) / 2); got < want-outputRate/10 || got > want+outputRate/10 {
		t.Fatalf("stretched tail has %d frames, want about %d", got, want)
	}
	// After the ramp, count one second of positive crossings in each channel.
	for ch, want := range frequencies {
		crossings := 0
		for frame := outputRate; frame < 2*outputRate; frame++ {
			if output[(frame-1)*2+ch] <= 0 && output[frame*2+ch] > 0 {
				crossings++
			}
		}
		if math.Abs(float64(crossings)-want) > 3 {
			t.Errorf("channel %d pitch is about %d Hz, want %.0f Hz", ch, crossings, want)
		}
	}
	// The incoming decoder must begin where the rendered audio hands it back.
	resumeAt := 2 * (rendered.overlap + rendered.release)
	resumed := make([]float32, 512)
	if n, err := rendered.resume.decoder.Read(resumed); n != len(resumed) || err != nil {
		t.Fatalf("resume read %d samples: %v", n, err)
	}
	if diff := maxDiff(resumed, headPCM[resumeAt:resumeAt+512]); diff > 1e-4 {
		t.Fatalf("incoming cut differs from sequential decode by %.6f", diff)
	}
}
