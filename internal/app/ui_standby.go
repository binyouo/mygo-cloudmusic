package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// standbyView fills the fullscreen window with a single iPhone StandBy-like
// playback card: the cover at its left, the song, its lyric and the slim
// transport at its right, over the same album-color bloom the playing style
// shows. It is a display, not a panel: no tabs, no close button, nothing
// that only appears under the pointer.
func (a *app) standbyView(c *ui.Context) {
	pl := a.pl
	tr := pl.Current()
	w, h := c.Size()
	art := a.art.get(netease.Sized(tr.Album.PicURL, 600), true)
	if tr.Album.PicURL == "" {
		a.npVisual.clearArtwork()
	}
	if a.npVisual.track != tr.ID {
		a.prefetchNowPlaying()
	}
	progress, duration, mixing := pl.TransitionProgress()
	a.npVisual.step(c.Now(), tr.ID, art, c.Preferences().ReduceMotion, progress, duration, mixing, a.settings.CoverTransition)
	// The colors stay the last song's while its cover loads, rather than
	// going gray between the two.
	shown := art
	if art == nil && tr.Album.PicURL != "" {
		shown = a.npVisual.current
	}
	tint := ui.RGB(120, 120, 130)
	if shown != nil && !shown.failed {
		tint = shown.tint
	}
	accent := tint.Mix(white, 0.45)

	root := ui.Box(c.Key("standby")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Background(ui.Hex("#0b0b0d")).TextColor(white)
	root.Children(func() {
		ui.Box(c.Key("bloom")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Children(func() {
			a.backdrop(c, shown, tint)
		})
		// The card rises into place with the window's reveal, as the playing
		// style's content does.
		content := ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().AlignItems(ui.Center)
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
		content.Children(func() { a.standbyCard(c, tr, art, accent, w, h) })
		if a.settings.StandbyClock {
			a.standbyClock(c)
		}
	})
	if root.DoubleClicked() {
		a.leaveWallpaper()
	}
}

// standbyCard is the translucent card the standby display is: cover at the
// left, the song, the lyric and the transport at the right.
func (a *app) standbyCard(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32) {
	pl := a.pl
	const pad = 40
	cardW := min(w*0.72, 1080)
	cardH := min(h*0.68, 560)
	cover := cardH - 2*pad
	ui.Row(c.Key("standby-card")).Width(cardW).Height(cardH).Padding(pad, 44).Gap(44).Radius(26).
		Background(ui.RGBA(18, 14, 18, 0.5)).AlignItems(ui.Center).Children(func() {
		a.npMorph(c, ui.Box(c.Key("cover-morph")).Size(cover, cover).AlignSelf(ui.Start)).Children(func() {
			a.npCover(c, tr, art, cover)
		})
		ui.Column(c).Grow(1).MinWidth(0).AlignSelf(ui.Stretch).Children(func() {
			metadata := ui.Column(c.Key(tr.ID)).Gap(4).MinWidth(0)
			if !c.Preferences().ReduceMotion {
				metadata.Transition(ui.ElementTransition{Duration: 380 * time.Millisecond, Enter: &ui.Motion{Y: 6}, Exit: &ui.Motion{Y: -4}})
			}
			metadata.Children(func() {
				ui.Text(c, tr.Name).FontSize(24).FontWeight(700).LetterSpacing(-0.3).SingleLine()
				a.artistLinks(c, tr, white.Alpha(0.6), white, true).FontSize(15).SingleLine()
			})
			a.standbyLyrics(c, tr)
			ui.Row(c).Gap(18).AlignItems(ui.Center).Justify(ui.Center).Margin(0, 0, 20, 0).Children(func() {
				if iconButton(c.Key("prev"), iconPrev, 22, white, "上一首").Clicked() {
					pl.Prev()
				}
				if playButton(c.Key("play"), pl.Playing(), 30, white).Clicked() {
					pl.TogglePlay()
				}
				if iconButton(c.Key("next"), iconNext, 22, white, "下一首").Clicked() {
					pl.Next()
				}
			})
			a.scrubber(c.Key("standby-scrub"), &a.standbyScrub, white.Alpha(0.18), white.Alpha(0.9), 16, accent.Mix(white, 0.35))
			ui.Row(c).Height(16).AlignItems(ui.Center).Margin(4, 0, 0, 0).Children(func() {
				ui.Text(c, clock(pl.Position())).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
				ui.Spacer(c)
				status := "-" + clock(pl.Duration()-pl.Position())
				if pl.Loading() {
					status = "正在缓冲…"
				}
				ui.Text(c, status).FontSize(11).FontFeatures("tnum").TextColor(white.Alpha(0.5))
			})
		})
	})
}

// standbyLyrics shows the line sung in bold and the next one dimmed in the
// middle of the card, where there are timed lyrics; an episode or a song
// without them leaves the space to the controls instead of a placeholder.
func (a *app) standbyLyrics(c *ui.Context, tr *netease.Track) {
	pl := a.pl
	a.tickTimes(c)
	lines, _ := pl.Lyrics()
	if tr.ProgramID != 0 || len(lines) == 0 {
		ui.Box(c).Grow(1)
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	show := max(cur, 0)
	if pl.Playing() && cur+1 < len(lines) {
		// Redraw as the next line begins.
		c.After(lines[cur+1].At - pos + 20*time.Millisecond)
	}
	ui.Column(c).Grow(1).Justify(ui.Center).Gap(8).Margin(18, 0, 18, 0).Children(func() {
		l := lines[show]
		line := ui.ButtonBase(c.Key("lyric")).Column().AlignItems(ui.Start).FocusRing(false).Label(l.Text)
		line.Children(func() {
			ui.Text(c, l.Text).FontSize(21).FontWeight(700).LineHeight(1.3).TextColor(white)
			if l.Translation != "" {
				ui.Text(c, l.Translation).FontSize(14).FontWeight(600).TextColor(white.Alpha(0.7))
			}
		})
		if line.Clicked() {
			pl.Seek(l.At)
		}
		if show+1 < len(lines) {
			ui.Text(c, lines[show+1].Text).FontSize(15).FontWeight(500).SingleLine().TextColor(white.Alpha(0.45))
		}
	})
}

// standbyClock is the small time at the display's top right, kept while the
// setting shows it.
func (a *app) standbyClock(c *ui.Context) {
	ui.Box(c.Key("standby-clock")).Absolute().Top(18).Right(24).Label("待机时钟").Children(func() {
		ui.Text(c, c.Now().Format("15:04")).FontSize(15).FontFeatures("tnum").TextColor(white.Alpha(0.55))
	})
	// The minute changes without a frame to ask for one.
	next := c.Now().Truncate(time.Minute).Add(time.Minute)
	c.After(next.Sub(c.Now()) + 20*time.Millisecond)
}
