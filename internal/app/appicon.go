package app

import (
	"runtime"

	"netease-music-mygo/internal/appicon"
)

// iconSwitchable is whether the home screen's icon can switch to the dark
// variant: iOS's. Tests flip it to draw the settings row.
var iconSwitchable = runtime.GOOS == "ios"

// The home screen's icon on iOS, which the dark icon's switch drives;
// stubs elsewhere, swapped in tests.
var (
	setAppIcon     = appicon.Set
	currentAppIcon = appicon.Current
)

// loadAppIcon reads which icon the home screen shows, for the settings
// switch.
func (a *app) loadAppIcon() {
	if !iconSwitchable {
		return
	}
	go func() {
		name, err := currentAppIcon()
		a.post(func() {
			if err == nil {
				a.iconDark = name == appicon.Dark
			}
		})
	}()
}

// setIconDark switches the home screen's icon between the app's and the
// dark variant's, and takes the switch back if it does not take.
func (a *app) setIconDark(dark bool) {
	name := ""
	if dark {
		name = appicon.Dark
	}
	go func() {
		if err := setAppIcon(name); err != nil {
			a.post(func() {
				a.iconDark = !dark
				a.toast = "图标切换失败"
			})
		}
	}()
}
