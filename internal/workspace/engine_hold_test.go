package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// held is last_tab's default modifiers, ⌃⌥, held down.
const held = platform.ModControl | platform.ModOption

// recentHarness returns a harness whose tabs were used in the order
// Code (current), Research (the last used tab), then Comms.
func recentHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := newHarness(t, s)
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if v := h.view(); v.Current != 1 || v.LastTab != 2 {
		t.Fatalf("current %d, last used tab %d; want Code, then Research", v.Current, v.LastTab)
	}
	return h
}

// press presses last_tab's shortcut, as the global shortcut does.
func (h *harness) press() { h.e.Shortcut(shortcuts.ActionLastTab, "") }

// letGo lets go of the modifiers, and lets the hold see it.
func (h *harness) letGo() {
	h.p.SetHeldModifiers(0)
	h.advance(holdPoll)
}

// TestHoldQuickTap checks that a quick tap of last_tab's shortcut goes back
// to the last used tab once its modifiers are let go of, without showing
// the tabs, as the shortcut did before there was a hold.
func TestHoldQuickTap(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(100 * time.Millisecond)
	if v := h.view(); v.Current != 1 || v.Hold != nil {
		t.Fatalf("while held for 100 ms: current %d, hold %+v; want Code still, and no tabs shown", v.Current, v.Hold)
	}
	h.letGo()
	if v := h.view(); v.Current != 2 || v.LastTab != 1 || v.Hold != nil {
		t.Errorf("after letting go: current %d, last used tab %d, hold %+v; want Research, then Code", v.Current, v.LastTab, v.Hold)
	}
	if got := h.em.commandsNamed(holdTabsCommand); len(got) != 0 {
		t.Errorf("a quick tap showed the tabs: %+v", got)
	}
	// Back and forth, as before.
	h.p.SetHeldModifiers(held)
	h.press()
	h.letGo()
	if v := h.view(); v.Current != 1 || v.LastTab != 2 {
		t.Errorf("the second tap: current %d, last used tab %d; want Code, then Research", v.Current, v.LastTab)
	}
	// The modifiers let go of before Bench asks: at once.
	h.press()
	if v := h.view(); v.Current != 2 {
		t.Errorf("with the modifiers let go of already: current %d, want Research at once", v.Current)
	}
}

// TestHoldShowsTabsInRecentOrder checks a hold: past holdDelay, Bench
// shows every tab, most recently used first, with the last used tab
// chosen; each press, the arrows, and StepHold choose another; and letting
// go switches to the one chosen.
func TestHoldShowsTabsInRecentOrder(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay - 2*holdPoll)
	if h.view().Hold != nil || len(h.em.commandsNamed(holdTabsCommand)) != 0 {
		t.Fatal("the tabs showed before holdDelay")
	}
	h.advance(2 * holdPoll)
	v := h.view()
	if v.Hold == nil || v.Hold.ID != 1 || !slices.Equal(v.Hold.Tabs, []int{1, 2, 0}) || v.Hold.Selected != 2 || v.Current != 1 {
		t.Fatalf("past holdDelay: hold %+v, current %d; want Code, Research, Comms with Research chosen", v.Hold, v.Current)
	}
	if got := h.em.commandsNamed(holdTabsCommand); len(got) != 1 {
		t.Errorf("hold-tabs sent %d times, want once", len(got))
	}

	steps := []struct {
		name string
		do   func()
		want int
	}{
		{"the shortcut again", h.press, 0},
		{"and again, round to the current tab", h.press, 1},
		{"⌃⌥→", func() { h.e.Shortcut(shortcuts.ActionFocusPane, "right") }, 2},
		// Up and Down ask the frontend, which knows the rows, to move the
		// choice (holdUpCommand).
		{"⌃⌥↓", func() { h.e.Shortcut(shortcuts.ActionFocusPane, "down") }, 2},
		{"⌃⌥←", func() { h.e.Shortcut(shortcuts.ActionFocusPane, "left") }, 1},
		{"⌃⌥↑", func() { h.e.Shortcut(shortcuts.ActionFocusPane, "up") }, 1},
		{"StepHold(1)", func() { mustOK(t, h.e.StepHold(1)) }, 2},
		{"StepHold(-1)", func() { mustOK(t, h.e.StepHold(-1)) }, 1},
		// A row's worth of cards, down and up, stopping at the ends.
		{"StepHold(2)", func() { mustOK(t, h.e.StepHold(2)) }, 0},
		{"StepHold(-2)", func() { mustOK(t, h.e.StepHold(-2)) }, 1},
		{"StepHold(3)", func() { mustOK(t, h.e.StepHold(3)) }, 0},
		{"StepHold(-3)", func() { mustOK(t, h.e.StepHold(-3)) }, 1},
		{"⌃⌥]", func() { h.e.Shortcut(shortcuts.ActionNextTab, "") }, 2},
		{"⌃⌥[", func() { h.e.Shortcut(shortcuts.ActionPreviousTab, "") }, 1},
		// ⌃⌥⇧`, send_to_last_tab, moves it back, as Command-Shift-Tab
		// does, and moves no window.
		{"⌃⌥⇧`", func() { h.e.Shortcut(shortcuts.ActionSendToLastTab, "") }, 0},
		// Other shortcuts do nothing.
		{"⌃⌥0", func() { h.e.Shortcut(shortcuts.ActionRearrange, "") }, 0},
		{"⌃⌥⇧→", func() { h.e.Shortcut(shortcuts.ActionMoveWindow, "right") }, 0},
		{"StepHold(-1) to Research", func() { mustOK(t, h.e.StepHold(-1)) }, 2},
		{"StepHold(1) to Comms", func() { mustOK(t, h.e.StepHold(1)) }, 0},
	}
	items := h.view().Items
	focus := h.view().Display.Focus
	for _, s := range steps {
		s.do()
		h.advance(holdPoll)
		if v := h.view(); v.Hold == nil || v.Hold.Selected != s.want || v.Current != 1 {
			t.Fatalf("%s: hold %+v, current %d; want tab %d chosen, Code current", s.name, v.Hold, v.Current, s.want)
		}
	}
	if got := h.view().Display.Focus; got != focus {
		t.Errorf("the arrows focused pane %s, want them to leave %s alone during the hold", got, focus)
	}
	if got := h.em.lastToast(); got != "" {
		t.Errorf("a shortcut during the hold said %q", got)
	}
	for id, it := range h.view().Items {
		if was := items[id]; it.Tab != was.Tab || it.Pane != was.Pane {
			t.Errorf("%s moved from tab %d pane %s to tab %d pane %s during the hold", id, was.Tab, was.Pane, it.Tab, it.Pane)
		}
	}
	if got := h.em.commandsNamed(holdTabsCommand); len(got) != 1 {
		t.Errorf("hold-tabs sent %d times, want once", len(got))
	}
	if up, down := h.em.commandsNamed(holdUpCommand), h.em.commandsNamed(holdDownCommand); len(up) != 1 || len(down) != 1 {
		t.Errorf("hold-up sent %d times and hold-down %d, want each once", len(up), len(down))
	}

	h.letGo()
	if v := h.view(); v.Current != 0 || v.LastTab != 1 || v.Hold != nil {
		t.Errorf("after letting go: current %d, last used tab %d, hold %+v; want Comms, then Code", v.Current, v.LastTab, v.Hold)
	}
	// The order goes on from there: Comms, Code, Research.
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	if v := h.view(); v.Hold == nil || !slices.Equal(v.Hold.Tabs, []int{0, 1, 2}) || v.Hold.Selected != 1 || v.Hold.ID != 2 {
		t.Errorf("the next hold: %+v, want the second, with Comms, Code, Research, and Code chosen", v.Hold)
	}
	mustOK(t, h.e.CancelHold())
}

// TestHoldZoomPaneSwitches checks that zoom_pane, ⌃⌥↩, switches at once
// while a hold shows the tabs, as Return does in the overview, and does
// what it always did before Bench shows them.
func TestHoldZoomPaneSwitches(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	h.e.Shortcut(shortcuts.ActionZoomPane, "")
	if v := h.view(); v.Current != 2 || v.Hold != nil || v.Display.Zoomed != "" {
		t.Fatalf("⌃⌥↩ during the hold: current %d, hold %+v, zoomed %q; want Research, no hold, nothing zoomed", v.Current, v.Hold, v.Display.Zoomed)
	}
	h.letGo()
	if v := h.view(); v.Current != 2 {
		t.Errorf("letting go after ⌃⌥↩ switched to tab %d", v.Current)
	}
}

// TestHoldCancel checks that CancelHold, for Escape, ends a hold without
// switching tabs, and that letting go afterwards does nothing.
func TestHoldCancel(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.CancelHold())
	if v := h.view(); v.Hold != nil || v.Current != 1 {
		t.Fatalf("after CancelHold: hold %+v, current %d; want none, and Code", v.Hold, v.Current)
	}
	h.letGo()
	if v := h.view(); v.Current != 1 || v.LastTab != 2 {
		t.Errorf("letting go after CancelHold: current %d, last used tab %d; want Code, then Research", v.Current, v.LastTab)
	}
	mustOK(t, h.e.SetOverlay(false))
	// With no hold, CancelHold and StepHold do nothing.
	mustOK(t, h.e.CancelHold())
	mustOK(t, h.e.StepHold(1))
	if v := h.view(); v.Current != 1 || v.Hold != nil {
		t.Errorf("CancelHold and StepHold without a hold: current %d, hold %+v", v.Current, v.Hold)
	}
}

// TestHoldEndsWithAnotherSwitch checks that a switch during a hold, such
// as a click on a tab in the overview or a tab's own shortcut, ends it.
func TestHoldEndsWithAnotherSwitch(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	h.e.Shortcut(shortcuts.ActionTab, "1")
	if v := h.view(); v.Current != 0 || v.Hold != nil {
		t.Fatalf("⌃⌥1 during the hold: current %d, hold %+v; want Comms, and no hold", v.Current, v.Hold)
	}
	h.letGo()
	if v := h.view(); v.Current != 0 || v.LastTab != 1 {
		t.Errorf("letting go after ⌃⌥1: current %d, last used tab %d; want Comms, then Code", v.Current, v.LastTab)
	}
}

// TestHoldPassesOverDeletedTabs checks that a tab deleted during a hold
// leaves its list, and isn't chosen.
func TestHoldPassesOverDeletedTabs(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	mustOK(t, h.e.DeleteTab(2))
	// Comms is still tab 0; Code, current, is tab 1.
	if v := h.view(); v.Hold == nil || !slices.Equal(v.Hold.Tabs, []int{1, 0}) || v.Hold.Selected != -1 {
		t.Fatalf("after deleting Research: hold %+v, want Code, Comms, with nothing chosen", v.Hold)
	}
	h.press()
	if v := h.view(); v.Hold == nil || v.Hold.Selected != 0 {
		t.Fatalf("the next press: hold %+v, want Comms chosen", v.Hold)
	}
	h.letGo()
	if v := h.view(); v.Current != 0 || v.Hold != nil {
		t.Errorf("after letting go: current %d, hold %+v; want Comms", v.Current, v.Hold)
	}
}

// TestHoldLimit checks that a hold longer than holdLimit ends without
// switching, as when Bench missed the user letting go.
func TestHoldLimit(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdLimit)
	if v := h.view(); v.Hold != nil || v.Current != 1 {
		t.Fatalf("past holdLimit: hold %+v, current %d; want no hold, and Code", v.Hold, v.Current)
	}
	h.letGo()
	if v := h.view(); v.Current != 1 {
		t.Errorf("letting go after holdLimit switched to tab %d", v.Current)
	}
}

// TestHoldOnlyWhenItCanShow checks when last_tab's shortcut goes back at
// once, without a hold: while a menu or dialog is open, while Bench is
// hidden and tab shortcuts leave it hidden, and with a shortcut that has
// no modifiers to hold. Only the shortcut that isn't let go of holds: the
// Tab menu's Last Used Tab doesn't, as nothing is held.
func TestHoldOnlyWhenItCanShow(t *testing.T) {
	cases := []struct {
		name  string
		setup setup
		do    func(h *harness)
	}{
		{"with a dialog open", setup{}, func(h *harness) { mustOK(t, h.e.SetOverlay(true)) }},
		{"while Bench is hidden, with show_on_tab_shortcut off", setup{
			config: strings.Replace(exampleConfig(t), "show_on_tab_shortcut = true", "show_on_tab_shortcut = false", 1),
		}, func(h *harness) {
			h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
			h.drain()
		}},
		{"with a function key", setup{
			config: strings.Replace(exampleConfig(t), "last_tab = \"ctrl+alt+`\"", "last_tab = \"f6\"", 1),
		}, func(*harness) {}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := recentHarness(t, c.setup)
			c.do(h)
			h.p.SetHeldModifiers(held)
			h.press()
			if v := h.view(); v.Current != 2 {
				t.Errorf("current %d, want Research at once", v.Current)
			}
			h.advance(holdDelay)
			if v := h.view(); v.Hold != nil || v.Current != 2 {
				t.Errorf("hold %+v, current %d; want no hold", v.Hold, v.Current)
			}
		})
	}
}

// TestHoldWhileBenchIsHidden checks that while Bench is hidden, with
// show_on_tab_shortcut on, a quick tap of last_tab's shortcut brings Bench
// back on the last used tab, as a tab shortcut does: Bench comes forward
// once the hold has switched, not as it begins, which showed the tab being
// left and unhid its apps first.
func TestHoldWhileBenchIsHidden(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.settle()
	h.p.ResetCalls()
	forwards := h.em.broughtForward()
	h.p.SetHeldModifiers(held)
	h.press()
	if v := h.view(); v.Current != 1 || h.e.hold == nil {
		t.Fatalf("while held: current %d, hold %v; want Code still, and a hold", v.Current, h.e.hold != nil)
	}
	if got := h.em.broughtForward() - forwards; got != 0 {
		t.Errorf("while held, brought Bench forward %d times, want none yet", got)
	}
	h.advance(80 * time.Millisecond)
	h.letGo()
	if v := h.view(); v.Current != 2 || v.Hold != nil {
		t.Fatalf("after letting go: current %d, hold %+v; want Research", v.Current, v.Hold)
	}
	if got := h.em.broughtForward() - forwards; got != 1 {
		t.Errorf("after letting go, brought Bench forward %d times, want once", got)
	}
	// The shell brings Bench forward, and macOS says it's shown.
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	for _, c := range h.calls("Unhide") {
		if c.BundleID == zedID || c.BundleID == vscodeID {
			t.Errorf("Bench unhid %s, an app of Code, the tab it left: %v", c.BundleID, h.calls("Unhide", "Hide"))
		}
	}

	// A tap without a tab to go back to says so, with Bench in front, so
	// that the toast shows.
	h = newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	forwards = h.em.broughtForward()
	h.p.SetHeldModifiers(held)
	h.press()
	h.letGo()
	if got, want := h.em.lastToast(), errNoLastTab.Error(); got != want {
		t.Errorf("a quick tap said %q, want %q", got, want)
	}
	if got := h.em.broughtForward() - forwards; got != 1 {
		t.Errorf("a quick tap without a last used tab brought Bench forward %d times, want once", got)
	}
}

// TestShowAllTabsShortcut checks that show_all_tabs asks the frontend to
// open or close the overview, and ends a hold.
func TestShowAllTabsShortcut(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	h.e.Shortcut(shortcuts.ActionShowAllTabs, "")
	if got := h.em.commandsNamed(toggleAllTabsCommand); len(got) != 1 {
		t.Errorf("toggle-all-tabs sent %d times, want once", len(got))
	}
	h.letGo()
	if v := h.view(); v.Hold != nil || v.Current != 1 {
		t.Errorf("after show_all_tabs during a hold: hold %+v, current %d; want no hold, and Code", v.Hold, v.Current)
	}
}

// TestDemoModifiers checks that the browser preview's page says which
// modifiers are held, and that only the preview may.
func TestDemoModifiers(t *testing.T) {
	h := newHarness(t, setup{})
	if err := h.e.DemoModifiers([]string{"ctrl"}); err == nil || err.Error() != errNoDemoKeys.Error() {
		t.Errorf("DemoModifiers outside the preview = %v, want %v", err, errNoDemoKeys)
	}
	h = newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustOK(t, h.e.DemoModifiers([]string{"ctrl", "alt"}))
	if got := h.p.HeldModifiers(); got != held {
		t.Errorf("held %v, want ⌃⌥", got)
	}
	mustOK(t, h.e.DemoModifiers(nil))
	if got := h.p.HeldModifiers(); got != 0 {
		t.Errorf("held %v, want none", got)
	}
}

// TestHoldShortcutsBeforeTabsShow checks that another shortcut pressed
// before a hold shows the tabs lands the quick tap's switch first, so that
// it acts on the last used tab, as it did when the tap switched at once.
func TestHoldShortcutsBeforeTabsShow(t *testing.T) {
	cases := []struct {
		name  string
		do    func(h *harness)
		check func(t *testing.T, h *harness)
	}{
		{"⌃⌥], the tab after the last used one", func(h *harness) {
			h.e.Shortcut(shortcuts.ActionNextTab, "")
		}, func(t *testing.T, h *harness) {
			if v := h.view(); v.Current != 0 || v.LastTab != 2 {
				t.Errorf("current %d, last used tab %d; want Comms, after Research", v.Current, v.LastTab)
			}
		}},
		{"⌃⌥1", func(h *harness) {
			h.e.Shortcut(shortcuts.ActionTab, "1")
		}, func(t *testing.T, h *harness) {
			if v := h.view(); v.Current != 0 || v.LastTab != 2 {
				t.Errorf("current %d, last used tab %d; want Comms, after Research", v.Current, v.LastTab)
			}
		}},
		{"⌃⌥⇧`, a window sent back to the tab left", func(h *harness) {
			h.e.Shortcut(shortcuts.ActionSendToLastTab, "")
		}, func(t *testing.T, h *harness) {
			v := h.view()
			if v.Current != 2 || v.Items["w3"].Tab != 1 {
				t.Errorf("current %d, Zed in tab %d; want Research, with Zed still in Code", v.Current, v.Items["w3"].Tab)
			}
			if got := h.em.lastToast(); !strings.HasSuffix(got, " to Code.") {
				t.Errorf("toast %q, want a window moved to Code", got)
			}
		}},
		{"⌃⌥→, in the last used tab", func(h *harness) {
			h.e.Shortcut(shortcuts.ActionFocusPane, "right")
		}, func(t *testing.T, h *harness) {
			if v := h.view(); v.Current != 2 || v.Display.Focus != "p15" || h.e.model.Tabs[1].Focus != "p11" {
				t.Errorf("current %d, focus %s, Code's focus %s; want Research's second pane focused, and Code's left alone",
					v.Current, v.Display.Focus, h.e.model.Tabs[1].Focus)
			}
		}},
		{"the jump shortcut, over the last used tab", func(h *harness) {
			h.e.Shortcut(shortcuts.ActionJump, "")
		}, func(t *testing.T, h *harness) {
			if v := h.view(); v.Current != 2 || len(h.em.commandsNamed("toggle-palette")) != 1 {
				t.Errorf("current %d, toggle-palette %d times; want Research, and the palette", v.Current, len(h.em.commandsNamed("toggle-palette")))
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := recentHarness(t, setup{})
			if f := h.view().Display.Focus; f != "p11" {
				t.Fatalf("Code's focused pane is %s, want p11", f)
			}
			h.p.SetHeldModifiers(held)
			h.press()
			h.advance(100 * time.Millisecond)
			c.do(h)
			h.drain()
			if v := h.view(); v.Hold != nil {
				t.Fatalf("hold %+v, want none", v.Hold)
			}
			c.check(t, h)
			// Letting go afterwards does nothing more.
			cur := h.view().Current
			h.letGo()
			h.advance(holdDelay)
			if v := h.view(); v.Current != cur || len(h.em.commandsNamed(holdTabsCommand)) != 0 {
				t.Errorf("after letting go: current %d, want %d still, and no tabs shown", v.Current, cur)
			}
		})
	}
}

// TestHoldWithoutLastTab checks that a hold works before Bench has
// switched tabs, with the first other tab in the tab bar chosen, while a
// quick tap still says there's no tab to go back to.
func TestHoldWithoutLastTab(t *testing.T) {
	h := newHarness(t, setup{})
	if v := h.view(); v.Current != 0 || v.LastTab != -1 {
		t.Fatalf("current %d, last used tab %d; want Comms, and none", v.Current, v.LastTab)
	}
	h.p.SetHeldModifiers(held)
	h.press()
	h.letGo()
	if v := h.view(); v.Current != 0 || v.Hold != nil {
		t.Errorf("a quick tap: current %d, hold %+v; want Comms, and no hold", v.Current, v.Hold)
	}
	if got, want := h.em.lastToast(), errNoLastTab.Error(); got != want {
		t.Errorf("a quick tap said %q, want %q", got, want)
	}
	toasts := len(h.em.toasts)
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(holdDelay)
	if v := h.view(); v.Hold == nil || !slices.Equal(v.Hold.Tabs, []int{0, 1, 2}) || v.Hold.Selected != 1 {
		t.Fatalf("held: %+v, want Comms, Code, Research, with Code chosen", v.Hold)
	}
	h.letGo()
	if v := h.view(); v.Current != 1 || v.LastTab != 0 || v.Hold != nil {
		t.Errorf("after letting go: current %d, last used tab %d, hold %+v; want Code, then Comms", v.Current, v.LastTab, v.Hold)
	}
	if got := h.em.toasts[toasts:]; len(got) != 0 {
		t.Errorf("the hold said %q", got)
	}
}

// TestHoldEndsAsTheMacWakes checks that the Mac waking, or the user coming
// back to their session, ends a hold without switching tabs: the keys were
// let go of meanwhile, and Bench may not have seen it.
func TestHoldEndsAsTheMacWakes(t *testing.T) {
	for _, kind := range []platform.EventKind{platform.Woke, platform.ScreensWoke, platform.SessionActive, platform.ScreenUnlocked} {
		t.Run(string(kind), func(t *testing.T) {
			h := recentHarness(t, setup{})
			h.p.SetHeldModifiers(held)
			h.press()
			h.advance(holdDelay)
			if h.view().Hold == nil {
				t.Fatal("the hold didn't show the tabs")
			}
			h.p.EmitEvent(platform.Event{Kind: kind})
			h.drain()
			if v := h.view(); v.Hold != nil || v.Current != 1 {
				t.Fatalf("hold %+v, current %d; want no hold, and Code", v.Hold, v.Current)
			}
			h.letGo()
			if v := h.view(); v.Current != 1 {
				t.Errorf("letting go after waking switched to tab %d", v.Current)
			}
		})
	}
}

// TestHoldAndTheKeyboard checks who gets the keyboard after a hold that
// showed the tabs, while typing in a docked App window: letting go
// switches, and the tab switched to gets it, so the window that had it
// before the hold doesn't take it back; Escape gives it back; and the jump
// shortcut during the hold ends it without switching, and its palette
// gives it back as it closes.
func TestHoldAndTheKeyboard(t *testing.T) {
	// open shows the tabs while Zed, in Code's focused pane, has the
	// keyboard. The shell brings Bench forward for them, and the frontend
	// opens the overview, which raises it.
	open := func(t *testing.T) (*harness, platform.WindowID) {
		h := recentHarness(t, setup{})
		if h.p.ActiveApp() != zedID {
			t.Fatalf("Zed should have the keyboard, not %q", h.p.ActiveApp())
		}
		h.p.SetHeldModifiers(held)
		h.press()
		h.advance(holdDelay)
		if h.view().Hold == nil {
			t.Fatal("the hold didn't show the tabs")
		}
		mustOK(t, h.p.ActivateApp(benchID))
		h.drain()
		mustOK(t, h.e.SetOverlay(true))
		h.p.ResetCalls()
		return h, h.windowID(zedID, "bench — switch.go")
	}
	noted := func(h *harness) string {
		h.e.lock()
		defer h.e.unlock()
		return h.e.jump.itemID
	}

	t.Run("letting go", func(t *testing.T) {
		h, zed := open(t)
		if noted(h) == "" {
			t.Fatal("the hold didn't note Zed's window")
		}
		h.letGo()
		if noted(h) != "" {
			t.Errorf("letting go kept the note of %s", noted(h))
		}
		// The overview closes once Go has switched.
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if v := h.view(); v.Current != 2 || slices.Contains(h.focusCalls(), zed) {
			t.Errorf("current %d, focus calls %v; want Research, and Zed's window left alone", v.Current, h.focusCalls())
		}
	})

	t.Run("escape", func(t *testing.T) {
		h, zed := open(t)
		mustOK(t, h.e.CancelHold())
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if got := h.focusCalls(); h.view().Current != 1 || !slices.Equal(got, []platform.WindowID{zed}) || h.p.ActiveApp() != zedID {
			t.Errorf("current %d, focus calls %v, active %q; want Code, with Zed's window given the keyboard back", h.view().Current, got, h.p.ActiveApp())
		}
		h.letGo()
		if v := h.view(); v.Current != 1 {
			t.Errorf("letting go after Escape switched to tab %d", v.Current)
		}
	})

	t.Run("the jump shortcut", func(t *testing.T) {
		h, zed := open(t)
		h.e.Shortcut(shortcuts.ActionJump, "")
		if v := h.view(); v.Hold != nil || v.Current != 1 || len(h.em.commandsNamed("toggle-palette")) != 1 {
			t.Fatalf("hold %+v, current %d, toggle-palette %d times; want no hold, Code, and the palette",
				v.Hold, v.Current, len(h.em.commandsNamed("toggle-palette")))
		}
		h.letGo()
		if v := h.view(); v.Current != 1 {
			t.Errorf("letting go with the palette open switched to tab %d", v.Current)
		}
		// The palette takes the overview's place, and Escape closes it.
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if got := h.focusCalls(); !slices.Equal(got, []platform.WindowID{zed}) {
			t.Errorf("focus calls %v; want Zed's window given the keyboard back", got)
		}
	})
}

// TestHoldWithADialogOpening checks that a dialog that opens during a hold,
// before it shows the tabs, keeps them hidden, and that letting go still
// switches to the last used tab.
func TestHoldWithADialogOpening(t *testing.T) {
	h := recentHarness(t, setup{})
	h.p.SetHeldModifiers(held)
	h.press()
	h.advance(100 * time.Millisecond)
	mustOK(t, h.e.SetOverlay(true))
	h.advance(holdDelay)
	if v := h.view(); v.Hold != nil || len(h.em.commandsNamed(holdTabsCommand)) != 0 {
		t.Fatalf("hold %+v: the tabs showed over the dialog", v.Hold)
	}
	h.letGo()
	if v := h.view(); v.Current != 2 || v.LastTab != 1 {
		t.Errorf("after letting go: current %d, last used tab %d; want Research, then Code", v.Current, v.LastTab)
	}
	mustOK(t, h.e.SetOverlay(false))
}
