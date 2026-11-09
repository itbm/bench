//go:build server

package ui

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"image/png"
	"math"
	"testing"

	"github.com/itbm/bench/internal/config"
)

// TestMenuDots checks the Tab menu's dots: one for each tab colour, a
// 24-pixel PNG at 144 pixels to the inch, so 12 points square, with the
// colour in the middle and nothing at the corners, and a blank one, as
// large, for no colour, "none", which a native menu's entry for a tab with
// no colour has, or one that isn't a colour.
func TestMenuDots(t *testing.T) {
	for _, colour := range append(config.TabColours(), "", "none", "teal") {
		b := menuDot(colour)
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%q: %v", colour, err)
		}
		if got := img.Bounds(); got.Dx() != dotPixels || got.Dy() != dotPixels {
			t.Errorf("%q: %v, want %d pixels square", colour, got, dotPixels)
		}
		if x, y, unit, ok := resolutionOf(b); !ok || x != dotPerMetre || y != dotPerMetre || unit != 1 {
			t.Errorf("%q: pHYs %d × %d, unit %d (%v); want %d pixels to the metre", colour, x, y, unit, ok, dotPerMetre)
		}
		corner := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
		middle := color.NRGBAModel.Convert(img.At(dotPixels/2, dotPixels/2)).(color.NRGBA)
		want, coloured := menuDotColours[colour]
		switch {
		case corner.A != 0:
			t.Errorf("%q: the corner is %v, want it clear", colour, corner)
		case coloured && middle != want:
			t.Errorf("%q: the middle is %v, want %v", colour, middle, want)
		case !coloured && middle.A != 0:
			t.Errorf("%q: the blank's middle is %v, want it clear", colour, middle)
		}
	}
	if len(menuDotColours) != len(config.TabColours()) {
		t.Errorf("%d dot colours for %d tab colours", len(menuDotColours), len(config.TabColours()))
	}
}

// resolutionOf returns a PNG's pHYs chunk: pixels to the unit, each way,
// and the unit.
func resolutionOf(p []byte) (x, y uint32, unit byte, ok bool) {
	for i := 8; i+12 <= len(p); {
		n := int(binary.BigEndian.Uint32(p[i:]))
		if string(p[i+4:i+8]) == "pHYs" && n == 9 && i+8+n <= len(p) {
			d := p[i+8:]
			return binary.BigEndian.Uint32(d), binary.BigEndian.Uint32(d[4:]), d[8], true
		}
		i += 12 + n
	}
	return 0, 0, 0, false
}

// TestMenuDotContrast checks that each dot is at 3:1 at least, as a mark,
// on a light menu and a dark one: a native menu follows macOS, not
// Bench's appearance setting. The menus' colours are the lightest and
// darkest that macOS's translucent menus show over most desktops.
func TestMenuDotContrast(t *testing.T) {
	menus := map[string]color.NRGBA{
		"light": {0xF6, 0xF6, 0xF6, 0xFF}, "light, over a darker desktop": {0xEB, 0xEB, 0xEB, 0xFF},
		"dark": {0x1E, 0x1E, 0x1E, 0xFF}, "dark, over a lighter desktop": {0x32, 0x32, 0x32, 0xFF},
	}
	for name, c := range menuDotColours {
		for menu, bg := range menus {
			if r := contrastRatio(c, bg); r < 3 {
				t.Errorf("%s's dot is at %.2f:1 on a %s menu, want 3:1", name, r, menu)
			}
		}
	}
}

// contrastRatio returns two colours' contrast ratio, as WCAG defines it.
func contrastRatio(a, b color.NRGBA) float64 {
	lum := func(c color.NRGBA) float64 {
		ch := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.03928 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	hi, lo := lum(a), lum(b)
	if lo > hi {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}
