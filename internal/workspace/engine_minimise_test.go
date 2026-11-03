package workspace

import (
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// minimised reports whether a window is minimised.
func (h *harness) minimised(id platform.WindowID) bool {
	h.t.Helper()
	w, ok := h.p.WindowState(id)
	if !ok {
		h.t.Fatalf("no window %d", id)
	}
	return w.Minimised
}

// drainFocusFirst feeds the platform's waiting events to the engine with
// the focus events first, as macOS often sends them when the user
// minimises a window: it gives the app's next window the keyboard as the
// minimise starts, and says the window is minimised once it's in the Dock.
func (h *harness) drainFocusFirst() {
	evs := h.p.DrainEvents()
	for _, focus := range []bool{true, false} {
		for _, ev := range evs {
			if (ev.Kind == platform.WindowFocused) == focus {
				h.e.handle(ev)
			}
		}
	}
	h.drain()
}

func TestMinimisedWindowStaysInTheDock(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	windows := h.view().Tabs[1].Windows
	h.clk.Advance(time.Second)

	// The user presses Command-M in Zed: its sub-tab says so, and the pane
	// shows VS Code, docked in Zed's place.
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	if it := h.item("w3"); it.Status != StatusMinimised || it.StatusText != "Minimised" {
		t.Fatalf("w3: %s %q, want minimised", it.Status, it.StatusText)
	}
	if p := h.paneView("p11"); p.Active != "w4" || !slices.Equal(p.Items, []string{"w3", "w4"}) {
		t.Errorf("p11 shows %s of %v, want VS Code, with Zed's sub-tab still there", p.Active, p.Items)
	}
	if h.frame(code) != h.bodyOf("p11") || h.e.win.IsParked(code) {
		t.Errorf("VS Code is at %v, want docked at %v", h.frame(code), h.bodyOf("p11"))
	}
	if v := h.view(); v.Tabs[1].Windows != windows {
		t.Errorf("the tab counts %d windows, want Zed's still among %d", v.Tabs[1].Windows, windows)
	}

	// Nothing that arranges the tab brings it back: a sub-tab click, a
	// focus change, Bench moving, a tab switch, Rearrange, or cycling
	// through the sub-tabs to it.
	h.p.ResetCalls()
	mustOK(t, h.e.ShowItem("p11", "w4"))
	mustOK(t, h.e.FocusPane("p12"))
	h.p.FocusWindow(code)
	h.drain()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.drain()
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.Rearrange())
	mustOK(t, h.e.FocusPane("p11"))
	h.settle()
	if !h.minimised(zed) || len(h.calls("Unminimise")) != 0 {
		t.Fatalf("Zed minimised %v, Unminimise calls %v: arranging brought it back", h.minimised(zed), h.calls("Unminimise"))
	}
	// Cycling shows its sub-tab, which says it's minimised, and VS Code
	// goes out of sight so that the pane's message shows. Bench takes the
	// keyboard from VS Code, for Show again.
	h.p.ResetCalls()
	mustOK(t, h.e.CycleSubtab(1))
	h.settle()
	if p := h.paneView("p11"); p.Active != "w3" || h.item("w3").Status != StatusMinimised {
		t.Errorf("after cycling, p11 shows %s, w3 %s", p.Active, h.item("w3").Status)
	}
	if !h.minimised(zed) || len(h.calls("Unminimise")) != 0 {
		t.Errorf("cycling brought Zed back: minimised %v", h.minimised(zed))
	}
	if !h.e.win.IsParked(code) {
		t.Errorf("VS Code should be parked while the pane shows Zed's message")
	}
	if len(h.focusCalls()) != 0 || len(h.calls("FocusUI")) != 1 {
		t.Errorf("Focus calls %v, FocusUI calls %d; want Bench to take the keyboard", h.focusCalls(), len(h.calls("FocusUI")))
	}

	// Clicking its sub-tab (or Show again, or Jump to…) brings it back,
	// docked and focused.
	h.p.ResetCalls()
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.drain()
	if h.minimised(zed) || h.item("w3").Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("after ShowItem, Zed minimised %v, status %s, at %v", h.minimised(zed), h.item("w3").Status, h.frame(zed))
	}
	if !slices.Contains(h.focusCalls(), zed) {
		t.Errorf("Zed should get the keyboard")
	}
	// Bench restoring it isn't the user's doing: nothing more happens.
	if p := h.paneView("p11"); p.Active != "w3" || h.view().Display.Focus != "p11" {
		t.Errorf("p11 shows %s, focus %s", p.Active, h.view().Display.Focus)
	}
}

func TestMinimisedOnlyWindowShowsItsMessage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	notes := h.windowID(obsidianID, "Daily note")
	id := h.itemOf(notes)
	pane := h.paneOf(id)
	h.clk.Advance(time.Second)
	mustOK(t, h.p.MinimiseWindow(notes))
	h.drain()
	// With no other window, the pane keeps the sub-tab, which says why.
	if p := h.paneView(pane); p.Active != id || h.item(id).Status != StatusMinimised {
		t.Errorf("%s shows %s, status %s", pane, p.Active, h.item(id).Status)
	}
	h.settle()
	if !h.minimised(notes) {
		t.Errorf("Obsidian came back")
	}

	// Brought back from the Dock, it's docked in its pane again.
	h.p.ResetCalls()
	mustOK(t, h.p.Unminimise(platform.Window{ID: notes}))
	h.drain()
	if it := h.item(id); it.Status != StatusReady || h.frame(notes) != h.bodyOf(pane) {
		t.Errorf("restored: %s, at %v, want docked at %v", it.Status, h.frame(notes), h.bodyOf(pane))
	}
}

func TestMinimisedWindowRestoredFromTheDockShowsInItsPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	h.clk.Advance(time.Second)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	mustOK(t, h.e.FocusPane("p12"))
	h.clk.Advance(2 * time.Second)

	// Clicking its tile in the Dock restores the window and focuses it.
	h.p.FocusWindow(zed)
	h.drain()
	v := h.view()
	if h.item("w3").Status != StatusReady || v.Display.Focus != "p11" || h.paneView("p11").Active != "w3" {
		t.Fatalf("w3 %s, focus %s, p11 shows %s", h.item("w3").Status, v.Display.Focus, h.paneView("p11").Active)
	}
	if h.frame(zed) != h.bodyOf("p11") || h.frame(code) != h.bodyOf("p11") {
		t.Errorf("Zed at %v, VS Code at %v; want both stacked at %v", h.frame(zed), h.frame(code), h.bodyOf("p11"))
	}

	// One that's in another tab now is shown when its tab is, as
	// follow_focus says.
	h.clk.Advance(time.Second)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	mustOK(t, h.e.SwitchTab(2))
	h.clk.Advance(2 * time.Second)
	h.p.FocusWindow(zed)
	h.drain()
	if v := h.view(); v.Current != 1 || h.paneView("p11").Active != "w3" || h.item("w3").Status != StatusReady {
		t.Errorf("current %d, p11 shows %s, w3 %s", v.Current, h.paneView("p11").Active, h.item("w3").Status)
	}
	if h.minimised(zed) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed minimised %v, at %v", h.minimised(zed), h.frame(zed))
	}
}

func TestMinimisedWindowFocusedBeforeItsRestoreIsHeardOf(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(time.Second)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	mustOK(t, h.e.FocusPane("p12"))
	h.clk.Advance(2 * time.Second)
	// The window's back, and has the keyboard, but WindowRestored hasn't
	// come yet.
	h.p.FocusWindow(zed)
	for _, ev := range h.p.DrainEvents() {
		if ev.Kind != platform.WindowRestored {
			h.e.handle(ev)
		}
	}
	h.drain()
	if h.item("w3").Status != StatusReady || h.paneView("p11").Active != "w3" || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("w3 %s, p11 shows %s, Zed at %v", h.item("w3").Status, h.paneView("p11").Active, h.frame(zed))
	}
}

func TestMinimisingDoesntSwitchToTheAppsWindowInAnotherTab(t *testing.T) {
	h := newHarness(t, setup{})
	docs := h.windowID(braveID, "Wails v3 docs")
	pr := h.windowID(braveID, "Pull request #42 · bench")
	mustOK(t, h.e.MoveItemToTab(h.itemOf(pr), 1))
	mustOK(t, h.e.SwitchTab(1))
	h.clk.Advance(2 * time.Second)
	// As the user minimises Brave's window here, macOS gives the keyboard
	// to Brave's next window, which is Research's, parked out of sight.
	h.p.ResetCalls()
	mustOK(t, h.p.MinimiseWindow(pr))
	h.p.FocusWindow(docs)
	h.drain()
	if v := h.view(); v.Current != 1 {
		t.Errorf("Bench switched to tab %d, following macOS's focus", v.Current)
	}
	// The keyboard goes back to what the focused pane shows now, rather
	// than stay in the window out of sight.
	if refocused(h) == 0 {
		t.Errorf("Bench left the keyboard in Research's window")
	}
	if !h.e.win.IsParked(docs) {
		t.Errorf("Research's window came out of hiding")
	}
	// Later, the user's own focus there is followed again.
	h.clk.Advance(2 * time.Second)
	h.p.FocusWindow(docs)
	h.drain()
	if v := h.view(); v.Current != 2 {
		t.Errorf("current %d, want Research after the user focused its window", v.Current)
	}
}

// windowOfItem returns an App item's window.
func (h *harness) windowOfItem(id string) platform.WindowID {
	h.e.lock()
	defer h.e.unlock()
	if it := h.e.model.Items[id]; it != nil {
		return it.WindowID
	}
	return 0
}

// refocused returns how many times Bench gave the keyboard to the current
// tab's focused pane since the calls were reset: to a window there, or to
// Bench itself.
func refocused(h *harness) int {
	return len(h.focusCalls()) + len(h.calls("FocusUI"))
}

func TestMinimiseHeardOfInAnyOrder(t *testing.T) {
	// macOS gives the keyboard to the app's next window as the minimise
	// starts, and often reports that first: the window minimised can come
	// back only if something arranges before Bench hears.
	type order struct {
		name string
		feed func(h *harness)
	}
	orders := []order{
		{"minimise first", func(h *harness) { h.drain() }},
		{"focus first", func(h *harness) { h.drainFocusFirst() }},
		{"arranged first", func(h *harness) {
			// Something arranges the tab before Bench hears of either.
			evs := h.p.DrainEvents()
			h.e.handle(platform.Event{Kind: platform.BenchMoved})
			for _, ev := range evs {
				h.e.handle(ev)
			}
			h.drain()
		}},
	}
	for _, o := range orders {
		t.Run(o.name+", same pane", func(t *testing.T) {
			// Brave's three windows share a pane in Research.
			h := newHarness(t, setup{})
			mustOK(t, h.e.SwitchTab(2))
			docs := h.windowID(braveID, "Wails v3 docs")
			pr := h.windowID(braveID, "Pull request #42 · bench")
			id, prID := h.itemOf(docs), h.itemOf(pr)
			pane := h.paneOf(id)
			if h.paneOf(prID) != pane {
				t.Fatalf("the two windows are in %s and %s", pane, h.paneOf(prID))
			}
			mustOK(t, h.e.ShowItem(pane, id))
			h.drain()
			h.clk.Advance(2 * time.Second)
			h.p.ResetCalls()
			mustOK(t, h.p.MinimiseWindow(docs))
			h.p.FocusWindow(pr)
			o.feed(h)
			h.settle()
			if !h.minimised(docs) || len(h.calls("Unminimise")) != 0 || h.item(id).Status != StatusMinimised {
				t.Errorf("docs minimised %v, status %s, Unminimise calls %v; want it left in the Dock", h.minimised(docs), h.item(id).Status, h.calls("Unminimise"))
			}
			// The pane shows another of Brave's windows, which has the
			// keyboard: the pull request, which macOS chose, or the one
			// Bench showed and focused before it heard.
			v := h.view()
			shown := h.paneView(pane).Active
			if w, ok := h.p.WindowState(h.windowOfItem(shown)); v.Current != 2 || shown == id || v.Display.Focus != pane || !ok || !w.Focused {
				t.Errorf("current %d, %s shows %s, focus %s, its window %+v; want one with the keyboard", v.Current, pane, shown, v.Display.Focus, w)
			}
		})
		t.Run(o.name+", another tab's window", func(t *testing.T) {
			h := newHarness(t, setup{})
			docs := h.windowID(braveID, "Wails v3 docs")
			pr := h.windowID(braveID, "Pull request #42 · bench")
			mustOK(t, h.e.MoveItemToTab(h.itemOf(pr), 1))
			mustOK(t, h.e.SwitchTab(1))
			h.clk.Advance(2 * time.Second)
			h.p.ResetCalls()
			mustOK(t, h.p.MinimiseWindow(pr))
			h.p.FocusWindow(docs)
			o.feed(h)
			if v := h.view(); v.Current != 1 || h.item(h.itemOf(pr)).Status != StatusMinimised || !h.minimised(pr) {
				t.Errorf("current %d, the pull request %s, minimised %v; want Code, and it left in the Dock", v.Current, h.item(h.itemOf(pr)).Status, h.minimised(pr))
			}
			if !h.e.win.IsParked(docs) || refocused(h) == 0 {
				t.Errorf("docs parked %v, refocused %d; want the keyboard back in Code", h.e.win.IsParked(docs), refocused(h))
			}
		})
	}
}

func TestOwnMinimiseIsntTheUsers(t *testing.T) {
	// A laptop between two taller monitors, which reach below it: a window
	// wider than the laptop has nowhere to park clear of them, so Bench
	// minimises it.
	left := platform.Screen{ID: "left", Frame: layout.Rect{X: -2560, Y: -229, W: 2560, H: 1440}, Visible: layout.Rect{X: -2560, Y: -204, W: 2560, H: 1415}}
	right := platform.Screen{ID: "right", Frame: layout.Rect{X: 1512, Y: -229, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: -204, W: 2560, H: 1415}}
	h := newHarness(t, setup{screen: laptop, world: func(p *platform.Fake) {
		p.SetScreens(laptop, left, right)
		standardWorld(p)
		p.AddWindow(zedID, "wide", platform.WithMinSize(1600, 800))
	}})
	mustOK(t, h.e.SetMode("wide"))
	mustOK(t, h.e.SwitchTab(1))
	wide := h.windowID(zedID, "wide")
	id := h.itemOf(wide)
	if p := h.paneOf(id); p != "p11" {
		t.Fatalf("the wide window is in %s", p)
	}
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.settle()
	if !h.minimised(wide) || !h.e.win.Minimised(wide) {
		t.Fatalf("Bench should have minimised the inactive wide window: minimised %v", h.minimised(wide))
	}
	if st := h.item(id).Status; st == StatusMinimised {
		t.Errorf("Bench's own minimise marked the window minimised")
	}
	// It comes back as it's shown.
	mustOK(t, h.e.ShowItem("p11", id))
	h.drain()
	if h.minimised(wide) || h.item(id).Status != StatusNeedsRoom {
		t.Errorf("shown, the wide window is minimised %v, status %s", h.minimised(wide), h.item(id).Status)
	}
}

func TestLateFocusOfAMinimisedWindowIsIgnored(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(2 * time.Second)
	// Zed's focus from before the minimise comes after it.
	w, _ := h.p.WindowState(zed)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	h.e.handle(platform.Event{Kind: platform.WindowFocused, Window: w, App: platform.App{BundleID: zedID, Name: "Zed"}})
	h.drain()
	if p := h.paneView("p11"); p.Active != "w4" || !h.minimised(zed) {
		t.Errorf("p11 shows %s, Zed minimised %v; want VS Code still shown", p.Active, h.minimised(zed))
	}
}

func TestLateMinimiseEventIsIgnored(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	// A WindowMinimised for a window that's no longer minimised, as after
	// Bench restored one it minimised itself, changes nothing.
	w, _ := h.p.WindowState(zed)
	h.e.handle(platform.Event{Kind: platform.WindowMinimised, Window: w})
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("w3 %s, want ready", it.Status)
	}
}

func TestMinimisedWindowOutOfItsPaneStaysUndocked(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(3 * time.Second)
	h.p.UserMoveWindow(zed, layout.Rect{X: 300, Y: 300, W: 800, H: 600})
	h.drain()
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	if it := h.item("w3"); it.Status != StatusUndocked {
		t.Errorf("w3 %s, want it still moved out of its pane", it.Status)
	}
	// Dock again brings it back from the Dock into its pane.
	mustOK(t, h.e.Redock("w3"))
	h.drain()
	if h.minimised(zed) || h.item("w3").Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("docked again, Zed minimised %v, %s, at %v", h.minimised(zed), h.item("w3").Status, h.frame(zed))
	}
}

func TestStillMinimisedWindowAtStartup(t *testing.T) {
	// Bench opens with Zed's window minimised: the user did that, since
	// Bench puts back what it minimised itself as it quits.
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		p.InstallApp(zedID, "Zed")
		p.AddApp(zedID, "Zed")
		p.AddWindow(zedID, "bench — switch.go", platform.WithMinimised())
		p.InstallApp(vscodeID, "Visual Studio Code")
		p.AddApp(vscodeID, "VS Code")
		p.AddWindow(vscodeID, "index.html — bench-ui")
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	zed := h.windowID(zedID, "bench — switch.go")
	if !h.minimised(zed) || h.item("w3").Status != StatusMinimised {
		t.Errorf("Zed minimised %v, w3 %s; want it left in the Dock", h.minimised(zed), h.item("w3").Status)
	}
	if p := h.paneView("p11"); p.Active != "w4" {
		t.Errorf("p11 shows %s, want VS Code", p.Active)
	}
}

func TestWindowOpenedMinimised(t *testing.T) {
	// Zed is open without windows as Bench starts, and then restores its
	// window as it was: minimised, as after a login.
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		p.InstallApp(zedID, "Zed")
		p.AddApp(zedID, "Zed")
		p.InstallApp(vscodeID, "Visual Studio Code")
		p.AddApp(vscodeID, "VS Code")
		p.AddWindow(vscodeID, "index.html — bench-ui")
	}})
	mustOK(t, h.e.SwitchTab(1))
	w, err := h.p.OpenWindow(zedID, "bench — switch.go", platform.WithMinimised())
	if err != nil {
		t.Fatal(err)
	}
	h.settle()
	id := h.itemOf(w.ID)
	if id == "" {
		t.Fatal("Bench didn't adopt the window")
	}
	if !h.minimised(w.ID) || h.item(id).Status != StatusMinimised || len(h.calls("Unminimise")) != 0 {
		t.Errorf("Zed minimised %v, %s %s; want it left in the Dock", h.minimised(w.ID), id, h.item(id).Status)
	}
}

func TestMinimisedWindowMovedToAPaneOfItsOwn(t *testing.T) {
	// Show side by side, from its menu or its pane's header, is for
	// seeing it: it comes back from the Dock in the new pane.
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(time.Second)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	mustOK(t, h.e.MoveItem("w3", 1, "p11", "right"))
	h.drain()
	pane := h.paneOf("w3")
	if pane == "p11" || h.item("w3").Status != StatusReady || h.minimised(zed) || h.frame(zed) != h.bodyOf(pane) {
		t.Errorf("w3 in %s, %s, minimised %v, at %v", pane, h.item("w3").Status, h.minimised(zed), h.frame(zed))
	}
}

func TestRemovingAMinimisedWindowFromItsTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(time.Second)
	mustOK(t, h.e.AddItemToTab("w3", 2))
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()

	// In another tab too, it stays there, still minimised.
	mustOK(t, h.e.RemoveItemFromTab("w3", 1))
	if v := h.view(); v.Items["w3"].Tab != 2 || v.Items["w3"].Status != StatusMinimised || !h.minimised(zed) {
		t.Errorf("w3 in tab %d, %s; Zed minimised %v", v.Items["w3"].Tab, v.Items["w3"].Status, h.minimised(zed))
	}

	// In one tab alone, it goes out of Bench, as Pop out takes it, and
	// stays in the Dock.
	mustOK(t, h.e.PopOut("w3"))
	h.settle()
	if v := h.view(); !slices.Contains(floatingIDs(v), "w3") || !h.minimised(zed) {
		t.Errorf("floating %v; Zed minimised %v", floatingIDs(v), h.minimised(zed))
	}
	if st := h.item("w3").Status; st != StatusReady {
		t.Errorf("popped out, w3 is %s", st)
	}
	// It can't be dragged back from the Dock: the toast says how to add it
	// back, which brings it out of the Dock.
	if got, want := h.em.lastToast(), "Moved Zed — bench — switch.go out of Bench. Its window stays in the Dock. Add it back with +."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	mustOK(t, h.e.AddToPane("p11", Source{Type: sourceFloating, ID: "w3"}))
	h.drain()
	if h.minimised(zed) || h.item("w3").Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("added back, Zed minimised %v, %s, at %v", h.minimised(zed), h.item("w3").Status, h.frame(zed))
	}
}

func TestMinimisedInTheTabsView(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SetMode("compact"))
	shown := h.paneView(layout.CompactPaneID).Active
	w := h.view().Items[shown]
	if w.Kind != KindApp {
		t.Fatalf("the tabs view shows %s, a %s", shown, w.Kind)
	}
	h.clk.Advance(time.Second)
	win := h.windowID(w.BundleID, w.Title)
	mustOK(t, h.p.MinimiseWindow(win))
	h.drain()
	if now := h.paneView(layout.CompactPaneID).Active; now == shown || now == "" {
		t.Errorf("the tabs view shows %q, want another window", now)
	}
	if !h.minimised(win) {
		t.Errorf("the window came back")
	}
	// Show again from the tabs view's pane.
	mustOK(t, h.e.ShowItem(layout.CompactPaneID, shown))
	h.drain()
	if h.minimised(win) || h.paneView(layout.CompactPaneID).Active != shown {
		t.Errorf("Show again: minimised %v, shows %s", h.minimised(win), h.paneView(layout.CompactPaneID).Active)
	}
}

func TestDemoMinimise(t *testing.T) {
	h := newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusPane("p11"))
	mustOK(t, h.e.DemoMinimise())
	h.drain()
	if it := h.item("w3"); it.Status != StatusMinimised || !h.minimised(h.windowID(zedID, "bench — switch.go")) {
		t.Errorf("w3 %s, want minimised", it.Status)
	}
	// A pane that shows a terminal has no window to minimise.
	mustOK(t, h.e.FocusPane("p12"))
	mustErr(t, h.e.DemoMinimise(), "The focused pane doesn't show an app's window to minimise.")

	h2 := newHarness(t, setup{})
	mustErr(t, h2.e.DemoMinimise(), "Simulated windows are only for the browser preview.")
}
