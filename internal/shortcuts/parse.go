package shortcuts

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// mods is a set of modifier keys.
type mods uint8

const (
	modCtrl mods = 1 << iota
	modAlt
	modShift
	modCmd
)

// typingMods are the modifiers that keep a shortcut from taking over normal
// typing. Shift alone doesn't: Shift+J is a capital J.
const typingMods = modCtrl | modAlt | modCmd

// modifier describes one modifier key in each of the forms Bench writes it.
type modifier struct {
	mod     mods
	name    string // Canonical config name.
	accel   string // The name Wails' accelerator parser accepts.
	display string // The symbol macOS shows.
}

// modOrder lists the modifiers in canonical order, which is also the order
// macOS shows them in: Control, Option, Shift, Command.
var modOrder = []modifier{
	{modCtrl, "ctrl", "Ctrl", "⌃"},
	{modAlt, "alt", "Option", "⌥"},
	{modShift, "shift", "Shift", "⇧"},
	{modCmd, "cmd", "Cmd", "⌘"},
}

// modNames maps every name a config may use for a modifier to the modifier.
var modNames = map[string]mods{
	"ctrl":    modCtrl,
	"control": modCtrl,
	"alt":     modAlt,
	"option":  modAlt,
	"opt":     modAlt,
	"shift":   modShift,
	"cmd":     modCmd,
	"command": modCmd,
}

// key describes one key, by its canonical config name.
type key struct {
	// accel is the key's name in a Wails accelerator. Wails lower-cases it
	// and looks it up in its macOS key-code table (macKeyCodes in
	// pkg/application/global_shortcut_darwin.go).
	accel string
	// display is how macOS shows the key in a shortcut.
	display string
	// bare keys may be shortcuts without ctrl, alt, or cmd (function keys,
	// which don't type anything).
	bare bool
}

// keys maps each canonical key name to the key. Wails names the Mac's
// Delete key (⌫) "backspace" and its Forward Delete key (⌦) "delete";
// Bench's config uses the Mac names, "delete" and "forwarddelete".
var keys = buildKeys()

func buildKeys() map[string]key {
	k := map[string]key{
		"space":         {accel: "Space", display: "Space"},
		"tab":           {accel: "Tab", display: "Tab"},
		"return":        {accel: "Return", display: "↩"},
		"escape":        {accel: "Escape", display: "⎋"},
		"delete":        {accel: "Backspace", display: "⌫"},
		"forwarddelete": {accel: "Delete", display: "⌦"},
		"home":          {accel: "Home", display: "↖"},
		"end":           {accel: "End", display: "↘"},
		"pageup":        {accel: "Page Up", display: "⇞"},
		"pagedown":      {accel: "Page Down", display: "⇟"},
		"left":          {accel: "Left", display: "←"},
		"right":         {accel: "Right", display: "→"},
		"up":            {accel: "Up", display: "↑"},
		"down":          {accel: "Down", display: "↓"},
	}
	for c := 'a'; c <= 'z'; c++ {
		upper := strings.ToUpper(string(c))
		k[string(c)] = key{accel: upper, display: upper}
	}
	for c := '0'; c <= '9'; c++ {
		k[string(c)] = key{accel: string(c), display: string(c)}
	}
	for _, c := range punctuation {
		k[string(c)] = key{accel: string(c), display: string(c)}
	}
	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("F%d", i)
		k[strings.ToLower(name)] = key{accel: name, display: name, bare: true}
	}
	return k
}

// punctuation is the punctuation keys of a US keyboard that Wails can
// register as global shortcuts on macOS. Wails binds hardware key codes, so
// on other layouts the key in the same place triggers the shortcut.
const punctuation = "=-[]';\\,/.`"

// keyAliases maps other names for keys to their canonical names. Names are
// matched after lower-casing and removing spaces, hyphens, and underscores,
// so "Page Up", "page-up", and "page_up" are all "pageup".
var keyAliases = map[string]string{
	"spacebar":  "space",
	"enter":     "return",
	"esc":       "escape",
	"backspace": "delete",
	"pgup":      "pageup",
	"pgdn":      "pagedown",
	"pgdown":    "pagedown",
}

// parsed is a parsed shortcut, or modifiers alone if key is "".
type parsed struct {
	mods mods
	key  string // Canonical key name.
}

// String returns the canonical config form, such as "ctrl+alt+shift+left".
func (p parsed) String() string {
	parts := make([]string, 0, len(modOrder)+1)
	for _, m := range modOrder {
		if p.mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	if p.key != "" {
		parts = append(parts, p.key)
	}
	return strings.Join(parts, "+")
}

// accelerator returns the Wails v3 accelerator, such as "Ctrl+Option+Space".
func (p parsed) accelerator() string {
	parts := make([]string, 0, len(modOrder)+1)
	for _, m := range modOrder {
		if p.mods&m.mod != 0 {
			parts = append(parts, m.accel)
		}
	}
	if p.key != "" {
		parts = append(parts, keys[p.key].accel)
	}
	return strings.Join(parts, "+")
}

// display returns the macOS form, such as "⌃⌥⇧→".
func (p parsed) display() string {
	var b strings.Builder
	for _, m := range modOrder {
		if p.mods&m.mod != 0 {
			b.WriteString(m.display)
		}
	}
	if p.key != "" {
		b.WriteString(keys[p.key].display)
	}
	return b.String()
}

// with returns p with the modifiers in m added and its key replaced by k,
// or kept if k is "".
func (p parsed) with(m mods, k string) parsed {
	if k == "" {
		k = p.key
	}
	return parsed{mods: p.mods | m, key: k}
}

// errEmpty is the error for an empty shortcut.
var errEmpty = errors.New("the shortcut is empty")

// parse reads a shortcut, or modifiers alone, without checking which of the
// two it is or whether it would take over typing.
func parse(s string) (parsed, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return parsed{}, errEmpty
	}
	parts := strings.Split(text, "+")
	last := len(parts) - 1
	var p parsed
	for i, raw := range parts {
		part := strings.ToLower(strings.TrimSpace(raw))
		if part == "" {
			return parsed{}, fmt.Errorf("%q has an empty part; write a shortcut such as \"ctrl+alt+space\"", text)
		}
		if m, ok := modNames[part]; ok {
			p.mods |= m
			continue
		}
		name, isKey := keyName(part)
		switch {
		case isKey && i < last:
			return parsed{}, fmt.Errorf("%q has to come last, after the modifiers", strings.TrimSpace(raw))
		case !isKey && i < last:
			return parsed{}, fmt.Errorf("%q isn't a modifier; use ctrl, alt, shift, or cmd", strings.TrimSpace(raw))
		case !isKey:
			return parsed{}, fmt.Errorf("%q isn't a key Bench can use; use a letter, a digit, punctuation, "+
				"space, tab, return, escape, delete, an arrow (left, right, up, or down), or f1 to f20",
				strings.TrimSpace(raw))
		}
		p.key = name
	}
	return p, nil
}

// keyName returns the canonical name of a lower-case key name, and whether
// it's a key Bench can use.
func keyName(part string) (string, bool) {
	if utf8.RuneCountInString(part) > 1 {
		part = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(part)
	}
	if alias, ok := keyAliases[part]; ok {
		part = alias
	}
	_, ok := keys[part]
	return part, ok
}

// check reports whether p has the right form: a key unless modifiersOnly,
// no key if modifiersOnly, and at least one of ctrl, alt, and cmd unless the
// key is a function key. text is what the user wrote, for messages.
func (p parsed) check(text string, modifiersOnly bool) error {
	text = strings.TrimSpace(text)
	switch {
	case modifiersOnly && p.key != "":
		return fmt.Errorf("%q includes a key; use modifiers alone, such as \"ctrl+alt\", and Bench adds the keys", text)
	case !modifiersOnly && p.key == "":
		return fmt.Errorf("%q has no key; add one, as in \"ctrl+alt+space\"", text)
	case p.mods&typingMods == 0 && !keys[p.key].bare:
		return fmt.Errorf("%q needs ctrl, alt, or cmd, so that it doesn't take over typing", text)
	}
	return nil
}

// valid reports whether Parse would accept p's canonical form: as modifiers
// alone if p has no key, and otherwise as a whole shortcut.
func (p parsed) valid() bool {
	return p.check(p.String(), p.key == "") == nil
}

// Parse checks a shortcut such as "ctrl+alt+space", "ctrl+alt+]", or
// "ctrl+alt+shift+left" and returns its canonical form: lower case,
// modifiers in the order ctrl, alt, shift, cmd ("option" and "opt" mean alt,
// "control" means ctrl, "command" means cmd). With modifiersOnly, it accepts
// modifiers alone, such as "ctrl+alt", and nothing else.
//
// Keys are letters, digits, the punctuation keys = - [ ] ' ; \ , / . and `,
// space, tab, return (or enter), escape (or esc), delete (the Mac's ⌫, or
// backspace), forwarddelete (⌦), home, end, pageup, pagedown, the arrows
// left, right, up, and down, and f1 to f20. Every shortcut except a bare
// function key needs ctrl, alt, or cmd, so that it doesn't take over typing.
func Parse(s string, modifiersOnly bool) (string, error) {
	p, err := parse(s)
	if err != nil {
		return "", err
	}
	if err := p.check(s, modifiersOnly); err != nil {
		return "", err
	}
	return p.String(), nil
}

// Modifiers returns the modifier keys of a shortcut, or of modifiers alone,
// by their canonical names in canonical order: "ctrl", "alt", "shift", and
// "cmd". It returns ["ctrl", "alt"] for "ctrl+alt+`", the keys that the
// hold after last_tab's shortcut waits for the user to let go of, and nil
// for anything that isn't a shortcut.
func Modifiers(shortcut string) []string {
	p, err := parse(shortcut)
	if err != nil {
		return nil
	}
	var names []string
	for _, m := range modOrder {
		if p.mods&m.mod != 0 {
			names = append(names, m.name)
		}
	}
	return names
}
