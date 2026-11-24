# 量度计

<!-- preview meter 200 -->

`ui.Meter` 创建条形指示器，显示给定上下限之间的数值，如同 AppKit 的电平指示器和 Web 的 `meter`：默认使用主题的 `Success` 颜色，达到指定阈值后改用 `Warning` 和 `Danger`；传 nil 表示不设置阈值。

```go
size := app.disk.Size
ui.Meter(c, app.disk.Used, 0, size, &ui.MeterLevels{Warning: 0.8 * size, Critical: 0.95 * size}).Label("Disk")
```

`MeterLevels` 指定警告和危险阈值：达到 `Warning` 时用 `Warning` 颜色，达到 `Critical` 时用 `Danger` 颜色。`Critical` 低于 `Warning` 时，则数值越低越需要警示，例如电池电量：

```go
ui.Meter(c, app.battery, 0, 100, &ui.MeterLevels{Warning: 20, Critical: 10}).Label("Battery")
```

## 颜色

`NormalColor`、`WarningColor` 和 `CriticalColor` 可以自定义填充色，分别替代主题的 `Success`、`Warning` 和 `Danger`；零值表示沿用主题颜色。下面的存储用量条在接近满容量之前显示为中性灰色，随后变为琥珀色，最后使用主题的 `Danger`：

```go
ui.Meter(c, used, 0, size, &ui.MeterLevels{
	Warning: 0.8 * size, Critical: 0.95 * size,
	NormalColor: ui.RGB(142, 142, 147), WarningColor: ui.RGB(255, 159, 10),
}).Label("Storage")
```

若没有 `Warning` 和 `Critical`，条会全程使用 `NormalColor`，例如产品自有配色的配额条。`Background` 为轨道着色，默认为主题的 `Border`：

```go
ui.Meter(c, app.used, 0, app.quota, &ui.MeterLevels{NormalColor: brand}).Background(brandTrack).Label("Backup quota")
```

自定义颜色不会改变指示条的形状、数值范围、从右到左布局的行为，也不影响辅助技术读取的信息。

对于进行中的工作，用[进度条](progress.md)。

## 辅助功能

辅助技术看到一个表示该值的电平指示器，由 `Label` 命名。
