package player

import (
	"io"
	"math"
	"testing"
)

// Crossfading must overlap exact samples with equal-power gains, rather than add silence between songs.
func TestCrossfadeEqualPower(t *testing.T) {
	a, b := testPCM(t, outputRate, 8, .25), testPCM(t, outputRate, 8, .5)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
	got := make([]float32, 40)
	n, err := s.Read(got)
	if n != 24 || err != io.EOF {
		t.Fatalf("samples=%d, error=%v", n, err)
	}
	for i := range n / 2 {
		want := .25
		if i >= 8 {
			want = .5
		} else if i >= 4 {
			phase := float64(i-4) / 4 * math.Pi / 2
			want = .25*math.Cos(phase) + .5*math.Sin(phase)
		}
		for ch := range 2 {
			if math.Abs(float64(got[i*2+ch])-want) > 1e-6 {
				t.Fatalf("frame %d channel %d: %v, want %v", i, ch, got[i*2+ch], want)
			}
		}
	}
	part, _, _ := s.state(4)
	if part.track != b || part.overlap != 4 {
		t.Fatal("metadata did not follow the audible overlap")
	}
}

// Seeking before a decoded overlap must restore its successor; seeking into the new song cancels the old tail.
func TestCrossfadeSeek(t *testing.T) {
	for _, at := range []int64{0, 5} {
		a, b := testPCM(t, outputRate, 8, .25), testPCM(t, outputRate, 8, .5)
		s := newSequence(a, nil)
		s.queue(b)
		s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
		s.Read(make([]float32, 12))
		if err := s.SetPosition(at); err != nil {
			t.Fatal(err)
		}
		if at == 0 {
			s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
		}
		got := make([]float32, 40)
		n, err := s.Read(got)
		want := 24
		if at == 5 {
			want = 14
			for _, sample := range got[:n] {
				if sample != .5 {
					t.Fatal("the outgoing song returned after a seek into the new song")
				}
			}
		}
		if n != want || err != io.EOF {
			t.Fatalf("seek %d: n=%d, error=%v", at, n, err)
		}
		s.Close()
		if a.refs.Load() != 1 || b.refs.Load() != 1 {
			t.Fatal("overlap retained a closed stream")
		}
	}
}

// Replacing a successor after decoded overlap must never replay the removed song when lookahead is flushed.
func TestCrossfadeReplacesLookahead(t *testing.T) {
	a, b, c := testPCM(t, outputRate, 8, .25), testPCM(t, outputRate, 8, .5), testPCM(t, outputRate, 8, .75)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
	s.Read(make([]float32, 12))
	s.queue(c)
	if err := s.SetPosition(3); err != nil {
		t.Fatal(err)
	}
	s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
	got := make([]float32, 40)
	n, err := s.Read(got)
	if n != 18 || err != io.EOF || got[n-1] != .75 || b.refs.Load() != 1 {
		t.Fatalf("replaced overlap: samples=%d, error=%v, old refs=%d", n, err, b.refs.Load())
	}
}
