package mac

import "github.com/itbm/bench/internal/platform"

// This file turns a window tool's preferences, as benchCopyToolPreferences
// reads them from the tool's own defaults domain, into
// platform.ToolPreferences, with the shortcuts in Bench's form, for
// ToolPreferences (decision 69). It's pure Go, so it builds and is tested
// on any OS.

// The kinds of preference that benchCopyToolPreferences reads
// (BENCH_PREF_ in bridge_darwin.h).
const (
	prefNumber     = 0
	prefText       = 1
	prefShortcut   = 2
	prefNoShortcut = 3
	prefOther      = 4
)

// rawPreference is one preference as the native side reads it: its key,
// its kind, and for a shortcut, its virtual key code and its modifier
// flags (Number); for a number or a boolean, Number; for text, Text.
type rawPreference struct {
	Key     string
	Kind    int
	KeyCode int
	Number  int64
	Text    string
}

// toolPreferencesFrom sorts a tool's preferences by kind. A shortcut keeps
// the modifier flags that Bench's shortcuts have, and leaves out others,
// such as the function key's for an arrow key, as a symbolic hot key does
// (rawHotKey). One that Bench's shortcuts couldn't be, one set to none,
// and a value of another kind, which MASShortcut reads as none, are "":
// the key is set, so the tool's default for it doesn't count, and Bench
// guesses none.
func toolPreferencesFrom(prefs []rawPreference) platform.ToolPreferences {
	var p platform.ToolPreferences
	for _, r := range prefs {
		switch r.Kind {
		case prefNumber:
			if p.Numbers == nil {
				p.Numbers = map[string]int64{}
			}
			p.Numbers[r.Key] = r.Number
		case prefText:
			if p.Texts == nil {
				p.Texts = map[string]string{}
			}
			p.Texts[r.Key] = r.Text
		case prefShortcut, prefNoShortcut, prefOther:
			if p.Shortcuts == nil {
				p.Shortcuts = map[string]string{}
			}
			p.Shortcuts[r.Key] = ""
			if r.Kind == prefShortcut {
				p.Shortcuts[r.Key] = rawHotKey{Enabled: true, KeyCode: r.KeyCode, Flags: int(r.Number)}.shortcut()
			}
		}
	}
	return p
}
