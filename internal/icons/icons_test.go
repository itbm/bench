package icons

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// square returns a w by h image of one colour.
func square(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fitted runs Fit and decodes what it returns.
func fitted(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	out, err := Fit(data, Pixels)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("Fit's PNG doesn't decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != Pixels || b.Dy() != Pixels {
		t.Fatalf("Fit made %v, want %d square", b, Pixels)
	}
	n := image.NewNRGBA(img.Bounds())
	for y := range Pixels {
		for x := range Pixels {
			n.Set(x, y, img.At(x, y))
		}
	}
	return n
}

var (
	red   = color.NRGBA{R: 255, A: 255}
	blue  = color.NRGBA{B: 255, A: 255}
	green = color.NRGBA{G: 255, A: 255}
	white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	none  = color.NRGBA{}
)

func TestFitScalesABigImageDownKeepingItsShape(t *testing.T) {
	img := fitted(t, encodePNG(t, square(160, 80, red)))
	// 160 by 80 becomes 40 by 20, between transparent bands 10 tall.
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{{20, 5, none}, {20, 10, red}, {0, 20, red}, {39, 29, red}, {20, 30, none}, {20, 39, none}} {
		if got := img.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("pixel (%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestFitScalesASmallImageUpByAWholeNumber(t *testing.T) {
	img := fitted(t, encodePNG(t, square(16, 16, blue)))
	// Doubled to 32, centred: a 4-pixel transparent border.
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{{3, 3, none}, {4, 4, blue}, {35, 35, blue}, {36, 36, none}} {
		if got := img.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("pixel (%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestFitKeepsEdgesFromDarkening(t *testing.T) {
	// Transparent black on the left, white on the right: a pixel that
	// mixes them is white, partly transparent, not grey.
	src := square(60, 60, white)
	for y := range 60 {
		for x := range 31 {
			src.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	img := fitted(t, encodePNG(t, src))
	got := img.NRGBAAt(20, 20)
	if got.R != 255 || got.G != 255 || got.B != 255 || got.A == 0 || got.A == 255 {
		t.Errorf("the edge pixel is %v, want white and partly transparent", got)
	}
}

func TestFitRefusesWhatIsntAnIcon(t *testing.T) {
	for name, data := range map[string][]byte{
		"nothing":     nil,
		"text":        []byte("<html>not found</html>"),
		"a huge PNG":  encodePNG(t, square(maxSide+1, 1, red)),
		"an SVG":      []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"a short ICO": {0, 0, 1, 0, 1, 0},
	} {
		if _, err := Fit(data, Pixels); !errors.Is(err, ErrNotImage) {
			t.Errorf("%s: Fit = %v, want ErrNotImage", name, err)
		}
	}
}

// ico builds an ICO file of the given images, each a PNG or a DIB, with
// the sizes its directory gives.
func ico(sizes []int, bits []int, parts [][]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0, 1, 0})
	_ = binary.Write(&b, binary.LittleEndian, uint16(len(parts)))
	offset := 6 + 16*len(parts)
	for i, p := range parts {
		size := byte(sizes[i])
		if sizes[i] >= 256 {
			size = 0
		}
		b.Write([]byte{size, size, 0, 0, 1, 0})
		_ = binary.Write(&b, binary.LittleEndian, uint16(bits[i]))
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(p)))
		_ = binary.Write(&b, binary.LittleEndian, uint32(offset))
		offset += len(p)
	}
	for _, p := range parts {
		b.Write(p)
	}
	return b.Bytes()
}

// dib builds an ICO's bitmap w pixels square: rows from the bottom up of
// the pixel data that row gives for each row from the top, padded, then
// the mask, where mask(x, y) says which pixels are transparent.
func dib(w, bits int, palette []color.NRGBA, row func(y int) []byte, mask func(x, y int) bool) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint32(40))
	_ = binary.Write(&b, binary.LittleEndian, int32(w))
	_ = binary.Write(&b, binary.LittleEndian, int32(2*w))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(bits))
	b.Write(make([]byte, 16))
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(palette)))
	b.Write(make([]byte, 4))
	for _, c := range palette {
		b.Write([]byte{c.B, c.G, c.R, 0})
	}
	stride := (w*bits + 31) / 32 * 4
	for y := w - 1; y >= 0; y-- {
		r := make([]byte, stride)
		copy(r, row(y))
		b.Write(r)
	}
	maskStride := (w + 31) / 32 * 4
	for y := w - 1; y >= 0; y-- {
		r := make([]byte, maskStride)
		for x := range w {
			if mask(x, y) {
				r[x/8] |= 0x80 >> uint(x%8)
			}
		}
		b.Write(r)
	}
	return b.Bytes()
}

func TestFitChoosesTheICOImageThatFitsBest(t *testing.T) {
	small, mid, big := encodePNG(t, square(16, 16, red)), encodePNG(t, square(48, 48, green)), encodePNG(t, square(256, 256, blue))
	// The smallest at least 40 wide.
	if got := fitted(t, ico([]int{16, 48, 256}, []int{32, 32, 32}, [][]byte{small, mid, big})).NRGBAAt(20, 20); got != green {
		t.Errorf("from 16, 48, and 256: %v, want the 48's green", got)
	}
	// Else the biggest.
	icon := encodePNG(t, square(32, 32, blue))
	if got := fitted(t, ico([]int{16, 32}, []int{32, 32}, [][]byte{small, icon})).NRGBAAt(20, 20); got != blue {
		t.Errorf("from 16 and 32: %v, want the 32's blue", got)
	}
}

func TestFitReadsICOBitmaps(t *testing.T) {
	cornerClear := func(x, y int) bool { return x < 2 && y < 2 }
	noMask := func(x, y int) bool { return false }
	solid := func(n int, b ...byte) func(int) []byte {
		return func(int) []byte { return bytes.Repeat(b, n) }
	}
	for _, c := range []struct {
		name string
		data []byte
		at   image.Point
		want color.NRGBA
	}{
		{"32-bit with alpha", dib(20, 32, nil, solid(20, 0, 0, 255, 255), noMask), image.Pt(20, 20), red},
		{"32-bit with alpha ignores the mask", dib(20, 32, nil, solid(20, 0, 0, 255, 255), cornerClear), image.Pt(0, 0), red},
		{"32-bit without alpha uses the mask", dib(20, 32, nil, solid(20, 0, 255, 0, 0), cornerClear), image.Pt(0, 0), none},
		{"32-bit without alpha, opaque", dib(20, 32, nil, solid(20, 0, 255, 0, 0), cornerClear), image.Pt(20, 20), green},
		{"24-bit", dib(20, 24, nil, solid(20, 255, 0, 0), cornerClear), image.Pt(20, 20), blue},
		{"24-bit's mask", dib(20, 24, nil, solid(20, 255, 0, 0), cornerClear), image.Pt(0, 0), none},
		{"8-bit", dib(20, 8, []color.NRGBA{red, green}, solid(20, 1), noMask), image.Pt(20, 20), green},
		{"4-bit", dib(20, 4, []color.NRGBA{red, blue}, solid(10, 0x11), noMask), image.Pt(20, 20), blue},
		{"1-bit", dib(20, 1, []color.NRGBA{white, red}, solid(3, 0xff), noMask), image.Pt(20, 20), red},
	} {
		img := fitted(t, ico([]int{20}, []int{0}, [][]byte{c.data}))
		if got := img.NRGBAAt(c.at.X, c.at.Y); got != c.want {
			t.Errorf("%s: pixel %v = %v, want %v", c.name, c.at, got, c.want)
		}
	}
}

func TestFitRefusesBrokenICOs(t *testing.T) {
	good := dib(16, 32, nil, func(int) []byte { return bytes.Repeat([]byte{0, 0, 255, 255}, 16) }, func(int, int) bool { return false })
	for name, data := range map[string][]byte{
		"an entry past the end": func() []byte { d := ico([]int{16}, []int{32}, [][]byte{good}); return d[:len(d)-200] }(),
		"a truncated bitmap":    ico([]int{16}, []int{32}, [][]byte{good[:60]}),
		"16-bit colour":         ico([]int{16}, []int{16}, [][]byte{dib(16, 16, nil, func(int) []byte { return nil }, func(int, int) bool { return false })}),
		"a bitmap too big": func() []byte {
			d := append([]byte{}, good...)
			binary.LittleEndian.PutUint32(d[4:8], 5000)
			return ico([]int{16}, []int{32}, [][]byte{d})
		}(),
	} {
		if _, err := Fit(data, Pixels); !errors.Is(err, ErrNotImage) {
			t.Errorf("%s: Fit = %v, want ErrNotImage", name, err)
		}
	}
}
