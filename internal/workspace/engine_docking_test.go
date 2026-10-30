package workspace

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// middleOf returns the middle of a rectangle, as a pointer there.
func middleOf(r layout.Rect) platform.Point {
	return platform.Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
}

// lastAction returns the latest toast's button.
func (r *recEmitter) lastAction() ToastAction {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.actions) == 0 {
		return ToastAction{}
	}
	return r.actions[len(r.actions)-1]
}

// menuBarNow returns what the menu bar is told once the view goes out.
func (h *harness) menuBarNow() MenuBar {
	h.t.Helper()
	h.advance(frame)
	return h.lastMenuBar()
}

// dragTo drags a window by its title bar so that the pointer is at a point,
// keeping its size, as the user would.
func (h *harness) dragTo(id platform.WindowID, at platform.Point) {
	h.t.Helper()
	f := h.frame(id)
	f.X, f.Y = at.X-f.W/2, at.Y-10
	if err := h.p.DragWindow(id, f, at); err != nil {
		h.t.Fatal(err)
	}
	h.drain()
}

// drop lets go of a window that dragTo dragged.
func (h *harness) drop(id platform.WindowID, at platform.Point) {
	h.t.Helper()
	if err := h.p.DropWindow(id, at); err != nil {
		h.t.Fatal(err)
	}
	h.drain()
}

// guide returns the drop guide showing, failing the test if none is.
func (h *harness) guide() platform.DropGuide {
	h.t.Helper()
	g, ok := h.p.DropGuideShown()
	if !ok {
		h.t.Fatal("no drop guide shows")
	}
	return g
}

func TestAddFrontWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusPane("p12"))
	finder := h.windowID(finderID, "Downloads")
	if v := h.view(); v.Front != "" {
		t.Errorf("Front = %q before any window outside Bench was focused", v.Front)
	}
	mustErr(t, h.e.AddFrontWindow(), errNoFrontWindow.Error())

	mustOK(t, h.p.FocusWindow(finder))
	h.drain()
	if v := h.view(); v.Front != floatingID(finder) {
		t.Fatalf("Front = %q, want the Finder window, which isn't in a tab", v.Front)
	}
	if !h.menuBarNow().AddFront {
		t.Error("Pane ▸ Add Front Window should be on while the window in front is out of the tabs")
	}
	h.e.Shortcut(shortcuts.ActionAddFrontWindow, "")
	id := h.itemOf(finder)
	if id == "" || h.paneOf(id) != "p12" || h.frame(finder) != h.bodyOf("p12") {
		t.Fatalf("Finder: item %q in pane %q, frame %v; want docked in p12", id, h.paneOf(id), h.frame(finder))
	}
	if got := h.em.lastToast(); got != "Added Finder — Downloads to this pane." {
		t.Errorf("toast %q", got)
	}
	if a := h.em.lastAction(); a != (ToastAction{Label: "Pop out", Method: "PopOut", Item: id}) {
		t.Errorf("the toast's button = %+v, want Pop out for the new item", a)
	}
	if v := h.view(); v.Front != "" || v.Display.Panes[1].Active != id {
		t.Errorf("Front = %q, p12 shows %q; want nothing in front out of the tabs, and Finder shown", v.Front, v.Display.Panes[1].Active)
	}
	if h.menuBarNow().AddFront {
		t.Error("Pane ▸ Add Front Window should be off once the window is in a pane")
	}
	mustErr(t, h.e.AddFrontWindow(), errNoFrontWindow.Error())

	// The toast's Pop out takes it out again, and it's in front still.
	mustOK(t, h.e.PopOut(id))
	if v := h.view(); v.Front != id {
		t.Errorf("Front = %q after Pop out, want %q", v.Front, id)
	}
}

func TestAddFrontWindowIgnoresBenchsOwnFocusAndFloatRules(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	mustOK(t, h.p.FocusWindow(finder))
	h.drain()
	// Bench focusing the pane's window, as it does when the pane is
	// clicked, isn't the user choosing a window in front.
	mustOK(t, h.e.FocusPane("p11"))
	h.drain()
	if v := h.view(); v.Front != floatingID(finder) {
		t.Errorf("Front = %q after Bench focused a docked window, want the Finder window still", v.Front)
	}
	pip, err := h.p.OpenWindow(braveID, "Picture in picture")
	mustOK(t, err)
	h.drain()
	mustOK(t, h.p.FocusWindow(pip.ID))
	h.drain()
	if v := h.view(); v.Front != "" {
		t.Errorf("Front = %q for a window that a float rule keeps floating, want none", v.Front)
	}
	mustErr(t, h.e.AddFrontWindow(), errNoFrontWindow.Error())
}

func TestAddFrontWindowDocksAWindowDraggedOut(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.settle()
	mustOK(t, h.p.FocusWindow(zed))
	h.settle()
	f := h.frame(zed)
	f.X += 200
	mustOK(t, h.p.UserMoveWindow(zed, f))
	h.drain()
	if !h.e.model.Items["w3"].Undocked {
		t.Fatal("Zed should be dragged out of its pane")
	}
	if v := h.view(); v.Front != "w3" {
		t.Fatalf("Front = %q, want w3, dragged out of its pane", v.Front)
	}
	mustOK(t, h.e.FocusPane("p12"))
	mustOK(t, h.e.AddFrontWindow())
	it := h.item("w3")
	if it.Pane != "p12" || it.Status != StatusReady || h.frame(zed) != h.bodyOf("p12") {
		t.Fatalf("w3: pane %s, status %s, frame %v; want docked in p12", it.Pane, it.Status, h.frame(zed))
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to this pane." {
		t.Errorf("toast %q", got)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if h.paneOf("w3") != "p11" {
		t.Errorf("Undo should put w3 back in p11, not %s", h.paneOf("w3"))
	}
}

func TestBackToBench(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w1"))
	if h.menuBarNow().BackToBench {
		t.Error("Window ▸ Back to Bench should be off while no popped-out window has the keyboard")
	}
	h.p.FocusPopOut("w1", true)
	h.drain()
	if !h.menuBarNow().BackToBench {
		t.Error("Window ▸ Back to Bench should be on while the popped-out page has the keyboard")
	}
	if v := h.view(); v.Front != "w1" {
		t.Errorf("Front = %q, want the popped-out page", v.Front)
	}
	forwards := h.em.broughtForward()
	mustOK(t, h.e.BackToBench(""))
	if it := h.item("w1"); it.Tab != 0 || it.Pane != "p9" {
		t.Fatalf("w1: tab %d pane %s, want the Comms tab's focused pane", it.Tab, it.Pane)
	}
	if wv := h.p.WebViews()["w1"]; wv.PoppedOut || !wv.Visible {
		t.Errorf("web view: %+v, want docked and shown", wv)
	}
	if h.em.broughtForward() != forwards+1 {
		t.Error("Back to Bench should bring Bench forward")
	}
	if a := h.em.lastAction(); a.Method != "PopOut" || a.Item != "w1" {
		t.Errorf("the toast's button = %+v, want Pop out", a)
	}
	if h.menuBarNow().BackToBench {
		t.Error("Window ▸ Back to Bench should be off once the page is back")
	}
	// Again, for an item in a tab, does nothing.
	mustOK(t, h.e.BackToBench("w1"))
	mustErr(t, h.e.BackToBench("w99"), errGone.Error())

	// The button in a popped-out terminal's window.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.PopOut("w5"))
	h.p.PressBackToBench("w5")
	h.drain()
	if it := h.item("w5"); it.Tab != 1 {
		t.Errorf("w5: tab %d, want back in Code", it.Tab)
	}
	if !slices.Contains(h.em.closed, "w5") {
		t.Errorf("the terminal's own window should close: %v", h.em.closed)
	}
}

func TestDragToAddWaitsForTheRest(t *testing.T) {
	h := newHarness(t, setup{})
	if !h.p.TrackingDrags() {
		t.Fatal("Bench should watch drags while drag_to_add is on")
	}
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	at := middleOf(h.bodyOf("p12"))

	// Let go at once: it stays out of the tabs.
	h.dragTo(finder, at)
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("the guide shows before the pointer has rested")
	}
	h.drop(finder, at)
	if h.itemOf(finder) != "" {
		t.Fatal("a window let go of without resting on a pane was added")
	}

	// Moving on resets the wait.
	h.dragTo(finder, at)
	h.advance(dropRest - 100*time.Millisecond)
	h.dragTo(finder, platform.Point{X: at.X + 40, Y: at.Y})
	h.advance(200 * time.Millisecond)
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("the guide shows though the pointer moved on")
	}
	h.advance(dropRest)
	g := h.guide()
	p12 := h.view().Display.Panes[1]
	cf := h.p.ContentFrame()
	wantPane := layout.Rect{X: cf.X + p12.Rect.X, Y: cf.Y + p12.Rect.Y, W: p12.Rect.W, H: p12.Rect.H}
	if g.Zone != "center" || g.Pane != wantPane || g.Target != h.bodyOf("p12") || !g.Splits || g.Label != "Add to this pane" {
		t.Errorf("guide = %+v, want p12's middle", g)
	}
	h.drop(finder, platform.Point{X: at.X + 40, Y: at.Y})
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("the guide still shows after the drop")
	}
	id := h.itemOf(finder)
	if id == "" || h.paneOf(id) != "p12" || h.frame(finder) != h.bodyOf("p12") {
		t.Fatalf("Finder: item %q pane %q frame %v; want docked in p12", id, h.paneOf(id), h.frame(finder))
	}
	if got := h.em.lastToast(); got != "Added Finder — Downloads to this pane." {
		t.Errorf("toast %q", got)
	}
	if a := h.em.lastAction(); a.Method != "PopOut" || a.Item != id {
		t.Errorf("the toast's button = %+v, want Pop out", a)
	}
}

func TestDragToAddSplitsAtAnEdge(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	body := h.bodyOf("p11")
	h.dragTo(finder, middleOf(body))
	h.advance(dropRest)
	if g := h.guide(); g.Zone != "center" {
		t.Fatalf("guide zone %q, want center", g.Zone)
	}
	// Once armed, the target follows the pointer on the pane at once.
	left := platform.Point{X: body.X + body.W/10, Y: body.Y + body.H/2}
	h.dragTo(finder, left)
	g := h.guide()
	if g.Zone != "left" || g.Target != (layout.Rect{X: body.X, Y: body.Y, W: body.W / 2, H: body.H}) || g.Label != "New pane on the left" {
		t.Errorf("guide = %+v, want the left half of p11", g)
	}
	h.drop(finder, left)
	id := h.itemOf(finder)
	v := h.view()
	if id == "" || len(v.Display.Panes) != 3 || v.Display.Panes[0].Items[0] != id {
		t.Fatalf("Finder %q should be in a new pane left of p11: %+v", id, v.Display.Panes)
	}
	if h.paneOf("w3") != "p11" {
		t.Errorf("Zed should stay in p11, not %s", h.paneOf("w3"))
	}
}

func TestDragThatResizesIsNoDrop(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	at := middleOf(h.bodyOf("p12"))
	f := h.frame(finder)
	f.Y, f.H = at.Y-10, f.H+60 // The top edge, dragged down.
	mustOK(t, h.p.DragWindow(finder, f, at))
	h.drain()
	h.advance(dropRest * 2)
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("a resize from the top edge shows the guide")
	}
	h.drop(finder, at)
	if h.itemOf(finder) != "" {
		t.Error("a resize from the top edge added the window")
	}
}

func TestDragOfAWindowThatFloats(t *testing.T) {
	h := newHarness(t, setup{})
	pip, err := h.p.OpenWindow(braveID, "Picture in picture")
	mustOK(t, err)
	h.drain()
	at := middleOf(h.bodyOf("p9"))
	h.dragTo(pip.ID, at)
	h.advance(dropRest)
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("a window that a float rule keeps floating shows the guide")
	}
	h.drop(pip.ID, at)
	if h.itemOf(pip.ID) != "" {
		t.Error("a window that a float rule keeps floating was added")
	}
}

func TestDragGuideGoes(t *testing.T) {
	for _, tc := range []struct {
		name string
		do   func(h *harness)
	}{
		{"when the drag goes quiet", func(h *harness) { h.advance(dragTimeout) }},
		{"when Bench is hidden", func(h *harness) { h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden}); h.drain() }},
		{"when Bench is raised", func(h *harness) { mustOK(t, h.e.SetOverlay(true)) }},
		{"when another app becomes active", func(h *harness) { mustOK(t, h.p.ActivateApp(obsidianID)); h.drain() }},
		{"when the pointer leaves the panes", func(h *harness) {
			h.dragTo(h.windowID(finderID, "Downloads"), platform.Point{X: 10, Y: 1430})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			mustOK(t, h.e.SwitchTab(1))
			finder := h.windowID(finderID, "Downloads")
			at := middleOf(h.bodyOf("p12"))
			h.dragTo(finder, at)
			h.advance(dropRest)
			h.guide()
			tc.do(h)
			if _, ok := h.p.DropGuideShown(); ok {
				t.Fatal("the guide still shows")
			}
			h.drop(finder, at)
			if h.itemOf(finder) != "" {
				t.Error("the window was added though the drag had ended")
			}
		})
	}
}

func TestDragToAddTurnedOff(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.DragToAdd = false
	mustOK(t, h.e.SaveSettings(s))
	h.advance(frame)
	if h.p.TrackingDrags() {
		t.Error("Bench still watches drags with drag_to_add off")
	}
	// Events that come anyway are ignored.
	h.p.TrackDrags(true)
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	at := middleOf(h.bodyOf("p12"))
	h.dragTo(finder, at)
	h.advance(dropRest)
	h.drop(finder, at)
	if _, ok := h.p.DropGuideShown(); ok || h.itemOf(finder) != "" {
		t.Error("a drag with drag_to_add off did something")
	}
	if got := h.e.cfg.Settings.DragToAdd; got {
		t.Error("the setting didn't change")
	}
}

func TestDragPopOutBack(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w1"))
	from := layout.Rect{X: 300, Y: 300, W: 800, H: 600}
	at := middleOf(h.bodyOf("p9"))
	to := layout.Rect{X: at.X - 400, Y: at.Y - 10, W: 800, H: 600}
	h.p.DragPopOut("w1", from, to, at)
	h.drain()
	h.advance(dropRest)
	if g := h.guide(); g.Zone != "center" {
		t.Errorf("guide zone %q", g.Zone)
	}
	forwards := h.em.broughtForward()
	h.p.DropPopOut("w1", to, at)
	h.drain()
	if it := h.item("w1"); it.Tab != 0 || it.Pane != "p9" {
		t.Fatalf("w1: tab %d pane %q, want back in p9", it.Tab, it.Pane)
	}
	if !h.called("DockWebView", func(c platform.Call) bool { return c.Arg == "w1" }) {
		t.Error("the page's view should dock in Bench's window again")
	}
	if h.em.broughtForward() != forwards+1 {
		t.Error("Bench should come forward after one of its own windows is dropped in")
	}
}

func TestDragDockedWindowToAnotherPane(t *testing.T) {
	h := newHarness(t, setup{})
	// Zed is in Research as well.
	mustOK(t, h.e.SwitchTab(2))
	research := h.view().Display.Focus
	mustOK(t, h.e.AddToPane(research, Source{Type: "item", ID: "w3"}))
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.settle()
	zed := h.windowID(zedID, "bench — switch.go")
	at := middleOf(h.bodyOf("p12"))
	// The first move, with the mouse held, drags it out of its pane.
	h.dragTo(zed, at)
	if !h.e.model.Items["w3"].Undocked {
		t.Fatal("Zed should be dragged out of its pane")
	}
	h.advance(dropRest)
	h.guide()
	h.drop(zed, at)
	if it := h.item("w3"); it.Pane != "p12" || it.Status != StatusReady || h.frame(zed) != h.bodyOf("p12") {
		t.Fatalf("w3: pane %s status %s frame %v; want docked in p12", it.Pane, it.Status, h.frame(zed))
	}
	if tabs := h.item("w3").Tabs; !slices.Contains(tabs, 2) {
		t.Errorf("w3 should still be in Research too: tabs %v", tabs)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if h.paneOf("w3") != "p11" {
		t.Errorf("Undo should put w3 back in p11, not %s", h.paneOf("w3"))
	}
}

// lateClock is a fake clock that keeps the functions its timers run, so
// that a test can run one after its timer was stopped, as a real timer's
// does when it fires just as it's stopped, and then waits for the lock.
type lateClock struct {
	*clock.Fake
	mu    sync.Mutex
	waits map[time.Duration][]func()
}

func (c *lateClock) AfterFunc(d time.Duration, f func()) clock.Timer {
	c.mu.Lock()
	c.waits[d] = append(c.waits[d], f)
	c.mu.Unlock()
	return c.Fake.AfterFunc(d, f)
}

// last returns the function of the latest timer set for d.
func (c *lateClock) last(d time.Duration) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	fs := c.waits[d]
	return fs[len(fs)-1]
}

func TestDragRestThatFiresLateArmsNothing(t *testing.T) {
	var clk *lateClock
	h := newHarness(t, setup{options: func(o *Options) {
		clk = &lateClock{Fake: o.Clock.(*clock.Fake), waits: map[time.Duration][]func(){}}
		o.Clock = clk
	}})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	h.dragTo(finder, middleOf(h.bodyOf("p11")))
	h.advance(dropRest - 10*time.Millisecond)
	late, quiet := clk.last(dropRest), clk.last(dragTimeout)
	// The pointer moves on to p12 just as the wait on p11 ends: its timer
	// has fired, and runs once the move is through.
	at := middleOf(h.bodyOf("p12"))
	h.dragTo(finder, at)
	late()
	quiet()
	if _, ok := h.p.DropGuideShown(); ok {
		t.Fatal("p12 armed without the pointer resting on it")
	}
	if h.e.drag == nil {
		t.Fatal("the drag ended though it isn't quiet")
	}
	// p12's own wait still arms it.
	h.advance(dropRest)
	if g := h.guide(); g.Pane.X != h.p.ContentFrame().X+h.view().Display.Panes[1].Rect.X {
		t.Errorf("guide = %+v, want p12", g)
	}
	h.drop(finder, at)
	if h.paneOf(h.itemOf(finder)) != "p12" {
		t.Error("Finder wasn't added to p12")
	}
}

func TestDragToAnEmptyPaneOnlyJoinsIt(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	empty, err := h.e.SplitPane("p12", "right")
	mustOK(t, err)
	h.settle()
	finder := h.windowID(finderID, "Downloads")
	body := h.bodyOf(empty)
	left := platform.Point{X: body.X + body.W/10, Y: body.Y + body.H/2}
	h.dragTo(finder, left)
	h.advance(dropRest)
	if g := h.guide(); g.Splits || g.Zone != "center" || g.Target != body || g.Label != "Add to this pane" {
		t.Errorf("guide = %+v; an empty pane can only be joined", g)
	}
	h.drop(finder, left)
	id := h.itemOf(finder)
	if id == "" || h.paneOf(id) != empty || h.frame(finder) != body {
		t.Fatalf("Finder: item %q pane %q frame %v; want all of the empty pane", id, h.paneOf(id), h.frame(finder))
	}

	// Dragged out and back to its own pane's edge, it's on its own there:
	// it docks again, filling the pane.
	h.settle()
	h.dragTo(finder, left)
	if !h.e.model.Items[id].Undocked {
		t.Fatal("Finder should be dragged out of its pane")
	}
	h.advance(dropRest)
	if g := h.guide(); g.Splits || g.Label != "Add to this pane" {
		t.Errorf("guide = %+v over the dragged window's own pane", g)
	}
	h.drop(finder, left)
	if h.paneOf(id) != empty || len(h.view().Display.Panes) != 3 {
		t.Errorf("Finder should dock in its pane again, not split it: %+v", h.view().Display.Panes)
	}
}

func TestDragOfAWindowInFullScreen(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	at := middleOf(h.bodyOf("p12"))

	// The window goes into full screen while it's held, as in a Split
	// View: its pane isn't armed, though the drag doesn't say.
	h.dragTo(finder, at)
	mustOK(t, h.p.SetFullScreen(finder, true))
	h.drain()
	h.advance(dropRest)
	if _, ok := h.p.DropGuideShown(); ok {
		t.Error("a window in full screen shows the guide")
	}
	h.drop(finder, at)
	if h.itemOf(finder) != "" {
		t.Fatal("a window in full screen was added")
	}

	// Nor is one that goes into full screen once its pane is armed.
	mustOK(t, h.p.SetFullScreen(finder, false))
	h.drain()
	h.dragTo(finder, at)
	h.advance(dropRest)
	h.guide()
	mustOK(t, h.p.SetFullScreen(finder, true))
	h.drain()
	h.drop(finder, at)
	if h.itemOf(finder) != "" {
		t.Error("a window that went into full screen as it was dropped was added")
	}
}

func TestDragOutlastsItsOwnAppComingForward(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	at := middleOf(h.bodyOf("p12"))
	h.dragTo(finder, at)
	h.advance(dropRest)
	h.guide()
	// Pressing the window's title bar made Finder active, but Bench hears
	// of it late.
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	h.guide()
	h.drop(finder, at)
	if h.paneOf(h.itemOf(finder)) != "p12" {
		t.Error("Finder wasn't added to p12")
	}
}
