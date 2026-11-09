package player

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"netease-music-mygo/internal/netease"
)

// Song is a track's audio found for the listener to keep a copy of, in a file
// of their own: kept in the cache, a file on disk, or at its source.
type Song struct {
	// Type is its file's: mp3, flac or ogg, as its source said.
	Type string
	file *os.File        // kept in the cache, or on disk
	st   *netease.Stream // or else where it streams from
}

// FindSong finds track t at quality, using the cache or its playback source.
// It blocks while resolving the audio; run
// it off the main thread, and close what it returns, or save it.
func (p *Player) FindSong(t netease.Track, quality string) (*Song, error) {
	if dl, st, ok := p.Cache.lookup(t.ID, quality); ok {
		// lookup opens a complete file for us; transfer its ownership.
		f := dl.file
		dl.file = nil
		dl.Close()
		return &Song{Type: songType(st.Type), file: f}, nil
	}
	st, err := resolveTrack(t, quality)
	if err != nil {
		return nil, err
	}
	return &Song{Type: songType(st.Type), st: st}, nil
}

// resolveTrack follows the same source selection for playback and downloads,
// including the full-song fallback when NetEase cannot provide a song.
func resolveTrack(t netease.Track, quality string) (*netease.Stream, error) {
	st, err := resolveSong(t.ID, quality)
	tracePlayback("resolve", t.ID, err)
	if errors.Is(err, netease.ErrUnavailable) && t.ProgramID == 0 {
		st, err = resolveAlternative(t)
		tracePlayback("alternative", t.ID, err)
	}
	if err != nil {
		return nil, err
	}
	if st == nil || st.URL == "" {
		return nil, netease.ErrUnavailable
	}
	return st, nil
}

// songType is the type a stream says it is, to name its file by: one the app
// knows, or else mp3, as the cache takes it. What a source sent may hold
// anything.
func songType(typ string) string {
	switch typ = strings.ToLower(typ); typ {
	case "mp3", "flac", "ogg":
		return typ
	}
	return "mp3"
}

// Close lets go of the song, unsaved.
func (s *Song) Close() {
	if s.file != nil {
		s.file.Close()
	}
}

// SaveTo writes the song to path, in place of what's there, and closes it. It
// comes into a file of its own beside path first, so one cut short leaves path
// as it was.
func (s *Song) SaveTo(path string) error {
	defer s.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.part")
	if err != nil {
		return err
	}
	err = s.copy(tmp)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// copy writes all of the song to w.
func (s *Song) copy(w io.Writer) error {
	if s.file != nil {
		_, err := io.Copy(w, s.file)
		return err
	}
	req, err := http.NewRequest("GET", s.st.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", netease.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New(res.Status)
	}
	n, err := io.Copy(w, res.Body)
	if err == nil && s.st.Size > 0 && n != s.st.Size {
		err = io.ErrUnexpectedEOF
	}
	return err
}
