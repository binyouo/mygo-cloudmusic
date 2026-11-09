package player

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Ogg files decode completely and seeking lands on the requested audio rather than restarting the song.
func TestOggDecodeAndSeek(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.ogg")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	s, err := openStream(downloaded(b), 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Channels() != 2 || s.SampleRate() != 44100 || len(ref) < 4*44100*2 {
		t.Fatalf("unexpected decoded audio: channels %d, rate %d, samples %d", s.Channels(), s.SampleRate(), len(ref))
	}
	buf := make([]float32, 4096)
	for _, frame := range []int64{0, 1, 4096, 50000, 123457} {
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, err := s.Read(buf)
		if err != nil || n == 0 {
			t.Fatalf("seek %d: read %d, %v", frame, n, err)
		}
		if d := maxDiff(buf[:n], ref[frame*2:]); d > 1e-4 {
			t.Errorf("seek %d: error %g", frame, d)
		}
	}
	if err := s.SetPosition(s.Len()); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("end: %d, %v", n, err)
	}
}

// Ogg playback starts from the first pages rather than waiting for the whole download.
func TestOggStartsWhileDownloading(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.ogg")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(b[:len(b)/2])
		w.(http.Flusher).Flush()
		select {
		case <-release:
			w.Write(b[len(b)/2:])
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	dl := startDownload(srv.URL, int64(len(b)), nil, nil)
	defer dl.Close()
	ready := make(chan error, 1)
	go func() {
		s, err := openStream(dl, 5000)
		if err == nil {
			defer s.Close()
			var n int
			n, err = s.Read(make([]float32, 4096))
			if n == 0 && err == nil {
				err = io.ErrNoProgress
			}
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ogg waited for the response to end")
	}
}

// A shared Ogg decoder releases only its own reference, leaving playback's download open.
func TestSharedOggKeepsPlayback(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.ogg")
	if err != nil {
		t.Fatal(err)
	}
	dl := downloaded(b)
	s, err := openStream(dl, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	shared, err := openShared(dl, 5000)
	if err != nil {
		t.Fatal(err)
	}
	shared.Close()
	shared.Close()
	if n, err := s.Read(make([]float32, 4096)); err != nil || n == 0 {
		t.Fatalf("playback lost: %d, %v", n, err)
	}
}
