package localmedia

import (
	"testing"

	"netease-music-mygo/internal/netease"
)

// Cloud uploads previously lost the local lyrics and cover even when the same song was imported.
func TestWithLocalMetadata(t *testing.T) {
	cloud := netease.Track{ID: 42, Name: "一满把", Duration: 152343,
		Artists: []netease.Artist{{Name: "LSGCsikoriot"}, {Name: "PSY.P"}},
		Album:   netease.Album{ID: 9, Name: "佛哦佛404", PicURL: "cloud.jpg"}}
	local := netease.Track{ID: -7, LocalPath: "/music/song.mp3", Name: cloud.Name, Duration: 152376,
		Artists:         []netease.Artist{{Name: "lsgcsikoriot/PSY.P"}},
		Album:           netease.Album{PicURL: "file:///cover.jpg"},
		LocalLyricsPath: "/music/song.lrc", LocalLyrics: "[00:01]歌词"}
	got := WithLocalMetadata(cloud, []netease.Track{local})
	if got.ID != cloud.ID || got.LocalPath != "" || got.Album.ID != cloud.Album.ID || got.Duration != cloud.Duration || got.Album.PicURL != local.Album.PicURL || got.LocalLyrics != local.LocalLyrics || got.LocalLyricsPath != local.LocalLyricsPath {
		t.Fatalf("display metadata or cloud identity changed incorrectly: %+v", got)
	}
	for _, test := range []struct {
		name   string
		change func(*netease.Track)
	}{
		{"other singer", func(t *netease.Track) { t.Artists = []netease.Artist{{Name: "another singer"}} }},
		{"other version", func(t *netease.Track) { t.Duration += 30000 }},
		{"other title", func(t *netease.Track) { t.Name += " (Live)" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			other := local
			test.change(&other)
			got := WithLocalMetadata(cloud, []netease.Track{other})
			if got.LocalLyrics != "" || got.Album.PicURL != cloud.Album.PicURL {
				t.Fatal("matched a different recording")
			}
		})
	}
	got = WithLocalMetadata(cloud, []netease.Track{local, local})
	if got.LocalLyrics != "" || got.Album.PicURL != cloud.Album.PicURL {
		t.Fatal("picked an ambiguous local import")
	}
}
