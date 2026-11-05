//go:build darwin

package app

import (
	"math"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

type wallpaperNative struct {
	window objc.ID
	closed bool
}

// A borderless window covers the current display in its existing Space.
// Size it before its first paint; the transition animates opacity, never geometry.
func newWallpaperNative(win, main *mygo.Window) *wallpaperNative {
	n := &wallpaperNative{window: objc.ID(win.NativeHandle())}
	screen := objc.ID(main.NativeHandle()).Send(objc.RegisterName("screen"))
	n.window.Send(objc.RegisterName("setStyleMask:"), uint(1<<15))                     // full-size content, no title bar
	n.window.Send(objc.RegisterName("setLevel:"), 28)                                  // above the menu bar, Dock and our notch
	n.window.Send(objc.RegisterName("setCollectionBehavior:"), uint(1|1<<4|1<<6|1<<8)) // all Spaces, stationary, out of cycle, full-screen auxiliary
	n.window.Send(objc.RegisterName("setHidesOnDeactivate:"), false)
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

func (n *wallpaperNative) close() {
	n.closed = true
}

var wallpaperIdleAPI struct {
	once    sync.Once
	seconds func(int32, uint32) float64
}

func wallpaperSystemIdle() time.Duration {
	wallpaperIdleAPI.once.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		address, err := purego.Dlsym(lib, "CGEventSourceSecondsSinceLastEventType")
		if err == nil {
			purego.RegisterFunc(&wallpaperIdleAPI.seconds, address)
		}
	})
	if wallpaperIdleAPI.seconds == nil {
		return -1
	}
	seconds := wallpaperIdleAPI.seconds(0, ^uint32(0)) // combined session, any hardware input
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return -1
	}
	return time.Duration(min(seconds, float64(24*time.Hour/time.Second)) * float64(time.Second))
}
