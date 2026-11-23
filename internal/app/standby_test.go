package app

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The screensaver group gains a style choice and the standby card's small
// clock, kept in the settings file.
func TestStandbySettings(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	defer func(apply func(mygo.ThemeSource)) { setThemeSource = apply }(setThemeSource)
	setThemeSource = func(mygo.ThemeSource) {}
	tt := ui.NewTester(a.settingsView, 560, 720)
	tt.Frame()

	// The clock row is off while the style is not standby.
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, "显示小时钟"); !ok || !sw.Disabled {
		t.Fatalf("clock switch should be disabled while the style is not standby: %+v", sw)
	}

	if err := tt.Click("待机显示"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.WallpaperStyle != "standby" {
		t.Fatalf("style %q", a.settings.WallpaperStyle)
	}
	if err := tt.Click("显示小时钟"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.StandbyClock {
		t.Fatal("clock switch did not turn off")
	}

	b, err := json.Marshal(a.settings)
	if err != nil {
		t.Fatal(err)
	}
	back := parseSettings(b)
	if back.WallpaperStyle != "standby" || back.StandbyClock {
		t.Fatalf("round-trip: %+v", back)
	}
}
