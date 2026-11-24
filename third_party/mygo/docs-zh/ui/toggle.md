# 切换按钮

<!-- preview toggle 150 -->

`ui.Toggle` 创建一个在 `*bool` 为 true 时保持按下状态的按钮，如同编辑器工具栏中的 Bold：单击会按下或松开它，具有键盘焦点时按 Space 也一样，`Changed` 报告变化。

```go
ui.Toggle(c, &app.inspector, "Inspector")
```

用 `Children` 和空标签为它提供其他内容，例如图标，并用 `Label` 命名：

```go
ui.Toggle(c, &app.bold, "").Label("Bold").Children(func() { ui.Icon(c, boldIcon) })
```

## 切换按钮组

`ui.ToggleGroup` 将内部的切换按钮和普通按钮连在一起绘制，呈现为同一个控件的多个分段，例如加粗、斜体和下划线。整组只占一个 Tab 焦点停留点，方向键在组内移动焦点，与[工具栏](toolbar.md)相同：

```go
ui.ToggleGroup(c, func() {
	ui.Toggle(c, &app.bold, "B")
	ui.Toggle(c, &app.italic, "I")
	ui.Toggle(c, &app.underline, "U")
}).Label("Style")
```

`FillEqually` 使按钮组填满父元素宽度，组内切换按钮等宽分配空间，与[分段控件](segmented.md#全宽)的用法相同。

若要在几个选项中选一个，例如视图模式，使用[分段控件](segmented.md)。

## 无外观

`ui.ToggleBase` 是无外观的切换按钮，提供一个单击时切换 `*bool` 值的行容器；见[自定义控件](custom-widgets.md)。

## 辅助功能

辅助技术看到一个切换按钮，开启时处于按下状态，并把切换按钮组看作一个由 `Label` 命名的组。
