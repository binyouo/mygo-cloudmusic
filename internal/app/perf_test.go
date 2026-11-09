package app

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// BenchmarkAccountMenuOpen measures the avatar menu's first frame and any follow-up frames on an otherwise idle page.
func BenchmarkAccountMenuOpen(b *testing.B) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "菜单测试"}
	a.route = route{page: pageLocal}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	frames := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := tt.Click("菜单测试"); err != nil {
			b.Fatal(err)
		}
		tt.Frame()
		for range 60 {
			tt.Advance(time.Second / 60)
			if tt.FramePending() {
				frames++
				tt.Frame()
			}
		}
		b.StopTimer()
		tt.Key(0, ui.KeyEscape)
		tt.Advance(time.Second)
		tt.Frame()
		b.StartTimer()
	}
	b.ReportMetric(float64(frames)/float64(b.N), "followup_frames/open")
}

// Opening the avatar menu should settle immediately; its spring transition previously requested about 48 extra frames per click.
func TestAccountMenuFrameBudget(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "菜单测试"}
	a.route = route{page: pageLocal}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("菜单测试"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	frames := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			frames++
			tt.Frame()
		}
	}
	if frames > 2 {
		t.Fatalf("opening the avatar menu requested %d extra frames", frames)
	}
}

// The daily card's grooves change only about three times a second, rather than redrawing its backdrop for every record frame.
func TestDailyRippleBudget(t *testing.T) {
	start := time.Unix(0, 0)
	s := spinState{last: start, speed: 1}
	changes, before := 0, float64(0)
	for frame := 1; frame <= 360; frame++ {
		s.step(start.Add(time.Duration(frame)*time.Second/60), 1)
		at := math.Floor(s.ripple)
		if at != before {
			changes++
		}
		before = at
	}
	if changes > 19 || changes < 17 {
		t.Fatalf("daily backdrop moved %d times in six seconds", changes)
	}
}

// A ten-thousand-song playlist and its cached search stay idle after their initial layout.
func TestPlaylistSearchFrameBudget(t *testing.T) {
	for _, query := range []string{"", "歌手 0999"} {
		a, _ := playlistSearchApp(10000)
		a.playlistFilter.query = query
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Fatalf("playlist with query %q keeps requesting frames", query)
		}
	}
}

func BenchmarkPlaylistList(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			a, _ := playlistSearchApp(n)
			tt := ui.NewTester(a.view, 1200, 800)
			tt.Frame()
			tt.Advance(time.Second)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tt.Frame()
			}
		})
	}
}

// An album's loaded comments and unfocused composer stay idle once shown.
func TestAlbumCommentsFrameBudget(t *testing.T) {
	a, d := albumApp()
	a.profile = &netease.Profile{UserID: 1}
	d.tab = 1
	d.comments.appendPage(&netease.Comments{Latest: []netease.Comment{{ID: 1, Content: "好听"}}, Total: 1})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.FramePending() {
		t.Fatal("loaded album comments keep requesting frames")
	}
}

// playingApp is an app playing a track with lyrics, at a position the test
// sets with pl.Seek, and the number of times its view was built.
func playingApp() (*app, *ui.Tester, *int) {
	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 7, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	// A line every 10 seconds, sung over the first 5 of them.
	var lrc string
	for i := range 12 {
		lrc += "[0" + string(rune('0'+i*10/60)) + ":" + string(rune('0'+i*10%60/10)) + "0.00]一二三四五六\n"
	}
	lines := lyrics.Build(lrc, "", "")
	for i := range lines {
		l := &lines[i]
		l.End = l.At + 5*time.Second
		l.Words = lyrics.SpreadWords(l.Text, l.At, l.End)
	}
	a.pl.SetLyrics(lines)
	a.pl.Hold(0)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) { builds++; a.view(c) }, 1200, 800)
	return a, tt, &builds
}

// framesAt counts the frames the view asks for over a second, at 60 a
// second, with playback held at pos.
func framesAt(a *app, tt *ui.Tester, pos time.Duration) int {
	a.pl.Seek(pos)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	asked := 0
	for range 60 {
		tt.Advance(time.Second / 60)
		if tt.FramePending() {
			asked++
			tt.Frame()
		}
	}
	return asked
}

// Now Playing asks for frames only as fast as what it shows changes. The
// Tester runs timers as it settles a frame, so this counts only what does
// not change for a while: a frame log of the app (MYGO_FRAME_STATS) measures
// the rest.
func TestNowPlayingFrameBudget(t *testing.T) {
	a, tt, _ := playingApp()
	if n := framesAt(a, tt, 12*time.Second); n > 3 {
		t.Errorf("a page under the player: %d frames a second", n)
	}
	// Its rows still, the wall asks for no more than the lyrics beside it
	// do: it moves only as it is moved.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	a.nowPlaying, a.npPane = true, paneLyrics
	lyricsFrames := framesAt(a, tt, 12*time.Second)
	a.npPane = paneWall
	if n := framesAt(a, tt, 12*time.Second); n > lyricsFrames+3 {
		t.Errorf("the wall, still: %d frames a second, the lyrics %d", n, lyricsFrames)
	}
	tt.SetPreferences(ui.Preferences{})
	a.npPane = paneStage
	a.settings.StageSparks = true
	for _, s := range stageStyles {
		a.settings.StageStyle = s.id
		if n := framesAt(a, tt, 18*time.Second); n > 12 {
			t.Errorf("the stage between lines, in %s: %d frames a second", s.label, n)
		}
	}
}

// Another song's backdrop fades in over the last one's, which is then let
// go, so that the backdrop stops asking for frames.
func TestBackdropFadeEnds(t *testing.T) {
	a, tt, _ := coverApp(t)
	a.art.art[netease.Sized("a", 600)+"#backdrop"] = &artwork{tint: ui.RGB(200, 40, 40)}
	a.art.art[netease.Sized("b", 600)+"#backdrop"] = &artwork{tint: ui.RGB(40, 40, 200)}
	q := a.pl.Queue()
	q[1].Album.PicURL, q[2].Album.PicURL = "a", "b"
	tt.Frame()
	a.pl.Next()
	tt.Frame()
	if a.backdropCache.before == nil {
		t.Fatal("the backdrop changed songs without fading")
	}
	tt.Advance(backdropFade + 100*time.Millisecond)
	tt.Frame()
	if a.backdropCache.before != nil {
		t.Error("the last song's backdrop still fades after the fade")
	}
}
