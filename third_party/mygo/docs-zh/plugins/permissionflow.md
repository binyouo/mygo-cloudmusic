# PermissionFlow

permissionflow 插件检查和请求辅助功能、屏幕录制、麦克风等需要用户授权的系统权限，并引导用户到相应位置授权。在 macOS 上，它会打开系统设置的对应页面，在窗口下方显示随窗口移动的面板，供用户将应用拖入权限列表，与 [PermissionFlow](https://github.com/jaywcjlove/PermissionFlow) 在 Swift 应用中的行为相同。插件完全用 Go 编写，没有 JavaScript 包，也无需调用 `mygo.Use`；网页通过应用自己的绑定访问它。

```go
import "github.com/egoist/mygo/plugins/permissionflow"

st, err := permissionflow.Check(permissionflow.Accessibility)
if err == nil && st != permissionflow.Granted {
	st, err = permissionflow.Guide(ctx, permissionflow.Accessibility, permissionflow.GuideOptions{})
}
```

[examples/permissionflow](../../examples/permissionflow) 列出系统的权限及其状态，并以两种方式请求它们。

## 权限

| 权限 | macOS | Windows | 提示（`Request`） | 拖放（`Guide`） |
| --- | --- | --- | --- | --- |
| `Accessibility` | 是 | — | 是 | 是 |
| `ScreenRecording` | 是 | — | 是 | 是 |
| `InputMonitoring` | 是 | — | 是 | 是 |
| `FullDiskAccess` | 是，通过读取受保护的文件 | — | — | 是 |
| `AppManagement` | 检查无法判断 | — | — | 是 |
| `DeveloperTools` | 检查无法判断 | — | — | 是 |
| `Microphone` | 是 | 是 | macOS | — |
| `Camera` | 是 | 是 | macOS | — |
| `Calendars` | 是 | — | 是 | — |
| `Reminders` | 是 | — | 是 | — |
| `Bluetooth` | 是 | — | 是 | 是 |
| `MediaAppleMusic` | 检查无法判断 | — | — | 是 |

如果系统不维护这类权限——例如 Windows 对桌面应用只提供麦克风和相机，Linux 对沙箱外的应用则什么都不提供——这些函数会返回 `errors.ErrUnsupported`。`Allowed` 把这一点也考虑在内：它报告应用能否使用某个权限所保护的内容，依据是应用已拥有该权限，或系统并不要求；在浏览器中为 false。

macOS 27 将辅助功能分别命名为“设备控制”和“数据访问”，`Title` 会跟随这一命名。

## 检查与请求

`Check` 返回权限状态 `Status`，不会询问用户：结果为 `Granted`、`Denied`、`NotDetermined`（尚未询问），或系统无法判断时的 `Unknown`。macOS 不提供辅助功能、屏幕录制和输入监控权限是否已请求过的信息，因此这些权限在获准前都返回 `Denied`；屏幕录制权限的变化还需重启应用后才能检测到。

`Request` 用系统的提示询问，macOS 对每个应用只显示一次；之后 `Request` 只报告状态，用户可以在系统设置中改变主意。麦克风、相机、日历、提醒事项和蓝牙的提示需要应用的 Info.plist 说明请求原因（`mygo.config.ts` 中的 `macos.infoPlist`）：

```ts
export default defineConfig({
  macos: {
    infoPlist: {
      NSMicrophoneUsageDescription: "Notes records the voice memos you start.",
    },
  },
});
```

应用没有这项说明却发起请求时，macOS 会终止该应用，因此 `Request` 改为返回 `ErrUsageDescription`。在应用包之外运行的程序，例如用 `go run` 运行的程序，会以启动它的应用（例如终端）的名义请求权限，获得权限的是该应用。

`Wait` 等待直到应用拥有某个权限，或上下文结束。

## 引导

`Guide` 打开系统设置的相应页面（`OpenSettings` 仅打开页面），并等待用户授予权限。在 macOS 上，在那些可以把应用拖进列表的页面上，它还会在系统设置的窗口下方显示一个面板：

- 面板上方显示“将 **MyApp** 拖到上方列表以允许**辅助功能**。”，下方是带图标和名称的应用卡片，可拖入列表。
- 它与页面对齐，位于侧边栏右侧，随窗口移动和调整大小，并保持在屏幕上。
- 它从不获取焦点，因此系统设置始终位于最前，而当另一个应用位于最前时它会隐藏。
- 应用是程序运行所在的应用包（`GuideOptions.App` 改变它，`Name` 改变其名称）。

`Guide` 在应用拥有权限、用户关闭面板或系统设置、或上下文结束时返回状态。一次只显示一个面板：另一个 `Guide` 会关闭前一个。在 Windows 上，它打开“设置”的“隐私和安全性”页面，并在应用拥有权限或上下文结束时返回。

从 goroutine 调用 `Request`、`Wait` 和 `Guide`，不要从主线程调用，它们会阻塞主线程：绑定的方法和页面的事件处理器在自己的 goroutine 上运行。`OpenSettings` 和 `Guide` 需要 `App.Run` 已启动。

## 声明

该插件参考了 PermissionFlow（MIT，© 小弟调调）的引导行为、设置页面、面板布局和文本；许可证见 [NOTICE.md](../../plugins/permissionflow/NOTICE.md)。代码由 MyGo 自行实现。
