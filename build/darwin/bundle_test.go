// Package darwin holds the files that `make app` packages into Bench.app.
// It has no code: its tests check the files against the build brief,
// against the Makefile and the macOS bridge that build Bench.app with them,
// and against the app menu that the credits name.
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
	"strings"
	"testing"
	"unicode"
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
	// Web items can be any web page, http:// ones on the local network and
	// on intranets among them, so App Transport Security mustn't block their
	// web views. The rest of the app needs only local networking, for
	// VS Code for the Web.
	ats, _ := info["NSAppTransportSecurity"].(map[string]any)
	for _, key := range []string{"NSAllowsArbitraryLoadsInWebContent", "NSAllowsLocalNetworking"} {
		if ats[key] != true {
			t.Errorf("NSAppTransportSecurity.%s = %#v, want true", key, ats[key])
		}
	}
	if v, ok := ats["NSAllowsArbitraryLoads"]; ok {
		t.Errorf("NSAppTransportSecurity.NSAllowsArbitraryLoads = %#v; only web content needs arbitrary loads", v)
	}
	// Web items' pages and the programs in Bench's terminals ask for the
	// camera, the microphone, and Apple Events as Bench, and macOS refuses
	// them without a prompt unless Info.plist says why Bench wants them.
	for _, key := range []string{"NSCameraUsageDescription", "NSMicrophoneUsageDescription", "NSAppleEventsUsageDescription"} {
		if str(info, key) == "" {
			t.Errorf("%s = %#v, want a reason that macOS can show", key, info[key])
		}
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
	// The hardened runtime denies the camera, the microphone, and Apple
	// Events unless the app opts in. Bench is the responsible process for
	// Web items' pages and for the programs in its terminals, so they need
	// these too, for a video call or an osascript command.
	for _, key := range []string{
		"com.apple.security.device.camera",
		"com.apple.security.device.audio-input",
		"com.apple.security.automation.apple-events",
	} {
		if v := ent[key]; v != true {
			t.Errorf("%s = %#v, want true", key, v)
		}
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
// Bench.app as Info.plist describes it, with the files here.
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
		// The standard About panel finds the credits by this name.
		{"Makefile's credits", makefile, `cp \$\(DARWIN\)/(\S+) \$\(APP\)/Contents/Resources/Credits\.rtf\n`, "Credits.rtf"},
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
			t.Errorf("%s = %q, want %q", tt.name, m[1], tt.want)
		}
	}
}

// TestCredits checks Credits.rtf, which the standard About panel shows
// under Bench's version.
func TestCredits(t *testing.T) {
	credits := readText(t, "Credits.rtf")
	if !strings.HasPrefix(credits, `{\rtf1`) {
		t.Error(`Credits.rtf doesn't start with {\rtf1`)
	}
	if !oneGroup(credits) {
		t.Error("Credits.rtf's braces don't pair up into one outer group")
	}
	// RTF is ASCII, with escapes for other characters. A character typed
	// as itself would show as something else.
	if i := strings.IndexFunc(credits, func(r rune) bool { return r > unicode.MaxASCII }); i >= 0 {
		t.Errorf("Credits.rtf has a character outside ASCII at byte %d; RTF needs an escape for it", i)
	}
	// The About panel draws text that has no colour of its own in the
	// panel's text colour, which follows Dark Mode, but keeps a colour that
	// the file sets, whatever the appearance.
	if c := regexp.MustCompile(`\\(colortbl|cf\d|cb\d|highlight\d|ulc\d)`).FindString(credits); c != "" {
		t.Errorf("Credits.rtf sets a colour with %s; it mustn't, so that the credits read in Dark Mode", c)
	}
	// The credits send people to the licences by the label of the
	// Acknowledgements item, which internal/ui gives the item and its
	// window.
	menu := readText(t, filepath.Join("..", "..", "internal", "ui", "menu.go"))
	m := regexp.MustCompile(`\backnowledgementsLabel\s*=\s*"([^"]+)"`).FindStringSubmatch(menu)
	if m == nil {
		t.Fatal("found no acknowledgementsLabel in internal/ui/menu.go")
	}
	// RTF writes ▸ as \u and its code point, then a ? for readers that
	// don't know Unicode. TextEdit writes \uc0 first, which says there's no
	// such character, then a space that ends the code point, and the real
	// space after it.
	want := regexp.MustCompile(fmt.Sprintf(`Bench (\\u%[1]d\? |\\uc0 ?\\u%[1]d  )%[2]s`, '▸', regexp.QuoteMeta(m[1])))
	if !want.MatchString(credits) {
		t.Errorf(`Credits.rtf doesn't name the menu item as Bench \u%d? %s`, '▸', m[1])
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

// oneGroup reports whether an RTF document's braces pair up, with the whole
// document, apart from white space around it, in one outer group. It skips
// the character after each backslash, so that escaped braces don't count.
func oneGroup(rtf string) bool {
	rtf = strings.TrimSpace(rtf)
	depth := 0
	for i := 0; i < len(rtf); i++ {
		// Only the outer group's opening brace is outside it.
		if depth == 0 && (i > 0 || rtf[i] != '{') {
			return false
		}
		switch rtf[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
		}
	}
	return rtf != "" && depth == 0
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
