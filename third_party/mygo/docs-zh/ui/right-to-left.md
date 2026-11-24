# 从右到左

阿拉伯语、希伯来语、波斯语和乌尔都语从右到左阅读，这些语言的应用界面也如此：一行从右侧开始，侧边栏在右侧，后退箭头指向右侧。应用使用这些语言时，原生 UI 会相应调整窗口布局，如同 Flutter 的 `Directionality`、SwiftUI 的 `layoutDirection`、CSS 的 `direction` 和 GTK 的文本方向。与 Godot 一样，只有布局镜像，坐标系不变：`Rect`、`Draw` 绘制的内容以及指针位置中的 X 坐标仍然向右增大。

## 方向

`ui.LeftToRight` 和 `ui.RightToLeft` 是两个方向（`ui.Direction`）。窗口采用应用语言的方向，由包 `mygo` 设置；每个元素则采用它所在元素的方向。`Direction` 让元素及其中构建的内容拥有自己的方向，例如电话号码、代码示例或媒体播放器的控件，它们在从右到左的窗口中仍保持从左到右：

```go
ui.Row(c).Direction(ui.LeftToRight).Gap(8).Children(func() {
	ui.Button(c, "⏮")
	ui.Button(c, "⏯")
	ui.Button(c, "⏭")
})
```

要像 `Key` 一样，在创建元素后立即设置方向，再构建子元素：控件会在构建时读取方向。附着在元素上的弹出内容（浮层、组合框选项、工具提示）以及 `Overlay` 构建的内容，都继承发起调用处的方向。

`Context.Direction` 返回当前构建内容的方向；自定义控件绘制箭头等具有水平方向的图形时，可以据此调整：

```go
forward := "M5 2 11 8 5 14"
if c.Direction() == ui.RightToLeft {
	forward = "M11 2 5 8 11 14"
}
```

## 什么会镜像

从右到左：

- `Row` 从右侧排布子元素，`Column` 在横向上从右侧对齐子元素：`Justify`、`AlignItems` 和 `AlignSelf` 的 `Start` 和 `End` 随布局方向变化。设置 `Reverse` 后，行改为从左侧排布。网格从右侧开始排列各列，`JustifyItems` 和 `JustifySelf` 的起始侧也是单元格的右侧。
- `TextAlign` 的 `Start` 指元素右侧，`End` 指左侧，不受文本自身阅读方向影响，与 CSS、AppKit 和 GTK 一致。段落的阅读方向则由 Unicode 双向算法根据首个强方向字符决定；没有此类字符的段落（例如纯数字或空行）采用从右到左方向。在从左到右布局中，原有规则不变：阿拉伯语段落仍从右侧开始，符合 Apple 按段落语言对齐的指南。
- 逻辑边缘的起始侧位于右边：`PaddingStart`、`PaddingEnd`、`MarginStart`、`MarginEnd`、`BorderStartWidth`、`BorderEndWidth`、定位元素的 `Start` 和 `End`（以及 `StartPercent`、`EndPercent`），以及锚点 `AnchorTopStart`、`AnchorStart`、`AnchorBottomStart`、`AnchorTopEnd`、`AnchorEnd` 和 `AnchorBottomEnd`，如同 CSS 的逻辑属性和 Flutter 的 `EdgeInsetsDirectional` 与 `AlignmentDirectional`。没有从任何一侧定位的绝对元素会去右边；同时给出 `Left` 和 `Right`、而宽度容不下两者时，`Right` 优先。
- 横向滚动从右边开始，`ScrollState.X` 从那里开始计数（0 显示起始，`MaxX` 显示末尾），如同 Flutter 的偏移；滚轮和触控板像从左到右时那样移动内容，Shift 加滚轮则朝末尾滚动。垂直滚动条在左边。
- 控件中具有水平方向的部分会镜像：开关的开启位置在左边；滑块、范围滑块、进度条和量度计从右边填充，方向键按箭头方向移动滑块；标签页、分段控件、工具栏、评分、网格视图和日历用 Left 前往下一项；展开箭头、面包屑的尖角符号和工具栏的溢出按钮指向左，后退按钮则指向右；树、大纲和展开控件从右侧缩进，并用 Left 展开；分栏的第一个窗格在右侧；表格的列从右侧开始，并从左边缘调整大小；路由器推入的页面从左侧滑入，Alt+Right 则后退。
- 浮层、选择器和组合框的选项，以及菜单按钮的菜单，在打开它们的元素的起始侧打开，上下文菜单按从右到左排版，并在指针左侧打开。
- 单行文本框的文本和占位文本都从右侧开始，空文本框的插入符也在右侧。无论文本方向如何，方向键都按箭头所指的方向移动插入符；在从右到左的段落中，Option（其他平台上为 Control）和 Command 配合方向键，分别按箭头方向逐词移动和移至行的两端，与 AppKit 和 GTK 一致。

## 什么不会镜像

- 使用物理方向的属性不变，如同 CSS：`Left`、`Right`、`Padding(top, right, bottom, left)`、`BorderWidth`、`AnchorTopLeft`、`Radius` 的各个角、渐变角度和变换。
- 文本、图像、图标和 `Draw` 绘制的内容不会镜像：对勾、放大镜、加载指示器和图表仍按从左到右的方式阅读。必须指向另一边的内容，要在读取 `Context.Direction` 之后再绘制。
- 代码从左到右阅读：代码编辑器和差异查看器（`ui/editor`）、Markdown 的代码块和终端保持自己的方向，时间输入也如此，因为时间就是这样书写的。
- 富文本编辑器（`ui/richtext`）按其文本阅读的方式对齐段落，如同文稿那样。
- 检查器从左到右阅读。

## 试用

`MYGO_DIRECTION=rtl` 把原生 UI 的每个窗口从右到左排版（`ltr` 则从左到右），无论应用的语言是什么，以便在界面翻译之前试用，如同 GTK 的 `GTK_DEBUG=invert-text-dir` 和 Qt 的 `-reverse`：

```sh
MYGO_DIRECTION=rtl mygo dev
```

[检查器](inspector.md)的 RTL 切换按钮可以在运行时更改当前窗口的布局方向。布局测试中，调用 `SetDirection(ui.RightToLeft)` 后，`Tester` 也会将测试窗口改为从右到左布局：

```go
tt := ui.NewTester(app.view, 400, 300)
tt.SetDirection(ui.RightToLeft)
back, _ := tt.Find("Back")
if back.X < 200 {
	t.Error("the back button is not at the right")
}
```

[gallery](../../examples/gallery)可以通过其工具栏中的切换按钮，以从右到左的方式显示每个控件。
