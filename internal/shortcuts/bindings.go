package shortcuts

import (
	"errors"
	"fmt"
	"strings"
)

// tabKeys are the keys added to tab_modifiers, and the bindings' Args.
var tabKeys = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}

// arrowKeys are the keys added to focus_pane_modifiers and
// move_window_modifiers, and the bindings' Args.
var arrowKeys = []string{"left", "right", "up", "down"}

// AddedKeys names the keys that Bench adds to a field that holds modifiers:
// "1 to 9" for tab_modifiers, and "the arrow keys" for focus_pane_modifiers
// and move_window_modifiers. It returns "" for the fields that hold whole
// shortcuts.
func AddedKeys(field string) string {
	switch field {
	case "tab_modifiers":
		return "1 to 9"
	case "focus_pane_modifiers", "move_window_modifiers":
		return "the arrow keys"
	}
	return ""
}

// Bindings expands the [shortcuts] config into every binding: tabs 1 to 9,
// next and previous tab, jump, rearrange, focus and move in four directions,
// next and previous window (next_window, and next_window with Shift added),
// zoom pane, and the last used tab. It returns the bindings that parse and an
// error for each that doesn't, naming the field: each error is a *FieldError.
//
// An empty field turns its bindings off without an error. When two fields
// make the same shortcut, the first in that order keeps it and the other
// gets an error, because the same global shortcut can't run two actions.
// last_tab comes last, though the table lists it with the tabs: a config
// written before it existed gets its default, which mustn't take a shortcut
// that the config gives another field, such as "ctrl+alt+`", which Suggest
// offers for next_window. config.Parse turns it off then.
func Bindings(s Config) ([]Binding, []error) {
	b := &builder{owner: make(map[string]string)}
	b.withKeys("tab_modifiers", s.TabModifiers, ActionTab, tabKeys)
	b.single("next_tab", s.NextTab, ActionNextTab)
	b.single("previous_tab", s.PreviousTab, ActionPreviousTab)
	b.single("jump", s.Jump, ActionJump)
	b.single("rearrange", s.Rearrange, ActionRearrange)
	b.withKeys("focus_pane_modifiers", s.FocusPaneModifiers, ActionFocusPane, arrowKeys)
	b.withKeys("move_window_modifiers", s.MoveWindowModifiers, ActionMoveWindow, arrowKeys)
	b.nextWindow("next_window", s.NextWindow)
	b.single("zoom_pane", s.ZoomPane, ActionZoomPane)
	b.single("last_tab", s.LastTab, ActionLastTab)
	return b.bindings, b.errs
}

// builder collects bindings and errors.
type builder struct {
	bindings []Binding
	errs     []error
	// owner maps each canonical shortcut bound so far to its field.
	owner map[string]string
}

// fail records a problem with a field.
func (b *builder) fail(field string, err error) {
	b.errs = append(b.errs, &FieldError{Field: fieldPrefix + field, Err: err})
}

// parseField parses a field's value. It returns false if the field is empty
// or has a problem, which it records.
func (b *builder) parseField(field, value string, modifiersOnly bool) (parsed, bool) {
	if strings.TrimSpace(value) == "" {
		return parsed{}, false
	}
	p, err := parse(value)
	if err == nil {
		err = p.check(value, modifiersOnly)
	}
	if err != nil {
		b.fail(field, err)
		return parsed{}, false
	}
	return p, true
}

// add binds p to an action, unless another field already has it. derived
// explains, in messages, a shortcut that Bench made from the field rather
// than one the field holds, such as "with shift added, for the previous
// window"; it's "" otherwise.
func (b *builder) add(field string, p parsed, action, arg, derived string) {
	shortcut := p.String()
	if other, taken := b.owner[shortcut]; taken {
		b.fail(field, &TakenError{Shortcut: shortcut, Other: fieldPrefix + other, derived: derived})
		return
	}
	b.owner[shortcut] = field
	b.bindings = append(b.bindings, Binding{
		Shortcut:    shortcut,
		Accelerator: p.accelerator(),
		Display:     p.display(),
		Action:      action,
		Arg:         arg,
		Field:       fieldPrefix + field,
	})
}

// single binds a field that holds a whole shortcut.
func (b *builder) single(field, value, action string) {
	if p, ok := b.parseField(field, value, false); ok {
		b.add(field, p, action, "", "")
	}
}

// withKeys binds a field that holds modifiers to each of added, the keys
// Bench adds to them, with the key as the binding's Arg.
func (b *builder) withKeys(field, value, action string, added []string) {
	p, ok := b.parseField(field, value, true)
	if !ok {
		return
	}
	for _, k := range added {
		b.add(field, p.with(0, k), action, k, "")
	}
}

// nextWindow binds next_window, and next_window with Shift added to the
// previous window.
func (b *builder) nextWindow(field, value string) {
	p, ok := b.parseField(field, value, false)
	if !ok {
		return
	}
	b.add(field, p, ActionNextWindow, "", "")
	if p.mods&modShift != 0 {
		b.fail(field, fmt.Errorf("%q already includes shift; leave it out, because Bench adds it for the previous window",
			strings.TrimSpace(value)))
		return
	}
	b.add(field, p.with(modShift, ""), ActionPreviousWindow, "", "with shift added, for the previous window")
}

// TakenError is a field's problem when one of its shortcuts is another
// field's already: the same global shortcut can't run two actions.
type TakenError struct {
	// Shortcut is the shortcut, in canonical form, such as "ctrl+alt+j".
	Shortcut string
	// Other is the field that has it, such as "shortcuts.jump".
	Other string
	// derived explains a shortcut that Bench made from the field rather
	// than one the field holds, such as "with shift added, for the previous
	// window"; it's "" otherwise.
	derived string
}

// Error says which shortcut is taken, and by which field.
func (e *TakenError) Error() string {
	what := fmt.Sprintf("%q", e.Shortcut)
	if e.derived != "" {
		what += " (" + e.derived + ")"
	}
	return fmt.Sprintf("%s is already the shortcut for %s", what, e.Other)
}

// Describe says what's wrong with a field of the table, for one of the
// errors that Bindings returns, as Settings shows it: shortcuts as macOS
// shows them, and fields by their labels, as in "⌃⌥] is already the
// shortcut for Next tab.". Other errors are described by their message.
func Describe(err error) string {
	var taken *TakenError
	if errors.As(err, &taken) {
		other := Label(strings.TrimPrefix(taken.Other, fieldPrefix))
		if taken.derived != "" {
			return fmt.Sprintf("%s, %s, is already the shortcut for %s.", Display(taken.Shortcut), taken.derived, other)
		}
		return fmt.Sprintf("%s is already the shortcut for %s.", Display(taken.Shortcut), other)
	}
	var fe *FieldError
	if errors.As(err, &fe) {
		err = fe.Err
	}
	text := strings.TrimSpace(err.Error())
	if text != "" && !strings.HasSuffix(text, ".") {
		text += "."
	}
	if text != "" {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	return text
}
