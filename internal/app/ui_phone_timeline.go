package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

type phoneScrubGesture struct {
	offset        ui.Point
	level         barState
	held          bool
	value, slider float64
	x, startY     float32
}

// Moving away from the track trades distance for precision. Integrating
// horizontal movement at the current rate keeps the playhead from
// jumping when a finger moves between bands.
func phoneScrubRate(distance float32) float64 {
	switch {
	case distance >= 132:
		return 0.125
	case distance >= 84:
		return 0.25
	case distance >= 40:
		return 0.5
	default:
		return 1
	}
}

func (a *app) phoneTimeline(c *ui.Context, tr *netease.Track, glow ui.Color) {
	timeline := ui.Column(c.Key(tr.ID)).Shrink(0)
	scrub := ui.Local(timeline, "gesture", func() phoneScrubGesture { return phoneScrubGesture{} })
	duration := a.pl.Duration()
	dur := duration.Seconds()
	timeline.Children(func() {
		scrub.slider = a.pl.Position().Seconds()
		bar := ui.SliderBase(c.Key("seek"), &scrub.slider, 0, max(dur, 1)).
			Height(34).Shrink(0).Label("播放进度").Cursor(ui.CursorPointer).Disabled(dur <= 0)
		// Own both drag axes while the slider stays in place. In the
		// vertical direction this keeps the sheet from taking the press.
		bar.Draggable(&scrub.offset, ui.DraggableOptions{Bounds: &ui.Rect{}, Rigid: true})
		_, _, held := bar.Dragged()
		_, _, released := bar.DragEnded()
		switch {
		case held:
			x, y, _ := bar.PointerPosition()
			if !scrub.held {
				scrub.held, scrub.startY, scrub.value = true, y, scrub.slider
			} else if width := bar.LayoutBounds().W; width > 0 {
				rate := phoneScrubRate(abs32(y - scrub.startY))
				delta := float64((x-scrub.x)/width) * dur * rate
				scrub.value = min(dur, max(0, scrub.value+delta))
			}
			scrub.x = x
		case scrub.held:
			scrub.held = false
			if released || bar.Clicked() || bar.DoubleClicked() {
				a.seekFromScrubber(scrub.value, true)
			}
		case bar.Changed():
			a.seekFromScrubber(scrub.slider, true)
		}

		thickness := float32(4)
		if scrub.held {
			thickness = 9
		}
		thickness = bar.AnimateTo("thickness", thickness, ui.Animation{Spring: npDragSpring})
		smoking := a.scrubSmoke(c, bar, glow.A > 0, scrub.held)
		bar.Draw(func(p *ui.Painter, r ui.Rect) {
			at := a.pl.Position().Seconds()
			if scrub.held {
				at = scrub.value
			}
			track := ui.Rect{X: r.X, Y: r.Y + (r.H-thickness)/2, W: r.W, H: thickness}
			p.Fill(track, white.Alpha(0.19), thickness/2)
			if got, ok := a.pl.Downloaded(); ok && got < 1 {
				buffered := track
				buffered.W *= clamp01(float32(got))
				p.Fill(buffered, white.Alpha(0.11), thickness/2)
			}
			track.W *= float32(min(at/max(dur, 1), 1))
			if track.W > 0 {
				if smoking > 0.01 {
					a.paintScrubSmoke(p, track, track.X+track.W, white, glow, smoking, &scrub.level)
				} else {
					p.Fill(track, white.Alpha(0.94), min(thickness, track.W)/2)
				}
			}
			if smoking > 0.01 && (a.pl.Playing() || smoking < 1) {
				p.After(time.Second / 30)
			} else if a.pl.Playing() && !scrub.held && dur > 0 && r.W > 0 {
				p.After(max(16*time.Millisecond, secs(dur/float64(r.W*p.Scale()))))
			}
		})

		at := a.pl.Position()
		if scrub.held {
			at = secs(scrub.value)
		}
		ui.Row(c).Height(18).AlignItems(ui.Center).TextColor(white.Alpha(0.67)).Children(func() {
			ui.Row(c).Grow(1).Basis(0).Children(func() {
				ui.Text(c, clock(at)).FontSize(11).FontFeatures("tnum")
			})
			qualityBadge(c, a.pl.Stream())
			ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Children(func() {
				remaining := max(0, duration-at)
				ui.Text(c, "−"+clock(remaining)).FontSize(11).FontFeatures("tnum")
			})
		})
		a.tickTimes(c)
	})
}
