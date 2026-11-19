package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Now Playing can show the song playing as the listener chooses
// (settings.PlayerStyle): its cover, or one of the retro players, drawn
// where the cover would be, which the pointer tilts, drags and flings as it
// does the cover. A record turns on a turntable, the cover its label, the
// tonearm on it while it plays and moving in as the song goes; a cassette's
// reels turn, the tape going from one to the other. What turns follows
// where playback is, so that it stops with it and a seek turns it there, and
// moves in frames that only paint: the view is not built for it.
var playerStyles = []struct{ id, label string }{
	{"", "封面"},
	{"vinyl", "黑胶"},
	{"cassette", "磁带"},
}

// retroFrameRate is how often what turns is painted: a record or a reel
// looks as smooth at it as at the display's rate.
const retroFrameRate = 30

// Turning, in degrees a second of playback: a record at 33⅓ turns a
// minute, a cassette's reels about as fast as they look turning.
const (
	vinylTurn = 200
	reelTurn  = 150
)

// retroPlayer shows tr in style, size across, art its cover and tint the
// cover's color.
func (a *app) retroPlayer(c *ui.Context, style string, tr *netease.Track, art *artwork, size float32, tint ui.Color) {
	if !retroLoaded() {
		prepareRetro()
		c.After(33 * time.Millisecond)
		ui.Box(c).Fill().Center().Children(func() { ui.Text(c, "正在准备播放器…").TextColor(white.Alpha(0.6)) })
		return
	}
	switch style {
	case "cassette":
		a.cassette(c, tr, art, size, tint)
	default:
		a.turntable(c, art, size, tint)
	}
}

// spinning is how far what turns at speed, in degrees a second of
// playback, has turned at pos, and asks for the next paint while it plays;
// asked for less motion, it stays still.
func (a *app) spinning(p *ui.Painter, speed float64) float32 {
	if p.Preferences().ReduceMotion {
		return 0
	}
	if a.pl.Playing() {
		p.AnimationFrameRate(retroFrameRate)
	}
	return float32(math.Mod(a.pl.Position().Seconds()*speed, 360))
}

// progress is how far the song playing has gone, from 0 to 1.
func (a *app) progress() float32 {
	d := a.pl.Duration()
	if d <= 0 {
		return 0
	}
	return clamp01(float32(a.pl.Position().Seconds() / d.Seconds()))
}

// circle is the square around a circle of radius r at (x, y), to fill or
// stroke with a radius of r.
func circle(x, y, r float32) ui.Rect { return ui.Rect{X: x - r, Y: y - r, W: 2 * r, H: 2 * r} }

var black = ui.RGB(0, 0, 0)

// turntable is a record turning on a turntable, size across, its label the
// cover: the tonearm lies on it while it plays, at its edge as a song
// starts and nearing the label as it ends, and lifts off to its rest as
// the song pauses.
func (a *app) turntable(c *ui.Context, art *artwork, size float32, tint ui.Color) {
	pl := a.pl
	bmp := bitmap(art)
	parts := retroBitmaps()
	radius := a.npRadius(c, 16, size)
	deck := ui.Box(c.Key("turntable")).Fill().Radius(radius).Shadow(0, 30, 60, -12, ui.RGBA(0, 0, 0, 0.6))
	on := float32(0)
	if pl.Playing() {
		on = 1
	}
	if c.Preferences().ReduceMotion {
		on = deck.Jump("arm", on)
	} else {
		on = deck.AnimateTo("arm", on, ui.Animation{Spring: ui.SpringOptions{Duration: 900 * time.Millisecond, Bounce: 0.08}})
	}
	deck.Draw(func(p *ui.Painter, r ui.Rect) {
		s := r.W
		p.Image(parts.deck, r, ui.FillBox)
		cx, cy := r.X+deckCX*s, r.Y+deckCY*s

		// The label, the cover printed on paper, turning with the record.
		lr := deckRecord * deckLabel * s
		spin := a.spinning(p, vinylTurn)
		p.Transform(ui.Rotation(spin), cx, cy, func() {
			p.Clip(circle(cx, cy, lr), lr, func() {
				p.Fill(circle(cx, cy, lr), tint, lr)
				if bmp != nil {
					p.Image(bmp, circle(cx, cy, lr), ui.Cover)
				} else {
					// A plain label shows it turning by a band across it.
					p.Fill(ui.Rect{X: cx - lr, Y: cy - lr*0.16, W: 2 * lr, H: lr * 0.32}, white.Alpha(0.22), 0)
				}
			})
		})
		// Matte paper: the light falls across it from the top left, and
		// its edge sinks into the vinyl.
		p.FillGradient(circle(cx, cy, lr), ui.LinearGradient{Angle: 135, From: white.Alpha(0.12), To: black.Alpha(0.18)}, lr)
		p.FillRadialGradient(circle(cx, cy, lr), ui.RadialGradient{Stops: []ui.GradientStop{
			{Color: black.Alpha(0), At: 0}, {Color: black.Alpha(0), At: 0.82}, {Color: black.Alpha(0.35), At: 1},
		}}, lr)
		// The spindle, chrome, its shadow on the label.
		sp := 0.011 * s
		p.FillRadialGradient(circle(cx+sp*0.4, cy+sp*0.6, sp*1.6), ui.RadialGradient{Stops: []ui.GradientStop{
			{Color: black.Alpha(0.5), At: 0}, {Color: black.Alpha(0), At: 1},
		}}, sp*1.6)
		p.FillRadialGradient(circle(cx, cy, sp), ui.RadialGradient{Center: ui.Point{X: 0.35, Y: 0.3}, Stops: []ui.GradientStop{
			{Color: ui.Hex("#ffffff"), At: 0}, {Color: ui.Hex("#a5a5ac"), At: 0.45}, {Color: ui.Hex("#34343a"), At: 1},
		}}, sp)

		// The strobe's lamp glows while it plays.
		lamp := clamp01(on)
		if lamp > 0.01 {
			lx, ly := r.X+deckLampX*s, r.Y+deckLampY*s
			p.FillRadialGradient(circle(lx, ly, 0.06*s), ui.RadialGradient{Stops: []ui.GradientStop{
				{Color: ui.RGBA(255, 70, 40, 0.75*lamp), At: 0}, {Color: ui.RGBA(255, 60, 30, 0.25*lamp), At: 0.3}, {Color: ui.RGBA(255, 60, 30, 0), At: 1},
			}}, 0.06*s)
			p.Fill(circle(lx, ly, 0.009*s), ui.RGBA(255, 190, 160, lamp), 0.009*s)
		}

		// The tonearm, from its pivot: at rest it points down beside the
		// record; on it, its stylus is as far from the middle as the song
		// has yet to go.
		px, py := r.X+deckPivotX*s, r.Y+deckPivotY*s
		stx, sty := armStylus()
		reach, offset := math.Hypot(stx, sty), math.Atan2(sty, stx)
		d := math.Hypot(deckCX-deckPivotX, deckCY-deckPivotY)
		needle := float64(lerp(deckRecord*0.955, deckRecord*deckLabel*1.2, a.progress()))
		cos := (reach*reach + d*d - needle*needle) / (2 * reach * d)
		toward := math.Atan2(deckCY-deckPivotY, deckCX-deckPivotX)
		played := float32(toward - math.Acos(max(-1, min(1, cos))) - offset)
		angle := lerp(deckRest, played, on) * 180 / math.Pi
		arm := func(img *ui.Bitmap, dx, dy float32) {
			x, y := px+dx, py+dy
			p.Transform(ui.Rotation(angle), x, y, func() {
				p.Image(img, ui.Rect{X: x - armBack*s, Y: y - armSide*s, W: (armBack + armFront) * s, H: 2 * armSide * s}, ui.FillBox)
			})
		}
		// Lifted, it is higher off the deck, its shadow farther and softer.
		lift := s * (0.01 + 0.016*(1-on))
		arm(parts.armShadow, lift*0.7, lift)
		arm(parts.arm, 0, 0)

		// What the buttons say, and the light of the speed playing.
		// Printed in the middle of (x, y).
		say := func(text string, x, y, size float32, color ui.Color) {
			span := ui.Span{Text: text, Size: size, Weight: 700, LetterSpacing: size * 0.08, Color: color}
			tw, th := p.MeasureText(0, span)
			p.RichText(x-tw/2, y-th/2, 0, span)
		}
		say("START", r.X+0.0975*s, r.Y+0.934*s, 0.016*s, black.Alpha(0.55))
		say("33", r.X+0.205*s, r.Y+0.937*s, 0.015*s, white.Alpha(0.75))
		say("45", r.X+0.273*s, r.Y+0.937*s, 0.015*s, white.Alpha(0.45))
		led := ui.Hex("#4a1d18")
		if pl.Playing() {
			led = ui.Hex("#ff6a4a")
			p.FillRadialGradient(circle(r.X+0.205*s, r.Y+0.904*s, 0.012*s), ui.RadialGradient{Stops: []ui.GradientStop{
				{Color: led.Alpha(0.6), At: 0}, {Color: led.Alpha(0), At: 1},
			}}, 0.012*s)
		}
		p.Fill(circle(r.X+0.205*s, r.Y+0.904*s, 0.0035*s), led, 0.0035*s)
	})
}

// cassette is a cassette playing, size wide: the song written on its
// label, the cover printed beside the window where its reels turn, the
// tape wound off the left one onto the right one as the song goes.
func (a *app) cassette(c *ui.Context, tr *netease.Track, art *artwork, size float32, tint ui.Color) {
	bmp := bitmap(art)
	parts := retroBitmaps()
	w, h := size, size*tapeHeight
	pen := ui.Hex("#1d2a6b")
	ui.Column(c).Fill().Center().Children(func() {
		shell := ui.Box(c.Key("cassette")).Size(w, h).Radius(w*tapeCorner).Shadow(0, 30, 60, -12, ui.RGBA(0, 0, 0, 0.6))
		shell.Draw(func(p *ui.Painter, r ui.Rect) {
			s := r.W
			at := func(x, y float32) (float32, float32) { return r.X + x*s, r.Y + y*s }

			// Inside, behind the window: the reels on the dark plastic.
			wx, wy := at(0.2, 0.24)
			win := ui.Rect{X: wx, Y: wy, W: 0.6 * s, H: 0.15 * s}
			p.Fill(win, ui.Hex("#151517"), 0.075*s)
			done := float64(a.progress())
			hub, full := tapeHub*s, tapeFull*s
			wound := func(f float64) float32 {
				return float32(math.Sqrt(float64(hub*hub) + float64(full*full-hub*hub)*f))
			}
			spin := a.spinning(p, reelTurn)
			p.Clip(win, 0.075*s, func() {
				for i, f := range []float64{1 - done, done} {
					x, y := at([]float32{tapeReelL, tapeReelR}[i], tapeReelY)
					t := wound(f)
					// Its shadow on the shell, then the tape, brown,
					// catching the light as wound grooves do.
					p.FillRadialGradient(circle(x+2, y+3, t+6), ui.RadialGradient{Stops: []ui.GradientStop{
						{Color: black.Alpha(0.6), At: 0}, {Color: black.Alpha(0.6), At: t / (t + 6)}, {Color: black.Alpha(0), At: 1},
					}}, t+6)
					p.Fill(circle(x, y, t), ui.Hex("#2c1e16"), t)
					p.FillConicGradient(circle(x, y, t), ui.ConicGradient{Angle: float32(lightTurn*180/math.Pi) + 90, Stops: []ui.GradientStop{
						{Color: ui.RGBA(255, 220, 190, 0), At: 0},
						{Color: ui.RGBA(255, 220, 190, 0.22), At: 0.25},
						{Color: ui.RGBA(255, 220, 190, 0), At: 0.4},
						{Color: ui.RGBA(255, 220, 190, 0), At: 0.6},
						{Color: ui.RGBA(255, 220, 190, 0.16), At: 0.75},
						{Color: ui.RGBA(255, 220, 190, 0), At: 0.9},
					}}, t)
					for k := float32(1); k < 6; k++ {
						g := hub + (t-hub)*k/6
						p.Stroke(circle(x, y, g), black.Alpha(0.12+0.06*float32(int(k)%2)), g, 0.5)
					}
					p.Stroke(circle(x, y, t), black.Alpha(0.5), t, 1)
				}
			})
			hw, _ := ui.ImageSize(parts.hub)
			across := hw / tapePixels * s
			for _, rx := range []float32{tapeReelL, tapeReelR} {
				x, y := at(rx, tapeReelY)
				p.Transform(ui.Rotation(spin), x, y, func() {
					p.Image(parts.hub, ui.Rect{X: x - across/2, Y: y - across/2, W: across, H: across}, ui.FillBox)
				})
			}

			// The shell over them, and on its label the cover's colors in
			// bands along its foot and the cover printed in its corner.
			p.Image(parts.cassette, r, ui.FillBox)
			lx, ly := at(0.06, tapeLabelY0)
			label := ui.Rect{X: lx, Y: ly, W: 0.88 * s, H: (tapeLabelY1 - tapeLabelY0) * s}
			p.Clip(label, 0.018*s, func() {
				_, y0 := at(0, 0.398)
				_, y1 := at(0, 0.418)
				p.Fill(ui.Rect{X: label.X, Y: y0, W: label.W, H: y1 - y0}, tint.Alpha(0.88), 0)
				p.Fill(ui.Rect{X: label.X, Y: y1, W: label.W, H: 0.012 * s}, tint.Mix(white, 0.4).Alpha(0.88), 0)
			})
			cx, cy := at(0.088, 0.262)
			cover := ui.Rect{X: cx, Y: cy, W: 0.095 * s, H: 0.095 * s}
			p.Fill(cover, tint, 0)
			if bmp != nil {
				p.Image(bmp, cover, ui.Cover)
			}
			p.Stroke(cover, black.Alpha(0.25), 0, 0.75)
		})
		shell.Children(func() {
			// What plays, written in pen on the label's lines.
			ui.Text(c, tr.Name).Absolute().Top(w * 0.103).Left(w * 0.1).Right(w * 0.1).
				FontSize(w * 0.044).FontWeight(600).Italic().TextColor(pen).SingleLine().PassThrough()
			ui.Text(c, tr.ArtistNames()).Absolute().Top(w * 0.172).Left(w * 0.1).Right(w * 0.1).
				FontSize(w * 0.03).FontWeight(500).Italic().TextColor(pen.Alpha(0.8)).SingleLine().PassThrough()
			ui.Text(c, "A").Absolute().Top(w * 0.262).Left(w * 0.835).FontSize(w * 0.075).FontWeight(800).
				TextColor(ui.Hex("#2a2420")).PassThrough()
		})
	})
}
