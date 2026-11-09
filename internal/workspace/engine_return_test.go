package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// The displays for the tests of going back to the display that the user
// chose (decision 70): a MacBook's built-in display, which is primary, and
// a monitor to its right.
var (
	builtIn = platform.Screen{ID: "1", Name: "Built-in Retina Display", Primary: true,
		Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}}
	dell = platform.Screen{ID: "7", Name: "DELL U2723QE",
		Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}}
	// onDell and onBuiltIn are Bench's content area at its usual size in
	// the middle of each display (openFrame).
	onDell    = layout.Rect{X: 2072, Y: 282.5, W: 1440, H: 900}
	onBuiltIn = layout.Rect{X: 36, Y: 57.5, W: 1440, H: 900}
)

// returnHarness returns a harness with Bench filling the built-in display,
// and the monitor plugged in, which the user then moved Bench onto.
func returnHarness(t *testing.T, s setup) *harness {
	t.Helper()
	s.screen = builtIn
	h := newHarness(t, s)
	h.p.PlugScreen(dell)
	h.settle()
	h.moveBench(dell.Visible)
	h.settle()
	if got := h.preferred(); got == nil || got.ID != dell.ID {
		t.Fatalf("after the user moved Bench onto the monitor, the display chosen is %+v, want the monitor", got)
	}
	return h
}

// preferred returns the display that the user chose for Bench's window.
func (h *harness) preferred() *config.ScreenState {
	h.e.lock()
	defer h.e.unlock()
	if h.e.preferred == nil {
		return nil
	}
	p := *h.e.preferred
	return &p
}

// toastCount returns how many toasts were sent.
func (r *recEmitter) toastCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.toasts)
}

// unplugDell unplugs the monitor, as macOS moves Bench's window onto the
// built-in display, and lets the screens settle; the shell fills the
// screen if Bench asks it to.
func (h *harness) unplugDell() {
	h.t.Helper()
	fills := h.em.filled()
	mustOK(h.t, h.p.UnplugScreen(dell.ID))
	h.settle()
	if h.em.filled() > fills {
		h.moveBench(builtIn.Visible)
		h.settle()
	}
	if got := h.p.ScreenOfWindow().ID; got != builtIn.ID {
		h.t.Fatalf("after the monitor went, Bench is on %q, want the built-in display", got)
	}
}

// TestBenchGoesBackToTheMonitorWhenItReturns checks the flow of decision
// 70: the monitor drops off, and Bench fills the built-in display; it comes
// back, and once the screens settle, Bench goes back to it, fills it again,
// chooses the wide layout again, and says so, with Undo, which moves it
// back to the built-in display, which the user has then chosen.
func TestBenchGoesBackToTheMonitorWhenItReturns(t *testing.T) {
	h := returnHarness(t, setup{})
	if got := h.view().SizeClass; got != "wide" {
		t.Fatalf("on the monitor: %s, want wide", got)
	}

	h.unplugDell()
	if got := h.view().SizeClass; got != "compact" {
		t.Errorf("on the built-in display: %s, want compact", got)
	}
	if got := h.preferred(); got == nil || got.ID != dell.ID {
		t.Errorf("after the monitor went, the display chosen is %+v, want the monitor still", got)
	}
	toasts := h.em.toastCount()

	// The monitor is plugged in again. While the screens settle, nothing
	// moves.
	h.p.PlugScreen(dell)
	h.advance(screenSettle - 100*time.Millisecond)
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Fatalf("Bench moved before the screens settled: %+v", got)
	}
	h.settle()
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Fatalf("after the monitor came back: moves %+v, want one to %v, filling the monitor", got, onDell)
	}
	if got := h.em.lastToast(); got != "Back on DELL U2723QE." || h.em.toastCount() != toasts+1 {
		t.Errorf("toast %q, want Back on DELL U2723QE.", got)
	}
	step := h.em.lastUndo(t)
	if v := h.view(); v.WentBack == nil || v.WentBack.Step != step || v.WentBack.Name != "Move to Display" || v.WentBack.Did != "moved Bench back to DELL U2723QE" {
		t.Errorf("the View's move back is %+v, want the move, step %d", v.WentBack, step)
	}
	if u := h.view().Undo; u != nil {
		t.Errorf("the View's latest change is %+v, want none: Tab ▸ Undo doesn't take back the move", u)
	}
	h.followMove()
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("back on the monitor: %s, want wide", got)
	}
	h.e.lock()
	filled, last := h.e.filled, *h.e.lastScreen
	h.e.unlock()
	if !filled || last.ID != dell.ID {
		t.Errorf("back on the monitor: filled %v, on %q; want filling the monitor", filled, last.ID)
	}

	// Undo moves Bench back to the built-in display, filling it as it did.
	mustOK(t, h.e.Undo(step))
	h.drain()
	if got := h.em.benchMoves(); len(got) != 2 || got[1] != (benchMove{onBuiltIn, true}) {
		t.Fatalf("after Undo: moves %+v, want a second one to %v, filling the built-in display", got, onBuiltIn)
	}
	if got := h.em.lastToast(); got != "Back on Built-in Retina Display." {
		t.Errorf("after Undo: toast %q, want Back on Built-in Retina Display.", got)
	}
	h.followMove()
	if got := h.view().SizeClass; got != "compact" {
		t.Errorf("after Undo: %s, want compact", got)
	}
	if got := h.preferred(); got == nil || got.ID != builtIn.ID {
		t.Errorf("after Undo, the display chosen is %+v, want the built-in display", got)
	}
	if w := h.view().WentBack; w != nil {
		t.Errorf("after Undo, the View's move back is %+v, want none", w)
	}
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")

	// The monitor drops off and comes back again: Bench stays where the
	// user chose by undoing.
	mustOK(t, h.p.UnplugScreen(dell.ID))
	h.settle()
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 2 {
		t.Errorf("after Undo, the monitor came back and Bench moved: %+v", got[2:])
	}
}

// TestBenchStaysOnTheDisplayTheUserMovedItTo checks that Bench doesn't go
// back to the monitor once the user has moved it to the built-in display
// on purpose, while both were connected.
func TestBenchStaysOnTheDisplayTheUserMovedItTo(t *testing.T) {
	h := returnHarness(t, setup{})
	h.moveBench(builtIn.Visible)
	h.settle()
	if got := h.preferred(); got == nil || got.ID != builtIn.ID {
		t.Fatalf("after the user moved Bench to the built-in display, the display chosen is %+v", got)
	}
	toasts := h.em.toastCount()
	mustOK(t, h.p.UnplugScreen(dell.ID))
	h.settle()
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Errorf("Bench moved: %+v, want it to stay on the built-in display", got)
	}
	if got := h.em.toastCount(); got != toasts {
		t.Errorf("Bench said %q, want nothing", h.em.lastToast())
	}

	// A display sleeping and waking without going doesn't move Bench either.
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensWoke})
	h.settle()
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Errorf("Bench moved after a wake: %+v", got)
	}
}

// TestMovesWhileTheMonitorIsAwayChoose checks that moving or resizing
// Bench's window on the built-in display while the monitor is away, or
// filling the screen there, chooses the built-in display, as any move by
// the user does: Bench stays there when the monitor comes back, as after
// a day with the lid closed, working on the monitor, it stays on the
// monitor when the lid opens.
func TestMovesWhileTheMonitorIsAwayChoose(t *testing.T) {
	tiled := layout.Rect{X: 0, Y: 33, W: 756, H: 949}
	for _, tc := range []struct {
		name string
		move func(h *harness)
	}{
		{"tiled", func(h *harness) { h.moveBench(tiled) }},
		{"tiled, then filled", func(h *harness) {
			h.moveBench(tiled)
			h.settle()
			mustOK(t, h.e.FillScreen())
			h.drain()
			h.moveBench(layout.Rect{X: 0, Y: 33, W: 1100, H: 949})
			h.moveBench(builtIn.Visible)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := returnHarness(t, setup{})
			h.unplugDell()
			toasts := h.em.toastCount()
			tc.move(h)
			h.settle()
			if got := h.preferred(); got == nil || got.ID != builtIn.ID {
				t.Fatalf("after the user moved Bench on the built-in display, the display chosen is %+v, want the built-in display", got)
			}
			h.p.PlugScreen(dell)
			h.settle()
			if got := h.em.benchMoves(); len(got) != 0 {
				t.Errorf("Bench moved: %+v, want it to stay on the built-in display", got)
			}
			if got := h.em.toastCount(); got != toasts {
				t.Errorf("Bench said %q, want nothing", h.em.lastToast())
			}
		})
	}
}

// TestBenchsOwnFillDoesntChoose checks that the zoom with which Bench fills
// the built-in display again, after macOS moved its window there without
// filling it, isn't the user's choice of that display: Bench still goes
// back to the monitor, filling it, and Undo puts the window back filling
// the built-in display. A move after the zoom has filled the screen is the
// user's again.
func TestBenchsOwnFillDoesntChoose(t *testing.T) {
	h := returnHarness(t, setup{})
	// macOS moves the window to the built-in display, smaller than it.
	h.changeScreens(layout.Rect{X: 0, Y: 33, W: 1200, H: 800}, builtIn)
	if got := h.em.filled(); got != 1 {
		t.Fatalf("Bench asked to fill the built-in display %d times, want once", got)
	}
	// The shell's zoom animates, and macOS says so as it goes.
	h.moveBench(layout.Rect{X: 0, Y: 33, W: 1350, H: 870})
	h.moveBench(builtIn.Visible)
	if got := h.preferred(); got == nil || got.ID != dell.ID {
		t.Fatalf("after Bench filled the built-in display, the display chosen is %+v, want the monitor", got)
	}
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Fatalf("moves %+v, want one onto the monitor, filling it", got)
	}
	step := h.em.lastUndo(t)
	h.followMove()
	mustOK(t, h.e.Undo(step))
	h.drain()
	if got := h.em.benchMoves(); len(got) != 2 || got[1] != (benchMove{onBuiltIn, true}) {
		t.Errorf("after Undo: moves %+v, want one back to the built-in display, filling it", got)
	}

	// Once the zoom has filled the screen, the user's move counts at once.
	h = returnHarness(t, setup{})
	h.changeScreens(layout.Rect{X: 0, Y: 33, W: 1200, H: 800}, builtIn)
	h.moveBench(builtIn.Visible)
	h.moveBench(layout.Rect{X: 0, Y: 33, W: 756, H: 949})
	if got := h.preferred(); got == nil || got.ID != builtIn.ID {
		t.Errorf("after the user tiled Bench once it filled the built-in display, the display chosen is %+v", got)
	}
}

// TestBenchGoesBackWhereItWasOnTheMonitor checks that a window the user
// had made smaller on the monitor goes back to where it was there, at that
// size, without filling the monitor.
func TestBenchGoesBackWhereItWasOnTheMonitor(t *testing.T) {
	h := returnHarness(t, setup{})
	half := layout.Rect{X: 1512, Y: 25, W: 1280, H: 1415}
	h.moveBench(half)
	h.unplugDell()
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{half, false}) {
		t.Errorf("moves %+v, want one to %v, not filling the monitor", got, half)
	}
}

// TestNoMoveWhenMacOSPutsBenchBack checks that when macOS moves Bench's
// window back to the monitor itself, as macOS 27 can, Bench neither moves
// it nor says anything.
func TestNoMoveWhenMacOSPutsBenchBack(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	toasts := h.em.toastCount()
	h.p.PlugScreen(dell)
	h.moveBench(dell.Visible)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Errorf("Bench moved: %+v, want macOS's move alone", got)
	}
	if got := h.em.toastCount(); got != toasts {
		t.Errorf("Bench said %q, want nothing", h.em.lastToast())
	}
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("back on the monitor: %s, want wide", got)
	}
}

// TestBenchGoesBackAfterAHold checks that a monitor that comes back during
// the hold after last_tab's shortcut waits for the hold to end before
// Bench moves, so that the tabs don't move from under the user.
func TestBenchGoesBackAfterAHold(t *testing.T) {
	h := returnHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	h.unplugDell()
	h.p.SetHeldModifiers(held)
	h.press()
	h.p.PlugScreen(dell)
	h.advance(3 * time.Second)
	if v := h.view(); v.Hold == nil {
		t.Fatal("the hold ended by itself")
	}
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Fatalf("Bench moved during the hold: %+v", got)
	}
	h.letGo()
	h.advance(returnRetry + holdPoll)
	if v := h.view(); v.Current != 0 {
		t.Errorf("after the hold: current %d, want the last used tab, Comms", v.Current)
	}
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Errorf("after the hold: moves %+v, want one onto the monitor", got)
	}
	if got := h.em.lastToast(); got != "Back on DELL U2723QE." {
		t.Errorf("after the hold: toast %q", got)
	}
}

// TestBenchGoesBackAfterATabSwitch checks that a monitor that comes back
// while a tab switch waits for an app to open waits for the switch to end
// before Bench moves.
func TestBenchGoesBackAfterATabSwitch(t *testing.T) {
	h := returnHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(2500*time.Millisecond), platform.AppLaunchWindows("Calendar"))
		p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://apple.com", "Apple"))
	}})
	h.unplugDell()
	mustOK(t, h.e.SwitchTab(1))
	h.p.PlugScreen(dell)
	h.advance(screenSettle + returnRetry)
	h.e.lock()
	waiting := h.e.wait != nil
	h.e.unlock()
	if !waiting {
		t.Fatal("the switch isn't waiting for Calendar any more")
	}
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Fatalf("Bench moved during the switch: %+v", got)
	}
	h.advance(2 * time.Second)
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Errorf("after the switch: moves %+v, want one onto the monitor", got)
	}
	if it := h.item("w2"); it.Status != StatusReady {
		t.Errorf("Calendar's item: %s, want ready", it.Status)
	}
}

// TestBenchGoesBackToAMonitorWithANewID checks that a display that comes
// back with a new ID, as macOS can give it, is the one chosen if it has
// the same name and frame, but not if it's somewhere else: another of the
// same model can have that name.
func TestBenchGoesBackToAMonitorWithANewID(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	again := dell
	again.ID = "12"
	h.p.PlugScreen(again)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Fatalf("the monitor with a new ID: moves %+v, want one onto it", got)
	}
	h.followMove()
	if got := h.preferred(); got == nil || got.ID != "12" {
		t.Errorf("the display chosen is %+v, want the monitor by its new ID", got)
	}

	h = returnHarness(t, setup{})
	h.unplugDell()
	twin := dell
	twin.ID = "12"
	twin.Frame.X, twin.Visible.X = -2560, -2560
	h.p.PlugScreen(twin)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Errorf("another monitor of the same model: moves %+v, want none", got)
	}
}

// TestBenchGoesBackOnceShown checks that while Bench's window is hidden,
// the move back waits until it's shown, as moving it could bring it back.
func TestBenchGoesBackOnceShown(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Fatalf("Bench moved while hidden: %+v", got)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Errorf("once shown: moves %+v, want one onto the monitor", got)
	}
}

// TestUndoAfterTheBuiltInDisplayWent checks that Undo of the move back
// refuses once the display that Bench was on has gone too, and forgets the
// move.
func TestUndoAfterTheBuiltInDisplayWent(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	h.p.PlugScreen(dell)
	h.settle()
	step := h.em.lastUndo(t)
	h.followMove()
	// The lid closes, and the built-in display goes.
	mustOK(t, h.p.UnplugScreen(builtIn.ID))
	h.settle()
	mustErr(t, h.e.Undo(step), "Built-in Retina Display isn't connected, so Bench can't move back to it.")
	if w := h.view().WentBack; w != nil {
		t.Errorf("the View's move back is %+v, want the move forgotten", w)
	}
	if got := h.em.benchMoves(); len(got) != 1 {
		t.Errorf("moves %+v, want only the move back", got)
	}
	mustErr(t, h.e.Undo(step), "Bench can't undo that any more.")
	if got := errCantGoBackTo(""); got.Error() != "That display isn't connected, so Bench can't move back to it." {
		t.Errorf("for a display without a name: %q", got)
	}
}

// TestBenchGoesBackAfterOneChange checks that Bench goes back to the
// monitor when it goes and comes back before Bench reads the displays
// in between, so that Bench never sees them without it: as when both
// changes come while the engine is busy, or macOS reports one change for
// both, as it can as the Mac wakes.
func TestBenchGoesBackAfterOneChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(h *harness)
	}{
		{"both changes at once", func(h *harness) {
			mustOK(t, h.p.UnplugScreen(dell.ID))
			h.p.PlugScreen(dell)
		}},
		{"one change as the Mac wakes", func(h *harness) {
			h.p.SetContentFrame(builtIn.Visible)
			h.p.EmitEvent(platform.Event{Kind: platform.Woke})
			h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
			h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := returnHarness(t, setup{})
			tc.change(h)
			h.settle()
			if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
				t.Fatalf("moves %+v, want one onto the monitor, filling it", got)
			}
			if got := h.em.lastToast(); got != "Back on DELL U2723QE." {
				t.Errorf("toast %q, want Back on DELL U2723QE.", got)
			}
			step := h.em.lastUndo(t)
			h.followMove()
			mustOK(t, h.e.Undo(step))
			h.drain()
			if got := h.em.benchMoves(); len(got) != 2 || got[1] != (benchMove{onBuiltIn, true}) {
				t.Errorf("after Undo: moves %+v, want one back to the built-in display", got)
			}
		})
	}
}

// TestUndoOfTheMoveBackWhileHidden checks that Undo of the move back,
// while Bench's window is hidden or minimised, as from the menu bar's
// recent messages, moves the window only once it's shown, as moving it
// could bring it back, and records nothing until then.
func TestUndoOfTheMoveBackWhileHidden(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	h.p.PlugScreen(dell)
	h.settle()
	step := h.em.lastUndo(t)
	h.followMove()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	mustOK(t, h.e.Undo(step))
	h.drain()
	if got := h.em.lastToast(); got != "Back on Built-in Retina Display." {
		t.Errorf("after Undo: toast %q", got)
	}
	if got := h.em.benchMoves(); len(got) != 1 {
		t.Fatalf("Bench moved its hidden window: %+v", got[1:])
	}
	h.e.lock()
	last := *h.e.lastScreen
	h.e.unlock()
	if last.ID != dell.ID {
		t.Errorf("while hidden, Bench records its window on %q, want the monitor still", last.ID)
	}
	if got := h.preferred(); got == nil || got.ID != builtIn.ID {
		t.Errorf("after Undo, the display chosen is %+v, want the built-in display", got)
	}
	h.settle()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if got := h.em.benchMoves(); len(got) != 2 || got[1] != (benchMove{onBuiltIn, true}) {
		t.Errorf("once shown: moves %+v, want one back to the built-in display, filling it", got)
	}
}

// TestUndoLatestLeavesTheMoveBack checks that Bench's move back isn't the
// latest change for Tab ▸ Undo, the jump palette, and the undo shortcut:
// they take back the user's own change, with the window where it is and
// the monitor still the display chosen; the move back's toast's Undo still
// takes it back. Undo of a change from before the move back leaves the
// window where it is too.
func TestUndoLatestLeavesTheMoveBack(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	name := h.view().Tabs[1].Name
	mustOK(t, h.e.MoveTab(1, 0))
	h.drain()
	moved := h.em.lastUndo(t)
	h.p.PlugScreen(dell)
	h.settle()
	back := h.em.lastUndo(t)
	h.followMove()
	if u := h.view().Undo; u == nil || u.Step != moved || u.Name != "Move Tab" {
		t.Fatalf("the View's latest change is %+v, want the tab's move", u)
	}
	mustOK(t, h.e.UndoLatest())
	h.drain()
	if got := h.em.lastToast(); got != "Undid: moved "+name+"." {
		t.Errorf("after UndoLatest: toast %q", got)
	}
	if got := h.view().Tabs[1].Name; got != name {
		t.Errorf("after UndoLatest, the second tab is %s, want %s back", got, name)
	}
	if got := h.em.benchMoves(); len(got) != 1 {
		t.Errorf("UndoLatest moved Bench: %+v", got[1:])
	}
	if got := h.preferred(); got == nil || got.ID != dell.ID {
		t.Errorf("after UndoLatest, the display chosen is %+v, want the monitor", got)
	}
	if w := h.view().WentBack; w == nil || w.Step != back {
		t.Errorf("after UndoLatest, the View's move back is %+v, want it still", w)
	}
	mustErr(t, h.e.UndoLatest(), "There's nothing to undo.")

	// The move back's own Undo still takes it back.
	mustOK(t, h.e.Undo(back))
	h.drain()
	if got := h.em.benchMoves(); len(got) != 2 || got[1] != (benchMove{onBuiltIn, true}) {
		t.Errorf("after the move back's Undo: moves %+v, want one to the built-in display", got)
	}

	// Undo of a change from before the move back leaves the window.
	h = returnHarness(t, setup{})
	h.unplugDell()
	mustOK(t, h.e.MoveTab(1, 0))
	h.drain()
	moved = h.em.lastUndo(t)
	h.p.PlugScreen(dell)
	h.settle()
	h.followMove()
	mustOK(t, h.e.Undo(moved))
	h.drain()
	if got := h.em.benchMoves(); len(got) != 1 {
		t.Errorf("Undo of the tab's move moved Bench: %+v", got[1:])
	}
	if w := h.view().WentBack; w == nil {
		t.Error("Undo of the tab's move forgot the move back")
	}
}

// TestChoosingAnotherDisplayForgetsTheMoveBack checks that once the user
// moves Bench to another display, or the monitor goes again, the move back
// can't be taken back any more: there's no window to put back.
func TestChoosingAnotherDisplayForgetsTheMoveBack(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	h.p.PlugScreen(dell)
	h.settle()
	h.followMove()
	h.moveBench(layout.Rect{X: 2000, Y: 100, W: 1440, H: 900})
	if w := h.view().WentBack; w == nil {
		t.Fatal("a move within the monitor forgot the move back")
	}
	h.moveBench(builtIn.Visible)
	h.settle()
	if w := h.view().WentBack; w != nil {
		t.Errorf("after the user moved Bench to the built-in display, the View's move back is %+v, want none", w)
	}

	h = returnHarness(t, setup{})
	h.unplugDell()
	h.p.PlugScreen(dell)
	h.settle()
	h.followMove()
	h.unplugDell()
	if w := h.view().WentBack; w != nil {
		t.Errorf("after the monitor went again, the View's move back is %+v, want none", w)
	}
}

// TestStateSavesTheDisplayChosen checks that state.json keeps the display
// chosen apart from the one Bench was on, and that Bench opens on the one
// chosen when it's connected as Bench starts.
func TestStateSavesTheDisplayChosen(t *testing.T) {
	h := returnHarness(t, setup{})
	h.unplugDell()
	mustOK(t, h.e.Close())
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		t.Fatal(err)
	}
	var st config.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Screen == nil || st.Screen.ID != builtIn.ID || st.PreferredScreen == nil || st.PreferredScreen.ID != dell.ID {
		t.Fatalf("state.json has screen %+v and preferredScreen %+v, want the built-in display and the monitor", st.Screen, st.PreferredScreen)
	}

	// Bench opens again with the monitor plugged in: on the monitor.
	restart := func(screens ...platform.Screen) *harness {
		h2 := newHarness(t, setup{screen: builtIn, state: func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, config.StateFile), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}})
		h2.p.SetScreens(screens...)
		return h2
	}
	h2 := restart(builtIn, dell)
	if got := h2.e.WindowStart(); got.Screen.ID != dell.ID || !got.Fill {
		t.Errorf("with the monitor plugged in: opens on %q, filled %v; want the monitor, filled", got.Screen.ID, got.Fill)
	}

	// Without it, Bench opens on the built-in display, and goes to the
	// monitor once it's plugged in.
	h3 := restart(builtIn)
	if got := h3.e.WindowStart(); got.Screen.ID != builtIn.ID {
		t.Errorf("without the monitor: opens on %q, want the built-in display", got.Screen.ID)
	}
	h3.p.PlugScreen(dell)
	h3.settle()
	if got := h3.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
		t.Errorf("the monitor plugged in after Bench opened: moves %+v, want one onto it", got)
	}
}

// TestFirstWindowStartChoosesItsDisplay checks that until the user chooses
// a display, the one that Bench first opens on is the one chosen, and that
// an older state.json's screen is.
func TestFirstWindowStartChoosesItsDisplay(t *testing.T) {
	h := newHarness(t, setup{screen: builtIn})
	h.e.WindowStart()
	if got := h.preferred(); got == nil || got.ID != builtIn.ID || got.Window != (layout.Rect{}) {
		t.Errorf("the display chosen is %+v, want the built-in display, filled", got)
	}

	old := &config.State{Version: config.StateVersion, Screen: &config.ScreenState{ID: dell.ID, Name: dell.Name, Frame: dell.Frame}}
	h = newHarness(t, setup{screen: builtIn, state: func(t *testing.T, dir string) {
		if err := config.SaveState(filepath.Join(dir, config.StateFile), old); err != nil {
			t.Fatal(err)
		}
	}})
	if got := h.preferred(); got == nil || got.ID != dell.ID {
		t.Errorf("from an older state.json, the display chosen is %+v, want the one Bench was on", got)
	}
}

// TestDemoMonitor checks the browser preview's Simulate ▸ Unplug monitor
// and Plug in monitor: Bench goes to the laptop's screen, then back to the
// monitor once the screens settle, with the toast and Undo.
func TestDemoMonitor(t *testing.T) {
	h := newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustOK(t, h.e.SetDemoScreen(platform.DemoLaptop, 0))
	mustErr(t, h.e.SetDemoMonitor(false), "Choose Monitor in the demo bar first, then unplug it.")
	mustErr(t, h.e.SetDemoMonitor(true), "The monitor is plugged in already.")
	mustOK(t, h.e.SetDemoScreen(platform.DemoMonitor, 0))
	h.settle()

	mustOK(t, h.e.SetDemoMonitor(false))
	h.settle()
	if d := h.view().Demo; d.Screen != platform.DemoLaptop || !d.Unplugged {
		t.Fatalf("unplugged: %+v, want on the laptop, unplugged", d)
	}
	mustOK(t, h.e.SetDemoMonitor(true))
	if d := h.view().Demo; d.Screen != platform.DemoLaptop || d.Unplugged {
		t.Errorf("plugged in, before the screens settle: %+v, want on the laptop still", d)
	}
	h.settle()
	if d := h.view().Demo; d.Screen != platform.DemoMonitor {
		t.Fatalf("plugged in: %+v, want back on the monitor", d)
	}
	if got := h.em.lastToast(); got != "Back on Monitor." {
		t.Errorf("plugged in: toast %q, want Back on Monitor.", got)
	}
	step := h.em.lastUndo(t)
	h.settle()

	mustOK(t, h.e.Undo(step))
	h.settle()
	if d := h.view().Demo; d.Screen != platform.DemoLaptop {
		t.Errorf("after Undo: %+v, want on the laptop", d)
	}
	if got := h.em.lastToast(); got != "Back on Laptop." {
		t.Errorf("after Undo: toast %q, want Back on Laptop.", got)
	}
	// The user chose the laptop by undoing: the monitor coming back again
	// leaves Bench there.
	mustOK(t, h.e.SetDemoMonitor(false))
	h.settle()
	mustOK(t, h.e.SetDemoMonitor(true))
	h.settle()
	if d := h.view().Demo; d.Screen != platform.DemoLaptop {
		t.Errorf("after Undo, the monitor came back: %+v, want on the laptop still", d)
	}
	if got := h.em.benchMoves(); len(got) != 0 {
		t.Errorf("the preview asked the shell to move the window: %+v", got)
	}

	h2 := newHarness(t, setup{})
	mustErr(t, h2.e.SetDemoMonitor(false), "Simulated screens are only for the browser preview.")
}

// TestBenchGoesBackOnceAMenuOrDialogCloses checks that a monitor that
// comes back while a native menu is open waits for the menu to close
// before Bench moves, as the menu stays where it opened (decision 71),
// and the same while a dialog is open.
func TestBenchGoesBackOnceAMenuOrDialogCloses(t *testing.T) {
	t.Run("a native menu", func(t *testing.T) {
		var h *harness
		var during []benchMove
		h = returnHarness(t, setup{options: func(o *Options) {
			o.ShowMenu = func(Menu) (string, error) {
				h.p.PlugScreen(dell)
				h.advance(3 * time.Second)
				during = h.em.benchMoves()
				return "", nil
			}
		}})
		h.unplugDell()
		_, err := h.e.ShowMenu(Menu{Entries: []MenuEntry{{ID: "rearrange", Label: "Rearrange"}}})
		mustOK(t, err)
		if len(during) != 0 {
			t.Fatalf("Bench moved while the menu was open: %+v", during)
		}
		h.advance(returnRetry)
		if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
			t.Errorf("once the menu closed: moves %+v, want one onto the monitor", got)
		}
		if got := h.em.lastToast(); got != "Back on DELL U2723QE." {
			t.Errorf("once the menu closed: toast %q", got)
		}
	})

	t.Run("a dialog", func(t *testing.T) {
		h := returnHarness(t, setup{})
		h.unplugDell()
		mustOK(t, h.e.SetOverlay(true))
		h.p.PlugScreen(dell)
		h.advance(3 * time.Second)
		if got := h.em.benchMoves(); len(got) != 0 {
			t.Fatalf("Bench moved while the dialog was open: %+v", got)
		}
		mustOK(t, h.e.SetOverlay(false))
		h.advance(returnRetry)
		if got := h.em.benchMoves(); len(got) != 1 || got[0] != (benchMove{onDell, true}) {
			t.Errorf("once the dialog closed: moves %+v, want one onto the monitor", got)
		}
	})
}
