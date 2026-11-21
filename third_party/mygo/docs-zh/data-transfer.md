# 剪贴板与拖放数据

`transfer` 包描述系统剪贴板与原生[拖放](ui/drag-and-drop.md)共用的序列化内容。一个 `Data` 值包含有序的数据项。每一项为**同一**内容提供多种备选格式；不同的项表示不同的值。自定义格式使用 MIME 名称和明确指定的编码。原生数据不包含 Go 指针或仅在当前进程内有效的 Go 值。

```go
const noteFormat transfer.Format = "application/vnd.example.note+json"
frozen := *note
data := transfer.New(transfer.NewItem(
    transfer.Bytes(transfer.Text, []byte(frozen.Title)),
    transfer.Bytes(transfer.HTML, []byte("<b>"+html.EscapeString(frozen.Title)+"</b>")),
    transfer.Lazy(noteFormat, func() ([]byte, error) {
        return json.Marshal(frozen)
    }),
))

err := mygo.Clipboard.Write(data, mygo.ClipboardOptions{
    OnRelease: func() { /* release resources captured by these providers */ },
})
row.DragData(data) // the same model, with an independent provider cache
```

`ClipboardOptions.Concealed` 把一次写入标记为机密（例如密码），供读取平台标记的剪贴板管理器使用；它们如何处理这个标记，比如把它排除在历史记录之外或按密码显示，由它们自己决定。在 macOS 上，每个项都会带上 nspasteboard.org 的 `org.nspasteboard.ConcealedType`，并且只写入当前主机，使内容不会进入通用剪贴板；Windows 上会设置 `ExcludeClipboardContentFromMonitorProcessing`，让内容不进入剪贴板历史，也不会出现在用户的其他设备上；Linux 提供 KDE 的 `x-kde-passwordManagerHint` 目标，值为 `secret`，Klipper、CopyQ 和 GPaste 会读取它。浏览器没有这样的标记。`Flush` 会保留这些标记。内容会一直留在剪贴板上，直到应用替换或清除它：过一段时间后清除密码，是应用自己的策略。

`Bytes` 会复制传入的内容。`Data.Items` 和格式列表返回独立的切片，读取操作返回独立的字节数据。空字节数据也是有效值。`Text`、`HTML`、`PNG`、`URIList` 和 `FileList` 是常见表示形式的名称。`TextData`、`URLData` 和 `FileData` 用于构造常见的待传输数据。文件以绝对路径表示，并编码为经过转义的文件 URL；`Data.Files` 只接受本地路径，忽略指向远程主机的 URL 和非文件 URL。不会读取文件，也不会删除文件。每个路径使用一个项（`FileData` 会自动这样做）。

## 剪贴板格式发现与读取

`Clipboard.Formats` 返回可移植的名称，而不请求表示形式。`transfer.PreferredFormat` 和 `Offer.Preferred` 按接收方的偏好顺序协商。较旧的 `AvailableFormats` 保留各平台的惯例：macOS 上是原生 UTI，Linux 上是 GTK 目标，Windows 上是规范名称（现在也包括自定义格式和文件列表）。

只请求接收方可以使用的格式：

```go
formats := mygo.Clipboard.Formats()
if format, ok := transfer.PreferredFormat(formats, noteFormat, transfer.HTML, transfer.Text); ok {
    bytes, err := mygo.Clipboard.ReadFormat(format)
    // Decode the explicitly agreed encoding; handle err.
}

// Read accepted alternatives together; other providers are not requested.
data, err := mygo.Clipboard.Read(noteFormat, transfer.HTML, transfer.Text)
if err == nil {
    if format, ok := data.Preferred(noteFormat, transfer.HTML, transfer.Text); ok {
        bytes, err := data.Read(format)
        // Use the receiver's preferred representation.
    }
}
```

不带参数的 `Read()` 复制所有公布的表示形式。空剪贴板返回空数据。没有匹配格式的显式请求返回 `transfer.ErrFormat`。`ReadFormat` 像 `Data.Read` 那样合并项：文本用换行连接，URI/文件列表用 CRLF 连接，其他格式使用第一个匹配的项。使用 `Data.Items()` 读取单独的项。

读取返回的是已经生成好的数据，不再依赖原生提供方。在所有者改变、窗口关闭或源应用退出之后，它仍可在任何 goroutine 中使用，也可以在之后的拖放中提供。原生操作在主线程上运行。观察到所有权改变的读取返回 `mygo.ErrClipboardChanged`；Windows 在读取其他应用的内容期间持有剪贴板锁。从剪贴板提供方访问剪贴板返回 `mygo.ErrClipboardReentrant`，防止递归读取和替换。

现有的 `ReadText`、`WriteText`、`ReadHTML`、`WriteHTML`、`ReadImage`、`WriteImage` 和 `Clear` 保持原有签名。`WriteHTML` 在所有平台上都提供 HTML 和文本回退，修复了 Linux 以前只能处理文本的行为。`WriteImage` 在替换现有数据之前校验 PNG；原生图像读取器仍然接受 TIFF、GTK 图像目标和 Windows DIB。`WriteFiles` 和 `ReadFiles` 交换原生文件列表。`Write`/`WriteFiles` 传入空内容时会清除剪贴板。写入失败时不会安装它的释放钩子。

## 观察变化

`Clipboard.OnChanged` 观察本应用或其他应用造成的变化，并像其他 `On…` 监听器一样返回停止观察的函数。它不读取剪贴板内容，也不调用内容的提供方。在回调中只读取你的应用需要的格式：

```go
off := mygo.Clipboard.OnChanged(func() {
    // Called on the main thread. The contents can change again before Read.
    data, err := mygo.Clipboard.Read(transfer.Text)
    if err == nil {
        text, _ := data.Read(transfer.Text)
        // Use text; avoid retaining sensitive clipboard contents by default.
        _ = text
    }
})
win.OnClosed(off) // when this window no longer needs the observation
```

注册时不会报告初始内容。多次通知可能被合并，所以这是让你读取当前剪贴板的提示，而不是完整的历史，也不是某一次复制的快照。读取或延迟渲染本身也可能产生原生通知；不要在每次回调中无条件地写入剪贴板。回调可以读取或写入剪贴板，并会推迟到原生剪贴板操作和之前的回调返回之后。像其他主线程事件处理器一样，让回调保持简短。

监听器共用一个原生观察，它在应用就绪后开始：可以在 `App.Run` 之前添加监听器。`off` 可以在任何 goroutine 中调用，可以重复调用，也可以在回调中调用；在一次变化的投递过程中被移除的监听器仍可能收到这一次。移除最后一个监听器会停止观察，退出应用也会。观察不会触碰剪贴板持有的任何内容，所以剪贴板提供方可以添加和移除监听器。

Windows 使用 `AddClipboardFormatListener`；Linux 使用 GTK 的 `owner-change` 信号，取决于桌面是否公开变化（尤其是在 Wayland 上）。macOS 没有公开的粘贴板变化通知，所以在订阅期间它每 250 ms 只检查一次 `changeCount`，并带有 25 ms 的容差，让系统可以把这个定时器和其他唤醒一起触发。两次检查之间被覆盖的变化无法恢复。浏览器不提供对系统剪贴板的观察：在那里回调永远不会被调用。

## 惰性提供方与所有权

每次写入和每次拖放都会获取一个新的 `Data.Snapshot`。惰性提供方被请求时，对那个快照最多运行一次；成功、错误和已恢复的 panic（`transfer.ErrProviderPanic`）都会被缓存。发现和偏好协商不会请求字节。原生接收方（包括剪贴板管理器）可能在发布后立刻请求字节。`Read` 只请求它选中的格式。提供方出错会导致该次读取被拒绝，其他表示形式仍然可用。

提供方在主线程上执行，必须迅速返回。耗时的内容应提前在 goroutine 中准备好，多种表示形式共享的值在此后不得再修改。绝不要等待另一个主线程操作、递归读取同一个提供方，或从剪贴板提供方访问剪贴板。单独调用 `Data.Read` 时，提供方在调用它的 goroutine 中执行。

剪贴板所有权属于**应用**，与源窗口无关。关闭窗口后，从该窗口复制的内容和提供方仍然可用。提供方使用的资源必须保留到 `ClipboardOptions.OnRelease` 运行；原生回调及所有外层剪贴板操作返回后（包括运行嵌套事件循环的存储操作），该回调会在主线程上运行一次。替换内容、`Clear`、成功的 `Flush` 和应用退出都会释放提供方。macOS 也可能在所有延迟提供的数据都已交付后释放它们。应用运行期间，该钩子可以写入新的剪贴板内容；应用退出后调用剪贴板接口，会返回事件循环已停止的错误，或返回便捷方法的空结果。该钩子不能假定窗口仍然存在。拖放源则使用其会话的 `Done` 回调。

`Clipboard.Flush` 先生成应用拥有的每种表示形式的完整数据，再交给原生存储。提供方失败时会保留所有权并返回错误；如果必须持久保存内容，请在退出前处理这些错误。应用退出时会尽力自动调用 `Flush`，然后释放剩余的提供方资源。剪贴板属于另一个应用时，`Flush` 不会改动其内容。在 Linux 上，退出后保留内容需要剪贴板管理器。没有管理器时，`Flush` 会返回 `mygo.ErrClipboardPersistence`，释放惰性资源，并在应用运行期间继续提供已生成的字节数据。

## 原生互操作

| 平台 | 剪贴板实现 | 项边界与格式 |
|---|---|---|
| macOS | NSPasteboard 项和应用拥有的数据提供方；执行 `Flush` 时提供已生成的项 | 项边界在原生读取和持久化后仍然保留。文本/HTML/PNG/文件 URL 使用系统 UTI；自定义 MIME 名称共享拖放的 UTType 映射。完整的 URI/文件列表有序列化类型，同时有单 URL 回退。UTF-16 文本和 TIFF 图像在读取时转换。 |
| Linux | GtkClipboard 选择目标、共享的拖放格式映射、get/clear 回调、所有者变更跟踪；GTK 剪贴板管理器存储 | 在拥有它的 MyGo 进程内，所有项都保留。原生选择会合并文本和 URI/文件列表；自定义格式使用第一个匹配的项。HTML 以 `text/html` 提供。持久化取决于桌面的剪贴板管理器。 |
| Windows | 与拖放所用相同的延迟渲染 IDataObject，配合 OleSetClipboard/OleFlushClipboard；加锁的 Win32 读取 | 在拥有它的进程内，执行 `Flush` 之前所有项都保留。原生读取会合并文本/URI/文件列表，并使用第一个自定义项。Unicode 文本、CF_HTML、PNG/DIB、CF_HDROP、URL 和注册的 MIME 格式可与系统应用互操作。可选的私有长度提示在 HGLOBAL 持久化后保留 MyGo 的空/二进制载荷；实际的原生格式仍是未包装的字节。 |

来自其他应用的剪贴板表示形式每个限制为 64 MiB。没有字节长度约定的其他 Windows HGLOBAL 自定义数据可能包含分配器填充；与这类源互操作时，请使用自带长度的编码。虚拟文件和延迟提供文件内容不在当前模型的范围内。`Drag(value)` 使用的 Go 值是进程本地的；它们的临时拖放注册表令牌会被剪贴板写入拒绝，也会从剪贴板发现中省略。

[剪贴板示例](../examples/clipboard)把一条笔记作为文本、HTML 和惰性 JSON 复制并拖放，显示提供方调用/释放的计数，粘贴文件列表，并执行 `Flush` 写出剪贴板数据。
