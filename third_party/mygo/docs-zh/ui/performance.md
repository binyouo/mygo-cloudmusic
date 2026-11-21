# 原生 UI 性能

这些测量涵盖用 `ui.Tester` 在 CPU 上渲染的稳定状态帧。它们于 2026 年 10 月 8 日在 Apple M5、macOS arm64、Go 1.27.1 上，以 `CGO_ENABLED=0 GOMAXPROCS=1` 测得。每个耗时都是预热视图后六个连续 300 ms 样本的中位数。

| 基准用例 | 每帧时间 | 每帧分配次数 | 每帧分配的字节数 |
|---|---:|---:|---:|
| 1,000 个静态标签 | 397 µs | 0 | 0 |
| 一百万行列表中可见的行 | 14.5 µs | 20 | 176 |
| 没有变化的 diff 视图 | 389 µs | 2 | 72 |
| 完整的 50 KB Markdown 回复 | 1.3 ms | 353 | 15,894 |
| 同一回复流式显示，每帧 16 字节 | 0.8 ms | 350 | 35,946 |

Markdown 的这几项——`ui/markdown` 的 `BenchmarkViewSteady` 和 `BenchmarkViewStream`——于 2026 年 10 月 14 日在一台更繁忙的机器上测得；解析器本身的开销见 [Markdown](markdown.md#性能)。

元素句柄会校验其所有者、内存池槽位和构建代次。静态标签的句柄不分配内存。输入绑定、动作队列和持久身份绑定在预热后复用存储。复合控件可能会分配回调闭包；这些基准用例并未覆盖每个控件或交互。

标签和列表的用例使用公开 API。diff 用例使用私有的渲染器 API，测量其稳定帧的分配下限。

## 一帧的开销

窗口只在有东西请求时绘制一帧：可能改变视图显示内容的输入、`Window.Update` 或 `Invalidate`、`After` 的定时器，或会移动的东西。大多数帧会重新构建视图：视图函数会完整运行，MyGo 会对其构建的每个元素进行比较、布局和定位，无论是否发生变化，然后绘制实际显示的内容。布局会复用未变化的部分，但工作量随视图构建的元素数量增长，而不是随变化量增长。这些帧于 2026 年 11 月 9 日在 Apple M5 上以 `GOMAXPROCS=1` 在一台繁忙的机器上测得，场景是 `Scroll` 中由标签和按钮组成的行：

| 发生的情况 | 每帧时间 |
|---|---:|
| 1,000 行中的一个标签发生变化 | 2.8 ms |
| 3,000 行中的一个标签发生变化 | 11 ms |
| 要求 3,000 行重绘，但没有任何变化 | 11 ms |
| `List` 中 3,000 行中的一个标签发生变化 | 0.2 ms |
| 指针移到 3,000 行中的某个按钮上 | 0.3 ms |
| 指针在 3,000 行的行间移动 | 0.1 ms |
| 在上方文本框中输入一个字母，下方是这 3,000 行 | 17 ms |

`List` 和 `Table` 只构建可见的行，因此它们的帧开销取决于显示的内容，与集合有多长无关。把长集合放进它们，而不是放进会构建每一行的 `Scroll`。

**悬停。** 指针移到元素上或离开它时，只有当有东西查看该元素的悬停状态时才会请求一帧。工具包的控件在绘制时决定悬停外观，滚动容器在绘制时显示滚动条，所以这一帧只会重绘上一帧的元素，不会运行你的视图。若在视图构建时调用 `Hovered`，指针移到该元素上或离开它时就会重新构建整个视图。若只有绘制随悬停变化，就改为在它的 `Draw` 回调中读取 `Hovered`：

```go
card := ui.Box(c).Size(200, 120)
card.Draw(func(p *ui.Painter, r ui.Rect) {
	if card.Hovered() {
		p.Fill(r, c.Theme().SurfaceHover, 8)
	}
})
```

**构建轮次。** 控件在构造函数返回时就处理输入，与 Dear ImGui 一样，因此后续代码能读到新值。随后视图会在同一帧中再构建一次，让控件之前构建的内容也能反映新值：输入一个字母会构建两次视图。`MYGO_FRAME_STATS` 记录每帧的构建轮次（见[渲染](rendering.md)）。

## 保留的内存

包含 1,000 个标签的视图在预热后，保留约 **2.50 MB 的 Go 堆内存**，结果取自三个样本。测量时先预热字体和渲染，释放用于预热的视图并强制垃圾回收，再创建第二个视图，运行十帧并再次回收后，测量它仍占用的内存。

测量包含无头渲染环境的内存，不包含原生窗口的进程内存或 GPU 资源。

## 测量范围

计时取决于机器、渲染后端、内容和其他进程中的活动。较小的计时差异应在多台机器上复核；分配次数更稳定。这些结果描述稳定帧，不将 MyGo 与另一个 GUI 框架进行比较。

<!-- repository-only:start -->

## 运行基准测试用例

在 MyGo 仓库的根目录：

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui -run '^$' \
  -bench '^(BenchmarkValueFrame|BenchmarkValueList|BenchmarkDiffSteady)$' \
  -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui -run '^$' \
  -bench '^Benchmark(LiveRows|ListRows)' -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui/markdown -run '^$' \
  -bench '^BenchmarkView' -benchmem -count=6 -benchtime=300ms
CGO_ENABLED=0 GOMAXPROCS=1 go test ./ui \
  -run '^TestValueFrameRetainedHeap$' -count=3 -v
```

计时期间不要同时编译或运行其他测试。

<!-- repository-only:end -->
