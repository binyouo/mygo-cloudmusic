package netease

import (
	"encoding/json"
	"fmt"
	"time"
)

// Scrobble tells NetEase the listener played song id for played, so it
// joins their listening history: the record RecentPlays reads back. id and
// sourceID are the song's, and source is where it played from ("list",
// "radio", "album", "search"...). A stream URL alone leaves no trace; this
// is the report that records the play.
func Scrobble(id, sourceID int64, played time.Duration, source string) error {
	if id <= 0 {
		return fmt.Errorf("netease: scrobble: no song")
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
	logs, err := json.Marshal([]map[string]any{{
		"action": "play",
		"json": map[string]any{
			"download": 0,
			"end":      "playend",
			"id":       id,
			"sourceId": sourceID,
			"time":     seconds,
			"type":     "song",
			"wifi":     0,
			"source":   source,
			"mainsite": 1,
			"content":  "",
		},
	}})
	if err != nil {
		return err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	if err := weapiPost("/api/feedback/weblog", map[string]any{"logs": string(logs)}, &res); err != nil {
		return err
	}
	if res.Code != 200 {
		msg := res.Message
		if msg == "" {
			msg = res.Msg
		}
		return fmt.Errorf("netease: scrobble: code %d %s", res.Code, msg)
	}
	return nil
}
