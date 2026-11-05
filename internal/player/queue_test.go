package player

import (
	"slices"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Queuing next moves an existing song, including one already played,
// and keeps both the visible queue and shuffled play order consistent.
func TestPlayNextOrder(t *testing.T) {
	for _, tc := range []struct {
		name        string
		order       []int
		at          int
		next        int64
		queue, play []int64
	}{
		{"new", []int{0, 1, 2, 3}, 1, 5, []int64{1, 2, 5, 3, 4}, []int64{1, 2, 5, 3, 4}},
		{"later", []int{0, 1, 2, 3}, 1, 4, []int64{1, 2, 4, 3}, []int64{1, 2, 4, 3}},
		{"earlier", []int{0, 1, 2, 3}, 2, 1, []int64{2, 3, 1, 4}, []int64{2, 3, 1, 4}},
		{"last", []int{0, 1, 2, 3}, 3, 2, []int64{1, 3, 4, 2}, []int64{1, 3, 4, 2}},
		{"shuffled", []int{2, 0, 3, 1}, 1, 2, []int64{1, 2, 3, 4}, []int64{3, 1, 2, 4}},
		{"shuffled-earlier", []int{2, 0, 3, 1}, 1, 3, []int64{1, 3, 2, 4}, []int64{1, 3, 4, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{}
			p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
			p.order, p.at = slices.Clone(tc.order), tc.at
			p.track = &p.queue[p.order[p.at]]
			current := p.track.ID
			p.shuffle = tc.name == "shuffled" || tc.name == "shuffled-earlier"
			p.Hold(42 * time.Second)
			for range 2 {
				p.PlayNext(netease.Track{ID: tc.next})
				var queue, play []int64
				for _, tr := range p.queue {
					queue = append(queue, tr.ID)
				}
				for _, i := range p.order {
					play = append(play, p.queue[i].ID)
				}
				if !slices.Equal(queue, tc.queue) || !slices.Equal(play, tc.play) || p.track.ID != current || p.queue[p.order[p.at]].ID != current || p.Position() != 42*time.Second || !p.Playing() {
					t.Fatalf("queue %v, play %v at %d, current %d", queue, play, p.at, p.track.ID)
				}
			}
		})
	}
}

// With no current song, a next-song choice becomes the queue and starts
// loading for playback instead of remaining an unreachable pending entry.
func TestPlayNextEmpty(t *testing.T) {
	job := &trackLoad{id: 7, level: "exhigh"}
	p := &Player{at: -1, post: func(func()) {}, nextJob: job}
	p.PlayNext(netease.Track{ID: 7, LocalPath: "/music/七.mp3"})
	defer p.stop()
	if p.Current() == nil || p.Current().ID != 7 || !p.Playing() || !p.Loading() || len(p.Queue()) != 1 || p.loadJob != job {
		t.Fatal("the empty queue did not begin playing the chosen song")
	}
}

// A pending track can fail after an earlier song is moved behind it.
// Skipping used the index captured before the move and bypassed that song.
func TestPlayNextDuringFailedLoad(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2, 3)
	p.stop()
	p.queue[0].LocalPath = "/music/一.mp3"
	p.at, p.track = 1, &p.queue[1]
	p.loadAudio(*p.track, p.at, 0)
	failed := <-requests
	p.PlayNext(p.queue[0])
	chosen := &trackLoad{id: 1, level: "exhigh"}
	p.nextJob = chosen
	failed.result <- netease.ErrUnavailable
	next(t, posts)
	if p.Current().ID != 1 || p.at != 1 || p.loadJob != chosen {
		t.Fatalf("skipped to %d at %d instead of the chosen successor", p.Current().ID, p.at)
	}
}

// The last next-song choice goes first, preserves pause, and leaves a
// radio's feed so it cannot replace the listener's chosen queue.
func TestPlayNextChoicesAndModes(t *testing.T) {
	p := &Player{}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}})
	p.repeat = RepeatOne
	p.AttachRadio("test", func(func([]netease.Track)) {})
	p.PlayNext(netease.Track{ID: 4})
	p.PlayNext(netease.Track{ID: 5})
	var got []int64
	for _, i := range p.order {
		got = append(got, p.queue[i].ID)
	}
	if !slices.Equal(got, []int64{1, 5, 4, 2, 3}) || p.Radio() != "" || p.feed != nil || p.Playing() || p.repeat != RepeatAll {
		t.Fatalf("order %v, radio %s, playing %v, repeat %v", got, p.Radio(), p.Playing(), p.repeat)
	}
	p.PlayNext(netease.Track{ID: 1})
	if len(p.queue) != 5 || p.track.ID != 1 {
		t.Fatal("duplicated or restarted the current song")
	}
}

// Replacing the queued successor cancels its pending download and prepares
// the requested song while the current output and pause state stay intact.
func TestPlayNextReplacesPreparation(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2, 3)
	p.prepareNext()
	old := <-requests
	job := p.nextJob
	out := p.out
	p.PlayNext(netease.Track{ID: 3})
	chosen := <-requests
	if chosen.track.ID != 3 || !job.cancelled || p.out != out || p.Playing() || p.track.ID != 1 {
		t.Fatal("did not replace the successor while keeping playback intact")
	}
	old.result <- nil
	next(t, posts)
	<-old.stream.closed
	chosen.result <- nil
	next(t, posts)
	if p.nextJob.id != 3 || !p.nextJob.ready {
		t.Fatal("the chosen successor was lost")
	}
}
