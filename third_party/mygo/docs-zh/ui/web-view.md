# Web 视图

`ui.WebView` 在原生 UI 窗口中显示网页，例如原生控件旁由 Web 引擎渲染的文稿、各标签页中的页面，或应用生成的 HTML 预览。网页由 `Window.NewWebView` 创建的系统 webview（WKWebView、WebView2、WebKitGTK）显示，工作方式与窗口的页面相同：加载应用前端，通过类型安全的客户端调用绑定的 Go 方法并接收事件。在 web 视图之后绘制的内容会显示在页面之上，包括上方按钮打开的菜单、下拉选择框的选项、对话框及其遮暗背景、工具提示、短暂提示和角标。

```go
type browser struct {
	win  *mygo.Window
	docs *mygo.WebView
	menu bool
}

func (app *browser) view(c *ui.Context) {
	ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			menu := ui.Button(c, "Menu ▾")
			if menu.Clicked() {
				app.menu = !app.menu
			}
			// The popover shows over the page.
			ui.Popover(c, menu, &app.menu, func() { ui.Text(c, "Over the page") })
		})
		ui.WebView(c, app.docs).Grow(1).Radius(8)
	})
}

func main() {
	app := &browser{}
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{Title: "Docs", Content: ui.View(app.view)})
		app.docs, _ = app.win.NewWebView(mygo.WebViewOptions{URL: "/docs"})
		app.win.Invalidate()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## 创建 web 视图

`Window.NewWebView` 为显示 `Content` 的窗口创建 web 视图，窗口关闭时它也随之关闭。选项包括页面选项（`Page`，与窗口的 `PageOptions` 相同）、要加载的 `URL`、页面绘制前使用的填充色 `BackgroundColor`、`Transparent`（通过页面的透明部分显示窗口背景或材质，因为原生 UI 不会在 web 视图下方绘制内容），以及 `ScrollToContainer`（将页面上的滚轮事件交给周围的原生 UI，[见下文](#滚动周围的原生-ui)）。`WebView.Page` 返回其页面，提供与窗口页面相同的方法：`LoadURL`、`LoadHTML`、`Eval`、`GoBack`、`OnDidNavigate`、`OnWillNavigate` 等。`Focus` 让页面获得键盘焦点；`SetBackgroundColor` 更改填充色，例如在应用主题变化时调用（使用 `light-dark(<light>, <dark>)` 指定的颜色会自动跟随外观）；`CapturePage` 截取页面；`Destroy` 关闭 web 视图。

web 视图会像窗口一样信任应用自己的页面，包括使用自定义协议（scheme）的页面：把第三方或用户提供的 HTML 放在[不受信任的页面](../frontend.md#不受信任的页面)中（`PageOptions.Untrusted`），或让它来自不信任的来源，参见[信任](../frontend.md#信任)。

web 视图要像窗口一样只创建一次，不要在视图中创建：视图只负责显示它。视图没有构建的 web 视图会隐藏起来并保留页面，因此标签页视图只构建当前标签页的 web 视图，切回来时页面还在原处。

```go
if app.tab == "docs" {
	ui.WebView(c, app.docs).Grow(1)
} else {
	ui.WebView(c, app.changelog).Grow(1)
}
```

## 它的盒子

web 视图没有固有尺寸。元素在容器的交叉轴上拉伸（`AlignSelf(Stretch)`），主轴上的空间则由 `Grow` 或指定尺寸决定。页面显示在元素的内容盒内，即边框和内边距之内；周围照常绘制边框、内边距区域和元素背景，圆角取边框内缘由 `Radius` 决定的形状。页面也会裁剪到这些圆角，即使下方没有绘制内容（例如透明窗口），页面也不会超出圆角，只有圆角以内的指针事件才会传给页面。周围的滚动容器和裁剪区域会像处理其他元素一样裁剪页面，滚出视口的 web 视图会隐藏。`Opacity` 让页面随元素一起淡入淡出，例如 `Router` 的页面淡入：页面与元素下方绘制的内容混合。如果下方没有绘制内容（例如透明窗口），页面就无法淡入淡出，因为 MyGo 无法直接设置系统 webview 的不透明度。

```go
ui.WebView(c, app.preview).Height(320).Radius(12).Border(1, t.Border)
```

### 随周围的原生 UI 缩放

缩放 web 视图的变换（它自己的或围绕它的）会把页面当作它的一张图片来缩放显示：页面保持排版时的尺寸，即变换前元素的内容盒，因此面板放大或缩小时，文本不会重新换行，`innerWidth` 保持不变。指针会在页面显示的位置按其自身坐标到达页面，且只在圆角之内。一个在窗口上方打开的速览面板，其 web 视图和原生控件作为一个整体，在变暗的背景上一起缩放：

```go
func (app *browser) peek(c *ui.Context) {
	scale := c.Root().AnimateTo("peek", app.peekScale(), ui.Animation{Duration: 250 * time.Millisecond})
	ui.Box(c).Absolute().Fill().Background(ui.RGBA(0, 0, 0, .4)) // the backdrop, unscaled
	ui.Column(c).Absolute().Left(80).Top(60).Right(80).Bottom(60).Scale(scale, scale).
		AlignItems(ui.Stretch).Radius(16).Background(t.Surface).Children(func() {
		toolbar(c)
		ui.WebView(c, app.peekPage).Grow(1).Radius(16)
	})
}
```

让 web 视图旋转、倾斜或处于透视中的变换，会把页面正立显示，并在变换后盒子的边界内排版。

- **macOS** 和 **Windows** 由合成器缩放页面：页面作为图片显示，动画以完整大小结束时再次变清晰。
- **Linux**：GTK 3 无法缩放控件。web 视图会调整为页面实际显示的尺寸，WebKitGTK 则通过页面缩放适配，以相同的 CSS 像素宽度重新排版，因此各种尺寸下都清晰：`innerWidth` 保持不变，但缩放变化时页面会收到 `resize` 事件，而且缩放比例仅由宽度决定。圆角按整像素裁剪，不做抗锯齿。

## 它上方显示什么

web 视图位于窗口的原生 UI 之下：元素在窗口绘制的内容中留出空洞，用于显示页面。在该元素之后绘制的内容都会覆盖在页面上，与覆盖其他元素一样，包括它的子元素、绝对定位在其上方的元素，以及浮层中的弹出面板、菜单、下拉选择框、组合框、对话框、工具提示和短暂提示。阴影和半透明背景也会与页面混合。

```go
ui.WebView(c, app.page).Grow(1).Children(func() {
	// A badge over the page's corner.
	ui.Text(c, "Draft").Absolute().Top(8).Right(8).Padding(2, 8).Radius(8).Background(t.Accent)
})
```

只有在页面显示的区域内，且上方绘制的内容没有接收指针事件时，事件才会传给页面：对话框或下拉选择框打开后，其背景遮罩会阻止指针事件传给页面；弹出面板只拦截面板范围内的事件；带有 `PassThrough` 的元素则允许事件穿透。拖到页面上的文件会交给页面处理。在页面上按下会关闭按下位置之外的弹出面板（`PressedOutside`），并让页面获得键盘焦点：原生 UI 的焦点移到 web 视图元素，用户再点击原生 UI 时，键盘焦点就会移回原生 UI。页面负责设置其区域内的鼠标指针样式。

Tab 在原生 UI 控件之间移动时也会进入 web 视图：焦点落在页面的第一个元素上，Shift+Tab 则落在最后一个元素上；之后 Tab 在页面元素之间移动，越过最后一个元素后移到 web 视图后面的原生控件。`Focus` 则让页面在上次获得焦点的位置重新获得键盘焦点。

## 页面留下的按键

例如，邮件应用在 web 视图中显示邮件的 HTML，旁边是支持键盘操作的列表：J 和 K 切换邮件，Cmd+K（Windows 和 Linux 上为 Ctrl+K）打开命令面板。用户点击邮件以选择文本或打开链接后，即使页面获得了键盘焦点，J、K 和 Cmd+K 仍然有效：页面未处理的按键会交给原生 UI，就像焦点位于 web 视图元素上一样。周围元素的快捷键（`Element.Shortcut`）和窗口快捷键（`c.Shortcut`）都能处理这些按键，滚动按键会滚动周围的容器，Escape 会关闭原生 UI 的对话框或浮出窗口。页面仍保持键盘焦点，用户仍可选择、复制、输入文本和打开链接。

```go
func (app *mail) view(c *ui.Context) {
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		app.palette = true
	}
	reader := ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		app.list(c)
		ui.WebView(c, app.message).Grow(1) // an untrusted page
	})
	if reader.Shortcut(0, ui.KeyJ) {
		app.next()
	}
	if reader.Shortcut(0, ui.KeyK) {
		app.previous()
	}
}
```

页面在以下情况保留一个按键：

- 取消它（在 `keydown` 处理器中调用 `preventDefault`）；
- 在文本框、`textarea` 或可编辑内容中用它输入或编辑，或输入法用它组合文字，或在任意焦点位置用 AltGr 输入字符，如德语键盘上的 @；
- 用它执行操作：链接或按钮上的 Enter 或空格、滑块上的方向键、Ctrl+C 复制选区，或方向键、空格和 Page Down 在它能向该方向滚动时滚动页面；
- 或者它是 Tab（在页面中移动并移出页面），或单独的修饰键。

不受信任页面以及页面中各子框架的未处理按键也会传来；页面无法伪造按键，原生 UI 只会收到用户实际按下的键。

- **macOS** 和 **Linux**：WebKit 决定页面处理了哪些按键，并把其他的发送给 web 视图的父级（AppKit 的响应者链，或 WebKit 的 Web 进程应答后的 GTK `key-press-event`），MyGo 在那里接收它们。在 Linux 上，WebKitGTK 保留方向键，而带角色的菜单项的快捷键（如 Edit › Copy 的）会继续传给菜单。
- **Windows**：WebView2 不提供未处理按键的通知。MyGo 在每个文档（页面及其各子框架）的隔离脚本环境中运行脚本，在页面按键处理器执行后，按上述规则将未处理的按键通知 MyGo。页面自身的脚本既无法访问该脚本，也无法访问其通知 MyGo 的通道；该脚本不依赖桥接，因此不受信任页面的按键也能传来。跨站子框架由 WebView2 在独立进程中运行，其按键不会传来。
- **浏览器**（`GOOS=js`）没有 web 视图。

辅助技术可以在 web 视图元素内部找到页面，将其作为原生 UI 的一部分：VoiceOver 和 Orca 能像读取和浏览周围控件一样读取和浏览页面。

## 滚动周围的原生 UI

一个阅读器在原生滚动容器中显示文稿：原生页头和它的评论之间的文章、邮件线程中的消息、原生卡片之间的渲染笔记。web 视图与页面一样高，页面自身没有可滚动的内容，页面上的滚轮应当滚动容器，就像在周围的原生元素上那样。`WebViewOptions.ScrollToContainer` 可以做到这一点：web 视图上的滚轮和触摸板会像在任何原生元素上一样滚动指针下的原生 UI，鼠标滚轮按行，触摸板按像素，方向相同，并带上按住的修饰键。页面永远不会随之滚动；它仍然接收指针按下、键盘、选择和触摸板的捏合。

```go
article, _ := win.NewWebView(mygo.WebViewOptions{URL: "/article", ScrollToContainer: true})

// In the view:
ui.Scroll(c).Fill().Children(func() {
	header(c)
	ui.WebView(c, article).Height(app.articleHeight) // the page's height, which it tells the app
	comments(c)
})
```

没有它时（默认情况），页面会在它显示的地方接收滚轮，这正是会自行滚动的页面所需要的：文档、浏览器的标签页、编辑器。

- **macOS**：web 视图的滚动事件（包括触摸板的惯性）会发送给原生 UI，而不是 WebKit。
- **Windows**：窗口保留 web 视图上的滚轮消息，而不转发给 WebView2。触摸板的捏合被 Windows 作为按住 Ctrl 的滚轮发送，仍然会缩放页面。
- **Linux**：web 视图的滚动事件会在 WebKitGTK 看到它们之前发送给原生 UI。
- **浏览器**（`GOOS=js`）没有 web 视图，所以该选项在那里不起作用。

## 它的页面

web 视图的页面工作方式与窗口的页面相同（参见[绑定](../bindings.md)）。`mygo.CallerPage(ctx)` 返回调用绑定方法的页面，`Page.WebView` 返回它的 web 视图，而 `mygo.CallerWindow(ctx)` 返回 web 视图所在的窗口，页面的窗口控件（`mygo.window.minimize()`、拖动区域）会作用于它。向窗口发送 `Event.Emit` 会到达其 web 视图的页面，`Event.EmitPage` 到达一个页面，`Broadcast` 则到达每个页面。

```go
func (s *Notes) Save(ctx context.Context, text string) error {
	page := mygo.CallerPage(ctx) // the web view's
	...
}
```

页面的 `window.close()` 会让窗口保持打开。内容规则会拦截 web 视图页面的加载，如同对窗口的页面一样（[阻止内容](../windows.md#阻止内容)），规则来自 `WebViewOptions.Page.ContentRules` 或 `Page.SetContentRules`。

### 类型安全的客户端

页面与窗口页面一样，通过 `mygo generate` 生成的客户端调用 Go，客户端无需区分 web 视图。原生 UI 项目默认没有前端，也就没有客户端；你可以添加前端，例如在 `frontend` 目录创建 Vite 应用，并在 `mygo.json` 中填写[前端](../configuration.md#开发与前端)相关字段：

```json
{
  "name": "Notes",
  "devUrl": "http://localhost:5173",
  "devCommand": "bun run --cwd frontend dev",
  "buildCommand": "bun run --cwd frontend build",
  "frontendDist": "frontend/dist"
}
```

客户端随后就是 `frontend/src/mygo.ts`（`bindings` 会改变位置），`mygo dev` 会在 Go 代码改变时重写它，`mygo build` 则会在构建前端之前重写它；前端依赖客户端所导入的 `mygo-runtime`。加载 `/` 的 web 视图会显示前端：在 `mygo dev` 期间来自开发服务器，在构建中则嵌入其中（参见[页面如何加载](../frontend.md#页面如何加载)）。它的页面会调用用 `mygo.Bind` 绑定的服务，并接收用 `mygo.NewEvent` 声明的事件：

```go
// Notes is bound for the pages.
type Notes struct{ likes atomic.Int64 }

// Liked tells pages how many likes there are.
var Liked = mygo.NewEvent[int]("liked")

// Like counts a like and tells every page.
func (n *Notes) Like() int {
	likes := int(n.likes.Add(1))
	Liked.Broadcast(likes)
	return likes
}

var page *mygo.WebView

func view(c *ui.Context) {
	if page != nil {
		ui.WebView(c, page).Fill()
	}
}

func main() {
	mygo.Bind(&Notes{})
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Notes", Content: ui.View(view)})
		page, _ = win.NewWebView(mygo.WebViewOptions{URL: "/"})
		win.Invalidate()
	})
	// ...
}
```

```ts
import { Notes, events } from "./mygo";

likeButton.onclick = async () => {
  count.textContent = String(await Notes.like());
};
events.liked.on((likes) => (count.textContent = String(likes)));
```

带上下文的调用、错误、[通道](../bindings.md#通道)和事件都与窗口中的一样。没有前端构建的页面（例如应用用 `LoadHTML` 加载的 HTML），可以借助桥接注入每个页面的运行时按名称调用，`mygo.call("Notes.Like")` 和 `mygo.on("liked", …)`（参见[不使用生成的客户端](../bindings.md#不用生成的客户端)）。不是应用自己的页面（例如显示 `https://example.com` 的 web 视图）根本无法调用 Go：参见[谁可以调用](../bindings.md#谁能调用)。

## 限制

页面是系统的 web 视图，窗口将其与原生 UI 合成；MyGo 不绘制它。如同 SwiftUI 的 `WebView`：

- 读取其背后内容的效果（如 glass 插件的模糊）看不到页面的任何内容：它不是窗口帧的一部分。glass 插件的 `Glass{Native: true}` 是例外：系统的玻璃视图位于页面和内容之间，因此在 macOS 26 及更高版本上，web 视图上方的工具栏能获得真正的 Liquid Glass。
- 窗口的 `Window.CapturePage` 会在页面所在处显示一个洞；`WebView.CapturePage` 截取页面。

在 Windows 上，WebView2 会把以这种方式显示的页面元素放在它自己的一个窗口下：Narrator 会在原处找到 web 视图，并在那个窗口中找到页面的元素，而不是在 web 视图里面。

## 测试

在 `Tester` 中，`WebViewAt` 按最后一帧的显示状态，返回窗口指定位置接收指针事件的 web 视图：即该点下方的 web 视图，但如果覆盖其上的内容在该处接收事件，则不会返回它。在 web 视图上使用 `ClickAt` 和 `Press` 会向页面发送按下事件，视图收到的通知与平台实际报告的一致。
