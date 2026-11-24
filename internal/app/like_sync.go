package app

import (
	"maps"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
)

var (
	songsLiked     = netease.LikedIDs
	playlistDetail = netease.PlaylistDetail
)

type likeSyncState struct {
	request  *likedSongsRequest
	revision uint64
	changes  map[int64]*songLikeChange
	pins     map[int64]*likePin
	verify   *time.Timer
	work     sync.WaitGroup // like writes in flight, drained before quitting
}

type likedSongsRequest struct {
	profile  *netease.Profile
	revision uint64
}

type songLikeChange struct {
	profile              *netease.Profile
	id                   int64
	before, sent, wanted bool
}

// A likePin is a like write NetEase answered, which the next read of the
// liked list has yet to reflect: the list can lag the write, and a read
// that misses it must not unmake it. A few disagreeing reads earn the
// server's list the last word.
type likePin struct {
	profile *netease.Profile
	wanted  bool
	reads   int // reads since the write that did not agree with it
}

const (
	// The reads a pin gets to agree with the server's list before the
	// server is trusted instead, and how soon it asks again.
	likeVerifyReads    = 5
	likeVerifyInterval = 2 * time.Second
)

// loadLiked refreshes the shared heart state, coalescing overlapping reads.
func (a *app) loadLiked(uid int64) {
	if a.profile == nil || a.profile.UserID != uid || a.likeSync.request != nil || len(a.likeSync.changes) != 0 {
		return
	}
	request := &likedSongsRequest{a.profile, a.likeSync.revision}
	a.likeSync.request = request
	fetch := songsLiked
	go func() {
		ids, err := fetch(uid)
		a.post(func() { a.likedSongsAnswered(request, ids, err) })
	}()
}

func (a *app) likedSongsAnswered(request *likedSongsRequest, ids []int64, err error) {
	if a.likeSync.request != request || a.profile != request.profile {
		return
	}
	a.likeSync.request = nil
	if request.revision != a.likeSync.revision || len(a.likeSync.changes) != 0 {
		a.loadLiked(a.profile.UserID)
		return // A read started before a local click cannot undo it.
	}
	if err != nil {
		a.ageLikePins()
		return // Keep the last good state while offline.
	}
	liked := make(map[int64]bool, len(ids))
	for _, id := range ids {
		liked[id] = true
	}
	resolved := false
	for id, pin := range a.likeSync.pins {
		if pin.profile != request.profile || liked[id] == pin.wanted {
			// The list caught up with the write, or the pin is another
			// account's.
			delete(a.likeSync.pins, id)
			resolved = true
			continue
		}
		if pin.reads++; pin.reads > likeVerifyReads {
			// The server insists after all: take its list.
			delete(a.likeSync.pins, id)
			resolved = true
			continue
		}
		if pin.wanted {
			liked[id] = true
		} else {
			delete(liked, id)
		}
	}
	if len(a.likeSync.pins) != 0 {
		a.verifyLikedSoon()
	}
	changed := a.liked == nil || !maps.Equal(a.liked, liked)
	a.liked = liked
	if changed || resolved {
		a.likeSync.revision++
		a.refreshLikedPlaylist()
	}
}

// refreshLikedSongs runs on focus or a repeat click on the liked playlist.
func (a *app) refreshLikedSongs() {
	if a.profile != nil {
		a.loadLiked(a.profile.UserID)
		a.refreshLikedPlaylist()
	}
}

// Refresh a visible liked playlist in place, keeping its rows and scroll
// while the request runs. Other cached copies are forgotten until opened.
func (a *app) refreshLikedPlaylist() {
	if len(a.likeSync.changes) != 0 {
		return
	}
	if id := a.likedPlaylist(); id != 0 {
		if a.route.page == pagePlaylist && a.route.id == id {
			a.loadPlaylist(id, false, true)
		} else {
			delete(a.playlists, id)
		}
	}
}

// Consecutive clicks update the heart immediately, but writes for a song
// run in order so a slow earlier request cannot win on the server.
func (a *app) toggleSongLike(id int64) {
	if a.liked == nil {
		a.liked = map[int64]bool{}
	}
	before := a.liked[id]
	a.setLiked(id, !before)
	a.likeSync.revision++
	if change := a.likeSync.changes[id]; change != nil {
		change.wanted = !before
		return
	}
	if a.likeSync.changes == nil {
		a.likeSync.changes = map[int64]*songLikeChange{}
	}
	change := &songLikeChange{profile: a.profile, id: id, before: before, sent: !before, wanted: !before}
	a.likeSync.changes[id] = change
	a.sendSongLike(change)
}

func (a *app) sendSongLike(change *songLikeChange) {
	send, sent := likeSong, change.sent
	a.likeSync.work.Add(1)
	go func() {
		defer a.likeSync.work.Done()
		err := send(change.id, sent)
		a.post(func() { a.songLikeAnswered(change, sent, err) })
	}()
}

func (a *app) songLikeAnswered(change *songLikeChange, sent bool, err error) {
	if a.profile != change.profile || a.likeSync.changes[change.id] != change {
		return
	}
	a.likeSync.revision++
	remote := sent
	if err != nil {
		remote = change.before
	}
	if change.wanted != sent && change.wanted != remote {
		change.before, change.sent = remote, change.wanted
		a.sendSongLike(change)
		return
	}
	if change.wanted == sent && err != nil {
		a.setLiked(change.id, remote)
		a.toast = "操作失败，请稍后再试"
	}
	a.pinLike(change.id, remote)
	delete(a.likeSync.changes, change.id)
	if len(a.likeSync.changes) == 0 {
		a.refreshLikedSongs()
	}
}

// pinLike remembers what the write for id left remotely, until a read of
// the liked list agrees with it.
func (a *app) pinLike(id int64, wanted bool) {
	if a.likeSync.pins == nil {
		a.likeSync.pins = map[int64]*likePin{}
	}
	a.likeSync.pins[id] = &likePin{profile: a.profile, wanted: wanted}
}

// verifyLikedSoon asks the liked list again while pins wait for a read to
// confirm them. A read of its own, or the writes still running, ask again
// when they answer.
func (a *app) verifyLikedSoon() {
	if a.likeSync.verify != nil {
		return
	}
	a.likeSync.verify = time.AfterFunc(likeVerifyInterval, func() {
		a.post(func() {
			a.likeSync.verify = nil
			if a.quitting || a.profile == nil || len(a.likeSync.pins) == 0 {
				return
			}
			a.loadLiked(a.profile.UserID)
			if a.likeSync.request == nil {
				a.verifyLikedSoon() // a read or a write was in the way
			}
		})
	})
}

// ageLikePins counts a read that could not confirm the pins toward giving
// the server's list the last word.
func (a *app) ageLikePins() {
	for id, pin := range a.likeSync.pins {
		if pin.reads++; pin.reads > likeVerifyReads {
			delete(a.likeSync.pins, id)
		}
	}
	if len(a.likeSync.pins) != 0 {
		a.verifyLikedSoon()
	}
}
