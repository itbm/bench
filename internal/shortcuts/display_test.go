package shortcuts

import "testing"

func TestDisplay(t *testing.T) {
	tests := []struct{ in, want string }{
		// As the prototype shows them.
		{"ctrl+alt+1", "⌃⌥1"},
		{"ctrl+alt+space", "⌃⌥Space"},
		{"ctrl+alt+shift+right", "⌃⌥⇧→"},
		{"ctrl+alt+shift+down", "⌃⌥⇧↓"},
		{"ctrl+alt+]", "⌃⌥]"},
		{"ctrl+alt+[", "⌃⌥["},
		{"ctrl+alt+tab", "⌃⌥Tab"},
		{"ctrl+alt+0", "⌃⌥0"},

		// Modifiers in macOS order, whatever order they're written in.
		{"cmd+shift+alt+ctrl+left", "⌃⌥⇧⌘←"},
		{"shift+cmd+up", "⇧⌘↑"},

		// Other keys.
		{"ctrl+alt+j", "⌃⌥J"},
		{"ctrl+alt+return", "⌃⌥↩"},
		{"ctrl+alt+escape", "⌃⌥⎋"},
		{"ctrl+alt+delete", "⌃⌥⌫"},
		{"ctrl+alt+forwarddelete", "⌃⌥⌦"},
		{"ctrl+alt+home", "⌃⌥↖"},
		{"ctrl+alt+end", "⌃⌥↘"},
		{"ctrl+alt+pageup", "⌃⌥⇞"},
		{"ctrl+alt+pagedown", "⌃⌥⇟"},
		{"ctrl+alt+`", "⌃⌥`"},
		{"cmd+f12", "⌘F12"},
		{"f13", "F13"},

		// Modifiers alone, for hints such as "⌃⌥⇧ arrows move window".
		{"ctrl+alt+shift", "⌃⌥⇧"},
		{"ctrl+alt", "⌃⌥"},

		// Forms that aren't canonical still display.
		{"Control+Option+Space", "⌃⌥Space"},

		// Not shortcuts: returned as they are.
		{"", ""},
		{"ctrl+hyper+j", "ctrl+hyper+j"},
		{"not a shortcut", "not a shortcut"},
	}
	for _, tt := range tests {
		if got := Display(tt.in); got != tt.want {
			t.Errorf("Display(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
