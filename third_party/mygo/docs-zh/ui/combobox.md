# 组合框

<!-- preview combobox 280 -->

`ui.Combobox` 创建一个组合框，在选项中选定一个 `*string`：一个显示当前选择的文本框，用户输入时其文本会过滤下方的选项，以输入内容开头的排在前面；单击或按 Down 则显示全部。单击显示全部时会选中当前值，以便直接输入替换。Up 和 Down 在选项之间移动，Enter 选择，单击也一样；Escape 关闭选项，文本框失去焦点时，文本恢复为当前选择。`Changed` 报告新的选择。

```go
ui.Combobox(c, &app.font, fonts).Label("Font")
```

用户输入和挑选时，文本框保持键盘焦点。`Placeholder` 在文本为空时显示一段占位文本。

若文本可以是任意内容并需要建议，用[自动补全](autocomplete.md)；若是一份无需输入的短列表，用[选择](select.md)。

## 无外观

`ui.ComboboxBase` 是无外观的组合框：`Input` 的文本用于筛选下方 `Popup` 中的 `Item`，方向键移动高亮，Enter 或单击确认选择（`Chosen`）：见[自定义控件](custom-widgets.md)。如果选项与输入内容匹配（如下例），用 [`cb.Input.AutoComplete(ui.AutoCompleteList)`](accessibility.md#建议) 向辅助技术声明这一点。

没有选项的弹出层会隐藏，除非它含有其他可读内容，例如一行“没有匹配项”的提示，这时它会显示在选项的位置上：

```go
cb.Popup(func(panel ui.Element) {
	panel.Padding(4).Background(t.Background).Border(1, t.Border)
	matches := app.matching(app.query)
	for _, f := range matches {
		cb.Item(f).Padding(6, 10).Children(func() { ui.Text(c, f) })
	}
	if len(matches) == 0 {
		ui.Text(c, "No matches found").Padding(6, 10).TextColor(t.TextMuted)
	}
})
```

## 辅助功能

辅助技术会将其识别为由 `Label` 命名、可展开的组合框，能像文本框一样编辑，选项用于补全输入内容（`aria-autocomplete="list"`）；方向键高亮的选项会被识别为焦点项，并朗读为“第几项，共几项”。没有选项时，焦点留在文本框中；弹出层中替代选项的提示行会在出现时被读出。
