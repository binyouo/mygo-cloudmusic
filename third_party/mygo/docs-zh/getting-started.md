# 快速开始

MyGo 应用用 Go 编写，窗口可以显示系统 webview 中的 Web 前端，也可以显示由 MyGo 自行绘制的[原生 UI](ui/README.md)。`mygo init` 可以创建这两类项目，之后也能在同一个应用中加入另一种窗口。

## 安装

安装 [Go](https://go.dev/dl/) 1.27 或更高版本。带 Web 前端的项目还需要 [Bun](https://bun.sh)，由它运行项目的工具；有网页的应用需要系统的 webview：

- 在 Linux 上，GTK 3 和 WebKitGTK 4.1，例如在 Debian 和 Ubuntu 上 `sudo apt install libwebkit2gtk-4.1-0`；
- 在 Windows 10 上，[WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)，Windows 11 已包含它。

原生 UI 不需要更多东西：Go 负责构建应用，运行时只依赖系统已有的组件（Linux 上是 GTK 3）。

`mygo` 命令行工具创建、运行和打包应用。它本身是一个 Go 程序，用 Go 就能运行，无需先安装：

```sh
go run github.com/egoist/mygo/cmd/mygo@latest init my-app
```

带 Web 前端的项目还通过 npm 依赖它，即 `mygo-cli` 包，所以用 Bun 或 npm 也能创建项目：

```sh
bunx mygo-cli init my-app      # or: npx mygo-cli init my-app
```

你也可以用 Go 安装 CLI，它会把 `mygo` 放进你的 `PATH`：

```sh
go install github.com/egoist/mygo/cmd/mygo@latest
mygo init my-app
```

`mygo doctor` 检查机器是否具备 MyGo 所需的一切。

## 创建项目

### Web 前端

`mygo init my-app` 会创建一个 Go 模块和一个用 [Vite](https://vite.dev) 构建的 TypeScript 前端，二者并排放置，并安装它们的依赖：

```
my-app/
├── main.go          the app: its windows and the Go code the page calls
├── go.mod
├── mygo.config.ts   the app's name, identifier and version, and how to build it
├── package.json     the scripts, and the frontend's dependencies
├── index.html       the page
├── src/
│   ├── main.ts      the page's code
│   ├── style.css
│   └── mygo.ts      the typed client of the Go code, generated
├── vite.config.ts
├── tsconfig.json
└── resources/
    └── icon.png     the app icon, a 1024×1024 PNG
```

构建产物写入 `dist/`（前端）和 `build/`（打包好的应用），开发版应用写入 `.mygo/`；`.gitignore` 会把它们排除在外。`mygo.config.ts` 的字段见[配置](configuration.md)。

### 原生 UI

`mygo init -template native my-app` 创建一个应用，它的窗口显示[原生 UI](ui/README.md)：只有一个 Go 模块，没有前端，也不需要 Bun。

```
my-app/
├── main.go          the app: its state, its view and its window
├── main_test.go     a test of the view, which runs without a window
├── go.mod           with the CLI as a tool
├── mygo.json        the app's name, identifier and version
└── resources/
    └── icon.png     the app icon, a 1024×1024 PNG
```

模块把 CLI 作为[工具](https://go.dev/doc/modules/managing-dependencies#tools)：`go tool mygo` 会按 `go.mod` 固定的版本运行它，`go get -tool github.com/egoist/mygo/cmd/mygo@latest` 在更新 MyGo 时会一并更新它。

## 开发

```sh
cd my-app
bun run dev        # a web frontend: runs mygo dev
go tool mygo dev   # native UI
```

`mygo dev` 构建应用的开发版并启动它，然后在你编辑 `.go` 文件、配置、图标或资源时重新构建并重启。编译失败时，上一个版本会继续运行。退出应用，或按 Ctrl+C，即可停止。

带 Web 前端时，它还会写入 `src/mygo.ts` 并启动 Vite 开发服务器，开发版应用从那里加载页面：编辑 `src/main.ts` 或 `src/style.css`，Vite 会立刻更新页面。开发版构建带有 Web 检查器：右键点击页面并选择“检查元素”（Windows 上为“检查”），或调用 `win.Page().OpenDevTools()`。

在 macOS 上，开发版应用是一个真正的应用包，名为 `My App Dev`，标识符是应用的标识符加上 `.dev`，这样它的数据、偏好设置和权限就与已安装的应用分开。

## 调用 Go

### 从页面调用

`main.go` 绑定一个 Go 值，页面可以调用它导出的方法：

```go
// Greeter is callable from the frontend: `mygo generate` turns its methods
// into typed TypeScript functions in src/mygo.ts.
type Greeter struct{}

// Greet returns a greeting for name.
func (Greeter) Greet(name string) string {
	if name == "" {
		name = "stranger"
	}
	return "Hello, " + name + "! This message comes from Go."
}

// Tick is sent to the page every second.
var Tick = mygo.NewEvent[time.Time]("tick")

func main() {
	mygo.Bind(Greeter{})
	// ...
}
```

`mygo dev` 会让 `src/mygo.ts` 保持最新；这个文件把这些方法转成带类型的函数和事件：

```ts
import { Greeter, events } from "./mygo";

const greeting = await Greeter.greet("Ada"); // Promise<string>
events.tick.on((time) => console.log(time)); // time: string
```

向 `Greeter` 添加一个方法，保存，等应用重启后就能调用它。[从前端调用 Go](bindings.md) 介绍方法能接收和返回哪些类型，以及错误、事件和安全。

### 从原生 UI 调用

原生 UI 本身就是 Go 代码，可以直接调用你的代码，无需绑定。视图是一个根据应用状态生成界面的函数，MyGo 每帧调用它来构建界面；视图通过查询元素来处理自上一帧以来的交互：

```go
func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(16).Children(func() {
		ui.TextInput(c, &a.name).Placeholder("Your name").Width(240)
		if ui.PrimaryButton(c, "Greet").Clicked() {
			a.greeting = greet(a.name) // your Go code
		}
		ui.Text(c, a.greeting)
	})
}
```

`main_test.go` 不开窗口就能点击视图、向其中输入，速度与单元测试相当：运行 `go test`。[原生 UI](ui/README.md) 介绍布局、控件、文本、输入、绘制，以及从其他 goroutine 修改状态。

## 构建

```sh
bun run build        # a web frontend: runs mygo build
go tool mygo build   # native UI
```

`mygo build` 编译应用，有前端时把 Vite 构建出的前端嵌入其中，并为当前机器的平台打包到 `build/<os>-<arch>/`：

| 平台 | 输出 |
|---|---|
| macOS | `My App.app`，以及磁盘映像 `my-app-0.1.0-arm64.dmg` |
| Windows | `My App.exe`，以及安装程序 `my-app-setup-0.1.0-amd64.exe` |
| Linux | 可执行文件 `my-app`，连同它的桌面条目和图标，以及包含 `install.sh` 的归档（该脚本会为用户安装应用），还有 `.deb` 和 `.rpm` 包 |

MyGo 不需要 cgo，所以任何机器都能为每个平台构建：

```sh
bun run build -- -platform darwin/universal,windows/amd64,linux/amd64
go tool mygo build -platform darwin/universal,windows/amd64,linux/amd64
```

生成的应用可以直接运行，但交付给用户前应进行签名：见[构建与分发](distribution.md)。

## 下一步

- [窗口](windows.md)和[应用](app.md)，了解应用的窗口与生命周期。
- [原生 UI](ui/README.md)，用 Go 构建界面；或[前端](frontend.md)，用于网页。
- [菜单与托盘](menus.md)和[桌面 API](native.md)。
- 仓库中的示例：
  - 有网页的：`examples/hello`（最小的应用）、`examples/todo`（带类型的服务和事件、持久化、对话框、菜单、多个窗口）、`examples/frameless`（自定义标题栏）和 `examples/native`（菜单、托盘图标、对话框、通知、全局快捷键、剪贴板和深色模式）；
  - 原生 UI 的：`examples/clipboard`（复制和拖动文本、HTML、按需生成的 JSON 和文件列表）、`examples/counter-native`（计数器，附带视图测试）、`examples/vibrancy`（内嵌标题栏下方的半透明侧边栏，可在其中选择材质）、`examples/effort-slider`（重绘 Web 演示中的动画滑块：字母模糊淡入、像素迸发、卡片晃动）、`examples/confetti`（canvas-confetti 的演示，彩纸飞舞时只更新绘制姿态，不触发布局）、`examples/wind`（Claude 应用 Code 标签页背后的字形纹理：风吹过时字符亮起，点击时泛起涟漪，由两个 Go 着色器绘制）、`examples/blackhole`（着色器追踪光线经过黑洞引力场的路径，呈现引力透镜下的吸积盘、多普勒增亮和星光形成的爱因斯坦环，分三遍绘制并加入泛光）、`examples/chat-glow`（Apple Intelligence 风格的发光助手：思考时窗口边缘发光，使用模糊的锥形渐变边框，以及着色器绘制的发光输入框、闪烁文字和光球）、`examples/timeline`（用同一播放头编排片头：卡片滑入，标题和各行依次跟入，支持播放、暂停、倒放和重新开始）和 `examples/gallery`（工具包导览：布局、控件、文本编辑、一万行的列表、绘制和浮层）。
