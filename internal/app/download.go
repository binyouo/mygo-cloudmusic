package app

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
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

// downloadFile saves on desktops after the listener picks a destination.
var downloadFile = func(a *app, song *player.Song, name string) (bool, error) {
	path := saveSongAs(a, name)
	if path == "" {
		return false, nil
	}
	a.post(func() { a.toast = "正在下载…" })
	return true, song.SaveTo(path)
}

// canDownload reports whether tr can be saved to a file of the listener's: a
// song its source streams for the app to play, rather than one already on disk
// or one that plays elsewhere.
func (a *app) canDownload(tr *netease.Track) bool {
	return tr.LocalPath == "" && (tr.ProgramID == 0 || tr.ID != 0)
}

func (a *app) phoneDownloading() bool {
	select {
	case <-a.phoneDownload:
		return false
	default:
		return a.phoneDownload != nil
	}
}

// download saves a song where the listener picks without changing playback.
func (a *app) download(tr netease.Track) {
	var done chan struct{}
	if a.phone {
		if a.phoneDownloading() {
			return
		}
		done = make(chan struct{})
		a.phoneDownload = done
		a.toast = "正在准备下载…"
	}
	quality := a.settings.Quality
	find, save := a.pl.FindSong, downloadFile
	go func() {
		song, err := find(tr, quality)
		var message string
		if err != nil {
			message = "无法下载：" + err.Error()
		} else {
			saved, err := save(a, song, songFileName(&tr, song.Type))
			song.Close()
			switch {
			case err != nil:
				message = "下载失败：" + err.Error()
			case saved:
				message = "已下载：" + tr.Name
			}
		}
		if done != nil {
			close(done)
		}
		a.post(func() {
			if message != "" {
				a.toast = message
			}
		})
	}()
}

// saveSongCopy keeps the staged audio readable until Files returns; neither
// canceling nor finishing the export leaves the app's staging copy behind.
func saveSongCopy(temp, name string, song *player.Song, saveCopy func(path, name string) (string, error)) (bool, error) {
	dir, err := os.MkdirTemp(temp, "cloudmusic-download-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, name)
	if err := song.SaveTo(path); err != nil {
		return false, err
	}
	saved, err := saveCopy(path, name)
	return saved != "", err
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
