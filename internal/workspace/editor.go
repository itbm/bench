package workspace

import (
	"fmt"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file has the methods of the Tab properties dialog, which edits a
// tab's name, layout, and small-screen setting, or adds a tab, and of the
// other changes to the tabs that save config.toml as it does: adding a tab
// for a window (Move to new tab), and deleting, moving, and resetting tabs.
// The settings that apply to every tab are the Settings dialog's
// (settings.go).

// PreviewTree returns the tree Tab properties would make for tab index
// (-1 for a new tab) with a preset, for its preview: the tab's tree with the
// preset applied, keeping its items in order. "custom" is the tab's own
// tree. The preview's new panes and splits get IDs that aren't kept.
func (e *Engine) PreviewTree(index int, preset string) (*layout.Node, error) {
	e.lock()
	defer e.unlock()
	var old *layout.Node
	var sizes []float64
	if index != -1 {
		t := e.model.Tab(index)
		if t == nil {
			return nil, errNoTab(index)
		}
		if preset == layout.PresetCustom {
			return layout.Clone(t.Root), nil
		}
		old = layout.Clone(t.Root)
		if preset == t.Preset && old.IsSplit() {
			sizes = old.Sizes
		}
	}
	root, err := layout.ApplyPreset(old, preset, sizes, e.previewIDsLocked())
	if err != nil {
		return nil, errNoPreset(preset)
	}
	return root, nil
}

// previewIDsLocked returns IDs for a preview's new panes and splits, which
// the model doesn't keep: they follow its last ID, as the panes and splits
// it makes next will.
func (e *Engine) previewIDsLocked() func(nodeType string) string {
	seq := e.model.Seq
	return func(nodeType string) string {
		seq++
		if nodeType == layout.TypeSplit {
			return fmt.Sprintf("%s%d", splitPrefix, seq)
		}
		return fmt.Sprintf("%s%d", panePrefix, seq)
	}
}

// SaveTab saves Tab properties: index -1 adds a tab and switches to it;
// otherwise it renames the tab, changes its small-screen setting, and
// applies a new preset, keeping the items in order. A name that another tab
// has is numbered (uniqueTabName). The draft's place, if it gives one, moves
// the tab there, as MoveTab does. It updates the config, and saves
// config.toml when it can.
func (e *Engine) SaveTab(index int, draft TabDraft) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	name := uniqueTabName(e.model.Tabs, strings.TrimSpace(draft.Name), index)
	cfg, err := e.draftConfigLocked(index, name, draft)
	if err != nil {
		e.unlock()
		return err
	}
	e.cfg = cfg
	if err := e.applyDraftLocked(index, name, draft); err != nil {
		e.unlock()
		return err
	}
	if index == -1 {
		index = len(e.model.Tabs) - 1
	}
	e.syncConfigTab(cfg, index)
	if draft.Place > 0 {
		e.moveTabLocked(cfg, index, min(draft.Place, len(e.model.Tabs))-1)
	}
	e.forgetUndoLocked()
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	if notSaved != "" {
		e.em.Toast(appliedText(notSaved))
		return nil
	}
	e.em.Toast("Saved " + name)
	return nil
}

// MoveItemToNewTab makes a tab for an item in one step: Move to new tab,
// or with also, Also show in new tab. The tab has the single layout, is
// named after the item's app, numbered if another tab has that name
// (uniqueTabName), and goes after the other tabs. The item moves there, or
// shows there as well as where it is, and Bench switches to the tab. The
// config gets the tab, with the item in it, saved as Tab properties saves
// a new tab. The toast's Undo takes the tab away again, from config.toml
// too, and puts the item back.
func (e *Engine) MoveItemToNewTab(itemID string, also bool) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	m := e.model
	it := m.Items[itemID]
	if it == nil {
		e.unlock()
		return errGone
	}
	name := uniqueTabName(m.Tabs, it.App, -1)
	if name == "" {
		name = uniqueTabName(m.Tabs, "New tab", -1)
	}
	cfg := e.cfg.Clone()
	cfg.Tabs = append(cfg.Tabs, config.Tab{Name: name, Layout: layout.PresetSingle})
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	floating := m.IsFloating(itemID)
	label := labelOf(it)
	step := e.beginUndoLocked()
	was := e.cfg.Clone()
	step.cfgTabs, step.cfgRules = was.Tabs, was.Rules
	ti, err := m.NewTabWith(itemID, name, also)
	if err != nil {
		e.unlock()
		return poppedOut(err, e.cfg.Settings.DragToAdd)
	}
	if floating {
		e.bringBackLocked(it)
	}
	e.cfg = cfg
	e.syncConfigTab(cfg, ti)
	undone := "Moved " + label + " back."
	if also {
		undone = "Deleted the new tab, " + name + "."
	}
	undo := e.keepUndoLocked(step, undone)
	e.switchLocked(ti, switchOpts{focus: true})
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Moved " + label + " to a new tab, " + name + "."
	if also {
		msg = "Showing " + label + " in a new tab, " + name + ", too."
	}
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// holdStateLocked stops state.json being saved, for a change to the tabs
// that config.toml hasn't been saved with yet, and cancels the save that the
// change scheduled. LoadState sets aside a state.json whose tabs differ from
// config.toml's, losing the arrangement, so state.json mustn't be written
// with the change until config.toml has been (see saveEdit).
func (e *Engine) holdStateLocked() {
	if !e.persistent() {
		return
	}
	e.stateHeld = true
	e.saveGen++
	if e.saveTimer != nil {
		e.saveTimer.Stop()
		e.saveTimer = nil
	}
}

// saveEdit saves config.toml after a change in Settings or Tab properties,
// then, if that worked, state.json at once, rather than saveDelay later, so
// that the two files stay in step. If config.toml can't be saved,
// state.json stays held, as the old config.toml's match, until a later
// change saves config.toml, or config.toml is reloaded. It returns why
// config.toml wasn't saved, if Bench didn't try (see saveConfig), for the
// toast, or "".
func (e *Engine) saveEdit(cfg *config.Config, writable bool) (string, error) {
	notSaved, err := e.saveConfig(cfg, writable)
	if err != nil {
		e.lock()
		if e.stateHeld {
			e.setNoticesLocked(sourceState, []string{stateHeldText})
			e.touch()
		}
		e.unlock()
		return "", err
	}
	if notSaved == "" {
		e.saveHeldState()
	}
	return notSaved, nil
}

// saveHeldState saves state.json at once if it's held, now that config.toml
// has been saved with the change to the tabs that held it.
func (e *Engine) saveHeldState() {
	e.lock()
	if !e.stateHeld {
		e.unlock()
		return
	}
	e.stateHeld = false
	st, seq := e.snapshotLocked()
	e.unlock()

	err := e.writeState(st, seq)

	e.lock()
	defer e.unlock()
	e.stateSavedLocked(err)
}

// stateHeldText is the status bar's notice while state.json is held because
// config.toml couldn't be saved.
const stateHeldText = "Bench couldn't save config.toml, so it isn't saving the arrangement either. " +
	"Save Settings or a tab's properties to try again."

// uniqueTabName returns name for tab except (-1 for a new tab), numbered if
// another tab has it, ignoring case, as tab names must be unique: "Code"
// becomes "Code (1)", or "Code (2)" if that's taken too, and so on. A number
// in brackets that the name already ends with is replaced, not added to, so
// "Code (1)" becomes "Code (2)". An empty name stays empty.
func uniqueTabName(tabs []*Tab, name string, except int) string {
	taken := func(n string) bool {
		for i, t := range tabs {
			if i != except && strings.EqualFold(t.Name, n) {
				return true
			}
		}
		return false
	}
	if name == "" || !taken(name) {
		return name
	}
	base := unnumbered(name)
	for n := 1; ; n++ {
		if next := fmt.Sprintf("%s (%d)", base, n); !taken(next) {
			return next
		}
	}
}

// unnumbered returns a tab name without the number in brackets that it ends
// with, if it has one: "Code (2)" gives "Code".
func unnumbered(name string) string {
	rest, ok := strings.CutSuffix(name, ")")
	i := strings.LastIndex(rest, " (")
	if !ok || i < 0 {
		return name
	}
	digits := rest[i+2:]
	base := strings.TrimSpace(rest[:i])
	if digits == "" || strings.Trim(digits, "0123456789") != "" || base == "" {
		return name
	}
	return base
}

// draftConfigLocked checks a draft, whose name is unique, and returns the
// config with it applied.
func (e *Engine) draftConfigLocked(index int, name string, draft TabDraft) (*config.Config, error) {
	if name == "" {
		return nil, errNoName
	}
	if index != -1 && e.model.Tab(index) == nil {
		return nil, errNoTab(index)
	}
	if _, ok := layout.LookupPreset(draft.Preset); !ok && (draft.Preset != layout.PresetCustom || index == -1) {
		return nil, errNoPreset(draft.Preset)
	}
	small := draft.SmallScreens
	switch layout.SmallScreens(small) {
	case "", layout.Stack, layout.Tabs, layout.Same:
	default:
		return nil, fmt.Errorf("There's no small-screen setting called %s.", small)
	}
	cfg := e.cfg.Clone()
	if index == -1 {
		cfg.Tabs = append(cfg.Tabs, config.Tab{Name: name, Layout: draft.Preset, SmallScreens: tidySmall(small)})
	} else {
		ct := &cfg.Tabs[index]
		old := ct.Name
		ct.Name, ct.SmallScreens = name, tidySmall(small)
		if draft.Preset != layout.PresetCustom && draft.Preset != ct.Layout {
			ct.Layout, ct.Sizes = draft.Preset, nil
		}
		for i := range cfg.Rules {
			if strings.EqualFold(cfg.Rules[i].Tab, old) {
				cfg.Rules[i].Tab = name
			}
		}
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	return cfg, nil
}

// tidySmall returns a small-screen setting as the config writes it: stack,
// the default, is left out.
func tidySmall(s string) string {
	if s == string(layout.Stack) {
		return ""
	}
	return s
}

// applyDraftLocked changes the model's tabs for a draft that has been
// checked: a new tab, which becomes the current one, or a renamed and
// re-laid-out one.
func (e *Engine) applyDraftLocked(index int, name string, draft TabDraft) error {
	m := e.model
	small := smallScreensOf(draft.SmallScreens)
	if index == -1 {
		if _, err := m.AddTab(name, draft.Preset, small); err != nil {
			return err
		}
		e.switchLocked(len(m.Tabs)-1, switchOpts{focus: true})
		return nil
	}
	t := m.Tabs[index]
	t.Name, t.SmallScreens = name, small
	if draft.Preset != layout.PresetCustom && draft.Preset != t.Preset {
		if err := m.ApplyPreset(index, draft.Preset, nil); err != nil {
			return err
		}
	}
	return nil
}

// syncConfigTab writes a tab's items into its config entry: its panes'
// items, one entry per app per pane, for the preset the config gives it. If
// that would make a config with problems, the entry keeps its panes. An App
// item with no window is written only if the config already lists its app
// (and title filter) in some tab, so that an app that has quit, or isn't
// installed, doesn't become an entry, while one the user moved here from
// another tab's entry stays in the file; either way, the item stays in the
// tab.
func (e *Engine) syncConfigTab(cfg *config.Config, index int) {
	if index < 0 || index >= len(cfg.Tabs) || index >= len(e.model.Tabs) {
		return
	}
	ct := &cfg.Tabs[index]
	slots := 1
	if p, ok := layout.LookupPreset(ct.Layout); ok {
		slots = p.Slots
	}
	listed := map[string]bool{}
	for _, t := range cfg.Tabs {
		for _, k := range entryKeys(t) {
			listed[k] = true
		}
	}
	keep := func(it *Item) bool {
		return it.Kind != KindApp || it.WindowID != 0 || listed[itemKeys(it)[0]]
	}
	old := ct.Panes
	ct.Panes = e.model.configPanesKeeping(e.model.Tabs[index], slots, keep)
	if len(cfg.Validate()) > 0 {
		ct.Panes = old
	}
}

// DeleteTab deletes a tab. Its windows aren't closed: they move to "Not in a
// tab". The last tab can't be deleted. The toast's Undo brings the tab back,
// and so does Reopen closed tab (see undo.go).
func (e *Engine) DeleteTab(index int) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	m := e.model
	t := m.Tab(index)
	if t == nil {
		e.unlock()
		return errNoTab(index)
	}
	if len(m.Tabs) == 1 {
		e.unlock()
		return errLastTab
	}
	name := t.Name
	step := e.beginDeleteLocked(index)
	keep := func(it *Item) bool { return it.Kind != KindApp || it.WindowID != 0 }
	saved, writable, err := e.deleteTabLocked(index, keep, e.leaveTabLocked, nil)
	if err != nil {
		e.unlock()
		return err
	}
	undo := e.keepUndoLocked(step, "Brought back "+name+".")
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Deleted " + name + ". Its windows are in Jump to… under “Not in a tab”."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// deleteTabLocked deletes tab index, which isn't the last, from the model
// and the config, with the rules that name it, for DeleteTab and CloseTab.
// Its items that are in other tabs too stay there. Of the rest, those for
// which keep reports true move to "Not in a tab", and leave is called for
// each; the others are forgotten. edit, if not nil, changes the config
// further, in the same save. It returns the config to save, with saveEdit,
// once the lock is released, and whether config.toml can be written.
func (e *Engine) deleteTabLocked(index int, keep func(*Item) bool, leave func(*Item), edit func(*config.Config)) (*config.Config, bool, error) {
	m := e.model
	name := m.Tabs[index].Name
	current := m.Current
	kept, _, err := m.DeleteTab(index, keep)
	if err != nil {
		return nil, false, err
	}
	for _, id := range kept {
		leave(m.Items[id])
	}
	cfg := e.cfg.Clone()
	cfg.Tabs = append(cfg.Tabs[:index:index], cfg.Tabs[index+1:]...)
	var rules []config.Rule
	for _, r := range cfg.Rules {
		if !strings.EqualFold(r.Tab, name) {
			rules = append(rules, r)
		}
	}
	cfg.Rules = rules
	if edit != nil {
		edit(cfg)
	}
	e.cfg = cfg
	if index == current {
		e.switchLocked(m.Current, switchOpts{focus: true})
	} else {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	e.holdStateLocked()
	return cfg.Clone(), e.writable, nil
}

// leaveTabLocked lets an item leave a deleted tab for Floating: its window is
// put back on screen if it's parked, and left free-floating; a web view is
// hidden.
func (e *Engine) leaveTabLocked(it *Item) {
	if it == nil {
		return
	}
	switch it.Kind {
	case KindApp:
		e.releaseWindowLocked(it)
	case KindWeb:
		if _, shown := e.webShown[it.ID]; shown {
			e.p.HideWebView(it.ID)
			delete(e.webShown, it.ID)
		}
	}
}

// MoveTab moves tab from to index to, where it ends up among the tabs: a
// drag along the tab strip, or Move left and Move right. Tab shortcuts
// follow the order, so the tab takes the shortcut of its new place. The
// current tab stays current, each window stays in its tabs, and each tab
// keeps its arrangement. config.toml's tabs take the same order: Bench saves
// it as Tab properties does, then state.json. It says so, with Undo.
func (e *Engine) MoveTab(from, to int) error {
	return e.moveTab(func() (int, int) { return from, to })
}

// MoveCurrentTab moves the current tab one place left (step -1) or right
// (step 1), as Tab ▸ Move Tab Left and Move Tab Right do.
func (e *Engine) MoveCurrentTab(step int) error {
	return e.moveTab(func() (int, int) { return e.model.Current, e.model.Current + step })
}

// moveTab is MoveTab for the tab and place that places returns, under the
// engine's lock.
func (e *Engine) moveTab(places func() (from, to int)) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	from, to := places()
	t := e.model.Tab(from)
	switch {
	case t == nil:
		e.unlock()
		return errNoTab(from)
	case to < 0 || to >= len(e.model.Tabs):
		e.unlock()
		return errTabAtEnd(t.Name, to < 0)
	case to == from:
		e.unlock()
		return nil
	}
	step := e.beginUndoLocked()
	step.reordered = true
	cfg := e.cfg.Clone()
	e.moveTabLocked(cfg, from, to)
	e.cfg = cfg
	e.changed()
	e.holdStateLocked()
	undo := e.keepUndoLocked(step, "Moved "+t.Name+" back.")
	keys := keysOf(cfg.Shortcuts)
	saved, writable := cfg.Clone(), e.writable
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Moved " + t.Name + " to " + ordinal(to+1) + " place"
	if k := tabShortcutText(keys, to); k != "" {
		msg += " (" + k + ")"
	}
	msg += "."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// moveTabLocked moves tab from to index to in the model, and in cfg, which
// is to be the engine's config: the model's tabs and the config's are in
// the same order.
func (e *Engine) moveTabLocked(cfg *config.Config, from, to int) {
	if moved, err := e.model.MoveTab(from, to); err != nil || !moved {
		return
	}
	if from < len(cfg.Tabs) && to < len(cfg.Tabs) {
		ct := cfg.Tabs[from]
		cfg.Tabs = slices.Insert(slices.Delete(cfg.Tabs, from, from+1), to, ct)
	}
}

// ResetLayout returns a tab to its config layout: the config's preset and
// sizes, with each item in the pane its config entry claims, and the rest
// in order. It says so, as the tab may not be the current one, with Undo.
func (e *Engine) ResetLayout(index int) error {
	e.lock()
	defer e.unlock()
	t := e.model.Tab(index)
	if t == nil || index >= len(e.cfg.Tabs) {
		return errNoTab(index)
	}
	step := e.beginUndoLocked()
	if err := e.model.ResetTab(index, e.cfg.Tabs[index], e.appName); err != nil {
		return err
	}
	if index == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	e.undoToastLocked("Reset the layout of "+t.Name, e.keepUndoLocked(step, "Put back the layout of "+t.Name+"."))
	return nil
}
