package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// TestPhoneSessionRestoresLocalQueue uses the current sandbox for a saved song and resumes at the saved position.
func TestPhoneSessionRestoresLocalQueue(t *testing.T) {
	old, moved := t.TempDir(), t.TempDir()
	file := filepath.Join("local-media", "saved-clip", "歌曲.flac")
	mygo.App.SetPath(mygo.PathUserData, old)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	a := newTestApp()
	a.phone = true
	const position = 12 * time.Second
	a.pl.Cue([]netease.Track{{ID: -10, LocalPath: filepath.Join(old, file), Duration: 180000}})
	a.pl.Hold(position)
	a.saveSession()
	if a.pl.Queue()[0].LocalPath != filepath.Join(old, file) {
		t.Fatal("saving the queue changed the live path")
	}
	if err := os.Rename(filepath.Join(old, "session.json"), filepath.Join(moved, "session.json")); err != nil {
		t.Fatal(err)
	}
	mygo.App.SetPath(mygo.PathUserData, moved)
	restarted := newTestApp()
	restarted.phone = true
	restarted.restoreSession()
	canonical, err := filepath.EvalSymlinks(moved)
	if err != nil {
		t.Fatal(err)
	}
	if track := restarted.pl.Current(); track == nil || track.ID != -10 || track.LocalPath != filepath.Join(canonical, file) || restarted.pl.Position() != position || restarted.pl.Playing() {
		t.Fatalf("restored song = %+v, position = %v, playing = %v", track, restarted.pl.Position(), restarted.pl.Playing())
	}
}

// Startup account restoration leaves a song selected while it was loading in place.
func TestPhoneRestoreKeepsNewSelection(t *testing.T) {
	data := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, data)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })

	previous := newTestApp()
	previous.phone = true
	previous.pl.Cue([]netease.Track{{ID: 7, Name: "上次播放"}})
	previous.saveSession()

	a := newTestApp()
	a.phone = true
	a.pl.Cue([]netease.Track{{ID: 8, Name: "刚选的歌曲"}})
	const position = 12 * time.Second
	a.pl.Hold(position)
	a.restoreSession()
	if a.pl.Current().ID != 8 || a.pl.Position() != position || len(a.pl.Queue()) != 1 {
		t.Fatalf("restoration replaced the new selection: current=%+v, position=%v, queue=%v",
			a.pl.Current(), a.pl.Position(), a.pl.Queue())
	}
}

// Shuffle and repeat changes made while paused schedule a checkpoint and survive a restart.
func TestPhonePausedPlayOrderIsSaved(t *testing.T) {
	data := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, data)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	var due []func()
	after := sessionAfter
	defer func() { sessionAfter = after }()
	sessionAfter = func(_ time.Duration, fn func()) { due = append(due, fn) }

	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 7, Duration: 180000}, {ID: 8, Duration: 90000}})
	a.pl.Seek(12 * time.Second)
	a.saveSession()
	a.pl.Changed = a.keepSession
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "待播清单")
	tapPhone(t, tt, "随机播放")
	if len(due) != 1 {
		t.Fatalf("shuffle left %d saves due, want 1", len(due))
	}
	due[0]()
	if len(due) != 1 {
		t.Fatal("a paused queue scheduled a periodic save")
	}
	tapPhone(t, tt, "循环")
	if len(due) != 2 {
		t.Fatalf("repeat left %d saves scheduled in total, want 2", len(due))
	}
	due[1]()
	order, at := a.pl.PlayOrder()

	restarted := newTestApp()
	restarted.phone = true
	restarted.restoreSession()
	gotOrder, gotAt := restarted.pl.PlayOrder()
	if !restarted.pl.Shuffle() || restarted.pl.Repeat() != a.pl.Repeat() ||
		!slices.Equal(gotOrder, order) || gotAt != at || restarted.pl.Playing() ||
		restarted.pl.Position() != 12*time.Second {
		t.Fatalf("restored shuffle=%t, repeat=%v, order=%v at %d, playing=%t, position=%v",
			restarted.pl.Shuffle(), restarted.pl.Repeat(), gotOrder, gotAt, restarted.pl.Playing(), restarted.pl.Position())
	}
}

// Exiting a paused radio saves the shortened queue so a restart cannot reattach the radio.
func TestPhoneRadioExitSavesPausedQueue(t *testing.T) {
	data := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, data)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	var due func()
	after := sessionAfter
	defer func() { sessionAfter = after }()
	sessionAfter = func(_ time.Duration, fn func()) { due = fn }

	a := newTestApp()
	a.phone, a.nowPlaying = true, true
	a.pl.Cue([]netease.Track{{ID: 7, Duration: 180000}, {ID: 8, Duration: 90000}})
	a.pl.AttachRadio(heartRadio, func(func([]netease.Track)) {})
	a.saveSession()
	a.pl.Changed = a.keepSession
	tt := ui.NewTester(a.view, 390, 780)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tapPhone(t, tt, "退出心动模式")
	if due == nil {
		t.Fatal("exiting the radio did not schedule saving the queue")
	}
	due()

	restarted := newTestApp()
	restarted.phone = true
	restarted.restoreSession()
	if restarted.pl.Radio() != "" || len(restarted.pl.Queue()) != 1 ||
		restarted.pl.Current().ID != 7 || restarted.pl.Playing() {
		t.Fatalf("restored radio=%q, queue=%v, playing=%t", restarted.pl.Radio(), restarted.pl.Queue(), restarted.pl.Playing())
	}
}

// A lock-screen seek saves the new position and queue immediately while the app is in the background.
func TestPhoneNativePlaybackCheckpointsSession(t *testing.T) {
	data := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, data)
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	a := newTestApp()
	a.phone, a.phoneSuspended = true, true
	a.pl.Cue([]netease.Track{{ID: 7, Duration: 180000}, {ID: 8, Duration: 90000}})
	a.pl.Hold(12 * time.Second)
	a.saveSession()
	playing := a.pl.Playing()
	const position = 49 * time.Second
	a.pl.Seek(position)
	a.nativePlaybackChanged()

	b, err := os.ReadFile(filepath.Join(data, "session.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved player.Session
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Position != position.Milliseconds() || saved.At != 0 || len(saved.Queue) != 2 || saved.Queue[0].ID != 7 || saved.Queue[1].ID != 8 || a.pl.Playing() != playing {
		t.Fatalf("saved session = %+v, playing = %t", saved, a.pl.Playing())
	}
}

// The session is kept a while after playback changes and every while as
// it plays, and paused nothing more is due. It was kept by a ticker that
// woke the app, and redrew its window, every half minute even at rest.
func TestSessionRestsWhilePaused(t *testing.T) {
	var due []func()
	defer func(after func(time.Duration, func())) { sessionAfter = after }(sessionAfter)
	sessionAfter = func(_ time.Duration, fn func()) { due = append(due, fn) }

	a := newTestApp()
	a.pl.Changed = a.keepSession
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	out.Play()
	a.pl.Attach(out)
	defer out.Close()

	a.pl.TogglePlay()
	a.pl.TogglePlay()
	if len(due) != 1 {
		t.Fatalf("%d saves due after pausing and playing", len(due))
	}
	due[0]()
	if len(due) != 2 {
		t.Fatalf("playing, %d saves due after one, want another", len(due)-1)
	}

	a.pl.TogglePlay()
	due[1]()
	if len(due) != 2 {
		t.Errorf("paused, %d more saves due", len(due)-2)
	}
}
