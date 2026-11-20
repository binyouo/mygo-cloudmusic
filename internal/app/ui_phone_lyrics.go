package app

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
)

// joinLyricWords joins the words of a line back into its text: karaoke
// timings may normalise the line's whitespace, so the words are what a
// word-timed line shows, sung or not.
func joinLyricWords(words []lyrics.Word) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(w.Text)
	}
	return b.String()
}

// utf16Len is the length of s in UTF-16 code units, the offsets a
// TextLayout's ranges take.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// phoneKaraoke is the cached shape of a phone line's words for the wipe:
// the layout it draws, and each word's rectangles in it, so a frame that
// only paints never shapes.
type phoneKaraoke struct {
	words  *lyrics.Word
	n      int
	size   float32
	weight int
	height float32
	lang   string
	width  float32
	layout *ui.TextLayout
	rects  [][]ui.Rect
}

// phoneLyricWords draws the line being sung on the phone as Apple Music
// does: the words dim, and the sung part wipes bright across them as a
// soft front, in frames that only paint. The Text element gives the line
// its place and height; everything seen comes from the layout.
func (a *app) phoneLyricWords(c *ui.Context, words []lyrics.Word, st linesStyle, lang string) {
	joined := joinLyricWords(words)
	text := ui.Text(c.Key("phone-words"), joined).Lang(lang).FontSize(st.size).FontWeight(st.weight).
		LineHeight(st.lineHeight).LetterSpacing(-0.4).TextColor(ui.Transparent)
	kc := ui.Local(text, "wipe", func() *phoneKaraoke { return nil })
	text.DrawOver(func(p *ui.Painter, r ui.Rect) {
		k := *kc
		if k == nil || k.words != &words[0] || k.n != len(words) || k.size != st.size ||
			k.weight != st.weight || k.height != st.lineHeight || k.lang != lang || k.width != r.W {
			k = shapePhoneKaraoke(words, st, lang, r.W)
			*kc = k
		}
		a.paintPhoneKaraoke(p, r, k, words, st)
	})
}

// shapePhoneKaraoke lays the joined words out as the line shows them and
// keeps each word's share of it, by its UTF-16 range.
func shapePhoneKaraoke(words []lyrics.Word, st linesStyle, lang string, w float32) *phoneKaraoke {
	layout := ui.ShapeRichText(
		[]ui.Span{{Text: joinLyricWords(words), Size: st.size, Weight: st.weight, Lang: lang, LetterSpacing: -0.4}},
		ui.Font{Size: st.size, Weight: st.weight, Lang: lang, LineHeight: st.lineHeight}, w+0.5)
	rects := make([][]ui.Rect, len(words))
	at := 0
	for i, wd := range words {
		n := utf16Len(wd.Text)
		rects[i] = layout.SelectionRects(ui.TextInputRange{Start: at, End: at + n})
		at += n
	}
	return &phoneKaraoke{words: &words[0], n: len(words), size: st.size, weight: st.weight,
		height: st.lineHeight, lang: lang, width: w, layout: layout, rects: rects}
}

// paintPhoneKaraoke paints the wipe: the words dim under the sung part
// drawn bright, line by line up to where the word being sung has got,
// then a soft ramp a little ahead of it.
func (a *app) paintPhoneKaraoke(p *ui.Painter, r ui.Rect, k *phoneKaraoke, words []lyrics.Word, st linesStyle) {
	pos := a.pl.Position()
	p.TextLayout(k.layout, r.X, r.Y, white.Alpha(0.35))
	// A band is one visual line of the layout, lit to right of its left.
	type band struct {
		y, h, right float32
	}
	var bands []band
	lit := func(rc ui.Rect, right float32) {
		for i := range bands {
			if b := &bands[i]; b.y == rc.Y && b.h == rc.H {
				b.right = max(b.right, right)
				return
			}
		}
		bands = append(bands, band{rc.Y, rc.H, right})
	}
	var edgeX, edgeY, edgeH float32
	edge := false
	var next time.Duration
	for i, wd := range words {
		span := max(50*time.Millisecond, wd.End-wd.Start)
		switch {
		case pos >= wd.Start+span:
			for _, rc := range k.rects[i] {
				lit(rc, rc.X+rc.W)
			}
		case pos >= wd.Start:
			f := float64(pos-wd.Start) / float64(span)
			var total float32
			for _, rc := range k.rects[i] {
				total += rc.W
			}
			rem := f * float64(total)
			for _, rc := range k.rects[i] {
				if rem > float64(rc.W) {
					lit(rc, rc.X+rc.W)
					rem -= float64(rc.W)
					continue
				}
				lit(rc, rc.X+float32(rem))
				edgeX, edgeY, edgeH, edge = rc.X+float32(rem), rc.Y, rc.H, true
				break
			}
		default:
			if d := wd.Start - pos; next == 0 || d < next {
				next = d
			}
		}
	}
	// The bright text draws inside clips, a little wider than the text on
	// the left, where a glyph's ink can pass its bounds.
	for _, b := range bands {
		right := b.right
		p.Clip(ui.Rect{X: r.X - 2, Y: r.Y + b.y, W: right + 2, H: b.h}, 0, func() {
			p.TextLayout(k.layout, r.X, r.Y, white)
		})
	}
	if edge {
		// A soft leading edge eases from bright into the dim ahead.
		fw := st.size * 0.5
		for j := range 4 {
			alpha := 0.35 + 0.65*(1-(float32(j)+0.5)/4)
			p.Clip(ui.Rect{X: r.X + edgeX + float32(j)*fw/4, Y: r.Y + edgeY, W: fw / 4, H: edgeH}, 0, func() {
				p.TextLayout(k.layout, r.X, r.Y, white.Alpha(alpha))
			})
		}
	}
	if a.pl.Playing() && !st.quiet && !a.phonePlayerMoving {
		if edge {
			p.AnimationFrameRate(60)
		} else if next > 0 {
			p.After(next)
		}
	}
}

// lyricOmegaPhone is the phone lyric glide's ω: critically damped with
// the spring the rows' poses run, it settles within half a pixel in well
// under the budget lyricScrollTime gives a line change.
const lyricOmegaPhone = 14

// lyricRowSpring is the pose spring of a phone lyric row's glide: its
// numbers are lyricOmegaPhone's, damping 2ω, mass 1.
var lyricRowSpring = ui.SpringOptions{Mass: 1, Stiffness: lyricOmegaPhone * lyricOmegaPhone, Damping: 2 * lyricOmegaPhone, Precision: 0.5}

// phoneLag is a row's lag τ after it was let go: a critically damped
// fall from a at rest, a·(1+ωτ)e^(−ωτ), a until then.
func phoneLag(a float64, tau time.Duration) float64 {
	t := tau.Seconds()
	if t <= 0 {
		return a
	}
	return a * (1 + lyricOmegaPhone*t) * math.Exp(-lyricOmegaPhone*t)
}
