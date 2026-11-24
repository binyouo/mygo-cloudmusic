# 头像

<!-- preview avatar 150 -->

`ui.Avatar` 为人物或事物创建头像：有图像时裁成圆形；没有图像时，在由名称决定的背景色上显示名称的首字母，因此同一个人的颜色始终一致。头像高度略大于主题字号的两倍。

```go
ui.Row(c).Gap(8).Children(func() {
	ui.Avatar(c, user.Name, user.Photo) // a *ui.Bitmap, or nil
	ui.Text(c, user.Name)
})
```

## 辅助功能

辅助技术看到一张以名字命名的图像。
