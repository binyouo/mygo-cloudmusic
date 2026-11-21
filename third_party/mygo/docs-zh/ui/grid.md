# 网格

<!-- preview grid 260 -->

`ui.Grid` 像 CSS grid 一样把子元素排成列和行：子元素逐行填入单元格，网格按需添加行。`Columns(3)` 创建三个等宽列；`ColumnTracks` 可以逐列设置尺寸，例如 `ui.Fixed(220)` 固定为 220 DIP，`ui.Fr(1)` 占剩余空间的一份，`ui.FitContent()` 则与内容一样宽：

```go
ui.Grid(c).Columns(3).Gap(12).Children(func() {
	for _, p := range app.photos {
		ui.Image(c, p).AspectRatio(1).Fit(ui.Cover).Radius(8)
	}
})

ui.Grid(c).ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(16).GapY(8).Children(func() {
	ui.Text(c, "Name").Bold().ColumnSpan(-1) // across every column
	ui.Text(c, "Email")
	ui.TextInput(c, &app.email)
	ui.Text(c, "Bio")
	ui.TextArea(c, &app.bio).RowSpan(2)
})
```

## 最小尺寸

`Fr` 轨道会共享空间，即使内容更宽，如同 CSS 的 `minmax(0, 1fr)`。`Min` 给轨道一个最小尺寸，如同 CSS 的 `minmax(240px, 1fr)`：主窗格旁的侧列保持可读的宽度，宁可让网格溢出它的盒子（由周围的滚动容器滚动），也不挤压窗格。若某个 `Fr` 轨道分到的空间会更少，它就保持最小值，其余 `Fr` 轨道再分配剩下的空间。`MinContent` 让 `Fr` 轨道不小于子元素内容所需的宽度，即它们的 min-content 宽度和 `MinWidth`（行则是高度），如同 CSS 的普通 `1fr`，即 `minmax(auto, 1fr)`：例如工具栏的标签列会占用剩余空间，但绝不会窄于标签中最长的词或它的 `MinWidth`。

```go
ui.Grid(c).ColumnTracks(ui.Fr(1).Min(240), ui.Fixed(320)).Children(func() {
	mainPane(c)
	sideColumn(c)
})

ui.Grid(c).ColumnTracks(ui.Fr(1).MinContent(), ui.FitContent()).GapX(8).Children(func() {
	ui.Text(c, app.title).MinWidth(120)
	ui.Button(c, "Share")
})
```

跨越多个 `MinContent` 轨道的子元素会按各轨道的因子比例将它们加宽，与 CSS 相同；没有 `MinContent` 的 `Fr` 轨道或 `Fixed` 轨道不参与这种分配。

## 放置子元素

`ColumnSpan` 和 `RowSpan` 让子元素跨越轨道，负的跨度表示直到最后一个轨道的所有轨道；`ColumnStart` 和 `RowStart` 把它放在指定的列和行中，从 1 开始计数。`RowTracks` 和 `Rows` 设置最前面的几行，`Fr` 让它们共享网格的高度。表格或你自己的网格中跨越轨道的单元格会告诉屏幕阅读器它也横跨了这些轨道（`aria-colspan`、`aria-rowspan`）：见[网格](accessibility.md#网格)。

## 子元素的尺寸

子元素的百分比，如 `FillWidth`、`WidthPercent` 和 `HeightPercent`，它们的 `Percent` 最小值和最大值，以及 `LeftPercent` 之类，都相对于它的单元格（即它跨越的轨道），而不是相对于网格，与 CSS 中一样。`ui.FitContent()` 轨道会按这类子元素的内容确定尺寸，如同它没有设置尺寸；随后子元素取得该轨道中的一份空间。

## 对齐

除非 `JustifyItems`（水平）和 `AlignItems`（垂直）另有说明，或对某一个子元素用 `JustifySelf` 和 `AlignSelf`，子元素会拉伸填满它们的单元格；自动外边距使它们居中。`AlignItems(ui.Baseline)` 让每一行各项的第一行文本对齐，如同[行](layout.md#对齐)中那样。适应内容的轨道会共享剩余空间，除非由 `Justify` 或 `AlignContent` 另行放置。

网格会构建它的所有子元素。对于项很多的网格，若只想构建显示出来的项，用[网格视图](grid-view.md)；对于控件旁的标签，用[表单](form.md)。
