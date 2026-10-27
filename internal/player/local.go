package player

import (
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// SetLocalLibrary associates imported artwork and lyrics with matching cloud songs.
// Audio, song IDs and queue order stay with the original cloud tracks.
func (p *Player) SetLocalLibrary(tracks []netease.Track) {
	p.localLibrary = append([]netease.Track(nil), tracks...)
	if p.track != nil && p.track.LocalPath == "" {
		p.loadMetadata(*p.track, p.gen)
	}
}

func (p *Player) UpdateLocalLyrics(path, source, text string) {
	for index := range p.queue {
		if p.queue[index].LocalPath == path {
			p.queue[index].LocalLyricsPath, p.queue[index].LocalLyrics = source, text
		}
	}
	if p.track != nil && p.track.LocalPath == path {
		p.track.LocalLyricsPath, p.track.LocalLyrics = source, text
		p.lyrics, p.lyricsLoaded = lyrics.Build(text, "", ""), true
		if p.Changed != nil {
			p.Changed()
		}
	}
}
