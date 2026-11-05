package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

var playlistSongs = netease.SongDetails

// A remainder belongs to one snapshot, so a refresh or login discards its replies.
type playlistRemainder struct {
	pl       *netease.Playlist
	profile  *netease.Profile
	revision uint64
	loading  bool
	err      string
	play     *playlistPlay
}

type playlistPlay struct {
	id      int64
	shuffle bool
	current *netease.Track
}

func (a *app) remainingPlaylist(pl *netease.Playlist) *playlistRemainder {
	if a.playlistMore == nil {
		a.playlistMore = map[int64]*playlistRemainder{}
	}
	d := a.playlistMore[pl.ID]
	if d == nil || d.pl != pl {
		d = &playlistRemainder{pl: pl, profile: a.profile, revision: a.likeSync.revision}
		a.playlistMore[pl.ID] = d
	}
	return d
}

func (a *app) morePlaylistSongs(d *playlistRemainder) {
	p := d.pl
	if d.loading || d.err != "" || p.TrackOffset >= len(p.TrackIDs) {
		return
	}
	d.loading = true
	end := min(p.TrackOffset+400, len(p.TrackIDs))
	ids := make([]int64, end-p.TrackOffset)
	for i := range ids {
		ids[i] = p.TrackIDs[p.TrackOffset+i].ID
	}
	fetch := playlistSongs
	go func() {
		tracks, err := fetch(ids)
		// Details can omit unavailable songs or return them out of order.
		byID := make(map[int64]netease.Track, len(tracks))
		for _, tr := range tracks {
			byID[tr.ID] = tr
		}
		ordered := make([]netease.Track, 0, len(ids))
		for _, id := range ids {
			if tr, ok := byID[id]; ok {
				ordered = append(ordered, tr)
			}
		}
		a.post(func() { a.playlistSongsLoaded(d, end, ordered, err) })
	}()
}

func (a *app) playlistSongsLoaded(d *playlistRemainder, end int, tracks []netease.Track, err error) {
	p := d.pl
	if a.profile != d.profile || a.playlists[p.ID] != p || a.playlistMore[p.ID] != d {
		return
	}
	d.loading = false
	if p.ID == a.likedPlaylist() && d.revision != a.likeSync.revision {
		d.err = "歌单已更新"
		a.loadPlaylist(p.ID, false, true)
		return
	}
	if err != nil {
		d.err = "歌曲加载失败"
		return
	}
	p.Tracks = append(p.Tracks, tracks...)
	p.TrackOffset = end
	if p.TrackOffset < len(p.TrackIDs) {
		if d.play != nil || a.route == (route{page: pagePlaylist, id: p.ID}) {
			a.morePlaylistSongs(d)
		}
		return
	}
	if play := d.play; play != nil {
		d.play = nil
		if a.pl.Current() == play.current {
			a.playPlaylist(p, play.id, play.shuffle)
		}
	}
}

// Play waits for the complete queue rather than silently stopping at the first batch.
func (a *app) playPlaylist(p *netease.Playlist, id int64, shuffle bool) {
	d := a.remainingPlaylist(p)
	// A later play choice supersedes one waiting for another playlist's details.
	for _, other := range a.playlistMore {
		other.play = nil
	}
	if p.TrackOffset < len(p.TrackIDs) {
		d.play = &playlistPlay{id, shuffle, a.pl.Current()}
		d.err = ""
		a.morePlaylistSongs(d)
		a.toast = "正在加载完整歌单，完成后播放"
		return
	}
	at := 0
	for i := range p.Tracks {
		if p.Tracks[i].ID == id {
			at = i
			break
		}
	}
	if a.pl.Shuffle() != shuffle {
		a.pl.ToggleShuffle()
	}
	a.pl.PlayQueue(p.Tracks, at)
}

// songFilter keeps normalized text and matching indices, never a second song queue.
type songFilter struct {
	query   string
	source  *netease.Playlist
	text    []string
	matched string
	count   int
	indices []int
}

func songSearchText(t *netease.Track) string {
	return strings.ToLower(strings.Join([]string{t.Name, t.ArtistNames(), t.Album.Name, strings.Join(t.Alias, " ")}, "\n"))
}

func (f *songFilter) rows(p *netease.Playlist) []int {
	if f.source != p || len(f.text) > len(p.Tracks) {
		f.source, f.text, f.count = p, nil, -1
	}
	q := strings.ToLower(strings.TrimSpace(f.query))
	if q == "" {
		f.matched, f.count, f.indices = q, len(p.Tracks), nil
		return nil
	}
	for i := len(f.text); i < len(p.Tracks); i++ {
		f.text = append(f.text, songSearchText(&p.Tracks[i]))
	}
	if f.matched == q && f.count == len(p.Tracks) {
		return f.indices
	}
	f.matched, f.count = q, len(p.Tracks)
	f.indices = make([]int, 0)
	words := strings.Fields(q)
	for i, text := range f.text {
		match := true
		for _, word := range words {
			if !strings.Contains(text, word) {
				match = false
				break
			}
		}
		if match {
			f.indices = append(f.indices, i)
		}
	}
	return f.indices
}

// playlistSearch is the playlist's search, at the right of the row of its
// header's buttons, over the songs it filters.
func (a *app) playlistSearch(c *ui.Context) {
	p := a.pal
	ui.Box(c).Grow(1)
	// A press anywhere in the field, its magnifier too, types in it. It is
	// a pill as tall as the buttons beside it.
	field := ui.Row(c).Width(220).Height(36).AlignSelf(ui.Center).PaddingX(14).Gap(7).Radius(999).AlignItems(ui.Center).Cursor(ui.CursorText).
		OnClick(func() { a.playlistSearchBox.Focus(c) })
	if c.Focused(a.playlistSearchBox) {
		field.Background(p.bg).Shadow(0, 0, 0, 3, p.accent.Alpha(0.25)).Border(1, p.accent.Alpha(0.6))
	} else {
		field.Background(p.hover.Over(p.bg))
	}
	field.Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(p.muted)
		ui.TextInputBase(c.Key("playlist-song-search"), &a.playlistFilter.query).Bind(&a.playlistSearchBox).
			Grow(1).MinWidth(0).FillHeight().FontSize(13).Placeholder("搜索歌单内歌曲").Label("搜索歌单内歌曲").
			OnChange(func() { a.trackList.ScrollTo(0, ui.Start) })
		if a.playlistFilter.query != "" {
			clear := ui.ButtonBase(c.Key("clear-song-search")).Size(18, 18).Radius(999).Center().Label("清除歌曲搜索").TextColor(p.muted).KeepFocus()
			if clear.Hovered() {
				clear.Background(p.press)
			}
			clear.Children(func() { ui.Icon(c, iconX).Size(11, 11) })
			clear.OnClick(func() {
				a.playlistFilter.query = ""
				a.trackList.ScrollTo(0, ui.Start)
			})
		}
	})
}

// playlistProgress says, under the songs, how many of the playlist's have
// loaded while the rest load, and retries a batch that failed.
func (a *app) playlistProgress(c *ui.Context, p *netease.Playlist) {
	if p.TrackOffset >= len(p.TrackIDs) {
		return
	}
	d := a.remainingPlaylist(p)
	ui.Row(c).Padding(16, 0).Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		label := fmt.Sprintf("已加载 %d / %d 首", len(p.Tracks), len(p.TrackIDs))
		if d.err != "" {
			label += " · " + d.err
		}
		ui.Text(c, label).FontSize(12).TextColor(a.pal.muted)
		if d.err != "" {
			ui.Button(c.Key("retry"), "重试").OnClick(func() { d.err = ""; a.morePlaylistSongs(d) })
		}
	})
}
