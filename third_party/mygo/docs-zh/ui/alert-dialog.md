# 警告对话框

<!-- preview alert-dialog 240 -->

`ui.AlertDialog` 在 `*bool` 为 true 时，在窗口上方显示警告框，用于确认重要操作，类似 AppKit 的警告框：包含标题、消息和多个按钮。最后一个是默认按钮，使用强调色并获得键盘焦点，按 Enter 即可触发。无论按钮的文字和语言是什么，按 Escape 都会关闭警告框，效果相当于取消，不选择任何按钮；单击警告框外部则无效。单击按钮的那一帧会关闭警告框，并返回该按钮的索引；按 Escape 关闭的那一帧返回 `ui.AlertDismissed`，其余情况返回 -1。

```go
if ui.Button(c, "Delete").Clicked() {
	app.asking = true
}
switch ui.AlertDialog(c, &app.asking, "Delete “Notes”?", "You can't undo this.", "Cancel", "Delete") {
case 1:
	app.delete()
}
```

必须得到回答的警告框，可以在 Escape 将其关闭时把 `*bool` 设回 true，从而保持打开：

```go
if ui.AlertDialog(c, &app.updating, "Update required", "Restart to finish updating.", "Restart") == ui.AlertDismissed {
	app.updating = true
}
```

警告框会在窗口允许的范围内超出通常的宽度，让按钮留在同一行，标题和消息也按这个宽度换行。在窄到无法做到这一点的窗口里，按钮会换到更多行；长过警告框的标签则以省略号结尾。

要展示你自己内容的对话框，用[对话框](dialog.md)；对于用户可以撤销的选择，立即执行，并在 [toast](toast.md) 中提供撤销。

## 辅助功能

辅助技术看到一个由其标题命名、由其消息描述的警告对话框，焦点在它的默认按钮上。
