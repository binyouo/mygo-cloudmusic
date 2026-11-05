package app

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

func albumApp() (*app, *albumData) {
	a := newTestApp()
	a.settings = defaultSettings()
	d := &albumData{
		album: &netease.AlbumProfile{Album: netease.Album{ID: 8, Name: "迷跡波"}, Artists: []netease.Artist{{Name: "MyGO!!!!!"}},
			Songs: []netease.Track{{ID: 1, Name: "春日影", Duration: 200000}}},
		activity: &netease.AlbumActivity{SubCount: 76019, CommentCount: 2},
		comments: commentState{thread: "R_AL_3_8"},
	}
	a.albums[8] = d
	a.route = route{page: pageAlbum, id: 8}
	return a, d
}

// The missing album action now collects and uncollects it; failed requests
// restore both the state and count, while repeated pending clicks do nothing.
func TestAlbumSubscribeToggleAndRollback(t *testing.T) {
	old := subscribeAlbum
	defer func() { subscribeAlbum = old }()
	for _, collected := range []bool{false, true} {
		type request struct {
			id  int64
			sub bool
		}
		requests := make(chan request, 4)
		subscribeAlbum = func(id int64, sub bool) error { requests <- request{id, sub}; return nil }
		a, d := albumApp()
		a.profile = &netease.Profile{UserID: 1}
		d.activity.IsSub, d.activity.SubCount = collected, 3
		before := *d.activity
		label, wantCount := "收藏专辑", 4
		if collected {
			label, wantCount = "取消收藏专辑", 2
		}
		tt := ui.NewTester(a.view, 1200, 800)
		tt.Frame()
		for attempt := range 2 {
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if got := wait(t, requests, "the album subscription"); got != (request{8, !collected}) {
				t.Fatalf("requested %+v", got)
			}
			if d.activity.IsSub == collected || d.activity.SubCount != wantCount || !d.subscribing {
				t.Fatal("did not update collection state and count")
			}
			tt.Frame()
			a.toggleAlbumSubscription(d)
			if len(requests) != 0 {
				t.Fatal("repeated a pending request")
			}
			if attempt == 0 {
				a.albumSubscribed(d, before, errors.New("offline"))
				if *d.activity != before || d.subscribing {
					t.Fatal("failed subscription did not roll back")
				}
			} else {
				a.albumSubscribed(d, before, nil)
				if d.activity.IsSub == collected || d.activity.SubCount != wantCount || d.subscribing {
					t.Fatal("successful subscription did not stick")
				}
			}
			tt.Frame()
		}
	}
}

// Choosing an album's comments loads its thread once, keeps the song's
// comments separate, and switching back leaves the playing song untouched.
func TestAlbumCommentsTab(t *testing.T) {
	type request struct {
		thread        string
		offset, limit int
	}
	requests := make(chan request, 4)
	old := threadComments
	threadComments = func(thread string, offset, limit int) (*netease.Comments, error) {
		requests <- request{thread, offset, limit}
		return &netease.Comments{}, nil
	}
	defer func() { threadComments = old }()
	a, d := albumApp()
	a.pl.Cue(d.album.Songs)
	a.comments = commentState{thread: "R_SO_4_1", track: 1, loaded: true, total: 99}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if len(requests) != 0 {
		t.Fatal("comments loaded before opening the tab")
	}
	if err := tt.Click("评论 2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := wait(t, requests, "album comments"); got != (request{"R_AL_3_8", 0, commentPage}) {
		t.Fatalf("requested %+v", got)
	}
	tt.Frame()
	if len(requests) != 0 || d.tab != 1 || a.comments.total != 99 || a.comments.thread != "R_SO_4_1" {
		t.Fatal("repeated the request or replaced the song's comments")
	}
	if err := tt.Click("歌曲"); err != nil {
		t.Fatal(err)
	}
	if d.tab != 0 || a.pl.Current().ID != 1 {
		t.Fatal("did not return to songs or changed playback")
	}
}

// Album comment pagination advances by server rows, even when comments
// repeat across pages; using the unique row count used to repeat an offset.
func TestAlbumCommentPages(t *testing.T) {
	cs := commentState{}
	cs.appendPage(&netease.Comments{Hot: []netease.Comment{{ID: 9}}, Latest: []netease.Comment{{ID: 1}, {ID: 2}}, Total: 4, More: true})
	cs.appendPage(&netease.Comments{Latest: []netease.Comment{{ID: 2}, {ID: 3}, {ID: 3}}, Total: 4, More: true})
	var got []int64
	for _, cm := range cs.latest {
		got = append(got, cm.ID)
	}
	if !slices.Equal(got, []int64{1, 2, 3}) || cs.offset != 5 || len(cs.hot) != 1 || !cs.more || cs.total != 4 {
		t.Fatalf("comments %v, offset %d, hot %v", got, cs.offset, cs.hot)
	}
	cs.appendPage(&netease.Comments{Total: 4, More: true})
	if cs.more {
		t.Fatal("an empty server page kept loading")
	}
}

// More and retry ask for the album's next server offset without clearing
// the comments already shown or falling back to the playing song's thread.
func TestAlbumCommentsMoreAndRetry(t *testing.T) {
	requests := make(chan int, 4)
	old := threadComments
	threadComments = func(thread string, offset, limit int) (*netease.Comments, error) {
		if thread != "R_AL_3_8" {
			return nil, errors.New("wrong thread")
		}
		requests <- offset
		return nil, errors.New("offline")
	}
	defer func() { threadComments = old }()
	a, d := albumApp()
	d.tab = 1
	d.comments.appendPage(&netease.Comments{Latest: []netease.Comment{{ID: 1, Content: "好听"}}, Total: 2, More: true})
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if err := tt.Click("加载更多评论"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the next page"); got != 1 {
		t.Fatalf("offset %d", got)
	}
	// Deliver the failure on the test's main thread, where app.post runs it.
	d.comments.loading, d.comments.err = false, "评论加载失败"
	tt.Frame()
	if err := tt.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the retry"); got != 1 || len(d.comments.latest) != 1 {
		t.Fatalf("retry at %d, comments %+v", got, d.comments.latest)
	}
}

// Liking an album comment updates both copies in its thread without
// touching a song comment that happens to have the same ID.
func TestAlbumCommentLike(t *testing.T) {
	type request struct {
		thread string
		id     int64
		like   bool
	}
	requests := make(chan request, 2)
	old := likeComment
	likeComment = func(thread string, id int64, like bool) error { requests <- request{thread, id, like}; return nil }
	defer func() { likeComment = old }()
	a, d := albumApp()
	a.profile = &netease.Profile{UserID: 1}
	d.tab = 1
	cm := netease.Comment{ID: 9, Content: "很好听", LikedCount: 41}
	d.comments.appendPage(&netease.Comments{Hot: []netease.Comment{cm}, Latest: []netease.Comment{cm}, Total: 1})
	a.comments = commentState{thread: "R_SO_4_1", latest: []netease.Comment{cm}}
	tt := ui.NewTester(a.view, 1200, 1000)
	tt.Frame()
	if err := tt.Click("赞"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the album comment like"); got != (request{"R_AL_3_8", 9, true}) {
		t.Fatalf("like %+v", got)
	}
	for _, c := range []netease.Comment{d.comments.hot[0], d.comments.latest[0]} {
		if !c.Liked || c.LikedCount != 42 {
			t.Fatalf("album comment %+v", c)
		}
	}
	if a.comments.latest[0].Liked {
		t.Fatal("liked the song's comment")
	}
}

// Replying sends the album's thread and parent comment, ignores duplicate
// submits, and keeps the draft and reply target on failure for a retry.
func TestAlbumCommentSendAndRetry(t *testing.T) {
	type request struct {
		thread, text string
		reply        int64
	}
	requests := make(chan request, 4)
	old := sendComment
	sendComment = func(thread, text string, reply int64) (*netease.Comment, error) {
		requests <- request{thread, text, reply}
		return nil, errors.New("offline")
	}
	defer func() { sendComment = old }()
	a, d := albumApp()
	a.profile = &netease.Profile{UserID: 1}
	d.tab = 1
	cm := netease.Comment{ID: 9, Content: "很好听"}
	cm.User.Nickname = "听歌的人"
	d.comments.appendPage(&netease.Comments{Latest: []netease.Comment{cm}, Total: 1})
	tt := ui.NewTester(a.view, 1200, 1000)
	tt.Frame()
	if err := tt.Click("回复"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("专辑评论"); err != nil {
		t.Fatal(err)
	}
	tt.Type("  同感  ")
	if err := tt.Click("发表评论"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the reply"); got != (request{"R_AL_3_8", "同感", 9}) {
		t.Fatalf("sent %+v", got)
	}
	a.publishAlbumComment(d)
	if len(requests) != 0 {
		t.Fatal("submitted twice while sending")
	}
	a.albumCommentSent(&d.comments, nil, errors.New("offline"))
	if d.comments.draft != "  同感  " || d.comments.replyID != 9 || d.comments.sendErr == "" || d.comments.sending {
		t.Fatal("lost the failed draft or reply")
	}
	tt.Frame()
	if err := tt.Click("发表评论"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the retry"); got.reply != 9 || got.text != "同感" {
		t.Fatalf("retried %+v", got)
	}
	a.albumCommentSent(&d.comments, &netease.Comment{ID: 10, Content: "同感"}, nil)
	if d.comments.draft != "" || d.comments.replyID != 0 || d.comments.total != 2 || d.comments.latest[0].ID != 10 || d.comments.offset != 2 {
		t.Fatal("did not commit the successful reply to the album")
	}
}

// Retrying unavailable collection counts requests that album without
// blocking its song list or replacing it with an empty album.
func TestAlbumActivityRetry(t *testing.T) {
	requests := make(chan int64, 2)
	old := albumDynamic
	albumDynamic = func(id int64) (*netease.AlbumActivity, error) { requests <- id; return nil, errors.New("offline") }
	defer func() { albumDynamic = old }()
	a, d := albumApp()
	d.activity, d.activityErr = nil, "收藏数加载失败"
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("春日影"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if err := tt.Click("重试收藏数"); err != nil {
		t.Fatal(err)
	}
	if got := wait(t, requests, "the album counts"); got != 8 || d.album == nil || len(d.album.Songs) != 1 {
		t.Fatal("retried the wrong album or removed its songs")
	}
}

// With NM_FRAMES set, save album songs and comments in both appearances,
// including a long reply and the composer, for visual review.
func TestAlbumFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		for _, tab := range []int{0, 1} {
			a, d := albumApp()
			a.profile = &netease.Profile{UserID: 1}
			d.tab = tab
			d.activity.IsSub = tab == 1
			cm := netease.Comment{ID: 9, Content: "一张适合完整听下来的专辑，每首歌都像是下一首的序章。想把那些没说出口的话，再唱一遍。", Time: time.Now().Add(-time.Hour).UnixMilli(), LikedCount: 128}
			cm.User.Nickname = "听歌的人"
			d.comments.appendPage(&netease.Comments{Hot: []netease.Comment{cm}, Latest: []netease.Comment{{ID: 10, Content: "封面和音乐都很好看，很好听。", Time: cm.Time}}, Total: 1988, More: true})
			tt := ui.NewTester(a.view, 1200, 1000)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			tt.Frame()
			name := "album-songs"
			if tab == 1 {
				name = "album-comments"
			}
			if dark {
				name += "-dark"
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}
