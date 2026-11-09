//go:build darwin || server

package ui

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"

	"github.com/itbm/bench/internal/config"
)

// The dot before a coloured tab's name in the app menu's Tab menu and
// Pane ▸ Move to Tab, and in the Move to Tab and Also Show In submenus of
// Bench's native menus (contextmenu.go, decision 74): a PNG that Wails
// gives the menu item as its image. It's dotPixels square, with a pHYs
// chunk that says it has 144 pixels to the inch, so that macOS draws it at
// half that many points, sharp on a Retina screen, and the dot is
// dotRadius pixels round its middle.
const (
	dotPixels = 24
	dotRadius = 9.0
	// dotPerMetre is 144 pixels to the inch, in pixels to the metre, as
	// PNG keeps it.
	dotPerMetre = 5669
)

// menuDotColours are the dots' colours. A native menu follows macOS's
// appearance, not Bench's, and its image can't change with it, so each
// colour is the one between the light and dark sets that's at 3:1 at
// least on both a light menu and a dark one (TestMenuDotContrast).
var menuDotColours = map[string]color.NRGBA{
	config.ColourRed:    {0xE4, 0x4A, 0x42, 0xFF},
	config.ColourYellow: {0x8A, 0x83, 0x00, 0xFF},
	config.ColourGreen:  {0x25, 0x93, 0x4A, 0xFF},
	config.ColourCyan:   {0x12, 0x8C, 0xA5, 0xFF},
	config.ColourBlue:   {0x3F, 0x7D, 0xEA, 0xFF},
	config.ColourPurple: {0x9A, 0x65, 0xE4, 0xFF},
	config.ColourPink:   {0xDE, 0x43, 0x96, 0xFF},
	config.ColourGrey:   {0x7A, 0x81, 0x8E, 0xFF},
}

var (
	dotsOnce sync.Once
	// dots are the PNGs, by colour, and "" for the blank one.
	dots map[string][]byte
)

// menuDot returns the PNG of a tab colour's dot, or for "", "none", or a
// colour that isn't one, a blank image as large, which keeps the names of
// a tab with no colour in line with the others.
func menuDot(colour string) []byte {
	dotsOnce.Do(func() {
		dots = map[string][]byte{"": dotPNG(color.NRGBA{})}
		for name, c := range menuDotColours {
			dots[name] = dotPNG(c)
		}
	})
	if b, ok := dots[colour]; ok {
		return b
	}
	return dots[""]
}

// dotPNG draws a dot of colour c, its edge smoothed, as a PNG for a menu
// item's image. A transparent c gives a blank image.
func dotPNG(c color.NRGBA) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, dotPixels, dotPixels))
	mid := dotPixels / 2.0
	for y := range dotPixels {
		for x := range dotPixels {
			d := math.Hypot(float64(x)+0.5-mid, float64(y)+0.5-mid)
			cover := math.Max(0, math.Min(1, dotRadius+0.5-d))
			if cover > 0 && c.A > 0 {
				img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(math.Round(cover * float64(c.A)))})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return withResolution(buf.Bytes(), dotPerMetre)
}

// withResolution puts a pHYs chunk, of perMetre pixels to the metre each
// way, after a PNG's header chunk, which is always first. It returns the
// PNG as it is if it's too short to have one.
func withResolution(p []byte, perMetre uint32) []byte {
	// The signature, and the header chunk: its length, type, 13 bytes, and
	// CRC.
	const afterHeader = 8 + 4 + 4 + 13 + 4
	if len(p) < afterHeader {
		return p
	}
	data := make([]byte, 9)
	binary.BigEndian.PutUint32(data[0:], perMetre)
	binary.BigEndian.PutUint32(data[4:], perMetre)
	data[8] = 1 // The unit is the metre.
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	chunk = append(chunk, "pHYs"...)
	chunk = append(chunk, data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := make([]byte, 0, len(p)+len(chunk))
	out = append(out, p[:afterHeader]...)
	out = append(out, chunk...)
	return append(out, p[afterHeader:]...)
}
