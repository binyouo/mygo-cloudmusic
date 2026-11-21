# 使用原生 UI 的窗口

带 `Content` 的窗口接受任何窗口的[窗口选项](../windows.md#选项)，例如大小、`StateKey`、`Frameless` 和 `Parent`，也支持菜单、对话框和其他[桌面 API](../native.md)。它没有页面：`Page()` 为 nil，`URL` 和 `WindowOptions.Page` 会被忽略。它的 Go 代码不需要绑定，视图会直接调用。`CapturePage` 返回它所显示内容的 PNG。

```go
mygo.NewWindow(mygo.WindowOptions{
	Title:    "Notes",
	Width:    900,
	Height:   600,
	StateKey: "main",
	Content:  ui.View(app.view),
})
```

视图在 `NewWindow` 返回后才首次运行，因此可以使用保存下来的窗口引用，例如调用 `app.win.Title()`。在主线程上调用 `NewWindow` 时，窗口会在主循环的下一轮显示（见[无闪烁地显示窗口](../windows.md#无闪烁地显示窗口)）。

## 标题栏

使用 `TitleBarStyle: mygo.TitleBarHidden` 时，视图会在窗口控件下方绘制标题栏，就像页面通过 `--mygo-titlebar-*` CSS 变量所做的那样：`c.TitleBar()` 返回控件占用的空间，全屏时为零，`DragWindow` 让元素可以拖动窗口，双击它们则会像标题栏一样缩放或最小化：

```go
bar := c.TitleBar()
ui.Row(c).Height(max(bar.Height, 32)).Padding(0, bar.Right+12, 0, bar.Left+12).DragWindow().Children(func() {
	ui.Text(c, "Inbox").Bold()
})
```

`bar.Bottom` 是控件结束的位置，从窗口顶边算起：在 macOS 上是红绿灯按钮的底部，`TrafficLightPosition` 可以移动它们；Linux 和 Windows 上则是 `bar.Height`。如果侧边栏一直延伸到窗口顶部、红绿灯按钮浮在它上面，侧边栏的条目就要从按钮下方开始：

```go
ui.Column(c).Width(220).Padding(bar.Bottom+8, 8, 8, 8).DragWindow().Children(func() {
	ui.Text(c, "Inbox")
})
```

在 `Frameless` 窗口中，`DragWindow` 让元素可以移动窗口，双击它则会使窗口最大化。

如果某个拖动区域在双击时有别的用途，例如侧边栏空白处用来新建内容，可以用 `WindowDoubleClicked` 接管双击，同时仍然可以移动窗口：

```go
side := ui.Scroll(c.Key("sidebar")).Grow(1).DragWindow()
if side.WindowDoubleClicked() {
	app.newItem()
}
```

如果还想自己绘制窗口控件，并放在视图希望的位置，可以在隐藏标题栏的基础上加上 `HideWindowButtons: true`：系统控件会消失，`c.TitleBar()` 变为零，但窗口仍保留标题栏的边缘，可以用来调整大小，顶部边缘也包括在内。视图中的按钮可以调用 `Minimize`、`Maximize`、`Unmaximize` 和 `Close`。

## 缩放

`c.SetZoom(z)` 会像浏览器缩放页面一样缩放窗口内容：视图按窗口尺寸的 `1/z` 布局，再放大 `z` 倍显示。这不是放大窗口截图：绘制时使用的像素密度为屏幕像素密度的 `z` 倍，文本、图标、矢量、图像及其边缘都保持与直接按目标尺寸绘制时相同的清晰度，而窗口每帧的总像素尺寸不变。例如，屏幕缩放系数为 2、内容放大 2 倍时，会按屏幕缩放系数为 4 的像素密度绘制，包括字形；未缩放时不受影响。缩放值会保持到下次设置，范围为 0.25 到 5；`c.Zoom()` 返回当前值，1 表示未缩放。

```go
c.SetZoom(app.zoom) // 1.25 for 125%, every frame or once
width, height := c.Size() // the content's: the window's divided by the zoom
```

视图使用缩放后的坐标：`c.Size()`、`c.TitleBar()`、`Bounds` 及所有长度均以内容 DIP 为单位，每个内容 DIP 对应 `z` 个窗口 DIP。某一帧改变缩放值后，会先按新尺寸重新构建，再显示。内容与窗口交互时会换算为窗口 DIP：指针命中缩放后实际显示的内容，输入法在插入点处组合文本，辅助技术按元素的显示位置绘制轮廓，上下文菜单和拖动从指针位置开始，Web 视图使用内容分配的区域，`MaximizeButton` 在按钮显示的位置弹出 Windows 贴靠布局。触摸板按手指移动距离滚动内容；鼠标滚轮的滚动距离与未缩放时相同。窗口控件本身不变大：`c.TitleBar()` 返回换算为内容坐标的占用空间，因此标题栏布局仍能避开控件。

窗口自身的尺寸不参与缩放：`WindowOptions.Width` 和 `Height`、`Window.SetContentSize` 和 `Window.ContentSize` 以窗口的 DIP 计，是视图所见尺寸的 `z` 倍，和页面在其缩放下的情况一样（`Page.SetZoomFactor`）。一个 320 × 400 的浮出窗口缩放 1.5 倍后，其视图会布置在约 213 × 267 的范围内。如果要用视图自己的 DIP 给窗口一个尺寸，就乘以缩放值：

```go
z := c.Zoom()
flyout.SetContentSize(int(math.Ceil(float64(320*z))), int(math.Ceil(float64(400*z))))
```

相比之下，变换的 `Scale` 会按内容自身的尺寸缩放绘制内容：其中的文本会以未变换尺寸的位图绘制并做双线性采样（[变换](transforms.md#渲染)），适合动效，用来缩放窗口则会模糊。

## 材质

窗口的 `Vibrancy` 会显示在原生 UI 不绘制背景的地方：macOS 上始终如此；Windows 11 22H2 及更高版本上，则仅在创建时启用了材质的窗口中如此。`c.Vibrancy()` 说明它是否生效：在其他地方，视图留作透明处不会显示材质，所以应改为在那里绘制背景。根元素默认绘制主题背景：可以把它设为透明，再给需要背景的部分加上背景，例如不透明内容旁的侧边栏：

```go
if c.Vibrancy() {
	c.Root().Background(ui.Transparent)
}
ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
	sidebar := ui.Column(c).Width(240)
	if !c.Vibrancy() {
		sidebar.Background(c.Theme().Surface)
	}
	sidebar.Children(func() { app.sidebar(c) }) // over the material
	ui.Column(c).Grow(1).Background(c.Theme().Background).Children(func() { app.content(c) })
})
```

在透明根元素下，不透明背景上的文本仍保留系统的次像素抗锯齿（ClearType）；而在材质上，则使用灰度抗锯齿。`examples/vibrancy` 会在这样的侧边栏中选择窗口材质。

### 覆盖在其他应用上的 Liquid Glass

`VibrancyGlass` 和 `VibrancyGlassClear` 用 macOS 26 的 Liquid Glass 填充窗口，系统会结合窗口背后的内容来绘制它：其他应用的窗口、桌面。因此，带 `CornerRadius` 的无边框窗口就成了浮在它们之上的一块玻璃，例如启动器的面板、平视显示器或浮动工具栏：

```go
hud := mygo.NewWindow(mygo.WindowOptions{
	Frameless:    true,
	Transparent:  true,
	AlwaysOnTop:  true,
	Vibrancy:     mygo.VibrancyGlassClear, // Acrylic on Windows 11
	CornerRadius: 18,
	Width:        360,
	Height:       64,
	Content:      ui.View(view), // its root transparent where c.Vibrancy()
})
hud.SetActiveAppearance(true) // stays clear over the app in front (macOS)
```

`VibrancyTint` 为玻璃材质着色，强度由 alpha 值决定，与 AppKit 的 `NSGlassEffectView` 相同：

```go
hud.SetVibrancyTint("light-dark(rgba(0, 122, 255, 0.2), rgba(255, 159, 10, 0.2))")
```

其他材质和平台会忽略它。`WindowOptions` 中的 `VibrancyTint` 可以在创建时设置它。

[glass 插件](../plugins/glass.md)也会在每个平台上绘制 Liquid Glass，但它用的是窗口自身绘制的内容：例如位于下方滚动内容之上的工具栏。窗口的玻璃显示应用自身的任何窗口都未绘制的内容，并且只在系统支持它的地方：在 macOS 26 之前是浮层或 HUD 材质，Windows 11 上是 Acrylic，Linux 上则没有；`c.Vibrancy()` 会告知结果。

透明玻璃只有在窗口呈活动外观时才保持透明，通常是前台应用中接收键盘输入的窗口；否则系统会改用磨砂效果，浮在用户当前应用上方的面板也是如此。`SetActiveAppearance` 通过 AppKit 私有调用保持活动外观；未来 macOS 可能移除该调用，届时窗口会恢复普通绘制方式。面向 Mac App Store 的应用不应启用它。

## 适应其内容的浮出窗口

设置 `FitContent` 后，浮出窗口会随原生 UI 测得的内容尺寸调整大小，并受 `MinWidth`、`MinHeight`、`MaxWidth`、`MaxHeight` 和工作区限制。宽度取根元素内容的最大内容宽度，高度取该宽度下的布局高度，适用于列表或错误提示会增减的托盘面板。某一帧测得的尺寸变化后，窗口先调整大小，下一帧再按新尺寸布局。隐藏的浮出窗口重新显示前，会重新布局隐藏期间变化的内容，并以所需尺寸显示。见[让浮出窗口适应其内容](../windows.md#让浮出窗口适应其内容)。

## 活动与非活动窗口

原生应用会把接收用户输入的窗口画得和其他窗口不同，让用户一眼看出输入落在哪个窗口：macOS 会把非活动窗口侧边栏的材质淡成不透明灰色，把其选区和控件的强调色淡成灰色，Windows 把它的 Mica 画成纯色填充，GTK 则将其样式设为 `:backdrop`。当窗口自身或它的某个浮出窗口具有键盘焦点时，窗口是活动的（菜单或浮层属于其窗口）；如果 `SetActiveAppearance` 让它保持这种状态，则始终活动。没有父窗口的面板和浮出窗口会浮在任何前台应用之上，始终是活动的。

内置控件会自动跟随：选中的行、侧边栏的图标、选中的复选框、单选按钮和开关、滑块、进度条以及所选标签页的线条，会从强调色淡成主题的 `InactiveAccent`（一种灰色）；主要按钮则看起来和其他按钮一样（[主题](styling.md#主题)用 `InactiveAccent: Accent` 保留强调色）。视图可以用 `c.WindowActive()` 自行处理：

```go
dot := c.Theme().Accent
if !c.WindowActive() {
	dot = c.Theme().TextMuted
}
```

读取它的帧会在它变化时重新绘制；不读取它的帧则不会因此多绘制任何内容。`Tester.SetWindowActive` 可以在测试中切换活动状态，键盘焦点仍由 `SetFocused` 控制。窗口材质会跟随系统（macOS 会在非面板、非浮出窗口的窗口中淡化它，Windows 会淡化其 Mica）；`examples/vibrancy` 一起展示了这两者。

## 隐藏的窗口

`c.WindowHidden()` 报告窗口是否完全没有显示在屏幕上，和 Web 的 `document.hidden` 一样：显示时钟、价格或播放位置的视图会在隐藏时停止工作，并在重新显示时刷新。查询它的帧会在窗口隐藏时重新构建一次（尽管隐藏期间不会绘制任何帧），窗口显示时再构建一次：

```go
app.ticker.SetPaused(c.WindowHidden()) // a ticker of your own
```

动画及其他动态绘制请求的帧本就会在窗口隐藏期间暂停。macOS 会报告窗口最小化、隐藏、位于其他桌面空间或被其他窗口覆盖的状态；浏览器会报告标签页或窗口不可见的状态；Linux（X11）和 Windows 则只报告最小化。`Tester.SetWindowHidden` 可以在测试中切换隐藏状态。

`Window.IsVisible` 回答的是另一个问题：窗口是否被 `Show` 显示过，并且此后没有因 `Hide` 而隐藏，因此最小化的窗口仍然算作可见。视图之外的工作（例如轮询的 goroutine）可以通过视图得知窗口离开了视野；窗口离开视野时视图会重新构建：

```go
paused.Store(c.WindowHidden()) // an atomic.Bool the goroutine reads
```

## 关闭

视图可以在处理按钮单击或执行回调时关闭自身窗口。调用 `Destroy`，或调用未被 `OnClose` 监听器阻止的 `Close`，都会立即关闭窗口。视图剩余部分仍照常构建，`Context` 也仍提供主题和窗口尺寸，但本帧不再布局、绘制或执行其他回调。即使应用仍持有 `*mygo.Window`，关闭的窗口也不会保留之前显示的内容，包括视图及其引用的对象。应先打开接替窗口，因为最后一个窗口关闭时应用会[退出](../app.md)：

```go
if ui.Button(c, "Done").Clicked() {
	app.showMain()
	win.Close()
}
```

## 没有 webview

所有窗口都显示原生 UI 的应用不需要 webview：在 Linux 上只需要 GTK 3，不需要 WebKitGTK；在 Windows 上则不需要 WebView2 Runtime。

在 Windows 上，它的可执行文件里仍有加载 WebView2 的代码，以及这些代码的字符串：`WebView2Loader.dll`、`msedgewebview2.exe`、`CreateCoreWebView2EnvironmentWithOptions`、`WEBVIEW2_USER_DATA_FOLDER`。对于没有签名的可执行文件，杀毒软件的启发式检测可能把它们当作可疑信号。构建标签 `mygo_nowebview` 让 Windows 后端不含 WebView2，可执行文件也随之小约 500 KB：

```sh
GOFLAGS=-tags=mygo_nowebview go tool mygo build -platform windows/amd64
GOOS=windows go build -tags mygo_nowebview .
```

`mygo build` 会把 `GOFLAGS` 的标签加到它自己的标签里。带这个标签构建的应用只能显示原生 UI：显示网页的窗口（没有 `Content` 的窗口）会让 `NewWindow` panic，`Window.NewWebView` 返回错误，`WebViewAvailable` 返回 false，cookie 调用会失败。其他平台忽略这个标签。
