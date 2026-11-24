//go:build !ios

package appicon

import "errors"

// Dark is the name the dark icon's CFBundleAlternateIcons entry and its
// files in the bundle share.
const Dark = "AppIconDark"

var errOther = errors.New("app icons switch on iOS")

// Current is the name of the icon the home screen shows, "" for the app's
// own.
func Current() (string, error) { return "", errOther }

// Set makes the home screen show the icon named name, or the app's own
// for "".
func Set(string) error { return errOther }
