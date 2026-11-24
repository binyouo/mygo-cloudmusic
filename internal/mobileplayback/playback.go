package mobileplayback

type coverImage struct{ data []byte }

type playback struct {
	TrackID  string
	Title    string
	Artist   string
	Album    string
	Position float64
	Duration float64
	Playing  bool
	Loading  bool
	cover    *coverImage
}

type playbackUpdate struct {
	playback
	ArtworkChanged bool   `json:",omitempty"`
	Artwork        []byte `json:",omitempty"`
}

// updatePlayback sends a cover only when it differs from the last successful
// update. Clearing the cover is an explicit change with no image bytes.
func updatePlayback(state *playback, lastCover *coverImage) *playbackUpdate {
	if state == nil {
		return nil
	}
	update := &playbackUpdate{playback: *state, ArtworkChanged: state.cover != lastCover}
	if update.ArtworkChanged && state.cover != nil {
		update.Artwork = state.cover.data
	}
	return update
}
