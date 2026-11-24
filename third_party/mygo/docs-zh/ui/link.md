# 链接

<!-- preview link 160 -->

`ui.Link` 创建一个文本链接，单击或获得键盘焦点后按 Enter，会在浏览器或为相应 URL 协议注册的应用中打开链接；按 Space 则滚动外围容器，与网页中的行为一致。链接使用强调色，指针悬停时显示下划线，下划线会连续穿过字母下伸笔画和斜杠，而 macOS 上的其他下划线会避开这些笔画：

```go
ui.Link(c, "Privacy policy", "https://example.com/privacy")
```

## 路由器中的路径

在[路由器](navigation.md)的页面中，不带协议前缀的路径，如 `/notes/42`、`edit` 或 `?tab=info`，会通过路由器跳转，效果与 `Push` 相同；相对路径以当前页面为基准解析，与网页链接一致：

```go
ui.Link(c, "Next note", fmt.Sprint(id+1))
```

## 句子内

在[富文本](text.md#句子中的元素)内，链接是段落的一部分，随它换行；Tab 仍能到达它：

```go
ui.RichText(c).Children(func() {
	ui.Text(c, "Read ")
	ui.Link(c, "the guide", "https://example.com/guide")
	ui.Text(c, " to get started.")
})
```

给它一个空标签并用 `Children` 为文本的各部分设置样式。

## 其他操作

如果只需单击文本来执行应用内操作，而不打开 URL，可以将 `Link` 的 URL 留空并查询 `Clicked`，也可以使用[按钮](button.md)。

```go
if ui.Link(c, "Show details", "").Clicked() {
	app.details = true
}
```

## 辅助功能

辅助技术看到一个由文本命名的链接，跟随它的方式与单击一样。
