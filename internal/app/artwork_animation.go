package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/gif"
	"time"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

// Animated covers keep bounded composited frames, so disposal and partial WebP frames render correctly.
type coverAnimation struct {
	artworkClock
	frames []*image.RGBA
	ends   []time.Duration
	cycles int
	memory int
	shown  int
	bmp    *ui.Bitmap // animation's texture; the original static artwork stays intact.
}

type artworkClock struct {
	elapsed time.Duration
	last    time.Time
	playing bool
}

func (a *artworkClock) advance(now time.Time, playing bool) {
	if !a.last.IsZero() && a.playing && playing {
		a.elapsed += max(0, now.Sub(a.last))
	}
	a.last, a.playing = now, playing
}

const animationBudget = 24 << 20

// animationDecodeBudget also covers GIF's retained source frames and its
// composition canvases, before DecodeAll has allocated any frame pixels.
const animationDecodeBudget = 64 << 20

// bitmap updates only the animation's texture, leaving the full-size static
// cover available when motion is disabled or the animation cannot be shown.
func (a *coverAnimation) bitmap(index int) *ui.Bitmap {
	if a == nil || index < 0 || index >= len(a.frames) {
		return nil
	}
	if a.bmp == nil {
		a.bmp = ui.NewBitmap(a.frames[index])
		a.shown = index
	} else if a.shown != index {
		a.bmp.Update(a.frames[index])
		a.shown = index
	}
	return a.bmp
}

func (a *coverAnimation) add(canvas image.Image, delay time.Duration) bool {
	b := canvas.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) > 384 {
		w, h = w*384/max(w, h), h*384/max(w, h)
	}
	if w < 1 || h < 1 || a.memory+w*h*4 > animationBudget || len(a.frames) >= 240 {
		return false
	}
	frame := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(frame, frame.Bounds(), canvas, b, draw.Src, nil)
	a.frames = append(a.frames, frame)
	total := time.Duration(0)
	if len(a.ends) > 0 {
		total = a.ends[len(a.ends)-1]
	}
	a.ends = append(a.ends, total+max(20*time.Millisecond, delay))
	a.memory += len(frame.Pix)
	return true
}

// step's clock advances only across two playing observations; resuming never counts paused wall time.
func (a *coverAnimation) step(now time.Time, playing bool) (int, time.Duration) {
	a.advance(now, playing)
	total := a.ends[len(a.ends)-1]
	if a.cycles > 0 && a.elapsed >= total*time.Duration(a.cycles) {
		return len(a.frames) - 1, 0
	}
	position := a.elapsed % total
	for i, end := range a.ends {
		if position < end {
			return i, end - position
		}
	}
	return 0, 0
}

func decodeCoverAnimation(data []byte) *coverAnimation {
	if len(data) >= 6 && (string(data[:6]) == "GIF89a" || string(data[:6]) == "GIF87a") {
		if !gifAnimationFits(data) {
			return nil
		}
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil || len(g.Image) < 2 {
			return nil
		}
		a := &coverAnimation{shown: -1}
		if g.LoopCount < 0 {
			a.cycles = 1
		} else if g.LoopCount > 0 {
			a.cycles = g.LoopCount + 1
		}
		canvas := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
		var previous *image.RGBA
		for i, frame := range g.Image {
			if i > 0 && i-1 < len(g.Disposal) {
				switch g.Disposal[i-1] {
				case gif.DisposalBackground:
					draw.Draw(canvas, g.Image[i-1].Bounds(), image.Transparent, image.Point{}, draw.Src)
				case gif.DisposalPrevious:
					if previous != nil {
						draw.Draw(canvas, canvas.Bounds(), previous, image.Point{}, draw.Src)
					}
				}
			}
			if i < len(g.Disposal) && g.Disposal[i] == gif.DisposalPrevious {
				previous = image.NewRGBA(canvas.Bounds())
				draw.Draw(previous, previous.Bounds(), canvas, image.Point{}, draw.Src)
			}
			draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
			if !a.add(canvas, time.Duration(g.Delay[i])*10*time.Millisecond) {
				return nil
			}
		}
		return a
	}
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil
	}
	var canvas *image.RGBA
	a := &coverAnimation{shown: -1}
	var disposed image.Rectangle
	uint24 := func(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }
	for rest := data[12:]; len(rest) >= 8; {
		n := int(binary.LittleEndian.Uint32(rest[4:8]))
		if n > len(rest)-8 {
			return nil
		}
		name, body := string(rest[:4]), rest[8:8+n]
		whole := rest[:8+n]
		rest = rest[min(len(rest), 8+n+(n&1)):]
		switch name {
		case "VP8X":
			if len(body) < 10 || body[0]&2 == 0 {
				return nil
			}
			w, h := uint24(body[4:7])+1, uint24(body[7:10])+1
			if w*h > 4096*4096 {
				return nil
			}
			canvas = image.NewRGBA(image.Rect(0, 0, w, h))
		case "ANIM":
			if len(body) >= 6 {
				a.cycles = int(binary.LittleEndian.Uint16(body[4:6]))
			}
		case "ANMF":
			if canvas == nil || len(body) < 16 {
				return nil
			}
			x, y := uint24(body[:3])*2, uint24(body[3:6])*2
			r := image.Rect(x, y, x+uint24(body[6:9])+1, y+uint24(body[9:12])+1)
			if !r.In(canvas.Bounds()) {
				return nil
			}
			if !disposed.Empty() {
				draw.Draw(canvas, disposed, image.Transparent, image.Point{}, draw.Src)
			}
			riff := binary.LittleEndian.AppendUint32([]byte("RIFF"), uint32(4+len(whole)))
			riff = append(append(riff, []byte("WEBP")...), whole...)
			frame, _, err := image.Decode(bytes.NewReader(stillCover(riff)))
			if err != nil {
				return nil
			}
			op := draw.Over
			if body[15]&2 != 0 {
				op = draw.Src
			}
			draw.Draw(canvas, r, frame, frame.Bounds().Min, op)
			if !a.add(canvas, time.Duration(uint24(body[12:15]))*time.Millisecond) {
				return nil
			}
			disposed = image.Rectangle{}
			if body[15]&1 != 0 {
				disposed = r
			}
		}
	}
	if len(a.frames) < 2 {
		return nil
	}
	return a
}

// gifAnimationFits scans descriptors and sub-block lengths without decoding
// LZW data. DecodeAll keeps every source frame, so a budget checked only when
// composing resized frames would not limit its peak memory.
func gifAnimationFits(data []byte) bool {
	if len(data) < 13 {
		return false
	}
	width := int(binary.LittleEndian.Uint16(data[6:8]))
	height := int(binary.LittleEndian.Uint16(data[8:10]))
	if width < 1 || height < 1 || uint64(width)*uint64(height) > 4096*4096 {
		return false
	}
	w, h := width, height
	if max(w, h) > 384 {
		w, h = w*384/max(w, h), h*384/max(w, h)
	}
	if w < 1 || h < 1 {
		return false
	}
	// One RGBA canvas plus a saved canvas for DisposalPrevious. The decoded
	// paletted frames and resized RGBA frames are accounted for below.
	canvasBytes := width * height * 8
	if canvasBytes > animationDecodeBudget {
		return false
	}
	frameBytes := w * h * 4
	at := 13
	if data[10]&0x80 != 0 {
		at += 3 * (1 << (int(data[10]&7) + 1))
	}
	skipBlocks := func() bool {
		for at < len(data) {
			n := int(data[at])
			at++
			if n == 0 {
				return true
			}
			if n > len(data)-at {
				return false
			}
			at += n
		}
		return false
	}
	frames, sourceBytes := 0, 0
	for at < len(data) {
		kind := data[at]
		at++
		switch kind {
		case 0x21: // An extension: label, followed by size-prefixed blocks.
			if at >= len(data) {
				return false
			}
			at++
			if !skipBlocks() {
				return false
			}
		case 0x2c: // An image descriptor, optional palette, then LZW blocks.
			if len(data)-at < 9 {
				return false
			}
			d := data[at : at+9]
			left, top := int(binary.LittleEndian.Uint16(d[:2])), int(binary.LittleEndian.Uint16(d[2:4]))
			fw, fh := int(binary.LittleEndian.Uint16(d[4:6])), int(binary.LittleEndian.Uint16(d[6:8]))
			if fw < 1 || fh < 1 || left+fw > width || top+fh > height {
				return false
			}
			frames++
			// A paletted frame uses one byte per pixel; allow a full local
			// palette and frame bookkeeping even if it shares the global one.
			sourceBytes += fw*fh + (8 << 10)
			retainedBytes := frames * frameBytes
			if frames > 240 || retainedBytes > animationBudget || canvasBytes+sourceBytes+retainedBytes > animationDecodeBudget {
				return false
			}
			at += 9
			if d[8]&0x80 != 0 {
				at += 3 * (1 << (int(d[8]&7) + 1))
			}
			if at >= len(data) || data[at] < 2 || data[at] > 8 {
				return false
			}
			at++
			if !skipBlocks() {
				return false
			}
		case 0x3b:
			return frames >= 2
		default:
			return false
		}
	}
	return false
}
