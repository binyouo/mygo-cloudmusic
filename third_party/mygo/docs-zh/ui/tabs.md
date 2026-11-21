# 标签页

<!-- preview tabs 160 -->

`ui.Tabs` 创建一行显示标题的标签页，`*int` 保存所选标签页的索引。单击可选择标签页；标签页获得键盘焦点后，也可用方向键、Home 和 End 选择，焦点随选择移动。`Changed` 报告选择变化。在标签栏下方构建所选页面：

```go
ui.Tabs(c, &app.tab, "General", "Appearance", "Advanced")
switch app.tab {
case 0:
	app.general(c)
case 1:
	app.appearance(c)
case 2:
	app.advanced(c)
}
```

整行标签页只占一个 Tab 焦点停留点，按 Tab 会聚焦当前选中的标签页。如果页面需要历史记录并支持后退，用[路由器](navigation.md)；如果要在工具栏中切换视图，用[分段控件](segmented.md)。

## 无外观

`ui.TabsBase` 是无外观的标签页列表：一个容纳标签页的 `List`，其中的 `Tab` 选择索引，方向键移动选择和焦点。带 `Disabled(true)` 的标签页无法单击，方向键、Home 和 End 会跳过它：

```go
tabs := ui.TabsBase(c, &app.tab, len(names))
tabs.List.Gap(4).Children(func() {
	for i, name := range names {
		tab := tabs.Tab(i).Padding(6, 12).Radius(6).Disabled(!app.enabled[i])
		if i == app.tab {
			tab.Background(t.Surface)
		}
		tab.Children(func() { ui.Text(c, name) })
	}
})
```

见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术将其识别为标签页列表，按各标签页的标题命名，并能读取选中和禁用状态。
