package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// lastMenuBar returns what the menu bar item was last told to show.
func (h *harness) lastMenuBar() MenuBar {
	h.t.Helper()
	bars := h.em.menuBars()
	if len(bars) == 0 {
		h.t.Fatal("the menu bar item was never told what to show")
	}
	return bars[len(bars)-1]
}

// TestMenuBarSentToShell checks what the shell is told to show in the menu
// bar item: the setting, the current tab, and the tabs with their
// shortcuts, as Wails accelerators.
func TestMenuBarSentToShell(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(50 * time.Millisecond)
	want := MenuBar{
		Show:    config.MenuBarName,
		Current: 1,
		Tabs: []MenuBarTab{
			{Name: "Comms", Shortcut: "Ctrl+Option+1", Keys: "⌃⌥1"},
			{Name: "Code", Shortcut: "Ctrl+Option+2", Keys: "⌃⌥2"},
			{Name: "Research", Shortcut: "Ctrl+Option+3", Keys: "⌃⌥3"},
		},
		Jump: "Ctrl+Option+Space",
		Mode: "auto",
		Keys: h.view().Keys,
		Pane: h.lastMenuBar().Pane,
	}
	if got := h.lastMenuBar(); !got.same(want) {
		t.Errorf("menu bar item = %+v, want %+v", got, want)
	}
	if got := h.lastMenuBar().TabName(); got != "Code" {
		t.Errorf("tab name = %q, want Code", got)
	}
	if got := h.lastMenuBar().Keys.Jump; got != "⌃⌥Space" {
		t.Errorf("the app menu shows the jump shortcut as %q, want ⌃⌥Space", got)
	}

	// A change that neither the menu bar item nor the app menu shows isn't
	// sent again.
	sent := len(h.em.menuBars())
	area := paneArea
	area.H -= 20
	mustOK(t, h.e.SetPaneArea(area))
	h.advance(50 * time.Millisecond)
	if got := len(h.em.menuBars()); got != sent {
		t.Errorf("a smaller pane area sent the menu bar item %d more times", got-sent)
	}

	// Settings changes what it shows.
	s := h.view().Settings
	s.MenuBar = config.MenuBarIcon
	mustOK(t, h.e.SaveSettings(s))
	h.advance(50 * time.Millisecond)
	if got := h.lastMenuBar().Show; got != config.MenuBarIcon {
		t.Errorf("after Settings, menu bar item shows %q, want icon", got)
	}
	if got := savedConfig(t, h).Settings.MenuBar; got != config.MenuBarIcon {
		t.Errorf("config.toml's menu_bar = %q, want icon", got)
	}

	// The View menu ticks the layout mode, however it's chosen.
	mustOK(t, h.e.SetMode("compact"))
	h.advance(50 * time.Millisecond)
	if got := h.lastMenuBar().Mode; got != "compact" {
		t.Errorf("after SetMode, the View menu ticks %q, want compact", got)
	}
}

// TestMenuBarPane checks what the app menu's Pane menu is told about the
// focused pane: the window it shows, as the frontend draws it, and what can
// be done with the pane.
func TestMenuBarPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(50 * time.Millisecond)
	for _, p := range h.view().Display.Panes {
		mustOK(t, h.e.FocusPane(p.ID))
		h.advance(50 * time.Millisecond)
		got := h.lastMenuBar().Pane
		if got.ID != p.ID || got.Item != shownIn(p) || got.Item == "" {
			t.Errorf("focusing %s: Pane menu for %q showing %q, want %q", p.ID, got.ID, got.Item, shownIn(p))
		}
		if !got.CanSplit || !got.CanZoom || got.CanClose || got.Undocked || got.NoWindow {
			t.Errorf("focusing %s, a pane with a window: %+v, want it to split and zoom, but not close", p.ID, got)
		}
	}

	// An empty pane can close, but has nothing to split.
	id, err := h.e.SplitPane(h.view().Display.Focus, "right")
	mustOK(t, err)
	h.advance(50 * time.Millisecond)
	got := h.lastMenuBar().Pane
	if got.ID != id || got.Item != "" || !got.CanClose || got.CanSplit {
		t.Errorf("an empty pane's Pane menu: %+v, want %s, closable, with nothing to split", got, id)
	}

	// While a pane is zoomed, Zoom Pane shows every pane again.
	mustOK(t, h.e.CancelSplit(id))
	mustOK(t, h.e.ToggleZoom())
	h.advance(50 * time.Millisecond)
	if bar := h.lastMenuBar(); !bar.Zoomed || !bar.Pane.CanZoom {
		t.Errorf("with a pane zoomed: zoomed %v, can zoom %v; want both", bar.Zoomed, bar.Pane.CanZoom)
	}
	mustOK(t, h.e.ToggleZoom())

	// The "tabs" view shows one pane, which can't split, zoom, or close:
	// Comms is shown that way in the compact layout.
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SetMode("compact"))
	h.advance(50 * time.Millisecond)
	tabs := h.view().Display
	if !tabs.Compact {
		t.Fatal("the compact layout didn't show the tab as one pane")
	}
	got = h.lastMenuBar().Pane
	if got.ID != "compact" || got.Item == "" || got.CanSplit || got.CanZoom || got.CanClose {
		t.Errorf("the \"tabs\" view's Pane menu: %+v, want the compact pane's window, with nothing to split, zoom, or close", got)
	}
}

// TestMenuBarReopen checks that the Tab menu's Reopen Closed Tab names the
// tab closed last.
func TestMenuBarReopen(t *testing.T) {
	h := newHarness(t, setup{})
	if got := h.lastMenuBar().Closed; got != "" {
		t.Errorf("before a tab is closed, Reopen Closed Tab is for %q", got)
	}
	mustOK(t, h.e.DeleteTab(2))
	h.advance(50 * time.Millisecond)
	if got := h.lastMenuBar().Closed; got != "Research" {
		t.Errorf("after deleting Research, Reopen Closed Tab is for %q", got)
	}
}

// TestMenuBarFollowsShortcuts checks that the menu bar item's menu shows the
// shortcuts that config.toml sets, and none for a shortcut that's off or for
// tabs after the ninth.
func TestMenuBarFollowsShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	editConfig(t, h, `tab_modifiers = "ctrl+alt"`, `tab_modifiers = "cmd+alt"`)
	editConfig(t, h, `jump = "ctrl+alt+space"`, `jump = ""`)
	editConfig(t, h, `menu_bar = "name"`, `menu_bar = "hidden"`)
	mustOK(t, h.e.ReloadConfig())
	h.advance(50 * time.Millisecond)
	got := h.lastMenuBar()
	if got.Show != config.MenuBarHidden || got.Jump != "" || got.Tabs[0].Shortcut != "Option+Cmd+1" || got.Tabs[0].Keys != "⌥⌘1" {
		t.Errorf("menu bar item = %+v, want hidden, no jump, and Option+Cmd+1 (⌥⌘1) for Comms", got)
	}
	if got.Keys.Jump != "" || got.Keys.Tabs != "⌥⌘" {
		t.Errorf("the app menu's keys = %+v, want no jump, and ⌥⌘ for tabs", got.Keys)
	}

	var b strings.Builder
	for i := range 11 {
		b.WriteString("[[tab]]\nname = \"Tab " + string(rune('A'+i)) + "\"\nlayout = \"single\"\n\n")
	}
	writeConfig(t, h, b.String())
	mustOK(t, h.e.ReloadConfig())
	h.advance(50 * time.Millisecond)
	got = h.lastMenuBar()
	if len(got.Tabs) != 11 || got.Tabs[8].Shortcut != "Ctrl+Option+9" || got.Tabs[9].Shortcut != "" || got.Tabs[10].Shortcut != "" {
		t.Errorf("tabs = %+v, want 11 with shortcuts for the first nine", got.Tabs)
	}
	if got.Show != config.MenuBarName {
		t.Errorf("a file without menu_bar shows %q, want name", got.Show)
	}
}

// TestTabShortcutBringsHiddenBenchForward checks that with Bench hidden, the
// tab shortcuts, last_tab among them, switch tabs and bring Bench forward,
// as the menu bar item does, while the other shortcuts wait for Bench to
// come back (decision 16).
func TestTabShortcutBringsHiddenBenchForward(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)

	// With Bench shown, a tab shortcut doesn't need to bring it forward.
	h.e.Shortcut(shortcuts.ActionTab, "1")
	h.e.Shortcut(shortcuts.ActionTab, "2")
	if got := h.em.broughtForward(); got != 0 {
		t.Fatalf("Bench was brought forward %d times while shown", got)
	}

	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.ResetCalls()
	for _, tc := range []struct {
		action, arg string
		current     int
	}{
		{shortcuts.ActionTab, "3", 2},
		{shortcuts.ActionNextTab, "", 0},
		{shortcuts.ActionPreviousTab, "", 2},
		// The last used tab goes back and forth.
		{shortcuts.ActionLastTab, "", 0},
		{shortcuts.ActionLastTab, "", 2},
	} {
		before := h.em.broughtForward()
		h.e.Shortcut(tc.action, tc.arg)
		if got := h.view().Current; got != tc.current {
			t.Errorf("%s %s: current %d, want %d", tc.action, tc.arg, got, tc.current)
		}
		if got := h.em.broughtForward() - before; got != 1 {
			t.Errorf("%s %s brought Bench forward %d times, want once", tc.action, tc.arg, got)
		}
	}
	// Until Bench is back, nothing is focused or shown.
	h.drain()
	if len(h.focusCalls()) != 0 || !h.p.IsHidden(braveID) {
		t.Errorf("nothing should be focused or shown until Bench is back: focus %v, Brave hidden %v", h.focusCalls(), h.p.IsHidden(braveID))
	}

	// A refusal brings Bench forward too, for its message to show.
	before := h.em.broughtForward()
	h.e.Shortcut(shortcuts.ActionTab, "9")
	if got := h.em.broughtForward() - before; got != 1 || h.em.lastToast() != "There's no tab 9." {
		t.Errorf("tab 9: brought forward %d times, toast %q", got, h.em.lastToast())
	}

	// Once Bench is back, the chosen tab's focused window is docked and
	// focused, as after the menu bar item.
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	brave := h.windowID(braveID, "Wails v3 docs")
	if got := h.focusCalls(); !slices.Contains(got, brave) || h.frame(brave) != h.bodyOf("p14") {
		t.Errorf("once Bench is back, Research's focused window should be docked and focused: focus %v, frame %v", got, h.frame(brave))
	}
	before = h.em.broughtForward()
	h.e.Shortcut(shortcuts.ActionTab, "2")
	if got := h.em.broughtForward(); got != before {
		t.Errorf("a tab shortcut brought Bench forward once it was back")
	}

	// With Bench hidden again, the other shortcuts wait.
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	before = h.em.broughtForward()
	for _, sc := range [][2]string{
		{shortcuts.ActionFocusPane, "right"},
		{shortcuts.ActionMoveWindow, "left"},
		{shortcuts.ActionNextWindow, ""},
		{shortcuts.ActionPreviousWindow, ""},
		{shortcuts.ActionRearrange, ""},
	} {
		h.e.Shortcut(sc[0], sc[1])
	}
	if got := h.em.broughtForward(); got != before {
		t.Errorf("pane, window, and rearrange shortcuts brought Bench forward %d times", got-before)
	}
}

// TestTabShortcutBringsBenchForwardAfterHideOthers checks that a tab
// shortcut brings Bench forward when another app's Hide Others hid it,
// which decision 16 doesn't count as hiding Bench's window.
func TestTabShortcutBringsBenchForwardAfterHideOthers(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	if err := h.p.Hide(h.bench); err != nil {
		t.Fatal(err)
	}
	h.e.Shortcut(shortcuts.ActionTab, "3")
	if got := h.view().Current; got != 2 {
		t.Errorf("current %d, want Research", got)
	}
	if got := h.em.broughtForward(); got != 1 {
		t.Errorf("brought Bench forward %d times, want once", got)
	}
}

// TestTabShortcutWithShowOnTabShortcutOff checks that with
// show_on_tab_shortcut off, a tab shortcut switches tabs while Bench is
// hidden but leaves it hidden, until the user brings it back.
func TestTabShortcutWithShowOnTabShortcutOff(t *testing.T) {
	cfg := strings.Replace(string(config.ExampleTOML()), "show_on_tab_shortcut = true", "show_on_tab_shortcut = false", 1)
	h := newHarness(t, setup{config: cfg})
	if h.view().Settings.ShowOnTabShortcut {
		t.Fatal("the test's config should turn show_on_tab_shortcut off")
	}
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.e.Shortcut(shortcuts.ActionTab, "3")
	h.e.Shortcut(shortcuts.ActionNextTab, "")
	if got := h.view().Current; got != 0 {
		t.Errorf("current %d, want Comms", got)
	}
	if got := h.em.broughtForward(); got != 0 {
		t.Errorf("brought Bench forward %d times, want none", got)
	}

	// Settings turns it back on.
	s := h.view().Settings
	s.ShowOnTabShortcut = true
	mustOK(t, h.e.SaveSettings(s))
	h.e.Shortcut(shortcuts.ActionTab, "2")
	if got := h.em.broughtForward(); got != 1 {
		t.Errorf("after Settings turned it on, brought Bench forward %d times, want once", got)
	}
	if !savedConfig(t, h).Settings.ShowOnTabShortcut {
		t.Error("config.toml should have show_on_tab_shortcut on")
	}
}

// TestProblemReport checks what Help ▸ Report a Problem… fills in: the
// displays, with the one Bench is on, the layout, the permissions, and the
// notices.
func TestProblemReport(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	r := h.e.ProblemReport()
	want := []ReportScreen{{Name: "Monitor", W: 2560, H: 1440, Primary: true, Bench: true}}
	if !slices.Equal(r.Screens, want) {
		t.Errorf("screens = %+v, want %+v", r.Screens, want)
	}
	if r.Mode != "auto" || r.SizeClass != "wide" || r.Density != "standard" {
		t.Errorf("layout = %s, %s, %s; want auto, wide, standard", r.Mode, r.SizeClass, r.Density)
	}
	if r.Accessibility {
		t.Error("the report says Bench has Accessibility permission, which it hasn't")
	}
	var notices []string
	for _, n := range h.view().Notices {
		notices = append(notices, n.Text)
	}
	if len(notices) == 0 || !slices.Equal(r.Notices, notices) {
		t.Errorf("notices = %q, want the status bar's, %q", r.Notices, notices)
	}
}
