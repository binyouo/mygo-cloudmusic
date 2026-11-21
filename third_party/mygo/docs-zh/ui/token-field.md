# 标记字段

<!-- preview token-field 260 -->

`ui.TokenField` 创建多项输入框，用于编辑 `*[]string`，例如标签或邮件收件人：每项显示为带移除按钮的小标签（chip），末尾是输入框。按 Enter 或输入逗号会添加当前输入的内容，输入框为空时按 Backspace 会移除最后一项。输入框下方会显示包含当前输入内容的建议，排除已添加的项；用 Up 和 Down 选择建议，按 Enter 或单击即可添加。`Changed` 报告变化。

```go
ui.TokenField(c, &app.tags, allTags).Label("Tags")
```

小标签排满一行后会自动换行。`Placeholder` 在输入框为空时显示提示文本，例如“Add a tag…”；未设置时则不显示。

## 随输入给出建议

`Query` 把一个 `*string` 绑定（binding）到正在输入的文本，供应用据此查找建议，例如在服务器上的通讯录中查找邮件收件人。调用 `Query` 时，字段已经处理完用户的输入，会把文本写入其中；添加一项时，它会像清空输入框一样清空这段文本。输入法正在组合的内容要等提交后才会写入，与文本输入的值一样。在 `Query` 之后调用 `Changed`，它也会报告文本的变化，就像报告各项的变化一样。应用把查到的结果作为之后某次构建的建议传入，并由应用自己负责匹配：字段按给出的顺序原样显示这些建议，只排除已添加的项，而不是只显示包含该文本的那些。这样，忽略重音的查找就可以为“emile”给出 Émile。在应用传入新的建议之前，字段会一直显示它们。

要等用户停止输入片刻后再查找，就记下 `Changed` 报告变化的时间，并在查找之前的每一帧里用 `c.After` 请求停顿结束时的一帧：每一帧都要重新请求它需要的帧。

```go
const pause = 300 * time.Millisecond

to := ui.TokenField(c, &app.to, app.found).Label("To").Query(&app.query)
if to.Changed() {
	app.typedAt = c.Now()
}
if app.query != app.asked {
	if wait := pause - c.Now().Sub(app.typedAt); wait > 0 {
		c.After(wait) // still typing
	} else {
		app.asked = app.query
		q := app.query
		go func() {
			found := app.book.Suggest(q) // shown as given: "emile" finds Émile
			win.Update(func() {
				if q == app.asked { // no newer lookup started since
					app.found = found
				}
			})
		}()
	}
}
```

写入这个字符串会设置输入框中的文本，与设置文本输入的值一样，不会报告变化：从草稿开始的应用可以设置它，自己添加了一项的应用可以清空它。

## 辅助功能

辅助技术将整个控件识别为由 `Label` 命名的输入区域：有建议或设置了 `Query` 时，内部输入框是提供列表补全的组合框（`aria-autocomplete="list"`），否则是普通文本框。每个小标签的移除按钮也可单独访问，名称由“Remove”和该项文本组成。
