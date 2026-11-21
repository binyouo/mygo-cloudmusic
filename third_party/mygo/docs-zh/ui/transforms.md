# 变换

<!-- preview transforms 200 -->

`Translate`、`Scale` 和 `Rotate` 在视觉上改变元素及其中所有内容，而布局仍保留它原来的空间。它们作用于盒子、文本、图像、控件和自定义绘制。指针跟随实际绘制出来的内容，包括嵌套的变换和裁剪：

```go
ui.Column(c).Size(180, 110).Padding(12).Gap(8).
	Scale(1.1, 0.9).Rotate(12).Translate(40, 0).
	Children(func() {
		ui.Text(c, "Transformed controls")
		ui.TextInput(c, &app.name)
	})
```

方法链中的变换按调用顺序依次生效：这个例子先缩放，再旋转，再平移。父元素的变换在子元素的变换之后生效。缩放和旋转默认以元素边框盒的中心为原点。`TransformOrigin(0, 0)` 使用左上角作为原点；`(1, 1)` 使用右下角。也可以使用其他比例，包括盒子之外的值。负缩放会镜像翻转内容；零缩放会让元素不可见，也不再参与指针命中测试。变换值必须是有限值。

若要复用一组变换，构建一个 `Transform` 并用 `Transform` 设置它：

```go
pose := ui.Scaling(1.2, 1.2).Then(ui.Rotation(15)).Then(ui.Translation(30, 0))
ui.Box(c).Transform(pose).TransformOrigin(0, 0)
```

零值 `Transform` 是恒等变换。设置 `Transform` 会替换元素原有的变换，包括 `TranslatePercent` 设置的变换；便捷方法则会追加到已有变换之后。

`TranslatePercent(x, y)` 按元素自身宽度和高度的百分比移动元素，如同 CSS 的 `translate(-50%, -50%)`：以帧布局时它的边框盒为基准计算，因此它从第一帧起就位于正确位置，无需等待它的 `Bounds`，并在尺寸改变的那一帧跟随新尺寸。它在元素的其他变换之后生效，无论在这些变换中的哪个位置调用，如同 CSS 的 `translate` 属性，并在它的姿态变换之前；多次调用会累加。一个在其指向的点上方居中的标注，无论尺寸如何：

```go
ui.Box(c).Absolute().Left(at.X).Top(at.Y - 8).TranslatePercent(-50, -100).Children(callout)
```

过渡的 `Motion` 和 `Pose` 也按元素尺寸的百分比移动（`XPercent`、`YPercent`）。变换要在视图中、帧布局之前设置。对图标使用 `Rotate` 会保留它现有的行为：围绕中心旋转 SVG 图案。要旋转图标的整个边框盒，使用 `Transform(ui.Rotation(degrees))`。

## 斜切和 3D

`Skew(x, y)` 沿各轴斜切元素，参数以度为单位，如同 CSS 的 `skew()`。`RotateX` 和 `RotateY` 分别绕经过原点的水平轴和垂直轴进行 3D 旋转，在它们之后调用 `Perspective(distance)`，相当于从前方相距该数值 DIP 的位置观察：距离越短，透视效果越强；800 左右比较自然。不使用 `Perspective` 时，旋转后的元素只会显得更窄或更短：

```go
// A card turning over: its back, turned half a turn more, shows past
// 90 degrees, and each face hides while it is turned away.
card := ui.Box(c).Size(160, 100)
turn := float32(0)
if app.flipped {
	turn = 180
}
turn = card.AnimateTo("turn", turn, ui.Snappy)
card.Children(func() {
	front := ui.Box(c).Background(t.Surface)
	back := ui.Box(c).Background(t.Accent)
	front.Absolute().Fill().RotateY(turn).Perspective(800).BackfaceHidden()
	back.Absolute().Fill().RotateY(turn + 180).Perspective(800).BackfaceHidden()
})
```

`TranslateZ(z)` 让元素沿 Z 移动，正值朝向观察者，负值远离观察者，如同 CSS 的 `translateZ()`。在其后的 `Perspective(distance)` 中观察时，它看起来是 `distance/(distance-z)` 倍大小，以它的原点为中心：从 800 处看后退 120 DIP 的元素，是 `800/920` 倍大小。没有 `Perspective` 时，沿 Z 移动不显示任何变化，如同 CSS；移动到观察者所在位置或越过该位置，会把元素放到观察者身后，那里它不显示，也不接受指针输入。在旋转之前，它会把元素从轴上推出，如同旋转木马的各项；在旋转之后，它会把旋转后的元素向观察者抬起：

```go
// A carousel: each item out on a ring of 300 DIPs, turned by spin, the
// ring's center 300 behind the plane so that the item in front shows at
// its size. Those farther round look smaller, and each hides while the
// viewer sees its back.
for i, item := range app.items {
	angle := spin + float32(i)*360/float32(len(app.items))
	ring := ui.TranslationZ(300).Then(ui.RotationY(angle)).Then(ui.TranslationZ(-300)).Then(ui.Perspective(800))
	ui.Box(c).Absolute().Left(200).Top(40).Size(160, 100).Background(item.color).
		Transform(ring).BackfaceHidden()
}

// A stack in depth: each card 60 DIPs behind the one in front of it,
// the farthest drawn first.
for i := 2; i >= 0; i-- {
	ui.Box(c).Absolute().Fill().Radius(12).Background(t.Surface).Border(1, t.Border).
		TranslateZ(-60 * float32(i)).Perspective(800)
}
```

沿 Z 移动的元素仍按它们的顺序绘制，而不是按深度，因为每个元素都会压平到父元素的平面中：最近的最后绘制。

`Transform` 值以同样的方式组合，使用 `Skewing`、`TranslationZ`、`Translation3D(x, y, z)`（CSS 的 `translate3d()`）、`RotationX`、`RotationY` 和 `Perspective`。每个元素会把 3D 变换后的结果压平到父元素的平面中，如同 CSS 默认的 `transform-style: flat`：在 3D 中旋转的父元素会带着子元素的绘制一起旋转。当元素自身的 3D 变换让它的背面朝向观察者时，`BackfaceHidden` 会隐藏它，并使它不接受指针：判断依据是观察者在透视中看到它的位置，如同浏览器的行为，因此沿 Z 推出的项在越过观察者视线时会转向背面。

指针、裁剪、文本输入和辅助技术都像跟随 2D 变换一样跟随 3D 变换，渲染器以透视绘制它们，平滑的边缘也不例外。

## 坐标、裁剪和浮层

`Bounds()` 是上一帧变换后的外接矩形，以窗口 DIP 为单位。`LayoutBounds()` 是它在变换前的盒子。`LocalToWindow` 和 `WindowToLocal` 转换相对于该盒子的坐标点；变换没有逆变换时，`WindowToLocal` 返回 false。`PointerPosition`、`Dragged` 和 `HandleInput` 报告局部位置和向量。捕获的指针输入即使在变换后的盒子之外也保持为局部坐标。

`Draw` 和 `DrawOver` 仍接收原始布局坐标：绘制器对它们绘制的一切应用变换，包括路径、文本、图像、阴影和效果。裁剪会保留设置时的坐标系，并在嵌套元素之间组合。输入法候选窗口和辅助技术使用插入点与元素变换后的外接矩形。浮层本身不旋转，而是附着到锚点变换后的边界，布局过渡期间也是如此。

## 让变换动起来

变换既可以接受固定值，也可以接受动画值，Z 轴也不例外：保留速度的弹簧、曲线、关键帧，或跟随指针的值（见 [Motion](motion.md)）：

```go
tile := ui.Box(c).Key("tile").Size(80, 50)
target := float32(0)
if app.expanded {
	target = 120
}
tile.Translate(tile.AnimateTo("x", target, ui.Snappy), 0)
```

[过渡](transitions.md)缩放和旋转进出场中的元素，并用变换让出现的元素从另一个元素显示过的位置移动过来（`Match`）。

## 渲染

Metal、Direct3D 11 和 OpenGL 在 GPU 上对实例化几何体应用一般变换。填充、边框、阴影、图像、文本和效果保持局部坐标；背景采样会映射回窗口坐标。Metal 在变换下保留广色域颜色。

嵌套的圆角裁剪或经过变换的裁剪，在内部内容被缩放、旋转、斜切或应用透视时，会使用缓存的覆盖率遮罩。仅做平移的内容（如平移了不足一个像素的标签）不需要遮罩，裁剪方式与未变换的内容相同。若内层裁剪伸入外层裁剪的圆角区域（如被裁剪的列填满圆角卡片底部），也会使用外层圆角裁剪的遮罩，与内部内容无关。在固定裁剪区域内移动内容时，可复用遮罩，无需 CPU 重新计算像素。可简化为单个圆角矩形的固定裁剪不需要遮罩，即使内部内容经过缩放、旋转或透视也是如此。这包括紧贴圆角应用边框的侧边栏或标题栏裁剪，以及坐标含小数、完全切除某些圆角的矩形视口。抗锯齿边缘重叠，以及使用图像或渐变遮罩时，仍沿用原有的遮罩处理方式。与内容使用相同坐标系的硬裁剪（包括变换后的文本输入框）直接在片段着色器中计算，并复用祖先元素的遮罩。修改复杂裁剪时会重新计算覆盖率；调整尺寸时会就地替换纹理，不会不断累积缓存图像。

没有 GPU 时，以及静止界面的局部更新，仍由 CPU 合成器渲染。变换动画运行时，若有 GPU 则使用 GPU。被变换缩放的文本（`Scale`、`AnimatePose`、`Transition{ScaleContent}`）按其显示尺寸光栅化：字形按窗口缩放比例乘以变换的缩放比例生成，留给字形的变换只负责旋转、倾斜或移动，因此静止时放大 1.6 倍的卡片和按 1.6 倍尺寸布局的卡片一样清晰。变换只缩放和移动文本时，字形落在屏幕的整像素上，与在那里布局的文本相同。缩放比例变化时（例如动画放大一个歌词单词），字形按不低于当前比例的 2^(1/8)（约 9%）步长光栅化，整个动画只用几个尺寸，而不是每帧一个；动画停止后的下一帧再按比例本身光栅化。`Cache()` 的位图以及用 `Opacity` 淡化的组图层也按同样的比例渲染。字形在剩余的变换下进行双线性采样，变换后的文本使用灰度覆盖率。GPU 的抗锯齿在设备像素坐标中与 CPU 渲染器一致。要清晰地缩放整个窗口，请使用内容缩放（`Context.SetZoom`，见[窗口](windows.md#缩放)），而不是变换。

gallery 的 **Motion** 页面展示了嵌套的、变换后的可编辑控件、锚定的浮层、重新设定目标并抛出的弹簧、关键帧序列，以及共享元素。
