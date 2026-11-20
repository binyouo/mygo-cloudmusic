package netease

import (
	"os"
	"strings"
	"testing"
)

// LIVE checks that NetEase's followed feed answers for a signed-in client.
func TestFollowFeedLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	events, last, more, err := FollowFeed(5, -1)
	if err != nil {
		if strings.Contains(err.Error(), "code 301") {
			t.Skipf("no NetEase session for the login-only feed: %v", err)
		}
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("the followed feed came back empty")
	}
	for i, e := range events[:min(3, len(events))] {
		t.Logf("%d: %s: %q (last=%d more=%v)", i+1, e.User.Nickname, e.Content.Msg, last, more)
	}
}
