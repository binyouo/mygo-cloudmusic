# 滑块

<!-- preview slider 180 -->

`ui.Slider` 创建一个滑块，用于在低值和高值之间设置 `*float64`：拖动滑块柄，或在轨道上按下，即可设置数值；具有键盘焦点时，方向键、Page Up 和 Page Down（范围的十分之一）、Home 和 End 也可以。`Changed` 报告新值。

```go
ui.Slider(c, &app.volume, 0, 100).Label("Volume")
```

## 步进

`ui.StepSlider` 把值吸附到低值以及从低值起算的步长整数倍，并在每个位置画一个刻度线，如同 AppKit 带刻度线的滑块：拖动时会吸附到刻度，方向键则按步长移动。

```go
ui.StepSlider(c, &app.quality, 0, 100, 25).Label("Quality")
```

若要同时设置低值和高值，例如价格范围，用[范围滑块](range-slider.md)；若要精确的数值，用[数字输入](number-input.md)或[步进器](stepper.md)。

## 无外观

`ui.SliderBase` 是不预设外观的滑块：在扣除内边距后的内容区域内拖动即可设置数值。按 `(value-lo)/(hi-lo)` 计算位置，绘制轨道和滑块柄；内边距设为滑块柄宽度的一半，以免滑块柄越界。在[从右到左](right-to-left.md)的布局中，低值位于右侧，与 `Slider` 一致，方向键按箭头所指方向移动滑块柄：`c.Direction()` 为 `ui.RightToLeft` 时，应镜像绘制：

```go
s := ui.SliderBase(c, &app.volume, 0, 100).Height(24).PaddingX(12)
at := float32(app.volume / 100)
if c.Direction() == ui.RightToLeft {
	at = 1 - at
}
s.Draw(func(p *ui.Painter, r ui.Rect) {
	x := r.X + 12 + (r.W-24)*at
	p.Fill(ui.Rect{X: r.X + 12, Y: r.Y + 10, W: r.W - 24, H: 4}, gray, 2)
	p.Fill(ui.Rect{X: x - 12, Y: r.Y, W: 24, H: 24}, blue, 12)
})
```

`Step` 将值吸附到从低值开始、以指定步长递增的各个位置，方向键在这些位置之间移动，而不再每次移动范围的百分之一；`Vertical` 将滑块改为垂直方向，数值从内容区域底部的低值向上递增；此时内边距应设为滑块柄高度的一半：

```go
ui.SliderBase(c, &app.level, 0, 10).Step(1).Vertical().Size(24, 120).PaddingY(12).Draw(drawLevel)
```

见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一个表示其范围和值的滑块，由 `Label` 命名，水平或垂直，可按方向键的方式递增和递减。
