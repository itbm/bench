package shortcuts

// alternativeKeys are keys to try instead of ones that other apps or macOS
// often take with the same modifiers.
var alternativeKeys = map[string]string{
	// Control+Option+Space switches input sources when more than one is set
	// up. J is for jump.
	"space": "j",
	// The keys beside M, which also read as > and <.
	"]": ".",
	"[": ",",
	// The key above Tab, which macOS uses to cycle through an app's windows.
	"tab": "`",
}

// suggestMods is the order in which Suggest adds a modifier the shortcut
// doesn't have yet. Command comes first: window tools such as Rectangle take
// Control+Option+arrow, but rarely Control+Option+Command+arrow.
var suggestMods = []mods{modCmd, modShift, modCtrl, modAlt}

// Suggest returns another shortcut to try when one can't be registered,
// such as "ctrl+alt+j" for "ctrl+alt+space". It swaps keys that are often
// taken for nearby ones (space for j, ] and [ for . and ",", tab for `), and
// otherwise adds a modifier, cmd first ("ctrl+alt+left" becomes
// "ctrl+alt+cmd+left"). Modifiers alone get a modifier added ("ctrl+alt"
// becomes "ctrl+alt+cmd").
//
// The result is canonical and is always a shortcut Parse accepts in the same
// form as the input, so asking again for it gives a further suggestion. It
// returns "" if s isn't a shortcut or nothing is left to try.
func Suggest(shortcut string) string {
	p, err := parse(shortcut)
	if err != nil {
		return ""
	}
	if k, ok := alternativeKeys[p.key]; ok {
		if alt := p.with(0, k); alt.valid() {
			return alt.String()
		}
	}
	for _, m := range suggestMods {
		if p.mods&m != 0 {
			continue
		}
		if next := p.with(m, ""); next.valid() {
			return next.String()
		}
	}
	return ""
}
