# 树

<!-- preview tree 280 -->

`ui.Tree` 创建一棵树，其项目由 `ui.TreeItem` 构建，每个项目构建在它所属的项目内。项目的 `*bool` 为 true 时显示它的子项目；单击它的箭头会打开或关闭它，`Clicked` 报告单击它的其他位置或按 Enter（用于选中它），`Selected` 则显示选中状态。

```go
item := func(path, label string, open *bool, children func()) {
	if ui.TreeItem(c, label, open, children).Selected(app.chosen == path).Clicked() {
		app.chosen = path
	}
}
ui.Tree(c, func() {
	item("src", "src", &app.srcOpen, func() {
		item("src/main.go", "main.go", nil, nil)
	})
	item("go.mod", "go.mod", nil, nil)
})
```

子项目回调为 nil 或 open 参数为 nil 时，该项目就是叶节点。

## 键盘

整棵树只占一个 Tab 焦点停留点，Tab 会将焦点移到上次获得焦点的项目，或标记为 `Selected` 的项目。项目获得键盘焦点后，Up 和 Down 将焦点移到相邻项目，Home 和 End 移到首尾项目，Right 展开项目或移到第一个子项目，Left 收起项目或移到父项目；从右到左布局时，Left 展开，Right 收起。输入项目名称的开头字母可将焦点移到该项目，再次输入同一字母则移到下一个以该字母开头的项目。

树会构建它显示的每个项目。对于数千个项目的树，或带列的树，使用[大纲](outline.md)，它只构建可见的行。

## 无外观

`ui.TreeBase` 是不预设外观的树，可自定义各行，例如 JSON 查看器：`Root` 是容纳项目的列，每个项目由 `Item` 创建；你可以为其 `Row` 设置样式和内容；其 `*bool` 为 true 时，`Group` 在行下方的列中构建子项目（nil 表示叶节点）。键盘操作和焦点行为与 `Tree` 相同，`Depth` 提供项目深度，可用于设置行缩进。在 `Item` 之前用 `Context.Key` 设置键，上方项目展开或收起时，焦点就能保持在同一项目上。

```go
var node func(n *Node)
tree := ui.TreeBase(c)
tree.Root.Children(func() {
	node = func(n *Node) {
		c.Key(n.path)
		item := tree.Item(n.open) // nil for a leaf
		row := item.Row.Gap(4).Padding(3, 8).PaddingStart(8 + 16*float32(item.Depth())).Radius(6)
		if app.chosen == n.path {
			row.Background(t.Surface)
		}
		if row.Clicked() {
			app.chosen = n.path
		}
		row.Children(func() {
			if n.open != nil {
				arrow := ui.Icon(c, chevron).Size(16, 16)
				if *n.open {
					arrow.Rotate(90)
				}
				if arrow.Clicked() {
					*n.open = !*n.open
				}
			}
			ui.Text(c, n.key).FontWeight(500)
			ui.Text(c, n.value).TextColor(t.TextMuted)
		})
		item.Group(func() {
			for _, kid := range n.children {
				node(kid)
			}
		}) // nil while closed; style it, as with a line along it
	}
	for _, n := range app.roots {
		node(n)
	}
})
```

`Tree` 是带主题外观的 `TreeBase`。`TreeBase` 构建每个显示出来的项目；对于数千个项目的树，使用带有你自己 `Prefix` 的[大纲](outline.md#自己的行)。见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一棵由项目组成的树，每个项目都有层级，在有子项目时还有打开或关闭状态，并说明它在显示的项目中的位置，如同大纲的行；它像箭头一样打开和关闭它们。
