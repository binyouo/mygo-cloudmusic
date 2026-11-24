# 可编辑文本

<!-- preview editable-text 120 -->

`ui.EditableText` 把一个 `*string` 显示为可就地编辑的文本，如同 Finder 中的文件名：双击文本，或在文本具有焦点时按 Enter，会显示文本框，并选中扩展名之前的文本。按 Enter 或移开焦点会保留输入的内容，Escape 则恢复原值；`Changed` 报告新值。

```go
if ui.EditableText(c, &app.title).Changed() {
	app.save()
}
```

## 在列表和表格中

在[列表](list.md)或[表格](table.md)的行中，焦点保留在列表上，文本不单独获取焦点：macOS 上按 Return，其他平台上按 F2，会编辑选中行的文本；双击行则触发 `Submitted`。选中一行后，等到双击判定间隔结束，再单击该行的文本即可编辑。

```go
ui.Table(c, &app.files, cols, len(files), func(row, col int) {
	if col == 0 && ui.EditableText(c, &files[row].Name).Changed() {
		app.rename(files[row])
	}
})
```

## 辅助功能

辅助技术读取文本，以及编辑时的文本框。
