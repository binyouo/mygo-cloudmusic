package netease

import (
	"errors"
	"net/url"
	"os"
	"testing"

	"netease-music-mygo/internal/lyrics"
)

// Desktop lyrics remain available when the web endpoint fails or returns no LRC; previously both cases lost them.
func TestLyricsFallsBackToDesktop(t *testing.T) {
	for _, webFails := range []bool{false, true} {
		web := func(_ string, _ url.Values, out any) error {
			if webFails {
				return errors.New("web unavailable")
			}
			return nil
		}
		desktop := func(_ string, _ map[string]any, out any) error {
			r := out.(*lyricResponse)
			r.Code = 200
			r.Lrc.Lyric = "[00:01]歌词"
			r.Tlyric.Lyric = "[00:01]翻译"
			return nil
		}
		lrc, translation, _, err := fetchLyricResponses(42, web, desktop)
		if err != nil || lrc != "[00:01]歌词" || translation != "[00:01]翻译" {
			t.Fatalf("web failure %v: lrc=%q translation=%q err=%v", webFails, lrc, translation, err)
		}
	}
}

// LIVE checks which lyric formats NetEase returns for JANCEH's RUN THIS GAME.
func TestRunThisGameLyricsLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to query NetEase")
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	lrc, trans, yrc, err := Lyrics(3317951859)
	if err != nil {
		t.Fatal(err)
	}
	if len(lyrics.Build(lrc, trans, yrc)) == 0 {
		t.Fatal("NetEase returned no usable lyrics")
	}
	t.Logf("LRC %d bytes, translation %d bytes, YRC %d bytes", len(lrc), len(trans), len(yrc))
}

// A failed desktop request must leave the web lyrics usable, and two failures must report an error.
func TestLyricsEndpointFailures(t *testing.T) {
	web := func(_ string, _ url.Values, out any) error {
		out.(*lyricResponse).Lrc.Lyric = "[00:01]歌词"
		return nil
	}
	desktop := func(_ string, _ map[string]any, out any) error { return errors.New("desktop unavailable") }
	lrc, _, _, err := fetchLyricResponses(42, web, desktop)
	if err != nil || lrc != "[00:01]歌词" {
		t.Fatalf("web lyrics: %q, %v", lrc, err)
	}
	webFail := func(string, url.Values, any) error { return errors.New("web unavailable") }
	if _, _, _, err := fetchLyricResponses(42, webFail, desktop); err == nil {
		t.Fatal("both endpoints failed without an error")
	}
}
