// Command icon draws the app icons: the layers of AppIcon.icon, the Icon
// Composer document that macOS 26 and later render in Liquid Glass, and
// AppIcon-dev.icon, cream where the app's is red. From them it renders the
// PNGs the other platforms take: resources/icon.png and
// resources/icon-dev.png, opaque and full-bleed, as iOS wants its app icon
// (and as Windows, Linux and Android scale theirs); and the dark
// variant's files of the home screen's alternate icon,
// resources/AppIconDark*.png, which the app offers in its settings.
//
// Run it from the repository root on a Mac with Xcode 26 or later:
//
//	go run ./icon
//
// The icon.json of each document, its background, glass and dark-mode
// colors, is edited by hand or in Icon Composer.
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// icons are the documents drawn, with the PNG rendered from each, the dark
// alternate icon's file names, and the colors of its tonearm: the gradient
// and the cap on its pivot.
var icons = []struct {
	document, output, dark string
	top, bottom, pivot     string
}{
	// Red, with a white tonearm.
	{"icon/AppIcon.icon", "resources/icon.png", "AppIconDark", "#FFFFFF", "#E6E3E5", "#D9D5D8"},
	// Cream, with a red one to match the label.
	{"icon/AppIcon-dev.icon", "resources/icon-dev.png", "", "#FF5B5B", "#E3122C", "#FFE1DE"},
}

// A record on Icon Composer's 1024-point full-bleed canvas, seen close up:
// larger than the canvas, it runs off the left and bottom edges. And the
// tonearm over it, from its pivot to the headshell resting on the grooves.
var (
	rec   = struct{ cx, cy, r float64 }{330, 712, 560}
	pivot = image.Pt(820, 214)
	head  = image.Pt(585, 520)
)

// darkFiles are the files of the dark alternate icon, at the home screen's
// sizes of each device, named as CFBundleIconFiles names them.
var darkFiles = []struct {
	file string
	px   int
}{
	{"resources/AppIconDark60x60@2x.png", 120},
	{"resources/AppIconDark60x60@3x.png", 180},
	{"resources/AppIconDark76x76@2x~ipad.png", 152},
	{"resources/AppIconDark83.5x83.5@2x~ipad.png", 167},
}

func main() {
	for _, ic := range icons {
		layers := map[string]string{
			"arm.svg":    armSVG(ic.top, ic.bottom, ic.pivot),
			"record.svg": recordSVG(),
			"label.svg":  labelSVG(),
		}
		for name, s := range layers {
			if err := os.WriteFile(filepath.Join(ic.document, "Assets", name), []byte(s), 0o644); err != nil {
				log.Fatal(err)
			}
		}
		fills, err := iconFills(ic.document)
		if err != nil {
			log.Fatal(err)
		}
		if err := writeIcon(ic.document, ic.output, fills.def); err != nil {
			log.Fatal(err)
		}
		if ic.dark != "" {
			if err := writeDark(ic.document, ic.dark, fills.dark); err != nil {
				log.Fatal(err)
			}
		}
	}
}

func svg(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">` + "\n" + body + "\n</svg>\n"
}

func n(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func armSVG(top, bottom, cap string) string {
	p := pivot
	length := math.Hypot(float64(head.X-p.X), float64(head.Y-p.Y))
	angle := math.Atan2(float64(head.Y-p.Y), float64(head.X-p.X)) * 180 / math.Pi
	end := float64(p.X) + length - 30
	return svg(fmt.Sprintf(`<defs>
<linearGradient id="m" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%[11]s"/><stop offset="1" stop-color="%[12]s"/></linearGradient>
</defs>
<g transform="rotate(%[3]s %[1]d %[2]d)">
<rect x="%[4]d" y="%[5]d" width="64" height="52" rx="15" fill="url(#m)"/>
<rect x="%[1]d" y="%[6]d" width="%[7]s" height="34" rx="17" fill="url(#m)"/>
<rect x="%[8]s" y="%[9]d" width="92" height="56" rx="14" fill="url(#m)" transform="rotate(28 %[10]s %[2]d)"/>
</g>
<circle cx="%[1]d" cy="%[2]d" r="72" fill="url(#m)"/>
<circle cx="%[1]d" cy="%[2]d" r="27" fill="%[13]s"/>`,
		p.X, p.Y, n(angle), p.X-104, p.Y-26, p.Y-17, n(length-30), n(end-22), p.Y-28, n(end), top, bottom, cap))
}

func recordSVG() string {
	var b strings.Builder
	fmt.Fprintf(&b, `<defs>
<radialGradient id="d" cx="%[1]s" cy="%[2]s" r="%[3]s" gradientUnits="userSpaceOnUse">
<stop offset="0.35" stop-color="#2A2A30"/><stop offset="0.95" stop-color="#17171B"/><stop offset="1" stop-color="#0E0E11"/></radialGradient>
</defs>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#d)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[4]s" fill="none" stroke="#FFFFFF" stroke-opacity="0.14" stroke-width="3"/>
`, n(rec.cx), n(rec.cy), n(rec.r), n(rec.r-8))
	// Grooves, unevenly bright like the bands of tracks.
	for i, r := 0, rec.r*0.42; r < rec.r-12; i, r = i+1, r+7 {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-opacity="%s" stroke-width="1.4"/>`+"\n",
			n(rec.cx), n(rec.cy), n(r), n([]float64{0.05, 0.08, 0.04, 0.07}[i%4]))
	}
	b.WriteString(sheen(-62, 54, 0.2, rec.r*0.39, rec.r-10))
	b.WriteString(sheen(118, 54, 0.14, rec.r*0.39, rec.r-10))
	return svg(strings.TrimSuffix(b.String(), "\n"))
}

// sheen is a soft band of reflected light across the grooves, centered on
// the angle mid, in degrees: slices whose opacity falls off smoothly from
// the band's middle, as SVG has no conic gradient.
func sheen(mid, width, peak, r0, r1 float64) string {
	const slices = 48
	pt := func(a, r float64) string {
		a *= math.Pi / 180
		return n(rec.cx+math.Cos(a)*r) + " " + n(rec.cy+math.Sin(a)*r)
	}
	var b strings.Builder
	for i := range slices {
		a0 := mid - width/2 + width*float64(i)/slices
		a1 := a0 + width/slices + 0.15 // overlap, so no seams show
		t := (float64(i)+0.5)/slices*2 - 1
		o := math.Round(peak*math.Exp(-t*t*3.2)*1000) / 1000
		fmt.Fprintf(&b, `<path d="M%sL%sA%s %s 0 0 1 %sL%sA%s %s 0 0 0 %sZ" fill="#FFFFFF" fill-opacity="%s"/>`+"\n",
			pt(a0, r0), pt(a0, r1), n(r1), n(r1), pt(a1, r1), pt(a1, r0), n(r0), n(r0), pt(a0, r0), n(o))
	}
	return b.String()
}

func labelSVG() string {
	return svg(fmt.Sprintf(`<defs>
<linearGradient id="l" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#FF5B5B"/><stop offset="1" stop-color="#E3122C"/></linearGradient>
</defs>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#l)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[4]s" fill="none" stroke="#FFFFFF" stroke-opacity="0.28" stroke-width="3"/>
<circle cx="%[1]s" cy="%[2]s" r="%[5]s" fill="#FFF6F2"/>`, n(rec.cx), n(rec.cy), n(rec.r*0.358), n(rec.r*0.247), n(rec.r*0.052)))
}

// The Icon Composer's command line tool, inside Xcode.
func ictool() (string, error) {
	if out, err := exec.Command("xcode-select", "-p").Output(); err == nil {
		p := filepath.Join(strings.TrimSpace(string(out)), "..",
			"Applications", "Icon Composer.app", "Contents", "Executables", "ictool")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	const p = "/Applications/Xcode.app/Contents/Applications/Icon Composer.app/Contents/Executables/ictool"
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("no ictool (it comes with Xcode 26 or later): %v", err)
	}
	return p, nil
}

// export draws document as iOS shows rendition of it, at the icon's full
// 1024 pixels, in a temporary directory.
func export(document, rendition string) (image.Image, error) {
	tool, err := ictool()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "icon")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	drawn := filepath.Join(dir, "icon.png")
	if out, err := exec.Command(tool, document, "--export-image", "--output-file", drawn,
		"--platform", "iOS", "--rendition", rendition, "--width", "1024", "--height", "1024", "--scale", "1").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ictool (it comes with Xcode 26 or later): %v\n%s", err, out)
	}
	// ictool draws in Display P3, and the PNG this writes carries no
	// profile, which readers take as sRGB: convert, or the reds go dull.
	if out, err := exec.Command("sips", "--matchTo", "/System/Library/ColorSync/Profiles/sRGB Profile.icc", drawn).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("sips: %v\n%s", err, out)
	}
	f, err := os.Open(drawn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// writeIcon renders document for iOS and flattens it over its background
// gradient: the system's squircle ends where the render's alpha does, so
// the corners take the document's fill, opaque as icons must be.
func writeIcon(document, output string, bg image.Image) error {
	drawn, err := export(document, "Default")
	if err != nil {
		return err
	}
	return writePNG(output, flattened(bg, drawn))
}

// writeDark renders document's dark appearance for iOS and writes the
// alternate icon's files at the home screen's sizes.
func writeDark(document, name string, bg image.Image) error {
	drawn, err := export(document, "Dark")
	if err != nil {
		return err
	}
	icon := flattened(bg, drawn)
	for _, f := range darkFiles {
		out := image.NewNRGBA(image.Rect(0, 0, f.px, f.px))
		xdraw.CatmullRom.Scale(out, out.Bounds(), icon, icon.Bounds(), xdraw.Over, nil)
		path := strings.Replace(f.file, "AppIconDark", name, 1)
		if err := writePNG(path, out); err != nil {
			return err
		}
	}
	return nil
}

// flattened draws img over bg, opaque, for icons that take no transparency.
func flattened(bg, img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, bg, image.Point{}, draw.Src)
	draw.Draw(out, b, img, b.Min, draw.Over)
	return out
}

func writePNG(path string, img image.Image) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// iconFills are the document's background gradients: the default one and
// the dark appearance's, each a two-stop gradient across the canvas.
func iconFills(document string) (fills struct{ def, dark image.Image }, err error) {
	var doc struct {
		Specs []struct {
			Appearance string `json:"appearance"`
			Value      struct {
				Stops       []string `json:"linear-gradient"`
				Orientation struct {
					Start, Stop struct{ X, Y float64 }
				} `json:"orientation"`
			} `json:"value"`
		} `json:"fill-specializations"`
	}
	data, err := os.ReadFile(filepath.Join(document, "icon.json"))
	if err != nil {
		return fills, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fills, fmt.Errorf("%s/icon.json: %w", document, err)
	}
	for _, s := range doc.Specs {
		g, err := gradient(s.Value.Stops, s.Value.Orientation.Start.X, s.Value.Orientation.Start.Y,
			s.Value.Orientation.Stop.X, s.Value.Orientation.Stop.Y)
		if err != nil {
			return fills, fmt.Errorf("%s/icon.json: %w", document, err)
		}
		if s.Appearance == "dark" {
			fills.dark = g
		} else {
			fills.def = g
		}
	}
	if fills.def == nil || fills.dark == nil {
		return fills, fmt.Errorf("%s/icon.json: fill-specializations need a default and a dark gradient", document)
	}
	return fills, nil
}

// linearGradient is a two-stop gradient over the 1024-pixel canvas, from
// (x0, y0) to (x1, y1), each a fraction of the canvas's side.
type linearGradient struct {
	x0, y0, x1, y1 float64
	c0, c1         color.NRGBA
}

func gradient(stops []string, x0, y0, x1, y1 float64) (*linearGradient, error) {
	if len(stops) != 2 {
		return nil, fmt.Errorf("gradient has %d stops, wants 2", len(stops))
	}
	c0, err := srgb(stops[0])
	if err != nil {
		return nil, err
	}
	c1, err := srgb(stops[1])
	if err != nil {
		return nil, err
	}
	return &linearGradient{x0, y0, x1, y1, c0, c1}, nil
}

// srgb parses a color of icon.json's, "srgb:r,g,b,a" with 0-1 channels.
func srgb(s string) (color.NRGBA, error) {
	v, ok := strings.CutPrefix(s, "srgb:")
	var c color.NRGBA
	if !ok {
		return c, fmt.Errorf("%q is not an srgb color", s)
	}
	f := strings.Split(v, ",")
	if len(f) != 4 {
		return c, fmt.Errorf("%q is not r,g,b,a", s)
	}
	var ch [4]float64
	for i, s := range f {
		if _, err := fmt.Sscanf(s, "%f", &ch[i]); err != nil {
			return c, fmt.Errorf("%q: %w", s, err)
		}
	}
	c = color.NRGBA{uint8(math.Round(ch[0] * 255)), uint8(math.Round(ch[1] * 255)),
		uint8(math.Round(ch[2] * 255)), uint8(math.Round(ch[3] * 255))}
	return c, nil
}

func (*linearGradient) ColorModel() color.Model { return color.NRGBAModel }

func (g *linearGradient) Bounds() image.Rectangle { return image.Rect(0, 0, 1024, 1024) }

func (g *linearGradient) At(x, y int) color.Color {
	dx, dy := g.x1-g.x0, g.y1-g.y0
	t := ((float64(x)-g.x0*1024)*dx + (float64(y)-g.y0*1024)*dy) / (dx*dx + dy*dy)
	t = max(0, min(1, t))
	return color.NRGBA{
		uint8(math.Round(float64(g.c0.R)*(1-t) + float64(g.c1.R)*t)),
		uint8(math.Round(float64(g.c0.G)*(1-t) + float64(g.c1.G)*t)),
		uint8(math.Round(float64(g.c0.B)*(1-t) + float64(g.c1.B)*t)),
		uint8(math.Round(float64(g.c0.A)*(1-t) + float64(g.c1.A)*t)),
	}
}
