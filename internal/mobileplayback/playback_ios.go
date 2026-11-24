// Package mobileplayback connects iOS media controls to the shared player.
package mobileplayback

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"netease-music-mygo/internal/player"
)

// Connect returns an update function for the main-thread player callback.
func Connect(p *player.Player, commandHandled func()) func() {
	native := mygo.Native("cloudmusic_playback")
	transport := remoteTransport{player: p}
	native.On("command", func(raw []byte) {
		var event playbackCommand
		if err := json.Unmarshal(raw, &event); err != nil {
			return
		}
		transport.handle(event)
		commandHandled()
	})

	pending := make(chan *playback, 1)
	go func() {
		var lastCover *coverImage
		for state := range pending {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			method := "update"
			if state == nil {
				method = "clear"
			}
			update := updatePlayback(state, lastCover)
			err := native.Call(ctx, method, update, nil)
			cancel()
			if err != nil {
				log.Printf("iOS media controls: %v", err)
			} else if state != nil {
				lastCover = state.cover
			} else {
				lastCover = nil
			}
		}
	}()

	var cover *coverImage
	update := func() {
		transport.playbackChanged()
		var state *playback
		if tr := p.Current(); tr != nil {
			state = &playback{
				TrackID: strconv.FormatInt(tr.ID, 10),
				Title:   tr.Name, Artist: tr.ArtistNames(), Album: tr.Album.Name,
				Position: p.Position().Seconds(), Duration: p.Duration().Seconds(),
				Playing: p.Playing() && p.Ready() && !p.Loading(),
				Loading: p.Playing() && p.Loading(),
				cover:   cover,
			}
		} else {
			cover = nil
		}
		// A seek or track change can publish several updates before the Swift
		// host answers. The host still sees the newest state in order.
		select {
		case <-pending:
		default:
		}
		pending <- state
	}
	p.ArtworkChanged = func(data []byte) {
		cover = nil
		if len(data) > 0 {
			cover = &coverImage{data: data}
		}
		update()
	}
	return update
}
