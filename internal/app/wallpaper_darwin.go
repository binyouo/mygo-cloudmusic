//go:build darwin

package app

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type wallpaperNative struct {
	window       objc.ID
	cursorHidden bool
	closed       bool
}

// A borderless window covers the current display in its existing Space.
// Size it before its first paint; the transition animates opacity, never geometry.
func newWallpaperNative(win, main *mygo.Window) *wallpaperNative {
	n := &wallpaperNative{window: objc.ID(win.NativeHandle())}
	screen := objc.ID(main.NativeHandle()).Send(objc.RegisterName("screen"))
	n.window.Send(objc.RegisterName("setStyleMask:"), uint(1<<15))                   // full-size content, no title bar
	n.window.Send(objc.RegisterName("setLevel:"), 28)                                // above the menu bar, Dock and our notch
	n.window.Send(objc.RegisterName("setCollectionBehavior:"), uint(1<<4|1<<6|1<<8)) // stationary, out of cycle, full-screen auxiliary
	n.window.Send(objc.RegisterName("setHidesOnDeactivate:"), true)
	n.window.Send(objc.RegisterName("setAcceptsMouseMovedEvents:"), true)
	n.window.Send(objc.RegisterName("setAnimationBehavior:"), int64(2)) // no AppKit opening/closing effect
	if screen != 0 {
		n.window.Send(objc.RegisterName("setFrame:display:"), objc.Send[nsRect](screen, objc.RegisterName("frame")), false)
	}
	return n
}

func (n *wallpaperNative) fade(show bool, done func()) {
	if n.closed {
		return
	}
	duration, alpha := 0.42, 0.0
	if show {
		duration, alpha = 0.72, 1
		if !n.cursorHidden {
			objc.ID(objc.GetClass("NSCursor")).Send(objc.RegisterName("hide"))
			n.cursorHidden = true
		}
	} else {
		n.showCursor()
	}
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	if objc.Send[bool](workspace, objc.RegisterName("accessibilityDisplayShouldReduceMotion")) {
		duration = 0.16
	}
	if !objc.Send[bool](n.window, objc.RegisterName("isVisible")) {
		duration = 0
	}
	changes := objc.NewBlock(func(_ objc.Block, context objc.ID) {
		context.Send(objc.RegisterName("setDuration:"), duration)
		curve := objc.ID(objc.GetClass("CAMediaTimingFunction")).Send(objc.RegisterName("functionWithControlPoints::::"), float32(0.42), float32(0), float32(0.58), float32(1))
		context.Send(objc.RegisterName("setTimingFunction:"), curve)
		n.window.Send(objc.RegisterName("animator")).Send(objc.RegisterName("setAlphaValue:"), alpha)
	})
	complete := objc.NewBlock(func(_ objc.Block) {
		if !n.closed && done != nil {
			done()
		}
	})
	objc.ID(objc.GetClass("NSAnimationContext")).Send(objc.RegisterName("runAnimationGroup:completionHandler:"), changes, complete)
	changes.Release()
	complete.Release()
}

func (n *wallpaperNative) showCursor() {
	if n.cursorHidden {
		objc.ID(objc.GetClass("NSCursor")).Send(objc.RegisterName("unhide"))
		n.cursorHidden = false
	}
}

func (n *wallpaperNative) close() {
	n.closed = true
	n.showCursor()
}

func watchWallpaperInput(fn func(wake, pointer bool) bool) func() {
	const mask = uint64(1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<6 | 1<<7 | 1<<10 | 1<<11 | 1<<12 | 1<<22 | 1<<25 | 1<<26 | 1<<27 | 1<<29 | 1<<30)
	events := objc.ID(objc.GetClass("NSEvent"))
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		kind := objc.Send[uint64](event, objc.RegisterName("type"))
		// Key releases and modifier changes belong to the shortcut that just
		// opened the screen saver. Cmd-T itself is handled by the app's command.
		wake := kind != 11 && kind != 12 && kind != 2 && kind != 4 && kind != 26
		if kind == 10 && objc.Send[uint16](event, objc.RegisterName("keyCode")) == 17 &&
			objc.Send[uint64](event, objc.RegisterName("modifierFlags"))&(1<<20) != 0 {
			wake = false
		}
		if fn(wake, kind == 5) {
			return 0
		}
		return event
	})
	monitor := events.Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"), mask, block)
	monitor.Send(objc.RegisterName("retain"))
	return func() {
		if monitor != 0 {
			events.Send(objc.RegisterName("removeMonitor:"), monitor)
			monitor.Send(objc.RegisterName("release"))
		}
		block.Release()
	}
}
