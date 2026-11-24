package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"netease-music-mygo/internal/netease"
)

// mapPhoneTrackPaths converts owned files at the storage boundary. The queue
// and the library keep absolute paths while they are in use.
func mapPhoneTrackPaths(tracks []netease.Track, root string, pathFor func(root, dir, name string) string) []netease.Track {
	original := root
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = canonical
	}
	mapName := func(dir, name string) string {
		if mapped := pathFor(root, dir, name); mapped != name || root == original {
			return mapped
		}
		return pathFor(original, dir, name)
	}
	tracks = slices.Clone(tracks)
	for i := range tracks {
		track := &tracks[i]
		track.LocalPath = mapName("local-media", track.LocalPath)
		track.LocalLyricsPath = mapName("local-media", track.LocalLyricsPath)
		picture, err := url.Parse(track.Album.PicURL)
		if err != nil || picture.Host != "" || (picture.Scheme != "file" && picture.Scheme != "") {
			continue
		}
		name := filepath.FromSlash(picture.Path)
		if mapped := mapName("local-artwork", name); mapped != name {
			u := url.URL{Path: filepath.ToSlash(mapped)}
			if filepath.IsAbs(mapped) {
				u.Scheme = "file"
			}
			track.Album.PicURL = u.String()
		}
	}
	return tracks
}

func relativePhonePath(root, dir, name string) string {
	if rel, err := filepath.Rel(filepath.Join(root, dir), name); err == nil && rel != "." && filepath.IsLocal(rel) {
		return filepath.Join(dir, rel)
	}
	return name
}

func resolvePhonePath(root, dir, name string) string {
	if filepath.IsLocal(name) && strings.HasPrefix(filepath.Clean(name), dir+string(filepath.Separator)) {
		return filepath.Join(root, name)
	}
	if !filepath.IsAbs(name) {
		return name
	}
	// Versions that wrote absolute paths placed owned files under the same
	// directory names. Rebase those paths when their copy is present here.
	old := filepath.Dir(name)
	if dir == "local-media" {
		old = filepath.Dir(old) // local-media/<digest>/<name>
	}
	if filepath.Base(old) == dir && filepath.Base(filepath.Dir(old)) == filepath.Base(root) {
		if rel, err := filepath.Rel(filepath.Dir(old), name); err == nil {
			candidate := filepath.Join(root, rel)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate
			}
		}
	}
	return name
}

// keepPhoneImport copies a document-picker file into the app's library.
// The picker's copy is temporary; the library needs a path it can reopen.
func keepPhoneImport(root, source string) (path string, created bool, err error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", false, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", false, err
	}
	in, err := os.Open(source)
	if err != nil {
		return "", false, err
	}
	defer in.Close()

	out, err := os.CreateTemp(root, ".import-*")
	if err != nil {
		return "", false, err
	}
	defer os.Remove(out.Name())
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(out, hash), in)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return "", false, err
	}
	dir := filepath.Join(root, hex.EncodeToString(hash.Sum(nil)))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", false, err
	}
	path = filepath.Join(dir, filepath.Base(source))
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if err := os.Rename(out.Name(), path); err != nil {
		return "", false, err
	}
	return path, true, nil
}
