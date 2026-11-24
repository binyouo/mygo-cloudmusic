# 布局

元素用 flexbox 排版子元素，如同 CSS，或用[网格](grid.md)，单位是设备无关像素（DIP）：

```go
ui.Row(c).Gap(8).Padding(12).Children(func() {
	ui.Icon(c, folder)
	ui.Text(c, "Documents").Grow(1)
	ui.Text(c, "12 items").TextColor(c.Theme().TextMuted)
})
```

## 容器

- `ui.Column` 从上到下堆叠子元素，并拉伸到它的宽度；`ui.Box` 也是一个列。`ui.Row` 从左到右放置它们，并垂直居中。`Reverse` 按相反方向排布它们，从右或从下，如同 CSS 的 `row-reverse` 和 `column-reverse`。
- `ui.Spacer` 占用所在行或列的空闲空间，把相邻元素推开；`ui.Divider` 在它所在的行或列中画一条细线。`Dividers(width, color)` 在行、列或[列表](list.md)的每两个子元素之间画一条线，位于它们之间间隙的中央，这不会加宽间隙：因此要给元素设置至少同样宽的 `Gap`。
- `ui.Grid` 把子元素排成列和行：见[网格](grid.md)。会滚动的容器是[滚动视图](scroll.md)，而[列表](list.md)只构建视口中的行。

## 大小

- `Width`、`Height` 和 `Size` 设置尺寸；`WidthPercent` 和 `HeightPercent` 取父元素的一部分；`MinWidth`、`MaxWidth`、`MinHeight` 和 `MaxHeight` 约束它们，它们的 `Percent` 形式则按父元素的比例约束。同时设置两者的元素会取较大的最小值和较小的最大值，如同 CSS 的 `max-width: min(78%, 34rem)` 对聊天气泡所做的那样：`MaxWidthPercent(78).MaxWidth(544)`。`Fill`、`FillWidth` 和 `FillHeight` 占满父元素的整个内容框。在[网格](grid.md#子元素的尺寸)中，子元素的百分比相对于它的单元格。
- `Grow(1)` 沿父元素的方向把空闲空间给元素，如同 `flex: 1`：填满窗口剩余部分的列表，或把其后的元素推到末尾的 `Spacer`。`Shrink`、`Basis` 和 `BasisPercent` 的工作方式与 CSS 相同。
- `AspectRatio` 保持元素的比例。

## 间距

`Gap` 设置子元素之间的间距，`GapX` 和 `GapY` 分别设置横向和纵向的间隔；`Padding` 和 `Margin` 接受一个、两个或四个值，如同 CSS，而 `PaddingX`、`PaddingY`、`MarginX` 和 `MarginY` 分别设置两侧。`ui.Auto` 的外边距占取所在侧的空闲空间，如同 CSS：`Margin(0, ui.Auto)` 在列中居中一个元素，`Margin(0, 0, 0, ui.Auto)` 把行中的一个子元素及其后的元素推到右边。

`PaddingStart` 和 `PaddingEnd`、`MarginStart` 和 `MarginEnd`，以及 `BorderStartWidth` 和 `BorderEndWidth` 设置行的起始侧和结束侧：通常分别为左侧和右侧，在[从右到左](right-to-left.md)布局中则分别为右侧和左侧，如同 CSS 的逻辑属性。`MarginStart(ui.Auto)` 无论方向如何，都把行中的子元素推到末尾。

## 对齐

`Justify` 设置子元素在主轴上的对齐方式（`Start`、`Center`、`End`、`SpaceBetween`、`SpaceAround`、`SpaceEvenly`），`AlignItems` 设置交叉轴上的对齐方式（`Start`、`Center`、`End`、`Stretch`、`Baseline`），`AlignSelf` 则单独设置某个子元素；`Center` 在两个方向上都居中。`Start` 通常是行的左端和列的顶部；在[从右到左](right-to-left.md)布局中，行从右端开始。`Wrap` 允许子元素换行，`WrapReverse` 让后续行向上堆叠，`AlignContent` 设置这些行的对齐方式（与 `Justify` 的选项相同，另有 `Stretch`）。

`Baseline` 让行中子元素的第一行文本对齐，例如大号价格旁的小号单位，行的高度也随之达到所需值。盒子的基线取其第一个子元素的基线（在行中，是第一个按基线对齐的子元素），没有文本的子元素则以底部对齐，如同 CSS：

```go
ui.Row(c).AlignItems(ui.Baseline).Gap(4).Children(func() {
	ui.Text(c, "$42").FontSize(40)
	ui.Text(c, "/month").TextColor(t.TextMuted)
})
```

## 定位

`Absolute` 让元素脱离文档流，用 `Top`、`Right`、`Bottom` 和 `Left` 把它定位在父元素中，以 DIP 为单位，或用 `TopPercent` 之类；自动外边距让它在所依据的两边之间居中。如同 CSS，这些距离从父元素边框内侧量起，无论其内边距如何：在边框宽 1 DIP 的盒子里，`Top(0)` 位于盒子顶部边缘下方 1 DIP。`Start` 和 `End`（以及 `StartPercent`、`EndPercent`）从行起始的一侧和另一侧定位它，[从右到左](right-to-left.md)时分别是右边和左边。对于文档流中的元素，`Top` 等属性会把它从布局放置的位置移开，而不移动它的兄弟元素，如同 CSS 的相对定位：例如一个略微高出文本的徽章。`Sticky` 则让它们把文档流中的元素固定在它周围的滚动容器内，如同 CSS 的 `position: sticky`（见[滚动视图](scroll.md#粘性元素)）。

```go
ui.Box(c).Size(40, 40).Children(func() {
	ui.Icon(c, bell).FontSize(24)
	ui.Badge(c, "3").Absolute().Top(-4).Right(-6)
})
```

`Attach(at, self)` 也让元素脱离文档流，并把它的一个点（`self`）放在其父元素盒子（`at`）的一个点上：一个角、一条边的中点或中心，例如 `ui.AnchorTopRight` 或 `ui.AnchorCenter`，或行起始或结束的一侧上的点，例如 `ui.AnchorTopEnd`，在从右到左的布局中它位于左边。元素保持自身尺寸，`Top`、`Right`、`Bottom` 和 `Left` 从那里起移动它：

```go
ui.Box(c).Children(func() {
	ui.Avatar(c, "Ada Lovelace", nil)
	// Centered on the avatar's top right corner, whatever its size.
	ui.Badge(c, "3").Attach(ui.AnchorTopRight, ui.AnchorCenter)
})
ui.Box(c).Fill().Children(func() {
	ui.PrimaryButton(c, "New").Attach(ui.AnchorBottomRight, ui.AnchorBottomRight).Right(16).Bottom(16)
})
```

## 绘制顺序

兄弟元素按构建顺序绘制，文档流中的先绘制，绝对定位和粘性定位的元素绘制在它们之上，重叠时较晚的元素取得指针。`ZIndex` 改变该顺序而不在树中移动元素，如同 CSS 的 `z-index`：值较高的兄弟元素绘制在值较低的之上，并先取得指针；值相同的保持上面的顺序；默认值为 0。负值把元素放在其兄弟元素之下、父元素背景之上。布局、`Tab` 移动焦点的顺序，以及辅助技术读取的内容仍然遵循树，因此重新排列子元素不能替代 `ZIndex`。

```go
// Overlapping avatars, the first on top.
ui.Row(c).Children(func() {
	for i, u := range users {
		a := ui.Avatar(c, u.Name, u.Photo).ZIndex(len(users) - i)
		if i > 0 {
			a.Margin(0, 0, 0, -8)
		}
	}
})

// A cell growing over its neighbours while the pointer is over it.
cell := ui.Box(c).Size(80, 80)
if cell.Hovered() {
	cell.Scale(1.2, 1.2).ZIndex(1)
}
```

该顺序只在兄弟元素之间：元素会带着它内部的所有内容一起移动，仍被其父元素裁剪，绝不会与其他父元素的元素混合，就像每个非 0 的 z-index 都创建了一个 CSS 堆叠上下文。提高列中一张卡片的 z-index 不会把它抬到下一列之上；需要提高的是列。窗口的内容始终在其[浮层](overlays.md)之下，浮层则保持打开时的顺序。带着 [`Exit`](transitions.md) 离开的元素的副本保持元素原有的 z-index，位于 z-index 相同的兄弟元素之下：一个抬高到页面之上的 toast 会在页面之上淡出。

## 裁剪与可见性

- `Clip` 把子元素裁剪到它的圆角盒子里，范围在边框内侧，边框绘制在它们之上；`ClipX` 和 `ClipY` 只裁剪左右两侧或上下两侧。
- `Invisible` 隐藏元素及其子元素，它们保留占位，但不绘制任何内容，也不取得指针或焦点，如同 CSS 的 `visibility: hidden`。
- `Hidden(true)` 隐藏它们并收回它们的空间，如同 CSS 的 `display: none`：兄弟元素会合拢，不会为它留下间隙，辅助技术也不会读取它。与不构建该元素不同，它会保留内部内容的状态，例如滚动偏移、正在编辑的文本和动画，以便 `Hidden(false)` 再次显示它时使用。它隐藏时，焦点会离开，附着到它上面的内容（`AttachTo`）也随之隐藏。

  ```go
  ui.Column(c).Hidden(!app.showDetails).Children(func() {
  	ui.TextArea(c, &app.notes)
  })
  ```
- `Debug` 勾勒元素及其内部的所有内容，包括它们的内边距和外边距，以看清布局为何如此。[检查器](inspector.md)会显示每个元素的盒子，并说明原因。
- 元素用[过渡](transitions.md)平滑地移动、调整大小和改变颜色。
