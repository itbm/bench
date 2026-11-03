package shortcuts

// voiceOverKeys are VoiceOver's modifiers, its VO keys, which it uses with
// every key while it's on. Caps Lock can stand for them too, but it adds to
// Control and Option rather than replacing them.
const voiceOverKeys = modCtrl | modAlt

// UsesVoiceOverKeys reports whether a shortcut, or modifiers alone, has both
// Control and Option, which VoiceOver uses with every key while it's on. It
// reports false for anything that isn't a shortcut.
func UsesVoiceOverKeys(shortcut string) bool {
	p, err := parse(shortcut)
	return err == nil && p.mods&voiceOverKeys == voiceOverKeys
}

// WithoutVoiceOverKeys returns a shortcut, or modifiers alone, with Command
// in place of Option if it has both Control and Option, so that VoiceOver
// leaves it alone: "ctrl+alt+shift" becomes "ctrl+shift+cmd", and
// "ctrl+alt+]" becomes "ctrl+cmd+]". Anything else comes back in canonical
// form, and something that isn't a shortcut as "".
func WithoutVoiceOverKeys(shortcut string) string {
	return withCommandFor(shortcut, modAlt)
}

// WithoutVoiceOverControl does what WithoutVoiceOverKeys does, but with
// Command in place of Control: the other way to leave VoiceOver alone, for
// when Command in place of Option gives shortcuts that macOS keeps, as
// ⌃⇧⌘3 is. "ctrl+alt+shift" becomes "alt+shift+cmd".
func WithoutVoiceOverControl(shortcut string) string {
	return withCommandFor(shortcut, modCtrl)
}

// withCommandFor returns a shortcut, or modifiers alone, with Command in
// place of mod, Control or Option, if it has both of VoiceOver's keys; in
// canonical form, or "" if it isn't a shortcut.
func withCommandFor(shortcut string, mod mods) string {
	p, err := parse(shortcut)
	if err != nil {
		return ""
	}
	if p.mods&voiceOverKeys == voiceOverKeys {
		p.mods = p.mods&^mod | modCmd
	}
	return p.String()
}

// reserved are shortcuts that macOS, or every Mac app, uses, and that
// RegisterEventHotKey lets Bench take all the same, without saying so. Bench
// never suggests them.
var reserved = map[string]bool{
	"cmd+space":        true, // Spotlight.
	"alt+cmd+space":    true, // A Finder search window.
	"ctrl+space":       true, // The previous input source.
	"ctrl+cmd+space":   true, // Emoji & Symbols.
	"ctrl+cmd+f":       true, // Full screen.
	"ctrl+cmd+q":       true, // Lock Screen.
	"cmd+tab":          true, // The app switcher.
	"shift+cmd+tab":    true,
	"cmd+`":            true, // The app's next window.
	"shift+cmd+`":      true,
	"shift+cmd+3":      true, // Screenshots.
	"shift+cmd+4":      true,
	"shift+cmd+5":      true,
	"ctrl+shift+cmd+3": true, // Screenshots to the clipboard.
	"ctrl+shift+cmd+4": true,
}

// Reserved reports whether macOS, or every Mac app, uses a shortcut, such
// as Command+Space for Spotlight, so that Bench shouldn't suggest it. It
// accepts any form Parse does.
func Reserved(shortcut string) bool {
	p, err := parse(shortcut)
	return err == nil && reserved[p.String()]
}
