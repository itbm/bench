package workspace

import (
	"slices"

	"github.com/itbm/bench/internal/layout"
)

// Move moves an item to a pane of tab ti: into it (layout.Center), where
// it's shown, or beside it (an edge zone), splitting the pane with the item
// in the new pane. Either way the item's new pane is focused and the tabs
// involved are marked custom. The item may be in any tab, or floating. An
// item in several tabs moves from its place in tab ti, if it has one there,
// or else from the place Where gives, and stays in its other tabs.
//
// Moving an item to the centre of its own pane does nothing and reports
// false. A pane can't be split by its only item (errOnlyItem).
func (m *Model) Move(id string, ti int, paneID string, zone layout.Zone) (bool, error) {
	return m.move(id, ti, paneID, zone, false)
}

// move is Move. With nest, a left or right split always puts the pane and
// the new one in a row of their own, rather than adding a sibling to the
// pane's parent row: for the stack view, which shows the top-level row as
// a column, so that the two still appear side by side.
func (m *Model) move(id string, ti int, paneID string, zone layout.Zone, nest bool) (bool, error) {
	t := m.Tab(ti)
	if t == nil {
		return false, errNoTab(ti)
	}
	if m.Items[id] == nil {
		return false, errGone
	}
	if t.Pane(paneID) == nil {
		return false, errNoPane
	}
	fromTab, from := ti, m.PaneIn(ti, id)
	if from == nil {
		fromTab, from = m.Where(id)
	}
	switch zone {
	case layout.Center:
		if from != nil && from.ID == paneID {
			return false, nil
		}
		m.leave(id, fromTab)
		insertAt(t.Pane(paneID), id, -1)
	case layout.Left, layout.Right, layout.Top, layout.Bottom:
		if from != nil && from.ID == paneID && len(from.Items) == 1 {
			return false, errOnlyItem
		}
		m.leave(id, fromTab)
		fresh := layout.NewPane(m.NewID(panePrefix), id)
		var root *layout.Node
		var err error
		if nest && (zone == layout.Left || zone == layout.Right) {
			root, err = m.nestRow(t.Root, paneID, zone, fresh)
		} else {
			root, err = layout.Split(t.Root, paneID, zone, fresh, m.nodeID)
		}
		if err != nil {
			// Never lose the item: put it in the pane instead.
			insertAt(t.Pane(paneID), id, -1)
		} else {
			t.Root = root
		}
	default:
		return false, errBadZone
	}
	m.ShowIn(id, ti)
	t.markCustom()
	if fromTab >= 0 {
		m.Tabs[fromTab].markCustom()
	}
	return true, nil
}

// nestRow replaces a pane with a new 50/50 row holding it and fresh, with
// fresh on the zone's side (left or right), and returns the new root.
func (m *Model) nestRow(root *layout.Node, paneID string, zone layout.Zone, fresh *layout.Node) (*layout.Node, error) {
	p, parent, i := layout.Find(root, paneID)
	if !p.IsPane() {
		return root, errNoPane
	}
	children := []*layout.Node{p, fresh}
	if zone == layout.Left {
		children = []*layout.Node{fresh, p}
	}
	row := layout.NewSplit(m.NewID(splitPrefix), layout.Row, []float64{50, 50}, children...)
	if parent == nil {
		return row, nil
	}
	parent.Children[i] = row
	return root, nil
}

// MoveToTab moves an item into tab ti's focused pane and shows it there,
// without changing the current tab. An item in several tabs moves from the
// place Where gives; if it's in tab ti already, it only leaves that place.
// It does nothing for an item whose place Where gives is in tab ti, and
// reports whether it moved the item.
func (m *Model) MoveToTab(id string, ti int) (bool, error) {
	t := m.Tab(ti)
	if t == nil {
		return false, errNoTab(ti)
	}
	if m.Items[id] == nil {
		return false, errGone
	}
	fromTab, _ := m.Where(id)
	if fromTab == ti {
		return false, nil
	}
	if m.PaneIn(ti, id) != nil {
		m.takeFrom(id, fromTab)
		m.ShowIn(id, ti)
		return true, nil
	}
	m.leave(id, fromTab)
	p := t.FocusedPane()
	insertAt(p, id, -1)
	p.Active = id
	t.Focus = p.ID
	t.CompactShown = id
	m.Use(id)
	t.markCustom()
	if fromTab >= 0 {
		m.Tabs[fromTab].markCustom()
	}
	return true, nil
}

// AddToTab puts an item into a pane of tab ti as well as where it is, and
// shows it there, without changing the current tab: one window, web page,
// or terminal can be in several tabs, and appears in its pane in whichever
// of them is current. paneID "" means the tab's focused pane. It does
// nothing for an item already in that tab, and reports whether it added
// the item. A popped-out item isn't in a tab to add it from.
func (m *Model) AddToTab(id string, ti int, paneID string) (bool, error) {
	t := m.Tab(ti)
	if t == nil {
		return false, errNoTab(ti)
	}
	if m.Items[id] == nil {
		return false, errGone
	}
	if m.IsFloating(id) {
		return false, errPoppedOut
	}
	if m.PaneIn(ti, id) != nil {
		return false, nil
	}
	p := t.FocusedPane()
	if paneID != "" {
		p = t.Pane(paneID)
	}
	if p == nil {
		return false, errNoPane
	}
	insertAt(p, id, -1)
	m.ShowIn(id, ti)
	return true, nil
}

// RemoveFromTab takes an item that's in several tabs out of tab ti,
// leaving it in the others. An item only in that tab can't leave it this
// way (errOnlyTab): that would take it out of Bench, which is what popping
// out is for.
func (m *Model) RemoveFromTab(id string, ti int) error {
	if m.Tab(ti) == nil {
		return errNoTab(ti)
	}
	if m.Items[id] == nil {
		return errGone
	}
	if m.PaneIn(ti, id) == nil {
		return errNotInTab
	}
	if len(m.TabsOf(id)) < 2 {
		return errOnlyTab
	}
	m.takeFrom(id, ti)
	return nil
}

// SetSizes sets a split's sizes, in whichever tab holds it, and returns that
// tab's index. It doesn't change the tab's preset.
func (m *Model) SetSizes(splitID string, sizes []float64) (int, error) {
	for i, t := range m.Tabs {
		n, _, _ := layout.Find(t.Root, splitID)
		if n == nil {
			continue
		}
		if !n.IsSplit() {
			return -1, errNoSplit
		}
		if err := layout.SetSizes(t.Root, splitID, sizes); err != nil {
			return -1, errBadSizes
		}
		return i, nil
	}
	return -1, errNoSplit
}

// ApplyPreset replaces tab ti's tree with a preset, keeping its items in
// order (layout.ApplyPreset), and sets its preset.
func (m *Model) ApplyPreset(ti int, presetID string, sizes []float64) error {
	t := m.Tab(ti)
	if t == nil {
		return errNoTab(ti)
	}
	root, err := layout.ApplyPreset(t.Root, presetID, sizes, m.nodeID)
	if err != nil {
		return errNoPreset(presetID)
	}
	t.Root = root
	t.Preset = presetID
	t.fixFocus()
	return nil
}

// AddTab appends a tab with a preset's empty panes and returns it.
func (m *Model) AddTab(name, presetID string, small layout.SmallScreens) (*Tab, error) {
	root, err := layout.ApplyPreset(nil, presetID, nil, m.nodeID)
	if err != nil {
		return nil, errNoPreset(presetID)
	}
	t := &Tab{Name: name, Preset: presetID, SmallScreens: small, Root: root}
	t.fixFocus()
	m.Tabs = append(m.Tabs, t)
	return t, nil
}

// DeleteTab removes tab i. Its items that are in other tabs too stay
// there. Of the rest, those for which keep reports true move to Floating,
// in reading order, and the others are forgotten. The current tab
// stays the same tab if it isn't the one deleted, and otherwise becomes the
// tab that took its place (or the new last tab). It returns the items kept
// and the items dropped.
func (m *Model) DeleteTab(i int, keep func(*Item) bool) (kept, dropped []string, err error) {
	t := m.Tab(i)
	if t == nil {
		return nil, nil, errNoTab(i)
	}
	if len(m.Tabs) == 1 {
		return nil, nil, errLastTab
	}
	for _, id := range t.ItemIDs() {
		if len(m.TabsOf(id)) > 1 {
			continue
		}
		if it := m.Items[id]; it != nil && keep(it) {
			kept = append(kept, id)
			continue
		}
		dropped = append(dropped, id)
	}
	m.Tabs = slices.Delete(m.Tabs, i, i+1)
	m.Floating = append(m.Floating, kept...)
	for _, id := range dropped {
		if it := m.Items[id]; it != nil && it.WindowID != 0 {
			delete(m.windows, it.WindowID)
		}
		delete(m.Items, id)
	}
	switch {
	case i < m.Current:
		m.Current--
	case m.Current >= len(m.Tabs):
		m.Current = len(m.Tabs) - 1
	}
	return kept, dropped, nil
}

// removeFromLeaf takes an item out of a pane without removing the pane.
func removeFromLeaf(p *layout.Node, id string) {
	i := slices.Index(p.Items, id)
	if i < 0 {
		return
	}
	p.Items = slices.Delete(p.Items, i, i+1)
	if len(p.Items) == 0 {
		p.Items = nil
	}
	if p.Active == id {
		p.Active = ""
		if len(p.Items) > 0 {
			p.Active = p.Items[min(i, len(p.Items)-1)]
		}
	}
}

// Cycle shows the next (step 1) or previous (step -1) item in a list,
// wrapping, given the one shown now. It returns "" for an empty list.
func cycle(items []string, current string, step int) string {
	if len(items) == 0 {
		return ""
	}
	i := slices.Index(items, current)
	if i < 0 {
		if step < 0 {
			return items[len(items)-1]
		}
		return items[0]
	}
	n := len(items)
	return items[((i+step)%n+n)%n]
}
