package player

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

const outputRate = 48000

// pcmTrack keeps a short decoded head ready. The sequence and a pending
// load each hold a reference, so changing the queue cannot close audio
// the decoder has already started reading.
type pcmTrack struct {
	raw             stream
	continuation    stream
	track           netease.Track
	info            *netease.Stream
	dec             audio.Decoder
	head            []float32
	refs            atomic.Int32
	gain            atomic.Uint64
	analysis        atomic.Pointer[trackAnalysis]
	analysisStarted atomic.Bool
	vocals          atomic.Pointer[mixVocalHints]
	analysisContext context.Context
	analysisCancel  context.CancelFunc
}

func preparePCM(s stream, t netease.Track, info *netease.Stream) (*pcmTrack, error) {
	p := &pcmTrack{raw: s, track: t, info: info, dec: s}
	p.refs.Store(1)
	p.analysisContext, p.analysisCancel = context.WithCancel(context.Background())
	p.setGain(1)
	if s.SampleRate() != outputRate {
		p.dec = newResampler(s)
	}
	p.head = make([]float32, 4096*2)
	n, err := p.dec.Read(p.head)
	p.head = p.head[:n]
	if err != nil && err != io.EOF {
		s.Close()
		return nil, err
	}
	return p, nil
}
func (p *pcmTrack) retain() { p.refs.Add(1) }
func (p *pcmTrack) release() {
	if p.refs.Add(-1) == 0 {
		if p.analysisCancel != nil {
			p.analysisCancel()
		}
		if p.continuation != nil {
			p.continuation.Close()
		}
		p.raw.Close()
	}
}
func (p *pcmTrack) read(b []float32) (int, error) {
	n := copy(b, p.head)
	p.head = p.head[n:]
	if n > 0 {
		return n, nil
	}
	return p.dec.Read(b)
}
func (p *pcmTrack) seek(f int64) error { p.head = nil; return p.dec.SetPosition(f) }

type audioPart struct {
	track    *pcmTrack
	start    int64
	overlap  int64
	offset   int64
	noMix    bool // an explicit seek into this song's ending keeps that ending intact
	warp     *smartWarp
	levels   *smartLevelWindow
	revision uint64
}

// sequence is one decoder for the lifetime of continuous playback. At
// EOF it fills the remainder of the SAME Read with the next song: neither
// the UI thread nor an end callback sits between the two sets of samples.
// Only MyGo's decoder goroutine calls Read and SetPosition.
type sequence struct {
	mu            sync.Mutex
	parts         []audioPart
	next          *pcmTrack
	revision      uint64
	pos           int64
	ended, closed bool
	changed       func()
	adaptive      bool
	normalized    bool
	plan          *mixPlan
	blend         *audioBlend
	mixBuffer     []float32
	seekSongStart int64
	seekFrame     int64
	hasSongSeek   bool
	seekSongFrame int64
	render        *smartRenderJob
	smart         *smartPlayback
}

func newSequence(p *pcmTrack, changed func()) *sequence {
	p.retain()
	return &sequence{parts: []audioPart{{track: p}}, changed: changed}
}
func (*sequence) SampleRate() int { return outputRate }
func (*sequence) Channels() int   { return 2 }
func (*sequence) Len() int64      { return -1 }

func (s *sequence) queue(p *pcmTrack) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	old := s.next
	if old == p && p != nil {
		s.mu.Unlock()
		return
	}
	s.revision++
	s.plan = nil
	s.cancelSmartRender()
	if s.smart != nil && !s.smart.promoted {
		s.smart.invalid = true
	}
	s.next = p
	if p != nil {
		p.retain()
	}
	s.mu.Unlock()
	if old != nil {
		old.release()
	}
}

func (s *sequence) Read(b []float32) (int, error) {
	if len(b)%2 != 0 {
		return 0, io.ErrShortBuffer
	}
	total := 0
	for total < len(b) {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return total, io.EOF
		}
		if s.smart != nil {
			n, changed, err := s.readSmart(b[total:])
			s.mu.Unlock()
			total += n
			if changed && s.changed != nil {
				s.changed()
			}
			if err != nil {
				return total, err
			}
			continue
		}
		part := s.parts[len(s.parts)-1]
		p := part.track
		limit := len(b) - total
		if !part.noMix && s.blend == nil && s.next != nil {
			if s.plan != nil && s.plan.outAnalysis != nil &&
				(s.plan.outAnalysis != p.analysis.Load() || s.plan.inAnalysis != s.next.analysis.Load()) {
				s.cancelSmartRender()
				s.plan = nil
			}
			if s.plan == nil {
				if s.adaptive {
					s.plan = planMix(p, s.next)
				}
			}
			fade, exit, enter := int64(0), p.dec.Len(), int64(0)
			if s.plan != nil {
				fade, exit, enter = s.plan.overlap, s.plan.exit, s.plan.enter
			}
			if s.plan != nil && s.plan.smart {
				s.startSmartRender(p, s.next, *s.plan)
				start := part.start + s.plan.sourceStart - part.offset
				data := s.render.ready.Load()
				if data != nil && s.pos == start {
					s.beginSmart(part, data)
					s.mu.Unlock()
					continue
				}
				if start > s.pos {
					limit = min(limit, int((start-s.pos)*2))
				} else {
					// Never block playback for analysis or replay a missed tail.
					s.parts[len(s.parts)-1].noMix = true
					s.cancelSmartRender()
					s.plan = nil
				}
				fade = 0
			}
			start := part.start + exit - part.offset - fade
			if fade > 0 && s.pos >= start && s.pos < start+fade {
				if enter > 0 {
					if err := s.next.seek(enter); err != nil {
						s.mu.Unlock()
						return total, err
					}
				}
				p.retain()
				s.blend = &audioBlend{previous: p, start: s.pos, end: start + fade}
				s.parts = append(s.parts, audioPart{track: s.next, start: s.pos, offset: enter, overlap: start + fade - s.pos, revision: s.revision})
				s.next = nil
				s.plan = nil
				s.mu.Unlock()
				if s.changed != nil {
					s.changed()
				}
				continue
			}
			if fade > 0 && start > s.pos {
				limit = min(limit, int((start-s.pos)*2))
			}
		}
		blend, position := s.blend, s.pos
		if blend != nil {
			limit = min(limit, int((blend.end-s.pos)*2))
			blend.previous.retain()
		}
		p.retain()
		s.mu.Unlock()
		n, err := p.read(b[total : total+limit])
		gain := p.volume()
		if blend != nil {
			s.mix(b[total:total+n], blend, position, gain)
			blend.previous.release()
		} else {
			for i := total; i < total+n; i++ {
				b[i] *= gain
			}
		}
		p.release()
		total += n
		s.mu.Lock()
		s.pos += int64(n / 2)
		if s.blend != nil && (s.pos >= s.blend.end || err != nil || n == 0) {
			s.blend.previous.release()
			s.blend = nil
		}
		if s.closed {
			s.mu.Unlock()
			return total, io.EOF
		}
		if err != nil || n == 0 {
			tracePlayback("decoded-end", p.track.ID, err)
			if err != nil && err != io.EOF || s.next == nil {
				s.ended = true
				s.mu.Unlock()
				return total, io.EOF
			}
			s.parts = append(s.parts, audioPart{track: s.next, start: s.pos, revision: s.revision})
			s.next = nil
			s.plan = nil
			s.mu.Unlock()
			if s.changed != nil {
				s.changed()
			}
			continue
		}
		s.mu.Unlock()
	}
	return total, nil
}

// requestSongSeek names the song shown to the listener. Output time alone is
// ambiguous once the decoder has already mixed a successor over its ending.
func (s *sequence) requestSongSeek(start, frame int64, songFrame ...int64) {
	s.mu.Lock()
	s.seekSongStart, s.seekFrame, s.hasSongSeek = start, frame, true
	s.seekSongFrame = -1
	if len(songFrame) > 0 {
		s.seekSongFrame = songFrame[0]
	}
	s.mu.Unlock()
}

func (s *sequence) SetPosition(frame int64) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return io.EOF
	}
	if s.blend != nil {
		s.blend.previous.release()
		s.blend = nil
	}
	s.cancelSmartRender()
	if s.smart != nil {
		s.smart.data.closeResume()
		s.smart.previous.release()
		s.smart.incoming.release()
		s.smart = nil
	}
	i := len(s.parts) - 1
	explicit := false
	if s.hasSongSeek && frame == s.seekFrame {
		for at, part := range s.parts {
			if part.start == s.seekSongStart {
				i, explicit = at, true
				break
			}
		}
	}
	s.hasSongSeek = false
	if !explicit {
		for i > 0 && frame < s.parts[i].start {
			i--
		}
	}
	part := s.parts[i]
	source := part.sourcePosition(frame)
	if explicit && s.seekSongFrame >= 0 {
		source = s.seekSongFrame
	}
	removed := append([]audioPart(nil), s.parts[i+1:]...)
	// Seeking back before a decoded transition restores the following song,
	// unless a queue change has already selected a different successor.
	if len(removed) > 0 && s.next == nil && removed[0].revision == s.revision {
		s.next = removed[0].track
		s.next.retain()
	}
	s.parts = s.parts[:i+1]
	s.parts[i].overlap = 0
	s.parts[i].warp = nil
	s.parts[i].levels = nil
	s.parts[i].offset = source - (frame - part.start)
	if explicit {
		// Seeking to the last part of A means hearing A's requested ending,
		// not immediately starting B again because its mix window has passed.
		local := source
		window := min(int64(12*outputRate), part.track.dec.Len()/2)
		s.parts[i].noMix = local >= part.track.dec.Len()-window
	}
	s.plan = nil
	s.pos, s.ended = frame, false
	next := s.next
	s.mu.Unlock()
	for _, p := range removed {
		if p.track == next {
			p.track.seek(0)
		}
		p.track.release()
	}
	return part.track.seek(max(source, 0))
}

// state reports the song heard at frame, and whether another boundary
// or the end still needs to be reflected by the UI.
func (s *sequence) state(frame int64) (audioPart, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasSongSeek {
		// audio.Player exposes the new clock before its decoder processes the
		// request. Keep the requested song visible until that request lands;
		// otherwise the UI could discard it as already-played lookahead.
		for _, part := range s.parts {
			if part.start == s.seekSongStart {
				return part, true, false
			}
		}
	}
	part := s.parts[0]
	for _, p := range s.parts[1:] {
		if p.start > frame {
			break
		}
		part = p
	}
	pending := s.parts[len(s.parts)-1].start > frame || s.ended && frame < s.pos
	return part, pending, s.ended && frame >= s.pos
}
func (s *sequence) started(p *pcmTrack) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, part := range s.parts {
		if part.track == p {
			return true
		}
	}
	return false
}
func (s *sequence) ahead(start int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parts[len(s.parts)-1].start > start || s.smart != nil && s.smart.invalid
}
func (s *sequence) discardBefore(start int64) {
	s.mu.Lock()
	var old []*pcmTrack
	for len(s.parts) > 1 && s.parts[1].start <= start {
		old = append(old, s.parts[0].track)
		s.parts = s.parts[1:]
	}
	s.mu.Unlock()
	for _, p := range old {
		p.release()
	}
}
func (s *sequence) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancelSmartRender()
	smart := s.smart
	s.smart = nil
	parts := append([]audioPart(nil), s.parts...)
	next := s.next
	s.next = nil
	blend := s.blend
	s.blend = nil
	s.mu.Unlock()
	for _, p := range parts {
		p.track.release()
	}
	if next != nil {
		next.release()
	}
	if blend != nil {
		blend.previous.release()
	}
	if smart != nil {
		smart.data.closeResume()
		smart.previous.release()
		smart.incoming.release()
	}
	return nil
}
