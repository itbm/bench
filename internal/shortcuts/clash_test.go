package shortcuts

import (
	"errors"
	"slices"
	"testing"
)

func TestUsesVoiceOverKeys(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"ctrl+alt", true},
		{"ctrl+alt+space", true},
		{"Option+Control+Shift+Left", true},
		{"ctrl+alt+shift+cmd+k", true},
		{"ctrl+cmd", false},
		{"alt+cmd+j", false},
		{"ctrl+space", false},
		{"", false},
		{"ctrl+hyper+k", false},
	}
	for _, tt := range tests {
		if got := UsesVoiceOverKeys(tt.in); got != tt.want {
			t.Errorf("UsesVoiceOverKeys(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestWithoutVoiceOverKeys(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ctrl+alt", "ctrl+cmd"},
		{"ctrl+alt+shift", "ctrl+shift+cmd"},
		{"ctrl+alt+]", "ctrl+cmd+]"},
		{"ctrl+alt+cmd+j", "ctrl+cmd+j"},
		{"Option+Control+Space", "ctrl+cmd+space"},
		// Nothing to change, but canonical.
		{"alt+cmd+j", "alt+cmd+j"},
		{"Control+Command+K", "ctrl+cmd+k"},
		{"", ""},
		{"ctrl+hyper+k", ""},
	}
	for _, tt := range tests {
		got := WithoutVoiceOverKeys(tt.in)
		if got != tt.want {
			t.Errorf("WithoutVoiceOverKeys(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if got != "" && UsesVoiceOverKeys(got) {
			t.Errorf("WithoutVoiceOverKeys(%q) = %q, which VoiceOver still uses", tt.in, got)
		}
	}
}

// TestWithoutVoiceOverKeysStaysValid checks that every default field
// without VoiceOver's keys is still a value its field accepts.
func TestWithoutVoiceOverKeysStaysValid(t *testing.T) {
	fields := defaultConfig().Fields()
	for name, value := range fields {
		fields[name] = WithoutVoiceOverKeys(value)
	}
	if _, errs := Bindings(FromFields(fields)); len(errs) != 0 {
		t.Errorf("the default table without VoiceOver's keys has problems: %v", errs)
	}
}

func TestReserved(t *testing.T) {
	for _, s := range []string{"cmd+space", "Control+Command+Space", "ctrl+cmd+f", "shift+cmd+4"} {
		if !Reserved(s) {
			t.Errorf("Reserved(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"ctrl+alt+space", "ctrl+cmd+j", "", "ctrl+hyper+k"} {
		if Reserved(s) {
			t.Errorf("Reserved(%q) = true, want false", s)
		}
	}
}

func TestFieldNamesAndLabels(t *testing.T) {
	names := FieldNames()
	if got := len(defaultConfig().Fields()); len(names) != got {
		t.Fatalf("FieldNames has %d names, want %d", len(names), got)
	}
	if names[0] != "tab_modifiers" || names[len(names)-1] != "add_front_window" {
		t.Errorf("FieldNames = %q, want config order", names)
	}
	for _, name := range names {
		if Label(name) == "" {
			t.Errorf("field %q has no label", name)
		}
	}
	if Label("nope") != "" {
		t.Error("Label of a name that isn't a field isn't empty")
	}
	if slices.Contains(names, "") {
		t.Error("FieldNames has an empty name")
	}
}

func TestTakenError(t *testing.T) {
	c := defaultConfig()
	c.Jump = "ctrl+alt+]"
	_, errs := Bindings(c)
	var taken *TakenError
	if len(errs) != 1 || !errors.As(errs[0], &taken) {
		t.Fatalf("errs = %v, want one TakenError", errs)
	}
	if taken.Shortcut != "ctrl+alt+]" || taken.Other != "shortcuts.next_tab" {
		t.Errorf("TakenError = %+v, want ctrl+alt+] taken by shortcuts.next_tab", taken)
	}
}

func TestDescribe(t *testing.T) {
	c := defaultConfig()
	c.Jump = "ctrl+alt+]"
	c.NextWindow = "ctrl+alt+shift+left"
	c.ZoomPane = "hyper+x"
	_, errs := Bindings(c)
	var got []string
	for _, err := range errs {
		got = append(got, Describe(err))
	}
	if len(got) != 4 {
		t.Fatalf("Describe gave %q, want four sentences", got)
	}
	if got[0] != "⌃⌥] is already the shortcut for Next tab." {
		t.Errorf("Describe(taken) = %q", got[0])
	}
	if got[1] != "⌃⌥⇧← is already the shortcut for Move a window." {
		t.Errorf("Describe(taken by modifiers) = %q", got[1])
	}
	if got[2] != `"ctrl+alt+shift+left" already includes shift; leave it out, because Bench adds it for the previous window.` {
		t.Errorf("Describe(field error) = %q", got[2])
	}
	if got[3] != `"hyper" isn't a modifier; use ctrl, alt, shift, or cmd.` {
		t.Errorf("Describe(parse error) = %q", got[3])
	}
}

func TestDescribeDerived(t *testing.T) {
	c := defaultConfig()
	c.Rearrange = "ctrl+alt+shift+tab"
	_, errs := Bindings(c)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want one", errs)
	}
	if got, want := Describe(errs[0]), "⌃⌥⇧Tab, with shift added, for the previous window, is already the shortcut for Rearrange."; got != want {
		t.Errorf("Describe = %q, want %q", got, want)
	}
}

func TestAddedKeys(t *testing.T) {
	for field, want := range map[string]string{
		"tab_modifiers": "1 to 9", "focus_pane_modifiers": "the arrow keys", "move_window_modifiers": "the arrow keys", "jump": "",
	} {
		if got := AddedKeys(field); got != want {
			t.Errorf("AddedKeys(%q) = %q, want %q", field, got, want)
		}
	}
}
