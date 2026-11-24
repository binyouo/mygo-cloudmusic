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

// Paused retro players request no animation frames after the tonearm has settled.
func TestRetroPlayerPausedFrameBudget(t *testing.T) {
	readyRetro()
	for _, style := range playerStyles[1:] {
		a := newTestApp()
		a.settings.PlayerStyle = style.id
		a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 180000}})
		a.pl.Seek(12 * time.Second)
		a.nowPlaying, a.npPane = true, paneQueue
		tt := ui.NewTester(a.view, 1200, 800)
		settleRetro(tt)
		tt.Advance(time.Second)
		if tt.FramePending() {
			t.Errorf("%s paused: requests frames", style.id)
		}
	}
}

// The phone pages stop asking for frames once a tab switch and a push transition have settled.
func TestPhoneTransitionFrameBudget(t *testing.T) {
	a := newTestApp()
	a.phone, a.phoneTab = true, pageHome
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "歌单甲", Tracks: []netease.Track{{ID: 11, Name: "第一首"}}}
	tt := ui.NewTester(a.view, 390, 844)
	tt.Frame()
	a.switchPhoneTab(pageLibrary)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("the phone pages keep requesting frames after a tab switch")
	}
	a.open(route{page: pagePlaylist, id: 42})
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("the phone pages keep requesting frames after a push transition")
	}
}

// Once the last word is fully lit, its line stops rebuilding the phone during the outro.
func TestPhoneLyricOutroFrameBudget(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 240000}})
	a.pl.SetLyrics([]lyrics.Line{{At: time.Second, End: 2 * time.Second, Text: "一二",
		Words: lyrics.SpreadWords("一二", time.Second, 2*time.Second)}})
	a.pl.Hold(20 * time.Second)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.lyricsPane(c, a.pl.Current(), 600)
	}, 390, 844)
	tt.Frame()
	tt.Advance(2 * time.Second)
	builds = 0
	tt.Advance(time.Second)
	if builds != 0 {
		t.Fatalf("settled lyrics rebuilt %d times in the outro", builds)
	}
}

// Playback within a sung line updates the ink without rebuilding the phone's lyrics or transport.
func TestPhoneKaraokePaintsWithoutRebuild(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌曲", LocalPath: "/test/song.mp3", Duration: 120000}})
	line := lyrics.Line{At: time.Second, End: 20 * time.Second, Text: "A long line with words still being sung"}
	line.Words = lyrics.SpreadWords(line.Text, line.At, line.End)
	a.pl.SetLyrics([]lyrics.Line{line})
	a.pl.Hold(4 * time.Second)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.view(c)
	}, 390, 844)
	tt.Advance(2 * time.Second)
	builds = 0
	for i := range 18 {
		a.pl.Hold(4*time.Second + time.Duration(i)*time.Second/30)
		tt.Advance(time.Second / 30)
	}
	if builds > 1 {
		t.Fatalf("painting one sung line rebuilt the whole view %d times", builds)
	}
}

// A new lyric line on the phone glides into place in frames that paint,
// without rebuilding the view for each of them.
func TestPhoneLyricGlideFrameBudget(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌曲", LocalPath: "/test/song.mp3", Duration: 120000}})
	var lrc string
	for i := range 12 {
		at := time.Duration(i) * 3 * time.Second
		lrc += fmt.Sprintf("[%02d:%02d.00]line %d of the song\n", int(at/time.Minute), int(at/time.Second)%60, i)
	}
	a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
	a.pl.Hold(8*time.Second + 900*time.Millisecond)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.view(c)
	}, 390, 844)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
	a.pl.Hold(9*time.Second + 100*time.Millisecond)
	builds = 0
	for range 30 {
		tt.Advance(time.Second / 60)
	}
	if builds > 4 {
		t.Fatalf("a line change rebuilt the phone's view %d times in 500ms", builds)
	}
	tt.Advance(800 * time.Millisecond)
	tt.Frame()
	first, ok := tt.Find("line 3 of the song")
	if !ok {
		t.Fatal("the line sung is not on screen")
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
	second, ok := tt.Find("line 3 of the song")
	if !ok {
		t.Fatal("the line sung is not on screen")
	}
	if dy := second.Y - first.Y; dy < -1 || dy > 1 {
		t.Fatalf("the line sung still moves after the glide: %v", dy)
	}
}

// The full-screen player rises from the mini player without rebuilding the page every frame.
func TestPhonePlayerOpenFrameBudget(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌曲", LocalPath: "/test/song.mp3", Duration: 120000}})
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.view(c)
	}, 390, 844)
	tt.Advance(time.Second)
	tapPhone(t, tt, "打开正在播放")
	tt.Advance(64 * time.Millisecond)
	builds = 0
	tt.Advance(150 * time.Millisecond)
	if !a.nowPlaying || builds > 2 {
		t.Fatalf("mid-opening: player open=%t, built %d times", a.nowPlaying, builds)
	}
	tt.Advance(500 * time.Millisecond)
	if _, ok := tt.Find("关闭正在播放"); !ok {
		t.Fatal("the full-screen player did not become interactive")
	}
}

// Dismissing the lyrics player slides it over the app without rebuilding either view every frame.
func TestPhoneLyricsDismissFrameBudget(t *testing.T) {
	a := newTestApp()
	a.phone, a.nowPlaying, a.phoneDetails = true, true, true
	a.pl.Cue([]netease.Track{{ID: 7, Name: "歌曲", LocalPath: "/test/song.mp3", Duration: 120000}})
	a.pl.SetLyrics(lyrics.Build("[00:00]The first line\n[00:20]The second line\n[00:40]The third line", "", ""))
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		a.view(c)
	}, 390, 844)
	tt.Advance(time.Second)
	a.startPhoneClose()
	tt.Frame()
	builds = 0
	tt.Advance(180 * time.Millisecond)
	if !a.nowPlaying || builds > 2 {
		t.Fatalf("mid-dismissal: player open=%t, built %d times", a.nowPlaying, builds)
	}
	tt.Advance(500 * time.Millisecond)
	if a.nowPlaying {
		t.Fatal("the player stayed open after the slide")
	}
}

// BenchmarkPhoneHomeFirstLayout measures opening a home feed with a full chart shelf.
func BenchmarkPhoneHomeFirstLayout(b *testing.B) {
	charts := make([]netease.Playlist, 70)
	for i := range charts {
		charts[i] = netease.Playlist{ID: int64(i + 1), Name: fmt.Sprintf("榜单 %d", i), UpdateFreq: "每日更新",
			Preview: []netease.ChartLine{
				{First: fmt.Sprintf("流行歌曲 %d", i), Second: "歌手甲"},
				{First: fmt.Sprintf("最新单曲 %d", i), Second: "歌手乙"},
				{First: fmt.Sprintf("经典金曲 %d", i), Second: "歌手丙"},
			}}
	}
	b.ReportAllocs()
	for b.Loop() {
		a := newTestApp()
		a.phone, a.charts = true, charts
		tt := ui.NewTester(a.view, 402, 874)
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		tt.Frame()
	}
}

// BenchmarkPhoneOpenLyrics measures the first layout of a long bilingual song mid-playback.
func BenchmarkPhoneOpenLyrics(b *testing.B) {
	lines := make([]lyrics.Line, 140)
	for i := range lines {
		lines[i] = lyrics.Line{At: time.Duration(i) * 2 * time.Second,
			Text:        fmt.Sprintf("These are all the things we remembered from night %d", i),
			Translation: fmt.Sprintf("这是第 %d 个夜晚留在记忆里的事", i)}
	}
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		a := newTestApp()
		a.settings = defaultSettings()
		a.phone, a.nowPlaying = true, true
		a.pl.Cue([]netease.Track{{ID: 31, Name: "测试曲目", Duration: 300000}})
		a.pl.SetLyrics(lines)
		a.pl.Hold(140 * time.Second)
		tt := ui.NewTester(a.view, 402, 874)
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		b.StartTimer()
		a.phoneDetails = true
		tt.Frame()
		tt.Frame()
	}
}
