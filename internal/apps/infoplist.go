package apps

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"syscall"

	"howett.net/plist"
)

// Limits on the Info.plist files Bench reads. Scan reads every bundle in
// the app folders, whether or not the user has ever opened it, and it runs
// while the engine is locked, so a crafted file mustn't be able to hang or
// crash Bench.
const (
	// maxPlistSize is far more than any real Info.plist needs: they're
	// rarely more than a few tens of kilobytes.
	maxPlistSize = 1 << 20
	// maxPlistDepth limits how deeply arrays and dictionaries nest (in an
	// XML file, elements of any kind, counting <plist>). Real files nest
	// them a few levels deep. The decoder recurses at each level, and for
	// a binary file it also does work in proportion to the depth at every
	// level, so a deep file could otherwise take minutes or overflow the
	// stack, which Go can't recover from.
	maxPlistDepth = 32
	// maxTextContainers limits the arrays and dictionaries in an old-style
	// text property list. Counting them is simpler and safer than
	// following the text format's quoting rules, and it limits the depth
	// too, as each level needs a bracket of its own.
	maxTextContainers = 4096
)

var (
	errPlistNotFile   = errors.New("it isn't a regular file")
	errPlistTooLarge  = fmt.Errorf("it's larger than %d bytes", maxPlistSize)
	errPlistTooDeep   = fmt.Errorf("its arrays and dictionaries nest more than %d deep", maxPlistDepth)
	errPlistMalformed = errors.New("it isn't a well-formed binary property list")
)

// infoPlist holds the keys Bench reads from an Info.plist. Decoding only
// these, rather than the whole file, means that other values are never
// built: a small binary file whose arrays share their contents would
// otherwise expand into billions of values.
type infoPlist struct {
	BundleID    plistString `plist:"CFBundleIdentifier"`
	DisplayName plistString `plist:"CFBundleDisplayName"`
	Name        plistString `plist:"CFBundleName"`
}

// plistString is a string in a property list. A value of another type,
// which some bundles have, reads as "", rather than spoiling the whole file.
type plistString string

// UnmarshalPlist implements plist.Unmarshaler.
func (s *plistString) UnmarshalPlist(unmarshal func(any) error) error {
	var v string
	if unmarshal(&v) == nil {
		*s = plistString(v)
	}
	return nil
}

// readPlistFile reads a property list file of at most maxPlistSize bytes.
// It reads only regular files, so that a link to a device such as
// /dev/zero can't fill memory, and it checks before opening the file, as
// opening some devices, such as a serial port, does something. It opens the
// file without blocking, so that a named pipe put in its place since the
// check can't make it wait for a writer, and checks again once it's open.
func readPlistFile(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errPlistNotFile
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errPlistNotFile
	}
	if fi.Size() > maxPlistSize {
		return nil, errPlistTooLarge
	}
	// The file may have grown since Stat, so the limit applies to the
	// read too.
	data, err := io.ReadAll(io.LimitReader(f, maxPlistSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPlistSize {
		return nil, errPlistTooLarge
	}
	return data, nil
}

// decodeInfoPlist decodes an Info.plist (binary, XML or old-style text),
// first checking that it doesn't nest too deeply.
func decodeInfoPlist(data []byte) (info infoPlist, err error) {
	// The decoder recovers from most problems with a file, but not from
	// runtime errors, such as an index out of range in a damaged binary
	// file, which would otherwise end Bench.
	defer func() {
		if r := recover(); r != nil {
			info, err = infoPlist{}, fmt.Errorf("the property list decoder failed: %v", r)
		}
	}()
	if err := checkNesting(data); err != nil {
		return infoPlist{}, err
	}
	_, err = plist.Unmarshal(data, &info)
	return info, err
}

// checkNesting returns errPlistTooDeep if a property list nests more deeply
// than the decoder can safely follow, or errPlistMalformed for a binary one
// whose references can't be followed. It chooses the format as the decoder
// does: binary if the file starts with "bplist", XML if its first XML
// element is a property list element, and text otherwise.
func checkNesting(data []byte) error {
	if bytes.HasPrefix(data, []byte("bplist")) {
		return checkBinaryNesting(data)
	}
	if isXML, err := checkXMLNesting(data); isXML {
		return err
	}
	// Each ( or { in the text is, or includes, a byte of that value,
	// whether the file is UTF-8 or UTF-16, so counting bytes counts at
	// least every bracket.
	if bytes.Count(data, []byte("("))+bytes.Count(data, []byte("{")) > maxTextContainers {
		return errPlistTooDeep
	}
	return nil
}

// xmlPlistElements are the elements that the decoder reads in an XML
// property list, apart from key, which can't be the first.
var xmlPlistElements = []string{"plist", "dict", "array", "string", "integer", "real", "true", "false", "date", "data"}

// checkXMLNesting checks an XML property list's elements, reading them as
// the decoder does. isXML is false if the file has no element before its
// first XML error, or if its first element isn't one of xmlPlistElements,
// such as <abab>, which in text is data: the decoder then reads the whole
// file as text. An XML error after that isn't returned, as the decoder
// reports it.
func checkXMLNesting(data []byte) (isXML bool, err error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			return isXML, nil
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if !isXML && !slices.Contains(xmlPlistElements, tok.Name.Local) {
				return false, nil
			}
			isXML = true
			depth++
			if depth > maxPlistDepth {
				return true, errPlistTooDeep
			}
		case xml.EndElement:
			// The decoder stops at the end of the first element.
			depth--
			if depth == 0 {
				return true, nil
			}
		}
	}
}

// Parts of a binary property list.
const (
	bplistHeaderSize  = 8  // "bplist00"
	bplistTrailerSize = 32 // At the end of the file.
	bplistTagArray    = 0xA0
	bplistTagDict     = 0xD0
	bplistTagInt      = 0x10
)

// checkBinaryNesting checks how deeply a binary property list's arrays and
// dictionaries nest, following the object references from the top object.
// Each object is examined once, however many containers share it, so this
// takes time in proportion to the file's size. A file that can't be
// followed is malformed, as the decoder would find too.
func checkBinaryNesting(data []byte) error {
	if len(data) < bplistHeaderSize+bplistTrailerSize {
		return errPlistMalformed
	}
	t := data[len(data)-bplistTrailerSize:]
	w := bplistWalker{
		data:    data,
		offSize: uint64(t[6]),
		refSize: uint64(t[7]),
		table:   binary.BigEndian.Uint64(t[24:]),
	}
	count := binary.BigEndian.Uint64(t[8:])
	top := binary.BigEndian.Uint64(t[16:])
	end := uint64(len(data) - bplistTrailerSize)
	// count is checked against end first, so that the products can't
	// overflow.
	if w.offSize < 1 || w.offSize > 8 || w.refSize < 1 || w.refSize > 8 ||
		count > end || top >= count || w.table < bplistHeaderSize || w.table > end ||
		count*w.offSize > end-w.table {
		return errPlistMalformed
	}
	w.heights = make([]int8, count)
	_, err := w.height(top, 0)
	return err
}

// bplistWalker follows the objects of a binary property list.
type bplistWalker struct {
	data             []byte
	offSize, refSize uint64 // The sizes of offsets and object references, in bytes.
	table            uint64 // Where the offset table starts, and so where objects end.
	// heights records each object's height (see height) plus 1, with 0
	// for an object not yet seen and -1 for one being examined.
	heights []int8
}

// height returns how many levels of arrays and dictionaries object i holds,
// counting itself: 0 for any other kind of object. depth is how many
// containers hold it, on the way from the top object. It fails if depth
// plus the height is more than maxPlistDepth, or if a reference can't be
// followed or leads back to a container that holds it.
func (w *bplistWalker) height(i uint64, depth int) (int, error) {
	if i >= uint64(len(w.heights)) {
		return 0, errPlistMalformed
	}
	switch h := w.heights[i]; {
	case h < 0:
		return 0, errPlistMalformed
	case h > 0:
		if depth+int(h)-1 > maxPlistDepth {
			return 0, errPlistTooDeep
		}
		return int(h) - 1, nil
	}
	off := w.uint(w.table+i*w.offSize, w.offSize)
	if off >= w.table {
		return 0, errPlistMalformed
	}
	var refsPer uint64
	switch w.data[off] & 0xF0 {
	case bplistTagArray:
		refsPer = 1
	case bplistTagDict:
		refsPer = 2 // Keys, then values.
	default:
		w.heights[i] = 1
		return 0, nil
	}
	if depth+1 > maxPlistDepth {
		return 0, errPlistTooDeep
	}
	n, start, ok := w.count(off)
	if !ok || n*refsPer*w.refSize > w.table-start {
		return 0, errPlistMalformed
	}
	w.heights[i] = -1
	height := 0
	for k := range n * refsPer {
		h, err := w.height(w.uint(start+k*w.refSize, w.refSize), depth+1)
		if err != nil {
			return 0, err
		}
		height = max(height, h)
	}
	height++
	w.heights[i] = int8(height + 1)
	return height, nil
}

// count returns how many entries the container at off has, and where its
// references start. A count of 15 or more follows the tag as an integer
// object.
func (w *bplistWalker) count(off uint64) (n, next uint64, ok bool) {
	n, next = uint64(w.data[off]&0x0F), off+1
	if n == 0x0F {
		if next >= w.table {
			return 0, 0, false
		}
		tag := w.data[next]
		if tag&0xF0 != bplistTagInt || tag&0x0F > 3 {
			return 0, 0, false
		}
		size := uint64(1) << (tag & 0x0F)
		if size > w.table-next-1 {
			return 0, 0, false
		}
		n, next = w.uint(next+1, size), next+1+size
	}
	// No container can have more entries than the file has bytes; this
	// also keeps the caller's products from overflowing.
	if n > w.table {
		return 0, 0, false
	}
	return n, next, true
}

// uint reads a big-endian unsigned integer of size bytes at off, which the
// caller has checked is in the file.
func (w *bplistWalker) uint(off, size uint64) uint64 {
	var v uint64
	for _, b := range w.data[off : off+size] {
		v = v<<8 | uint64(b)
	}
	return v
}
