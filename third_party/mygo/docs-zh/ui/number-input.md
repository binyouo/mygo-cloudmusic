# 数字输入

<!-- preview number-input 160 -->

`ui.NumberInput` 创建一个用于编辑 `*float64` 的文本框，数值限制在指定范围内；Up、Down 及旁边的按钮按步长调整数值。输入内容成为范围内的有效数字后立即生效，失去焦点时，按步长的小数位数舍入显示。`Changed` 报告新值。`Width` 设置包含按钮在内的整体宽度；文本框会相应收窄，给按钮留出空间。

```go
ui.NumberInput(c, &app.copies, 1, 99, 1).Label("Copies")
ui.NumberInput(c, &app.opacity, 0, 1, 0.05).Label("Opacity")
```

若只需按步长调整数值，用[步进器](stepper.md)；若只需粗略选择，用[滑块](slider.md)；金额无法用 `float64` 精确表示，应使用[小数输入](decimal-input.md)。

## 辅助功能

辅助技术看到一个表示该值的微调按钮，由 `Label` 或它所在的[字段](form.md)命名，可按步长增减它。
