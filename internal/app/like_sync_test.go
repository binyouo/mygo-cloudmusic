package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

type songLikeCall struct {
	id   int64
	like bool
}

func fakeSongSync(t *testing.T) (chan int64, chan songLikeCall, chan int64) {
	t.Helper()
	reads, writes, playlists := make(chan int64, 16), make(chan songLikeCall, 16), make(chan int64, 16)
	oldRead, oldWrite, oldPlaylist := songsLiked, likeSong, playlistDetail
	t.Cleanup(func() { songsLiked, likeSong, playlistDetail = oldRead, oldWrite, oldPlaylist })
	songsLiked = func(uid int64) ([]int64, error) { reads <- uid; return nil, nil }
	likeSong = func(id int64, like bool) error { writes <- songLikeCall{id, like}; return nil }
	playlistDetail = func(id int64) (*netease.Playlist, error) { playlists <- id; return nil, errors.New("offline") }
	return reads, writes, playlists
}

// A phone's like and unlike must change the player's actual heart action;
// the liked ID list used to be read only on login, so it stayed stale.
func TestLikedSongsRefreshFromOtherDevice(t *testing.T) {
	reads, writes, _ := fakeSongSync(t)
	for _, wasLiked := range []bool{false, true} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.profile = &netease.Profile{UserID: 1}
		a.liked = map[int64]bool{7: wasLiked}
		a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		a.refreshLikedSongs()
		request := a.likeSync.request
		if uid := wait(t, reads, "liked songs"); uid != 1 {
			t.Fatalf("requested user %d", uid)
		}
		a.loadLiked(1)
		if len(reads) != 0 {
			t.Fatal("duplicated an in-flight refresh")
		}
		var ids []int64
		label := "喜欢"
		if !wasLiked {
			ids, label = []int64{7}, "取消喜欢"
		}
		a.likedSongsAnswered(request, ids, nil)
		tt.Frame()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if got := wait(t, writes, "the heart action"); got != (songLikeCall{7, wasLiked}) {
			t.Fatalf("after phone change, sent %+v", got)
		}
		if a.liked[7] != wasLiked {
			t.Fatal("heart did not act on the refreshed state")
		}
	}
}

// Reopening the liked playlist refreshes both independent caches while
// keeping the visible rows, current song and playback position in place.
func TestLikedPlaylistRefreshKeepsPlayback(t *testing.T) {
	reads, _, playlists := fakeSongSync(t)
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1}
	a.mine = []netease.Playlist{{ID: 42, SpecialType: 5}}
	a.route = route{page: pagePlaylist, id: 42}
	old := &netease.Playlist{ID: 42, Tracks: []netease.Track{{ID: 2}}}
	a.playlists[42] = old
	a.liked = map[int64]bool{2: true}
	a.pl.Cue([]netease.Track{{ID: 2, Duration: 200000}})
	a.pl.Hold(42 * time.Second)
	a.open(a.route)
	request, revision := a.likeSync.request, a.likeSync.revision
	wait(t, reads, "the hearts")
	wait(t, playlists, "the playlist")
	if a.playlists[42] != old {
		t.Fatal("cleared the visible playlist while refreshing")
	}
	a.likedSongsAnswered(request, []int64{1}, nil)
	// A playlist answer started before the new hearts must be re-read too.
	a.playlistLoaded(42, a.profile, revision, false, old, nil)
	wait(t, playlists, "the updated playlist")
	newPlaylist := &netease.Playlist{ID: 42, Tracks: []netease.Track{{ID: 1}}}
	a.playlistLoaded(42, a.profile, a.likeSync.revision, false, newPlaylist, nil)
	if !a.liked[1] || a.liked[2] || a.playlists[42] != newPlaylist {
		t.Fatal("the hearts and liked playlist disagree after refresh")
	}
	if a.pl.Current().ID != 2 || a.pl.Position() != 42*time.Second || !a.pl.Playing() {
		t.Fatal("refresh interrupted playback")
	}
}

// A read started before clicking a heart cannot undo that click, and rapid
// like/unlike writes are serialized so a late first write cannot win remotely.
func TestLikeClickBeatsStaleRead(t *testing.T) {
	reads, writes, _ := fakeSongSync(t)
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &netease.Profile{UserID: 1}
	a.liked = map[int64]bool{}
	a.pl.Cue([]netease.Track{{ID: 7, Name: "宝船", Duration: 200000}})
	a.loadLiked(1)
	oldRead := a.likeSync.request
	wait(t, reads, "the initial read")
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("喜欢"); err != nil {
		t.Fatal(err)
	}
	change := a.likeSync.changes[7]
	if got := wait(t, writes, "the like"); !got.like {
		t.Fatal("did not like")
	}
	a.likedSongsAnswered(oldRead, nil, nil)
	if !a.liked[7] {
		t.Fatal("an old snapshot undid the like")
	}
	tt.Frame()
	if err := tt.Click("取消喜欢"); err != nil {
		t.Fatal(err)
	}
	if a.liked[7] || len(writes) != 0 {
		t.Fatal("did not update the heart immediately or sent overlapping writes")
	}
	a.songLikeAnswered(change, true, nil)
	if got := wait(t, writes, "the queued unlike"); got != (songLikeCall{7, false}) {
		t.Fatalf("queued %+v", got)
	}
	if a.liked[7] {
		t.Fatal("the first write changed the latest desired heart")
	}
	a.songLikeAnswered(change, false, nil)
	wait(t, reads, "the post-write refresh")
	a.likedSongsAnswered(a.likeSync.request, nil, nil)
	if a.liked[7] || len(a.likeSync.changes) != 0 {
		t.Fatal("unlike did not settle")
	}
}

// Failed reads keep the last good heart state; failed writes roll back,
// and responses belonging to a prior account never touch the new account.
func TestLikeSyncFailureAndAccountChange(t *testing.T) {
	reads, writes, _ := fakeSongSync(t)
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1}
	a.liked = map[int64]bool{7: true}
	a.loadLiked(1)
	wait(t, reads, "the read")
	a.likedSongsAnswered(a.likeSync.request, nil, errors.New("offline"))
	if !a.liked[7] {
		t.Fatal("a failed read cleared the hearts")
	}
	a.toggleLike(&netease.Track{ID: 7})
	change := a.likeSync.changes[7]
	wait(t, writes, "the unlike")
	a.songLikeAnswered(change, false, errors.New("refused"))
	if !a.liked[7] || len(a.likeSync.changes) != 0 {
		t.Fatal("failed unlike did not roll back")
	}
	wait(t, reads, "the rollback refresh")
	oldRead := a.likeSync.request
	a.profile = &netease.Profile{UserID: 1} // A new login, including the same UID.
	a.liked = map[int64]bool{8: true}
	a.likeSync.request, a.likeSync.changes = nil, nil
	a.loadLiked(1)
	newRead := a.likeSync.request
	wait(t, reads, "the new account read")
	a.likedSongsAnswered(oldRead, []int64{7}, nil)
	a.songLikeAnswered(change, false, errors.New("late refusal"))
	if !a.liked[8] || a.liked[7] || a.likeSync.request != newRead {
		t.Fatal("a prior login overwrote the new session")
	}
}

// A stale playlist response must not resurrect a song just unliked while
// that response was in flight, even when the same playlist remains open.
func TestLikedPlaylistResponseCannotUndoClick(t *testing.T) {
	reads, writes, playlists := fakeSongSync(t)
	a := newTestApp()
	a.profile = &netease.Profile{UserID: 1}
	a.mine = []netease.Playlist{{ID: 42, SpecialType: 5}}
	a.route = route{page: pagePlaylist, id: 42}
	old := &netease.Playlist{ID: 42, Tracks: []netease.Track{{ID: 7}}}
	a.playlists[42], a.liked = old, map[int64]bool{7: true}
	a.loadPlaylist(42, false, true)
	revision := a.likeSync.revision
	wait(t, playlists, "the first playlist read")
	a.toggleLike(&netease.Track{ID: 7})
	change := a.likeSync.changes[7]
	wait(t, writes, "the unlike")
	a.playlistLoaded(42, a.profile, revision, false, &netease.Playlist{ID: 42, Tracks: []netease.Track{{ID: 7}, {ID: 8}}}, nil)
	if a.playlists[42] != old || a.liked[7] {
		t.Fatal("stale playlist replaced the current view")
	}
	a.songLikeAnswered(change, false, nil)
	wait(t, reads, "fresh hearts")
	wait(t, playlists, "fresh playlist")
	a.playlistLoaded(42, a.profile, a.likeSync.revision, false, &netease.Playlist{ID: 42}, nil)
	if len(a.playlists[42].Tracks) != 0 || a.liked[7] {
		t.Fatal("the unliked song remained after refresh")
	}
}
