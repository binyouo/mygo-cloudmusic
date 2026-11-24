# 可折叠

<!-- preview collapsible 220 -->

`ui.Collapsible` 创建一个展开控件，类似 SwiftUI 的 `DisclosureGroup`：标签旁有一个箭头，单击或按 Enter、Space 可展开和折叠。`*bool` 为 true 时，传入的函数构建的内容显示在下方。展开时箭头旋转，内容逐渐展开显示。`Changed` 报告单击。

```go
ui.Collapsible(c, "Advanced", &app.advanced, func() {
	ui.Checkbox(c, &app.verbose, "Verbose logging")
	ui.Checkbox(c, &app.experimental, "Experimental features")
})
```

系统设置了减少动效时，内容会立即显示。

若要在一个容器中放置多个可折叠分区，用[手风琴](accordion.md)。

## 无外观

`ui.CollapsibleBase` 是无外观的可折叠控件：`Trigger` 显示和隐藏 `Panel` 构建的内容，并用 `Progress` 让它展开和收起，后者从 0（关闭）到 1（打开），供你实现自己的外观和动效：

```go
p := ui.CollapsibleBase(c, &app.open)
p.Trigger.Gap(6).Children(func() {
	ui.Icon(c, chevron).Rotate(90 * p.Progress())
	ui.Text(c, "Details")
})
p.Panel(func() {
	ui.Text(c, app.details)
})
```

见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术识别为由标签命名的展开控件，可读取其展开或折叠状态，也可像单击一样展开和折叠内容。
