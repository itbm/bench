package workspace

import (
	"reflect"
	"slices"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/windows"
)

// This file has what a pane's menu and the dividers do to the panes
// themselves: zooming a pane, so that it fills the tab for a while, giving
// a split's panes equal sizes, and splitting a pane with an empty one for
// the add dialog to fill.
//
// A zoom changes only what's drawn, never the model, as hiding does: the
// tab keeps its panes, sizes, and items, and state.json saves them as they
// are. It isn't saved itself, since it's meant for a moment: Bench always
// starts with every pane shown.

// paneZoom is the pane that fills its tab's pane area (ZoomPane), or none.
type paneZoom struct {
	tab  *Tab
	pane string
}

// emptySplit is the empty pane that SplitPane made for the add dialog to
// fill, with its tab as it was before (before, preset, and focus) and just
// after (after), so that CancelSplit can put it back as it was.
type emptySplit struct {
	tab    *Tab
	pane   string
	before *layout.Node
	after  *layout.Node
	preset string
	focus  string
}

// zoomedLocked returns the pane of the current tab that's zoomed, or nil. A
// zoom ends once it no longer applies, and is forgotten: when the tab isn't
// current any more, when another pane is focused, by any means, or the
// pane has gone, when the tab has only that pane left, and when the tab is
// shown in the "tabs" view, which shows one pane anyway.
func (e *Engine) zoomedLocked() *layout.Node {
	if e.zoom.pane == "" {
		return nil
	}
	t := e.model.CurrentTab()
	var p *layout.Node
	if t != nil && t == e.zoom.tab && t.Focus == e.zoom.pane && len(t.Leaves()) > 1 && !e.isTabsView(t) {
		p = t.Pane(e.zoom.pane)
	}
	if p == nil {
		e.zoom = paneZoom{}
	}
	return p
}

// endZoomLocked forgets the zoom, if there is one, and reports whether there
// was.
func (e *Engine) endZoomLocked() bool {
	zoomed := e.zoomedLocked() != nil
	e.zoom = paneZoom{}
	return zoomed
}

// unshownStacksLocked returns, while a pane is zoomed, a stack for each of
// the tab's other panes, with no window shown, so that the windows manager
// parks their windows, as it does those of a pane that shows a page or a
// terminal. Their items keep their places, and their windows come back as
// the zoom ends.
func (e *Engine) unshownStacksLocked(d display) []windows.Stack {
	if !d.zoomed {
		return nil
	}
	var out []windows.Stack
	for _, p := range d.tab.Leaves() {
		if p.ID == d.root.ID {
			continue
		}
		s := windows.Stack{PaneID: p.ID}
		for _, id := range p.Items {
			if it := e.model.Items[id]; dockable(it) {
				s.Windows = append(s.Windows, windowOf(it))
			}
		}
		if len(s.Windows) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// ZoomPane zooms a pane of the current tab, or the focused pane for "": it
// fills the tab's pane area, with its own sub-tabs, and focuses it, while
// the windows of the tab's other panes go out of sight. It lasts until
// ShowAllPanes, or until the tab changes or another pane is focused (see
// zoomedLocked). Refused in the "tabs" view, which shows one pane already,
// and for a tab with only one pane.
func (e *Engine) ZoomPane(paneID string) error {
	e.lock()
	defer e.unlock()
	return e.zoomPaneLocked(paneID)
}

func (e *Engine) zoomPaneLocked(paneID string) error {
	t := e.model.CurrentTab()
	if t == nil {
		return errNoPane
	}
	if e.isTabsView(t) {
		return errZoomTabsView
	}
	p := t.FocusedPane()
	if paneID != "" {
		p = t.Pane(paneID)
	}
	if p == nil {
		return errNoPane
	}
	if len(t.Leaves()) < 2 {
		return errZoomOnePane
	}
	if z := e.zoomedLocked(); z != nil && z.ID == p.ID {
		return nil
	}
	e.model.FocusPane(t, p)
	e.zoom = paneZoom{tab: t, pane: p.ID}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return nil
}

// ShowAllPanes ends ZoomPane's zoom: the tab shows every pane again, with
// the zoomed pane still focused. It does nothing while no pane is zoomed.
func (e *Engine) ShowAllPanes() error {
	e.lock()
	defer e.unlock()
	e.showAllPanesLocked()
	return nil
}

func (e *Engine) showAllPanesLocked() {
	if !e.endZoomLocked() {
		return
	}
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
}

// ToggleZoom zooms the focused pane, or shows every pane again while a pane
// is zoomed: the zoom_pane shortcut, and View ▸ Zoom Pane.
func (e *Engine) ToggleZoom() error {
	e.lock()
	defer e.unlock()
	if e.zoomedLocked() != nil {
		e.showAllPanesLocked()
		return nil
	}
	return e.zoomPaneLocked("")
}

// EvenSizes gives a split's children equal sizes, as when the user
// double-clicks one of its dividers, and says so, with Undo. A split whose
// children have equal sizes already stays as it is, without a toast.
func (e *Engine) EvenSizes(splitID string) error {
	e.lock()
	defer e.unlock()
	var split *layout.Node
	for _, t := range e.model.Tabs {
		if n, _, _ := layout.Find(t.Root, splitID); n != nil {
			split = n
			break
		}
	}
	if !split.IsSplit() {
		return errNoSplit
	}
	even := layout.Normalise(nil, len(split.Children)) // Equal sizes.
	if slices.EqualFunc(layout.Normalise(split.Sizes, len(split.Children)), even, nearSize) {
		return nil
	}
	step := e.beginUndoLocked()
	if err := e.setSizesLocked(splitID, even); err != nil {
		return err
	}
	e.undoToastLocked("Made the panes the same size.", e.keepUndoLocked(step, "Put back the sizes of the panes."))
	return nil
}

// nearSize reports whether two sizes, in percent, are equal for the eye.
func nearSize(a, b float64) bool {
	return a-b < 0.05 && b-a < 0.05
}

// SplitPane puts an empty pane beside a pane of the current tab, on the
// side zone gives, and focuses it, for the add dialog to fill: a window
// menu's Split right with an empty pane and Split down with an empty pane.
// It returns the new pane's ID. Splits are refused in the "tabs" view. If
// the add dialog closes without adding anything, CancelSplit takes the pane
// away again, so a pane can still never be split by its only window.
func (e *Engine) SplitPane(paneID, zone string) (string, error) {
	e.lock()
	defer e.unlock()
	m := e.model
	t := m.CurrentTab()
	if t == nil || t.Pane(paneID) == nil {
		return "", errNoPane
	}
	if e.isTabsView(t) {
		return "", errSplitsHidden
	}
	z := layout.Zone(zone)
	if !edgeZones[z] {
		return "", errBadZone
	}
	s := emptySplit{tab: t, before: layout.Clone(t.Root), preset: t.Preset, focus: t.Focus}
	id, err := m.SplitEmpty(m.Current, paneID, z, e.stackViewLocked(t))
	if err != nil {
		return "", err
	}
	s.pane, s.after = id, layout.Clone(t.Root)
	e.split = s
	e.arrangeLocked(arrangeOpts{})
	e.focusShownLocked()
	e.changed()
	return id, nil
}

// edgeZones are the zones at a pane's edges, which split it.
var edgeZones = map[layout.Zone]bool{layout.Left: true, layout.Right: true, layout.Top: true, layout.Bottom: true}

// CancelSplit takes away the empty pane that SplitPane made, as its add
// dialog closes without adding anything to it. The tab goes back to how it
// was, sizes and all, if nothing else has changed it since; otherwise the
// pane closes up as an emptied pane does. A pane that has something in it
// now stays, and so does one that a new window is on its way to, from the
// add dialog's New window. Any other pane is left alone.
func (e *Engine) CancelSplit(paneID string) error {
	e.lock()
	defer e.unlock()
	s := e.split
	if s.pane == "" || s.pane != paneID {
		return nil
	}
	e.split = emptySplit{}
	m := e.model
	t := s.tab
	ti := m.IndexOf(t)
	if ti < 0 {
		return nil
	}
	p := t.Pane(paneID)
	if p == nil || len(p.Items) > 0 || e.awaitedLocked(t, paneID) {
		return nil
	}
	if reflect.DeepEqual(t.Root, s.after) {
		t.Root, t.Preset, t.Focus = s.before, s.preset, s.focus
	} else {
		root, err := layout.RemovePane(t.Root, paneID)
		if err != nil {
			return nil
		}
		t.Root = root
		t.fixFocus()
	}
	if ti == m.Current {
		e.arrangeLocked(arrangeOpts{})
		e.focusShownLocked()
	}
	e.changed()
	return nil
}

// awaitedLocked reports whether a new window is on its way to a pane of tab
// t: one that the add dialog asked for there, or one of an app that it
// opened for the pane (see addPendingLocked).
func (e *Engine) awaitedLocked(t *Tab, paneID string) bool {
	now := e.clk.Now()
	return slices.ContainsFunc(e.pending, func(pl pendingPlace) bool {
		return pl.tab == t && pl.pane == paneID && now.Before(pl.until)
	})
}
