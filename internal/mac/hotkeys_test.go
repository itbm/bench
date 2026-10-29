package mac

import (
	"slices"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

func TestRawHotKeyShortcut(t *testing.T) {
	tests := []struct {
		key  rawHotKey
		want string
	}{
		// macOS's defaults: Control-Space and Control-Option-Space.
		{rawHotKey{KeyCode: 49, Flags: 262144}, "ctrl+space"},
		{rawHotKey{KeyCode: 49, Flags: 786432}, "ctrl+alt+space"},
		// Every modifier, in canonical order.
		{rawHotKey{KeyCode: 38, Flags: hotKeyCmd | hotKeyShift | hotKeyAlt | hotKeyCtrl}, "ctrl+alt+shift+cmd+j"},
		// The function key's flag, which arrow keys carry, is left out.
		{rawHotKey{KeyCode: 123, Flags: hotKeyCtrl | 1<<23}, "ctrl+left"},
		{rawHotKey{KeyCode: 30, Flags: hotKeyAlt}, "alt+]"},
		{rawHotKey{KeyCode: 122}, "f1"},
		// Keys that Bench's shortcuts couldn't be.
		{rawHotKey{KeyCode: 49}, ""},
		{rawHotKey{KeyCode: 999, Flags: hotKeyCtrl}, ""},
	}
	for _, tt := range tests {
		if got := tt.key.shortcut(); got != tt.want {
			t.Errorf("shortcut of %+v = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// TestKeyNamesMatchWails checks the table against the key codes Wails
// registers, for the keys both have.
func TestKeyNamesMatchWails(t *testing.T) {
	wails := map[string]int{
		"a": 0, "j": 38, "m": 46, "1": 18, "0": 29, "=": 24, "]": 30, "`": 50,
		"return": 36, "tab": 48, "space": 49, "delete": 51, "forwarddelete": 117, "escape": 53,
		"home": 115, "pageup": 116, "end": 119, "pagedown": 121,
		"left": 123, "right": 124, "down": 125, "up": 126, "f1": 122, "f20": 90,
	}
	for name, code := range wails {
		if keyNames[code] != name {
			t.Errorf("key code %d is %q, want %q", code, keyNames[code], name)
		}
	}
	seen := map[string]bool{}
	for _, name := range keyNames {
		if seen[name] {
			t.Errorf("%q has two key codes", name)
		}
		seen[name] = true
	}
}

func TestSystemKeysFrom(t *testing.T) {
	defaults := []rawHotKey{
		{Enabled: true, KeyCode: 49, Flags: hotKeyCtrl},
		{Enabled: true, KeyCode: 49, Flags: hotKeyCtrl | hotKeyAlt},
	}
	tests := []struct {
		name      string
		hotKeys   []rawHotKey
		sources   int
		voiceOver bool
		want      platform.SystemKeys
	}{
		{"one input source", defaults, 1, false, platform.SystemKeys{}},
		{"two input sources", defaults, 2, false, platform.SystemKeys{InputSources: []string{"ctrl+space", "ctrl+alt+space"}}},
		{"turned off", []rawHotKey{defaults[0], {KeyCode: 49, Flags: hotKeyCtrl | hotKeyAlt}}, 3, false,
			platform.SystemKeys{InputSources: []string{"ctrl+space"}}},
		{"the same twice", []rawHotKey{defaults[0], defaults[0]}, 2, false,
			platform.SystemKeys{InputSources: []string{"ctrl+space"}}},
		{"voiceover", nil, 0, true, platform.SystemKeys{VoiceOver: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := systemKeysFrom(tt.hotKeys, tt.sources, tt.voiceOver)
			if got.VoiceOver != tt.want.VoiceOver || !slices.Equal(got.InputSources, tt.want.InputSources) {
				t.Errorf("systemKeysFrom = %+v, want %+v", got, tt.want)
			}
			if !sameSystemKeys(got, tt.want) {
				t.Error("sameSystemKeys says they differ")
			}
		})
	}
	if sameSystemKeys(platform.SystemKeys{}, platform.SystemKeys{VoiceOver: true}) {
		t.Error("sameSystemKeys says VoiceOver on and off are the same")
	}
}
