package localmedia

import (
	"slices"
	"strings"

	"netease-music-mygo/internal/netease"
)

// WithLocalMetadata supplies a cloud song's display metadata from one matching
// local import. Never replace its identity or audio source with the local file.
func WithLocalMetadata(track netease.Track, library []netease.Track) netease.Track {
	var match *netease.Track
	for i := range library {
		local := &library[i]
		if local.LocalPath == "" || !strings.EqualFold(strings.TrimSpace(track.Name), strings.TrimSpace(local.Name)) {
			continue
		}
		artists := artistNames(track)
		if len(artists) == 0 || !slices.Equal(artists, artistNames(*local)) {
			continue
		}
		// Different edits of a song can share a title and artist.
		if track.Duration > 0 && local.Duration > 0 && (track.Duration-local.Duration > 2000 || local.Duration-track.Duration > 2000) {
			continue
		}
		if match != nil {
			return track
		}
		match = local
	}
	if match != nil {
		if match.Album.PicURL != "" {
			track.Album.PicURL = match.Album.PicURL
		}
		if match.LocalLyrics != "" || match.LocalLyricsPath != "" {
			track.LocalLyrics, track.LocalLyricsPath = match.LocalLyrics, match.LocalLyricsPath
		}
	}
	return track
}

func artistNames(track netease.Track) []string {
	var names []string
	for _, artist := range track.Artists {
		for _, name := range strings.FieldsFunc(artist.Name, func(r rune) bool { return r == '/' || r == ',' || r == '、' }) {
			if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}
