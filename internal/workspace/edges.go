package workspace

import (
	"math"
	"slices"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file turns a drag of a docked window's edge into new sizes for the
// panes (decision 62). Grabbing the edge of a window that faces another
// pane is the natural way to make its pane larger; without this, the drag
// would take the window out of its pane. While the user drags, Bench
// leaves the window, and the other windows of its pane, where they are
// (edgeDrag). When the user lets go, which the drag tracker reports
// (WindowDropped), or else the mouse button's state shows
// (Platform.MouseHeld), each dragged edge's new place becomes the place of
// the divider it faces, the tab's windows dock again, and a toast offers
// Undo. A drag that moves the window, or an edge that faces the edge of
// the pane area, takes the window out of its pane, as before. So does any
// drag of an edge in the "tabs" view and of a zoomed pane, as their panes
// have no neighbours. There's one mouse: while it holds one window's
// edge, a move of another docked window is its app's (otherEdgeLocked).
// For dividerGrace after a resize, a move that would take a window out of
// its pane is put back, but a new drag of an inner edge is the user's
// (inGraceLocked).

const (
	// edgeSlack is how far, in points, an edge of a docked window can be
	// from where Bench docked it and still count as staying put: as near
	// as a window must be to a frame that the windows manager set for its
	// move to count as the manager's own.
	edgeSlack = 2
	// edgePoll is how often Bench asks whether the mouse button is still
	// held during a drag of a docked window's edge.
	edgePoll = 50 * time.Millisecond
	// edgeLimit is the longest a drag of a docked window's edge lasts.
	// Past it, Bench takes the window out of its pane, as before.
	edgeLimit = time.Minute
)

// edgeDrag is a docked window of the current tab whose edge the user is
// dragging: the window, its item, the tab, when the drag began, and the
// timer that asks whether the mouse button is still held.
type edgeDrag struct {
	window platform.WindowID
	item   string
	tab    *Tab
	start  time.Time
	timer  clock.Timer
}

// edgeKind is what a change to a docked window's frame is.
type edgeKind int

const (
	// edgeStill: no edge has moved by more than edgeSlack.
	edgeStill edgeKind = iota
	// edgeInner: only edges that face other panes have moved, and not
	// both left and right, nor both top and bottom: a drag of an inner
	// edge, or of a corner between two inner edges.
	edgeInner
	// edgeOut: anything else. The window has moved, an edge that faces
	// the edge of the pane area has moved, or the window's pane has no
	// neighbours.
	edgeOut
)

// edgeMove is an inner edge of a docked window that the user has dragged:
// the divider that it faces, where the divider's middle goes, across its
// split, in the display's coordinates, and whether that makes the
// window's pane larger.
type edgeMove struct {
	divider layout.DividerRect
	pos     float64
	grow    bool
}

// edgeChangeLocked says what a change of item it's window, docked in the
// current tab, to frame, in global coordinates, is, by how far each edge
// is from where Bench docked the window. For edgeInner, it returns each
// moved edge with its divider and where the divider goes: the pane's edge
// goes where the window's edge is, as a docked window fills its pane's
// body.
func (e *Engine) edgeChangeLocked(it *Item, frame layout.Rect) (edgeKind, []edgeMove) {
	from, ok := e.win.DockedFrame(it.WindowID)
	if !ok {
		return edgeOut, nil
	}
	d := e.displayLocked()
	i := slices.IndexFunc(d.panes, func(g paneGeom) bool { return g.node.Active == it.ID })
	if i < 0 {
		return edgeOut, nil
	}
	g := d.panes[i]
	// The body in global coordinates, as the window was docked in it.
	body := g.body
	if !e.opts.Headless {
		cf := e.p.ContentFrame()
		body.X += cf.X
		body.Y += cf.Y
	}
	edges := []struct {
		side layout.Direction
		// was is where the edge was docked, now where it is, and body
		// where the body's edge is.
		was, now, body float64
		// out is 1 if a larger value makes the pane larger, or else -1.
		out float64
	}{
		{layout.DirLeft, from.X, frame.X, body.X, -1},
		{layout.DirRight, from.X + from.W, frame.X + frame.W, body.X + body.W, 1},
		{layout.DirUp, from.Y, frame.Y, body.Y, -1},
		{layout.DirDown, from.Y + from.H, frame.Y + frame.H, body.Y + body.H, 1},
	}
	moved := make([]bool, len(edges))
	still := true
	for k, edge := range edges {
		moved[k] = math.Abs(edge.now-edge.was) > edgeSlack
		still = still && !moved[k]
	}
	switch {
	case moved[0] && moved[1] || moved[2] && moved[3]:
		return edgeOut, nil // Opposite edges moved: the window did.
	case still:
		return edgeStill, nil
	}
	dividers := layout.Dividers(d.root, d.area, d.gap)
	var out []edgeMove
	for k, edge := range edges {
		if !moved[k] {
			continue
		}
		dv, ok := layout.DividerBeside(d.root, dividers, g.node.ID, edge.side)
		if !ok {
			return edgeOut, nil // It faces the edge of the pane area.
		}
		by := edge.now - edge.body
		out = append(out, edgeMove{divider: dv, pos: dv.Middle() + by, grow: by*edge.out > 0})
	}
	return edgeInner, out
}

// holdEdgeLocked follows a change of item it's window, docked in the
// current tab, to frame, in global coordinates, while the user holds the
// mouse on the window. A change that's nothing yet, or a drag of edges
// that face other panes, holds the window where the user puts it until
// they let go (edgeDrag), and it reports true. Anything else ends a hold
// of the window, and reports false: the window is to come out of its
// pane, as before.
func (e *Engine) holdEdgeLocked(it *Item, frame layout.Rect) bool {
	ed := e.edge
	held := ed != nil && ed.window == it.WindowID
	if kind, _ := e.edgeChangeLocked(it, frame); kind == edgeOut {
		if held {
			e.stopEdgeLocked()
		}
		return false
	}
	if held {
		return true
	}
	if ed != nil {
		// Only the drag tracker gets here while another window's edge is
		// held, as onWindowMoved puts such moves back: it has seen a press
		// on this window, and there's one mouse, so the user has let go
		// of the other window, unheard. It goes back in its pane.
		e.stopEdgeLocked()
		defer e.arrangeLocked(arrangeOpts{})
	}
	ed = &edgeDrag{window: it.WindowID, item: it.ID, tab: e.model.CurrentTab(), start: e.clk.Now()}
	e.edge = ed
	ed.timer = e.after(edgePoll, func() { e.pollEdgeLocked(ed) })
	e.log.Debug("the user is dragging a docked window's edge", "window", it.WindowID, "item", it.ID, "frame", frame)
	return true
}

// pollEdgeLocked asks whether the mouse button is still held during a
// drag of a docked window's edge. Once it isn't, the drag ends with the
// window where it is now (endEdgeLocked). Past edgeLimit, the window comes
// out of its pane, as before: Bench may have missed the user letting go.
func (e *Engine) pollEdgeLocked(ed *edgeDrag) {
	if e.edge != ed {
		return
	}
	ed.timer = nil
	if e.clk.Now().Sub(ed.start) >= edgeLimit {
		e.stopEdgeLocked()
		it := e.model.Items[ed.item]
		if it != nil && it.WindowID == ed.window && e.model.CurrentTab() == ed.tab && e.dockedHereLocked(it) {
			e.undockLocked(it, layout.Rect{})
			return
		}
		e.arrangeLocked(arrangeOpts{})
		return
	}
	if e.p.MouseHeld() {
		ed.timer = e.after(edgePoll, func() { e.pollEdgeLocked(ed) })
		return
	}
	e.letGoEdgeLocked(ed)
}

// letGoEdgeLocked ends a drag of a docked window's edge once the mouse
// button is up, with the window where the platform says it is now
// (endEdgeLocked).
func (e *Engine) letGoEdgeLocked(ed *edgeDrag) {
	w, err := e.p.Window(ed.window)
	if err != nil {
		e.stopEdgeLocked()
		e.arrangeLocked(arrangeOpts{})
		return
	}
	e.endEdgeLocked(ed, w.Frame)
}

// otherEdgeLocked follows a move with the mouse held of docked window id
// while the user drags the edge of another docked window, and reports
// whether there was such a drag. There's one mouse. While its button is
// held, it's on the other window's edge, which can be dragged over this
// window, so the move is this window's app's, and the window goes back,
// shortly. Once the button is up, the user has let go of the edge before
// Bench next asked: the drag ends now, and the tab's windows, this one
// too, dock again.
func (e *Engine) otherEdgeLocked(id platform.WindowID) bool {
	ed := e.edge
	if ed == nil || ed.window == id {
		return false
	}
	if e.p.MouseHeld() {
		e.log.Debug("an app moved its docked window during a drag of another window's edge; putting it back", "window", id, "dragging", ed.window)
		e.redockSoonLocked(id)
		return true
	}
	e.letGoEdgeLocked(ed)
	return true
}

// inGraceLocked reports whether a move with the mouse held of item it's
// docked window, to frame, comes too soon after the user resized the panes
// to be theirs (dividerGrace). Just after the user dragged one of Bench's
// dividers, the mouse that's held down is Bench's, not on the window, so
// no such move is theirs. Just after a drag of a docked window's edge
// resized the panes, Bench has seen the user let go, so the press is a new
// one: a drag of edges that face other panes is theirs, as they change the
// sizes again. A move that would take the window out of its pane is put
// back, as the tab's windows have only just docked again, and an app can
// still be settling its window in its new pane.
func (e *Engine) inGraceLocked(it *Item, frame layout.Rect) bool {
	if e.clk.Now().Sub(e.sizedAt) >= dividerGrace {
		return false
	}
	if !e.edgeSized {
		return true
	}
	kind, _ := e.edgeChangeLocked(it, frame)
	return kind == edgeOut
}

// endEdgeLocked ends a drag of a docked window's edge as the user lets
// go, with the window at frame, in global coordinates. Inner edges move
// their dividers (resizeByEdgeLocked); a window that hasn't changed goes
// back where it was; and anything else comes out of its pane, as before.
// A window that has left the current tab, or its pane, since the drag
// began is only arranged again.
func (e *Engine) endEdgeLocked(ed *edgeDrag, frame layout.Rect) {
	e.stopEdgeLocked()
	it := e.model.Items[ed.item]
	if it == nil || it.WindowID != ed.window || e.model.CurrentTab() != ed.tab || !e.dockedHereLocked(it) {
		e.arrangeLocked(arrangeOpts{})
		return
	}
	switch kind, moves := e.edgeChangeLocked(it, frame); kind {
	case edgeInner:
		e.log.Debug("the user let go of a docked window's edge; resizing the panes", "window", ed.window, "item", ed.item, "frame", frame)
		e.resizeByEdgeLocked(moves)
	case edgeOut:
		e.undockLocked(it, frame)
	default:
		e.arrangeLocked(arrangeOpts{})
	}
}

// resizeByEdgeLocked moves the dividers that a dragged window's inner
// edges face to where the edges are, as dragging the dividers would,
// keeping each pane at layout.MinSize or more; docks the tab's windows
// again; and says so, with Undo. For dividerGrace, a move with the mouse
// held that would take a docked window out of its pane then doesn't count
// as the user's, as after the user drags a divider, though a drag of an
// inner edge does (inGraceLocked). If no divider can move, the window goes
// back, and the toast says that its pane can't change any more that way.
func (e *Engine) resizeByEdgeLocked(moves []edgeMove) {
	step := e.beginUndoLocked()
	resized := false
	var limit error
	for _, mv := range moves {
		sizes, ok := layout.SizesAt(mv.divider, mv.pos, e.cfg.Settings.Gap)
		if !ok || slices.EqualFunc(sizes, mv.divider.Sizes, nearSize) {
			limit = errPaneLimit(mv.divider.Dir, mv.grow)
			continue
		}
		if _, err := e.model.SetSizes(mv.divider.SplitID, sizes); err != nil {
			e.log.Debug("couldn't resize the panes for a dragged edge", "split", mv.divider.SplitID, "error", err)
			continue
		}
		resized = true
	}
	if resized {
		e.sizedAt, e.edgeSized = e.clk.Now(), true
	}
	e.arrangeLocked(arrangeOpts{})
	if !resized {
		if limit != nil {
			e.toastLocked(limit.Error())
		}
		return
	}
	e.changed()
	e.undoToastLocked("Resized the panes.", e.keepUndoLocked(step, resizePanesName, "Put back the sizes of the panes."))
}

// stopEdgeLocked forgets the drag of a docked window's edge that's under
// way, if any.
func (e *Engine) stopEdgeLocked() {
	ed := e.edge
	if ed == nil {
		return
	}
	e.edge = nil
	if ed.timer != nil {
		ed.timer.Stop()
		ed.timer = nil
	}
}

// edgeHeldLocked reports whether a displayed pane shows the window whose
// edge the user is dragging. Arranging leaves the pane's windows where they
// are until the user lets go, so that Bench doesn't pull the window from
// under the pointer.
func (e *Engine) edgeHeldLocked(d display, g paneGeom) bool {
	ed := e.edge
	return ed != nil && ed.tab == d.tab && g.node.Active == ed.item
}

// dragEdgeLocked follows the drag tracker's report (WindowDragged) of a
// drag that has changed the size of a docked window of the current tab, or
// of the window whose edge drag is under way, as onWindowMoved does, and
// reports whether it was one. The tracker follows only a press that went
// to the window itself, so the drag is the user's even when a move with
// the mouse held wouldn't count as theirs: just after they resized the
// panes (dividerGrace), or with the pointer run ahead of the window's
// edge, too far for MouseDown.
func (e *Engine) dragEdgeLocked(ev platform.Event) bool {
	if ev.ItemID != "" {
		return false
	}
	ed := e.edge
	held := ed != nil && ed.window == ev.Window.ID
	if !held && sameSize(ev.Start, ev.Window.Frame) {
		return false
	}
	it := e.model.ItemByWindow(ev.Window.ID)
	if it == nil || !e.dockedHereLocked(it) {
		return false
	}
	if !e.holdEdgeLocked(it, ev.Window.Frame) {
		e.undockLocked(it, ev.Window.Frame)
	}
	return true
}

// dockedHereLocked reports whether item it's window is docked in a pane
// of the current tab: an App window that isn't out of its pane, floating,
// minimised, or in full screen, which the windows manager has docked.
func (e *Engine) dockedHereLocked(it *Item) bool {
	if it.Kind != KindApp || it.WindowID == 0 || it.Undocked || it.minimised || it.Status == StatusFullScreen || e.model.IsFloating(it.ID) {
		return false
	}
	if ti, _ := e.model.Where(it.ID); ti != e.model.Current {
		return false
	}
	_, ok := e.win.Docked(it.WindowID)
	return ok
}

// undockLocked takes item it's window out of its pane, as the user has
// dragged it away, to frame: its pane says so, and arranging leaves the
// window where it is.
func (e *Engine) undockLocked(it *Item, frame layout.Rect) {
	if ed := e.edge; ed != nil && ed.window == it.WindowID {
		e.stopEdgeLocked()
	}
	e.log.Debug("the user dragged a window out of its pane", "window", it.WindowID, "item", it.ID, "frame", frame)
	it.Undocked = true
	it.Status, it.StatusText = StatusUndocked, undockedText
	e.win.Forget(it.WindowID)
	e.arrangeLocked(arrangeOpts{})
	e.changed()
}
