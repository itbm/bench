package workspace

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/windows"
)

// wakeKinds are the events that say the Mac woke or the user came back to
// it, after which macOS can move windows.
var wakeKinds = []platform.EventKind{platform.Woke, platform.ScreensWoke, platform.SessionActive, platform.ScreenUnlocked}

// codeTab switches to the Code tab, whose first pane docks Zed's window,
// and lets everything settle. It returns Zed's window.
func codeTab(t *testing.T, h *harness) platform.WindowID {
	t.Helper()
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.drain()
	h.clk.Advance(3 * time.Second)
	if h.frame(zed) != h.bodyOf("p11") {
		t.Fatalf("Zed starts docked: %v", h.frame(zed))
	}
	return zed
}

// After each wake event, Bench waits for the screens to settle, then puts
// back the windows that macOS moved meanwhile, without a word and without
// moving the focus.
func TestWakePutsWindowsBackOnceSettled(t *testing.T) {
	for _, kind := range wakeKinds {
		t.Run(string(kind), func(t *testing.T) {
			h := newHarness(t, setup{})
			zed := codeTab(t, h)
			body := h.bodyOf("p11")
			toasts := len(h.em.toasts)
			h.p.ResetCalls()
			h.p.EmitEvent(platform.Event{Kind: kind})
			h.drain()
			// macOS moves the window as the displays come back.
			where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
			mustOK(t, h.p.AppMoveWindow(zed, where))
			h.advance(time.Second)
			if h.frame(zed) != where {
				t.Errorf("Bench should wait for the screens to settle: %v", h.frame(zed))
			}
			h.advance(600 * time.Millisecond)
			if h.frame(zed) != body {
				t.Errorf("after settling: %v, want %v", h.frame(zed), body)
			}
			if it := h.item("w3"); it.Status != StatusReady {
				t.Errorf("w3 after settling: %s %q", it.Status, it.StatusText)
			}
			if len(h.focusCalls()) != 0 {
				t.Errorf("a wake mustn't change focus: %v", h.focusCalls())
			}
			if len(h.em.toasts) != toasts {
				t.Errorf("putting windows back is silent: %v", h.em.toasts[toasts:])
			}
		})
	}
}

// macOS can move a window many times as the Mac wakes. Those moves don't use
// up the tries that Bench gives an app that moves its own window, so the
// window is back in its pane after the wake, and Bench still puts it back
// when its app moves it afterwards.
func TestWakeMovesDontUseUpRedocks(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	body := h.bodyOf("p11")
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.drain()
	before := len(h.calls("SetFrame"))
	for i := range 2 * redockLimit {
		mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250 + 10*float64(i), Y: 170, W: 878, H: 758}))
		h.advance(200 * time.Millisecond)
	}
	if after := len(h.calls("SetFrame")); after != before {
		t.Errorf("Bench put the window back %d times while the screens settled", after-before)
	}
	h.advance(500 * time.Millisecond)
	if h.frame(zed) != body {
		t.Fatalf("after settling: %v, want %v", h.frame(zed), body)
	}
	if len(h.e.redocks) != 0 {
		t.Errorf("tries left after settling: %v", h.e.redocks)
	}
	h.clk.Advance(time.Second)
	where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
	for i := range redockLimit {
		mustOK(t, h.p.AppMoveWindow(zed, where))
		h.drain()
		h.advance(redockDelay)
		if h.frame(zed) != body {
			t.Fatalf("try %d after the wake: Bench should put the window back: %v", i+1, h.frame(zed))
		}
		h.clk.Advance(time.Second)
	}
}

// A window that its app moved just before the wake waits for the settle too,
// rather than going back while macOS is still moving windows.
func TestWakeHoldsAPendingRedock(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.drain()
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.drain()
	before := len(h.calls("SetFrame"))
	h.advance(time.Second)
	if after := len(h.calls("SetFrame")); after != before {
		t.Errorf("Bench put the window back %d times before the screens settled", after-before)
	}
	h.advance(600 * time.Millisecond)
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("after settling: %v", h.frame(zed))
	}
}

// An app that used up its tries before the screens changed gets them back
// once they've settled: the moves that used them up may have been macOS's.
func TestSettleGivesRedocksBack(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	body := h.bodyOf("p11")
	where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
	for range redockLimit {
		mustOK(t, h.p.AppMoveWindow(zed, where))
		h.drain()
		h.advance(redockDelay)
		h.clk.Advance(time.Second)
	}
	mustOK(t, h.p.AppMoveWindow(zed, where))
	h.drain()
	h.advance(redockDelay)
	if h.frame(zed) != where {
		t.Fatalf("Bench should have stopped fighting the app: %v", h.frame(zed))
	}
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensWoke})
	h.advance(screenSettle + 100*time.Millisecond)
	if h.frame(zed) != body {
		t.Fatalf("the settle puts the window back: %v", h.frame(zed))
	}
	h.clk.Advance(time.Second)
	mustOK(t, h.p.AppMoveWindow(zed, where))
	h.drain()
	h.advance(redockDelay)
	if h.frame(zed) != body {
		t.Errorf("after the settle, Bench puts the window back again: %v", h.frame(zed))
	}
}

// The user can tile or resize Bench's window just after a wake or a desktop
// change. Neither changes the screens, so the move is the user's: Bench
// doesn't fill the screen again once the wait is over (decision 30), and
// the layout follows the window's width at once (decision 58).
func TestResizeAfterWakeOrDesktopChangeIsKept(t *testing.T) {
	for _, kind := range append([]platform.EventKind{platform.DesktopChanged}, wakeKinds...) {
		t.Run(string(kind), func(t *testing.T) {
			h := newHarness(t, setup{})
			h.moveBench(monitor.Visible)
			h.settle()
			h.p.EmitEvent(platform.Event{Kind: kind})
			h.advance(300 * time.Millisecond)
			h.moveBench(layout.Rect{Y: 25, W: 1280, H: 1415})
			if got := h.view().SizeClass; got != "compact" {
				t.Errorf("tiled into half the monitor, the layout is %s, want compact at once", got)
			}
			h.settle()
			if got := h.em.filled(); got != 0 {
				t.Errorf("asked to fill the screen %d times, want none", got)
			}
			if h.e.filled {
				t.Error("Bench should record that the user made its window smaller")
			}
		})
	}
}

// While the screens settle after they changed, macOS moves and resizes
// Bench's window, so the layout stays as it was until they have settled
// (decision 58). Then it follows the window's width.
func TestSizeClassWaitsForTheScreensToSettle(t *testing.T) {
	h := newHarness(t, setup{})
	h.moveBench(monitor.Visible)
	h.settle()
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.advance(300 * time.Millisecond)
	h.moveBench(layout.Rect{Y: 25, W: 1280, H: 1415})
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("while the screens settle, the layout is %s, want wide, as it was", got)
	}
	h.settle()
	if got := h.view().SizeClass; got != "compact" {
		t.Errorf("once the screens have settled, the layout is %s, want compact for a window 1,280 pt wide", got)
	}
}

// The screens can change as the Mac wakes, as when the Dock moves to a
// monitor attached to a closed laptop. Once they change, Bench's window's
// moves are macOS's until the screens settle, whatever started the wait,
// and a later wake event doesn't make them the user's again.
func TestWakeThenScreensChangeFillsTheScreenAgain(t *testing.T) {
	h := newHarness(t, setup{})
	h.moveBench(monitor.Visible)
	h.settle()
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.advance(300 * time.Millisecond)
	docked := monitor
	docked.Visible = layout.Rect{Y: 25, W: 2560, H: 1340}
	h.p.SetScreens(docked)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensWoke})
	h.advance(300 * time.Millisecond)
	h.moveBench(monitor.Visible)
	h.settle()
	if got := h.em.filled(); got != 1 {
		t.Errorf("asked to fill the screen %d times, want once", got)
	}
}

// awayFromDesktop marks Zed's window as on another desktop, as when Bench
// arranges the tab while the user is on another desktop, where the
// Accessibility API doesn't list it. Then the window is listed again
// without an event, as when the user comes back to Bench's desktop, where
// the window never left.
func awayFromDesktop(t *testing.T, h *harness, zed platform.WindowID) {
	t.Helper()
	mustOK(t, h.p.MoveToOtherDesktop(zed))
	h.drain()
	if it := h.item("w3"); it.Status != StatusUnavailable {
		t.Fatalf("w3 away: %s", it.Status)
	}
	mustOK(t, h.p.ReturnToDesktop(zed))
	h.p.DrainEvents()
}

// Going back to Bench's desktop looks again, at once, for the windows that
// Bench couldn't see from another desktop, and docks them.
func TestDesktopChangeFindsWindowsAgain(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	awayFromDesktop(t, h, zed)
	if it := h.item("w3"); it.Status != StatusUnavailable {
		t.Fatalf("nothing has looked yet: %s", it.Status)
	}
	h.p.ResetCalls()
	h.p.EmitEvent(platform.Event{Kind: platform.DesktopChanged})
	h.drain()
	if it := h.item("w3"); it.Status != StatusReady || it.StatusText != "" {
		t.Errorf("w3 back on Bench's desktop: %s %q", it.Status, it.StatusText)
	}
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should be docked again: %v", h.frame(zed))
	}
	h.advance(screenSettle + 100*time.Millisecond)
	if len(h.focusCalls()) != 0 {
		t.Errorf("a desktop change mustn't change focus: %v", h.focusCalls())
	}
}

// macOS can list a window on the desktop that the user has just gone to a
// moment late, so Bench looks once more for a window that's still away.
func TestDesktopChangeLooksOnceMore(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	mustOK(t, h.p.MoveToOtherDesktop(zed))
	h.drain()
	h.p.EmitEvent(platform.Event{Kind: platform.DesktopChanged})
	h.drain()
	if it := h.item("w3"); it.Status != StatusUnavailable {
		t.Fatalf("w3 is still away: %s", it.Status)
	}
	mustOK(t, h.p.ReturnToDesktop(zed))
	h.p.DrainEvents()
	h.p.ResetCalls()
	h.advance(desktopRecheck - 100*time.Millisecond)
	if it := h.item("w3"); it.Status != StatusUnavailable {
		t.Errorf("Bench looks once more after %v: %s", desktopRecheck, it.Status)
	}
	h.advance(200 * time.Millisecond)
	if it := h.item("w3"); it.Status != StatusReady || it.StatusText != "" {
		t.Errorf("w3 after looking once more: %s %q", it.Status, it.StatusText)
	}
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should be docked again: %v", h.frame(zed))
	}
	if len(h.focusCalls()) != 0 {
		t.Errorf("looking again mustn't change focus: %v", h.focusCalls())
	}
	// With nothing away, a desktop change looks only once.
	h.p.EmitEvent(platform.Event{Kind: platform.DesktopChanged})
	h.drain()
	if h.e.desktopTimer != nil {
		t.Error("nothing is away, so Bench shouldn't look again")
	}
}

// macOS doesn't move windows as the desktop changes, so a desktop change
// doesn't make Bench wait for the screens to settle. A docked window that
// its app moves just after goes back as at any other time, and the move
// counts as one of the app's tries.
func TestDesktopChangeDoesntHoldRedocks(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	h.p.EmitEvent(platform.Event{Kind: platform.DesktopChanged})
	h.drain()
	if h.e.screenTimer != nil {
		t.Error("a desktop change shouldn't wait for the screens to settle")
	}
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.drain()
	h.advance(redockDelay)
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Bench should put the window back at once: %v", h.frame(zed))
	}
	if n := len(h.e.redocks[zed].at); n != 1 {
		t.Errorf("tries used: %d, want 1", n)
	}
}

// Once the screens settle after a wake, Bench looks again for the windows
// it said were on another desktop.
func TestWakeFindsWindowsAgain(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	awayFromDesktop(t, h, zed)
	h.p.EmitEvent(platform.Event{Kind: platform.SessionActive})
	h.advance(time.Second)
	if it := h.item("w3"); it.Status != StatusUnavailable {
		t.Errorf("Bench waits for the screens to settle: %s", it.Status)
	}
	h.advance(600 * time.Millisecond)
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("w3 after settling: %s %q", it.Status, it.StatusText)
	}
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should be docked again: %v", h.frame(zed))
	}
}

// When macOS takes Accessibility permission away while Bench runs, the next
// arrangement fails. Bench stops arranging, shows its banner again, and
// shows the apps it hid; the windows it parked stay parked, and recorded.
// It checks for the permission every 2 seconds, and once it's back,
// arranges the tab again.
func TestLosingAccessibility(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	code := h.windowID(vscodeID, "index.html — bench-ui")
	// The terminal in front of Zed and VS Code parks their windows.
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	h.drain()
	h.clk.Advance(3 * time.Second)
	if !h.e.win.IsParked(zed) || !h.e.win.IsParked(code) {
		t.Fatal("a terminal is shown, so the pane's windows start parked")
	}
	if !h.p.IsHidden(braveID) || !h.p.IsHidden(obsidianID) {
		t.Fatal("Brave and Obsidian belong to another tab, so start hidden")
	}
	zedAt, codeAt := h.frame(zed), h.frame(code)
	toasts := len(h.em.toasts)

	h.p.SetTrusted(false)
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.advance(screenSettle + 100*time.Millisecond)
	v := h.view()
	if v.Permissions.Accessibility || !slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Fatalf("permissions %+v notices %v", v.Permissions, texts(v.Notices))
	}
	if h.p.IsHidden(braveID) || h.p.IsHidden(obsidianID) {
		t.Errorf("the apps Bench hid should be shown: Brave %v, Obsidian %v", h.p.IsHidden(braveID), h.p.IsHidden(obsidianID))
	}
	if !h.e.win.IsParked(zed) || !h.e.win.IsParked(code) || h.frame(zed) != zedAt || h.frame(code) != codeAt {
		t.Errorf("Zed and VS Code can't be moved without the permission: Zed %v, VS Code %v", h.frame(zed), h.frame(code))
	}
	pf, err := windows.ReadParked(filepath.Join(h.dir, config.ParkedFile))
	if err != nil || len(pf.Windows) != 2 || len(pf.Hidden) != 0 {
		t.Errorf("parked.json = %+v, %v; want the two parked windows and no apps", pf, err)
	}
	if len(h.em.toasts) != toasts {
		t.Errorf("the banner says it, not a toast: %v", h.em.toasts[toasts:])
	}
	if h.item("w3").Pane != "p11" || h.item("w5").Pane != "p11" {
		t.Errorf("the arrangement stays: w3 in %s, w5 in %s", h.item("w3").Pane, h.item("w5").Pane)
	}

	h.p.ResetCalls()
	prompts := h.p.TrustPrompts()
	h.advance(4 * trustEvery)
	if n := len(h.calls("Trusted")); n < 4 {
		t.Errorf("Trusted calls: %d, want one every 2 s", n)
	}
	if h.p.TrustPrompts() != prompts {
		t.Errorf("checking mustn't ask macOS to prompt")
	}
	if n := len(h.calls("SetFrame", "Park", "Hide")); n != 0 {
		t.Errorf("Bench moved or hid things without the permission: %d calls", n)
	}

	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	v = h.view()
	if !v.Permissions.Accessibility || slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Errorf("still untrusted: %+v %v", v.Permissions, texts(v.Notices))
	}
	if !h.p.IsHidden(braveID) || !h.p.IsHidden(obsidianID) {
		t.Errorf("Bench should hide the other tabs' apps again")
	}
	mustOK(t, h.e.ShowItem("p11", "w3"))
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Bench docks windows again: Zed %v", h.frame(zed))
	}
}

// loseAccessibility takes Accessibility permission away, and has Bench
// notice, as it does when it becomes active.
func loseAccessibility(t *testing.T, h *harness) {
	t.Helper()
	h.p.SetTrusted(false)
	h.p.EmitEvent(platform.Event{Kind: platform.AppActivated, App: h.bench})
	h.drain()
	if h.view().Permissions.Accessibility {
		t.Fatal("Bench didn't notice that it lost Accessibility permission")
	}
}

// A window that the user minimised, then asked to see again while Bench
// had no Accessibility permission, comes back from the Dock once the
// permission is back: the scan then doesn't mark it again.
func TestShowAgainWhileAccessibilityIsLost(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	if st := h.item("w3").Status; st != StatusMinimised {
		t.Fatalf("w3 %s, want minimised", st)
	}
	loseAccessibility(t, h)
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.drain()

	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	h.settle()
	if it := h.item("w3"); it.Status != StatusReady || h.minimised(zed) || h.paneView("p11").Active != "w3" || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("w3 %s, Zed minimised %v, p11 shows %s, Zed at %v; want Zed back from the Dock, shown and docked at %v",
			it.Status, h.minimised(zed), h.paneView("p11").Active, h.frame(zed), h.bodyOf("p11"))
	}
}

// Without Accessibility permission, Bench hears nothing of windows. A
// window that the user brought back from the Dock meanwhile loses its
// Minimised mark once the permission is back, and docks in its pane again.
func TestRestoredWhileAccessibilityIsLost(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	mustOK(t, h.p.MinimiseWindow(zed))
	h.drain()
	loseAccessibility(t, h)
	// The fake platform restores a window only with the permission.
	h.p.SetTrusted(true)
	mustOK(t, h.p.Unminimise(platform.Window{ID: zed}))
	h.p.SetTrusted(false)
	h.p.DrainEvents()
	h.advance(trustEvery)

	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	h.settle()
	if it := h.item("w3"); it.Status != StatusReady || h.minimised(zed) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("w3 %s, Zed minimised %v, at %v; want it docked at %v, without its Minimised mark",
			it.Status, h.minimised(zed), h.frame(zed), h.bodyOf("p11"))
	}
}

// Losing the permission is noticed by any arrangement, such as a tab
// switch's.
func TestLosingAccessibilityAtATabSwitch(t *testing.T) {
	h := newHarness(t, setup{})
	codeTab(t, h)
	h.p.SetTrusted(false)
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if v := h.view(); v.Permissions.Accessibility {
		t.Fatalf("permissions %+v", v.Permissions)
	}
	if h.p.IsHidden(zedID) || h.p.IsHidden(vscodeID) {
		t.Errorf("no app should stay hidden: Zed %v, VS Code %v", h.p.IsHidden(zedID), h.p.IsHidden(vscodeID))
	}
}

// The user usually turns Bench off in System Settings, then clicks Bench's
// window. Bench checks the permission as it becomes active, so the banner
// comes back then, and the apps it hid show, without waiting for an
// arrangement.
func TestLosingAccessibilityWhenBenchBecomesActive(t *testing.T) {
	h := newHarness(t, setup{})
	codeTab(t, h)
	if !h.p.IsHidden(braveID) {
		t.Fatal("Brave belongs to another tab, so starts hidden")
	}
	h.p.EmitEvent(platform.Event{Kind: platform.AppActivated, App: h.bench})
	h.drain()
	if v := h.view(); !v.Permissions.Accessibility || !h.p.IsHidden(braveID) {
		t.Fatalf("with the permission, becoming active changes nothing: %+v", v.Permissions)
	}
	h.p.SetTrusted(false)
	h.p.EmitEvent(platform.Event{Kind: platform.AppActivated, App: h.bench})
	h.drain()
	v := h.view()
	if v.Permissions.Accessibility || !slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Fatalf("permissions %+v notices %v", v.Permissions, texts(v.Notices))
	}
	if h.p.IsHidden(braveID) {
		t.Error("Brave, which Bench hid, should be shown")
	}
	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	if v := h.view(); !v.Permissions.Accessibility {
		t.Errorf("still untrusted: %+v", v.Permissions)
	}
	if !h.p.IsHidden(braveID) {
		t.Error("Bench should hide Brave again")
	}
}

// A call that fails as if Bench had no permission, while macOS says it has
// it, changes nothing: Bench goes on arranging, and doesn't show the banner.
func TestNotTrustedErrorWhileTrusted(t *testing.T) {
	h := newHarness(t, setup{})
	codeTab(t, h)
	h.p.SetErr("Raise", fmt.Errorf("raising: %w", platform.ErrNotTrusted))
	mustOK(t, h.e.Rearrange())
	h.p.SetErr("Raise", nil)
	v := h.view()
	if !v.Permissions.Accessibility || slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Errorf("permissions %+v notices %v", v.Permissions, texts(v.Notices))
	}
	if !h.p.IsHidden(braveID) {
		t.Errorf("Brave should stay hidden")
	}
}
