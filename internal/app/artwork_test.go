package app

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"
)

type artFetchCall struct {
	url    string
	result chan error
}

func controlledArtCache(t *testing.T) (*artCache, chan func(), chan artFetchCall) {
	t.Helper()
	data, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	posts := make(chan func(), 128)
	calls := make(chan artFetchCall, 128)
	stop := make(chan struct{})
	c := newArtCache(func(fn func()) { posts <- fn })
	c.sem = make(chan struct{}, 1)
	c.after = func(time.Duration, func()) {}
	c.fetch = func(url string) ([]byte, error) {
		call := artFetchCall{url, make(chan error, 1)}
		calls <- call
		select {
		case err := <-call.result:
			return data, err
		case <-stop:
			return nil, errors.New("test finished")
		}
	}
	t.Cleanup(func() { close(stop) })
	return c, posts, calls
}

// Scrolling far ahead used to leave visible covers waiting behind every offscreen request.
func TestArtCachePrioritizesNewVisibleCover(t *testing.T) {
	c, posts, calls := controlledArtCache(t)
	c.get("active", false)
	active := wait(t, calls, "active cover")
	for i := range 2000 {
		c.get(fmt.Sprint("old-", i), false)
	}
	c.get("visible", false)
	active.result <- nil
	wait(t, posts, "active completion")()
	visible := wait(t, calls, "visible cover")
	if visible.url != "visible" {
		t.Fatalf("offscreen request %s delayed visible cover", visible.url)
	}
	visible.result <- nil
	wait(t, posts, "visible completion")()
	if a := c.get("visible", false); a == nil || a.bmp == nil {
		t.Fatal("visible cover was not published")
	}
}

// Returning to a queued row must promote its existing request instead of opening it twice or keeping it behind stale rows.
func TestArtCachePromotesQueuedCover(t *testing.T) {
	c, posts, calls := controlledArtCache(t)
	c.get("active", false)
	active := wait(t, calls, "active cover")
	c.get("return-to", false)
	c.get("left-behind", false)
	c.get("return-to", false)
	active.result <- nil
	wait(t, posts, "active completion")()
	returned := wait(t, calls, "returned cover")
	if returned.url != "return-to" {
		t.Fatalf("loaded %q before the requested row", returned.url)
	}
	returned.result <- nil
	wait(t, posts, "returned completion")()
	other := wait(t, calls, "remaining cover")
	if other.url != "left-behind" {
		t.Fatalf("duplicated request %q", other.url)
	}
	other.result <- nil
	wait(t, posts, "remaining completion")()
	if len(calls) != 0 {
		t.Fatal("opened the same queued cover twice")
	}
}

// A transient image failure used to remain cached forever, leaving later playlist rows as permanent placeholders.
func TestArtCacheRetriesFailureAfterWake(t *testing.T) {
	c, posts, calls := controlledArtCache(t)
	now := time.Unix(1e9, 0)
	c.now = func() time.Time { return now }
	var delay time.Duration
	var wake func()
	c.after = func(d time.Duration, fn func()) { delay, wake = d, fn }
	c.get("cover", false)
	first := wait(t, calls, "first cover attempt")
	first.result <- errors.New("temporary network failure")
	wait(t, posts, "failure completion")()
	for range 5 {
		if a := c.get("cover", false); a == nil || !a.failed {
			t.Fatal("lost the failure while waiting to retry")
		}
	}
	if len(calls) != 0 || wake == nil || delay <= 0 {
		t.Fatal("retried in a tight loop or failed to wake the idle view")
	}
	now = now.Add(delay)
	wake()
	wait(t, posts, "retry redraw")()
	c.get("cover", false)
	c.get("cover", false)
	retry := wait(t, calls, "cover retry")
	retry.result <- nil
	wait(t, posts, "retry completion")()
	if a := c.get("cover", false); a == nil || a.failed || a.bmp == nil {
		t.Fatal("recovered cover stayed a placeholder")
	}
	if len(calls) != 0 {
		t.Fatal("duplicated the retry")
	}
}

// An animated WebP cover, as some playlists have, shows its first frame,
// in the bitmap too, which decodes its data again once it drops its pixels.
func TestAnimatedWebPCover(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	bmp, img, err := decodeArt(b)
	if err != nil {
		t.Fatal(err)
	}
	if r := img.Bounds(); r.Dx() != 24 || r.Dy() != 24 {
		t.Errorf("first frame is %v, want 24×24", r)
	}
	if w, h := bmp.Size(); w != 24 || h != 24 {
		t.Errorf("bitmap is %d×%d, want 24×24", w, h)
	}
}

// Once the covers take more than the limit, those asked for least
// recently go, so that a long session does not keep every cover it
// showed; one asked for again stays, however long ago it loaded.
func TestArtCacheDropsLeastRecentCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.limit = func() time.Time { return now }, 100
	for _, key := range []string{"first", "second", "third"} {
		c.add(key, &artwork{used: now, size: 30})
		now = now.Add(time.Minute)
	}
	c.get("first", false)
	c.add("fourth", &artwork{used: now, size: 30})
	if _, ok := c.art["second"]; ok {
		t.Error("kept the cover asked for least recently over the limit")
	}
	for _, key := range []string{"first", "fourth"} {
		if _, ok := c.art[key]; !ok {
			t.Errorf("dropped %q, asked for last", key)
		}
	}
}

// Covers stay under the limit however long since they showed. They went
// after 5 minutes not asked for: going back to a page after a while on
// others, or to the window after a while away, loaded them again.
func TestArtCacheKeepsCoversUnderLimit(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now = func() time.Time { return now }
	page := &artwork{used: now, size: 100 << 10}
	c.add("page", page)
	for range 60 {
		now = now.Add(time.Minute)
		c.get("other", false)
	}
	if c.get("page", false) != page {
		t.Error("dropped a cover not asked for in an hour, under the limit")
	}
}

// A cover dropped while something still holds it comes back as it was.
// It loaded again: Now Playing's cover went while other pages showed,
// though Now Playing still held it, and flashed in once it opened again.
func TestArtCacheKeepsHeldCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.limit = func() time.Time { return now }, 60
	held := &artwork{used: now, size: 30}
	c.add("now playing", held)
	for _, key := range []string{"other", "another"} {
		now = now.Add(time.Minute)
		c.add(key, &artwork{used: now, size: 30})
	}
	if _, ok := c.art["now playing"]; ok {
		t.Fatal("kept a cover over the limit")
	}
	if got := c.get("now playing", false); got != held {
		t.Error("loaded again a cover something still held")
	}
	runtime.KeepAlive(held)
}
