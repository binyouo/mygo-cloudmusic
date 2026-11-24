package app

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync"
)

type transcriptSnapshot struct{ data []byte }

// transcriptStore keeps the newest snapshot of each episode available even
// while a write is pending. The disk writer runs separately from queuing.
type transcriptStore struct {
	mu      sync.Mutex
	pending map[string]*transcriptSnapshot
	writer  sync.Mutex
}

func (s *transcriptStore) queue(path string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = make(map[string]*transcriptSnapshot)
	}
	s.pending[path] = &transcriptSnapshot{data: data}
}

func (s *transcriptStore) read(path string) ([]byte, error) {
	if s != nil {
		s.mu.Lock()
		pending := s.pending[path]
		s.mu.Unlock()
		if pending != nil {
			return pending.data, nil
		}
	}
	return os.ReadFile(path)
}

// flush serializes writes. A snapshot replaced while it was being written
// remains queued for the next flush.
func (s *transcriptStore) flush() {
	s.writer.Lock()
	defer s.writer.Unlock()
	s.mu.Lock()
	pending := maps.Clone(s.pending)
	s.mu.Unlock()
	for path, snapshot := range pending {
		if err := writeTranscriptFile(path, snapshot.data); err != nil {
			continue
		}
		s.mu.Lock()
		if s.pending[path] == snapshot {
			delete(s.pending, path)
		}
		s.mu.Unlock()
	}
}

func writeTranscriptFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".transcript-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
