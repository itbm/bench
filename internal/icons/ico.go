package icons

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// This file reads ICO files, which most sites' /favicon.ico are: a list of
// images of a few sizes, each a PNG or a Windows bitmap (a DIB) without its
// file header, whose rows run from the bottom up, followed by a 1-bit mask
// of the pixels to leave transparent.

// pngMagic starts every PNG.
var pngMagic = []byte("\x89PNG\r\n\x1a\n")

// isICO reports whether data starts as an ICO file does.
func isICO(data []byte) bool {
	return len(data) >= 6 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0
}

// icoEntry is one image in an ICO file's directory.
type icoEntry struct {
	size   int // Its width, which 0 in the file means is 256.
	bits   int
	offset int
	length int
}

// decodeICO decodes the image in an ICO file that fits pixels best: the
// smallest at least pixels wide, or else the biggest, the one with the
// most colours of a size.
func decodeICO(data []byte, pixels int) (image.Image, error) {
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 || len(data) < 6+16*count {
		return nil, ErrNotImage
	}
	var best *icoEntry
	better := func(e, than icoEntry) bool {
		switch {
		case e.size >= pixels && than.size >= pixels && e.size != than.size:
			return e.size < than.size
		case (e.size >= pixels) != (than.size >= pixels):
			return e.size >= pixels
		case e.size != than.size:
			return e.size > than.size
		}
		return e.bits > than.bits
	}
	for i := range count {
		d := data[6+16*i : 6+16*i+16]
		e := icoEntry{
			size:   int(d[0]),
			bits:   int(binary.LittleEndian.Uint16(d[6:8])),
			length: int(binary.LittleEndian.Uint32(d[8:12])),
			offset: int(binary.LittleEndian.Uint32(d[12:16])),
		}
		if e.size == 0 {
			e.size = 256
		}
		if e.offset < 0 || e.length <= 0 || e.offset > len(data) || e.length > len(data)-e.offset {
			continue
		}
		if best == nil || better(e, *best) {
			best = &e
		}
	}
	if best == nil {
		return nil, ErrNotImage
	}
	part := data[best.offset : best.offset+best.length]
	if bytes.HasPrefix(part, pngMagic) {
		cfg, err := png.DecodeConfig(bytes.NewReader(part))
		if err != nil || !sideOK(cfg.Width) || !sideOK(cfg.Height) {
			return nil, ErrNotImage
		}
		img, err := png.Decode(bytes.NewReader(part))
		if err != nil {
			return nil, ErrNotImage
		}
		return img, nil
	}
	return decodeDIB(part)
}

// decodeDIB decodes an ICO's bitmap: 32-bit colour with alpha, or 24-bit,
// 8-bit, 4-bit, or 1-bit colour with the mask for transparency. A 32-bit
// bitmap whose alpha is all 0, as old ones' is, uses the mask too.
func decodeDIB(d []byte) (image.Image, error) {
	if len(d) < 40 {
		return nil, ErrNotImage
	}
	header := int(binary.LittleEndian.Uint32(d[0:4]))
	w := int(int32(binary.LittleEndian.Uint32(d[4:8])))
	// The height counts the mask's rows too.
	h := int(int32(binary.LittleEndian.Uint32(d[8:12]))) / 2
	bits := int(binary.LittleEndian.Uint16(d[14:16]))
	compression := binary.LittleEndian.Uint32(d[16:20])
	colours := int(binary.LittleEndian.Uint32(d[32:36]))
	if header < 40 || header > len(d) || w <= 0 || h <= 0 || w > 256 || h > 256 {
		return nil, ErrNotImage
	}
	// Uncompressed, or 32-bit with the usual masks.
	if compression != 0 && !(compression == 3 && bits == 32) {
		return nil, ErrNotImage
	}
	var palette []color.NRGBA
	switch bits {
	case 1, 4, 8:
		if colours <= 0 || colours > 1<<bits {
			colours = 1 << bits
		}
		if header+4*colours > len(d) {
			return nil, ErrNotImage
		}
		for i := range colours {
			p := d[header+4*i:]
			palette = append(palette, color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255})
		}
	case 24, 32:
	default:
		return nil, ErrNotImage
	}
	if compression == 3 {
		// The three masks follow a 40-byte header.
		header = max(header, 52)
	}
	start := header + 4*len(palette)
	stride := (w*bits + 31) / 32 * 4
	maskStride := (w + 31) / 32 * 4
	if start > len(d) || stride*h > len(d)-start {
		return nil, ErrNotImage
	}
	maskStart := start + stride*h
	hasMask := maskStride*h <= len(d)-maskStart
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := range h {
		// Rows run from the bottom up.
		row := d[start+(h-1-y)*stride:]
		for x := range w {
			var c color.NRGBA
			switch bits {
			case 32:
				c = color.NRGBA{R: row[4*x+2], G: row[4*x+1], B: row[4*x], A: row[4*x+3]}
				anyAlpha = anyAlpha || c.A != 0
			case 24:
				c = color.NRGBA{R: row[3*x+2], G: row[3*x+1], B: row[3*x], A: 255}
			default:
				per := 8 / bits
				shift := uint(8 - bits*(x%per+1))
				i := int(row[x/per]>>shift) & (1<<bits - 1)
				if i < len(palette) {
					c = palette[i]
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	if bits == 32 && anyAlpha {
		return img, nil
	}
	if !hasMask {
		if bits == 32 {
			// No alpha and no mask: every pixel shows.
			for i := 3; i < len(img.Pix); i += 4 {
				img.Pix[i] = 255
			}
		}
		return img, nil
	}
	for y := range h {
		row := d[maskStart+(h-1-y)*maskStride:]
		for x := range w {
			transparent := row[x/8]&(0x80>>uint(x%8)) != 0
			c := img.NRGBAAt(x, y)
			if transparent {
				c = color.NRGBA{}
			} else {
				c.A = 255
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img, nil
}
