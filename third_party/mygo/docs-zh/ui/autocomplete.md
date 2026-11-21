# 自动补全

<!-- preview autocomplete 260 -->

`ui.Autocomplete` 创建一个编辑 `*string` 的文本输入框，下方显示字符串列表中包含输入内容的建议，以输入内容开头的排在前面。Up 和 Down 切换建议，Enter 或单击采用建议；未选中建议时按 Enter 会触发 `Submitted`。无论是输入文本还是采用建议，内容变化都会触发 `Changed`。

```go
ui.Autocomplete(c, &app.city, cities).Label("City")
```

与[组合框](combobox.md)不同——后者的值是它的某个选项——自动补全保留用户输入的内容。

## 辅助功能

辅助技术将其识别为一个由 `Label` 命名、可像文本框一样编辑的组合框，建议列表用于补全输入内容（`aria-autocomplete="list"`）；方向键当前指向的建议会作为焦点项提供给辅助技术。
