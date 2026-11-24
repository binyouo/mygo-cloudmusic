# 文本输入原语

<!-- preview text-input-client 200 -->

`TextInputBase` 和 `TextAreaBase` 提供无外观的普通可编辑字符串控件。对于自带存储、选区、渲染或编辑策略的控件，可以把 `TextInputClient` 附加到元素上：

```go
ui.Box(c).
    HandleTextInput(app.input).
    HandleInput(app.input.HandleInput).
    Draw(app.input.Draw).
    Label("Document").
    Role(ui.RoleTextField)
```

这个原语将控件状态与平台的输入法和文本查询对接。客户端自行管理缓冲区、选区模型、标记文本和撤销历史。底层可以使用 rope、持久化树或增量段落存储；MyGo 不会在每次编辑时复制整个文稿，也不决定文本格式行为。已有的 `HandleInput`、`TextCaret`、`TextInput` 和 `TextArea` API 仍然可用。`TextCaret` 适合终端等需要提供输入法候选窗口位置、但不提供文本文稿的控件。`InputMethod(false)` 可关闭输入法，同时保留按键输入文本的能力，文本不经过输入法组字就直接传给 `ReplaceText`，适用于模态编辑器的普通模式；浏览器无法关闭输入法。

内置的 `TextInput` 和 `TextArea` 也实现这个客户端契约。它们的 `*string` 绑定在控件构建时发布已提交的文本；原生查询在那帧之前就能看到排队的导航和编辑。预编辑在提交前只是对原生文稿的虚拟插入，它的替换和提交共享同一个撤销事务。密码字段不会向原生查询提供已提交的文本；只读字段则禁用输入。段落索引包含 UTF-16 偏移，因此大型文稿末尾附近的原生查询无需扫描前面的全部内容。

每次提交编辑时，字符串绑定仍然会生成整个字符串。如需按索引存储，可以使用 `TextBuffer` 配合 `TextInputBuffer` 或 `TextAreaBuffer`：

```go
// Keep the buffer across frames; its zero value is also usable.
document := ui.NewTextBuffer(initialText)
// In the view:
input := ui.TextAreaBuffer(c, document).Fill().Font("monospace")
if input.Changed() {
    // Length/version queries read tree summaries without flattening text.
    version := document.Version()
    _ = version
}
```

`TextInputBufferBase` 和 `TextAreaBufferBase` 提供无样式的变体。它们共享字符串输入框的选区、样式、剪贴板、IME 和撤销行为。原生修改会立即更新缓冲区，控件构建时 `Changed` 会报告这些修改。以编程方式编辑缓冲区会刷新控件，并清除过期的选区/撤销状态；这类修改要用 `Window.Update` 安排，并让视图失效。

`TextBuffer` 是由大小受限、独占其存储的 UTF-8 块组成的平衡树。它为字节、rune、UTF-16 单元和换行数建立索引。局部编辑只复制受影响的块和树路径，不会复制整个文稿，也不会调整后续每个段落的绝对偏移。`Snapshot` 共享未改动的块，编辑后依然有效；`Restore` 可在常数时间内恢复快照。缓冲区方法可从任何 goroutine 安全调用，编辑时也可以并发读取快照。

`Replace(start, end, text)` 和 `Slice(start, end)` 按 rune 计数，与 `Element.TextSelection` 一致。`ReplaceUTF16`、`TextForRange`、`UTF16Offset` 和 `RuneOffset` 在原生范围与偏移之间转换，并保持代理对完整。快照还提供 `Line`、`LineRange` 和 `LineAt`，用于自定义编辑器的布局。

`String` 会显式地把文稿完整展开。导出文件时，`WriteTo` 会把块流式写入 `io.Writer`。普通的多行编辑和有界的 IME 查询不会展开整个文稿；只有显式的全文稿查询，或当前的辅助功能 Value 协议，才可能请求完整文本。布局仍然一次只排版一个逻辑段落。行数变化时，会更新控件的段落/高度元数据。

[text-buffer 示例](../../examples/text-buffer)展示一个 100,000 行的可编辑文稿。
自定义编辑器可以通过自己的 `TextInputClient` 使用同一个缓冲区，并自行选择选区、历史和渲染策略。

[text-input 示例](../../examples/text-input)展示一个小型、由应用自己管理的文本框。它演示组合、指针选择、“编辑”菜单命令和自定义渲染。它只是原语的示例，不是编辑器的实现。

## 客户端契约

传入一个在帧之间保留的客户端指针。`HandleTextInput(nil)`（包括有类型的 nil 指针）会断开它。元素可聚焦，并有文本光标；它的大小、外观和其他输入行为都由你决定。

| 方法 | 用途 |
|---|---|
| `TextForRange(range)` | 返回请求的文本，以及裁限或调整后的实际范围。支持全文查询，不限于插入点周围的固定范围。 |
| `Selection()` | 报告主要的原生选区及其方向。额外的光标或不连续的选区保存在应用状态中。 |
| `MarkedRange()` | 报告当前文稿中的预编辑，没有时返回 false。存在预编辑时，系统的输入法接收它用于组合的键，`Tester` 同样接收不带 Ctrl 或 Command 的键：它们既不会到达 `HandleInput`，也不会触发快捷键。 |
| `ReplaceText(range, text)` | 在指定范围内提交文本；范围传入 nil 时，若有预编辑则为预编辑，否则为选区。按你的编辑策略清除预编辑。 |
| `SetMarkedText(range, text, selected)` | 替换预编辑，并保留其标记范围。`selected` 相对于新标记的文本。 |
| `UnmarkText()` | 清除标记状态，同时保留文稿文本。焦点丢失、客户端更换或元素被释放时也会调用。 |
| `BoundsForRange(range)` | 返回相对于元素的文本或插入点几何信息、实际对应的范围，以及几何信息是否可用。文本不可用或不在屏幕内时返回 false。因聚焦等操作而将元素滚入视口时，若插入点边界可用，会确保插入点可见，而不是整个元素盒子。 |
| `IndexForPoint(point)` | 对相对于元素的点做命中测试，返回文稿偏移，或返回 false。 |

范围以 **UTF-16 代码单元** 计数，与原生文本服务和 GPUI 的输入处理器契约一致。这是一个互操作坐标系，不是存储格式。`ui.UTF16Len` 和 `ui.UTF16ByteOffset` 帮助简单的 UTF-8 缓冲区在边界处转换；按索引存储的缓冲区可以自己提供更快的转换。代理对必须保持完整，编辑策略也应尊重 Unicode 字素边界。`TextForRange` 会返回调整后的范围，让平台知道实际收到的是哪段文本。

回调在 UI 线程上同步运行，且发生在下一帧之前。它们不能等待需要该线程完成的工作。客户端自己负责与其他 goroutine 同步；修改应用状态时，和别的原生 UI 视图一样用 `Window.Update` 安排。原生修改后，MyGo 会请求一帧，并在下一个输入事件之前读取最新的选区/光标。过期的原生引用无法修改未聚焦、已被替换或已释放的客户端。

在 macOS 上，客户端提供 `NSTextInputClient` 的选区、标记范围、子字符串查询、范围边界和点命中测试。GTK 使用有界的周围文本，保持 UTF-8/rune/UTF-16 转换，并转发对周围文本的删除以及预编辑/提交回调。Windows 的 IMM32 使用有界的文稿输入/重转换上下文，并转发组合与提交范围。更换客户端会重置原生预编辑，避免候选窗口跳到新获得焦点的字段。平台回调在启动时一次性分配。辅助技术也会编辑客户端的文本：设置元素的值会替换全部文本，插入文本会替换选区，和在 `TextInput` 中一样。

键盘导航、指针选择、剪贴板策略和“编辑”菜单命令都通过 `HandleInput` 提供。这些都由你的控件负责。这样编辑器就可以自行选择多选区、自定义键位映射、分组和历史，而不必覆盖框架自带编辑器的内置行为。

## 保留的文本几何信息

`ShapeText(text, font, width)` 和 `ShapeRichText(spans, font, width)` 使用 Core Text、Pango 或 DirectWrite 生成 `TextLayout`。`width` 为正时文本会换行；为零时只在换行符处断开。`Font.LineHeight` 把行高设为字号的倍数，和 `Element.LineHeight` 对 `Text` 的作用一样（CJK 文本用 1.8 左右比较合适）；为 0 时用字体自身的行高。按段落或行缓存布局，只替换受编辑影响的那部分。

```go
layout := ui.ShapeText(paragraph, ui.Font{Size: 16}, availableWidth)
caret := layout.Caret(offset)
offset = layout.IndexAt(ui.Point{X: pointerX, Y: pointerY})
rects := layout.SelectionRects(selectedRanges...)
// Inside Draw:
p.TextLayout(layout, originX, originY, color)
```

布局中的偏移同样以 UTF-16 计。`Caret` 和 `IndexAt` 会对齐到完整的字素；`PreviousBoundary` 和 `NextBoundary` 支持逻辑导航/删除。`SelectionRects` 接受多个范围，并保留双向文本选区中分开的视觉片段。它只提供几何信息，不会强加单范围选区模型，也不替编辑器选择导航策略。尺寸和几何查询可从任何 goroutine 安全调用；绘制则属于 UI 帧的一部分。

`ShapeRichTextWith` 按 `TextShape` 的设定布置各 span，就像富文本编辑器或书籍中的段落：

- `Align` 在 `Width` 内对齐各行：`Start`、`Center` 或 `End`。
- `Justify` 将按 `Width` 自动折行的各行拉伸到 `Width`，但每段的最后一行除外，具体通过加宽词间空格以及汉字、假名之后的间隙实现。各段末行和没有可扩展间隙的行（如折行的长单词）由 `Align` 对齐。行尾空白会保留，并延伸到 `Width` 之外。
- `Indent` 把文本第一行从段落起始的一侧向内缩进这么多 DIP，并相应收窄换行宽度，如同 CSS 的 `text-indent`；换行符之后的行不缩进。
- `NoBreakWords` 让比 `Width` 还宽的词溢出，而不是断开。

```go
layout := ui.ShapeRichTextWith(spans, ui.Font{Size: 17, LineHeight: 1.5},
	ui.TextShape{Width: column, Justify: true, Indent: 17 * 1.2})
```

这些都由布局自己完成，基于系统引擎断出的行，因此同一段落在每个平台上的换行、拉伸和缩进都一致。

任何文本中的软连字符（U+00AD）都是行可以断开的位置，许多 EPUB 里都有：行在那里断开时显示为连字符，其他地方则不显示，也不占空间，和浏览器中一样。以软连字符结尾的行会为连字符留出空间，必要时提前断开。MyGo 不会自行对单词断字。

`Lines` 从顶部开始返回布局中的各行：包括每一行的盒子（`Rect`，其 `X` 已计入对齐和缩进）、基线、上升部和下降部、该行包含的 UTF-16 偏移（`Start` 和 `End`，不含结尾的换行符），以及这一行是否从右向左阅读。需要分页的阅读器可以算出段落中有多少行能放在页面底部以上，以及下一页从哪个偏移开始：

```go
for _, line := range layout.Lines() {
	if top+line.Rect.Y+line.Rect.H > pageBottom {
		nextPageStarts = line.Start
		break
	}
}
```

span 设置 `ObjectWidth` 后，其中的对象替换字符（U+FFFC）就成了内联对象：盒子宽 `ObjectWidth`、高 `ObjectHeight`，立在基线上，基线以下再延伸 `ObjectDescent`；行会绕开它们换行，而它们本身不绘制任何内容。`TextLayout.Objects` 给出这些盒子的位置，编辑器在这里绘制图片，阅读器绘制内联图像，数学公式渲染器绘制公式。span 的 `BaselineShift` 会把文本或对象在基线上方抬高这么多 DIP，为负时降低，就像上标和下标；行会为它们留出空间，下划线也随之移动。

```go
ui.ShapeRichText([]ui.Span{
	{Text: "as Melville wrote"},
	{Text: "12", Size: 11, BaselineShift: 6},
	{Text: ", the whale"},
}, ui.Font{Size: 17}, column)
```

要进行视觉导航，需要保留 `TextCaretPosition`，包括其中的 `Affinity`。`TextDownstream` 选择后一个逻辑文本的边缘，`TextUpstream` 选择前一个边缘。在双向文本边界或换行处，两个边缘可能不同。`CaretAt` 绘制该边缘，`PositionAt` 在命中测试时保留它，`MoveCaret(position, direction)` 向左（负）或向右（正）移动一个视觉字素。`SelectionRanges(anchor, caret)` 把视觉手势映射为逻辑 UTF-16 范围；在混合方向的文本中，这些范围之间可能有空隙：

```go
caret = layout.MoveCaret(caret, 1)
ranges := layout.SelectionRanges(anchor, caret)
rects := layout.SelectionRects(ranges...)
// Use the same ranges for copy, deletion and replacement in your buffer.
```

内置控件也使用这些视觉边缘和范围集合。复制会按逻辑顺序拼接选中的片段；替换会保留空隙中的文本，撤销则同时恢复文本和视觉选区。
