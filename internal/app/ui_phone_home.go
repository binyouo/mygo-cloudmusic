package app

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// phoneHome is the phone's 推荐 page: a search capsule over round
// entries and shelves of playlists and charts, as the mobile app's.
func (a *app) phoneHome(c *ui.Context) {
	loading := len(a.featured) == 0 && len(a.charts) == 0
	if loading && !a.homeLoading {
		a.centered(c, a.homeErr, a.loadHome)
		return
	}
	pad := a.pagePadding()
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("home")).TrackScroll(a.ps()).Grow(1).Padding(top, pad, bottom+24, pad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		if loading {
			a.phoneHomeSkeleton(c, a.pageShimmer(c))
			return
		}
		a.phoneHomeEntries(c)
		if len(a.recPlaylists) > 0 {
			a.phoneHomeHeader(c, "推荐歌单", nil)
			a.phoneShelf(c, ui.SnapProximity, func() {
				for i := range a.recPlaylists {
					a.phoneShelfCard(c, &a.recPlaylists[i])
				}
			})
		}
		if len(a.charts) > 0 {
			a.phoneHomeHeader(c, "排行榜", func() { a.open(route{page: pageCharts}) })
			w, _ := c.Size()
			a.phoneShelf(c, ui.SnapMandatory, func() {
				for i := range a.charts {
					a.phoneChartCard(c, &a.charts[i], w-2*pad-24)
				}
			})
		}
		if len(a.featured) > 0 {
			a.phoneHomeHeader(c, "精选歌单", nil)
			a.phoneShelf(c, ui.SnapProximity, func() {
				for i := range a.featured {
					a.phoneShelfCard(c, &a.featured[i])
				}
			})
		}
	})
}

// phoneHomeBar pairs the account and search surfaces on the same baseline.
func (a *app) phoneHomeBar(c *ui.Context) {
	p := a.pal
	const surfaceHeight = 38
	account := ui.ButtonBase(c.Key("account")).Size(44, 44).Radius(22).Center().Label("账户")
	phonePressScale(c, account, 0.95)
	account.Children(func() {
		ui.Box(c).Size(surfaceHeight, surfaceHeight).Radius(surfaceHeight / 2).Center().Background(p.hover).Children(func() {
			if a.profile != nil && a.profile.AvatarURL != "" {
				a.coverFill(c, a.profile.AvatarURL, 120, surfaceHeight/2)
			} else {
				ui.Icon(c, iconUser).Size(18, 18).TextColor(p.muted)
			}
		})
	})
	account.OnClick(func() {
		if a.profile == nil {
			a.openLogin()
		} else {
			a.open(route{page: pageUser, id: a.profile.UserID})
		}
	})
	capsule := ui.ButtonBase(c.Key("search")).Grow(1).MinWidth(0).Height(44).Radius(22).
		AlignItems(ui.Center).Label("搜索歌曲、歌手、播客")
	phonePressScale(c, capsule, 0.985)
	capsule.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Height(surfaceHeight).Radius(surfaceHeight / 2).PaddingX(13).Gap(8).
			AlignItems(ui.Center).Background(p.hover).Children(func() {
			ui.Icon(c, iconSearch).Size(16, 16).Shrink(0).TextColor(p.muted)
			ui.Text(c, "搜索歌曲、歌手、播客").MinWidth(0).FontSize(14).TextColor(p.muted).SingleLine()
		})
	})
	capsule.OnClick(func() {
		a.phoneSearchEditing = true
		a.open(route{page: pageSearch})
	})
}

// phoneHomeHeader is a shelf's 18/700 title, with a muted "更多 >" where
// it opens a page.
func (a *app) phoneHomeHeader(c *ui.Context, title string, open func()) {
	p := a.pal
	if open == nil {
		ui.Text(c, title).FontSize(18).FontWeight(700).Margin(24, 0, 10, 0)
		return
	}
	ui.ButtonBase(c).AlignSelf(ui.Stretch).Margin(24, 0, 10, 0).AlignItems(ui.Center).Label(title).OnClick(open).Children(func() {
		ui.Text(c, title).FontSize(18).FontWeight(700).Grow(1)
		ui.Text(c, "更多").FontSize(13).TextColor(p.muted)
		ui.Icon(c, iconForward).Size(12, 12).TextColor(p.muted)
	})
}

const phoneHomeShortcutSize = 44

// phoneHomeEntries is the row of four round entries under the search bar.
func (a *app) phoneHomeEntries(c *ui.Context) {
	p := a.pal
	entry := func(key, label string, inside func(), run func()) {
		b := ui.ButtonBase(c.Key(key)).Grow(1).Basis(0).Column().AlignItems(ui.Center).Gap(6).Padding(3, 0).
			Label(label).OnClick(run)
		phonePressScale(c, b, 0.95)
		b.Children(func() {
			ui.Box(c).Size(phoneHomeShortcutSize, phoneHomeShortcutSize).Radius(phoneHomeShortcutSize / 2).
				Center().Background(p.hover).Children(inside)
			ui.Text(c, label).FontSize(12).FontWeight(500).SingleLine()
		})
	}
	ui.Row(c).Margin(10, 0, 0, 0).Children(func() {
		entry("daily", "每日推荐", func() {
			ui.Textf(c, "%d", c.Now().Day()).FontSize(17).FontWeight(700).FontFeatures("tnum").TextColor(p.accent)
		}, func() { a.open(route{page: pageDaily}) })
		entry("fm", "私人FM", func() {
			ui.Icon(c, iconRadio).Size(20, 20).TextColor(p.text)
		}, func() { a.open(route{page: pageFM}) })
		entry("charts", "排行榜", func() {
			ui.Icon(c, iconCharts).Size(20, 20).TextColor(p.text)
		}, func() { a.open(route{page: pageCharts}) })
		entry("podcasts", "播客", func() {
			ui.Icon(c, iconPodcast).Size(20, 20).TextColor(p.text)
		}, func() { a.switchPhoneTab(pagePodcasts) })
	})
}

// phoneShelf is a home shelf: a horizontal strip that bleeds to the
// screen's edges and snaps per its mode.
func (a *app) phoneShelf(c *ui.Context, snap ui.Snap, children func()) {
	pad := a.pagePadding()
	ui.ScrollHorizontal(c).Scrollbars(false).Gap(12).Margin(0, -pad, 0, -pad).Padding(0, pad).
		ScrollSnap(snap).Shrink(0).Children(children)
}

// phoneShelfCard is a 120 square playlist card of the shelves: its cover
// with a play-count badge, its name under it.
func (a *app) phoneShelfCard(c *ui.Context, pl *netease.Playlist) {
	card := ui.ButtonBase(c.Key(pl.ID)).Width(120).Column().Gap(6).AlignItems(ui.Stretch).FocusRing(false).Label(pl.Name)
	phonePressScale(c, card, 0.97)
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		if !art.InViewMargin(100, 132) {
			art.Background(a.pal.hover)
		} else {
			art.Children(func() {
				a.coverFill(c, pl.Cover(), 400, 10)
				if pl.PlayCount > 0 {
					ui.Row(c).Absolute().Top(6).Right(6).Padding(1, 6).Radius(999).Gap(3).AlignItems(ui.Center).
						Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(white).Children(func() {
						ui.Icon(c, iconPlay).Size(7, 7)
						ui.Text(c, plays(pl.PlayCount)).FontSize(10).FontWeight(600).SingleLine()
					})
				}
			})
		}
		ui.Text(c, pl.Name).FontSize(13).MaxLines(2).LineHeight(1.3)
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, id: pl.ID})
	}
}

// phoneChartCard is a chart's card of the paged shelf: the chart cover
// beside its name and its top three songs.
func (a *app) phoneChartCard(c *ui.Context, pl *netease.Playlist, w float32) {
	p := a.pal
	card := ui.ButtonBase(c.Key(pl.ID)).Width(w).MinHeight(120).Padding(14).Radius(14).Gap(12).AlignItems(ui.Center).
		Background(p.hover).FocusRing(false).Label(pl.Name).OnClick(func() { a.open(route{page: pagePlaylist, id: pl.ID}) })
	phonePressScale(c, card, 0.97)
	// The shells keep every snap point; only nearby cards lay out their
	// three-song preview and ask for a cover.
	if !card.InViewMargin(100, w) {
		return
	}
	card.Children(func() {
		ui.Box(c).Size(64, 64).Radius(9).Shrink(0).Children(func() {
			a.coverFill(c, pl.Cover(), 200, 9)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, pl.Name).FontSize(16).FontWeight(700).SingleLine().Shrink(1).MinWidth(0)
				ui.Text(c, pl.UpdateFreq).FontSize(12).TextColor(p.muted).SingleLine()
			})
			for i, line := range pl.Preview {
				if i >= 3 {
					break
				}
				ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Textf(c, "%d", i+1).FontSize(15).FontWeight(700).Width(13).FontFeatures("tnum").TextColor(p.accent)
					ui.RichText(c,
						ui.Span{Text: line.First, Weight: 500, Size: 14},
						ui.Span{Text: "  " + line.Second, Color: p.muted, Size: 12},
					).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	})
}

// phoneHomeSkeleton mirrors the page while home loads: the four
// circles, a shelf header and a row of squares under the bar's capsule.
func (a *app) phoneHomeSkeleton(c *ui.Context, s shimmer) {
	ui.Row(c).Margin(10, 0, 0, 0).Children(func() {
		for range 4 {
			ui.Column(c).Grow(1).Basis(0).AlignItems(ui.Center).Gap(6).Padding(3, 0).Children(func() {
				s.bone(c, phoneHomeShortcutSize/2).Size(phoneHomeShortcutSize, phoneHomeShortcutSize)
				s.text(c, 12, 1.2).Width(48)
			})
		}
	})
	s.text(c, 18, 1.2).Width(120).Margin(24, 0, 10, 0)
	ui.Row(c).Gap(12).Children(func() {
		for range 3 {
			ui.Column(c).Grow(1).Basis(0).Gap(6).Children(func() {
				s.bone(c, 10).AspectRatio(1)
				s.text(c, 13, 1.3).WidthPercent(70)
			})
		}
	})
}
