package mac

import (
	"slices"
	"strings"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// This file turns the input sources' shortcuts, as macOS keeps them in
// com.apple.symbolichotkeys, into shortcuts in Bench's form, for
// SystemKeys. It's pure Go, so it builds and is tested on any OS.

// keyNames maps macOS's virtual key codes (kVK_* in Carbon's
// HIToolbox/Events.h) to Bench's key names (shortcuts.Parse), for the keys
// that Bench can use. Codes name places on a US keyboard, as Wails' do.
var keyNames = map[int]string{
	0: "a", 1: "s", 2: "d", 3: "f", 4: "h", 5: "g", 6: "z", 7: "x", 8: "c", 9: "v",
	11: "b", 12: "q", 13: "w", 14: "e", 15: "r", 16: "y", 17: "t", 31: "o", 32: "u",
	34: "i", 35: "p", 37: "l", 38: "j", 40: "k", 45: "n", 46: "m",
	18: "1", 19: "2", 20: "3", 21: "4", 22: "6", 23: "5", 25: "9", 26: "7", 28: "8", 29: "0",
	24: "=", 27: "-", 30: "]", 33: "[", 39: "'", 41: ";", 42: "\\", 43: ",", 44: "/", 47: ".", 50: "`",
	36: "return", 48: "tab", 49: "space", 51: "delete", 53: "escape", 117: "forwarddelete",
	115: "home", 116: "pageup", 119: "end", 121: "pagedown",
	123: "left", 124: "right", 125: "down", 126: "up",
	122: "f1", 120: "f2", 99: "f3", 118: "f4", 96: "f5", 97: "f6", 98: "f7", 100: "f8",
	101: "f9", 109: "f10", 103: "f11", 111: "f12", 105: "f13", 107: "f14", 113: "f15",
	106: "f16", 64: "f17", 79: "f18", 80: "f19", 90: "f20",
}

// The modifier flags that a symbolic hot key keeps, as NSEventModifierFlags
// has them. It may keep others too, such as the function key's for an
// arrow key, which Bench's shortcuts leave out.
const (
	hotKeyShift = 1 << 17
	hotKeyCtrl  = 1 << 18
	hotKeyAlt   = 1 << 19
	hotKeyCmd   = 1 << 20
)

// rawHotKey is a symbolic hot key as the native side reads it: whether it's
// turned on, its virtual key code, and its modifier flags.
type rawHotKey struct {
	Enabled bool
	KeyCode int
	Flags   int
}

// shortcut returns the hot key as a shortcut in canonical form, such as
// "ctrl+alt+space", or "" for one that Bench's shortcuts couldn't be.
func (k rawHotKey) shortcut() string {
	name, ok := keyNames[k.KeyCode]
	if !ok {
		return ""
	}
	var parts []string
	for _, m := range []struct {
		flag int
		name string
	}{{hotKeyCtrl, "ctrl"}, {hotKeyAlt, "alt"}, {hotKeyShift, "shift"}, {hotKeyCmd, "cmd"}} {
		if k.Flags&m.flag != 0 {
			parts = append(parts, m.name)
		}
	}
	s, err := shortcuts.Parse(strings.Join(append(parts, name), "+"), false)
	if err != nil {
		return ""
	}
	return s
}

// systemKeysFrom returns the keys that macOS keeps for itself: the input
// sources' hot keys that are turned on, while sources, the number of input
// sources that the user can switch between, is more than one, since they do
// nothing otherwise; and VoiceOver's, while it's on.
func systemKeysFrom(hotKeys []rawHotKey, sources int, voiceOver bool) platform.SystemKeys {
	k := platform.SystemKeys{VoiceOver: voiceOver}
	if sources < 2 {
		return k
	}
	for _, h := range hotKeys {
		if s := h.shortcut(); h.Enabled && s != "" && !slices.Contains(k.InputSources, s) {
			k.InputSources = append(k.InputSources, s)
		}
	}
	return k
}

// sameSystemKeys reports whether a and b are the same keys.
func sameSystemKeys(a, b platform.SystemKeys) bool {
	return a.VoiceOver == b.VoiceOver && slices.Equal(a.InputSources, b.InputSources)
}
