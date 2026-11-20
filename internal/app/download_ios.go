package app

import (
	"github.com/egoist/mygo"

	"netease-music-mygo/internal/player"
)

func init() {
	downloadFile = exportSongToFiles
}

// exportSongToFiles downloads into the sandbox, then lets Files export a copy.
func exportSongToFiles(a *app, song *player.Song, name string) (bool, error) {
	temp, err := mygo.App.Path(mygo.PathTemp)
	if err != nil {
		return false, err
	}
	a.post(func() { a.toast = "正在下载…" })
	return saveSongCopy(temp, name, song, mygo.Dialog.SaveCopy)
}
