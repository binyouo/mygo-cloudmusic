package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

var phoneTabs = []struct {
	page  pageKind
	title string
	icon  *ui.SVG
}{
	{pageHome, "推荐", iconHome},
	{pagePodcasts, "播客", iconPodcast},
	{pageFeed, "动态", iconFeed},
	{pageLibrary, "我的", iconUser},
}

type phoneNavigation struct {
	route            route
	history, forward []route
}

func (a *app) switchPhoneTab(page pageKind) {
	a.phoneEdge = ui.Point{}
	if page == a.phoneTab {
		// Tapping the tab again pops its navigation to the root, as
		// Apple's tabs do; at the root already, it scrolls back to the top.
		if len(a.history) > 0 {
			a.phoneNavDir = -1
			a.history, a.forward = nil, nil
			a.dropPhoneScrolls(1)
			a.show(a.phoneTabRoot(page))
			return
		}
		r := a.phoneTabRoot(page)
		if r == a.route {
			a.ls().ScrollTo(0, ui.Start)
			a.ps().ScrollTo(0, 0, ui.Smooth)
			if page == pageFeed {
				a.refreshFeed()
			}
			return
		}
		a.phoneNavDir = 0
		a.show(r)
		return
	}
	a.phoneNavDir = 0
	r := route{page: page}
	history, forward := []route(nil), []route(nil)
	if a.phoneRoutes == nil {
		a.phoneRoutes = make(map[pageKind]phoneNavigation)
	}
	a.phoneRoutes[a.phoneTab] = phoneNavigation{a.route, a.history, a.forward}
	if previous, ok := a.phoneRoutes[page]; ok {
		r, history, forward = previous.route, previous.history, previous.forward
	}
	a.phoneTab, a.history, a.forward, a.nowPlaying = page, history, forward, false
	a.show(r)
	mygo.Haptics.Selection()
}

// phoneTabRoot is the route a tab's root page shows.
func (a *app) phoneTabRoot(page pageKind) route {
	return route{page: page}
}

func (a *app) phoneView(c *ui.Context) {
	c.Root().Background(a.pal.bg)
	tr := a.pl.Current()
	if tr == nil {
		a.phoneIslandOpen = false
	}
	if a.nowPlaying && tr != nil {
		if !a.phoneShown {
			// The player rises into view on the pull machinery, which
			// starts it under the fold and brings it up in paint-only frames.
			// A drag on the mini player drives the morph itself.
			a.phoneShown = true
			a.phoneOpening = a.phoneMiniAxis != phoneAxisY
			if a.phoneOpening {
				mygo.Haptics.Light()
			}
		}
		var revealPage bool
		ui.Overlay(c, func() { revealPage = a.phoneNowPlaying(c, tr) })
		if revealPage {
			a.phonePages(c, tr, true)
		}
		if !a.phoneClosing {
			a.phonePlaybackSheet(c, tr)
		}
	} else {
		a.phonePull, a.phoneClosing, a.phoneShown = ui.Point{}, false, false
		a.phoneCoverOff = 0
		a.phoneSheetHeld, a.phoneSheetAxis, a.phoneSheetDX, a.phoneSheetDY = "", 0, 0, 0
		if a.phoneMiniHeld == "" {
			// A press holding the mini player still owns its totals.
			a.phoneMiniAxis, a.phoneMiniDX, a.phoneMiniDY = 0, 0, 0
			a.phoneOpenPull = 0
		}
		a.phonePaneAnim = false
		a.phoneMorph = phoneMorph{}
		a.phoneGhostLive, a.phoneContentIn, a.phoneControlsIn = false, false, false
		a.phoneCoverRect, a.phonePlayerMoving = ui.Rect{}, false
		a.phonePages(c, tr, false)
	}
	a.phoneAlbumComments(c.Key("album-comments-sheet"))
	a.phoneTrackSheet(c.Key("track-actions-sheet"))
	a.phoneProgramSheet(c.Key("program-sheet"))
	a.phonePlaylistActionsSheet(c.Key("playlist-actions-sheet"))
	a.phonePlaylistPicker(c.Key("playlist-picker"))
	if a.phoneDesc != "" {
		desc := a.phoneDesc
		a.phoneBottomSheet(c.Key("desc-sheet"), "简介", 0, false, func() { a.phoneDesc = "" }, nil, nil, func() {
			ui.Scroll(c.Key("desc-body")).Grow(1).MinHeight(0).Padding(4, 4).Children(func() {
				ui.Text(c, desc).FontSize(15).LineHeight(1.55).Selectable()
			})
		})
	}
	a.loginDialog(c)
	a.playlistNameDialog(c)
	a.deletePlaylistDialog(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
}

// npCoverMatch is the shared element the player's cover and the lyrics
// header's thumbnail are for their Magic Move, of one track.
// phonePressScale gives a phone control the little give of a press: it
// shrinks toward amount while held and springs back on release.
func phonePressScale(c *ui.Context, e ui.Element, amount float32) {
	pose := ui.Pose{Scale: 1}
	if e.Pressed() {
		pose.Scale = amount
	}
	e.AnimatePose(pose, ui.Snappy)
}

const (
	phoneDockInset    = 16
	phoneDockGap      = 7
	phonePlayerCorner = 36
	phoneMiniSize     = 56
	phoneMiniRadius   = 21
	phoneTabHeight    = 58
)

// phoneDock floats over the current tab and gives Now Playing a measured
// destination to return to.
func (a *app) phoneDock(c *ui.Context, tr *netease.Track, bottom float32, covered bool) {
	_, right, _, left := phoneSafeArea(c)
	winW, _ := c.Size()
	inset := max(phoneDockInset, (winW-left-right-560)/2)
	dock := ui.Column(c.Key("phone-dock")).Absolute().Left(left + inset).Right(right + inset).
		Bottom(max(10, bottom-14)).Gap(phoneDockGap).AccessHidden(covered)
	dock.Children(func() {
		if tr != nil {
			a.phoneMiniPlayer(c, tr)
		}
		a.phoneTabBar(c.Key("phone-tabs"))
	})
}

var phoneTabSnap = ui.Animation{Spring: ui.SpringOptions{Duration: 310 * time.Millisecond, Bounce: 0.18}}
var phoneTabTouch = ui.Animation{Duration: time.Nanosecond}

type phoneTabDrag struct {
	from, last int
	dx, dy     float32
}

func (d phoneTabDrag) horizontal() bool {
	return d.from >= 0 && abs32(d.dx) > 8 && abs32(d.dx) > abs32(d.dy)
}

func (d phoneTabDrag) slot(slotW float32) float32 {
	return clampL(float32(d.from)+d.dx/slotW, 0, float32(len(phoneTabs)-1))
}

func phoneTabIndex(page pageKind) int {
	for i, tab := range phoneTabs {
		if page == tab.page {
			return i
		}
	}
	return 0
}

func (a *app) phoneTabBar(c *ui.Context) {
	const padding = float32(4)
	p := a.pal
	bar := ui.Row(c.Key("tab-capsule")).Height(phoneTabHeight).Radius(23).Padding(padding).
		Material(glass.Glass{Tint: p.bg.Alpha(0.52)}).Shadow(0, 6, 20, -9, p.cardShade)
	drag := ui.Local(bar, "drag", func() phoneTabDrag { return phoneTabDrag{from: -1, last: -1} })
	width := bar.LayoutBounds().W
	if width <= 0 {
		winW, _ := c.Size()
		_, right, _, left := phoneSafeArea(c)
		width = min(winW-left-right-2*phoneDockInset, 560)
	}
	slotW := max(1, (width-2*padding)/float32(len(phoneTabs)))
	bar.Children(func() {
		selected := phoneTabIndex(a.phoneTab)
		held := false
		for i, tab := range phoneTabs {
			fg := p.text
			if i == selected {
				fg = p.accent
			}
			b := ui.ButtonBase(c.Key(tab.page)).Column().Grow(1).Basis(0).MinWidth(0).Radius(18).Center().
				Role(ui.RoleTab).Checked(i == selected).Label(tab.title).TextColor(fg).
				OnClick(func() { a.switchPhoneTab(tab.page) })
			if dx, dy, ok := b.Dragged(); ok {
				held = true
				if drag.from < 0 {
					*drag = phoneTabDrag{from: i, last: i}
				}
				drag.dx, drag.dy = drag.dx+dx, drag.dy+dy
				if drag.horizontal() {
					nearest := int(math.Round(float64(drag.slot(slotW))))
					if nearest != drag.last {
						drag.last = nearest
						mygo.Haptics.Selection()
					}
				}
			}
			if vx, _, ended := b.DragEnded(); ended && drag.horizontal() {
				projected := *drag
				projected.dx += vx * 0.10
				i := int(math.Round(float64(projected.slot(slotW))))
				if i != selected {
					a.switchPhoneTab(phoneTabs[i].page)
				}
			}
			b.Children(func() {
				pose := ui.Pose{}
				if b.Pressed() {
					pose.Scale = 0.90
				}
				label := ui.Column(c).Center().Gap(3).AnimatePose(pose, phoneTabSnap)
				label.Children(func() {
					ui.Icon(c, tab.icon).Size(21, 21)
					ui.Text(c, tab.title).FontSize(10).FontWeight(600)
				})
			})
		}
		if !held {
			*drag = phoneTabDrag{from: -1, last: -1}
		}
		slot, motion := float32(phoneTabIndex(a.phoneTab)), phoneTabSnap
		pose := ui.Pose{}
		if drag.horizontal() {
			slot, motion = drag.slot(slotW), phoneTabTouch
			pose.Scale = 0.94
		}
		pose.X = slot * slotW
		ui.Box(c.Key("selected-tab")).Absolute().Left(padding+1).Top(padding).Bottom(padding).
			Width(max(1, slotW-2)).Radius(18).ZIndex(-1).IgnoreInput(true).AccessHidden(true).
			Background(p.accent.Alpha(0.13)).AnimatePose(pose, motion)
	})
}

func clampL(v, lo, hi float32) float32 {
	return max(lo, min(v, hi))
}

// phoneHeaderInFlow reports whether the page keeps the header in its
// column: the search field and the playlist search sit in it, the other
// routes under the collapsing navigation bar.
func (a *app) phoneHeaderInFlow() bool {
	return a.route.page == pageSearch || (a.route.page == pagePlaylist && a.phonePlaylistSearch && a.phonePreviewScroll == nil)
}

// phoneNavBar is the collapsing bar over the top of the phone's pages:
// clear at rest, frosting as content scrolls under it, when its inline
// title fades in.
func (a *app) phoneNavBar(c *ui.Context, top float32, canBack bool) {
	p := a.pal
	edge := ui.Box(c.Key("phone-top-edge")).Absolute().Top(0).Left(0).Right(0).Height(top + 44).PassThrough()
	var shown float32
	if a.route.page == pagePlaylist || a.route.page == pageAlbum {
		shown = clamp01(a.ps().Y / 44)
		edge.Background(p.bg.Alpha(shown))
	} else {
		edge.Material(glass.ScrollEdge{Hard: true, Background: p.bg})
		shown = edgeShown(edge, a.underTop)
		edge.Opacity(shown)
	}
	if title := a.phoneTitle(); title != "" && a.route.page != pageHome {
		ui.Box(c).Absolute().Top(top).Left(56).Right(56).Height(44).Center().PassThrough().Children(func() {
			ui.Text(c, title).FontSize(17).FontWeight(600).SingleLine().Opacity(shown)
		})
	}
	padding, gap := float32(16), float32(10)
	if a.route.page == pageHome {
		padding, gap = a.pagePadding(), 8
	}
	ui.Row(c).Absolute().Top(top).Left(0).Right(0).Height(44).PaddingX(padding).Gap(gap).AlignItems(ui.Center).Children(func() {
		if canBack {
			a.phoneGlassButton(c.Key("back"), iconBack, "返回", a.back)
		}
		if a.route.page == pageHome {
			a.phoneHomeBar(c)
			return
		}
		ui.Spacer(c)
		switch a.route.page {
		case pagePlaylist:
			if a.playlists[a.route.id] != nil {
				a.phoneGlassButton(c.Key("search-playlist"), iconSearch, "在歌单内搜索", func() {
					a.phonePlaylistSearch, a.phonePlaylistEditing = true, true
					a.ls().ScrollTo(0, ui.Start)
				})
			}
		case pageLibrary:
			a.phoneGlassButton(c.Key("settings"), iconSettings, "设置", a.openSettings)
		}
	})
}

// phoneGlassButton is one of the round glass buttons of the navigation
// bar. The faint tint gives the disc a body: untinted glass over a plain
// background shows nothing but its rim, which reads as a broken ring.
func (a *app) phoneGlassButton(c *ui.Context, icon *ui.SVG, label string, onClick func()) ui.Element {
	tint := ui.RGBA(120, 120, 128, 0.13)
	if a.pal.dark {
		tint = ui.RGBA(255, 255, 255, 0.15)
	}
	return ui.ButtonBase(c).Size(36, 36).Radius(18).Center().Material(glass.Glass{Interactive: true, Tint: tint}).
		Label(label).TextColor(a.pal.text).OnClick(onClick).Children(func() {
		ui.Icon(c, icon).Size(18, 18)
	})
}

// phoneTitle is the inline title of the collapsing navigation bar, which
// shows once the page has scrolled under it.
func (a *app) phoneTitle() string {
	switch a.route.page {
	case pageHome:
		return "推荐"
	case pagePodcasts:
		return "播客"
	case pageFeed:
		return "动态"
	case pageLibrary:
		return "我的"
	case pageSettings:
		return "设置"
	case pageCharts:
		return "排行榜"
	case pageDaily:
		return "每日推荐"
	case pageFM:
		return "私人 FM"
	case pageLocal:
		return "本地音乐"
	case pagePlaylist:
		if pl := a.playlists[a.route.id]; pl != nil {
			return pl.Name
		}
	case pageAlbum:
		if d := a.albums[a.route.id]; d != nil && d.album != nil {
			return d.album.Name
		}
	case pageArtist:
		if d := a.artists[a.route.id]; d != nil && d.profile != nil {
			return d.profile.Name
		}
	case pageUser:
		if d := a.users[a.route.id]; d != nil && d.user != nil {
			return d.user.Nickname
		}
	case pagePodcast:
		if d := a.podcasts[a.route.id]; d != nil && d.podcast != nil {
			return d.podcast.Name
		}
	}
	return ""
}

func (a *app) phoneHeader(c *ui.Context, canBack bool) {
	p := a.pal
	top, _, _, _ := phoneSafeArea(c)
	if a.route.page == pageSearch {
		ui.Column(c.Key("phone-header")).Padding(top+8, 20, 10, 20).Shrink(0).Children(func() {
			ui.Row(c.Key("search-row")).Gap(8).AlignItems(ui.Center).Children(func() {
				if canBack {
					// KeepFocus: taking the tap's focus would blur the
					// field, the binding would clear editing, and the
					// header's rebuild mid-press could lose the click.
					iconButton(c.Key("back"), iconBack, 20, p.accent, "返回").Size(44, 44).KeepFocus().OnClick(func() {
						a.query = a.route.query
						a.endPhoneSearch()
						a.back()
					})
				}
				ui.Column(c.Key("search-field")).Grow(1).MinWidth(0).Children(func() {
					if a.phonePreviewScroll == nil {
						a.searchField(c)
						return
					}
					a.phoneSearchPreview(c)
				})
			})
		})
		return
	}
	ui.Row(c.Key("phone-header")).Padding(top+4, 12, 4, 12).MinHeight(top + 48).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
		if canBack {
			iconButton(c.Key("back"), iconBack, 20, p.accent, "返回").Size(44, 44).OnClick(a.back)
		}
		if a.route.page == pagePlaylist && a.phonePlaylistSearch {
			a.playlistSearch(c.Key("playlist-search"))
			ui.ButtonBase(c.Key("cancel-playlist-search")).MinWidth(44).Height(44).Shrink(0).Center().Label("取消歌单搜索").OnClick(func() {
				a.phonePlaylistSearch, a.phonePlaylistEditing = false, false
				a.clearPlaylistSearch()
			}).Children(func() { ui.Text(c, "取消").FontSize(14).TextColor(p.accent) })
		}
	})
}

func (a *app) phoneSearchPreview(c *ui.Context) {
	p := a.pal
	text, color := a.route.query, p.text
	if text == "" {
		text, color = "搜索歌曲、歌手、播客", p.muted
	}
	ui.Row(c).Height(40).Radius(12).PaddingX(10).Gap(7).AlignItems(ui.Center).Background(phoneSearchFill(p)).Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.muted)
		ui.Text(c, text).FontSize(15).TextColor(color).SingleLine().MinWidth(0).Shrink(1)
	})
}

// phoneSearchHistory is the search tab's idle body: the searches lately
// made and a grid of charts to browse.
func (a *app) phoneSearchHistory(c *ui.Context) {
	if len(a.charts) == 0 && !a.homeLoading && !a.searchBrowseAsked {
		a.searchBrowseAsked = true
		a.loadHome()
	}
	if len(a.hotSearches) == 0 && !a.hotLoading && !a.hotAsked && !a.hotFailed {
		a.hotAsked, a.hotLoading = true, true
		go func() {
			list, err := fetchHotSearches()
			a.post(func() {
				a.hotLoading = false
				if err != nil || len(list) == 0 {
					// Hides silently; showing the page again retries.
					a.hotFailed = true
					return
				}
				a.hotSearches = list
			})
		}()
	}
	p := a.pal
	a.scrollPage(c, func() {
		if len(a.settings.SearchHistory) > 0 {
			ui.Row(c).AlignItems(ui.Center).Margin(0, 0, 8, 0).Children(func() {
				ui.Text(c, "搜索历史").FontSize(16).FontWeight(700).Grow(1)
				iconButton(c.Key("clear-history"), iconTrash, 17, p.muted, "清除搜索历史").Size(44, 32).
					OnClick(a.clearSearchHistory)
			})
			// The chips wrap by hand, two rows at most: there is no flow
			// element, so the first row's break is measured up front and
			// whatever is left clips after the second row.
			winW, _ := c.Size()
			budget := winW - 2*a.pagePadding()
			split := len(a.settings.SearchHistory)
			x := float32(0)
			for i, q := range a.settings.SearchHistory {
				w, _ := c.MeasureText(0, ui.Span{Text: q, Size: 13})
				w += 28
				if x > 0 && x+8+w > budget {
					split = i
					break
				}
				x += w + 8
			}
			ui.Column(c).Gap(8).Children(func() {
				x = 0
				ui.Row(c).Gap(8).Children(func() {
					for _, q := range a.settings.SearchHistory[:split] {
						a.phoneHistoryChip(c, q)
					}
				})
				if split < len(a.settings.SearchHistory) {
					ui.Row(c).Gap(8).Children(func() {
						for _, q := range a.settings.SearchHistory[split:] {
							w, _ := c.MeasureText(0, ui.Span{Text: q, Size: 13})
							w += 28
							if x > 0 && x+8+w > budget {
								break
							}
							x += w + 8
							a.phoneHistoryChip(c, q)
						}
					})
				}
			})
		}
		a.phoneHotSearches(c)
		if len(a.charts) == 0 {
			return
		}
		ui.Text(c, "浏览榜单").FontSize(16).FontWeight(700).Margin(20, 0, 12, 0)
		ui.Grid(c).Columns(2).Gap(12).Children(func() {
			for i := range a.charts {
				pl := &a.charts[i]
				b := ui.ButtonBase(c.Key(pl.ID)).AspectRatio(1.6).Radius(12).Clip().Label(pl.Name).
					OnClick(func() { a.open(route{page: pagePlaylist, id: pl.ID}) })
				phonePressScale(c, b, 0.97)
				b.Children(func() {
					a.coverFill(c, pl.Cover(), 400, 12)
					ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(56).PassThrough().
						LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
							{Color: ui.RGBA(0, 0, 0, 0), At: 0},
							{Color: ui.RGBA(0, 0, 0, 0.62), At: 1},
						}})
					ui.Text(c, pl.Name).Absolute().Left(10).Bottom(8).Right(10).FontSize(15).FontWeight(700).
						TextColor(white).SingleLine()
				})
			}
		})
	})
}

// endPhoneSearch leaves the field's editing: the popup closes, and the
// focus parks on the anchor so its return to the input can't rearm it.
func (a *app) endPhoneSearch() {
	a.phoneSearchEditing = false
	a.dismissSuggestions()
	if a.phoneSearchPopup != nil {
		a.phoneSearchPopup(false)
	}
	if a.phoneSearchBlur != nil {
		a.phoneSearchBlur()
	}
}

// phoneHistoryChip is one past search, tapped to run it again.
func (a *app) phoneHistoryChip(c *ui.Context, q string) {
	// KeepFocus: a tap that blurs the field ends editing mid-press, which
	// rekeys the header and loses the click before it lands.
	b := ui.ButtonBase(c.Key("history:" + q)).Height(32).PaddingX(14).Radius(16).Center().KeepFocus().FocusRing(false).
		Label(q).OnClick(func() {
		a.endPhoneSearch()
		a.search(q)
	})
	if b.Pressed() {
		b.Background(a.pal.press)
	} else {
		b.Background(a.pal.hover)
	}
	b.Children(func() {
		ui.Text(c, q).FontSize(13).SingleLine()
	})
}

// phoneHotSearches is the 热搜榜 card of NetEase's trending keywords.
func (a *app) phoneHotSearches(c *ui.Context) {
	p := a.pal
	if len(a.hotSearches) == 0 {
		if !a.hotLoading || a.hotFailed {
			return
		}
		s := a.pageShimmer(c)
		ui.Column(c).Radius(14).Background(p.hover).Padding(14).Gap(2).Margin(20, 0, 0, 0).Children(func() {
			s.text(c, 16, 1.3).Width(64).Margin(0, 0, 6, 0)
			for range 10 {
				ui.Row(c).Height(40).Gap(10).AlignItems(ui.Center).Children(func() {
					s.text(c, 15, 1.3).Width(14)
					s.text(c, 15, 1.3).Grow(1).MaxWidth(160)
				})
			}
		})
		return
	}
	ui.Column(c).Radius(14).Background(p.hover).Padding(14).Margin(20, 0, 0, 0).Children(func() {
		ui.Text(c, "热搜榜").FontSize(16).FontWeight(700).Margin(0, 0, 6, 0)
		for i, h := range a.hotSearches[:min(10, len(a.hotSearches))] {
			word := h.Word
			b := ui.ButtonBase(c.Key("hot:" + word)).Height(40).Gap(10).AlignItems(ui.Center).KeepFocus().FocusRing(false).
				Label(word).OnClick(func() {
				a.endPhoneSearch()
				a.search(word)
			})
			if b.Pressed() {
				b.Background(p.press)
			}
			rank := i + 1
			b.Children(func() {
				ui.Box(c).Width(24).Center().Children(func() {
					t := ui.Text(c, fmt.Sprint(rank)).FontSize(15).FontWeight(700).FontFeatures("tnum")
					if rank <= 3 {
						t.TextColor(p.accent)
					} else {
						t.TextColor(p.muted)
					}
				})
				ui.Text(c, word).FontSize(15).Grow(1).Shrink(1).MinWidth(0).SingleLine()
				if badge := hotBadge(h.Icon); badge != "" {
					ui.Text(c, badge).FontSize(10).FontWeight(700).TextColor(p.accent).
						Padding(2, 6).Radius(8).Border(0.5, p.accent.Alpha(0.4))
				}
			})
		}
	})
}

// hotBadge reads the icon type NetEase marks a hot search with.
func hotBadge(icon int) string {
	switch icon {
	case 1:
		return "热"
	case 2:
		return "新"
	case 4:
		return "爆"
	}
	return ""
}

func (a *app) phoneLibrary(c *ui.Context) {
	p := a.pal
	a.scrollPage(c, func() {
		// The listener's own row doubles as the page's title.
		if a.profile == nil {
			b := ui.ButtonBase(c.Key("login")).Height(72).Gap(14).AlignItems(ui.Center).Label("登录网易云音乐").OnClick(a.openLogin)
			if b.Pressed() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Box(c).Size(64, 64).Radius(32).Center().Background(p.hover).Children(func() {
					ui.Icon(c, iconUser).Size(28, 28).TextColor(p.muted)
				})
				ui.Text(c, "立即登录").FontSize(20).FontWeight(700).Grow(1).SingleLine()
				ui.Icon(c, iconForward).Size(15, 15).TextColor(p.muted)
			})
		} else {
			id := a.profile.UserID
			b := ui.ButtonBase(c.Key("profile")).Height(72).Gap(14).AlignItems(ui.Center).Label(a.profile.Nickname).
				OnClick(func() { a.open(route{page: pageUser, id: id}) })
			if b.Pressed() {
				b.Background(p.hover)
			}
			b.Children(func() {
				a.avatar(c, a.profile.AvatarURL, 64, p.hover)
				ui.Text(c, a.profile.Nickname).FontSize(20).FontWeight(700).Grow(1).MinWidth(0).SingleLine()
				ui.Icon(c, iconForward).Size(15, 15).TextColor(p.muted)
			})
			for i := range a.mine {
				if a.mine[i].SpecialType == 5 {
					a.phoneLikedCard(c, &a.mine[i])
					break
				}
			}
		}
		a.phoneCategoryRow(c, iconNote, "本地音乐", func() { a.open(route{page: pageLocal}) })
		if a.profile == nil {
			return
		}
		// 创建 and 收藏 swap under the tabs; + makes a playlist of one's own.
		ui.Row(c).AlignItems(ui.End).Margin(24, 0, 4, 0).Children(func() {
			ui.Box(c).Grow(1).Children(func() {
				a.phoneTabs(c, &a.phoneMineTab, "创建歌单", "收藏歌单")
			})
			if a.phoneMineTab == 0 {
				b := ui.ButtonBase(c.Key("new-playlist")).Size(32, 32).Center().Label("新建歌单").
					OnClick(func() { a.newPlaylist(nil) })
				b.Children(func() {
					ui.Icon(c, iconPlus).Size(20, 20).TextColor(p.accent)
				})
			}
		})
		if a.phoneMineTab == 0 {
			var lists []netease.Playlist
			for i := range a.mine {
				if a.mine[i].SpecialType != 5 {
					lists = append(lists, a.mine[i])
				}
			}
			a.phonePlaylistRows(c, lists, true)
		} else {
			a.phonePlaylistRows(c, a.saved, true)
		}
	})
}

// phoneLikedCard is the 我喜欢的音乐 card of the 我的 page: an accent
// heart square, the name over its count, and the heart-mode pill.
func (a *app) phoneLikedCard(c *ui.Context, pl *netease.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c.Key("liked")).Height(72).Radius(14).PaddingX(14).Gap(12).AlignItems(ui.Center).Margin(0, 0, 4, 0).
		Background(p.hover).Label("我喜欢的音乐")
	card.Children(func() {
		ui.Box(c).Size(48, 48).Radius(10).Center().
			LinearGradient(ui.LinearGradient{Angle: 145, From: p.accent, To: p.accent.Mix(ui.RGB(0, 0, 0), 0.35)}).Children(func() {
			ui.Icon(c, iconHeartFill).Size(22, 22).TextColor(white)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, "我喜欢的音乐").FontSize(16).FontWeight(600).SingleLine()
			n := pl.TrackCount
			if n <= 0 {
				n = max(len(pl.Tracks), len(pl.TrackIDs))
			}
			ui.Textf(c, "%d 首", n).FontSize(13).TextColor(p.muted).SingleLine()
		})
		icon, text, run := a.heartModeAction()
		pill := ui.ButtonBase(c.Key("heart")).Height(30).PaddingX(10).Gap(4).Radius(15).AlignItems(ui.Center).
			Background(p.bg).Label(text)
		pill.Children(func() {
			ui.Icon(c, icon).Size(12, 12).TextColor(p.accent)
			ui.Text(c, text).FontSize(12).FontWeight(600).TextColor(p.accent).SingleLine()
		})
		if pill.Clicked() {
			run()
		}
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, id: pl.ID})
	}
}

// phonePlaylistRows lists playlists on the 我的 page: a cover, the name
// over a count, the actions control, divided by hairlines.
func (a *app) phonePlaylistRows(c *ui.Context, lists []netease.Playlist, manage bool) {
	p := a.pal
	for i := range lists {
		pl := &lists[i]
		if i > 0 {
			ui.Box(c).Height(1).Margin(0, 0, 0, 62).Background(p.hairline)
		}
		b := ui.ButtonBase(c.Key(pl.ID)).Height(68).Gap(10).AlignItems(ui.Center).Justify(ui.Start).FocusRing(false).Label(pl.Name)
		if b.Pressed() || b.Hovered() {
			b.Background(p.hover)
		}
		canManage := manage && pl.SpecialType != 5
		b.Children(func() {
			a.cover(c, pl.Cover(), 120, 52, 6)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, pl.Name).FontSize(16).SingleLine()
				n := pl.TrackCount
				if n <= 0 {
					n = max(len(pl.Tracks), len(pl.TrackIDs))
				}
				ui.Textf(c, "歌单 · %d 首", n).FontSize(13).TextColor(p.muted).SingleLine()
			})
			if canManage {
				a.phonePlaylistActions(c.Key("playlist-actions"), pl)
			}
		})
		if b.Clicked() {
			a.open(route{page: pagePlaylist, id: pl.ID})
		}
	}
}

// phoneCategoryRow is one of the library's flush category rows: an accent
// icon, the title, and a chevron, with a hairline under it from the
// caller.
func (a *app) phoneCategoryRow(c *ui.Context, icon *ui.SVG, title string, onClick func()) {
	p := a.pal
	b := ui.ButtonBase(c).Height(48).Gap(0).AlignItems(ui.Center).Label(title).OnClick(onClick)
	if b.Pressed() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Width(28).Children(func() {
			ui.Icon(c, icon).Size(22, 22).TextColor(p.accent)
		})
		ui.Text(c, title).Grow(1).FontSize(17).SingleLine()
		ui.Icon(c, iconForward).Size(13, 13).TextColor(p.faint)
	})
}

// phoneUnderline is the Match the selected tab's underline shares as it
// slides from one tab to the next.
type phoneUnderline struct{}

// phoneTabs is a row of underline text tabs, as the mobile app's: the
// chosen one is bold with an accent underline that slides to it. It
// reports whether the choice changed.
func (a *app) phoneTabs(c *ui.Context, selected *int, labels ...string) bool {
	p := a.pal
	before := *selected
	ui.Row(c).Gap(24).AlignItems(ui.End).Children(func() {
		for i, l := range labels {
			b := ui.ButtonBase(c.Key(l)).Column().AlignItems(ui.Center).Padding(6, 2, 0, 2).Gap(6).
				Role(ui.RoleTab).Checked(i == *selected).Label(l)
			b.Children(func() {
				fg, weight := p.muted, 400
				if i == *selected {
					fg, weight = p.text, 600
				}
				ui.Text(c, l).FontSize(15).FontWeight(weight).TextColor(fg).SingleLine()
				ui.Box(c).Size(16, 3).Radius(1.5).Children(func() {
					if i == *selected {
						bar := ui.Box(c).Fill().Radius(1.5).Background(p.accent)
						if !c.Preferences().ReduceMotion {
							bar.Transition(ui.ElementTransition{Match: phoneUnderline{}, Animation: &ui.Snappy})
						}
					}
				})
			})
			if b.Clicked() {
				*selected = i
			}
		}
	})
	if *selected != before {
		mygo.Haptics.Selection()
		return true
	}
	return false
}

// heartModeAction is the 心动模式 pill's icon, text and click, shared by
// the liked card and the playlist action bar.
func (a *app) heartModeAction() (*ui.SVG, string, func()) {
	switch {
	case a.pl.Radio() == heartRadio:
		return iconX, "退出心动模式", a.exitRadio
	case a.startingRadio() == heartRadio:
		return iconHeartFill, "取消开启心动模式", a.startHeartMode
	default:
		return iconHeartFill, "心动模式", a.startHeartMode
	}
}

func (a *app) openPhonePlayer() {
	a.dismissSuggestions()
	a.phoneSearchEditing, a.phonePlaylistEditing = false, false
	a.phoneDetails = false
	a.phoneSheet = phoneSheetClosed
	a.nowPlaying = true
}

func (a *app) phoneMiniPlayer(c *ui.Context, tr *netease.Track) {
	p := a.pal
	open := func() {
		if a.phoneMiniAxis != 0 || a.phoneMiniAte {
			// The press was a drag the mini player read — a pull up or a
			// track swipe — not the tap this button would answer.
			a.phoneMiniAte = false
			return
		}
		a.openPhonePlayer()
	}
	bar := ui.Row(c.Key("mini-player")).Height(phoneMiniSize).Shrink(0).Padding(0, 8).
		AlignItems(ui.Center).Radius(phoneMiniRadius).Material(glass.Glass{Tint: p.bg.Alpha(0.42)}).
		Shadow(0, 6, 20, -9, p.cardShade)
	if r := bar.LayoutBounds(); r.W > 0 && r.H > 0 {
		a.phonePlatter = r
	}
	a.phoneMiniDrag(c, "mini-player", bar)
	bar.Children(func() {
		openButton := ui.ButtonBase(c.Key("open-player")).Grow(1).MinWidth(0).Height(phoneMiniSize).Gap(10).
			AlignItems(ui.Center).Justify(ui.Start).Label("打开正在播放").OnClick(open)
		a.phoneMiniDrag(c, "open-player", openButton)
		// The platter gives on a press and bounces back, as Liquid Glass
		// does: a squash with an overshoot, in paint-only frames. The pose
		// carries the glass with it, so the whole capsule dents. The
		// transport buttons keep their own give.
		pillPose := ui.Pose{Scale: 1}
		if openButton.Pressed() {
			pillPose.Scale = 0.92
		}
		bar.AnimatePose(pillPose, ui.Animation{Spring: npGlassPress})
		pressPose := ui.Pose{Scale: 1}
		if openButton.Pressed() {
			pressPose.Scale = 0.96
		}
		openButton.AnimatePose(pressPose, ui.Animation{Spring: npGlassPress})
		// A track swipe slides the capsule's contents a little with it,
		// as the iPhone's does, and it springs home on release.
		slide, slideAnim := float32(0), ui.Snappy
		if a.phoneMiniAxis == phoneAxisX {
			slide = ui.Rubberband(a.phoneMiniDX, -120, 120, 300) * 0.45
			if a.phoneMiniHeld == "open-player" {
				slideAnim = ui.Animation{Duration: time.Nanosecond}
			}
		}
		openButton.AnimatePose(ui.Pose{X: slide}, slideAnim)
		openButton.Children(func() {
			// Apple Music's mini player is the cover thumbnail, the song
			// title on one line, and nothing else — no artist line, no
			// progress ring, no previous button.
			a.miniRing(c, tr, open)
			ui.Column(c).Grow(1).MinWidth(0).Justify(ui.Center).IgnoreInput(true).Children(func() {
				a.phoneMarquee(c, tr.Name)
			})
		})
		playButton(c.Key("mini-play"), a.pl.Playing(), 21, p.text).Size(44, 44).OnClick(a.pl.TogglePlay)
		iconButton(c.Key("mini-next"), iconNext, 20, p.text, "下一首").Size(44, 44).OnClick(a.pl.Next)
	})
}

// A drag's locked direction on the phone's touch surfaces: up on the
// mini player opens the player, down on the sheet pulls it back, across
// changes the track, as the iPhone's own does.
const (
	phoneAxisY = 1
	phoneAxisX = 2
)

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// phoneMiniDrag reads a drag on the mini player: straight up opens the
// player by hand — the sheet grows out of the capsule as far as the
// finger takes it and locks open past halfway or a fast enough fling —
// and across changes the track. A touch that ends before it moves far
// stays the button's own tap, which opens the player at once.
func (a *app) phoneMiniDrag(c *ui.Context, key string, e ui.Element) {
	dx, dy, held := e.Dragged()
	vx, vy, ended := e.DragEnded()
	canceled := e.PressCanceled()
	switch {
	case held && (a.phoneMiniHeld == key || (!a.nowPlaying && a.phoneMiniHeld == "")):
		if a.phoneMiniHeld != key {
			// A press on this part starts the totals over.
			a.phoneMiniHeld, a.phoneMiniAxis, a.phoneMiniDX, a.phoneMiniDY = key, 0, 0, 0
			a.phoneMiniAte = false
		}
		a.phoneMiniDX += dx
		a.phoneMiniDY += dy
		if a.phoneMiniAxis == 0 {
			if ax, ay := absf(a.phoneMiniDX), absf(a.phoneMiniDY); max(ax, ay) >= 8 {
				if ay > ax {
					a.phoneMiniAxis = phoneAxisY
				} else {
					a.phoneMiniAxis = phoneAxisX
				}
			}
		}
		if a.phoneMiniAxis == phoneAxisY {
			a.phoneOpenPull = max(0, -a.phoneMiniDY)
			if a.phoneOpenPull > 4 && !a.nowPlaying {
				a.openPhonePlayer()
			}
			if a.nowPlaying {
				a.phoneMorph = phoneMorph{to: float64(clamp01(a.phoneOpenPull / a.phoneMorphTravel(c)))}
			}
		}
	case a.phoneMiniHeld == key:
		if ended && !canceled {
			switch a.phoneMiniAxis {
			case phoneAxisY:
				travel := a.phoneMorphTravel(c)
				if a.nowPlaying && phonePullCommits(a.phoneOpenPull, -vy, travel) {
					a.phoneMorph.aim(1, c.Now(), -vy/travel)
					mygo.Haptics.Light()
				} else if a.nowPlaying {
					a.phoneClosing = true
					a.phoneMorph.aim(0, c.Now(), -vy/travel)
				}
			case phoneAxisX:
				if total := a.phoneMiniDX; absf(total) >= 48 || absf(vx) >= 800 {
					if total < 0 {
						a.pl.Next()
					} else {
						a.pl.Prev()
					}
					mygo.Haptics.Selection()
				}
			}
		} else if a.phoneMiniAxis == phoneAxisY && a.nowPlaying && !a.phoneClosing {
			a.phoneClosing = true
			a.phoneMorph.aim(0, c.Now(), 0)
		}
		// The press's own click lands on a later build; a drag's is no tap.
		a.phoneMiniAte = a.phoneMiniAxis != 0
		a.phoneMiniHeld, a.phoneMiniAxis, a.phoneMiniDX, a.phoneMiniDY = "", 0, 0, 0
		a.phoneOpenPull = 0
	}
}

// miniRing is the mini player's cover thumbnail: a plain rounded square
// like Apple Music's, so the flight to the big cover keeps one shape. A
// tap opens the player, as pressing the title beside it does.
func (a *app) miniRing(c *ui.Context, tr *netease.Track, open func()) {
	const size = 40
	art := ui.ButtonBase(c.Key("mini-ring")).Size(size, size).Radius(7).FocusRing(false).
		Label("迷你播放进度").Cursor(ui.CursorPointer).OnClick(open)
	a.phoneMiniDrag(c, "mini-ring", art)
	phonePressScale(c, art, 0.94)
	art.Children(func() {
		cover := a.coverTile(c, a.phoneThumbBitmap(tr), size, 7).IgnoreInput(true)
		if r := cover.LayoutBounds(); r.W > 0 && r.H > 0 {
			a.phoneMiniArt = r
		}
	})
}

// marqueeFrame is the repaint interval while the title is on the move.
const marqueeFrame = 1000 * time.Millisecond / 60

// phoneMarqueeOffset is how far the mini title is scrolled at now, the
// way Apple's MarqueeView does it: it holds for a beat, runs sideways at
// a steady rate, holds at the far end, and comes back. It returns the
// offset and how long until the next frame is needed, so a held title
// asks for no frames at all. (MusicApplication's marqueeDelay ≈ 3 s and
// marqueeScrollRate ≈ 30 pt/s, from the MarqueeView const pool.)
func phoneMarqueeOffset(now, start time.Time, overflow float32) (off float32, wait time.Duration) {
	const (
		delay = 3000 * time.Millisecond
		rate  = 30.0
		dwell = 900 * time.Millisecond
	)
	if overflow <= 0 {
		return 0, delay
	}
	travel := time.Duration(float64(overflow) / rate * float64(time.Second))
	cycle := 2*travel + 2*dwell
	t := now.Sub(start)
	if t < delay {
		return 0, delay - t
	}
	ph := (t - delay) % cycle
	leg := func(d time.Duration) float32 { return float32(float64(d) / float64(travel) * float64(overflow)) }
	switch {
	case ph < travel:
		return leg(ph), marqueeFrame
	case ph < travel+dwell:
		return overflow, travel + dwell - ph
	case ph < 2*travel+dwell:
		return overflow - leg(ph-travel-dwell), marqueeFrame
	default:
		return 0, cycle - ph
	}
}

// phoneMarquee is the mini player's song title. When the name is
// wider than the room the column gives it, the line scrolls sideways the
// way Apple's does instead of truncating; a name that fits draws once
// and asks for no frames.
func (a *app) phoneMarquee(c *ui.Context, title string) {
	const size, weight = 15, 600
	layout := ui.ShapeText(title, ui.Font{Size: size, Weight: weight}, 0)
	tw, th := layout.Size()
	if a.phoneMarqueeTitle != title {
		a.phoneMarqueeTitle, a.phoneMarqueeAt = title, c.Now()
	}
	start := a.phoneMarqueeAt
	ui.Box(c.Key("mini-title")).Height(th).MinWidth(0).Label(title).Draw(func(p *ui.Painter, r ui.Rect) {
		if tw <= r.W+0.5 || a.nowPlaying || a.phonePlayerMoving {
			p.TextLayout(layout, r.X, r.Y, p.Theme().Text)
			return
		}
		off, wait := phoneMarqueeOffset(p.Now(), start, tw-r.W)
		p.Clip(r, 0, func() { p.TextLayout(layout, r.X-off, r.Y, p.Theme().Text) })
		p.After(wait)
	})
}

func (a *app) phonePlaybackStatus(tr *netease.Track) string {
	heard := a.pickupHint(tr)
	if err := a.pl.Err(); err != "" {
		return err
	}
	if heard != "" {
		return heard
	}
	if a.pl.Loading() {
		return "正在缓冲…"
	}
	return ""
}

type phonePlayerSheet int

const (
	phoneSheetClosed phonePlayerSheet = iota
	phoneSheetQueue
	phoneSheetComments
	phoneSheetOptions
	phoneSheetDisplay
)

// phoneSheetDrag is a bottom sheet's pull: it starts under the fold and
// springs up, and a drag or a dismiss sends it back down before it goes.
type phoneSheetDrag struct {
	pull    ui.Point
	built   bool // shown under the fold for a frame already
	closing bool
}

func (a *app) openPhoneComments(tr *netease.Track) {
	if tr == nil || tr.LocalPath != "" {
		return
	}
	cs := a.commentsFor(tr)
	cs.name = tr.Name
	a.phoneSheet = phoneSheetComments
}

func (a *app) phonePlaybackSheet(c *ui.Context, tr *netease.Track) {
	sheet := a.phoneSheet
	var title string
	heightLimit := float32(0)
	switch sheet {
	case phoneSheetQueue:
		title = "待播清单"
	case phoneSheetComments:
		title = "评论"
	case phoneSheetOptions:
		title = "更多操作"
	case phoneSheetDisplay:
		title = "播放显示"
		switch {
		case tr.ProgramID != 0:
			heightLimit = 232
		case a.settings.LyricsStage:
			heightLimit = 540
		default:
			heightLimit = 396
		}
	default:
		return
	}
	a.phoneBottomSheet(c.Key(sheet), title, heightLimit, true, func() { a.phoneSheet = phoneSheetClosed }, func() {
		if sheet == phoneSheetDisplay {
			iconButton(c.Key("back"), iconBack, 18, white.Alpha(0.85), "返回更多操作").Size(44, 44).
				OnClick(func() { a.phoneSheet = phoneSheetOptions })
		}
	}, nil, func() {
		switch sheet {
		case phoneSheetQueue:
			// The queue's tools take a row of their own under the header:
			// up there beside the title they would cover it.
			ui.Row(c.Key("queue-tools")).Gap(8).Shrink(0).Padding(0, 0, 4, 0).Children(func() {
				ui.Spacer(c)
				pillButton(c.Key("queue-current"), nil, "当前", white.Alpha(0.08), white.Alpha(0.85)).
					Height(44).PaddingX(10).Label("定位当前歌曲").OnClick(func() { a.queueAt = -1 })
				label := "封面网格"
				if a.phoneQueueGrid {
					label = "列表"
				}
				pillButton(c.Key("queue-layout"), nil, label, white.Alpha(0.08), white.Alpha(0.85)).
					Height(44).PaddingX(12).Label(label).
					OnClick(func() {
						a.phoneQueueGrid = !a.phoneQueueGrid
						a.queueAt = -1
					})
			})
			a.phoneQueuePlaybackModes(c.Key("queue-modes"))
			if a.phoneQueueGrid {
				a.phoneCoverQueue(c)
			} else {
				a.queuePane(c)
			}
		case phoneSheetComments:
			ui.Text(c.Key("comment-track"), a.comments.name).FontSize(14).FontWeight(650).
				TextColor(white.Alpha(0.75)).SingleLine().Padding(8, 12).Shrink(0)
			a.commentsPane(c)
		case phoneSheetOptions:
			a.phonePlayerOptions(c, tr)
		case phoneSheetDisplay:
			a.phoneDisplayOptions(c, tr.ProgramID == 0)
		}
	})
}

func (a *app) phoneQueuePlaybackModes(c *ui.Context) {
	pl := a.pl
	repeatIcon, repeatLabel := iconRepeat, "关闭"
	switch pl.Repeat() {
	case player.RepeatAll:
		repeatLabel = "列表循环"
	case player.RepeatOne:
		repeatIcon, repeatLabel = iconRepeat1, "单曲循环"
	}
	ui.Row(c).Gap(8).Shrink(0).Padding(2, 2, 4, 2).Children(func() {
		phoneQueueMode(c.Key("shuffle"), iconShuffle, "随机播放", pl.Shuffle(), pl.ToggleShuffle).
			Role(ui.RoleToggleButton).Checked(pl.Shuffle())
		phoneQueueMode(c.Key("repeat"), repeatIcon, "循环", pl.Repeat() != player.RepeatOff, pl.CycleRepeat).
			Value(repeatLabel)
	})
}

func phoneQueueMode(c *ui.Context, icon *ui.SVG, label string, selected bool, run func()) ui.Element {
	bg, fg := white.Alpha(0.07), white.Alpha(0.76)
	if selected {
		bg, fg = white.Alpha(0.19), white
	}
	return pillButton(c, icon, label, bg, fg).Height(44).PaddingX(8).
		Grow(1).Basis(0).Justify(ui.Center).Label(label).OnClick(run)
}

func (a *app) phonePlayerOptions(c *ui.Context, tr *netease.Track) {
	a.phoneTrackOptions(c, tr, 0, white, func() { a.phoneSheet = phoneSheetClosed }, func() {
		if tr.ProgramID != 0 {
			phoneOptionRow(c.Key("program"), "节目简介", iconPodcast, iconForward, white,
				func() {
					a.phoneSheet = phoneSheetClosed
					a.openPhoneProgram(tr)
				})
		}
		phoneOptionRow(c.Key("display"), "播放显示", iconSettings, iconForward, white,
			func() { a.phoneSheet = phoneSheetDisplay })
		phoneOptionDivider(c.Key("track-actions"), white)
	})
}

func (a *app) phoneTrackSheet(c *ui.Context) {
	target := a.phoneTrackTarget
	if target == nil {
		return
	}
	tr := &target.track
	dismiss := func() { a.phoneTrackTarget = nil }
	fg := a.pal.text
	if a.nowPlaying {
		fg = white
	}
	a.phoneBottomSheet(c, trackActionsTitle(tr), 420, a.nowPlaying, dismiss, nil, nil, func() {
		a.phoneTrackOptions(c, tr, target.from, fg, dismiss, func() {
			if tr.ProgramID != 0 {
				phoneOptionRow(c.Key("program"), "节目简介", iconPodcast, iconForward, fg, func() {
					dismiss()
					a.openPhoneProgram(tr)
				})
			}
		})
	})
}

func (a *app) phoneTrackOptions(c *ui.Context, tr *netease.Track, pid int64, fg ui.Color, dismiss, leading func()) {
	ui.Row(c.Key("track")).MinHeight(62).Shrink(0).Padding(7, 10, 9, 10).Gap(12).
		AlignItems(ui.Center).Children(func() {
		a.cover(c, tr.Album.PicURL, 120, 46, 7)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, tr.Name).FontSize(16).FontWeight(700).SingleLine()
			ui.Text(c, tr.ArtistNames()).FontSize(13).TextColor(fg.Alpha(0.65)).SingleLine()
		})
	})
	ui.Scroll(c.Key("actions")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Children(func() {
			if leading != nil {
				leading()
			}
			if tr.ProgramID == 0 && tr.LocalPath == "" && a.profile != nil {
				phoneOptionRow(c.Key("playlist"), "添加到歌单…", iconPlus, iconForward, fg, func() {
					dismiss()
					a.phoneAddTo = &trackSelection{track: *tr, from: pid}
				})
			}
			index := 0
			a.songActions(tr, func(label string, disabled bool, run func()) {
				phoneOptionRow(c.Key(index), label, nil, nil, fg, func() {
					dismiss()
					run()
				}).Disabled(disabled)
				index++
			}, func() {
				phoneOptionDivider(c.Key(index), fg)
				index++
			})
			if tr.ProgramID == 0 && tr.LocalPath == "" && a.profile != nil && a.editable(pid) != nil {
				phoneOptionDivider(c.Key("remove-divider"), fg)
				phoneOptionRow(c.Key("remove"), "从歌单中移除", iconTrash, nil, fg, func() {
					dismiss()
					a.removeFrom(pid, tr)
				})
			}
		})
	})
}

func (a *app) phoneDisplayOptions(c *ui.Context, song bool) {
	ui.Scroll(c.Key("display-options")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Children(func() {
			if song {
				mode := a.lyricsPaneChosen()
				for _, choice := range []struct {
					pane  int
					label string
				}{{paneLyrics, "滚动歌词"}, {paneStage, "律动"}} {
					phoneDisplayChoice(c.Key(choice.pane), choice.label, mode == choice.pane, func() {
						a.phoneDetails = true
						a.choosePane(choice.pane)
						a.phoneSheet = phoneSheetClosed
					}).Role(ui.RoleRadio)
				}
				phoneOptionDivider(c.Key("lyric-options"), white)
				if mode == paneStage {
					for _, s := range stageStyles {
						phoneDisplayChoice(c.Key(s.id), s.label, a.settings.StageStyle == s.id, func() {
							if a.settings.StageStyle != s.id {
								a.settings.StageStyle = s.id
								a.settings.save()
							}
						}).Role(ui.RoleRadio)
					}
					phoneDisplayChoice(c.Key("sparks"), "火花", a.settings.StageSparks, func() {
						a.settings.StageSparks = !a.settings.StageSparks
						a.settings.save()
					})
				} else {
					phoneDisplayChoice(c.Key("blur"), "歌词模糊", a.settings.LyricsBlur, func() {
						a.settings.LyricsBlur = !a.settings.LyricsBlur
						a.settings.save()
					})
				}
				phoneOptionDivider(c.Key("cover-options"), white)
			}
			phoneDisplayChoice(c.Key("transition"), "封面过渡", a.settings.CoverTransition, func() {
				a.settings.CoverTransition = !a.settings.CoverTransition
				a.settings.save()
			})
			phoneOptionDivider(c.Key("fx-options"), white)
			phoneDisplayChoice(c.Key("smoke"), "进度条烟雾", a.settings.Smoke, func() {
				a.settings.Smoke = !a.settings.Smoke
				a.settings.save()
			})
			phoneDisplayChoice(c.Key("backdrop-motion"), "动态背景", a.settings.phoneBackdropMotion(), func() {
				enable := !a.settings.phoneBackdropMotion()
				a.settings.PhoneBackdropMotion = &enable
				a.settings.save()
			})
		})
	})
}

func phoneDisplayChoice(c *ui.Context, label string, selected bool, run func()) ui.Element {
	var check *ui.SVG
	if selected {
		check = iconCheck
	}
	return phoneOptionRow(c, label, nil, check, white, func() {
		run()
		mygo.Haptics.Selection()
	}).Role(ui.RoleCheckBox).Checked(selected)
}

func phoneOptionRow(c *ui.Context, label string, icon, trailing *ui.SVG, fg ui.Color, run func()) ui.Element {
	row := ui.ButtonBase(c).MinHeight(48).Padding(0, 11).Gap(12).Radius(9).AlignItems(ui.Center).
		Label(label).OnClick(run)
	if row.Pressed() || row.Hovered() {
		row.Background(fg.Alpha(0.08))
	}
	row.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(19, 19).TextColor(fg.Alpha(0.8))
		}
		ui.Text(c, label).Grow(1).MinWidth(0).FontSize(15).SingleLine()
		if trailing != nil {
			ui.Icon(c, trailing).Size(15, 15).TextColor(fg.Alpha(0.68))
		}
	})
	return row
}

func phoneOptionDivider(c *ui.Context, fg ui.Color) {
	ui.Box(c).Height(1).Margin(4, 11).Background(fg.Alpha(0.1))
}

const phonePanelMaxWidth, phonePanelSidePad = 560, 14

func (a *app) phoneBottomSheet(c *ui.Context, title string, heightLimit float32, playbackColors bool, dismiss, leading, header, body func()) {
	width, height := c.Size()
	top, right, bottom, left := phoneSafeArea(c)
	room := max(0, width-left-right)
	panelWidth := min(room, phonePanelMaxWidth)
	// The bottom inset also includes the keyboard. Keep the sheet content
	// in the visible area and let the panel paint down behind the inset.
	contentHeight := min(height*0.78, max(0, height-top-bottom))
	if heightLimit > 0 {
		contentHeight = min(contentHeight, heightLimit)
	}
	// Cramped sheets (short landscape windows, a keyboard up) shrink
	// their header as before.
	headerHeight := float32(52)
	if contentHeight < 220 {
		headerHeight = 46
	}
	p := a.pal
	bg, fg := p.bg, p.text
	if playbackColors {
		bg, fg = ui.Hex("#252528"), white
	}
	if a.phoneSheetDrags == nil {
		a.phoneSheetDrags = make(map[string]*phoneSheetDrag)
	}
	callerDismiss := dismiss
	dismiss = func() {
		delete(a.phoneSheetDrags, title)
		callerDismiss()
	}
	drag := a.phoneSheetDrags[title]
	if drag == nil {
		drag = &phoneSheetDrag{}
		a.phoneSheetDrags[title] = drag
	}
	open := true
	requestClose := func() { drag.closing = true }
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		total := contentHeight + bottom
		if !drag.built {
			// The rise is a pose: the sheet paints under the fold and
			// springs up, while what it holds stays tappable.
			drag.built = true
			panel.Jump("rise", 0)
		}
		rise := panel.AnimateTo("rise", 1, ui.Animation{Spring: npSpring})
		panel.AnimatePose(ui.Pose{Y: (1 - rise) * total}, ui.Animation{Duration: time.Nanosecond})
		if drag.closing {
			drag.pull.Y = total
			panel.IgnoreInput(true)
		}
		panel.Draggable(&drag.pull, ui.DraggableOptions{
			Axis:   ui.Vertical,
			Bounds: &ui.Rect{H: total},
			Rigid:  true,
			Snap: func(release ui.Point) ui.Point {
				if release.Y > 100 {
					drag.closing = true
					return ui.Point{Y: total}
				}
				return ui.Point{}
			},
			Animation: ui.Animation{Spring: npDragSpring},
		})
		shown := panel.DragOffset().Y + (1-rise)*total
		if drag.closing && shown >= total-2 {
			dismiss()
			return
		}
		backdrop.Background(ui.RGBA(0, 0, 0, 0.48*max(0, 1-shown/total)))
		panel.Absolute().Bottom(0).Left(left+(room-panelWidth)/2).Width(panelWidth).
			Height(contentHeight+bottom).Padding(0, 0, bottom, 0).
			Radius(26, 26, 0, 0).Clip().Background(bg).TextColor(fg).Label(title)
		ui.Box(c).Height(11).Shrink(0).Center().Children(func() {
			ui.Box(c).Size(36, 5).Radius(2.5).Background(fg.Alpha(0.25)).Margin(6, 0, 0, 0)
		})
		ui.Box(c).Height(headerHeight).Shrink(0).Children(func() {
			ui.Box(c).Absolute().Top(0).Bottom(0).Left(56).Right(56).Center().PassThrough().Children(func() {
				ui.Text(c, title).FontSize(17).FontWeight(600).SingleLine()
			})
			ui.Row(c).Fill().PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c).MinWidth(44).AlignItems(ui.Start).Children(func() {
					if leading != nil {
						leading()
					}
				})
				ui.Spacer(c)
				if header != nil {
					header()
				}
				ui.ButtonBase(c.Key("dismiss")).Size(44, 44).Center().Label("关闭" + title).OnClick(requestClose).Children(func() {
					ui.Box(c).Size(30, 30).Radius(15).Center().Background(fg.Alpha(0.1)).Children(func() {
						ui.Icon(c, iconX).Size(13, 13).TextColor(fg.Alpha(0.85))
					})
				})
			})
		})
		ui.Column(c).Grow(1).MinHeight(0).Padding(6, phonePanelSidePad, 4, phonePanelSidePad).Children(body)
	})
	if !open {
		// A backdrop tap slides the sheet down rather than taking it away.
		requestClose()
	}
}

// Overlays extend past the root's safe area. The older desktop MyGo
// module has no phone window or insets.
func phoneSafeArea(c *ui.Context) (top, right, bottom, left float32) {
	if safe, ok := any(c).(interface {
		SafeArea() (float32, float32, float32, float32)
	}); ok {
		return safe.SafeArea()
	}
	return 0, 0, 0, 0
}

func (a *app) phonePlaylistPicker(c *ui.Context) {
	pick := a.phoneAddTo
	if pick == nil {
		return
	}
	dismiss := func() { a.phoneAddTo = nil }
	a.phoneBottomSheet(c, "添加到歌单", 0, a.nowPlaying, dismiss, nil, nil, func() {
		ui.Scroll(c.Key("destinations")).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.ButtonBase(c.Key("new")).Height(48).PaddingX(10).Gap(10).AlignItems(ui.Center).Justify(ui.Start).
					Label("新建歌单").OnClick(func() {
					dismiss()
					a.newPlaylist(&pick.track)
				}).Children(func() {
					ui.Box(c).Size(32, 32).Center().Children(func() { ui.Icon(c, iconPlus).Size(20, 20) })
					ui.Text(c, "新建歌单").FontSize(15)
				})
				for _, pl := range a.playlistDestinations(pick.from) {
					ui.ButtonBase(c.Key(pl.ID)).Height(48).PaddingX(10).Gap(10).AlignItems(ui.Center).
						Label("添加到 " + pl.Name).OnClick(func() {
						dismiss()
						a.addTo(pl.ID, &pick.track)
					}).Children(func() {
						a.cover(c, pl.Cover(), 80, 32, 5)
						ui.Text(c, pl.Name).Grow(1).MinWidth(0).FontSize(15).SingleLine()
					})
				}
			})
		})
	})
}

func (a *app) phoneCoverQueue(c *ui.Context) {
	const gap, minTile = 10, 88
	_, right, _, left := phoneSafeArea(c)
	width, _ := c.Size()
	room := min(width-left-right, phonePanelMaxWidth) - 2*phonePanelSidePad
	columns := max(1, int((room+gap)/(minTile+gap)))
	tracks := a.pl.Queue()
	order, at := a.pl.PlayOrder()
	if at >= 0 {
		a.followQueueRow(&a.phoneWall, at/columns)
	}
	ui.List(c.Key("cover-queue"), &a.phoneWall, (len(order)+columns-1)/columns, func(row int) {
		ui.Grid(c).Columns(columns).Gap(gap).Children(func() {
			for n := row * columns; n < min(len(order), (row+1)*columns); n++ {
				index := order[n]
				tr := &tracks[index]
				playing := n == at
				button := ui.ButtonBase(c.Key(index)).Column().AlignItems(ui.Stretch).Gap(5).Label("选歌：" + tr.Name).
					Role(ui.RoleRadio).Checked(playing).OnClick(func() {
					if !playing {
						a.pl.Jump(index)
					}
				})
				button.Children(func() {
					cover := ui.Box(c).AspectRatio(1).Radius(8)
					if playing {
						cover.Border(2, a.pal.accent)
					}
					if tr.Album.PicURL == "" {
						cover.Center().Background(white.Alpha(0.06)).Children(func() {
							ui.Icon(c, iconNote).Size(26, 26).TextColor(white.Alpha(0.3))
						})
					} else {
						cover.Children(func() { a.coverFill(c, tr.Album.PicURL, 240, 8) })
					}
					ui.Text(c, tr.Name).FontSize(12).TextColor(white.Alpha(0.84)).SingleLine()
				})
			}
		})
	}).Grow(1).MinHeight(0).Gap(gap)
}
