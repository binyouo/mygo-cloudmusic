# 迁移到带校验的元素

原生 UI 视图接收 `*ui.Context`，子元素构建器使用 `func()` 回调，控件构造函数返回 `ui.Element` 值。本指南介绍源码迁移、构建生命周期、持久控件标识和焦点绑定。

## 应用源码迁移

从应用目录运行迁移：

```sh
go tool mygo migrate-ui .          # preview affected files
go tool mygo migrate-ui -write .   # write the source migration
go tool mygo vet .
go test ./...
```

提交前请检查差异。源码迁移工具会解析 Go 语法和实际使用的 UI 导入别名，并在同一包的文件之间共享字段声明和辅助函数的返回类型信息。它会转换元素和自定义部件的指针类型、针对元素的 nil 检查和零值赋值，把链式构造中的键移到 `Context.Key`，并把网格的 `Rows(n)` 重命名为 `GridRows(n)`。上下文指针、子元素回调签名、普通模型指针和无关的 nil 检查不变。点导入和单独保存的元素上的键需要人工检查。工具不会把任意轮询逻辑自动改为回调。如下所述，轮询 `Changed` 和 `Submitted` 可以在同一次构建中提交局部绑定值。

## 元素值和键

```go
var element ui.Element
if element.Valid() {
    element.Focus()
}
```

赋值或返回空元素时，使用 `ui.Element{}`，不要用 `nil`。自定义部件（如 `ui.SelectParts[T]`）同样是值类型，包含带生命周期校验的元素。

元素会在下一次构建开始前失效，同一帧内的重建也不例外。即使其内存池槽位已被复用，代际校验仍能识别出过期元素。`Tester` 和开发构建会 panic，消息会列出相关构建轮次并建议使用 `ui.Handle`；带 `mygo_noinspector` 的生产构建则返回空查询结果，或忽略对过期元素的修改。表示“无元素”的零值在两种模式下都可安全使用。`Valid` 不会 panic。

在状态初始化之前赋键：

```go
ui.TextInput(c.Key("search"), &a.query).ReadOnly(a.readOnly)
ui.Checkbox(c.Key(todo.ID), &todo.Done, todo.Title)
parts := ui.SelectBase(c.Key("choice"), &a.choice)
```

`Element.Key` 在构建容器的子元素或本地状态之前为容器赋键。对有状态的控件使用 `Context.Key`，包括自定义基础控件。键指定下一个外层控件，并在该控件初始化状态之前被消耗。

## 输入和操作

控件在构造函数返回之前，就通过它绑定的指针写入用户的输入，所以绑定的值，包括本地副本，可以立即读取；`Changed` 和 `Submitted` 报告它。输入遵循用户操作那一帧的选项：在构建中设置的 `Disabled`、`ReadOnly` 和滑块设置支配其后各帧的输入。

```go
ui.Button(c, "Save").Disabled(a.saving).OnClick(a.save)
ui.TextInput(c.Key("query"), &a.query).OnChange(a.search)
ui.TextInput(c.Key("draft"), &a.draft).OnSubmit(a.send)
c.OnShortcut(ui.Cmd, ui.KeyS, a.save)
```

单击、快捷键、更改和提交操作在控件配置完成、输入写入绑定值后运行，早于重建。已处理的输入会在下一次构建前被消费，因此操作不会重复。轮询方法仍然可用；操作会改变正在构建的集合时，应优先使用回调。I/O 应在后台执行，再用 `Window.Update` 将结果更新到模型。

尽可能把控件直接绑定到持久的模型字段。每次构建都会重新计算的本地值，可以立即通过 `Changed` 或 `Submitted` 提交，或在构造后用回调提交：

```go
viewed := a.isViewed(file)
if ui.Checkbox(c, &viewed, "Viewed").Changed() {
    a.setViewed(file, viewed)
}
```

同样的规则也适用于选中值由多个模型字段推导出的分段控件。运行源码迁移工具后，请人工检查这些绑定；工具无法推断如何把派生值写回模型。

## 持久身份

存储 `ui.Handle`，而不是元素：

```go
type app struct {
    query string
    search ui.Handle
}

func (a *app) view(c *ui.Context) {
    ui.TextInput(c.Key("search"), &a.query).Bind(&a.search)
}

// On the UI thread, including inside Window.Update:
a.search.Focus()
```

焦点请求会合并，并在控件隐藏期间等待。关闭窗口会取消该窗口的请求，也可以用 `CancelFocus` 提前取消。

`handle.Focused(c)` 和 `handle.FocusWithin(c)` 根据持久标识查询焦点，控件尚未构建时也可调用。`c.Resolve(handle)` 只返回当前这轮构建中的元素。`handle.Bounds(c)` 在 UI 线程上读取已提交的控件边界框，也可在输入回调中调用。这些查询的含义不同：移除控件的那轮构建中，仍能查询到它之前的焦点；构建提交后，才会清除实际焦点。

一个句柄支持多个窗口中的独立绑定。查询接受窗口的 Context。用 `handle.Focus(c)` 和 `CancelFocus(c)` 选择窗口；不接受参数的 Focus 形式要求最多有一个打开的绑定。在第一次绑定之前，它会等待第一个控件。每个窗口应有自己的控件状态和焦点字段。`Ref`/`RequestFocus` 是 `Handle`/`Focus` 的别名。

`ListState`、`ScrollState`、`GridState` 和 `Router` 都带有一个 Handle；其他控件可以绑定任何应用拥有的句柄。用 `state.Handle` 实现持久焦点和命令。

## 绑定到应用数据的焦点

```go
type pane int
const (none pane = iota; files; diff)

ui.List(c, &a.list, len(a.files)).FocusBind(&a.pane, files).
    Rows(func(row ui.ListRow) { ui.Text(row.Context, a.files[row.Index].Name) })

// An action requests the diff pane, waiting while hidden:
a.pane = diff

// Actual focus can differ while that request waits:
focused := ui.FocusedValue(c, &a.pane)
```

`FocusBind` 需要一个指向可比较字段的指针和一个与之匹配的值。值在该窗口的该字段内必须唯一；把它的零值保留给无焦点。字段保存的是期望的焦点。没有请求等待时，用户改变焦点会更新它。将其设为零会清除焦点。`FocusedValue` 使用已提交的身份读取实际焦点，与构造顺序无关。隐藏的绑定保留它期望的请求，而实际焦点可以在另一个控件上，也可以不存在。为不同的窗口使用不同的字段。

## 列表和键盘命令

```go
a.filesView.OnShortcut(c, ui.Cmd, ui.KeyK, a.openSelected)
ui.List(c.Key("files"), &a.list, len(a.files)).Bind(&a.filesView).Grow(1).
    ItemKey(func(i int) any { return a.files[i].ID }).
    Selection(&a.selection).
    Rows(func(row ui.ListRow) {
        text := ui.Text(row.Context, a.files[row.Index].Name)
        if row.Selected() && row.ListFocused() {
            text.TextColor(c.Theme().Accent)
        }
    })
```

句柄的快捷键可以在 Bind 之前声明。只有当那个窗口为该句柄构建了已启用的控件时，它才会在构造后运行，因此隐藏的控件不会接受命令。`ListRow` 提供共享的 Context、索引、选择、列表焦点，以及行的悬停和按下状态（`Hovered`、`Pressed`、`PointerPosition`），行内容可以绘制它们而不接收单击。`List(c, state, n, func(i int))` 也直接接受行回调。在调用 Rows 之前配置链式列表。

## 持久服务和验证

保存 `c.Services()` 的返回值，供剪贴板、URL 回调和后台重绘使用。Services 仅持有窗口的弱引用，不包含构建数据。剪贴板和 URL 方法在 UI 线程上运行；Invalidate 可从其他 goroutine 安全调用。更新模型时使用 `Window.Update`。

`mygo vet` 运行普通的 Go vet 以及类型感知检查，检查元素、部件，以及存储在结构体字段、包变量中，或被 goroutine 捕获、传给 goroutine 的 Context 指针。它跟踪导入、类型别名和推断的变量；允许 Handle、Services 和视图回调签名。这些检查有助于约束构建生命周期；它们不能证明所有生命周期或 goroutine 安全。运行时代际检查仍然必要。

请实际测试控件消失后重现、待处理的焦点、列表重排、文本编辑与输入法、禁用和只读设置，以及命令路由。运行常规测试并检查应用。测量出的开销和复现命令见[性能](performance.md)。
