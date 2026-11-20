package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// phoneDetail is what a phone detail page's header shows.
type phoneDetail struct {
	cover, title, creator, creatorAvatar, meta, desc string
	playCount                                        float64
	openCreator                                      func() // nil = plain text
	play, shuffle                                    func()
	count                                            int
	actions                                          func() // action bar segments, may be nil
}

// phoneDetailHeader is a detail page's header on the phone, as the mobile
// app's: the cover blurred and darkened under the nav bar, a cover row
// over it, an action capsule, and the list's card overlapping its foot.
func (a *app) phoneDetailHeader(c *ui.Context, d phoneDetail) {
	p := a.pal
	top, _ := a.insets(c)
	pad := a.pagePadding()
	art := a.art.get(netease.Sized(d.cover, 400), true)
	head := ui.Box(c.Key("detail-head")).Margin(-(top + pageGap), -pad, 0, -pad).
		Clip().Background(ui.Hex("#343238")).TextColor(white)
	in := artIn(head, bitmap(art))
	head.Children(func() {
		artImage(c, backdrop(art), 0, in)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(ui.RGBA(0, 0, 0, 0.38))
		// Over the absolute backdrop, which paints after flow children.
		ui.Column(c).ZIndex(1).Padding(top+pageGap+12, pad, 26, pad).Gap(16).Children(func() {
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(120, 120).Radius(10).Shrink(0).Background(white.Alpha(0.12)).Children(func() {
					artImage(c, bitmap(art), 10, in)
					if d.playCount > 0 {
						ui.Row(c).Absolute().Top(6).Right(6).Padding(1, 6).Radius(999).Gap(3).AlignItems(ui.Center).
							Background(ui.RGBA(0, 0, 0, 0.35)).Children(func() {
							ui.Icon(c, iconPlay).Size(7, 7)
							ui.Text(c, plays(d.playCount)).FontSize(10).FontWeight(600).SingleLine()
						})
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(7).Children(func() {
					ui.Text(c, d.title).FontSize(18).FontWeight(700).MaxLines(2).LineHeight(1.3)
					if d.creator != "" {
						row := func() {
							if d.creatorAvatar != "" {
								a.avatar(c, d.creatorAvatar, 20, white.Alpha(0.2))
							}
							ui.Text(c, d.creator).FontSize(13).TextColor(white.Alpha(0.8)).SingleLine().Shrink(1).MinWidth(0)
							ui.Icon(c, iconForward).Size(11, 11).TextColor(white.Alpha(0.5))
						}
						if d.openCreator != nil {
							ui.ButtonBase(c.Key("creator")).AlignSelf(ui.Start).Gap(6).AlignItems(ui.Center).
								Label(d.creator).OnClick(d.openCreator).Children(row)
						} else {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Children(row)
						}
					}
					if d.desc != "" {
						ui.ButtonBase(c.Key("desc")).AlignSelf(ui.Start).Gap(4).AlignItems(ui.Center).Shrink(1).MinWidth(0).
							Label("简介").OnClick(func() { a.phoneDesc = d.desc }).Children(func() {
							ui.Text(c, d.desc).FontSize(12).TextColor(white.Alpha(0.6)).SingleLine().Shrink(1).MinWidth(0)
							ui.Icon(c, iconForward).Size(11, 11).TextColor(white.Alpha(0.5))
						})
					}
					if d.meta != "" {
						ui.Text(c, d.meta).FontSize(12).TextColor(white.Alpha(0.6)).SingleLine()
					}
				})
			})
			if d.actions != nil {
				ui.Box(c).Height(40).Children(func() {
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(20).Background(white.Alpha(0.16)).PassThrough()
					ui.Row(c).Fill().Children(d.actions)
				})
			}
		})
	})
	// The list's card is full-bleed and rides over the header's bottom.
	ui.Column(c).Margin(-16, -pad, 0, -pad).Radius(16, 16, 0, 0).Background(p.bg).Children(func() {
		ui.Row(c).Height(52).PaddingX(pad).AlignItems(ui.Center).Children(func() {
			row := ui.ButtonBase(c.Key("play-all")).Grow(1).MinWidth(0).Height(52).Gap(10).AlignItems(ui.Center).
				Justify(ui.Start).Label("播放").OnClick(d.play)
			row.Children(func() {
				ui.Box(c).Size(28, 28).Radius(14).Center().Background(p.accent).Children(func() {
					ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.onAccent)
				})
				ui.Text(c, "播放全部").FontSize(16).FontWeight(700).SingleLine()
				ui.Textf(c, "(%d)", d.count).FontSize(13).TextColor(p.muted).SingleLine()
			})
			iconButton(c.Key("shuffle"), iconShuffle, 18, p.muted, "随机播放").Size(36, 36).OnClick(d.shuffle)
		})
	})
}

// phoneDetailSegment is one equal share of a detail page's action
// capsule.
func (a *app) phoneDetailSegment(c *ui.Context, icon *ui.SVG, label string, run func()) ui.Element {
	return ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Height(40).Center().Gap(5).Label(label).TextColor(white).OnClick(run).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(15, 15)
		}
		ui.Text(c, label).FontSize(13).FontWeight(600).SingleLine()
	})
}

// phoneDetailDivider parts a detail action capsule's segments.
func phoneDetailDivider(c *ui.Context) {
	ui.Box(c).Width(1).Height(16).Shrink(0).Background(white.Alpha(0.18))
}

// playlistPhoneHeader is the playlist detail page's header on the phone.
func (a *app) playlistPhoneHeader(c *ui.Context, pl *netease.Playlist) {
	n := max(len(pl.Tracks), len(pl.TrackIDs))
	d := phoneDetail{
		cover:     pl.Cover(),
		title:     pl.Name,
		playCount: pl.PlayCount,
		count:     n,
		play:      func() { a.playPlaylist(pl, 0, false) },
		shuffle:   func() { a.playPlaylist(pl, 0, true) },
		desc:      strings.ReplaceAll(strings.TrimSpace(pl.Description), "\n", " "),
	}
	if pl.Creator.Nickname != "" {
		d.creator = pl.Creator.Nickname
		if pl.Creator.UserID != 0 {
			d.openCreator = func() { a.open(route{page: pageUser, id: pl.Creator.UserID}) }
		}
	}
	d.actions = func() {
		segments := 0
		segment := func(icon *ui.SVG, label string, run func()) {
			if segments > 0 {
				phoneDetailDivider(c)
			}
			a.phoneDetailSegment(c, icon, label, run)
			segments++
		}
		if a.savable(pl) {
			icon, text := iconPlus, "收藏"
			if a.isSaved(pl) {
				icon, text = iconCheck, "已收藏"
			}
			segment(icon, text, func() { a.toggleSaved(*pl) })
		}
		segment(iconShare, "分享", func() {
			copyText(fmt.Sprintf("https://music.163.com/playlist?id=%d", pl.ID))
			a.toast = "已复制链接"
		})
		if pl.SpecialType == 5 {
			icon, text, run := a.heartModeAction()
			segment(icon, text, run)
		}
		if mine := a.editable(pl.ID); mine != nil {
			if segments > 0 {
				phoneDetailDivider(c)
			}
			ui.Box(c).Grow(1).Basis(0).MinWidth(0).Center().Children(func() {
				a.phonePlaylistActions(c, mine)
			})
		}
	}
	a.phoneDetailHeader(c, d)
}

// albumPhoneHeader is the album detail page's header on the phone.
func (a *app) albumPhoneHeader(c *ui.Context, d *albumData) {
	al := d.album
	commentText := "评论"
	if d.comments.loaded {
		commentText += " " + count(d.comments.total)
	} else if d.activity != nil {
		commentText += " " + count(d.activity.CommentCount)
	}
	meta := ""
	if al.PublishTime > 0 {
		meta = time.UnixMilli(al.PublishTime).Format("2006")
	}
	meta += fmt.Sprintf(" · %d 首", len(al.Songs))
	meta = strings.TrimPrefix(meta, " · ")
	a.phoneDetailHeader(c, phoneDetail{
		cover:   al.PicURL,
		title:   al.Name,
		creator: al.ArtistNames(),
		openCreator: func() {
			if len(al.Artists) > 0 {
				a.open(route{page: pageArtist, id: al.Artists[0].ID})
			}
		},
		meta:  meta,
		desc:  strings.ReplaceAll(strings.TrimSpace(al.Description), "\n", " "),
		count: len(al.Songs),
		play: func() {
			if a.pl.Shuffle() {
				a.pl.ToggleShuffle()
			}
			a.pl.PlayQueue(al.Songs, 0)
		},
		shuffle: func() {
			if !a.pl.Shuffle() {
				a.pl.ToggleShuffle()
			}
			a.pl.PlayQueue(al.Songs, 0)
		},
		actions: func() {
			icon, text, label := iconPlus, "收藏", "收藏专辑"
			if d.activity != nil {
				if d.activity.IsSub {
					icon, text, label = iconCheck, "已收藏", "取消收藏专辑"
				}
				text += " " + count(d.activity.SubCount)
			}
			a.phoneDetailSegment(c, icon, text, func() { a.toggleAlbumSubscription(d) }).
				Disabled(d.subscribing || a.profile != nil && (d.activity == nil || d.activityLoading)).Label(label)
			phoneDetailDivider(c)
			a.phoneDetailSegment(c, iconComment, commentText, func() {
				a.phoneAlbumID = al.ID
				a.loadAlbumComments(d)
			}).Label("查看专辑评论")
			phoneDetailDivider(c)
			a.phoneDetailSegment(c, iconShare, "分享", func() {
				copyText(fmt.Sprintf("https://music.163.com/album?id=%d", al.ID))
				a.toast = "已复制链接"
			})
		},
	})
}

// artistPhoneHeader is the artist page's header on the phone: a full-bleed
// photo reaching the top of the screen, the name and a play button over a
// gradient at its foot, the other actions as pills below.
func (a *app) artistPhoneHeader(c *ui.Context, ar *netease.ArtistProfile, songs []netease.Track) {
	p := a.pal
	top, _ := a.insets(c)
	w := a.contentWidth(c)
	photo := ar.Avatar
	if photo == "" {
		photo = ar.PicURL
	}
	ui.Box(c.Key("photo")).Width(w).Height(w*0.85).Margin(-(top + pageGap), -a.pagePadding(), 0, -a.pagePadding()).
		TextColor(white).Children(func() {
		a.coverFill(c, photo, 800, 0)
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(w * 0.45).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0), At: 0},
				{Color: ui.RGBA(0, 0, 0, 0.6), At: 1},
			}})
		ui.Text(c, ar.Name).Absolute().Left(20).Bottom(14).FontSize(34).FontWeight(800).SingleLine()
		ui.ButtonBase(c.Key("play")).Absolute().Right(20).Bottom(12).Size(50, 50).Radius(25).Center().
			Background(p.accent).Label("播放").TextColor(p.onAccent).OnClick(func() {
			if a.pl.Shuffle() {
				a.pl.ToggleShuffle()
			}
			a.pl.PlayQueue(songs, 0)
		}).Children(func() {
			ui.Icon(c, iconPlay).Size(22, 22)
		})
	})
	ui.Row(c).Gap(8).Margin(14, 0, 0, 0).Children(func() {
		if a.phonePill(c.Key("shuffle"), iconShuffle, "随机播放", "").Clicked() {
			a.pl.PlayShuffled(songs)
		}
		icon, label := iconPlus, "关注"
		if ar.Followed {
			icon, label = iconCheck, "已关注"
		}
		if a.phonePill(c.Key("follow"), icon, label, label).Clicked() {
			a.toggleFollow(ar.ID)
		}
	})
}

// phonePill is a secondary action's compact pill; label is its
// accessibility name where it differs from the text shown.
func (a *app) phonePill(c *ui.Context, icon *ui.SVG, text, label string) ui.Element {
	p := a.pal
	if label == "" {
		label = text
	}
	return ui.ButtonBase(c).Height(34).PaddingX(12).Gap(5).Radius(17).AlignItems(ui.Center).
		Background(p.hover).Label(label).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14).TextColor(p.text)
		}
		ui.Text(c, text).FontSize(13).FontWeight(600).TextColor(p.text).SingleLine()
	})
}
