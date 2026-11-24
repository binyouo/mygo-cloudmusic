# 日期输入

<!-- preview date-input 320 -->

`ui.DateInput` 创建日期输入框，按用户所在地区的数字日期格式显示 `*time.Time`，例如美国的 1/5/2026、英国的 05/01/2026 或中国的 2026/1/5，通过下方的[日历](calendar.md)修改日期。单击或按 Enter、Space 打开日历；在日历中单击，或用方向键配合 Enter 选择日期；Page Up、Page Down 或日历上的按钮按月切换，Escape 关闭日历。`Changed` 报告新日期，保留原值的时分秒与时区。

```go
ui.Field(c, "Birthday", func() {
	ui.DateInput(c, &app.birthday)
})
```

在它旁边放一个[时间输入](time-input.md)，两者编辑同一个 `time.Time`：

```go
ui.Row(c).Gap(8).Children(func() {
	ui.DateInput(c, &app.meeting).Label("Date")
	ui.TimeInput(c, &app.meeting).Label("Time")
})
```

## 辅助功能

辅助技术看到一个弹出按钮，其值是日期，由它的 `Label` 或它所在的[字段](form.md)命名；还会看到打开时的日历。
