# 手风琴

<!-- preview accordion 300 -->

`ui.Accordion` 在带边框的容器中纵向排列多个分区，每个分区由 `ui.AccordionItem` 构建：单击标题栏，或按 Enter、Space，可以展开或折叠分区；`*bool` 为 true 时，回调函数在标题栏下方构建内容。各分区独立开合；Up 和 Down 在标题之间移动焦点，Home 和 End 分别跳到首尾标题。每个分区的 `Changed` 报告单击。

```go
ui.Accordion(c, func() {
	ui.AccordionItem(c, "General", &app.general, func() { app.generalSettings(c) })
	ui.AccordionItem(c, "Privacy", &app.privacy, func() { app.privacySettings(c) })
	ui.AccordionItem(c, "Advanced", &app.advanced, func() { app.advancedSettings(c) })
})
```

若一次只展开一个分区，就在它展开时折叠其余分区：

```go
for i, s := range sections {
	open := app.section == i
	ui.AccordionItem(c, s.Title, &open, s.Build).OnChange(func() {
		app.section = -1
		if open {
			app.section = i
		}
	})
}
```

分区展开时，按 Tab 可将焦点从标题移入内容。

## 无外观

`ui.AccordionBase` 是无外观的手风琴，用于你自己的布局中的标题和面板：一个 `Root` 列容纳各节，每一节由 `Item` 创建，是一个在 `*bool` 为 true 时打开的 [`CollapsibleBase`](collapsible.md#无外观)，其 `Trigger` 是标题，其 `Panel` 构建内容。标题的按键与 `Accordion` 相同：Up 和 Down 在标题之间移动焦点，Home 和 End 也一样，Enter 和 Space 则打开和关闭它们。

```go
acc := ui.AccordionBase(c)
acc.Root.Gap(4).Children(func() {
	for i := range app.sections {
		s := &app.sections[i]
		item := acc.Item(&s.open)
		item.Trigger.Gap(8).Padding(8, 12).Radius(8).Children(func() {
			ui.Icon(c, s.icon)
			ui.Text(c, s.title).Grow(1)
			ui.Text(c, s.summary).TextColor(t.TextMuted)
			ui.Icon(c, chevron).Rotate(180 * item.Progress())
		})
		item.Panel(func() { s.build(c) })
	}
})
```

`Accordion` 是带主题外观的 `AccordionBase`：它的标题把焦点环画在标题内部（`FocusRingOffset`），因为盒子会裁剪超出它的部分。见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术将每个标题识别为展开控件（disclosure），状态为展开或折叠，顺序位于对应分区的内容之前。
