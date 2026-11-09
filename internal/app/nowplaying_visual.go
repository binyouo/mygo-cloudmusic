package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"netease-music-mygo/internal/netease"
)

const artworkTransition = 1200 * time.Millisecond

// nowPlayingVisual retains the last ready artwork while the new track's media loads.
type nowPlayingVisual struct {
	track           int64
	current, before *artwork
	at, trackAt     time.Time
	duration        time.Duration
	alpha           float32
	reduced         bool
	initial         bool
	followedMix     bool
}

func (v *nowPlayingVisual) step(now time.Time, track int64, art *artwork, reduced bool, audio float32, duration time.Duration, mixing bool, fade bool) {
	v.reduced = reduced
	if v.track != track {
		v.track, v.trackAt = track, now
		v.followedMix = false
	}
	if art != nil && art.failed {
		v.clearArtwork()
		return
	}
	if art != nil && !art.failed && art != v.current {
		v.before, v.current, v.at = v.current, art, now
		v.initial, v.followedMix, v.alpha = v.before == nil, false, 0
		v.duration = artworkTransition
		if v.initial {
			v.duration = 320 * time.Millisecond
		}
		if mixing && !v.initial {
			v.duration = duration
		}
		if reduced {
			v.duration = 160 * time.Millisecond
		}
		if !fade {
			// No dissolve: the new cover shows at once.
			v.before, v.initial, v.alpha = nil, false, 1
		}
	}
	v.opacity(now, audio, mixing)
}

func (v *nowPlayingVisual) clearArtwork() {
	v.current, v.before = nil, nil
	v.initial, v.followedMix, v.alpha = false, false, 1
}

func (v *nowPlayingVisual) opacity(now time.Time, audio float32, mixing bool) float32 {
	if v.before != nil || v.initial {
		alpha := clamp01(float32(now.Sub(v.at)) / float32(max(time.Nanosecond, v.duration)))
		alpha = alpha * alpha * (3 - 2*alpha)
		if mixing && !v.reduced && !v.initial {
			alpha, v.followedMix = audio, true
		} else if v.followedMix {
			// The heard mix ended; a late image must not restart its own clock.
			alpha = 1
		}
		v.alpha = max(v.alpha, alpha)
		if v.alpha >= 1 {
			v.before = nil
			v.initial = false
		}
	} else {
		v.alpha = 1
	}
	return v.alpha
}

func (a *app) visualProgress(now time.Time) (float32, bool) {
	progress, _, mixing := a.pl.TransitionProgress()
	return a.npVisual.opacity(now, progress, mixing), mixing
}

func (a *app) prefetchNowPlaying() {
	order, at := a.pl.PlayOrder()
	if at < 0 || at+1 >= len(order) {
		return
	}
	next := a.pl.Queue()[order[at+1]]
	a.art.get(netease.Sized(next.Album.PicURL, 600), true)
}

func (a *app) paintPlayingArtwork(p *ui.Painter, r ui.Rect, track int64, art *artwork) {
	v := &a.npVisual
	// A departing slide keeps its own image after the current song changes.
	if track != v.track {
		paintCoverBitmap(p, r, a.playingArtworkStill(art), 1)
		return
	}
	alpha, mixing := a.visualProgress(p.Now())
	if v.current != nil {
		bmp := bitmap(v.current)
		if animation := v.current.animation; animation != nil {
			enabled := a.settings.MotionArtwork && !v.reduced
			playing := a.pl.Playing() && enabled
			index, delay := animation.step(p.Now(), playing)
			if enabled {
				bmp = animation.bitmap(index)
			}
			if playing && delay > 0 {
				p.After(delay)
			}
		}
		imageAlpha := float32(1)
		if v.initial {
			imageAlpha = alpha
			p.AnimationFrame()
		}
		paintCoverBitmap(p, r, bmp, imageAlpha)
		// Every song change dissolves in place; an audio mix supplies its own
		// progress, while a button/keyboard change uses the artwork clock.
		if v.before != nil && bitmap(v.before) != nil && alpha < 1 {
			paintCoverBitmap(p, r, a.playingArtworkStill(v.before), 1-alpha)
			if !mixing || a.pl.Playing() {
				p.AnimationFrame()
			}
		}
	} else if art != nil {
		p.Image(bitmap(art), r, ui.Cover)
	}
}

// The outgoing cover holds its last drawn animation frame without advancing it.
func (a *app) playingArtworkStill(art *artwork) *ui.Bitmap {
	if art != nil && a.settings.MotionArtwork && !a.npVisual.reduced && art.animation != nil && art.animation.bmp != nil {
		return art.animation.bmp
	}
	return bitmap(art)
}

func paintCoverBitmap(p *ui.Painter, r ui.Rect, bmp *ui.Bitmap, alpha float32) {
	if bmp == nil {
		return
	}
	if alpha >= 1 {
		p.Image(bmp, r, ui.Cover)
		return
	}
	w, h := bmp.Size()
	if w == 0 || h == 0 {
		return
	}
	scale := max(r.W/float32(w), r.H/float32(h))
	sw, sh := r.W/scale, r.H/scale
	p.ImageRegion(bmp, ui.Rect{X: (float32(w) - sw) / 2, Y: (float32(h) - sh) / 2, W: sw, H: sh}, r, ui.ImageOptions{Tint: white.Alpha(alpha)})
}
