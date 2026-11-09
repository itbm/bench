package workspace

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// pageMenu is a web page's ⋯ menu as the frontend sends it, cut short: a
// status line, an entry, an entry that's off, a note, a heading, the Move
// to Pane and Page submenus, and a submenu that's off.
func pageMenu() Menu {
	on := true
	return Menu{X: 812, Y: 64, Entries: []MenuEntry{
		{Status: "Loading"},
		{Separator: true},
		{ID: "reload-page", Label: "Reload Page"},
		{ID: "close", Label: "Close Page", Disabled: true},
		{Note: "Closing an app's window needs Accessibility permission."},
		{Separator: true},
		{Heading: "Closed Windows"},
		{ID: "open-closed:0", Label: "Open Calendar"},
		{ID: "move-to-pane", Label: "Move to Pane", Submenu: []MenuEntry{
			{ID: "move-to-pane:p2", Label: "Pane with Outlook"},
		}},
		{ID: "page", Label: "Page", Submenu: []MenuEntry{
			{ID: "page-back", Label: "Back", Disabled: true},
			{ID: "page-zoom-in", Label: "Zoom In"},
			{ID: "layout:auto", Label: "Auto", Checked: &on},
		}},
		{ID: "also-show-in", Label: "Also Show In", Disabled: true, Submenu: []MenuEntry{
			{ID: "also-show-in:Code", Label: "Code"},
		}},
	}}
}

// TestShowMenuNeedsNativeMenus checks that the browser preview, which has
// no native menus, says so in its View and refuses ShowMenu: its frontend
// draws the menus in HTML.
func TestShowMenuNeedsNativeMenus(t *testing.T) {
	h := newHarness(t, setup{})
	if h.view().NativeMenus {
		t.Error("nativeMenus is on without native menus")
	}
	_, err := h.e.ShowMenu(pageMenu())
	mustErr(t, err, "This build of Bench draws its menus itself.")
}

// TestShowMenu checks that ShowMenu hands the menu to the shell as the
// frontend sent it, without the lock and without raising Bench, so that the
// web views stay shown, and gives back the entry chosen, but only one that
// the user could choose.
func TestShowMenu(t *testing.T) {
	var shown []Menu
	answer, fail := "", error(nil)
	var raised, hidden bool
	var h *harness
	h = newHarness(t, setup{options: func(o *Options) {
		o.ShowMenu = func(m Menu) (string, error) {
			shown = append(shown, m)
			// The engine's lock isn't held: another call goes through.
			done := make(chan error, 1)
			go func() { done <- h.e.FocusPane(h.paneOf("w1")) }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("FocusPane while the menu is open: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Error("the engine's lock was held while the menu was open")
			}
			raised = h.p.Raised()
			wv := h.p.WebViews()
			hidden = !wv["w1"].Visible || !wv["w2"].Visible
			return answer, fail
		}
	}})
	if !h.view().NativeMenus {
		t.Fatal("nativeMenus is off with native menus")
	}
	menu := pageMenu()
	for _, c := range []struct{ answer, want string }{
		{"reload-page", "reload-page"},
		{"page-zoom-in", "page-zoom-in"},
		{"layout:auto", "layout:auto"},
		{"move-to-pane:p2", "move-to-pane:p2"},
		{"open-closed:0", "open-closed:0"},
		{"", ""},
		// Off, in a submenu that's off, a submenu's own entry, and none.
		{"close", ""},
		{"page-back", ""},
		{"also-show-in:Code", ""},
		{"page", ""},
		{"move-to-pane", ""},
		{"nothing", ""},
	} {
		answer = c.answer
		got, err := h.e.ShowMenu(menu)
		mustOK(t, err)
		if got != c.want {
			t.Errorf("chose %q: got %q, want %q", c.answer, got, c.want)
		}
	}
	if len(shown) == 0 || !reflect.DeepEqual(shown[0], menu) {
		t.Errorf("the shell was given %+v, want %+v", shown, menu)
	}
	if raised || hidden || h.p.Raised() {
		t.Errorf("raised %v, web views hidden %v while the menu was open, or raised after it", raised, hidden)
	}

	// An empty menu shows nothing, and a mistake that long isn't shown.
	n := len(shown)
	got, err := h.e.ShowMenu(Menu{})
	mustOK(t, err)
	long := Menu{Entries: make([]MenuEntry, maxMenuEntries+1)}
	_, longErr := h.e.ShowMenu(long)
	mustErr(t, longErr, "That menu is too long to show.")
	if got != "" || len(shown) != n {
		t.Errorf("an empty or long menu: got %q, shown %d times, want none", got, len(shown)-n)
	}

	// The shell couldn't show it.
	answer, fail = "reload-page", errors.New("no window")
	_, err = h.e.ShowMenu(menu)
	mustErr(t, err, "Bench couldn't show the menu. Try again.")
}

// menuHarness returns a harness whose native menus stay open for as long
// as the user takes, a second, unless during says otherwise: during runs
// while the menu is open, without the engine's lock, and its answer is the
// entry chosen.
func menuHarness(t *testing.T, s setup, during *func(h *harness) string) *harness {
	t.Helper()
	var h *harness
	s.options = func(o *Options) {
		o.ShowMenu = func(Menu) (string, error) {
			if *during != nil {
				return (*during)(h), nil
			}
			h.advance(time.Second)
			return "", nil
		}
	}
	h = newHarness(t, s)
	return h
}

// TestANativeMenuFromTheJumpPaletteKeepsTheKeyboard checks that a native
// menu opened over a docked app, as the jump palette's Window Options…
// opens the ⋯ menu, stays open: the palette, as it closes, lowers Bench,
// which would give the keyboard back to the app's window in a moment, and
// that would make the app active, which closes the menu. The keyboard goes
// back to the window once the menu closes with nothing chosen, as Escape
// in the palette gives it back, but not after an entry that focuses
// something else.
func TestANativeMenuFromTheJumpPaletteKeepsTheKeyboard(t *testing.T) {
	// open opens the jump palette while Zed has the keyboard, and closes
	// it for Window Options…, which opens Zed's ⋯ menu natively.
	open := func(t *testing.T, during func(h *harness) string) (*harness, platform.WindowID) {
		h := menuHarness(t, setup{}, &during)
		mustOK(t, h.e.SwitchTab(1))
		h.settle()
		if h.p.ActiveApp() != zedID {
			t.Fatalf("Zed should have the keyboard, not %q", h.p.ActiveApp())
		}
		h.e.Shortcut(shortcuts.ActionJump, "")
		mustOK(t, h.p.ActivateApp(benchID))
		h.drain()
		mustOK(t, h.e.SetOverlay(true))
		h.p.ResetCalls()
		mustOK(t, h.e.SetOverlay(false))
		return h, h.windowID(zedID, "bench — switch.go")
	}
	// whileOpen notes the focus calls and the active app while the menu is
	// open, a second.
	type seen struct {
		focus  []platform.WindowID
		active string
	}
	whileOpen := func(got *seen, answer string) func(h *harness) string {
		return func(h *harness) string {
			h.advance(time.Second)
			got.focus, got.active = h.focusCalls(), h.p.ActiveApp()
			return answer
		}
	}

	t.Run("closed with nothing chosen", func(t *testing.T) {
		var during seen
		h, zed := open(t, whileOpen(&during, ""))
		id, err := h.e.ShowMenu(pageMenu())
		mustOK(t, err)
		if id != "" || len(during.focus) != 0 || during.active != benchID {
			t.Fatalf("while the menu was open: chose %q, focus calls %v, active %q; want none, and Bench", id, during.focus, during.active)
		}
		h.advance(time.Second)
		if got := h.focusCalls(); !slices.Equal(got, []platform.WindowID{zed}) || h.p.ActiveApp() != zedID {
			t.Errorf("once it closed: focus calls %v, active %q; want Zed's window to get the keyboard back", got, h.p.ActiveApp())
		}
	})

	t.Run("an entry chosen that switches tabs", func(t *testing.T) {
		var during seen
		h, zed := open(t, whileOpen(&during, "move-to-pane:p2"))
		id, err := h.e.ShowMenu(pageMenu())
		mustOK(t, err)
		if id != "move-to-pane:p2" || len(during.focus) != 0 {
			t.Fatalf("chose %q, focus calls %v while the menu was open", id, during.focus)
		}
		// The frontend runs the entry once the menu has closed.
		h.advance(50 * time.Millisecond)
		mustOK(t, h.e.SwitchTab(2))
		h.advance(time.Second)
		if slices.Contains(h.focusCalls(), zed) {
			t.Errorf("focus calls %v: Zed's window took the keyboard from Research", h.focusCalls())
		}
	})

	t.Run("lowered while the menu is open", func(t *testing.T) {
		// The palette's SetOverlay(false) can reach Go after ShowMenu.
		var during seen
		h, zed := open(t, func(h *harness) string {
			mustOK(t, h.e.SetOverlay(false))
			return whileOpen(&during, "")(h)
		})
		mustOK(t, h.e.SetOverlay(true))
		_, err := h.e.ShowMenu(pageMenu())
		mustOK(t, err)
		if len(during.focus) != 0 {
			t.Fatalf("focus calls %v while the menu was open", during.focus)
		}
		h.advance(time.Second)
		if !slices.Contains(h.focusCalls(), zed) {
			t.Errorf("focus calls %v; want Zed's window once the menu closed", h.focusCalls())
		}
	})
}

// TestANativeMenuDropsASwitchsPutOffFocus checks that a switch's focus,
// put off until a window that's opening arrives, isn't made while a native
// menu is open, which focusing another app's window would close, nor
// afterwards: the user has turned to the menu.
func TestANativeMenuDropsASwitchsPutOffFocus(t *testing.T) {
	var during func(h *harness) string
	var focus []platform.WindowID
	var commands []Command
	during = func(h *harness) string {
		// Calendar's window arrives while the menu is open.
		h.advance(3 * time.Second)
		if !h.p.IsRunning(calendarID) {
			t.Fatal("Calendar should have opened")
		}
		focus, commands = h.focusCalls(), h.takeCommands()
		return ""
	}
	h := menuHarness(t, setup{config: planConfig, world: slowCalendar}, &during)
	mustOK(t, h.e.SwitchTab(1))
	h.advance(1500 * time.Millisecond)
	h.takeCommands()
	h.p.ResetCalls()
	_, err := h.e.ShowMenu(pageMenu())
	mustOK(t, err)
	if len(focus) != 0 || len(commands) != 0 {
		t.Fatalf("while the menu was open, the put-off focus took the keyboard: %v, %v", focus, commands)
	}
	h.advance(3 * time.Second)
	if got := h.focusCalls(); len(got) != 0 {
		t.Errorf("the put-off focus took the keyboard once the menu closed: %v", got)
	}
	if got := h.takeCommands(); len(got) != 0 {
		t.Errorf("the put-off focus took the keyboard once the menu closed: %v", got)
	}
}

// TestNoHoldWhileANativeMenuIsOpen checks that last_tab's shortcut,
// pressed while a native menu is open, goes back to the last used tab at
// once, as it does while a dialog is open: the overview, which a hold
// shows, takes the keyboard, which would close the menu.
func TestNoHoldWhileANativeMenuIsOpen(t *testing.T) {
	var during func(h *harness) string
	var current int
	var hold *HoldView
	during = func(h *harness) string {
		h.p.SetHeldModifiers(held)
		h.press()
		h.advance(holdDelay)
		v := h.view()
		current, hold = v.Current, v.Hold
		return ""
	}
	h := menuHarness(t, setup{}, &during)
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	h.takeCommands()
	_, err := h.e.ShowMenu(pageMenu())
	mustOK(t, err)
	if current != 2 || hold != nil {
		t.Errorf("while the menu was open: current %d, hold %+v; want Research at once, and no hold", current, hold)
	}
	for _, c := range h.takeCommands() {
		if c.Name == holdTabsCommand {
			t.Errorf("the hold showed the tabs while the menu was open")
		}
	}
}

// TestANativeMenuIsntBenchComingForward checks that opening a native menu
// doesn't put away the windows that cover Bench (decision 68): a native
// menu doesn't make Bench active, as a click on Bench does, and isn't
// raised mode either. A dialog's raised mode leaves that as it was: once
// Bench comes forward, the windows over it go.
func TestANativeMenuIsntBenchComingForward(t *testing.T) {
	var during func(h *harness) string
	var hiddenDuring bool
	during = func(h *harness) string {
		h.advance(time.Second)
		hiddenDuring = h.p.IsHidden(finderID)
		return "split-right"
	}
	h := menuHarness(t, setup{}, &during)
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder is hidden before Bench came forward")
	}
	_, err := h.e.ShowMenu(pageMenu())
	mustOK(t, err)
	h.settle()
	if hiddenDuring || h.p.IsHidden(finderID) {
		t.Errorf("Finder hidden: %v while the menu was open, %v after; want neither", hiddenDuring, h.p.IsHidden(finderID))
	}
	if got := h.coveredToasts(); len(got) != 0 {
		t.Errorf("toasts %q after a native menu, want none", got)
	}

	// A dialog raises Bench, and the shell brings it forward for it.
	mustOK(t, h.e.SetOverlay(true))
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Error("Bench came forward for a dialog, but Finder, which covers it, isn't hidden")
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts %q", got)
	}
	mustOK(t, h.e.SetOverlay(false))
}

// TestANativeMenusChoiceIsTimedAsAnHTMLMenus checks that a native menu
// keeps no time of its own (decision 67): the frontend runs the entry
// chosen, as from an HTML menu, so a switch that it makes is timed as a
// tab switch, and Rearrange as one.
func TestANativeMenusChoiceIsTimedAsAnHTMLMenus(t *testing.T) {
	var during func(h *harness) string
	during = func(h *harness) string { return "rearrange" }
	h := menuHarness(t, setup{}, &during)
	mark := markTimes(h)
	id, err := h.e.ShowMenu(Menu{Entries: []MenuEntry{{ID: "rearrange", Label: "Rearrange"}}})
	mustOK(t, err)
	if id != "rearrange" {
		t.Fatalf("chose %q, want rearrange", id)
	}
	if got := timesSince(h, mark); len(got) != 0 {
		t.Errorf("the menu itself kept times %+v, want none", got)
	}
	// A tab's Rearrange, for another tab, switches to it first.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.Rearrange())
	if got := arrangeReasons(h, mark); !slices.Equal(got, []string{reasonTab, reasonRearrange}) {
		t.Errorf("reasons %v, want %s, then %s", got, reasonTab, reasonRearrange)
	}
}
