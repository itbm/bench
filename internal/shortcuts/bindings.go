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

// lastTabKey is the key added to send_to_tab_modifiers, after 1 to 9, for
// the tab used before this one: the key left of 1 on a US keyboard, as in
// last_tab's default.
const lastTabKey = "`"

// AddedKeys names the keys that Bench adds to a field that holds modifiers:
// "1 to 9" for tab_modifiers, "the arrow keys" for focus_pane_modifiers
// and move_window_modifiers, and "1 to 9, and ` for the last used tab" for
// send_to_tab_modifiers, which says what ` does, as Settings ▸ Keyboard
// says "and ⇧ for the previous" for next_window. It returns "" for the
// fields that hold whole shortcuts.
func AddedKeys(field string) string {
	switch field {
	case "tab_modifiers":
		return "1 to 9"
	case "focus_pane_modifiers", "move_window_modifiers":
		return "the arrow keys"
	case "send_to_tab_modifiers":
		return "1 to 9, and " + lastTabKey + " for the last used tab"
	}
	return ""
}

// Bindings expands the [shortcuts] config into every binding: tabs 1 to 9,
// next and previous tab, jump, show all tabs, rearrange, focus and move in four directions,
// next and previous window (next_window, and next_window with Shift added),
// zoom pane, the keys that arrange the panes (grow and shrink pane, equal
// panes, and next and previous layout), the last used tab, adding the front
// window to the focused pane, undo, and sending the shown window to tabs 1
// to 9 and the last used tab. It returns the bindings that parse and an
// error for each that doesn't, naming the field: each error is a
// *FieldError.
//
// An empty field turns its bindings off without an error. When two fields
// make the same shortcut, the first in that order keeps it and the other
// gets an error, because the same global shortcut can't run two actions.
// show_all_tabs and the keys that arrange the panes are off by default, so
// a config written before they existed can't clash with them, and they keep
// their place in the table's order. last_tab comes after them, though the
// table lists it with the tabs: a config written before it existed gets its
// default, which mustn't take a shortcut that the config gives another
// field, such as "ctrl+alt+`", which Suggest offers for next_window.
// config.Parse turns it off then. add_front_window comes after it: it's off
// by default, and newer still, and undo after that, for the same reason.
// send_to_tab_modifiers comes last, though the table lists it with
// move_window_modifiers, for the same reason as last_tab: config.Parse
// turns it off in a config that leaves it out and gives one of its
// shortcuts to another field, even add_front_window or undo.
func Bindings(s Config) ([]Binding, []error) {
	b := &builder{owner: make(map[string]string)}
	b.withKeys("tab_modifiers", s.TabModifiers, ActionTab, tabKeys)
	b.single("next_tab", s.NextTab, ActionNextTab)
	b.single("previous_tab", s.PreviousTab, ActionPreviousTab)
	b.single("jump", s.Jump, ActionJump)
	b.single("show_all_tabs", s.ShowAllTabs, ActionShowAllTabs)
	b.single("rearrange", s.Rearrange, ActionRearrange)
	b.withKeys("focus_pane_modifiers", s.FocusPaneModifiers, ActionFocusPane, arrowKeys)
	b.withKeys("move_window_modifiers", s.MoveWindowModifiers, ActionMoveWindow, arrowKeys)
	b.nextWindow("next_window", s.NextWindow)
	b.single("zoom_pane", s.ZoomPane, ActionZoomPane)
	b.single("grow_pane", s.GrowPane, ActionGrowPane)
	b.single("shrink_pane", s.ShrinkPane, ActionShrinkPane)
	b.single("equal_panes", s.EqualPanes, ActionEqualPanes)
	b.single("next_layout", s.NextLayout, ActionNextLayout)
	b.single("previous_layout", s.PreviousLayout, ActionPreviousLayout)
	b.single("last_tab", s.LastTab, ActionLastTab)
	b.single("add_front_window", s.AddFrontWindow, ActionAddFrontWindow)
	b.single("undo", s.Undo, ActionUndo)
	b.sendToTab("send_to_tab_modifiers", s.SendToTabModifiers)
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

// sendToTab binds send_to_tab_modifiers: with 1 to 9 to send the shown
// window to that tab, with the tab number as the Arg, and with ` to send it
// to the tab used before this one.
func (b *builder) sendToTab(field, value string) {
	p, ok := b.parseField(field, value, true)
	if !ok {
		return
	}
	for _, k := range tabKeys {
		b.add(field, p.with(0, k), ActionSendToTab, k, "")
	}
	b.add(field, p.with(0, lastTabKey), ActionSendToLastTab, "", "")
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
