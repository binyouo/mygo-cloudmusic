package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func TestCommentTime(t *testing.T) {
	now := time.Date(2026, 10, 25, 22, 0, 0, 0, time.Local)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "刚刚"},
		{now.Add(-42 * time.Minute), "42分钟前"},
		{now.Add(-5 * time.Hour), "17:00"},
		{now.Add(-23 * time.Hour), "昨天 23:00"},
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.Local), "3月8日"},
		{time.Date(2021, 4, 10, 9, 0, 0, 0, time.Local), "2021年4月10日"},
	} {
		if got := commentTime(c.t, now); got != c.want {
			t.Errorf("%v: %q, want %q", c.t, got, c.want)
		}
	}
}

// Heart mode playing, its card and Now Playing offer to leave it, which
// keeps the song playing.
func TestExitHeartMode(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1, Nickname: "听众"}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "晚风"}, {ID: 2, Name: "推荐的歌"}})
	a.pl.AttachRadio(heartRadio, func(got func([]netease.Track)) {})
	a.route = route{page: pageFM}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("退出心动模式"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current().ID != 1 || len(a.pl.Queue()) != 1 {
		t.Fatalf("radio %q, current %v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
	if !tt.HasText("开启心动模式") {
		t.Errorf("card still on: %q", tt.Texts())
	}

	a.pl.AttachRadio(heartRadio, func(got func([]netease.Track)) {})
	a.nowPlaying = true
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("退出心动模式"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Radio() != "" || a.pl.Current().ID != 1 {
		t.Fatalf("from Now Playing: radio %q, current %v", a.pl.Radio(), a.pl.Current())
	}
}

// A canceled tune and an earlier mode request cannot take over a restarted FM.
func TestPhoneFMDiscardsEarlierTune(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.settings = defaultSettings()
	a.route = route{page: pageFM}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "当前歌曲"}})

	requested := make(chan string, 3)
	personal := fetchPersonalFM
	defer func() { fetchPersonalFM = personal }()
	fetchPersonalFM = func(mode string) ([]netease.Track, error) {
		requested <- mode
		return []netease.Track{{ID: 9}}, nil
	}
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tapPhone(t, tt, "开始收听")
	canceled := a.radioStarting
	if mode := wait(t, requested, "first tune"); mode != netease.FMDefault {
		t.Fatalf("first tune requested %q", mode)
	}
	tt.Frame()
	tapPhone(t, tt, "取消调频")
	if a.radioStarting != nil || a.pl.Radio() != "" || a.pl.Current().ID != 7 {
		t.Fatal("canceling the tune changed playback")
	}
	tt.Frame()
	tapPhone(t, tt, "开始收听")
	restarted := a.radioStarting
	if mode := wait(t, requested, "restarted tune"); mode != netease.FMDefault {
		t.Fatalf("restarted tune requested %q", mode)
	}
	feed := func(func([]netease.Track)) {}
	a.fmStarted(canceled, []netease.Track{{ID: 9}}, feed)
	if a.radioStarting != restarted || a.pl.Radio() != "" || a.pl.Current().ID != 7 {
		t.Fatal("the canceled response took over the restarted tune")
	}
	tt.Frame()
	tapPhone(t, tt, "探索")
	exploring := a.radioStarting
	if mode := wait(t, requested, "explore tune"); mode != netease.FMExplore {
		t.Fatalf("switching modes requested %q", mode)
	}
	a.fmStarted(restarted, []netease.Track{{ID: 10}}, feed)
	if a.radioStarting != exploring || a.pl.Radio() != "" || a.pl.Current().ID != 7 {
		t.Fatal("an earlier response changed playback or cleared the latest tune")
	}
	a.fmStarted(exploring, []netease.Track{{ID: 11, LocalPath: "/missing/explore.flac"}}, feed)
	if a.radioStarting != nil || a.pl.Radio() != fmRadio(netease.FMExplore) || a.pl.Current().ID != 11 {
		t.Fatalf("latest tune left radio %q and current %v", a.pl.Radio(), a.pl.Current())
	}
}

// Selecting a queued song while FM loads keeps the selected song when FM returns.
func TestPhoneFMTuneRespectsQueueSelection(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 7}, {ID: 8, LocalPath: "/missing/queue.flac"}})
	start := a.beginRadioStart(fmRadio(netease.FMDefault))
	a.pl.Jump(1)
	a.fmStarted(start, []netease.Track{{ID: 9}}, func(func([]netease.Track)) {})
	if a.radioStarting != nil || a.pl.Radio() != "" || a.pl.Current().ID != 8 {
		t.Fatalf("the pending tune left radio %q and current %v", a.pl.Radio(), a.pl.Current())
	}
}
