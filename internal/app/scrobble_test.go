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

// The account is bound once playback starts, not on pause or resume.
func TestScrobbleStartsOnlyOnPlayback(t *testing.T) {
	var started []int64
	s := &scrobbler{bind: func(id int64) func(time.Duration) {
		started = append(started, id)
		return nil
	}}
	a, b := &netease.Track{ID: 7, Duration: 200000}, &netease.Track{ID: 8, Duration: 200000}
	s.note(a, 0, false)
	if len(started) != 0 {
		t.Fatal("reported a paused song")
	}
	s.note(a, 0, true)
	s.note(a, 5*time.Second, true)
	s.note(a, 5*time.Second, false)
	s.note(a, 5*time.Second, true)
	s.note(b, 0, true)
	if len(started) != 2 || started[0] != 7 || started[1] != 8 {
		t.Fatalf("starts %v", started)
	}
}

// A playback is settled once with its measured duration, not truncated at 30 seconds.
func TestScrobblesASongOnceHeardEnough(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 200000}
	for pos := time.Duration(0); pos <= 60*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	if len(got) != 0 {
		t.Fatalf("reported before settlement: %v", got)
	}
	s.stop()
	if len(got) != 1 || got[0].id != 7 || got[0].played != 60*time.Second {
		t.Fatalf("reported %v", got)
	}
	s.stop()
	if len(got) != 1 {
		t.Fatalf("reported twice: %v", got)
	}
}

// A sub-second playback is not sent as a completed listening event.
func TestScrobbleNeedsEnough(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 200000}
	s.note(tr, 0, true)
	s.note(tr, 500*time.Millisecond, false)
	s.stop()
	if len(got) != 0 {
		t.Fatalf("reported %v", got)
	}
}

// A seek forward does not inflate the measured listening duration.
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
	s.stop()
	if len(got) != 1 || got[0].played != 40*time.Second {
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
	s.stop()
	if len(got) != 1 || got[0].played != 30*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// A short song reports its measured partial duration when settled.
func TestScrobblesAHalfOfAShortSong(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 20000}
	s.note(tr, 0, true)
	s.note(tr, 10*time.Second, true)
	s.stop()
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
	a.scrob.stop()
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

// Finishing the last song settles it even when no next track is selected.
func TestScrobbleSettlesAtEnd(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 40000}
	for pos := time.Duration(0); pos < 40*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	s.note(tr, 40*time.Second, false)
	s.stop()
	if len(got) != 1 || got[0].played != 40*time.Second {
		t.Fatalf("reported %v", got)
	}
}

// A settled playback uses the account-bound closure from its start.
func TestScrobbleKeepsBoundReporter(t *testing.T) {
	account, got := 1, 0
	s := &scrobbler{bind: func(int64) func(time.Duration) {
		owner := account
		return func(time.Duration) { got = owner }
	}}
	tr := &netease.Track{ID: 7, Duration: 40000}
	for pos := time.Duration(0); pos <= 30*time.Second; pos += 5 * time.Second {
		s.note(tr, pos, true)
	}
	account = 2
	s.stop()
	if got != 1 {
		t.Fatalf("reported as account %d", got)
	}
}

// Seeking back to zero is still the same playback and must not submit it early.
func TestScrobbleSeekToZeroDoesNotSettle(t *testing.T) {
	var got []recorded
	s := newScrobbler(&got)
	tr := &netease.Track{ID: 7, Duration: 40000}
	s.note(tr, 0, true)
	s.note(tr, 5*time.Second, true)
	s.note(tr, 0, true)
	if len(got) != 0 {
		t.Fatal("seek ended the listening session")
	}
	s.note(tr, 5*time.Second, true)
	s.stop()
	if len(got) != 1 || got[0].played != 10*time.Second {
		t.Fatalf("reported %v", got)
	}
}
