package app

import (
	"context"
	"log"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// NetEase knows a song was played only if the client says so: a stream URL
// leaves nothing in the listener's listening history. So as songs play,
// this settles each playback once, with its measured duration when it ends.

// Like the reference desktop implementation, skip sub-second/no-play events.
// This is a client policy, not a documented NetEase ingestion guarantee.
const scrobbleMin = time.Second

// scrobbleStep is how far the position may move between two notes of a
// playing song without its being a seek: what a jump skips is not heard.
const scrobbleStep = 10 * time.Second

// scrobbler follows the song playing and how much of it has been heard,
// and settles it when it gives way to another or the app quits.
type scrobbler struct {
	report func(id int64, played time.Duration)
	bind   func(id int64) func(time.Duration)
	settle func(time.Duration)

	id      int64
	length  time.Duration
	heard   time.Duration // heard before the stretch now playing
	from    time.Duration // where the stretch now playing began
	far     time.Duration // the furthest of the stretch now playing
	play    bool          // the stretch now playing is playing
	sent    bool          // this playback was handed off, not confirmed ingested
	started bool          // the playback has an account-bound reporter
}

// note follows what plays now: t, at pos, playing or not, and reports the
// song it leaves behind.
func (s *scrobbler) note(t *netease.Track, pos time.Duration, playing bool) {
	if !scrobbled(t) {
		s.stop()
		return
	}
	if t.ID != s.id {
		// Repeats have an explicit player boundary; a seek to zero is not one.
		s.stop()
		s.id, s.length = t.ID, t.Length()
	}
	if playing {
		if !s.started {
			s.started = true
			if s.bind != nil {
				s.settle = s.bind(s.id)
			}
		}
		switch {
		case !s.play:
			s.play, s.from, s.far = true, pos, pos
		case pos > s.far && pos-s.far > scrobbleStep:
			// A seek forward: what was skipped is not heard.
			s.heard += s.far - s.from
			s.from, s.far = pos, pos
		case pos > s.far:
			s.far = pos
		case pos < s.far:
			// A seek back: hear on from there.
			s.heard += s.far - s.from
			s.from, s.far = pos, pos
		}
	} else if s.play {
		if pos >= s.far && pos-s.far <= scrobbleStep {
			s.far = pos
		}
		s.play = false
		s.heard += s.far - s.from
	}
	if !playing && s.length > 0 && pos >= s.length {
		s.stop()
	}
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
		heard += s.far - s.from
	}
	return heard
}

// reportIfEnough reports the song once it has been heard enough.
func (s *scrobbler) reportIfEnough() {
	if s.sent || s.id == 0 || s.progress() < scrobbleMin {
		return
	}
	s.sent = true
	played := s.progress()
	if s.length > 0 && played > s.length {
		played = s.length
	}
	if s.settle != nil {
		s.settle(played)
	} else if s.report != nil {
		s.report(s.id, played)
	}
}

// stop settles the song followed, reporting it if it was heard enough.
func (s *scrobbler) stop() {
	s.reportIfEnough()
	*s = scrobbler{report: s.report, bind: s.bind}
}

// scrobbleEvery is how often what plays is noted, so a song skipped or
// paused is reported with how much of it was heard.
const scrobbleEvery = 5 * time.Second

// noteScrobble notes what plays now, if someone is signed in whose
// listening history may be told.
func (a *app) noteScrobble() {
	if a.quitting {
		return
	}
	if a.profile == nil {
		a.scrob.stop()
		a.scrobbleUser = 0
		return
	}
	if a.scrobbleUser != a.profile.UserID {
		a.scrob.stop()
		a.scrobbleUser = a.profile.UserID
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
		if !a.quitting && a.pl.Playing() {
			a.keepScrobble()
		}
	})
}

// scrobbleAfter runs fn on the main thread after d, without redrawing a
// window as Update would.
var scrobbleAfter = func(d time.Duration, fn func()) {
	time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
}

// bindPlayback captures the account now; only the measured duration is supplied
// later, when the playback finishes. Network failures never block audio playback.
func (a *app) bindPlayback(id int64) func(time.Duration) {
	report := netease.PlaybackReporter(id)
	return func(played time.Duration) {
		a.scrobbleWork.Add(1)
		go func() {
			defer a.scrobbleWork.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := report(ctx, played); err != nil {
				// Do not log URLs, cookies or raw server bodies. A lost response can
				// mean the server accepted it, so do not blindly replay this play.
				log.Printf("listening history: %s; not confirmed", netease.PlaybackFailure(err))
			}
		}()
	}
}
