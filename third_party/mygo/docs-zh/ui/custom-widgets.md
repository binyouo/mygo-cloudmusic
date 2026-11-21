# 自定义控件

每个控件都基于一个无预设外观的基础版本，由它处理指针、键盘和焦点，并向辅助技术提供控件信息。颜色、大小和形状由返回的元素决定，你可以像普通元素一样为它们设置样式，实现自己的设计，类似 Web 上的无预设外观组件库。

| 基础 | 它做什么 |
|---|---|
| `ButtonBase` | 一个可获得焦点的行，为指针、Enter 和 Space 报告 `Clicked` |
| `CheckboxBase`、`SwitchBase`、`ToggleBase` | 一个切换 `*bool` 的行 |
| `CheckboxStateBase` | 支持混合状态的复选框，设置 `*ui.CheckState`（`CheckOff`、`CheckOn`、`CheckMixed`），按下可勾选，已勾选时按下则取消勾选（[复选框](checkbox.md#无外观)） |
| `RadioBase` | 一个把选中的值写入 `*T` 的行 |
| `SliderBase` | 根据指针在内容框（内边距之内）中横向的位置，或纵向的位置（`Vertical`）设置 `*float64`；也可用方向键、Page Up 和 Page Down、Home 和 End 按 `Step` 调整 |
| `TabsBase` | 标签页的 `List`，其中各个 `Tab` 将选中项写入 `*int`；方向键移动选中项和焦点，跳过禁用项 |
| `SegmentedBase` | 一个由 `Segment` 组成的 `Track`，即一个选择 `*int` 的单选组 |
| `CollapsibleBase` | 一个 `Trigger`，显示和隐藏 `Panel` 构建的内容，用 `Progress` 让它展开和收起，后者从 0（关闭）到 1（打开） |
| `AccordionBase` | 一个由各节组成的 `Root`，每个 `Item` 是一个 `CollapsibleBase`，方向键、Home 和 End 在它们的触发器之间移动焦点（[手风琴](accordion.md#无外观)） |
| `TreeBase` | 一个由 `Item` 组成的 `Root`，每个 `Item` 是一个 `Row`，其 `*bool` 为 true 时还是包含其中各项的 `Group`，方向键、Home、End 和键入移动焦点，并打开和关闭它们（[树](tree.md#无外观)） |
| `SelectBase` | 一个 `Trigger`，打开由 `Item` 组成的 `Popup`，从中选择一个 `*T`，方向键高亮（`Highlighted`），Enter 选择；`SetOpen` 打开和关闭它，`SelectBaseOpen` 由你提供打开标志 |
| `ComboboxBase` | 一个 `Input`，其文本过滤它下方 `Popup` 的 `Item`，方向键高亮，Enter 或单击选择（`Chosen`），没有匹配项时显示你自己的一行（[组合框](combobox.md#无外观)） |
| `PopoverBase`、`DialogBase` | 锚点旁的面板，或覆盖窗口的背景之上的面板，按下外部或按 Escape 会关闭 |
| `MenuBase`、`ContextMenuBase` | 菜单按钮的 `Trigger`，或元素的上下文菜单，打开由 `Item`、`CheckItem`、`RadioItem` 和 `SubmenuItem`（及其 `Submenu`）组成的 `Popup`，指针和按键高亮（`Highlighted`）并选择（`Clicked`），除非 `KeepOpen` 否则关闭菜单，按键把 `Group` 中的项当作一个焦点停留点（[菜单按钮](menu-button.md#无外观)） |
| `TooltipBase` | 指针停留在锚点上或键盘焦点在锚点上时显示的提示，按下或按 Escape 会隐藏（[工具提示](tooltip.md#无外观)） |
| `ToastViewportBase`、`ToastBase` | 窗口的 toast 位于你自己的视口中，每个是一个带有 `ActionButton` 和 `CloseButton` 的状态，指针或焦点在它们上面时暂停计时（[toast](toast.md#无外观)） |
| `TextInputBase`、`TextAreaBase` | 没有内边距、背景、边框或圆角的文本输入框，可设置 `ReadOnly` |

图表也有基础版本：[`ui/chart`](chart.md) 包提供比例尺、坐标轴、标记，以及无预设外观的选区。

基础版本的构造函数返回值类型的部件，其构建生命周期与其他元素相同。有状态的基础版本需要在构造前用 `Context.Key` 指定键。

基于 `SegmentedBase` 的分段控件，以及基于 `SelectBase` 的选择器：

```go
t := c.Theme()
view := ui.SegmentedBase(c.Key("period"), &app.view, 3)
view.Track.Padding(3).Radius(999).Background(t.Surface).Children(func() {
	for i, name := range []string{"Day", "Week", "Month"} {
		seg := view.Segment(i).Padding(5, 14).Radius(999)
		if i == app.view {
			seg.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.15))
		}
		seg.Children(func() { ui.Text(c, name) })
	}
})

sel := ui.SelectBase(c.Key("size"), &app.size)
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

与其他获得焦点的元素一样，基础会用主题的焦点环为获得键盘焦点的元素画环：宽度为 `FocusRingWidth`，位于元素边缘外 `FocusRingOffset` 处，颜色为 `Focus`（见[焦点环](styling.md#焦点环)）。`FocusRingOffset(d)` 移动元素的焦点环，`d` 为负时向内移，例如盒子中会裁剪超出部分的标题；`FocusRingColor` 设置它的颜色。`FocusRing(false)` 为自行绘制焦点环的控件关闭焦点环，这类控件在 `FocusVisible` 时用 `Painter.FocusRing` 绘制。

禁用元素及其内部内容（包括基础版本）都以一半的不透明度显示。`DimDisabled(false)` 关闭这种自动变淡效果，让控件自行表现禁用状态：例如卡片保持清晰可读，并说明不可用原因，或按你设定的样式将选项变灰。在 `IsDisabled` 为 true 时设置样式：

```go
card := ui.RadioBase(c, &app.plan, p.ID).Disabled(!p.Available).DimDisabled(false)
card.Column().Gap(4).Padding(12).Radius(8).Border(1, t.Border).Children(func() {
	name := ui.Text(c, p.Name)
	if card.IsDisabled() {
		name.TextColor(t.TextMuted)
		ui.Text(c, p.DisabledReason).FontSize(12)
	}
})
```

若控件在不可用时仍必须可获得焦点，以便屏幕阅读器到达，就用 [`AccessDisabled`](accessibility.md#不可用但仍可到达) 而不是 `Disabled` 告诉屏幕阅读器，并在此时为它设置样式。

## 你自己的控件

若文本控件有自己的缓冲区或选择模型，使用 [`HandleTextInput` 和保留的文本几何信息](text-input-client.md)。

用元素构建你自己的控件。元素用 `ui.Local` 在帧之间保留自己的状态：

```go
// Spoiler hides text until clicked.
func Spoiler(c *ui.Context, text string) {
	t := c.Theme()
	box := ui.Box(c).Padding(2, 6).Radius(4).Focusable()
	shown := ui.Local(box, "shown", func() bool { return false })
	if box.Clicked() {
		*shown = !*shown
	}
	if *shown {
		box.Background(t.Surface)
	} else {
		box.Background(t.Text) // the color of the text, hiding it
	}
	box.Children(func() { ui.Text(c, text) })
}
```

### 停止控件启动的工作

`Local` 的 `init` 创建的东西属于元素。当某一帧不再构建该元素时，它的状态随之消失；若某个值是 `io.Closer`（`T` 或 `*T` 有 `Close() error`），它会被关闭，就像 Web 在元素被移除时中止 `AbortController`。在后台启动工作的控件，例如流式请求、计时器或订阅，要在 `Close` 中停止这些工作：

```go
type stream struct{ cancel context.CancelFunc }

func (s *stream) Close() error { s.cancel(); return nil }

// Feed shows the lines read from url while it is in the view.
func Feed(c *ui.Context, win *mygo.Window, app *App, url string) {
	e := ui.Column(c)
	ui.Local(e, "stream", func() *stream {
		ctx, cancel := context.WithCancel(context.Background())
		go read(ctx, url, func(line string) {
			win.Update(func() { app.lines = append(app.lines, line) })
		})
		return &stream{cancel}
	})
	e.Children(func() {
		for _, line := range app.lines {
			ui.Text(c, line)
		}
	})
}
```

- `Close` 只运行一次，在主线程上，位于不再构建该元素的那一帧之后；元素最后创建的 `Local` 最先关闭，与延迟调用的执行顺序相同。窗口会等待它：应取消工作后立即返回，而不是等待工作自行停止。
- `Close` 可以在主线程上改变应用的状态；用 `Window.Invalidate`，或控件保存的 `c.Services()` 的 `Invalidate`，请求新的一帧来显示变化。
- 元素以退出[过渡](transitions.md#出现与离开)离开时，其副本会继续绘制原有内容；退出结束、副本消失后，才会关闭元素及子元素的值。若在退出结束前重新构建该元素，会重新运行 `init`，并关闭副本的值。
- [列表](list.md)只构建视口中的行：行滚出视口时会关闭其值，再次进入视口时用 `init` 重新创建。需要在行销毁后继续保留的数据，应按列表项保存在应用状态中。
- [路由器](navigation.md#页面保留其状态)会保留历史页面中各元素的值，直到页面从历史中移除时才关闭这些值。
- 窗口关闭时，其中每个元素的值都会关闭；应用退出时也是如此。
- `Close` 中发生的 panic 会像视图中一样从帧中抛出；其余的值会在下一帧之后关闭。
- nil 值不会被关闭，例如 `init` 留待以后启动的 nil `*stream`；`Close` 的错误会被忽略。
- 元素不应关闭的值（例如应用共享的值）应放在应用状态中，而不是 `Local` 中。
- 若控件只需要在元素消失时运行一个函数（例如 `cancel`），把它交给元素的 `OnUnmount` 即可；它会像 `Close` 一样运行该函数，无需自己的类型（[元素的状态](views.md#元素的状态)）。

若用户输入改变了控件所编辑的内容，就在构建时调用元素的 `SetChanged`：此后该元素的 `Changed` 会返回 true，与内置控件的行为一致。[富文本编辑器](rich-text-editor.md)就是这样做的。

为控件提供[角色](accessibility.md#角色)、名称和它的[状态](accessibility.md#状态)，供辅助技术使用；用它元素上的 `Shortcut` 接收它需要的按键，或用 [`HandleInput`](input.md#每个键随到随处理) 接收每一个到来的按键；用 [`Draw`](drawing.md) 绘制元素不绘制的内容。你自己的浮层，例如抽屉、悬停卡片和菜单，用 `AttachTo` 放置，用 `PressedOutside` 和 `OverlayShortcut` 关闭；`Modal` 把焦点保持在一个浮层中：见[浮层](overlays.md#你自己的浮层)。
