# 面包屑导航

<!-- preview breadcrumbs 180 -->

`ui.Breadcrumbs` 创建路径导航，如同 Finder 的路径栏和 AppKit 的路径控件，各项之间用尖角符号分隔：除最后一项外，其余都是链接，单击或按 Enter 选中后，会把索引写入 `*int`，并由 `Changed` 报告变化。空间不足时，较长的条目会以省略号截断。

```go
path := []string{"Macintosh HD", "Users", "ada", "Documents"}
if ui.Breadcrumbs(c, path, &app.chosen).Label("Path").Changed() {
	app.open(path[:app.chosen+1])
}
```

最后一项是用户所在的位置，它不是链接。

## 辅助功能

辅助技术会将其识别为导航地标中的链接，`Label` 为地标命名，屏幕阅读器可在窗口的各地标之间跳转；最后一项则被识别并朗读为当前页（`ui.CurrentPage`）。自定义路径导航可用 `Role(ui.RoleNavigation)` 和 [`Current`](accessibility.md#状态) 表达相同语义：

```go
ui.Row(c).Gap(6).Role(ui.RoleNavigation).Label("Breadcrumb").Children(func() {
	ui.Link(c, "Home", "/")
	ui.Text(c, "/").AccessHidden(true)
	ui.Text(c, "Settings").Current(ui.CurrentPage)
})
```
