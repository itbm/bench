package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/shortcuts"
)

// TestSendToTabShortcut checks the send_to_tab_modifiers shortcut with a
// tab's number: the focused pane's shown window moves into that tab's
// focused pane, as Move to tab does, Bench stays on the current tab, and
// the toast has Undo and Go there.
func TestSendToTabShortcut(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	if got := h.view().Display.Panes[0].Active; got != "w3" {
		t.Fatalf("Code's focused pane shows %s, want Zed (w3)", got)
	}
	h.p.ResetCalls()
	h.e.Shortcut(shortcuts.ActionSendToTab, "3")
	v := h.view()
	if v.Current != 1 {
		t.Errorf("the shortcut switched to tab %d; it mustn't switch tabs", v.Current)
	}
	if it := v.Items["w3"]; it.Tab != 2 || it.Pane != "p14" {
		t.Errorf("w3: tab %d, pane %s; want Research's focused pane, p14", it.Tab, it.Pane)
	}
	if active := layout.Leaves(v.Tabs[2].Root)[0].Active; active != "w3" {
		t.Errorf("Research's focused pane shows %s, want w3", active)
	}
	// The pane it left shows VS Code, which gets the keyboard.
	vscode := h.windowID(vscodeID, "index.html — bench-ui")
	if got := v.Display.Panes[0].Active; got != "w4" || !slices.Contains(h.focusCalls(), vscode) {
		t.Errorf("Code's pane shows %s, focus calls %v; want VS Code (w4), focused", got, h.focusCalls())
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to Research." {
		t.Errorf("toast %q", got)
	}
	step := h.em.lastUndo(t)
	if a := h.em.lastSecond(); a == nil || *a != (ToastAction{Label: "Go there", Method: "SwitchTab", Arg: 2}) {
		t.Errorf("the toast's second button = %+v, want Go there, which switches to Research", a)
	}

	// Undo puts it back where it was.
	mustOK(t, h.e.Undo(step))
	if it := h.item("w3"); it.Tab != 1 || it.Pane != "p11" {
		t.Errorf("after Undo, w3: tab %d, pane %s; want Code's p11", it.Tab, it.Pane)
	}

	// Go there is SwitchTab with the tab: Research, with the window shown
	// in its focused pane.
	h.e.Shortcut(shortcuts.ActionSendToTab, "3")
	mustOK(t, h.e.SwitchTab(h.em.lastSecond().Arg))
	v = h.view()
	if v.Current != 2 || v.Display.Focus != "p14" || v.Display.Panes[0].Active != "w3" {
		t.Errorf("Go there: tab %d, focus %s, shows %s; want Research, with w3 in p14", v.Current, v.Display.Focus, v.Display.Panes[0].Active)
	}
}

// TestSendToLastTab checks send_to_tab_modifiers with `: the window goes
// to the tab used before this one, which LastTab goes back to.
func TestSendToLastTab(t *testing.T) {
	h := newHarness(t, setup{})
	h.e.Shortcut(shortcuts.ActionSendToLastTab, "")
	if got := h.em.lastToast(); got != "There's no last used tab to send the window to yet. Switch tabs first." {
		t.Errorf("before a tab switch, toast %q", got)
	}
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SwitchTab(1))
	h.e.Shortcut(shortcuts.ActionSendToLastTab, "")
	v := h.view()
	if v.Current != 1 || v.Items["w3"].Tab != 2 || v.Items["w3"].Pane != "p14" {
		t.Errorf("tab %d, w3 in tab %d, pane %s; want Code still current, with w3 in Research's p14",
			v.Current, v.Items["w3"].Tab, v.Items["w3"].Pane)
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to Research." {
		t.Errorf("toast %q", got)
	}
	if a := h.em.lastSecond(); a == nil || a.Arg != 2 {
		t.Errorf("Go there = %+v, want it to switch to Research", a)
	}
	// The last used tab stays Research: the move didn't switch tabs.
	if v.LastTab != 2 {
		t.Errorf("the last used tab is %d, want Research still", v.LastTab)
	}
	mustOK(t, h.e.SendToLastTab())
	if it := h.item("w4"); it.Tab != 2 {
		t.Errorf("SendToLastTab: w4 in tab %d, want Research", it.Tab)
	}
}

// TestSendToTabRefusals checks the toasts of the shortcut's refusals: the
// current tab, a tab that doesn't exist, and an empty pane.
func TestSendToTabRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.e.Shortcut(shortcuts.ActionSendToTab, "2")
	if got := h.em.lastToast(); got != "That window is already in Code." {
		t.Errorf("to the current tab, toast %q", got)
	}
	h.e.Shortcut(shortcuts.ActionSendToTab, "9")
	if got := h.em.lastToast(); got != "There's no tab 9." {
		t.Errorf("to tab 9, toast %q", got)
	}
	mustErr(t, h.e.SendToTab(-1), "There's no tab 0.")
	if it := h.item("w3"); it.Tab != 1 || it.Pane != "p11" {
		t.Errorf("after the refusals, w3: tab %d, pane %s; want it where it was", it.Tab, it.Pane)
	}
	// Code's last pane, once its windows have gone, has nothing to send.
	for _, id := range []string{"w3", "w4", "w5"} {
		mustOK(t, h.e.MoveItemToTab(id, 0))
	}
	mustErr(t, h.e.SendToTab(0), "There's no window in this pane to move.")
}

// TestSendToTabSharedWindow checks that a window that's in the target tab
// as well follows Move to tab's rules: it leaves the current tab, and
// stays where it is in the other.
func TestSendToTabSharedWindow(t *testing.T) {
	h := newHarness(t, setup{})
	shareZed(t, h)
	before := slices.Clone(h.tabNamed("Research").Root.Children[0].Items)
	mustOK(t, h.e.SendToTab(2))
	if it := h.item("w3"); len(it.Tabs) != 0 || it.Tab != 2 || it.Pane != "p14" {
		t.Errorf("w3: tabs %v, tab %d, pane %s; want Research's p14 only", it.Tabs, it.Tab, it.Pane)
	}
	if got := h.tabNamed("Research").Root.Children[0].Items; !slices.Equal(got, before) {
		t.Errorf("Research's focused pane = %v, want it as it was, %v", got, before)
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to Research." {
		t.Errorf("toast %q", got)
	}
}

// TestMoveItemToTabToastHasGoThere checks that the ⋯ menu's Move to tab,
// which the shortcut shares, says so with Undo and Go there too.
func TestMoveItemToTabToastHasGoThere(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItemToTab("w4", 0))
	h.em.lastUndo(t)
	if a := h.em.lastSecond(); a == nil || *a != (ToastAction{Label: "Go there", Method: "SwitchTab", Arg: 0}) {
		t.Errorf("the toast's second button = %+v, want Go there, which switches to Comms", a)
	}
}

// TestSendToTabKeysFromBindings checks that the Move to tab entries show
// only the send_to_tab_modifiers shortcuts that Bench registers: with
// add_front_window set to ⌃⌥⇧3, Bindings leaves ⌃⌥⇧3 to it, so Research,
// the third tab, shows none, in the ⋯ menu (TabView.SendShortcut) and in
// Pane ▸ Move to Tab (MenuBarTab.SendKeys), while the others keep theirs.
func TestSendToTabKeysFromBindings(t *testing.T) {
	h := newHarness(t, setup{config: strings.Replace(exampleConfig(t),
		`add_front_window = ""`, `add_front_window = "ctrl+alt+shift+3"`, 1)})
	h.advance(50 * time.Millisecond)
	v := h.view()
	if v.Keys.SendToTab != "⌃⌥⇧" || v.Keys.AddFrontWindow != "⌃⌥⇧3" {
		t.Fatalf("keys %+v, want send_to_tab_modifiers ⌃⌥⇧ and add_front_window ⌃⌥⇧3", v.Keys)
	}
	var view, menu []string
	for _, tv := range v.Tabs {
		view = append(view, tv.SendShortcut)
	}
	for _, mt := range h.lastMenuBar().Tabs {
		menu = append(menu, mt.SendKeys)
	}
	want := []string{"⌃⌥⇧1", "⌃⌥⇧2", ""}
	if !slices.Equal(view, want) || !slices.Equal(menu, want) {
		t.Errorf("send shortcuts: view %q, menu %q; want %q", view, menu, want)
	}
}
