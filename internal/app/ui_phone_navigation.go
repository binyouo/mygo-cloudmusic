package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Scroll and element identity belong to one entry in a tab's history.
type phonePageKey struct {
	route
	tab   pageKind
	depth int
}

type phonePageScroll struct {
	list   ui.ListState
	scroll ui.ScrollState
}

func (a *app) phonePageKey() phonePageKey {
	return phonePageKey{route: a.route, tab: a.phoneTab, depth: len(a.history)}
}

func (a *app) phoneScroll(key phonePageKey) *phonePageScroll {
	if a.phoneScrolls == nil {
		a.phoneScrolls = make(map[phonePageKey]*phonePageScroll)
	}
	st := a.phoneScrolls[key]
	if st == nil {
		st = &phonePageScroll{}
		a.phoneScrolls[key] = st
	}
	return st
}

// A new push replaces the tab's forward history, including its scroll positions.
func (a *app) dropPhoneScrolls(depth int) {
	for key := range a.phoneScrolls {
		if key.tab == a.phoneTab && key.depth >= depth {
			delete(a.phoneScrolls, key)
		}
	}
}

func (a *app) ps() *ui.ScrollState {
	if a.phonePreviewScroll != nil {
		return &a.phonePreviewScroll.scroll
	}
	if a.phone {
		return &a.phoneScroll(a.phonePageKey()).scroll
	}
	return &a.pageScroll
}

func (a *app) ls() *ui.ListState {
	if a.phonePreviewScroll != nil {
		return &a.phonePreviewScroll.list
	}
	if a.phone {
		return &a.phoneScroll(a.phonePageKey()).list
	}
	return &a.trackList
}

var pageSwipe = ui.Animation{Spring: ui.SpringOptions{Duration: 400 * time.Millisecond}}

// Keep the transition registered when a page is still or follows the
// finger. Omitting it would let MyGo create a leaving copy of a live page.
var phonePageStill = ui.ElementTransition{Duration: time.Nanosecond}

const phoneBackReach, phoneBackMinPull = 24, 32

func (a *app) phoneBackGesture(c *ui.Context, key phonePageKey, top, bottom, width float32) float32 {
	edge := ui.Box(c.Key(struct{ edge phonePageKey }{key})).Absolute().Top(top + 44).Left(0).Bottom(bottom).
		Width(phoneBackReach).ZIndex(1).KeepFocus().AccessHidden(true)
	// The rail lies over the page's scroller. Give a vertical pan back
	// to that scroller while the horizontal drag is still at rest.
	scroll := a.ps()
	edge.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || abs32(ev.DY) <= abs32(ev.DX) || a.phoneEdge.X != 0 || scroll.MaxY <= 0 {
			return false
		}
		scroll.Y = clampL(scroll.Y+ev.DY, 0, scroll.MaxY)
		return true
	})
	_, _, released := edge.DragEnded()
	edge.Draggable(&a.phoneEdge, ui.DraggableOptions{
		Axis:   ui.Horizontal,
		Bounds: &ui.Rect{W: width},
		Rigid:  true,
		Snap: func(projected ui.Point) ui.Point {
			if released && a.phoneEdge.X >= phoneBackMinPull && projected.X >= width/3 {
				return ui.Point{X: width}
			}
			return ui.Point{}
		},
		Animation: ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut},
	})
	shown := clampL(edge.DragOffset().X, 0, width)
	// Draggable writes the destination to phoneEdge on release. Wait
	// for the painted page to reach it before consuming the history entry.
	if !edge.Dragging() && a.phoneEdge.X == width && shown >= width-0.5 {
		a.popPage(0)
		return 0
	}
	return shown / width
}

func (a *app) phonePages(c *ui.Context, tr *netease.Track, covered bool) {
	top, _, bottom, _ := phoneSafeArea(c)
	w, h := c.Size()
	// Expand into the safe-area bands, which the page and dock pad separately.
	pages := ui.Column(c.Key("phone-pages")).Fill().Margin(-top, 0, -bottom, 0).MinHeight(h).IgnoreInput(covered)
	if covered {
		// Keep the tab, its artwork and both glass bars in one composited layer during the pull.
		pages.Cache().AccessHidden(true)
	}
	pages.Children(func() {
		progress := float32(0)
		if !covered && bottom <= 120 && w > 0 && len(a.history) > 0 {
			progress = a.phoneBackGesture(c, a.phonePageKey(), top, bottom, w)
		} else {
			a.phoneEdge = ui.Point{}
		}
		deck := ui.Column(c.Key("phone-deck")).Fill().Clip()
		reduced := c.Preferences().ReduceMotion
		lastTab := ui.Local(deck, "last-tab", func() pageKind { return a.phoneTab })
		travel := float32(0)
		if *lastTab != a.phoneTab {
			travel = 12
			if phoneTabIndex(a.phoneTab) < phoneTabIndex(*lastTab) {
				travel = -travel
			}
			*lastTab = a.phoneTab
		}
		if progress == 0 && !reduced {
			switch a.phoneNavDir {
			case 1:
				deck.ChildExit(ui.Motion{XPercent: -25, Opacity: 0.82})
			case -1:
				deck.ChildExit(ui.Motion{XPercent: 100})
			default:
				if travel != 0 {
					deck.ChildExit(ui.Motion{X: -travel})
				}
			}
		}
		deck.Children(func() {
			key := a.phonePageKey()
			if progress > 0 {
				prev := phonePageKey{route: a.history[key.depth-1], tab: key.tab, depth: key.depth - 1}
				a.phonePage(c, prev, top, true, func(page ui.Element) {
					page.Transition(phonePageStill)
					if !reduced {
						page.Translate(-w*0.25*(1-progress), 0).DrawOver(func(p *ui.Painter, r ui.Rect) {
							p.Fill(r, ui.RGBA(0, 0, 0, 0.10*(1-progress)), 0)
						})
					}
				})
			}
			a.phonePage(c, key, top, false, func(page ui.Element) {
				switch {
				case covered:
					page.Transition(phonePageStill)
				case progress > 0:
					page.Transition(phonePageStill)
					if reduced {
						page.Opacity(1 - progress)
					} else {
						page.Translate(w*progress, 0).Shadow(-8, 0, 20, 0, ui.RGBA(0, 0, 0, 0.14*(1-progress)))
					}
				case !reduced:
					switch a.phoneNavDir {
					case 1:
						page.ZIndex(1).Transition(ui.ElementTransition{Animation: &pageSwipe, Enter: &ui.Motion{XPercent: 100}, Exit: &ui.Motion{Opacity: 0}})
					case -1:
						page.ZIndex(-1).Transition(ui.ElementTransition{Animation: &pageSwipe, Enter: &ui.Motion{XPercent: -25, Opacity: 0.82}, Exit: &ui.Motion{Opacity: 0}})
					default:
						page.Transition(ui.ElementTransition{Duration: 150 * time.Millisecond, Enter: &ui.Motion{X: travel}, Exit: &ui.Motion{Opacity: 0}})
					}
				default:
					page.Transition(phonePageStill)
				}
			})
		})
		if bottom <= 120 {
			a.phoneDock(c, tr, bottom, covered)
		}
		// On iOS the system draws Now Playing around the Dynamic Island.
		if connectNativePlayback == nil {
			a.phoneIsland(c, tr)
		}
	})
}

func (a *app) phonePage(c *ui.Context, key phonePageKey, top float32, preview bool, motion func(ui.Element)) {
	page := ui.Column(c.Key(key)).Grow(1).MinHeight(0).MinWidth(0).Background(a.pal.bg)
	if preview {
		page.Absolute().Top(0).Bottom(0).Left(0).Right(0).ZIndex(-2).IgnoreInput(true).AccessHidden(true)
		r, underTop, underBottom := a.route, a.underTop, a.underBottom
		a.route, a.phonePreviewScroll = key.route, a.phoneScroll(key)
		defer func() {
			a.route, a.phonePreviewScroll = r, nil
			a.underTop, a.underBottom = underTop, underBottom
		}()
	}
	motion(page)
	page.Children(func() {
		inFlow := a.phoneHeaderInFlow()
		if inFlow {
			a.phoneHeader(c, key.depth > 0)
		}
		if key.page == pagePlaylist {
			a.phonePlaylistPending(c.Key("playlist-pending"))
		}
		body := ui.Column(c.Key("phone-content")).Grow(1).MinHeight(0).Clip()
		if key.page == pageSearch && !preview {
			// An empty-space tap gives the keyboard a place to leave the search field.
			body.Focusable().FocusRing(false)
		}
		body.Children(func() { a.content(c) })
		if !inFlow {
			a.phoneNavBar(c, top, key.depth > 0)
		}
	})
}
