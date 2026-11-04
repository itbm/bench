package workspace

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/term"
)

// This file keeps the web pages and terminals that the user closed from a
// tab (CloseItem), for Undo on the toast after it, and for Reopen Closed
// Page or Terminal: a page's address, profile, and name, a terminal's
// folder, command, and name, and, while keep_terminal_output is on, the
// terminal's output, as state.json and terms/ keep them when Bench quits;
// and where each was. Reopening one opens it again in its place, as a
// tab's pages and terminals open when Bench starts. Bench keeps the last
// closedItemsKept, in memory only, so they go when Bench quits, and a
// terminal's output never reaches the disk this way. One that comes back
// another way, as with its tab (Reopen Closed Tab), isn't kept any more.

// closedItemsKept is how many closed pages and terminals Bench keeps.
const closedItemsKept = 10

// errNoClosedItem refuses ReopenItem for a page or terminal that Bench no
// longer keeps.
var errNoClosedItem = errors.New("Bench no longer has that page or terminal to reopen.")

// closedItem is a web page or terminal that the user closed, as Bench keeps
// it to open it again.
type closedItem struct {
	// id is the item's ID, which no other item gets, as Model.Seq only
	// grows.
	id string
	// state is the item as state.json saves it, and label its name in
	// Bench's messages (labelOfLocked), such as "Slack — # general".
	state config.ItemState
	label string
	// places are where it was, in each tab that had it.
	places []itemPlace
	// step is the number of the change that closed it (keepUndoLocked).
	// The changes after it have arrangements from while it was closed.
	step int
	// output is a terminal's output, made safe to show again
	// (term.Restorable), and at when it closed, while keep_terminal_output
	// is on.
	output []byte
	at     time.Time
}

// itemPlace is where a closed page or terminal was in a tab: its pane, its
// place among the pane's sub-tabs, and whether the pane showed it. For the
// only item of a pane, which closes up with it, beside is a pane that was
// next to it, side the side of that pane it was on, and share its size in
// its split, in percent, so that it can go back there in a pane of its own
// (besideOf).
type itemPlace struct {
	tab    *Tab
	name   string
	pane   string
	index  int
	active bool
	beside string
	side   layout.Zone
	share  float64
}

// closedItemLocked returns a web page or terminal as Bench keeps it once
// it's closed, before it's closed.
func (e *Engine) closedItemLocked(it *Item) *closedItem {
	m := e.model
	c := &closedItem{id: it.ID, state: itemState(it, false), label: e.labelOfLocked(it), at: e.clk.Now()}
	for ti, t := range m.Tabs {
		if p := m.PaneIn(ti, it.ID); p != nil {
			pl := itemPlace{tab: t, name: t.Name, pane: p.ID, index: slices.Index(p.Items, it.ID), active: p.Active == it.ID}
			if len(p.Items) == 1 {
				pl.beside, pl.side, pl.share = besideOf(t.Root, p.ID)
			}
			c.places = append(c.places, pl)
		}
	}
	if it.Kind == KindBuiltin && e.cfg.Settings.KeepTerminalOutput && e.termOn[it.ID] {
		c.output = term.Restorable(e.terms.Snapshot(it.ID).Data, term.SnapshotLimit)
	}
	return c
}

// besideOf returns where pane paneID of a tree goes back if it closes up:
// a pane that's next to it, the side of that pane it's on, and its size in
// its split, in percent; or "" for the tree's only pane. The pane next to
// it is the nearest of its neighbour in its split: the last, in reading
// order, of the one before it, or else the first of the one after it.
func besideOf(root *layout.Node, paneID string) (string, layout.Zone, float64) {
	_, parent, i := layout.Find(root, paneID)
	if parent == nil || len(parent.Children) < 2 {
		return "", "", 0
	}
	share := layout.Normalise(parent.Sizes, len(parent.Children))[i]
	after := i > 0
	var leaves []*layout.Node
	if after {
		leaves = layout.Leaves(parent.Children[i-1])
	} else {
		leaves = layout.Leaves(parent.Children[i+1])
	}
	if len(leaves) == 0 {
		return "", "", 0
	}
	beside := leaves[0]
	if after {
		beside = leaves[len(leaves)-1]
	}
	switch {
	case parent.Dir == layout.Row && after:
		return beside.ID, layout.Right, share
	case parent.Dir == layout.Row:
		return beside.ID, layout.Left, share
	case after:
		return beside.ID, layout.Bottom, share
	}
	return beside.ID, layout.Top, share
}

// resizeTo gives pane paneID, which layout.Split has just put beside pane
// beside, share percent of its split, as it had before it closed up. Split
// gave it half of beside's space, which beside gets back first; then the
// others give up the space in proportion to their sizes, which keeps them
// as they were, if the split is the one the pane was in.
func resizeTo(root *layout.Node, paneID, beside string, share float64) {
	_, parent, i := layout.Find(root, paneID)
	if parent == nil || share <= 0 || share >= 100 {
		return
	}
	sizes := layout.Normalise(parent.Sizes, len(parent.Children))
	if b := slices.IndexFunc(parent.Children, func(n *layout.Node) bool { return n.ID == beside }); b >= 0 {
		sizes[b] += sizes[i]
		sizes[i] = 0
	}
	rest := 100 - sizes[i]
	for j := range sizes {
		sizes[j] *= (100 - share) / rest
	}
	sizes[i] = share
	_ = layout.SetSizes(root, parent.ID, sizes)
}

// noteClosedItemLocked keeps a closed page or terminal, the latest first.
// An older one with the same ID gives way to it, as do those that are open
// again.
func (e *Engine) noteClosedItemLocked(c *closedItem) {
	e.closedItems = slices.DeleteFunc(e.closedItems, func(o *closedItem) bool { return o.id == c.id })
	e.dropReopenedLocked()
	e.closedItems = slices.Insert(e.closedItems, 0, c)
	if len(e.closedItems) > closedItemsKept {
		e.closedItems = e.closedItems[:closedItemsKept]
	}
	e.touch()
}

// reopenedItemLocked returns the item that a closed page or terminal opens
// again as: a page at the address it had, and a terminal in its folder,
// with its command, whose session starts with the output it had, if Bench
// kept it, then a line that says when that was from (restoredPreface).
func (e *Engine) reopenedItemLocked(c *closedItem) *Item {
	it := itemFromState(c.id, c.state, e.appName)
	// The output, if any, is the closed one's, not a file's.
	it.savedOutput = false
	if it.Kind == KindBuiltin && len(c.output) > 0 && e.cfg.Settings.KeepTerminalOutput {
		it.preface = string(restoredPreface(c.output, c.at, e.clk.Now()))
	}
	return it
}

// dropReopenedLocked forgets the closed pages and terminals that are open
// again, with the same IDs, as after Reopen Closed Tab or Restore previous
// arrangement brought them back with their tab.
func (e *Engine) dropReopenedLocked() {
	n := len(e.closedItems)
	e.closedItems = slices.DeleteFunc(e.closedItems, func(c *closedItem) bool { return e.model.Items[c.id] != nil })
	if len(e.closedItems) != n {
		e.touch()
	}
}

// forgetClosedOutputLocked drops the output of the closed terminals, and of
// those reopened whose sessions haven't started, as keep_terminal_output is
// turned off.
func (e *Engine) forgetClosedOutputLocked() {
	for _, c := range e.closedItems {
		c.output = nil
	}
	for _, it := range e.model.Items {
		it.preface = ""
	}
}

// ReopenItem is Reopen Closed Page or Terminal: it opens the closed web page
// or terminal id again where it was, in each tab that had it and still
// does, matched by the tab itself or else by name, in its pane at its
// place among the sub-tabs. If the pane has gone, as a pane does when its
// only item closes, it gets a pane of its own again, beside the pane that
// was next to it; or else it goes in the tab's focused pane. A page or
// terminal whose tabs have all gone comes back in the current tab's
// focused pane. Bench shows it, switching to its tab, and says so. The
// change that closed it can no longer be undone, as it's back, and Undo of
// a change made after it keeps it where Reopen put it. One that's open
// again already, as after Reopen Closed Tab, is shown where it is.
func (e *Engine) ReopenItem(id string) error {
	e.lock()
	defer e.unlock()
	ci := slices.IndexFunc(e.closedItems, func(c *closedItem) bool { return c.id == id })
	if e.closed || ci < 0 {
		return errNoClosedItem
	}
	c := e.closedItems[ci]
	m := e.model
	if m.Items[id] != nil {
		e.closedItems = slices.Delete(e.closedItems, ci, ci+1)
		e.touch()
		if ti, _ := m.Where(id); ti >= 0 {
			e.showInTabLocked(id, ti)
		}
		return nil
	}
	shown := -1
	type backIn struct {
		tab *Tab
		pl  itemPlace
	}
	var back []backIn
	for _, pl := range c.places {
		ti := m.IndexOf(pl.tab)
		if ti < 0 {
			ti = slices.IndexFunc(m.Tabs, func(t *Tab) bool { return strings.EqualFold(t.Name, pl.name) })
		}
		if ti < 0 || m.PaneIn(ti, id) != nil || !m.putBack(m.Tabs[ti], id, pl) {
			continue
		}
		back = append(back, backIn{m.Tabs[ti], pl})
		if shown < 0 {
			shown = ti
		}
	}
	// The changes made while it was closed have arrangements from before
	// them with no place for it, which Undo would put back, and then it
	// would go in the tab's focused pane, as one that has come since, even
	// if it has a pane of its own again. It goes back in those too, as here.
	for _, s := range e.undo {
		if s.id <= c.step {
			continue
		}
		for _, b := range back {
			if i := slices.Index(s.tabs, b.tab); i >= 0 && layout.PaneOf(s.saved[i].Root, id) == nil {
				m.putBack(&s.saved[i], id, b.pl)
			}
		}
	}
	if shown < 0 {
		t := m.CurrentTab()
		p := t.FocusedPane()
		if p == nil {
			return errNoPane
		}
		insertAt(p, id, -1)
		shown = m.Current
	}
	it := e.reopenedItemLocked(c)
	m.Items[id] = it
	e.closedItems = slices.Delete(e.closedItems, ci, ci+1)
	e.undo = slices.DeleteFunc(e.undo, func(s *undoStep) bool { return s.reopen == c })
	e.showInTabLocked(id, shown)
	e.toastLocked("Reopened " + e.labelOfLocked(it) + ".")
	return nil
}

// putBack puts item id back in tab t where it was (pl): in its pane, at
// its place, or, if the pane has gone, in a new pane of its size where it
// was beside another, or else in the tab's focused pane. It reports false
// if the tab has no pane for it.
func (m *Model) putBack(t *Tab, id string, pl itemPlace) bool {
	if p := t.Pane(pl.pane); p != nil {
		insertAt(p, id, min(pl.index, len(p.Items)))
		if pl.active {
			p.Active = id
		}
		return true
	}
	if pl.beside != "" && t.Pane(pl.beside) != nil {
		fresh := layout.NewPane(m.NewID(panePrefix), id)
		if err := m.split(t, pl.beside, pl.side, fresh, false); err == nil {
			resizeTo(t.Root, fresh.ID, pl.beside, pl.share)
			t.markCustom()
			return true
		}
	}
	p := t.FocusedPane()
	if p == nil {
		return false
	}
	insertAt(p, id, min(pl.index, len(p.Items)))
	return true
}

// showInTabLocked shows item id in tab ti, switching to the tab, as
// Reopen does.
func (e *Engine) showInTabLocked(id string, ti int) {
	m := e.model
	m.ShowIn(id, ti)
	if ti != m.Current {
		e.switchLocked(ti, switchOpts{focus: true})
	} else {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
	}
	e.changed()
}

// closedItemsViewLocked returns the closed pages and terminals that Reopen
// Closed Page or Terminal can open again, the latest first, for the View:
// not those that are open again.
func (e *Engine) closedItemsViewLocked() []ClosedItemView {
	var out []ClosedItemView
	for _, c := range e.closedItems {
		if e.model.Items[c.id] != nil {
			continue
		}
		v := ClosedItemView{ID: c.id, Kind: c.state.Kind, Label: c.label}
		if len(c.places) > 0 {
			v.Tab = c.places[0].name
		}
		out = append(out, v)
	}
	return out
}
