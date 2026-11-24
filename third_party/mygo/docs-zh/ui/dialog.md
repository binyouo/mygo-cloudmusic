# 对话框

<!-- preview dialog 260 -->

`ui.Modal` 在 `*bool` 为 true 时，在变暗的窗口上方显示一个由它的函数构建的对话框；单击它外面或按 Escape 会把它设为 false。

```go
if ui.Button(c, "Rename…").Clicked() {
	app.renaming = true
}
ui.Modal(c, &app.renaming, func() {
	ui.Text(c, "Rename").Bold()
	if ui.TextInput(c, &app.name).AutoFocus().Submitted() {
		app.renaming = false
	}
	ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
		if ui.Button(c, "Cancel").Clicked() {
			app.renaming = false
		}
		if ui.PrimaryButton(c, "Rename").Clicked() {
			app.rename()
			app.renaming = false
		}
	})
})
```

## 键盘

对话框打开后，键盘焦点限制在其内部：默认聚焦第一个可聚焦元素，若有元素设置了 `AutoFocus`，则优先聚焦该元素。Tab 在内部元素之间循环；背后的内容无法交互，在对话框外构建的窗口快捷键也会暂停响应。关闭时，焦点恢复到原来的元素，例如打开对话框的按钮。见[浮层](overlays.md)。

## 让它保持打开

`Dismissed` 在 Escape 或单击外部关闭对话框的那一帧报告；必须保持打开的对话框（例如有更改要保存）此时把 `*bool` 设回 true：

```go
if ui.Modal(c, &app.editing, app.editor).Dismissed() && app.unsaved {
	app.editing = true
}
```

要询问重要的事，需要按钮且单击外部不会关闭，用[警告对话框](alert-dialog.md)。

## 窗口之外

对话框绘制在所属窗口内，超出窗口边缘的部分会被裁剪：在托盘面板等浮出窗口中显示的对话框，宽度不会超过浮出窗口。若要超出此范围，应另开一个以浮出窗口为父窗口的模态窗口，它会显示在浮出窗口上方，并随父窗口关闭（见[叠在浮出窗口上的对话框](../windows.md#叠在浮出窗口上的对话框)）。

## 无外观

`ui.DialogBase` 是无外观的对话框：由它的函数为覆盖窗口的背板（背板使面板居中）和面板设置样式，并构建面板的内容：

```go
ui.DialogBase(c, &app.open, func(backdrop, panel ui.Element) {
	backdrop.Background(ui.RGBA(0, 0, 0, 0.5))
	panel.Width(360).Padding(24).Radius(16).Background(t.Background)
	ui.Text(c, "Welcome").FontSize(20).Bold()
})
```

见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一个对话框，并在它显示时看不到它背后的任何内容。
