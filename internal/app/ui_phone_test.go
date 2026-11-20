package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

func tapPhone(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerUp(float32, float32)
	}); ok {
		box, found := tt.Find(label)
		if !found || box.W == 0 {
			// A sheet or player on the way up paints its targets under
			// the fold until the rise lands.
			tt.Advance(600 * time.Millisecond)
			tt.Frame()
			box, found = tt.Find(label)
		}
		if !found {
			t.Fatalf("missing tap target %q", label)
		}
		x, y := box.X+box.W/2, box.Y+box.H/2
		touch.FingerDown(x, y)
		touch.FingerUp(x, y)
		return
	}
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

func dragPhone(t *testing.T, tt *ui.Tester, x, y, dx, dy float32) {
	t.Helper()
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	touch.FingerDown(x, y)
	for step := 1; step <= 8; step++ {
		tt.Advance(24 * time.Millisecond)
		fraction := float32(step) / 8
		touch.FingerMove(x+fraction*dx, y+fraction*dy)
	}
	tt.Advance(150 * time.Millisecond)
	touch.FingerUp(x+dx, y+dy)
	tt.Advance(time.Second)
	tt.Frame()
}

// Closing the phone quality chooser keeps the current quality, and selecting a new one saves it without leaving Settings.
func TestPhoneQualityChoiceSavesSettings(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageSettings}
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.view, 320, 670)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Scroll(155, 450, 0, 160)
	tapPhone(t, tt, "选择音质")
	tapPhone(t, tt, "关闭音质")
	if a.settings.Quality != "exhigh" || a.phoneSetting != "" {
		t.Fatalf("closing the chooser left quality = %q, sheet = %q", a.settings.Quality, a.phoneSetting)
	}
	tapPhone(t, tt, "选择音质")
	tapPhone(t, tt, "无损（FLAC）")
	if a.settings.Quality != "lossless" || a.phoneSetting != "" || a.route.page != pageSettings {
		t.Fatalf("quality = %q, sheet = %q, route = %+v", a.settings.Quality, a.phoneSetting, a.route)
	}
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err := json.Unmarshal(data, &saved); err != nil || saved.Quality != "lossless" {
		t.Fatalf("saved quality = %q, read error = %v", saved.Quality, err)
	}
}

// A short pull returns to the player, and a longer pull restores the same page with playback still active.
func TestPhonePlayerPullToMiniPlayer(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.route = route{page: pageLibrary}
	a.phoneTab = pageLibrary
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000}})
	a.pl.Hold(34 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()

	dragPhone(t, tt, 195, 26, 0, 45)
	if !a.nowPlaying || a.phonePull.Y != 0 {
		t.Fatal("a short pull closed the player or left it offset")
	}
	dragPhone(t, tt, 195, 26, 0, 220)
	if a.nowPlaying || a.route.page != pageLibrary || a.pl.Current().ID != 31 ||
		a.pl.Position() != 34*time.Second || !a.pl.Playing() {
		t.Fatal("pulling down did not return to the same library page and keep the music playing")
	}
	tapPhone(t, tt, "打开正在播放")
	if !a.nowPlaying || a.phonePull.Y != 0 || a.phoneClosing {
		t.Fatal("the player reopened mid-dismiss")
	}
}

// The mini player pauses and skips, and tapping its cover opens the player.
func TestPhoneMiniPlayerControls(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 200000}, {ID: 22, Name: "第二首", Duration: 200000}})
	a.pl.Jump(0)
	a.pl.Hold(30 * time.Second)
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "播放/暂停")
	if a.pl.Playing() {
		t.Fatal("the mini player's pause did not stop the music")
	}
	tapPhone(t, tt, "下一首")
	if a.pl.Current().ID != 22 {
		t.Fatal("the mini player's next button did not skip a song")
	}
	tapPhone(t, tt, "迷你播放进度")
	if !a.nowPlaying {
		t.Fatal("tapping the mini player's cover did not open the player")
	}
}

// A sideways swipe on the mini player skips the song, as the iPhone's does, without opening the player.
func TestPhoneMiniPlayerSwipeTrack(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 200000}, {ID: 22, Name: "第二首", Duration: 200000}})
	a.pl.Jump(0)
	tt := ui.NewTester(a.view, 390, 780)
	tt.Frame()
	title, ok := tt.Find("打开正在播放")
	if !ok {
		t.Fatal("the mini player was not there to swipe")
	}
	x, y := title.X+title.W*0.6, title.Y+title.H/2
	dragPhone(t, tt, x, y, -120, 8)
	if a.pl.Current().ID != 22 || a.nowPlaying {
		t.Fatalf("a left swipe on the mini player did not skip to the next song: current=%v nowPlaying=%v", a.pl.Current().ID, a.nowPlaying)
	}
	dragPhone(t, tt, x, y, 120, 8)
	if a.pl.Current().ID != 11 || a.nowPlaying {
		t.Fatalf("a right swipe on the mini player did not go back a song: current=%v nowPlaying=%v", a.pl.Current().ID, a.nowPlaying)
	}
}

// A pull up on the mini player grows the player under the finger: a short one falls back, a long one opens.
func TestPhoneMiniPlayerPullUp(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 200000}})
	a.pl.Jump(0)
	tt := ui.NewTester(a.view, 390, 780)
	tt.Frame()
	title, ok := tt.Find("打开正在播放")
	if !ok {
		t.Fatal("the mini player was not there to pull")
	}
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	travel := a.phonePlatter.Y
	if travel <= 0 {
		t.Fatal("the mini player's capsule was not measured")
	}
	x, y := title.X+title.W*0.6, title.Y+title.H/2
	pull := func(dy float32) {
		touch.FingerDown(x, y)
		for step := 1; step <= 8; step++ {
			tt.Advance(24 * time.Millisecond)
			touch.FingerMove(x, y+dy*float32(step)/8)
		}
	}
	// Held just started up: the sheet is already growing out of the
	// capsule.
	pull(-0.05 * travel)
	tt.Frame()
	if !a.nowPlaying || a.phoneOpenPull <= 0 {
		t.Fatal("pulling up on the mini player did not start opening the player")
	}
	// Let go holding still first so there is no throw: it falls back to
	// the capsule.
	tt.Advance(150 * time.Millisecond)
	heldProgress := a.phoneMorph.to
	touch.FingerUp(x, y-0.05*travel)
	tt.Frame()
	if a.phoneMorph.from != heldProgress {
		t.Fatalf("cancelled open started from %v, held pose was %v", a.phoneMorph.from, heldProgress)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.nowPlaying {
		t.Fatal("a barely started pull did not fall back to the mini player")
	}
	// A quarter pull, Apple's small pull, opens without any throw.
	pull(-0.25 * travel)
	tt.Advance(150 * time.Millisecond)
	heldProgress = a.phoneMorph.to
	touch.FingerUp(x, y-0.25*travel)
	tt.Frame()
	if a.phoneMorph.from != heldProgress {
		t.Fatalf("committed open started from %v, held pose was %v", a.phoneMorph.from, heldProgress)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if !a.nowPlaying {
		t.Fatal("a quarter pull up did not open the player")
	}
}

// A pull down that starts on the cover sends the player home from where it let go, rather than snapping back first.
func TestPhonePlayerPullFromCover(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000}})
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	cover, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("the cover was not there to pull")
	}
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	x, y := cover.X+cover.W/2, cover.Y+cover.H/2
	touch.FingerDown(x, y)
	for step := 1; step <= 8; step++ {
		tt.Advance(24 * time.Millisecond)
		touch.FingerMove(x, y+float32(step)*30)
	}
	tt.Advance(150 * time.Millisecond)
	heldProgress := a.phoneMorph.to
	touch.FingerUp(x, y+240)
	tt.Frame()
	if !a.phoneClosing || a.phonePull.Y <= 0 {
		t.Fatal("a pull down on the cover did not start the player's trip home from where it was")
	}
	if a.phoneMorph.from != heldProgress || heldProgress <= 0 || heldProgress >= 1 {
		t.Fatalf("dismissal started from %v, held pose was %v", a.phoneMorph.from, heldProgress)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.nowPlaying {
		t.Fatal("the player stayed up after a pull down on the cover")
	}
}

// The mini title's marquee holds for Apple's opening beat, runs sideways
// at a steady rate, rests at the far end, and comes back — the timing
// MarqueeView's delay and scroll rate gave.
func TestPhoneMarqueeOffset(t *testing.T) {
	start := time.Unix(0, 0)
	const overflow = 60 // pt wider than the room it has
	at := func(d time.Duration) float32 {
		off, _ := phoneMarqueeOffset(start.Add(d), start, overflow)
		return off
	}
	if off := at(2 * time.Second); off != 0 {
		t.Fatalf("the title moved during its opening beat: %v", off)
	}
	// overflow 60pt at 30pt/s crosses in 2s, so 4s in (1s into the leg)
	// it is about halfway across.
	if off := at(4 * time.Second); off < 20 || off > 40 {
		t.Fatalf("mid-scroll offset = %v, want about half of %v", off, overflow)
	}
	// 5.5s in it rests at the far end.
	if off := at(5500 * time.Millisecond); off != overflow {
		t.Fatalf("far-end offset = %v, want %v", off, overflow)
	}
	// 7s in it is coming back, partway.
	if off := at(7 * time.Second); off <= 0 || off >= overflow {
		t.Fatalf("return-leg offset = %v, want between 0 and %v", off, overflow)
	}
	// 8.3s in it rests at the start again.
	if off := at(8300 * time.Millisecond); off != 0 {
		t.Fatalf("start-rest offset = %v, want 0", off)
	}
}

// A song title wider than the mini player scrolls instead of truncating:
// two frames taken while it moves differ in the title's strip.
func TestPhoneMiniTitleMarquee(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "一段实在是长到完全放不下的歌名标题用于跑马灯", Duration: 200000}})
	tt := ui.NewTester(a.view, 390, 780)
	tt.Frame()
	if a.phoneMarqueeTitle == "" {
		t.Fatal("the mini title's marquee clock never started")
	}
	// Park the playhead so nothing else in the dock is moving, then step
	// into the scroll and take two frames of the title strip.
	a.pl.Hold(0)
	tt.Advance(3400 * time.Millisecond) // past the 3s beat, into the leg
	tt.Frame()
	first := tt.Image()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	second := tt.Image()
	if a.phoneMarqueeTitle == "" {
		t.Fatal("the marquee clock reset mid-scroll")
	}
	// Only the title moves in the dock while the playhead is parked, so
	// any churn across the capsule's text column is the marquee at work.
	if !titleStripMoved(first, second) {
		t.Fatal("the long mini title did not scroll between frames")
	}
}

// titleStripMoved reports whether pixels in the mini player's title
// column changed between two frames. The capsule floats just above the
// tab bar; its text column runs from beside the ring to before the
// transport buttons.
func titleStripMoved(a, b image.Image) bool {
	ra, rb := a.Bounds(), b.Bounds()
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		return true
	}
	y0, y1 := ra.Dy()-150, ra.Dy()-80
	changed := 0
	for y := y0; y < y1; y++ {
		for x := 62; x < ra.Dx()-150; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			if absI(int(r1>>8)-int(r2>>8))+absI(int(g1>>8)-int(g2>>8))+absI(int(b1>>8)-int(b2>>8)) > 90 {
				changed++
			}
		}
	}
	return changed > 40
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// A sideways swipe on the cover skips the song, as the iPhone's player does, without opening another view.
func TestPhoneCoverSwipeSkips(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "第一首"}, {ID: 32, Name: "第二首"}})
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	cover, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("the cover was not available to swipe")
	}
	dragPhone(t, tt, cover.X+cover.W*0.82, cover.Y+cover.H/2, -cover.W*0.62, 18)
	if !a.nowPlaying || a.phoneDetails || a.pl.Current().ID != 32 {
		t.Fatal("swiping the cover left did not go to the next song")
	}
	tt.Frame()
	cover, ok = tt.Find("专辑封面")
	if !ok {
		t.Fatal("the cover was not available to swipe back")
	}
	dragPhone(t, tt, cover.X+cover.W*0.2, cover.Y+cover.H/2, cover.W*0.62, 18)
	if a.pl.Current().ID != 31 {
		t.Fatal("swiping the cover right did not go back to the previous song")
	}
}

// Pulling within overflowing lyrics scrolls the text, even past the player's dismissal distance.
func TestPhoneLyricsScrollStaysInPlayer(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 360000}})
	lines := ""
	for i := range 18 {
		lines += fmt.Sprintf("[%02d:%02d]第 %d 句歌词\n", i/6, i%6*10, i+1)
	}
	a.pl.SetLyrics(lyrics.Build(lines, "", ""))
	a.pl.Hold(105 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌词")
	before := a.lyricScroll.Y
	dragPhone(t, tt, 195, 145, 0, 220)
	if !a.nowPlaying || !a.phoneDetails || a.lyricScroll.Y >= before {
		t.Fatalf("pulling down over lyrics left scroll at %.0f (was %.0f) or closed the player", a.lyricScroll.Y, before)
	}
}

// TestPhoneLibraryBack keeps the library tab selected when returning from a detail page.
func TestPhoneLibraryBack(t *testing.T) {
	a := newTestApp()
	a.phone = true
	tt := ui.NewTester(a.view, 390, 780)
	for _, action := range []struct {
		label string
		want  pageKind
	}{
		{"我的", pageLibrary},
		{"本地音乐", pageLocal},
		{"返回", pageLibrary},
	} {
		if err := tt.Click(action.label); err != nil {
			t.Fatal(err)
		}
		if a.route.page != action.want {
			t.Fatalf("%s opened page %v, want %v", action.label, a.route.page, action.want)
		}
		tt.Advance(500 * time.Millisecond)
	}
	if a.phoneTab != pageLibrary || len(a.history) != 0 || len(a.searches) != 0 {
		t.Fatal("the tab switch reused navigation history or made a search request")
	}
}

// TestPhoneQueueJump changes the shared playback queue from the phone player.
func TestPhoneQueueJump(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首"}, {ID: 22, Name: "第二首"}})
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	for _, label := range []string{"打开正在播放", "待播清单", "第二首", "关闭待播清单", "关闭正在播放"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if tr := a.pl.Current(); tr == nil || tr.ID != 22 || a.nowPlaying {
		t.Fatalf("closing the player lost the selected track or left the player open: %v", tr)
	}
}

// The playback sheet keeps its close button and its scrolled queue above the system insets.
func TestPhoneQueueKeepsActionsInSafeArea(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	tracks := make([]netease.Track, 24)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("测试曲目 %d", i+1)}
	}
	a.pl.Cue(tracks)
	const width, height = 390, 780
	const right, bottom = 46, 64
	tt := ui.NewTester(a.view, width, height)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tester, ok := any(tt).(interface {
		SetInsets(float32, float32, float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no mobile insets")
	}
	tester.SetInsets(46, right, bottom, 32)
	if err := tt.Click("待播清单"); err != nil {
		t.Fatal(err)
	}
	closeLabel := "关闭待播清单"
	if box, ok := tt.Find(closeLabel); !ok || box.X+box.W > width-right {
		t.Fatalf("close button crosses the safe right edge: %+v", box)
	}
	tt.Scroll(170, 600, 0, 5000)
	last := tracks[len(tracks)-1]
	if box, ok := tt.Find(last.Name); !ok || box.Y+box.H > height-bottom {
		t.Fatalf("last queue row crosses the safe bottom edge: %+v", box)
	}
	if err := tt.Click(last.Name); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(closeLabel); err != nil {
		t.Fatal(err)
	}
	if a.pl.Current().ID != last.ID || a.phoneSheet != phoneSheetClosed {
		t.Fatal("queued track was not selected or sheet did not close")
	}
}

// The cover grid opens at the playing row and can return there after browsing elsewhere without restarting playback.
func TestPhoneCoverJump(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	tracks := make([]netease.Track, 66)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("待播 %02d", i+1), Duration: 90000, LocalPath: "/missing/music.flac"}
	}
	a.pl.Cue(tracks)
	a.pl.Jump(36)
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	for _, label := range []string{"待播清单", "封面网格", "选歌：待播 38"} {
		tapPhone(t, tt, label)
	}
	if tr := a.pl.Current(); tr == nil || tr.ID != 38 || len(a.pl.Queue()) != len(tracks) {
		t.Fatalf("cover selection changed the queue or chose %v", tr)
	}
	a.pl.Seek(33 * time.Second)
	tt.Scroll(190, 480, 0, -6000)
	if box, visible := tt.Find("选歌：待播 38"); visible && box.W > 0 && box.H > 0 {
		t.Fatal("browsing did not leave the playing row")
	}
	tapPhone(t, tt, "定位当前歌曲")
	tapPhone(t, tt, "选歌：待播 38")
	if a.pl.Position() != 33*time.Second {
		t.Fatalf("tapping the current cover restarted at %v", a.pl.Position())
	}
	tt.SetSize(844, 390)
	tapPhone(t, tt, "选歌：待播 39")
	if tr := a.pl.Current(); tr == nil || tr.ID != 39 {
		t.Fatalf("after rotation, cover selection chose %v", tr)
	}
}

// TestPhoneTabsRestoreDetail returns to the prior detail and its back stack after changing tabs.
func TestPhoneTabsRestoreDetail(t *testing.T) {
	a := newTestApp()
	a.phone = true
	tt := ui.NewTester(a.view, 390, 780)
	for _, label := range []string{"我的", "本地音乐", "播客", "我的"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.route.page != pageLocal || a.phoneTab != pageLibrary {
		t.Fatalf("reopened tab = %v, page = %v", a.phoneTab, a.route.page)
	}
	tt.Advance(500 * time.Millisecond)
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	if a.route.page != pageLibrary {
		t.Fatalf("back returned to page %v, want library", a.route.page)
	}
}

// TestPhoneSongActionsQueueNext queues the row from its visible actions without starting that track.
func TestPhoneSongActionsQueueNext(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pagePlaylist, id: 42}
	tracks := []netease.Track{{ID: 11, Name: "第一首"}, {ID: 22, Name: "第二首"}, {ID: 33, Name: "第三首"}}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单", Tracks: tracks}
	a.pl.Cue(tracks)
	tt := ui.NewTester(a.view, 390, 780)
	// The taller detail header leaves the last row under the dock: scroll
	// it clear of the mini player before tapping its actions.
	tt.Scroll(195, 400, 0, 300)
	tapPhone(t, tt, "歌曲操作：第三首")
	tapPhone(t, tt, "下一首播放")
	order, at := a.pl.PlayOrder()
	if a.pl.Current().ID != 11 || at+1 >= len(order) || a.pl.Queue()[order[at+1]].ID != 33 {
		t.Fatalf("current track = %v, queue = %v, position = %d; want first track with third queued next", a.pl.Current(), order, at)
	}
}

// Closing the track sheet returns to the queue, and scheduling from it keeps the current song playing.
func TestPhoneQueueActionsKeepPlaying(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	tracks := []netease.Track{{ID: 11, Name: "第一首"}, {ID: 22, Name: "第二首"}, {ID: 33, Name: "第三首"}}
	a.pl.Cue(tracks)
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "待播清单")
	tapPhone(t, tt, "歌曲操作：第三首")
	tapPhone(t, tt, "关闭歌曲操作")
	if a.phoneTrackTarget != nil || a.phoneSheet != phoneSheetQueue {
		t.Fatalf("track sheet = %v, player sheet = %v; want only the queue", a.phoneTrackTarget, a.phoneSheet)
	}
	tapPhone(t, tt, "歌曲操作：第三首")
	tapPhone(t, tt, "下一首播放")
	order, at := a.pl.PlayOrder()
	if a.phoneSheet != phoneSheetQueue || a.pl.Current().ID != 11 || at+1 >= len(order) || a.pl.Queue()[order[at+1]].ID != 33 {
		t.Fatalf("queue sheet = %v, current track = %v, play order = %v at %d", a.phoneSheet, a.pl.Current(), order, at)
	}
}

// The library playlist sheet opens its rename dialog without navigating to the playlist.
func TestPhoneLibraryPlaylistActions(t *testing.T) {
	a := editTestApp()
	a.phone = true
	a.route = route{page: pageLibrary}
	tt := ui.NewTester(a.view, 320, 670)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌单操作：通勤")
	tapPhone(t, tt, "重命名…")
	if !a.naming.open || a.naming.rename != 10 || a.route.page != pageLibrary || a.phonePlaylistTarget != 0 {
		t.Fatalf("rename dialog = %+v, route = %+v", a.naming, a.route)
	}
	tapPhone(t, tt, "取消")
	tapPhone(t, tt, "通勤")
	if a.naming.open || a.route != (route{page: pagePlaylist, id: 10}) {
		t.Fatalf("rename remained open or the row did not navigate: %+v", a.route)
	}
}

// Deleting from a phone playlist page asks for confirmation and then returns to the library.
func TestPhonePlaylistDetailsConfirmDelete(t *testing.T) {
	a := editTestApp()
	a.phone = true
	a.phoneTab = pageLibrary
	a.history = []route{{page: pageLibrary}}
	ids := make(chan int64, 1)
	old := deletePlaylist
	deletePlaylist = func(id int64) error { ids <- id; return nil }
	defer func() { deletePlaylist = old }()

	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌单操作：通勤")
	tapPhone(t, tt, "删除歌单…")
	if !a.deleting.open || a.deleting.id != 10 || a.editable(10) == nil || a.phonePlaylistTarget != 0 {
		t.Fatalf("playlist confirmation = %+v, open sheet target = %d", a.deleting, a.phonePlaylistTarget)
	}
	tapPhone(t, tt, "删除")
	if id := wait(t, ids, "the playlist deletion"); id != 10 || a.editable(10) != nil || a.route.page != pageLibrary {
		t.Fatalf("deleted %d, editable playlist = %v, route = %+v", id, a.editable(10), a.route)
	}
}

// Unsaving from a phone playlist sheet clears its saved status without navigating to it.
func TestPhoneSavedPlaylistActionsUnsave(t *testing.T) {
	a := editTestApp()
	a.phone = true
	a.route = route{page: pageLibrary}
	a.saved[0].Subscribed = true
	a.playlists[20] = &netease.Playlist{ID: 20, Name: "飙升榜", Subscribed: true}
	calls := make(chan saveCall, 1)
	old := savePlaylist
	savePlaylist = func(id int64, save bool) error { calls <- saveCall{id, save}; return nil }
	defer func() { savePlaylist = old }()

	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "收藏歌单")
	tapPhone(t, tt, "歌单操作：飙升榜")
	tapPhone(t, tt, "取消收藏")
	if call := nextSave(t, calls); call != (saveCall{20, false}) || a.isSaved(a.playlists[20]) || a.route.page != pageLibrary {
		t.Fatalf("playlist save request = %+v, saved playlists = %v, route = %+v", call, a.saved, a.route)
	}
}

// The episode sheet sends a like for the episode without starting its audio.
func TestPhoneEpisodeMenuLikesEpisode(t *testing.T) {
	a := catalogApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 1}
	a.route = route{page: pagePodcast, id: 5}
	ep := a.podcasts[5].programs[0]
	liked := make(chan int64, 1)
	old := likeProgram
	likeProgram = func(id int64, like bool) error {
		if like {
			liked <- id
		}
		return nil
	}
	defer func() { likeProgram = old }()
	tt := ui.NewTester(a.view, 390, 780)
	tapPhone(t, tt, "节目操作："+ep.Name)
	tapPhone(t, tt, "喜欢节目")
	if id := wait(t, liked, "the episode like"); id != ep.ID || !a.likedPrograms[id] || a.pl.Current() != nil {
		t.Fatalf("liked episode = %d, current track = %v", id, a.pl.Current())
	}
}

// Reading an episode from the list keeps playback untouched, even if that episode has no audio.
func TestPhoneEpisodeNotesFromList(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprintf("unavailable=%t", unavailable), func(t *testing.T) {
			a := catalogApp()
			a.phone = true
			a.route = route{page: pagePodcast, id: 5}
			ep := &a.podcasts[5].programs[0]
			ep.Description = "节目第一部分从这里开始。"
			if unavailable {
				ep.MainSong.ID = 0
			}
			tt := ui.NewTester(a.view, 390, 780)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			if unavailable {
				tapPhone(t, tt, ep.Name)
			} else {
				tapPhone(t, tt, "节目操作："+ep.Name)
				tapPhone(t, tt, "节目简介")
			}
			if a.phoneProgram == nil || a.phoneProgram.ProgramID != ep.ID || a.pl.Current() != nil || !tt.HasText(ep.Description) {
				t.Fatalf("program notes = %+v, current track = %v", a.phoneProgram, a.pl.Current())
			}
			tapPhone(t, tt, "关闭节目简介")
			if a.phoneProgram != nil || a.phoneTrackTarget != nil || a.route != (route{page: pagePodcast, id: 5}) {
				t.Fatalf("closing the notes left the wrong view: %+v, %+v", a.route, a.phoneProgram)
			}
		})
	}
}

// Advancing playback while reading one episode leaves those notes open until dismissed.
func TestPhoneEpisodeNotesStayOpenAcrossNext(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	first := netease.Track{ID: 7, ProgramID: 17, Name: "第一期", Description: "这是第一期的简介。", LocalPath: "/missing/episode.flac"}
	second := netease.Track{ID: 8, ProgramID: 18, Name: "第二期", Description: "这是第二期的简介。", LocalPath: "/missing/episode.flac"}
	a.pl.Cue([]netease.Track{first, second})
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "更多")
	tapPhone(t, tt, "节目简介")
	a.pl.Next()
	tt.Frame()
	if a.pl.Current().ProgramID != second.ProgramID || a.phoneProgram == nil || a.phoneProgram.ProgramID != first.ProgramID || !tt.HasText(first.Description) {
		t.Fatalf("playing %v, open notes = %+v", a.pl.Current(), a.phoneProgram)
	}
	tapPhone(t, tt, "关闭节目简介")
	if !a.nowPlaying || a.phoneSheet != phoneSheetClosed || a.phoneProgram != nil || a.pl.Current().ProgramID != second.ProgramID {
		t.Fatal("dismissing the notes did not return to the currently playing episode")
	}
}

// Picking a destination from the phone's sheet adds the selected song without leaving its source.
func TestPhoneAddToPlaylistPicker(t *testing.T) {
	a := editTestApp()
	a.phone = true
	type addition struct{ playlist, song int64 }
	calls := make(chan addition, 1)
	old := addToPlaylist
	addToPlaylist = func(id int64, songs ...int64) error {
		calls <- addition{id, songs[0]}
		return nil
	}
	defer func() { addToPlaylist = old }()
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌曲操作：宝船")
	tapPhone(t, tt, "添加到歌单…")
	if _, ok := tt.Find("添加到 通勤"); ok {
		t.Fatal("the picker offered to add the song to its source playlist")
	}
	tapPhone(t, tt, "添加到 睡前")
	if call := wait(t, calls, "the playlist addition"); call != (addition{11, 2}) || a.phoneAddTo != nil || a.route.id != 10 {
		t.Fatalf("added %+v, picker = %+v, route = %+v", call, a.phoneAddTo, a.route)
	}
}

// Scrolling to Remove in a track sheet removes that song from its source without changing the play queue.
func TestPhoneTrackActionsRemoveFromSource(t *testing.T) {
	a := editTestApp()
	a.phone = true
	playlist := a.playlists[10]
	playlist.Tracks[1].Artists[0].ID = 6
	playlist.Tracks[1].Album = netease.Album{ID: 7, Name: "专辑"}
	type removal struct{ playlist, song int64 }
	calls := make(chan removal, 1)
	old := removeFromPlaylist
	removeFromPlaylist = func(id int64, songs ...int64) error {
		calls <- removal{id, songs[0]}
		return nil
	}
	defer func() { removeFromPlaylist = old }()
	a.pl.Cue(playlist.Tracks)
	tt := ui.NewTester(a.view, 390, 570)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Scroll(195, 430, 0, 210)
	tapPhone(t, tt, "歌曲操作：宝船")
	dragPhone(t, tt, 195, 500, 0, -267)
	tapPhone(t, tt, "从歌单中移除")
	if call := wait(t, calls, "the playlist removal"); call != (removal{10, 2}) || len(playlist.Tracks) != 1 || playlist.Tracks[0].ID != 1 {
		t.Fatalf("removed %+v, remaining tracks = %v", call, playlist.Tracks)
	}
	if queue := a.pl.Queue(); len(queue) != 2 || queue[1].ID != 2 || a.phoneTrackTarget != nil {
		t.Fatalf("queue = %v, track sheet = %v", queue, a.phoneTrackTarget)
	}
}

// A phone download keeps its staging copy until Files returns and allows a new export after cancellation.
func TestPhoneSongDownloadWaitsForFiles(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	a.route = route{page: pagePlaylist, id: 1}
	a.playlists[1] = &netease.Playlist{ID: 1, Tracks: []netease.Track{{ID: 42, Name: "下载的歌"}}}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "正在播放"}})
	cache, staging, destination := t.TempDir(), t.TempDir(), t.TempDir()
	a.pl.Cache = player.NewCache(cache, 1<<20)
	const data = "cached audio"
	meta, err := json.Marshal(netease.Stream{Type: "flac", Level: "lossless", Size: int64(len(data))})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"42-lossless.flac": []byte(data), "42-lossless.json": meta} {
		if err := os.WriteFile(filepath.Join(cache, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	requests, chose := make(chan string, 2), make(chan string)
	ctx := t.Context()
	previous := downloadFile
	downloadFile = func(_ *app, song *player.Song, name string) (bool, error) {
		return saveSongCopy(staging, name, song, func(path, _ string) (string, error) {
			requests <- path
			select {
			case target := <-chose:
				if target == "" {
					return "", nil
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return "", err
				}
				return target, os.WriteFile(target, body, 0o600)
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	}
	defer func() { downloadFile = previous }()
	checkStaged := func(path string) {
		t.Helper()
		body, err := os.ReadFile(path)
		if filepath.Dir(filepath.Dir(path)) != staging || filepath.Base(path) != "下载的歌.flac" || err != nil || string(body) != data {
			t.Fatalf("staged export %q = %q, error = %v", path, body, err)
		}
	}
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌曲操作：下载的歌")
	tapPhone(t, tt, "下载")
	done := a.phoneDownload
	first := wait(t, requests, "the first Files export")
	checkStaged(first)
	tapPhone(t, tt, "歌曲操作：下载的歌")
	tapPhone(t, tt, "正在下载…")
	if a.phoneDownload != done || a.phoneTrackTarget == nil {
		t.Fatal("another download started while Files was already open")
	}
	chose <- ""
	wait(t, done, "the canceled Files export to finish")
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("canceled staging copy remains: %v", err)
	}
	tt.Frame()
	tapPhone(t, tt, "下载")
	second := wait(t, requests, "the retried Files export")
	checkStaged(second)
	target := filepath.Join(destination, filepath.Base(second))
	chose <- target
	wait(t, a.phoneDownload, "the completed Files export")
	if files, err := os.ReadDir(staging); err != nil || len(files) != 0 {
		t.Fatalf("staging contains %v, error = %v", files, err)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != data || a.pl.Current().ID != 7 {
		t.Fatalf("exported %q, error = %v, playback = %v", body, err, a.pl.Current())
	}
}

// Reading and liking comments stay on the opened song when playback advances, and closing returns to the next song's lyrics.
func TestPhoneCommentsReturnToLyrics(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.profile = &netease.Profile{UserID: 1}
	tr := netease.Track{ID: 11, Name: "第一首"}
	next := netease.Track{ID: 12, Name: "第二首"}
	a.pl.Cue([]netease.Track{tr, next})
	type pageRequest struct {
		thread string
		offset int
	}
	requests := make(chan pageRequest, 4)
	old := threadComments
	threadComments = func(thread string, offset, _ int) (*netease.Comments, error) {
		requests <- pageRequest{thread, offset}
		return &netease.Comments{}, nil
	}
	defer func() { threadComments = old }()
	type likeRequest struct {
		thread string
		id     int64
		like   bool
	}
	likes := make(chan likeRequest, 1)
	oldLike := likeComment
	likeComment = func(thread string, id int64, like bool) error {
		likes <- likeRequest{thread, id, like}
		return nil
	}
	defer func() { likeComment = oldLike }()
	tt := ui.NewTester(a.view, 390, 780)
	tapPhone(t, tt, "歌词")
	tapPhone(t, tt, "评论")
	if got := wait(t, requests, "the song's comment thread"); got != (pageRequest{tr.Thread(), 0}) {
		t.Fatalf("requested %+v, want the first song's first page", got)
	}
	if a.phoneSheet != phoneSheetComments || !a.phoneDetails || a.npPane != paneLyrics {
		t.Fatalf("comment sheet = %v, lyrics visible = %t, pane = %v", a.phoneSheet, a.phoneDetails, a.npPane)
	}
	a.comments.loading = false
	a.comments.appendPage(&netease.Comments{Latest: []netease.Comment{{ID: 7, Content: "第一首的评论"}}, Total: 1})
	a.settings.HotComments = true
	a.pl.Next()
	tt.Frame()
	if a.pl.Current().ID != next.ID || a.comments.track != tr.ID || a.comments.name != tr.Name || a.phoneSheet != phoneSheetComments {
		t.Fatalf("current track = %v, comment thread = %q, sheet = %v", a.pl.Current(), a.comments.thread, a.phoneSheet)
	}
	tapPhone(t, tt, "赞")
	if got := wait(t, likes, "the comment like"); got != (likeRequest{tr.Thread(), 7, true}) || !a.comments.latest[0].Liked {
		t.Fatalf("comment like = %+v, comments = %v", got, a.comments.latest)
	}
	a.comments.more = true
	tt.Frame()
	if got := wait(t, requests, "the next comment page"); got != (pageRequest{tr.Thread(), 1}) {
		t.Fatalf("requested %+v, want the first song's next page", got)
	}
	tapPhone(t, tt, "关闭评论")
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	if a.phoneSheet != phoneSheetClosed || !a.nowPlaying || !a.phoneDetails || a.npPane != paneLyrics {
		t.Fatalf("dismissing comments did not return to the lyric view: sheet=%v playing=%t details=%t pane=%v",
			a.phoneSheet, a.nowPlaying, a.phoneDetails, a.npPane)
	}
	if got := wait(t, requests, "the next song's hot comments"); got != (pageRequest{next.Thread(), 0}) {
		t.Fatalf("requested %+v, want the next song's first page", got)
	}
}

// A song and a podcast episode with the same audio ID open separate NetEase comment threads.
func TestPhoneCommentsUseEpisodeThread(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	song := netease.Track{ID: 13, Name: "歌曲"}
	episode := netease.Track{ID: 13, Name: "节目", ProgramID: 3}
	a.pl.Cue([]netease.Track{song})
	requested := make(chan string, 2)
	comments := threadComments
	defer func() { threadComments = comments }()
	threadComments = func(thread string, _, _ int) (*netease.Comments, error) {
		requested <- thread
		return &netease.Comments{}, nil
	}
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "评论")
	if thread := wait(t, requested, "song comments"); thread != song.Thread() {
		t.Fatalf("opening the song requested %q", thread)
	}
	a.comments.loading = false
	a.comments.appendPage(&netease.Comments{
		Latest: []netease.Comment{{ID: 7, Content: "歌曲评论"}},
		Total:  1,
	})
	tapPhone(t, tt, "关闭评论")
	a.pl.Cue([]netease.Track{episode})
	tt.Frame()
	tapPhone(t, tt, "评论")
	if thread := wait(t, requested, "episode comments"); thread != episode.Thread() {
		t.Fatalf("opening the episode requested %q", thread)
	}
	if a.comments.name != episode.Name || a.comments.total != 0 || len(a.comments.latest) != 0 {
		t.Fatal("the episode inherited the song's comments")
	}
}

// A session change reloads the open comments for the same thread and clears the previous listener's likes.
func TestPhoneCommentsRefreshOnAccountChange(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	opened := netease.Track{ID: 11, Name: "第一首"}
	a.pl.Cue([]netease.Track{{ID: 12, Name: "下一首"}})
	a.phoneSheet = phoneSheetComments
	a.comments = &commentState{
		track: opened.ID, thread: opened.Thread(), name: opened.Name, loaded: true, offset: 30,
		latest: []netease.Comment{{ID: 7, Liked: true}},
	}
	previous := a.comments
	type request struct {
		thread string
		offset int
	}
	requested := make(chan request, 1)
	comments := threadComments
	defer func() { threadComments = comments }()
	threadComments = func(thread string, offset, _ int) (*netease.Comments, error) {
		requested <- request{thread, offset}
		return &netease.Comments{}, nil
	}
	a.resetTrackComments()
	if got := wait(t, requested, "comments after the account change"); got != (request{opened.Thread(), 0}) {
		t.Fatalf("refresh requested %+v", got)
	}
	if a.phoneSheet != phoneSheetComments || a.comments == previous || a.comments.name != opened.Name ||
		len(a.comments.latest) != 0 || a.comments.offset != 0 {
		t.Fatal("the sheet lost its thread or inherited the previous listener's state")
	}
}

// Choosing a playlist from playback options adds the current track and returns straight to the lyrics.
func TestPhonePlayerOptionsAddToPlaylist(t *testing.T) {
	a := editTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 22, Name: "正在播放的歌"}})
	calls := make(chan [2]int64, 1)
	old := addToPlaylist
	addToPlaylist = func(id int64, songs ...int64) error {
		calls <- [2]int64{id, songs[0]}
		return nil
	}
	defer func() { addToPlaylist = old }()
	tt := ui.NewTester(a.view, 390, 780)
	for i, label := range []string{"歌词", "更多", "添加到歌单…", "添加到 睡前"} {
		if i > 0 {
			// Each tap rearranges the panes; let it land before the next.
			tt.Advance(400 * time.Millisecond)
			tt.Frame()
		}
		tapPhone(t, tt, label)
	}
	if call := wait(t, calls, "the playlist addition"); call != ([2]int64{11, 22}) {
		t.Fatalf("playlist and song = %v, want [11 22]", call)
	}
	if a.phoneAddTo != nil || a.phoneSheet != phoneSheetClosed || !a.nowPlaying || !a.phoneDetails {
		t.Fatal("the playlist picker did not return directly to the lyrics")
	}
}

// Opening lyrics with interlude comments enabled loads the song's thread, and tapping a hot comment keeps the lyric pane behind its sheet.
func TestPhoneInterludeCommentOpensSheet(t *testing.T) {
	a, tr, _ := quietApp(5 * time.Second)
	a.phone, a.nowPlaying = true, true
	comments := a.comments
	a.comments = nil
	requests := make(chan string, 1)
	old := threadComments
	threadComments = func(thread string, _, _ int) (*netease.Comments, error) {
		requests <- thread
		return &netease.Comments{}, nil
	}
	defer func() { threadComments = old }()
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌词")
	if thread := wait(t, requests, "the song's comment thread"); thread != tr.Thread() {
		t.Fatalf("requested %q, want %q", thread, tr.Thread())
	}
	comments.thread = tr.Thread()
	a.comments = comments
	tt.Frame()
	tt.Advance(time.Second)
	tapPhone(t, tt, "查看评论")
	if a.phoneSheet != phoneSheetComments || a.npPane != paneLyrics || !a.phoneDetails {
		t.Fatalf("sheet = %v, pane = %v, lyrics open = %v", a.phoneSheet, a.npPane, a.phoneDetails)
	}
}

// The phone player follows the ready artwork and honors the persisted choice to cut directly to the next cover.
func TestPhoneCoverTransitionMenuCutsToNextCover(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tracks := []netease.Track{
		{ID: 1, Name: "第一首", Album: netease.Album{PicURL: "https://example.com/cover-1.jpg"}},
		{ID: 2, Name: "第二首", Album: netease.Album{PicURL: "https://example.com/cover-2.jpg"}},
	}
	first := &artwork{tint: ui.Hex("#836340")}
	second := &artwork{tint: ui.Hex("#3e7b85")}
	a.art.add(netease.Sized(tracks[0].Album.PicURL, 600)+"#backdrop", first)
	a.art.add(netease.Sized(tracks[1].Album.PicURL, 600)+"#backdrop", second)
	a.pl.Cue(tracks[:1])
	tt := ui.NewTester(a.view, 390, 780)
	tt.Frame()
	if a.npVisual.current != first || a.npArt != first {
		t.Fatal("the current cover was not prepared for the player and its backdrop")
	}
	tapPhone(t, tt, "更多")
	tapPhone(t, tt, "播放显示")
	tapPhone(t, tt, "封面过渡")
	data, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	if a.settings.CoverTransition || parseSettings(data).CoverTransition {
		t.Fatal("the menu did not turn off and save the cover transition")
	}
	a.pl.Cue(tracks[1:])
	tt.Frame()
	if a.npVisual.current != second || a.npVisual.before != nil || a.npVisual.alpha != 1 {
		t.Fatal("the player did not cut directly to the ready cover")
	}
}

// The local track's phone menu opens a single-file LRC picker without playing the row or leaving the library.
func TestPhoneLocalSongMenuPicksLyrics(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageLocal}
	a.localTracks = []netease.Track{
		{ID: -1, Name: "本地录音", LocalPath: "/tmp/recording.mp3"},
	}
	requests := make(chan mygo.OpenDialogOptions, 1)
	old := pickLocalFiles
	pickLocalFiles = func(options mygo.OpenDialogOptions) ([]string, error) {
		requests <- options
		return nil, nil
	}
	defer func() { pickLocalFiles = old }()
	tt := ui.NewTester(a.view, 390, 780)
	tapPhone(t, tt, "歌曲操作：本地录音")
	tapPhone(t, tt, "导入歌词…")
	options := wait(t, requests, "the lyric file picker")
	if options.Multiple || len(options.Filters) != 1 || !slices.Equal(options.Filters[0].Extensions, []string{"lrc"}) {
		t.Fatalf("picker options = %+v", options)
	}
	if a.pl.Current() != nil || a.route.page != pageLocal {
		t.Fatal("opening the lyric picker also played or navigated")
	}
}

// After rotating the player, lyrics still seek and the queue can still select another song.
func TestPhoneRotationKeepsLyricsAndQueueUsable(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 90000}, {ID: 22, Name: "第二首", Duration: 90000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句\n[00:16]第二句\n[00:30]第三句\n[00:45]第四句", "", ""))
	a.pl.Hold(5 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌词")
	tt.SetSize(844, 390)
	if tester, ok := any(tt).(interface {
		SetInsets(float32, float32, float32, float32)
	}); ok {
		tester.SetInsets(0, 47, 21, 47)
	}
	tt.Frame()
	if a.pl.Position() != 5*time.Second || !a.phoneDetails {
		t.Fatal("rotation changed the song position or closed the lyrics")
	}
	dragPhone(t, tt, 422, 200, 0, -60)
	if !a.lyricPaneState.seeking {
		t.Fatal("scrolling the rotated lyrics did not unfollow playback")
	}
	tapPhone(t, tt, "第四句")
	if a.pl.Position() != 45*time.Second || a.seeking {
		t.Fatalf("tapping the rotated lyric left playback at %v, browsing=%t", a.pl.Position(), a.seeking)
	}
	tapPhone(t, tt, "待播清单")
	tapPhone(t, tt, "第二首")
	tapPhone(t, tt, "歌词")
	if a.pl.Current().ID != 22 || !a.phoneDetails {
		t.Fatal("selecting from the landscape queue did not preserve the playback view")
	}
}

// The landscape player lays out as the desktop's: the panes beside the cover, over the full transport.
func TestPhoneLandscapePlayerPanes(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 90000}, {ID: 22, Name: "第二首", Duration: 90000}})
	tt := ui.NewTester(a.view, 844, 390)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	if _, ok := tt.Find("随机播放"); !ok {
		t.Fatal("the landscape player is missing the transport")
	}
	tapPhone(t, tt, "待播清单")
	if a.npPane != paneQueue {
		t.Fatal("the queue tab did not open the queue pane")
	}
	tapPhone(t, tt, "第二首")
	if a.pl.Current().ID != 22 || a.npPane != paneQueue {
		t.Fatal("selecting from the landscape queue left the queue")
	}
	tapPhone(t, tt, "律动")
	if a.npPane != paneStage || !a.settings.LyricsStage {
		t.Fatal("the stage tab did not choose the stage")
	}
	tapPhone(t, tt, "上一首")
	if a.pl.Current().ID != 11 {
		t.Fatal("the landscape previous button did not go back a song")
	}
}

// The phone's 动态背景 choice saves the living backdrop's switch, which
// stops and starts its paint frames while music plays.
func TestPhoneBackdropMotionChoice(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	readSaved := func() settings {
		t.Helper()
		data, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		return parseSettings(data)
	}
	art := &artwork{backdrop: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))}
	if !a.livingBackdrop(art, true, false, false) {
		t.Fatal("the living backdrop is off by default")
	}
	tt := ui.NewTester(func(c *ui.Context) { a.phoneDisplayOptions(c, false) }, 390, 844)
	tapPhone(t, tt, "动态背景")
	if saved := readSaved(); saved.phoneBackdropMotion() {
		t.Fatal("turning off 动态背景 did not save")
	}
	if a.livingBackdrop(art, true, false, false) {
		t.Fatal("the living backdrop paints while its choice is off")
	}
	// The sheet's own motion and the listener's less-motion ask still gate it.
	a.settings.PhoneBackdropMotion = nil
	for _, gate := range []struct {
		gpu, still, moving bool
		want               bool
	}{{true, true, false, false}, {true, false, true, false}, {false, false, false, false}, {true, false, false, true}} {
		if got := a.livingBackdrop(art, gate.gpu, gate.still, gate.moving); got != gate.want {
			t.Errorf("livingBackdrop(gpu %v, still %v, moving %v) = %v, want %v", gate.gpu, gate.still, gate.moving, got, gate.want)
		}
	}
	off := false
	a.settings.PhoneBackdropMotion = &off
	for _, on := range []bool{true, false} {
		tapPhone(t, tt, "动态背景")
		tt.Frame()
		if saved := readSaved(); saved.phoneBackdropMotion() != on {
			t.Fatalf("动态背景 saved as %v, want %v", saved.phoneBackdropMotion(), on)
		}
	}
}

// Rotating with a later bilingual lyric active keeps the whole line in the lyrics viewport.
func TestPhoneRotationKeepsActiveLyricVisible(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "测试歌曲", Duration: 90000}})
	const text = "I wish I could go back in time"
	var lines []lyrics.Line
	for i := range 10 {
		lines = append(lines, lyrics.Line{At: time.Duration(i) * 3 * time.Second, Text: fmt.Sprintf("Why do we only have one chance to try? %d", i), Translation: "我们一生中有太多遗憾"})
	}
	lines = append(lines,
		lyrics.Line{At: 30 * time.Second, Text: text, Translation: "我真希望可以回到过去"},
		lyrics.Line{At: 45 * time.Second, Text: "Each time I fall asleep", Translation: "每当我睡着时"},
	)
	a.pl.SetLyrics(lines)
	a.pl.Hold(33 * time.Second)
	var viewport ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		viewport = a.lyricScroll.Bounds(c)
	}, 402, 874)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	if tester, ok := any(tt).(interface {
		SetInsets(float32, float32, float32, float32)
	}); ok {
		tester.SetInsets(62, 0, 34, 0)
		tt.SetSize(874, 402)
		tester.SetInsets(0, 62, 21, 62)
	} else {
		tt.SetSize(874, 402)
	}
	tt.Frame()
	line, found := tt.Find(text)
	if !found || line.H == 0 || viewport.H == 0 || line.Y < viewport.Y || line.Y+line.H > viewport.Y+viewport.H {
		t.Fatalf("the active lyric is clipped after rotation: line %+v, viewport %+v", line, viewport)
	}
}

// The lyrics button reopens the chosen stage style until the listener explicitly switches to scrolling lyrics.
func TestPhoneLyricsToggleKeepsChosenMode(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首"}})
	tt := ui.NewTester(a.view, 390, 780)
	for _, label := range []string{"更多", "播放显示", "律动"} {
		tapPhone(t, tt, label)
	}
	tapPhone(t, tt, "返回封面")
	tapPhone(t, tt, "歌词")
	if !a.phoneDetails || a.npPane != paneStage {
		t.Fatal("reopening lyrics discarded the chosen stage mode")
	}
	for _, label := range []string{"更多", "播放显示", "滚动歌词"} {
		tapPhone(t, tt, label)
	}
	if !a.phoneDetails || a.npPane != paneLyrics || a.settings.LyricsStage {
		t.Fatal("switching to scrolling lyrics did not update the chosen mode")
	}
}

// The daily card on the phone home opens the daily page, and the FM card the FM.
func TestPhoneHomeCards(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.daily = []netease.Track{{ID: 1, Name: "每日一"}, {ID: 2, Name: "每日二"}}
	a.recPlaylists = []netease.Playlist{{ID: 5, Name: "推荐歌单一"}}
	a.featured = []netease.Playlist{{ID: 9, Name: "精选歌单"}}
	a.charts = []netease.Playlist{{ID: 7, Name: "飙升榜"}}
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "每日推荐")
	if a.route.page != pageDaily {
		t.Fatalf("the daily card opened %v", a.route)
	}
	tapPhone(t, tt, "推荐")
	tapPhone(t, tt, "私人FM")
	if a.route.page != pageFM {
		t.Fatalf("the FM card opened %v", a.route)
	}
}

// The home shelf fetches distant chart covers only after scrolling near them, and the last card still opens its chart.
func TestPhoneHomeChartShelfFetchesNearbyCovers(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.art.fetch = func(string) ([]byte, error) { return nil, nil }
	for i := range 8 {
		pl := netease.Playlist{ID: int64(i + 100), Name: fmt.Sprintf("榜单 %d", i+1), CoverURL: fmt.Sprintf("cover-%d", i+1)}
		a.charts = append(a.charts, pl)
		a.playlists[pl.ID] = &pl
	}
	tt := ui.NewTester(a.view, 402, 874)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	first, last := a.charts[0], a.charts[len(a.charts)-1]
	firstCover, lastCover := netease.Sized(first.Cover(), 200), netease.Sized(last.Cover(), 200)
	if !a.art.busy[firstCover] || a.art.busy[lastCover] {
		t.Fatal("opening home should queue the visible chart cover and leave the last cover unloaded")
	}
	box, ok := tt.Find(first.Name)
	if !ok {
		t.Fatal("the first chart is missing")
	}
	tt.Scroll(box.X+box.W/2, box.Y+box.H/2, 4000, 0)
	tt.Advance(time.Second)
	tt.Frame()
	if !a.art.busy[lastCover] {
		t.Fatal("scrolling to the last chart did not queue its cover")
	}
	tapPhone(t, tt, last.Name)
	if a.route != (route{page: pagePlaylist, id: last.ID}) {
		t.Fatalf("the last chart opened %+v", a.route)
	}
}

// A phone playlist's rows fetch their songs' covers once in view, and a tap
// through a cover reaches the row and plays its song.
func TestPhonePlaylistCoverRows(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.art.fetch = func(string) ([]byte, error) { return nil, nil }
	pl := &netease.Playlist{ID: 42, Name: "歌单"}
	for i := range 30 {
		pl.Tracks = append(pl.Tracks, netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌曲 %d", i+1),
			Artists: []netease.Artist{{Name: "歌手"}}, Album: netease.Album{PicURL: fmt.Sprintf("cover-%d", i+1)}})
	}
	a.playlists[pl.ID] = pl
	a.route, a.history = route{page: pagePlaylist, id: pl.ID}, []route{{page: pageLibrary}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	firstCover := netease.Sized(pl.Tracks[0].Album.PicURL, 120)
	lastCover := netease.Sized(pl.Tracks[len(pl.Tracks)-1].Album.PicURL, 120)
	if !a.art.busy[firstCover] || a.art.busy[lastCover] {
		t.Fatal("the visible rows did not queue their covers, or an offscreen row queued its")
	}
	box, ok := tt.Find(pl.Tracks[1].Name)
	if !ok {
		t.Fatal("the second row is missing")
	}
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the tester has no touch")
	}
	// The cover sits 12 from the row's left, 44 wide, centered.
	x, y := box.X+12+22, box.Y+box.H/2
	touch.FingerDown(x, y)
	touch.FingerUp(x, y)
	if a.pl.Current() == nil || a.pl.Current().ID != pl.Tracks[1].ID {
		t.Fatalf("a tap on the cover played %v", a.pl.Current())
	}
}

// A chart tile in the phone search tab's idle body opens that chart's playlist.
func TestPhoneSearchBrowseCharts(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab, a.route = true, pageHome, route{page: pageSearch}
	a.charts = []netease.Playlist{{ID: 300, Name: "飙升榜"}}
	a.playlists[300] = &netease.Playlist{ID: 300, Name: "飙升榜"}
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "飙升榜")
	if a.route.page != pagePlaylist || a.route.id != 300 {
		t.Fatalf("the chart tile opened %+v", a.route)
	}
}

// With NM_FRAMES set, saves the phone's home, its scrolled nav bar, the
// library tab and a playlist page for visual inspection.
func TestPhoneFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newPhone := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.daily = []netease.Track{
			{ID: 1, Name: "每日第一首", Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}},
			{ID: 2, Name: "每日第二首", Artists: []netease.Artist{{Name: "歌手乙"}}},
		}
		for i := range 6 {
			a.recPlaylists = append(a.recPlaylists, netease.Playlist{ID: int64(100 + i), Name: fmt.Sprintf("推荐歌单 %d", i+1), CoverURL: "qa-cover"})
		}
		for i := range 5 {
			a.featured = append(a.featured, netease.Playlist{ID: int64(200 + i), Name: fmt.Sprintf("精选歌单 %d", i+1)})
		}
		for i := range 4 {
			a.charts = append(a.charts, netease.Playlist{ID: int64(300 + i), Name: fmt.Sprintf("榜单 %d", i+1), UpdateFreq: "每日更新",
				Preview: []netease.ChartLine{{First: "第一首", Second: "歌手甲"}, {First: "第二首", Second: "歌手乙"}, {First: "第三首", Second: "歌手丙"}}})
		}
		a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}}})
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art[netease.Sized("qa-cover", 120)] = art
		a.art.art["qa-cover"] = art
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}
	a, tt := newPhone(false)
	save(tt, "phone-home-light")
	tt.Scroll(195, 500, 0, 400)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-home-scrolled-light")
	if err := tt.Click("我的"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-library-light")
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
		Tracks: []netease.Track{
			{ID: 11, Name: "第一首", Duration: 200000, Artists: []netease.Artist{{Name: "歌手甲"}}, Album: netease.Album{Name: "专辑甲", PicURL: "qa-cover"}},
			{ID: 12, Name: "第二首", Duration: 180000, Artists: []netease.Artist{{Name: "歌手乙"}}, Album: netease.Album{Name: "专辑乙", PicURL: "qa-cover"}},
			{ID: 13, Name: "第三首", Duration: 240000, Artists: []netease.Artist{{Name: "歌手丙"}}, Album: netease.Album{Name: "专辑丙"}},
		}}
	a.pl.Cue(a.playlists[42].Tracks)
	a.open(route{page: pagePlaylist, id: 42})
	// Settle past the 400ms push spring so the page rests at the edge.
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-playlist-light")
	a, tt = newPhone(true)
	save(tt, "phone-home-dark")

	// The home page scrolled to its very end, under the dock.
	tt.Scroll(195, 500, 0, 10000)
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-home-bottom-dark")

	// The playlist page in the dark.
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
		Tracks: []netease.Track{
			{ID: 11, Name: "第一首", Duration: 200000, Artists: []netease.Artist{{Name: "歌手甲"}}, Album: netease.Album{Name: "专辑甲", PicURL: "qa-cover"}},
			{ID: 12, Name: "第二首", Duration: 180000, Artists: []netease.Artist{{Name: "歌手乙"}}, Album: netease.Album{Name: "专辑乙", PicURL: "qa-cover"}},
			{ID: 13, Name: "第三首", Duration: 240000, Artists: []netease.Artist{{Name: "歌手丙"}}, Album: netease.Album{Name: "专辑丙"}},
		}}
	a.route = route{page: pagePlaylist, id: 42}
	a.history = []route{{page: pageLibrary}}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-playlist-dark")

	// The library, signed in with its own and saved playlists.
	a, tt = newPhone(false)
	a.profile = &netease.Profile{UserID: 7, Nickname: "听友", AvatarURL: "qa-cover"}
	a.mine = []netease.Playlist{
		{ID: 5, Name: "听友喜欢的音乐", SpecialType: 5, TrackCount: 231},
		{ID: 10, Name: "通勤", TrackCount: 12, CoverURL: "qa-cover"},
	}
	a.saved = []netease.Playlist{{ID: 20, Name: "飙升榜", TrackCount: 100}}
	a.route, a.phoneTab = route{page: pageLibrary}, pageLibrary
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-library-signed-in")

	// The album page.
	a, tt = newPhone(false)
	a.albums[8] = &albumData{album: &netease.AlbumProfile{
		Album:   netease.Album{ID: 8, Name: "迷跡波", PicURL: "qa-cover"},
		Artists: []netease.Artist{{ID: 9, Name: "MyGO!!!!!"}},
		Songs:   []netease.Track{{ID: 1, Name: "春日影", Duration: 200000}, {ID: 2, Name: "歌いましょう", Duration: 180000}},
	}}
	a.route = route{page: pageAlbum, id: 8}
	a.history = []route{{page: pageLibrary}}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-album-light")

	// The artist page.
	a, tt = newPhone(false)
	a.artists[9] = &artistData{profile: &netease.ArtistProfile{
		Artist: netease.Artist{ID: 9, Name: "MyGO!!!!!"},
		Avatar: "qa-cover",
		Songs:  []netease.Track{{ID: 1, Name: "春日影", Duration: 200000}, {ID: 2, Name: "歌いましょう", Duration: 180000}},
	}}
	a.route = route{page: pageArtist, id: 9}
	a.history = []route{{page: pageLibrary}}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-artist-light")

	// The search tab, empty, with history and charts to browse.
	search := func(dark bool) (*app, *ui.Tester) {
		a, tt := newPhone(dark)
		a.settings = defaultSettings()
		a.settings.SearchHistory = []string{"春日影", "迷跡波", "夜航电台"}
		a.route, a.phoneTab = route{page: pageSearch}, pageHome
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}
	a, tt = search(false)
	save(tt, "phone-search-light")
	a, tt = search(true)
	save(tt, "phone-search-dark")

	// Search results with the kind capsules.
	a, tt = newPhone(false)
	r := route{page: pageSearch, query: "春日影", kind: netease.SearchSongs}
	a.searched, a.query = r, "春日影"
	a.searches[r] = &searchData{page: paged{loaded: true}, res: netease.SearchResult{
		Total: 3,
		Songs: []netease.Track{
			{ID: 1, Name: "春日影", Duration: 200000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
			{ID: 2, Name: "歌いましょう鳴らしましょう", Duration: 180000, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
			{ID: 3, Name: "壱雫空", Duration: 190000, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
		},
	}}
	a.route, a.phoneTab = r, pageHome
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-search-results-light")

	// The field being edited.
	a, tt = newPhone(false)
	a.route, a.phoneTab = route{page: pageSearch}, pageHome
	a.phoneSearchEditing = true
	a.query = "春"
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-search-editing-light")
	a.dismissSuggestions()

	// Now Playing, playing and paused.
	playerTracks := []netease.Track{
		{ID: 31, Name: "正在播放的歌", Duration: 200000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}},
		{ID: 32, Name: "下一首歌", Duration: 180000, Artists: []netease.Artist{{Name: "歌手乙"}}},
		{ID: 33, Name: "第三首歌", Duration: 190000, Artists: []netease.Artist{{Name: "歌手丙"}}},
	}
	a, tt = newPhone(false)
	a.nowPlaying = true
	a.pl.Cue(playerTracks)
	a.pl.Hold(34 * time.Second)
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-nowplaying-playing")

	a, tt = newPhone(false)
	a.nowPlaying = true
	a.pl.Cue(playerTracks)
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-nowplaying-paused")

	// Now Playing with the lyrics shown, and its queue and a track's
	// actions sheet.
	a, tt = newPhone(false)
	a.nowPlaying = true
	a.phoneDetails = true
	a.pl.Cue(playerTracks)
	a.pl.Hold(34 * time.Second)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-nowplaying-lyrics")

	a.phoneDetails = false
	a.phoneSheet = phoneSheetQueue
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-queue-sheet")

	a.phoneSheet = phoneSheetClosed
	a.phoneTrackTarget = &trackSelection{track: playerTracks[0]}
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-track-actions-sheet")

	// The secondary pages.
	secondary := []struct {
		name string
		goTo func(*app)
	}{
		{"phone-daily-light", func(a *app) { a.route = route{page: pageDaily} }},
		{"phone-charts-light", func(a *app) { a.route = route{page: pageCharts} }},
		{"phone-podcasts-light", func(a *app) {
			a.podcastTab.hotPage.loaded = true
			a.podcastTab.picks = []netease.Podcast{{ID: 5, Name: "夜航电台", ProgramCount: 120}}
			a.podcastTab.hot = []netease.Podcast{{ID: 6, Name: "通勤十分钟", ProgramCount: 48}}
			a.route, a.phoneTab = route{page: pagePodcasts}, pagePodcasts
		}},
		{"phone-fm-light", func(a *app) { a.route = route{page: pageFM} }},
		{"phone-local-light", func(a *app) { a.route = route{page: pageLocal} }},
		{"phone-user-light", func(a *app) {
			a.users[9] = &userData{user: &netease.User{UserID: 9, Nickname: "听歌的人", Signature: "在音乐里，寻找同路的人。"},
				playlists: []netease.Playlist{{ID: 30, Name: "通勤路上", TrackCount: 40}}}
			a.route = route{page: pageUser, id: 9}
		}},
	}
	for _, s := range secondary {
		a, tt = newPhone(false)
		s.goTo(a)
		a.history = []route{{page: pageLibrary}}
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		save(tt, s.name)
	}

	// The 我的 page's 收藏 tab, signed in.
	a, tt = newPhone(false)
	a.profile = &netease.Profile{UserID: 7, Nickname: "听友", AvatarURL: "qa-cover"}
	a.mine = []netease.Playlist{
		{ID: 5, Name: "听友喜欢的音乐", SpecialType: 5, TrackCount: 231},
		{ID: 10, Name: "通勤", TrackCount: 12, CoverURL: "qa-cover"},
	}
	a.saved = []netease.Playlist{{ID: 20, Name: "飙升榜", TrackCount: 100, CoverURL: "qa-cover"}}
	a.route, a.phoneTab, a.phoneMineTab = route{page: pageLibrary}, pageLibrary, 1
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-mine-saved-light")

	// Settings, in both appearances.
	for _, dark := range []bool{false, true} {
		a, tt = newPhone(dark)
		a.settings = defaultSettings()
		a.route = route{page: pageSettings}
		a.history = []route{{page: pageLibrary}}
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		name := "phone-settings-light"
		if dark {
			name = "phone-settings-dark"
		}
		save(tt, name)
	}

	// A playlist's description sheet.
	a, tt = newPhone(false)
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
		Description: "给深夜读书的一张歌单，安静、缓慢，偶有钢琴。",
		Tracks:      []netease.Track{{ID: 11, Name: "第一首"}, {ID: 12, Name: "第二首"}, {ID: 13, Name: "第三首"}}}
	a.route = route{page: pagePlaylist, id: 42}
	a.history = []route{{page: pageLibrary}}
	a.phoneDesc = a.playlists[42].Description
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-desc-sheet")
}

// Tapping 随机播放 on the phone album page plays the album with shuffle on.
func TestPhoneAlbumShuffle(t *testing.T) {
	a, d := albumApp()
	a.phone = true
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "随机播放")
	if !a.pl.Shuffle() || a.pl.Current() == nil || len(a.pl.Queue()) != len(d.album.Songs) {
		t.Fatalf("shuffle = %v, current = %v, queue = %d tracks", a.pl.Shuffle(), a.pl.Current(), len(a.pl.Queue()))
	}
}

// Re-tapping the active tab pops its pushed detail page back to the root, and a second tap at the root leaves the route alone.
func TestPhoneTabRetapPopsToRoot(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab = true, pageLibrary
	a.route = route{page: pageLibrary}
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单甲", Tracks: []netease.Track{{ID: 11, Name: "第一首"}}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	tapPhone(t, tt, "我的")
	if a.route.page != pageLibrary || len(a.history) != 0 {
		t.Fatalf("re-tapping the tab left %v with %d pages of history", a.route, len(a.history))
	}
	tapPhone(t, tt, "我的")
	if a.route != (route{page: pageLibrary}) {
		t.Fatalf("re-tapping at the root changed the route to %v", a.route)
	}
}

// A short drag from the left edge springs the page back, while one past a third of the window pops it.
func TestPhoneEdgeSwipeBack(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab = true, pageHome
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单甲", Tracks: []netease.Track{{ID: 11, Name: "第一首"}}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	dragPhone(t, tt, 10, 400, 60, 0)
	if a.route.page != pagePlaylist {
		t.Fatalf("a short edge pull popped to %v", a.route)
	}
	dragPhone(t, tt, 10, 400, 300, 0)
	if a.route.page != pageHome || len(a.history) != 0 {
		t.Fatalf("a long edge pull left %v with %d pages of history", a.route, len(a.history))
	}
}

// A short, fast edge swipe carries the header with the finger and pops only after the page has slid out.
func TestPhoneBackFlickFinishesSlide(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单甲", Tracks: []netease.Track{{ID: 11, Name: "第一首"}}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	back, ok := tt.Find("返回")
	if !ok {
		t.Fatal("the header has no back button")
	}
	tt.FingerDown(10, 400)
	tt.Advance(20 * time.Millisecond)
	tt.FingerMove(43, 400)
	tt.Advance(20 * time.Millisecond)
	tt.FingerMove(90, 400)
	moved, ok := tt.Find("返回")
	if !ok || moved.X < back.X+70 || moved.X > back.X+90 {
		t.Fatalf("the header did not follow the finger: %.1f → %.1f", back.X, moved.X)
	}
	tt.Advance(10 * time.Millisecond)
	tt.FingerUp(98, 400)
	tt.Advance(60 * time.Millisecond)
	if a.route.page != pagePlaylist {
		t.Fatal("the swipe changed the route before the slide finished")
	}
	tt.Advance(350 * time.Millisecond)
	if a.route.page != pageHome || len(a.history) != 0 || len(a.forward) != 1 {
		t.Fatalf("the flick left route %v with back=%d and forward=%d", a.route, len(a.history), len(a.forward))
	}
}

// A system-cancelled swipe leaves the page in place and the next swipe returns to the prior list's saved position.
func TestPhoneBackSwipeRestoresScrolledPage(t *testing.T) {
	a := newTestApp()
	a.phone = true
	first := route{page: pagePlaylist, id: 7}
	second := route{page: pagePlaylist, id: 8}
	a.route = first
	for _, r := range []route{first, second} {
		tracks := make([]netease.Track, 100)
		for i := range tracks {
			tracks[i] = netease.Track{ID: int64(i + 1), Name: fmt.Sprintf("歌单 %d / %d", r.id, i)}
		}
		a.playlists[r.id] = &netease.Playlist{ID: r.id, Name: fmt.Sprintf("歌单 %d", r.id), Tracks: tracks}
	}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.ls().ScrollTo(24, ui.Start)
	tt.Frame()
	firstY := a.ps().Y
	if firstY <= 100 {
		t.Fatal("the previous list did not scroll")
	}
	a.open(second)
	tt.Frame()
	tt.Advance(600 * time.Millisecond)
	tt.FingerDown(10, 460)
	tt.FingerMove(235, 460)
	tt.FingerCancel()
	tt.Advance(400 * time.Millisecond)
	if a.route != second || len(a.history) != 1 {
		t.Fatalf("the cancelled swipe changed route to %v", a.route)
	}
	dragPhone(t, tt, 10, 460, 230, 0)
	if a.route != first || abs32(a.ps().Y-firstY) > 2 {
		t.Fatalf("the completed swipe returned to %v, scroll %.1f, want %v at %.1f", a.route, a.ps().Y, first, firstY)
	}
}

// Starting a vertical scroll in the back gesture's edge strip still moves the playlist.
func TestPhoneVerticalEdgeDragScrollsList(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pagePlaylist, id: 7}
	a.history = []route{{page: pageHome}}
	tracks := make([]netease.Track, 100)
	for i := range tracks {
		tracks[i] = netease.Track{ID: int64(i + 1), Name: "歌曲"}
	}
	a.playlists[7] = &netease.Playlist{ID: 7, Name: "歌单", Tracks: tracks}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	dragPhone(t, tt, 10, 550, 0, -220)
	if a.route.page != pagePlaylist || a.ps().Y < 120 {
		t.Fatalf("the vertical drag left route %v and scroll %.1f", a.route, a.ps().Y)
	}
}

// Tapping the Now Playing cover swaps it for the lyrics, and the compact header's 返回封面 button swaps back.
func TestPhoneCoverTapShowsLyrics(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 90000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句\n[00:16]第二句", "", ""))
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tapPhone(t, tt, "专辑封面")
	if !a.phoneDetails || a.npPane != paneLyrics {
		t.Fatalf("the cover tap left details=%t pane=%v", a.phoneDetails, a.npPane)
	}
	tapPhone(t, tt, "返回封面")
	if a.phoneDetails {
		t.Fatal("返回封面 did not bring the cover back")
	}
}

// After tapping the cover, the current lyric stays anchored from the first frame that displays it.
func TestPhoneCoverSwitchKeepsCurrentLyricStill(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "测试", Duration: 120000, LocalPath: "/test/song.mp3"}})
	const current = "The line already playing when the listener opens the lyrics"
	lines := []lyrics.Line{
		{Text: "第一行"},
		{At: 5 * time.Second, Text: "第二行"},
		{At: 10 * time.Second, Text: "第三行"},
		{At: 15 * time.Second, Text: current, Translation: "正在播放的这一句"},
		{At: 20 * time.Second, Text: "第五行"},
	}
	lines[3].Words = lyrics.SpreadWords(current, 15*time.Second, 20*time.Second)
	a.pl.SetLyrics(lines)
	a.pl.Hold(17 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.Advance(time.Second)
	tapPhone(t, tt, "专辑封面")
	var first ui.Rect
	for range 18 {
		tt.Advance(time.Second / 60)
		row, shown := tt.Find(current)
		if !shown {
			continue
		}
		if first.H == 0 {
			first = row
		}
		if abs32(row.Y-first.Y) > 2 {
			t.Fatalf("the visible current lyric moved from %.1f to %.1f during entry", first.Y, row.Y)
		}
	}
	if !a.phoneDetails || first.H == 0 {
		t.Fatal("the current lyric did not appear")
	}
}

func fingerTap(t *testing.T, tt *ui.Tester, x, y float32) {
	t.Helper()
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	touch.FingerDown(x, y)
	touch.FingerUp(x, y)
	tt.Frame()
}

// A lyric tap seeks to that line and leaves the lyrics open, while tapping empty space returns to the cover.
func TestPhoneLyricsTapSeeksAndSpaceReturnsToCover(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 90000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句\n[00:16]第二句\n[00:30]第三句\n[00:45]第四句", "", ""))
	a.pl.Hold(5 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tapPhone(t, tt, "歌词")
	if !a.phoneDetails {
		t.Fatal("the lyrics did not open")
	}
	tapPhone(t, tt, "第三句")
	if !a.phoneDetails || a.pl.Position() != 30*time.Second {
		t.Fatalf("a line tap left details=%t at %v, want open at 30s", a.phoneDetails, a.pl.Position())
	}
	tt.Advance(time.Second)
	tt.Frame()
	ln, ok := tt.Find("第四句")
	if !ok {
		t.Fatal("the lyrics did not reopen")
	}
	fingerTap(t, tt, ln.X+ln.W/2, ln.Y+ln.H+40)
	if a.phoneDetails {
		t.Fatal("tapping the lyrics' empty space did not return to the cover")
	}
}

// Browsing lyrics leaves playback alone until the listener taps a visible line or seeks with the slider.
func TestPhoneLyricsBrowseAndSeek(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 360000}})
	lines := ""
	for i := range 18 {
		lines += fmt.Sprintf("[%02d:%02d]第 %d 句歌词\n", i/6, i%6*10, i+1)
	}
	a.pl.SetLyrics(lyrics.Build(lines, "", ""))
	a.pl.Hold(105 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌词")
	dragPhone(t, tt, 195, 400, 0, -160)
	tt.Frame()
	if !a.lyricPaneState.seeking {
		t.Fatal("scrolling the lyrics did not unfollow playback")
	}
	if a.pl.Position() != 105*time.Second {
		t.Fatalf("scrolling sought to %v", a.pl.Position())
	}
	// Pick the row by its own hit target, even with other rows between it and the playhead.
	tapPhone(t, tt, "第 15 句歌词")
	if a.seeking || !a.phoneDetails || a.pl.Position() != 140*time.Second {
		t.Fatalf("row tap left browsing=%t, lyrics=%t at %v", a.seeking, a.phoneDetails, a.pl.Position())
	}
	dragPhone(t, tt, 195, 400, 0, 160)
	if !a.seeking {
		t.Fatal("the second scroll did not suspend follow")
	}
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("the progress slider is missing")
	}
	fingerTap(t, tt, bar.X+bar.W*0.1, bar.Y+bar.H/2)
	if a.seeking || a.pl.Position() > 40*time.Second || a.pl.Position() < 32*time.Second {
		t.Fatalf("slider left browsing=%t at %v", a.seeking, a.pl.Position())
	}
}

// Seeking beyond the rendered lyrics follows the new line and resumes following playback.
func TestPhoneLyricsFollowFarSeek(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "测试歌曲", Duration: 240000}})
	lines := make([]lyrics.Line, 120)
	for i := range lines {
		lines[i] = lyrics.Line{At: time.Duration(i) * 2 * time.Second, Text: fmt.Sprintf("第 %d 句", i)}
	}
	a.pl.SetLyrics(lines)
	a.pl.Hold(12 * time.Second)
	var viewport ui.Rect
	tt := ui.NewTester(func(c *ui.Context) {
		a.view(c)
		viewport = a.lyricScroll.Bounds(c)
	}, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	slider, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("the playback slider is missing")
	}
	fingerTap(t, tt, slider.X+slider.W*0.8, slider.Y+slider.H/2)
	tt.Frame()
	cur := lyrics.Index(lines, a.pl.Position())
	if cur < 80 {
		t.Fatalf("the slider did not seek far through the song: line %d", cur)
	}
	line, ok := tt.Find(lines[cur].Text)
	if !ok || line.H == 0 || viewport.H == 0 || line.Y < viewport.Y || line.Y+line.H > viewport.Y+viewport.H || a.seeking {
		t.Fatalf("the new line did not stay in view: line %+v, viewport %+v, seeking %t", line, viewport, a.seeking)
	}
}

// A long transcript follows the scrubber across the episode without building every caption row.
func TestPhoneCaptionsFollowFarSeek(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.pl.Cue([]netease.Track{{ID: 31, ProgramID: 65, Name: "测试节目", Duration: 2400000}})
	a.pl.Hold(12 * time.Second)
	a.captions.program = 65
	a.captions.model = modelState{checked: true, installed: true}
	for i := range 1200 {
		at := time.Duration(i) * 2 * time.Second
		a.captions.lines = append(a.captions.lines, lyrics.Line{At: at, End: at + time.Second, Text: fmt.Sprintf("第 %d 句", i)})
	}
	old := captionsAvailable
	captionsAvailable = func() bool { return true }
	defer func() { captionsAvailable = old }()
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	slider, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("the playback slider is missing")
	}
	fingerTap(t, tt, slider.X+slider.W*0.8, slider.Y+slider.H/2)
	first, last := a.lyricList.Visible()
	cur := lyrics.Index(a.captions.lines, a.pl.Position())
	if cur < 800 || cur < first || cur > last || last-first > 20 || a.seeking {
		t.Fatalf("the scrubber left line %d with rows %d–%d visible and seeking=%t", cur, first, last, a.seeking)
	}
}

// The close button animates before returning to the mini player with playback intact.
func TestPhoneCloseButtonReturnsToMiniPlayer(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000}})
	a.pl.Hold(34 * time.Second)
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tapPhone(t, tt, "关闭正在播放")
	tt.Advance(70 * time.Millisecond)
	if !a.nowPlaying || !a.phoneClosing {
		t.Fatal("the player vanished before its close animation could run")
	}
	tt.Advance(1500 * time.Millisecond)
	tt.Frame()
	if a.nowPlaying {
		t.Fatal("the close button left the player open")
	}
	if _, ok := tt.Find("打开正在播放"); !ok {
		t.Fatal("the mini player is missing after the player closed")
	}
	if tr := a.pl.Current(); tr == nil || tr.ID != 31 || !a.pl.Playing() || a.pl.Position() != 34*time.Second {
		t.Fatal("closing the player changed the ongoing playback")
	}
}

// With NM_FRAMES set, saves the phone's interactions in motion: the push
// and edge-swipe midway, the tab lozenge sliding, the player rising, the
// lyric panes and a sheet on its way up.
func TestPhoneUXFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newPhone := func() (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.daily = []netease.Track{{ID: 1, Name: "每日第一首", Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}}}
		for i := range 6 {
			a.recPlaylists = append(a.recPlaylists, netease.Playlist{ID: int64(100 + i), Name: fmt.Sprintf("推荐歌单 %d", i+1), CoverURL: "qa-cover"})
		}
		for i := range 5 {
			a.featured = append(a.featured, netease.Playlist{ID: int64(200 + i), Name: fmt.Sprintf("精选歌单 %d", i+1), CoverURL: "qa-cover"})
		}
		for i := range 4 {
			a.charts = append(a.charts, netease.Playlist{ID: int64(300 + i), Name: fmt.Sprintf("榜单 %d", i+1), CoverURL: "qa-cover"})
		}
		a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}},
			{ID: 32, Name: "下一首歌", Duration: 180000}})
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 600)+"#backdrop"] = art
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art[netease.Sized("qa-cover", 120)] = art
		a.art.art["qa-cover"] = art
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}

	// The push midway: the playlist slides in over a dimming home.
	a, tt := newPhone()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
		Tracks: []netease.Track{{ID: 11, Name: "第一首"}, {ID: 12, Name: "第二首"}}}
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	tt.Advance(140 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-push-mid")

	// A finger dragging the page back by the left edge.
	a, tt = newPhone()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
		Tracks: []netease.Track{{ID: 11, Name: "第一首"}, {ID: 12, Name: "第二首"}}}
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	touch := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
	})
	touch.FingerDown(10, 400)
	tt.Advance(50 * time.Millisecond)
	touch.FingerMove(150, 400)
	tt.Frame()
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-edge-swipe-mid")

	// The tab lozenge on its way between tabs.
	a, tt = newPhone()
	a.switchPhoneTab(pageLibrary)
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-tab-lozenge-mid")

	// The player moving between the mini player and its full-screen view.
	a, tt = newPhone()
	tapPhone(t, tt, "打开正在播放")
	tt.Advance(140 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-nowplaying-open-mid")
	tt.Advance(400 * time.Millisecond)
	tapPhone(t, tt, "关闭正在播放")
	tt.Advance(140 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-nowplaying-close-mid")

	// The lyrics with the compact header.
	a, tt = newPhone()
	a.nowPlaying, a.phoneDetails = true, true
	a.pl.SetLyrics(lyrics.Build("[00:00]第一句歌词\n[00:16]第二句歌词\n[00:30]第三句歌词\n[00:45]第四句歌词", "", ""))
	a.pl.Hold(18 * time.Second)
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-lyrics-compact")

	// Landscape has its own two-column layout and uses the smoky backdrop.
	a.pl.SetLyrics(lyrics.Build("[00:00]夜色落在窗边\n[00:16]让明天的风吹散未说出口的话\n[00:30]I still remember how the sky looked that night\n[00:45]天亮以后", "[00:30]我还记得那天夜里的天空", ""))
	tt.SetSize(844, 390)
	tt.SetInsets(0, 47, 21, 47)
	tt.Frame()
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-lyrics-landscape")

	// A sheet on its way up.
	a, tt = newPhone()
	a.nowPlaying, a.phoneSheet = true, phoneSheetQueue
	tt.Frame()
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	save(tt, "ux-sheet-mid")
}

// The home's search capsule opens the 搜索 tab already editing.
func TestPhoneHomeSearchCapsuleStartsEditing(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.featured = []netease.Playlist{{ID: 9, Name: "精选歌单"}}
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "搜索歌曲、歌手、播客")
	if a.phoneTab != pageHome || a.route.page != pageSearch || !a.phoneSearchEditing {
		t.Fatalf("the capsule left tab %v, route %+v, editing %v", a.phoneTab, a.route, a.phoneSearchEditing)
	}
}

// The 我的 tabs swap the created and the saved lists, and + only offers a new playlist on 创建.
func TestPhoneMineTabsSwitchLists(t *testing.T) {
	a := editTestApp()
	a.phone = true
	a.route, a.phoneTab = route{page: pageLibrary}, pageLibrary
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	if _, found := tt.Find("新建歌单"); !found {
		t.Fatal("the 创建 tab hid the new playlist button")
	}
	if _, found := tt.Find("通勤"); !found {
		t.Fatal("the 创建 tab did not list the listener's own playlist")
	}
	tapPhone(t, tt, "收藏歌单")
	tt.Frame()
	if a.phoneMineTab != 1 {
		t.Fatal("the tab did not switch to 收藏")
	}
	if _, found := tt.Find("新建歌单"); found {
		t.Fatal("the 收藏 tab kept the new playlist button")
	}
	if _, found := tt.Find("飙升榜"); !found {
		t.Fatal("the 收藏 tab did not list the saved playlist")
	}
	tapPhone(t, tt, "创建歌单")
	tt.Frame()
	if _, found := tt.Find("通勤"); !found {
		t.Fatal("the 创建 tab did not bring the listener's playlist back")
	}
}

// The playlist action bar's 分享 puts the playlist's link on the clipboard.
func TestPhonePlaylistShareCopiesLink(t *testing.T) {
	a := editTestApp()
	a.phone = true
	copied := make(chan string, 1)
	old := copyText
	copyText = func(text string) error { copied <- text; return nil }
	defer func() { copyText = old }()
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "分享")
	select {
	case text := <-copied:
		if text != "https://music.163.com/playlist?id=10" {
			t.Fatalf("copied %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sharing copied nothing")
	}
}

// A phone settings switch toggles its setting.
func TestPhoneSettingsSwitchToggles(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	a.route = route{page: pageSettings}
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	// 动态封面与背景's handler saves and redraws; the rows whose handler
	// applies the appearance block under the tester, which has no theme.
	row, found := tt.Find("动态封面与背景")
	if !found {
		t.Fatal("the settings page has no 动态封面与背景 row")
	}
	// The label also names the row's text; the switch is at its right edge.
	y := row.Y + row.H/2
	touch.FingerDown(335, y)
	touch.FingerUp(335, y)
	tt.Frame()
	if a.settings.MotionArtwork {
		t.Fatal("the switch did not turn 动态封面与背景 off")
	}
	touch.FingerDown(335, y)
	touch.FingerUp(335, y)
	tt.Frame()
	if !a.settings.MotionArtwork {
		t.Fatal("the switch did not turn 动态封面与背景 back on")
	}
}

// With NM_FRAMES set, saves the phone's user page, the search-cancel
// states and the pages whose grids changed to two columns.
func TestPhoneFixUserSearchFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newPhone := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art[netease.Sized("qa-cover", 160)] = art
		a.art.art[netease.Sized("qa-cover", 120)] = art
		a.art.art["qa-cover"] = art
		a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}}})
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}

	// A user's own page, with everything its tabs can show.
	user := func(a *app) {
		a.profile = &netease.Profile{UserID: 9, Nickname: "听友"}
		a.users[9] = &userData{
			user: &netease.User{UserID: 9, Nickname: "听友", Signature: "在音乐里，寻找同路的人。",
				Follows: 12, Followeds: 24, AvatarURL: "qa-cover"},
			playlists: []netease.Playlist{
				{ID: 30, Name: "通勤路上", TrackCount: 40, CoverURL: "qa-cover"},
				{ID: 31, Name: "睡前", TrackCount: 12, CoverURL: "qa-cover"},
			},
			artists:       []netease.ArtistProfile{{Artist: netease.Artist{ID: 7, Name: "MyGO!!!!!"}, Avatar: "qa-cover"}},
			artistsShown:  true,
			podcasts:      []netease.Podcast{{ID: 5, Name: "夜航电台", ProgramCount: 120, PicURL: "qa-cover"}},
			podcastsShown: true,
		}
		a.route = route{page: pageUser, id: 9}
		a.history = []route{{page: pageLibrary}}
	}
	a, tt := newPhone(false)
	user(a)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-user-light")
	a, tt = newPhone(true)
	user(a)
	tt.Frame()
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-user-dark")

	// The artists tab of it.
	tapPhone(t, tt, "关注的歌手")
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-user-artists-dark")

	// 搜索 editing with the suggestion popup open, and after 返回 pops.
	a, tt = newPhone(false)
	a.route, a.phoneTab = route{page: pageSearch}, pageHome
	a.history = []route{{page: pageHome}}
	old := searchSuggest
	searchSuggest = func(context.Context, string) (netease.Suggestions, error) { return netease.Suggestions{}, nil }
	defer func() { searchSuggest = old }()
	defer a.dismissSuggestions()
	tt.Frame()
	tapPhone(t, tt, "搜索")
	tt.Type("春日")
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-search-back-before")
	tapPhone(t, tt, "返回")
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-search-back-after")

	// The grids that now keep two columns on the phone.
	grids := []struct {
		name string
		goTo func(a *app)
	}{
		{"phone-charts-grid", func(a *app) {
			for i := range 9 {
				a.charts = append(a.charts, netease.Playlist{ID: int64(300 + i), Name: fmt.Sprintf("榜单 %d", i+1), UpdateFreq: "每日更新",
					Preview: []netease.ChartLine{{First: "第一首", Second: "歌手甲"}, {First: "第二首", Second: "歌手乙"}, {First: "第三首", Second: "歌手丙"}}})
			}
			a.route = route{page: pageCharts}
		}},
		{"phone-podcasts-grid", func(a *app) {
			var hot []netease.Podcast
			for i, name := range []string{"深夜书店", "城市漫游", "慢慢说", "周末早餐", "通勤十分钟", "老唱片"} {
				hot = append(hot, netease.Podcast{ID: int64(60 + i), Name: name, DJ: netease.User{Nickname: "主播"}, PicURL: "qa-cover", RcmdText: "陪你度过每个夜晚", PlayCount: float64(1e6 * (i + 1))})
			}
			a.podcastTab = &podcastTab{hot: hot, hotPage: paged{loaded: true}}
			a.route, a.phoneTab = route{page: pagePodcasts}, pagePodcasts
		}},
		{"phone-search-artists-grid", func(a *app) {
			r := route{page: pageSearch, query: "MyGO", kind: netease.SearchArtists}
			a.searched, a.query = r, "MyGO"
			a.searches[r] = &searchData{page: paged{loaded: true}, res: netease.SearchResult{Total: 4, Artists: []netease.ArtistProfile{
				{Artist: netease.Artist{ID: 7, Name: "MyGO!!!!!"}},
				{Artist: netease.Artist{ID: 10, Name: "Ave Mujica"}},
				{Artist: netease.Artist{ID: 11, Name: "Poppin'Party"}},
				{Artist: netease.Artist{ID: 12, Name: "Roselia"}},
			}}}
			a.route, a.phoneTab = r, pageHome
		}},
		{"phone-search-albums-grid", func(a *app) {
			r := route{page: pageSearch, query: "MyGO", kind: netease.SearchAlbums}
			a.searched, a.query = r, "MyGO"
			a.searches[r] = &searchData{page: paged{loaded: true}, res: netease.SearchResult{Total: 4, Albums: []netease.AlbumProfile{
				{Album: netease.Album{ID: 8, Name: "迷跡波", PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
				{Album: netease.Album{ID: 9, Name: "跡暖空", PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
				{Album: netease.Album{ID: 10, Name: "壹雫空", PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
				{Album: netease.Album{ID: 11, Name: "端程山", PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}},
			}}}
			a.route, a.phoneTab = r, pageHome
		}},
		{"phone-artist-albums-grid", func(a *app) {
			var albums []netease.AlbumProfile
			for i, name := range []string{"迷跡波", "跡暖空", "壹雫空", "端程山"} {
				albums = append(albums, netease.AlbumProfile{Album: netease.Album{ID: int64(8 + i), Name: name, PicURL: "qa-cover"}})
			}
			a.artists[7] = &artistData{profile: &netease.ArtistProfile{
				Artist: netease.Artist{ID: 7, Name: "MyGO!!!!!"}, Avatar: "qa-cover",
			}, albums: albums, albumsPage: paged{loaded: true}}
			a.artistTab = artistAlbums
			a.route = route{page: pageArtist, id: 7}
		}},
	}
	for _, s := range grids {
		a, tt = newPhone(false)
		s.goTo(a)
		a.history = []route{{page: pageLibrary}}
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		save(tt, s.name)
	}
}

// With NM_FRAMES set, saves the phone search page's NetEase look: idle in
// both appearances, editing with an empty and a typed query, and the
// 热搜榜 shimmer while it loads.
func TestPhoneSearchNeteaseFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newPhone := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.settings = defaultSettings()
		a.settings.SearchHistory = []string{"春日影", "迷星叫", "MyGO!!!!!", "Ave Mujica", "雑踏、僕らの街", "夜に駆ける", "アイドル", "八仙饭店"}
		a.hotAsked = true
		a.hotSearches = []netease.HotSearch{
			{Word: "哪里都是你", Icon: 1}, {Word: "我不难过"}, {Word: "慌啥子嘛", Icon: 2}, {Word: "春日影"},
			{Word: "迷星叫"}, {Word: "跳楼机", Icon: 4}, {Word: "唯一"}, {Word: "若月亮没来"},
			{Word: "罗生门"}, {Word: "暮色回响"},
		}
		for i := range 6 {
			a.charts = append(a.charts, netease.Playlist{ID: int64(300 + i), Name: fmt.Sprintf("榜单 %d", i+1), CoverURL: "qa-cover"})
		}
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art["qa-cover"] = art
		a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 90000, Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "歌手甲"}}}})
		a.route, a.phoneTab = route{page: pageSearch}, pageHome
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}

	a, tt := newPhone(false)
	save(tt, "phone-search-idle-light")
	a, tt = newPhone(true)
	save(tt, "phone-search-idle-dark")

	// Editing with the query empty: the same body.
	a, tt = newPhone(false)
	tapPhone(t, tt, "搜索")
	tt.Frame()
	save(tt, "phone-search-editing-empty")

	// Editing with a query typed: the suggestion popup over the body.
	a, tt = newPhone(false)
	old := searchSuggest
	searchSuggest = func(context.Context, string) (netease.Suggestions, error) { return netease.Suggestions{}, nil }
	defer func() { searchSuggest = old }()
	defer a.dismissSuggestions()
	tapPhone(t, tt, "搜索")
	tt.Type("春日")
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	save(tt, "phone-search-editing-typed")

	// The 热搜榜 shimmer while its list loads.
	a, tt = newPhone(false)
	a.hotAsked, a.hotLoading, a.hotSearches = true, true, nil
	tt.Frame()
	save(tt, "phone-search-hot-loading")
}

// Dragging a finger along the tab capsule from 推荐 to 我的 releases onto 我的 and selects it.
func TestPhoneTabDragSelectsLibrary(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageHome}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	from, ok1 := tt.Find("推荐")
	to, ok2 := tt.Find("我的")
	if !ok1 || !ok2 {
		t.Fatal("tab labels not found")
	}
	dragPhone(t, tt, from.X+from.W/2, from.Y+from.H/2, to.X+to.W/2-(from.X+from.W/2), 0)
	if a.phoneTab != pageLibrary || a.route.page != pageLibrary {
		t.Fatalf("dragging 推荐 to 我的 left tab %v at %v", a.phoneTab, a.route)
	}
	// Once the selection has come to rest the bar asks for no more frames.
	for i := 0; i < 120 && tt.FramePending(); i++ {
		tt.Advance(33 * time.Millisecond)
		tt.Frame()
	}
	if tt.FramePending() {
		t.Fatal("frames stayed pending after the tab switch settled")
	}
}

// A drag that wanders but ends back over 推荐 commits no new tab.
func TestPhoneTabDragBackKeepsTab(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageHome}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	from, ok1 := tt.Find("推荐")
	to, ok2 := tt.Find("我的")
	if !ok1 || !ok2 {
		t.Fatal("tab labels not found")
	}
	x0, y := from.X+from.W/2, from.Y+from.H/2
	x1 := to.X + to.W/2
	touch.FingerDown(x0, y)
	for step := 1; step <= 4; step++ {
		tt.Advance(24 * time.Millisecond)
		touch.FingerMove(x0+(x1-x0)*float32(step)/4, y)
	}
	for step := 1; step <= 4; step++ {
		tt.Advance(24 * time.Millisecond)
		touch.FingerMove(x1-(x1-x0)*float32(step)/4, y)
	}
	tt.Advance(150 * time.Millisecond)
	touch.FingerUp(x0, y)
	tt.Advance(time.Second)
	tt.Frame()
	if a.phoneTab != pageHome || a.route.page != pageHome {
		t.Fatalf("a drag that returned to 推荐 left tab %v at %v", a.phoneTab, a.route)
	}
}

// Signed out, the 动态 tab offers sign-in, which opens the login dialog.
func TestPhoneFeedLoggedOutShowsLogin(t *testing.T) {
	a := newTestApp()
	a.phone = true
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "动态")
	if a.phoneTab != pageFeed || a.route.page != pageFeed {
		t.Fatalf("the 动态 tab left tab %v at %v", a.phoneTab, a.route)
	}
	tapPhone(t, tt, "登录")
	if !a.login.open {
		t.Fatal("登录 did not open the login dialog")
	}
	a.login.open = false
}

// Tapping the 动态 tab again at its root reloads the followed feed.
func TestPhoneFeedRetapReloads(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 1}
	called := make(chan int64, 4)
	old := followFeed
	followFeed = func(_ int, before int64) ([]netease.Event, int64, bool, error) {
		called <- before
		return nil, 0, false, nil
	}
	defer func() { followFeed = old }()
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "动态")
	if first := wait(t, called, "the first feed page"); first != -1 {
		t.Fatalf("the first feed page asked for before=%d, want -1", first)
	}
	// The page's own 动态 title takes the label now: tap the dock slot,
	// one to 我的's left.
	mine, ok := tt.Find("我的")
	if !ok {
		t.Fatal("the 我的 tab was missing")
	}
	fingerTap(t, tt, mine.X-mine.W/2, mine.Y+mine.H/2)
	if before := wait(t, called, "the feed reload"); before != -1 {
		t.Fatalf("the re-tap reload asked for before=%d, want -1", before)
	}
}

// Tapping an event's author on the 动态 tab opens their page.
func TestPhoneFeedEventOpensUser(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 1}
	a.route, a.phoneTab = route{page: pageFeed}, pageFeed
	a.feedPage.loaded = true
	a.feedEvents = []netease.Event{{ID: 5, Time: 1e12, User: netease.User{UserID: 77, Nickname: "某乐迷"}}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tapPhone(t, tt, "某乐迷")
	if a.route != (route{page: pageUser, id: 77}) {
		t.Fatalf("tapping the event's author left route %+v", a.route)
	}
}

// A long feed leaves distant avatars unloaded until a scroll brings them into view and requests the next page.
func TestPhoneFeedLoadsVisibleEvents(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab, a.route = true, pageFeed, route{page: pageFeed}
	a.profile = &netease.Profile{UserID: 1}
	a.feedPage = paged{loaded: true, more: true}
	a.feedBefore = 42
	a.art.fetch = func(string) ([]byte, error) { return nil, nil }
	for i := range 60 {
		a.feedEvents = append(a.feedEvents, netease.Event{ID: int64(i + 1), User: netease.User{
			UserID: 1, Nickname: fmt.Sprintf("乐迷 %d", i), AvatarURL: fmt.Sprintf("avatar-%d", i),
		}})
	}
	called := make(chan int64, 1)
	old := followFeed
	followFeed = func(_ int, before int64) ([]netease.Event, int64, bool, error) {
		called <- before
		return nil, 0, false, nil
	}
	defer func() { followFeed = old }()
	tt := ui.NewTester(a.view, 390, 844)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	first := netease.Sized(a.feedEvents[0].User.AvatarURL, 120)
	last := netease.Sized(a.feedEvents[len(a.feedEvents)-1].User.AvatarURL, 120)
	if !a.art.busy[first] || a.art.busy[last] || a.feedPage.loading {
		t.Fatal("opening the feed did not limit loading to nearby events")
	}
	tt.Scroll(180, 420, 0, 12000)
	tt.Advance(time.Second)
	tt.Frame()
	if !a.art.busy[last] {
		t.Fatal("scrolling to the end did not load its avatars")
	}
	if before := wait(t, called, "the next feed page"); before != 42 {
		t.Fatalf("the next feed page used before=%d, want 42", before)
	}
}

// The Now Playing bottom row's 评论 opens the comments sheet for the song.
func TestPhonePlayerCommentsButton(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 11, Name: "第一首", Duration: 90000}})
	old := threadComments
	threadComments = func(string, int, int) (*netease.Comments, error) {
		return &netease.Comments{Total: 3}, nil
	}
	defer func() { threadComments = old }()
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	tapPhone(t, tt, "评论")
	if a.phoneSheet != phoneSheetComments {
		t.Fatalf("评论 left the sheet %v, want comments", a.phoneSheet)
	}
}

// The home capsule pushes the search page onto 推荐's stack, with the 推荐 tab still selected.
func TestPhoneSearchCapsulePushesSearch(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.featured = []netease.Playlist{{ID: 9, Name: "精选歌单"}}
	tt := ui.NewTester(a.view, 390, 844)
	tapPhone(t, tt, "搜索歌曲、歌手、播客")
	if a.phoneTab != pageHome || a.route.page != pageSearch || len(a.history) != 1 {
		t.Fatalf("the capsule left tab %v, route %+v, history %d", a.phoneTab, a.route, len(a.history))
	}
}

// Cancelling a tab drag leaves the original page active and the next tab tap usable.
func TestPhoneTabCancelledDragKeepsTab(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageHome}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	from, ok := tt.Find("推荐")
	if !ok {
		t.Fatal("推荐 tab missing")
	}
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerCancel()
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch cancellation")
	}
	x, y := from.X+from.W/2, from.Y+from.H/2
	touch.FingerDown(x, y)
	tt.Advance(24 * time.Millisecond)
	touch.FingerMove(x+100, y)
	touch.FingerCancel()
	tt.Frame()
	if a.phoneTab != pageHome || a.route.page != pageHome {
		t.Fatal("the cancelled drag switched tabs")
	}
	tapPhone(t, tt, "我的")
	if a.phoneTab != pageLibrary || a.route.page != pageLibrary {
		t.Fatal("the cancelled drag blocked the next tab tap")
	}
}

// A mostly-vertical slide that starts on a tab is a scroll, not a
// drag-select: releasing it commits nothing.
func TestPhoneTabVerticalSlideKeepsTab(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.route = route{page: pageHome}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	from, ok1 := tt.Find("推荐")
	if !ok1 {
		t.Fatal("tab label not found")
	}
	dragPhone(t, tt, from.X+from.W/2, from.Y+from.H/2, 60, -120)
	if a.phoneTab != pageHome || a.route.page != pageHome {
		t.Fatalf("a vertical slide on a tab left tab %v at %v", a.phoneTab, a.route)
	}
}

// With NM_FRAMES set, saves the Liquid Glass tab bar: at rest in light
// and dark over the colorful playlist page, mid-switch at about a
// third, two thirds and nine tenths of the spring, mid-drag between
// slots and with 搜索 selected.
func TestPhoneTabBarFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// Home, where the search capsule lives.
	newOnHome := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.featured = []netease.Playlist{{ID: 9, Name: "精选歌单", PicURL: "qa-cover"}}
		a.settings = defaultSettings()
		a.settings.SearchHistory = []string{"春日影", "迷跡波"}
		a.hotAsked, a.hotSearches = true, []netease.HotSearch{
			{Word: "哪里都是你", Icon: 4}, {Word: "我不难过"}, {Word: "慌啥子嘛", Icon: 2}}
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 150, G: 60, B: 110, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(150, 60, 110)}
		a.art.art["qa-cover"] = art
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}
	// A playlist page gives the glass colorful content to bend.
	newOnPlaylist := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
			Tracks: []netease.Track{{ID: 11, Name: "第一首"}, {ID: 12, Name: "第二首"}, {ID: 13, Name: "第三首"}}}
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art["qa-cover"] = art
		a.route = route{page: pagePlaylist, id: 42}
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}
	// Rest, light and dark.
	_, tt := newOnPlaylist(false)
	save(tt, "tabbar-rest-light")
	_, tt = newOnPlaylist(true)
	save(tt, "tabbar-rest-dark")
	// Mid-switch 推荐→我的, sampled from the rendered selection motion.
	_, tt = newOnPlaylist(false)
	tapPhone(t, tt, "我的")
	for i := range 3 {
		tt.Advance(55 * time.Millisecond)
		tt.Frame()
		save(tt, fmt.Sprintf("tabbar-switch-%d", (i+1)*55))
	}
	// Mid-drag, finger held between 播客 and 我的.
	_, tt = newOnPlaylist(false)
	touch, ok := any(tt).(interface {
		FingerDown(float32, float32)
		FingerMove(float32, float32)
		FingerUp(float32, float32)
	})
	if !ok {
		t.Skip("the desktop MyGo snapshot has no touch input")
	}
	from, _ := tt.Find("推荐")
	to, _ := tt.Find("我的")
	x0, y := from.X+from.W/2, from.Y+from.H/2
	touch.FingerDown(x0, y)
	tt.Advance(50 * time.Millisecond)
	tt.Frame()
	touch.FingerMove(x0+(to.X+to.W/2-x0)*0.55, y)
	tt.Advance(24 * time.Millisecond)
	tt.Frame()
	save(tt, "tabbar-mid-drag")
	touch.FingerUp(x0+(to.X+to.W/2-x0)*0.55, y)
	tt.Advance(time.Second)
	tt.Frame()
	// Home with its search capsule, and the pushed search page with 推荐 lit.
	_, tt = newOnHome(false)
	save(tt, "home")
	_, tt = newOnHome(false)
	tapPhone(t, tt, "搜索歌曲、歌手、播客")
	tt.Advance(600 * time.Millisecond)
	tt.Frame()
	save(tt, "search-idle")
}

// TestPhoneFeedLyricsFrames captures the four-tab bar, the 动态 tab's states,
// the lyrics seek hint and the Now Playing comment button.
func TestPhoneFeedLyricsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect the phone UI")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	save := func(tt *ui.Tester, name string) {
		t.Helper()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	newOnPlaylist := func(dark bool) (*app, *ui.Tester) {
		a := newTestApp()
		a.phone = true
		a.playlists[42] = &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover",
			Tracks: []netease.Track{{ID: 11, Name: "第一首"}, {ID: 12, Name: "第二首"}, {ID: 13, Name: "第三首"}}}
		img := image.NewRGBA(image.Rect(0, 0, 160, 160))
		draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 125, B: 150, A: 255}), image.Point{}, draw.Src)
		art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(40, 125, 150)}
		a.art.art[netease.Sized("qa-cover", 600)] = art
		a.art.art[netease.Sized("qa-cover", 400)] = art
		a.art.art["qa-cover"] = art
		a.route = route{page: pagePlaylist, id: 42}
		tt := ui.NewTester(a.view, 390, 844)
		tt.SetDark(dark)
		tt.SetInsets(47, 0, 34, 0)
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Frame()
		return a, tt
	}
	// The four-tab bar at rest.
	_, tt := newOnPlaylist(false)
	save(tt, "tabbar-rest-light")
	_, tt = newOnPlaylist(true)
	save(tt, "tabbar-rest-dark")

	// 动态 signed out.
	a := newTestApp()
	a.phone = true
	a.route, a.phoneTab = route{page: pageFeed}, pageFeed
	tt = ui.NewTester(a.view, 390, 844)
	tt.SetInsets(47, 0, 34, 0)
	tt.Frame()
	save(tt, "feed-logged-out")

	// 动态 with events: words, a shared song and a shared playlist.
	a = newTestApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 1}
	a.route, a.phoneTab = route{page: pageFeed}, pageFeed
	a.feedPage.loaded = true
	a.feedEvents = []netease.Event{
		{ID: 1, Time: time.Now().Add(-time.Hour).UnixMilli(), User: netease.User{UserID: 7, Nickname: "某乐迷"},
			Content: netease.EventContent{Msg: "这首最近一直在循环"}},
		{ID: 2, Time: time.Now().Add(-3 * time.Hour).UnixMilli(), User: netease.User{UserID: 8, Nickname: "另一个乐迷"},
			Content: netease.EventContent{Msg: "新专辑来了", Song: &netease.Track{ID: 11, Name: "春日影", Album: netease.Album{PicURL: "qa-cover"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}}}}},
		{ID: 3, Time: time.Now().Add(-24 * time.Hour).UnixMilli(), User: netease.User{UserID: 9, Nickname: "电台朋友"},
			Content: netease.EventContent{Playlist: &netease.Playlist{ID: 42, Name: "深夜书房", CoverURL: "qa-cover"}}},
	}
	img := image.NewRGBA(image.Rect(0, 0, 160, 160))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 150, G: 60, B: 110, A: 255}), image.Point{}, draw.Src)
	art := &artwork{bmp: ui.NewBitmap(img), backdrop: ui.NewBitmap(img), tint: ui.RGB(150, 60, 110)}
	a.art.art["qa-cover"] = art
	tt = ui.NewTester(a.view, 390, 844)
	tt.SetInsets(47, 0, 34, 0)
	tt.Frame()
	save(tt, "feed-events")

	// The lyrics' seek hint, mid-scroll.
	a = newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 31, Name: "正在播放的歌", Duration: 360000}})
	lyr := ""
	for i := range 18 {
		lyr += fmt.Sprintf("[%02d:%02d]第 %d 句歌词\n", i/6, i%6*10, i+1)
	}
	a.pl.SetLyrics(lyrics.Build(lyr, "", ""))
	a.pl.Hold(105 * time.Second)
	old := threadComments
	threadComments = func(string, int, int) (*netease.Comments, error) { return &netease.Comments{Total: 999}, nil }
	defer func() { threadComments = old }()
	tt = ui.NewTester(a.view, 390, 844)
	tt.SetInsets(47, 0, 34, 0)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "歌词")
	dragPhone(t, tt, 195, 400, 0, -160)
	tt.Frame()
	save(tt, "lyrics-seek-hint")

	// The Now Playing bottom row's comment button, with the count.
	a.lyricPaneState.seeking = false
	a.comments = &commentState{track: 31, thread: "R_SO_4_31", loaded: true, total: 999}
	tapPhone(t, tt, "返回封面")
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	save(tt, "nowplaying-comments")

	// The search header's back without 取消.
	a, tt = newOnPlaylist(false)
	a.route, a.phoneTab = route{page: pageSearch}, pageHome
	a.history = []route{{page: pageHome}}
	tapPhone(t, tt, "搜索")
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	save(tt, "search-header")
}
