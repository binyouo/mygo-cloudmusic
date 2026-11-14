package player

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

// A decoded transition must preserve the output and pause state, and only
// change song metadata and relative position when the boundary is heard.
func TestAudibleTransitionKeepsOutput(t *testing.T) {
	oldArt, oldLyrics := fetchArtwork, fetchLyrics
	fetchArtwork = func(string) ([]byte, error) { return nil, errors.New("no artwork") }
	fetchLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }
	defer func() { fetchArtwork, fetchLyrics = oldArt, oldLyrics }()
	a, b := testPCM(t, 48000, 48000, .25), testPCM(t, 48000, 48000, .5)
	a.track = netease.Track{ID: 1, Duration: 1000}
	b.track = netease.Track{ID: 2, Duration: 1000}
	seq := newSequence(a, nil)
	defer seq.Close()
	seq.queue(b)
	seq.Read(make([]float32, 50000*2))
	// A separate, paused audio clock lets the test place the heard position
	// exactly, without depending on the device or wall clock.
	out := audio.NewSound(48000, 2, make([]float32, 96000*2)).NewPlayer()
	defer out.Close()
	posts := make(chan func(), 4)
	p := &Player{post: func(fn func()) { posts <- fn }, out: out, sequence: seq, stream: a.raw}
	p.Cue([]netease.Track{a.track, b.track})
	out.Seek(900 * time.Millisecond)
	p.pollAudio(seq)
	if p.Current().ID != 1 {
		t.Fatal("metadata changed before boundary")
	}
	out.Seek(1010 * time.Millisecond)
	p.pollAudio(seq)
	next(t, posts) // lyrics completion
	if p.Current().ID != 2 || p.at != 1 || p.out != out || p.playing || out.Playing() {
		t.Fatal("transition replaced output, lost queue position or resumed playback")
	}
	if p.Position() != 10*time.Millisecond {
		t.Fatalf("relative position %v", p.Position())
	}
}

// RepeatOne must close the old listening session before announcing the restarted playback.
func TestRepeatOneAnnouncesPlaybackBoundary(t *testing.T) {
	out := audio.NewSound(48000, 2, make([]float32, 48000*2)).NewPlayer()
	defer out.Close()
	var events []string
	p := &Player{out: out, repeat: RepeatOne, playing: true}
	p.Ended = func() { events = append(events, "ended") }
	p.Changed = func() { events = append(events, "changed") }
	p.ended()
	if len(events) != 2 || events[0] != "ended" || events[1] != "changed" {
		t.Fatalf("playback boundary %v", events)
	}
}

// Two gapless parts with the same song ID still represent two separate playbacks.
func TestSameSongGaplessBoundary(t *testing.T) {
	oldArt, oldLyrics := fetchArtwork, fetchLyrics
	fetchArtwork = func(string) ([]byte, error) { return nil, errors.New("no artwork") }
	fetchLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }
	defer func() { fetchArtwork, fetchLyrics = oldArt, oldLyrics }()
	a, b := testPCM(t, 48000, 48000, .25), testPCM(t, 48000, 48000, .5)
	a.track = netease.Track{ID: 1, Duration: 1000}
	b.track = a.track
	seq := newSequence(a, nil)
	defer seq.Close()
	seq.queue(b)
	seq.Read(make([]float32, 50000*2))
	out := audio.NewSound(48000, 2, make([]float32, 96000*2)).NewPlayer()
	defer out.Close()
	posts := make(chan func(), 4)
	p := &Player{post: func(fn func()) { posts <- fn }, out: out, sequence: seq, stream: a.raw}
	p.Cue([]netease.Track{a.track, b.track})
	boundaries := 0
	p.Ended = func() {
		boundaries++
		if p.partStart != 0 {
			t.Fatal("old playback already replaced")
		}
	}
	out.Seek(1010 * time.Millisecond)
	p.pollAudio(seq)
	next(t, posts)
	if boundaries != 1 || p.at != 1 {
		t.Fatalf("boundaries=%d, index=%d", boundaries, p.at)
	}
}

// The final EOF closes the session explicitly rather than trusting metadata duration.
func TestFinalEOFAnnouncesPlaybackBoundary(t *testing.T) {
	var events []string
	p := &Player{repeat: RepeatOff, playing: true, at: 0, order: []int{0}}
	p.Ended = func() { events = append(events, "ended") }
	p.Changed = func() { events = append(events, "changed") }
	p.ended()
	if p.playing || len(events) != 2 || events[0] != "ended" || events[1] != "changed" {
		t.Fatalf("final playback boundary %v, playing=%v", events, p.playing)
	}
}
