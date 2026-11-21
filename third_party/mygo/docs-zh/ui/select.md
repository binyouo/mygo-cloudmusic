# 选择器

<!-- preview select 230 -->

`ui.Select` 创建一个下拉选择器，从字符串列表中选一个值写入 `*string`：单击它，或它具有键盘焦点时按 Enter、Space、Up 或 Down，会在它下方打开选项，方向键、Home 和 End 在选项之间移动，Enter、Space 或单击选中一项；Escape 关闭选项。`Changed` 报告新选择。

```go
ui.Select(c, &app.plan, []string{"Free", "Pro", "Team"}).Label("Plan")
```

下方没有空间时，弹出层显示在选择器上方，关闭时把焦点还给选择器。选项多到窗口容纳不下时，它的高度等于可用空间，选项可以滚动。

若列表较长，需要通过输入筛选，使用[组合框](combobox.md)；若选项较少且需要同时显示，使用[单选按钮](radio.md)。

## 无外观

`ui.SelectBase` 是无外观的选择器，可选择任何可比较类型的值：一个 `Trigger` 打开由 `Item` 组成的 `Popup`，方向键高亮它们（`Highlighted`），Enter 选中。`Highlight` 把高亮移到某个选项，你自己实现的选择器可以在用户输入选项首字母时这样做：

```go
sel := ui.SelectBase(c, &app.size)
sel.Trigger.Gap(6).Padding(6, 10).Radius(8).Border(1, t.Border).Children(func() {
	ui.Text(c, app.size)
	ui.Icon(c, chevron)
})
sel.Popup(func(panel ui.Element) {
	panel.Margin(4, 0, 0, 0).Padding(4).Radius(10).Background(t.Background).Border(1, t.Border)
	for _, size := range sizes {
		item := sel.Item(size).Padding(6, 10).Radius(6)
		if item.Highlighted() {
			item.Background(t.Accent).TextColor(t.AccentText)
		}
		item.Children(func() { ui.Text(c, size) })
	}
})
```

`SetOpen` 用于打开和关闭弹出层，例如在关闭的触发器上输入可以打开它并高亮所输入的选项，或按 Tab 移开时关闭它；弹出层打开时，高亮落在当前选择上，随后可由 `Highlight` 移动；若选项尚未构建，则等构建之后再移动：

```go
if !sel.Open() && sel.Trigger.Shortcut(0, ui.KeyL) {
	sel.SetOpen(true)
	sel.Highlight("L")
}
if !sel.Trigger.Focused() {
	sel.SetOpen(false) // Tab moved on
}
```

`ui.SelectBaseOpen` 改由你提供打开状态，类似 [`PopoverBase`](popover.md)：`*open` 为 true 时显示弹出层，选择器打开和关闭时会更新该值，你也可以设置它。自行管理状态后，就能在关闭时暂时保留弹出层以播放动画，例如让列表从触发器处展开，再收回；收回期间不接受输入：

```go
// The select closed the list since the last pass: it stays until it
// has shrunk back into the trigger.
if !app.open && app.listShown {
	app.open, app.closing = true, true
}
sel := ui.SelectBaseOpen(c, &app.size, &app.open)
sel.Trigger.Height(32).Padding(0, 10).Children(func() { ui.Text(c, app.size) })
var target float32
if app.open && !app.closing {
	target = 1
}
grow := sel.Trigger.Animate("grow", target, 200*time.Millisecond)
if app.closing && grow == 0 {
	app.open, app.closing = false, false
}
panel := sel.Popup(func(panel ui.Element) {
	// From the trigger's height to the options', over the trigger.
	full := float32(len(sizes)) * 32
	panel.AttachTo(sel.Trigger, ui.AnchorTopStart, ui.AnchorTopStart).Height(32 + grow*(full-32)).ClipY().
		IgnoreInput(app.closing).Radius(8).Background(t.Background).Border(1, t.Border)
	for _, size := range sizes {
		sel.Item(size).Height(32).Padding(0, 10).Children(func() { ui.Text(c, size) })
	}
})
app.listShown = panel.Valid()
```

见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一个弹出按钮，其值是当前选择，由 `Label` 命名，并在它打开时看到一个包含其选项的列表框（`ui.RoleListBox`、`ui.RoleOption`），每个选项都会说明它在全部选项中的位置。如同原生选择器的弹出层，方向键或指针所在的选项就是当前选中的选项，选择器保有焦点时焦点会跟随该选项；辅助技术聚焦某个选项时会高亮它，与方向键一样，按下某个选项则会选中它，与单击一样。
