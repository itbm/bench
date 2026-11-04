package workspace

import (
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file puts windows that are out of the tabs into a pane: the window
// in front (AddFrontWindow), Bench's own popped-out web pages and terminals
// (BackToBench), and windows that the user drags onto a pane and rests there
// (drag_to_add). A window out of the tabs is a popped-out item, a window not
// in a tab, or one dragged out of its pane (decision 13 in
// docs/architecture.md).

// Timings for dragging windows onto Bench's panes.
const (
	// dropRest is how long the pointer has to rest on a pane during a drag
	// before the pane becomes the drop's target and its guide shows, so
	// that a window moved across Bench on its way somewhere else isn't
	// caught: the pane headers at the top of a Bench that fills the screen
	// are just where a window's title bar goes as it's moved to the top.
	dropRest = 500 * time.Millisecond
	// dropSlack is how far the pointer can wander, in points, while it
	// rests.
	dropSlack = 8
	// dragTimeout is how long a drag can go without an event before Bench
	// takes it that it missed the release, and hides the guide. The
	// platform reports a drag a few times a second while the pointer rests.
	dragTimeout = 2 * time.Second
)

// errNoFrontWindow refuses AddFrontWindow.
var errNoFrontWindow = errors.New("There's no window to add: the window you used last before Bench is in a tab already, or a rule keeps it floating. Click the window you want to add, then try again.")

// frontNote is the window in front: the one the user last gave the keyboard
// to outside Bench's main window, another app's window, or one of Bench's
// popped-out windows (item).
type frontNote struct {
	window platform.WindowID
	item   string
}

// dragState is a window that the user is dragging by its title bar
// (platform.WindowDragged).
type dragState struct {
	// key names the window as View.Front does: an item's ID, or
	// "f<window id>" for a window not in a tab. own is set for one of
	// Bench's own popped-out windows. window is another app's window, and
	// pid the process of the app it belongs to, Bench's for its own.
	key    string
	own    bool
	window platform.WindowID
	pid    int
	// pointer is where the pointer is, in global coordinates.
	pointer platform.Point
	// restPane is the pane the pointer rests on, restAt where it came to
	// rest, and restTimer arms the pane once it has rested for dropRest.
	restPane  string
	restAt    platform.Point
	restTimer clock.Timer
	// pane and zone are the drop's target once the pointer has rested on
	// a pane: pane is "" until then.
	pane string
	zone layout.Zone
	// timeout ends the drag if nothing is heard of it for dragTimeout.
	timeout clock.Timer
}

// dropTarget is where a window dropped now would go: a displayed pane of
// the current tab, as drawn (layout.CompactPaneID in the "tabs" view), the
// zone on it, and the guide that shows it.
type dropTarget struct {
	pane  string
	zone  layout.Zone
	guide platform.DropGuide
}

// noteFrontLocked records the window in front, which View.Front shows if
// it's out of the tabs.
func (e *Engine) noteFrontLocked(f frontNote) {
	if e.front != f {
		e.front = f
		e.touch()
	}
}

// frontLocked returns the window in front, named as View.Front names it,
// if it's out of the tabs and not one that a float rule keeps floating, or
// else "".
func (e *Engine) frontLocked() string {
	m := e.model
	out := func(it *Item) string {
		if it != nil && (m.IsFloating(it.ID) || it.Undocked) {
			return it.ID
		}
		return ""
	}
	switch f := e.front; {
	case f.item != "":
		return out(m.Items[f.item])
	case f.window == 0:
		return ""
	}
	if w, ok := e.unmanaged[e.front.window]; ok {
		if float, _ := matchRule(e.cfg.Rules, w.BundleID, w.Title); float {
			return ""
		}
		return floatingID(w.ID)
	}
	return out(m.ItemByWindow(e.front.window))
}

// AddFrontWindow adds the window in front (View.Front) to the current tab's
// focused pane, focuses it there, and says so, with Pop out, or Undo for a
// window dragged out of another pane: the add_front_window shortcut, Add
// front window to this pane in the jump palette, and Pane ▸ Add Front
// Window. A hidden Bench comes back to show it.
func (e *Engine) AddFrontWindow() error {
	e.lock()
	defer e.unlock()
	id := e.frontLocked()
	if id == "" {
		return errNoFrontWindow
	}
	t := e.model.CurrentTab()
	p := t.FocusedPane()
	if p == nil {
		return errNoPane
	}
	if err := e.dockAndSayLocked(id, t, p, layout.Center); err != nil {
		return err
	}
	if e.benchHidden {
		e.later(e.em.BringForward)
	}
	return nil
}

// BackToBench puts a popped-out item back in Bench, in the current tab's
// focused pane, brings Bench forward on it, and says so, with Pop out:
// Back to Bench in a popped-out web page's title bar or a terminal's
// window, and Window ▸ Back to Bench, which passes "" for the popped-out
// window that has the keyboard. An item that's in a tab already stays
// where it is.
func (e *Engine) BackToBench(itemID string) error {
	e.lock()
	defer e.unlock()
	return e.backToBenchLocked(itemID)
}

// backToBenchLocked is BackToBench with the lock held.
func (e *Engine) backToBenchLocked(itemID string) error {
	if itemID == "" {
		itemID = e.keyPopOut
	}
	m := e.model
	if m.Items[itemID] == nil {
		return errGone
	}
	if !m.IsFloating(itemID) {
		return nil
	}
	t := m.CurrentTab()
	p := t.FocusedPane()
	if p == nil {
		return errNoPane
	}
	if err := e.dockAndSayLocked(itemID, t, p, layout.Center); err != nil {
		return err
	}
	e.later(e.em.BringForward)
	return nil
}

// dockAndSayLocked docks a window that's out of the tabs in pane p of tab t
// (dockLocked), shows and focuses it there, and says so: with Pop out for a
// window that was out of Bench, and with Undo for one dragged out of
// another pane, which puts it back there.
func (e *Engine) dockAndSayLocked(id string, t *Tab, p *layout.Node, zone layout.Zone) error {
	m := e.model
	undocked := !strings.HasPrefix(id, floatingPrefix) && !m.IsFloating(id)
	again := undocked && m.PaneIn(m.IndexOf(t), id) == p && (zone == layout.Center || len(p.Items) < 2)
	step := e.beginUndoLocked()
	it, err := e.dockLocked(id, t, p, zone)
	if err != nil {
		return err
	}
	e.finishAddLocked(it)
	switch {
	case again:
		e.toastLocked("Docked " + e.labelOfLocked(it) + " again.")
	case undocked:
		e.undoToastLocked("Moved "+e.labelOfLocked(it)+" to this pane.", e.keepUndoLocked(step, "Moved "+e.labelOfLocked(it)+" back."))
	default:
		em, toast := e.em, Toast{
			Text:   "Added " + e.labelOfLocked(it) + " to this pane.",
			Action: ToastAction{Label: "Pop out", Method: "PopOut", Item: it.ID},
		}
		e.later(func() { em.ActionToast(toast) })
	}
	return nil
}

// dockLocked puts a window that's out of the tabs, named as View.Front
// names it, in pane p of tab t, at zone: in the middle, joining the pane,
// or at an edge, splitting it, unless the pane has nothing else in it. A
// popped-out item comes back into the tab (bringBackLocked), and a window
// not in a tab becomes an item. A window dragged out of its pane docks
// again, moving to p from its pane in t, if that's another, or joining p
// as well if it isn't in t; its places in other tabs stay as they are.
func (e *Engine) dockLocked(id string, t *Tab, p *layout.Node, zone layout.Zone) (*Item, error) {
	m := e.model
	ti := m.IndexOf(t)
	var it *Item
	if strings.HasPrefix(id, floatingPrefix) || m.IsFloating(id) {
		var err error
		if it, err = e.addFloatingLocked(t, p, id); err != nil {
			return nil, err
		}
		if e.keyPopOut == it.ID {
			e.keyPopOut = ""
		}
	} else {
		it = m.Items[id]
		if it == nil {
			return nil, errGone
		}
		if !it.Undocked {
			return nil, errNotUndocked
		}
		it.Undocked = false
		it.Status, it.StatusText = StatusReady, ""
		switch from := m.PaneIn(ti, id); {
		case from == nil:
			if _, err := m.AddToTab(id, ti, p.ID); err != nil {
				return nil, err
			}
		case from != p:
			if _, err := m.move(id, ti, p.ID, layout.Center, e.stackViewLocked(t)); err != nil {
				return nil, err
			}
		}
	}
	if zone != "" && zone != layout.Center && len(p.Items) > 1 {
		if _, err := m.move(it.ID, ti, p.ID, zone, e.stackViewLocked(t)); err != nil {
			// It stays in the pane, which it has joined already.
			e.log.Debug("couldn't split the pane for a dropped window", "item", it.ID, "pane", p.ID, "zone", zone, "error", err)
		}
	}
	return it, nil
}

// dropsAllowedLocked reports whether a window can be dropped onto Bench
// now: drag_to_add is on, and Bench's window is shown, and isn't raised,
// as it is while a menu or dialog is open.
func (e *Engine) dropsAllowedLocked() bool {
	return e.cfg.Settings.DragToAdd && !e.benchHidden && !e.overlay && !e.opts.Headless
}

// droppableLocked returns the window that a drag event is about, named as
// View.Front names it, and whether it's one of Bench's own popped-out
// windows, if it can be dropped onto a pane now: drops are allowed, the
// drag hasn't changed its size, as a drag from an edge does, and it's out
// of the tabs, standard, and not one that a float rule keeps floating. It
// reports false otherwise. The event doesn't say whether the window is in
// full screen: dockableLocked asks, once the pointer has rested.
func (e *Engine) droppableLocked(ev platform.Event) (key string, own, ok bool) {
	if !e.dropsAllowedLocked() || !sameSize(ev.Start, ev.Window.Frame) {
		return "", false, false
	}
	m := e.model
	if ev.ItemID != "" {
		it := m.Items[ev.ItemID]
		return ev.ItemID, true, it != nil && it.OwnWindow && m.IsFloating(it.ID)
	}
	w := ev.Window
	if !e.trusted || w.ID == 0 {
		return "", false, false
	}
	if u, free := e.unmanaged[w.ID]; free {
		if float, _ := matchRule(e.cfg.Rules, u.BundleID, u.Title); float || !u.Standard() {
			return "", false, false
		}
		return floatingID(w.ID), false, true
	}
	if it := m.ItemByWindow(w.ID); it != nil && (m.IsFloating(it.ID) || it.Undocked) {
		return it.ID, false, true
	}
	return "", false, false
}

// sameSize reports whether two frames have the same size, give or take
// rounding.
func sameSize(a, b layout.Rect) bool {
	return math.Abs(a.W-b.W) < 1 && math.Abs(a.H-b.H) < 1
}

// onWindowDragged follows a window that the user drags by its title bar.
// Once the pointer has rested on a pane of the current tab for dropRest,
// the pane is the drop's target, and the drop guide shows over it; while
// the pointer stays on that pane, the target follows it, from the middle to
// the edges. A window that can't be dropped in has no target.
func (e *Engine) onWindowDragged(ev platform.Event) {
	key, own, ok := e.droppableLocked(ev)
	if !ok {
		e.endDragLocked()
		return
	}
	d := e.drag
	if d == nil || d.key != key {
		e.endDragLocked()
		d = &dragState{key: key, own: own, window: ev.Window.ID, pid: ev.Window.PID}
		if own {
			d.window, d.pid = 0, e.ownPID
		}
		e.drag = d
	}
	d.pointer = ev.Pointer
	if d.timeout != nil {
		d.timeout.Stop()
	}
	// A timer that has fired as this event came in waits for the lock, and
	// Stop can't hold it back: it does nothing unless it's still the
	// drag's.
	var timeout clock.Timer
	timeout = e.after(dragTimeout, func() {
		if e.drag == d && d.timeout == timeout {
			e.log.Debug("a drag went quiet; hiding the drop guide", "window", d.key)
			e.endDragLocked()
		}
	})
	d.timeout = timeout
	target, ok := e.dropTargetAtLocked(key, ev.Pointer)
	switch {
	case !ok:
		e.disarmLocked(d)
		e.stopRestLocked(d)
	case target.pane == d.pane:
		if target.zone != d.zone {
			d.zone = target.zone
			e.p.ShowDropGuide(target.guide)
		}
	default:
		e.disarmLocked(d)
		e.restLocked(d, target.pane, ev.Pointer)
	}
}

// restLocked notes the pointer on a pane during a drag, and arms the pane
// once the pointer has rested there for dropRest, within dropSlack.
func (e *Engine) restLocked(d *dragState, pane string, at platform.Point) {
	if d.restTimer != nil && d.restPane == pane && math.Hypot(at.X-d.restAt.X, at.Y-d.restAt.Y) <= dropSlack {
		return
	}
	e.stopRestLocked(d)
	d.restPane, d.restAt = pane, at
	// As with the drag's timeout, a timer that fired as the pointer moved
	// on does nothing: the pane it would arm hasn't had its rest.
	var rest clock.Timer
	rest = e.after(dropRest, func() {
		if d.restTimer != rest {
			return
		}
		d.restTimer = nil
		if e.drag == d && e.dropsAllowedLocked() {
			e.armLocked(d)
		}
	})
	d.restTimer = rest
}

// stopRestLocked forgets where the pointer rested.
func (e *Engine) stopRestLocked(d *dragState) {
	if d.restTimer != nil {
		d.restTimer.Stop()
		d.restTimer = nil
	}
	d.restPane = ""
}

// armLocked makes the pane the pointer rested on the drop's target, if the
// pointer is still on it and the window can join it (dockableLocked), and
// shows the drop guide over it.
func (e *Engine) armLocked(d *dragState) {
	target, ok := e.dropTargetAtLocked(d.key, d.pointer)
	if !ok || target.pane != d.restPane || !e.dockableLocked(d) {
		return
	}
	d.pane, d.zone = target.pane, target.zone
	e.p.ShowDropGuide(target.guide)
}

// dockableLocked reports whether the window a drag is about can join a
// pane: one of Bench's own, or another app's that isn't in full screen, as
// in a Split View, nor minimised. Drag events don't say, so the platform is
// asked as a pane is armed and at the drop, not on every event.
func (e *Engine) dockableLocked(d *dragState) bool {
	if d.own {
		return true
	}
	w, err := e.p.Window(d.window)
	return err == nil && !w.FullScreen && !w.Minimised
}

// disarmLocked takes away the drop's target, and the guide, if there is
// one.
func (e *Engine) disarmLocked(d *dragState) {
	if d.pane != "" {
		d.pane, d.zone = "", ""
		e.p.HideDropGuide()
	}
}

// endDragLocked forgets the drag under way, if any, and hides the drop
// guide: when the drag ends or goes quiet, and when Bench is hidden,
// raised, or another app becomes active.
func (e *Engine) endDragLocked() {
	d := e.drag
	if d == nil {
		return
	}
	e.drag = nil
	e.stopRestLocked(d)
	if d.timeout != nil {
		d.timeout.Stop()
	}
	e.disarmLocked(d)
}

// onWindowDropped docks a window that the user has let go of over the pane
// that the drag made its target, at the zone the pointer is on, if it can
// still be dropped in there (droppableLocked): a window let go of anywhere
// else, or before the pointer rested on a pane, stays where it is. Bench
// comes forward for one of its own popped-out windows.
func (e *Engine) onWindowDropped(ev platform.Event) {
	d := e.drag
	if d == nil {
		return
	}
	armed := d.pane
	e.endDragLocked()
	key, own, ok := e.droppableLocked(ev)
	if !ok || d.key != key || armed == "" {
		return
	}
	target, ok := e.dropTargetAtLocked(key, ev.Pointer)
	if !ok || target.pane != armed || !e.dockableLocked(d) {
		return
	}
	t := e.model.CurrentTab()
	p := t.Pane(target.pane)
	if target.pane == layout.CompactPaneID {
		p = t.FocusedPane()
	}
	if p == nil {
		return
	}
	e.log.Debug("the user dropped a window onto a pane", "window", key, "pane", p.ID, "zone", target.zone)
	if err := e.dockAndSayLocked(key, t, p, target.zone); err != nil {
		e.toastLocked(err.Error())
		return
	}
	if own {
		e.later(e.em.BringForward)
	}
}

// dropTargetAtLocked returns where the window key, named as View.Front
// names it, would go if dropped with the pointer at a point, in global
// coordinates: the displayed pane of the current tab under the pointer, and
// the zone on it, as layout.DropAt finds for a sub-tab dragged there, the
// header joining the pane as its middle does. A pane splits only if it
// holds another window, as dockLocked does, and never in the "tabs" view.
// It reports false when the pointer isn't over a pane.
func (e *Engine) dropTargetAtLocked(key string, at platform.Point) (dropTarget, bool) {
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return dropTarget{}, false
	}
	x, y := at.X-content.X, at.Y-content.Y
	d := e.displayLocked()
	for _, g := range d.panes {
		f := g.frame
		if x < f.X || x >= f.X+f.W || y < f.Y || y >= f.Y+f.H {
			continue
		}
		splits := !d.compact && e.holdsOtherLocked(d.tab, g.node, key)
		zone := layout.DropAt(f, g.header, x, y, splits).Zone
		global := func(r layout.Rect) layout.Rect {
			return layout.Rect{X: content.X + r.X, Y: content.Y + r.Y, W: r.W, H: r.H}
		}
		return dropTarget{pane: g.node.ID, zone: zone, guide: platform.DropGuide{
			Pane: global(f), Body: global(g.body), Target: global(dropArea(g.body, zone)),
			Zone: string(zone), Splits: splits, Label: dropLabel(zone),
		}}, true
	}
	return dropTarget{}, false
}

// holdsOtherLocked reports whether a displayed pane of tab t holds a window
// other than key, which a window dropped at its edge would split it from.
func (e *Engine) holdsOtherLocked(t *Tab, n *layout.Node, key string) bool {
	if p := t.Pane(n.ID); p != nil {
		n = p
	}
	return slices.ContainsFunc(n.Items, func(id string) bool { return id != key })
}

// dropArea returns the part of a pane's body that a window dropped at zone
// takes: all of it for the middle, or the half on the side of an edge, which
// splitting the pane gives it.
func dropArea(body layout.Rect, zone layout.Zone) layout.Rect {
	half := body
	switch zone {
	case layout.Left:
		half.W = body.W / 2
	case layout.Right:
		half.W = body.W / 2
		half.X = body.X + half.W
	case layout.Top:
		half.H = body.H / 2
	case layout.Bottom:
		half.H = body.H / 2
		half.Y = body.Y + half.H
	}
	return half
}

// dropLabel says what dropping a window at zone does, for the drop guide.
func dropLabel(zone layout.Zone) string {
	switch zone {
	case layout.Left:
		return "New pane on the left"
	case layout.Right:
		return "New pane on the right"
	case layout.Top:
		return "New pane above"
	case layout.Bottom:
		return "New pane below"
	}
	return "Add to this pane"
}

// dragTrackingChangedLocked returns whether the platform should watch the
// user drag windows (Platform.TrackDrags), and whether that differs from
// what it was last told: while drag_to_add is on, once Bench has started.
func (e *Engine) dragTrackingChangedLocked() (bool, bool) {
	want := e.started && e.cfg.Settings.DragToAdd && !e.opts.Headless
	if e.trackingSent && want == e.tracking {
		return want, false
	}
	e.tracking, e.trackingSent = want, true
	return want, true
}
