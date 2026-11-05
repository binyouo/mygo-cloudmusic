package app

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// recorded is a report the scrobbler sent, for the tests to read.
type recorded struct {
	id     int64
	played time.Duration
}

func newScrobbler(got *[]recorded) *scrobbler {
	s := &scrobbler{}
	s.report = func(id int64, played time.Duration) {
		*got = append(*got, recorded{id, played})
	}
	return s
}

// A song is reported once it has played 30 seconds, and once only.
func TestScrobblesASongOnceHeardEnough(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 200000}
	for pos := time.Duration(0); pos <= 60*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	if len(got) != 1 || got[0].id != 7 || got[0].played != 30*time.Second {
		t.Fatalf("reported %v", got)
	}
	s.stop()
	if len(got) != 1 {
		t.Fatalf("reported twice: %v", got)
	}
}

// A song left before it played enough is not recorded.
func TestScrobbleNeedsEnough(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 200000}
	for pos := time.Duration(0); pos <= 25*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	s.note(tr, 25*time.Second, false)
	s.stop()
	if len(got) != 0 {
		t.Fatalf("reported %v", got)
	}
}

// What a seek forward skips is not time heard, so it does not count toward
// the thirty seconds.
func TestScrobbleDoesNotCountASeek(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 300000}
	for pos := time.Duration(0); pos <= 10*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	for pos := 100 * time.Second; pos <= 130*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	if len(got) != 1 || got[0].played != 30*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// Picking a song up where it was left does not count what was heard before.
func TestScrobbleResumesFromWhereItStopped(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 200000}
	s.note(tr, 42*time.Second, false)
	for pos := 42 * time.Second; pos <= 72*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	if len(got) != 1 || got[0].played != 30*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// A song shorter than a minute counts once half of it has played.
func TestScrobblesAHalfOfAShortSong(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 20000}
	s.note(tr, 0, true)
	s.note(tr, 10*time.Second, true)
	if len(got) != 1 || got[0].played != 10*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// A song from the listener's own files or a podcast episode is not a
// NetEase song, and is not reported.
func TestScrobbleLeavesLocalAndPodcastSongs(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	for _, tr := range []*netease.Track{
		{ID: 7, LocalPath: "/tmp/song.mp3", Duration: 200000},
		{ID: 8, ProgramID: 9, Duration: 200000},
		{ID: 0, Duration: 200000},
	} {
		for pos := time.Duration(0); pos <= 60*time.Second; pos += 5 * time.Second {
			s.note(tr, pos, true)
		}
	}
	if len(got) != 0 {
		t.Fatalf("reported %v", got)
	}
}

// The app notes what the player says it plays, and what it has heard of it.
func TestNoteScrobbleFollowsThePlayer(t *testing.T) {
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 9, Nickname: "听歌的人"}
	var got []recorded
	a.scrob.report = func(id int64, played time.Duration) {
		got = append(got, recorded{id, played})
	}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.Hold(30 * time.Second) // where a restored song waits
	a.noteScrobble()
	if len(got) != 0 {
		t.Fatalf("reported a resumed song at once: %v", got)
	}
	for pos := 35 * time.Second; pos <= 60*time.Second; pos += 5 * time.Second {
		a.pl.Hold(pos)
		a.noteScrobble()
	}
	if len(got) != 1 || got[0].id != 7 || got[0].played != 30*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// Signed out, nothing plays into a listening history.
func TestScrobbleNeedsALogin(t *testing.T) {
	a := newTestApp()
	var got []recorded
	a.scrob.report = func(id int64, played time.Duration) {
		got = append(got, recorded{id, played})
	}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.pl.Hold(0)
	for pos := time.Duration(0); pos <= 60*time.Second; pos += 5 * time.Second {
		a.pl.Hold(pos)
		a.noteScrobble()
	}
	if len(got) != 0 {
		t.Fatalf("reported %v signed out", got)
	}
}
