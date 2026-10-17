package shortcuts

import (
	"fmt"
	"strings"
)

// tabKeys are the keys added to tab_modifiers, and the bindings' Args.
var tabKeys = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}

// arrowKeys are the keys added to focus_pane_modifiers and
// move_window_modifiers, and the bindings' Args.
var arrowKeys = []string{"left", "right", "up", "down"}

// Bindings expands the [shortcuts] config into every binding: tabs 1 to 9,
// next and previous tab, jump, rearrange, focus and move in four directions,
// and next and previous window (next_window, and next_window with Shift
// added). It returns the bindings that parse and an error for each that
// doesn't, naming the field: each error is a *FieldError.
//
// An empty field turns its bindings off without an error. When two fields
// make the same shortcut, the first in that order keeps it and the other
// gets an error, because the same global shortcut can't run two actions.
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
		what := fmt.Sprintf("%q", shortcut)
		if derived != "" {
			what += " (" + derived + ")"
		}
		b.fail(field, fmt.Errorf("%s is already the shortcut for %s%s", what, fieldPrefix, other))
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
