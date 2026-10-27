package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/localmedia"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// NM_FRAMES renders the imported sample's library, wallpaper and settings without starting or touching a running app.
func TestLocalWallpaperFrames(t *testing.T) {
	dir, sample := os.Getenv("NM_FRAMES"), os.Getenv("LOCAL_AUDIO_SAMPLE")
	if dir == "" || sample == "" {
		t.Skip("set NM_FRAMES and LOCAL_AUDIO_SAMPLE to render local music")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	track, err := localmedia.Read(sample, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settings = defaultSettings()
	a.localTracks = []netease.Track{track}
	a.route = route{page: pageLocal}
	a.pl.Cue(a.localTracks)
	a.pl.SetLyrics(lyrics.Build(track.LocalLyrics, "", ""))
	a.pl.Hold(30 * time.Second)
	bmp, cover, err := loadArt(track.Album.PicURL)
	if err != nil {
		t.Fatal(err)
	}
	art := &artwork{bmp: bmp, backdrop: ui.NewBitmap(blurImage(cover)), tint: dominantColor(cover), used: time.Now()}
	a.art.art[track.Album.PicURL] = art
	a.art.art[track.Album.PicURL+"#backdrop"] = art
	save := func(name string, tester *ui.Tester, advance time.Duration) {
		t.Helper()
		tester.Advance(advance)
		tester.Frame()
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tester.Image()); err != nil {
			t.Fatal(err)
		}
	}
	tester := ui.NewTester(a.view, 1440, 900)
	save("local-library", tester, 2*time.Second)
	tester.Key(ui.Cmd, ui.KeyT)
	saver := ui.NewTester(a.wallpaperView, 1728, 1117)
	save("wallpaper-enter-start", saver, 0)
	a.wallpaper.revealed = true
	save("wallpaper-enter-middle", saver, 300*time.Millisecond)
	save("local-wallpaper", saver, time.Second)
	a.wallpaper.revealed, a.wallpaper.closing = false, true
	save("wallpaper-exit-start", saver, 0)
	save("wallpaper-exit-middle", saver, 140*time.Millisecond)
	save("wallpaper-exit-end", saver, time.Second)
	a.finishWallpaper()
	settingsTester := ui.NewTester(a.settingsView, 560, 720)
	save("wallpaper-settings", settingsTester, 2*time.Second)
}
