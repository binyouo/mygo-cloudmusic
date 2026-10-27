package app

import (
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/lyrics"
)

type menuBarState struct {
	text, tip                string
	width                    float64
	hasTrack, playing, liked bool
}

// menuBarLine says how far into the line shown playback is, and how long the
// line lasts, for the scroll to be done by the next one.
type menuBarLine struct {
	elapsed, dur time.Duration
}

type menuBar struct {
	native *menuBarNative
	last   menuBarState
	timer  *time.Timer
	epoch  uint64
}

// menuBarSnapshot follows the player's position, including seeks while
// paused. The next lyric boundary is the only update the bar needs.
func (a *app) menuBarSnapshot() (s menuBarState, line menuBarLine, next time.Duration) {
	s.text = mygo.App.Name()
	s.width = min(300, max(100, a.settings.MenuBarWidth))
	tr := a.pl.Current()
	if tr == nil {
		return s, line, 0
	}
	s.hasTrack, s.playing, s.liked = true, a.pl.Playing(), a.liked[tr.ID]
	s.text = tr.Name
	s.tip = tr.Name + " — " + tr.ArtistNames()
	lines, loaded := a.pl.Lyrics()
	pos := a.pl.Position()
	i := lyrics.Index(lines, pos)
	if i >= 0 {
		if text := strings.TrimSpace(lines[i].Text); text != "" {
			s.text = text
		}
		// The line scrolls over the time it is sung, as NetEase's marquee
		// does over the line's own display time.
		line.elapsed, line.dur = max(0, pos-lines[i].At), lines[i].End-lines[i].At
	} else if loaded && len(lines) == 0 {
		s.text = "纯音乐，请欣赏"
	}
	// Keep a single line for scrolling and the original in the tooltip.
	s.tip = s.text + "\n" + s.tip
	s.text = strings.Join(strings.Fields(s.text), " ")
	if s.playing && i+1 < len(lines) {
		next = lines[i+1].At - pos
	}
	return s, line, next
}

func (b *menuBar) stopTimer() {
	b.epoch++
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

// syncMenuBar runs on the main thread, even when the main window is
// hidden. Core Animation moves the lyric between boundaries without Go
// timers or main-thread work for each frame.
func (a *app) syncMenuBar() {
	if runtime.GOOS != "darwin" || a.win == nil {
		return
	}
	if !a.settings.MenuBarLyrics || a.quitting {
		if b := a.menuBar; b != nil {
			b.stopTimer()
			b.native.destroy()
			a.menuBar = nil
		}
		return
	}
	if a.menuBar == nil {
		a.menuBar = &menuBar{native: newMenuBarNative(a)}
	}
	b := a.menuBar
	s, line, next := a.menuBarSnapshot()
	// A seek can change the next deadline without changing the text (or
	// even the line). Always replace the old boundary timer.
	b.stopTimer()
	if s != b.last {
		b.native.update(s)
		b.last = s
	}
	b.native.animate(line)
	if next > 0 {
		epoch := b.epoch
		b.timer = time.AfterFunc(max(next, 10*time.Millisecond), func() {
			mygo.RunOnMain(func() {
				if a.menuBar != b || b.epoch != epoch {
					return
				}
				b.timer = nil
				a.syncMenuBar()
			})
		})
	}
}

// menuBarOverflow is how much of a line textWidth wide is wider than the
// viewport, and so has to scroll. The viewport itself keeps the width set
// whatever the line: a status item that resized with each line would shift
// every neighbour in the menu bar along with it.
func menuBarOverflow(viewport, textWidth float64) float64 {
	return max(0, textWidth-viewport)
}

// menuBarScrollOffset reads an overflowing line from its start to its end
// within the time the line is sung, as NetEase's marquee reads a line over
// its display time: it rests at each end for half of what the scroll does
// not take, so the text is at its end as the next line comes in, rather
// than the scroll still on its way.
//
// NetEase's marquee (animateTextFieldWithBeginStayDuration:animateDuration:
// endStayDuration:repeat:) spends as much of the line's time scrolling as
// the hidden part is of the text, and divides the rest between its ends.
func menuBarScrollOffset(overflow, textWidth float64, elapsed, lineDur time.Duration) float64 {
	if overflow <= 0 || textWidth <= 0 {
		return 0
	}
	stay, moving := menuBarScrollTiming(overflow, textWidth, lineDur)
	t := elapsed - stay
	switch {
	case t <= 0:
		return 0
	case t >= moving:
		return overflow
	default:
		return overflow * float64(t) / float64(moving)
	}
}

// Both a paused seek and the native animation use the same reading timeline.
func menuBarScrollTiming(overflow, textWidth float64, lineDur time.Duration) (stay, moving time.Duration) {
	if lineDur <= 0 {
		lineDur = 5 * time.Second
	}
	if textWidth > 0 {
		moving = time.Duration(float64(lineDur) * min(1, max(0, overflow/textWidth)))
	}
	return (lineDur - moving) / 2, moving
}

// Resume only the unfinished part of a scroll. Applying a timeOffset to
// the full Core Animation duration wraps around before it is removed.
func menuBarScrollRemaining(overflow, textWidth float64, elapsed, lineDur time.Duration) (wait, moving time.Duration) {
	stay, duration := menuBarScrollTiming(overflow, textWidth, lineDur)
	elapsed = max(0, elapsed)
	return max(0, stay-elapsed), max(0, stay+duration-max(stay, elapsed))
}
