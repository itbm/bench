// Package darwin holds the files that `make app` packages into Bench.app.
// It has no code: its tests check the files against the build brief, and
// against the Makefile and the macOS bridge that build Bench.app with them.
package darwin

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestInfoPlist(t *testing.T) {
	info := readPlist(t, "Info.plist")
	tests := []struct {
		key  string
		want any
	}{
		{"CFBundleIdentifier", "io.github.itbm.bench"},
		{"CFBundleName", "Bench"},
		{"CFBundleDisplayName", "Bench"},
		{"CFBundleExecutable", "bench"},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleIconFile", "Bench.icns"},
		{"LSMinimumSystemVersion", "14.0"},
		{"LSApplicationCategoryType", "public.app-category.productivity"},
		{"NSHighResolutionCapable", true},
	}
	for _, tt := range tests {
		if got, ok := info[tt.key]; !ok || got != tt.want {
			t.Errorf("%s = %#v, want %#v", tt.key, got, tt.want)
		}
	}
	// The Makefile reads the version from CFBundleShortVersionString for
	// `bench version`.
	version := str(info, "CFBundleShortVersionString")
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) || str(info, "CFBundleVersion") != version {
		t.Errorf("CFBundleShortVersionString = %q and CFBundleVersion = %q, want the same version, such as 0.1.0",
			version, str(info, "CFBundleVersion"))
	}
	// Bench is a regular app, with a Dock icon as well as its menu bar
	// item.
	for _, key := range []string{"LSUIElement", "LSBackgroundOnly"} {
		if v, ok := info[key]; ok {
			t.Errorf("%s = %#v; Bench is a regular app with a Dock icon", key, v)
		}
	}
}

func TestEntitlements(t *testing.T) {
	ent := readPlist(t, "entitlements.plist")
	// The Accessibility API doesn't work in the App Sandbox.
	if v, ok := ent["com.apple.security.app-sandbox"]; ok && v != false {
		t.Errorf("com.apple.security.app-sandbox = %#v, want false or absent", v)
	}
	// The hardened runtime's exceptions, which Bench doesn't need.
	for _, key := range []string{
		"com.apple.security.cs.allow-jit",
		"com.apple.security.cs.allow-unsigned-executable-memory",
		"com.apple.security.cs.disable-executable-page-protection",
		"com.apple.security.cs.disable-library-validation",
		"com.apple.security.cs.allow-dyld-environment-variables",
		"com.apple.security.get-task-allow",
	} {
		if v, ok := ent[key]; ok {
			t.Errorf("%s = %#v; Bench needs no hardened-runtime exceptions", key, v)
		}
	}
}

// TestBuildAgrees checks that the Makefile and the macOS bridge build
// Bench.app as Info.plist describes it.
func TestBuildAgrees(t *testing.T) {
	info := readPlist(t, "Info.plist")
	minimum := str(info, "LSMinimumSystemVersion")
	makefile := readText(t, filepath.Join("..", "..", "Makefile"))
	bridge := readText(t, filepath.Join("..", "..", "internal", "mac", "platform_darwin.go"))

	tests := []struct {
		name    string
		text    string
		pattern string
		want    string
	}{
		{"Makefile's deployment target", makefile, `(?m)^MACOS_MIN := (\S+)$`, minimum},
		{"Makefile's executable", makefile, `Contents/MacOS/(\w+)\n`, str(info, "CFBundleExecutable")},
		{"Makefile's icon", makefile, `Contents/Resources/(\w+\.icns)\n`, str(info, "CFBundleIconFile")},
		{"Makefile's entitlements", makefile, `--entitlements \$\(DARWIN\)/(\S+)`, "entitlements.plist"},
		{"bridge's CFLAGS", bridge, `#cgo CFLAGS:.*-mmacosx-version-min=(\S+)`, minimum},
		{"bridge's LDFLAGS", bridge, `#cgo LDFLAGS:.*-mmacosx-version-min=(\S+)`, minimum},
	}
	for _, tt := range tests {
		m := regexp.MustCompile(tt.pattern).FindStringSubmatch(tt.text)
		if m == nil {
			t.Errorf("%s: found no match for %q", tt.name, tt.pattern)
			continue
		}
		if m[1] != tt.want {
			t.Errorf("%s = %q, want %q to match Info.plist", tt.name, m[1], tt.want)
		}
	}
}

func TestIcon(t *testing.T) {
	f, err := os.Open("icon.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding icon.png: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("icon.png is %d by %d pixels, want 1024 by 1024", b.Dx(), b.Dy())
	}
	// Points well inside each shape, away from anti-aliased edges. See
	// icon/main.go for the geometry.
	tests := []struct {
		name string
		x, y int
		want color.NRGBA
	}{
		{"outside the rounded square", 4, 4, color.NRGBA{}},
		{"the rounded square", 150, 512, rgb(0x12, 0x14, 0x19)},
		{"the main pane's outline", 214, 512, rgb(0xF2, 0xA3, 0x3A)},
		{"the main pane", 380, 512, rgb(0x2A, 0x30, 0x3B)},
		{"the top side pane", 700, 350, rgb(0x2A, 0x30, 0x3B)},
		{"the bottom side pane", 700, 680, rgb(0x2A, 0x30, 0x3B)},
		{"between the side panes", 700, 512, rgb(0x12, 0x14, 0x19)},
	}
	for _, tt := range tests {
		got := color.NRGBAModel.Convert(img.At(tt.x, tt.y)).(color.NRGBA)
		if got != tt.want {
			t.Errorf("%s at (%d, %d) = %v, want %v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

// rgb returns an opaque colour.
func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 0xFF} }

// str returns a string value from a property list, or "" if it isn't one.
func str(dict map[string]any, key string) string {
	s, _ := dict[key].(string)
	return s
}

// readText returns a file's contents.
func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// readPlist reads an XML property list whose root is a dictionary. Values
// are strings (for strings, numbers, and dates), bools, nested
// dictionaries, or nil for arrays and data.
func readPlist(t *testing.T, path string) map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dict, err := parsePlist(xml.NewDecoder(f))
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return dict
}

// parsePlist parses a property list up to the end of its root dictionary.
func parsePlist(d *xml.Decoder) (map[string]any, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("finding the root dictionary: %w", err)
		}
		if el, ok := tok.(xml.StartElement); ok && el.Name.Local == "dict" {
			return parseDict(d)
		}
	}
}

// parseDict parses a dictionary's keys and values, after its <dict>, up to
// its </dict>.
func parseDict(d *xml.Decoder) (map[string]any, error) {
	dict := map[string]any{}
	key := ""
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, fmt.Errorf("reading a dictionary: %w", err)
		}
		switch el := tok.(type) {
		case xml.EndElement:
			return dict, nil
		case xml.StartElement:
			if el.Name.Local == "key" {
				if err := d.DecodeElement(&key, &el); err != nil {
					return nil, fmt.Errorf("reading a key: %w", err)
				}
				continue
			}
			if key == "" {
				return nil, errors.New("a value without a key")
			}
			v, err := parseValue(d, el)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", key, err)
			}
			dict[key] = v
			key = ""
		}
	}
}

// parseValue parses the value that starts with el.
func parseValue(d *xml.Decoder, el xml.StartElement) (any, error) {
	switch el.Name.Local {
	case "dict":
		return parseDict(d)
	case "true", "false":
		return el.Name.Local == "true", d.Skip()
	case "string", "integer", "real", "date":
		var s string
		err := d.DecodeElement(&s, &el)
		return s, err
	default:
		return nil, d.Skip()
	}
}
