package mobileplayback

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestPlaybackArtworkUpdate replaces or clears a track cover without resending it on seeks.
func TestPlaybackArtworkUpdate(t *testing.T) {
	cover := &coverImage{data: []byte{1, 2, 3}}
	a := playback{Title: "A", cover: cover}
	b := playback{Title: "B", Position: 2}

	for _, tc := range []struct {
		name    string
		state   *playback
		last    *coverImage
		changed bool
		image   []byte
	}{
		{"cover arrived", &a, nil, true, cover.data},
		{"seek on same cover", &a, cover, false, nil},
		{"next track still loading", &b, cover, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(updatePlayback(tc.state, tc.last))
			if err != nil {
				t.Fatal(err)
			}
			var wire struct {
				Title          string
				Position       float64
				ArtworkChanged bool
				Artwork        []byte
			}
			if err := json.Unmarshal(got, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Title != tc.state.Title || wire.Position != tc.state.Position ||
				wire.ArtworkChanged != tc.changed || !bytes.Equal(wire.Artwork, tc.image) {
				t.Fatalf("unexpected playback update: %s", got)
			}
		})
	}
}
