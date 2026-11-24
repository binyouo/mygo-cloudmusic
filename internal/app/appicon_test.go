package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The 深色图标 switch names the home screen's alternate icon, and back.
func TestPhoneDarkIconSwitch(t *testing.T) {
	was := iconSwitchable
	iconSwitchable = true
	defer func() { iconSwitchable = was }()
	names := make(chan string, 2)
	real := setAppIcon
	setAppIcon = func(name string) error { names <- name; return nil }
	defer func() { setAppIcon = real }()

	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	tt := ui.NewTester(a.settingsView, 430, 1500)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tapPhone(t, tt, "使用深色图标")
	if name := wait(t, names, "the dark icon"); name != "AppIconDark" || !a.iconDark {
		t.Fatalf("icon %q, dark %v", name, a.iconDark)
	}
	tapPhone(t, tt, "使用深色图标")
	if name := wait(t, names, "the default icon"); name != "" || a.iconDark {
		t.Fatalf("icon %q, dark %v", name, a.iconDark)
	}
}
