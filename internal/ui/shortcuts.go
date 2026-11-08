//go:build darwin || server

package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/workspace"
)

// suggestTries is how many suggestions to try for a shortcut that doesn't
// work before giving up on finding one that would.
const suggestTries = 8

// The buttons of the notices about shortcuts: Use this instead takes the
// suggestions (Engine.UseSuggestedShortcuts, with the fields to change), and
// Keyboard settings opens Settings at its Keyboard section.
var actionKeyboardSettings = workspace.NoticeAction{Label: "Keyboard settings", Command: "keyboard-settings"}

func actionUseSuggestions(fields []string) workspace.NoticeAction {
	return workspace.NoticeAction{Label: "Use this instead", Method: "UseSuggestedShortcuts", Arg: strings.Join(fields, " ")}
}

// registrar registers global shortcuts. Wails' app.GlobalShortcut satisfies
// it in the app, and previewRegistrar in the browser preview.
type registrar interface {
	Register(accelerator string, callback func()) error
	Unregister(accelerator string) error
}

// previewRegistrar is the browser preview's registrar. The preview has no
// global shortcuts, as the frontend handles keys itself, so every shortcut
// is free and none runs anything.
type previewRegistrar struct{}

func (previewRegistrar) Register(string, func()) error { return nil }
func (previewRegistrar) Unregister(string) error       { return nil }

// shortcutSet keeps the global shortcuts registered for the config's
// [shortcuts] table, registering them again when the table changes, as when
// config.toml is reloaded or Settings ▸ Keyboard is saved, or when the keys
// that macOS keeps for itself may have. Each time, it tells the engine which
// work, and why the others don't (Engine.ReportShortcuts).
type shortcutSet struct {
	reg registrar
	e   engine
	// system returns the keys that macOS keeps for itself
	// (Platform.SystemKeys); nil means none.
	system func() platform.SystemKeys
	// mu keeps registrations in turn, and guards held, the accelerators
	// registered, and paused.
	mu   sync.Mutex
	held []string
	// paused is true while Settings ▸ Keyboard records a shortcut: the
	// shortcuts are released, so that the keys reach it.
	paused bool
}

// register releases the shortcuts registered before, if any, registers the
// config's, and reports which work. While paused, it waits until the pause
// ends.
func (s *shortcutSet) register() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused {
		return
	}
	s.registerLocked()
}

// registerLocked registers the config's shortcuts in place of those held,
// each running its action through the engine, and reports which work.
func (s *shortcutSet) registerLocked() {
	s.releaseLocked()
	cfg := s.e.ConfigCopy().Shortcuts
	bindings, errs := shortcuts.Bindings(cfg)
	var failed []shortcuts.Binding
	for _, b := range bindings {
		run := func() { s.e.Shortcut(b.Action, b.Arg) }
		if err := s.reg.Register(b.Accelerator, run); err != nil {
			failed = append(failed, b)
			continue
		}
		s.held = append(s.held, b.Accelerator)
	}
	s.e.ReportShortcuts(assess(cfg, bindings, errs, failed, s.systemKeys(), s.e.WindowToolKeys(), s.freeLocked()))
}

// releaseLocked releases the shortcuts held.
func (s *shortcutSet) releaseLocked() {
	for _, a := range s.held {
		_ = s.reg.Unregister(a)
	}
	s.held = nil
}

// pause releases the shortcuts, or, with on false, registers them again.
func (s *shortcutSet) pause(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on == s.paused {
		return
	}
	s.paused = on
	if on {
		s.releaseLocked()
		return
	}
	s.registerLocked()
}

// check says whether each field of a [shortcuts] table, by the fields'
// names in config.toml, would work, for Settings ▸ Keyboard as the user
// records shortcuts. It keeps nothing registered: it tries each shortcut
// that Bench doesn't hold already, and releases it again.
func (s *shortcutSet) check(fields map[string]string) []workspace.ShortcutView {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := shortcuts.FromFields(fields)
	bindings, errs := shortcuts.Bindings(cfg)
	free := s.freeLocked()
	var failed []shortcuts.Binding
	for _, b := range bindings {
		if !free([]string{b.Accelerator}) {
			failed = append(failed, b)
		}
	}
	return assess(cfg, bindings, errs, failed, s.systemKeys(), s.e.WindowToolKeys(), free).Fields
}

// freeLocked returns a function that reports whether every one of a set of
// accelerators can be registered (probe), taking those that Bench holds as
// free, since it would release them to register them again.
func (s *shortcutSet) freeLocked() func(accelerators []string) bool {
	try := probe(s.reg)
	held := slices.Clone(s.held)
	return func(accelerators []string) bool {
		var rest []string
		for _, a := range accelerators {
			if !slices.Contains(held, a) {
				rest = append(rest, a)
			}
		}
		return try(rest)
	}
}

// systemKeys returns the keys that macOS keeps for itself.
func (s *shortcutSet) systemKeys() platform.SystemKeys {
	if s.system == nil {
		return platform.SystemKeys{}
	}
	return s.system()
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

// problem is why a field's shortcuts don't work, or may not, in the order
// assess looks for them: a field has the first that applies.
type problem int

const (
	problemNone        problem = iota
	problemField               // The field has a problem in config.toml.
	problemTaken               // Bench couldn't register some of them.
	problemInputSource         // macOS uses one to switch input sources.
	problemVoiceOver           // VoiceOver is on, and they have ⌃⌥.
	problemReserved            // macOS usually keeps one for itself.
	problemAppKey              // Every Mac app uses one, such as ⌘Z.
	problemTool                // A window tool that's running uses one too.
)

// fieldCheck is what assess found about one field.
type fieldCheck struct {
	view    workspace.ShortcutView
	problem problem
	// err is the field's problem in config.toml, for problemField, and
	// group the shortcuts with the problem, for the others.
	err   error
	group []shortcuts.Binding
}

// assess works out, for each field of the [shortcuts] table, whether its
// shortcuts work, and if not why not, with a suggestion of another value
// that would; and the notices about those that don't. bindings and errs
// are the table's (shortcuts.Bindings), failed the bindings that couldn't be
// registered, sys the keys that macOS keeps for itself, tools the
// shortcuts of the window tools that are running, as far as Bench could
// read them (Engine.WindowToolKeys), and free reports whether shortcuts
// can be registered.
//
// Each suggestion makes no new problem in the table with the suggestions
// before it, so that a notice's Use this instead can take them all: a
// suggestion is never another field's shortcut, one that couldn't be
// registered or isn't free, one that macOS or (while it's on) VoiceOver
// uses, one that macOS usually keeps for itself, one that every Mac app
// uses, such as ⌘Z, or one that a window tool uses.
//
// A window tool's shortcut registers all the same, as neither app keeps
// its keys to itself, so macOS gives no clash (decision 69): the field
// says so, but has no notice of its own, as the tool's notice names them.
func assess(cfg shortcuts.Config, bindings []shortcuts.Binding, errs []error, failed []shortcuts.Binding,
	sys platform.SystemKeys, tools []workspace.ToolKeys, free func([]string) bool) workspace.ShortcutReport {
	values := cfg.Fields()
	working := cfg.Fields()
	defaults := config.DefaultShortcuts().Fields()
	fieldErrs := map[string]error{}
	for _, err := range errs {
		name := errField(err)
		if _, ok := fieldErrs[name]; !ok {
			fieldErrs[name] = err
		}
	}
	avoid := func(b shortcuts.Binding) bool {
		return slices.ContainsFunc(failed, func(f shortcuts.Binding) bool { return f.Shortcut == b.Shortcut }) ||
			slices.Contains(sys.InputSources, b.Shortcut) ||
			(sys.VoiceOver && shortcuts.UsesVoiceOverKeys(b.Shortcut)) ||
			shortcuts.Reserved(b.Shortcut) || shortcuts.AppKey(b.Shortcut) ||
			slices.ContainsFunc(tools, func(t workspace.ToolKeys) bool { return slices.Contains(t.Keys, b.Shortcut) })
	}

	var checks []fieldCheck
	for _, name := range shortcuts.FieldNames() {
		c := fieldCheck{view: workspace.ShortcutView{
			Field: name,
			Label: shortcuts.Label(name),
			Keys:  shortcuts.AddedKeys(name),
			Value: values[name],
		}}
		if c.view.Value != "" {
			c.view.Display = shortcuts.Display(c.view.Value)
		}
		mine := bindingsOf(bindings, name)
		if err, ok := fieldErrs[name]; ok {
			c.problem, c.err = problemField, err
			c.view.Problem = shortcuts.Describe(err)
		} else if g := filter(mine, func(b shortcuts.Binding) bool {
			return slices.ContainsFunc(failed, func(f shortcuts.Binding) bool { return f.Accelerator == b.Accelerator })
		}); len(g) > 0 {
			c.problem, c.group = problemTaken, g
			c.view.Problem = fmt.Sprintf("Bench couldn't register %s: %s.", displays(g), reason(g))
		} else if g := filter(mine, func(b shortcuts.Binding) bool { return slices.Contains(sys.InputSources, b.Shortcut) }); len(g) > 0 {
			c.problem, c.group = problemInputSource, g
			c.view.Problem = fmt.Sprintf("macOS uses %s to switch input sources, so Bench won't see %s.", displays(g), pronoun(g))
		} else if g := filter(mine, func(b shortcuts.Binding) bool {
			return sys.VoiceOver && shortcuts.UsesVoiceOverKeys(b.Shortcut)
		}); len(g) > 0 {
			c.problem, c.group = problemVoiceOver, g
			c.view.Problem = fmt.Sprintf("VoiceOver is on, and it uses ⌃⌥ for its own commands, so Bench may not see %s.", pronoun(g))
		} else if g := filter(mine, func(b shortcuts.Binding) bool { return shortcuts.Reserved(b.Shortcut) }); len(g) > 0 {
			c.problem, c.group = problemReserved, g
			c.view.Problem = fmt.Sprintf("macOS usually keeps %s for itself, so Bench may not see %s.", displays(g), pronoun(g))
		} else if g := filter(mine, func(b shortcuts.Binding) bool { return shortcuts.AppKey(b.Shortcut) }); len(g) > 0 {
			// RegisterEventHotKey takes it all the same, from every app.
			c.problem, c.group = problemAppKey, g
			c.view.Problem = appKeyText(g)
		} else if tool, g := toolGroup(mine, tools); len(g) > 0 {
			c.problem, c.group = problemTool, g
			c.view.Problem = toolKeyText(tool, g)
		}
		if c.problem != problemNone {
			if value, ok := suggest(name, c.problem, working, defaults[name], avoid, free); ok {
				c.view.Suggestion = value
				c.view.SuggestionDisplay = shortcuts.Display(value)
				working[name] = value
			}
		}
		checks = append(checks, c)
	}
	return workspace.ShortcutReport{Fields: views(checks), Notices: shortcutNotices(checks)}
}

// suggest returns another value for a field of the table, working, that
// gives the field no problem and the table no new one: none of the field's
// shortcuts is one to avoid, and they're all free. It tries Suggest's
// suggestions, starting, for a clash with VoiceOver, with Command in place
// of Option, then with Command in place of Control, as for
// send_to_tab_modifiers, whose ⌃⇧⌘3 macOS keeps; and then the field's
// default and its suggestions.
func suggest(name string, p problem, working map[string]string, def string,
	avoid func(shortcuts.Binding) bool, free func([]string) bool) (string, bool) {
	value := working[name]
	_, baseErrs := shortcuts.Bindings(shortcuts.FromFields(working))
	others := len(baseErrs) - countField(baseErrs, name)
	starts := []string{shortcuts.Suggest(value), def}
	if p == problemVoiceOver {
		starts = []string{shortcuts.WithoutVoiceOverKeys(value), shortcuts.WithoutVoiceOverControl(value), def}
	}
	tried := map[string]bool{value: true}
	for _, first := range starts {
		candidate := first
		for range suggestTries {
			if candidate == "" {
				break
			}
			if !tried[candidate] {
				tried[candidate] = true
				if fits(name, candidate, working, others, avoid, free) {
					return candidate, true
				}
			}
			candidate = shortcuts.Suggest(candidate)
		}
	}
	return "", false
}

// fits reports whether a field could take a value: it makes the field no
// problem, and the other fields no more than they have (others), and its
// shortcuts are free and none to avoid.
func fits(name, value string, working map[string]string, others int,
	avoid func(shortcuts.Binding) bool, free func([]string) bool) bool {
	fields := make(map[string]string, len(working))
	for k, v := range working {
		fields[k] = v
	}
	fields[name] = value
	bindings, errs := shortcuts.Bindings(shortcuts.FromFields(fields))
	if countField(errs, name) > 0 || len(errs) > others {
		return false
	}
	mine := bindingsOf(bindings, name)
	if len(mine) == 0 || slices.ContainsFunc(mine, avoid) {
		return false
	}
	return free(accelerators(mine))
}

// shortcutNotices returns the notices about the fields that don't work, or
// may not: one for each field with a problem in config.toml, then one for
// each field with shortcuts that couldn't be registered, that switch input
// sources, that macOS usually keeps for itself, or that every Mac app uses,
// and one for those that VoiceOver uses. Each names the problem and the suggestion, and has a
// button to use it, and one to open Settings ▸ Keyboard.
func shortcutNotices(checks []fieldCheck) []workspace.NoticeView {
	var notices []workspace.NoticeView
	add := func(text string, fields []string) {
		actions := []workspace.NoticeAction{actionKeyboardSettings}
		if len(fields) > 0 {
			actions = append([]workspace.NoticeAction{actionUseSuggestions(fields)}, actions...)
		}
		notices = append(notices, workspace.NoticeView{Text: text, Actions: actions})
	}
	for _, kind := range []problem{problemField, problemTaken, problemInputSource, problemReserved, problemAppKey} {
		for _, c := range checks {
			if c.problem != kind {
				continue
			}
			var text string
			switch kind {
			case problemField:
				text = fieldErrorNotice(c.err)
			case problemTaken:
				text = fmt.Sprintf("Couldn't register %s (%s).", displays(c.group), reason(c.group))
			case problemInputSource:
				text = fmt.Sprintf("macOS uses %s to switch input sources, so Bench won't see %s.", displays(c.group), pronoun(c.group))
			case problemReserved:
				text = fmt.Sprintf("macOS usually keeps %s for itself, so Bench may not see %s.", displays(c.group), pronoun(c.group))
			case problemAppKey:
				text = appKeyText(c.group)
			}
			var fields []string
			if try := tryPhrase(c.view); try != "" {
				text += " Try " + try + " instead."
				fields = []string{c.view.Field}
			}
			add(text, fields)
		}
	}
	if text, fields := voiceOverNotice(checks); text != "" {
		add(text, fields)
	}
	return notices
}

// toolGroup returns the first window tool, of tools, that uses any of a
// field's bindings, mine, and those bindings; or none.
func toolGroup(mine []shortcuts.Binding, tools []workspace.ToolKeys) (string, []shortcuts.Binding) {
	for _, t := range tools {
		if g := filter(mine, func(b shortcuts.Binding) bool { return slices.Contains(t.Keys, b.Shortcut) }); len(g) > 0 {
			return t.Name, g
		}
	}
	return "", nil
}

// toolKeyText says that a window tool uses some of a field's shortcuts too,
// such as "Rectangle uses ⌃⌥← and ⌃⌥→ too. Both apps may act on these
// keys."
func toolKeyText(tool string, group []shortcuts.Binding) string {
	keys := "this key"
	if len(group) > 1 {
		keys = "these keys"
	}
	return fmt.Sprintf("%s uses %s too. Both apps may act on %s.", tool, displays(group), keys)
}

// appKeyText says why shortcuts that every Mac app uses, such as ⌘Z, don't
// suit Bench.
func appKeyText(group []shortcuts.Binding) string {
	return fmt.Sprintf("Mac apps use %s for their own commands, so Bench would take %s from every app.", displays(group), pronoun(group))
}

// voiceOverNotice returns the notice about the fields whose shortcuts
// VoiceOver uses, if any, and those with suggestions.
func voiceOverNotice(checks []fieldCheck) (string, []string) {
	var fields, others []string
	clash := false
	for _, c := range checks {
		if c.problem != problemVoiceOver {
			continue
		}
		clash = true
		if c.view.Suggestion == "" {
			continue
		}
		fields = append(fields, c.view.Field)
		if c.view.Suggestion != shortcuts.WithoutVoiceOverKeys(c.view.Value) {
			// Without the keys Bench adds, as "⌃⌘ in place of ⌃⌥" is:
			// "⌥⇧⌘ for Send a window to a tab".
			others = append(others, c.view.SuggestionDisplay+" for "+c.view.Label)
		}
	}
	if !clash {
		return "", nil
	}
	text := "VoiceOver is on, and it uses ⌃⌥ for its own commands, so Bench's shortcuts with ⌃⌥ may not work."
	switch {
	case len(fields) == 0:
	case len(others) == len(fields):
		text += " Try " + listAnd(others) + " instead."
	case len(others) > 0:
		text += " Try ⌃⌘ in place of ⌃⌥ (and " + listAnd(others) + ")."
	default:
		text += " Try ⌃⌘ in place of ⌃⌥."
	}
	return text, fields
}

// tryPhrase names a row's suggestion, with the keys Bench adds to it, such
// as "⌃⌥J" or "⌃⌥⌘ with the arrow keys", or returns "" without one.
func tryPhrase(v workspace.ShortcutView) string {
	if v.Suggestion == "" {
		return ""
	}
	if v.Keys != "" {
		return v.SuggestionDisplay + " with " + v.Keys
	}
	return v.SuggestionDisplay
}

// views returns the rows of Settings ▸ Keyboard that checks found.
func views(checks []fieldCheck) []workspace.ShortcutView {
	out := make([]workspace.ShortcutView, len(checks))
	for i, c := range checks {
		out[i] = c.view
	}
	return out
}

// fieldErrorNotice explains a problem with a field of the [shortcuts] table.
func fieldErrorNotice(err error) string {
	var fe *shortcuts.FieldError
	if errors.As(err, &fe) {
		return sentence(fmt.Sprintf("Couldn't use %s in config.toml: %v", fe.Field, fe.Err))
	}
	return sentence(err.Error())
}

// errField returns the name of the field that a problem with the table is
// about, such as "jump", or "" if it isn't about one.
func errField(err error) string {
	var fe *shortcuts.FieldError
	if errors.As(err, &fe) {
		return fieldName(fe.Field)
	}
	return ""
}

// countField returns how many of errs are about a field.
func countField(errs []error, name string) int {
	n := 0
	for _, err := range errs {
		if errField(err) == name {
			n++
		}
	}
	return n
}

// bindingsOf returns the bindings of a field, by its name in config.toml.
func bindingsOf(bindings []shortcuts.Binding, name string) []shortcuts.Binding {
	return filter(bindings, func(b shortcuts.Binding) bool { return fieldName(b.Field) == name })
}

// filter returns the bindings that keep reports true for.
func filter(bindings []shortcuts.Binding, keep func(shortcuts.Binding) bool) []shortcuts.Binding {
	var out []shortcuts.Binding
	for _, b := range bindings {
		if keep(b) {
			out = append(out, b)
		}
	}
	return out
}

// accelerators returns the bindings' accelerators.
func accelerators(bindings []shortcuts.Binding) []string {
	out := make([]string, len(bindings))
	for i, b := range bindings {
		out[i] = b.Accelerator
	}
	return out
}

// displays names the bindings as macOS shows them, such as "⌃⌥1 and ⌃⌥2".
func displays(group []shortcuts.Binding) string {
	names := make([]string, len(group))
	for i, b := range group {
		names[i] = b.Display
	}
	return listAnd(names)
}

// pronoun returns "it" for one binding, or "them".
func pronoun(group []shortcuts.Binding) string {
	if len(group) > 1 {
		return "them"
	}
	return "it"
}

// reason says why shortcuts might be taken.
func reason(group []shortcuts.Binding) string {
	key, mods := splitShortcut(group[0].Shortcut)
	switch {
	case key == "space" && slices.Contains(mods, "ctrl"):
		return "macOS uses " + pronoun(group) + " to switch input sources"
	case isArrow(key) && slices.Contains(mods, "ctrl") && slices.Contains(mods, "alt"):
		return "window tools such as Rectangle often use " + pronoun(group)
	default:
		return "another app might be using " + pronoun(group)
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
