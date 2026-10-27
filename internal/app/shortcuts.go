package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var registerGlobalShortcut = mygo.GlobalShortcut.Register
var unregisterGlobalShortcut = mygo.GlobalShortcut.Unregister

type shortcutKey struct {
	Key  ui.Key       `json:"key"`
	Mods ui.Modifiers `json:"modifiers"`
}
type shortcutConfig struct {
	Local  [3]shortcutKey `json:"local"`
	Global [3]shortcutKey `json:"global"`
}
type shortcutEdit struct {
	active, global, focused bool
	index                   int
	message                 string
}

func defaultShortcutConfig() shortcutConfig {
	return shortcutConfig{
		Local:  [3]shortcutKey{{ui.KeySpace, 0}, {ui.KeyLeft, ui.Cmd}, {ui.KeyRight, ui.Cmd}},
		Global: [3]shortcutKey{{ui.Key1, ui.Alt | ui.Super}, {ui.KeyLeft, ui.Alt | ui.Super}, {ui.KeyRight, ui.Alt | ui.Super}},
	}
}
func (a *app) shortcutConfig() shortcutConfig {
	if a.settings.Shortcuts == nil {
		return defaultShortcutConfig()
	}
	return *a.settings.Shortcuts
}

func keyName(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('A' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0))
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		return fmt.Sprintf("F%d", key-ui.KeyF1+1)
	}
	switch key {
	case ui.KeySpace:
		return "Space"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	case ui.KeyEnter:
		return "Enter"
	case ui.KeyTab:
		return "Tab"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyHome:
		return "Home"
	case ui.KeyEnd:
		return "End"
	case ui.KeyPageUp:
		return "PageUp"
	case ui.KeyPageDown:
		return "PageDown"
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	case ui.KeyComma:
		return ","
	case ui.KeyPeriod:
		return "."
	case ui.KeySlash:
		return "/"
	case ui.KeySemicolon:
		return ";"
	case ui.KeyQuote:
		return "'"
	case ui.KeyBracketLeft:
		return "["
	case ui.KeyBracketRight:
		return "]"
	case ui.KeyBackslash:
		return "\\"
	case ui.KeyBackquote:
		return "`"
	}
	return ""
}
func (k shortcutKey) accelerator() string {
	name := keyName(k.Key)
	if name == "" {
		return ""
	}
	var parts []string
	if k.Mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if k.Mods&ui.Super != 0 {
		parts = append(parts, "Command")
	}
	if k.Mods&ui.Ctrl != 0 {
		parts = append(parts, "Control")
	}
	if k.Mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+")
}
func (k shortcutKey) label() string {
	name := keyName(k.Key)
	if name == "" {
		return "未设置"
	}
	names := map[string]string{"Space": "空格", "Left": "←", "Right": "→", "Up": "↑", "Down": "↓", "Enter": "↩", "Tab": "⇥", "Delete": "⌦", "PageUp": "Page Up", "PageDown": "Page Down"}
	if n := names[name]; n != "" {
		name = n
	}
	var label string
	if k.Mods&ui.Ctrl != 0 {
		label += "⌃"
	}
	if k.Mods&ui.Alt != 0 {
		label += "⌥"
	}
	if k.Mods&ui.Shift != 0 {
		label += "⇧"
	}
	if k.Mods&ui.Super != 0 {
		label += "⌘"
	}
	return label + name
}

type playbackShortcut struct {
	label         string
	local, global shortcutKey
	run           func()
}

func (a *app) playbackShortcuts() []playbackShortcut {
	s := a.shortcutConfig()
	return []playbackShortcut{{"播放/暂停", s.Local[0], s.Global[0], a.pl.TogglePlay}, {"上一首", s.Local[1], s.Global[1], a.pl.Prev}, {"下一首", s.Local[2], s.Global[2], a.pl.Next}}
}
func (a *app) registerPlaybackShortcuts() {
	if runtime.GOOS != "darwin" {
		return
	}
	a.unregisterPlaybackShortcuts()
	a.shortcutErrors = nil
	for _, shortcut := range a.playbackShortcuts() {
		key := shortcut.global.accelerator()
		if key == "" {
			continue
		}
		err := registerGlobalShortcut(key, func() { shortcut.run(); a.redraw() })
		if err != nil {
			a.shortcutErrors = append(a.shortcutErrors, shortcut.label+"（"+shortcut.global.label()+"）注册失败，可能已被其他应用占用")
			continue
		}
		a.globalKeys = append(a.globalKeys, key)
	}
}
func (a *app) unregisterPlaybackShortcuts() {
	for _, key := range a.globalKeys {
		unregisterGlobalShortcut(key)
	}
	a.globalKeys = nil
}

func (a *app) beginShortcutEdit(index int, global bool) {
	if !a.shortcutEdit.active {
		a.unregisterPlaybackShortcuts()
	}
	a.shortcutEdit = shortcutEdit{active: true, index: index, global: global}
}
func (a *app) cancelShortcutEdit() {
	if a.shortcutEdit.active {
		a.shortcutEdit = shortcutEdit{}
		a.registerPlaybackShortcuts()
	}
}
func (a *app) setShortcut(index int, global bool, key shortcutKey) {
	cfg := a.shortcutConfig()
	bindings := &cfg.Local
	if global {
		bindings = &cfg.Global
	}
	if key.Key != ui.KeyUnknown {
		for i, other := range bindings {
			if i != index && other == key {
				a.shortcutEdit.message = "此快捷键已用于其他播放操作"
				return
			}
		}
		if global && key.Mods == 0 {
			a.shortcutEdit.message = "全局快捷键需要组合修饰键"
			return
		}
		otherScope := cfg.Global
		if global {
			otherScope = cfg.Local
		}
		for i, other := range otherScope {
			if i != index && other == key {
				a.shortcutEdit.message = "此快捷键已用于其他播放操作"
				return
			}
		}
		if !global && reservedLocalShortcut(key) {
			a.shortcutEdit.message = "此快捷键已用于其他应用操作"
			return
		}
	}
	old := a.shortcutConfig()
	bindings[index] = key
	a.settings.Shortcuts = &cfg
	a.registerPlaybackShortcuts()
	if global && key.Key != ui.KeyUnknown {
		found := false
		for _, registered := range a.globalKeys {
			if registered == key.accelerator() {
				found = true
			}
		}
		if runtime.GOOS == "darwin" && !found {
			a.settings.Shortcuts = &old
			a.registerPlaybackShortcuts()
			a.shortcutEdit = shortcutEdit{message: "快捷键注册失败，可能已被其他应用占用"}
			return
		}
	}
	a.shortcutEdit = shortcutEdit{}
	a.settings.save()
	if a.win != nil {
		a.setMenu()
		a.redraw()
	}
}

// Keep the app's navigation, window and menu commands reachable.
func reservedLocalShortcut(k shortcutKey) bool {
	if k.Mods != ui.Cmd {
		return false
	}
	switch k.Key {
	case ui.KeyT, ui.KeyO, ui.KeyF, ui.KeyD, ui.KeyL, ui.KeyN, ui.KeyW, ui.KeyQ, ui.KeyComma, ui.KeyBracketLeft, ui.KeyBracketRight, ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5:
		return true
	}
	return false
}
func (a *app) shortcutSettings(c *ui.Context, p palette) {
	edit := a.shortcutEdit
	handleIndex := edit.index
	if edit.global {
		handleIndex += 3
	}
	if edit.active && edit.focused && !a.shortcutHandles[handleIndex].Focused(c) {
		a.cancelShortcutEdit()
		edit = a.shortcutEdit
	}
	if edit.active && a.shortcutHandles[handleIndex].Focused(c) {
		a.shortcutEdit.focused = true
	}
	ui.Column(c).Padding(12, 16).Gap(10).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "功能").Grow(1).Basis(0).FontSize(12).TextColor(p.muted)
			ui.Text(c, "应用内").Width(100).TextAlign(ui.Center).FontSize(12).TextColor(p.muted)
			ui.Text(c, "全局快捷键").Width(120).TextAlign(ui.Center).FontSize(12).TextColor(p.muted)
		})
		for i, shortcut := range a.playbackShortcuts() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, shortcut.label).Grow(1).Basis(0).FontSize(13)
				for col, key := range []shortcutKey{shortcut.local, shortcut.global} {
					global := col == 1
					handle := i + col*3
					editing := a.shortcutEdit.active && a.shortcutEdit.index == i && a.shortcutEdit.global == global
					title := key.label()
					if editing {
						title = "按下快捷键…"
					}
					width := float32(100)
					if global {
						width = 120
					}
					button := ui.ButtonBase(c.Key(fmt.Sprintf("shortcut-%d-%d", i, col))).Width(width).Height(28).Radius(5).PaddingX(6).Center().Background(p.sidebar).Border(1, p.hairline).Label(shortcut.label + map[bool]string{false: "应用内快捷键", true: "全局快捷键"}[global]).Bind(&a.shortcutHandles[handle])
					if editing {
						button.Background(p.hover)
					}
					button.HandleInput(func(ev ui.InputEvent) bool {
						if !editing || ev.Kind != ui.InputKeyDown {
							return false
						}
						if ev.Repeat {
							return true
						}
						if ev.Key == ui.KeyEscape && ev.Mods == 0 {
							a.cancelShortcutEdit()
							return true
						}
						if ev.Key == ui.KeyBackspace && ev.Mods == 0 {
							a.setShortcut(i, global, shortcutKey{})
							return true
						}
						key := ev.Key
						if key == ui.KeyUnknown {
							key = ev.PhysicalKey
						}
						if keyName(key) == "" {
							a.shortcutEdit.message = "不支持这个按键"
							return true
						}
						a.setShortcut(i, global, shortcutKey{key, ev.Mods})
						return true
					}).OnClick(func() { a.beginShortcutEdit(i, global); a.shortcutHandles[handle].Focus(c) })
					button.Children(func() { ui.Text(c, title).FontSize(13).TextColor(p.text) })
				}
			})
		}
		if a.shortcutEdit.active {
			ui.Text(c, "按 Esc 取消，按删除键清除").FontSize(11).TextColor(p.muted)
		}
		if a.shortcutEdit.message != "" {
			ui.Text(c, a.shortcutEdit.message).FontSize(12).TextColor(p.muted)
		}
		for _, message := range a.shortcutErrors {
			ui.Text(c, message).FontSize(12).TextColor(p.muted)
		}
		ui.Row(c).Gap(12).Children(func() {
			ui.Button(c, "恢复默认").OnClick(func() {
				a.cancelShortcutEdit()
				cfg := defaultShortcutConfig()
				a.settings.Shortcuts = &cfg
				a.settings.save()
				a.registerPlaybackShortcuts()
				if a.win != nil {
					a.setMenu()
					a.redraw()
				}
			})
			if len(a.shortcutErrors) > 0 {
				ui.Button(c, "重试注册").OnClick(a.registerPlaybackShortcuts)
			}
		})
	})
}
