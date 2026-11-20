package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// A login picks up the last stretch of listening on one device, oldest
// first: it ends where the device changes or the listener stopped for half
// an hour, keeping VIP songs for alternative streams and each song's last play.
func TestLastStretch(t *testing.T) {
	now := time.Date(2026, 10, 26, 22, 0, 0, 0, time.Local)
	play := func(id int64, ago time.Duration, device string, fee int) netease.Play {
		return netease.Play{Track: netease.Track{ID: id, Fee: fee}, At: now.Add(-ago), Device: device}
	}
	plays := []netease.Play{
		play(5, 3*time.Minute, "android", 0),
		play(4, 6*time.Minute, "android", 1), // VIP only
		play(3, 9*time.Minute, "android", 0),
		play(5, 12*time.Minute, "android", 0), // played again since
		play(2, 30*time.Minute, "android", 0),
		play(1, 70*time.Minute, "android", 0), // after a break
	}
	songs, last, ok := lastStretch(plays, now)
	var ids []int64
	for _, s := range songs {
		ids = append(ids, s.ID)
	}
	if !ok || len(ids) != 4 || ids[0] != 2 || ids[1] != 3 || ids[2] != 4 || ids[3] != 5 || last.Track.ID != 5 {
		t.Fatalf("picked %v, last %d", ids, last.Track.ID)
	}
	if got := heardAt(last, now); got != "3分钟前在手机上听过" {
		t.Errorf("heard %q", got)
	}

	plays[1] = play(4, 6*time.Minute, "osx", 0)
	if songs, _, _ := lastStretch(plays, now); len(songs) != 1 || songs[0].ID != 5 {
		t.Errorf("crossed devices: %v", songs)
	}
	plays[0] = play(5, 4, "android", 1)
	if songs, last, ok := lastStretch(plays[:1], now); !ok || len(songs) != 1 || last.Track.ID != 5 {
		t.Errorf("lost a VIP song before trying its stream: %v", songs)
	}
	if _, _, ok := lastStretch([]netease.Play{play(1, 8*24*time.Hour, "iphone", 0)}, now); ok {
		t.Error("picked up a week-old song")
	}
}

// Recent listening appears in the phone mini player until its play button is tapped.
func TestPhoneRecentPlaysResumeThroughMiniPlayer(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 7}
	a.accountRevision = 1
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	plays := []netease.Play{
		{
			Track:  netease.Track{ID: 9, Name: "继续播放", LocalPath: filepath.Join(dir, "current.flac"), Duration: 90000},
			At:     now.Add(-3 * time.Minute),
			Device: "osx",
		},
		{
			Track:  netease.Track{ID: 8, Name: "上一首", LocalPath: filepath.Join(dir, "previous.flac"), Duration: 90000},
			At:     now.Add(-6 * time.Minute),
			Device: "osx",
		},
	}
	a.applyRecentPlays(1, 7, plays, now)
	order, at := a.pl.PlayOrder()
	queue := a.pl.Queue()
	if len(order) != 2 || at != 1 || queue[order[0]].ID != 8 || a.pl.Current().ID != 9 || a.pl.Playing() {
		t.Fatalf("picked-up order=%v at %d, current=%v, playing=%t", order, at, a.pl.Current(), a.pl.Playing())
	}
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	hint := heardAt(plays[0], now)
	if !tt.HasText(hint) {
		t.Fatal("the mini player did not identify the listening being resumed")
	}
	tapPhone(t, tt, "播放/暂停")
	tt.Frame()
	if !a.pl.Playing() || a.pl.Current().ID != 9 || tt.HasText(hint) {
		t.Fatal("playing did not resume the picked-up song and dismiss its hint")
	}
}

// A recent-plays response cannot cross a new login or replace a newly selected queue.
func TestPhoneRecentPlaysLeavesNewerStateAlone(t *testing.T) {
	a := newTestApp()
	a.phone = true
	a.profile = &netease.Profile{UserID: 7}
	a.accountRevision = 2
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	plays := []netease.Play{{Track: netease.Track{ID: 9}, At: now, Device: "osx"}}
	a.applyRecentPlays(1, 7, plays, now)
	if a.pl.Current() != nil {
		t.Fatal("a response from the previous login restored its queue")
	}
	a.pl.Cue([]netease.Track{{ID: 11}})
	a.applyRecentPlays(2, 7, plays, now)
	if a.pl.Current().ID != 11 {
		t.Fatal("a recent-plays response replaced the listener's selection")
	}
}
