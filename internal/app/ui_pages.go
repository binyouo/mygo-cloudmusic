package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func (a *app) pagePadding() float32 {
	if a.phone {
		return 20
	}
	return 40
}

func (a *app) contentWidth(c *ui.Context) float32 {
	w, _ := c.Size()
	if !a.phone {
		w -= sidebarWidth
	}
	return w
}

// detailHeader stacks the artwork over its metadata on phones.
func (a *app) detailHeader(c *ui.Context) ui.Element {
	if a.phone {
		return ui.Column(c).Gap(16).AlignItems(ui.Stretch)
	}
	return ui.Row(c).Gap(28).AlignItems(ui.End)
}

// columns returns how many cards of at least min DIPs fit across the page.
func (a *app) columns(c *ui.Context, min float32) int {
	inner := a.contentWidth(c) - 2*a.pagePadding()
	if a.phone {
		// A phone's grids keep two across at least, as NetEase's do.
		return max(2, int((inner+16)/(min+16)))
	}
	return max(2, int((inner+20)/(min+20)))
}

func (a *app) heading(c *ui.Context, title, sub string) {
	if a.phone {
		// Apple's large title, its descriptor over it.
		ui.Column(c).Gap(2).Margin(0, 0, 14, 0).Children(func() {
			if sub != "" {
				ui.Text(c, sub).FontSize(13).FontWeight(600).TextColor(a.pal.muted)
			}
			ui.Text(c, title).FontSize(34).FontWeight(800).LetterSpacing(-0.6)
		})
		return
	}
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(a.pal.muted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

// largeTitle is a phone page's heading where it has no descriptor.
func (a *app) largeTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(34).FontWeight(800).LetterSpacing(-0.6).Margin(0, 0, 14, 0)
}

func (a *app) section(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
}

func (a *app) homePage(c *ui.Context) {
	if a.phone {
		a.phoneHome(c)
		return
	}
	loading := len(a.featured) == 0 && len(a.charts) == 0
	if loading && !a.homeLoading {
		a.centered(c, a.homeErr, a.loadHome)
		return
	}
	now := c.Now()
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("home")).TrackScroll(a.ps()).Grow(1).Padding(top+pageGap, a.pagePadding(), bottom+48, a.pagePadding()).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		hello := greeting(now)
		if a.profile != nil {
			hello += "，" + a.profile.Nickname
		}
		a.heading(c, hello, fmt.Sprintf("%d月%d日 %s", now.Month(), now.Day(), weekday(now)))
		if loading {
			a.homeSkeleton(c, a.pageShimmer(c))
			return
		}
		a.dailyCard(c, now)

		if len(a.recPlaylists) > 0 {
			a.section(c, "今日为你推荐")
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range a.recPlaylists[:min(len(a.recPlaylists), a.columns(c, 168)*2)] {
					a.playlistCard(c.Key(a.recPlaylists[i].ID), &a.recPlaylists[i])
				}
			})
		}

		a.section(c, "官方榜单")
		official := a.charts[:min(4, len(a.charts))]
		cols := min(4, a.columns(c, 260))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for i := range official {
				a.chartCard(c.Key(official[i].ID), &official[i])
			}
		})

		a.section(c, "推荐歌单")
		ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
			for i := range a.featured {
				a.playlistCard(c.Key(a.featured[i].ID), &a.featured[i])
			}
		})
	})
}

func weekday(t time.Time) string {
	return [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[t.Weekday()]
}

func (a *app) chartsPage(c *ui.Context) {
	loading := len(a.charts) == 0
	if loading && !a.homeLoading {
		a.centered(c, a.homeErr, a.loadHome)
		return
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("charts")).TrackScroll(a.ps()).Grow(1).Padding(top+pageGap, a.pagePadding(), bottom+48, a.pagePadding()).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, "排行榜", "每日更新")
		if loading {
			s := a.pageShimmer(c)
			a.chartCardsSkeleton(c, s)
			a.section(c, "全部榜单")
			a.playlistCardsSkeleton(c, s, a.columns(c, 150)*2, 150, true)
			return
		}
		official := a.charts[:min(4, len(a.charts))]
		cols := min(4, a.columns(c, 260))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for i := range official {
				a.chartCard(c.Key(official[i].ID), &official[i])
			}
		})
		if len(a.charts) > 4 {
			a.section(c, "全部榜单")
			rest := a.charts[4:]
			ui.Grid(c).Columns(a.columns(c, 150)).GapX(20).GapY(28).Children(func() {
				for i := range rest {
					a.playlistCard(c.Key(rest[i].ID), &rest[i])
				}
			})
		}
	})
}

// playlistCard is a square cover with the playlist's name below: it lifts
// under the pointer, where a play button shows on it.
func (a *app) playlistCard(c *ui.Context, pl *netease.Playlist) {
	if a.coverCard(c, pl.Name, pl.Cover(), pl.PlayCount, pl.UpdateFreq, func() { a.loadPlaylist(pl.ID, true) }) {
		a.open(route{page: pagePlaylist, id: pl.ID})
	}
}

// coverCard is a playlist's card, or a podcast's: its cover, with how many
// times it was played on it, over its name and a line under that. The
// button on the cover calls play; it reports whether the rest was clicked.
func (a *app) coverCard(c *ui.Context, name, cover string, playCount float64, sub string, play func()) bool {
	p := a.pal
	// From the top: a grid stretches the cards of a row as tall as the
	// tallest, whose name or line under it takes two lines.
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(name)
	hovered := card.Hovered()
	if a.phone {
		phonePressScale(c, card, 0.97)
	}
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() {
			a.coverFill(c, cover, 400, 10)
			if playCount > 0 {
				ui.Row(c).Absolute().Top(8).Right(8).Padding(2, 7).Radius(999).Gap(3).AlignItems(ui.Center).
					Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(ui.RGB(255, 255, 255)).Children(func() {
					ui.Icon(c, iconHeadset).Size(10, 10)
					ui.Text(c, plays(playCount)).FontSize(10.5).FontWeight(600)
				})
			}
			if !a.phone {
				a.coverPlayButton(c, hovered, play)
			}
		})
		ui.Text(c, name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if sub != "" {
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	return card.Clicked()
}

// coverFill fills its parent with a cover, its corners rounded by radius.
// The cover rounds itself rather than its parent clipping it: a clip moved
// by a transform, as a card lifting under the pointer, is drawn on the CPU.
func (a *app) coverFill(c *ui.Context, url string, px int, radius float32) {
	bmp := bitmap(a.art.get(netease.Sized(url, px), false))
	// PassThrough: a cover paints, it never takes a tap from a sibling.
	box := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).PassThrough()
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(a.pal.hover)
	}
	box.Children(func() { artImage(c, bmp, radius, in) })
}

// coverPlayButton plays a playlist or a podcast from its card, without
// opening it.
func (a *app) coverPlayButton(c *ui.Context, shown bool, play func()) {
	b := ui.ButtonBase(c.Key("play")).Absolute().Bottom(10).Right(10).Size(38, 38).Radius(999).Center().
		Background(a.pal.accent).TextColor(ui.RGB(255, 255, 255)).Label("播放").FocusRing(false).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.3))
	pose := ui.Pose{Fade: 1, Scale: 0.8, Y: 6}
	if shown || b.Hovered() {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			pose.Scale = 1.08
		}
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconPlay).Size(15, 15).Margin(0, 0, 0, 2) })
	if b.Clicked() {
		play()
	}
}

// chartCard shows a chart with its top three songs beside its cover.
func (a *app) chartCard(c *ui.Context, pl *netease.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c).Gap(14).Padding(12).Radius(14).AlignItems(ui.Center).FocusRing(false).Label(pl.Name)
	hovered := card.Hovered()
	if a.phone {
		phonePressScale(c, card, 0.97)
	}
	bg := p.hover
	if hovered {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond}))
	card.Children(func() {
		ui.Box(c).Size(84, 84).Radius(9).Shadow(0, 3, 10, -2, p.cardShade).Children(func() {
			a.coverFill(c, pl.Cover(), 200, 9)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, pl.Name).FontSize(14).FontWeight(700).SingleLine()
				ui.Text(c, pl.UpdateFreq).FontSize(11).TextColor(p.faint).SingleLine()
			})
			for i, line := range pl.Preview {
				ui.Row(c).Gap(7).Children(func() {
					ui.Textf(c, "%d", i+1).FontSize(12).FontWeight(700).Width(10).FontFeatures("tnum").TextColor(p.muted)
					ui.RichText(c,
						ui.Span{Text: line.First, Weight: 500},
						ui.Span{Text: "  " + line.Second, Color: p.muted},
					).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, id: pl.ID})
	}
}

func (a *app) playlistPage(c *ui.Context) {
	id := a.route.id
	pl := a.playlists[id]
	if pl == nil {
		if !a.loadingPl[id] {
			a.centered(c, a.plErr[id], func() { a.loadPlaylist(id, false) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, true, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	a.morePlaylistSongs(a.remainingPlaylist(pl))
	var indices []int
	if a.phonePreviewScroll == nil {
		indices = a.playlistFilter.rows(pl)
	}
	showHeader := !a.phonePlaylistSearch || a.phonePreviewScroll != nil
	a.trackRowsIndexed(c, pl.Tracks, indices, true, func(c *ui.Context) {
		if showHeader {
			a.playlistHeader(c, pl)
		}
	}, func(c *ui.Context) {
		if indices != nil && len(indices) == 0 && pl.TrackOffset >= len(pl.TrackIDs) {
			a.empty(c, "没有找到匹配的歌曲")
		}
		a.playlistProgress(c, pl)
	})
}

func (a *app) playlistHeader(c *ui.Context, pl *netease.Playlist) {
	if a.phone {
		a.playlistPhoneHeader(c, pl)
		return
	}
	p := a.pal
	a.detailHeader(c).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, pl.Cover(), 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			kind := "歌单"
			if pl.UpdateFreq != "" {
				kind = "榜单 · " + pl.UpdateFreq
			}
			ui.Text(c, kind).FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, pl.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if pl.Creator.Nickname != "" {
				meta = append(meta, pl.Creator.Nickname)
			}
			meta = append(meta, fmt.Sprintf("%d 首", max(len(pl.Tracks), len(pl.TrackIDs))))
			if pl.PlayCount > 0 {
				meta = append(meta, plays(pl.PlayCount)+" 次播放")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			if d := strings.TrimSpace(pl.Description); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			actions := ui.Row(c).Gap(10).Margin(10, 0, 0, 0)
			if a.phone {
				actions.Wrap()
			}
			actions.Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					a.playPlaylist(pl, 0, false)
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					a.playPlaylist(pl, 0, true)
				}
				if a.savable(pl) {
					icon, label := iconPlus, "收藏"
					if a.isSaved(pl) {
						icon, label = iconCheck, "已收藏"
					}
					if pillButton(c.Key("save"), icon, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.toggleSaved(*pl)
					}
				}
				if mine := a.editable(pl.ID); mine != nil {
					if a.phone {
						a.phonePlaylistActions(c.Key("more"), mine)
					} else {
						iconButton(c.Key("more"), iconMore, 18, p.muted, "更多").Menu(a.playlistManagementItems(mine, true))
					}
				}
				switch {
				case pl.SpecialType != 5:
				case a.pl.Radio() == heartRadio:
					if pillButton(c.Key("heart"), iconX, "退出心动模式", p.hover.Over(p.bg), p.text).Clicked() {
						a.exitRadio()
					}
				default:
					label := "心动模式"
					if a.startingRadio() == heartRadio {
						label = "取消开启心动模式"
					}
					if pillButton(c.Key("heart"), iconHeartFill, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.startHeartMode()
					}
				}
				if !a.phone {
					ui.Box(c).Grow(1)
					a.playlistSearch(c)
				}
			})
		})
	})
}

// trackRows lists tracks under a header, which scrolls with them; a click
// plays the list from the track clicked.
func (a *app) trackRows(c *ui.Context, tracks []netease.Track, covers bool, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	a.trackRowsIndexed(c, tracks, nil, covers, header, footer...)
}

func (a *app) trackRowsIndexed(c *ui.Context, tracks []netease.Track, indices []int, covers bool, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	p := a.pal
	cur := a.pl.Current()
	// A song's menu can take it out of the playlist it is listed in.
	var pid int64
	if a.route.page == pagePlaylist {
		pid = a.route.id
	}
	// An album's page has no column of the album its songs are on.
	wide := !a.phone && a.contentWidth(c) > 820 && a.route.page != pageAlbum
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	count := len(tracks)
	if indices != nil {
		count = len(indices)
	}
	rows := count + 1 + len(footer)
	list := ui.List(c.Key(a.route), a.ls(), rows, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				// The list's padding is the rows': headers line their text
				// up with what a row's own padding leaves.
				if a.phone {
					ui.Column(c).Padding(0, 12).Children(func() { header(c) })
				} else {
					header(c)
				}
			})
			return
		}
		if i == rows-1 {
			defer a.endMark(c)
		}
		if i > count {
			footer[i-count-1](c)
			return
		}
		i--
		if indices != nil {
			i = indices[i]
		}
		tr := &tracks[i]
		playing := cur != nil && cur.ID == tr.ID
		row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
		textAt := float32(12 + 26 + 14)
		if a.phone {
			row.Height(60).Radius(0)
			// Phone rows carry the song's cover where the list's songs
			// differ in albums, and number them where they do not.
			textAt = 12 + 32 + 14
			if covers {
				textAt = 12 + 44 + 14
			}
		}
		if !a.phone {
			row.ContextMenu(a.songItems(tr, pid))
		}
		hovered := row.Hovered()
		bg := ui.Transparent
		switch {
		case a.phone && row.Pressed():
			bg = p.press
		case playing && !a.phone:
			bg = p.accent.Alpha(0.09)
		case hovered:
			bg = p.hover
		}
		if !a.phone {
			bg = row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond})
		}
		row.Background(bg)
		playable := tr.Playable()
		if !playable {
			row.Opacity(0.42)
		}
		row.Children(func() {
			if a.phone {
				ui.Box(c).Absolute().Left(textAt).Right(-8).Bottom(0).Height(1).Background(p.hairline)
				if covers {
					cov := a.cover(c, tr.Album.PicURL, 120, 44, 8).Shrink(0)
					if playing {
						cov.Children(func() {
							ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Radius(8).
								Background(ui.RGBA(0, 0, 0, 0.45)).PassThrough().Children(func() {
								a.nowPlayingBars(c)
							})
						})
					}
				} else {
					ui.Box(c).Width(32).Shrink(0).Center().Children(func() {
						if playing {
							ui.Icon(c, iconVolume).Size(16, 16).TextColor(p.accent)
						} else {
							ui.Textf(c, "%d", i+1).FontSize(15).FontFeatures("tnum").TextColor(p.muted)
						}
					})
				}
			} else if covers {
				a.cover(c, tr.Album.PicURL, 96, 38, 6)
			} else {
				ui.Box(c).Width(26).Center().Children(func() {
					switch {
					case playing:
						a.nowPlayingBars(c)
					case hovered:
						ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.text)
					default:
						ui.Textf(c, "%d", i+1).FontSize(13).FontFeatures("tnum").TextColor(p.faint)
					}
				})
			}
			ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
				name := p.text
				if playing {
					name = p.accent
				}
				spans := []ui.Span{{Text: tr.Name, Weight: 500, Color: name}}
				if len(tr.Alias) > 0 {
					spans = append(spans, ui.Span{Text: "（" + tr.Alias[0] + "）", Color: p.faint})
				}
				nameSize, artistSize := float32(14), float32(12.5)
				if a.phone {
					nameSize, artistSize = 16, 12
				}
				ui.RichText(c, spans...).FontSize(nameSize).SingleLine()
				// A second line only exists when it has something to say:
				// an empty one would push the title off the row's middle.
				artists := tr.ArtistNames()
				vip := !playable && (tr.Fee == 1 || tr.Fee == 4)
				sub := artists != "" || tr.Reason != "" || vip || a.route.page != pageAlbum && tr.Album.Name != ""
				if !a.phone || sub {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						if vip {
							ui.Text(c, "VIP").FontSize(9).FontWeight(800).TextColor(p.accent).Padding(0, 4).Radius(3).Border(1, p.accent.Alpha(0.6))
						}
						if a.phone {
							// "artist - album", the album left off its own page.
							text := artists
							if a.route.page != pageAlbum && tr.Album.Name != "" {
								if text != "" {
									text += " - "
								}
								text += tr.Album.Name
							}
							ui.Text(c, text).FontSize(artistSize).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
						} else {
							a.artistLinks(c, tr, p.muted, p.text).FontSize(artistSize).SingleLine().Shrink(1).MinWidth(0)
						}
						if tr.Reason != "" {
							ui.Text(c, tr.Reason).FontSize(10.5).FontWeight(600).TextColor(p.accent).Padding(1, 6).Radius(999).Background(p.accent.Alpha(0.1)).SingleLine()
						}
					})
				}
			})
			if wide {
				album := ui.RichText(c).Grow(2).Basis(0).MinWidth(0).FontSize(12.5).TextColor(p.muted).SingleLine()
				album.Children(func() {
					if tr.Album.ID == 0 {
						ui.Text(c, tr.Album.Name)
					} else {
						a.link(c.Key("album"), tr.Album.Name, p.muted, p.text, route{page: pageAlbum, id: tr.Album.ID})
					}
				})
			}
			// A liked song's heart shows; on the desktop the others' as the
			// pointer is on them, on the phone only when already liked.
			ui.Box(c).Size(32, 32).Center().Children(func() {
				if tr.LocalPath == "" && (a.liked[tr.ID] || (!a.phone && hovered)) {
					a.likeButton(c.Key("like"), tr, 15, p.muted)
				}
			})
			if a.phone {
				a.phoneTrackActions(c.Key("song-actions"), tr, pid, p.muted)
			} else {
				ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
			}
		})
		if row.Clicked() {
			if pid != 0 {
				a.phonePlaylistEditing = false
				a.playPlaylist(a.playlists[pid], tr.ID, a.pl.Shuffle())
			} else {
				a.pl.PlayQueue(tracks, i)
			}
		}
	}).Grow(1).Padding(top+pageGap, a.pagePadding()-12, bottom+32, a.pagePadding()-12).ScrollbarInsets(top, 0, bottom, 0)
	if a.phone {
		list.TrackScroll(a.ps())
		if a.route.page == pagePlaylist || a.route.page == pageAlbum {
			list.ScrollbarStyle(ui.ScrollbarStyle{TrackWidth: 18, ThumbWidth: 3})
		}
	}
}

// nowPlayingBars moved to playing_bars.go, which keeps Apple's patterns.
