# 图表

<!-- preview chart 300 -->

`ui/chart` 包不预设图表外观，类似 Web 上的 visx 等无外观图表库，同时提供 Swift Charts 中常见的图表组件：将数据映射到绘图区并选取规整刻度的标度、带坐标轴的绘图区、可像普通元素一样设置样式的数据标记、供 `Draw` 使用的折线、面积和饼图扇区路径、数据堆叠，以及支持指针和键盘操作的选择功能。颜色、尺寸和字体均由你决定。

```go
import "github.com/egoist/mygo/ui/chart"

t := c.Theme()
x := chart.NewBand(app.months...).Padding(0.3, 0.15)
_, hi := chart.Extent(app.sales...)
y := chart.NewLinear(0, hi).Nice(4)
ticks := y.Ticks(4)
chart.Plot(c, chart.Axes{Left: ticks, Bottom: x.Ticks()}, func(area ui.Element) {
	area.Draw(func(p *ui.Painter, r ui.Rect) {
		for _, tick := range ticks { // grid lines
			p.Line(r.X, chart.Y(r, tick.At), r.X+r.W, chart.Y(r, tick.At), 1, t.Border)
		}
	})
	for i, v := range app.sales {
		chart.Rect(c, x.Band(i), y.Span(0, v)).Radius(4).Background(t.Accent).
			Tooltip(fmt.Sprintf("%s: %.0f", app.months[i], v))
	}
}).Label("Sales by month").Height(200).Gap(6)
```

绘图区上的位置用 0 到 1 的比例表示：x 轴从左边缘算起，y 轴从底部向上，与数值增长方向一致。标度把数据转换为比例；标记以绘图区的百分比定位，因此构建时无需等待布局，并会随绘图区缩放而调整；`Draw` 函数会收到绘图区的盒子，`chart.X(r, f)` 和 `chart.Y(r, f)` 把比例换算成其中的坐标点。

## 标度

- `chart.NewLinear(lo, hi)` 映射数字。`Nice(n)` 向外扩展定义域，使边界落在约 n 个规整刻度上，`Ticks(n)` 返回这些刻度：间隔为 1、2 或 5 乘以 10 的幂，和 d3、Swift Charts 的取法一样；小数位数按步长需要而定，或者由 `Format` 标注。`At(v)` 返回某个值的位置，`Span(a, b)` 返回两个值之间的区间，例如柱子从 0 到它的值；`Invert(f)` 返回某个比例处的值，例如指针下方的值。`Clamp` 把定义域之外的值限制在两端；`chart.Extent(values...)` 返回最小值和最大值。
- `chart.NewLog(lo, hi)` 让相等的比值有相等的距离：1、10 和 100 等距。两端必须同时为正或同时为负；反向的定义域也可以。`Base` 默认为 10（2 和 `math.E` 也可以）；它改变刻度和 `Nice()`，不改变位置。`Nice()` 会扩展到基数的整数次幂。`Ticks(n)` 只是密度提示：基数取整数、定义域又较短时，会包含次刻度，它们太拥挤时则省略标签。设置 `Format` 时标注每个刻度。定义域上下限相同时，只在该值处生成一个刻度；基数不是整数时也如此。
- `chart.NewSymLog(lo, hi)` 包含负值和零，使用 `sign(v) * log1p(abs(v)/c)`。`Constant` 是 c，为零时是 1：即零附近近似线性区域的范围；数值更大时则按对数处理。`Ticks(n)` 和 `Nice(n)` 按数据的原始单位选取规整的刻度值，再按非线性关系定位。如果计数不是正数，或取整请求无法得到稳定的定义域，就保持原始定义域不变。
- `chart.NewBand(labels...)` 为每个标签分配一个带（band），就像柱状图的柱子：`Band(i)` 是第 i 个的区间，`At(i)` 是它的中间，`Width()` 是它们的宽度，`Index(f)` 是某个比例处的带。`Padding(inner, outer)` 在带之间和带周围留出空间，单位是带步长的比例。它的 `Ticks()` 是标签，位于各自带的中间。
- `chart.NewTime(from, to)` 映射时间。它按时间跨度在对应时区内选取规整的刻度，和 d3 一样：秒、一刻钟、午夜、周日、每月或每年的第一天；每个刻度的标签只显示相对上一个较大时间单位边界发生变化的部分：“2026”、“Mar”、“Mar 5”、“14:00”、“:30”。

刻度由 `chart.Tick` 值表示，其中 `At` 是位置比例，`Value` 和 `Label` 分别是数值和标签；标度没有提供所需刻度时，可以自行创建。

### 宽范围与带符号数据

```go
latency := chart.NewLog(0.01, 1000) // milliseconds, five orders of magnitude
position := latency.At(0.1)       // 0.2 of the plot
value := latency.Invert(0.5)      // about 3.162 ms

changes := chart.NewSymLog(-1000, 1000)
changes.Constant = 10
baseline := changes.At(0)        // 0.5; zero is valid
bar := changes.Span(0, -100)     // a signed bar around that baseline
```

两种标度都有 `At`、`Invert`、`Span`、`Clamp` 和 `Format`，和 `Linear` 一样。让定义域保持在数据单位中；对每个样本应用 `At`，然后使用普通的标记和路径。对数柱需要非零基线：零附近的柱应使用 `SymLog`。NaN 样本按缺失值传播，线和区域的路径把它们留作空隙。不启用裁剪时，符号不符的值在对数标度上得到 NaN，零则映射到绘图区之外的无穷远。启用 `Clamp` 时，定义域之外的值（包括零）会停在两端。

定义域必须是有限的。对数定义域不能接触或跨过零，它的非零基数必须有限且大于 1，symlog 标度的非零常数必须有限且为正。无效的标度给出 NaN 位置且没有刻度；`Nice` 不会改动它们。接近 float64 的极限时，如果下一个幂无法表示，对数 `Nice` 会保留一个端点。标度是值类型，没有可变的缓存或计时器，也不会增加运行时依赖。Gallery 示例的 Charts 页面会比较线性与对数延迟，还可以调整 symlog 的线性区域。

## 绘图区及其坐标轴

`chart.Plot` 用网格布置绘图区，每侧有一条坐标轴，刻度由 `chart.Axes` 提供：`Left` 和 `Right` 沿高度排列，`Top` 和 `Bottom` 沿宽度排列。标签参与布局，因此坐标轴占用所需的空间，剩余空间归绘图区：给绘图区一个高度，或让它在父元素中 `Fill`，并用 `Gap`（或 `GapX` 和 `GapY`）设置标签与区域之间的间距。`Axes.Label` 构建每个标签，并让它以对应刻度为中心；默认使用纯文本。标签放在宽高为零、用于居中的盒子里，因此要用 `NoWrap` 让文本保持在一行：

```go
label := func(side chart.Side, tick chart.Tick) {
	ui.Text(c, tick.Label).NoWrap().FontSize(11).TextColor(t.TextMuted)
}
chart.Plot(c, chart.Axes{Left: y.Ticks(4), Bottom: x.Ticks(), Label: label}, build)
```

`Plot` 的函数会收到绘图区，这是一个可设置样式的元素，可以有背景、边框，或用 `Draw` 函数绘制网格线和线条，也可以向其中添加标记。

## 标记

标记是绘图区中的元素，按绘图区的百分比定位：像任何元素一样设置样式、为辅助技术设置名称、添加工具提示，并处理它们的点击。

- `chart.Rect(c, x, y)` 覆盖 x 和 y 方向上各自指定的区间：可用于柱形、范围带或热图单元格。
- `chart.Point(c, x, y, w, h)` 以指定点为中心，宽 w、高 h DIP：可用于散点图中的点或选中点。
- `chart.RuleX(c, x, width)` 是横跨绘图区的竖线，`chart.RuleY(c, y, height)` 是横线：一个阈值、一个平均值。
- `chart.Anchor(c, x, y, h, v, build)` 把 `build` 添加的内容放在某个点上，尺寸不限：`ui.Center` 将它居中于该点，`ui.Start` 把它的左边缘（h）或上边缘（v）放在那里，`ui.End` 把它的右边缘或下边缘放在那里，因此 `ui.Center, ui.End` 会把标签显示在柱子上方。

```go
for i, v := range app.sales {
	bar := chart.Rect(c, x.Band(i), y.Span(0, v)).Background(t.Accent)
	if bar.Clicked() {
		app.month = i
	}
	chart.Anchor(c, x.At(i), y.At(v), ui.Center, ui.End, func() {
		ui.Text(c, fmt.Sprint(v)).NoWrap().FontSize(11).Margin(0, 0, 4, 0)
	})
}
```

带 [`Transition`](transitions.md) 的标记会在数据变化时移到新位置，例如柱子增长到新的值：

```go
chart.Rect(c, x.Band(i), y.Span(0, v)).Background(t.Accent).
	Transition(ui.ElementTransition{Duration: 300 * time.Millisecond})
```

## 线与区域

`chart.Line(r, xs, ys, curve)` 返回穿过各点的线条路径，`chart.Area(r, xs, lo, hi, curve)` 返回两条线之间区域的路径，供绘图区的 `Draw` 函数描边和填充。`xs` 和 `ys` 是比例，与标度返回的一致；`lo` 为 nil 时表示绘图区底部，NaN 表示缺失值，会在该处断开线或区域。各点的连接方式由曲线类型决定：`chart.CurveLinear` 用直线，`chart.CurveMonotone` 平滑连接且不超出相邻点的数值范围，`chart.CurveStepAfter`、`CurveStepBefore` 和 `CurveStep` 则用阶梯线连接。

```go
area.Draw(func(p *ui.Painter, r ui.Rect) {
	p.FillPath(chart.Area(r, xs, nil, ys, chart.CurveMonotone), t.Accent.Alpha(0.15))
	p.StrokePath(chart.Line(r, xs, ys, chart.CurveMonotone), 2, t.Accent)
})
```

### 实时数据

包含数千个样本、随新样本逐帧重绘的折线，通常无需绘制全部采样点：`chart.MinMax` 会保留每个像素列中最低和最高的样本（按样本顺序），以及每一段的端点，这样线条看起来不变，狭窄的尖峰也仍能显示。它的 `xs` 是从不递减的比例；NaN 仍会断开线。它写入你传入的切片，`chart.LineInto` 写入你传入的路径，因此视图保留它们后，预热完成即可不再分配内存地绘制。样本本身则要保留，供选择和读数使用。

`Painter.StrokePolyline` 按 DIP 描边各个点，使用圆角连接和端点，效果和 `StrokePath` 一样，但对于每帧都变化的线更快：在 GPU 上，它把每一段画成圆角矩形，而不是在 CPU 上把整条线栅格化为遮罩。在段与段重叠的地方，抗锯齿边缘可能比 `StrokePath` 的略粗。半透明的线以及没有 GPU 时的绘制则走 `StrokePath` 的路径，因为在那里重叠的段会显得更深；只有一段的半透明线没有可重叠的部分，在 GPU 上依然很快。

```go
// bx, by and points live in the view's state, reused every frame.
area.Draw(func(p *ui.Painter, r ui.Rect) {
	bx, by = chart.MinMax(bx, by, xs, ys, int(math.Ceil(float64(r.W*p.Scale()))))
	points = points[:0]
	for i, x := range bx {
		points = append(points, [2]float32{chart.X(r, x), chart.Y(r, by[i])})
	}
	p.StrokePolyline(points, 2, t.Accent)
})
```

用 `Context.AnimationFrame` 请求帧，并集中处理自上一帧以来到达的样本，不要每收到一个样本就请求重绘。`examples/chart-streaming` 滚动显示三条各含 15,000 个样本的波形，每条每秒接收 1,000 个样本；`-mode path` 和 `-mode raw` 用 `StrokePath` 绘制，以便比较。

## 堆叠

`chart.Stack(series, how)` 把各系列堆叠起来，就像堆叠柱和堆叠区域那样：`series[s][i]` 是第 s 个系列的第 i 个值，结果的 `[s][i]` 是它在第 i 个堆叠中所占的 `Segment`。`chart.StackStandard` 把正值从 0 向上堆叠，负值从 0 向下堆叠；`chart.StackNormalized` 按堆叠总和的比例堆叠，范围从 0 到 1；`chart.StackCenter` 围绕 0 堆叠，形成流图。

```go
stacked := chart.Stack([][]float64{app.search, app.direct}, chart.StackStandard)
for s, series := range stacked {
	for i, seg := range series {
		chart.Rect(c, x.Band(i), y.Span(seg.Lo, seg.Hi)).Background(colors[s])
	}
}
```

## 饼图与环形图

`chart.Pie(values, pad)` 按各值的顺序从十二点钟方向起分配整个圆，相邻扇区之间留出 `pad` 弧度，`chart.Sector(cx, cy, inner, outer, slice)` 返回一个扇区的路径：`inner` 为 0 时是楔形，否则是环形图的一块。`chart.SliceAt` 找到某个点所在的扇区，例如指针下方；`chart.Polar` 找到某个角度处的点，即扇区标签所在的位置：

```go
slices := chart.Pie(app.shares, 0.03)
ring := ui.Box(c).Size(160, 160).Label("Users by platform")
hover := -1
if x, y, over := ring.PointerPosition(); over {
	if i, ok := chart.SliceAt(slices, x-80, y-80, 50, 80); ok {
		hover = i
	}
}
ring.Draw(func(p *ui.Painter, r ui.Rect) {
	for i, s := range slices {
		outer := float32(76)
		if i == hover {
			outer = 80
		}
		p.FillPath(chart.Sector(r.X+80, r.Y+80, 50, outer, s), colors[i])
	}
})
```

## 选择

`chart.Select` 让绘图区从 `xs` 中按升序排列的点里选择一个，如同 Swift Charts 的 `chartXSelection`：指针位于绘图区上时选离它最近的点，离开后则为无（-1）。绘图区会成为键盘焦点的一个 Tab 停留点；获得焦点时，左右方向键、Home 和 End 可以移动选择，Escape 清除选择。选择变化时它会上报。用标记显示选择：

```go
describe := func(i int) string { return fmt.Sprintf("%s: %.1f °C", hours[i], temps[i]) }
chart.Select(c, area, &app.selected, xs, describe)
if i := app.selected; i >= 0 {
	chart.RuleX(c, xs[i], 1).Background(t.TextMuted)
	chart.Point(c, xs[i], ys[i], 10, 10).Radius(5).Background(t.Accent)
	chart.Anchor(c, xs[i], 1, ui.Center, ui.Start, func() {
		ui.Text(c, describe(i)).NoWrap().Padding(3, 8).Radius(6).Background(t.Surface)
	})
}
```

对于带标度的各点，`xs` 是它们的中心位置；`chart.Nearest` 可以为你自己的选择找到最接近某个比例的点，带的 `Index` 则找到对应的带。

## 辅助功能

用 `Label` 命名绘图区，用 `Description` 说明它显示什么。标记是元素：辅助技术需要读取的每个标记都要命名，例如柱子的月份和值。`Select` 会把选中的点告诉辅助技术：`describe` 的返回值就是该绘图区的值，方向键移到某个点时，屏幕阅读器会读出它。

这个包省略的内容：缩放和平移、图例（即自行构建的一行行元素，如上所示）和彼此避让的标签。
