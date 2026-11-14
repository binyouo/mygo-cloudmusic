package netease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

var scrobbleClient = map[string]string{"os": "osx", "appver": "3.1.10.5100", "osver": "15.5", "channel": "netease"}

const scrobbleUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36 NeteaseMusicDesktop/3.1.10.5100"

// ScrobbleStart records a song as recently played when playback starts.
func ScrobbleStart(id, sourceID int64) error {
	send, err := PrepareScrobble(id, sourceID, 0, "", true)
	if err != nil {
		return err
	}
	return send(context.Background())
}

// Scrobble reports the time actually heard for a song.
func Scrobble(id, sourceID int64, played time.Duration, source string) error {
	send, err := PrepareScrobble(id, sourceID, played, source, false)
	if err != nil {
		return err
	}
	return send(context.Background())
}

// PrepareScrobble snapshots authentication before work leaves the UI thread.
// The returned function neither reads nor updates a later account's session.
func PrepareScrobble(id, sourceID int64, played time.Duration, source string, start bool) (func(context.Context) error, error) {
	return prepareScrobble(id, sourceID, played, source, start, httpClient.Jar.Cookies(siteURL))
}

func prepareScrobble(id, sourceID int64, played time.Duration, source string, start bool, cookies []*http.Cookie) (func(context.Context) error, error) {
	if id <= 0 {
		return nil, fmt.Errorf("netease: scrobble: no song")
	}
	if sourceID == 0 {
		sourceID = id
	}
	if source == "" {
		source = "list"
	}
	seconds := int(played / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	action := "play"
	detail := map[string]any{
		"download":    0,
		"end":         "playend",
		"id":          id,
		"sourceId":    strconv.FormatInt(sourceID, 10),
		"time":        seconds,
		"type":        "song",
		"wifi":        0,
		"source":      source,
		"mainsite":    "1",
		"mainsiteWeb": "1",
		"content":     fmt.Sprintf("id=%d", sourceID),
	}
	if start {
		action = "startplay"
		detail = map[string]any{"id": id, "type": "song", "mainsite": "1", "mainsiteWeb": "1", "content": fmt.Sprintf("id=%d", sourceID)}
	}
	logs, err := json.Marshal([]map[string]any{{"action": action, "json": detail}})
	if err != nil {
		return nil, err
	}
	authenticated := false
	for _, c := range cookies {
		if c.Name == "MUSIC_U" && c.Value != "" {
			authenticated = true
		}
	}
	if !authenticated {
		return nil, &playbackError{kind: "login_required"}
	}
	return func(ctx context.Context) error {
		var res struct {
			Code int `json:"code"`
		}
		if err := eapiPostAsSession(ctx, "/api/feedback/weblog", map[string]any{"logs": string(logs)}, &res, "https://clientlog.music.163.com", scrobbleClient, scrobbleUA, cookies, false); err != nil {
			return err
		}
		if res.Code != 200 {
			return &playbackError{kind: "api", code: res.Code}
		}
		return nil
	}, nil
}

// PlaybackReporter captures the login at the start of a playback, not when a
// later goroutine happens to run. Never persist or log this closure's cookies.
// A successful response is API acceptance, not proof of history ingestion.
func PlaybackReporter(id int64) func(context.Context, time.Duration) error {
	cookies := httpClient.Jar.Cookies(siteURL)
	return func(ctx context.Context, played time.Duration) error {
		// Match the desktop client's event order; do not send play if start fails.
		for _, start := range []bool{true, false} {
			send, err := prepareScrobble(id, id, played, "list", start, cookies)
			if err != nil {
				return err
			}
			if err := send(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// playbackError contains only safe diagnostics, never upstream bodies or cookies.
type playbackError struct {
	kind string
	code int
}

func (e *playbackError) Error() string { return fmt.Sprintf("netease: %s %d", e.kind, e.code) }

// PlaybackFailure is safe to show in diagnostic logs, even for transport errors
// whose original text can contain a URL or details supplied by the server.
func PlaybackFailure(err error) string {
	var failure *playbackError
	if errors.As(err, &failure) {
		return failure.Error()
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout (acceptance unknown)"
	}
	var network net.Error
	if errors.As(err, &network) {
		return "network error (acceptance unknown)"
	}
	return "invalid response or request (acceptance unknown)"
}
