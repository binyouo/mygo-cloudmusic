# Token field

<!-- preview token-field 260 -->

`ui.TokenField` creates a field of tokens, as of tags or the recipients of
a mail, editing a `*[]string`: the tokens show as chips, each with a button
taking it out, before an input where Enter or a comma adds what was typed,
and Backspace in it while empty takes out the last. The suggestions
containing what was typed show below it, those not already tokens: Up and
Down move among them, and Enter or a click adds one. `Changed` reports a
change.

```go
ui.TokenField(c, &app.tags, allTags).Label("Tags")
```

Chips wrap onto more lines as they fill the field. `Placeholder` shows a
text in its input while it is empty, as "Add a tag…"; it shows none
without one.

## Suggestions as the user types

`Query` binds a `*string` to the text being typed, for suggestions the app
looks up for it, as a mail's recipients in an address book on a server:
the field writes the text there as `Query` is called, after the field
took what was typed, and clears it, as it clears the input, when a token
is added. What an input method is composing comes once committed, as in a
text input's value. `Changed`, asked after `Query`, reports a change of the
text too, as it does of the tokens. The app passes what it finds as the
suggestions of a later build, and matches them itself: the field shows
them as given, in their order, leaving out only those already tokens,
rather than those containing the text, so that a lookup that folds
accents can suggest Émile for "emile". They show until the app passes
others.

To look up only once the user pauses in typing, note when `Changed`
reported a change and, until the lookup, ask `c.After` for a frame at the
end of the pause, in every frame: each frame asks again for the frames it
needs.

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

Writing the string sets the text in the input, as setting a text input's
value does, without a change to report: an app starting from a draft sets
it, and one adding a token itself clears it.

## Accessibility

Assistive technology sees the field named by its `Label`, its input as a
combo box whose suggestions complete what was typed
(`aria-autocomplete="list"`), or a text field when it has none and no
`Query`, and each chip's button, named "Remove" and its token.
