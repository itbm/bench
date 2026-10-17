package shortcuts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Bench registers global shortcuts with Wails v3's GlobalShortcutManager,
// which parses accelerators and, on macOS, looks keys up in a table of
// hardware key codes. These tables mirror what Bench relies on from Wails
// v3.0.0-beta.27 (github.com/wailsapp/wails/v3, in pkg/application):
//
//   - keys.go: parseAccelerator splits on "+", lower-cases each part, looks
//     every part but the last up in modifierMap, and accepts the last part
//     (parseKey) if it's in namedKeys or a single printable character.
//   - global_shortcut_darwin.go: macosGlobalShortcuts.register looks the
//     lower-cased key up in macKeyCodes, and rejects keys that aren't there.
//
// TestWailsTablesMatchSource checks the mirror against the Wails source.

// wailsModifiers are the modifierMap entries Bench uses. "cmd" is
// CmdOrCtrlKey, which is Command on macOS.
var wailsModifiers = map[string]bool{"ctrl": true, "option": true, "shift": true, "cmd": true}

// wailsNamedKeys are the namedKeys entries Bench uses.
var wailsNamedKeys = map[string]bool{
	"space": true, "tab": true, "return": true, "escape": true,
	"backspace": true, "delete": true, "home": true, "end": true,
	"page up": true, "page down": true,
	"left": true, "right": true, "up": true, "down": true,
	"f1": true, "f2": true, "f3": true, "f4": true, "f5": true,
	"f6": true, "f7": true, "f8": true, "f9": true, "f10": true,
	"f11": true, "f12": true, "f13": true, "f14": true, "f15": true,
	"f16": true, "f17": true, "f18": true, "f19": true, "f20": true,
}

// wailsMacKeyCodes are the macKeyCodes entries Bench uses (kVK_* codes).
var wailsMacKeyCodes = map[string]int{
	// Letters.
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7,
	"c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15, "y": 16,
	"t": 17, "o": 31, "u": 32, "i": 34, "p": 35, "l": 37, "j": 38, "k": 40,
	"n": 45, "m": 46,
	// Digits.
	"1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "9": 25, "7": 26,
	"8": 28, "0": 29,
	// Punctuation.
	"=": 24, "-": 27, "]": 30, "[": 33, "'": 39, ";": 41, "\\": 42,
	",": 43, "/": 44, ".": 47, "`": 50,
	// Named keys. Wails' "backspace" is the Mac's Delete key and its
	// "delete" is Forward Delete.
	"return": 36, "tab": 48, "space": 49, "backspace": 51, "delete": 117,
	"escape": 53, "home": 115, "page up": 116, "end": 119, "page down": 121,
	"left": 123, "right": 124, "down": 125, "up": 126,
	// Function keys.
	"f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97,
	"f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
	"f13": 105, "f14": 107, "f15": 113, "f16": 106, "f17": 64, "f18": 79,
	"f19": 80, "f20": 90,
}

// wailsAccepts reports whether Wails would parse accel and register it as a
// global shortcut on macOS, following the code the tables mirror.
func wailsAccepts(accel string) error {
	parts := strings.Split(accel, "+")
	last := len(parts) - 1
	for _, m := range parts[:last] {
		if !wailsModifiers[strings.ToLower(m)] {
			return fmt.Errorf("%q isn't a modifier Wails knows", m)
		}
	}
	k := strings.ToLower(parts[last])
	single := len(k) == 1 && strconv.IsPrint(rune(k[0]))
	if !wailsNamedKeys[k] && !single {
		return fmt.Errorf("%q isn't a key Wails can parse", parts[last])
	}
	if _, ok := wailsMacKeyCodes[k]; !ok {
		return fmt.Errorf("%q has no macOS key code in Wails", parts[last])
	}
	return nil
}

// TestEveryKeyHasAnAccelerator checks that Wails accepts every key and
// modifier Parse does.
func TestEveryKeyHasAnAccelerator(t *testing.T) {
	for name := range keys {
		for _, m := range modOrder {
			p := parsed{mods: m.mod, key: name}
			if err := wailsAccepts(p.accelerator()); err != nil {
				t.Errorf("%q as %q: %v", p, p.accelerator(), err)
			}
		}
	}
	all := parsed{mods: modCtrl | modAlt | modShift | modCmd, key: "k"}
	if got, want := all.accelerator(), "Ctrl+Option+Shift+Cmd+K"; got != want {
		t.Errorf("accelerator(%q) = %q, want %q", all, got, want)
	}
}

// TestWailsTablesMatchSource checks the mirrored tables against the Wails
// source in the module cache. It's skipped if the source isn't there.
func TestWailsTablesMatchSource(t *testing.T) {
	dir := wailsDir(t)
	app := filepath.Join(dir, "pkg", "application")

	modifiers := mapKeys(t, filepath.Join(app, "keys.go"), "modifierMap")
	for m := range wailsModifiers {
		if _, ok := modifiers[m]; !ok {
			t.Errorf("keys.go: modifierMap has no %q", m)
		}
	}
	named := mapKeys(t, filepath.Join(app, "keys.go"), "namedKeys")
	for k := range wailsNamedKeys {
		if _, ok := named[k]; !ok {
			t.Errorf("keys.go: namedKeys has no %q", k)
		}
	}
	codes := mapKeys(t, filepath.Join(app, "global_shortcut_darwin.go"), "macKeyCodes")
	for k, want := range wailsMacKeyCodes {
		got, ok := codes[k]
		if !ok {
			t.Errorf("global_shortcut_darwin.go: macKeyCodes has no %q", k)
			continue
		}
		if got != strconv.Itoa(want) {
			t.Errorf("global_shortcut_darwin.go: macKeyCodes[%q] = %s, want %d", k, got, want)
		}
	}
}

// wailsDir finds the Wails module's source, or skips the test.
func wailsDir(t *testing.T) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command to find the Wails source")
	}
	out, err := exec.Command(gobin, "list", "-m", "-f", "{{.Dir}}", "github.com/wailsapp/wails/v3").Output()
	dir := strings.TrimSpace(string(out))
	if err != nil || dir == "" {
		t.Skipf("Wails source isn't in the module cache (%v)", err)
	}
	return dir
}

// mapKeys returns the string keys of the map literal assigned to a
// package-level variable, with each value's text (see valueText).
func mapKeys(t *testing.T, path, variable string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if name.Name != variable || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.CompositeLit)
				if !ok {
					t.Fatalf("%s: %s isn't a literal", path, variable)
				}
				return literalEntries(t, lit)
			}
		}
	}
	t.Fatalf("%s: no variable %s", path, variable)
	return nil
}

// literalEntries returns a map literal's string keys and its values' text.
func literalEntries(t *testing.T, lit *ast.CompositeLit) map[string]string {
	t.Helper()
	entries := make(map[string]string, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			continue
		}
		k, err := strconv.Unquote(key.Value)
		if err != nil {
			t.Fatalf("unquoting %s: %v", key.Value, err)
		}
		entries[k] = valueText(kv.Value)
	}
	return entries
}

// valueText returns a value's text for comparison: an int literal's digits,
// or a placeholder for anything else.
func valueText(v ast.Expr) string {
	if lit, ok := v.(*ast.BasicLit); ok {
		return lit.Value
	}
	return "…"
}
