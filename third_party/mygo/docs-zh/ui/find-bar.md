# 查找栏

<!-- preview find-bar 200 -->

`ui.FindBar` 在 `*bool` 为 true 时显示查找栏，类似 Safari 和 Xcode 的查找栏：包括查询文本框（打开时获得焦点，并选中上次的查询文本）、匹配项数量、用于切换当前匹配项的“上一个”和“下一个”按钮，以及“完成”按钮。

```go
if c.Shortcut(ui.Cmd, ui.KeyF) {
	app.finding = true
}
matches := app.find(app.query) // the app counts them
ui.FindBar(c, &app.finding, &app.query, len(matches), &app.match)
if app.finding && len(matches) > 0 {
	app.show(matches[app.match])
}
```

应用负责执行查找：统计查询的匹配项并显示当前匹配项，`Changed` 报告当前匹配项的切换。

## 键盘

在文本框中按 Enter 跳到下一个匹配项，Shift+Enter 跳到上一个；macOS 上的 Cmd+G 和 Shift+Cmd+G、其他平台上的 F3 和 Shift+F3 也一样，到达两端后循环查找；Escape 和“完成”关闭查找栏。

关闭时，它返回一个不显示任何内容的元素。

## 辅助功能

辅助技术将其识别为名为“查找”的工具栏，其中包含搜索框，并在匹配项数量变化时读出计数。名称、计数（“第 3 项，共 12 项”、“无匹配项”）和“完成”都使用窗口的语言（[MyGo 自己的字符串](../i18n.md#mygo-自己的字符串)）。
