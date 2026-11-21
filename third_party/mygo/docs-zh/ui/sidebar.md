# 侧边栏

<!-- preview sidebar 280 -->

`ui.Sidebar` 创建源列表，类似 Finder 和 Mail 的侧边栏：列表项带有图标，按节分组，单击节标题可展开或收起其中的项；选中项的 ID 写入 `*string`。内容放不下时可滚动。需要为它设置宽度：

```go
ui.Sidebar(c, &app.mailbox, func() {
	ui.SidebarSection(c, "Mailboxes", &app.mailboxesShown, func() {
		ui.SidebarItem(c, "inbox", inboxIcon, "Inbox").Children(func() {
			ui.Badge(c, fmt.Sprint(app.unread))
		})
		ui.SidebarItem(c, "sent", sentIcon, "Sent")
	})
	ui.SidebarSection(c, "Tags", nil, func() {
		ui.SidebarItem(c, "work", nil, "Work")
	})
}).Width(220)
```

- `ui.SidebarItem` 选中它自己的 ID，在图标之后显示标签，没有图标时传 nil；用 `Children` 向它添加内容，如[徽章](badge.md)。
- `ui.SidebarSection` 在 `*bool` 为 true 时，将回调函数构建的项显示在标题下：单击标题或箭头可展开或收起这些项；指针悬停在标题上时才显示箭头。`*bool` 为 nil 时，该节始终展开。

## 选择

单击可选中列表项；侧边栏获得焦点后，可用 Up、Down、Home、End 或输入项的首字母来选择，Tab 则移出侧边栏，整个侧边栏只占一个 Tab 停留点。`Changed` 报告选择变化。侧边栏有焦点时，选中项以强调色显示，否则显示为灰色，与 AppKit 的侧边栏一致。

配合[路由器](navigation.md)使用时，在应用状态中保存 `*ui.Router`，将项的 ID 设为路径，选择变化时将对应页面推入导航栈：

```go
page := app.router.Path()
if ui.Sidebar(c, &page, func() {
	ui.SidebarItem(c, "/notes", nil, "Notes")
	ui.SidebarItem(c, "/files", nil, "Files")
}).Width(220).Changed() {
	app.router.Push(page)
}
```

## 辅助功能

辅助技术将其识别为树：各节标题是可展开和收起的节点，包含该节的列表项；侧边栏有焦点时，焦点位于选中项。
