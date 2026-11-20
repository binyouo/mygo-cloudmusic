package mobileplayback

import (
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

type testTransportPlayer struct {
	track       *netease.Track
	playing     bool
	interrupted int
	position    time.Duration
	changed     func()
}

func (p *testTransportPlayer) Current() *netease.Track { return p.track }
func (p *testTransportPlayer) Playing() bool           { return p.playing }
func (p *testTransportPlayer) TogglePlay() {
	p.playing = !p.playing
	p.changed()
}
func (p *testTransportPlayer) Interrupt() {
	p.interrupted++
	p.playing = false
	p.changed()
}
func (p *testTransportPlayer) Next() {
	p.track = &netease.Track{ID: p.track.ID + 1}
	p.changed()
}
func (p *testTransportPlayer) Prev() {
	p.track = &netease.Track{ID: p.track.ID - 1}
	p.changed()
}
func (p *testTransportPlayer) Seek(position time.Duration) {
	p.position = position
	p.changed()
}

// The system transport buttons drive the active queue and seek the selected song.
func TestSystemTransport(t *testing.T) {
	p := &testTransportPlayer{track: &netease.Track{ID: 7}, playing: true}
	transport := remoteTransport{player: p}
	p.changed = transport.playbackChanged

	transport.handle(playbackCommand{Action: "next"})
	if p.track.ID != 8 {
		t.Fatalf("next selected track %d, want 8", p.track.ID)
	}
	transport.handle(playbackCommand{Action: "previous"})
	if p.track.ID != 7 {
		t.Fatalf("previous selected track %d, want 7", p.track.ID)
	}
	transport.handle(playbackCommand{Action: "seek", Position: 37.5})
	if p.position != 37500*time.Millisecond {
		t.Fatalf("seek landed at %v, want 37.5s", p.position)
	}
	transport.handle(playbackCommand{Action: "pause"})
	if p.playing {
		t.Fatal("system pause left the track playing")
	}
	transport.handle(playbackCommand{Action: "play"})
	if !p.playing {
		t.Fatal("system play did not resume the track")
	}
}

// An interrupted song resumes at its current position only when the system recommends it and no user action has superseded it.
func TestAudioInterruptionRecovery(t *testing.T) {
	for _, tc := range []struct {
		name        string
		playing     bool
		allowed     bool
		during      func(*remoteTransport, *testTransportPlayer)
		wantResumed bool
	}{
		{name: "already paused", allowed: true},
		{name: "system declines resume", playing: true},
		{name: "same song", playing: true, allowed: true, wantResumed: true},
		{
			name: "Siri pause cancels resume", playing: true, allowed: true,
			during: func(t *remoteTransport, _ *testTransportPlayer) { t.handle(playbackCommand{Action: "pause"}) },
		},
		{
			name: "headphones disconnected", playing: true, allowed: true,
			during: func(t *remoteTransport, _ *testTransportPlayer) {
				t.handle(playbackCommand{Action: "routeDisconnected"})
			},
		},
		{
			name: "reselected the same song", playing: true, allowed: true,
			during: func(_ *remoteTransport, p *testTransportPlayer) {
				selectedAgain := *p.track
				p.track = &selectedAgain
				p.changed()
			},
		},
		{
			name: "manual play then pause", playing: true, allowed: true,
			during: func(_ *remoteTransport, p *testTransportPlayer) {
				p.TogglePlay()
				p.TogglePlay()
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &testTransportPlayer{track: &netease.Track{ID: 7}, playing: tc.playing, position: 37 * time.Second}
			transport := remoteTransport{player: p}
			p.changed = transport.playbackChanged
			transport.handle(playbackCommand{Action: "interruptionBegan"})
			transport.handle(playbackCommand{Action: "interruptionBegan"})
			if p.Playing() || p.interrupted != 2 {
				t.Fatal("interruption did not stop playback immediately")
			}
			if tc.during != nil {
				tc.during(&transport, p)
			}
			transport.handle(playbackCommand{Action: "interruptionEnded", ShouldResume: tc.allowed})
			if p.Playing() != tc.wantResumed || p.position != 37*time.Second {
				t.Fatalf("playing = %t at %v, want %t at 37s", p.Playing(), p.position, tc.wantResumed)
			}
			if p.Playing() {
				p.TogglePlay()
			}
			transport.handle(playbackCommand{Action: "interruptionEnded", ShouldResume: true})
			if p.Playing() {
				t.Fatal("a duplicate end notification restarted playback")
			}
		})
	}
}

// An unplug notification pauses the output even when an app pause is already fading, and never arms a resume.
func TestRouteDisconnectInterruptsPausedOutput(t *testing.T) {
	p := &testTransportPlayer{track: &netease.Track{ID: 7}, position: 37 * time.Second}
	tpt := remoteTransport{player: p}
	p.changed = tpt.playbackChanged
	tpt.handle(playbackCommand{Action: "routeDisconnected"})
	tpt.handle(playbackCommand{Action: "interruptionBegan"})
	tpt.handle(playbackCommand{Action: "interruptionEnded", ShouldResume: true})
	if p.Playing() || p.interrupted != 2 || p.position != 37*time.Second {
		t.Fatalf("route change left playing=%v, interruptions=%d, position=%v", p.Playing(), p.interrupted, p.position)
	}
}
