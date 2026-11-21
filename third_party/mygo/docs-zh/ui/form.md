# 表单

<!-- preview form 340 -->

`ui.Field` 为它的函数构建的控件加上标签：标签会为辅助技术命名控件，单击标签会聚焦控件，或单击复选框、开关或单选按钮。`Description` 和 `Error` 会在控件下方添加文本，辅助技术会连同控件一起读取；错误还会把控件的值标记为无效，文本框则用主题的 `Danger` 颜色绘制边框。错误为空时，控件保持有效：

```go
var invalid string
if app.email != "" && !strings.Contains(app.email, "@") {
	invalid = "Enter an email address."
}
ui.Field(c, "Email", func() {
	ui.TextInput(c, &app.email)
}).Description("For receipts.").Error(invalid)
```

控件是函数构建的第一个可获得焦点的元素，或这样的一组元素，如[单选按钮组](radio.md)。

## 表单

字段的标签位于控件上方，与 Web 上一样。在 `ui.Form` 中，字段的标签位于控件旁独立的一列，如同 macOS 的表单：右对齐，宽度与最宽的一致，并与控件第一行文本对齐；对于没有文本的控件（如开关）则居中。

```go
ui.Form(c, func() {
	ui.Field(c, "Name", func() { ui.TextInput(c, &app.name) })
	ui.Field(c, "Plan", func() { ui.Select(c, &app.plan, plans) })
	ui.Field(c, "Sound", func() { ui.Switch(c, &app.sound) })
})
```

## 字段组

`ui.Fieldset` 将字段归为一组，组标题同时作为该组的名称；其中字段的标签与表单中的其他标签对齐，`Disabled` 会禁用整组字段：

```go
ui.Form(c, func() {
	ui.Field(c, "Name", func() { ui.TextInput(c, &app.name) })
	ui.Fieldset(c, "Notifications", func() {
		ui.Field(c, "Email", func() { ui.Checkbox(c, &app.mail, "Weekly summary") })
		ui.Field(c, "Sound", func() { ui.Switch(c, &app.sound) })
	}).Disabled(!app.signedIn)
})
```

本身就带名称的控件（如带文本的复选框）会保留自己的名称，字段的标签则命名它们所在的组。

## 自己的标签

自行绘制控件说明文字的视图，例如组件自身布局中位于输入框上方的文本，用 `LabelledBy` 把两者关联起来，如同 Web 上的 `<label for>`：这段文本代替控件的 `Label` 为辅助技术命名控件，单击它会聚焦控件，或单击复选框、开关或单选按钮，与字段的标签一样。标签可以在控件之前或之后出现：

```go
caption := ui.Text(c, "Card number").TextColor(t.TextMuted)
ui.TextInput(c, &app.card).LabelledBy(caption)

ui.Row(c).Gap(8).Children(func() {
	agree := ui.Checkbox(c, &app.agree, "")
	agree.LabelledBy(ui.Text(c, "I agree to the terms"))
})
```

## 自己的提示和错误

若视图把提示或错误消息作为独立元素绘制，而不放在字段的 `Description` 和 `Error` 中，可以用 `DescribedBy` 和 `ErrorMessage` 让辅助技术读到它们，作用如同 `aria-describedby` 和 `aria-errormessage`。`Invalid` 可以在没有消息的情况下把值标记为无效，例如用户留空的必填字段，其消息在表单提交后显示：

```go
hint := ui.Text(c, "As printed on the card").TextColor(t.TextMuted)
ui.Field(c, "Name", func() {
	ui.TextInput(c, &app.name).Invalid(app.nameTouched && app.name == "")
}).DescribedBy(hint)
```

在字段上使用时，它们会作用于该字段的控件，与 `Description` 和 `Error` 一样（[辅助功能](accessibility.md#描述和错误)）。

## 辅助功能

辅助技术以字段标签作为控件名称，随后读出描述和错误；存在错误或设置了 `Invalid` 时，会将值标为无效。字段组识别为以组标题命名的组。
