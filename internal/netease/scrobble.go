package netease

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

var scrobbleClient = map[string]string{"os": "osx", "appver": "3.1.10.5100", "osver": "15.5", "channel": "netease"}

const scrobbleUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36 NeteaseMusicDesktop/3.1.10.5100"

// ScrobbleStart records a song as recently played when playback starts.
func ScrobbleStart(id, sourceID int64) error {
	return scrobble(id, sourceID, 0, "", true)
}

// Scrobble reports the time actually heard for a song.
func Scrobble(id, sourceID int64, played time.Duration, source string) error {
	return scrobble(id, sourceID, played, source, false)
}

func scrobble(id, sourceID int64, played time.Duration, source string, start bool) error {
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
		return err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	if err := eapiPostAs("/api/feedback/weblog", map[string]any{"logs": string(logs)}, &res, "https://clientlog.music.163.com", scrobbleClient, scrobbleUA); err != nil {
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
