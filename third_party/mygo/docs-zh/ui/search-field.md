# 搜索框

<!-- preview search-field 200 -->

`ui.SearchField` 创建用于搜索的文本框，编辑 `*string`：包含放大镜图标、文本，以及有内容时显示的清空按钮；此时按 Escape 也会清空。为空时显示“Search”，或用 `Placeholder` 设置文字，说明搜索范围。`Changed` 报告文本变化，按 Enter 时由 `Submitted` 报告提交。

```go
if ui.SearchField(c, &app.query).Label("Search mail").Changed() {
	app.filter()
}

ui.SearchField(c, &app.pageQuery).Placeholder("Search pages, or #tag to filter")
```

若要在用户输入时搜索，就在 `Changed` 时筛选；若搜索较耗时，则在 `Submitted` 时搜索，或用 `c.After` 在停顿之后搜索。

## 辅助功能

辅助技术会将其识别为由 `Label` 命名的搜索框（macOS 上的 `AXSearchField`，Linux 上带 `xml-roles:searchbox` 的输入框，Windows 上标为“search box”的 Edit 控件）。自绘图标和清空按钮的文本输入可用 `Search` 声明为搜索框，对应 ARIA 的 `searchbox`：

```go
ui.TextInput(c, &app.query).Placeholder("Search").Label("Search mail").Search()
```
把它连同执行搜索的按钮（如果有）放进一个 `Role(ui.RoleSearch)` 的元素，即一个搜索地标，屏幕阅读器会列出它并跳转到它（[辅助功能](accessibility.md#地标实时区域和分隔线)）。
