# 网格视图

<!-- preview grid-view 310 -->

`ui.GridView` 以网格展示条目，类似照片应用的图库或 Finder 的图标视图：在满足最小列宽的前提下尽量多排几列，各列均分剩余宽度，行高按指定值设置。它只构建可见条目，因此即使有数百万项也能保持流畅。`Gap` 设置条目间距。

```go
app.photos.Selected = &app.photo
ui.GridView(c, &app.photos, len(photos), 140, 120, func(i int) {
	ui.Image(c, thumbs[i]).Fit(ui.Contain).Grow(1)
	ui.Text(c, photos[i].Name).SingleLine()
}).Grow(1)
```

## 网格状态

`GridState` 保存网格的滚动位置并控制条目的选择方式，与列表行的 `ListState` 类似；也可以传入 `nil`：

- `Selected` 允许单击选择条目；网格获得键盘焦点时，还可用方向键在行列间移动、用 Home 和 End 跳到首尾，并用 Enter 提交选择（`Submitted`）。
- `Selection` 让用户选择多个项，以 `Key` 区分：Cmd 单击会添加或移除一项；Shift 单击和 Shift 加方向键会选择从最后选中项开始的一段；Cmd+A 选择全部。
- `Label` 返回项的文本：输入它的首字母即可选中它，辅助技术把它读作项的名称。
- `Reorder` 让用户把项拖到其他位置，如同列表的行：[拖放](drag-and-drop.md)。

## 辅助功能

辅助技术把网格读作一个项的列表，每一项都会说明它在全部项中的位置，无论是否已构建。
