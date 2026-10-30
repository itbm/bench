package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/shortcuts"
)

// This file has Settings ▸ Keyboard's side of the engine: what the shell
// reports as it registers the global shortcuts, and changing the
// [shortcuts] table. The shell registers them, and works out which work
// (internal/ui); the engine keeps the config, saves it, and tells the shell
// to register them again (Emitter.ShortcutsChanged).

// sourceShortcuts is the source of the notices about global shortcuts,
// which the shell reports (ReportShortcuts).
const sourceShortcuts = "shortcuts"

// ReportShortcuts records what the shell found as it registered the global
// shortcuts: whether each field's shortcuts work, for Settings ▸ Keyboard,
// and the notices about those that don't.
func (e *Engine) ReportShortcuts(r ShortcutReport) {
	e.lock()
	defer e.unlock()
	e.keyStates = make(map[string]ShortcutView, len(r.Fields))
	for _, f := range r.Fields {
		e.keyStates[f.Field] = f
	}
	e.setNoticeViewsLocked(sourceShortcuts, r.Notices)
	e.touch()
}

// shortcutViewsLocked returns the [shortcuts] table's fields, in config
// order, for Settings ▸ Keyboard. A field has what the shell last reported
// about it, if that was for its value; a field whose value has changed
// since, as the shell registers it again, is taken to work until the shell
// says otherwise.
func (e *Engine) shortcutViewsLocked() []ShortcutView {
	values := e.cfg.Shortcuts.Fields()
	defaults := config.DefaultShortcuts().Fields()
	var out []ShortcutView
	for _, name := range shortcuts.FieldNames() {
		v := ShortcutView{Field: name, Value: values[name]}
		if known, ok := e.keyStates[name]; ok && known.Value == v.Value {
			v = known
		}
		out = append(out, describeShortcut(v, defaults))
	}
	return out
}

// describeShortcut fills in what a row of Settings ▸ Keyboard shows about
// its field whatever the shell says: what it does, the keys Bench adds, how
// macOS shows its value, and its default.
func describeShortcut(v ShortcutView, defaults map[string]string) ShortcutView {
	v.Label = shortcuts.Label(v.Field)
	v.Keys = shortcuts.AddedKeys(v.Field)
	v.Default = defaults[v.Field]
	v.Display = ""
	if v.Value != "" {
		v.Display = shortcuts.Display(v.Value)
	}
	return v
}

// CheckShortcuts says whether each field of the [shortcuts] table would
// work with Settings ▸ Keyboard's changes, by the fields' names in
// config.toml, as the user records shortcuts: whether the shell could
// register them, whether macOS or VoiceOver uses them, or what's wrong with
// the table, with a suggestion for each that wouldn't work. It saves
// nothing, and refuses a name that isn't a field.
func (e *Engine) CheckShortcuts(changes map[string]string) ([]ShortcutView, error) {
	e.lock()
	values := e.cfg.Shortcuts.Fields()
	e.unlock()
	for name, value := range changes {
		if _, ok := values[name]; !ok {
			return nil, fmt.Errorf("Bench has no shortcut called %s.", name)
		}
		values[name] = strings.TrimSpace(value)
	}
	checked := map[string]ShortcutView{}
	for _, v := range e.em.CheckShortcuts(values) {
		checked[v.Field] = v
	}
	defaults := config.DefaultShortcuts().Fields()
	var out []ShortcutView
	for _, name := range shortcuts.FieldNames() {
		v := ShortcutView{Field: name, Value: values[name]}
		if known, ok := checked[name]; ok && known.Value == v.Value {
			v = known
		}
		out = append(out, describeShortcut(v, defaults))
	}
	return out, nil
}

// PauseShortcuts releases Bench's global shortcuts while Settings ▸
// Keyboard records one, so that the keys the user presses reach it rather
// than run Bench's actions, and, with paused false, registers them again.
// Closing the last menu or dialog (SetOverlay), or loading the page again
// (Ready), registers them again too, in case the page didn't.
func (e *Engine) PauseShortcuts(paused bool) error {
	e.lock()
	defer e.unlock()
	e.pauseShortcutsLocked(paused)
	return nil
}

// pauseShortcutsLocked tells the shell to release the global shortcuts or
// register them again, if they aren't so already.
func (e *Engine) pauseShortcutsLocked(paused bool) {
	if e.keysPaused.Swap(paused) == paused {
		return
	}
	e.later(func() { e.em.PauseShortcuts(e.keysPaused.Load()) })
}

// UseSuggestedShortcuts sets fields of the [shortcuts] table, by their
// names in config.toml, separated by spaces, such as "jump" or
// "tab_modifiers focus_pane_modifiers", to the shortcuts that the shell
// last suggested for them, as a notice's Use this instead does, and saves
// config.toml as Settings does. The shell registers the shortcuts again. A
// field without a suggestion for its value as it is now stays as it is; if
// no field has one, it says so. It refuses a change that would give the
// table a problem, such as a shortcut that another field has.
func (e *Engine) UseSuggestedShortcuts(fields string) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	values := e.cfg.Shortcuts.Fields()
	changes := map[string]string{}
	for _, name := range strings.Fields(fields) {
		if s, ok := e.keyStates[name]; ok && s.Suggestion != "" && s.Value == values[name] {
			changes[name] = s.Suggestion
		}
	}
	if len(changes) == 0 {
		e.unlock()
		return errors.New("Bench has no other shortcut to suggest now. Choose one in Settings ▸ Keyboard.")
	}
	cfg := e.cfg.Clone()
	if err := applyShortcuts(&cfg.Shortcuts, changes); err != nil {
		e.unlock()
		return err
	}
	e.cfg = cfg
	e.later(e.em.ShortcutsChanged)
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	if notSaved != "" {
		e.em.Toast(appliedText(notSaved))
		return nil
	}
	e.em.Toast(shortcutsToast(changes))
	return nil
}

// shortcutsToast says what UseSuggestedShortcuts changed: one field's new
// shortcut, or that it changed several.
func shortcutsToast(fields map[string]string) string {
	if len(fields) != 1 {
		return "Changed the shortcuts"
	}
	for name, value := range fields {
		if value == "" {
			return "Turned off the shortcut for " + shortcuts.Label(name)
		}
		if keys := shortcuts.AddedKeys(name); keys != "" {
			return fmt.Sprintf("Changed %s to %s with %s", shortcuts.Label(name), shortcuts.Display(value), keys)
		}
		return fmt.Sprintf("Changed %s to %s", shortcuts.Label(name), shortcuts.Display(value))
	}
	return ""
}

// applyShortcuts sets fields of the [shortcuts] table, by their names in
// config.toml, to shortcuts in any form that config.toml takes, or "" to
// turn them off, keeping the other fields. It refuses a name that isn't a
// field, and a change that gives the table a problem it didn't have, such
// as a shortcut that another field has, saying what the problem is. A
// problem that the table had already, in a field left as it was, stays.
func applyShortcuts(s *shortcuts.Config, changes map[string]string) error {
	if len(changes) == 0 {
		return nil
	}
	_, before := shortcuts.Bindings(*s)
	had := map[string]bool{}
	for _, err := range before {
		had[err.Error()] = true
	}
	values := s.Fields()
	for name, value := range changes {
		if _, ok := values[name]; !ok {
			return fmt.Errorf("Bench has no shortcut called %s.", name)
		}
		values[name] = strings.TrimSpace(value)
	}
	next := shortcuts.FromFields(values)
	_, errs := shortcuts.Bindings(next)
	for _, err := range errs {
		if !had[err.Error()] {
			return fmt.Errorf("Bench can't save that shortcut: %s", shortcuts.Describe(err))
		}
	}
	*s = next
	return nil
}
