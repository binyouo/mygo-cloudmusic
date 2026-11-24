# 日历

<!-- preview calendar 340 -->

`ui.Calendar` 创建按月显示的日历，用来选择 `*time.Time` 的日期，类似 SwiftUI 的图形日期选择器：单击可选择日期，日历有键盘焦点时也可用方向键选择；Page Up、Page Down 或日历上的按钮按月切换，Home 和 End 跳到当月的第一天和最后一天。`Changed` 报告新日期，保留原值的时分秒与时区。

```go
ui.Calendar(c, &app.day).Label("Due date")
```

它从用户区域格式（`Context.Region`：美国为周日，欧洲大部分地区为周一）的每周第一天起显示六周，前后月份的日子显示为灰色，并标记今天。它的标题（"January 2026"、"2026年1月"）、星期几的名称，以及供辅助技术使用的日期名称，都取自窗口语言对应的 CLDR 数据，日期使用该语言的数字；按钮的名称则是 [MyGo 自己的字符串](../i18n.md#mygo-自己的字符串)。

若要在紧凑的文本框中选择日期，用[日期输入](date-input.md)，它会在下方打开这个日历。

## 浏览而不选择

`Browse` 让用户浏览日历而不改变选中的日期：日历按钮、Page Up 和 Page Down 只切换月份，方向键、Home 和 End 只移动焦点，单击或按 Enter、Space 才会选中日期，与日期输入中的日历一样。此时 `Changed` 只报告用户确认选择的日期，适合选中日期后就关闭的[浮层](popover.md)日历：

```go
chip := ui.Button(c, task.Due.Format("Jan 2"))
if chip.Clicked() {
	app.dueOpen = !app.dueOpen
}
ui.Popover(c, chip, &app.dueOpen, func() {
	if ui.Calendar(c, &task.Due).Browse().Changed() {
		app.dueOpen = false
	}
})
```

应用设置另一个日期时，日历会回到该值所在的月份。

## 辅助功能

辅助技术看到一个以月份命名的表格，其中每一天都是以其日期命名的按钮，选中的那一天处于勾选状态；方向键移动时会朗读选中的日期，使用 `Browse` 时则朗读方向键所在的日期。
