package player

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"netease-music-mygo/internal/netease"
)

// Saving a cached song uses its original bytes and quality without asking the network or changing the queue.
func TestSaveCachedSong(t *testing.T) {
	p := New(func(func()) {})
	p.Cache = NewCache(t.TempDir(), 1<<20)
	data := []byte("cached audio")
	st := &netease.Stream{Type: "ogg", Level: "lossless"}
	f := p.Cache.part(st)
	if f == nil {
		t.Fatal("no cache file")
	}
	f.Write(data)
	f.Close()
	p.Cache.keep(42, st, f.Name(), int64(len(data)))
	real := resolveSong
	defer func() { resolveSong = real }()
	resolveSong = func(int64, string) (*netease.Stream, error) {
		t.Error("cached song asked the network")
		return nil, netease.ErrUnavailable
	}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	song, err := p.FindSong(netease.Track{ID: 42}, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	if song.Type != "ogg" {
		t.Fatalf("type %q", song.Type)
	}
	path := filepath.Join(t.TempDir(), "song.ogg")
	if err := song.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, data) {
		t.Fatalf("saved %q, %v", b, err)
	}
	if p.Current().ID != 1 || len(p.Queue()) != 2 {
		t.Fatal("download changed playback")
	}
}

// Downloads use playback's full-song fallback and leave an existing file untouched when the response is cut short.
func TestSaveSongFallbackAndTruncation(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "truncated"}[truncated], func(t *testing.T) {
			data := []byte("whole audio")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.UserAgent() != netease.UserAgent {
					t.Error("missing playback user agent")
				}
				w.Write(data)
			}))
			defer srv.Close()
			real, alt := resolveSong, resolveAlternative
			defer func() { resolveSong, resolveAlternative = real, alt }()
			resolveSong = func(int64, string) (*netease.Stream, error) { return nil, netease.ErrUnavailable }
			resolveAlternative = func(tr netease.Track) (*netease.Stream, error) {
				if tr.ID != 42 {
					t.Errorf("resolved track %d", tr.ID)
				}
				size := int64(len(data))
				if truncated {
					size++
				}
				return &netease.Stream{URL: srv.URL, Type: "mp3", Size: size}, nil
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "song.mp3")
			os.WriteFile(path, []byte("before"), 0600)
			song, err := New(func(func()) {}).FindSong(netease.Track{ID: 42}, "exhigh")
			if err != nil {
				t.Fatal(err)
			}
			err = song.SaveTo(path)
			if (err != nil) != truncated {
				t.Fatalf("save error %v, truncated %v", err, truncated)
			}
			want := data
			if truncated {
				want = []byte("before")
			}
			if b, _ := os.ReadFile(path); !bytes.Equal(b, want) {
				t.Errorf("saved %q", b)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 1 {
				t.Errorf("left partial files: %v", entries)
			}
		})
	}
}
