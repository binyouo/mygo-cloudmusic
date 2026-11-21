# 图像

<!-- preview image 240 -->

`ui.Image` 显示一个位图，或按自身颜色显示 SVG，默认按它的 DIP 尺寸显示，指定其他尺寸时缩放以适应。

```go
//go:embed logo.png
var logoPNG []byte

var logo, _ = ui.DecodeBitmap(logoPNG)

ui.Image(c, logo).Size(64, 64).Fit(ui.Contain).Radius(12)
```

## 位图

`ui.NewBitmap` 用 `image.Image` 创建位图，`ui.DecodeBitmap` 用 PNG、JPEG、GIF（其第一帧）、WebP 或 BMP 数据创建，并按照片相机的 EXIF 方向把照片摆正。只创建一次位图，不要在视图中创建：只要你使用它，MyGo 就把位图保留在 GPU 上。显示尺寸远小于其像素的位图（例如缩略图中的照片）会从一个副本绘制：该副本反复减半，直到再减半就会小于显示尺寸为止，且只创建一次，因此每个像素都能参与显示，而不会闪烁。

位图会同时在 CPU 和 GPU 上保留像素，照片较多的应用需要留意这部分内存。位图显示过之后，`ReleasePixels` 可释放原图及各级减半图像在 CPU 上的像素副本：仍可从 GPU 正常绘制；需要时再重新生成像素，例如 GPU 丢失纹理或因连续多帧未使用而释放纹理、使用 CPU 渲染器，或需要生成更小的减半副本时；上传到 GPU 后，CPU 上的像素会再次释放。由 `DecodeBitmap` 创建的位图会从原始数据重新解码，保留这些数据所需的内存远少于像素数据。由 `NewBitmap` 创建的位图会重新转换原来的 `image.Image`；调用 `ReleasePixels` 后会直接保留该图像，不再复制：如果应用本就需要保留图像，例如以原尺寸处理的截图，内存中便只需保留一份像素，而非两份。两种情况下，原始数据或图像都必须保持不变。如果应用不再保留 `NewBitmap` 所用的图像（或图像类型并非来自 `image` 包），或位图来自画布、已被 `Update` 修改，就没有其他来源可重新生成像素，因此仍会保留像素。与绘制操作一样，`ReleasePixels` 必须在主线程上调用：

```go
cover, _ := ui.DecodeBitmap(fileData)
// … once the cover has shown:
cover.ReleasePixels()

shot := ui.NewBitmap(capture) // the app keeps capture
// … once the shot has shown:
shot.ReleasePixels()
```

窗口关闭时会释放它显示过的内容：视图、元素、最后一帧的场景，以及它们绘制的位图。此后位图只在应用保留它期间存在。

`Update` 更新位图中的图片，适用于视频等持续变化的图像。尺寸相同时会复用 GPU 纹理，只上传新像素；而每帧新建位图则会每帧创建纹理。由 `image.NewRGBA` 创建的 `*image.RGBA` 会被直接接收，不复制：下一次 `Update` 之前必须保持像素不变。在 goroutine 上解码的帧应通过 `Window.Update` 更新位图，并触发窗口重绘：

```go
go func() {
	for frame := range frames { // *image.RGBA, a new one each time
		win.Update(func() { app.video.Update(frame) })
	}
}()
```

[`ui.Video`](video.md) 就这样播放视频的帧，提前解码，每一帧到期时显示，开箱即支持动画 GIF 和 PNG；视频文件和流用系统的媒体引擎（`ui/video`）播放，其帧保留在 GPU 上。

`UpdateYUV` 直接显示解码器输出的 Y′CbCr 视频帧，无需先转换为 RGBA。`ui.YUVImage` 保存各平面，按各自的行跨距存储：可以是硬件解码器常用的 NV12，即亮度平面和 Cb、Cr 交错的色度平面；也可以是软件解码器常用的 I420，即亮度、Cb 和 Cr 三个平面；同时保存转换矩阵（`ui.BT601`、`ui.BT709` 或 `ui.BT2020`）和是否使用全范围。GPU 上传这些平面时每像素只需 1.5 字节，而 RGBA 需要 4 字节，并在采样时完成转换，省去 CPU 显示一帧时原本最主要的转换开销。这些平面直接使用，不复制，但 I420 的色度数据会交错写入位图自己的平面；下一次更新之前必须保持输入平面不变。

```go
win.Update(func() {
	app.video.UpdateYUV(&ui.YUVImage{
		Format: ui.NV12, Width: 1920, Height: 1080,
		Planes:  [3][]byte{luma, chroma},
		Strides: [3]int{lumaStride, chromaStride},
		Matrix:  ui.BT709,
	})
})
```

这样的帧按它的尺寸显示，GPU 在采样时缩放它，不会像显示尺寸远小于自身尺寸的位图那样生成减半副本：只要解码器能做到，就让它给出帧显示时的尺寸。CPU 渲染器和截图则在绘制时转换该帧。

`Pixelated` 在放大位图时，将每个像素绘制成对应颜色的方块，而不是平滑混合相邻像素，适用于像素画和游戏精灵。整数倍放大时，每个方块占据整数个屏幕像素。在其他尺寸下，例如 2.5 倍缩放，各方块保持相同尺寸，边缘只在一个屏幕像素内混合，避免部分方块比其他方块宽一个像素；移动也会保持平滑，不会跳动，与 Ebitengine 的 `FilterPixelated` 一致。缩小时仍与普通位图一样平滑。

```go
tiles, err := ui.DecodeBitmap(tilesPNG)
if err != nil {
	log.Fatal(err)
}
tiles.Pixelated()
ui.Image(c, tiles).Size(64, 64) // 16×16 pixels, each 4×4
```

## 适应

`Fit` 说明图像如何填充盒子：`Contain`（默认）把它完整放进盒子，`Cover` 覆盖盒子并裁剪其余部分，`FillBox` 拉伸它，`ScaleDown` 按自身尺寸显示它，除非放不下，`NaturalSize` 按自身尺寸显示并裁剪。`Grayscale` 用灰阶绘制它，例如用于禁用的内容。`Radius` 把它的角做成圆角，`AspectRatio` 在会增长的盒子中保持它的比例。

```go
ui.Image(c, photo).AspectRatio(16.0 / 9).Fit(ui.Cover).Radius(8)
```

`ObjectPosition(x, y)` 决定 `Fit` 裁剪图片或在其周围留白时图片的位置，如同 CSS 的 `object-position`：即在该空间中横向和纵向的比例，0 表示左边缘或上边缘，1 表示右边缘或下边缘，0.5（默认）表示居中。其边缘关键字是 0 和 1：`left top` 就是 `ObjectPosition(0, 0)`。例如一个头像，人脸靠近照片顶部，需要保持在视野中，如同 `object-position: 50% 15%`：

```go
ui.Image(c, photo).Size(40, 40).Radius(20).Fit(ui.Cover).ObjectPosition(0.5, 0.15)
```

## SVG

`ui.ParseSVG` 解析 SVG，`ui.MustParseSVG` 解析作为程序一部分（例如嵌入文件）的 SVG，出错时会 panic。`Image` 按 SVG 自身的颜色显示它，其 `currentColor` 用文本颜色，尺寸采用 SVG 给出的尺寸（它的 `width` 和 `height`，或其 `viewBox` 的尺寸），除非另有指定：

```go
ui.Image(c, illustration).Width(240)
```

要按文本颜色显示 SVG，例如作为图标，请使用[图标](icon.md)。

对 SVG 显示的每个尺寸，MyGo 都像绘制文本那样把它的形状绘制进 GPU 的图集一次，之后由 GPU 从图集绘制：移动、滚动图像或改变它的颜色都不会再次绘制。MyGo 绘制路径和基本形状、组、`use` 和 `symbol` 元素、变换、颜色、`currentColor` 以及线性和径向渐变的填充和描边（及其连接、端点和虚线）、不透明度、裁剪路径、蒙版，以及简单选择器（类型、类和 id）的样式表。它不绘制文本、嵌入图像、图案、标记和滤镜：在导出前请在编辑器中把文本转换为路径。

在 `Draw` 回调中，`Painter.Image` 也绘制位图和 SVG。

## 任何格式的图片

`ui.DecodeImage` 解码事先不知道格式的图片，例如从链接下载的图片：若 `DecodeBitmap` 支持该格式，则返回位图，否则按 SVG 解析并确定尺寸。`ui.ImageSize` 返回 `Image` 默认显示位图或 SVG 的 DIP 尺寸，便于按宽高比设置大小：

```go
pic, err := ui.DecodeImage(data) // once, as it arrives, not in the view
if err != nil {
	return err
}
// … in the view:
if w, h := ui.ImageSize(pic); w > 0 && h > 0 {
	ui.Image(c, pic).MaxWidthPercent(100).AspectRatio(w / h)
}
```

## 辅助功能

图像是辅助技术看不到的装饰，除非用 `Label` 为它们命名，例如一张重要的照片。
