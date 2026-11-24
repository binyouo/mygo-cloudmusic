# 渲染

MyGo 在 macOS 上使用 Metal，在 Windows 上使用 Direct3D 11（没有可用的 GPU 驱动时使用 Windows 自带的软件渲染器 WARP），在 Linux 上通过 GTK 的 GtkGLArea 使用 OpenGL，在[浏览器](browsers.md)中使用 WebGL 2，并复用 Linux 的着色器。着色器根据与边缘的距离绘制圆角矩形、边框、渐变和阴影，因此在任意尺寸和缩放比例下都能保持清晰；文本使用字形图集，且只上传变化的部分。

有 GPU 时，每一帧都在 GPU 上绘制。Linux 的首个原生 UI 窗口会加载 OpenGL：Mesa 驱动会常驻约 50 MB 内存，约等于一个小应用其余部分的占用。在 OpenGL 无法使用 GPU 的环境中，例如某些虚拟机或 WSL，Linux 会改用 CPU 渲染器（WSL 可用 `GALLIUM_DRIVER=d3d12` 让 Mesa 使用 GPU）。在高密度显示器上，多核绘制一个完整的大窗口约需 1 毫秒；通常发生变化的局部内容（如指针下的按钮）只需不到 0.1 毫秒，因为只会重绘该区域，并仅将该区域提交给合成器。设置 `MYGO_GPU=0` 可在所有平台上使用 CPU 渲染器，便于对比；Linux 上设置 `MYGO_GPU=1` 则强制使用 OpenGL，即使它实际运行在 CPU 上。

设置 `MYGO_FRAME_STATS=1` 可记录耗时超过 8 毫秒的帧，也可用 `MYGO_FRAME_STATS=4` 等值指定其他毫秒阈值（`all` 记录每一帧）。每行日志包含构建（含构建轮次）、布局、绘制和呈现的耗时，绘制位置是 GPU 还是内存（窗口没有 GPU 渲染器时），期间整个进程的内存分配量和文本排版次数，以及是否发生垃圾回收、goroutine 协助标记的耗时：

```
mygo: frame 412 took 17.3 ms: build 3.1 (2 passes), layout 1.2, paint 0.8, present 11.5 (drawn on the GPU), other 0.7; meanwhile the process made about 2312 allocations, 171.0 KB, 12 text layouts; GC: 1 cycles ended, 4.1 ms of assists
```

内存分配和垃圾回收统计覆盖整个进程，包括其他 goroutine，并沿用 `runtime/metrics` 按 span 统计的方式，因此分配较少时读数可能偏低。未设置该变量时，不会进行帧测量。连续帧会等待显示器的下一次刷新，呈现操作也可能等待刷新；要测量最高帧率，可同时设置 `MYGO_UNCAPPED=1`（[帧节奏](#帧节奏)）。

MyGo 只在有东西变化时绘制一帧：输入、`Invalidate`、`After`，或会移动的动画。空闲窗口不绘制任何东西，在最后一帧之后两秒，它会释放内存中绘制的帧。

macOS 上播放动画的窗口会为显示输出保留三个帧缓冲区，而不是交替使用两个：在 120 Hz 显示器上，系统经常会同时占用两个缓冲区，导致绘制等待可用缓冲区，部分帧因而无法绘制。额外的第三个缓冲区会保留到窗口空闲，按窗口每像素约占 4 字节（使用超出 sRGB 的颜色时为 8 字节）。

## 帧节奏

连续帧（如动画）按显示器刷新节奏绘制。只要每帧耗时小于刷新间隔，无论场景负载高低，都显示为刷新率对应的帧率，例如 120 Hz 显示器上均为每秒 120 帧。设置 `MYGO_UNCAPPED=1` 可不等待刷新就呈现帧，用于测量场景在一个刷新周期内还剩多少性能余量、比较不同负载下的两个构建，或满足手写笔迹、远程桌面和游戏等低延迟需求。只要仍有动画或帧请求，下一帧就会在上一帧呈现后立即开始；内容不变的窗口仍然不绘制。同时设置 `MYGO_FRAME_STATS` 时，MyGo 会记录每秒帧数，并为每条帧日志标注 uncapped：

```
mygo: 2481 frames a second, uncapped: 0.40 ms a frame
```

默认仍同步刷新节奏：不限帧率会尽可能占用 CPU 和 GPU，还可能出现画面撕裂，通常不适合普通应用。各平台行为如下：

| 平台 | Uncapped |
|---|---|
| Windows | 在系统允许时，交换链允许撕裂（`DXGI_FEATURE_PRESENT_ALLOW_TEARING`），每一帧都以同步间隔 0 呈现，既不等交换链的帧延迟对象，也不等 DWM 的下一次合成。只有当窗口占据整个显示器时，帧才会出现撕裂，如覆盖屏幕的无边框窗口；在其他地方，DWM 仍会在每次刷新时显示最新的帧。不允许撕裂时，帧仍会立即呈现，新帧会取代 DWM 尚未显示的那一帧。位于材质（DirectComposition）之上的窗口没有撕裂。窗口改变大小时，从开始到上次改变后半秒的那一帧为止，帧都按节奏呈现，如同没有 `MYGO_UNCAPPED`，并记录 `paced while resizing`：uncapped 的帧在 DWM 调整窗口大小时排队，使一次呈现停顿数秒。新尺寸的帧最多等待 DWM 两次刷新来显示，此后每一帧都用 `DXGI_PRESENT_DO_NOT_WAIT` 呈现：Present 无法在一次刷新内接受的帧，在下次合成后呈现，或让位给下一帧，同时窗口继续处理自己的消息，这样任何帧都不会长时间等待 DWM。100 毫秒后仍未被接受的帧会不带该标志呈现，在另一个线程上等待 DXGI，同时窗口继续处理消息，它完成后下一帧再绘制。此后，帧保持按节奏，使用 `DXGI_PRESENT_DO_NOT_WAIT`，直到调整大小稳定后 DXGI 立即接受一帧；缓冲区的调整大小（无论为了稳定还是增长）也在另一个线程上进行：除非有一次调用等待它，否则 DXGI 不会再以该标志接受帧，而在主线程上进行这样一次调用会让一帧卡住约一秒。这样一次调用等待期间到来的帧会被推迟，窗口显示最后一帧。Present 拒绝的帧记录 `Present still drawing N times`，等待时记录 `presenting once DXGI can`；被推迟的帧记录 `put off while Present waits on another thread`（或 `ResizeBuffers`），它之后的帧记录 `after Present waited on another thread`，调整大小稳定后按节奏的帧记录 `paced until DXGI takes a frame at once`。 |
| macOS | 不再由显示链接控制帧节奏：事件循环处理完新输入后，在下一轮开始绘制，将帧提交到关闭了 `displaySyncEnabled` 的 `CAMetalLayer`。窗口服务器仍在每次刷新时合成窗口，显示最新帧，不会撕裂。超过显示器刷新率的帧仍会绘制并计入测量，但不会全部显示。 |
| Linux | GTK 帧时钟在上一帧绘制完毕后立即开始下一帧。X11 上窗口的 GLX 交换间隔设为 0（Wayland 上 GTK 的 EGL 交换间隔原本就是 0）。如果合成器也会控制窗口的帧节奏，这部分限制仍然有效：X11 下包括发送 `_NET_WM_FRAME_TIMINGS` 的合成器，Wayland 下包括内存绘制帧的帧回调。 |
| 浏览器 | 忽略此设置：帧节奏由 `requestAnimationFrame` 控制，无法提前呈现。 |

游戏在不限帧率时仍保持固定的逻辑更新频率：无论绘制多少帧，[`game.View`](../plugins/game.md) 都按实际经过的时间，每秒运行 `Update` 60 次，`Draw` 则根据当前帧在下一个逻辑步长内的进度绘制。
