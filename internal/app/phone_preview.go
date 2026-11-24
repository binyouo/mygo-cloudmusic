package app

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

func setPhonePreviewPaths(root string) error {
	for name, leaf := range map[mygo.PathName]string{
		mygo.PathUserData: "data", mygo.PathCache: "cache", mygo.PathLogs: "logs",
		mygo.PathDownloads: "downloads", mygo.PathTemp: "tmp",
	} {
		dir := filepath.Join(root, leaf)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		mygo.App.SetPath(name, dir)
	}
	// Route Go and subprocess scratch files to the preview disk too.
	return os.Setenv("TMPDIR", filepath.Join(root, "tmp"))
}

// placePhonePreview only opens the preview when it fits on a secondary display.
func placePhonePreview(window *mygo.WindowOptions) error {
	for _, display := range mygo.Screen.Displays() {
		area := display.WorkArea
		if display.Primary || area.Width < window.Width || area.Height < 550 {
			continue
		}
		window.Title = "MyGo 云音乐 · iOS 预览"
		window.Height = min(window.Height, area.Height-48)
		window.X = area.X + (area.Width-window.Width)/2
		window.Y = area.Y + (area.Height-window.Height)/2
		return nil
	}
	return errors.New("no secondary display can fit the iOS preview")
}
