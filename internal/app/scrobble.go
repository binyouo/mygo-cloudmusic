package app

import (
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// NetEase knows a song was played only if the client says so: a stream URL
// leaves nothing in the listener's listening history. So as songs play,
// this reports them, once each, once enough of one has been heard.

// reportPlay tells NetEase a song played; tests replace it.
var reportPlay = func(id int64, played time.Duration) error {
	return netease.Scrobble(id, id, played, "list")
}

// scrobbleMin is how much of a song counts as heard: NetEase records one
// played over 30 seconds, or half of a shorter one.
const scrobbleMin = 30 * time.Second

// scrobbleLength is how long a song of length d must play to be recorded.
func scrobbleLength(d time.Duration) time.Duration {
	if d > 0 && d/2 < scrobbleMin {
		return d / 2
	}
	return scrobbleMin
}

// scrobbleStep is how far the position may move between two notes of a
// playing song without its being a seek: what a jump skips is not heard.
const scrobbleStep = 10 * time.Second

// scrobbler follows the song playing and how much of it has been heard,
// and reports each song once enough of it plays, or when it gives way to
// another, or the app quits.
type scrobbler struct {
	report func(id int64, played time.Duration)

	id     int64
	length time.Duration
	heard  time.Duration // heard before the stretch now playing
	start  time.Duration // where the stretch now playing began
	far    time.Duration // the furthest of the stretch now playing
	play   bool          // the stretch now playing is playing
	sent   bool          // the song was reported
}

// note follows what plays now: t, at pos, playing or not, and reports the
// song it leaves behind.
func (s *scrobbler) note(t *netease.Track, pos time.Duration, playing bool) {
	if !scrobbled(t) {
		s.stop()
		return
	}
	if t.ID != s.id || pos == 0 && s.heard+max(0, s.far-s.start) > 0 {
		// Another song, or the same one from its beginning again.
		s.stop()
		s.id, s.length = t.ID, t.Length()
	}
	if playing {
		switch {
		case !s.play:
			s.play, s.start, s.far = true, pos, pos
		case pos > s.far && pos-s.far > scrobbleStep:
			// A seek forward: what was skipped is not heard.
			s.heard += s.far - s.start
			s.start, s.far = pos, pos
		case pos > s.far:
			s.far = pos
		case pos < s.far:
			// A seek back: hear on from there.
			s.heard += s.far - s.start
			s.start, s.far = pos, pos
		}
	} else if s.play {
		s.play = false
		s.heard += s.far - s.start
	}
	s.reportIfEnough()
}

// scrobbled reports whether t is a song NetEase may be told about: one of
// its own, not a local file and not a podcast episode.
func scrobbled(t *netease.Track) bool {
	return t != nil && t.ID > 0 && t.ProgramID == 0 && t.LocalPath == ""
}

// progress is how much of the song has been heard.
func (s *scrobbler) progress() time.Duration {
	heard := s.heard
	if s.play {
		heard += s.far - s.start
	}
	return heard
}

// reportIfEnough reports the song once it has been heard enough.
func (s *scrobbler) reportIfEnough() {
	if s.sent || s.id == 0 || s.progress() < scrobbleLength(s.length) {
		return
	}
	s.sent = true
	played := s.progress()
	if s.length > 0 && played > s.length {
		played = s.length
	}
	if s.report != nil {
		s.report(s.id, played)
	}
}

// stop settles the song followed, reporting it if it was heard enough.
func (s *scrobbler) stop() {
	s.reportIfEnough()
	*s = scrobbler{report: s.report}
}

// scrobbleEvery is how often what plays is noted, so a song skipped or
// paused is reported with how much of it was heard.
const scrobbleEvery = 5 * time.Second

// noteScrobble notes what plays now, if someone is signed in whose
// listening history may be told.
func (a *app) noteScrobble() {
	if a.profile == nil {
		a.scrob.stop()
		return
	}
	a.scrob.note(a.pl.Current(), a.pl.Position(), a.pl.Playing())
}

// keepScrobble notes what plays a while after playback changed, and every
// while again as it plays. Paused, nothing more is due.
func (a *app) keepScrobble() {
	if a.scrobbleDue {
		return
	}
	a.scrobbleDue = true
	scrobbleAfter(scrobbleEvery, func() {
		a.scrobbleDue = false
		a.noteScrobble()
		if a.pl.Playing() {
			a.keepScrobble()
		}
	})
}

// scrobbleAfter runs fn on the main thread after d, without redrawing a
// window as Update would.
var scrobbleAfter = func(d time.Duration, fn func()) {
	time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
}

// postPlay reports a song from a goroutine: NetEase answers by network.
func (a *app) postPlay(id int64, played time.Duration) {
	go func() { _ = reportPlay(id, played) }()
}
