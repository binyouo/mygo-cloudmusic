# 原生 UI

窗口可以显示由 MyGo 自己绘制的用户界面，而不是网页。你用 Go 和 `ui` 包来写它：没有 HTML，没有 JavaScript，也没有前端构建，窗口不会启动 webview，因此能立刻打开，内存占用也很少。MyGo 用 GPU 绘制它：macOS 上用 Metal，Windows 上用 Direct3D 11，Linux 上用 OpenGL。

```go
package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// counter is the app's state.
type counter struct{ n int }

// view builds the window's interface from the state, for every frame.
func (app *counter) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, fmt.Sprint(app.n)).FontSize(40).Bold()
		if ui.PrimaryButton(c, "Increment").Clicked() {
			app.n++
		}
	})
}

func main() {
	app := &counter{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:   "Counter",
			Width:   320,
			Height:  240,
			Content: ui.View(app.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

**示例约定。** 指南中的示例都是 `counter.view` 这类视图方法的一部分：`c` 是视图的 `*ui.Context`，`app` 是方法的接收者，即应用用来保存状态的自定义类型实例。`app.volume` 这样的字段和 `app.save()` 这样的方法都需要在该类型上自行声明，正如 `counter` 声明了 `n`；详见[视图](views.md)。

`mygo init -template native my-app` 会创建一个原生 UI 项目（见 [CLI](../cli.md#mygo-init)）。[counter 示例](../../examples/counter-native)包含一个视图测试，[gallery](../../examples/gallery) 展示各种控件和布局。

一个应用可以同时拥有这两类窗口。原生 UI 适合工具、设置、检查器和实用程序，以及必须瞬间启动的应用；网页则适合富文档、已有的 Web 代码，以及任何只有浏览器才能胜任的场景。屏幕阅读器和其他辅助技术会像读取其他应用一样读取原生 UI（见[辅助功能](accessibility.md)）。

## 指南

- [视图](views.md)：构建结果的生命周期、事件查询与回调、键，以及从其他 goroutine 更新状态。
- [布局](layout.md)：行与列、尺寸、间距、对齐和定位。
- [文本](text.md)：文本及其样式、富文本、句子内的链接和字体。
- [样式与主题](styling.md)：背景、边框、阴影和渐变，以及跟随桌面的主题。
- [输入](input.md)：指针、键盘焦点、快捷键、按键状态、输入法、拖入的文件，以及需要接收所有按键的控件。
- [拖放](drag-and-drop.md)：在窗口内拖动的值，以及通过拖动重新排序的行。
- [页面与导航](navigation.md)：带历史记录的页面路由器、布局、过渡，以及后退和前进。
- [浮层](overlays.md)：对话框和浮层共用的行为：焦点管理、Escape 关闭，以及如何处理后方内容。
- [辅助功能](accessibility.md)：屏幕阅读器看到的内容、命名元素、角色、描述和播报。
- [从右到左](right-to-left.md)：面向阿拉伯语和希伯来语的从右到左布局、逻辑边缘，以及镜像规则。
- [语言](../i18n.md#原生-ui)：窗口的语言和区域格式、用它们书写的数字和日期，以及控件自身的字符串。
- [绘制与动画](drawing.md)：在元素上绘制、文本塑形，以及缓动到目标的值。
- [着色器](shaders.md)：用 Go 语法编写着色器，在 GPU 上计算像素，并从应用传入数值和图像。
- [过渡](transitions.md)：布局变化时平滑调整位置、尺寸和颜色，以及元素的入场和离场动画。
- [动效](motion.md)：保留速度的弹簧、曲线、延迟、拖动与惯性、关键帧、用同一播放头驱动多个补间的时间线，以及动画完成检测。
- [变换](transforms.md)：任何元素的视觉平移、缩放和旋转，指针、文本输入和辅助功能都跟随它们。
- [自定义控件](custom-widgets.md)：所有控件共用的无外观基础，以及如何构建自己的控件。
- [使用原生 UI 的窗口](windows.md)：窗口选项、由视图绘制的标题栏、缩放内容，以及材质。
- [测试](testing.md)：在没有窗口的情况下运行视图，和单元测试一样快。
- [检查器](inspector.md)：在内容旁查看窗口元素、盒模型、样式和每帧耗时。
- [渲染](rendering.md)：MyGo 如何在 GPU 或 CPU 上绘制。
- [浏览器中的原生 UI](browsers.md)：将同一个应用编译为 WebAssembly，在网页中用 WebGL 2 绘制。
- [迁移到带校验的元素](migration.md)：元素值、持久句柄、焦点绑定和源码迁移。
- [原生 UI 性能](performance.md)：帧开销和常驻内存的实测结果。

## 动作

- [按钮](button.md)：普通按钮，或使用强调色的按钮。
- [链接](link.md)：打开 URL 或路由器页面的文本。
- [菜单按钮](menu-button.md)：在下方打开菜单的按钮，菜单可以是系统的，也可以是 MyGo 按主题外观在窗口中绘制的。
- [拆分按钮](split-button.md)：主按钮旁带下拉箭头，可打开相关操作的菜单，如“保存”和“另存为…”。
- [上下文菜单](context-menu.md)：为任意元素添加右键菜单，使用系统菜单或由 MyGo 在窗口内按主题绘制；也可统一自绘[窗口的每个菜单](context-menu.md#窗口的每个菜单)，包括文本框和表格的菜单。
- [切换按钮](toggle.md)：一个保持按下的按钮，以及连成一组的切换按钮。
- [分段控件](segmented.md)：通过多个分段切换视图。
- [工具栏](toolbar.md)：沿窗口的一行控件，溢出时进入菜单。

## 选择

- [复选框](checkbox.md)：一个复选框，以及控制一组其他复选框的复选框。
- [开关](switch.md)：一个打开和关闭设置的开关。
- [单选按钮](radio.md)：在组中选择一个值的单选按钮。
- [选择器](select.md)：从列表中选择一项的下拉菜单。
- [滑块](slider.md)：在范围内设置值的滑块，可带步长，也可不带。
- [范围滑块](range-slider.md)：两个旋钮设置低值和高值。
- [步进器](stepper.md)：上下步进值的箭头。
- [评分](rating.md)：设置评分的星星。

## 文本输入

- [文本输入](text-input.md)：一行或多行的文本，带有选择、撤销、剪贴板和输入法。
- [文本输入原语](text-input-client.md)：由应用管理文本，处理原生输入法的组合输入与查询，并由自定义控件维护几何信息。
- [数字输入](number-input.md)：范围内的数字，键入或步进。
- [小数输入](decimal-input.md)：通过键入或步进编辑金额等精确小数，不做舍入。
- [搜索框](search-field.md)：用于搜索的字段，Escape 会清除它。
- [组合框](combobox.md)：从列表中选择一项，输入文本可筛选列表。
- [自动补全](autocomplete.md)：根据已输入的文本提供补全建议。
- [标记字段](token-field.md)：以小标签呈现的字符串列表，如标签或收件人。
- [可编辑文本](editable-text.md)：就地重命名的文本，如 Finder 中的文件名。
- [查找栏](find-bar.md)：查找文本、显示匹配数量，并跳转到上一个或下一个匹配项。
- [富文本编辑器](rich-text-editor.md)：支持格式、链接、标题、列表、任务和图像，提供文字处理器常用快捷键、“格式”菜单、富文本复制粘贴和 Markdown 支持。

## 代码

- [代码编辑器](code-editor.md)：如 Zed 的代码编辑器，带有高亮、多个光标、折叠、补全和诊断，用于任意大小的文件。
- [差异查看器](diff-viewer.md)：以分栏或合并视图显示两段文本的差异，可编辑新文本并审阅更改。

## 文稿

- [Markdown](markdown.md)：CommonMark 和 GitHub 的 Markdown，可作为已完成的文稿，也可作为语言模型正在流式传入的回复；可选择，并能作为文本或 Markdown 复制。

## 日期与颜色

- [日历](calendar.md)：用于选择某一天的月份视图。
- [日期输入](date-input.md)：字段中的日期，在其下方的日历中选择。
- [时间输入](time-input.md)：通过键入或步进调整小时和分钟。
- [颜色选择器](color-picker.md)：显示当前颜色的色块，单击可打开颜色选择器。

## 集合

- [列表](list.md)：任意数量、任意高度的行，只在进入视口时构建。
- [表格](table.md)：按列展示列表数据，支持排序、调整列宽和移动列。
- [树](tree.md)：可展开和折叠的条目，子条目在父条目内部构建。
- [大纲](outline.md)：以列表或表格呈现任意规模的树。
- [网格视图](grid-view.md)：任意大小的网格中的条目，如图片库。

## 容器

- [滚动视图](scroll.md)：滚动其子元素的容器。
- [网格](grid.md)：按行和列排列的子元素，布局方式如 CSS grid。
- [分栏视图](split.md)：两个窗格，通过可拖动的分隔条调整大小。
- [可折叠](collapsible.md)：由标签控制显示和隐藏的内容。
- [手风琴](accordion.md)：上下排列、可打开和关闭的分区。
- [表单](form.md)：带标签、描述和错误的控件。
- [Web 视图](web-view.md)：窗口中的网页，位于其后绘制的原生 UI 之下，如菜单和对话框。

## 导航

- [标签页](tabs.md)：选择页面的一行标签页。
- [侧边栏](sidebar.md)：分组显示条目，类似 Finder 的侧边栏。
- [面包屑导航](breadcrumbs.md)：显示当前路径，点击任一层级即可返回。
- [后退和前进按钮](history-buttons.md)：在路由器历史中前进和后退的按钮。

## 对话框与消息

- [对话框](dialog.md)：显示对话框，并使后方窗口内容变暗。
- [警告对话框](alert-dialog.md)：用于确认重要操作，提供可选操作按钮。
- [浮层](popover.md)：元素下方的面板。
- [工具提示](tooltip.md)：指针停在元素上，或键盘焦点移到元素上时显示的提示。
- [Toast](toast.md)：窗口底部附近的短消息，可带动作，也可不带。

## 状态

- [进度条](progress.md)：显示已完成的进度，也支持进度不确定的任务。
- [旋转指示器](spinner.md)：用旋转动画表示进度不确定的任务正在进行。
- [量度计](meter.md)：范围内的一个值，按它的高低着色。
- [徽标](badge.md)：以胶囊形背景显示计数等短文本。

## 图表

- [图表](chart.md)：无预设样式的比例尺、坐标轴和图形标记，用于绘制柱状图、折线图、面积图、堆叠图和环形图，支持指针和键盘选择。

## 图像

- [图像](image.md)：以其自身颜色显示的位图和 SVG。
- [视频](video.md)：带声音的视频文件和流，由系统的媒体引擎播放，以及动画 GIF 和 PNG，可作为预告片、内容背后的片段或预览。
- [图标](icon.md)：以文本颜色显示的 SVG 图标。
- [头像](avatar.md)：显示头像图片或姓名首字母。
