# 图标

<!-- preview icon 180 -->

`ui.Icon` 把 SVG 显示为图标：颜色取自文本颜色，与文本一样从祖先继承，而不论文件本身是什么颜色；高度等于字体大小，并保持 SVG 的宽高比。Lucide、Heroicons、Tabler 或 Material Symbols 等图标集可直接使用：

```go
//go:embed icons/save.svg
var saveSVG []byte

var save = ui.MustParseSVG(saveSVG)

ui.Row(c).Gap(6).Children(func() {
	ui.Icon(c, save)
	ui.Text(c, "Save")
})
ui.Icon(c, save).FontSize(24).TextColor(c.Theme().Danger)
```

`FontSize` 和 `TextColor` 与文本一样控制图标的大小和颜色；也可以用 `Size` 指定其他尺寸。图标不会为填满列宽而拉伸。

## 按钮中

按钮会将子元素横向排列，并让它们继承按钮的文本颜色；在按钮中，把图标放在标签之前：

```go
ui.PrimaryButton(c, "").Children(func() {
	ui.Icon(c, save)
	ui.Text(c, "Save").SingleLine()
})
```

`Rotate` 旋转图标，例如旋转指示器或表示展开的箭头：

```go
spin := ui.Icon(c, loader)
spin.Rotate(spin.Loop("spin", time.Second, ui.Linear) * 360)
```

MyGo 如何绘制 SVG、会绘制其中的哪些内容，见[图像](image.md#svg)页面。在 `Draw` 回调中，`Painter.Icon` 绘制图标。

## 辅助功能

图标是辅助技术看不到的装饰，除非用 `Label` 为它们命名：只显示图标的按钮应改为给按钮命名。
