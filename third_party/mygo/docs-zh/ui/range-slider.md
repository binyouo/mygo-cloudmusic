# 范围滑块

<!-- preview range-slider 120 -->

`ui.RangeSlider` 创建一个带两个旋钮的滑块，用两个 `*float64` 设置指定上下界内的最小值和最大值，如价格范围：每个旋钮都可获得键盘焦点，Tab 在两者之间切换焦点，方向键、Home 和 End 可移动当前聚焦的旋钮；按下轨道会移动离按下点最近的旋钮。两个旋钮不会互相越过。`Changed` 报告新值。

```go
ui.RangeSlider(c, &app.minPrice, &app.maxPrice, 0, 1000, 10).Label("Price")
ui.Textf(c, "$%.0f – $%.0f", app.minPrice, app.maxPrice)
```

步长大于 0 时，值会吸附到下界或下界加上步长整数倍的位置，并显示刻度线；步长为 0 时可连续调整。

## 辅助功能

辅助技术看到两个滑块，名称由范围滑块的 `Label` 加上“minimum”和“maximum”组成。
