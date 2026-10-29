//go:build darwin || server

package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/itbm/bench/internal/shortcuts"
)

// noticeSource is the source of the notices about global shortcuts, for
// Engine.SetNotices.
const noticeSource = "shortcuts"

// suggestTries is how many suggestions to try for a shortcut that couldn't
// be registered before giving up on finding one that's free.
const suggestTries = 8

// registrar registers global shortcuts. Wails' app.GlobalShortcut satisfies
// it.
type registrar interface {
	Register(accelerator string, callback func()) error
	Unregister(accelerator string) error
}

// registerShortcuts registers a global shortcut for each binding of the
// config's [shortcuts] table, each running its action through the engine,
// and reports the ones that fail, and problems with the table, as notices.
// It returns the accelerators it registered.
func registerShortcuts(reg registrar, e engine) []string {
	cfg := e.ConfigCopy().Shortcuts
	bindings, errs := shortcuts.Bindings(cfg)
	var failed []shortcuts.Binding
	var held []string
	for _, b := range bindings {
		run := func() { e.Shortcut(b.Action, b.Arg) }
		if err := reg.Register(b.Accelerator, run); err != nil {
			failed = append(failed, b)
			continue
		}
		held = append(held, b.Accelerator)
	}
	e.SetNotices(noticeSource, shortcutNotices(cfg, failed, errs, probe(reg)))
	return held
}

// shortcutSet keeps the global shortcuts registered for the config's
// [shortcuts] table, registering them again when the table changes, as when
// config.toml is reloaded.
type shortcutSet struct {
	reg registrar
	e   engine
	// mu keeps registrations in turn, and guards held, the accelerators
	// registered.
	mu   sync.Mutex
	held []string
}

// register releases the shortcuts registered before, if any, and registers
// the config's.
func (s *shortcutSet) register() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.held {
		_ = s.reg.Unregister(a)
	}
	s.held = registerShortcuts(s.reg, s.e)
}

// probe returns a function that reports whether every one of a set of
// accelerators can be registered, by registering each and then releasing
// them again.
func probe(reg registrar) func(accelerators []string) bool {
	return func(accelerators []string) bool {
		var held []string
		defer func() {
			for _, a := range held {
				_ = reg.Unregister(a)
			}
		}()
		for _, a := range accelerators {
			if err := reg.Register(a, func() {}); err != nil {
				return false
			}
			held = append(held, a)
		}
		return true
	}
}

// shortcutNotices returns a sentence for each problem with the [shortcuts]
// table, then one for each field with shortcuts that couldn't be registered,
// naming them in display form and suggesting others that free reports can
// be registered.
func shortcutNotices(cfg shortcuts.Config, failed []shortcuts.Binding, errs []error, free func([]string) bool) []string {
	var notices []string
	for _, err := range errs {
		notices = append(notices, fieldErrorNotice(err))
	}
	for _, group := range groupByField(failed) {
		notices = append(notices, failedNotice(cfg, group, failed, free))
	}
	return notices
}

// fieldErrorNotice explains a problem with a field of the [shortcuts] table.
func fieldErrorNotice(err error) string {
	var fe *shortcuts.FieldError
	if errors.As(err, &fe) {
		return sentence(fmt.Sprintf("Couldn't use %s in config.toml: %v", fe.Field, fe.Err))
	}
	return sentence(err.Error())
}

// groupByField splits bindings by their config field, keeping the order in
// which fields first appear.
func groupByField(bindings []shortcuts.Binding) [][]shortcuts.Binding {
	var groups [][]shortcuts.Binding
	index := map[string]int{}
	for _, b := range bindings {
		i, ok := index[b.Field]
		if !ok {
			i = len(groups)
			index[b.Field] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], b)
	}
	return groups
}

// failedNotice explains that a field's shortcuts couldn't be registered, and
// suggests another value for the field if one is free. failed lists every
// shortcut that couldn't be registered, so as not to suggest one of them.
func failedNotice(cfg shortcuts.Config, group, failed []shortcuts.Binding, free func([]string) bool) string {
	displays := make([]string, len(group))
	for i, b := range group {
		displays[i] = b.Display
	}
	text := fmt.Sprintf("Couldn't register %s (%s).", listAnd(displays), reason(group))
	if try, ok := suggestion(cfg, group, failed, free); ok {
		text += fmt.Sprintf(" Try %s: set %s in config.toml.", try, fieldName(group[0].Field))
	}
	return text
}

// reason says why shortcuts might be taken.
func reason(group []shortcuts.Binding) string {
	pronoun := "it"
	if len(group) > 1 {
		pronoun = "them"
	}
	key, mods := splitShortcut(group[0].Shortcut)
	switch {
	case key == "space" && slices.Contains(mods, "ctrl"):
		return "macOS uses " + pronoun + " to switch input sources"
	case isArrow(key) && slices.Contains(mods, "ctrl") && slices.Contains(mods, "alt"):
		return "window tools such as Rectangle often use " + pronoun
	default:
		return "another app might be using " + pronoun
	}
}

// suggestion returns another value to try for the group's field, in display
// form, if there's one that makes no new problems in the table, includes
// none of the shortcuts that failed, and can be registered (free). For
// fields that hold modifiers, it's the
// modifiers and the keys they go with, such as "⌃⌥⌘ with the arrow keys".
// For next_window, it's the next window's shortcut; Bench adds Shift for the
// previous window.
func suggestion(cfg shortcuts.Config, group, failed []shortcuts.Binding, free func([]string) bool) (string, bool) {
	first := group[0]
	name := fieldName(first.Field)
	fields := cfg.Fields()
	_, baseErrs := shortcuts.Bindings(cfg)
	value := fields[name]
	for range suggestTries {
		value = shortcuts.Suggest(value)
		if value == "" {
			return "", false
		}
		fields[name] = value
		bindings, errs := shortcuts.Bindings(shortcuts.FromFields(fields))
		if len(errs) > len(baseErrs) || failedAgain(bindings, first.Field, failed) {
			continue
		}
		if !free(accelerators(bindings, first.Field)) {
			continue
		}
		if keys := keysPhrase(first.Action); keys != "" {
			return shortcuts.Display(value) + " " + keys, true
		}
		return shortcuts.Display(value), true
	}
	return "", false
}

// failedAgain reports whether any of field's bindings is a shortcut that
// couldn't be registered.
func failedAgain(bindings []shortcuts.Binding, field string, failed []shortcuts.Binding) bool {
	for _, b := range bindings {
		if b.Field != field {
			continue
		}
		for _, f := range failed {
			if b.Shortcut == f.Shortcut {
				return true
			}
		}
	}
	return false
}

// accelerators returns the accelerators of field's bindings.
func accelerators(bindings []shortcuts.Binding, field string) []string {
	var out []string
	for _, b := range bindings {
		if b.Field == field {
			out = append(out, b.Accelerator)
		}
	}
	return out
}

// keysPhrase names the keys Bench adds to a field of modifiers, for the
// field's action, or returns "" for fields that hold whole shortcuts.
func keysPhrase(action string) string {
	switch action {
	case shortcuts.ActionTab:
		return "with 1 to 9"
	case shortcuts.ActionFocusPane, shortcuts.ActionMoveWindow:
		return "with the arrow keys"
	default:
		return ""
	}
}

// fieldName returns a binding's field without the table, such as "jump" for
// "shortcuts.jump".
func fieldName(field string) string {
	return strings.TrimPrefix(field, "shortcuts.")
}

// splitShortcut splits a canonical shortcut, such as "ctrl+alt+space", into
// its key and its modifiers. "+" isn't a key, so splitting on it is safe.
func splitShortcut(s string) (key string, mods []string) {
	parts := strings.Split(s, "+")
	return parts[len(parts)-1], parts[:len(parts)-1]
}

// isArrow reports whether key is one of the arrow keys.
func isArrow(key string) bool {
	switch key {
	case "left", "right", "up", "down":
		return true
	}
	return false
}

// listAnd joins words as "a", "a and b", or "a, b, and c".
func listAnd(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}

// sentence ends text with a full stop if it doesn't end with one.
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasSuffix(text, ".") {
		return text
	}
	return text + "."
}
