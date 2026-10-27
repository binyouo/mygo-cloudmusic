package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

// The screen saver owns its scroll and backdrop state, so the main player's
// page can keep rendering through the crossfade without sharing layout state.
func (a *app) wallpaperView(c *ui.Context) {
	w := &a.wallpaper
	c.OnShortcut(ui.Cmd, ui.KeyT, a.toggleWallpaper)
	c.OnShortcut(0, ui.KeyEscape, a.leaveWallpaper)
	tr := a.pl.Current()
	if tr == nil {
		ui.Box(c).Fill().Background(ui.Hex("#0b0b0d"))
		return
	}
	width, height := c.Size()
	art := a.art.get(netease.Sized(tr.Album.PicURL, 1000), true)
	if art != nil {
		w.art = art
	}
	tint := ui.RGB(120, 120, 130)
	if w.art != nil && !w.art.failed {
		tint = w.art.tint
	}
	ui.Box(c).Fill().Background(ui.Hex("#0b0b0d")).TextColor(white).Children(func() {
		a.backdropWithState(c, w.art, tint, &w.backdrop)
		// A small, non-spring lift accompanies the content's fade. The canvas
		// already fills the screen, so no window or artwork is enlarged.
		pose := ui.Pose{Fade: 1, Y: 10}
		motion := ui.Animation{Duration: 620 * time.Millisecond, Delay: 90 * time.Millisecond, Ease: ui.CubicBezier(0.22, 1, 0.36, 1)}
		if w.revealed {
			pose = ui.Pose{}
		} else if w.closing {
			pose.Y = -4
			motion = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.EaseOut}
		}
		if c.Preferences().ReduceMotion {
			pose.Y = 0
			motion.Duration, motion.Delay = 160*time.Millisecond, 0
		}
		side := min(width*0.32, height*0.53, float32(500))
		padding := max(float32(48), width*0.075)
		row := ui.Row(c.Key("screen-saver-content")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
			Padding(height*0.10, padding).Gap(width * 0.075).AlignItems(ui.Center)
		row.AnimatePose(pose, motion).Children(func() {
			ui.Column(c.Key("cover")).Width(side).Gap(24).Children(func() {
				ui.Box(c).Size(side, side).Radius(18).Clip().Background(white.Alpha(0.06)).Center().Children(func() {
					if bmp := bitmap(w.art); bmp != nil {
						ui.Image(c, bmp).Size(side, side).Fit(ui.Cover)
					} else {
						ui.Icon(c, iconNote).Size(side*0.25, side*0.25).TextColor(white.Alpha(0.35))
					}
				})
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, tr.Name).FontSize(28).FontWeight(700).LineHeight(1.2)
					ui.Text(c, tr.ArtistNames()).FontSize(18).TextColor(white.Alpha(0.6))
				})
			})
			ui.Column(c.Key("lyrics")).Grow(1).Basis(0).MinWidth(0).Height(height * 0.8).Children(func() {
				a.lyricsPaneWithState(c, tr, height*0.8, &w.lyrics, false)
			})
		})
	})
}
