package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
)

// switchControl is a settings row's switch: the mobile control on the
// phone, the desktop one elsewhere.
func (a *app) switchControl(c *ui.Context, key string, on *bool, label string) ui.Element {
	if a.phone {
		return mobile.Switch(c.Key(key), on).Label(label)
	}
	return ui.Switch(c.Key(key), on).Label(label)
}

// settingButton is a settings row's button: a plain accent text on the
// phone, the desktop button elsewhere.
func (a *app) settingButton(c *ui.Context, key, label string, disabled bool) ui.Element {
	if a.phone {
		return ui.ButtonBase(c.Key(key)).Padding(6, 4).MinHeight(44).Center().
			Label(label).Disabled(disabled).Children(func() {
			ui.Text(c, label).FontSize(15).TextColor(a.pal.accent).SingleLine()
		})
	}
	return ui.Button(c.Key(key), label).Disabled(disabled)
}

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.phone {
		a.measureCache()
		a.open(route{page: pageSettings})
		return
	}
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.measureCache()
		return
	}
	a.measureCache()
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
		Width:           560,
		Height:          720,
		MinWidth:        480,
		MinHeight:       420,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.cancelShortcutEdit()
		a.settingsWin = nil
		a.syncDock()
	})
	a.syncDock()
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.wallpaperInput()
	a.resetWallpaperTimer()
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	if !a.phone {
		c.Root().Background(p.sidebar)
	}
	if !a.phone && !a.shortcutEdit.active {
		c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	}
	s := &a.settings

	scroll := ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8)
	if a.phone {
		top, bottom := a.insets(c)
		scroll.TrackScroll(a.ps()).Padding(top+pageGap, 16, bottom+16, 16).ScrollbarInsets(top, 0, bottom, 0)
	}
	scroll.Children(func() {
		if a.phone {
			a.startMark(c)
			defer a.endMark(c)
			ui.Column(c).PaddingX(4).Children(func() {
				a.largeTitle(c, "设置")
			})
		}
		group := func(title string, rows func()) {
			if a.phone {
				// Inset-grouped, as iOS settings.
				ui.Text(c, title).FontSize(13).TextColor(p.muted).Margin(20, 16, 6, 16)
				ui.Column(c).Radius(12).Background(p.hover).Dividers(1, p.hairline).Children(rows)
				return
			}
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		settingRow := func(label, desc string, stacked bool, control func()) {
			r := ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48)
			if a.phone {
				r.Padding(10, 16).MinHeight(50)
				stacked = false
			} else if stacked {
				r.Column().AlignItems(ui.Stretch).Gap(7)
			}
			r.Children(func() {
				text := ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3)
				if stacked {
					text.Grow(0)
				}
				text.Children(func() {
					size := float32(13.5)
					if a.phone {
						size = 16
					}
					ui.Text(c, label).FontSize(size).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4).MaxLines(2)
					}
				})
				control()
			})
		}
		row := func(label, desc string, control func()) { settingRow(label, desc, false, control) }

		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if !a.phone && runtime.GOOS == "darwin" {
			group("通用", func() {
				row("应用图标", "显示在菜单栏时，关闭窗口后可从菜单栏图标打开", func() {
					labels := make([]string, len(appIcons))
					chosen := appIcons[0].label
					for i, ic := range appIcons {
						labels[i] = ic.label
						if ic.id == s.AppIcon {
							chosen = ic.label
						}
					}
					if ui.Select(c.Key("app-icon"), &chosen, labels).Label("应用图标").Width(150).Changed() {
						for _, ic := range appIcons {
							if ic.label == chosen {
								s.AppIcon = ic.id
							}
						}
						a.changed()
						a.changeAppIcon()
					}
				})
			})
		}
		if !a.phone && runtime.GOOS == "darwin" {
			group("播放屏保", func() {
				row("闲置后显示屏保", "前台播放时全屏渐显，双击空白处或按 Esc 退出；⌘T 手动开关", func() {
					choices := []string{"关闭", "30 秒", "1 分钟", "2 分钟", "5 分钟", "10 分钟"}
					seconds := []int{0, 30, 60, 120, 300, 600}
					chosen := choices[0]
					for index, value := range seconds {
						if value == s.WallpaperIdleSeconds {
							chosen = choices[index]
						}
					}
					if ui.Select(c.Key("wallpaper-idle"), &chosen, choices).Label("屏保等待时间").Width(115).Changed() {
						for index, label := range choices {
							if chosen == label {
								s.WallpaperIdleSeconds = seconds[index]
							}
						}
						a.changed()
					}
				})
			})
			group("菜单栏", func() {
				row("菜单栏歌词", "在菜单栏显示当前歌词，可切歌、暂停和喜欢歌曲", func() {
					if a.switchControl(c, "menu-bar-lyrics", &s.MenuBarLyrics, "显示菜单栏歌词").Changed() {
						s.save()
						a.syncMenuBar()
					}
				})
				row("展示宽度", "短句居中显示，长句自动滚动", func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						if ui.StepSlider(c.Key("menu-bar-width"), &s.MenuBarWidth, 100, 300, 1).Width(140).Label("歌词展示宽度").Disabled(!s.MenuBarLyrics).Changed() {
							s.save()
							a.syncMenuBar()
						}
						ui.Textf(c, "%.0f", s.MenuBarWidth).FontSize(12).TextColor(p.muted).FontFeatures("tnum").Width(28)
					})
				})
			})
			group("快捷键", func() { a.shortcutSettings(c, p) })
		}

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group("刘海播放器", func() {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if a.switchControl(c, "notch", &s.Notch, "显示刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if a.switchControl(c, "activity", &s.NotchActivity, "播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if a.switchControl(c, "virtual", &s.NotchVirtual, "在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group("播放", func() {
			settingRow("播放页样式", "用专辑封面、旋转的黑胶唱片或磁带展示当前歌曲", false, func() {
				labels := make([]string, len(playerStyles))
				chosen := playerStyles[0].label
				for i, style := range playerStyles {
					labels[i] = style.label
					if style.id == s.PlayerStyle {
						chosen = style.label
					}
				}
				a.settingsChoice(c, "player-style", "播放页样式", chosen, labels, 150, func(chosen string) {
					for _, style := range playerStyles {
						if style.label == chosen {
							s.PlayerStyle = style.id
						}
					}
					s.save()
					a.redraw()
					if s.PlayerStyle != "" {
						prepareRetro()
					}
				})
			})
			settingRow("歌曲过渡", "根据节拍、调性和人声选择衔接，保音高对拍并交接频段；不适合混音时保留自然衔接", false, func() {
				labels := []string{"自动混音（AutoMix）", "无缝衔接"}
				chosen := labels[0]
				if !s.AdaptiveMix {
					chosen = labels[1]
				}
				a.settingsChoice(c, "song-transition", "歌曲过渡方式", chosen, labels, 160, func(chosen string) {
					s.AdaptiveMix = chosen == labels[0]
					a.pl.ApplyTransitions()
					s.save()
					a.redraw()
				})
			})
			row("动态封面与背景", "播放时展示动态封面，让专辑配色缓慢流动；暂停后停留在当前画面", func() {
				if a.switchControl(c, "motion-artwork", &s.MotionArtwork, "动态封面与背景").Changed() {
					s.save()
					a.redraw()
				}
			})
			settingRow("音质", "无损与 Hi-Res 需要黑胶 VIP，没有时播放所能播放的最高音质；下一首歌曲开始生效", false, func() {
				labels := make([]string, len(qualityLevels))
				chosen := qualityLevels[2].label
				for i, q := range qualityLevels {
					labels[i] = q.label
					if q.level == s.Quality {
						chosen = q.label
					}
				}
				a.settingsChoice(c, "quality", "音质", chosen, labels, 150, func(chosen string) {
					for _, q := range qualityLevels {
						if q.label == chosen {
							s.Quality = q.level
						}
					}
					s.save()
					a.redraw()
				})
			})
			row("间奏时显示热评", "前奏、间奏与尾奏时，歌词处换成这首歌的热门评论", func() {
				if a.switchControl(c, "hot-comments", &s.HotComments, "间奏时显示热评").Changed() {
					a.changed()
				}
			})
			row("音量均衡", "按网易云音乐测得的响度调整每首歌的音量，让歌曲听起来一样响", func() {
				if a.switchControl(c, "normalize", &s.Normalize, "音量均衡").Changed() {
					a.pl.SetVolume(a.volume)
					a.changed()
				}
			})
		})

		if a.captionsShown() {
			group("播客", func() { a.captionSettings(row, c) })
		}

		group("缓存", func() {
			settingRow("歌曲缓存", "听过的歌曲保存在本机，再次播放无需等待，也不耗流量；"+a.cacheUsage(), false, func() {
				labels := make([]string, len(cacheLimits))
				chosen := cacheLimits[2].label
				for i, l := range cacheLimits {
					labels[i] = l.label
					if l.mb == s.CacheMB {
						chosen = l.label
					}
				}
				a.settingsChoice(c, "cache", "歌曲缓存", chosen, labels, 150, func(chosen string) {
					for _, l := range cacheLimits {
						if l.label == chosen {
							s.CacheMB = l.mb
						}
					}
					s.save()
					a.redraw()
					a.applyCacheLimit()
				})
			})
			row("清除缓存", "删除保存在本机的全部歌曲", func() {
				if a.settingButton(c, "clear-cache", "清除", a.cacheUsed <= 0).Clicked() {
					a.clearCache()
				}
			})
		})

		group("外观", func() {
			if iconSwitchable {
				row("深色图标", "主屏幕上的应用图标使用黑色背景", func() {
					if a.switchControl(c, "dark-icon", &a.iconDark, "使用深色图标").Changed() {
						a.setIconDark(a.iconDark)
					}
				})
			}
			settingRow("外观", "", false, func() {
				modes := []string{"system", "light", "dark"}
				if a.phone {
					labels := []string{"跟随系统", "浅色", "深色"}
					chosen := labels[0]
					for k, m := range modes {
						if m == s.Appearance {
							chosen = labels[k]
						}
					}
					a.settingsChoice(c, "appearance", "外观", chosen, labels, 0, func(chosen string) {
						for k, l := range labels {
							if l == chosen {
								s.Appearance = modes[k]
							}
						}
						a.changed()
					})
					return
				}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
		})

		if !a.phone {
			group("实验性功能", func() {
				row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
					if a.switchControl(c, "smoke", &s.Smoke, "进度条烟雾特效").Changed() {
						a.changed()
					}
				})
				row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
					if a.switchControl(c, "fog", &s.Fog, "烟雾背景").Changed() {
						a.changed()
					}
				})
			})
		}

		group("账号", func() {
			if a.profile == nil {
				if a.phone {
					ui.ButtonBase(c.Key("login")).MinHeight(50).Center().Label("扫码登录").OnClick(func() {
						a.showMain()
						a.openLogin()
						a.redraw()
					}).Children(func() {
						ui.Text(c, "扫码登录").FontSize(16).TextColor(p.accent)
					})
					return
				}
				row("未登录", "登录后，每日推荐与歌单将根据你的口味生成", func() {
					if ui.PrimaryButton(c.Key("login"), "扫码登录").Clicked() {
						a.showMain()
						a.openLogin()
						a.redraw()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.avatar(c, prof.AvatarURL, 36, p.press)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Nickname).FontSize(13.5).FontWeight(600)
					ui.Text(c, "已登录网易云音乐").FontSize(12).TextColor(p.muted)
				})
				if !a.phone && ui.Button(c.Key("logout"), "退出登录").Clicked() {
					a.signOut()
				}
			})
			if a.phone {
				ui.ButtonBase(c.Key("logout")).MinHeight(50).Center().Label("退出登录").OnClick(a.signOut).Children(func() {
					ui.Text(c, "退出登录").FontSize(16).TextColor(p.accent)
				})
			}
		})

		if !a.phone && a.updates {
			group("更新", func() {
				desc := "每天检查一次是否有新版本"
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += "；上次检查：" + commentTime(t, time.Now())
				}
				row("自动检查更新", desc, func() {
					on := updateChecks()
					if a.switchControl(c, "update-checks", &on, "自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
					on := updateDownloads()
					if a.switchControl(c, "update-downloads", &on, "自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row("当前版本 "+mygo.App.Version(), "", func() {
					if a.settingButton(c, "check-updates", "检查更新", false).Clicked() {
						checkForUpdates()
					}
				})
			})
		}
	})
}

func (a *app) settingsChoice(c *ui.Context, key, title, chosen string, choices []string, width float32, apply func(string)) {
	if !a.phone {
		if ui.Select(c.Key(key), &chosen, choices).Label(title).Width(width).Changed() {
			apply(chosen)
		}
		return
	}
	p := a.pal
	ui.ButtonBase(c.Key(key)).MinHeight(44).PaddingX(2).Gap(5).AlignItems(ui.Center).
		Label("选择" + title).Value(chosen).OnClick(func() { a.phoneSetting = key }).Children(func() {
		ui.Text(c, chosen).FontSize(15).TextColor(p.muted).SingleLine()
		ui.Icon(c, iconForward).Size(13, 13).TextColor(p.muted)
	})
	if a.phoneSetting != key {
		return
	}
	dismiss := func() { a.phoneSetting = "" }
	a.phoneBottomSheet(c.Key(key+"-choices"), title, 66+48*float32(len(choices)), false, dismiss, nil, nil, func() {
		ui.Scroll(c.Key("choices")).Grow(1).MinHeight(0).Children(func() {
			for _, choice := range choices {
				var check *ui.SVG
				if choice == chosen {
					check = iconCheck
				}
				phoneOptionRow(c.Key(choice), choice, nil, check, p.text, func() {
					dismiss()
					if choice != chosen {
						apply(choice)
					}
				}).Role(ui.RoleRadio).Checked(choice == chosen)
			}
		})
	})
}
