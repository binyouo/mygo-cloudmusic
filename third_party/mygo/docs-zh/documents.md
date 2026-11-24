# 文稿

每个窗口对应一个文件的应用，例如文本编辑器、绘图应用或笔记应用，都是基于文稿的应用。`mygo.NewDocuments` 让它们在每个平台上获得与 Mac 上 AppKit 的 `NSDocument`、SwiftUI 的 `DocumentGroup` 所提供的相同功能：

- 每个文稿一个窗口，标题显示文稿名；在 macOS 上，标题栏中还有文件的图标，有未保存的更改时关闭按钮中有一个圆点；
- “文件”菜单中的新建、打开、打开最近使用、保存、另存为、创建副本、重新命名、移到和还原；
- 自动保存，只在无法自动保存时才询问是否保存；
- 未保存的更改都有恢复副本，崩溃或退出也不会丢失任何内容；
- 从 Finder、资源管理器、Dock 和跳转列表打开的文稿；
- 其他应用在文稿打开期间更改的文件。

下面用[原生 UI](ui/README.md) 编写一个笔记编辑器，代码就是完整的应用：

```go
func main() {
	mygo.NewDocuments(mygo.DocumentsOptions[string]{
		Types: []mygo.DocumentType{{Name: "Markdown Document", Extensions: []string{"md"}}},
		Read: func(r io.Reader) (string, error) {
			b, err := io.ReadAll(r)
			return string(b), err
		},
		Write: func(w io.Writer, text string) error {
			_, err := io.WriteString(w, text)
			return err
		},
		Equal: func(a, b string) bool { return a == b },
		Window: func(doc *mygo.Document[string]) *mygo.Window {
			text := doc.Content()
			var win *mygo.Window
			win = mygo.NewWindow(mygo.WindowOptions{Content: ui.View(func(c *ui.Context) {
				ui.TextArea(c, &text).Fill().OnChange(func() { doc.Set(text) })
			})})
			doc.OnReload(func() { text = doc.Content(); win.Update(func() {}) })
			return win
		},
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
```

`examples/documents` 在这个编辑器的基础上增加了状态栏，以及提示文件被其他应用更改的横条：

```sh
go run ./cmd/mygo dev examples/documents
```

## 文稿及其内容

`DocumentsOptions[T]` 描述一种文稿，其内容类型是 `T`：

| 字段 | |
|---|---|
| `Types` | 文件类型，由名称和扩展名指定，例如 `{Name: "Markdown Document", Extensions: []string{"md", "markdown"}}`。打开对话框显示它们；新文稿用第一个扩展名保存 |
| `Read` | 在 goroutine 中从文件读取内容 |
| `Write` | 在 goroutine 中把内容写入文件 |
| `Window` | 在主线程上创建文稿的窗口 |
| `New` | 新文稿的内容；未提供时使用 `T` 的零值 |
| `Equal` | 比较内容，使文稿撤销到与文件内容一致时恢复为未编辑状态 |
| `Snapshot` | 在写入之前复制可变内容 |
| `Autosave` | `AutosaveInPlace`（默认）或 `AutosaveOff` |
| `AutosaveDelay` | 最后一次编辑后多久进行自动保存：5 秒 |
| `AskOnQuit` | 退出时询问是否保存，而不是为下次启动保留文稿 |
| `NoUntitled` | 应用启动时若没有要打开的内容，则不创建无标题文稿 |

在 `App.Run` 之前调用 `NewDocuments`。应用启动时，会打开随启动传入的文件，或者崩溃、退出时留有未保存更改的文稿，否则创建一个新的无标题文稿。一个应用可以创建多种文稿；第一种是“文件”>“新建”所创建的。

`Document[T]` 是一个打开的文稿。它的方法可以从任何 goroutine 安全调用：

```go
doc.Content()   // the content
doc.Set(v)      // replaces it, which edits the document
doc.State()     // name, path, edited, read-only, conflict, deleted
doc.Window()    // its window
doc.Save()      // as File > Save
```

### 值与可变内容

内容最好是一个由应用通过 `Set` 替换的值：一个字符串、一个由值组成的结构体，或一个持久快照，例如 `ui.TextSnapshot` 或 `editor.Snapshot`，它们只共享未改变的部分。保存会在 goroutine 上写入保存开始时的内容，同时用户可以继续编辑，所以一个不会被就地修改的值本身就已经是快照。

使用可变内容（例如指向应用就地修改的模型的指针）时，需要提供 `Snapshot`，在主线程上复制内容（应用也在主线程上修改内容），并调用 `Changed` 通知更改：

```go
mygo.DocumentsOptions[*Drawing]{
	Snapshot: func(d *Drawing) *Drawing { return d.Clone() },
	// …
}

drawing.Shapes = append(drawing.Shapes, shape)
doc.Changed()
```

### 编辑状态

文稿的内容和文件不一致时，它处于已编辑状态：

- `Set` 和 `Changed` 会将文稿标记为已编辑。提供 `Equal` 后，用 `Set` 将内容设为与文件一致，就会恢复为未编辑状态；撤销到该内容，或者输入一个字符再删掉，都会得到同样的结果：文本控件的撤销功能无需额外处理。
- `SetEdited` 用于直接设置这个状态，适合自行判断编辑状态的应用，例如维护自身撤销历史的应用：传入 `false` 会将当前内容视为与文件一致，但不写入文件。
- 保存会使它变为未编辑，除非它在写入期间发生了变化：那些更改仍未保存。

`OnStateChange` 在主线程上告知状态的变化。

## 窗口

文稿的窗口显示文稿名：文件名，或者应用显示语言中的“未命名”、“未命名 2”……。文稿窗口的标题在 macOS 上就是文稿名，在 Linux 和 Windows 上则是名称加上应用名，就像记事本和 gedit 显示的那样。页面的 `<title>` 不会改变它。

文稿有未保存的更改时，macOS 会在窗口的关闭按钮中显示一个圆点，Linux 和 Windows 则在标题前显示 `*`。在 macOS 上，标题栏还会显示文件的图标，用户可以把它拖到另一个应用；按住 Command 键点击标题，会列出文件所在的各级文件夹。

任何应用的窗口都可以用 `Window.SetRepresentedFile` 和 `Window.SetDocumentEdited` 显示同样的内容。`Window.Title` 返回不带 `*` 的标题。

## 保存

| | |
|---|---|
| `Save()` | 把文稿写入它的文件。无标题文稿会询问位置，只读文稿会询问另一个文件，文件被其他应用更改的文稿会询问是否覆盖它。`ErrCanceled` 表示用户取消了 |
| `SaveAs(path)` | 把它写入另一个文件，文稿随后对应这个文件；`""` 会询问位置 |
| `SaveAnyway()` | 即使另一个应用更改了它的文件也写入 |
| `Duplicate()` | 打开一个无标题副本，名为“Notes copy” |
| `Rename(name)` | 在它所在的文件夹中重新命名它的文件；`""` 会询问 |
| `MoveTo(dir)` | 把它的文件移到另一个文件夹；`""` 会询问 |
| `Revert()` | 再次读取它的文件，放弃未保存的更改 |
| `Close()` | 关闭它的窗口，必要时会先询问是否保存 |

文件按 `NSDocument` 和 GIO 的方式写入：先写入文稿旁边的新文件，刷入磁盘，再让新文件替换原文件，这样崩溃或磁盘已满都不会留下半个文件。文件会保留元数据：在 macOS 上通过 `NSFileManager`，保留权限、创建日期、Finder 标签和其他扩展属性、ACL 以及文件身份，使别名仍能找到它；在 Windows 上通过 `ReplaceFileW`，保留安全描述符、属性、备用数据流和创建时间，并为另一个正打开着它的程序稍等片刻；在 Linux 上则保留权限、所有者和组。符号链接的目标会被替换；还有其他硬链接的文件，或位于用户无法创建文件的文件夹中的文件，会就地写入。`ReplaceFileW` 中途停止、已经移除了文件时，新文件会被移到它的位置；如果连这也失败，新文件会留在文稿原位置的旁边，保存的错误会指明它。

写入之前，会对照文稿上次读取或写入时记下的修改时间和大小来检查文件：如果另一个应用更改了它，`Save` 会询问，而自动保存不会写入任何内容。

## 自动保存

默认的 `AutosaveInPlace` 会在用户停止编辑片刻后把文稿保存到文件（间隔由 `AutosaveDelay` 决定；用户一直在编辑时，至少每 30 秒保存一次），窗口失去焦点时保存，关闭窗口时也保存，就像 Mac 应用那样。已保存的文稿从不会询问是否保存。

无法就地保存的文稿不会自动保存：无标题文稿，以及文件只读、已被删除或被其他应用更改的文稿。它们关闭时会询问。

`AutosaveOff` 下，文稿只在用户保存时写入，关闭时会询问，这是 Windows 和 Linux 应用的传统做法。

## 恢复副本

有未保存更改、而自动保存又不会写入的文稿，都会在用户停止编辑一秒后保留一份恢复副本，就像 VS Code 的备份和 Zed 的数据库那样：无标题文稿、没有自动保存的应用的文稿、处于冲突中的文稿。副本位于 `PathUserData` 中的 `Recovery/<kind>`，由 `Write` 写出，并记录文件的修改时间和大小；文稿保存、还原或不保存而关闭时，副本会被删除。

应用启动时，崩溃或强制退出留下的文稿会以已编辑状态恢复。文件在此期间发生变化的文稿处于冲突中。应用的每个运行实例都把副本保存在自己的会话里，运行期间锁定，所以另一个实例绝不会打开它们。

## 关闭与退出

| | 询问 |
|---|---|
| 关闭未编辑的文稿 | 无 |
| 关闭已保存、已编辑的文稿，有自动保存 | 无：它会先被保存 |
| 关闭已编辑的无标题文稿，有自动保存 | “要保留新文稿‘未命名’吗？”保存…、删除、取消 |
| 关闭已编辑的文稿，没有自动保存，或无法就地保存 | “要保存对文稿‘Notes.md’所做的更改吗？”保存、不保存、取消 |
| 退出 | 无：文稿会被就地保存或保留，并在下次启动时以已编辑状态恢复 |
| 使用 `AskOnQuit` 退出，有多个文稿 | macOS：“你有 3 个文稿包含未保存的更改。要在退出前检查这些更改吗？”检查更改…、放弃更改、取消，然后依次在各文稿窗口上显示附着式对话框。Linux 和 Windows：在一个对话框中列出这些文稿，提供全部保存、不保存、取消选项 |

这些提示、按钮文本和未命名文稿的名称，都属于 [MyGo 自己的字符串](i18n.md#mygo-自己的字符串)，与应用使用同一种语言。在 macOS 上，提示以附着式对话框（sheet）形式显示。关闭窗口时如果需要询问，窗口会保留到用户作出选择；退出应用时会等待所有文稿处理完毕，取消则保留所有窗口。随后执行退出时，`OnBeforeQuit` 监听器会再次运行。

操作系统结束会话时（注销、重启、在 Windows 和 macOS 上关机），没有时间询问：文稿会立即就地保存或保留，无论有没有 `AskOnQuit`，并在下次启动时以已编辑状态恢复。

不经询问就销毁的窗口（`Window.Destroy`、`App.Exit`）会保留它所属文稿中未保存更改的恢复副本，这些更改会在下次启动时恢复。

## 其他应用更改的文件

文稿会监视自己的文件（见[监视文件](native.md#监视文件)），并在窗口获得焦点时重新查看文件，就像 VS Code 和 Notepad++ 那样，因为监视会漏掉一些更改：

- 没有未保存更改的文稿会再次读取它的文件，`OnReload` 告知应用显示新内容；
- 有未保存更改的文稿处于冲突中：它的状态说明了这一点，保存时会询问保留它的内容还是另一个应用的内容（“仍然保存”、“还原”）；
- 文件被其他应用在原文件夹中重命名后，文稿会跟踪新的文件名；
- 文件被删除后，对应的文稿仍保持打开，状态中标记为 `Deleted`，保存时会再次写入该文件。

## 文件菜单

文稿的菜单角色作用于焦点窗口的文稿，也只对它启用：

| 角色 | macOS | Linux、Windows |
|---|---|---|
| `RoleNew` | 新建 ⌘N | 新建 Ctrl+N |
| `RoleOpen` | 打开… ⌘O | 打开… Ctrl+O |
| `RoleOpenRecent` | 打开最近使用 | 打开最近使用 |
| `RoleSave` | 保存 ⌘S | 保存 Ctrl+S |
| `RoleSaveAs` | 另存为… ⌥⇧⌘S | 另存为… Ctrl+Shift+S |
| `RoleDuplicate` | 创建副本 ⇧⌘S | 创建副本 |
| `RoleRename` | 重新命名… | 重新命名… |
| `RoleMoveTo` | 移到… | 移到… |
| `RoleRevert` | 还原到上次存储的版本 | 还原 |

应用有文稿时，`RoleFileMenu` 包含各平台的文件菜单：macOS 上是 DocumentGroup 的（新建、打开、打开最近使用、关闭、保存、创建副本、重新命名、移到、还原到上次存储的版本），其他地方是记事本和 KDE 的（新建、打开、打开最近使用、保存、另存为、还原、关闭、退出）。有文稿的应用在 Linux 和 Windows 上也会得到菜单栏，带有文件和编辑菜单，直到应用设置自己的菜单。

“打开最近使用”列出应用最近打开和保存的文稿，最新的在前，由 `App.RecentDocuments` 返回：文稿用 `App.AddRecentDocument` 记录它们的文件，这也会把文件记录到系统中（macOS 上的 Dock 菜单、Windows 上的跳转列表、Linux 上最近使用的文件），其中的“清除菜单”会调用 `App.ClearRecentDocuments`。见[最近使用的文稿](app.md#最近使用的文稿)。

## 打开文件

文稿会打开系统要求应用打开的、扩展名匹配的文件（`App.OnOpenFile`）：来自 Finder 或资源管理器、Dock 或跳转列表，或命令行。在[配置](configuration.md)的 `fileAssociations` 中列出这些类型，系统就会在打开它们时提供这个应用：

```json
"fileAssociations": [
  { "ext": ["md", "markdown"], "name": "Markdown Document", "mimeType": "text/markdown" }
]
```

在 Linux 和 Windows 上，每个打开的文件都会再次启动应用：调用 `App.RequestSingleInstanceLock`，让第一个实例打开它们。

打开已打开的文件时，会聚焦对应窗口。在 macOS 上，应用会像文稿应用那样在没有窗口时保持运行，点击它的 Dock 图标会创建一个无标题文稿。

## 网页

文稿窗口的页面通过生成的 TypeScript 客户端获得各自的文稿：把文稿服务绑定为客户端调用它时使用的名称，

```go
notes := mygo.NewDocuments(mygo.DocumentsOptions[Note]{ /* … */ })
mygo.BindAs("Note", notes.Service())
```

页面读取和替换所在窗口中文稿的内容，并跟踪文稿状态：

```ts
import { Note } from "./mygo"; // generated
import { onDocumentReload, onDocumentState } from "mygo-runtime";

editor.value = (await Note.content()).body;
editor.oninput = () => Note.setContent({ body: editor.value });
onDocumentState(({ name, edited }) => { status.textContent = edited ? "Edited" : "Saved" });
onDocumentReload(async () => { editor.value = (await Note.content()).body });
```

该服务提供 `content`、`setContent`、`state`、`save`、`saveAs`、`duplicate`、`rename`、`moveTo` 和 `revert`，作用于调用它们的窗口。页面在每次按键时发送整个内容，适合笔记和小型文稿；大型文稿则适合在用户停顿时发送。

## 平台差异

| | macOS | Linux | Windows |
|---|---|---|---|
| 编辑标记 | 关闭按钮中的圆点 | 标题前的 `*` | 标题前的 `*` |
| 标题栏中的文件 | 代理图标，按住 Command 点击可查看所在文件夹 | 无 | 无 |
| 标题 | 名称 | 名称 - 应用 | 名称 - 应用 |
| 替换文件 | `NSFileManager` `replaceItemAtURL:` | 重命名，带模式、所有者和组 | `ReplaceFileW` |
| 提示 | 附着在窗口上的对话框 | 对话框 | 对话框 |
| 没有窗口时 | 应用保持打开；Dock 图标创建文稿 | 应用退出 | 应用退出 |

## 未包含的内容

在 macOS 上只有真正的 `NSDocument` 才会做的事：在标题栏的浮层中重命名、标题旁的“— 已编辑”、浏览版本并还原到某个版本、锁定文件，以及 iCloud 中的草稿。作为文件夹的文稿（包）尚不支持，浏览器没有可打开的文件。
