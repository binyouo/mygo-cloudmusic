package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type wallpaperState struct {
	active       bool
	playing      bool
	closing      bool
	revealed     bool
	win          *mygo.Window
	native       *wallpaperNative
	lastInput    time.Time
	timer        *time.Timer
	timerVersion uint64
}

func (a *app) toggleWallpaper() {
	if a.wallpaper.closing {
		return
	}
	if a.wallpaper.active {
		a.leaveWallpaper()
		return
	}
	if a.pl.Current() == nil {
		a.toast = "先选择一首歌曲，再打开屏保"
		return
	}
	w := &a.wallpaper
	w.active = true
	if a.win != nil {
		a.openWallpaperWindow()
	}
	a.resetWallpaperTimer()
}

func (a *app) openWallpaperWindow() {
	w := &a.wallpaper
	bounds := mygo.Screen.DisplayMatching(a.win.Bounds()).Bounds
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: "播放屏保", Hidden: true, Frameless: true,
		DisableResize: true, DisableMove: true, DisableShadow: true,
		X: bounds.X, Y: bounds.Y, Width: bounds.Width, Height: bounds.Height,
		BackgroundColor: "#0b0b0d", Content: ui.View(a.wallpaperView),
	})
	w.win = win
	w.native = newWallpaperNative(win, a.win)
	win.SetOpacity(0)
	win.OnReadyToShow(func() {
		if w.win != win || !w.active || w.closing {
			return
		}
		// The first frame has reached the surface at the final screen size.
		// Only now reveal the window, then let its content follow the backdrop.
		w.revealed = true
		win.Invalidate()
		win.Show()
		w.native.fade(true, nil)
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		a.leaveWallpaper()
	})
}

func (a *app) leaveWallpaper() {
	w := &a.wallpaper
	if !w.active {
		return
	}
	w.active = false
	w.revealed = false
	if w.win != nil {
		w.closing = true
		// Restore the covered window while the fullscreen window fades away.
		a.win.Invalidate()
		w.win.Invalidate()
		w.native.fade(false, a.finishWallpaper)
		return
	}
	a.finishWallpaper()
}

func (a *app) finishWallpaper() {
	w := &a.wallpaper
	win := w.win
	w.win = nil
	if w.native != nil {
		w.native.close()
		w.native = nil
	}
	if win != nil {
		focused := win.IsFocused()
		win.Destroy()
		if focused && !a.quitting {
			a.win.Focus()
		}
	}
	w.active, w.closing, w.revealed = false, false, false
	w.lastInput = time.Now()
	a.resetWallpaperTimer()
	a.redraw()
}

func (a *app) closeNowPlaying() {
	if a.wallpaper.active {
		a.leaveWallpaper()
		return
	}
	a.nowPlaying = false
}

func wallpaperWait(now, last time.Time, seconds int, focused, playing, blocked bool) (time.Duration, bool) {
	if seconds <= 0 || !focused || !playing || blocked {
		return 0, false
	}
	return max(0, time.Duration(seconds)*time.Second-now.Sub(last)), true
}

// Native input idle time is independent of rendering and AppKit's synthetic
// events. The local baseline still protects a fresh focus/resume or exit.
func wallpaperIdleWait(now, last time.Time, idle time.Duration, seconds int, focused, playing, blocked bool) (time.Duration, bool) {
	if idle >= 0 {
		last = maxTime(last, now.Add(-idle))
	}
	return wallpaperWait(now, last, seconds, focused, playing, blocked)
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func (a *app) resetWallpaperTimer() {
	w := &a.wallpaper
	w.timerVersion++
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	if runtime.GOOS != "darwin" || a.win == nil || a.quitting || a.settings.WallpaperIdleSeconds <= 0 || w.active || w.closing {
		return
	}
	now := time.Now()
	if w.lastInput.IsZero() {
		w.lastInput = now
	}
	blocked := w.active || w.closing || a.localImporting || a.login.open || a.naming.open || a.deleting.open
	wait, enabled := wallpaperIdleWait(now, w.lastInput, wallpaperSystemIdle(), a.settings.WallpaperIdleSeconds, a.win.IsFocused(), a.pl.Playing(), blocked)
	if !enabled {
		w.lastInput = now
	}
	// Recheck focus and physical input even when no UI frame or focus callback
	// arrives. This timer never requests a rendered frame while waiting.
	if !enabled || wait > time.Second {
		wait = time.Second
	}
	version := w.timerVersion
	w.timer = time.AfterFunc(wait, func() {
		mygo.RunOnMain(func() {
			if version != w.timerVersion || a.quitting {
				return
			}
			w.timer = nil
			blocked := w.active || w.closing || a.localImporting || a.login.open || a.naming.open || a.deleting.open
			wait, enabled := wallpaperIdleWait(time.Now(), w.lastInput, wallpaperSystemIdle(), a.settings.WallpaperIdleSeconds, a.win.IsFocused(), a.pl.Playing(), blocked)
			if enabled && wait == 0 {
				a.toggleWallpaper()
				a.redraw()
			} else {
				a.resetWallpaperTimer()
			}
		})
	})
}

func (a *app) wallpaperInput() {
	a.wallpaper.lastInput = time.Now()
	if a.wallpaper.timer == nil {
		a.resetWallpaperTimer()
	}
}

func (a *app) wallpaperPlaybackChanged() {
	if a.pl.Current() == nil {
		a.leaveWallpaper()
	}
	playing := a.pl.Playing()
	if playing != a.wallpaper.playing {
		a.wallpaper.playing = playing
		a.wallpaper.lastInput = time.Now()
	}
	a.resetWallpaperTimer()
}

func (a *app) ensureWallpaperTimer() {
	if a.win != nil && a.wallpaper.timer == nil && !a.wallpaper.active && !a.wallpaper.closing && a.settings.WallpaperIdleSeconds > 0 && a.pl.Playing() {
		a.resetWallpaperTimer()
	}
}

func (a *app) watchWallpaper() {
	a.win.OnFocus(func() {
		a.wallpaper.lastInput = time.Now()
		a.resetWallpaperTimer()
	})
	a.win.OnBlur(func() {
		a.resetWallpaperTimer()
	})
	mygo.Screen.OnDisplaysChanged(a.leaveWallpaper)
	a.wallpaperInput()
}

func (a *app) stopWallpaper() {
	a.finishWallpaper()
	a.wallpaper.timerVersion++
	if a.wallpaper.timer != nil {
		a.wallpaper.timer.Stop()
	}
}
