//go:build !darwin

package app

import "github.com/egoist/mygo"

type wallpaperNative struct{ win *mygo.Window }

func newWallpaperNative(win, main *mygo.Window) *wallpaperNative {
	win.SetBounds(mygo.Screen.DisplayMatching(main.Bounds()).Bounds)
	win.SetAlwaysOnTop(true)
	return &wallpaperNative{win: win}
}

func (n *wallpaperNative) fade(show bool, done func()) {
	if show {
		n.win.SetOpacity(1)
	} else {
		n.win.SetOpacity(0)
	}
	if done != nil {
		done()
	}
}

func (n *wallpaperNative) close() {}

func watchWallpaperInput(func(bool, bool) bool) func() { return func() {} }
