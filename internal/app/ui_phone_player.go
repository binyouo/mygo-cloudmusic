package app

import (
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

type npCoverMatch struct{ track int64 }

func phonePlayerIsWide(w, h float32) bool {
	return w >= 700 || w >= 560 && w > h
}

// phoneMorph is the sheet's grow out of the mini player's capsule into
// the window, and back: k(now) is how far it is, 0 the capsule, 1 the
// window, on the trajectory the sheet's poses follow.
type phoneMorph struct {
	from, to float64
	at, end  time.Time
	spring   ui.SpringOptions
}

var phoneMorphSpring = ui.SpringOptions{Duration: 325 * time.Millisecond, Bounce: 0.2, Precision: 0.002}

// phoneCoverDrop lowers the cover off dead center, as Apple's artwork sits
// nearer the transport than the void-centered spot. A fixed inset keeps the
// flight's landing target still while the sheet opens.
const phoneCoverDrop = 96

// k is how far the sheet is from its capsule to the window at now.
func (m phoneMorph) k(now time.Time) float64 {
	if m.settled(now) {
		return m.to
	}
	return m.from + (m.to-m.from)*m.spring.Progress(now.Sub(m.at))
}

// settled reports whether the sheet's spring has come to rest.
func (m phoneMorph) settled(now time.Time) bool {
	return !now.Before(m.end)
}

// retarget aims the morph at to from where it is now, keeping its
// velocity so a close in the middle of an open does not jump.
func (m *phoneMorph) retarget(to float64, now time.Time) {
	const sample = time.Millisecond
	velocity := (m.k(now.Add(sample)) - m.k(now)) / sample.Seconds()
	m.aim(to, now, float32(velocity))
}

func (m *phoneMorph) aim(to float64, now time.Time, velocity float32) {
	from := float64(clamp01(float32(m.k(now))))
	distance := to - from
	if math.Abs(distance) <= phoneMorphSpring.Precision {
		*m = phoneMorph{to: to}
		return
	}
	spring := phoneMorphSpring
	spring.Velocity = float32(float64(velocity) / distance)
	*m = phoneMorph{from: from, to: to, at: now, end: now.Add(spring.SettleTime()), spring: spring}
}

func phonePullCommits(distance, velocity, travel float32) bool {
	return distance >= 20 && ui.Project(distance, velocity) >= min(120, travel*0.25)
}

func phoneMorphRect(from, to ui.Rect, progress float32) ui.Rect {
	return ui.Rect{
		X: lerp(from.X, to.X, progress), Y: lerp(from.Y, to.Y, progress),
		W: lerp(from.W, to.W, progress), H: lerp(from.H, to.H, progress),
	}
}

func (a *app) phoneMorphTravel(c *ui.Context) float32 {
	if a.phonePlatter.Y > 0 {
		return a.phonePlatter.Y
	}
	_, height := c.Size()
	return max(1, height-phoneMiniSize)
}

func (a *app) phoneNowPlaying(c *ui.Context, tr *netease.Track) bool {
	winW, winH := c.Size()
	a.npT = 1 // The phone opens a full-screen player without the desktop cover morph.
	art, tint := a.prepareNowPlayingArt(c, tr)
	now, reduced := c.Now(), c.Preferences().ReduceMotion
	if a.phoneOpening {
		a.phoneMorph.aim(1, now, 0)
		a.phoneOpening = false
	}
	player := ui.Column(c.Key("phone-playing")).Absolute().Top(0).Left(0).Right(0).Bottom(0).TransformOrigin(0, 0)
	miniPull := a.phoneMiniAxis == phoneAxisY
	if !miniPull && !a.phoneClosing && a.phoneSheet == phoneSheetClosed {
		a.phonePartDrag(c, "surface", player, 0)
	}
	closing := a.phoneClosing
	if closing && a.phoneMorph.to != 0 {
		a.phoneMorph.retarget(0, now)
	}
	if closing {
		if reduced || a.phoneMorph.settled(now) {
			a.closeNowPlaying()
			c.AnimationFrame()
			return true
		}
	}
	if miniPull || closing {
		player.IgnoreInput(true).AccessHidden(true)
	}
	morph := a.phoneMorph
	if !reduced && !morph.settled(now) {
		c.After(morph.end.Sub(now))
	}
	moving := miniPull || a.phonePull.Y > 0 || !reduced && !morph.settled(now)
	a.phonePlayerMoving = moving
	from := a.phonePlatter
	if from.W <= 0 || from.H <= 0 {
		from = ui.Rect{Y: winH, W: winW, H: phoneMiniSize}
	}
	shape := func(r ui.Rect, at time.Time) (box ui.Rect, radius, alpha float32, sheetMoving bool) {
		if reduced {
			return r, 0, 1, false
		}
		k := clamp01(float32(morph.k(at)))
		box = phoneMorphRect(from, r, k)
		radius = lerp(phoneMiniRadius, 44, k) * (1 - smoothstep(0.92, 1, k))
		alpha = smoothstep(0, 0.18, k)
		sheetMoving = !morph.settled(at) || moving
		return
	}
	a.phoneGhostLive = moving && !reduced && a.settings.PlayerStyle == "" && a.phoneMiniArt.W > 0
	player.Children(func() {
		surface := ui.Column(c.Key("player-surface")).Fill().TextColor(white)
		a.phonePlayingContent(c, surface, tr, art, tint, shape, closing)
		a.phoneCoverGhost(c, tr, art, morph)
	})
	return moving
}

func (a *app) phoneMorphArtwork(c *ui.Context, box ui.Element, thumbnail bool) {
	if bounds := box.LayoutBounds(); bounds.W > 0 && bounds.H > 0 {
		a.phoneCoverRect, a.phoneCoverThumbnail = bounds, thumbnail
		if a.phoneGhostLive {
			box.Opacity(0)
		}
		return
	}
	a.phoneCoverRect = ui.Rect{}
	if a.phoneGhostLive {
		box.Opacity(0)
		c.AnimationFrame()
	}
}

func (a *app) phoneThumbBitmap(tr *netease.Track) *ui.Bitmap {
	url := tr.Album.PicURL
	if bmp := bitmap(a.art.get(netease.Sized(url, 120), false)); bmp != nil {
		return bmp
	}
	return bitmap(a.art.art[netease.Sized(url, 600)+"#backdrop"])
}

func (a *app) phoneCoverGhost(c *ui.Context, tr *netease.Track, art *artwork, morph phoneMorph) {
	if !a.phoneGhostLive || a.phoneCoverRect.W <= 0 {
		return
	}
	mini, expanded := a.phoneMiniArt, a.phoneCoverRect
	radius, shadowY, blur, spread, shade := min(18, expanded.W*0.057), float32(20), float32(42), float32(-12), float32(0.52)
	if a.phoneCoverThumbnail {
		radius, shadowY, blur, spread, shade = 8, 6, 14, -5, 0.45
	} else if !a.pl.Playing() {
		expanded.X += expanded.W * 0.05
		expanded.Y += expanded.H * 0.05
		expanded.W *= 0.9
		expanded.H *= 0.9
		radius, shadowY, blur, spread = radius*0.9, shadowY*0.9, blur*0.9, spread*0.9
	}
	bmp := a.playingArtworkStill(art)
	if bmp == nil || a.phoneCoverThumbnail {
		bmp = a.phoneThumbBitmap(tr)
	}
	ui.Box(c.Key("cover-ghost")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		IgnoreInput(true).AccessHidden(true).Draw(func(p *ui.Painter, _ ui.Rect) {
		progress := clamp01(float32(morph.k(p.Now())))
		if progress == 0 {
			return
		}
		box := phoneMorphRect(mini, expanded, progress)
		corner := lerp(7, radius, progress)
		alpha := smoothstep(0, 0.04, progress)
		p.Shadow(box, corner, 0, shadowY*progress, blur*progress, spread*progress, ui.RGBA(0, 0, 0, shade*progress))
		if bmp == nil {
			p.Fill(box, white.Alpha(0.08*alpha), corner)
		}
		p.Clip(box, corner, func() { paintCoverBitmap(p, box, bmp, alpha) })
	})
}

func (a *app) phonePlayingContent(c *ui.Context, player ui.Element, tr *netease.Track, art *artwork, tint ui.Color, shape func(r ui.Rect, now time.Time) (box ui.Rect, radius, alpha float32, moving bool), closing bool) {
	winW, winH := c.Size()
	top, right, bottom, left := phoneSafeArea(c)
	w, h := winW-left-right, winH-top-bottom
	wide := phonePlayerIsWide(w, h)
	accent := tint.Mix(white, 0.45)
	a.drawPlayingBackdrop(c, player, a.npArt, tint, shape)
	isPodcast := tr.ProgramID != 0
	hasText := !isPodcast || a.captionsShown()
	// With room for two columns, keep the panes beside the cover.
	if wide {
		if isPodcast {
			switch a.npPane {
			case paneLyrics, paneStage, paneWall:
				a.npPane = paneProgram
				if a.captionsShown() {
					a.npPane = paneCaptions
				}
			case paneCaptions:
				if !a.captionsShown() {
					a.npPane = paneProgram
				}
			}
		} else {
			switch a.npPane {
			case paneLyrics, paneStage, paneQueue, paneWall:
			case paneComments:
				if tr.LocalPath != "" {
					a.npPane = a.lyricsPaneChosen()
				}
			default:
				a.npPane = a.lyricsPaneChosen()
			}
		}
		// Back upright, the words stay open where there are any.
		if hasText {
			a.phoneDetails = true
		}
	} else if isPodcast {
		if a.npPane != paneCaptions {
			a.npPane = paneCaptions
		}
	} else if a.npPane != paneLyrics && a.npPane != paneStage {
		a.npPane = a.lyricsPaneChosen()
	}
	if a.npPane != paneQueue && a.phoneSheet != phoneSheetQueue {
		a.queueAt = -1
	}
	if a.npPane != paneWall {
		a.wallState.track = 0
	}
	if tr.LocalPath == "" && (a.phoneSheet != phoneSheetComments || a.comments.matchesTrack(tr)) {
		// commentsFor is a no-op once a track's comments load: it kicks
		// the load once a song, so the bottom row's count has one to show.
		// A comments sheet open on another track keeps its own thread.
		a.commentsFor(tr)
	}
	padding := min(28, max(18, w*0.062))
	if wide && h >= 560 {
		padding = min(72, max(28, w*0.057))
	}
	contentWidth := max(0, w-2*padding)
	// Bring the controls in once the sheet reaches them, and fade them
	// early on close before the shrinking edge passes through them.
	player.Children(func() {
		content := ui.Column(c.Key("player-content")).Fill().Padding(top+4, right+padding, bottom+12, left+padding)
		contentPose, contentMotion := ui.Pose{}, ui.Animation{}
		switch {
		case closing:
			start := clamp01(float32(a.phoneMorph.from))
			contentPose = ui.Pose{Y: a.phoneMorphTravel(c)*(1-start) + 8, Fade: 1}
			contentMotion = ui.Animation{Duration: 75 * time.Millisecond, Ease: ui.EaseOut}
		case !a.phoneContentIn:
			contentPose = ui.Pose{Y: 8, Fade: 1}
			a.phoneContentIn = true
			c.AnimationFrame()
		case a.phoneMiniAxis == phoneAxisY || a.phoneSheetAxis == phoneAxisY:
			// The pull's own reveal: the sheet's inside shows only as
			// the card nears the open end, not on a timer.
			k := clamp01(float32(a.phoneMorph.k(c.Now())))
			contentPose = ui.Pose{Y: a.phoneMorphTravel(c) * (1 - k), Fade: 1 - smoothstep(0.35, 0.8, k)}
			contentMotion = ui.Animation{Duration: time.Nanosecond}
		default:
			delay := time.Duration(150 * float64(time.Millisecond) * (1 - a.phoneMorph.from))
			contentMotion = ui.Animation{Duration: 180 * time.Millisecond, Delay: delay, Ease: ui.EaseOut}
		}
		if !c.Preferences().ReduceMotion {
			content.AnimatePose(contentPose, contentMotion)
		}
		content.Children(func() {
			if wide {
				a.phoneWidePlayer(c, tr, art, accent, contentWidth, h)
			} else {
				a.phonePortraitPlayer(c, tr, art, accent, contentWidth, h, hasText)
			}
		})
	})
}

func (a *app) phonePortraitPlayer(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, width, height float32, hasText bool) {
	details := a.phoneDetails && hasText
	ui.Box(c.Key("player-handle")).Height(34).Shrink(0).Center().Children(func() {
		handle := ui.ButtonBase(c.Key("close")).Size(100, 34).Center().Label("关闭正在播放").OnClick(func() {
			// A press the sheet read as a drag is no tap on the handle.
			if a.phoneSheetAxis == 0 && !a.phoneSheetAte {
				a.startPhoneClose()
			}
			a.phoneSheetAte = false
		})
		a.phonePartDrag(c, "handle", handle, 0)
		handle.Children(func() {
			ui.Box(c).Size(36, 4).Radius(2).Background(white.Alpha(0.42))
		})
	})
	if radio := a.pl.Radio(); radio != "" {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
			a.radioChip(c, radio)
		})
	}
	if details {
		a.phonePlayerTitle(c.Key("player-header"), tr, 17, 13, true)
	}

	pane := -1
	if details {
		pane = a.npPane
	}
	media := ui.Column(c.Key("player-media").Key(pane)).Grow(1).MinWidth(0).MinHeight(0)
	if a.phonePaneAnim {
		a.phonePaneAnim = false
		media.Transition(ui.ElementTransition{
			Animation: &ui.Animation{Spring: npSpring},
			Enter:     &ui.Motion{YPercent: 4, Opacity: 0},
			Exit:      &ui.Motion{Opacity: 0},
		})
	}
	if details {
		media.Clip()
	}
	mediaHeight := max(0, height-292)
	if bounds := media.LayoutBounds(); bounds.W > 0 && bounds.H > 0 {
		mediaHeight = bounds.H
	}
	media.Children(func() {
		a.phonePlayerMedia(c, tr, art, accent, width, mediaHeight, details)
	})

	compact := height < 650
	controls := ui.Column(c.Key("player-controls")).Shrink(0).Gap(6)
	// Apple's reveal is staged: the artwork lands first, then the
	// transport and actions cascade in a beat behind it.
	pose, motion := ui.Pose{}, ui.Animation{}
	if !a.phoneControlsIn {
		pose.Fade = 1
		a.phoneControlsIn = true
		c.AnimationFrame()
	} else if a.phoneMiniAxis == phoneAxisY || a.phoneSheetAxis == phoneAxisY {
		// The pull stages the controls in behind the artwork, a beat
		// behind the rest of the sheet's inside.
		k := clamp01(float32(a.phoneMorph.k(c.Now())))
		pose.Fade = 1 - smoothstep(0.55, 0.9, k)
		motion = ui.Animation{Duration: time.Nanosecond}
	} else {
		delay := time.Duration(190 * float64(time.Millisecond) * (1 - a.phoneMorph.from))
		motion = ui.Animation{Duration: 200 * time.Millisecond, Delay: delay, Ease: ui.EaseOut}
	}
	if !c.Preferences().ReduceMotion {
		controls.AnimatePose(pose, motion)
	}
	controls.Children(func() {
		if !details {
			titleSize := float32(25)
			if compact {
				titleSize = 22
			}
			a.phonePlayerTitle(c.Key("cover-title"), tr, titleSize, 16, false)
		}
		a.phoneTimeline(c.Key("timeline"), tr, accent)
		a.phoneTransport(c.Key("transport"), compact)
		a.phonePlayerActions(c, tr, hasText)
	})
}

func (a *app) phonePlayerTitle(c *ui.Context, tr *netease.Track, titleSize, subtitleSize float32, thumbnail bool) {
	ui.Row(c).AlignItems(ui.Center).Gap(12).Shrink(0).MinHeight(48).Children(func() {
		if thumbnail {
			ui.ButtonBase(c.Key("to-cover")).Size(44, 44).Shrink(0).Radius(8).Label("返回封面").
				OnClick(func() {
					a.phoneDetails = false
					a.phonePaneAnim = true
				}).Children(func() {
				box := ui.Box(c.Key("header-art")).Fill().IgnoreInput(true).Radius(8).
					Shadow(0, 6, 14, -5, ui.RGBA(0, 0, 0, 0.45)).
					Transition(ui.ElementTransition{
						Match: npCoverMatch{tr.ID}, Fly: true, Animation: &ui.Animation{Spring: npSpring},
					})
				a.phoneMorphArtwork(c, box, true)
				box.Children(func() { a.coverFill(c, tr.Album.PicURL, 120, 8) })
			})
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, tr.Name).FontSize(titleSize).FontWeight(700).SingleLine()
			if status := a.phonePlaybackStatus(tr); status != "" && !thumbnail {
				ui.Text(c, status).FontSize(subtitleSize).TextColor(white.Alpha(0.72)).SingleLine()
			} else {
				a.artistLinks(c, tr, white.Alpha(0.72), white, true).FontSize(subtitleSize).SingleLine()
			}
		})
		ui.Row(c.Key("track-actions")).Shrink(0).Height(44).AlignItems(ui.Center).
			Radius(22).Background(white.Alpha(0.10)).Children(func() {
			ui.Box(c).Size(44, 44).Center().Children(func() {
				a.likeButton(c.Key("like"), tr, 17, white.Alpha(0.92))
			})
			ui.Box(c).Size(1, 18).Background(white.Alpha(0.14))
			ui.ButtonBase(c.Key("more")).Size(44, 44).Radius(22).Center().Label("更多").
				OnClick(func() { a.phoneSheet = phoneSheetOptions }).Children(func() {
				ui.Icon(c, iconMore).Size(18, 18).TextColor(white.Alpha(0.92))
			})
		})
	})
}

// phoneWidePlayer follows Apple's landscape: the artwork owns the left
// column and fills its height, while the right stacks the transport over
// the content pane, with the tabs above both columns.
func (a *app) phoneWidePlayer(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, w, h float32) {
	gutter := min(72, max(20, w*0.055))
	artSide := min(max(200, w*0.40), 440)
	ui.Row(c.Key("pane-tabs")).Height(44).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
		a.paneTabs(c)
		ui.Spacer(c)
		if h < 560 && w >= 650 {
			a.phoneWidePlayerSwitches(c)
		}
		ui.ButtonBase(c.Key("close")).Height(44).MinWidth(48).Radius(22).Center().
			Label("关闭正在播放").OnClick(a.startPhoneClose).Children(func() {
			ui.Text(c, "收起").FontSize(13).FontWeight(600).TextColor(white.Alpha(0.75))
		})
	})
	ui.Row(c.Key("player-columns")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(gutter).Children(func() {
		ui.Column(c.Key("player-art")).Width(artSide).Shrink(0).Children(func() {
			coverBox := ui.Column(c.Key("cover")).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center)
			size := min(artSide, max(0, h-8))
			if bounds := coverBox.LayoutBounds(); bounds.W > 0 && bounds.H > 0 {
				size = min(size, bounds.W, bounds.H)
			}
			coverBox.Children(func() {
				if size > 0 {
					box := ui.Box(c).Size(size, size)
					a.phoneMorphArtwork(c, box, false)
					box.Children(func() { a.npCover(c, tr, art, size) })
				}
			})
		})
		ui.Column(c.Key("player-side")).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			if h >= 560 {
				ui.Row(c.Key("player-switches")).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
					a.phoneWidePlayerSwitches(c)
				})
			}
			titleSize := float32(17)
			if h >= 560 {
				titleSize = 22
			}
			a.phonePlayerTitle(c.Key("wide-title"), tr, titleSize, 13.5, false)
			a.phoneTimeline(c.Key("timeline"), tr, accent)
			pane := ui.Column(c.Key("pane")).Grow(1).MinHeight(0).MinWidth(0).Clip()
			pw, ph := max(0, w-artSide-gutter), max(0, h-64)
			if bounds := pane.LayoutBounds(); bounds.W > 0 && bounds.H > 0 {
				pw, ph = bounds.W, bounds.H
			}
			pane.Children(func() { a.phoneWidePane(c, tr, accent, pw, ph) })
			transportSize := float32(20)
			if h >= 560 {
				transportSize = 27
			}
			a.transport(c, white, white.Alpha(0.45), accent, transportSize)
			if radio := a.pl.Radio(); radio != "" {
				a.radioChip(c, radio)
			}
		})
	})
}

func (a *app) phoneWidePlayerSwitches(c *ui.Context) {
	if a.npPane == paneLyrics {
		a.lyricClarityButton(c.Key("lyric-clarity"))
	}
	a.coverTransitionButton(c.Key("cover-transition"))
}

func (a *app) phoneWidePane(c *ui.Context, tr *netease.Track, accent ui.Color, w, h float32) {
	switch a.npPane {
	case paneStage:
		a.stageIn(c, tr, accent, w, h)
	case paneQueue:
		a.queuePane(c)
	case paneWall:
		a.wall(c, 0, w, h, 8, 0, 0)
	case paneComments:
		if a.comments != nil {
			a.commentsPane(c)
		}
	case paneProgram:
		a.programPane(c, tr, white)
	case paneCaptions:
		a.captionsPane(c, tr, h)
	default:
		a.lyricsPane(c, tr, h)
	}
}

func (a *app) phonePlayerMedia(c *ui.Context, tr *netease.Track, art *artwork, accent ui.Color, width, height float32, details bool) {
	if details && a.npPane == paneStage {
		// Timed lists handle their own taps. The stage has no lyric rows.
		ui.ButtonBase(c.Key("pane-bg")).Absolute().Top(0).Left(0).Right(0).Bottom(0).ZIndex(-1).
			Label("返回封面").OnClick(func() {
			a.phoneDetails = false
			a.phonePaneAnim = true
			mygo.Haptics.Selection()
		})
	}
	if !details {
		// Apple's cover keeps clear margins on every side rather than
		// filling the media area wall to wall.
		size := min(max(0, width-40), max(0, height-24), 400)
		if size > 0 {
			ui.Column(c.Key("cover")).Fill().Center().Padding(phoneCoverDrop, 0, 0, 0).Children(func() {
				box := ui.Box(c).Size(size, size)
				// Lyrics opening flies it into the header's thumbnail, as
				// Apple's artwork shrinks into the title row.
				box.Transition(ui.ElementTransition{
					Match:     npCoverMatch{tr.ID},
					Fly:       true,
					Animation: &ui.Animation{Spring: npSpring},
				})
				a.phoneMorphArtwork(c, box, false)
				box.Children(func() { a.npCover(c, tr, art, size) })
			})
		}
		return
	}
	switch a.npPane {
	case paneLyrics:
		a.lyricsPane(c, tr, height)
	case paneStage:
		a.stageIn(c, tr, accent, width, height)
	case paneCaptions:
		a.captionsPane(c, tr, height)
	}
}

func (a *app) phonePlayerActions(c *ui.Context, tr *netease.Track, hasText bool) {
	ui.Row(c.Key("phone-player-actions")).Height(48).Shrink(0).AlignItems(ui.Center).
		Radius(16).Background(white.Alpha(0.09)).Children(func() {
		if hasText {
			label, name := "歌词", "歌词"
			if tr.ProgramID != 0 {
				label, name = "字幕", "字幕"
			}
			if a.phoneDetails {
				label = "返回封面"
			}
			phonePlayerAction(c.Key("text"), iconLyrics, label, name, "", a.phoneDetails, func() {
				a.phoneDetails = !a.phoneDetails
				a.phonePaneAnim = true
				if tr.ProgramID != 0 {
					a.npPane = paneCaptions
				} else {
					a.npPane = a.lyricsPaneChosen()
				}
			}).Role(ui.RoleToggleButton).Checked(a.phoneDetails)
		}
		if tr.LocalPath == "" {
			value := ""
			if n := a.commentCount(); n > 0 {
				value = count(n)
			}
			phonePlayerAction(c.Key("comments"), iconComment, "评论", "评论", value, false, func() {
				a.openPhoneComments(tr)
			})
		}
		phonePlayerAction(c.Key("queue"), iconQueue, "待播清单", "待播", "", a.phoneSheet == phoneSheetQueue, func() {
			a.phoneSheet = phoneSheetQueue
		})
	})
}

func phonePlayerAction(c *ui.Context, icon *ui.SVG, label, title, value string, selected bool, run func()) ui.Element {
	fg := white.Alpha(0.78)
	b := ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Height(44).Radius(13).
		Margin(0, 2).Center().Label(label).OnClick(run)
	if selected || b.Pressed() {
		b.Background(white.Alpha(0.15))
		fg = white
	}
	b.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).TextColor(fg).Children(func() {
			ui.Icon(c, icon).Size(17, 17).Shrink(0)
			ui.Text(c, title).FontSize(11.5).FontWeight(600).SingleLine()
			if value != "" {
				ui.Text(c, value).FontSize(10).FontFeatures("tnum").TextColor(fg.Alpha(0.72)).SingleLine()
			}
		})
	})
	return b
}

var phoneSkipSpring = ui.Animation{Spring: ui.SpringOptions{Mass: 1, Stiffness: 1311.13, Damping: 72.4189}}

// MediaCoreUI's transport uses separate springs for the two glyphs and
// the layer they share. Use the same curve in both playback directions.
var (
	phoneGlyphIn     = ui.Animation{Spring: ui.SpringOptions{Mass: 2, Stiffness: 300, Damping: 30}}
	phoneGlyphOut    = ui.Animation{Spring: ui.SpringOptions{Mass: 1, Stiffness: 845.74, Damping: 58.1632}}
	phoneGlyphOrigin = ui.Animation{Spring: ui.SpringOptions{Mass: 2, Stiffness: 300, Damping: 50}}
)

func phonePlayPause(c *ui.Context, button ui.Element, playing bool, size float32) {
	previous := ui.Local(button, "playing", func() bool { return playing })
	if *previous != playing {
		*previous = playing
		if !c.Preferences().ReduceMotion {
			button.Jump("origin", 0.92)
		}
	}
	pulse := button.AnimateTo("origin", 1, phoneGlyphOrigin)
	ui.Box(c).Size(size, size).Scale(pulse, pulse).Children(func() {
		for _, glyph := range []struct {
			key     string
			icon    *ui.SVG
			visible bool
		}{{"play-glyph", iconPlay, !playing}, {"pause-glyph", iconPause, playing}} {
			target, spring := float32(0), phoneGlyphOut
			if glyph.visible {
				target, spring = 1, phoneGlyphIn
			}
			shown := button.AnimateTo(glyph.key, target, spring)
			if shown > 0.01 {
				scale := 0.5 + 0.5*shown
				ui.Icon(c.Key(glyph.key), glyph.icon).Size(size, size).Absolute().Top(0).Left(0).
					Scale(scale, scale).Opacity(min(1, shown))
			}
		}
	})
}

func (a *app) phoneTransport(c *ui.Context, compact bool) {
	height, playSize := float32(74), float32(62)
	if compact {
		height, playSize = 60, 52
	}
	ui.Row(c).Height(height).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Gap(18).Children(func() {
		phoneSkipButton(c.Key("prev"), "上一首", height, -1, iconPrev, a.pl.Prev)
		play := ui.ButtonBase(c.Key("play")).Size(playSize, playSize).Radius(playSize / 2).Center().
			Background(white.Alpha(0.96)).TextColor(ui.Hex("#17171b")).Label("播放/暂停").OnClick(func() {
			a.pl.TogglePlay()
			mygo.Haptics.Light()
		})
		phonePressScale(c, play, 0.9)
		play.Children(func() { phonePlayPause(c, play, a.pl.Playing(), playSize*0.59) })
		phoneSkipButton(c.Key("next"), "下一首", height, 1, iconNext, a.pl.Next)
	})
}

func phoneSkipButton(c *ui.Context, label string, height, direction float32, icon *ui.SVG, run func()) {
	b := ui.ButtonBase(c).Size(64, height).Radius(32).Center().Label(label).OnClick(run).TextColor(white)
	pressed := float32(0)
	if b.Pressed() {
		pressed = 1
		b.Background(white.Alpha(0.1))
	}
	press := b.AnimateTo("press", pressed, phoneSkipSpring)
	b.Children(func() {
		size := 1 - 0.12*press
		glyph := ui.Icon(c, icon).Size(28, 28).Scale(size, size)
		if !c.Preferences().ReduceMotion {
			glyph.Translate(direction*4*press, 0)
		}
	})
}

// phonePartDrag reads a drag on a part of the sheet — the handle at
// the top, the cover — as a pull on the sheet itself when it goes down,
// which a release far enough sends home, and on the cover a sideways
// one as a track swipe; slide is how wide that part is, 0 that it does
// not slide. A touch that stays put remains the part's own tap.
func (a *app) phonePartDrag(c *ui.Context, key string, e ui.Element, slide float32) {
	dx, dy, held := e.Dragged()
	vx, vy, ended := e.DragEnded()
	canceled := e.PressCanceled()
	switch {
	case held && !a.phoneClosing && (a.phoneSheetHeld == key || a.phoneSheetHeld == ""):
		if a.phoneSheetHeld != key {
			// A press on this part starts the totals over.
			a.phoneSheetHeld, a.phoneSheetAxis, a.phoneSheetDX, a.phoneSheetDY = key, 0, 0, 0
			a.phoneSheetAte = false
		}
		a.phoneSheetDX += dx
		a.phoneSheetDY += dy
		if a.phoneSheetAxis == 0 {
			if ax, ay := absf(a.phoneSheetDX), absf(a.phoneSheetDY); max(ax, ay) >= 12 {
				if ay > ax {
					a.phoneSheetAxis = phoneAxisY
					a.phoneSheetStart = clamp01(float32(a.phoneMorph.k(c.Now())))
				} else {
					a.phoneSheetAxis = phoneAxisX
				}
			}
		}
		switch a.phoneSheetAxis {
		case phoneAxisY:
			travel := a.phoneMorphTravel(c)
			a.phonePull.Y = clampL(a.phoneSheetDY, 0, travel)
			progress := float64(clamp01(a.phoneSheetStart - a.phonePull.Y/travel))
			if a.phoneMorph.to != progress || !a.phoneMorph.at.IsZero() {
				a.phoneMorph = phoneMorph{to: progress}
				c.AnimationFrame()
			}
		case phoneAxisX:
			if slide > 0 {
				a.phoneCoverOff = ui.Rubberband(a.phoneSheetDX, -slide*0.55, slide*0.55, slide)
			}
		}
	case a.phoneSheetHeld == key:
		switch a.phoneSheetAxis {
		case phoneAxisY:
			travel := a.phoneMorphTravel(c)
			if ended && !canceled && phonePullCommits(a.phonePull.Y, vy, travel) {
				a.phoneClosing = true
				a.phoneMorph.aim(0, c.Now(), -vy/travel)
			} else {
				a.phonePull = ui.Point{}
				a.phoneMorph.aim(1, c.Now(), -vy/travel)
			}
			c.AnimationFrame()
		case phoneAxisX:
			if total := a.phoneSheetDX; ended && !canceled && slide > 0 && (absf(total) >= slide*0.3 || absf(vx) >= 800) {
				if total < 0 {
					a.pl.Next()
				} else {
					a.pl.Prev()
				}
				mygo.Haptics.Selection()
			}
		}
		// The press's own click lands on a later build; a drag's is no tap.
		a.phoneSheetAte = a.phoneSheetAxis != 0
		a.phoneSheetHeld, a.phoneSheetAxis, a.phoneSheetDX, a.phoneSheetDY = "", 0, 0, 0
		a.phoneCoverOff = 0
	}
}

// startPhoneClose asks Now Playing to go: on the phone it shrinks back
// into the mini player's capsule on the morph machinery, which closes it
// at the bottom; elsewhere it closes.
func (a *app) startPhoneClose() {
	if a.phone && a.nowPlaying && a.pl.Current() != nil {
		a.phoneClosing = true
		return
	}
	a.closeNowPlaying()
}
