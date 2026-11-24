# 进度条

<!-- preview progress 180 -->

`ui.Progress` 创建一个进度条，用 0 到 1 之间的值表示完成比例；负值表示进度不确定，此时会显示持续移动的色块。

```go
ui.Progress(c, float64(app.done)/float64(app.total)).Label("Uploading")
ui.Progress(c, -1).Label("Connecting")
```

`Reverse` 让它从右侧填充，用于从右到左布局的界面。它会横向铺满所在的列；在行中则要给它设置 `Width`。

进度条默认使用主题强调色填充，轨道使用 `Border` 颜色。要采用自定义配色，可用 `TextColor` 设置填充色，用 `Background` 设置轨道颜色；进度不确定时的移动色块也使用该填充色：

```go
ui.Progress(c, app.synced/app.total).TextColor(brand).Background(brandTrack).Label("Syncing")
```

进度不确定时，进度条只在可见时播放动画，开销仅限于绘制：窗口按显示器刷新率重绘上一帧，无需重新运行视图。

若要占用空间更少的旋转指示器，用[旋转指示器](spinner.md)；若要显示磁盘使用量等不表示进度的数值，用[量度计](meter.md)。

## 辅助功能

辅助技术会将其识别为进度指示器，读取当前进度或进度不确定的状态，名称由 `Label` 指定。
