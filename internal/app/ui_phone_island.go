package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// islandBox is a capsule's rectangle in window DIPs.
type islandBox struct{ x, y, w, h float32 }

// phoneIslandSpec places the playback island against the screen's cutout
// and returns its compact and expanded rectangles. Phones with a Dynamic
// Island report a 59–62pt top inset and get a capsule grown from the
// island's pill; phones with a notch (44–50pt) grow theirs from the
// notch's height at the top edge; phones with neither get a floating
// pill. In landscape UIKit insets both sides alike and MyGo reports no
// orientation, so the island's side cannot be told and it stays away —
// the dock's mini player carries playback there.
func phoneIslandSpec(c *ui.Context) (compact, expanded islandBox, ok bool) {
	top, right, _, left := phoneSafeArea(c)
	winW, winH := c.Size()
	if winW > winH || max(left, right) > top {
		return islandBox{}, islandBox{}, false
	}
	var cutTop, cutH float32
	hasCut := true
	switch {
	case top >= 56: // Dynamic Island phones
		cutH = 37.33
		cutTop = (top - cutH) / 2
	case top >= 38: // notch phones
		cutH = 32
	default: // no cutout: home-button phones, previews, tests
		hasCut = false
	}
	cx := winW / 2
	cw, ch := float32(204), float32(44)
	var cy float32
	if hasCut {
		cy = cutTop + cutH/2
	} else {
		// With no cutout there is no room above the navigation bar, so
		// the capsule floats under it instead.
		cy = top + 44 + ch/2 + 6
	}
	compact = islandBox{cx - cw/2, max(0, cy-ch/2), cw, ch}
	ew := min(winW-24, 400)
	eh := float32(138)
	ey := float32(6)
	if hasCut {
		ey = max(0, cutTop-4)
		if cutH <= 34 { // a notch's capsule grows from the top edge
			ey = 0
		}
	}
	expanded = islandBox{(winW - ew) / 2, ey, ew, eh}
	return compact, expanded, true
}

var islandSpring = ui.SpringOptions{Duration: 400 * time.Millisecond, Bounce: 0.25}

// phoneIsland is the capsule at the top of the phone's pages: it rests on
// the screen's cutout with the cover, the track's name and the music's
// levels, and grows into mini controls on a tap.
func (a *app) phoneIsland(c *ui.Context, tr *netease.Track) {
	if tr == nil {
		return
	}
	compact, expanded, ok := phoneIslandSpec(c)
	if !ok {
		return
	}
	target := float32(0)
	if a.phoneIslandOpen {
		target = 1
	} else {
		// The scrub lets go of a drag the closing capsule cut short.
		a.phoneIslandScrub = scrubState{}
	}
	capsule := ui.Box(c.Key("phone-island"))
	t := capsule.AnimateTo("open", target, ui.Animation{Spring: islandSpring})
	w := compact.w + (expanded.w-compact.w)*t
	h := compact.h + (expanded.h-compact.h)*t
	x := compact.x + (expanded.x-compact.x)*t
	y := compact.y + (expanded.y-compact.y)*t
	capsule.Absolute().Top(y).Left(x).Width(w).Height(h).Radius(min(h/2, 36)).Clip().
		Background(ui.RGB(0, 0, 0)).TextColor(white).
		Shadow(0, 12, 30, -12, ui.RGBA(0, 0, 0, 0.4*t)).
		Label("播放灵动岛").Role(ui.RoleToggleButton).Checked(a.phoneIslandOpen).
		OnClick(func() {
			a.phoneIslandOpen = !a.phoneIslandOpen
			mygo.Haptics.Selection()
		})
	capsule.Children(func() {
		a.phoneIslandCompact(c, tr, compact.h, t)
		if t > 0.05 {
			a.phoneIslandExpanded(c, tr, t)
		}
	})
}

// phoneIslandCompact is the capsule's resting face, which fades as it
// grows: the cover, the track's name and a strip of the music's levels.
func (a *app) phoneIslandCompact(c *ui.Context, tr *netease.Track, h, open float32) {
	shown := max(0, 1-open*1.5)
	if shown < 0.02 {
		return
	}
	art := a.art.get(netease.Sized(tr.Album.PicURL, 96), false)
	ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(h).PaddingX(14).Gap(10).
		AlignItems(ui.Center).Opacity(shown).Children(func() {
		cov := ui.Box(c).Size(28, 28).Radius(8).Shrink(0)
		in := artIn(cov, bitmap(art))
		if in < 1 {
			cov.Background(white.Alpha(0.12))
		}
		cov.Children(func() { artImage(c, bitmap(art), 8, in) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, tr.Name).FontSize(12).FontWeight(600).SingleLine()
			sub := a.phonePlaybackStatus(tr)
			if sub == "" {
				sub = tr.ArtistNames()
			}
			ui.Text(c, sub).FontSize(10).TextColor(white.Alpha(0.55)).SingleLine()
		})
		a.phoneIslandLevels(c, 18, 26, 4)
	})
}

// phoneIslandLevels draws the strip of bars that follows the music's
// levels on the compact capsule.
func (a *app) phoneIslandLevels(c *ui.Context, w, h float32, n int) {
	ui.Box(c).Size(w, h).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := a.pl.Levels()
		a.phoneIslandBars.step(p.Now(), lv, live)
		drawBars(p, r, &a.phoneIslandBars, n, white.Alpha(0.85), 2)
		if live || a.phoneIslandBars.group(0, player.Bands) > 0.01 {
			p.AnimationFrame()
		}
	})
}

// phoneIslandExpanded is the capsule opened into mini controls: the
// cover and track, a scrubber with its times, and transport.
func (a *app) phoneIslandExpanded(c *ui.Context, tr *netease.Track, open float32) {
	shown := max(0, min(1, open*1.6-0.35))
	if shown < 0.02 {
		return
	}
	pl := a.pl
	art := a.art.get(netease.Sized(tr.Album.PicURL, 200), false)
	a.tickTimes(c)
	ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Clip().
		Padding(12, 18, 10, 18).Gap(6).Opacity(shown).IgnoreInput(shown < 0.98).
		TextColor(white).Children(func() {
		ui.Row(c).Height(42).AlignItems(ui.Center).Gap(10).Children(func() {
			openButton := ui.ButtonBase(c.Key("island-open")).Grow(1).MinWidth(0).Height(42).Gap(10).
				AlignItems(ui.Center).Justify(ui.Start).Label("灵动岛：打开正在播放").OnClick(a.phoneIslandNowPlaying)
			phonePressScale(c, openButton, 0.98)
			openButton.Children(func() {
				cov := ui.Box(c).Size(40, 40).Radius(10).Shrink(0)
				in := artIn(cov, bitmap(art))
				if in < 1 {
					cov.Background(white.Alpha(0.12))
				}
				cov.Children(func() { artImage(c, bitmap(art), 10, in) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, tr.Name).FontSize(15).FontWeight(700).SingleLine()
					if status := a.phonePlaybackStatus(tr); status != "" {
						ui.Text(c, status).FontSize(12).TextColor(white.Alpha(0.6)).SingleLine()
					} else {
						ui.Text(c, tr.ArtistNames()).FontSize(12).TextColor(white.Alpha(0.6)).SingleLine()
					}
				})
			})
			ui.Box(c).Size(34, 34).Center().Shrink(0).Children(func() {
				a.likeButton(c.Key("island-like"), tr, 16, white.Alpha(0.6))
			})
		})
		position := pl.Position()
		if a.phoneIslandScrub.active {
			position = secs(a.phoneIslandScrub.v)
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, clock(position)).FontSize(10.5).FontFeatures("tnum").SingleLine().TextColor(white.Alpha(0.5)).Width(36)
			a.scrubber(c.Key("island-scrub"), &a.phoneIslandScrub, white.Alpha(0.16), white.Alpha(0.9), 12, ui.Color{}).Grow(1)
			ui.Text(c, "-"+clock(pl.Duration()-position)).FontSize(10.5).FontFeatures("tnum").SingleLine().TextColor(white.Alpha(0.5)).Width(38).TextAlign(ui.End)
		})
		ui.Row(c).Height(44).AlignItems(ui.Center).Justify(ui.Center).Gap(36).Children(func() {
			iconButton(c.Key("island-prev"), iconPrev, 20, white.Alpha(0.85), "灵动岛上一首").Size(44, 44).OnClick(pl.Prev)
			islandPlay(c, pl.Playing(), func() {
				pl.TogglePlay()
				mygo.Haptics.Light()
			})
			iconButton(c.Key("island-next"), iconNext, 20, white.Alpha(0.85), "灵动岛下一首").Size(44, 44).OnClick(pl.Next)
		})
	})
}

// islandPlay is the capsule's play/pause button, labeled for the island.
func islandPlay(c *ui.Context, playing bool, onClick func()) {
	b := ui.ButtonBase(c.Key("island-play")).Size(52, 52).Center().Radius(999).
		Label("灵动岛播放/暂停").TextColor(white).OnClick(onClick)
	phonePressScale(c, b, 0.85)
	b.Children(func() { playPause(c, b, playing, 24) })
}

// phoneIslandNowPlaying is the open path shared by the capsule's cover
// and title, the same one the mini player takes.
func (a *app) phoneIslandNowPlaying() {
	a.phoneIslandOpen = false
	a.dismissSuggestions()
	a.phoneSearchEditing, a.phonePlaylistEditing = false, false
	a.phoneDetails = false
	a.phoneSheet = phoneSheetClosed
	a.nowPlaying = true
}
