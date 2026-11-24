# 滚动视图

<!-- preview scroll 280 -->

`ui.Scroll` 和 `ui.ScrollHorizontal` 通过滚轮、触摸板、滚动条和键盘来滚动子元素：方向键、Page Up、Page Down、空格、Home 和 End 会滚动焦点所在的容器，或指针下方的容器，除非获得焦点的元素自己接收这些键，例如文本输入框。`ui.ScrollBoth` 则可向两个方向滚动，像画布或宽表格那样。鼠标滚轮滚动一格时会经过短暂的缓出滑动，和浏览器一样；连续几格会累加，直到应用自己滚动容器（`ScrollState`、`ListState` 的 `ScrollTo`、`ScrollIntoView` 和 `ScrollToEnd`，或容器中某个元素的 `ScrollIntoView`），这时就由应用接管。触摸板会跟随手指，触摸屏上拖动手指在 Linux 上也是如此；桌面要求减少动效时，滚动一格则立即到位。

```go
ui.Scroll(c).Grow(1).Padding(16).Gap(12).Children(func() {
	for _, p := range app.posts {
		post(c, p)
	}
})
```

需要设置容器大小，或让容器在父元素中扩展。要显示多行数据，使用[列表](list.md)，只构建视口内的行。

在[从右向左](right-to-left.md)布局时，横向滚动的容器会先显示内容的开头，位置在右侧，`ScrollState.X` 也从那里开始计数：0 是开头，`MaxX` 是末尾，如同 Flutter 的偏移那样。

## 保持偏移

滚动容器和其他状态一样，依靠自己在视图中的位置或 `Key` 来保持偏移。要读取偏移、设置偏移，或把它存进应用状态，可以用 `TrackScroll` 给容器一个 `ui.ScrollState`：容器从状态的 `X` 和 `Y` 显示内容，在用户滚动时写入它们，并把 `MaxX` 和 `MaxY` 设为它们能达到的最大值。`Y = 0` 滚动到顶部，`math.MaxFloat32` 滚动到末尾，容器显示的每个页面各用一个 `ScrollState`，就能分别保持每个页面的位置。一个跟随末尾的日志——除非用户向上滚离：

```go
if app.log.Y >= app.log.MaxY {
	app.log.Y = math.MaxFloat32
}
ui.Scroll(c).TrackScroll(&app.log).Grow(1).Children(app.lines)
```

`ScrollTo(x, y, animation)` 按弹簧或曲线动画平滑滚动到指定位置（见[动效](motion.md)）；途中再次调用会改为滚向新目标。用户滚动会停止动画，`Scrolling()` 报告动画是否仍在进行。系统启用减少动效时，会立即滚动到目标位置：

```go
if ui.Button(c, "Back to top").Clicked() {
	app.feed.ScrollTo(0, 0, ui.Smooth)
}
```

## 粘性元素

`Sticky` 让元素保留在布局流中并占用原有空间，同时在父元素可见时，将其固定在周围的滚动容器内，类似 CSS 的 `position: sticky`。`Top`、`Right`、`Bottom` 和 `Left`（或 `Start` 和 `End`，也可用容器尺寸的百分比）指定元素距容器边框内侧的距离。元素不会超出父元素的内容盒，因此父元素滚出视口时，元素也会随之离开，随后由下一组的对应元素接替。例如，时间线上的日期标题会固定在顶部，直到下一天的标题将其顶出：

```go
ui.Scroll(c).Fill().Children(func() {
	for _, day := range app.days {
		ui.Column(c).Children(func() {
			ui.Text(c, day.Title).Bold().Padding(8, 16).Background(t.Surface).Sticky().Top(0)
			for _, ev := range day.Events {
				eventRow(c, ev)
			}
		})
	}
})
```

每个轴跟随最近一个按该方向滚动的容器（或通过 `ScrollWith` 横向跟随其他元素滚动的表头），没有这样的容器时跟随窗口。因此，用于全选横向滚动表格各行的复选框会留在起始端，而它所在的表头行会粘在纵向滚动页面的顶部；CSS 做不到这一点，因为在 CSS 里单向滚动的容器会同时滚动两个方向：

```go
ui.Scroll(c).Fill().Children(func() {
	ui.ScrollHorizontal(c).Shrink(0).Children(func() {
		ui.Column(c).Width(app.tableWidth()).Children(func() {
			ui.Row(c).Background(t.Surface).Sticky().Top(0).Children(func() {
				ui.Checkbox(c, &app.all, "").Background(t.Surface).Sticky().Start(0)
				headerCells(c)
			})
			rows(c)
		})
	})
})
```

如果某个轴两端的内缩都设置了，而容器小到无法同时满足，以 `Top` 为准；水平方向以 `Left` 为准，[从右向左](right-to-left.md)时以 `Right` 为准。粘性元素和绝对定位元素一样绘制在兄弟之上，所以从它下面滚过的内容不需要裁剪，但要给它一个背景；指针、辅助技术、附加到它的内容（`AttachTo`）以及 `ScrollIntoView` 都会在它显示的位置找到它。焦点和辅助技术仍按它在兄弟中的位置访问它。它对 `Absolute` 元素或文本中的元素不起作用。[列表](list.md)只构建视口中的行，所以分区标题应通过 `ListState.Header` 提供，它会固定这些标题，而不是让某一行变成粘性元素。

## 滚动到视图中

布局完成后，`ScrollIntoView` 会以最小的幅度滚动元素周围的容器，让元素显示出来，因此它在添加元素的那一帧就能生效：一条新消息，或按键选择的项。只在那一帧调用它，否则用户就无法把元素滚走。`List` 中尚未构建的行用它的 `ListState` 滚动到视图内。

```go
if app.added {
	ui.Text(c, msg.Text).ScrollIntoView()
}
```

`ScrollIntoViewOptions` 和 Web 的 `scrollIntoView` 选项一样，可以把元素对齐到别处，或滑行过去。`Block` 控制纵向对齐，`Inline` 控制横向对齐：默认的 `ui.ScrollNearest` 以最小幅度滚动让它显示，`ui.ScrollStart`、`ui.ScrollCenter` 和 `ui.ScrollEnd` 把它的开头、中心或末尾与每个容器的对应位置对齐（在边框以内）；[从右向左](right-to-left.md)时，内联方向的开头在右侧。设置 `Animation` 时，周围的每个容器都会按动画的设定滑行过去，如同 Web 的 `behavior: "smooth"`，而不是立即滚动：用户滚动或应用操作都会停止它，桌面要求减少动效时则会立即滚动。搜索的当前匹配项，居中：

```go
if app.matchMoved {
	hit.ScrollIntoView(ui.ScrollIntoViewOptions{Block: ui.ScrollCenter, Animation: &ui.Smooth})
}
```

零值选项与不带选项的 `ScrollIntoView()` 相同。

键盘焦点移动时会滚动到视图内。带有应用自己的 `TextInputClient` 的元素（[文本输入客户端](text-input-client.md)）会把插入符（位置由 `BoundsForRange` 给出）滚动到视图内，而不是滚动整个盒子：和行数一样高的代码编辑器在获得焦点时会显示正在编辑的那一行，而不是第一行。

## 吸附

`ScrollSnap` 让容器停靠到其中元素用 `SnapAlign` 标记的位置，如同 CSS 的 `scroll-snap-type` 和 `scroll-snap-align`，作用于它滚动的轴：例如一条停下时总从开头显示一张卡片的卡片带、轮播的页面，或长页面的各个分区。

```go
ui.ScrollHorizontal(c).ScrollSnap(ui.SnapMandatory).Padding(16).Gap(12).Children(func() {
	for _, card := range app.cards {
		cardView(c, card).Width(240).Shrink(0).SnapAlign(ui.Start)
	}
})
```

`SnapAlign(ui.Start)`、`ui.Center` 或 `ui.End` 把元素的开头、中心或末尾与容器的内容盒对齐（在容器的内边距以内），因此卡片带的第一张卡片会停在它的起始位置；从右向左时，起始位置在右侧。`SnapAlignX` 和 `SnapAlignY` 只标记一个轴，例如 `ScrollBoth` 横向吸附列、纵向可以自由滚动。这里的元素指容器内的任何元素，不限于直接子元素；但另一个滚动容器除外，它的元素归它自己管。`List` 的行在构建时会标记自己的位置。

滚动停稳时容器会吸附，和浏览器一样：

- 触摸板、其后的甩动惯性、触摸屏上的手指，以及松开后的滚动条滑块，都会滑到最近的位置；
- 鼠标滚轮滚动一格、方向键、Page Up 和 Page Down、Home 和 End，以及按下滚动条的轨道，会沿该方向越过当前位置，前往下一个位置，也就是离它们原本会滚到的位置最近的那个。到了最后一个位置，滚轮就会滚动它外面的容器。

`ui.SnapMandatory` 总是停在某个位置上：它首次显示时会先对齐一个位置，滚轮和按键则逐位移动。`ui.SnapProximity` 只在滚动停在距某个位置不到三分之一个视图时吸附过去，否则可以停在任意位置。比视图大的元素（例如很高的分区）可以在填满视图的任意位置停住，所以滚轮会先滚过它，再到下一个。

停靠在某个元素上后，即使布局移动了它（例如窗口缩放，或上面的行改变高度），容器也会继续停在那里；而进入视图的元素（通过 `ScrollIntoView`，或焦点移动）会对齐到它周围被标记元素的位置。应用通过 `ScrollState`、`ScrollTo` 或 `ListState` 滚动容器时，容器就停在应用放置的位置。桌面要求减少动效时，它会立即停到位。

## 滚动条

内容溢出的滚动容器会在指针位于其上时，在内容上方显示滚动条的滑块，供用户拖动；滑块不占空间。从右向左时，垂直滚动条在左侧。主题的 `ScrollbarWidth` 设置它们的宽度，`Scrollbar` 设置它们的颜色。

`ScrollbarStyle` 进一步设置样式：滑块背后轨道的颜色、滑块悬停和拖动时的颜色、滚动条和滑块的宽度、圆角半径，以及最短滑块长度。某个字段有设置时，它会覆盖 `Scrollbar` 和 `ScrollbarWidth`；零值字段跟随主题，因此两者都不设置的主题，绘制效果和以往一样。默认值由 MyGo 自己决定：12 DIP 宽的滚动条中放一个 6 DIP 宽的圆角滑块，距两端 2 DIP。可以在主题中为应用的所有滚动条设置样式，也可以在某个容器上设置它自己的样式，或在编辑器的 `State.Scrollbar` 上设置：

```go
t := *c.Theme()
// As VS Code's editor: a 14 DIP square thumb in a 14 DIP bar, flush
// with its ends.
t.ScrollbarStyle = ui.ScrollbarStyle{
	TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true, MinLength: 20,
	Thumb: ui.RGBA(121, 121, 121, 0.4),
	Hover: ui.RGBA(100, 100, 100, 0.7),
	Active: ui.RGBA(191, 191, 191, 0.4),
}
c.SetTheme(&t)
```

`TrackWidth` 是滚动条轨道和命中区域的宽度，以 DIP 计，0 表示 `ThumbWidth` 加 6；`ThumbWidth` 是滑块的宽度，0 表示主题的 `ScrollbarWidth`；`Radius` 让滑块圆角化，0 表示宽度的一半；`Square` 让角保持方形，和 VS Code 的滚动条一样；`Flush` 让滑块与轨道两端之间不留间隙，和 VS Code 的一样，否则留出 2 DIP；`MinLength` 是滑块的最小长度，0 表示 24 DIP。`Track` 填充滑块背后的滚动条，零值不绘制任何内容。`Thumb` 给静止时的滑块着色，`Hover` 在指针位于滚动条上时着色，`Active` 在它被拖动时着色；零值 `Thumb` 采用主题的 `Scrollbar`，零值 `Hover` 和 `Active` 采用 `Thumb`。常驻的滚动条（`KeepScrollbars`）显示它的 `Hover` 颜色，和悬停时一样。

容器自己的样式优先于主题的：

```go
ui.Scroll(c).ScrollbarStyle(ui.ScrollbarStyle{TrackWidth: 14, ThumbWidth: 14, Square: true, Flush: true}).Children(func() { /* ... */ })
```

`ScrollbarInsets` 以类似 CSS `Padding` 的方式设置滚动条距容器边缘的距离：垂直滚动条的上、下端分别避开顶部和底部指定距离，整体距右边缘指定距离；水平滚动条的左、右端分别避开左右指定距离，整体距底边指定距离。这样就能避开覆盖在滚动内容上方的工具栏等元素，与 AppKit 和 UIKit 的做法相同：

```go
ui.Scroll(c).Fill().Padding(64, 16, 16).ScrollbarInsets(64, 0, 0).Children(func() { /* ... */ })
// The toolbar floats over the top 64 DIPs.
ui.Row(c).Absolute().Top(0).Left(0).Right(0).Height(64).Children(func() { /* ... */ })
```

对于自行绘制外观或指示当前位置的容器，例如轮播或地图，可用 `Scrollbars(false)` 隐藏滚动条，同时禁用其点击区域；滚轮、`TrackScroll`、键盘和辅助技术仍可滚动容器。

`ScrollbarAxes(horizontal, vertical)` 独立控制两个指示器及其指针命中目标。对于自带垂直导航的视图，保留水平滚动条并隐藏垂直滚动条：

```go
ui.ScrollBoth(c).Fill().ScrollbarAxes(true, false).Children(func() { /* ... */ })
```

隐藏的轴仍可通过滚轮、触摸板、`TrackScroll`、键盘焦点和辅助技术滚动。如果隐藏正在拖动的轴，这次拖动会被取消，下面的内容也不会被点击。剩下的滚动条保留它的轨道，并让共享的角落空着。`Scrollbars(false)` 无论调用顺序如何都会隐藏两者；`Scrollbars(true)` 保留轴的设置。重复调用 `ScrollbarAxes` 时以最后一次的值为准；下一次构建中省略它，就会重新启用两个轴。

`KeepScrollbars(true)` 让滚动条在内容溢出时始终显示，不论指针是否在容器上，适合需要让用户一眼看出自己在内容中位置的视图，例如填满窗口的长日志或差异。它们看起来和悬停时一样，如果样式有轨道就显示在轨道上，所以指针移到它们上面时什么都不会改变，也像悬停时一样接受按压；`Scrollbars(false)` 和 `ScrollbarAxes` 仍会隐藏它们。这三项都适用于 `Scroll`、`ScrollHorizontal`、`ScrollBoth`、`List` 和 `Table`，省略时保持默认值，且不会被继承：

```go
ui.List(c, &app.rows, len(app.lines), app.line).Fill().KeepScrollbars(true)
```

自行滚动而不经过容器的元素，例如代码编辑器和终端，用 `ui.Scrollbar` 绘制滚动条，让它们的外观与容器一致，并跟随主题：`Painter.Scrollbar` 绘制轨道和滑块，`Thumb` 和 `OffsetAt` 把对滑块的拖动映射为偏移。在滚动条上绘制标记的视图先绘制 `ScrollbarTrack`，再绘制标记，然后绘制 `ScrollbarThumb`。像容器那样显示：指针位于元素上或正在拖动滑块时显示，或者在应用让滚动条常驻时一直显示，如同代码编辑器的 `KeepScrollbars`：

```go
bar := ui.Scrollbar{Box: r, View: r.H, Content: contentH, Offset: scrollY}
if hovered || dragging {
	p.Scrollbar(bar, ui.ScrollbarState{Hovered: hovered, Dragging: dragging})
}
```

内部用滚轮、触摸板和捏合进行缩放或平移的元素，例如图表，会从容器那里接管这些操作：见[滚动与捏合](input.md#滚动和捏合)。

## 辅助功能

辅助技术看到一个滚动区域，并滚动它以显示正在读取的内容。
