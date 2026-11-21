# 玻璃

`glass` 插件在原生 UI 中绘制 macOS 26 及更高版本的 Liquid Glass 材质：下方内容透过玻璃呈现磨砂效果，边缘附近像透过镜片一样弯曲，细边缘带有高光。插件还提供 macOS 的滚动边缘效果，让滚入栏下方的内容逐渐淡出或模糊（[滚动边缘](#滚动边缘)）；也可不使用玻璃，直接均匀模糊下方内容，或沿渐变逐渐减弱模糊（[模糊](#模糊)）。MyGo 通过插件的着色器在 GPU 或 CPU 上绘制这些效果，在 macOS、Windows 和 Linux 上外观一致。插件完全使用 Go 编写，没有 JavaScript 包，也无需调用 `mygo.Use`。`examples/liquid-glass` 展示了合并窗格和交互式按钮，以及位于系统玻璃下方的网页。

```go
import "github.com/egoist/mygo/plugins/glass"

ui.Row(c).Padding(8, 16).Gap(8).Radius(22).Material(glass.Glass{}).Children(func() {
	ui.Text(c, "On glass")
})
```

`glass.Glass` 是一种材质（`ui.Material`）：`Material` 用它代替背景填充元素，`Radius` 决定形状，子元素绘制在玻璃上。玻璃透出同一窗口中此前绘制在下方的内容，包括先绘制的元素和其他玻璃，例如用 `Absolute` 将工具栏放在滚动内容上方，就能透过工具栏看到下方内容。

```go
ui.Box(c).Fill().Children(func() {
	ui.Scroll(c).Fill().Padding(64, 16, 16).ScrollbarInsets(64, 0, 0).Children(func() { /* ... */ })
	// The toolbar floats over the content, which scrolls under it.
	ui.Row(c).Absolute().Top(12).Left(12).Right(12).Padding(6).Radius(26).Material(glass.Glass{}).Children(func() {
		// ...
	})
})
```

## 样式

- **`glass.Regular`** 是默认样式，会模糊透出的内容，在浅色模式下提亮、深色模式下压暗，让栏、按钮和面板上的内容在各种背景下都清晰可读。与 macOS 一样，窗格越大，磨砂效果越强。
- **`glass.Clear`** 几乎不模糊透出的内容，也很少调整色调，适合放在照片或视频上方，但玻璃上的内容需自行保证对比度。

`Tint` 按颜色的 alpha 值把玻璃染向该颜色，例如醒目的按钮：`glass.Glass{Tint: t.Accent}`，文本用 `t.AccentText`。`Interactive` 让玻璃在按下时稍微变大，如 AppKit 的交互式玻璃在 macOS 27 上那样：无论尺寸如何，在 150 ms 内左右各约 1.1 DIP、上下各 0.45 DIP。AppKit 还会把内容也拉伸约一个像素，这里的子元素则不会。

```go
done := ui.Row(c).Padding(8, 16).Radius(20).Material(glass.Glass{Tint: t.Accent, Interactive: true}).Children(func() {
	ui.Text(c, "Done").Bold().TextColor(t.AccentText)
})
if done.Clicked() {
	// ...
}
```

`Clarity` 的取值范围为 0 到 1，可提高侧边栏、检查器等大型窗格的透明程度，让下方内容更清楚：0（默认值）保持 macOS 原有材质；0.5 将最大模糊量从 10 DIP 降为 5 DIP，浅色模式下将黑色映射到 35%，深色模式下将白色映射到 60%，增强边缘高光，并将边缘弯曲范围扩至 48 DIP；1 接近全透明，最大模糊量为 2 DIP，浅色模式下黑色映射到 20%，深色模式下黑色映射到 10%、白色映射到 85%。0 到 0.5、0.5 到 1 两段分别在这些值之间线性插值。不超过 100 DIP 的窗格不受清晰度影响；尺寸从 100 增至 200 DIP 时，清晰度效果逐渐生效：`glass.Glass{Clarity: 0.5}`。透明玻璃本就很少调整透出内容的色调，因此清晰度只影响边缘高光和弯曲程度。

在绘制中，`glass.Paint(p, r, radius, g)` 在绘制器此前绘制的任何内容之上绘制一块玻璃。

## 合并的窗格

`glass.Group` 把一行元素放在同一个玻璃表面上，如 AppKit 的 `NSGlassEffectContainerView` 那样：几乎接触的窗格会融合为一个，整个表面周围只有一个边框，分开移动时又会彼此分离。

```go
g := glass.NewGroup(c)
g.Radius = 22
g.Gap(6).Children(func() {
	g.Pane(ui.Box(c).Size(76, 44).Center().Children(func() { ui.Text(c, "Cut") }))
	g.Pane(ui.Box(c).Size(76, 44).Center().Children(func() { ui.Text(c, "Copy") }))
})
```

`Merge`，默认 24 DIP，是窗格融合的平滑程度：它连接相距最多约 12 DIP 的窗格，0 让它们保持分开。一个组最多容纳 8 个窗格。这些元素与普通行中的一样，保留各自的背景、边框和点击区域；玻璃在它们后面绘制。

## Web 视图上的原生玻璃

在 macOS 26 及更高版本上，`Glass{Native: true}` 请求使用系统的 `NSGlassEffectView`，而不是着色器：

```go
ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(60).Radius(18).
	Material(glass.Glass{Native: true})
```

系统的玻璃放在窗口内容之下、`ui.WebView` 的页面之上，在那里采样页面，因此 webview 上方的原生 UI 工具栏能得到真正的 Liquid Glass，如 macOS 自己的工具栏那样。在其他地方，以及开启“降低透明度”时，由着色器绘制，而着色器看不到页面的任何内容。原生玻璃采样的是表面之下的内容：webview 的页面，或窗口的材质；它从不会采样帧自身绘制的元素，因为内容是在玻璃之上的一个表面。如果要在任意原生 UI 上使用原生窗格，就需要把帧绘制到两个表面中，让玻璃夹在它们之间，而 MyGo 不这样做；这类情况由着色器绘制。

## 辅助功能

启用“降低透明度”后，`Glass`、`Blur` 和 `ScrollEdge` 改为绘制不透明表面：玻璃使用主题的 `Surface`，模糊和滚动边缘使用主题的 `Background`。启用“提高对比度”后，会增强玻璃边缘高光，并为不透明玻璃添加边框。

## 滚动边缘

`glass.ScrollEdge` 是 macOS 26 及更高版本的滚动边缘效果，一种材质，用于盖在内容边缘之上的元素：内容会在控制栏下方滚动，这层材质让控制栏从内容中突显出来，就像 SwiftUI 用 `scrollEdgeEffectStyle` 绘制 `safeAreaBar` 那样：

```go
ui.Box(c).Fill().Children(func() {
	ui.Scroll(c).Fill().Padding(64, 16, 16).ScrollbarInsets(64, 0, 0).Children(func() { /* ... */ })
	// The content fades into the background under the bar.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(74).PassThrough().Material(glass.ScrollEdge{})
	// Buttons of glass float over it.
	ui.Row(c).Absolute().Top(12).Left(12).Right(12).Gap(6).PassThrough().Children(func() {
		// ...
	})
})
```

- **Soft** 是默认样式，通过渐变背景遮盖滚动内容：栏边缘处的背景不透明度为 85%，向元素另一边缘逐渐降至零，不做模糊。与 macOS 一样，将元素高度设为栏高加 10 DIP。
- **`Hard`** 均匀地磨砂栏下的内容：模糊约 6 DIP，颜色饱和度提高四分之一，再叠上 82% 的背景，沿元素的另一边缘有一条细线，浅色模式下是 10% 的黑色，深色模式下是 7% 的白色。让元素的高度为栏的高度。

`Bottom` 指定栏位于元素底部，`Background` 设置滚动内容后方的背景，默认为主题的 `Background`。应先绘制边缘效果，再绘制上方控件，并用 `PassThrough` 让指针事件穿透到内容。

这些数值依据 macOS 27 上 SwiftUI 对测试图案的绘制结果测得：软边缘图层通过不透明度从 85% 降至零的渐变遮罩叠加窗口背景，范围超出栏 10 点；硬边缘先模糊内容（高斯半径 6，饱和度乘以 1.25 后再加 0.03），再以 82.45% 的不透明度叠加背景。浅色和深色模式下，灰色测试内容的每一行与 AppKit 的差异都不超过 1/255；高饱和度颜色最多相差 0.05，因为 AppKit 在显示器的更广色域中混合，而 MyGo 渲染器使用 sRGB。macOS 窗口工具栏另有自己的边缘效果，即均匀模糊后叠加 60% 的背景；原生 UI 的栏不自带此效果。

## 模糊

`glass.Blur` 是背景模糊，也是一种材质：元素下方的内容透过它显示，模糊量为 `Radius` DIP（标准差，如 CSS 的 `blur()` 所取），没有着色、边缘或阴影。

`Mask` 使它成为渐进模糊：一个 `LinearGradient`，其 alpha 表示每个位置模糊 `Radius` 的多少，如 CSS 的 `mask-image` 那样。渐变不透明的地方，透过它显示的内容按 `Radius` 模糊，半透明的地方模糊得更少，透明的地方完全不模糊；它的颜色不显示。在栏下，内容因此在它的边缘处模糊得最多，离它越远越少。

```go
ui.Box(c).Fill().Children(func() {
	ui.Scroll(c).Fill().Padding(64, 16, 16).ScrollbarInsets(88, 0, 0).Children(func() { /* ... */ })
	// What scrolls under the toolbar blurs, the more the nearer the top.
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(88).PassThrough().
		Material(glass.Blur{Radius: 6, Mask: &ui.LinearGradient{
			From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180, Start: 0.3, End: 1,
		}})
	// Buttons of glass float over it.
	ui.Row(c).Absolute().Top(12).Left(12).Right(12).Gap(6).PassThrough().Children(func() {
		// ...
	})
})
```

这里，模糊在条带顶部 30% 的范围内完全生效，并在下方逐渐淡出；`PassThrough` 让指针到达条带下方和按钮周围的内容，`ScrollbarInsets` 让滚动条从条带下方开始，如 AppKit 在其工具栏的边缘效果下方开始滚动条。在浮在它上面的内容之前绘制模糊：其上的玻璃会透出模糊后的内容。`Start` 和 `End` 沿渐变放置它的颜色，与其他地方一样；让 `End` 保持 0 会把两者都放在 `Start`，形成一个硬边缘。

变化的模糊按步骤绘制：每一步都把上一步的结果再模糊一些，在所需模糊量大于该步自身模糊量的位置显示，并在两步之间与下一步混合；从两个像素起，模糊量每翻一倍加一步，在 Retina 显示器上 12 DIP 对应五步。两步之间的像素显示两种模糊的混合，如 Core Animation 的可变模糊在其金字塔的层级之间那样，也如 web 上渐进模糊堆叠的 `backdrop-filter` 图层那样。如果渐变不透明的位置在窗口中部某个元素的边缘，而不是窗口边缘，那么该边缘几个像素内的模糊，会与逐像素按各自模糊量处理的结果略有不同，因为每一步读取的都是元素周围未模糊的内容。

## 它如何做到像 macOS

玻璃以 macOS 27 的玻璃为准，依据 AppKit 的 `NSGlassEffectView` 测量，并采用 Liquid Glass 开源复刻版本的光学特性：

- **边框。** 表面在中间是平的，并沿 Apple 的方圆形轮廓在最后 36 DIP 内向下弯曲到边缘，最多为窗格的一半。垂直向下的光进入时折射，如同穿过折射率为 1.5 的玻璃，因此边缘附近的内容来自更靠内的位置：边缘反射它内侧紧邻的内容，如镜片那样。
- **材质。** 普通玻璃把透过它显示的内容最多模糊 10 DIP，更大的窗格更多，并映射其亮度，浅色模式下黑色到 54%、白色到 100%，深色模式下到 15% 和 51%，同时保持其颜色。清晰的玻璃在此基础上再加八分之一。
- **光。** 在距边缘一个像素以内，边缘朝上或朝下处受光，朝侧面处则带阴影：形成一条细线，与 AppKit 的一样。
- **没有阴影。** `NSGlassEffectView` 和玻璃边框中的按钮都不投射阴影：窗格的边缘就只有一圈边沿。

## 性能

每个窗格读取下方内容，求平均并模糊，再用各自的着色器绘制；GPU 上只需几次小范围渲染。下方内容变化时（例如在栏下滚动），窗格也需更新。窗口在没有 GPU 的情况下使用 CPU 绘制时（见[渲染](../ui/rendering.md)），下方内容的变化会触发窗格及其周围模糊影响范围的重绘。渐进模糊的每个步骤都会在其可见区域执行这些工作：在 M5 的 CPU 上，对 1360×176 像素条带做模糊量为 24 的渐进模糊耗时 9 ms，均匀模糊耗时 1.7 ms；在 GPU 上，每个步骤只需几次小范围渲染。

单个材质是一个圆角矩形。`glass.Group` 最多把 8 个这样的窗格合并起来，如 AppKit 的 `NSGlassEffectContainerView` 那样。
