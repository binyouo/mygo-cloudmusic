package app

import (
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// The phone's living backdrop is the cover itself, alive as Apple's
// Now Playing backdrop is: copies of the blurred cover, rotated and
// swirled a little at a time so slowly that a canvas an eighth of the
// window across, painted thirty times a second, still looks perfect.
// Vibrancy keeps it rich and a little dark, so white reads over it.
const livingSource = `package main

var Time float

func rot(p vec2, a float) vec2 {
	s := sin(a)
	c := cos(a)
	return vec2(c*p.x-s*p.y, s*p.x+c*p.y)
}

// cover samples the blurred cover at p, in cover widths from its center,
// mirrored past its edges.
func cover(p vec2) vec3 {
	q := abs(fract((p+0.5)*0.5)*2.0 - 1.0)
	return image0Sample(q * image0Size()).rgb
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	p := (pos - size*0.5) / max(size.x, size.y)
	t := Time
	// A slow swirl, stronger toward the edges, as MediaCoreUI's pinch.
	p = rot(p, 0.35*sin(t*0.11)*length(p))
	a := cover(rot(p-vec2(0.18*sin(t*0.07), 0.12*cos(t*0.05)), t*0.045) / 1.35)
	b := cover(rot(p+vec2(0.22*cos(t*0.06), 0.16*sin(t*0.08)), 2.1-t*0.06) / 0.95)
	d := cover(rot(p+vec2(0.1*sin(t*0.09+1.0), -0.2), 4.2+t*0.03) / 1.9)
	col := a*0.5 + b*0.3 + d*0.2
	l := dot(col, vec3(0.2126, 0.7152, 0.0722))
	col = mix(vec3(l), col, 0.92) * 0.82
	return vec4(clamp(col, 0.0, 1.0), 1)
}
`

var livingShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(livingSource) })

// drawLivingBackdrop paints the living cover in r at clock seconds, then
// the vibrancy gradient drawBackdrop lays over its wash.
func drawLivingBackdrop(p *ui.Painter, r ui.Rect, art *artwork, clock float32) {
	livingShader().Draw(p, r, shader.Options{
		Uniforms: map[string]any{"Time": clock},
		Images:   [4]*ui.Bitmap{art.backdrop},
	})
	drawVibrancy(p, r, true)
}

// drawVibrancy lays the vertical color adjustment of drawBackdrop over r:
// dimmed a little at the top, clear through the middle, near black at the
// foot.
func drawVibrancy(p *ui.Painter, r ui.Rect, phone bool) {
	if phone {
		// The artwork keeps its color near the cover. A cooler shade
		// behind the controls keeps it legible with very bright covers.
		ink := ui.Hex("#101016")
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: ink.Alpha(0.60), At: 0},
			{Color: ink.Alpha(0.32), At: 0.20},
			{Color: ink.Alpha(0.43), At: 0.50},
			{Color: ink.Alpha(0.78), At: 0.76},
			{Color: ink.Alpha(0.98), At: 1},
		}}, 0)
		return
	}
	p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
		{Color: ui.RGBA(0, 0, 0, 0.5), At: 0},
		{Color: ui.RGBA(0, 0, 0, 0.06), At: 0.14},
		{Color: ui.RGBA(0, 0, 0, 0), At: 0.22},
		{Color: ui.RGBA(0, 0, 0, 0.3), At: 0.62},
		{Color: ui.RGBA(0, 0, 0, 0.72), At: 0.85},
		{Color: ui.RGBA(0, 0, 0, 0.92), At: 1},
	}}, 0)
}
