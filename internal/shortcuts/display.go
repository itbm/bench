package shortcuts

// Display formats a canonical shortcut the way macOS does: ⌃ for ctrl, ⌥ for
// alt, ⇧ for shift, ⌘ for cmd, then the key ("Space", "Tab", "←", "]", "1").
// Letters are in upper case ("⌃⌥J"), and modifiers alone are shown alone
// ("⌃⌥⇧"). It accepts any form Parse does, and returns s unchanged if it
// isn't a shortcut.
func Display(shortcut string) string {
	p, err := parse(shortcut)
	if err != nil {
		return shortcut
	}
	return p.display()
}
