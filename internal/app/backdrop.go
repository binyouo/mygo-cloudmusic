package app

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/player"
)

// The backdrop of Now Playing paints the cover's colors behind it, as
// NetEase's own player does: a deep gradient of its main color, lighter at
// the top, with the cover itself, blurred, faintly through it. It stays
// still, so it is painted once a song, into a small canvas an eighth of the
// window across, which shows scaled up as smooth as it is: what moves over
// it then draws one image under it.
//
// The experimental fog (settings.Fog) makes it alive, as NetEase's mobile
// player's is: fog of the cover's color, billowing as it rises slowly
// through a deep shade of it, with specks of dust glinting in it, brighter
// and thicker with the bass. A shader paints it into a canvas, half the
// window across on the GPU, thirty times a second while the music plays;
// the CPU paints it still. Paused, the fog glides to a stop, and the
// canvas stays as it is.
//
// Another song's backdrop fades in over the last one's, which stays as it
// was, in a canvas of its own, until it is gone.

// fogSource paints the fog: noise of five octaves, warped by noise of
// its own so that it billows rather than slides, rising with Time, Fog
// over Deep where it is thick; thickest a little above the middle, and
// thinning toward the top and the controls at the foot.
const fogSource = `package main

var Time float
var Bass float
var Deep vec4
var Fog vec4

func hash(p vec2) float {
	return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453)
}

func noise(p vec2) float {
	i := floor(p)
	f := fract(p)
	u := f * f * (3.0 - 2.0*f)
	a := hash(i)
	b := hash(i + vec2(1, 0))
	c := hash(i + vec2(0, 1))
	d := hash(i + vec2(1, 1))
	return mix(mix(a, b, u.x), mix(c, d, u.x), u.y)
}

func fbm(p vec2) float {
	v := 0.0
	amp := 0.5
	for i := 0; i < 5; i++ {
		v += amp * noise(p)
		p = p*2.03 + vec2(1.7, 9.2)
		amp *= 0.5
	}
	return v
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	uv := pos / size
	p := pos / size.y * 2.4
	t := Time
	rise := vec2(0, t*0.1)
	q := vec2(fbm(p+rise+vec2(0, t*0.04)), fbm(p+rise+vec2(5.2, 1.3)-vec2(t*0.05, 0)))
	f := fbm(p + rise + 0.9*q)
	// Broad banks of it, and the puffs within them.
	bank := fbm(p*0.45 + rise*0.6 + vec2(3.7, 0))
	cloud := smoothstep(0.3-0.06*Bass, 0.78, f) * (0.45 + 0.75*smoothstep(0.3, 0.65, bank))
	// Thickest a little above the middle.
	m := smoothstep(-0.1, 0.4, uv.y) * (1.0 - 0.5*smoothstep(0.65, 1.0, uv.y))
	c := mix(Deep.rgb, Fog.rgb, min(cloud*m*(0.85+0.35*Bass), 1.0))
	// A broad glow behind it all, darker at the corners.
	g := length((uv - vec2(0.5, 0.42)) * vec2(1.0, 1.25))
	c *= 1.08 - 0.45*smoothstep(0.2, 0.85, g)
	// Specks of dust, rising with the fog, each glinting in its own time.
	cell := size.y / 22
	sp := (pos + vec2(0, t*cell*0.9)) / cell
	id := floor(sp)
	h := hash(id + vec2(3.1, 7.7))
	if h > 0.93 {
		at := vec2(hash(id+vec2(1.3, 2.9)), hash(id+vec2(8.1, 4.3)))*0.8 + 0.1
		d := length((fract(sp)-at)*cell) / max(cell*0.05, 0.7)
		glint := 0.5 + 0.5*sin(t*(1.5+2.0*h)+h*40.0)
		c += vec3(0.75, 0.82, 1.0) * exp(-d*d) * glint * (0.25 + 0.3*m)
	}
	return vec4(c, 1)
}
`

var fogShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(fogSource) })

// backdropFPS is how often the background is painted while it moves.
const backdropFPS = 30
const backdropFrame = time.Second / backdropFPS

// backdropFade is how long another song's backdrop takes to fade in.
const backdropFade = artworkTransition

// backdropCache is the backdrop as last painted, and where the fog's
// drift and its pulse are.
type backdropCache struct {
	canvas  *ui.Canvas
	before  *ui.Canvas // the song before's, fading out over it
	fadeAt  time.Time  // when it began to
	fadeFor time.Duration
	art     *artwork
	tint    ui.Color
	fog     bool
	flow    bool
	living  bool
	at      time.Time // when it was painted
	drift   float64   // the clouds' time, in seconds, which stops on pause
	speed   float64   // how fast drift goes: 1 playing, 0 paused
	level   barState
	last    time.Time
}

// backdrop paints the backdrop behind Now Playing, still or fog.
func (a *app) backdrop(c *ui.Context, art *artwork, tint ui.Color) {
	layer := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
	a.drawPlayingBackdrop(c, layer, art, tint, nil)
}

// drawPlayingBackdrop paints the backdrop behind the player in layer.
// shape, on the phone, morphs the sheet the backdrop fills from the mini
// player's capsule to the window and back: it returns the box to paint,
// its corner radius, its alpha and whether the sheet is still moving;
// nil, on the desktop, fills the layer's whole rect, still.
func (a *app) drawPlayingBackdrop(c *ui.Context, layer ui.Element, art *artwork, tint ui.Color, shape func(r ui.Rect, now time.Time) (box ui.Rect, radius, alpha float32, moving bool)) {
	bd := &a.backdropCache
	pl := a.pl
	// On the phone the fog and the flow give way to the living cover.
	wantFog, still := !a.phone && a.settings.Fog, c.Preferences().ReduceMotion
	wantFlow := !a.phone && a.settings.MotionArtwork && art != nil && !wantFog
	layer.Draw(func(p *ui.Painter, r ui.Rect) {
		now := p.Now()
		box, radius, alpha, sheetMoving := r, float32(0), float32(1), false
		if shape != nil {
			box, radius, alpha, sheetMoving = shape(r, now)
		}
		if sheetMoving {
			p.AnimationFrame()
		}
		// The mini player is already underneath this layer. When the
		// sheet fades, an opaque undercoat here would cover it.
		if alpha <= 0 || box.W <= 0 || box.H <= 0 {
			return
		}
		gpu := p.Accelerated()
		// Hold the last living frame through a pull or close. A first
		// opening can start with the static wash while the shader warms.
		living := a.livingBackdrop(art, gpu, still, sheetMoving && !bd.living)
		fog, flow := wantFog && !sheetMoving, wantFlow && !sheetMoving
		div := float32(8)
		if fog && gpu {
			div = 2
		} else if flow && gpu {
			div = 4
		}
		if a.phone && gpu && (fog || flow) {
			// Spend fewer pixels on the soft background so it can update
			// at the same cadence as desktop.
			div *= 2
		}
		cw, ch := int(r.W/div)+1, int(r.H/div)+1
		if bd.canvas != nil {
			newArt := bd.art != art || bd.tint != tint
			styleChanged := bd.fog != fog || bd.flow != flow || bd.living != living
			if newArt || styleChanged {
				bd.canvas, bd.before, bd.fadeAt = bd.before, bd.canvas, now
				bd.fadeFor = backdropFade
				if !newArt {
					bd.fadeFor = 180 * time.Millisecond
				}
			}
		}
		if bd.canvas != nil {
			if w, h := bd.canvas.Size(); w != cw || h != ch {
				bd.canvas = nil
			}
		}
		moving := (fog || flow || living) && gpu && !still && !sheetMoving
		due := now.Sub(bd.at) >= backdropFrame-2*time.Millisecond
		moved := moving && (pl.Playing() || bd.speed > 0 || bd.level.group(0, 6) > 0.004)
		if due || !a.phone {
			// Lyric-only frames keep the cached fog and skip audio
			// spectrum sampling until a new fog frame is due.
			var lv [player.Bands]float32
			var live bool
			if moving && fog && pl.Playing() {
				lv, live = pl.Levels()
			}
			moved = bd.step(now, pl.Playing() && moving, lv, live && moving)
		}
		if bd.canvas == nil || bd.art != art || bd.tint != tint || bd.fog != fog || bd.flow != flow || bd.living != living || moved && due {
			if bd.canvas == nil {
				bd.canvas = ui.NewCanvas(cw, ch)
			}
			bd.art, bd.tint, bd.fog, bd.living, bd.at = art, tint, fog, living, now
			bd.flow = flow
			bass := bd.level.group(0, 6)
			bd.canvas.Draw(p, func(cp *ui.Painter) {
				box := ui.Rect{W: float32(cw), H: float32(ch)}
				switch {
				case living:
					drawLivingBackdrop(cp, box, art, float32(bd.drift))
				case fog:
					drawFog(cp, box, tint, float32(bd.drift), bass)
				case flow:
					drawAlbumMesh(cp, box, art, tint, float32(bd.drift))
				default:
					drawBackdrop(cp, box, art, tint, a.phone)
				}
			})
		}
		paint := func() {
			// The canvas covers box as FillBox covers a rect, so a morphing
			// sheet shows the middle of it, not a stretched one.
			src := coverSrc(bd.canvas.Bitmap(), box)
			p.ImageRegion(bd.canvas.Bitmap(), src, box, ui.ImageOptions{Tint: white.Alpha(alpha)})
			if bd.before != nil {
				k := clamp01(float32(now.Sub(bd.fadeAt)) / float32(bd.fadeFor))
				mixing := false
				if bd.fadeFor == backdropFade && a.npVisual.current == art && a.npVisual.before != nil {
					k, mixing = a.visualProgress(now)
				}
				if k < 1 {
					p.ImageRegion(bd.before.Bitmap(), src, box, ui.ImageOptions{Tint: white.Alpha((1 - k) * alpha)})
					if !mixing || pl.Playing() {
						p.AnimationFrame()
					}
				} else {
					bd.before = nil
				}
			}
		}
		if shape != nil {
			p.Clip(box, radius, paint)
		} else {
			paint()
		}
		if moving && moved {
			if a.phone {
				p.AnimationFrameRate(backdropFPS)
			} else {
				p.After(backdropFrame)
			}
		}
	})
}

// livingBackdrop reports whether the phone's backdrop is the cover come
// alive this frame: the setting on, a blurred cover to swirl, a GPU to
// run the shader on, motion allowed, and the sheet at rest.
func (a *app) livingBackdrop(art *artwork, gpu, still, sheetMoving bool) bool {
	return a.phone && a.settings.phoneBackdropMotion() && art != nil && art.backdrop != nil && gpu && !still && !sheetMoving
}

// coverSrc is the part of bmp, canvas-sized, that covers dst as FillBox
// would draw it there: the middle of it, keeping its shape.
func coverSrc(b *ui.Bitmap, dst ui.Rect) ui.Rect {
	if b == nil {
		return ui.Rect{}
	}
	w, h := b.Size()
	fw, fh := float32(w), float32(h)
	if fw <= 0 || fh <= 0 || dst.W <= 0 || dst.H <= 0 {
		return ui.Rect{W: fw, H: fh}
	}
	s := max(dst.W/fw, dst.H/fh)
	return ui.Rect{X: (fw - dst.W/s) / 2, Y: (fh - dst.H/s) / 2, W: dst.W / s, H: dst.H / s}
}

// step moves the drift and the pulse on to now, and reports whether
// either still moves.
func (bd *backdropCache) step(now time.Time, playing bool, lv [player.Bands]float32, live bool) bool {
	dt := now.Sub(bd.last).Seconds()
	if bd.last.IsZero() || dt > 0.2 {
		dt = 1.0 / 30
	}
	bd.last = now
	to := 0.0
	if playing {
		to = 1
	}
	// The drift eases in as the music plays, and glides to a stop.
	bd.speed += (to - bd.speed) * min(1, 2.5*dt)
	if math.Abs(to-bd.speed) < 0.005 {
		bd.speed = to
	}
	bd.drift += bd.speed * dt
	bd.level.step(now, lv, live)
	return bd.speed > 0 || bd.level.group(0, 6) > 0.004
}

// drawBackdrop paints the still backdrop in r, as Apple's Now Playing
// does: the cover itself, blown up and blurred to a soft wash of its
// colors over a deep tint of them, under a vibrancy gradient that dips
// at the top and goes near black over the controls at the foot
// (MediaCoreUI's LiveColorAdjustmentsLayer: black .54 at the top, clear
// by .2, black at the foot).
func drawBackdrop(p *ui.Painter, r ui.Rect, art *artwork, tint ui.Color, phone bool) {
	top, foot := backdropColors(tint)
	p.FillGradient(r, ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
		{Color: top, At: 0}, {Color: foot, At: 1},
	}}, 0)
	if art != nil && art.backdrop != nil {
		// The blurred cover is the backdrop, not a tint over it: Apple's
		// variableBlur radius 50 at this scale reads as a smooth wash.
		p.ImageRegion(art.backdrop, ui.Rect{W: 48, H: 48}, coverRect(r, 48, 48), ui.ImageOptions{Tint: white.Alpha(0.85)})
	}
	drawVibrancy(p, r, phone)
}

// coverRect returns where an image of w×h goes to cover r, centered.
func coverRect(r ui.Rect, w, h float32) ui.Rect {
	s := max(r.W/w, r.H/h)
	return ui.Rect{X: r.X + (r.W-w*s)/2, Y: r.Y + (r.H-h*s)/2, W: w * s, H: h * s}
}

// backdropColors returns the top and the foot of the backdrop's gradient
// for a cover of tint: its hue, deep and rich, at the same depth whatever
// the cover, so that white reads over it, as NetEase's player paints it.
// A gray cover stays gray.
func backdropColors(tint ui.Color) (top, foot ui.Color) {
	h, s, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	if s > 0.12 {
		s = min(max(s*1.3, 0.45), 0.66)
	}
	return hsvColor(h, s, 0.45), hsvColor(h, min(s*1.05, 0.7), 0.3)
}

// drawFog paints the fog of tint in r at drift seconds, lit by bass.
func drawFog(p *ui.Painter, r ui.Rect, tint ui.Color, drift, bass float32) {
	deep, fog := fogColors(tint)
	fogShader().Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Time": drift, "Bass": min(bass, 1), "Deep": deep, "Fog": fog,
	}})
}

// fogColors returns the deep shade behind the fog and the fog's own for a
// cover of tint: its hue, rich, at the same depths whatever the cover, so
// that white reads over them. A gray cover stays gray.
func fogColors(tint ui.Color) (deep, fog ui.Color) {
	h, s, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	if s > 0.12 {
		s = min(max(s*1.3, 0.6), 0.85)
	}
	return hsvColor(h, s, 0.38), hsvColor(h, s*0.6, 0.78)
}

// hsvColor is the color of hue h (degrees), saturation s and value v.
func hsvColor(h, s, v float64) ui.Color {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	return ui.RGB(uint8((r+m)*255+0.5), uint8((g+m)*255+0.5), uint8((b+m)*255+0.5))
}

// backdropOver paints the backdrop again over band, a strip of win, the
// box the backdrop fills, as opaque at each height in band as alpha says
// for it, from 0 at its top to 1 at its foot: what moves under the strip
// fades into the backdrop with no edge, the very pixels around it where
// it is opaque.
func (a *app) backdropOver(p *ui.Painter, win, band ui.Rect, alpha func(t float32) float32) {
	bd := &a.backdropCache
	if bd.canvas == nil || win.W <= 0 || win.H <= 0 || band.H <= 0 {
		return
	}
	cw, ch := bd.canvas.Size()
	sx, sy := float32(cw)/win.W, float32(ch)/win.H
	// Its song's fading in over the song before's, as the backdrop.
	before := float32(0)
	if bd.before != nil {
		k := clamp01(float32(p.Now().Sub(bd.fadeAt)) / float32(backdropFade))
		if a.npVisual.current == bd.art && a.npVisual.before != nil {
			k, _ = a.visualProgress(p.Now())
		}
		before = 1 - k
	}
	const strip = 2
	for y := float32(0); y < band.H; y += strip {
		o := alpha((y + strip/2) / band.H)
		if o <= 0.004 {
			continue
		}
		dst := ui.Rect{X: band.X, Y: band.Y + y, W: band.W, H: min(strip, band.H-y)}
		src := ui.Rect{X: (dst.X - win.X) * sx, Y: (dst.Y - win.Y) * sy, W: dst.W * sx, H: dst.H * sy}
		p.ImageRegion(bd.canvas.Bitmap(), src, dst, ui.ImageOptions{Tint: white.Alpha(o)})
		if before > 0 {
			p.ImageRegion(bd.before.Bitmap(), src, dst, ui.ImageOptions{Tint: white.Alpha(o * before)})
		}
	}
}
