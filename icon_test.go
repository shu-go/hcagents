package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayIcon(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    [3]uint8
	}{{"black", [3]uint8{0, 0, 0}}, {"white", [3]uint8{255, 255, 255}}} {
		col := black
		if tc.name == "white" {
			col = white
		}
		img, err := png.Decode(bytes.NewReader(trayIcon(appIcon, col)))
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		// The background (the corner) is transparent.
		if _, _, _, a := img.At(b.Min.X, b.Min.Y).RGBA(); a != 0 {
			t.Errorf("%s: corner alpha = %d, want 0", tc.name, a>>8)
		}
		// Some pixel is the glyph: opaque, in the wanted color.
		found := false
		for y := b.Min.Y; y < b.Max.Y && !found; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				if a>>8 == 255 && uint8(r>>8) == tc.c[0] && uint8(g>>8) == tc.c[1] && uint8(bl>>8) == tc.c[2] {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("%s: no opaque glyph pixel in the wanted color", tc.name)
		}
	}
}
