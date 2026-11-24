# 浮层

浮层显示在窗口内容上方，包括带有遮暗背景的[对话框](dialog.md)、[警告对话框](alert-dialog.md)、显示在元素下方的[弹出面板](popover.md)、[下拉选择框](select.md)和[组合框](combobox.md)的弹出内容、在窗口中绘制的菜单（[`PopoverMenu`](context-menu.md#在窗口中绘制)、[`PopoverMenuButton`](menu-button.md#在窗口中绘制)，以及启用[窗口内菜单绘制](context-menu.md#窗口的每个菜单)后的所有菜单）、[工具提示](tooltip.md)和[短暂提示](toast.md)。这些浮层对键盘和背后内容的处理方式相同。虽然显示在窗口内容之上，浮层仍采用创建位置的主题：在 [`Themed`](styling.md#窗口一部分的主题) 内打开时使用该处主题，在其中构建的 `ui.Overlay` 元素也一样；短暂提示则使用窗口主题。

```go
more := ui.Button(c, "More ▾")
if more.Clicked() {
	app.menu = !app.menu
}
ui.Popover(c, more, &app.menu, func() {
	if ui.Button(c, "Rename").Clicked() {
		app.menu, app.renaming = false, true
	}
})
ui.Modal(c, &app.renaming, func() {
	ui.Text(c, "Rename").Bold()
	if ui.TextInput(c, &app.name).AutoFocus().Submitted() {
		app.renaming = false
	}
})
```

## 键盘

对话框会将键盘焦点限制在内部：打开时，焦点落在其中第一个可聚焦的元素上，除非某个元素指定了 `AutoFocus`；Tab 只在内部元素间循环，对话框外部构建的窗口快捷键暂不响应，背后的内容不可交互，也不会向屏幕阅读器暴露。使用 Tab 导航时，弹出面板中的元素紧随其锚点元素。

Escape 关闭最上层浮层，例如先关闭下拉选择框的弹出列表，再关闭所在的对话框，除非当前焦点元素（例如终端）接管该按键。如果浮层关闭时焦点仍在内部，会将焦点还给打开前的焦点元素，例如打开浮层的按钮。

在弹出面板外按下会关闭面板，并将事件继续传给指针下的内容，与 Web 的弹出面板一样：单击另一个按钮会关闭面板，同时触发该按钮。下拉选择框的弹出列表则会拦截这次按下，与系统弹出菜单一样；单击对话框的背景遮罩会关闭对话框。

## 你自己的浮层

`ui.Overlay` 在所有其他内容之上构建元素，用 `Absolute` 按窗口的 DIP 放置：

```go
ui.Overlay(c, func() {
	ui.Text(c, "Offline").Absolute().Top(12).Right(12).
		Padding(4, 10).Radius(999).Background(t.Danger).TextColor(t.AccentText)
})
```

`PopoverBase` 和 `DialogBase` 是无外观的浮层和对话框，用于你自己设计的浮层：见[自定义控件](custom-widgets.md)。两者都不适用的浮层，例如抽屉、悬停卡片或菜单栏的菜单，用构成它们的基础部件构建：

- `AttachTo(target, at, self)` 把一个元素放在另一个元素旁边，无论目标在哪里：把它的点 `self` 放在目标的点 `at` 上，例如 `AttachTo(button, ui.AnchorBottomLeft, ui.AnchorTopLeft)` 放在按钮下方。如果会超出窗口，它就翻到另一侧，或沿目标的另一个方向放置，然后移回窗口内；纵向滚动的元素不会高于目标较大一侧的空间，并会滚动。它的外边距使它和目标保持距离。它是目标的浮层：它的元素随 Tab 移动而跟随目标。
- `AttachEdgeMargin(v)` 让这类元素与窗口边缘保持 `v` DIP 的距离，默认为 4：宽面板需要避免贴到窄窗口边缘时，可用 `AttachEdgeMargin(12)`，作用类似 CSS Anchor Positioning 中包含块的内缩距离。
- `AttachedAnchors` 返回最终采用的锚点，类似 Floating UI 的定位结果：原本要放在按钮下方、实际翻到上方的面板会返回 `ui.AnchorTop, ui.AnchorBottom`。视图读取上一帧的锚点，`Draw` 读取当前帧的；面板首次显示时，视图先于布局构建，因此读到的是请求的锚点。首帧效果会直接采用布局确定的实际方向：`AttachedOrigin` 将变换原点设为元素实际使用的自身锚点，让面板从最靠近目标的边缘缩放进入；`Enter` 或 `Exit` [动效](transitions.md#出现与离开)的 `Flip` 会镜像偏移，使原本在按钮下方向下滑入的菜单翻到上方时改为向上滑入：

  ```go
  tip := ui.Column(c).AttachTo(b, ui.AnchorBottom, ui.AnchorTop).Margin(6, 0, 0, 0).
  	AttachedOrigin().Transition(ui.ElementTransition{
  		Duration: 150 * time.Millisecond, ExitDuration: 100 * time.Millisecond,
  		Enter: &ui.Motion{Scale: 0.96, Y: -4, Flip: true},
  		Exit:  &ui.Motion{Scale: 0.96, Y: -4, Flip: true},
  	})
  _, self := tip.AttachedAnchors()
  tip.AlignItems(ui.Center).Children(func() {
  	if self == ui.AnchorTop { // below the button, as asked
  		ui.Text(c, "▲")
  	}
  	ui.Text(c, "Copied")
  	if self == ui.AnchorBottom { // above it, without room below
  		ui.Text(c, "▼")
  	}
  })
  ```
- `PressedOutside` 报告在元素、它的目标以及其中内容所开的浮层之外发生的按下，浮层据此关闭；`PressedInside` 报告它们之内的一次按下，而不接收指针。元素显示之前的按下不算在它的外部：右键单击打开的浮层会保持打开，因为这次按下在同一帧打开了它。
- `OverlayShortcut` 接收具有焦点的元素及其周围元素未处理的按键，无论焦点在哪里，最后构建的浮层优先，如同 Escape 关闭最上面的那个。
- `Modal` 让元素充当对话框的背景遮罩：焦点移入并限制在内部，外部构建的窗口快捷键暂不响应，屏幕阅读器只能访问该元素及其上方的内容。

在 `ui.Overlay` 顶层构建的元素消失时，如果焦点仍在其内部，就会还给该元素出现前的焦点元素。下面是右侧抽屉，可用 Escape 或单击背景遮罩关闭：

```go
if app.drawer {
	ui.Overlay(c, func() {
		back := ui.Row(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Justify(ui.End).
			Background(ui.RGBA(0, 0, 0, 0.3)).Modal()
		back.Children(func() {
			panel := ui.Column(c).Width(320).FillHeight().Padding(16).Background(t.Background).
				Role(ui.RoleDialog).Label("Filters").Children(app.filters)
			if panel.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				app.drawer = false
			}
		})
	})
}
```

原生[对话框](../native.md#对话框)也可以使用：从 goroutine 调用它们，以便视图不必等待它们。
