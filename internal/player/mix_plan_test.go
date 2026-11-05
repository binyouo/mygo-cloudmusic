package player

import (
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

func analyzedPCM(t *testing.T, seconds float64, amplitude func(float64) float64) *pcmTrack {
	t.Helper()
	frames := int(seconds * outputRate)
	data := make([]float32, frames*2)
	for i := range frames {
		at := float64(i) / outputRate
		v := float32(math.Sin(at*220*2*math.Pi) * amplitude(at))
		data[i*2], data[i*2+1] = v, v
	}
	p, err := preparePCM(&samplesStream{samples: data, rate: outputRate}, netease.Track{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.release)
	a, err := analyzeTrack(&samplesStream{samples: data, rate: outputRate})
	if err != nil {
		t.Fatal(err)
	}
	p.analysis.Store(a)
	return p
}

// Natural decays of different lengths must produce different handoffs, not a hidden four-second fade.
func TestAdaptiveMixFollowsOutro(t *testing.T) {
	b := analyzedPCM(t, 20, func(at float64) float64 {
		if at < 10 {
			return .025
		}
		return .4
	})
	var durations []int64
	for _, fade := range []float64{3, 8} {
		a := analyzedPCM(t, 20, func(at float64) float64 { return .4 * min(1, (20-at)/fade) })
		plan := planMix(a, b)
		if plan == nil {
			t.Fatal("a measured decay did not produce a handoff")
		}
		t.Logf("outro %.0fs: overlap %.2fs", fade, float64(plan.overlap)/outputRate)
		durations = append(durations, plan.overlap)
	}
	if durations[1] < durations[0]*2 || durations[0] >= 4*outputRate {
		t.Fatalf("outro lengths ignored: overlaps %v", durations)
	}
}

// Seeking back across an adaptive join must restore the complete queued head.
func TestAdaptiveMixSeekBackAcrossJoin(t *testing.T) {
	a := analyzedPCM(t, 15, func(at float64) float64 { return .4 * max(0, min(1, (14-at)/5)) })
	b := analyzedPCM(t, 15, func(at float64) float64 {
		if at < 1 {
			return 0
		}
		return .4
	})
	plan := planMix(a, b)
	if plan == nil {
		t.Fatal("missing test handoff")
	}
	s := newSequence(a, nil)
	defer s.Close()
	s.adaptive = true
	s.queue(b)
	start := plan.exit - plan.overlap
	if err := s.SetPosition(start); err != nil {
		t.Fatal(err)
	}
	s.Read(make([]float32, outputRate))
	if err := s.SetPosition(0); err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 60*outputRate)
	n, err := s.Read(buf)
	want := (a.dec.Len() + b.dec.Len() - plan.overlap) * 2
	if int64(n) != want || err != io.EOF {
		t.Fatalf("restored cue duplicated/lost audio: %d, want %d; %v", n, want, err)
	}
	part, _, _ := s.state(start)
	if part.offset != plan.enter || part.overlap != plan.overlap {
		t.Fatal("restored transition lost its cue")
	}
}

// Adaptive planning must preserve album, repeat and podcast boundaries just as fixed mixing does.
func TestAdaptiveMixKeepsOriginalBoundaries(t *testing.T) {
	a := analyzedPCM(t, 15, func(at float64) float64 { return .4 * min(1, (15-at)/5) })
	b := analyzedPCM(t, 15, func(float64) float64 { return .4 })
	for _, kind := range []string{"album", "repeat", "podcast"} {
		a.track, b.track = netease.Track{}, netease.Track{}
		switch kind {
		case "album":
			a.track.Album.ID, b.track.Album.ID = 7, 7
		case "repeat":
			a.track.ID, b.track.ID = 1, 1
		case "podcast":
			b.track.ProgramID = 7
		}
		if planMix(a, b) != nil {
			t.Fatalf("%s boundary was mixed", kind)
		}
	}
}

// Strong unrelated hard endings and missing analysis keep the complete boundary instead of forcing an overlap.
func TestAdaptiveMixRejectsUncertainAudio(t *testing.T) {
	a := analyzedPCM(t, 15, func(float64) float64 { return .4 })
	b := analyzedPCM(t, 15, func(float64) float64 { return .4 })
	if planMix(a, b) != nil {
		t.Fatal("sustained audio acquired an invented beat/outro")
	}
	b.analysis.Store(nil)
	if planMix(a, b) != nil {
		t.Fatal("missing analysis forced a timed fade")
	}
	s := newSequence(a, nil)
	defer s.Close()
	s.adaptive = true
	s.queue(b)
	if err := s.SetPosition(14 * outputRate); err != nil {
		t.Fatal(err)
	}
	n, err := s.Read(make([]float32, 17*outputRate*2))
	if n != 16*outputRate*2 || err != io.EOF {
		t.Fatalf("fallback lost samples: %d, %v", n, err)
	}
}

// Adaptive mixing must retain boundary silence, song-time seeking and every prepared stream reference.
func TestAdaptiveMixSilenceAndSeek(t *testing.T) {
	a := analyzedPCM(t, 15, func(at float64) float64 { return .4 * max(0, min(1, (14-at)/5)) })
	b := analyzedPCM(t, 15, func(at float64) float64 {
		if at < 1 {
			return 0
		}
		return .4
	})
	plan := planMix(a, b)
	if plan == nil || plan.enter != 0 || plan.exit != a.dec.Len() {
		t.Fatalf("adaptive mixing cropped the original audio: %+v", plan)
	}
	s := newSequence(a, nil)
	s.adaptive = true
	s.queue(b)
	if err := s.SetPosition(plan.exit - plan.overlap); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(make([]float32, outputRate)); err != nil {
		t.Fatal(err)
	}
	part, _, _ := s.state(plan.exit - plan.overlap)
	if part.track != b || part.offset != plan.enter {
		t.Fatal("incoming song time lost its cue offset")
	}
	if err := s.SetPosition(part.start + outputRate); err != nil {
		t.Fatal(err)
	}
	if b.raw.(*samplesStream).pos != plan.enter+outputRate {
		t.Fatal("seek did not use the original song time")
	}
	s.Close()
	if a.refs.Load() != 1 || b.refs.Load() != 1 {
		t.Fatal("adaptive overlap leaked a decoder")
	}
	p := &Player{partStart: part.start, partOffset: part.offset}
	if got := p.audioPosition(time.Second); got != time.Duration(part.start-part.offset+outputRate)*time.Second/outputRate {
		t.Fatal("visible song time no longer maps to the output clock")
	}
}

// Quiet musical frames below the analysis floor must never be physically removed from a transition.
func TestAdaptiveMixKeepsQuietBoundaries(t *testing.T) {
	a := analyzedPCM(t, 20, func(at float64) float64 {
		if at > 19 {
			return .0002
		}
		return .4 * min(1, (20-at)/8)
	})
	b := analyzedPCM(t, 20, func(at float64) float64 {
		if at < 2 {
			return .0002
		}
		return .4
	})
	plan := planMix(a, b)
	if plan == nil {
		t.Fatal("missing quiet-boundary fixture handoff")
	}
	if plan.enter != 0 || plan.exit != a.dec.Len() {
		t.Fatal("low-volume music was treated as disposable silence")
	}
	s := newSequence(a, nil)
	defer s.Close()
	s.adaptive = true
	s.queue(b)
	n, err := s.Read(make([]float32, (a.dec.Len()+b.dec.Len())*2+2))
	if int64(n) != (a.dec.Len()+b.dec.Len()-plan.overlap)*2 || err != io.EOF {
		t.Fatalf("quiet boundary frames were removed: %d samples, %v", n, err)
	}
}

// A seek to the displayed song's ending must not select its already-decoded successor.
func TestSeekKeepsDisplayedSong(t *testing.T) {
	a, b := testPCM(t, outputRate, 8, .25), testPCM(t, outputRate, 8, .5)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
	s.Read(make([]float32, 12)) // Decoder is inside B, but A is still the visible song.
	s.requestSongSeek(0, 7)
	part, pending, _ := s.state(7)
	if part.track != a || !pending {
		t.Fatal("seek announced/discarded the requested song before the decoder handled it")
	}
	if err := s.SetPosition(7); err != nil {
		t.Fatal(err)
	}
	part, _, _ = s.state(7)
	if part.track != a {
		t.Fatal("seeking inside A selected the mixed successor")
	}
	buf := make([]float32, 2)
	if n, err := s.Read(buf); n != 2 || err != nil || buf[0] != .25 || buf[1] != .25 {
		t.Fatalf("the requested ending was remixed or skipped: %v, %d, %v", buf, n, err)
	}
	if n, err := s.Read(make([]float32, 20)); n != 16 || err != io.EOF {
		t.Fatalf("successor did not restart from its full head: %d, %v", n, err)
	}
}

// Equal-power overlap must preserve stereo balance without emitting peaks outside the decoder's range.
func TestCrossfadeProtectsCombinedPeaks(t *testing.T) {
	a, b := testPCM(t, outputRate, 8, 1), testPCM(t, outputRate, 8, 1)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.plan = &mixPlan{exit: a.dec.Len(), overlap: 4}
	buf := make([]float32, 32)
	n, _ := s.Read(buf)
	for i := 0; i < n; i += 2 {
		if buf[i] < -1 || buf[i] > 1 || buf[i] != buf[i+1] {
			t.Fatalf("unsafe or unbalanced mixed peak at %d: %v %v", i, buf[i], buf[i+1])
		}
	}
}

// Beat-aligned material may overlap, while incompatible tempos must not produce a forced rhythmic mix.
func TestAdaptiveMixBeatCompatibility(t *testing.T) {
	pulses := func(period float64) func(float64) float64 {
		return func(at float64) float64 {
			phase := math.Mod(at, period)
			return .015 + .4*math.Exp(-phase*80)
		}
	}
	a := analyzedPCM(t, 20, pulses(.5))
	b := analyzedPCM(t, 20, pulses(.5))
	c := analyzedPCM(t, 20, pulses(.36))
	matched, unmatched := planMix(a, b), planMix(a, c)
	if matched == nil || matched.overlap < outputRate {
		t.Fatalf("regular shared beat not used: %+v, grid=%+v", matched, a.analysis.Load().tail)
	}
	if unmatched != nil && unmatched.overlap >= outputRate {
		t.Fatalf("incompatible grids forced together: %+v", unmatched)
	}
}

type failedMixDecoder struct{ audio.Decoder }

func (failedMixDecoder) Read([]float32) (int, error) { return 0, io.ErrUnexpectedEOF }

// Incomplete downloaded audio must not publish fabricated silence boundaries.
func TestMixAnalysisRejectsTruncatedStream(t *testing.T) {
	d := &samplesStream{samples: make([]float32, outputRate*20), rate: outputRate}
	if _, err := analyzeTrack(failedMixDecoder{d}); err == nil {
		t.Fatal("truncated analysis succeeded")
	}
}

// Real streamed songs must support independent intro/outro analysis without a second download.
func TestAdaptiveMixLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to analyze real streamed songs")
	}
	tracks := []netease.Track{
		{ID: 1901371647, Name: "孤勇者", Artists: []netease.Artist{{Name: "陈奕迅"}}, Duration: 256000, Fee: 1},
		{ID: 531051217, Name: "等你下课 (with 杨瑞代)", Artists: []netease.Artist{{Name: "周杰伦"}, {Name: "杨瑞代"}}, Duration: 270000},
	}
	var prepared []*pcmTrack
	for _, track := range tracks {
		_, s, err := (&Player{}).open(track, "exhigh")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		dl, _ := s.source()
		position := func() int64 {
			switch v := s.(type) {
			case *mp3Stream:
				return v.pos
			case *flacStream:
				return v.pos
			}
			return -1
		}
		before, _ := s.Read(make([]float32, 2048))
		if before == 0 {
			t.Fatal("playback decoder not ready")
		}
		at := position()
		shared, err := openShared(dl, track.Duration)
		if err != nil {
			t.Fatal(err)
		}
		var dec audio.Decoder = shared
		if shared.SampleRate() != outputRate {
			dec = newResampler(shared)
		}
		profile, err := analyzeTrack(dec)
		shared.Close()
		if position() != at {
			t.Fatal("outro analysis moved the playback decoder")
		}
		if err != nil {
			t.Fatalf("%s: %v", track.Name, err)
		}
		if profile.head.last == 0 || profile.tail.last == 0 {
			t.Fatal("real audio was mistaken for silence")
		}
		pcm := &pcmTrack{track: track, dec: dec}
		pcm.analysis.Store(profile)
		prepared = append(prepared, pcm)
		t.Logf("%s: tail %.2f..%.2fs, beat %.3fs confidence %.2f", track.Name, float64(profile.tail.first)/outputRate, float64(profile.tail.last)/outputRate, float64(profile.tail.beat)/outputRate, profile.tail.confidence)
	}
	for i := range prepared {
		if plan := planMix(prepared[i], prepared[1-i]); plan != nil {
			t.Logf("%s → %s: exit=%.2fs enter=%.2fs overlap=%.2fs", tracks[i].Name, tracks[1-i].Name, float64(plan.exit)/outputRate, float64(plan.enter)/outputRate, float64(plan.overlap)/outputRate)
		} else {
			t.Logf("%s → %s: preserve original boundary", tracks[i].Name, tracks[1-i].Name)
		}
	}
}
