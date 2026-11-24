package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

var white = ui.RGB(255, 255, 255)

// npSpring is how Now Playing grows out of the player bar's cover and
// shrinks back into it, and how it springs back from a pull too short to
// close it: Apple's quick spring, 0.325 s at damping 0.8. Its channels
// are fractions of the way, so its precision stays the spring's own.
var npSpring = ui.SpringOptions{Duration: 325 * time.Millisecond, Bounce: 0.2}

// npDragSpring is npSpring for the pull channels, whose values are
// pixels: a sheet let go off-screen counts as arrived a couple of pixels
// short, rather than at a thousandth.
var npDragSpring = ui.SpringOptions{Duration: 325 * time.Millisecond, Bounce: 0.2, Precision: 2}

// npGlassPress is how the glass capsules give on a press and spring back:
// a quick squash with a visible bounce, as iOS's Liquid Glass does.
var npGlassPress = ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.45}

// npFade is how Now Playing comes and goes while the desktop asks for less
// motion: it fades, in place.
var npFade = ui.Animation{Duration: 200 * time.Millisecond, NonMotion: true}

// npState tracks Now Playing opening and closing, and reports whether it is
// open and whether it has come to rest, open or closed, as it last showed.
// Call it once a build, before the window's content.
func (a *app) npState(c *ui.Context) (open, settled bool) {
	open = a.nowPlaying && a.pl.Current() != nil
	if open != a.npOpen {
		a.npOpen = open
		// The window buttons light over its dark backdrop, and as the
		// theme over the pages (Windows).
		if a.win != nil {
			buttons := mygo.ThemeSystem
			if open {
				buttons = mygo.ThemeDark
			}
			a.win.SetWindowButtonsTheme(buttons)
		}
	}
	return open, a.npAtRest(open)
}

// npAtRest reports whether Now Playing showed at rest: all open, with
// nothing pulling on it, or all gone.
func (a *app) npAtRest(open bool) bool {
	if open {
		return a.npT >= 1 && a.npShift == 0 && !a.npHeld
	}
	return a.npT <= 0
}

// nowPlayingLayer holds Now Playing over the whole window. It grows out of
// the cover in the player bar as it opens, a card that takes the window,
// the cover flying out of the bar to its place, and shrinks back into it
// as it closes. Pulled down, it follows the pointer, smaller and rounder
// the farther it goes, and let go far or fast enough it falls into the
// bar; otherwise it springs back. The layer stays on top all the way,
// rather than leaving as an exit transition would, under the rest of the
// window: that would show it going over the sidebar's material while the
// page above it hid it at once.
func (a *app) nowPlayingLayer(c *ui.Context, open, settled bool) {
	w, h := c.Size()
	layer := ui.Box(c.Key("now-playing-layer")).Absolute().Top(0).Left(0).Right(0).Bottom(0)
	if a.wallpaper.active || a.wallpaper.closing && !open {
		layer.Invisible()
		return
	}
	a.npStill = c.Preferences().ReduceMotion
	spring := npSpring
	_, dy, held := layer.Dragged()
	held = held && open
	if held {
		a.npPull += dy
	}
	if _, vy, ok := layer.DragEnded(); ok && open {
		if a.npPull > h*0.2 || a.npPull > 24 && vy > 700 {
			// Let go far enough, or thrown: it falls the rest of the way
			// as fast as it was thrown.
			a.nowPlaying, open = false, false
			spring.Velocity = -max(vy, 0) / h
		}
	}
	if !held {
		a.npPull = 0
	}
	a.npHeld = held

	target := float32(0)
	if open {
		target = 1
	}
	var t, shift float32
	if a.npStill {
		t = layer.AnimateTo("open", target, npFade)
	} else {
		t = layer.AnimateTo("open", target, ui.Animation{Spring: spring})
	}
	if held {
		// Pulled up, it gives less and less.
		shift = layer.Jump("pull", ui.Rubberband(a.npPull, 0, h, h))
	} else {
		shift = layer.AnimateTo("pull", 0, ui.Animation{Spring: npDragSpring})
	}
	was := a.npT
	a.npT, a.npShift = t, shift
	if a.npAtRest(open) != settled {
		// Another build, for the page under it to come or go, and the
		// bar's cover with it.
		c.AnimationFrame()
	}
	// Back in the bar, it shows a frame more, its cover over the bar's:
	// the bar, built before it, hid its own cover in this frame, and
	// shows it from the next.
	if t <= 0 && !open && was <= 0 || a.pl.Current() == nil {
		// Closed: nothing to paint or click.
		layer.Invisible()
		a.queueAt, a.wallState.track = -1, 0
		return
	}
	if a.npStill {
		if t < 1 {
			layer.Opacity(max(t, 0))
		}
	} else {
		a.npCard(layer, t, shift, w, h)
	}
	if !open {
		// On its way out, the window under it takes the clicks.
		layer.IgnoreInput(true).PassThrough()
	}
	layer.Children(func() { a.nowPlayingView(c) })
}

// npCard clips the layer to the card Now Playing is, t of the way open,
// from the player bar's cover to the window, and moves it shift down, a
// little smaller and rounder for it.
func (a *app) npCard(layer ui.Element, t, shift, w, h float32) {
	k := 1 - clamp01(t) // how near the bar's cover it is
	pulled := clamp01(shift / h)
	if k == 0 && shift == 0 {
		return
	}
	from := a.npFromRect(w, h)
	radius := npFromRadius*k + 28*clamp01(shift/120)
	layer.ClipInset(from.Y*k, (w-from.X-from.W)*k, (h-from.Y-from.H)*k, from.X*k, radius)
	if shift != 0 {
		s := 1 - 0.12*pulled
		layer.TransformOrigin(0.5, 0).Scale(s, s).Translate(0, shift)
	}
}

// npFromRect is the player bar's cover, in a window w by h, that Now
// Playing grows out of: where the bar last showed it, or where the bar
// would, before it has.
func (a *app) npFromRect(w, h float32) ui.Rect {
	if a.npFrom.W > 0 {
		return a.npFrom
	}
	return ui.Rect{X: sidebarWidth + 21, Y: h - (playerBarHeight+48)/2, W: 48, H: 48}
}

// npMorph moves e, a cover of Now Playing, out of the player bar's cover
// to its place as Now Playing opens, and back into it as it closes: the
// song in the bar grows into the view.
func (a *app) npMorph(c *ui.Context, e ui.Element) ui.Element {
	if a.npT >= 1 || a.npStill || a.wallpaper.active || a.wallpaper.closing {
		return e
	}
	b := e.LayoutBounds()
	if b.W <= 0 {
		// Not laid out yet: the next frame places it.
		c.AnimationFrame()
		return e.Opacity(0)
	}
	w, h := c.Size()
	from := a.npFromRect(w, h)
	k := 1 - a.npT
	s := 1 + (from.W/b.W-1)*k
	return e.TransformOrigin(0, 0).Scale(s, s).Translate((from.X-b.X)*k, (from.Y-b.Y)*k)
}

// npBreathe scales e, a cover that breathes, to scale: there as a pose
// moves it, while Now Playing is open, and at once on its way to or from
// the bar, coming to its full size as it gets there, the size of the
// bar's cover it takes the place of.
func (a *app) npBreathe(e ui.Element, scale float32) {
	if a.npT < 1 && !a.npStill && !a.wallpaper.active && !a.wallpaper.closing {
		s := lerp(1, scale, clamp01(a.npT))
		e.Scale(s, s)
		return
	}
	e.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
}

// npRadius is the corner radius, r at rest, of a cover size across that
// npMorph moves: its corners, shrinking with it, stay as round on screen
// as they are on its way to the bar's cover's, and are that round as it
// gets there, rather than going sharp and rounding at once as the bar's
// cover takes its place.
func (a *app) npRadius(c *ui.Context, r, size float32) float32 {
	if a.npT >= 1 || a.npStill || a.wallpaper.active || a.wallpaper.closing {
		return r
	}
	w, h := c.Size()
	k := 1 - clamp01(a.npT)
	s := 1 + (a.npFromRect(w, h).W/size-1)*k // as npMorph scales it
	return min(lerp(r, npFromRadius, k)/s, size/2)
}

// npFromRadius is the corner radius of the player bar's cover.
const npFromRadius = 8

// npRise fades e in and lifts it into place as Now Playing opens, once the
// cover is on its way: the nth of the things that do, one after another,
// going again the other way as it closes.
func (a *app) npRise(e ui.Element, n int) ui.Element {
	if a.npT >= 1 || a.npStill || a.wallpaper.active || a.wallpaper.closing {
		return e
	}
	from := 0.3 + 0.07*float32(n)
	k := smoothstep(from, from+0.4, a.npT)
	return e.Opacity(k).Translate(0, 24*(1-k))
}

// nowPlayingView fills the window with what plays: its cover over a slow
// drift of its own colors, the controls, and lyrics that follow the song.
func (a *app) nowPlayingView(c *ui.Context) {
	a.nowPlayingSurface(c, false)
}

func (a *app) nowPlayingSurface(c *ui.Context, fullscreen bool) {
	tr := a.pl.Current()
	w, h := c.Size()
	art, tint := a.prepareNowPlayingArt(c, tr)
	shown := a.npArt
	accent := tint.Mix(white, 0.45)

	bg := ui.Hex("#151519")
	if !fullscreen && a.npT < 1 && !a.npStill {
		bg = bg.Alpha(smoothstep(0, 0.35, a.npT))
	}
	root := ui.Box(c.Key("now-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(bg).TextColor(white)
	// The pointer is over the view whenever it is in the window.
	pointerIn := root.Hovered()
	root.Children(func() {
		bloom := ui.Box(c.Key("bloom")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
		if !fullscreen && a.npT < 1 && !a.npStill {
			bloom.Opacity(smoothstep(0.05, 0.55, a.npT))
		}
		bloom.Children(func() { a.backdrop(c, shown, tint) })
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		content := ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0)
		if fullscreen {
			pose := ui.Pose{Fade: 1, Y: 10}
			motion := ui.Animation{Duration: 620 * time.Millisecond, Delay: 90 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}
			if a.wallpaper.revealed {
				pose = ui.Pose{}
			} else if a.wallpaper.closing {
				pose.Y = -4
				motion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.EaseOut}
			}
			if c.Preferences().ReduceMotion {
				pose.Y = 0
				motion.Duration, motion.Delay = 160*time.Millisecond, 0
			}
			content.AnimatePose(pose, motion)
		}
		content.Children(func() { a.nowPlayingBody(c, tr, art, accent, w, h, pointerIn, fullscreen) })
	})
	if fullscreen && root.DoubleClicked() {
		a.leaveWallpaper()
	}
}

func (a *app) nowPlayingBody(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32, pointerIn, fullscreen bool) {
	pl := a.pl
	if tr.LocalPath != "" && a.npPane == paneComments {
		a.npPane = paneLyrics
	}
	a.commentsFor(tr)
	// An episode has what it is about and its captions in place of lyrics;
	// a song, lyrics in place of those.
	switch program := tr.ProgramID != 0; {
	case program && a.npPane == paneWall:
		a.npPane = paneQueue
	case program && (a.npPane == paneLyrics || a.npPane == paneStage):
		a.npPane = paneProgram
		if a.captionsOn() {
			a.npPane = paneCaptions
		}
	case program && a.npPane == paneCaptions && !a.captionsShown():
		a.npPane = paneProgram
	case !program && (a.npPane == paneProgram || a.npPane == paneCaptions):
		a.npPane = a.lyricsPaneChosen()
	}
	if a.npPane != paneQueue {
		a.queueAt = -1
	}
	if a.npPane != paneWall {
		a.wallState.track = 0
	}
	{
		// The top strip drags the window; the chevron closes the view,
		// showing only while the pointer is in the window, as the pane tabs do.
		top := ui.Row(c).Height(topBar(c)).Justify(ui.Center).AlignItems(ui.Center)
		if !fullscreen {
			top.DragWindow()
		}
		a.npRise(top, 0).Children(func() {
			reveal(ui.Box(c.Key("close-reveal")), pointerIn).Children(func() {
				if iconButton(c.Key("close"), iconDown, 22, white.Alpha(0.7), "关闭").Clicked() {
					a.closeNowPlaying()
				}
			})
		})
		if a.npPane == paneStage {
			// The stage takes the whole view: the lyrics are the show, the
			// controls a slim bar at the foot.
			a.npRise(reveal(npTabsRow(c, w), pointerIn), 0).Children(func() {
				a.paneTabs(c)
				a.stageStyleButton(c)
			})
			a.stage(c, tr, accent)
			a.stageBar(c, tr, art, accent)
			return
		}
		if a.npPane == paneWall {
			// The wall takes the whole view too, under the song playing.
			a.wallPane(c, tr, art, accent, w, h, pointerIn)
			return
		}

		side := float32(math.Min(float64(w)*0.36, 440))
		// The cover leaves room above it for the tabs, starting as far
		// under them as the wall's song does.
		coverSize := min(side, h-436)
		// The column is as wide as the cover, for the cover, what plays
		// and the tabs over it to start at one edge, but wide enough for
		// the controls.
		side = max(coverSize, 280)
		padX := npSideX(w)
		a.npRise(reveal(npTabsRow(c, w), pointerIn), 0).Children(func() { a.paneTabs(c) })
		ui.Row(c).Grow(1).Padding(0, padX, 44, padX).Gap(72).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(side).Gap(0).Children(func() {
				// Extra metadata pushes controls down, never the cover into the tabs.
				ui.Box(c.Key("over-cover")).Grow(1).Basis(0).MinHeight(npCoverTop)
				a.npMorph(c, ui.Box(c.Key("cover-morph")).Size(coverSize, coverSize).AlignSelf(ui.Start)).Children(func() {
					a.npCover(c, tr, art, coverSize)
				})

				a.npRise(ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(36, 0, 18, 0), 1).Children(func() {
					// In the gap over the title, their left edge level with
					// the title's, without reserving a row for them.
					ui.Row(c.Key("np-switches")).Absolute().Top(-30).Left(0).Gap(6).Children(func() {
						if a.npPane == paneLyrics {
							a.lyricClarityButton(c.Key("lyric-clarity"))
						}
						a.coverTransitionButton(c.Key("cover-transition"))
					})
					metadata := ui.Column(c.Key(tr.ID)).Grow(1).Shrink(1).MinWidth(0).Gap(4)
					if !c.Preferences().ReduceMotion {
						metadata.Transition(ui.ElementTransition{Duration: 380 * time.Millisecond, Enter: &ui.Motion{Y: 6}, Exit: &ui.Motion{Y: -4}})
					}
					metadata.Children(func() {
						if r := pl.Radio(); r != "" {
							a.radioChip(c, r)
						}
						ui.Text(c, tr.Name).FontSize(22).FontWeight(700).LetterSpacing(-0.3).SingleLine()
						a.artistLinks(c, tr, white.Alpha(0.6), white, true).FontSize(15).SingleLine()
					})
					a.likeButton(c.Key("like"), tr, 22, white.Alpha(0.7))
					a.songMenu(c.Key("more"), tr, 22, white.Alpha(0.7)).Margin(0, -8, 0, 0)
				})
				a.npRise(a.scrubber(c.Key("np-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 16, accent.Mix(white, 0.35)), 2)
				// As tall with the quality's badge as without it: the badge goes
				// while the next song loads, and the column would move with it.
				a.npRise(ui.Row(c).Height(16).AlignItems(ui.Center), 2).Children(func() {
					ui.Text(c, clock(pl.Position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
					ui.Spacer(c)
					qualityBadge(c, pl.Stream())
					ui.Spacer(c)
					status := "-" + clock(pl.Duration()-pl.Position())
					if pl.Loading() {
						status = "正在缓冲…"
					}
					ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				})
				if pl.Err() != "" {
					ui.Text(c, pl.Err()).FontSize(12).LineHeight(1.35).TextColor(white.Alpha(0.75)).MaxLines(3).Margin(8, 0, 0, 0)
				}
				a.npRise(ui.Box(c).Margin(14, 0, 18, 0), 3).Children(func() {
					a.transport(c, white, white.Alpha(0.45), accent, 28)
				})
				a.npRise(ui.Row(c).Justify(ui.Center), 4).Children(func() {
					a.volumeSlider(c, white.Alpha(0.7), white.Alpha(0.18), 180)
				})
				ui.Box(c.Key("under-controls")).Grow(1).Basis(0)
			})

			pane := ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0)
			a.npRise(pane, 2).Children(func() {
				// Lyrics stay near the tabs even when a tall window centers the cover lower.
				ui.Box(c).Height(npCoverTop)
				switch a.npPane {
				case paneLyrics:
					a.lyricsPane(c, tr, h)
				case paneComments:
					a.commentsPane(c)
				case paneProgram:
					a.programPane(c, tr, white)
				case paneCaptions:
					a.captionsPane(c, tr, h)
				default:
					a.queuePane(c)
				}
			})
		})
	}
}

// npMaxWidth is how wide the cover and the pane beside it go, at most.
const npMaxWidth = 1280

// npSideX is how far in from the sides of a view w wide the cover and the
// pane beside it are: in a wide window they keep together in the middle,
// rather than the pane taking all the width the cover leaves.
func npSideX(w float32) float32 {
	return max(72, (w-npMaxWidth)/2)
}

// npTabsHeight is how tall the row of Now Playing's tabs is, for what is
// under it to start below it.
const npTabsHeight = 28

// Minimum space below the top strip, clear of the tabs.
const npCoverTop = npTabsHeight + 22

// npTabsRow is the row of Now Playing's tabs, in the same place for every
// pane, so that they stay put going from one to another: at the top, level
// with the cover's left. They start from the left, so that only the last,
// the comments, moves as its count changes.
func npTabsRow(c *ui.Context, w float32) ui.Element {
	return ui.Row(c.Key("tabs")).Absolute().Top(topBar(c)).Left(npSideX(w)).Height(npTabsHeight).Gap(6).AlignItems(ui.Center)
}

// reveal fades e in while the pointer is in the window, and out when it
// leaves.
func reveal(e ui.Element, shown bool) ui.Element {
	pose := ui.Pose{Fade: 1}
	if shown {
		pose = ui.Pose{}
	}
	return e.AnimatePose(pose, ui.Animation{Duration: 200 * time.Millisecond})
}

// lyricsPaneChosen is the pane of lyrics the listener chose last: what
// the app starts on, before the player is made. Now Playing shows an
// episode's in place of either.
func (a *app) lyricsPaneChosen() int {
	if a.settings.LyricsStage {
		return paneStage
	}
	return paneLyrics
}

// The panes of Now Playing.
const (
	paneLyrics = iota
	paneQueue
	paneStage
	paneComments
	paneProgram
	paneCaptions
	paneWall
)

// npTab is a tab of Now Playing: its pane, and what it says.
type npTab struct {
	pane  int
	label string
}

// npTabs returns the tabs of Now Playing, in order, for what plays: ⌘1
// chooses the first, ⌘2 the second, and on.
func (a *app) npTabs() []npTab {
	if tr := a.pl.Current(); tr != nil && tr.ProgramID != 0 {
		tabs := []npTab{{paneProgram, "简介"}}
		if a.captionsShown() {
			tabs = append(tabs, npTab{paneCaptions, "字幕"})
		}
		return append(tabs, npTab{paneQueue, "待播清单"}, npTab{paneComments, "评论"})
	}
	tabs := []npTab{{paneLyrics, "歌词"}, {paneStage, "律动"}, {paneQueue, "待播清单"}, {paneWall, "选歌"}}
	if tr := a.pl.Current(); tr != nil && tr.LocalPath == "" {
		tabs = append(tabs, npTab{paneComments, "评论"})
	}
	return tabs
}

func (a *app) paneTabs(c *ui.Context) {
	for _, t := range a.npTabs() {
		label := t.label
		if t.pane == paneComments {
			label = a.commentsTab()
		}
		a.paneTab(c, t.pane, label)
	}
}

// choosePane shows pane i in Now Playing, remembering lyrics or the stage,
// whichever was chosen, for the next song.
func (a *app) choosePane(i int) {
	a.npPane = i
	if a.phone {
		a.phonePaneAnim = true
	}
	if (i == paneLyrics || i == paneStage) && a.settings.LyricsStage != (i == paneStage) {
		a.settings.LyricsStage = i == paneStage
		a.settings.save()
	}
}

// radioChip says which radio plays, over the title: a click on it, or on
// its cross, leaves the radio, playing on the song.
func (a *app) radioChip(c *ui.Context, name string) {
	icon, exit := iconRadio, "退出私人 FM"
	if name == heartRadio {
		icon, exit = iconHeartFill, "退出心动模式"
	}
	chip := ui.ButtonBase(c.Key("radio-chip")).Gap(5).Padding(3, 6, 3, 8).Margin(0, 0, 4, -8).Radius(999).
		AlignItems(ui.Center).AlignSelf(ui.Start).FocusRing(false).Label(exit).Tooltip(exit)
	bg, fg, cross := ui.Transparent, white.Alpha(0.6), float32(0.5)
	if chip.Hovered() {
		bg, fg, cross = white.Alpha(0.12), white.Alpha(0.85), 1
	}
	chip.Background(chip.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond})).TextColor(fg)
	chip.Children(func() {
		ui.Icon(c, icon).Size(12, 12)
		ui.Text(c, radioLabel(name)).FontSize(11.5).FontWeight(700).LetterSpacing(0.3)
		ui.Icon(c, iconX).Size(11, 11).Opacity(cross)
	})
	if chip.Clicked() {
		a.exitRadio()
	}
}

// qualityBadge marks a lossless stream, as it plays.
func qualityBadge(c *ui.Context, st *netease.Stream) {
	if st == nil || !netease.Lossless(st.Level) {
		return
	}
	label := "无损"
	if st.Level == "hires" {
		label = "Hi-Res"
	}
	ui.Text(c, label).FontSize(9.5).FontWeight(800).LetterSpacing(0.6).TextColor(ui.Hex("#f5d58a")).
		Padding(1, 6).Radius(4).Border(1, ui.Hex("#f5d58a").Alpha(0.55))
}

// stageBar is the slim bar along the foot of the stage: what plays, where
// it is, and the controls.
func (a *app) stageBar(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color) {
	pl := a.pl
	ui.Column(c).Padding(0, 48, 28, 48).Gap(10).Children(func() {
		a.npRise(a.scrubber(c.Key("stage-scrub"), &a.npScrub, white.Alpha(0.18), white.Alpha(0.9), 14, accent.Mix(white, 0.35)), 2)
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				if bmp := bitmap(art); bmp != nil {
					cov := a.npMorph(c, ui.Box(c.Key(tr.ID)).Size(44, 44))
					in := artIn(cov, bmp)
					cov.Children(func() { artImage(c, bmp, a.npRadius(c, 8, 44), in) })
				}
				// All but the cover, which flies to the bar's, fade as the
				// rest of Now Playing does.
				a.npRise(ui.Row(c).Shrink(1).MinWidth(0).Gap(12).AlignItems(ui.Center), 3).Children(func() {
					ui.Column(c).Shrink(1).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, tr.Name).FontSize(14).FontWeight(700).SingleLine()
						a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12).SingleLine()
					})
					a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.6))
					a.songMenu(c.Key("more"), tr, 17, white.Alpha(0.6)).Margin(0, 0, 0, -10)
				})
			})
			a.npRise(ui.Row(c), 3).Children(func() { a.transport(c, white, white.Alpha(0.45), white, 22) })
			a.npRise(ui.Row(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.End), 4).Children(func() {
				ui.Text(c, clock(pl.Position())+" / "+clock(pl.Duration())).FontSize(11.5).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

func (a *app) paneTab(c *ui.Context, i int, label string) {
	b := ui.ButtonBase(c.Key(i)).Padding(6, 14).Radius(999).FocusRing(false).Label(label)
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if a.npPane == i {
		bg, fg = white.Alpha(0.14), white
	} else if b.Hovered() {
		bg = white.Alpha(0.06)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		if i != paneComments {
			ui.Text(c, label).FontSize(13).FontWeight(600).TextColor(fg)
			return
		}
		// The count rolls as it changes, from one song's to the next.
		n, text := a.commentCount(), ""
		if n > 0 {
			text = " " + count(n)
		}
		ui.Text(c, "评论").FontSize(13).FontWeight(600).TextColor(fg)
		rollCount(c, &a.tabCount, n, text, fg)
	})
	if b.Clicked() {
		a.choosePane(i)
	}
}

// glideRow keys a phone lyric row's wrapper, which carries the glide's
// pose where the row itself already carries its scale's.
type glideRow int

type lyricPaneState struct {
	lyricScroll   ui.ScrollState
	lyricGlide    lyricGlide
	lyricAt       int
	lyricTrack    int64
	lyricList     ui.ListState
	lyricListJump bool
	lyricViewport ui.Point
	lyricWindow   ui.Point
	linesFirst    *lyrics.Line
	linesCount    int
	lang          string
	// Manual browsing suspends follow until the listener chooses a line or stops scrolling.
	seeking   bool
	seekStamp time.Time
	seekY     float32
}

// lyricClarityButton toggles the blur of the lyrics not sung, floating
// over the title as a capsule that says which look they have.
func (a *app) lyricClarityButton(c *ui.Context) ui.Element {
	blur := a.settings.LyricsBlur
	label := "清晰"
	if blur {
		label = "模糊"
	}
	button := ui.ButtonBase(c).Bind(&a.lyricBlurSwitch).Padding(4, 10).Radius(999).
		Role(ui.RoleSwitch).Checked(blur).Label("歌词模糊").Tooltip("歌词模糊")
	bg, fg := white.Alpha(0.07), white.Alpha(0.6)
	if button.Hovered() {
		bg, fg = white.Alpha(0.14), white.Alpha(0.9)
	}
	button.Background(button.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	button.Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(fg)
	})
	button.OnClick(func() {
		a.settings.LyricsBlur = !a.settings.LyricsBlur
		a.settings.save()
	})
	return button
}

// coverTransitionButton toggles the dissolve from one song's cover into the
// next's, beside the lyric clarity switch.
func (a *app) coverTransitionButton(c *ui.Context) ui.Element {
	on := a.settings.CoverTransition
	label := "硬切"
	if on {
		label = "淡入"
	}
	button := ui.ButtonBase(c).Bind(&a.coverTransitionSwitch).Padding(4, 10).Radius(999).
		Role(ui.RoleSwitch).Checked(on).Label("封面过渡").Tooltip("封面过渡")
	bg, fg := white.Alpha(0.07), white.Alpha(0.6)
	if button.Hovered() {
		bg, fg = white.Alpha(0.14), white.Alpha(0.9)
	}
	button.Background(button.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	button.Children(func() {
		ui.Text(c, label).FontSize(11.5).FontWeight(600).TextColor(fg)
	})
	button.OnClick(func() {
		a.settings.CoverTransition = !a.settings.CoverTransition
		a.settings.save()
	})
	return button
}

func (a *app) lyricsPane(c *ui.Context, tr *netease.Track, h float32) {
	a.lyricsPaneWithState(c, tr, h, &a.lyricPaneState, true)
}

func (a *app) lyricsPaneWithState(c *ui.Context, tr *netease.Track, h float32, state *lyricPaneState, comments bool) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	if loaded && len(lines) == 0 && tr.LocalLyrics != "" {
		ui.Scroll(c.Key("plain-lyrics").Key(tr.ID)).Grow(1).Padding(24, 8).Children(func() {
			ui.Text(c, tr.LocalLyrics).FontSize(24).FontWeight(600).LineHeight(1.8).TextColor(white.Alpha(0.75))
		})
		return
	}
	if len(lines) == 0 {
		col := ui.Column(c).Grow(1)
		if loaded {
			col.Center()
		}
		var dim float32
		if comments {
			dim, _ = a.quietShown(c, col, tr, lines, loaded)
		}
		quietUnder(col, dim)
		col.Children(func() {
			if !loaded {
				lyricsSkeleton(c, npShimmer(c))
				return
			}
			ui.Text(c, "纯音乐，请欣赏").FontSize(24).FontWeight(700).TextColor(white.Alpha(0.4))
		})
		if comments {
			a.quietCard(c, dim, h*0.18)
		}
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Redraw as the next line begins.
	if pl.Playing() && cur+1 < len(lines) && !a.phonePlayerMoving {
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	if state.lyricTrack != tr.ID {
		*state = lyricPaneState{lyricTrack: tr.ID, lyricAt: -2}
	}
	if state.lyricListJump {
		// A jump changes the list's offset as it measures the new rows.
		state.lyricGlide.set = state.lyricScroll.Y
		state.lyricListJump = false
	}
	following := state.lyricGlide.pending
	lag, took := state.lyricGlide.step(c, &state.lyricScroll, a.phone)
	if a.phone {
		if took {
			state.seeking, state.seekStamp, state.seekY = true, c.Now(), state.lyricScroll.Y
		}
		a.phoneLyricSeekWatch(c, state)
	}

	var scroll ui.Element
	if a.phone {
		scroll = ui.Column(c.Key("lyrics").Key(tr.ID)).Grow(1).MinHeight(0)
	} else {
		scroll = ui.Scroll(c.Key("lyrics")).Grow(1).TrackScroll(&state.lyricScroll).Padding(0, 8, h*0.5, 0)
	}
	if !a.phone && !c.Preferences().ReduceMotion {
		scroll.Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Enter: &ui.Motion{Y: 12}})
	}
	var dim float32
	var quiet bool
	if comments {
		dim, quiet = a.quietShown(c, scroll, tr, lines, loaded)
		defer a.quietCard(c, dim, h*0.18)
	}
	if dim > 0.98 {
		// Under the hot comment, gone, not drawn faint.
		scroll.Invisible()
	}
	size := float32(30)
	if a.phone {
		w, height := c.Size()
		top, right, bottom, left := phoneSafeArea(c)
		height -= top + bottom
		if phonePlayerIsWide(w-left-right, height) {
			size = clampL((height-64)*0.09, 24, size)
		}
	}
	a.timedLines(c, scroll, lines, pos, cur, lag, linesStyle{size: size, weight: 800, lineHeight: 1.25, quiet: quiet, state: state, depth: true, virtual: a.phone, listPadding: h * 0.5}, nil)
	if a.phone && !c.Preferences().ReduceMotion {
		ready := ui.Local(scroll, "lyrics-ready", func() bool { return false })
		if !*ready && scroll.LayoutBounds().H > 0 && (state.seeking || state.lyricAt == cur) && !state.lyricListJump && !state.lyricGlide.pending {
			*ready = true
		}
		fade := float32(1)
		if *ready {
			fade = 0
		} else {
			scroll.IgnoreInput(true).AccessHidden(true)
		}
		scroll.AnimatePose(ui.Pose{Fade: fade}, ui.Animation{Duration: 120 * time.Millisecond})
	}
	if a.phone && following {
		state.lyricListJump = true
	}
	if !a.phone && state == &a.lyricPaneState && dim < .98 {
		w, _ := c.Size()
		ui.Box(c.Key("lyric-top-fade")).Absolute().Top(npCoverTop).Left(0).Right(0).Height(36).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			a.backdropOver(p, ui.Rect{W: w, H: h}, r, func(t float32) float32 { return 1 - smoothstep(0, .9, t) })
		})
		ui.Box(c.Key("lyric-bottom-fade")).Absolute().Bottom(0).Left(0).Right(0).Height(70).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			a.backdropOver(p, ui.Rect{W: w, H: h}, r, func(t float32) float32 { return smoothstep(0, 1, t) })
		})

	}
}

// phoneLyricSeekWatch keeps the lyrics unfollowed while the listener
// holds the scroll, then sends them back to the line sung after a few
// seconds at rest.
func (a *app) phoneLyricSeekWatch(c *ui.Context, state *lyricPaneState) {
	if !state.seeking {
		return
	}
	if state.lyricScroll.Y != state.seekY {
		state.seekStamp, state.seekY = c.Now(), state.lyricScroll.Y
	}
	if left := 3*time.Second - c.Now().Sub(state.seekStamp); left > 0 {
		c.After(left)
	} else {
		state.seeking, state.lyricAt = false, -2
	}
}

// linesStyle is how timedLines sets its lines: the size, weight and
// leading of their type; quiet nearly hides them, under a hot comment.
type linesStyle struct {
	state       *lyricPaneState
	size        float32
	weight      int
	lineHeight  float32
	virtual     bool
	listPadding float32 // space below the last lyric, inside the phone list
	quiet       bool
	depth       bool // lyric depth must not change spoken podcast captions.
}

// timedLines fills scroll with lines that follow the track playing at
// pos: the line at cur lit, and brought to a third of the way down once
// it changes, but none with cur -1 before the first; a click on one seeks
// to it. The lines are held back by lag, how far the glide to the line
// lit has still to go (lyricGlide). foot, if set, follows the lines.
func (a *app) timedLines(c *ui.Context, scroll ui.Element, lines []lyrics.Line, pos time.Duration, cur int, lag float32, st linesStyle, foot func()) {
	pl := a.pl
	state := st.state
	if state == nil {
		state = &a.lyricPaneState
	}
	var active ui.Element
	// Lyrics and transcript updates publish new slices. Keep the language
	// across frames so scrolling doesn't scan an episode's whole transcript.
	if state.linesFirst != &lines[0] || state.linesCount != len(lines) {
		state.linesFirst, state.linesCount, state.lang = &lines[0], len(lines), lyrics.Lang(lines)
	}
	lang := state.lang
	seeking := a.phone && state.seeking
	virtual := st.virtual
	glide := &state.lyricGlide
	now := c.Now()
	// The build that first sees a new line on the phone holds the rows
	// where the glide leaving has them, unposed, so the next build's
	// poses take their lags at once.
	glideDetect := virtual && a.phone && cur != state.lyricAt && state.lyricAt >= -1 && !seeking && !a.phonePlayerMoving
	if virtual && !seeking && cur >= 0 && cur != state.lyricAt {
		first, last := state.lyricList.Visible()
		if cur < first || cur > last {
			// Start a long seek at the active row without laying out the
			// lines before it. Follow the measured row on the next frame.
			state.lyricList.ScrollTo(cur, ui.Center)
			state.lyricGlide, state.lyricListJump = lyricGlide{}, true
			lag = 0
			c.AnimationFrame()
		}
	}
	render := func(i int) {
		l := lines[i]
		padding := float32(10)
		if a.phone && st.depth && st.size < 30 {
			padding = 7
		}
		line := ui.ButtonBase(c.Key(i)).Column().AlignItems(ui.Start).Padding(padding, 12).Radius(12).Gap(6).FocusRing(false).Label(l.Text)
		on := i == cur
		alpha := float32(0.28)
		if on {
			alpha = 1
		} else if line.Hovered() {
			alpha = 0.55
		}
		if st.quiet {
			// Nearly gone under a hot comment.
			alpha *= quietFaded
		}
		if line.Hovered() || a.phone && line.Pressed() {
			line.Background(white.Alpha(0.06))
		}
		scale := float32(0.94)
		if on {
			scale = 1
		}
		scaleMotion := ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.1}}
		fadeTime := 380 * time.Millisecond
		if a.phone {
			scaleMotion = ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut}
			fadeTime = 220 * time.Millisecond
		}
		line.TransformOrigin(0, 0.5).AnimatePose(ui.Pose{Scale: scale}, scaleMotion)
		line.Children(func() {
			// The line fades as a pose, in frames that only paint, rather
			// than building the view at the display's rate as it does.
			text := ui.Column(c).AlignItems(ui.Start).Gap(6)
			blurEnabled := st.depth && a.settings.LyricsBlur && !c.Preferences().ReduceMotion
			var blur float32
			if blurEnabled && !on && !seeking && !line.Hovered() && !a.phonePlayerMoving {
				distance := i - cur
				if distance < 0 {
					distance = -distance
				}
				if a.phone {
					// Two radii reach furthest: a line change re-renders
					// only the caches that move between them.
					blur = 0.8
					if distance >= 2 {
						blur = 1.8
					}
				} else {
					blur = min(3, float32(distance)*0.65)
				}
			}
			paintWords := func(text ui.Element) {
				// The phone's sung line wipes bright across its words as
				// Apple's do, painted rather than built; a line with
				// words shows them joined, so turning lit doesn't reflow it.
				if a.phone && st.depth {
					switch {
					case on && len(l.Words) > 0:
						a.phoneLyricWords(c, l.Words, st, lang)
					case len(l.Words) > 0:
						ui.Text(c, joinLyricWords(l.Words)).Lang(lang).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
					default:
						ui.Text(c, l.Text).Lang(lang).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
					}
				} else if on && len(l.Words) > 0 {
					// The line sung lights up word by word as it is sung.
					var next time.Duration
					ui.RichText(c.Key("words")).Lang(lang).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).Children(func() {
						for wi, wd := range l.Words {
							span := max(50*time.Millisecond, wd.End-wd.Start)
							elapsed := pos - wd.Start
							lit := clamp01(float32(elapsed) / float32(span))
							ui.Text(c.Key(wi), wd.Text).TextColor(white.Alpha(0.38 + 0.62*lit))
							if elapsed >= span {
								continue
							}
							delay := min(40*time.Millisecond, span-elapsed)
							if elapsed < 0 {
								delay = -elapsed
							}
							if next == 0 || delay < next {
								next = delay
							}
						}
					})
					if pl.Playing() && next > 0 && !st.quiet && !a.phonePlayerMoving {
						c.After(next)
					}
				} else {
					ui.Text(c, l.Text).Lang(lang).FontSize(st.size).FontWeight(st.weight).LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(white)
				}
				if l.Translation != "" {
					ui.Text(c, l.Translation).FontSize(17).FontWeight(600).TextColor(white.Alpha(0.7))
				}
			}
			text.AnimatePose(ui.Pose{Fade: 1 - alpha}, ui.Animation{Duration: fadeTime})
			if a.phone && blurEnabled {
				// Cache the result of the blur, so moving or fading the line
				// can composite it without another GPU blur pass.
				if blur > 0 {
					text.Cache()
				}
				text.Children(func() {
					ink := ui.Column(c).AlignItems(ui.Start).Gap(6)
					if blur > 0 {
						ink.Blur(blur).Cache()
					}
					ink.Children(func() { paintWords(ink) })
				})
			} else {
				if blur > 0 {
					text.Blur(blur).Cache()
				}
				text.Children(func() { paintWords(text) })
			}
		})
		if line.Clicked() {
			if a.phone {
				state.seeking, state.lyricAt = false, -2
				mygo.Haptics.Selection()
			}
			pl.Seek(l.At)
		}
		if on {
			active = line
		}
	}
	returnToCover := func() {
		a.phoneDetails = false
		mygo.Haptics.Selection()
	}
	scroll.Children(func() {
		if virtual {
			scroll = ui.List(c.Key("visible-lines"), &state.lyricList, len(lines)+1).
				Grow(1).MinHeight(0).Gap(6).Padding(0, 8, 0, 0).TrackScroll(&state.lyricScroll).Rows(func(row ui.ListRow) {
				i := row.Index
				if i == len(lines) {
					ui.Column(c.Key("tail")).Children(func() {
						if foot != nil {
							foot()
						}
						ui.ButtonBase(c.Key("tail-space")).Height(st.listPadding).Label("返回封面").OnClick(returnToCover)
					})
					return
				}
				wrap := ui.Column(c.Key(glideRow(i))).AlignItems(ui.Stretch)
				// A new line moves the rows as poses, which paint
				// themselves: the build that sees it holds each where the
				// glide leaving has it, the next holds the lag it owes,
				// and the one after lets it go, staggered down the list.
				switch {
				case glideDetect:
					wrap.Translate(0, -float32(glide.rowLag(i, now)))
				case glide.posing:
					wrap.AnimatePose(ui.Pose{Y: -float32(glide.rowAmp(i))}, ui.Animation{Spring: lyricRowSpring})
				case glide.on:
					wrap.AnimatePose(ui.Pose{}, ui.Animation{Spring: lyricRowSpring, Delay: glide.delays[i]})
				}
				wrap.Children(func() { render(i) })
			})
			return
		}
		ui.Column(c).Gap(6).Translate(0, -lag).Children(func() {
			for i := range lines {
				render(i)
			}
		})
		if foot != nil {
			foot()
		}
	})
	if virtual && state.lyricListJump {
		return
	}
	if a.phone {
		w, h := c.Size()
		top, right, bottom, left := phoneSafeArea(c)
		window := ui.Point{X: w - left - right, Y: h - top - bottom}
		if window != state.lyricWindow {
			state.lyricWindow = window
			state.lyricAt = -2
			// Bounds still describe the layout before the resize.
			c.AnimationFrame()
			return
		}
	}
	// Reflow can move the active line even while the playback position is fixed.
	box := scroll.LayoutBounds()
	viewport := ui.Point{X: box.W, Y: box.H}
	if (cur != state.lyricAt || (a.phone && viewport != state.lyricViewport)) && !seeking {
		target := float32(0)
		if active.Valid() {
			// Where the glide is going, without where it still is.
			lb := active.LayoutBounds()
			if lb.H == 0 || box.H == 0 {
				c.AnimationFrame()
				return
			}
			top := box.H * 0.3
			if a.phone {
				top = min(top, max(0, box.H-lb.H))
			}
			target = lb.Y - box.Y + state.lyricScroll.Y - top
		}
		animate := !a.phone || state.lyricAt >= -1 && viewport == state.lyricViewport && !a.phonePlayerMoving
		state.lyricAt, state.lyricViewport = cur, viewport
		first, last := state.lyricList.Visible()
		state.lyricGlide.to(c, &state.lyricScroll, min(max(0, target), state.lyricScroll.MaxY), animate, a.phone, cur, first, last)
	}
}

// lyricGlide scrolls the lyrics to the line sung, as a critically damped
// spring, as ScrollTo's would. ScrollTo moves the offset in every frame,
// and MyGo builds the view again in the frame after each that moved it,
// and its spring creeps over the last pixel for half a second: with a line
// following another every second or two, most of a song. So the offset
// goes to the line at once, in one frame, and the lines are held back by
// what the glide has still to go, its lag, which settles to nothing within
// half a pixel.
type lyricGlide struct {
	on bool
	// The lag t after start: (a + b·t)·e^(−ωt), in DIPs.
	a, b  float64
	omega float64
	start time.Time
	// The offset it goes to, set in the next frame (pending), with the lag
	// that holds the lines back from it: the layout of the frame that
	// chose it takes the offset too, the lines built without the lag.
	target  float32
	pending bool
	set     float32 // the offset it set
	// On the phone the rows glide as poses instead: amps is each visible
	// row's starting lag and delays its stagger, base the lag of a row
	// with neither, cur the line the glide goes to, start when they were
	// let go, and posing the build that holds them at their lags.
	amps   map[int]float64
	delays map[int]time.Duration
	base   float64
	cur    int
	posing bool
}

// lyricOmega is the spring's ω: a line or two away, it comes within half a
// pixel in about 800 ms, which is the budget lyricScrollTime gives it.
const lyricOmega = 10

// lyricScrollTime is how long the lyric glide may take to come to rest on
// the line being sung.
const lyricScrollTime = 800 * time.Millisecond

// at returns the lag at now, and how fast it changes, in DIPs a second.
func (g *lyricGlide) at(now time.Time) (lag, v float64) {
	if !g.on {
		return 0, 0
	}
	t := now.Sub(g.start).Seconds()
	e := math.Exp(-g.omega * t)
	return (g.a + g.b*t) * e, (g.b - g.omega*(g.a+g.b*t)) * e
}

// to scrolls s to target from the next frame, gliding from where it
// shows, at the speed it went if it was on its way.
func (g *lyricGlide) to(c *ui.Context, s *ui.ScrollState, target float32, animate, phone bool, cur, first, last int) {
	now := c.Now()
	if phone {
		g.toPhone(c, s, target, animate, cur, first, last, now)
		return
	}
	lag, v := g.at(now)
	shown := float64(s.Y) + lag
	g.target, g.pending = target, true
	g.on = animate && !c.Preferences().ReduceMotion && math.Abs(shown-float64(target)) >= 0.5
	g.start, g.a = now, shown-float64(target)
	g.omega = lyricOmega
	g.b = v + g.omega*g.a
	c.AnimationFrame()
}

// toPhone starts a phone glide: the offset jumps to target in the next
// frame, and each visible row keeps where it showed as a pose let go a
// stagger later, the next row first when the glide moves down the lines.
func (g *lyricGlide) toPhone(c *ui.Context, s *ui.ScrollState, target float32, animate bool, cur, first, last int, now time.Time) {
	// The rows hold where the glide already on the way has them.
	oldLag := func(i int) float64 { return g.rowLag(i, now) }
	shown := float64(s.Y) + oldLag(cur)
	amps := map[int]float64{}
	delays := map[int]time.Duration{}
	for i := max(0, first-2); i <= last+2; i++ {
		amps[i] = float64(s.Y) + oldLag(i) - float64(target)
		if target > s.Y {
			delays[i] = time.Duration(min(max(i-(cur-1), 0), 6)) * 35 * time.Millisecond
		}
	}
	g.amps, g.delays, g.cur = amps, delays, cur
	var ok bool
	if g.base, ok = amps[cur]; !ok {
		g.base = float64(s.Y) + oldLag(cur) - float64(target)
	}
	g.target, g.pending = target, true
	g.on = animate && !c.Preferences().ReduceMotion && math.Abs(shown-float64(target)) >= 0.5
	g.start = now
	c.AnimationFrame()
}

// rowLag is where a phone row's pose has it at now: its start lag fallen
// since start, delayed by its stagger, zero once the glide is off.
func (g *lyricGlide) rowLag(i int, now time.Time) float64 {
	if !g.on {
		return 0
	}
	return phoneLag(g.rowAmp(i), now.Sub(g.start)-g.delays[i])
}

func (g *lyricGlide) rowAmp(i int) float64 {
	if a, ok := g.amps[i]; ok {
		return a
	}
	return g.base
}

// settled reports whether every stored row's lag has fallen under half a
// pixel, where the pose springs rest.
func (g *lyricGlide) settled(now time.Time) bool {
	for i, a := range g.amps {
		if math.Abs(phoneLag(a, now.Sub(g.start)-g.delays[i])) >= 0.5 {
			return false
		}
	}
	return math.Abs(phoneLag(g.base, now.Sub(g.start))) < 0.5
}

// step returns the lag of the lines in this frame, and asks for the next
// while there is one, and whether the listener took the scroll over since
// the last frame. The listener scrolling stops the glide where it shows.
func (g *lyricGlide) step(c *ui.Context, s *ui.ScrollState, phone bool) (float32, bool) {
	if g.posing {
		// The rows were posed at their lags in the frame before: they
		// are let go now, and their lags count from here.
		g.posing, g.start = false, c.Now()
	}
	if g.pending {
		s.Y, g.set, g.pending = g.target, g.target, false
		if g.on && phone {
			// The offset jumps: this build's rows take their lags as
			// poses, which the next build lets go.
			g.posing = true
			c.AnimationFrame()
		}
	}
	if !g.on {
		if s.Y != g.set {
			// The listener moved it: report once per change.
			g.set = s.Y
			return 0, true
		}
		return 0, false
	}
	if phone {
		if s.Y != g.set {
			// The listener took over: the offset takes what showed.
			s.Y = max(0, min(s.Y+float32(g.rowLag(g.cur, c.Now())), s.MaxY))
			g.on, g.set = false, s.Y
			return 0, true
		}
		// The poses paint themselves; when whatever build finds them
		// settled, the glide is over.
		if g.settled(c.Now()) {
			g.on = false
		}
		return 0, false
	}
	lag, v := g.at(c.Now())
	if s.Y != g.set {
		// The listener took over: the offset takes what showed.
		s.Y, g.on = max(0, min(s.Y+float32(lag), s.MaxY)), false
		g.set = s.Y
		return 0, true
	}
	if math.Abs(lag) < 0.5 && math.Abs(v) < 20 {
		g.on = false
		return 0, false
	}
	c.AnimationFrame()
	return float32(lag), false
}

// followQueueRow starts at the current row, then scrolls only as far as
// needed to follow a change in playback.
func (a *app) followQueueRow(list *ui.ListState, row int) {
	if row >= 0 && row != a.queueAt {
		if a.queueAt < 0 {
			list.ScrollTo(row, ui.Start)
		} else {
			list.ScrollIntoView(row)
		}
		a.queueAt = row
	}
}

// queuePane lists the whole queue in play order, the track playing marked
// among the others: a click plays one there, and the list stays as it is.
func (a *app) queuePane(c *ui.Context) {
	pl := a.pl
	order, at := pl.PlayOrder()
	a.followQueueRow(&a.queueList, at)
	a.queueList.Key = func(row int) any { return order[row] }
	ui.List(c.Key("queue"), &a.queueList, len(order), func(r int) {
		i := order[r]
		tr := &pl.Queue()[i]
		playing := r == at
		row := ui.ButtonBase(c.Key(i)).Height(58).PaddingX(10).Gap(14).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
		if !a.phone {
			row.ContextMenu(a.songItems(tr, 0))
		}
		switch {
		case playing:
			row.Background(white.Alpha(0.12))
		case row.Hovered():
			row.Background(white.Alpha(0.08))
		}
		if !tr.Playable() {
			row.Opacity(0.4)
		}
		row.Children(func() {
			a.cover(c, tr.Album.PicURL, 96, 42, 7)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, tr.Name).FontSize(14).FontWeight(600).SingleLine()
				a.artistLinks(c, tr, white.Alpha(0.55), white).FontSize(12.5).SingleLine()
			})
			if a.phone {
				a.phoneTrackActions(c.Key("queue-actions"), tr, 0, white.Alpha(0.7))
			} else {
				ui.Text(c, clock(tr.Length())).FontSize(12).FontFeatures("tnum").TextColor(white.Alpha(0.4))
			}
		})
		if row.Clicked() && !playing {
			pl.Jump(i)
		}
	}).Grow(1).Padding(8, 0, 40, 0).Gap(2)
}
