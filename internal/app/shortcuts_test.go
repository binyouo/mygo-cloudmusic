package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A failed global registration leaves the other commands usable; retry
// releases only this app's successful bindings, and shutdown releases them.
func TestPlaybackGlobalShortcuts(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS global shortcuts")
	}
	register, unregister := registerGlobalShortcut, unregisterGlobalShortcut
	defer func() { registerGlobalShortcut, unregisterGlobalShortcut = register, unregister }()
	callbacks := map[string]func(){}
	blocked := true
	registerGlobalShortcut = func(key string, fn func()) error {
		if key == "Alt+Command+Right" && blocked {
			return errors.New("taken")
		}
		if callbacks[key] != nil {
			t.Fatalf("registered twice: %s", key)
		}
		callbacks[key] = fn
		return nil
	}
	unregisterGlobalShortcut = func(key string) {
		if callbacks[key] == nil {
			t.Fatalf("released a key we did not own: %s", key)
		}
		delete(callbacks, key)
	}
	a := newTestApp()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "本地测试", Duration: 200000}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()
	a.registerPlaybackShortcuts()
	if len(callbacks) != 2 || len(a.shortcutErrors) != 1 {
		t.Fatalf("registered %d, failures %v", len(callbacks), a.shortcutErrors)
	}
	callbacks["Alt+Command+1"]()
	if a.pl.Playing() {
		t.Fatal("global shortcut did not pause")
	}
	callbacks["Alt+Command+1"]()
	if !a.pl.Playing() {
		t.Fatal("global shortcut did not resume")
	}
	blocked = false
	a.registerPlaybackShortcuts()
	if len(callbacks) != 3 || len(a.shortcutErrors) != 0 {
		t.Fatalf("retry: registered %d, failures %v", len(callbacks), a.shortcutErrors)
	}
	a.unregisterPlaybackShortcuts()
	if len(callbacks) != 0 {
		t.Fatal("global shortcuts leaked after unregister")
	}
}

// Cmd-Left reaches the player's previous action: after three seconds it
// returns to the beginning, matching the previous button without loading.
func TestPreviousKeyboardShortcut(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "本地测试", Duration: 200000}})
	a.pl.Hold(10 * time.Second)
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Key(ui.Cmd, ui.KeyLeft)
	tt.Frame()
	if a.pl.Position() != 0 || a.pl.Current().ID != 1 {
		t.Fatalf("previous shortcut: position %v, track %+v", a.pl.Position(), a.pl.Current())
	}
}

// Recording a shortcut changes playback, persists it, and Escape leaves it unchanged.
func TestRecordPlaybackShortcut(t *testing.T) {
	register, unregister := registerGlobalShortcut, unregisterGlobalShortcut
	defer func() { registerGlobalShortcut, unregisterGlobalShortcut = register, unregister }()
	registerGlobalShortcut = func(string, func()) error { return nil }
	unregisterGlobalShortcut = func(string) {}
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.pl.Cue([]netease.Track{{ID: 1, Name: "测试", Duration: 200000}})
	a.pl.Hold(10 * time.Second)
	tt := ui.NewTester(a.settingsView, 800, 1800)
	tt.Frame()
	if err := tt.Click("上一首应用内快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	tt.Frame()
	want := shortcutKey{ui.KeyP, ui.Ctrl | ui.Shift}
	if got := a.shortcutConfig().Local[1]; got != want {
		t.Fatalf("shortcut=%+v, want %+v", got, want)
	}
	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Shortcuts == nil || saved.Shortcuts.Local[1] != want {
		t.Fatalf("saved shortcut=%+v", saved.Shortcuts)
	}
	main := ui.NewTester(a.view, 1200, 800)
	main.Key(ui.Ctrl|ui.Shift, ui.KeyP)
	main.Frame()
	if a.pl.Position() != 0 {
		t.Fatal("custom shortcut did not reach previous action")
	}
	if err := tt.Click("上一首应用内快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.shortcutEdit.active || a.shortcutConfig().Local[1] != want {
		t.Fatal("Escape did not cancel recording")
	}
}

// A duplicate or unavailable global shortcut leaves the previous binding intact.
func TestRecordGlobalShortcutConflict(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS global shortcuts")
	}
	register, unregister := registerGlobalShortcut, unregisterGlobalShortcut
	defer func() { registerGlobalShortcut, unregisterGlobalShortcut = register, unregister }()
	registerGlobalShortcut = func(key string, _ func()) error {
		if key == "Command+P" {
			return errors.New("taken")
		}
		return nil
	}
	unregisterGlobalShortcut = func(string) {}
	a := newTestApp()
	a.settings = defaultSettings()
	a.registerPlaybackShortcuts()
	old := a.shortcutConfig().Global[1]
	a.beginShortcutEdit(1, true)
	a.setShortcut(1, true, a.shortcutConfig().Global[0])
	if !a.shortcutEdit.active || a.shortcutConfig().Global[1] != old {
		t.Fatal("duplicate accepted")
	}
	a.setShortcut(1, true, shortcutKey{ui.KeyP, ui.Super})
	if a.shortcutConfig().Global[1] != old || len(a.globalKeys) != 3 || a.shortcutEdit.message == "" {
		t.Fatalf("failed registration lost old binding: %+v", a.shortcutConfig())
	}
}
