//go:build !darwin

package app

type menuBarNative struct{}

func newMenuBarNative(*app) *menuBarNative { return nil }
func (*menuBarNative) update(menuBarState) {}
func (*menuBarNative) destroy()            {}
func (*menuBarNative) animate(menuBarLine) {}
