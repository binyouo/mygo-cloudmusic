# 检查器

检查器在内容旁的停靠面板中显示原生 UI 窗口的元素，类似 Chrome 开发者工具：用标记形式呈现元素树，显示所选元素的样式、盒模型和属性，并在内容中高亮它，还可查看帧耗时和问题。

开启开发者工具的窗口可以通过“视图 → 切换开发者工具”、`F12`，或在 macOS 上用 `Alt+Cmd+I`、在其他平台上用 `Ctrl+Shift+I` 来打开它；`Shift+Cmd+C`（`Ctrl+Shift+C`）会以挑选元素的方式打开它。开发构建中开发者工具默认开启，如同网页检查器；`WindowOptions.Page.DevTools` 可为单个窗口开启或关闭它们：

```go
mygo.NewWindow(mygo.WindowOptions{
	Content: ui.View(app.view),
	Page:    mygo.PageOptions{DevTools: mygo.DevToolsDisabled},
})
```

面板占用窗口右侧的空间，内容区域相应缩窄，`c.Size()` 返回的也是缩窄后的尺寸。

`mygo build` 的生产构建会去掉检查器，因为这些构建中开发者工具是关闭的：检查器约占二进制文件 400 KB。`mygo build -debug` 会保留它，在生产构建中运行 `MYGO_INSPECTOR=1 mygo build` 也一样，这样带有 `DevToolsEnabled` 的窗口就能在那里打开它。用 `go build` 构建的应用保留它，除非 `-tags mygo_noinspector` 将其排除。拖动面板的左边缘可以调整它的大小。面板会跟随浅色或深色外观。

工具栏的 RTL 切换按钮会把窗口排成[从右到左](right-to-left.md)，或切回，便于在界面翻译前试用；面板本身保持从左到右。元素的计算样式会显示它的 `direction`。

## 元素

树把每个元素显示为一个标签：是控件就显示其控件名，例如 `<Button>` 或 `<List>`，否则显示 `<Row>`、`<Column>`、`<Text>` 之类，其 `key` 和 `label` 作为属性，并显示其中文本元素的内容。靠近根的元素显示其中的内容；元素的箭头打开和关闭它，按住 Alt 单击会打开或关闭其中的所有元素。

- 将指针悬停在某行上，会在内容中高亮它的元素，如同 Chrome：内容为蓝色、内边距为绿色、边框为黄色、外边距为橙色，并用工具提示说明它的尺寸、颜色、字体和间距，以及辅助技术看到的内容：名称和角色，以及键盘能否聚焦它。带有[过渡](transitions.md)的元素所在的行会说明这一点。
- 单击一行可选中对应元素；方向键可切换选择，Left 和 Right 可折叠或展开所选元素，或移到父元素、进入子元素。树下方的面包屑显示所选元素的路径，单击也可选择路径中的元素。
- 先点工具栏的选择器，再单击内容，即可选中指针下的元素：这次单击会交给检查器，而不会交给元素。按 Escape 停止选择。
- `Cmd+F`（`Ctrl+F`）按标签、文本、label 或 key 查找元素。
- 右键单击一行，即可复制检查器显示的该元素内容，便于粘贴到 issue、聊天或编辑器中：**Copy element** 把它及其中的所有元素复制为标记，所有元素都展开，文本完整保留；**Copy styles**、**Copy computed** 和 **Copy properties** 把树下方的标签页复制为文本，样式和计算值以 CSS 规则呈现。在树中按 `Cmd+C`（`Ctrl+C`）复制所选元素，与 Copy element 相同：

  ```html
  <Row key="header">
    <Button>
      <Text>Toggle sidebar</Text>
    </Button>
    <Text>Inbox</Text>
  </Row>
  ```

树下方，所选元素有三个标签页：

- **Styles**：它设置的样式，以一条 CSS 规则呈现，并注明应用构建它的位置（创建它或给它 `Key` 的调用所在文件和行），然后是它继承的文本样式，按设置这些样式的元素列出。MyGo 的布局是 CSS 的，因此属性也是 CSS 的：`flex-direction`、`gap`、`padding`、`flex-grow`、`width`、`color` 等。[更改它们](#更改样式)，在窗口中试用一个值。
- **Computed**：盒模型：外边距、边框、内边距和内容及其尺寸，以及元素布局后的值：它在窗口中的尺寸和位置、字体和滚动偏移。
- **Properties**：它是什么（类型、ID、key、文本）、它的状态（聚焦、指针悬停、按下、禁用、移动中），以及辅助技术看到的内容。

## 更改样式

Styles 标签页支持直接编辑样式，类似 Chrome 的样式面板，无需重新构建应用就能试用新值：

- 单击一个值即可输入另一个：输入时窗口会实时更新，按 Enter 或单击别处保留新值，按 Escape 则还原。`Up` 和 `Down` 把它的第一个数字加减 1，按住 `Shift` 时加减 10，按住 `Alt` 时加减 0.1。清除一个值会关闭该项。
- 声明前的复选框（指针悬停时显示）可关闭或开启该声明。元素继承的值要在设置它的元素上修改。
- 规则旁的 `+` 用于添加声明，按 `opacity: 0.5; gap: 8px` 这样的形式输入。MyGo 无法理解的内容会显示删除线并给出警告。

长度以 DIP 为单位，可带或不带 `px`，在元素接受百分比时也可用百分比；颜色可以是十六进制、`rgb()`、`oklch()` 或少数几个名称。标签页无法更改的声明（例如 `overflow` 或 `transition`）没有复选框。

这些更改会覆盖视图设置的样式，即使视图状态变化、重新构建，也会保留，直到单击 **Reset** 或关闭检查器。更改既不修改代码，也不修改视图状态。**Copy as Go** 会将更改转为 Go 调用并复制到剪贴板，便于粘贴进视图；结果按元素分组，附上选中元素时的构建位置，以及需要移除的内容：

```go
// Column#box, main.go:42
// Remove border-radius: 10px
.Padding(26).Background(ui.Hex("#fde68a"))
```

## 强制状态

Styles 标签页顶部的 `:hov` 可以强制所选元素进入某些状态，便于在不把指针放在上面的情况下为悬停或按下状态设置样式：`:hover`、`:active`、`:focus`、`:focus-visible` 和 `:focus-within`。此后元素的 `Hovered`、`Pressed`、`Focused`、`FocusVisible` 和 `FocusWithin` 会报告这些状态，树也会在元素旁标出它们。它们只会作用于视图所询问的元素：如果一行询问自己是否被悬停，以便给其中的文本着色，就要在行上强制该状态，而不是在文本上。真实的焦点和指针位置不变，检查器关闭时强制状态消失。

## 显示布局

容器在树中有一个徽章。单击它可以让它们的布局持续显示在内容之上，直到再次单击它或检查器关闭：

- **`grid`** 显示轨道线，编号方式与 `ColumnStart` 和 `RowStart` 的计数一致，间隙用斜线填充。
- **`flex`** 显示子元素留下的空间，用斜线填充：间隙、空闲空间以及换行之间的空间。每个盒子都按 flexbox 布局，因此只有两个或更多子元素的盒子才有该徽章。

显示布局时会禁用布局缓存，因此 Performance 标签页中的布局耗时不能代表应用正常运行时的开销。

## 打开源代码

Styles 标签页中元素旁的链接（例如 `main.go:42`）会在你的编辑器中打开对应行。用 `MYGO_EDITOR` 设置编辑器；没有设置时，MyGo 会依次使用 `VISUAL` 和 `EDITOR`：

```sh
MYGO_EDITOR=code mygo dev
```

VS Code 及其分支（`code`、`cursor`、`windsurf`）、Zed、Sublime Text、JetBrains 的 IDE（`goland`）、TextMate、GVim 和 Emacs 会在对应行打开。其他编辑器只会收到文件，除非命令用 `{file}` 和 `{line}` 说明位置：

```sh
MYGO_EDITOR='tmux new-window nvim +{line} {file}'
```

应用没有可用于运行编辑器的终端，因此当 `VISUAL` 和 `EDITOR` 指向终端编辑器（例如 `vim` 或 `nano`）时会被跳过。像上面那样的命令会打开一个终端编辑器。路径含空格时要用引号。没有设置编辑器时，文件会在系统的默认应用中从顶部打开。出错时的信息列在 Issues 下。

应用必须能读取这些环境变量：从运行 `mygo dev` 的 shell 启动时会继承它们，从 Dock 或文件管理器启动时则不会。使用 `-trimpath` 构建时（例如 `mygo build --debug`），产物中没有可打开的源码路径，因此源码位置不会显示为链接。

## 性能与问题

**Performance** 用条形图显示最近各帧的构建、布局和绘制耗时，与窗口所在显示器的刷新间隔（120 Hz 时为 8.3 毫秒，60 Hz 时为 16.7 毫秒）对照，并列出最后一帧、平均和最慢耗时。设置 `MYGO_FRAME_STATS` 还可以记录慢帧（见[渲染](rendering.md)）。

**Issues** 列出构建时发现的问题，工具栏会统计它们：例如，一个父元素下两个元素得到相同的 `Key`，它们共享一个状态，因此本应给其中一个的单击、焦点或滚动会去到另一个。应用会把每个问题记录一次；[Tester](testing.md) 在给出第二个 key 的位置 panic，以便测试在那里失败。
