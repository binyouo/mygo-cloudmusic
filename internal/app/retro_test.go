package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func settleRetro(tt *ui.Tester) {
	for range 4 {
		tt.Frame()
		tt.Advance(time.Second)
	}
	tt.Frame()
}

func readyRetro() { retroBitmaps(); retroIsReady.Store(true) }

// Selecting each player style updates and persists it, including returning to the cover.
func TestChoosePlayerStyle(t *testing.T) {
	a := newTestApp()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	tt := ui.NewTester(a.settingsView, 780, 1500)
	settleRetro(tt)
	shown := "封面"
	for _, choice := range playerStyles[1:] {
		if err := tt.Click(shown); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		shown = choice.label
		if a.settings.PlayerStyle != choice.id {
			t.Fatalf("selected %s: got %q", choice.label, a.settings.PlayerStyle)
		}
		b, err := os.ReadFile(a.settings.path)
		if err != nil {
			t.Fatal(err)
		}
		var saved settings
		if err := json.Unmarshal(b, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.PlayerStyle != choice.id {
			t.Fatalf("saved %q", saved.PlayerStyle)
		}
	}
	if err := tt.Click(shown); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("封面"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settings.PlayerStyle != "" {
		t.Fatalf("cover kept %q", a.settings.PlayerStyle)
	}
}

// Flinging either retro player switches tracks in both directions, just as the cover does.
func TestFlingRetroPlayer(t *testing.T) {
	readyRetro()
	for _, style := range playerStyles[1:] {
		a, tt, r := coverApp(t)
		a.settings.PlayerStyle = style.id
		settleRetro(tt)
		drag(tt, r, -r.W*0.8, 0)
		if id := a.pl.Current().ID; id != 3 {
			t.Fatalf("%s left: %d", style.id, id)
		}
		drag(tt, r, r.W*0.8, 0)
		if id := a.pl.Current().ID; id != 2 {
			t.Fatalf("%s right: %d", style.id, id)
		}
	}
}
