package app

import "github.com/egoist/mygo/ui"

// The fullscreen window uses the same Now Playing surface and controls.
func (a *app) wallpaperView(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyT, a.toggleWallpaper)
	c.OnShortcut(0, ui.KeyEscape, a.leaveWallpaper)
	local := a.shortcutConfig().Local
	if local[1].Key != ui.KeyUnknown {
		c.OnShortcut(local[1].Mods, local[1].Key, a.pl.Prev)
	}
	if local[2].Key != ui.KeyUnknown {
		c.OnShortcut(local[2].Mods, local[2].Key, a.pl.Next)
	}
	if a.pl.Current() == nil {
		ui.Box(c).Fill().Background(ui.Hex("#0b0b0d"))
		return
	}
	for i, tab := range a.npTabs() {
		c.OnShortcut(ui.Cmd, ui.Key1+ui.Key(i), func() { a.choosePane(tab.pane) })
	}
	root := ui.Box(c).Fill()
	if local[0].Key != ui.KeyUnknown {
		root.OnShortcut(local[0].Mods, local[0].Key, a.pl.TogglePlay)
		c.OnShortcut(local[0].Mods, local[0].Key, a.pl.TogglePlay)
	}
	root.Children(func() { a.nowPlayingSurface(c, true) })
	a.loginDialog(c)
	a.playlistNameDialog(c)
	a.deletePlaylistDialog(c)
	if a.toast != "" {
		c.Toast(a.toast)
		a.toast = ""
	}
}
