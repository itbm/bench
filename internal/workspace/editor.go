package workspace

import (
	"fmt"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file has the methods of the Tab properties dialog, which edits a
// tab's name, layout, and small-screen setting, or adds a tab. The settings
// that apply to every tab are the Settings dialog's (settings.go).

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
	seq := e.model.Seq
	newID := func(nodeType string) string {
		seq++
		if nodeType == layout.TypeSplit {
			return fmt.Sprintf("%s%d", splitPrefix, seq)
		}
		return fmt.Sprintf("%s%d", panePrefix, seq)
	}
	root, err := layout.ApplyPreset(old, preset, sizes, newID)
	if err != nil {
		return nil, errNoPreset(preset)
	}
	return root, nil
}

// SaveTab saves Tab properties: index -1 adds a tab and switches to it;
// otherwise it renames the tab, changes its small-screen setting, and
// applies a new preset, keeping the items in order. It updates the config,
// and saves config.toml when it can.
func (e *Engine) SaveTab(index int, draft TabDraft) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	name := strings.TrimSpace(draft.Name)
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
	e.arrangeLocked(arrangeOpts{})
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	if err := e.saveEdit(saved, writable); err != nil {
		return err
	}
	e.em.Toast("Saved " + name)
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
// change saves config.toml.
func (e *Engine) saveEdit(cfg *config.Config, writable bool) error {
	if err := e.saveConfig(cfg, writable); err != nil {
		e.lock()
		if e.stateHeld {
			e.setNoticesLocked(sourceState, []string{stateHeldText})
			e.touch()
		}
		e.unlock()
		return err
	}
	e.lock()
	if !e.stateHeld {
		e.unlock()
		return nil
	}
	e.stateHeld = false
	st, seq := e.snapshotLocked()
	e.unlock()

	err := e.writeState(st, seq)

	e.lock()
	defer e.unlock()
	e.stateSavedLocked(err)
	return nil
}

// stateHeldText is the status bar's notice while state.json is held because
// config.toml couldn't be saved.
const stateHeldText = "Bench couldn't save config.toml, so it isn't saving the arrangement either. " +
	"Save Settings or a tab's properties to try again."

// draftConfigLocked checks a draft and returns the config with it applied.
func (e *Engine) draftConfigLocked(index int, name string, draft TabDraft) (*config.Config, error) {
	if name == "" {
		return nil, errNoName
	}
	for i, t := range e.model.Tabs {
		if i != index && strings.EqualFold(t.Name, name) {
			return nil, errTabExists(t.Name)
		}
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
// that would make a config with problems, the entry keeps its panes.
func (e *Engine) syncConfigTab(cfg *config.Config, index int) {
	if index < 0 || index >= len(cfg.Tabs) || index >= len(e.model.Tabs) {
		return
	}
	ct := &cfg.Tabs[index]
	slots := 1
	if p, ok := layout.LookupPreset(ct.Layout); ok {
		slots = p.Slots
	}
	old := ct.Panes
	ct.Panes = e.model.configPanes(e.model.Tabs[index], slots)
	if len(cfg.Validate()) > 0 {
		ct.Panes = old
	}
}

// DeleteTab deletes a tab. Its windows aren't closed: they move to "Not in a
// tab". The last tab can't be deleted.
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
	current := m.Current
	keep := func(it *Item) bool { return it.Kind != KindApp || it.WindowID != 0 }
	kept, _, err := m.DeleteTab(index, keep)
	if err != nil {
		e.unlock()
		return err
	}
	for _, id := range kept {
		e.leaveTabLocked(m.Items[id])
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
	e.cfg = cfg
	if index == current {
		e.switchLocked(m.Current, switchOpts{focus: true})
	} else {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	if err := e.saveEdit(saved, writable); err != nil {
		return err
	}
	e.em.Toast("Deleted " + name + ". Its windows are in Jump to… under “Not in a tab”.")
	return nil
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

// ResetLayout returns a tab to its config layout: the config's preset and
// sizes, with each item in the pane its config entry claims, and the rest
// in order.
func (e *Engine) ResetLayout(index int) error {
	e.lock()
	defer e.unlock()
	if e.model.Tab(index) == nil || index >= len(e.cfg.Tabs) {
		return errNoTab(index)
	}
	if err := e.model.ResetTab(index, e.cfg.Tabs[index], e.appName); err != nil {
		return err
	}
	if index == e.model.Current {
		e.arrangeLocked(arrangeOpts{})
	}
	e.changed()
	return nil
}
