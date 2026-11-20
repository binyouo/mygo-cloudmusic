package netease

import (
	"os"
	"testing"
)

// LIVE checks that NetEase's hot search list answers for this client.
func TestHotSearchesLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	hots, err := HotSearches()
	if err != nil {
		t.Fatal(err)
	}
	if len(hots) == 0 {
		t.Fatal("the hot search list came back empty")
	}
	for i, h := range hots[:min(3, len(hots))] {
		t.Logf("%d: %q icon=%d", i+1, h.Word, h.Icon)
	}
}
