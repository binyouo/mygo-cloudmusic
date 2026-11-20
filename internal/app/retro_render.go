package app

import (
	"image"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/ui"
)

// The retro players are rendered, not drawn: each part is shaded pixel by
// pixel once, as light from the top left falls on its metal, plastic,
// vinyl and paper, into bitmaps that the players then only place, turn
// and scale. What never moves is one bitmap; what turns, a tonearm or a
// reel's hub, its own, with a soft shadow to go under it.

// texel is a color, premultiplied by how much of the pixel it covers.
type texel struct{ r, g, b, a float64 }

type rgb struct{ r, g, b float64 }

func gray(v float64) rgb               { return rgb{v, v, v} }
func (c rgb) scale(k float64) rgb      { return rgb{c.r * k, c.g * k, c.b * k} }
func (c rgb) plus(o rgb) rgb           { return rgb{c.r + o.r, c.g + o.g, c.b + o.b} }
func (c rgb) mix(o rgb, t float64) rgb { return c.scale(1 - t).plus(o.scale(t)) }

// over lays c over the pixel, covering cover of it.
func (t *texel) over(c rgb, cover float64) {
	if cover <= 0 {
		return
	}
	cover = min(cover, 1)
	k := 1 - cover
	t.r, t.g, t.b = c.r*cover+t.r*k, c.g*cover+t.g*k, c.b*cover+t.b*k
	t.a = cover + t.a*k
}

// shade darkens or lightens what is on the pixel, by multiplying it by k
// where it covers.
func (t *texel) shade(k, cover float64) {
	f := 1 + (k-1)*clampF(cover)
	t.r, t.g, t.b = t.r*f, t.g*f, t.b*f
}

// render shades a w by h bitmap, of px pixels a unit, calling fn for the
// middle of each pixel, in units, on all the cores.
func render(w, h int, px float64, fn func(x, y float64, t *texel)) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rows := make(chan int, h)
	for y := range h {
		rows <- y
	}
	close(rows)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for y := range rows {
				for x := range w {
					var t texel
					fn((float64(x)+0.5)/px, (float64(y)+0.5)/px, &t)
					i := img.PixOffset(x, y)
					img.Pix[i] = byte(clampF(t.r)*255 + 0.5)
					img.Pix[i+1] = byte(clampF(t.g)*255 + 0.5)
					img.Pix[i+2] = byte(clampF(t.b)*255 + 0.5)
					img.Pix[i+3] = byte(clampF(t.a)*255 + 0.5)
				}
			}
		})
	}
	wg.Wait()
	return img
}

func clampF(v float64) float64 { return max(0, min(v, 1)) }

func smooth(a, b, x float64) float64 {
	t := clampF((x - a) / (b - a))
	return t * t * (3 - 2*t)
}

// coverage is how much of a pixel aa across a shape at signed distance d
// covers.
func coverage(d, aa float64) float64 { return clampF(0.5 - d/aa) }

func sdCircle(x, y, cx, cy, r float64) float64 { return math.Hypot(x-cx, y-cy) - r }

// sdBox is the signed distance to a rectangle from (x0, y0) to (x1, y1),
// its corners rounded by r.
func sdBox(x, y, x0, y0, x1, y1, r float64) float64 {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	hx, hy := (x1-x0)/2-r, (y1-y0)/2-r
	qx, qy := math.Abs(x-cx)-hx, math.Abs(y-cy)-hy
	return math.Hypot(max(qx, 0), max(qy, 0)) + min(max(qx, qy), 0) - r
}

// sdSegment is the signed distance to a capsule r wide around the segment
// from (ax, ay) to (bx, by).
func sdSegment(x, y, ax, ay, bx, by, r float64) float64 {
	px, py, dx, dy := x-ax, y-ay, bx-ax, by-ay
	h := clampF((px*dx + py*dy) / (dx*dx + dy*dy))
	return math.Hypot(px-dx*h, py-dy*h) - r
}

// hash is a number from 0 to 1 that looks random, the same for the same
// i and j.
func hash(i, j int) float64 {
	h := uint32(i)*374761393 + uint32(j)*668265263
	h = (h ^ h>>13) * 1274126177
	return float64(h^h>>16) / float64(1<<32)
}

// noise is smooth value noise.
func noise(x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	ix, iy := int(fx), int(fy)
	tx, ty := x-fx, y-fy
	tx, ty = tx*tx*(3-2*tx), ty*ty*(3-2*ty)
	a, b := hash(ix, iy), hash(ix+1, iy)
	c, d := hash(ix, iy+1), hash(ix+1, iy+1)
	return (a+(b-a)*tx)*(1-ty) + (c+(d-c)*tx)*ty
}

// vec is a direction: x right, y down and z out of the screen.
type vec struct{ x, y, z float64 }

func (v vec) norm() vec {
	l := math.Sqrt(v.x*v.x + v.y*v.y + v.z*v.z)
	return vec{v.x / l, v.y / l, v.z / l}
}
func (v vec) dot(o vec) float64 { return v.x*o.x + v.y*o.y + v.z*o.z }

// lights are a soft window up to the top left, key, and a dimmer one to
// the right, fill.
type lights struct{ key, fill vec }

var room = lights{vec{-0.45, -0.65, 1}.norm(), vec{0.7, 0.2, 1}.norm()}

// lightTurn is which way the key light is, around what faces the viewer.
var lightTurn = math.Atan2(room.key.y, room.key.x)

// turned is the lights as something turned by angle, in radians, sees
// them.
func (l lights) turned(angle float64) lights {
	turn := func(v vec) vec {
		c, s := math.Cos(-angle), math.Sin(-angle)
		return vec{v.x*c - v.y*s, v.x*s + v.y*c, v.z}
	}
	return lights{turn(l.key), turn(l.fill)}
}

// flat is the normal of what faces the viewer.
var flat = vec{0, 0, 1}

// diffuse is how lit a matte surface facing n is, 1 where it faces the
// viewer.
func (l lights) diffuse(n vec) float64 {
	return (max(0, n.dot(l.key))*0.8 + max(0, n.dot(l.fill))*0.2) / (l.key.z*0.8 + l.fill.z*0.2)
}

// gloss is the highlight of the lights on a surface facing n, as sharp as
// power.
func (l lights) gloss(n vec, power float64) float64 {
	h := vec{l.key.x, l.key.y, l.key.z + 1}.norm()
	h2 := vec{l.fill.x, l.fill.y, l.fill.z + 1}.norm()
	return math.Pow(max(0, n.dot(h)), power) + 0.35*math.Pow(max(0, n.dot(h2)), power)
}

// chrome is a mirror facing n: what it reflects of the room, the lights
// in it, a bright ceiling and a darker floor.
func (l lights) chrome(n vec) float64 {
	r := vec{2 * n.z * n.x, 2 * n.z * n.y, 2*n.z*n.z - 1}
	v := 0.16 + 0.3*max(0, r.z)
	v += 0.95*math.Pow(max(0, r.dot(l.key)), 24) + 0.35*math.Pow(max(0, r.dot(l.key)), 4)
	v += 0.4 * math.Pow(max(0, r.dot(l.fill)), 16)
	return v
}

// bevel is the normal at distance d inside the edge of a shape whose
// edge rounds off over width, going away along the gradient (gx, gy) of
// d; flat further in.
func bevel(d, gx, gy, width float64) vec {
	t := clampF(-d / width)
	if t >= 1 {
		return flat
	}
	// A quarter round: steep at the edge, level where it ends.
	s := (1 - t) / math.Sqrt(max(1e-3, 1-(1-t)*(1-t)))
	s = min(s, 6)
	return vec{gx * s, gy * s, 1}.norm()
}

// grad is the gradient of sd at (x, y), e across.
func grad(sd func(x, y float64) float64, x, y, e float64) (float64, float64) {
	gx := (sd(x+e, y) - sd(x-e, y)) / (2 * e)
	gy := (sd(x, y+e) - sd(x, y-e)) / (2 * e)
	l := math.Hypot(gx, gy)
	if l == 0 {
		return 0, 0
	}
	return gx / l, gy / l
}

// sheen is the highlight of grooves or brushing in circles around a
// point, at angle from it: two bright wedges, toward the light and away
// from it, as sharp as width is small.
func sheen(angle, width float64) float64 {
	s := math.Sin(angle - lightTurn)
	return math.Exp(-s * s / width)
}

// blurAlpha is the shadow of img: its coverage, blurred by r pixels, in
// black.
func blurAlpha(img *image.RGBA, r int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	a := make([]float64, w*h)
	for y := range h {
		for x := range w {
			a[y*w+x] = float64(img.Pix[img.PixOffset(x, y)+3]) / 255
		}
	}
	tmp := make([]float64, w*h)
	for range 3 {
		for y := range h {
			sum := 0.0
			for x := -r; x < w+r; x++ {
				if x+r < w {
					sum += a[y*w+x+r]
				}
				if x-r-1 >= 0 {
					sum -= a[y*w+x-r-1]
				}
				if x >= 0 && x < w {
					tmp[y*w+x] = sum / float64(2*r+1)
				}
			}
		}
		for x := range w {
			sum := 0.0
			for y := -r; y < h+r; y++ {
				if y+r < h {
					sum += tmp[(y+r)*w+x]
				}
				if y-r-1 >= 0 {
					sum -= tmp[(y-r-1)*w+x]
				}
				if y >= 0 && y < h {
					a[y*w+x] = sum / float64(2*r+1)
				}
			}
		}
	}
	out := image.NewRGBA(b)
	for i, v := range a {
		out.Pix[i*4+3] = byte(clampF(v)*255 + 0.5)
	}
	return out
}

// The turntable, in units of its width. The record turns around deckC,
// the platter around it, and the tonearm turns around deckPivot.
const (
	deckCX, deckCY         = 0.445, 0.5
	deckPivotX, deckPivotY = 0.835, 0.185
	deckPlatter            = 0.415 // the platter's radius
	deckRecord             = 0.385 // the record's
	deckLabel              = 0.36  // the label's, of the record's
	deckRadius             = 0.045 // the corners of the plinth
	// deckRest is the angle the tonearm rests at, clear of the platter:
	// pointing down, a little out.
	deckRest = math.Pi/2 - 0.08
)

// deckRestX and deckRestY are where the tonearm's rest is, under it.
var deckRestX, deckRestY = deckPivotX + 0.42*math.Cos(deckRest), deckPivotY + 0.42*math.Sin(deckRest)

// deckLampX and deckLampY are where the strobe's lamp is, by the platter.
const deckLampX, deckLampY = 0.127, 0.818

// deckPixels is how many pixels across the turntable is rendered at:
// enough for one 440 DIPs across on a screen of twice as many pixels.
const deckPixels = 1024

// renderDeck is the turntable without what moves: the plinth, the
// platter and the record on it, the base of the tonearm, its rest and
// the controls.
func renderDeck() *image.RGBA {
	const n = deckPixels
	aa := 1.0 / n
	lit := room
	plinth := func(x, y float64) float64 { return sdBox(x, y, 0, 0, 1, 1, deckRadius) }
	return render(n, n, n, func(x, y float64, t *texel) {
		// The plinth: black anodized aluminium, brushed left to right,
		// its edges rounded off and lit along the top and left.
		d := plinth(x, y)
		if d > aa {
			return
		}
		brushed := (noise(x*7, y*1400)-0.5)*0.035 + (noise(x*40, y*3000)-0.5)*0.02
		base := 0.082 + brushed + 0.035*(1-(x+y)/2) + 0.03*math.Exp(-sq((x-0.3)/0.28))
		gx, gy := grad(plinth, x, y, aa)
		nrm := bevel(d, gx, gy, 0.016)
		t.over(gray(base*lit.diffuse(nrm)+0.22*lit.gloss(nrm, 40)), coverage(d, aa))

		// Shapes raised off the plinth, as it is, with shadows under them.
		raised := func(sd func(x, y float64) float64, color rgb, round, shine float64, shadow float64) {
			if s := sd(x-0.004, y-0.007); shadow > 0 && s < 0.03 {
				t.shade(1-shadow*(1-smooth(-0.01, 0.03, s)), 1)
			}
			d := sd(x, y)
			if d > aa {
				return
			}
			gx, gy := grad(sd, x, y, aa)
			nrm := bevel(d, gx, gy, round)
			c := color.scale(lit.diffuse(nrm)).plus(gray(shine * lit.gloss(nrm, 50)))
			t.over(c, coverage(d, aa))
		}
		recessed := func(sd func(x, y float64) float64, color rgb) {
			d := sd(x, y)
			if d > aa {
				return
			}
			gx, gy := grad(sd, x, y, aa)
			nrm := bevel(d, -gx, -gy, 0.006)
			t.over(color.scale(lit.diffuse(nrm)), coverage(d, aa))
		}

		// The platter's shadow, wide and soft.
		r := math.Hypot(x-deckCX, y-deckCY)
		if s := math.Hypot(x-deckCX-0.006, y-deckCY-0.012) - deckPlatter; s < 0.05 {
			t.shade(1-0.75*(1-smooth(-0.02, 0.05, s)), 1)
		}
		if r < deckPlatter+aa {
			angle := math.Atan2(y-deckCY, x-deckCX)
			platter(x, y, r, angle, aa, t)
		}

		// The tonearm's base: a boss on the plinth, a ring of brushed
		// metal turned on a lathe, knurled around its edge, and the
		// socket the arm's gimbal sits in.
		px, py := deckPivotX, deckPivotY
		raised(func(x, y float64) float64 { return sdCircle(x, y, px, py, 0.088) }, gray(0.1), 0.012, 0.25, 0.5)
		if pr := math.Hypot(x-px, y-py); pr < 0.066 {
			a := math.Atan2(y-py, x-px)
			ring := 0.42 + 0.35*sheen(a, 0.02) + 0.15*sheen(a, 0.3) + (hash(int(pr*n), 7)-0.5)*0.06
			if pr > 0.058 {
				// Knurls.
				k := math.Sin(a * 90)
				ring *= 0.8 + 0.2*k
			}
			nrm := bevel(pr-0.066, (x-px)/pr, (y-py)/pr, 0.004)
			t.over(gray(ring*lit.diffuse(nrm)), coverage(pr-0.066, aa))
			t.over(gray(0.05), coverage(pr-0.046, aa)*0.9)
		}

		// The arm's rest: a post with a rubber cradle.
		rx, ry := deckRestX, deckRestY
		raised(func(x, y float64) float64 { return sdCircle(x, y, rx+0.002, ry, 0.016) }, gray(0.55), 0.016, 0.5, 0.45)
		raised(func(x, y float64) float64 { return sdBox(x, y, rx-0.021, ry-0.011, rx+0.021, ry+0.011, 0.006) }, gray(0.05), 0.006, 0.15, 0.3)
		// The cue lever, above it.
		raised(func(x, y float64) float64 { return sdCircle(x, y, 0.915, 0.5, 0.022) }, gray(0.11), 0.01, 0.25, 0.4)
		raised(func(x, y float64) float64 { return sdSegment(x, y, 0.915, 0.5, 0.968, 0.5, 0.006) }, gray(0.6), 0.006, 0.6, 0.45)
		raised(func(x, y float64) float64 { return sdCircle(x, y, 0.968, 0.5, 0.009) }, gray(0.08), 0.009, 0.5, 0.3)
		// The anti-skating knob, knurled.
		if kr := math.Hypot(x-0.94, y-0.3); kr < 0.06 {
			a := math.Atan2(y-0.3, x-0.94)
			knob := func(x, y float64) float64 { return sdCircle(x, y, 0.94, 0.3, 0.026) }
			raised(knob, gray(0.12+0.03*math.Sin(a*48)), 0.008, 0.35, 0.5)
			if kr < 0.018 {
				t.over(gray(0.18+0.25*sheen(a, 0.05)), coverage(kr-0.018, aa))
			}
			// Its mark.
			t.over(gray(0.85), coverage(sdSegment(x, y, 0.94, 0.3-0.006, 0.94, 0.3-0.02, 0.0012), aa))
		}

		// The pitch control: a slot, a scale beside it and a fader.
		recessed(func(x, y float64) float64 { return sdBox(x, y, 0.922, 0.8, 0.936, 0.955, 0.007) }, gray(0.025))
		for i := range 9 {
			ty := 0.81 + float64(i)*0.0169
			w := 0.008
			if i == 4 {
				w = 0.014
			}
			t.over(gray(0.5), coverage(sdSegment(x, y, 0.914-w, ty, 0.912, ty, 0.001), aa))
		}
		fader := func(x, y float64) float64 { return sdBox(x, y, 0.906, 0.862, 0.952, 0.894, 0.005) }
		raised(fader, gray(0.5), 0.004, 0.55, 0.55)
		if fader(x, y) < 0 {
			for i := range 5 {
				ly := 0.868 + float64(i)*0.005
				t.shade(0.6, coverage(sdSegment(x, y, 0.912, ly, 0.946, ly, 0.0008), aa))
			}
		}

		// The strobe's lamp: its housing, and red glass in it.
		raised(func(x, y float64) float64 { return sdCircle(x, y, deckLampX, deckLampY, 0.027) }, gray(0.1), 0.01, 0.3, 0.45)
		if lr := math.Hypot(x-deckLampX, y-deckLampY); lr < 0.016 {
			nrm := vec{(x - deckLampX) / 0.016 * 0.6, (y - deckLampY) / 0.016 * 0.6, 1}.norm()
			t.over(rgb{0.22, 0.03, 0.03}.plus(gray(0.6*lit.gloss(nrm, 60))), coverage(lr-0.016, aa))
		}

		// Start and stop, and the speeds.
		raised(func(x, y float64) float64 { return sdBox(x, y, 0.035, 0.905, 0.16, 0.963, 0.01) }, gray(0.58), 0.008, 0.6, 0.6)
		raised(func(x, y float64) float64 { return sdBox(x, y, 0.178, 0.918, 0.232, 0.956, 0.008) }, gray(0.1), 0.007, 0.3, 0.5)
		raised(func(x, y float64) float64 { return sdBox(x, y, 0.246, 0.918, 0.3, 0.956, 0.008) }, gray(0.1), 0.007, 0.3, 0.5)

		// A little grain, as of a photograph.
		t.shade(1+(hash(int(x*n), int(y*n))-0.5)*0.05, 1)
	})
}

// platter shades the platter at (x, y), r from its middle at angle: its
// rim of polished aluminium ringed with the strobe's dots, the rubber
// mat, and the record on it.
func platter(x, y, r, angle, aa float64, t *texel) {
	const n = deckPixels
	lit := room
	edge := r - deckPlatter
	// The rim, turned on a lathe: brushed in circles.
	rim := 0.5 + 0.38*sheen(angle, 0.012) + 0.12*sheen(angle, 0.25) + (hash(int(r*n*0.8), 3)-0.5)*0.05
	nrm := bevel(edge, (x-deckCX)/r, (y-deckCY)/r, 0.008)
	c := gray(rim * lit.diffuse(nrm)).plus(gray(0.3 * lit.gloss(nrm, 30)))
	// The strobe's dots, in rows: mirrors between darker gaps.
	for i, count := range []float64{180, 167, 138} {
		row := deckRecord + 0.01 + float64(i)*0.0072
		if math.Abs(r-row) < 0.0028 {
			f := math.Mod((angle/(2*math.Pi)+1)*count, 1)
			dot := smooth(0.15, 0.25, f) * (1 - smooth(0.75, 0.85, f))
			c = c.scale(0.72 + 0.38*dot)
		}
	}
	t.over(c, coverage(edge, aa))

	// The mat, a little wider than the record, in its shadow.
	if r < deckRecord+0.005 {
		mat := 0.06 + (hash(int(x*n), int(y*n))-0.5)*0.02
		t.over(gray(mat*(0.55+0.45*smooth(deckRecord, deckRecord+0.005, r))), coverage(r-deckRecord-0.005, aa))
	}
	if r > deckRecord+aa {
		return
	}

	// The record: black vinyl, its grooves catching the light in two
	// wedges, brighter on the louder passages and fainter on the quiet,
	// smooth between the tracks, in the lead-in and the run-out.
	f := r / deckRecord
	gaps := []float64{0.53, 0.62, 0.7, 0.79, 0.88}
	smoothBand := f < deckLabel*1.2 || f > 0.965
	for _, g := range gaps {
		if math.Abs(f-g) < 0.006 {
			smoothBand = true
		}
	}
	base := rgb{0.03, 0.03, 0.034}
	reach := 0.55 + 0.45*f
	var spec float64
	if smoothBand {
		spec = (0.42*sheen(angle, 0.0018) + 0.08*sheen(angle, 0.05)) * reach
	} else {
		groove := 0.55 + 0.45*hash(int(r*n*0.7), 11)
		loud := 0.55 + 0.45*noise(r*140, 3.5)
		spec = (0.6*sheen(angle, 0.004) + 0.2*sheen(angle, 0.06) + 0.07*sheen(angle, 0.6) + 0.035) * groove * loud * reach
	}
	c = base.plus(rgb{0.85, 0.88, 0.95}.scale(spec))
	// Its rim, a bead rounded off.
	if f > 0.985 {
		nrm := bevel(r-deckRecord, (x-deckCX)/r, (y-deckCY)/r, deckRecord*0.015)
		c = c.scale(lit.diffuse(nrm)).plus(gray(0.5 * lit.gloss(nrm, 40)))
	}
	t.over(c, coverage(r-deckRecord, aa))
}

func sq(v float64) float64 { return v * v }

// The tonearm, laid along x from its pivot: a tube to the headshell,
// turned headshellTurn toward the record, the counterweight behind the
// pivot.
const (
	armBack       = 0.17  // how far its bitmap goes behind the pivot
	armFront      = 0.66  // and in front
	armSide       = 0.085 // and to each side
	armTube       = 0.49  // where the tube ends
	headshellTurn = 20 * math.Pi / 180
	stylusAt      = 0.1 // how far along the headshell the stylus is
)

// armStylus is where the stylus is, from the pivot, along the arm and
// across it.
func armStylus() (x, y float64) {
	return armTube - 0.005 + stylusAt*math.Cos(headshellTurn), stylusAt * math.Sin(headshellTurn)
}

// renderArm is the tonearm, its pivot armBack from the left of the
// bitmap and armSide from its top.
func renderArm() *image.RGBA {
	const n = deckPixels
	aa := 1.0 / n
	span, side := (armBack+armFront)*n, 2*armSide*n
	w, h := int(span), int(side)
	// Shaded once, as it lies about as it does on a record, for all the
	// angles it lies at.
	lit := room.turned(105 * math.Pi / 180)
	return render(w, h, n, func(x, y float64, t *texel) {
		x, y = x-armBack, y-armSide
		// The headshell, in its own frame.
		hx := (x-armTube+0.005)*math.Cos(headshellTurn) + y*math.Sin(headshellTurn)
		hy := -(x-armTube+0.005)*math.Sin(headshellTurn) + y*math.Cos(headshellTurn)
		inHead := func(sd func(hx, hy float64) float64) func(x, y float64) float64 {
			return func(x, y float64) float64 {
				hx := (x-armTube+0.005)*math.Cos(headshellTurn) + y*math.Sin(headshellTurn)
				hy := -(x-armTube+0.005)*math.Sin(headshellTurn) + y*math.Cos(headshellTurn)
				return sd(hx, hy)
			}
		}
		solid := func(sd func(x, y float64) float64, color rgb, round, shine float64) {
			d := sd(x, y)
			if d > aa {
				return
			}
			gx, gy := grad(sd, x, y, aa)
			nrm := bevel(d, gx, gy, round)
			t.over(color.scale(lit.diffuse(nrm)).plus(gray(shine*lit.gloss(nrm, 60))), coverage(d, aa))
		}
		// A chrome cylinder along a segment: shaded across it.
		tube := func(ax, ay, bx, by, r float64) {
			d := sdSegment(x, y, ax, ay, bx, by, r)
			if d > aa {
				return
			}
			dx, dy := bx-ax, by-ay
			l := math.Hypot(dx, dy)
			across := ((x-ax)*-dy + (y-ay)*dx) / l / r
			across = max(-1, min(across, 1))
			nx, ny := -dy/l*across, dx/l*across
			t.over(gray(lit.chrome(vec{nx, ny, math.Sqrt(max(0, 1-across*across))})), coverage(d, aa))
		}

		// The finger lift, out from the headshell's side.
		solid(inHead(func(hx, hy float64) float64 { return sdSegment(hx, hy, 0.06, -0.022, 0.078, -0.044, 0.003) }), gray(0.75), 0.0032, 0.9)
		// The headshell, brushed aluminium, and the cartridge's nose
		// out of its end.
		head := inHead(func(hx, hy float64) float64 { return sdBox(hx, hy, -0.012, -0.025, stylusAt-0.006, 0.025, 0.01) })
		if d := head(x, y); d < aa {
			gx, gy := grad(head, x, y, aa)
			nrm := bevel(d, gx, gy, 0.006)
			v := 0.6 + (noise(hx*900, hy*20)-0.5)*0.08
			t.over(gray(v*lit.diffuse(nrm)).plus(gray(0.45*lit.gloss(nrm, 30))), coverage(d, aa))
		}
		solid(inHead(func(hx, hy float64) float64 {
			return sdBox(hx, hy, stylusAt-0.012, -0.015, stylusAt+0.012, 0.015, 0.004)
		}), rgb{0.09, 0.09, 0.1}, 0.005, 0.8)
		solid(inHead(func(hx, hy float64) float64 {
			return sdBox(hx, hy, stylusAt+0.004, -0.009, stylusAt+0.012, 0.009, 0.003)
		}), rgb{0.55, 0.08, 0.06}, 0.003, 0.6)
		for _, sx := range []float64{0.022, 0.058} {
			solid(inHead(func(hx, hy float64) float64 { return sdCircle(hx, hy, sx, 0, 0.0058) }), gray(0.7), 0.0058, 1)
			if math.Hypot(hx-sx, hy) < 0.0058 {
				t.shade(0.35, coverage(math.Abs(hy)-0.0009, aa))
			}
		}

		// The tube, and the collar where the headshell plugs into it.
		tube(0, 0, armTube-0.01, 0, 0.0095)
		tube(armTube-0.024, 0, armTube+0.004, 0, 0.0135)
		// The counterweight's shaft and the weight on it, knurled.
		tube(-0.08, 0, 0, 0, 0.006)
		cw := func(x, y float64) float64 { return sdBox(x, y, -0.162, -0.037, -0.072, 0.037, 0.008) }
		if d := cw(x, y); d < aa {
			across := clampF((y+0.037)/0.074)*2 - 1
			// Knurled in a band around its middle, smooth either side.
			knurl := 0.0
			if x > -0.135 && x < -0.1 {
				knurl = 0.12 * math.Sin(x*2*math.Pi*260)
			}
			nrm := vec{knurl, across * 0.9, math.Sqrt(max(0.05, 1-across*across))}.norm()
			t.over(gray(0.35*lit.chrome(nrm)+0.04), coverage(d, aa))
		}
		// The gimbal: a dark ring and a chrome dome.
		solid(func(x, y float64) float64 { return sdCircle(x, y, 0, 0, 0.041) }, gray(0.1), 0.008, 0.4)
		if r := math.Hypot(x, y); r < 0.029 {
			nx, ny := x/0.029, y/0.029
			t.over(gray(lit.chrome(vec{nx, ny, math.Sqrt(max(0, 1-nx*nx-ny*ny))})), coverage(r-0.029, aa))
		}
	})
}

// The cassette, in units of its width.
const (
	tapeHeight  = 0.64
	tapePixels  = 1024
	tapeReelY   = 0.315
	tapeReelL   = 0.2875
	tapeReelR   = 0.7125
	tapeHub     = 0.055 // the hub's radius, where the tape starts
	tapeFull    = 0.19  // a full reel's
	tapeCorner  = 0.035
	tapeLabelY0 = 0.045
	tapeLabelY1 = 0.445
)

// tapeWindow is the window over the reels: a rectangle with round ends.
func tapeWindow(x, y float64) float64 {
	return sdBox(x, y, 0.2, 0.24, 0.8, 0.39, 0.075)
}

// renderCassette is the cassette's shell over its reels: black plastic,
// its paper label, the window clear, and the openings along its foot
// where the tape shows.
func renderCassette() *image.RGBA {
	const n = tapePixels
	aa := 1.0 / n
	lit := room
	shell := func(x, y float64) float64 { return sdBox(x, y, 0, 0, 1, tapeHeight, tapeCorner) }
	label := func(x, y float64) float64 { return sdBox(x, y, 0.06, tapeLabelY0, 0.94, tapeLabelY1, 0.018) }
	foot := func(x, y float64) float64 {
		// A trapezoid, its top corners rounded.
		if y < 0.52 {
			return math.Max(sdBox(x, y, 0.2, 0.52, 0.8, tapeHeight+0.05, 0.02), 0.52-y)
		}
		slope := (y - 0.52) / (tapeHeight - 0.52) * 0.065
		return math.Max(math.Max(0.235-slope-x, x-(0.765+slope)), 0.52-y)
	}
	tall := tapeHeight * n
	return render(n, int(tall), n, func(x, y float64, t *texel) {
		d := shell(x, y)
		if d > aa {
			return
		}
		// Black plastic, faintly textured, its edges rounded and a seam
		// just inside them.
		grain := (noise(x*300, y*300)-0.5)*0.025 + (hash(int(x*n), int(y*n))-0.5)*0.02
		gx, gy := grad(shell, x, y, aa)
		nrm := bevel(d, gx, gy, 0.012)
		base := 0.075 + grain + 0.025*(1-(x+y)/1.6)
		t.over(gray(base*lit.diffuse(nrm)+0.18*lit.gloss(nrm, 30)), coverage(d, aa))
		if s := math.Abs(d + 0.016); s < 0.002 {
			t.shade(0.6, 1-s/0.002)
		}

		// The foot, raised, its openings dark, the tape across them.
		if f := foot(x, y); f < aa {
			gx, gy := grad(foot, x, y, aa)
			nrm := bevel(f, gx, gy, 0.01)
			t.over(gray((0.09+grain)*lit.diffuse(nrm)+0.15*lit.gloss(nrm, 30)), coverage(f, aa))
			holes := []func(x, y float64) float64{
				func(x, y float64) float64 { return sdCircle(x, y, 0.29, 0.585, 0.02) },
				func(x, y float64) float64 { return sdCircle(x, y, 0.71, 0.585, 0.02) },
				func(x, y float64) float64 { return sdBox(x, y, 0.405, 0.565, 0.435, 0.595, 0.004) },
				func(x, y float64) float64 { return sdBox(x, y, 0.565, 0.565, 0.595, 0.595, 0.004) },
				func(x, y float64) float64 { return sdBox(x, y, 0.45, 0.6, 0.55, 0.7, 0.004) },
			}
			for _, hole := range holes {
				h := hole(x, y)
				if h > aa {
					continue
				}
				gx, gy := grad(hole, x, y, aa)
				inner := bevel(h, -gx, -gy, 0.005)
				t.over(gray(0.02*lit.diffuse(inner)+0.01), coverage(h, aa))
				// The tape, brown, along the bottom, and the felt pad
				// pressing it in the middle.
				if y > 0.612 && y < 0.626 {
					t.over(rgb{0.2, 0.12, 0.07}.scale(0.8+0.4*smooth(0.612, 0.62, y)), coverage(h, aa))
				}
				if x > 0.48 && x < 0.52 && y > 0.6 && y < 0.612 {
					t.over(rgb{0.75, 0.7, 0.6}.scale(0.7+0.2*noise(x*800, y*800)), coverage(h, aa))
				}
			}
		}

		// The label: cream paper, its fibers, a ruled frame, and lines
		// to write on.
		if l := label(x, y); l < aa {
			paper := rgb{0.93, 0.9, 0.82}
			fiber := (noise(x*180, y*900)-0.5)*0.03 + (hash(int(x*n), int(y*n))-0.5)*0.025
			v := 0.97 + fiber - 0.06*smooth(-0.02, 0, l)
			c := paper.scale(v)
			ink := rgb{0.35, 0.32, 0.3}
			if f := math.Abs(sdBox(x, y, 0.075, tapeLabelY0+0.015, 0.925, tapeLabelY1-0.015, 0.01)); f < 0.0012 {
				c = c.mix(ink, 0.5*(1-f/0.0012))
			}
			for _, ly := range []float64{0.155, 0.212} {
				if f := math.Abs(y - ly); f < 0.0009 && x > 0.09 && x < 0.91 {
					c = c.mix(ink, 0.35*(1-f/0.0009))
				}
			}
			t.over(c, coverage(l, aa))
		}

		// The window: clear plastic, set in, a little light on it.
		if wd := tapeWindow(x, y); wd < aa {
			gx, gy := grad(tapeWindow, x, y, aa)
			// Its frame, a step down into the shell.
			if wd > -0.008 {
				nrm := bevel(wd, -gx, -gy, 0.008)
				frame := gray(0.06*lit.diffuse(nrm) + 0.2*lit.gloss(nrm, 40))
				t.r, t.g, t.b, t.a = 0, 0, 0, 0
				t.over(frame, coverage(wd, aa)*(1-smooth(-0.008, -0.006, wd)))
			} else {
				t.r, t.g, t.b, t.a = 0, 0, 0, 0
			}
			// The glass: a streak of light across it, and its rim.
			streak := math.Exp(-sq((x-0.9*y-0.12)/0.04))*0.12 + math.Exp(-sq((x-0.9*y-0.2)/0.012))*0.1
			glass := 0.05 + streak
			if wd < -0.006 {
				t.over(gray(0.9), glass*coverage(wd+0.006, aa))
			}
		}

		// The screws: domed, a cross in each.
		for _, s := range [][2]float64{{0.04, 0.04}, {0.96, 0.04}, {0.04, 0.6}, {0.96, 0.6}, {0.5, 0.535}} {
			sr := math.Hypot(x-s[0], y-s[1])
			if sr > 0.022 {
				continue
			}
			// Its countersink, shadowed.
			t.shade(0.55, coverage(sr-0.018, aa))
			if sr < 0.0125+aa {
				nx, ny := (x-s[0])/0.0125, (y-s[1])/0.0125
				nrm := vec{nx * 0.7, ny * 0.7, math.Sqrt(max(0.05, 1-0.49*(nx*nx+ny*ny)))}.norm()
				t.over(gray(0.5*lit.chrome(nrm)+0.1), coverage(sr-0.0125, aa))
				cross := min(math.Abs(x-s[0])+0.0002, math.Abs(y-s[1])+0.0002)
				if math.Max(math.Abs(x-s[0]), math.Abs(y-s[1])) < 0.008 {
					t.shade(0.25, coverage(cross-0.0016, aa))
				}
			}
		}
	})
}

// renderHub is a reel's hub, tapeHub across, in its middle: white
// plastic, toothed inside to drive it, shaded alike all around, as it
// turns.
func renderHub() *image.RGBA {
	const n = tapePixels
	aa := 1.0 / n
	across := 2 * tapeHub * n
	size := int(across) + 2
	mid := float64(size) / 2 / n
	return render(size, size, n, func(x, y float64, t *texel) {
		r := math.Hypot(x-mid, y-mid)
		a := math.Atan2(y-mid, x-mid)
		if r > tapeHub+aa {
			return
		}
		plastic := rgb{0.9, 0.88, 0.83}
		// Lighter on its rim, darker into the hole.
		v := 0.82 + 0.15*smooth(0.034, tapeHub, r) - 0.12*smooth(tapeHub-0.006, tapeHub, r)
		t.over(plastic.scale(v), coverage(r-tapeHub, aa))
		// A groove around it.
		if g := math.Abs(r - 0.047); g < 0.0012 {
			t.shade(0.75, 1-g/0.0012)
		}
		// The hole, and the six teeth into it.
		hole := r - 0.033
		for k := range 6 {
			ta := float64(k)*math.Pi/3 - a
			ta = math.Atan2(math.Sin(ta), math.Cos(ta))
			// Across the tooth, in units.
			across := math.Abs(ta) * r
			if across < 0.004 && r > 0.022 {
				hole = max(hole, 0.004-across)
			}
		}
		if hole < aa {
			t.over(gray(0.03), coverage(hole, aa))
		}
	})
}

// retroArt is what the players are made of, rendered once.
type retroArt struct {
	deck, arm, armShadow, cassette, hub *ui.Bitmap
}

var retroBitmaps = sync.OnceValue(func() retroArt {
	var art retroArt
	var wg sync.WaitGroup
	wg.Go(func() { art.deck = ui.NewBitmap(renderDeck()) })
	wg.Go(func() {
		arm := renderArm()
		art.arm, art.armShadow = ui.NewBitmap(arm), ui.NewBitmap(blurAlpha(arm, 9))
	})
	wg.Go(func() { art.cassette = ui.NewBitmap(renderCassette()) })
	wg.Go(func() { art.hub = ui.NewBitmap(renderHub()) })
	wg.Wait()
	return art
})

// Prepare the shaded parts off the UI thread; the first view never waits for rendering.
var retroStarted sync.Once
var retroIsReady atomic.Bool

func prepareRetro() {
	retroStarted.Do(func() { go func() { retroBitmaps(); retroIsReady.Store(true) }() })
}
func retroLoaded() bool { return retroIsReady.Load() }
