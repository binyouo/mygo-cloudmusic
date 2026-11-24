package mobileplayback

import (
	"time"

	"netease-music-mygo/internal/netease"
)

type playbackCommand struct {
	Action       string
	Position     float64
	ShouldResume bool
}

type transportPlayer interface {
	Current() *netease.Track
	Playing() bool
	TogglePlay()
	Interrupt()
	Next()
	Prev()
	Seek(time.Duration)
}

// remoteTransport applies system commands to the same player as the app.
// An interruption may resume only the specific selection that it paused.
type remoteTransport struct {
	player      transportPlayer
	pausedTrack *netease.Track
}

func (t *remoteTransport) playbackChanged() {
	if t.player.Playing() || t.player.Current() != t.pausedTrack {
		t.pausedTrack = nil
	}
}

func (t *remoteTransport) setPlaying(play bool) {
	if t.player.Current() != nil && play != t.player.Playing() {
		t.player.TogglePlay()
	}
}

func (t *remoteTransport) handle(command playbackCommand) {
	switch command.Action {
	case "interruptionBegan":
		if t.player.Playing() {
			t.pausedTrack = t.player.Current()
		}
		t.player.Interrupt()
	case "interruptionEnded":
		paused := t.pausedTrack
		t.pausedTrack = nil
		if command.ShouldResume && paused != nil && t.player.Current() == paused {
			t.setPlaying(true)
		}
	case "routeDisconnected":
		t.pausedTrack = nil
		t.player.Interrupt()
	case "play", "pause":
		// A deliberate pause also cancels recovery when we are already paused.
		t.pausedTrack = nil
		t.setPlaying(command.Action == "play")
	case "toggle":
		t.pausedTrack = nil
		t.setPlaying(!t.player.Playing())
	case "previous":
		t.pausedTrack = nil
		t.player.Prev()
	case "next":
		t.pausedTrack = nil
		t.player.Next()
	case "seek":
		if t.player.Current() != nil {
			t.player.Seek(time.Duration(command.Position * float64(time.Second)))
		}
	}
}
