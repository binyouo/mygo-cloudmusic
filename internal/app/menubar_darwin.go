//go:build darwin

package app

import (
	"embed"
	"math"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// One status item keeps the lyric and controls together when moved with
// Cmd-drag. AppKit supplies the menu bar's appearance and accessibility.
type menuBarNative struct {
	item, target, label, clip objc.ID
	prev, play, next, like    objc.ID
	buttons                   []objc.ID
	// width is how wide the lyric shows, from the settings, and overflow
	// how much of the line is wider than it and scrolls.
	width, overflow float64
	text            string
	playing         bool
}

var menuBarActions = map[objc.ID]func(){}
var menuBarTargetClass objc.Class

// Explicit vector bounds keep the small transport icons and the round
// brand mark at the same optical sizes as the reference menu bar.
//
//go:embed assets/menubar/*.svg
var menuBarIconFiles embed.FS

// The normal and smaller pressed templates live with the app.
var menuBarIconImages = map[string]struct{ normal, pressed objc.ID }{}

// The final button already has half a hit target of trailing space. Extra
// outer padding made a visible gap before the neighbouring status item.
const menuBarButtonWidth, menuBarLeading = 22.0, 4.0

func menuBarString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
}

func newMenuBarNative(a *app) *menuBarNative {
	if menuBarTargetClass == 0 {
		var err error
		menuBarTargetClass, err = objc.RegisterClass("CloudMusicMenuBarTarget", objc.GetClass("NSObject"), nil, nil,
			[]objc.MethodDef{{
				Cmd: objc.RegisterName("activate:"),
				Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
					if action := menuBarActions[sender]; action != nil {
						action()
					}
				},
			}})
		if err != nil {
			panic(err)
		}
	}
	b := &menuBarNative{}
	b.target = objc.ID(menuBarTargetClass).Send(objc.RegisterName("new"))
	bar := objc.ID(objc.GetClass("NSStatusBar")).Send(objc.RegisterName("systemStatusBar"))
	const buttonWidth, padding = menuBarButtonWidth, menuBarLeading
	b.width = min(300.0, max(100.0, a.settings.MenuBarWidth))
	textWidth := b.width
	b.item = bar.Send(objc.RegisterName("statusItemWithLength:"), textWidth+5*buttonWidth+padding)
	b.item.Send(objc.RegisterName("retain"))
	b.item.Send(objc.RegisterName("setAutosaveName:"), menuBarString("cloudmusic.lyrics"))
	root := b.item.Send(objc.RegisterName("button"))
	// Expose each child button to VoiceOver, rather than a single opaque
	// status button. Its children inherit the bar's effective appearance.
	root.Send(objc.RegisterName("setAccessibilityElement:"), false)
	// A blank title still supplies the native text height. An empty
	// status button collapses on width changes because its children do
	// not contribute to AppKit's intrinsic content size.
	root.Send(objc.RegisterName("setTitle:"), menuBarString(" "))
	height := objc.Send[nsRect](root, objc.RegisterName("bounds")).Size.H
	font := objc.ID(objc.GetClass("NSFont")).Send(objc.RegisterName("systemFontOfSize:weight:"), 13.0, 0.23)
	x := padding
	button := func(width float64, label, symbol string, action func()) objc.ID {
		v := objc.ID(objc.GetClass("NSButton")).Send(objc.RegisterName("alloc"))
		v = v.Send(objc.RegisterName("initWithFrame:"), nsRect{nsPoint{x, 0}, nsSize{width, height}})
		x += width
		v.Send(objc.RegisterName("setBordered:"), false)
		v.Send(objc.RegisterName("setButtonType:"), uint(0)) // momentary light
		v.Send(objc.RegisterName("setFont:"), font)
		v.Send(objc.RegisterName("setTitle:"), menuBarString(""))
		v.Send(objc.RegisterName("setAutoresizingMask:"), uint(16)) // follow the status bar height
		v.Send(objc.RegisterName("setTarget:"), b.target)
		v.Send(objc.RegisterName("setAction:"), objc.RegisterName("activate:"))
		v.Send(objc.RegisterName("setAccessibilityLabel:"), menuBarString(label))
		v.Send(objc.RegisterName("setToolTip:"), menuBarString(label))
		if symbol != "" {
			// Swap only the image while pressed. Native tracking restores it
			// on release or cancellation, with no hover bezel or tint change.
			v.Send(objc.RegisterName("setButtonType:"), uint(5)) // momentary change
			menuBarSymbol(v, symbol, label)
		}
		menuBarActions[v] = func() { action(); a.redraw() }
		root.Send(objc.RegisterName("addSubview:"), v)
		v.Send(objc.RegisterName("release")) // root owns it
		b.buttons = append(b.buttons, v)
		return v
	}
	b.label = button(textWidth, "打开正在播放", "", func() {
		a.nowPlaying = a.pl.Current() != nil
		a.showMain()
	})
	// Clip the full-width text button, not its title. NSClipView keeps
	// overflowing glyphs out of the playback controls without ellipsis.
	b.label.Send(objc.RegisterName("retain"))
	b.label.Send(objc.RegisterName("removeFromSuperview"))
	b.clip = objc.ID(objc.GetClass("NSClipView")).Send(objc.RegisterName("alloc"))
	b.clip = b.clip.Send(objc.RegisterName("initWithFrame:"), nsRect{nsPoint{padding, 0}, nsSize{textWidth, height}})
	b.clip.Send(objc.RegisterName("setDrawsBackground:"), false)
	b.clip.Send(objc.RegisterName("setAutoresizingMask:"), uint(16))
	b.clip.Send(objc.RegisterName("setDocumentView:"), b.label)
	root.Send(objc.RegisterName("addSubview:"), b.clip)
	b.clip.Send(objc.RegisterName("setWantsLayer:"), true)
	b.clip.Send(objc.RegisterName("release"))
	b.label.Send(objc.RegisterName("release"))
	b.label.Send(objc.RegisterName("setAlignment:"), uint(1))     // centered
	b.label.Send(objc.RegisterName("setLineBreakMode:"), uint(2)) // clip, never truncate
	b.label.Send(objc.RegisterName("cell")).Send(objc.RegisterName("setWraps:"), false)
	// Keep the text in its own backing layer so the compositor can move
	// the rendered glyphs without redrawing an NSButton on every frame.
	b.label.Send(objc.RegisterName("setWantsLayer:"), true)
	b.clip.Send(objc.RegisterName("layer")).Send(objc.RegisterName("setMasksToBounds:"), true)
	b.prev = button(buttonWidth, "上一首", "previous", a.pl.Prev)
	b.play = button(buttonWidth, "播放", "play", a.pl.TogglePlay)
	b.next = button(buttonWidth, "下一首", "next", a.pl.Next)
	b.like = button(buttonWidth, "喜欢", "heart", a.likeCurrent)
	button(buttonWidth, "打开"+mygo.App.Name(), "cloudmusic", a.showMain)
	return b
}

func menuBarSymbol(button objc.ID, symbol, label string) {
	icon := menuBarIconImages[symbol]
	if icon.normal == 0 {
		data, err := menuBarIconFiles.ReadFile("assets/menubar/" + symbol + ".svg")
		if err != nil {
			panic(err)
		}
		nsData := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), &data[0], uint(len(data)))
		icon.normal = objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), nsData)
		icon.normal.Send(objc.RegisterName("setTemplate:"), true)
		icon.pressed = icon.normal.Send(objc.RegisterName("copy"))
		size := objc.Send[nsSize](icon.normal, objc.RegisterName("size"))
		icon.pressed.Send(objc.RegisterName("setSize:"), nsSize{size.W * 0.85, size.H * 0.85})
		menuBarIconImages[symbol] = icon
	}
	button.Send(objc.RegisterName("setImagePosition:"), uint(1)) // image only, centered
	button.Send(objc.RegisterName("setImage:"), icon.normal)
	button.Send(objc.RegisterName("setAlternateImage:"), icon.pressed)
	button.Send(objc.RegisterName("setAccessibilityLabel:"), menuBarString(label))
	button.Send(objc.RegisterName("setToolTip:"), menuBarString(label))
}

func (b *menuBarNative) update(s menuBarState) {
	if b.text != s.text || b.width != s.width {
		b.text, b.width = s.text, s.width
		b.layout()
	}
	b.playing = s.playing
	b.label.Send(objc.RegisterName("setToolTip:"), menuBarString(s.tip))
	b.label.Send(objc.RegisterName("setAccessibilityLabel:"), menuBarString(s.text+"，打开正在播放"))
	for _, button := range []objc.ID{b.prev, b.play, b.next, b.like} {
		button.Send(objc.RegisterName("setEnabled:"), s.hasTrack)
	}
	if s.playing {
		menuBarSymbol(b.play, "pause", "暂停")
	} else {
		menuBarSymbol(b.play, "play", "播放")
	}
	if s.liked {
		menuBarSymbol(b.like, "heart-filled", "取消喜欢")
	} else {
		menuBarSymbol(b.like, "heart", "喜欢")
	}
}

// layout gives every control the same vertical bounds. The viewport keeps the
// width set, whatever the line, so the status item does not resize as the
// lines change: a short line sits centered in it, and only a longer one
// scrolls.
func (b *menuBarNative) layout() {
	const padding, buttonWidth = menuBarLeading, menuBarButtonWidth
	root := b.item.Send(objc.RegisterName("button"))
	height := objc.Send[nsRect](root, objc.RegisterName("bounds")).Size.H
	b.label.Send(objc.RegisterName("setTitle:"), menuBarString(b.text))
	// The cell measures the title as it lays it out, where cellSize would
	// still hold the previous title's size.
	cell := b.label.Send(objc.RegisterName("cell"))
	size := objc.Send[nsSize](cell, objc.RegisterName("cellSizeForBounds:"), nsRect{nsPoint{}, nsSize{10000, height}})
	textWidth := math.Ceil(size.W)
	b.overflow = menuBarOverflow(b.width, textWidth)

	length := b.width + 5*buttonWidth + padding
	if objc.Send[float64](b.item, objc.RegisterName("length")) != length {
		b.item.Send(objc.RegisterName("setLength:"), length)
	}
	b.clip.Send(objc.RegisterName("setFrame:"), nsRect{nsPoint{padding, 0}, nsSize{b.width, height}})
	b.label.Send(objc.RegisterName("setFrame:"), nsRect{nsPoint{}, nsSize{max(b.width, textWidth), height}})
	b.clip.Send(objc.RegisterName("scrollToPoint:"), nsPoint{})
	for i, button := range b.buttons[1:] {
		button.Send(objc.RegisterName("setFrame:"), nsRect{nsPoint{padding + b.width + float64(i)*buttonWidth, 0}, nsSize{buttonWidth, height}})
	}
}

func (b *menuBarNative) animate(line menuBarLine) {
	// Move the clip's sublayers, not the label's view-owned transform:
	// AppKit resets that transform during layout, snapping the text back.
	layer := b.clip.Send(objc.RegisterName("layer"))
	key := menuBarString("lyrics.scroll")
	path := menuBarString("sublayerTransform.translation.x")
	transaction := objc.ID(objc.GetClass("CATransaction"))
	transaction.Send(objc.RegisterName("begin"))
	transaction.Send(objc.RegisterName("setDisableActions:"), true)
	defer transaction.Send(objc.RegisterName("commit"))
	layer.Send(objc.RegisterName("removeAnimationForKey:"), key)
	offset := menuBarScrollOffset(b.overflow, b.width+b.overflow, line.elapsed, line.dur)
	number := func(v float64) objc.ID {
		return objc.ID(objc.GetClass("NSNumber")).Send(objc.RegisterName("numberWithDouble:"), v)
	}
	setOffset := func(v float64) { layer.Send(objc.RegisterName("setValue:forKeyPath:"), number(-v), path) }
	setOffset(offset)
	if b.overflow <= 0 || !b.playing {
		return
	}
	wait, moving := menuBarScrollRemaining(b.overflow, b.width+b.overflow, line.elapsed, line.dur)
	if moving <= 0 {
		return
	}
	array := func(values ...float64) objc.ID {
		a := objc.ID(objc.GetClass("NSMutableArray")).Send(objc.RegisterName("arrayWithCapacity:"), uint(len(values)))
		for _, v := range values {
			a.Send(objc.RegisterName("addObject:"), number(v))
		}
		return a
	}
	animation := objc.ID(objc.GetClass("CAKeyframeAnimation")).Send(objc.RegisterName("animationWithKeyPath:"), path)
	if wait > 0 {
		animation.Send(objc.RegisterName("setValues:"), array(-offset, -offset, -b.overflow))
		animation.Send(objc.RegisterName("setKeyTimes:"), array(0, float64(wait)/float64(wait+moving), 1))
	} else {
		animation.Send(objc.RegisterName("setValues:"), array(-offset, -b.overflow))
		animation.Send(objc.RegisterName("setKeyTimes:"), array(0, 1))
	}
	animation.Send(objc.RegisterName("setDuration:"), (wait + moving).Seconds())
	animation.Send(objc.RegisterName("setCalculationMode:"), menuBarString("linear"))
	// The model holds the final position after the finite animation is
	// removed. Pausing or seeking replaces it with the exact player offset.
	setOffset(b.overflow)
	layer.Send(objc.RegisterName("addAnimation:forKey:"), animation, key)
}

func (b *menuBarNative) destroy() {
	for _, button := range b.buttons {
		delete(menuBarActions, button)
	}
	bar := objc.ID(objc.GetClass("NSStatusBar")).Send(objc.RegisterName("systemStatusBar"))
	// Removing a named item clears AppKit's saved position. Detach its
	// autosave name first so quitting or toggling lyrics off preserves the
	// user's last Cmd-drag position for the next item with that name.
	b.item.Send(objc.RegisterName("setAutosaveName:"), objc.ID(0))
	bar.Send(objc.RegisterName("removeStatusItem:"), b.item)
	b.item.Send(objc.RegisterName("release"))
	b.target.Send(objc.RegisterName("release"))
}
