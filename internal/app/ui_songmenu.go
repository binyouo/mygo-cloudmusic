package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// copyText puts text on the clipboard; tests replace it.
var copyText = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }

type trackSelection struct {
	track netease.Track
	from  int64 // source playlist, when opened from one
}

// songMenu is three dots that open the menu of tr.
func (a *app) songMenu(c *ui.Context, tr *netease.Track, size float32, color ui.Color) ui.Element {
	b := iconButton(c, iconMore, size, color, "更多")
	if a.phone {
		// On the phone the menu is the track actions sheet.
		b.OnClick(func() { a.phoneTrackTarget = &trackSelection{track: *tr} })
		return b
	}
	b.Menu(a.songItems(tr, 0))
	return b
}

func (a *app) phoneTrackActions(c *ui.Context, tr *netease.Track, pid int64, color ui.Color) {
	iconButton(c, iconMore, 18, color, trackActionsTitle(tr)+"："+tr.Name).Size(44, 44).OnClick(func() {
		a.phoneTrackTarget = &trackSelection{track: *tr, from: pid}
	})
}

func trackActionsTitle(tr *netease.Track) string {
	if tr.ProgramID != 0 {
		return "节目操作"
	}
	return "歌曲操作"
}

// songItems builds the menu of what can be done with tr, behind its dots
// or a right click on its row: copying its title, its title with its
// artists, or the link to its page, opening its artists' or album's, and,
// signed in, adding it to a playlist or taking it out of pid, the playlist
// it is listed in, if set.
func (a *app) songItems(tr *netease.Track, pid int64) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		a.songActions(tr, func(label string, disabled bool, run func()) {
			if m.Item(label).Disabled(disabled).Chosen() {
				run()
			}
		}, m.Separator)
		if tr.ProgramID == 0 && tr.LocalPath == "" {
			a.playlistItems(m, tr, pid)
		}
	}
}

// songActions supplies the track commands to both menus and the playback sheet.
func (a *app) songActions(tr *netease.Track, item func(string, bool, func()), separator func()) {
	if tr.ProgramID == 0 || tr.ID != 0 {
		item("下一首播放", false, func() {
			a.pl.PlayNext(*tr)
			a.toast = "已添加到下一首播放"
		})
		separator()
	}
	if a.canDownload(tr) {
		label, busy := "下载", a.phoneDownloading()
		if busy {
			label = "正在下载…"
		}
		item(label, busy, func() { a.download(*tr) })
	}
	if tr.ProgramID != 0 {
		if a.phone {
			label := "喜欢节目"
			if a.isLiked(tr) {
				label = "取消喜欢节目"
			}
			item(label, false, func() { a.toggleLike(tr) })
			separator()
		}
		item("复制节目名", false, func() { copyText(tr.Name) })
		item("复制链接", false, func() { copyText(tr.URL()) })
		separator()
		item("查看播客", false, func() { a.openPage(route{page: pagePodcast, id: tr.PodcastID}) })
		return
	}
	item("复制歌名", false, func() { copyText(tr.Name) })
	item("复制歌名和歌手", false, func() { copyText(tr.Name + " - " + tr.ArtistNames()) })
	if tr.LocalPath != "" {
		if a.phone {
			item("导入歌词…", a.localPicking || a.localImporting, func() { a.chooseLocalFiles(tr.ID) })
		}
		item("复制文件路径", false, func() { copyText(tr.LocalPath) })
		return
	}
	separator()
	item("复制链接", false, func() { copyText(tr.URL()) })
	linked := tr.Album.ID != 0
	for _, ar := range tr.Artists {
		linked = linked || ar.ID != 0
	}
	if linked {
		separator()
	}
	for _, ar := range tr.Artists {
		if ar.ID != 0 {
			item("查看歌手："+ar.Name, false, func() { a.openArtist(ar.ID) })
		}
	}
	if tr.Album.ID != 0 {
		item("查看专辑："+tr.Album.Name, false, func() { a.openPage(route{page: pageAlbum, id: tr.Album.ID}) })
	}
}
