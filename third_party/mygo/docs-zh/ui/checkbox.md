# 复选框

<!-- preview checkbox 220 -->

`ui.Checkbox` 创建带标签的复选框，用于切换 `*bool` 的值：单击复选框或标签会勾选或取消勾选，获得键盘焦点后按 Space 也一样，`Changed` 报告这次操作。

```go
ui.Checkbox(c, &app.settings.Sync, "Sync across devices")
if ui.Checkbox(c, &app.showHidden, "Show hidden files").Changed() {
	app.reload()
}
```

用户单击时，复选框会立即修改值：直接读取即可；需要进一步处理时，用 `Changed` 判断是否发生了操作。

## 组

`ui.CheckboxGroup` 在一组复选框上方创建一个总复选框，内部的复选框缩进显示，例如权限列表：全部勾选时，总复选框显示为勾选状态；部分勾选时显示为混合状态。单击总复选框，或在它获得焦点后按 Space，会全部勾选；若已全部勾选，则全部取消。`Changed` 报告这次操作。

```go
ui.CheckboxGroup(c, "Notifications", func() {
	ui.Checkbox(c, &app.mail, "Mail")
	ui.Checkbox(c, &app.calendar, "Calendar")
	ui.Checkbox(c, &app.messages, "Messages")
})
```

## 混合

`ui.CheckboxState` 是支持混合状态的复选框，如表头中的“全选”框：全部行都勾选时显示为勾选状态，部分行勾选时显示为混合状态。它设置 `*ui.CheckState`，取值为 `ui.CheckOff`、`ui.CheckOn` 或 `ui.CheckMixed`，混合状态下显示短横线；单击它，或在它获得焦点后按 Space，会勾选它，已勾选时则取消勾选，`Changed` 报告这次操作：

```go
all := ui.CheckOff
switch n := app.selected(); {
case n == len(app.rows):
	all = ui.CheckOn
case n > 0:
	all = ui.CheckMixed
}
if ui.CheckboxState(c, &all, "").Label("Select all").Changed() {
	app.selectAll(all == ui.CheckOn)
}
```

`CheckboxGroup` 就是这样一个复选框，位于它所容纳的复选框之上。

## 无外观

`ui.CheckboxBase` 是无外观的复选框：单击一行即可切换它的 `*bool`，外观由你根据值绘制；`ui.CheckboxStateBase` 则设置 `*ui.CheckState`，如 `CheckboxState` 所做：见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术将其识别为由标签命名的复选框，能读取勾选状态，并像单击一样切换它；支持混合状态的复选框还会报告混合状态。复选框组包含一个总复选框（部分勾选时为混合状态），以及由其标签命名的一组子复选框。
