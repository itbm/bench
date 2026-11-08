package workspace

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file takes back changes to the arrangement, with the Undo on the
// toast that each sends: Delete tab, Close tab, Reset layout, Close this
// pane, moves of windows and tabs by dragging or from a menu, Also Show
// In, Move to Tab ▸ New Tab, a tab made of open windows, Remove from This
// Tab, Show side by side in the "tabs" view, which changes the layout's
// mode too, equal sizes from a divider, the keys that arrange the panes (a
// run of which is one change, panekeys.go), a drag of a docked window's
// edge that resizes the panes, closing a web page or a terminal, and Keep
// over Bench, which adds a float rule to config.toml (covering.go). Each
// names itself (undoName), for Undo away from its toast. Before each, the
// engine notes the arrangement (undoStep): the tabs, in order, with their
// trees, sizes, and items, and what's "Not in a tab", as state.json saves
// them.
// Undo puts it back. What has gone since, such as the windows that Close
// tab closed, stays gone, and what has come since stays where it is. A
// deleted tab's entry in config.toml comes back too, with its rules, a
// moved tab's goes back to its place, and a new tab's goes. Windows that a
// new tab took from "Not in a tab" go back where they were, and a page or
// terminal that was closed opens again in its place (closeditems.go).
// The latest change can also be taken back away from its toast
// (UndoLatest): from Tab ▸ Undo in the menu bar, the jump palette, and the
// undo shortcut, which is off until the user sets it, as Command-Z
// belongs to the pages and terminals.
//
// Bench's move back to the display that the user chose (returning.go)
// isn't one of these changes: it moves only the window, which the user
// didn't ask for, so it isn't the latest change that UndoLatest takes
// back, and Undo of an earlier change leaves the window where it is. Its
// toast's Undo, whose number is counted with theirs, takes it back alone.
//
// It also keeps the tabs that were deleted or closed most recently, which
// state.json saves, for Reopen Closed Tab.

// undoKept is how many changes Undo can take back.
const undoKept = 10

// closedKept is how many deleted or closed tabs Reopen Closed Tab can
// bring back.
const closedKept = 5

// errCantUndo refuses Undo for a change it no longer has, or one that a
// change to the tabs since, such as a new tab or a reloaded config.toml,
// has put out of reach.
var errCantUndo = errors.New("Bench can't undo that any more.")

// errNothingToUndo refuses UndoLatest when there's no change to take back.
var errNothingToUndo = errors.New("There's nothing to undo.")

// errNoClosedTab refuses ReopenTab for a tab that Bench no longer keeps.
var errNoClosedTab = errors.New("Bench no longer has that tab to reopen.")

// undoName names a change where Undo is offered away from its toast: name
// is the command, in title case, for "Undo Close Tab" in the menu bar and
// the jump palette, and did says what the change did, from a verb in lower
// case, for the toast after UndoLatest takes it back: "Undid: closed
// Plan.", and the palette's hint.
type undoName struct {
	name, did string
}

// undoStep is the arrangement before a change that Undo can take back, and
// what else the change did.
type undoStep struct {
	id int
	// tabs are the model's tabs before the change, and saved their values,
	// with copies of their trees. Undo gives the same tabs those values
	// back, so that a deleted tab comes back as itself.
	tabs     []*Tab
	saved    []Tab
	current  int
	floating []string
	// dropdown is the drop-down terminal before the change, or "" (see
	// restoreLocked).
	dropdown string
	// items holds the model's items before the change, until it's kept.
	items map[string]*Item
	// kept holds the items that the change forgot, which Undo brings back:
	// the places of windows that had closed (App items without a window).
	// added holds those that it added, which Undo takes away, such as the
	// pages and terminals that Reset layout added for config entries.
	kept  map[string]*Item
	added []string
	// cfgTabs and cfgRules are config.toml's tabs and rules before a
	// change that deleted a tab (closed), or added one (Move to Tab ▸ New Tab),
	// which Undo puts back.
	cfgTabs  []config.Tab
	cfgRules []config.Rule
	closed   *config.ClosedTab
	// reordered is set for a change that moved a tab (MoveTab): Undo puts
	// config.toml's tabs back in the order from before it.
	reordered bool
	// outside are the windows that the change took into a tab from "Not
	// in a tab" (NewTabFromWindows), as they were, which Undo puts back.
	outside []outsideWindow
	// setsMode is set for a change that changed the layout's mode
	// (ShowSideBySide), and mode is the mode from before it, which Undo
	// puts back.
	setsMode bool
	mode     layout.Mode
	// covering is set for a change that added a float rule for an app that
	// covered Bench (KeepOverBench), which changed config.toml's rules
	// alone: Undo takes the rule back with cfgRules, leaves the tabs, in
	// config.toml and in the arrangement, as they are, and puts what covers
	// Bench out of sight again.
	covering bool
	// reopen is the web page or terminal that the change closed (CloseItem),
	// as Reopen Closed Page or Terminal keeps it, which Undo opens again in
	// its place.
	reopen *closedItem
	// name names the change, and undone is the toast after Undo.
	name   undoName
	undone string
}

// beginUndoLocked notes the arrangement before a change that Undo can take
// back. keepUndoLocked keeps it once the change is made.
func (e *Engine) beginUndoLocked() *undoStep {
	m := e.model
	s := &undoStep{
		tabs: slices.Clone(m.Tabs), current: m.Current, floating: slices.Clone(m.Floating),
		items: maps.Clone(m.Items), kept: map[string]*Item{},
	}
	for _, t := range m.Tabs {
		c := *t
		c.Root = layout.Clone(t.Root)
		s.saved = append(s.saved, c)
	}
	if d := e.dropdownLocked(); d != nil {
		s.dropdown = d.ID
	}
	return s
}

// beginDeleteLocked is beginUndoLocked for a change that deletes tab index:
// it also notes config.toml's tabs and rules, and the tab as Reopen closed
// tab keeps it.
func (e *Engine) beginDeleteLocked(index int) *undoStep {
	s := e.beginUndoLocked()
	cfg := e.cfg.Clone()
	s.cfgTabs, s.cfgRules = cfg.Tabs, cfg.Rules
	s.closed = e.closedTabLocked(index)
	return s
}

// keepUndoLocked keeps a change's step, now that the change is made, with
// its name and the toast that Undo sends, and returns its number, for the
// change's toast (undoToast). A tab that the change deleted joins the
// closed tabs, and a page or terminal that it closed the closed pages and
// terminals.
func (e *Engine) keepUndoLocked(s *undoStep, name undoName, undone string) int {
	m := e.model
	for id, it := range s.items {
		if m.Items[id] == nil && it.Kind == KindApp && it.WindowID == 0 {
			s.kept[id] = it
		}
	}
	for _, id := range slices.Sorted(maps.Keys(m.Items)) {
		if s.items[id] == nil {
			s.added = append(s.added, id)
		}
	}
	s.items = nil
	s.name, s.undone = name, undone
	e.undoSeq++
	s.id = e.undoSeq
	e.undo = append(e.undo, s)
	if len(e.undo) > undoKept {
		e.undo = slices.Delete(e.undo, 0, len(e.undo)-undoKept)
	}
	if s.closed != nil {
		e.noteClosedLocked(s.closed)
	}
	if s.reopen != nil {
		s.reopen.step = s.id
		e.noteClosedItemLocked(s.reopen)
	}
	// The View names the latest change, for Undo away from its toast.
	e.touch()
	return s.id
}

// forgetUndoLocked forgets the changes that Undo could take back, after a
// change to the tabs that it can't, such as a new or renamed tab, or a
// reloaded config.toml: the arrangement before them would name tabs that
// config.toml no longer has, or miss ones it has.
func (e *Engine) forgetUndoLocked() {
	e.undo = nil
	e.touch()
}

// latestUndoLocked returns the change that UndoLatest takes back, for the
// View, or nil if there's none.
func (e *Engine) latestUndoLocked() *UndoView {
	if len(e.undo) == 0 {
		return nil
	}
	s := e.undo[len(e.undo)-1]
	return &UndoView{Step: s.id, Name: s.name.name, Did: s.name.did}
}

// undoToast is a change's toast, with Undo, which takes back change step.
func undoToast(msg string, step int) Toast {
	return Toast{Text: msg, Action: ToastAction{Label: "Undo", Method: "Undo", Arg: step}}
}

// undoToastLocked queues a change's toast, with Undo.
func (e *Engine) undoToastLocked(msg string, step int) {
	em, t := e.em, undoToast(msg, step)
	e.later(func() { em.ActionToast(t) })
}

// Undo takes back change step, which a toast's Undo names, and the changes
// made after it, latest first, putting back the arrangement from before it.
// A web page or terminal that was closed opens again in its place.
// A tab that was deleted comes back, with its entry in config.toml and its
// rules, which Bench saves, and the windows, pages, and terminals that are
// still open dock in it again. A tab that was moved goes back to its place,
// in config.toml too, and one that Move to Tab ▸ New Tab or a tab made of
// open windows added goes; the windows that the latter took from "Not in a tab"
// go back where they were. What has closed since stays closed, and what has
// come since stays where it is. The Undo of Bench's move back to the display
// that the user chose moves the window back alone (undoWentBackLocked).
func (e *Engine) Undo(step int) error {
	return e.takeBack(step, false)
}

// UndoLatest takes back the latest change that Undo can, as its toast's
// Undo would, and says which: "Undid: closed Plan.". Tab ▸ Undo in the
// menu bar, the jump palette's Undo row, and the undo shortcut call it.
func (e *Engine) UndoLatest() error {
	return e.takeBack(0, true)
}

// takeBack is Undo for change step, or, with latest, for the latest
// change.
func (e *Engine) takeBack(step int, latest bool) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	if latest && !e.closed {
		if len(e.undo) == 0 {
			e.unlock()
			return errNothingToUndo
		}
		step = e.undo[len(e.undo)-1].id
	}
	if w := e.wentBack; w != nil && !latest && !e.closed && step == w.step {
		msg, err := e.undoWentBackLocked()
		e.unlock()
		if err != nil {
			return err
		}
		e.em.Toast(msg)
		return nil
	}
	i := slices.IndexFunc(e.undo, func(s *undoStep) bool { return s.id == step })
	if e.closed || i < 0 {
		e.unlock()
		return errCantUndo
	}
	steps := e.undo[i:]
	var cfg *config.Config
	kept, added := map[string]*Item{}, map[string]bool{}
	for _, s := range slices.Backward(steps) {
		if s.cfgTabs != nil || s.covering {
			if cfg == nil {
				cfg = e.cfg.Clone()
			}
			// Settings saved since, or turned off by the change itself (Don't
			// ask again), stay as they are, and so do the tabs if the change
			// was to the rules alone.
			was := (&config.Config{Tabs: s.cfgTabs, Rules: s.cfgRules}).Clone()
			if !s.covering {
				cfg.Tabs = was.Tabs
			}
			cfg.Rules = was.Rules
		}
		maps.Copy(kept, s.kept)
		for _, id := range s.added {
			added[id] = true
		}
	}
	// A window that several of the changes took from "Not in a tab" goes
	// back to where it was before the first.
	var outside []outsideWindow
	for _, s := range steps {
		for _, o := range s.outside {
			if !slices.ContainsFunc(outside, func(x outsideWindow) bool { return x.item == o.item }) {
				outside = append(outside, o)
			}
		}
	}
	first := steps[0]
	// The arrangement to put back is the one from before the first change
	// that changed it. Keep over Bench changed only config.toml's rules, so
	// with nothing else to take back, the arrangement stays as it is, with
	// the tab switches and sizes since.
	var arranged *undoStep
	if j := slices.IndexFunc(steps, func(s *undoStep) bool { return !s.covering }); j >= 0 {
		arranged = steps[j]
	}
	if slices.ContainsFunc(steps, func(s *undoStep) bool { return s.reordered }) {
		if cfg == nil {
			cfg = e.cfg.Clone()
		}
		cfg.Tabs = inTabOrder(cfg.Tabs, arranged.saved)
	}
	names := e.cfg.TabNames()
	if cfg != nil {
		names = cfg.TabNames()
	}
	if arranged != nil && !slices.EqualFunc(arranged.saved, names, func(t Tab, name string) bool { return strings.EqualFold(t.Name, name) }) {
		e.forgetUndoLocked()
		e.unlock()
		return errCantUndo
	}
	e.undo = e.undo[:i]
	for _, s := range steps {
		if s.closed != nil {
			e.closedTabs = slices.DeleteFunc(e.closedTabs, func(c *config.ClosedTab) bool { return c == s.closed })
		}
		// A page or terminal that a change closed opens again in its
		// place, unless Reopen has brought it back already, or it came after
		// the first change, which has no place for it: it stays for Reopen.
		if c := s.reopen; c != nil && arranged.hadItem(c.id) && slices.Contains(e.closedItems, c) && e.model.Items[c.id] == nil {
			kept[c.id] = e.reopenedItemLocked(c)
			e.closedItems = slices.DeleteFunc(e.closedItems, func(o *closedItem) bool { return o == c })
		}
	}
	if cfg != nil {
		e.cfg = cfg
	}
	if i := slices.IndexFunc(steps, func(s *undoStep) bool { return s.setsMode }); i >= 0 {
		e.model.Mode = steps[i].mode
	}
	if arranged != nil {
		e.restoreLocked(arranged, kept, added, outside)
	} else {
		e.touch()
	}
	// What covers Bench goes again, if Bench is in front, now that the
	// tab it shows is the one to keep apps for.
	if slices.ContainsFunc(steps, func(s *undoStep) bool { return s.covering }) && e.benchActive && e.trusted && !e.benchHidden && !e.opts.Headless {
		e.hideCoveringLocked()
	}
	msg := first.undone
	if latest && first.name.did != "" {
		msg = "Undid: " + first.name.did + "."
	}
	if cfg == nil {
		e.unlock()
		e.em.Toast(msg)
		return nil
	}
	e.holdStateLocked()
	saved, writable := cfg.Clone(), e.writable
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.Toast(msg)
	return nil
}

// hadItem reports whether the arrangement from before change s had item id
// in a tab.
func (s *undoStep) hadItem(id string) bool {
	return slices.ContainsFunc(s.saved, func(t Tab) bool { return layout.PaneOf(t.Root, id) != nil })
}

// inTabOrder returns config.toml's tabs in the order of the tabs of a
// change's step, by name, ignoring case, or as they are if the two don't
// have the same names.
func inTabOrder(tabs []config.Tab, order []Tab) []config.Tab {
	if len(tabs) != len(order) {
		return tabs
	}
	out := make([]config.Tab, 0, len(tabs))
	for _, t := range order {
		i := slices.IndexFunc(tabs, func(ct config.Tab) bool { return strings.EqualFold(ct.Name, t.Name) })
		if i < 0 {
			return tabs
		}
		out = append(out, tabs[i])
	}
	return out
}

// restoreLocked puts back the arrangement from before change s, with the
// items that the changes since it forgot (kept) and without those they
// added (added), puts the windows that they took from "Not in a tab" back
// where they were (outside), and arranges the current tab.
//
// The drop-down terminal isn't part of the arrangement (decision 72): Use
// as Drop-down isn't a change that Undo takes back. So the drop-down stays
// the drop-down, out of the tabs, even if the arrangement had it in a pane,
// or added it, and a terminal that was the drop-down before change s, and
// has gone back into a pane since, stays where it is now, as what has come
// since does.
func (e *Engine) restoreLocked(s *undoStep, kept map[string]*Item, added map[string]bool, outside []outsideWindow) {
	m := e.model
	drop := ""
	if d := e.dropdownLocked(); d != nil {
		drop = d.ID
		delete(added, drop)
	}
	before := maps.Clone(m.Items)
	wasFloating := slices.Clone(m.Floating)
	current := m.CurrentTab()

	// Where each item is now, for those that have come since.
	type place struct {
		tab  *Tab
		pane string
	}
	now := map[string]place{}
	var order []string
	for _, t := range m.Tabs {
		for _, p := range t.Leaves() {
			for _, id := range p.Items {
				if _, ok := now[id]; !ok {
					now[id] = place{t, p.ID}
					order = append(order, id)
				}
			}
		}
	}
	order = append(order, m.Floating...)

	for id, it := range kept {
		if m.Items[id] == nil {
			m.Items[id] = it
		}
	}
	for id := range added {
		delete(m.Items, id)
	}

	// The tabs as they were, without what has gone since.
	m.Tabs = slices.Clone(s.tabs)
	in := map[string]bool{}
	for i, t := range m.Tabs {
		*t = s.saved[i]
		t.Root = layout.Clone(s.saved[i].Root)
		for _, id := range layout.Items(t.Root) {
			if m.Items[id] == nil || id == drop {
				t.Root, _, _ = layout.TakeItem(t.Root, id)
			}
		}
		for _, id := range t.ItemIDs() {
			in[id] = true
		}
	}
	floating := slices.DeleteFunc(slices.Clone(s.floating), func(id string) bool {
		return m.Items[id] == nil || in[id] || (id == s.dropdown && id != drop)
	})

	// What has come since stays where it is: in its pane, if its tab still
	// has it, or else in the tab's focused pane, or else "Not in a tab".
	for _, id := range order {
		if m.Items[id] == nil || in[id] || slices.Contains(floating, id) {
			continue
		}
		pl, ok := now[id]
		if !ok || !slices.Contains(m.Tabs, pl.tab) {
			floating = append(floating, id)
			continue
		}
		p := pl.tab.Pane(pl.pane)
		if p == nil {
			p = pl.tab.FocusedPane()
		}
		insertAt(p, id, -1)
		in[id] = true
	}
	m.Floating = floating
	m.Current = min(max(s.current, 0), len(m.Tabs)-1)
	for _, t := range m.Tabs {
		t.fixFocus()
		if layout.PaneOf(t.Root, t.CompactShown) == nil {
			t.CompactShown = ""
		}
	}
	m.reindex()
	// A window that the user has minimised since isn't shown in its place
	// again: the pane shows another of its windows, as it did (see
	// markMinimisedLocked).
	for _, t := range m.Tabs {
		for _, p := range t.Leaves() {
			if it := m.Items[p.Active]; it != nil && it.minimised {
				if next := e.insteadOfMinimisedLocked(p.Items); next != "" {
					p.Active = next
				}
			}
		}
		if it := m.Items[t.CompactShown]; it != nil && it.minimised {
			if next := e.insteadOfMinimisedLocked(t.ItemIDs()); next != "" {
				t.CompactShown = next
			}
		}
	}

	// What Bench runs for the items that went, came back, or left.
	e.endRemovedItemsLocked(before)
	for _, id := range wasFloating {
		if it := m.Items[id]; it != nil && in[id] {
			e.bringBackLocked(it)
		}
	}
	for _, id := range m.Floating {
		if !slices.Contains(wasFloating, id) {
			e.leaveTabLocked(m.Items[id])
		}
	}
	e.putBackOutsideLocked(outside)
	// A window back in a tab that Close hasn't asked to close yet stays.
	e.closePress = slices.DeleteFunc(e.closePress, func(id string) bool {
		if it := m.Items[id]; it != nil && in[id] {
			it.closing = false
			return true
		}
		return false
	})
	if m.CurrentTab() != current {
		// The tab shown until now is the one to go back to, as after any
		// switch, if it's still there: switchLocked doesn't see it, as the
		// current tab is set already.
		if m.IndexOf(current) >= 0 {
			e.noteLeftTabLocked(current)
		}
		e.switchLocked(m.Current, switchOpts{focus: true})
		return
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
}

// closedTabLocked returns tab index as Reopen Closed Tab keeps it: its
// arrangement and items, and its entry in config.toml with the rules that
// name it.
func (e *Engine) closedTabLocked(index int) *config.ClosedTab {
	m := e.model
	t := m.Tabs[index]
	c := &config.ClosedTab{Tab: tabState(t), Items: map[string]config.ItemState{}, Index: index, Closed: e.clk.Now()}
	for _, id := range t.ItemIDs() {
		if it := m.Items[id]; it != nil {
			c.Items[id] = itemState(it, false)
		}
	}
	if index < len(e.cfg.Tabs) && strings.EqualFold(e.cfg.Tabs[index].Name, t.Name) {
		c.Config = e.cfg.Tabs[index].Clone()
		c.Tab.Entries = entryKeys(c.Config)
	} else {
		c.Config = config.Tab{Name: t.Name, Layout: layout.PresetSingle}
	}
	for _, r := range e.cfg.Rules {
		if strings.EqualFold(r.Tab, t.Name) {
			c.Rules = append(c.Rules, r)
		}
	}
	return c
}

// noteClosedLocked keeps a deleted or closed tab for Reopen Closed Tab, the
// latest first. An older one of the same name gives way to it.
func (e *Engine) noteClosedLocked(c *config.ClosedTab) {
	e.closedTabs = slices.DeleteFunc(e.closedTabs, func(o *config.ClosedTab) bool { return strings.EqualFold(o.Tab.Name, c.Tab.Name) })
	e.closedTabs = slices.Insert(e.closedTabs, 0, c)
	if len(e.closedTabs) > closedKept {
		e.closedTabs = e.closedTabs[:closedKept]
	}
}

// closedTabsOf returns state.json's closed tabs, as the engine keeps them.
func closedTabsOf(closed []config.ClosedTab) []*config.ClosedTab {
	var out []*config.ClosedTab
	for i := range closed {
		if len(out) < closedKept {
			out = append(out, &closed[i])
		}
	}
	return out
}

// closedNamesLocked returns the names of the tabs that Reopen Closed Tab
// can bring back, the latest first, for the View.
func (e *Engine) closedNamesLocked() []string {
	var out []string
	for _, c := range e.closedTabs {
		out = append(out, c.Tab.Name)
	}
	return out
}

// ReopenTab is Reopen Closed Tab: it brings back the tab called name that
// was deleted or closed most recently, where it was among the tabs, and
// switches to it. Its entry in config.toml comes back, with its rules, which
// Bench saves; a tab of its name since gets it numbered (uniqueTabName). Its
// arrangement comes back as Restore previous arrangement brings one back:
// its windows that are "Not in a tab" dock in it again, and so do its pages
// and terminals, or else they open again, and the places of its windows
// that have closed wait for them, as at startup. What another tab has now
// stays there.
func (e *Engine) ReopenTab(name string) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	ci := slices.IndexFunc(e.closedTabs, func(c *config.ClosedTab) bool { return strings.EqualFold(c.Tab.Name, name) })
	if e.closed || ci < 0 {
		e.unlock()
		return errNoClosedTab
	}
	c := e.closedTabs[ci]
	m := e.model
	at := min(max(c.Index, 0), len(m.Tabs))
	as := uniqueTabName(m.Tabs, c.Tab.Name, -1)
	cfg := e.cfg.Clone()
	ct := c.Config.Clone()
	ct.Name = as
	cfg.Tabs = slices.Insert(cfg.Tabs, min(at, len(cfg.Tabs)), ct)
	for _, r := range c.Rules {
		r.Tab = as
		cfg.Rules = append(cfg.Rules, r)
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return fmt.Errorf("Bench can't reopen %s: %s.", c.Tab.Name, problems[0].Message)
	}
	t, err := m.InsertTab(at, as, ct.Layout, smallScreensOf(ct.SmallScreens))
	if err != nil {
		e.unlock()
		return err
	}
	e.cfg = cfg
	e.closedTabs = slices.Delete(e.closedTabs, ci, ci+1)
	e.forgetUndoLocked()

	// Its items that another tab has now stay there. Those "Not in a tab"
	// come back.
	ts := c.Tab
	ts.Root = layout.Clone(c.Tab.Root)
	floated := map[string]bool{}
	for _, p := range layout.Leaves(ts.Root) {
		p.Items = slices.DeleteFunc(p.Items, func(id string) bool { return len(m.TabsOf(id)) > 0 })
		if !slices.Contains(p.Items, p.Active) {
			p.Active = ""
		}
		for _, id := range p.Items {
			floated[id] = m.IsFloating(id)
		}
	}
	before := maps.Clone(m.Items)
	e.restoreTabLocked(m.IndexOf(t), droppedTab{tab: ts, items: c.Items, floated: floated})
	for _, id := range t.ItemIDs() {
		if before[id] != nil {
			e.bringBackLocked(m.Items[id])
		}
	}
	e.endRemovedItemsLocked(before)
	e.switchLocked(m.IndexOf(t), switchOpts{focus: true})
	e.holdStateLocked()
	saved, writable := cfg.Clone(), e.writable
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Reopened " + as + "."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.Toast(msg)
	return nil
}
