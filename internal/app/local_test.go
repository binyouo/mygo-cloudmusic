package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Reimporting a file refreshes its metadata without duplicating it or replacing another track.
func TestMergeLocalTracks(t *testing.T) {
	existing := []netease.Track{{ID: -1, Name: "旧标题"}, {ID: -2, Name: "另一首"}}
	merged := mergeLocalTracks(existing, []netease.Track{{ID: -1, Name: "新标题", LocalLyrics: "[00:01]歌词"}, {ID: -3, Name: "新歌"}, {ID: -3, Name: "新歌更新"}})
	if len(merged) != 3 || merged[0].Name != "新标题" || merged[0].LocalLyrics == "" || merged[1].ID != -2 || merged[2].Name != "新歌更新" {
		t.Fatalf("unexpected catalog: %+v", merged)
	}
	if existing[0].Name != "旧标题" {
		t.Fatal("merge changed the old snapshot")
	}
}

// The local catalog preserves artwork, file paths and lyrics across a restart.
func TestSaveLocalLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library", "local-library.json")
	track := netease.Track{ID: -7, LocalPath: "/音乐/歌曲.mp3", LocalLyrics: "[00:01]歌词", LocalLyricsPath: "/歌词/歌曲.lrc", Album: netease.Album{PicURL: "file:///音乐/封面.cover"}}
	if err := saveLocalLibrary(path, []netease.Track{track}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var restored []netease.Track
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 || restored[0].LocalPath != track.LocalPath || restored[0].LocalLyrics != track.LocalLyrics || restored[0].LocalLyricsPath != track.LocalLyricsPath || restored[0].Album.PicURL != track.Album.PicURL {
		t.Fatalf("local metadata was lost: %+v", restored)
	}
}

// Dropping an LRC alone updates its matching song instead of silently skipping the file.
func TestImportStandaloneLRC(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "歌曲.LrC")
	text := "[00:03.500]单独导入的歌词"
	if err := os.WriteFile(source, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	track := netease.Track{ID: -1, LocalPath: filepath.Join(dir, "歌曲.mp3")}
	result := readLocalImports([]string{source}, []netease.Track{track}, "", filepath.Join(dir, "art"))
	if result.lyrics != 1 || result.audio != 0 || len(result.failures) != 0 || result.tracks[0].LocalLyrics != text || result.tracks[0].LocalLyricsPath == "" {
		t.Fatalf("standalone LRC was ignored: %+v", result)
	}
	a := newTestApp()
	a.pl.Cue([]netease.Track{track})
	a.pl.Hold(20 * time.Second)
	updated := result.tracks[0]
	a.pl.UpdateLocalLyrics(updated.LocalPath, updated.LocalLyricsPath, updated.LocalLyrics)
	lines, loaded := a.pl.Lyrics()
	if !loaded || len(lines) != 1 || lines[0].At != 3500*time.Millisecond || a.pl.Position() != 20*time.Second || !a.pl.Playing() {
		t.Fatalf("lyrics did not apply without interrupting playback: %+v", lines)
	}
}

// A separately named lyric can target the current local song, while ambiguous same-name songs are left untouched.
func TestImportLRCMatching(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "自选歌词.lrc")
	if err := os.WriteFile(source, []byte("[00:01]歌词"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracks := []netease.Track{{ID: -1, LocalPath: filepath.Join(dir, "a", "歌曲.mp3")}, {ID: -2, LocalPath: filepath.Join(dir, "b", "歌曲.flac")}}
	result := readLocalImports([]string{source}, tracks, tracks[1].LocalPath, filepath.Join(dir, "art"))
	if result.lyrics != 1 || result.tracks[0].LocalLyrics != "" || result.tracks[1].LocalLyrics == "" {
		t.Fatalf("current song did not receive explicit lyrics: %+v", result)
	}
	matchedSource := filepath.Join(dir, "歌曲.lrc")
	if err := os.Rename(source, matchedSource); err != nil {
		t.Fatal(err)
	}
	result = readLocalImports([]string{matchedSource}, tracks, tracks[1].LocalPath, filepath.Join(dir, "art"))
	if result.lyrics != 0 || len(result.failures) != 1 || result.tracks[0].LocalLyrics != "" || result.tracks[1].LocalLyrics != "" {
		t.Fatalf("ambiguous lyrics were attached to the wrong song: %+v", result)
	}
}

// Audio and LRC selected together are paired even when the lyric appears first in the selection.
func TestImportAudioAndLRCInEitherOrder(t *testing.T) {
	dir := t.TempDir()
	audio, err := os.ReadFile("../player/testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "歌曲.mp3")
	if err := os.WriteFile(path, audio, 0o600); err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSuffix(path, ".mp3") + ".lrc"
	if err := os.WriteFile(source, []byte("[00:02]同步歌词"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{{source, path}, {path, source}} {
		result := readLocalImports(paths, nil, "", filepath.Join(dir, "art"))
		if result.audio != 1 || result.lyrics != 1 || len(result.tracks) != 1 || len(result.failures) > 0 || result.tracks[0].LocalLyricsPath == "" {
			t.Fatalf("pairing depends on selection order: %+v", result)
		}
	}
}
