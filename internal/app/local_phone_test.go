package app

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"netease-music-mygo/internal/netease"
)

// TestPhoneImportKeepsPickerCopy checks that an imported file stays readable after the document picker discards its copy.
func TestPhoneImportKeepsPickerCopy(t *testing.T) {
	source := filepath.Join(t.TempDir(), "Example.lrc")
	want := "[00:01.00]Example"
	if err := os.WriteFile(source, []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path, created, err := keepPhoneImport(root, source)
	if err != nil || !created || filepath.Base(path) != filepath.Base(source) {
		t.Fatalf("keepPhoneImport = %q, %v, %v", path, created, err)
	}
	same, created, err := keepPhoneImport(root, source)
	if err != nil || created || same != path {
		t.Fatalf("importing again = %q, %v, %v", same, created, err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Fatalf("persisted import = %q, %v", got, err)
	}
}

// TestPhoneLibrarySurvivesContainerMove restores audio, lyrics and artwork after an iOS update moves the sandbox.
func TestPhoneLibrarySurvivesContainerMove(t *testing.T) {
	for _, tc := range []struct {
		name  string
		phone bool
	}{{"relative", true}, {"legacy absolute", false}} {
		t.Run(tc.name, func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			old := filepath.Join(parent, "before", "Application Support")
			moved := filepath.Join(parent, "after", "Application Support")
			audioPath := filepath.Join("local-media", "audio-digest", "南方 #1.flac")
			lyricsPath := filepath.Join("local-media", "lyrics-digest", "南方 #1.lrc")
			artPath := filepath.Join("local-artwork", "front #1.cover")
			coverURL := func(path string) string { return (&url.URL{Scheme: "file", Path: path}).String() }
			audioData, err := os.ReadFile("../player/testdata/seek.flac")
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{audioPath: audioData, lyricsPath: []byte("[00:01]歌词"), artPath: []byte(artPath)}
			for path, data := range files {
				name := filepath.Join(old, path)
				if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			tracks := []netease.Track{{ID: -123, LocalPath: filepath.Join(old, audioPath), LocalLyricsPath: filepath.Join(old, lyricsPath), Album: netease.Album{PicURL: coverURL(filepath.Join(old, artPath))}}}
			const library = "local-library.json"
			if err := saveLocalLibrary(filepath.Join(old, library), tracks, tc.phone); err != nil {
				t.Fatal(err)
			}
			if tracks[0].LocalPath != filepath.Join(old, audioPath) {
				t.Fatal("saving the library changed the live track path")
			}
			if tc.phone {
				saved, err := readLocalLibrary(filepath.Join(old, library), false)
				if err != nil || len(saved) != 1 || saved[0].LocalPath != audioPath || saved[0].LocalLyricsPath != lyricsPath {
					t.Fatalf("saved paths are not relative: %+v, %v", saved, err)
				}
			}
			if err := os.Rename(filepath.Dir(old), filepath.Dir(moved)); err != nil {
				t.Fatal(err)
			}
			restored, err := readLocalLibrary(filepath.Join(moved, library), true)
			if err != nil || len(restored) != 1 {
				t.Fatalf("read library after moving = %v, %v", restored, err)
			}
			track := restored[0]
			if track.ID != tracks[0].ID || track.LocalPath != filepath.Join(moved, audioPath) || track.LocalLyricsPath != filepath.Join(moved, lyricsPath) {
				t.Fatalf("restored track = %+v", track)
			}
			if art, err := netease.Fetch(track.Album.PicURL); err != nil || string(art) != artPath {
				t.Fatalf("restored artwork = %q, %v", art, err)
			}
			updated := readLocalImports([]string{track.LocalPath}, restored, filepath.Join(moved, "local-artwork"), localImportOptions{phone: true})
			if len(updated.failures) != 0 || len(updated.tracks) != 1 || updated.tracks[0].ID != track.ID || updated.tracks[0].LocalLyrics != "[00:01]歌词" {
				t.Fatalf("reimport after moving did not keep track identity and lyrics: %+v", updated)
			}
		})
	}
}

// With two same-named songs in different import folders, a picked LRC belongs to the selected track even while the other one plays.
func TestPhoneLyricsAttachToSelectedSong(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "local-media")
	tracks := []netease.Track{
		{ID: -1, LocalPath: filepath.Join(media, "audio-a", "夜航.mp3"), LocalLyrics: "[00:01]原歌词"},
		{ID: -2, LocalPath: filepath.Join(media, "audio-b", "夜航.flac")},
	}
	source := filepath.Join(t.TempDir(), "夜航.lrc")
	const text = "[00:05]新歌词"
	if err := os.WriteFile(source, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	kept, _, err := keepPhoneImport(media, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	options := localImportOptions{phone: true, lyricsFor: -2, currentPath: tracks[0].LocalPath}
	result := readLocalImports([]string{kept}, tracks, filepath.Join(root, "local-artwork"), options)
	if result.lyrics != 1 || len(result.failures) != 0 || result.updated[-1] || !result.updated[-2] {
		t.Fatalf("updated tracks = %v, failures = %v", result.updated, result.failures)
	}
	missing := readLocalImports([]string{kept}, tracks[:1], filepath.Join(root, "local-artwork"), options)
	if missing.lyrics != 0 || len(missing.failures) != 1 || missing.tracks[0].LocalLyrics != tracks[0].LocalLyrics {
		t.Fatalf("attached lyrics after the selected track disappeared: %+v", missing)
	}
	index := filepath.Join(root, "local-library.json")
	if err := saveLocalLibrary(index, result.tracks, true); err != nil {
		t.Fatal(err)
	}
	restored, err := readLocalLibrary(index, true)
	if err != nil || len(restored) != 2 || restored[0].LocalLyrics != tracks[0].LocalLyrics || restored[1].LocalLyrics != text || restored[1].LocalLyricsPath != kept {
		t.Fatalf("restored library = %+v, err = %v", restored, err)
	}
}

// Importing audio and a matching LRC together favors that batch and leaves ambiguous same-name selections alone.
func TestPhoneBatchImportPairsLyrics(t *testing.T) {
	flac, err := os.ReadFile("../player/testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	mp3, err := os.ReadFile("../player/testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, lyricFolder, secondFolder string
		wantLyrics                      int
	}{
		{name: "one selected song with matching filename", lyricFolder: "picker-copy", wantLyrics: 1},
		{name: "same folder within the selected batch", lyricFolder: "album-one", secondFolder: "album-two", wantLyrics: 1},
		{name: "two matches from unrelated folders", lyricFolder: "picker-copy", secondFolder: "album-two"},
		{name: "two exact matches in the same folder", lyricFolder: "album-one", secondFolder: "album-one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			media := filepath.Join(root, "local-media")
			pickedFrom := map[string]string{}
			stage := func(folder, name string, data []byte) string {
				t.Helper()
				dir := filepath.Join(root, folder)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				source := filepath.Join(dir, name)
				if err := os.WriteFile(source, data, 0o600); err != nil {
					t.Fatal(err)
				}
				path, _, err := keepPhoneImport(media, source)
				if err != nil {
					t.Fatal(err)
				}
				pickedFrom[path] = source
				return path
			}
			const text = "[00:05]这次导入的歌词"
			lyric := stage(tc.lyricFolder, "夜航.lrc", []byte(text))
			song := stage("album-one", "夜航.flac", flac)
			paths := []string{lyric, song}
			if tc.secondFolder != "" {
				paths = append(paths, stage(tc.secondFolder, "夜航.mp3", mp3))
			}
			old := netease.Track{ID: -12, LocalPath: filepath.Join(media, "previous", "夜航.mp3"), LocalLyrics: "[00:02]原歌词"}
			result := readLocalImports(paths, []netease.Track{old}, filepath.Join(root, "local-artwork"), localImportOptions{
				phone: true, currentPath: old.LocalPath, pickedFrom: pickedFrom,
			})
			if result.audio != len(paths)-1 || result.lyrics != tc.wantLyrics || len(result.failures) != 1-tc.wantLyrics {
				t.Fatalf("imported %d audio, %d lyrics, errors = %v", result.audio, result.lyrics, result.failures)
			}
			if result.tracks[0].LocalLyrics != old.LocalLyrics || result.updated[old.ID] {
				t.Fatal("the existing song's lyrics were changed")
			}
			for _, track := range result.tracks[1:] {
				if tc.wantLyrics == 1 && track.LocalPath == song {
					if track.LocalLyrics != text || track.LocalLyricsPath != lyric {
						t.Fatal("the matching song did not retain the staged lyrics")
					}
				} else if track.LocalLyricsPath != "" {
					t.Fatalf("batch attached lyrics to the wrong song: %s", track.LocalPath)
				}
			}
		})
	}
}
