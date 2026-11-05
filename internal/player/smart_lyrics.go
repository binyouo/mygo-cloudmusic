package player

import "netease-music-mygo/internal/lyrics"

// Lyrics are conservative activity hints, not an audio vocal classifier.
// Empty/unavailable lyrics never mean an instrumental song.
func mixHintsFromLyrics(lines []lyrics.Line) *mixVocalHints {
	h := &mixVocalHints{known: true}
	for _, line := range lines {
		if lyrics.IsCredit(line.Text) || line.End <= line.At || line.Text == "" {
			continue
		}
		start, end := line.At, line.End
		if len(line.Words) > 0 {
			start = line.Words[0].Start
			end = line.Words[len(line.Words)-1].End
		}
		// Collision scoring uses the sung interval itself. Padding would turn
		// two separated phrases into a false overlap and reject every candidate.
		h.windows = append(h.windows, mixVocalWindow{start: max(0, int64(start.Seconds()*outputRate)), end: int64(end.Seconds() * outputRate)})
		if !line.Timed {
			h.known = false
		}
	}
	if len(h.windows) == 0 {
		h.known = false
	}
	return h
}

// Runs on the main thread alongside metadata publication. The atomic value is
// immutable; a decoder/preparation worker may safely snapshot it later.
func (p *Player) publishMixLyrics(id int64, lines []lyrics.Line) {
	hints := mixHintsFromLyrics(lines)
	for _, job := range []*trackLoad{p.loadJob, p.nextJob} {
		if job != nil && job.id == id {
			job.mixLyrics, job.mixLyricsReady = lines, true
			if job.pcm != nil {
				job.pcm.vocals.Store(hints)
			}
		}
	}
	if s := p.sequence; s != nil {
		s.mu.Lock()
		for _, part := range s.parts {
			if part.track.track.ID == id {
				part.track.vocals.Store(hints)
			}
		}
		if s.next != nil && s.next.track.ID == id {
			s.next.vocals.Store(hints)
		}
		if s.smart == nil {
			s.plan = nil
			s.cancelSmartRender()
		}
		s.mu.Unlock()
	}
}
