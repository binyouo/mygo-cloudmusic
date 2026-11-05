package app

import (
	"image"
	"math"
	"sync"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// Album colors drive a slowly moving mesh, the same rendering approach explored by AMLL.
const albumMeshSource = `package main
var Time float
var C0 vec4
var C1 vec4
var C2 vec4
var C3 vec4
func Fragment(dst vec4, pos vec2) vec4 {
 uv := pos / rectSize()
 t := Time * 0.12
 p0 := vec2(0.22+0.17*sin(t), 0.25+0.20*cos(t*0.83))
 p1 := vec2(0.78+0.18*cos(t*0.91), 0.22+0.19*sin(t*1.13))
 p2 := vec2(0.24+0.19*cos(t*0.71), 0.78+0.17*sin(t*0.93))
 p3 := vec2(0.77+0.17*sin(t*0.79), 0.76+0.20*cos(t*1.07))
 uv += vec2(sin(uv.y*4.0+t),cos(uv.x*3.0-t))*0.055
 d0 := uv-p0; d1 := uv-p1; d2 := uv-p2; d3 := uv-p3
 w0 := exp(-dot(d0,d0)*5.0); w1 := exp(-dot(d1,d1)*5.0)
 w2 := exp(-dot(d2,d2)*5.0); w3 := exp(-dot(d3,d3)*5.0)
 color := (C0.rgb*w0+C1.rgb*w1+C2.rgb*w2+C3.rgb*w3)/(w0+w1+w2+w3)
 shade := 0.92-0.16*smoothstep(0.35,1.0,uv.y)
 return vec4(color*shade,1)
}`

var albumMesh = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(albumMeshSource) })

// albumPalette clusters actual artwork colors rather than tinting every region with a single hue.
func albumPalette(img image.Image) [4]ui.Color {
	small := shrink(img, 16)
	var centers [4][3]float64
	distance := func(a, b [3]float64) float64 { x, y, z := a[0]-b[0], a[1]-b[1], a[2]-b[2]; return x*x + y*y + z*z }
	points := make([][3]float64, 0, 256)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			c := small.RGBAAt(x, y)
			h, s, v := hsv(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
			if s < .08 || v < .20 {
				continue
			}
			if s > .10 && v > .20 {
				s = min(.72, max(.30, s*1.6))
				v = .64
			} else {
				v = min(.48, max(.20, v))
			}
			c0 := hsvColor(h, s, v)
			points = append(points, [3]float64{float64(c0.R), float64(c0.G), float64(c0.B)})
		}
	}
	if len(points) < 4 {
		base := dominantColor(img).Mix(ui.RGB(90, 90, 100), .4)
		return [4]ui.Color{base, base, base, base}
	}
	centers[0] = points[0]
	for i := 1; i < 4; i++ {
		best := -1.0
		for _, point := range points {
			nearest := math.MaxFloat64
			for j := 0; j < i; j++ {
				nearest = min(nearest, distance(point, centers[j]))
			}
			if nearest > best {
				best = nearest
				centers[i] = point
			}
		}
	}
	for iteration := 0; iteration < 6; iteration++ {
		var sums [4][3]float64
		var counts [4]int
		for _, point := range points {
			index, best := 0, math.MaxFloat64
			for i, center := range centers {
				if d := distance(point, center); d < best {
					index, best = i, d
				}
			}
			counts[index]++
			for ch := range 3 {
				sums[index][ch] += point[ch]
			}
		}
		for i := range 4 {
			if counts[i] > 0 {
				for ch := range 3 {
					centers[i][ch] = sums[i][ch] / float64(counts[i])
				}
			}
		}
	}
	var palette [4]ui.Color
	for i, c := range centers {
		luma := (.2126*c[0] + .7152*c[1] + .0722*c[2]) / 255
		scale := 1.0
		if luma > .46 {
			scale = .46 / luma
		}
		for ch := range 3 {
			c[ch] *= scale
			if luma < .12 {
				c[ch] = c[ch]*.75 + 30
			}
		}
		palette[i] = ui.RGB(uint8(c[0]), uint8(c[1]), uint8(c[2]))
	}
	return palette
}

func drawAlbumMesh(p *ui.Painter, r ui.Rect, art *artwork, tint ui.Color, clock float32) {
	colors := [4]ui.Color{tint, tint.Mix(white, .1), tint, tint.Mix(white, .05)}
	if art != nil && art.palette[0] != (ui.Color{}) {
		colors = art.palette
	}
	albumMesh().Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Time": clock, "C0": colors[0], "C1": colors[1], "C2": colors[2], "C3": colors[3],
	}})
}
