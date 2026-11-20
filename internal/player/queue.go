package player

import (
	"math/rand/v2"

	"netease-music-mygo/internal/netease"
)

// PlayNext puts t immediately after what plays, moving it if already queued.
// The current song, its position and pause state stay as they are; with no
// current song, t starts playing. Repeated choices put the last chosen first.
func (p *Player) PlayNext(t netease.Track) {
	if p.sequence != nil {
		p.pollAudio(p.sequence)
	}
	if p.track == nil || p.at < 0 || p.at >= len(p.order) {
		p.PlayQueue([]netease.Track{t}, 0)
		return
	}
	if t.ID == p.track.ID {
		return
	}
	current := p.order[p.at]
	queue := make([]netease.Track, 0, len(p.queue)+1)
	indices := make([]int, len(p.queue))
	inserted := -1
	for i, tr := range p.queue {
		indices[i] = -1
		if tr.ID == t.ID {
			continue
		}
		indices[i] = len(queue)
		queue = append(queue, tr)
		if i == current {
			inserted = len(queue)
			queue = append(queue, t)
		}
	}
	order := make([]int, 0, len(queue))
	for _, i := range p.order {
		if indices[i] < 0 {
			continue
		}
		order = append(order, indices[i])
		if i == current {
			p.at = len(order) - 1
			order = append(order, inserted)
		}
	}
	p.queue, p.order = queue, order
	p.endRadio()
	// An explicit next song takes effect even when one song was looping.
	if p.repeat == RepeatOne {
		p.repeat = RepeatAll
	}
	p.prepareNext()
	p.sync()
}

// ExtendQueue grows the queue to tracks, which starts with the songs it
// holds: the song playing keeps its place, and the new songs join the
// order still ahead, shuffled among it when shuffle is on.
func (p *Player) ExtendQueue(tracks []netease.Track) {
	if p.track == nil || p.at < 0 || p.at >= len(p.order) || len(tracks) <= len(p.queue) {
		return
	}
	for i := range p.queue {
		if p.queue[i].ID != tracks[i].ID {
			return
		}
	}
	tail := make([]int, 0, len(tracks)-p.at-1)
	tail = append(tail, p.order[p.at+1:]...)
	for i := len(p.queue); i < len(tracks); i++ {
		tail = append(tail, i)
	}
	if p.shuffle {
		rand.Shuffle(len(tail), func(i, j int) { tail[i], tail[j] = tail[j], tail[i] })
	}
	p.order = append(p.order[:p.at+1], tail...)
	p.queue = append(p.queue, tracks[len(p.queue):]...)
	p.prepareNext()
	p.sync()
}
