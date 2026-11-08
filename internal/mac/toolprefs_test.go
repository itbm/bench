package mac

import (
	"reflect"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// Rectangle keeps a shortcut the user changed as MASShortcut does: a key
// code, and modifier flags that may have device bits as well. These are as
// a Rectangle's preferences have them.
func TestToolPreferencesFrom(t *testing.T) {
	got := toolPreferencesFrom([]rawPreference{
		// ⌃⌥←, with device bits, as Rectangle keeps it.
		{Key: "leftHalf", Kind: prefShortcut, KeyCode: 123, Number: 0x00AC0101 | hotKeyCtrl | hotKeyAlt},
		// ⌃⌥↩.
		{Key: "maximize", Kind: prefShortcut, KeyCode: 36, Number: hotKeyCtrl | hotKeyAlt},
		// ⌃⌥⇧⌘F.
		{Key: "center", Kind: prefShortcut, KeyCode: 3, Number: hotKeyCtrl | hotKeyAlt | hotKeyShift | hotKeyCmd},
		// A key that Bench's shortcuts can't use: the keypad's 1.
		{Key: "topLeft", Kind: prefShortcut, KeyCode: 83, Number: hotKeyCtrl | hotKeyAlt},
		// Set to none, or to a dictionary without a key code, as the
		// native side reads both.
		{Key: "rightHalf", Kind: prefNoShortcut},
		// Data, or anything else that MASShortcut reads as no shortcut,
		// such as a mouse button's shortcut in a form of its own.
		{Key: "bottomHalf", Kind: prefOther},
		{Key: "alternateDefaultShortcuts", Kind: prefNumber, Number: 1},
		{Key: "installVersion", Kind: prefText, Text: "95"},
		// A kind that Bench doesn't know is left out.
		{Key: "other", Kind: 9},
	})
	want := platform.ToolPreferences{
		Shortcuts: map[string]string{
			"leftHalf":   "ctrl+alt+left",
			"maximize":   "ctrl+alt+return",
			"center":     "ctrl+alt+shift+cmd+f",
			"topLeft":    "",
			"rightHalf":  "",
			"bottomHalf": "",
		},
		Numbers: map[string]int64{"alternateDefaultShortcuts": 1},
		Texts:   map[string]string{"installVersion": "95"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toolPreferencesFrom = %+v, want %+v", got, want)
	}
	if got := toolPreferencesFrom(nil); got.Shortcuts != nil || got.Numbers != nil || got.Texts != nil {
		t.Errorf("toolPreferencesFrom(nil) = %+v, want none", got)
	}
}
