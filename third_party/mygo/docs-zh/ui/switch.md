# 开关

<!-- preview switch 130 -->

`ui.Switch` 创建一个开关，用于切换 `*bool` 的值，适合立即生效的设置：单击会切换状态，获得键盘焦点后按 Space 也一样，滑块随之滑动。`Changed` 报告这次操作。

```go
ui.Row(c).Gap(12).Children(func() {
	ui.Text(c, "Wi-Fi").Grow(1)
	ui.Switch(c, &app.wifi).Label("Wi-Fi")
})
```

开关本身不显示文本：为它设置供辅助技术使用的 `Label`，或把它放进[字段](form.md)，字段的标签为它命名，单击标签会切换开关：

```go
ui.Form(c, func() {
	ui.Field(c, "Sound", func() { ui.Switch(c, &app.sound) })
})
```

若某个选择要在表单保存时才生效，用[复选框](checkbox.md)。

## 无外观

`ui.SwitchBase` 是无外观的开关，由你根据值绘制：见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一个开关，处于开启或关闭状态，由 `Label` 或它所在的字段命名。
