# 游戏

游戏插件（`plugins/game`）提供游戏在普通应用功能之外所需的功能：MyGo 是应用框架，因此这些功能放在独立的包中，按需启用。全部用 Go 编写。原生 UI 绘制游戏的方式与其他内容相同：`game` 包在元素中运行游戏循环，并通过摄像机观察游戏世界，`game/gamepad` 包读取游戏手柄输入，核心包负责绘制像素画（[像素化位图](../ui/image.md)和[位图局部](../ui/drawing.md#位图的部分)）和播放声音（[`audio`](../audio.md)）。

[examples/mayoi](../../examples/mayoi) 是一个小型幸存者类游戏，以动画 MyGO!!!!! 为题材，MyGo 的名字正来源于此；它用到了上述全部功能，编译成 WebAssembly 后可以在这里游玩。点击游戏，然后用 WASD 或方向键移动：Tomori 会在怯场时独自唱歌、捡起石头，每次升级都可能有乐队成员加入，并在音乐中加入各自的声部。

<!-- app mayoi 480 -->

[examples/farm](../../examples/farm) 更平静，是一个可以耕种和浇水的农场。

## 游戏循环

`game.Game` 有两个方法：`Update` 根据玩家的输入把世界推进一步，`Draw` 把世界绘制出来。`game.View` 在元素中运行游戏：

```go
import "github.com/egoist/mygo/plugins/game"

type farm struct {
	x, y   float32 // the player, in the world
	px, py float32 // where they were a step before
}

func (f *farm) Update(in *game.Input) {
	f.px, f.py = f.x, f.y
	if in.Down(ui.KeyD) {
		f.x += 2
	}
	if in.Pressed(ui.KeySpace) {
		f.jump()
	}
}

func (f *farm) Draw(p *ui.Painter, r ui.Rect, alpha float64) {
	// Between the last two steps, as far as the frame is into the next.
	x := f.px + (f.x-f.px)*float32(alpha)
	y := f.py + (f.y-f.py)*float32(alpha)
	p.ImageRegion(sheet, hero, ui.Rect{X: r.X + x, Y: r.Y + y, W: 48, H: 48}, ui.ImageOptions{})
}

mygo.NewWindow(mygo.WindowOptions{
	Title:   "Farm",
	Content: ui.View(func(c *ui.Context) { game.View(c, f).Fill() }),
})
```

游戏以固定时间步长更新，每秒 `game.Rate`（60）步，也可由游戏的 `Rate() int` 方法指定，与显示器刷新率无关，这与 Ebitengine 和 MonoGame 的做法相同：游戏在 60 Hz 和 120 Hz 的显示器上表现一致，物理模拟保持稳定。`Draw` 每帧运行；`alpha` 的范围是 0 到 1，表示当前帧在下一个时间步内的进度，用于在最近两次更新的位置之间插值绘制。暂停后（例如窗口不可见时），最多补算四分之一秒的更新。不限制帧率时（`MYGO_UNCAPPED=1`，见[帧节奏](../ui/rendering.md#帧节奏)），即使每秒绘制数千帧，更新步数仍不变，只是在步进之间更频繁地调用 `Draw`。

### 输入

`game.Input` 提供当前更新步所读取的玩家输入：

| 方法 | 报告 |
|---|---|
| `Down(k)` | 键 `k` 被按住 |
| `Pressed(k)`, `Released(k)` | `k` 自上一步以来被按下或松开，即使发生在两步之间 |
| `Pointer()` | 指针相对于视图角落的位置，以及指针是否在视图上 |
| `PointerDown(b)`, `PointerPressed(b)`, `PointerReleased(b)` | 指针按钮 `b`（0 是主按钮） |
| `Wheel()` | 自上一步以来滚轮滚动的距离 |
| `Size()` | 视图的大小 |
| `Focused()` | 视图具有键盘焦点：失去焦点时游戏会暂停 |

按键按位置识别：无论哪种键盘布局，`KeyW` 都是 W 所在位置的键，WASD 正需要这样。视图出现时和被点击时获得键盘焦点，把带 ⌘、Ctrl 或 Alt 的按键留给菜单的快捷键，并在窗口失去键盘焦点时松开所有按住的键——这些键的松开事件永远不会到来。在视图之外，例如显示游戏按键速查表时，[`c.KeyDown`](../ui/input.md#按住的键) 会报告窗口中按住的键，无论哪个元素拥有焦点。

### 测试

游戏的 `Update` 就是普通 Go 代码：不用窗口也能测试游戏世界。视图和其他元素一样在 `ui.Tester` 中运行：用 `KeyDown` 和 `KeyUp` 让按键在帧之间保持按下；新帧到来时，若距上次更新已过一个时间步，就会执行一次更新。

## 摄像机

`game.Camera` 对准世界中的点 `X, Y`：该点显示在视图中央，放大 `Zoom` 倍，并旋转 `Rotation` 度，与 raylib 的 `Camera2D` 相同。`Draw` 通过它用世界自己的坐标绘制世界，并裁剪到视图：

```go
cam := game.Camera{X: player.x, Y: player.y, Zoom: 3}
cam.Draw(p, r, func() {
	for _, t := range tiles {
		p.ImageRegion(sheet, t.src, t.box, ui.ImageOptions{}) // culled where the camera does not look
	}
})
wx, wy := cam.ToWorld(r, px, py) // the pointer, in the world
```

不旋转时，世界按屏幕整像素移动，因此以整数单位绘制的图块边缘会与像素对齐；位图局部则按变换后的实际位置绘制，可以平滑移动。`Visible` 返回视图中可见的世界区域，供自行遍历图块的游戏使用；`Transform` 返回摄像机的变换，供 `Painter.Transform` 使用。

整数缩放让 `Pixelated` 位图的每个像素占据相同整数个屏幕像素；其他缩放让像素保持方形，边缘在一个屏幕像素上混合。要在任意窗口大小下得到整数倍，请绘制到画布中（见下文）。

## 像素级精确缩放

对于在任意窗口大小下都像素级精确的像素画，如 Godot 的视口拉伸和 Stardew Valley 绘制的那样，游戏把世界绘制到一个逻辑尺寸的 [`ui.Canvas`](../ui/drawing.md#画布) 中，并以适合的最大整数倍显示它，带黑边（`game.IntegerFit`）：

```go
world := ui.NewCanvas(480, 270)
world.Bitmap().Pixelated()

func (f *farm) Draw(p *ui.Painter, r ui.Rect, alpha float64) {
	world.Draw(p, func(cp *ui.Painter) {
		cam := game.Camera{X: f.x, Y: f.y}
		cam.Draw(cp, ui.Rect{W: 480, H: 270}, f.drawWorld)
	})
	dst, _ := game.IntegerFit(p, 480, 270, r)
	p.Image(world.Bitmap(), dst, ui.FillBox)
}
```

## 游戏手柄

`plugins/game/gamepad` 包读取游戏控制器，适用于原生 UI 应用和网页，覆盖 MyGo 运行的每个平台，包括浏览器。无论哪种手柄，都按同一种布局读取，即 W3C Gamepad API 和 SDL 游戏控制器数据库的标准布局：按位置排列的按钮、两个摇杆、模拟扳机和方向键。

```go
import "github.com/egoist/mygo/plugins/game/gamepad"

var last gamepad.State

func update() {
	pads := gamepad.Pads()
	if len(pads) == 0 {
		return
	}
	s := pads[0].State()
	x, y := s.LeftStick() // -1 to 1, right and down, with a dead zone
	player.move(x, y)
	if s.JustPressed(last, gamepad.South) {
		player.jump()
	}
	last = s
}
```

### 手柄

`Pads` 按连接顺序返回已连接的手柄，并且最多每秒查找一次新设备；游戏每帧调用一次。`Pad` 从连接时起存在，直到断开（`Connected` 变为 false，它离开 `Pads`）；重新插入后，它会是一个新的手柄。`Name` 是制造商对它的称呼。

`State` 以值的形式读取手柄当前各个控件的状态：保留上一个状态，就能区分刚刚按下（`JustPressed`）或松开（`JustReleased`）的按钮和一直按住的按钮。

### 布局

| 按钮 | Xbox | PlayStation | Nintendo |
|---|---|---|---|
| `South` | A | Cross | B |
| `East` | B | Circle | A |
| `West` | X | Square | Y |
| `North` | Y | Triangle | X |
| `LeftShoulder`, `RightShoulder` | LB, RB | L1, R1 | L, R |
| `LeftTrigger`, `RightTrigger` | LT, RT | L2, R2 | ZL, ZR |
| `Back`, `Start` | View, Menu | Create, Options | −, + |
| `LeftStickButton`, `RightStickButton` | LS, RS | L3, R3 | |
| `Up`, `Down`, `Left`, `Right` | d-pad | d-pad | d-pad |
| `Guide` | Xbox | PS | Home |

按钮的读数范围为 0 到 1（`Value`），扳机的值表示按压程度；达到四分之一时，`Pressed` 为真。轴（`Axis`）按手柄报告的值读取，范围为 -1 到 1，向右和向下为正；`LeftStick` 和 `RightStick` 应用径向死区（`Deadzone`，0.15），使摇杆静止时读数为零，并在各方向上都能达到最大值。

### 平台

| 平台 | 来源 | 手柄 |
|---|---|---|
| macOS | GameController.framework | Xbox、PlayStation、Switch Pro 和 MFi 控制器 |
| Windows | XInput | Xbox 控制器及兼容控制器（Steam Input 可将其他手柄映射为兼容设备）；Guide 通过 `XInputGetStateEx` |
| Linux | evdev，`/dev/input/event*` | 内核驱动报告为游戏手柄的设备，当前座席上的用户可读；Xbox 控制器的 X 和 Y 报告为 BTN_X 和 BTN_Y，按位置读取 |
| 浏览器 | Gamepad API | 具有标准映射的手柄；其他手柄按浏览器给出的顺序 |

在 macOS 上，手柄的输入只到达前台应用：应用在后台时读不到任何按下的键，正如游戏所期望。浏览器只有在页面看到手柄的某个按钮被按下后才会报告该手柄。没有 Steam Input 或让它变成 XInput 设备的驱动，Windows 看不到 PlayStation 或 Nintendo 控制器。

[examples/farm](../../examples/farm) 既可以用游戏手柄玩，也可以用键盘玩。
