package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// appIcon is the app's icon: a dark glyph on a light (or transparent) background.
// Source of truth: build/appicon.png (also turned into the .ico / .icns by `task icons`).
//
//go:embed build/appicon.png
var appIcon []byte

// trayIcon makes a one-color tray icon from src: the glyph in c on a transparent background.
// A transparent src keeps its alpha; an opaque one (a dark glyph on white)
// gets its alpha from darkness, so white goes away and edges stay smooth.
// Black suits light taskbars and macOS template icons; white suits dark taskbars.
func trayIcon(src []byte, c color.NRGBA) []byte {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	b := img.Bounds()
	in := image.NewNRGBA(b)
	draw.Draw(in, b, img, b.Min, draw.Src)

	opaque := true
	for i := 3; i < len(in.Pix); i += 4 {
		if in.Pix[i] != 255 {
			opaque = false
			break
		}
	}

	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := in.NRGBAAt(x, y)
			a := p.A
			if opaque {
				lum := (299*uint32(p.R) + 587*uint32(p.G) + 114*uint32(p.B)) / 1000
				a = uint8(255 - lum)
			}
			out.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return src
	}
	return buf.Bytes()
}

var (
	black = color.NRGBA{A: 255}
	white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
)

// withBadge puts a round badge in c at the bottom right of a PNG icon, with a
// transparent gap around it so it stands out on light and dark taskbars alike.
// The badge is large (44% of the icon): tray icons are shown at about 16px.
func withBadge(src []byte, c color.NRGBA) []byte {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)

	size := float64(min(b.Dx(), b.Dy()))
	r := size * 0.22   // badge radius
	gap := size * 0.07 // the cut-out ring around it
	cx := float64(b.Max.X) - r - 0.5
	cy := float64(b.Max.Y) - r - 0.5
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Sqrt(dx*dx + dy*dy)
			p := out.NRGBAAt(x, y)
			// Cut the glyph away around the badge (anti-aliased edge).
			keep := clamp01(d - (r + gap) + 0.5)
			p.A = uint8(float64(p.A) * keep)
			// Then the badge itself, over what is left.
			if a := clamp01(r + 0.5 - d); a > 0 {
				p = over(color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(float64(c.A) * a)}, p)
			}
			out.SetNRGBA(x, y, p)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return src
	}
	return buf.Bytes()
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// over composites src over dst (non-premultiplied colors).
func over(src, dst color.NRGBA) color.NRGBA {
	sa, da := float64(src.A)/255, float64(dst.A)/255
	oa := sa + da*(1-sa)
	if oa == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8((float64(s)*sa + float64(d)*da*(1-sa)) / oa)
	}
	return color.NRGBA{R: mix(src.R, dst.R), G: mix(src.G, dst.G), B: mix(src.B, dst.B), A: uint8(oa*255 + 0.5)}
}
