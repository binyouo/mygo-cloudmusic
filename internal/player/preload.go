package player

import (
	"netease-music-mygo/internal/localmedia"
	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// A trackLoad owns a song until playback takes it. Its fields, including
// the completion callback, belong to the main thread.
type trackLoad struct {
	id             int64
	level          string
	info           *netease.Stream
	stream         stream
	pcm            *pcmTrack
	released       bool
	err            error
	ready          bool
	cancelled      bool
	done           func()
	mixLyrics      []lyrics.Line
	mixLyricsReady bool
}

var openTrack = (*Player).open

func (p *Player) quality() string {
	if p.Quality != nil {
		return p.Quality()
	}
	return "exhigh"
}

func (p *Player) startLoad(t netease.Track, level string) *trackLoad {
	j := &trackLoad{id: t.ID, level: level}
	open := openTrack
	go func() {
		info, s, err := open(p, t, level)
		var pcm *pcmTrack
		if err == nil {
			pcm, err = preparePCM(s, t, info)
		}
		p.post(func() {
			j.info, j.stream, j.pcm, j.err, j.ready = info, s, pcm, err, true
			if j.cancelled {
				j.close()
			} else if j.done != nil {
				j.done()
			}
		})
	}()
	return j
}

func (j *trackLoad) use(done func()) {
	j.done = done
	if j.ready {
		done()
	}
}

func (j *trackLoad) close() {
	j.cancelled = true
	j.done = nil
	if j.pcm != nil && !j.released {
		j.released = true
		j.pcm.release()
	}
}

// takeNext transfers the prepared song into the foreground. A skip while
// it is opening waits for that same request instead of starting another.
// The second result says it was speculative, so a failure can be retried.
func (p *Player) takeNext(t netease.Track, level string) (*trackLoad, bool) {
	j := p.nextJob
	p.nextJob = nil
	if j != nil {
		if j.id == t.ID && j.level == level {
			return j, true
		}
		j.close()
	}
	return p.startLoad(t, level), false
}

// prepareNext keeps just the next song ready, following the actual play
// order. Queue and repeat changes replace a preparation that no longer fits.
func (p *Player) prepareNext() {
	if p.out == nil || p.stream == nil {
		return
	}
	at := p.at + 1
	if at >= len(p.order) && p.repeat == RepeatAll && p.feed == nil {
		at = 0
	}
	if p.repeat == RepeatOne {
		at = p.at
	}
	if at >= len(p.order) {
		p.queueAudio(nil)
		if p.nextJob != nil {
			p.nextJob.close()
			p.nextJob = nil
		}
		return
	}
	t, level := p.queue[p.order[at]], p.quality()
	if j := p.nextJob; j != nil {
		if j.id == t.ID && j.level == level {
			return
		}
		p.queueAudio(nil)
		j.close()
	}
	p.nextJob = p.startLoad(t, level)
	job := p.nextJob
	if p.Prepared != nil {
		p.Prepared(t)
	}
	if p.PreloadMetadata {
		metadata := t
		if metadata.LocalPath == "" {
			metadata = localmedia.WithLocalMetadata(metadata, p.localLibrary)
		}
		cloud := metadata.LocalPath == "" && metadata.LocalLyricsPath == "" && metadata.LocalLyrics == ""
		fetch := fetchLyrics
		go func() {
			lrc, trans, yrc := metadata.LocalLyrics, "", ""
			var err error
			switch {
			case cloud:
				lrc, trans, yrc, err = fetch(t.ID)
			case metadata.LocalLyricsPath != "":
				lrc, err = localmedia.ReadLyrics(metadata.LocalLyricsPath)
			case metadata.LocalPath != "" && lrc == "":
				lrc, err = localmedia.SidecarLyrics(metadata.LocalPath)
			}
			if err != nil {
				return
			}
			lines := lyrics.Build(lrc, trans, yrc)
			p.post(func() {
				if p.nextJob != job || job.cancelled {
					return
				}
				job.mixLyrics, job.mixLyricsReady = lines, true
				if job.pcm != nil {
					job.pcm.vocals.Store(mixHintsFromLyrics(lines))
				}
				if cloud {
					p.prefetchedLyrics.id, p.prefetchedLyrics.lines, p.prefetchedLyrics.ready = t.ID, lines, true
				}
				if p.sequence != nil {
					p.sequence.mu.Lock()
					if p.sequence.smart == nil {
						p.sequence.plan = nil
						p.sequence.cancelSmartRender()
					}
					p.sequence.mu.Unlock()
				}
			})
		}()
	}
	job.use(func() {
		if job.err == nil {
			p.queueAudio(job.pcm)
		}
	})
}
