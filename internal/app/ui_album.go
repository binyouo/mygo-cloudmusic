package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// albumDetail loads an album from NetEase; tests replace it.
var (
	albumDetail    = netease.AlbumDetail
	albumDynamic   = netease.AlbumDynamic
	subscribeAlbum = netease.SubscribeAlbum
)

// albumData is what the app has of an album.
type albumData struct {
	album           *netease.AlbumProfile
	loading         bool
	err             string
	activity        *netease.AlbumActivity
	activityLoading bool
	activityErr     string
	subscribing     bool
	tab             int
	comments        commentState
}

func (a *app) loadAlbum(id int64) {
	d := a.albums[id]
	if d == nil {
		d = &albumData{comments: commentState{thread: fmt.Sprintf("R_AL_3_%d", id)}}
		a.albums[id] = d
	}
	if d.activity == nil && !d.activityLoading {
		a.loadAlbumActivity(id, d)
	}
	if d.album != nil || d.loading {
		return
	}
	d.loading, d.err = true, ""
	detail := albumDetail
	go func() {
		al, err := detail(id)
		a.post(func() {
			d.loading = false
			if err != nil {
				d.err = "专辑加载失败"
				return
			}
			d.album = al
		})
	}()
}

// Counts failing to load leave the album and its songs available.
func (a *app) loadAlbumActivity(id int64, d *albumData) {
	if d.activityLoading {
		return
	}
	d.activityLoading, d.activityErr = true, ""
	dynamic := albumDynamic
	go func() {
		activity, err := dynamic(id)
		a.post(func() {
			d.activityLoading = false
			if err != nil {
				d.activityErr = "收藏数加载失败"
				return
			}
			d.activity = activity
		})
	}()
}

func (a *app) albumPage(c *ui.Context) {
	id := a.route.id
	d := a.albums[id]
	if d.album == nil {
		if !d.loading {
			a.centered(c, d.err, func() { a.loadAlbum(id) })
			return
		}
		s := a.pageShimmer(c)
		a.trackSkeleton(c, s, false, func(c *ui.Context) { a.playlistHeaderSkeleton(c, s) })
		return
	}
	if d.tab == 1 {
		a.scrollPage(c, func() {
			a.albumHeader(c, d)
			a.albumComments(c, d)
		})
		return
	}
	a.trackRows(c, d.album.Songs, false, func(c *ui.Context) { a.albumHeader(c, d) })
}

func (a *app) albumHeader(c *ui.Context, d *albumData) {
	al := d.album
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, al.PicURL, 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "专辑").FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, al.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			ui.RichText(c).FontSize(13).TextColor(p.muted).Children(func() {
				for i, ar := range al.Artists {
					if i > 0 {
						ui.Text(c, ", ")
					}
					a.link(c.Key(i), ar.Name, p.muted, p.text, route{page: pageArtist, id: ar.ID})
				}
				meta := fmt.Sprintf(" · %d 首", len(al.Songs))
				if al.PublishTime > 0 {
					meta = " · " + time.UnixMilli(al.PublishTime).Format("2006-01-02") + meta
				}
				ui.Text(c, meta)
			})
			if d.activityErr != "" {
				if ui.ButtonBase(c.Key("activity-retry")).Label("重试收藏数").Children(func() {
					ui.Text(c, d.activityErr+"，重试").FontSize(12).TextColor(p.muted)
				}).Clicked() {
					a.loadAlbumActivity(al.ID, d)
				}
			}
			if d := strings.TrimSpace(al.Description); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(al.Songs, 0)
				}
				icon, label, action := iconPlus, "收藏", "收藏专辑"
				if d.activity != nil {
					if d.activity.IsSub {
						icon, label, action = iconCheck, "已收藏", "取消收藏专辑"
					}
					label += " " + count(d.activity.SubCount)
				}
				if pillButton(c.Key("subscribe"), icon, label, p.hover.Over(p.bg), p.text).
					Label(action).Disabled(d.subscribing || a.profile != nil && (d.activity == nil || d.activityLoading)).Clicked() {
					a.toggleAlbumSubscription(d)
				}
			})
		})
	})
	label := "评论"
	if d.comments.loaded {
		label += " " + count(d.comments.total)
	} else if d.activity != nil {
		label += " " + count(d.activity.CommentCount)
	}
	ui.Row(c).Margin(0, 0, 18, 0).Children(func() {
		if segmented(c.Key("album-tabs"), &d.tab, "歌曲", label) && d.tab == 1 && !d.comments.loaded {
			if d.comments.thread == "" {
				d.comments.thread = fmt.Sprintf("R_AL_3_%d", al.ID)
			}
			a.loadComments(&d.comments)
		}
	})
}

// Collecting is optimistic, but a second click waits for the request to finish.
func (a *app) toggleAlbumSubscription(d *albumData) {
	if d.subscribing {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	if d.album == nil || d.activity == nil || d.activityLoading {
		return
	}
	before := *d.activity
	d.activity.IsSub, d.subscribing = !before.IsSub, true
	if d.activity.IsSub {
		d.activity.SubCount++
	} else {
		d.activity.SubCount = max(0, d.activity.SubCount-1)
	}
	id, sub, uid := d.album.ID, d.activity.IsSub, a.profile.UserID
	subscribe := subscribeAlbum
	go func() {
		err := subscribe(id, sub)
		a.post(func() {
			if a.profile == nil || a.profile.UserID != uid || a.albums[id] != d {
				return
			}
			a.albumSubscribed(d, before, err)
		})
	}()
}

func (a *app) albumSubscribed(d *albumData, before netease.AlbumActivity, err error) {
	d.subscribing = false
	switch {
	case err != nil:
		*d.activity = before
		a.toast = "操作失败，请稍后再试"
	case d.activity.IsSub:
		a.toast = "已收藏专辑"
	default:
		a.toast = "已取消收藏专辑"
	}
}

// albumComments keeps its thread separate from the song Now Playing shows.
func (a *app) albumComments(c *ui.Context, d *albumData) {
	cs, p := &d.comments, a.pal
	ui.Column(c.Key("album-comments")).Gap(2).Children(func() {
		a.albumCommentEditor(c.Key("editor"), d)
		if len(cs.hot) > 0 {
			commentHeading(c, "精彩评论", p.text)
			for i := range cs.hot {
				a.commentRow(c.Key(commentKey{true, cs.hot[i].ID}), &cs.hot[i], c.Now(), cs)
			}
		}
		if len(cs.latest) > 0 {
			commentHeading(c, "最新评论 "+count(cs.total), p.text)
			for i := range cs.latest {
				a.commentRow(c.Key(commentKey{false, cs.latest[i].ID}), &cs.latest[i], c.Now(), cs)
			}
		}
		if cs.loading {
			commentSkeleton(c.Key("loading"), a.pageShimmer(c), 3)
			return
		}
		switch {
		case cs.err != "":
			ui.Row(c).Gap(12).Padding(24, 12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cs.err).FontSize(13).TextColor(p.muted)
				if ui.Button(c.Key("retry"), "重试").Clicked() {
					a.loadComments(cs)
				}
			})
		case cs.loaded && cs.more:
			if ui.Button(c.Key("more"), "加载更多评论").AlignSelf(ui.Center).Margin(20, 0).Clicked() {
				a.loadComments(cs)
			}
		case cs.loaded && len(cs.latest) == 0 && len(cs.hot) == 0:
			a.empty(c, "还没有评论")
		case cs.loaded:
			a.empty(c, "没有更多评论了")
		}
	})
}

// albumCommentEditor keeps a draft with its album, including when sending fails.
func (a *app) albumCommentEditor(c *ui.Context, d *albumData) {
	cs, p := &d.comments, a.pal
	ui.Column(c).Gap(10).Padding(12).Children(func() {
		if a.profile == nil {
			if pillButton(c.Key("login"), iconComment, "登录后发表评论", p.hover, p.text).Clicked() {
				a.openLogin()
			}
			return
		}
		if cs.replyID != 0 {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "回复 @"+cs.replyTo).FontSize(12.5).TextColor(p.muted)
				if ui.ButtonBase(c.Key("cancel-reply")).Disabled(cs.sending).Label("取消回复").Children(func() {
					ui.Text(c, "取消").FontSize(12).TextColor(p.muted)
				}).Clicked() {
					cs.replyID, cs.replyTo = 0, ""
				}
			})
		}
		ui.TextArea(c.Key("draft"), &cs.draft).Bind(&cs.editor).Height(88).ReadOnly(cs.sending).Placeholder("说说你对这张专辑的感受…").Label("专辑评论")
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if cs.sendErr != "" {
				ui.Text(c, cs.sendErr).FontSize(12).TextColor(p.muted).Grow(1)
			} else {
				ui.Spacer(c)
			}
			label := "发表评论"
			if cs.sending {
				label = "正在发送…"
			}
			if ui.Button(c.Key("send"), label).Disabled(cs.sending || cs.loading || strings.TrimSpace(cs.draft) == "").Clicked() {
				a.publishAlbumComment(d)
			}
		})
	})
}

func (a *app) publishAlbumComment(d *albumData) {
	cs := &d.comments
	if cs.sending || cs.loading || strings.TrimSpace(cs.draft) == "" {
		return
	}
	if a.profile == nil {
		a.openLogin()
		return
	}
	thread, text, reply := cs.thread, strings.TrimSpace(cs.draft), cs.replyID
	cs.sending, cs.sendErr = true, ""
	send := sendComment
	go func() {
		cm, err := send(thread, text, reply)
		a.post(func() { a.albumCommentSent(cs, cm, err) })
	}()
}

func (a *app) albumCommentSent(cs *commentState, cm *netease.Comment, err error) {
	cs.sending = false
	if err != nil {
		cs.sendErr = "发送失败，请稍后重试"
		return
	}
	cs.draft, cs.replyID, cs.replyTo, cs.sendErr = "", 0, "", ""
	if cm != nil && cm.ID != 0 {
		cs.latest = append([]netease.Comment{*cm}, cs.latest...)
		cs.total++
		cs.offset++
		cs.loaded = true
	} else {
		cs.hot, cs.latest, cs.offset, cs.loaded = nil, nil, 0, false
		a.loadComments(cs)
	}
	a.toast = "评论已提交"
}

// albumGrid shows albums as cards, each opening its page.
func (a *app) albumGrid(c *ui.Context, albums []netease.AlbumProfile) {
	ui.Grid(c).Columns(a.columns(c, 156)).GapX(20).GapY(28).Children(func() {
		for i := range albums {
			a.albumCard(c.Key(albums[i].ID), &albums[i])
		}
	})
}

// albumCard is an album's cover, lifting under the pointer, over its name
// and the year it came out.
func (a *app) albumCard(c *ui.Context, al *netease.AlbumProfile) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(al.Name)
	hovered := card.Hovered()
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
		art.Children(func() { a.coverFill(c, al.PicURL, 400, 10) })
		ui.Text(c, al.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		var meta []string
		if a.route.page != pageArtist {
			meta = append(meta, al.ArtistNames())
		}
		if al.PublishTime > 0 {
			meta = append(meta, time.UnixMilli(al.PublishTime).Format("2006"))
		}
		ui.Text(c, strings.Join(meta, " · ")).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
	})
	if card.Clicked() {
		a.open(route{page: pageAlbum, id: al.ID})
	}
}
