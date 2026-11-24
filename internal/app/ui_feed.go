package app

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// followFeed is NetEase's followed-accounts timeline, swapped out in tests.
var followFeed = netease.FollowFeed

// feedPageSize is how many events a feed page asks for.
const feedPageSize = 30

// loadFeed loads the feed's first page, or its next one for more: the
// listener's follow timeline, off the UI thread, published through post.
func (a *app) loadFeed(more bool) {
	if a.profile == nil || a.feedPage.loading {
		return
	}
	if a.feedPage.loaded && (!more || !a.feedPage.more) {
		return
	}
	a.feedPage.loading, a.feedPage.err = true, ""
	before := a.feedBefore
	if !a.feedPage.loaded {
		before = -1
	}
	go func() {
		events, last, m, err := followFeed(feedPageSize, before)
		a.post(func() {
			a.feedPage.loading = false
			if err != nil {
				a.feedPage.err = "动态加载失败"
				return
			}
			a.feedEvents = append(a.feedEvents, events...)
			a.feedPage.loaded = true
			a.feedPage.more = m && len(events) > 0 && (before == -1 || last < before)
			a.feedBefore = last
		})
	}()
}

// refreshFeed starts the feed over, as tapping the tab again does.
func (a *app) refreshFeed() {
	a.feedEvents, a.feedPage, a.feedBefore = nil, paged{}, 0
	a.loadFeed(false)
}

// phoneFeed is the 动态 tab: the events of those the listener follows.
func (a *app) phoneFeed(c *ui.Context) {
	if a.profile != nil && a.feedPage.loaded && len(a.feedEvents) > 0 {
		top, bottom := a.insets(c)
		pad := a.pagePadding()
		scroll := a.ps()
		a.underTop = scroll.Y > 0
		events := a.feedEvents
		ui.List(c.Key("feed"), a.ls(), len(events)+2, func(i int) {
			switch i {
			case 0:
				a.heading(c, "动态", "")
			case len(events) + 1:
				a.pageFoot(c, &a.feedPage, func() { a.loadFeed(true) })
			default:
				e := &events[i-1]
				a.event(c.Key(e.ID), e)
			}
		}).Grow(1).TrackScroll(scroll).Padding(top+pageGap, pad, bottom+32, pad).ScrollbarInsets(top, 0, bottom, 0)
		return
	}
	p := a.pal
	a.scrollPage(c, func() {
		a.heading(c, "动态", "")
		switch {
		case a.profile == nil:
			ui.Column(c).Grow(1).Center().Padding(60, 0).Gap(18).Children(func() {
				ui.Text(c, "登录后查看关注的人的动态").FontSize(14).TextColor(p.muted)
				ui.PrimaryButton(c.Key("login"), "登录").OnClick(a.openLogin)
			})
		case a.feedPage.err != "" && !a.feedPage.loaded:
			a.centered(c, a.feedPage.err, func() { a.loadFeed(false) })
		case !a.feedPage.loaded:
			a.loadFeed(false)
			commentSkeleton(c.Key("loading"), a.pageShimmer(c), 3)
		default:
			ui.Text(c, "还没有动态，去关注喜欢的歌手和朋友吧").FontSize(14).TextColor(p.muted).Padding(40, 0).AlignSelf(ui.Center)
		}
	})
}
