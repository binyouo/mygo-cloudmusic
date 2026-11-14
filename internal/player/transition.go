package player

import (
	"math"
	"time"
)

func (p *Player) audioPosition(t time.Duration) time.Duration {
	if p.sequence != nil {
		frame := p.sequence.outputPosition(p.partStart, int64(math.Round(t.Seconds()*outputRate)))
		return time.Duration(float64(frame) * float64(time.Second) / outputRate)
	}
	return time.Duration(float64(p.partStart-p.partOffset)*float64(time.Second)/outputRate) + t
}

func (p *Player) prepareSongSeek(absolute, timeInSong time.Duration) {
	if p.sequence != nil {
		// Match audio.Player.Seek's conversion exactly; its decoder runs later.
		frame := max(int64(math.Round(absolute.Seconds()*outputRate)), 0)
		p.sequence.requestSongSeek(p.partStart, frame, max(0, int64(math.Round(timeInSong.Seconds()*outputRate))))
	}
}

func (s *sequence) setNormalize(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalized = on
	gain := func(p *pcmTrack) {
		g := 1.0
		if on {
			g = normGain(p.info)
		}
		p.setGain(g)
	}
	for _, part := range s.parts {
		gain(part.track)
	}
	if s.next != nil {
		gain(s.next)
	}
	if s.blend != nil {
		gain(s.blend.previous)
	}
	if s.smart != nil {
		gain(s.smart.previous)
		gain(s.smart.incoming)
	}
}

// Queue edits may happen after the decoder read ahead into the next song.
// Flush that decoded lookahead when replacing it; ordinary transitions
// never seek, pause, or recreate the audio output.
func (p *Player) queueAudio(track *pcmTrack) {
	if p.sequence == nil {
		return
	}
	if track != nil {
		if p.nextJob != nil && p.nextJob.pcm == track && p.nextJob.mixLyricsReady {
			track.vocals.Store(mixHintsFromLyrics(p.nextJob.mixLyrics))
		}
		if p.AutoMix != nil && p.AutoMix() {
			startMixAnalysis(track)
		}
		g := 1.0
		if p.Normalize != nil && p.Normalize() {
			g = normGain(track.info)
		}
		track.setGain(g)
	}
	p.sequence.queue(track)
	if p.sequence.ahead(p.partStart) {
		p.out.Seek(p.out.Position())
	}
}

// pollAudio updates the visible song only once its first sample is heard.
// The audio has already joined the tracks without waiting for this timer.
func (p *Player) pollAudio(seq *sequence) {
	if p.sequence != seq || p.seeking {
		return
	}
	frame := int64((p.out.Position().Seconds() * outputRate) + 0.5)
	part, pending, ended := seq.state(frame)
	if part.start != p.partStart {
		if p.Ended != nil {
			p.Ended()
		}
		p.partStart = part.start
		p.partOffset = part.offset
		p.gen++
		if p.nextJob != nil && p.nextJob.pcm == part.track {
			p.nextJob.close()
			p.nextJob = nil
		}
		// The prepared successor follows the current order, including repeats.
		if p.repeat != RepeatOne {
			p.at = (p.at + 1) % len(p.order)
		}
		t := part.track.track
		p.track, p.stream, p.info = &t, part.track.raw, part.track.info
		p.gain = normGain(p.info)
		p.lyrics, p.lyricsLoaded = nil, false
		p.err = ""
		seq.discardBefore(part.start)
		p.announce(&t, nil)
		p.loadMetadata(t, p.gen)
		p.refill()
		p.prepareNext()
		p.sync()
	}
	if ended {
		if p.playing {
			p.ended()
		}
		return
	}
	if !pending || !p.playing || p.polling {
		return
	}
	p.polling = true
	time.AfterFunc(10*time.Millisecond, func() {
		p.post(func() {
			if p.sequence != seq {
				return
			}
			p.polling = false
			p.pollAudio(seq)
		})
	})
}
