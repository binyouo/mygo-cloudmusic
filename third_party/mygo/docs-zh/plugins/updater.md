# 更新器

更新器插件在每个平台上为应用提供 Mac 用户从 Sparkle 中熟悉的更新窗口：它在后台检查更新，显示新版本的发布说明，并提供安装、跳过或稍后提醒的选项，然后带进度条下载更新，重启应用并进入新版本。全部用 Go 写成，没有 npm 包，在网页应用和[原生 UI](../ui/README.md)应用中同样可用。它的窗口是一个网页；对于完全不显示网页的应用，也可以使用带 [`native.Plugin`](#原生-ui) 的原生 UI。

它构建在 `mygo.Updater` 之上，因此应用构建时必须启用更新功能：见[自动更新](../updates.md)，了解如何签名和发布更新。

## 设置

在 Go 中使用插件：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

func main() {
	mygo.Use(updater.Plugin)
	// ...
}
```

- 它会在启动 10 秒后于后台检查，之后在应用运行期间每天检查一次，因此自应用上次运行以来发布的版本会在它再次打开时显示。有新版本时，窗口会显示发布说明，并提供 **安装更新**、**稍后提醒我** 和 **跳过这个版本**。后台检查不会再次提供已跳过的版本，但会提供下一个版本。
- 安装会用进度条下载更新，然后询问是否重新启动；否则更新会在下一次启动时生效。
- 勾选窗口中的“以后自动下载并安装更新”后，后台检查会不经询问就安装更新，更新会在下一次启动时生效。

无法自我更新的构建，例如开发构建和由包管理器安装的应用，从不进行后台检查；用户手动检查时，窗口会说明原因。

## 从菜单检查

`updater.CheckForUpdates()` 按用户的请求检查：窗口会立即显示，并在应用已是最新或检查失败时说明。它也会显示用户跳过的更新。`updater.MenuItem()` 是一个调用它的“检查更新…”菜单项，在 macOS 上放在“关于”之后：

```go
mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
	{Label: "My App", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		updater.MenuItem(),
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}},
	{Role: mygo.RoleEditMenu},
	{Role: mygo.RoleWindowMenu},
}))
```

应用页面上的按钮也可以通过绑定方法调用 `CheckForUpdates`。检查进行中时，再次调用它会把其窗口带到前面。

## 选项

`updater.New` 接受选项，替代 `updater.Plugin` 的默认值：

```go
mygo.Use(updater.New(updater.Options{
	Interval:               12 * time.Hour, // between checks (default a day)
	DisableAutomaticChecks: true,           // until SetAutomaticChecks(true)
	Icon:                   iconPNG,        // default: icon.png among the resources
}))
```

- `Interval` 是后台检查之间的时间，默认一天。
- `DisableAutomaticChecks` 关闭后台检查，直到 `SetAutomaticChecks(true)`，用于先询问用户或只从菜单检查的应用。
- `Icon` 是窗口的 PNG 图像；默认是应用资源中的 `icon.png`，即 `mygo build` 使用的图标（如果存在）。
- `Language` 和 `Strings` 选择并更改窗口的文本（见[语言](#语言)）。

## 偏好设置

用户的选择保存在 `PathUserData` 中的 `updater.json` 里。`AutomaticChecks` 和 `AutomaticDownloads` 读取这些选择，`SetAutomaticChecks` 和 `SetAutomaticDownloads` 更改它们，供偏好设置页面使用；`LastCheck` 返回应用上次检查的时间，`AvailableVersion` 返回它找到的较新版本，可用于徽章或设置行（“版本 1.4 可用”）：`AvailableVersion` 在检查前、检查失败或没有找到更新后，以及对于已跳过的版本，都为空。每当其中一项发生变化时，`OnChange` 就在主线程上调用一个函数，包括检查之后（`AvailableVersion` 也如此）或在更新窗口中作出回答之后，以便页面再次显示这些值：

```go
// UpdatePreferences tells the preferences page what they are now.
var UpdatePreferences = mygo.NewEvent[Prefs]("update-preferences")

updater.OnChange(func() {
	UpdatePreferences.Broadcast(Prefs{
		AutomaticChecks:    updater.AutomaticChecks(),
		AutomaticDownloads: updater.AutomaticDownloads(),
		LastCheck:          updater.LastCheck(),
	})
})
```

`OnInstalled` 在更新安装完成后在主线程上调用一个函数，无论是在更新窗口中还是在后台安装，都在应用重新启动到新版本之前。在 Windows 和 Linux 上，此时应用的可执行文件已是新版本，因此应用可以提前用它启动辅助进程。

Sparkle 会在第二次启动时询问是否自动检查：想要询问用户的应用设置 `DisableAutomaticChecks`，并用用户的回答调用 `SetAutomaticChecks`。

## 语言

插件支持用户的语言（`App.Locale`）时，窗口就使用该语言：英语、中文（简体和繁体）、荷兰语、法语、德语、意大利语、日语、韩语、波兰语、葡萄牙语（巴西）、俄语、西班牙语、土耳其语和乌克兰语，并采用对应的小数分隔符和单位（“1,5 Mo”）。其他语言回退到英语。`Language` 可指定语言，例如应用自身使用的语言；`Strings` 可按语言标签修改文本或添加语言；应用留空的字段沿用插件的文本：

```go
mygo.Use(updater.New(updater.Options{
	Language: settings.Language, // "" follows the system
	Strings: map[string]updater.Strings{
		"en": {Install: "Update Now"},
		"sv": {Title: "Programuppdatering", Install: "Installera uppdatering" /* … */},
	},
}))
```

窗口在检查开始时使用应用当时显示的语言，并在打开期间保持不变。在 `App.SetLocale` 之后，或应用跟随用户语言、用户的语言发生变化之后，下一个窗口会采用新的语言，`MenuItem` 此时返回的菜单项也如此：在 `App.OnLocaleChanged` 中重建菜单的应用会得到新语言的“检查更新…”。

有些文本是格式字符串，例如 `AvailableMessage`，其参数为应用名称、新版本和当前运行的版本；带索引的格式占位符（`%[2]s`）可按不同语言的语序调整参数顺序。格式与参数不匹配时，`mygo.Use` 会失败。`go doc github.com/egoist/mygo/plugins/updater.Strings` 列出了这些文本及格式字符串的参数。

从右到左书写的语言，例如阿拉伯语和希伯来语，会得到镜像的窗口，发布说明采用其书写语言的方向。

## 原生 UI

`updater.Plugin` 的窗口是一个网页，因此即使在自身窗口全部显示[原生 UI](../ui/README.md)的应用中，它在 Linux 上也需要 WebKitGTK，在 Windows 10 上也需要 WebView2 Runtime。包 `native` 改用原生 UI 绘制同一个窗口，不需要 webview：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

func main() {
	mygo.Use(native.Plugin) // or native.New(updater.Options{...})
	// ...
}
```

它接受相同的选项，使用相同的语言，其余一切都在包 `updater` 中：`updater.MenuItem()`、`updater.CheckForUpdates()` 和偏好设置的工作方式与 `updater.Plugin` 相同。只使用两个插件中的一个。窗口外观相同；其发布说明是相同的 Markdown，文本可选择，链接在浏览器中打开。

## 你自己的更新 UI

窗口是提供更新的一种方式。对于你自己的界面，例如应用页面中的横幅，使用 `mygo.Updater` 而不是插件：见[你自己的更新 UI](../updates.md#你自己的更新-ui)。
