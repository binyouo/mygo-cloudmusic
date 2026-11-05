package player

import (
	"errors"
	"log"
	"net/url"
	"os"
)

var playbackTrace = os.Getenv("MYGO_PLAYER_TRACE") != ""

// Trace only when requested; signed stream URLs stay out of diagnostics.
func tracePlayback(event string, id int64, err error) {
	if !playbackTrace {
		return
	}
	var request *url.Error
	if errors.As(err, &request) {
		err = request.Err
	}
	log.Printf("player: %s song=%d error=%v", event, id, err)
}
