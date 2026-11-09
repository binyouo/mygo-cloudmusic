package app

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

// saveSongAs asks the listener where to save a song with the native dialog,
// suggesting name in their Downloads folder, and returns the path, or "" if
// they didn't pick one. It blocks, so it runs on a goroutine; tests replace it.
var saveSongAs = func(a *app, name string) string {
	dir, _ := mygo.App.Path(mygo.PathDownloads)
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:            a.win,
		Title:             "下载",
		DefaultPath:       filepath.Join(dir, name),
		ButtonLabel:       "下载",
		CreateDirectories: true,
	})
	if err != nil {
		return ""
	}
	return path
}

// canDownload reports whether tr can be saved to a file of the listener's: a
// song its source streams for the app to play, rather than one already on disk
// or one that plays elsewhere.
func (a *app) canDownload(tr *netease.Track) bool {
	return tr.LocalPath == ""
}

// download saves a song where the listener picks without changing playback.
func (a *app) download(tr netease.Track) {
	quality := a.settings.Quality
	find, choose := a.pl.FindSong, saveSongAs
	go func() {
		song, err := find(tr, quality)
		if err != nil {
			a.post(func() { a.toast = "无法下载：" + err.Error() })
			return
		}
		defer song.Close()
		path := choose(a, songFileName(&tr, song.Type))
		if path == "" {
			return
		}
		a.post(func() { a.toast = "正在下载…" })
		err = song.SaveTo(path)
		a.post(func() {
			if err != nil {
				a.toast = "下载失败：" + err.Error()
			} else {
				a.toast = "已下载：" + tr.Name
			}
		})
	}()
}

// songFileName names the file of a song of type ext as its credits read,
// "Artists - Title.ext", with what no file name may hold taken out.
func songFileName(tr *netease.Track, ext string) string {
	name := tr.Name
	if artists := tr.ArtistNames(); artists != "" {
		name = artists + " - " + name
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	// Most systems take 255 bytes, with room left for the extension.
	for len(name) > 200 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	// Windows drops a trailing dot or space, and a leading dot hides the file.
	name = strings.Trim(name, ". ")
	if name == "" {
		name = "歌曲"
	}
	return name + "." + ext
}
