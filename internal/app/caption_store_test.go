package app

import (
	"os"
	"path/filepath"
	"testing"
)

// Reopening an episode before its background save finishes shows the latest transcript, and deferred saves cannot overwrite it.
func TestTranscriptStoreKeepsLatestAcrossEpisodes(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "1.json"), filepath.Join(dir, "2.json")
	if err := os.WriteFile(first, []byte("already on disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	var store transcriptStore
	store.queue(first, []byte("earlier snapshot"))
	deferredSave := store.flush
	store.queue(second, []byte("another episode"))
	store.queue(first, []byte("latest snapshot"))
	if value, err := store.read(first); err != nil || string(value) != "latest snapshot" {
		t.Fatalf("reopened episode read %q, %v", value, err)
	}
	store.flush()
	deferredSave()
	for path, want := range map[string]string{first: "latest snapshot", second: "another episode"} {
		if value, err := os.ReadFile(path); err != nil || string(value) != want {
			t.Fatalf("persisted %q: %q, %v", filepath.Base(path), value, err)
		}
	}
}

// A failed save stays readable and can be persisted once the cache becomes writable again.
func TestTranscriptStoreRetriesFailedSave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "transcripts")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "1.json")
	var store transcriptStore
	store.queue(path, []byte("pending transcript"))
	store.flush()
	if text, err := store.read(path); err != nil || string(text) != "pending transcript" {
		t.Fatalf("failed write lost the pending transcript: %q, %v", text, err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	store.flush()
	if text, err := os.ReadFile(path); err != nil || string(text) != "pending transcript" {
		t.Fatalf("retry did not save the transcript: %q, %v", text, err)
	}
}
