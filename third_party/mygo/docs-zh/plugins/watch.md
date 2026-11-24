# 文件监视

watch 插件让页面监听应用授权的目录和文件的变化，例如编辑器中打开的项目。Go 代码和原生 UI 无需通过插件，可以直接调用 `mygo.WatchFiles`（见[监视文件](../native.md#监视文件)），插件底层也使用这个 API。

## 设置

在 Go 中使用插件，并给出页面可以监视的目录和文件，每一项都对应一个名称：

```go
import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/watch"
)

func main() {
	mygo.Use(watch.New(watch.Options{
		Roots: map[string]watch.Root{
			"project": {Path: projectDir, Recursive: true},
			"settings": {Path: settingsFile},
		},
	}))
	// ...
}
```

并把它的包添加到前端：

```sh
bun add @mygo-plugins/watch
```

页面通过名称而非路径指定监视根，因此无法监视授权范围之外的内容。名称由小写字母、数字和短横线组成，并以字母开头。路径必须是绝对路径，且在 `mygo.Use` 运行时指向现有目录或文件，否则会 panic；`mygo.Use` 会解析路径中的符号链接，例如 macOS 的 `/tmp`。未配置监视根的 `watch.Plugin` 不允许页面监视任何内容。

`Recursive` 允许页面监视子目录及其内容；未设置时，只能监视根目录本身及其直接包含的文件。`Ignore` 按相对于根的路径排除文件和目录，与 `mygo.FileWatchOptions.Ignore` 相同：被排除的目录（如 `node_modules`）不会被打开，也不计入监视条目上限（`MaxEntries`：8192，macOS 上为 65536）。

```go
"project": {Path: projectDir, Recursive: true, Ignore: func(p string, isDir bool) bool {
	return isDir && (path.Base(p) == "node_modules" || path.Base(p) == ".git")
}},
```

根也可以在应用运行时授予，例如用户选择的文件夹：对于不在 `Roots` 中的名称，会带着调用的上下文向 `Resolve` 询问该名称对应的根；对于它不知道的名称，返回 false。

```go
var granted sync.Map // name → watch.Root

watch.New(watch.Options{
	Resolve: func(ctx context.Context, name string) (watch.Root, bool) {
		root, ok := granted.Load(name)
		if !ok {
			return watch.Root{}, false
		}
		return root.(watch.Root), true
	},
})

// After the user picked dir in a dialog, a bound method returns the name
// the page then watches.
granted.Store("picked-1", watch.Root{Path: dir, Recursive: true})
```

设置 `Allow` 后，会在每次监视之前询问它，传入调用的上下文（`mygo.CallerWindow(ctx)` 是发出请求的窗口）和 `watch.Request`：根的名称、其中的路径以及是否递归。它可以拒绝：

```go
watch.New(watch.Options{
	Roots: roots,
	Allow: func(ctx context.Context, r watch.Request) bool {
		return mygo.CallerWindow(ctx) == editorWindow
	},
})
```

每个页面一次可以监视 8 个目录和文件（`MaxWatchesPerPage`），所有页面共 128 个（`MaxWatches`）。

## 监视

`watch` 启动监视，待所有目标都纳入监视后返回。迭代返回值即可获取变化，每次变化暂歇后会收到一批事件：

```ts
import { watch } from "@mygo-plugins/watch";

const watcher = await watch("project", { recursive: true });
for await (const events of watcher) {
  for (const event of events) console.log(event.op, event.path, event.oldPath);
}
```

`path` 监视根中的目录或文件，而不是根本身，路径相对于根并用斜杠分隔（`"src"`、`"package.json"`）；它不能离开根，也不能经过符号链接。

事件包含 `op`（`create`、`write`、`remove` 和 `rename` 之一）、`path`（相对于被监视的内容，用斜杠分隔；`.` 表示被监视的内容本身）、`oldPath`（重命名从哪里移来）和 `isDir`。监视启动时已经存在的内容不会被报告。被监视的文件通过其目录来监视：文件被替换，例如编辑器保存时，是对 `.` 的 `write`；被移除是对 `.` 的 `remove`；再次出现是对 `.` 的 `create`。当被监视的内容，或被监视文件的目录被移除、重命名或替换时，最后一批会有一个对 `.` 的 `remove`，迭代随之结束。

最后一次更改后等待 50 毫秒再报告；若更改持续发生，最多等待 500 毫秒。`debounceMs` 设置这段防抖等待时间，与 Go 的 `Debounce` 相同：`0` 表示 50 毫秒，负数表示立即报告。监视通过比较前后状态生成事件，只报告差异，不记录每个中间步骤：文件连续写入多次只报告一次写入，在原路径被另一个文件替换（如原子保存）也算一次写入，两次检查之间创建又移除的文件则不产生事件。应将这些事件视为重新读取文件的提示。页面取走一批事件后，Go 才会再次检查，因此页面忙碌时也不会漏掉状态变化。

调用 `watcher.close()`、用 `break` 跳出循环，或触发通过 `signal` 传入的 `AbortSignal`，都会停止监视，页面导航或关闭也一样。Go 停止监视后，`watcher.closed` 会完成；若因错误停止，则会以该错误拒绝。错误类型为 `WatchError`，其 `code` 为：

- `denied`：名称不是根之一、`Allow` 拒绝、路径不在根中，或页面在没有 `Recursive` 的根上请求递归监视，或请求监视该根中的某个目录；
- `not-found`：路径不存在；
- `limit`：页面或应用监视得太多，或目录中的文件和目录超过 `MaxEntries`（8192，macOS 上为 65536）；
- `unsupported`：平台无法监视文件；
- `path-encoding`：文件名不是有效的 UTF-8，无法提供给页面；
- `io`：其他任何情况，例如自应用启动以来根被替换。

## 平台

监视只检查目录中的内容，不跟随其中的符号链接或 Windows 目录联接。只应开放应用可控制的目录：使用名称可以防止页面自行选择路径，但其他程序仍能移动或替换目录内容。

- macOS 用 FSEvents 监视：一次监视是一个事件流，覆盖其根之下的一切，因此不会为每个文件花费一个文件描述符，而被监视的文件由其目录的流覆盖。只有当 FSEvents 表示内容已改变时，写入才会被报告，因为 APFS 也会把 O_TRUNC 写入称为创建；其余情况由监视的比较来报告。
- Linux 用 inotify 监视每个目录，整个应用共用一个实例，其限制是系统的限制（`fs.inotify.max_user_watches`）。
- Windows 用一个 `ReadDirectoryChangesW` 监视递归目录，因此其中的目录可以被重命名，它还监视其父目录以得知它何时被重命名。在目录被监视期间，Windows 不允许其父目录及上面的目录被重命名，对每个监视文件的程序都是如此。驱动器的根可以被监视，云文件文件夹也可以，例如 OneDrive 的文件夹。
