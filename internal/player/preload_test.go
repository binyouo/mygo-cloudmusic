package player

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/netease"
)

type preparedStream struct {
	silence
	read, closed        chan struct{}
	readOnce, closeOnce sync.Once
}

func newPreparedStream() *preparedStream {
	return &preparedStream{read: make(chan struct{}), closed: make(chan struct{})}
}

func (s *preparedStream) Read(b []float32) (int, error) {
	s.readOnce.Do(func() { close(s.read) })
	return s.silence.Read(b)
}

func (s *preparedStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *preparedStream) source() (*download, *spectrum) { return nil, nil }

type openRequest struct {
	track  netease.Track
	level  string
	stream *preparedStream
	result chan error
}

// preparationPlayer holds each open until the test supplies its result.
func preparationPlayer(t *testing.T, ids ...int64) (*Player, chan func(), chan openRequest) {
	t.Helper()
	posts := make(chan func(), 16)
	requests := make(chan openRequest, 16)
	old := openTrack
	openTrack = func(_ *Player, track netease.Track, level string) (*netease.Stream, stream, error) {
		r := openRequest{track, level, newPreparedStream(), make(chan error, 1)}
		requests <- r
		if err := <-r.result; err != nil {
			return nil, nil, err
		}
		return &netease.Stream{Level: level}, r.stream, nil
	}
	p := &Player{post: func(fn func()) { posts <- fn }, volume: 0.8, gain: 1}
	tracks := make([]netease.Track, len(ids))
	for i, id := range ids {
		tracks[i] = netease.Track{ID: id, Duration: 600000}
	}
	p.Cue(tracks)
	p.stream = newPreparedStream()
	p.out = audio.NewPlayer(p.stream)
	t.Cleanup(func() {
		openTrack = old
		p.fade.attach(nil, 1)
		if p.out != nil {
			p.out.Close()
		}
		if p.nextJob != nil {
			p.nextJob.close()
		}
		if p.loadJob != nil {
			p.loadJob.close()
		}
	})
	return p, posts, requests
}

// Switching used to open the next song only after the old one ended;
// a prepared song now decodes beforehand and is attached without another open.
func TestPreparedTrackStartsWithoutLoading(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	if r.track.ID != 2 {
		t.Fatalf("prepared %d", r.track.ID)
	}
	r.result <- nil
	next(t, posts)
	<-r.stream.read
	pcm := p.nextJob.pcm
	p.stop()
	p.at, p.track, p.loading, p.playing = 1, &p.queue[1], true, true
	p.loadAudio(*p.track, p.at, 0)
	if p.loading || p.stream != pcm.raw || !p.out.Playing() || p.nextJob != nil {
		t.Fatal("did not immediately play the prepared audio")
	}
	if len(requests) != 0 {
		t.Fatal("opened the same song again")
	}
}

// Skipping while preparation is in flight reuses it and honors a pause
// requested before the download finishes.
func TestPreparedTrackInFlightIsReused(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	job := p.nextJob
	p.stop()
	p.at, p.track, p.loading = 1, &p.queue[1], true
	p.loadAudio(*p.track, p.at, 0)
	if p.loadJob != job || !p.loading {
		t.Fatal("restarted preparation")
	}
	r.result <- nil
	next(t, posts)
	if p.loading || p.stream != job.stream || p.out.Playing() {
		t.Fatal("lost the prepared song or ignored the pause")
	}
	if len(requests) != 0 {
		t.Fatal("opened the same song again")
	}
}

// A foreground load overtaken by a skip must release its audio and never
// become the current song when its result arrives later.
func TestOvertakenPreparationCloses(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	p.stop()
	p.at, p.track = 1, &p.queue[1]
	p.loadAudio(*p.track, p.at, 0)
	p.stop()
	r.result <- nil
	next(t, posts)
	<-r.stream.closed
	if p.out != nil || p.loadJob != nil {
		t.Fatal("attached a stale song")
	}
}

// A new play order discards the old preparation, including a late result.
func TestPreparationFollowsOrder(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2, 3)
	p.prepareNext()
	old := <-requests
	p.order[1], p.order[2] = p.order[2], p.order[1]
	p.prepareNext()
	current := <-requests
	if current.track.ID != 3 {
		t.Fatalf("prepared %d", current.track.ID)
	}
	old.result <- nil
	next(t, posts)
	<-old.stream.closed
	current.result <- nil
	next(t, posts)
	if p.nextJob.id != 3 || !p.nextJob.ready {
		t.Fatal("lost the new preparation")
	}
}

// Changing quality must not consume audio prepared at the previous setting.
func TestPreparationRespectsQuality(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	old := <-requests
	old.result <- nil
	next(t, posts)
	p.Quality = func() string { return "lossless" }
	p.prepareNext()
	<-old.stream.closed
	r := <-requests
	if r.level != "lossless" {
		t.Fatalf("quality %q", r.level)
	}
	r.result <- nil
	next(t, posts)
}

// Repeating prepares the appropriate song again, and leaving a radio
// drops the songs it had queued.
func TestPreparationRepeatAndRadio(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.at, p.track = 1, &p.queue[1]
	p.prepareNext()
	if p.nextJob != nil {
		t.Fatal("prepared past the queue's end")
	}
	p.CycleRepeat()
	r := <-requests
	if r.track.ID != 1 {
		t.Fatal("did not prepare the queue's first song")
	}
	r.result <- nil
	next(t, posts)
	p.CycleRepeat()
	<-r.stream.closed
	r = <-requests
	if r.track.ID != 2 {
		t.Fatal("did not prepare the repeated song")
	}
	r.result <- nil
	next(t, posts)
	p.repeat, p.at, p.track = RepeatOff, 0, &p.queue[0]
	p.AttachRadio("test", func(got func([]netease.Track)) { got(nil) })
	p.EndRadio()
	<-r.stream.closed
	if p.nextJob != nil {
		t.Fatal("kept the departed radio's next song")
	}
}

// A failed preparation leaves the current song playing and reports the
// failure only when that prepared song is actually selected.
func TestPreparationFailureWaitsForSelection(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	r.result <- errors.New("download failed")
	next(t, posts)
	if p.err != "" || p.Current().ID != 1 {
		t.Fatal("failed preparation interrupted playback")
	}
	p.stop()
	p.at, p.track, p.loading = 1, &p.queue[1], true
	p.loadAudio(*p.track, p.at, 0)
	retry := <-requests
	if retry.track.ID != 2 || !p.loading || p.err != "" {
		t.Fatal("did not retry the selected song's failed preparation")
	}
	retry.result <- errors.New("download failed")
	next(t, posts)
	if p.loading || p.playing || p.err != "download failed" {
		t.Fatal("did not report the selected song's error")
	}
}

// Skipping during the new foreground retry must cancel it so a late success cannot replace the chosen song.
func TestPreparationRetryOvertaken(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2, 3)
	p.prepareNext()
	first := <-requests
	first.result <- errors.New("temporary download failure")
	next(t, posts)
	p.stop()
	p.at, p.track, p.loading = 1, &p.queue[1], true
	p.loadAudio(*p.track, p.at, 0)
	retry := <-requests
	p.stop()
	p.at, p.track, p.loading = 2, &p.queue[2], true
	p.loadAudio(*p.track, p.at, 0)
	chosen := <-requests
	chosenJob := p.loadJob
	retry.result <- nil
	next(t, posts)
	<-retry.stream.closed
	if p.Current().ID != 3 || p.loadJob != chosenJob || p.out != nil || !p.loading {
		t.Fatal("the retry replaced the newer song")
	}
	chosen.result <- nil
	next(t, posts)
	if p.loading || p.stream != chosen.stream || p.out.Playing() {
		t.Fatal("the newer selection did not load paused")
	}
}

// A failed speculative open used to skip a playable song on both Next and EOF, including when it failed after selection.
func TestFailedPreparationRetriesSelectedTrack(t *testing.T) {
	old := fetchLyrics
	fetchLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }
	defer func() { fetchLyrics = old }()
	for _, automatic := range []bool{false, true} {
		for _, ready := range []bool{false, true} {
			name := "manual"
			if automatic {
				name = "automatic"
			}
			if ready {
				name += "/already-failed"
			} else {
				name += "/in-flight"
			}
			t.Run(name, func(t *testing.T) {
				p, posts, requests := preparationPlayer(t, 1, 2, 3)
				p.prepareNext()
				first := <-requests
				if ready {
					first.result <- errors.New("temporary download failure")
					next(t, posts)
				}
				if automatic {
					p.ended()
				} else {
					p.Next()
				}
				if !ready {
					first.result <- errors.New("temporary download failure")
				}
				deadline := time.After(2 * time.Second)
				var retry openRequest
			waitRetry:
				for {
					select {
					case fn := <-posts:
						fn()
					case retry = <-requests:
						break waitRetry
					case <-deadline:
						t.Fatal("did not retry the failed preparation")
					}
				}
				if retry.track.ID != 2 || p.Current().ID != 2 || p.at != 1 {
					retry.result <- errors.New("test cleanup")
					t.Fatalf("skipped song 2: opened %d, current %d at %d", retry.track.ID, p.Current().ID, p.at)
				}
				// Keep playback paused so a device clock cannot advance the test.
				p.playing = false
				retry.result <- nil
				for p.loading {
					next(t, posts)
				}
				if p.Current().ID != 2 || p.stream != retry.stream || p.err != "" {
					t.Fatal("the recovered song did not become ready")
				}
				successor := <-requests
				if successor.track.ID != 3 {
					t.Fatalf("prepared %d after recovery", successor.track.ID)
				}
				successor.result <- nil
				for p.nextJob != nil && !p.nextJob.ready {
					next(t, posts)
				}
			})
		}
	}
}

// Foreground network and decoder errors used to cascade through the queue; only confirmed unavailability may skip a song.
func TestLoadFailureKeepsSelectedTrack(t *testing.T) {
	old := fetchLyrics
	fetchLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }
	defer func() { fetchLyrics = old }()
	for _, tc := range []struct {
		name string
		err  error
		skip bool
	}{
		{"network", errors.New("connection reset"), false},
		{"decode", errors.New("invalid audio header"), false},
		{"unavailable", netease.ErrUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, posts, requests := preparationPlayer(t, 1, 2, 3)
			p.stop()
			p.at, p.track, p.loading, p.playing = 1, &p.queue[1], true, true
			p.loadAudio(*p.track, p.at, 0)
			request := <-requests
			request.result <- tc.err
			next(t, posts)
			if tc.skip {
				if p.Current().ID != 3 || !p.loading {
					t.Fatal("confirmed unavailability did not advance the queue")
				}
				following := <-requests
				job := p.loadJob
				p.stop()
				following.result <- nil
				for !job.ready {
					next(t, posts)
				}
			} else if p.Current().ID != 2 || p.at != 1 || p.playing || p.loading || p.err != tc.err.Error() {
				t.Fatalf("failure skipped or hid the selected song: at %d, error %q", p.at, p.err)
			}
		})
	}
}
