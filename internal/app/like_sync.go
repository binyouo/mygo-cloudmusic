package app

import (
	"maps"
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
	timer    *time.Timer
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
		return // Keep the last good state while offline.
	}
	liked := make(map[int64]bool, len(ids))
	for _, id := range ids {
		liked[id] = true
	}
	changed := a.liked == nil || !maps.Equal(a.liked, liked)
	a.liked = liked
	if changed {
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
	go func() {
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
	delete(a.likeSync.changes, change.id)
	if len(a.likeSync.changes) == 0 {
		a.refreshLikedSongs()
	}
}

// Poll while the app is in front so a phone's changes appear without
// switching windows. This uses a timer, never continuous rendering.
const likedRefreshInterval = 30 * time.Second

func (a *app) watchLikedSongs() {
	a.likeSync.timer = time.AfterFunc(likedRefreshInterval, func() {
		a.post(func() {
			if a.quitting {
				return
			}
			if a.profile != nil && (a.win.IsFocused() || a.wallpaper.active) {
				a.loadLiked(a.profile.UserID)
			}
			a.watchLikedSongs()
		})
	})
}
