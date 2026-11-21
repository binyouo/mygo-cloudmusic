# 单选按钮

<!-- preview radio 160 -->

`ui.Radio` 创建带标签的单选按钮，将选中的值写入 `*T`：单击即可选中，获得键盘焦点时按 Space 也一样，`Changed` 报告选择变化。值可以是任何可比较类型。

`ui.RadioGroup` 把单选按钮组当作一个 Tab 焦点停留点：Tab 会跳到选中的那一个，方向键在它们之间选择，Home 和 End 也一样。

```go
ui.RadioGroup(c, func() {
	for _, size := range []string{"Small", "Medium", "Large"} {
		ui.Radio(c, &app.size, size, size)
	}
}).Label("Size")
```

单选按钮组默认是一列；用 `Row` 可以把它变成一行：

```go
ui.RadioGroup(c, func() {
	ui.Radio(c, &app.align, ui.Start, "Left")
	ui.Radio(c, &app.align, ui.Center, "Center")
	ui.Radio(c, &app.align, ui.End, "Right")
}).Row().Gap(16).Label("Alignment")
```

要从许多项中选择，用[选择](select.md)或[组合框](combobox.md)；若要在视图之间切换，用[分段控件](segmented.md)。

## 无外观

`ui.RadioBase` 是无外观的单选按钮，以行布局呈现，单击时选中其值：见[自定义控件](custom-widgets.md)，其中的示例卡片在禁用时仍保持清晰可读（`DimDisabled(false)`）。

## 辅助功能

辅助技术看到一个由 `Label` 命名的单选按钮组，其中单选按钮由各自的标签命名，选中的那个处于选中状态。
