package player

import (
	"math"
	"time"
)

type audioBlend struct {
	previous *pcmTrack
	start    int64
	end      int64
}

// mix uses equal-power gains, keeping each track's own loudness correction.
func (s *sequence) mix(dst []float32, blend *audioBlend, position int64, incoming float32) {
	if cap(s.mixBuffer) < len(dst) {
		s.mixBuffer = make([]float32, len(dst))
	}
	tail := s.mixBuffer[:len(dst)]
	clear(tail)
	for n := 0; n < len(tail); {
		got, err := blend.previous.read(tail[n:])
		n += got
		if err != nil || got == 0 {
			break
		}
	}
	outgoing := blend.previous.volume()
	for i := 0; i+1 < len(dst); i += 2 {
		phase := float64(position+int64(i/2)-blend.start) / float64(max(1, blend.end-blend.start))
		sin, cos := math.Sincos(min(1, max(0, phase)) * math.Pi / 2)
		for ch := range 2 {
			dst[i+ch] = tail[i+ch]*outgoing*float32(cos) + dst[i+ch]*incoming*float32(sin)
		}
		// Two individually safe peaks can sum above full scale. Link the
		// channels so protection never changes the stereo image or wraps/clips
		// at the output device; ordinary in-range samples remain untouched.
		peak := max(float32(math.Abs(float64(dst[i]))), float32(math.Abs(float64(dst[i+1]))))
		if peak > 1 {
			dst[i] /= peak
			dst[i+1] /= peak
		}
	}
}

// TransitionProgress follows the heard audio clock, including while a decoder has read ahead.
func (p *Player) TransitionProgress() (float32, time.Duration, bool) {
	if p.sequence == nil || p.out == nil || p.seeking {
		return 1, 0, false
	}
	frame := int64(p.out.Position().Seconds() * outputRate)
	s := p.sequence
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasSongSeek {
		return 1, 0, false
	}
	for i := len(s.parts) - 1; i >= 0; i-- {
		part := s.parts[i]
		if frame >= part.start {
			if part.overlap > 0 && frame < part.start+part.overlap {
				return float32(frame-part.start) / float32(part.overlap), time.Duration(part.overlap) * time.Second / outputRate, true
			}
			break
		}
	}
	return 1, 0, false
}

// ApplyTransitions updates the live sequence after a settings change.
func (p *Player) ApplyTransitions() {
	if p.sequence != nil {
		s := p.sequence
		s.mu.Lock()
		s.plan = nil
		s.cancelSmartRender()
		s.adaptive = p.AutoMix != nil && p.AutoMix()
		if s.adaptive {
			for _, part := range s.parts {
				startMixAnalysis(part.track)
			}
			if s.next != nil {
				startMixAnalysis(s.next)
			}
		}
		s.mu.Unlock()
	}
}
