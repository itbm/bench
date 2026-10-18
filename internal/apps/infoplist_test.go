package apps

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"howett.net/plist"
)

// writeInfoPlist creates a bundle in a new folder whose Contents/Info.plist
// holds data, and returns the bundle's path.
func writeInfoPlist(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Crafted.app")
	plistPath := filepath.Join(path, "Contents", "Info.plist")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plistPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// bplistObject is an object in a binary property list built by
// buildBplist: a string, or an array or dictionary of references to other
// objects by index.
type bplistObject struct {
	str  string
	refs []int // For an array, or a dictionary's keys then its values.
	dict bool
}

// buildBplist writes a binary property list by hand, with one-byte
// references, so that tests can share objects between containers and nest
// them, which plist.Marshal doesn't do.
func buildBplist(objects []bplistObject, top int) []byte {
	var buf bytes.Buffer
	buf.WriteString("bplist00")
	refSize := 1
	if len(objects) > 0xFF {
		refSize = 2
	}
	offsets := make([]int, len(objects))
	writeCount := func(tag byte, n int) {
		if n < 15 {
			buf.WriteByte(tag | byte(n))
			return
		}
		buf.WriteByte(tag | 0x0F)
		buf.WriteByte(0x11) // A two-byte integer.
		_ = binary.Write(&buf, binary.BigEndian, uint16(n))
	}
	for i, o := range objects {
		offsets[i] = buf.Len()
		switch {
		case o.refs == nil:
			writeCount(0x50, len(o.str))
			buf.WriteString(o.str)
		default:
			n := len(o.refs)
			tag := byte(0xA0)
			if o.dict {
				tag, n = 0xD0, n/2
			}
			writeCount(tag, n)
			for _, r := range o.refs {
				if refSize == 1 {
					buf.WriteByte(byte(r))
				} else {
					_ = binary.Write(&buf, binary.BigEndian, uint16(r))
				}
			}
		}
	}
	table := buf.Len()
	for _, off := range offsets {
		_ = binary.Write(&buf, binary.BigEndian, uint32(off))
	}
	trailer := make([]byte, 32)
	trailer[6], trailer[7] = 4, byte(refSize)
	binary.BigEndian.PutUint64(trailer[8:], uint64(len(objects)))
	binary.BigEndian.PutUint64(trailer[16:], uint64(top))
	binary.BigEndian.PutUint64(trailer[24:], uint64(table))
	buf.Write(trailer)
	return buf.Bytes()
}

// bplistWith returns a binary Info.plist whose top dictionary has a bundle
// ID and, under the key "Extra", the last of objects, which may refer to
// the others by index.
func bplistWith(objects []bplistObject) []byte {
	n := len(objects)
	objects = append(objects,
		bplistObject{str: "CFBundleIdentifier"},
		bplistObject{str: "com.example.crafted"},
		bplistObject{str: "Extra"},
	)
	objects = append(objects, bplistObject{dict: true, refs: []int{n, n + 2, n + 1, n - 1}})
	return buildBplist(objects, len(objects)-1)
}

// readBundleWithin runs ReadBundle, failing the test if it takes more than
// a few seconds.
func readBundleWithin(t *testing.T, path string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := ReadBundle(path)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("ReadBundle is still reading after 5s")
		return nil
	}
}

func TestReadBundleSharedArrays(t *testing.T) {
	// Each array holds the one before it twice, so decoding the whole file
	// would build 2^30 values. Bench needs only the bundle ID.
	objects := []bplistObject{{str: "leaf"}}
	for i := 1; i <= 30; i++ {
		objects = append(objects, bplistObject{refs: []int{i - 1, i - 1}})
	}
	if err := readBundleWithin(t, writeInfoPlist(t, bplistWith(objects))); err != nil {
		t.Errorf("ReadBundle error: %v", err)
	}
}

func TestReadBundleRefusesDeepNesting(t *testing.T) {
	const depth = 20000
	var objects []bplistObject
	objects = append(objects, bplistObject{str: "leaf"})
	for i := 1; i <= depth; i++ {
		objects = append(objects, bplistObject{refs: []int{i - 1}})
	}
	var xmlDeep, textDeep, stringsDeep strings.Builder
	xmlDeep.WriteString(`<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` +
		`<key>CFBundleIdentifier</key><string>com.example.crafted</string><key>Extra</key>`)
	textDeep.WriteString(`{ CFBundleIdentifier = com.example.crafted; Extra = `)
	// A text file in the format of a .strings file, with no braces around
	// it, whose first XML element is data in text, not an XML element of
	// a property list.
	stringsDeep.WriteString(`CFBundleIdentifier = com.example.crafted; Extra = `)
	for range depth {
		xmlDeep.WriteString("<array>")
		textDeep.WriteString("(")
		stringsDeep.WriteString("(")
	}
	for range depth {
		xmlDeep.WriteString("</array>")
		textDeep.WriteString(")")
		stringsDeep.WriteString(")")
	}
	xmlDeep.WriteString("</dict></plist>")
	textDeep.WriteString("; }")
	stringsDeep.WriteString("; Data = <abab>;")

	tests := []struct {
		name string
		data []byte
	}{
		{"binary", bplistWith(objects)},
		{"XML", []byte(xmlDeep.String())},
		{"text", []byte(textDeep.String())},
		{"text with data that looks like XML", []byte(stringsDeep.String())},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := readBundleWithin(t, writeInfoPlist(t, tt.data)); err == nil {
				t.Error("ReadBundle read a plist nested 20,000 deep, want an error")
			}
		})
	}
}

func TestReadBundleRefusesLargeFiles(t *testing.T) {
	info, err := plist.Marshal(map[string]any{
		"CFBundleIdentifier": "com.example.large",
		"Padding":            strings.Repeat("x", 1<<20),
	}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBundle(writeInfoPlist(t, info)); err == nil {
		t.Error("ReadBundle read a plist larger than 1 MiB, want an error")
	}
}

func TestReadBundleRealisticNesting(t *testing.T) {
	// Document and exported types nest dictionaries in arrays in
	// dictionaries, which must still read, in every format.
	info := map[string]any{
		"CFBundleIdentifier": "com.example.editor",
		"CFBundleName":       "Editor",
		"CFBundleDocumentTypes": []any{map[string]any{
			"CFBundleTypeName":       "Text",
			"LSItemContentTypes":     []any{"public.plain-text", "public.source-code"},
			"CFBundleTypeExtensions": []any{"txt", "md"},
		}},
		"UTExportedTypeDeclarations": []any{map[string]any{
			"UTTypeIdentifier":       "com.example.note",
			"UTTypeTagSpecification": map[string]any{"public.filename-extension": []any{"note"}},
			"UTTypeConformsTo":       []any{"public.data"},
			"NSAppTransportSecurity": map[string]any{"NSExceptionDomains": map[string]any{"example.com": map[string]any{"NSIncludesSubdomains": "YES"}}},
		}},
	}
	for _, format := range []int{plist.XMLFormat, plist.BinaryFormat, plist.OpenStepFormat} {
		data, err := plist.Marshal(info, format)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReadBundle(writeInfoPlist(t, data))
		if err != nil {
			t.Errorf("format %d: ReadBundle error: %v", format, err)
			continue
		}
		if got.BundleID != "com.example.editor" || got.Name != "Editor" {
			t.Errorf("format %d: ReadBundle = %+v, want com.example.editor called Editor", format, got)
		}
	}
}

func TestReadBundleSurvivesDamagedBinaryPlists(t *testing.T) {
	// Damaged references and counts, and truncated files, must be errors,
	// not panics.
	good := bplistWith([]bplistObject{{str: "leaf"}, {refs: []int{0, 0}}})
	for cut := 0; cut < len(good); cut++ {
		damaged := bytes.Clone(good)
		damaged[cut] ^= 0xFF
		for _, data := range [][]byte{damaged, good[:cut], append([]byte("bplist00"), good[cut:]...)} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("ReadBundle panicked at byte %d: %v", cut, r)
					}
				}()
				_, _ = ReadBundle(writeInfoPlist(t, data))
			}()
		}
	}
}
