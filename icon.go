package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
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
