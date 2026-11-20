package player

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/netease"
)

func TestStreamLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	mygo.App.SetPath(mygo.PathUserData, t.TempDir())
	t.Cleanup(func() { mygo.App.SetPath(mygo.PathUserData, "") })
	st, err := netease.SongURL(2652820720, "exhigh")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	dl := startDownload(st.URL, st.Size, nil, nil)
	s, err := openMP3Stream(dl, 278961)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Log("opened in", time.Since(start), s.rate, s.frames, s.start)
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("first read decoded %d samples: %v", n, err)
	}
	if err := s.SetPosition(int64(s.rate) * 120); err != nil {
		t.Fatal(err, dl.n, dl.done, dl.err)
	}
	total := 0
	for i := 0; i < 200; i++ {
		n, err := s.Read(buf)
		total += n
		if err != nil {
			t.Fatal(err)
		}
	}
	if total == 0 {
		t.Fatal("seek did not find decodable audio")
	}
	t.Log("after seek read", total, "in", time.Since(start), "progress", dl.Progress())
	b, ok := s.spec.at(int64(s.rate)*120 + 20000)
	t.Log(ok, b)
}
