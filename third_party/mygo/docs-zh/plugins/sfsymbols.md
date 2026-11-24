# SF Symbols

sfsymbols 插件为 macOS 上的原生 UI 绘制 Apple 的 SF Symbols。符号从运行中的系统读取：AppKit 按各自的设计和粗细绘制每个符号，插件再把像素复制到图像中。你的应用及其构建产物中不包含 Apple 的任何图形素材。全部用 Go 写成：没有 JavaScript 包，也不需要 `mygo.Use`。

显示一个符号，就像 SwiftUI 的 `Image(systemName:)` 那样：

```go
import "github.com/egoist/mygo/plugins/sfsymbols"

sfsymbols.Symbol(c, "gearshape")
sfsymbols.Symbol(c, "trash").Size(20).Color(t.Accent).Weight(sfsymbols.Semibold).Fill()
sfsymbols.Symbol(c, "sparkles").Fallback(sparklesSVG) // where the system has none
```

## Symbol

`Symbol(c, name)` 创建一个元素，以主题的字体大小和文本颜色显示该符号，并按显示器的缩放绘制以保持锐利；它的辅助功能名称是 `name`。它的修饰符可以链式调用，与 SwiftUI 的相同：

| sfsymbols | SwiftUI |
|---|---|
| `.Size(24)` | `.font(.system(size: 24))` |
| `.Color(col)` | `.foregroundStyle(col)` |
| `.Weight(sfsymbols.Bold)` | `.fontWeight(.bold)` |
| `.Fill()` | `.symbolVariant(.fill)`：`trash` 用 `trash.fill`，符号没有填充变体时用符号本身 |
| `.Natural()` | 符号与同字号的其他符号并排时的大小，而不是填满自己的盒子 |
| `.Fallback(svg)` | 系统没有该符号时使用的 SVG，采用符号的颜色 |

`SymbolElement` 也是 `ui.Element`：`Margin`、`Tooltip` 等对它都有效。系统没有该名称的符号时（在另一个操作系统上，或较旧的 macOS 上），它以相同的大小绘制回退内容，或什么都不画。

与 SwiftUI 不同，它不跟随所在元素的文本颜色（而是使用主题的文本颜色，或 `Color` 指定的颜色），不与旁边的文本按基线对齐，也不支持以多种颜色绘制多色符号和分层符号。

## 名称

`Names()` 返回当前运行的 macOS 拥有的每个 SF Symbol 的名称，顺序与 SF Symbols 应用显示的一致（有数千个，其中包含 `.ar` 和 `.ja` 之类的本地化变体），可用于符号选择器或搜索。它读取系统自己的目录，因此你的应用中没有 Apple 的任何内容。该目录不是公开 API：如果某个 macOS 版本移动了它的位置，就会返回 nil，而 `Symbol` 和 `Image` 仍会绘制它们指定的符号。其他操作系统都返回 nil。

## 图像

用 `ui.Painter` 绘制时，`Image(name, px, col, opts)` 返回名为 `name`（如 `"gearshape"`）的符号，以 `col` 颜色绘制为 `px` 像素见方，作为 `ui.ImageSource`。系统没有该符号时它返回 false：在另一个操作系统上，或缺少该符号的 macOS 上。`Available` 报告系统是否绘制 SF Symbols。

- `opts` 含有 `Natural` 和 `Weight`，与 `Symbol` 的修饰符相同。
- `px` 限于 8 到 512；像素尺寸超过此范围的 `Symbol` 会从 512 放大。请按 DIP 尺寸的两倍或三倍绘制，让符号在任何显示器上都保持锐利。
- 默认情况下，符号图形会填满边界框，居中并留少量内边距。`Options{Natural: true}` 改为以统一比例绘制所有符号，保留同字号符号并排时的相对大小：十字形比宽矩形小，以维持 SF Symbols 的视觉平衡。
- 结果按名称、大小、颜色和选项缓存，系统没有对应符号的名称也会缓存；超过限制时丢弃最久未使用的，因此动画变化的颜色或大小不会让内存无休止地增长。`Image` 可从任何 goroutine 安全调用。

其他操作系统都没有 SF Symbols：`Image` 在那里返回 false，所以请准备一个自己的图标作为回退，并对它返回的内容使用 `ui.Image` 或 `Painter.Image`（配合 `ui.Contain`）。

不导入该插件的应用不会因此增加开销。

见 `examples/sf-symbols`，可查看所有符号并搜索：`go run ./examples/sf-symbols`。
