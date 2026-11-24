# 过渡

<!-- preview transitions 230 -->

`Transition` 让元素从上一帧的状态平滑过渡到当前布局指定的状态，包括在父元素中的位置、尺寸、背景色、边框色和阴影。视图只需描述目标状态，过渡负责生成中间的动画。

```go
var rowMotion = ui.ElementTransition{
	Enter: &ui.Motion{Collapse: true},
	Exit:  &ui.Motion{Collapse: true},
}

ui.Column(c).Children(func() {
	removed := -1
	for _, it := range app.items {
		ui.Row(c).Key(it.ID).Padding(8).Transition(rowMotion).Children(func() {
			ui.Text(c, it.Title).Grow(1)
			if ui.Button(c, "Remove").Clicked() {
				removed = it.ID // once the loop is done (see Views)
			}
		})
	}
	if removed >= 0 {
		app.remove(removed)
	}
})
```

新增的行展开出现，移除的行收起，其余的行滑到自己的位置，无论条目是新增、移除、排序还是打乱。

## 移动的是什么

`ElementTransition` 的动画持续时间由 `Duration` 指定（零值为 200 ms，负值表示立即完成，见下文），缓动由 `Ease` 指定（nil 时使用 `ui.EaseOut`，也可使用任意[缓动函数](drawing.md#动画)）。设置 `Animation` 后，则按其中的配置播放：使用 `ui.Snappy` 等弹簧动画时，即使途中改变目标位置，元素也会保持当前速度、平滑转向（见[动效](motion.md)）。

- **位置**：元素在父元素中的位置。随父元素移动，例如滚动视图滚动时，不算变化。
- **大小**：元素的宽度和高度。动画中每次尺寸变化都会重新排版内容，例如面板展开时文字会重新换行。用 `Width` 或 `Height` 代替大小动画，可只让一个维度过渡，类似 CSS 的 `transition-property: height`；另一个维度会立即采用布局指定的值。
- **颜色**：它的背景色和边框色，例如悬停淡入淡出时。它们在 sRGB 中混合，或在 `Animation` 设置了 `Oklab` 时在 Oklab 中混合，后者混合出的中间色保持颜色的明度和饱和度，而不会经过两个饱和色之间一个暗淡、更深的中间色：`Animation: &ui.Animation{Duration: 200 * time.Millisecond, Oklab: true}`。
- **阴影**：`Shadow` 和 `InsetShadow` 的偏移、模糊、扩散和颜色，类似 CSS 的 `box-shadow` 过渡。各阴影从上一帧相同序号的阴影过渡而来；新增阴影从零偏移的透明阴影开始，移除阴影则过渡到该状态。相同序号的阴影在内阴影与外阴影之间切换时，会立即改变，不做过渡。

这四类都会做动画，除非用 `Position`、`Size`（或 `Width`、`Height`）、`Colors` 或 `Shadows` 指定其中一部分：

```go
tile := ui.Box(c).Size(64, 48).Radius(8).Background(t.Surface)
if tile.Hovered() {
	tile.Background(t.SurfaceHover)
}
tile.Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})

// A button whose resting shadow fades as the pointer comes.
b := ui.ButtonBase(c).Padding(8, 14).Radius(8).Background(t.Surface)
if !b.Hovered() {
	b.Shadow(0, 2, 6, 0, ui.RGBA(0, 0, 0, 0.2))
}
b.Transition(ui.ElementTransition{Shadows: true, Duration: 150 * time.Millisecond}).
	Children(func() { ui.Text(c, "Save") })
```

元素必须在各帧中保持同一 ID：使用相同的 `Key`，或保持在兄弟元素中的位置不变。过渡只作用于元素本身；受其变化影响而移动的其他元素，也需要各自设置过渡。要像上面的各行一样同步移动，就使用相同的过渡配置；容纳这些行的列如果也会改变大小，也需要设置过渡：

```go
open := app.sidebar
w := float32(56)
if open {
	w = 240
}
ui.Row(c).Fill().Children(func() {
	ui.Column(c).Width(w).ClipX().Transition(ui.ElementTransition{Size: true})
	ui.Column(c).Grow(1).Transition(ui.ElementTransition{})
})
```

高度跟随内容的面板，例如一个详情区域，在用户把窗格拖宽或拖窄时，只让高度做动画：它弹开，而宽度逐帧跟上拖动。

```go
ui.Row(c).Width(app.paneWidth).Children(func() { // follows a drag handle
	ui.Column(c).Key("details").Grow(1).
		Transition(ui.ElementTransition{Height: true, Animation: &ui.Snappy}).
		Children(func() {
			ui.Text(c, "Details")
			if app.open {
				ui.Text(c, app.body)
			}
		})
})
```

`Animation` 中的 `Delay` 会把变化推迟一段时间：按索引递增的延迟会让列表的行依次错开：

```go
for i, it := range app.items {
	a := ui.Snappy
	a.Delay = time.Duration(i) * 25 * time.Millisecond
	ui.Row(c.Key(it.ID)).Transition(ui.ElementTransition{Animation: &a, Enter: &ui.Motion{Y: 8}})
}
```

`Transitioning()` 报告元素在上一帧中是否仍在进行过渡动画。

## 出现与离开

`Enter` 是元素出现时的起点，`Exit` 是它离开时的去向，都从它在布局中的位置算起：一个 `Motion` 会按 `X` 和 `Y` 指定的 DIP 数，以及自身宽度和高度的 `XPercent`、`YPercent` 百分比移动，处于 `Opacity` 指定的不透明度（未设置时为 0，即透明），按 `Scale` 缩放（0 保持它的大小），绕 `TransformOrigin` 旋转 `Rotate` 度，按 `Blur` 指定的 DIP 模糊（与 [`Blur`](styling.md) 的模糊一样）；使用 `Collapse` 时，在列中没有高度、在行中没有宽度，内容会被裁剪。使用 `Flip` 时，附着在另一个元素上的元素（[`AttachTo`](overlays.md#你自己的浮层)）会在布局把它放在目标另一侧时，镜像 `X` 与 `XPercent`，或 `Y` 与 `YPercent`：一个从按钮下方 `Y: -8` 处降下的菜单，在只有上方有空间时，会从 8 处升起，从它显示的第一帧起就这样。

```go
toast := ui.ElementTransition{
	Enter: &ui.Motion{Y: 12},
	Exit:  &ui.Motion{Opacity: 1, X: 300},
}
popup := ui.ElementTransition{
	Animation: &ui.Snappy,
	Enter:     &ui.Motion{Scale: 0.95},
	Exit:      &ui.Motion{Scale: 0.95},
}
// Text or an icon crossing over to another through a short blur, and a
// sheet sliding in by its own height, however tall it is.
swap := ui.ElementTransition{
	Enter: &ui.Motion{Y: 4, Blur: 4},
	Exit:  &ui.Motion{Y: -4, Blur: 4},
}
sheet := ui.ElementTransition{
	Animation: &ui.Snappy,
	Enter:     &ui.Motion{Opacity: 1, YPercent: 100},
	Exit:      &ui.Motion{Opacity: 1, YPercent: 100},
}
```

元素只有在上一帧就已存在的父元素里，才会带着它的 `Enter` 出现，这样一次性显示的页面或对话框不会让它里面的每个元素都做动画。它出现期间，布局对它的移动（比如有行落在它上方）会在过渡不负责动画的方面（`Position`、`Size`、`Width`、`Height`）立即生效；改变它的大小会按比例改变它的 `Collapse`：只播放它的 `Enter`。父元素仍在、元素自己却不再被构建时，它的一个副本（如上一帧显示的那样）会在兄弟元素下方走向它的 `Exit`：它不占空间，所以兄弟元素立刻移到它的位置，如果它们有过渡就平滑移动；它既不接收指针，也不接收焦点，也不会被辅助技术读取，就像一个同时有 `IgnoreInput` 和 [`AccessHidden`](accessibility.md#不挡辅助功能的路) 的元素。副本保留元素的 [z-index](layout.md#绘制顺序)，并且只位于 z-index 相同的兄弟元素之下：用 `ZIndex` 抬起的对话框会在页面上方淡出，而不是在它下方。在退出结束前重新构建时，元素从副本所在的位置回来。

### 边走边定的离开

副本默认使用元素最后一次构建时的 `Exit`。有时退出方向要到移除元素时才能确定，例如日历、向导或轮播前进或后退时，当前页面应按这次切换的方向退出。此时可用父元素的 `ChildExit` 覆盖本帧移除的子元素的退出目标；只有设置了 `Exit`、移除时会保留副本的子元素受影响。已经开始退出的副本不变。

```go
dir := float32(app.dir) // 1 forward, -1 back, set as the step changes
page := ui.ElementTransition{
	Animation: &ui.Snappy,
	Enter:     &ui.Motion{X: 40 * dir},
	Exit:      &ui.Motion{X: -40 * dir},
}
ui.Box(c).Grow(1).ChildExit(ui.Motion{X: -40 * dir}).Children(func() {
	ui.Column(c.Key(app.step)).Absolute().Fill().Transition(page).Children(app.stepView)
})
```

否则，切换方向改为后退的那一帧，原先按前进方向构建的页面仍会向前退出。

### 一次一个

默认情况下，退出和进入的元素同时显示，动画会相互交叠。`AfterExits` 可让新元素等待兄弟元素的副本退出：新元素先占据布局空间但保持不可见，等副本消失后再播放 `Enter`。这样父元素一次只显示一个子元素，例如先让一个面板退出，再显示下一个：

```go
panel := ui.ElementTransition{
	Enter:      &ui.Motion{Y: 8},
	Exit:       &ui.Motion{Y: -8},
	AfterExits: true,
}
ui.Box(c).Grow(1).Children(func() {
	ui.Column(c.Key(app.tab)).Absolute().Fill().Transition(panel).Children(app.tabView)
})
```

如果它在显示之前就消失了，这个被推迟的元素不会留下任何痕迹。

### 给退出定时

`Exit` 默认与其他变化使用相同的动画配置，可用 `ExitDuration` 或 `ExitEase` 覆盖：例如弹层进入用 150 ms、退出用 100 ms，类似 CSS 在不同状态下设置不同的 `transition-duration`。只设置 `ExitDuration` 时保留 `Ease`，只设置 `ExitEase` 时保留 `Duration`；设置任一项，都会将 `Animation` 中的弹簧退出动画改为曲线动画。`ExitDuration` 为零时，时长约等于弹簧动画，并在 `Animation` 的 `Delay` 后开始：

```go
popup := ui.ElementTransition{
	Duration:     150 * time.Millisecond,
	ExitDuration: 100 * time.Millisecond,
	ExitEase:     ui.EaseIn,
	Enter:        &ui.Motion{Scale: 0.96},
	Exit:         &ui.Motion{Scale: 0.96},
}
```

`ExitDelay` 是 `Exit` 出发前等待多久，就像 CSS 的 `transition-delay` 在两个状态之间不同。为零保留 `Animation` 的 `Delay`，和其他变化一样等待；负值不等待，所以一个接一个进入、每个 `Delay` 更长的行会一起离开：

```go
row := ui.ElementTransition{
	Animation: &ui.Animation{Duration: 200 * time.Millisecond, Ease: ui.EaseOut, Delay: 80 * time.Millisecond},
	ExitDelay: -1,
	Enter:     &ui.Motion{Y: 8},
	Exit:      &ui.Motion{Y: 8},
}
```

负的 `Duration` 完全不花时间：每个变化立刻显示，`Enter` 和 `Exit` 也一样，元素离开时不留下副本，就像自带减少动效设置的应用那样；负的 `ExitDuration` 只对 `Exit` 这样做。为零的 `Duration` 是 200 ms：

```go
if app.settings.ReduceMotion {
	popup.Duration, popup.ExitDuration = -1, -1
}
```

## 共享元素

`Match` 为两个元素指定共享标识，例如网格中的照片缩略图和详情视图中的照片。如果新出现的 `Match` 元素与上一帧中的另一个元素具有相同的 `Match`，就会从后者的位置、大小和颜色过渡到自身状态，即使两者的父元素不同，类似 SwiftUI 的 `matchedGeometryEffect` 和 Motion 的 `layoutId`：

```go
type photoMatch struct{ id int }

if app.open == nil {
	for _, p := range app.photos {
		thumb := ui.Image(c.Key(p.ID), p.Thumb).Size(64, 64).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{p.ID}})
		if thumb.Clicked() {
			app.open = p
		}
	}
} else {
	ui.Image(c.Key("hero"), app.open.Full).Size(320, 240).
		Transition(ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{app.open.ID}})
}
```

使用自定义类型作为标识，避免与其他标识冲突。新元素从原元素的边界框过渡到自己的边界框，内容会像 `Size` 动画一样随尺寸变化重新排版，因此内容和圆角都不会被拉伸。同一时刻只给一个元素使用某个 `Match`；原元素如果有自己的 `Exit`，仍按该配置退出。

### 飞行

`Fly` 让出现的元素从另一个那里飞过来，就像 Keynote 的 Magic Move 和 Flutter 的 Hero：

```go
tr := ui.ElementTransition{Animation: &ui.Snappy, Match: photoMatch{p.ID}, Fly: true}
```

在途中，出现的元素：

- 从原元素的边界框移动到自己的边界框，内容随尺寸变化重新排版（或用下面的 `ScaleContent` 缩放）；
- 从另一个的外观变到自己的：它的背景色和边框色、圆角、不透明度、文字颜色，以及它的变换，沿较短的方向旋转，在 3D 和透视中也是如此，就像 CSS 插值变换那样；还会从另一个周围元素的变换（例如一个被缩放的容器）过渡到它周围元素的变换；
- 绘制在窗口其他内容之上；如果位于对话框等浮层中，则绘制在该浮层的其他内容之上、后续浮层之下，并在实际绘制的位置接收指针输入。它不受周围滚动视图和容器的裁剪，盖在后续元素上，不透明度只由自身决定，不受祖先元素影响，主题则取自外层的 `Themed`；
- 把它的内容作为一个整体平滑移动，而不是让文本一行一行地逐像素移动，到达之后就像静止的文本那样；
- 如果原元素也设置了 `Fly`，新元素会在其上方淡入，原元素同步移动、随边界框变化重新排版并淡出。新元素在动画过半时完全不透明，原元素在后半段淡出，类似 Motion 的交叉淡入淡出。原元素已从视图移除时，会保留副本完成动画，不使用自身的 `Exit`；如果仍被构建，例如处于正在退出的页面中，则直接移动原元素。

它在途中随周围的元素一起被移除时，例如它的视图带着 `Exit` 关闭，元素就在它们的副本中继续飞：从它显示的位置到那个副本让它到的位置，随副本一起淡出。

### 其余一切

Keynote 的 Magic Move 会在共享元素移动时，让幻灯片中其余内容淡入淡出。要在两个视图之间实现此效果，可在保持不变的父元素中为各视图设置 `Key`，并配置淡入的 `Enter` 和淡出的 `Exit`：共享元素在两者之间飞行，其余内容淡出或淡入。退出视图的副本不再绘制已经参与共享飞行动画的元素。

```go
fade := ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{}, Exit: &ui.Motion{}}
ui.Box(c.Key("stage")).Grow(1).Children(func() {
	if app.open == nil {
		ui.Column(c.Key("grid")).Absolute().Fill().Transition(fade).Children(app.grid)
	} else {
		ui.Column(c.Key("detail")).Absolute().Fill().Transition(fade).Children(app.detail)
	}
})
```

[`Router`](navigation.md#过渡) 用 `TransitionMatch` 为它的页面做到这一点。

### 缩放的内容

`ScaleContent` 通过从元素自己的大小缩放来表现它的大小变化，内容只排版一次，而不是在每个大小上排版：文本保持行数，随元素一起放大或缩小，例如一个小标题变成大标题，图片则随它的盒子拉伸。它会在 `Match` 的元素移动途中缩放它，也会缩放 `Size` 的变化。

### 变形的文字

文本同时设置 `Match` 和 `ContentTransition{Morph: true}` 后，飞行时会从具有相同 `Match` 的元素的文本逐字变形：共有字符移动到新位置，过渡到新字号和颜色；其余字符随文本移动，旧字符淡出、新字符淡入，类似 Keynote 的按字符 Magic Move。边界框与其他 `Match` 元素一样飞行，文本则按自身尺寸排版一次，再移入框内。如果尚未到达就由原元素或同一 `Match` 的其他元素接续动画，会从字符当前的位置继续变形。富文本也支持此效果：字符过渡到各自文本段的颜色，下划线、删除线和高亮则淡出或淡入：

```go
type titleMatch struct{ id int }
title := ui.ElementTransition{Animation: &ui.Snappy, Match: titleMatch{p.ID}, Fly: true}
morph := ui.ContentTransition{Morph: true}

// In the grid.
ui.Text(c, p.Title).FontSize(12).Transition(title).ContentTransition(morph)
// In the detail view.
ui.Text(c, p.Title+" in "+p.Place).FontSize(24).Bold().Transition(title).ContentTransition(morph)
```

`Morph` 也会让原地变化的文本变形（见 [Motion](motion.md#变化的内容)）。

## 什么都不动的时候

改变窗口大小引起的变化不做动画。当桌面要求更少的动效（`c.Preferences().ReduceMotion`）时，什么都不动：元素立刻显示在布局放置它们的地方，共享元素也不会飞行。颜色和阴影仍然平滑改变；一个只淡入淡出或模糊、而不收起的 `Enter` 或 `Exit` 会就地淡入淡出、模糊，正如 Apple 的指南所建议的；会收起的那个则会立刻完成，因为它会移动兄弟元素。只有在有东西移动时才会绘制帧。

[`List`](list.md) 的行由列表放置，列表只构建视口中的那些：它们的内容可以有过渡，但行本身不会随之滑动、出现或离开。
