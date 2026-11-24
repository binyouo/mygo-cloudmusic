//go:build ios

package appicon

import (
	"context"
	"time"

	"github.com/egoist/mygo"
)

// Dark is the name the dark icon's CFBundleAlternateIcons entry and its
// files in the bundle share.
const Dark = "AppIconDark"

// Current is the name of the icon the home screen shows, "" for the app's
// own.
func Current() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out struct{ Name string }
	err := mygo.Native("cloudmusic_icon").Call(ctx, "current", nil, &out)
	return out.Name, err
}

// Set makes the home screen show the icon named name, or the app's own
// for "".
func Set(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return mygo.Native("cloudmusic_icon").Call(ctx, "set", map[string]string{"Name": name}, nil)
}
