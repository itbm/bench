package shortcuts

import "testing"

func TestSuggest(t *testing.T) {
	tests := []struct{ in, want string }{
		// Keys that are often taken, swapped for nearby ones.
		{"ctrl+alt+space", "ctrl+alt+j"},
		{"ctrl+alt+]", "ctrl+alt+."},
		{"ctrl+alt+[", "ctrl+alt+,"},
		{"ctrl+alt+tab", "ctrl+alt+`"},
		{"ctrl+alt+shift+tab", "ctrl+alt+shift+`"},

		// Otherwise a modifier is added, Command first.
		{"ctrl+alt+left", "ctrl+alt+cmd+left"},
		{"ctrl+alt+shift+down", "ctrl+alt+shift+cmd+down"},
		{"ctrl+alt+1", "ctrl+alt+cmd+1"},
		{"ctrl+alt+0", "ctrl+alt+cmd+0"},
		{"ctrl+alt+j", "ctrl+alt+cmd+j"},
		{"ctrl+alt+cmd+j", "ctrl+alt+shift+cmd+j"},
		{"alt+shift+cmd+k", "ctrl+alt+shift+cmd+k"},
		{"ctrl+shift+cmd+k", "ctrl+alt+shift+cmd+k"},
		{"f13", "cmd+f13"},
		{"ctrl+alt", "ctrl+alt+cmd"},

		// Any form Parse accepts, with a canonical result.
		{"Option+Control+Space", "ctrl+alt+j"},

		// Shortcuts Parse would refuse get one it accepts.
		{"shift+space", "shift+cmd+space"},
		{"j", "cmd+j"},
		{"shift", "shift+cmd"},

		// Nothing left to try, or not a shortcut.
		{"ctrl+alt+shift+cmd+k", ""},
		{"", ""},
		{"ctrl+hyper+k", ""},
	}
	for _, tt := range tests {
		if got := Suggest(tt.in); got != tt.want {
			t.Errorf("Suggest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestSuggestEndsAndStaysValid follows the suggestions for every default
// binding until they run out, checking that each is new, would be accepted
// in the same field, and would be registered by Wails.
func TestSuggestEndsAndStaysValid(t *testing.T) {
	bindings, errs := Bindings(defaultConfig())
	if len(errs) != 0 {
		t.Fatalf("Bindings(default) errors: %v", errs)
	}
	for _, b := range bindings {
		seen := map[string]bool{b.Shortcut: true}
		for s := Suggest(b.Shortcut); s != ""; s = Suggest(s) {
			if seen[s] {
				t.Errorf("suggestions for %q repeat %q", b.Shortcut, s)
				break
			}
			seen[s] = true
			if got, err := Parse(s, false); err != nil || got != s {
				t.Errorf("suggestion %q for %q: Parse = %q, %v; want it unchanged", s, b.Shortcut, got, err)
			}
			p, _ := parse(s)
			if err := wailsAccepts(p.accelerator()); err != nil {
				t.Errorf("suggestion %q for %q: Wails wouldn't register it: %v", s, b.Shortcut, err)
			}
		}
		if len(seen) < 2 {
			t.Errorf("no suggestion for %q", b.Shortcut)
		}
	}
}

// TestSuggestIsAlwaysValid checks every key with every combination of
// modifiers, and modifiers alone: each suggestion is new, canonical, and
// accepted by Parse in the same form as the input, and Wails would register
// it.
func TestSuggestIsAlwaysValid(t *testing.T) {
	names := []string{""} // Modifiers alone.
	for name := range keys {
		names = append(names, name)
	}
	for _, name := range names {
		for m := mods(0); m <= modCtrl|modAlt|modShift|modCmd; m++ {
			p := parsed{mods: m, key: name}
			if p.String() == "" {
				continue
			}
			s := Suggest(p.String())
			if s == "" {
				if m != modCtrl|modAlt|modShift|modCmd || alternativeKeys[name] != "" {
					t.Errorf("Suggest(%q) = \"\", want a suggestion", p)
				}
				continue
			}
			modifiersOnly := name == ""
			got, err := Parse(s, modifiersOnly)
			if err != nil || got != s || s == p.String() {
				t.Errorf("Suggest(%q) = %q: Parse(%q, %t) = %q, %v; want a new, canonical shortcut",
					p, s, s, modifiersOnly, got, err)
				continue
			}
			if q, _ := parse(s); !modifiersOnly {
				if err := wailsAccepts(q.accelerator()); err != nil {
					t.Errorf("Suggest(%q) = %q: Wails wouldn't register it: %v", p, s, err)
				}
			}
		}
	}
}
