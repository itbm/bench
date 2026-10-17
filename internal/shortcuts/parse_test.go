package shortcuts

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in            string
		modifiersOnly bool
		want          string
	}{
		// The default config.
		{"ctrl+alt+1", false, "ctrl+alt+1"},
		{"ctrl+alt+]", false, "ctrl+alt+]"},
		{"ctrl+alt+[", false, "ctrl+alt+["},
		{"ctrl+alt+space", false, "ctrl+alt+space"},
		{"ctrl+alt+0", false, "ctrl+alt+0"},
		{"ctrl+alt+tab", false, "ctrl+alt+tab"},
		{"ctrl+alt", true, "ctrl+alt"},
		{"ctrl+alt+shift", true, "ctrl+alt+shift"},

		// Case, spaces, and modifier order and names.
		{"Ctrl+Alt+Space", false, "ctrl+alt+space"},
		{"  ctrl + alt + space  ", false, "ctrl+alt+space"},
		{"alt+ctrl+j", false, "ctrl+alt+j"},
		{"command+shift+option+control+left", false, "ctrl+alt+shift+cmd+left"},
		{"opt+cmd+k", false, "alt+cmd+k"},
		{"cmd+shift+alt+ctrl", true, "ctrl+alt+shift+cmd"},
		{"ctrl+ctrl+alt+j", false, "ctrl+alt+j"},
		{"shift+ctrl", true, "ctrl+shift"},

		// Keys and their other names.
		{"ctrl+alt+shift+left", false, "ctrl+alt+shift+left"},
		{"ctrl+alt+right", false, "ctrl+alt+right"},
		{"ctrl+alt+up", false, "ctrl+alt+up"},
		{"ctrl+alt+down", false, "ctrl+alt+down"},
		{"ctrl+alt+J", false, "ctrl+alt+j"},
		{"ctrl+alt+enter", false, "ctrl+alt+return"},
		{"ctrl+alt+esc", false, "ctrl+alt+escape"},
		{"ctrl+alt+backspace", false, "ctrl+alt+delete"},
		{"ctrl+alt+Forward Delete", false, "ctrl+alt+forwarddelete"},
		{"ctrl+alt+page up", false, "ctrl+alt+pageup"},
		{"ctrl+alt+page_down", false, "ctrl+alt+pagedown"},
		{"ctrl+alt+pgdn", false, "ctrl+alt+pagedown"},
		{"ctrl+alt+spacebar", false, "ctrl+alt+space"},
		{"ctrl+alt+-", false, "ctrl+alt+-"},
		{"ctrl+alt+`", false, "ctrl+alt+`"},
		{"ctrl+alt+\\", false, "ctrl+alt+\\"},
		{"cmd+F12", false, "cmd+f12"},

		// Function keys don't type, so they may stand alone.
		{"f13", false, "f13"},
		{"shift+f20", false, "shift+f20"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, tt.modifiersOnly)
		if err != nil {
			t.Errorf("Parse(%q, %t) error: %v", tt.in, tt.modifiersOnly, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q, %t) = %q, want %q", tt.in, tt.modifiersOnly, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		in            string
		modifiersOnly bool
		want          string // A part of the message.
	}{
		{"", false, "the shortcut is empty"},
		{"   ", true, "the shortcut is empty"},
		{"ctrl++j", false, `"ctrl++j" has an empty part`},
		{"ctrl+alt+", false, `"ctrl+alt+" has an empty part`},
		{"ctrl+hyper+j", false, `"hyper" isn't a modifier; use ctrl, alt, shift, or cmd`},
		{"ctrl+j+alt", false, `"j" has to come last, after the modifiers`},
		{"ctrl+alt+banana", false, `"banana" isn't a key Bench can use`},
		{"ctrl+alt+f21", false, `"f21" isn't a key Bench can use`},
		{"ctrl+alt+é", false, `"é" isn't a key Bench can use`},
		{"ctrl+alt+plus", false, `"plus" isn't a key Bench can use`},
		{"ctrl+alt", false, `"ctrl+alt" has no key; add one`},
		{"ctrl+alt+1", true, `"ctrl+alt+1" includes a key; use modifiers alone`},
		{"j", false, `"j" needs ctrl, alt, or cmd`},
		{"shift+space", false, `"shift+space" needs ctrl, alt, or cmd`},
		{"shift", true, `"shift" needs ctrl, alt, or cmd`},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, tt.modifiersOnly)
		if err == nil {
			t.Errorf("Parse(%q, %t) = %q, want an error", tt.in, tt.modifiersOnly, got)
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Parse(%q, %t) error = %q, want it to contain %q", tt.in, tt.modifiersOnly, err, tt.want)
		}
	}
}

// TestParseCanonicalIsStable checks that every key's canonical form parses
// back to itself, with and without every combination of modifiers.
func TestParseCanonicalIsStable(t *testing.T) {
	for name := range keys {
		for m := mods(0); m <= modCtrl|modAlt|modShift|modCmd; m++ {
			p := parsed{mods: m, key: name}
			if p.check(p.String(), false) != nil {
				continue
			}
			got, err := Parse(p.String(), false)
			if err != nil {
				t.Errorf("Parse(%q) error: %v", p.String(), err)
				continue
			}
			if got != p.String() {
				t.Errorf("Parse(%q) = %q, want it unchanged", p.String(), got)
			}
		}
	}
}
