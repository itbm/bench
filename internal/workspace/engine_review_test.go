package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/windows"
)

// These tests cover defects found by review: keyboard focus for Web and
// Built-in panes, focus events that Bench causes, Bench's window hiding,
// and where new windows go.

// commands returns the commands sent, and forgets them.
func (h *harness) takeCommands() []Command {
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	out := h.em.commands
	h.em.commands = nil
	return out
}

// focusEvent is a WindowFocused event for a window as it is now.
func (h *harness) focusEvent(id platform.WindowID) platform.Event {
	h.t.Helper()
	w, ok := h.p.WindowState(id)
	if !ok {
		h.t.Fatalf("no window %d", id)
	}
	app, _ := h.p.App(w.BundleID)
	return platform.Event{Kind: platform.WindowFocused, Window: w, App: app}
}

func TestFocusingATerminalPaneGivesItTheKeyboard(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.takeCommands()
	h.p.ResetCalls()
	// Control+Option+Right from Zed to the terminal.
	h.e.Shortcut("focus-pane", "right")
	if got := h.takeCommands(); !slices.Equal(got, []Command{{Name: "focus-terminal", ItemID: "w5"}}) {
		t.Errorf("commands: %v, want focus-terminal for w5", got)
	}
	if len(h.focusCalls()) != 0 {
		t.Errorf("no window should be focused: %v", h.focusCalls())
	}
	// Back to Zed: its window gets the keyboard.
	h.e.Shortcut("focus-pane", "left")
	if got := h.focusCalls(); len(got) != 1 || got[0] != h.windowID(zedID, "bench — switch.go") {
		t.Errorf("focus: %v, want Zed's window", got)
	}
	if got := h.takeCommands(); len(got) != 0 {
		t.Errorf("commands: %v, want none", got)
	}
}

func TestFocusingAWebPaneGivesItTheKeyboard(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Control+Option+1 from Zed to Comms: Slack's view gets the keyboard.
	h.e.Shortcut("tab", "1")
	if got := h.p.FocusedWebView(); got != "w1" {
		t.Errorf("focused web view: %q, want Slack's", got)
	}
	mustOK(t, h.e.FocusPane("p9"))
	if got := h.p.FocusedWebView(); got != "w2" {
		t.Errorf("focused web view: %q, want Outlook's", got)
	}
	// Choosing a window in the palette, which is open: Bench is raised and
	// its web views are hidden, so the focus waits until it's lowered.
	mustOK(t, h.e.SetOverlay(true))
	h.p.ResetCalls()
	mustOK(t, h.e.ShowItem("p8", "w1"))
	if len(h.calls("FocusWebView")) != 0 {
		t.Errorf("focused a hidden web view: %v", h.calls("FocusWebView"))
	}
	mustOK(t, h.e.SetOverlay(false))
	if calls := h.calls("FocusWebView"); len(calls) != 1 || calls[0].Arg != "w1" {
		t.Errorf("FocusWebView calls once lowered: %v", calls)
	}
}

func TestWebFocusedFocusesItsPane(t *testing.T) {
	h := newHarness(t, setup{})
	if h.view().Display.Focus != "p8" {
		t.Fatalf("focus: %s", h.view().Display.Focus)
	}
	h.p.ResetCalls()
	h.p.EmitEvent(platform.Event{Kind: platform.WebFocused, ItemID: "w2"})
	h.drain()
	if got := h.view().Display.Focus; got != "p9" {
		t.Errorf("focus: %s, want Outlook's pane", got)
	}
	if len(h.calls("FocusWebView")) != 0 {
		t.Errorf("the page has the keyboard already: %v", h.calls("FocusWebView"))
	}
}

func TestFocusIsAppliedWhenTheFocusedPanesWindowArrives(t *testing.T) {
	h := newHarness(t, setup{config: `
[settings]
new_windows = "float"

[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Plans"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.apple.iCal" }]
  [[tab.pane]]
  items = [{ app = "com.apple.Safari", title = "Never" }]
`})
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	cal := h.windowID(calendarID, "Calendar")
	if !slices.Contains(h.focusCalls(), cal) {
		t.Errorf("Calendar's window should be focused once it arrives, not when the wait ends: %v", h.focusCalls())
	}
	if h.e.wait == nil {
		t.Error("the switch should still be waiting for Safari's window")
	}
}

func TestHidingAppsOnASwitchDoesntBounceBack(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	code := h.windowID(vscodeID, "index.html — bench-ui")
	mustOK(t, h.e.SwitchTab(0))
	// Hiding Zed makes macOS activate VS Code, whose window is behind it,
	// just before Bench hides VS Code too.
	ev := h.focusEvent(code)
	ev.App.Hidden = false
	h.p.EmitEvent(ev)
	h.drain()
	if got := h.view().Current; got != 0 {
		t.Fatalf("current: %d, want Comms: Bench's own hiding was followed", got)
	}
	// Later the user brings VS Code forward: that's followed.
	h.advance(2 * time.Second)
	h.p.FocusWindow(code)
	h.drain()
	if got := h.view().Current; got != 1 {
		t.Errorf("current: %d, want Code", got)
	}
}

func TestOpeningAnAppOnASwitchDoesntBounceBack(t *testing.T) {
	cfg := strings.Replace(string(config.ExampleTOML()), `items = [{ terminal = "~/code" }]`,
		`items = [{ terminal = "~/code" }, { app = "com.brave.Browser", title = "localhost", url = "https://localhost:3000" }]`, 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	mustOK(t, h.e.SwitchTab(1))
	if len(h.calls("OpenApp")) == 0 {
		t.Fatal("Brave should be opened for the localhost placeholder")
	}
	// Opening Brave activates it, which reports its focused window, which
	// is Research's.
	ev := h.focusEvent(h.windowID(braveID, "Wails v3 docs"))
	ev.App.Hidden = false
	h.p.EmitEvent(ev)
	h.drain()
	if got := h.view().Current; got != 1 {
		t.Errorf("current: %d, want Code: Bench's own OpenApp was followed", got)
	}
}

func TestRaisingAStacksWindowsIsntFollowed(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	mustOK(t, h.e.Rearrange())
	// Raising w17 behind the shown w6 made it Brave's focused window.
	h.p.EmitEvent(h.focusEvent(h.windowID(braveID, "Pull request #42 · bench")))
	h.drain()
	if got := h.view().Display.Panes[0].Active; got != "w6" {
		t.Errorf("active: %s, want w6: Bench's own raise was followed", got)
	}
}

func TestOwnFocusReportedTwice(t *testing.T) {
	h := newHarness(t, setup{})
	// A Zed window in Research too, so that Zed isn't hidden there.
	mustOK(t, h.e.SwitchTab(2))
	notes, _ := h.p.OpenWindow(zedID, "notes.md")
	h.drain()
	mustOK(t, h.e.MoveItemToTab(h.itemOf(notes.ID), 2))
	h.advance(time.Second)

	zed := h.windowID(zedID, "bench — switch.go")
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ShowItem("p11", h.itemOf(zed)))
	// Long after Zed's window was last raised, focusing its pane focuses it
	// again, without arranging: one report.
	h.advance(time.Second)
	h.p.ResetCalls()
	mustOK(t, h.e.FocusPane("p11"))
	if got := h.focusCalls(); !slices.Equal(got, []platform.WindowID{zed}) {
		t.Fatalf("focus calls: %v, want Zed's window", got)
	}
	h.drain()
	mustOK(t, h.e.SwitchTab(2))
	h.p.EmitEvent(h.focusEvent(zed)) // A second report of the same focus.
	h.drain()
	if got := h.view().Current; got != 2 {
		t.Errorf("current: %d, want Research", got)
	}
}

func TestBenchHiddenAndShown(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	if !h.p.IsHidden(zedID) || !h.p.IsHidden(vscodeID) {
		t.Errorf("Code's apps should be hidden with Bench")
	}
	// The user brings Zed forward from the Dock while Bench is hidden:
	// arranging doesn't hide it again.
	h.p.FocusWindow(zed)
	h.drain()
	mustOK(t, h.e.Rearrange())
	if h.p.IsHidden(zedID) {
		t.Error("Zed was hidden again while Bench was hidden")
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if h.p.IsHidden(zedID) || h.p.IsHidden(vscodeID) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Code's windows should be back: Zed hidden %v at %v", h.p.IsHidden(zedID), h.frame(zed))
	}

	// On Comms, the web views are hidden too.
	mustOK(t, h.e.SwitchTab(0))
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	if wv := h.p.WebViews(); wv["w1"].Visible || wv["w2"].Visible {
		t.Errorf("web views should be hidden with Bench: %+v", wv)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if wv := h.p.WebViews(); !wv["w1"].Visible || !wv["w2"].Visible {
		t.Errorf("web views should be back: %+v", wv)
	}
}

func TestBenchHiddenParksWindowsOfAppsPoppedOut(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.PopOut("w18"))
	docs := h.windowID(braveID, "Wails v3 docs")
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	if h.p.IsHidden(braveID) {
		t.Error("Brave has a window popped out, so it shouldn't be hidden")
	}
	if !h.e.win.IsParked(docs) {
		t.Error("Research's Brave windows should be parked with Bench hidden")
	}
	if h.e.win.IsParked(h.windowID(braveID, "Go packages")) {
		t.Error("the popped-out window should stay where it is")
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if h.frame(docs) != h.bodyOf("p14") {
		t.Errorf("docs should be docked again: %v", h.frame(docs))
	}
}

func TestDraggingADividerDoesntUndock(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(3 * time.Second)
	zed := h.windowID(zedID, "bench — switch.go")
	split := h.view().Tabs[1].Root.ID
	mustOK(t, h.e.SetSizes(split, []float64{60, 40}))
	// The mouse is down on the divider, next to Zed, as Zed adjusts its
	// own size.
	h.clk.Advance(400 * time.Millisecond)
	f := h.frame(zed)
	f.W -= 3
	h.p.UserMoveWindow(zed, f)
	h.drain()
	if it := h.item("w3"); it.Status == StatusUndocked {
		t.Fatalf("dragging a divider undocked Zed")
	}
	h.advance(time.Second)
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should be put back in its pane: %v, body %v", h.frame(zed), h.bodyOf("p11"))
	}
	// Long after, a drag is the user's.
	h.advance(3 * time.Second)
	h.p.UserMoveWindow(zed, layout.Rect{X: 300, Y: 300, W: 800, H: 600})
	h.drain()
	if it := h.item("w3"); it.Status != StatusUndocked {
		t.Errorf("w3: %s, want undocked", it.Status)
	}
}

// ruledConfig gives Code a pane for Brave windows about GitHub, and sends
// new Brave windows to Research.
func ruledConfig(newWindows string) string {
	cfg := strings.Replace(string(config.ExampleTOML()), `items = [{ terminal = "~/code" }]`,
		`items = [{ terminal = "~/code" }, { app = "com.brave.Browser", title = "GitHub" }]`, 1)
	cfg = strings.Replace(cfg, `new_windows = "app-pane"`, `new_windows = "`+newWindows+`"`, 1)
	// So that switching to Code doesn't wait for a GitHub window.
	cfg = strings.Replace(cfg, "open_missing_apps = true", "open_missing_apps = false", 1)
	return cfg + "\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"\n"
}

func TestNewWindowOrder(t *testing.T) {
	t.Run("a tab rule comes before a title filter", func(t *testing.T) {
		cfg := ruledConfig("app-pane") + "\n[[rule]]\napp = \"com.brave.Browser\"\ntab = \"Research\"\n"
		h := newHarness(t, setup{config: cfg})
		mustOK(t, h.e.SwitchTab(1))
		w, _ := h.p.OpenWindow(braveID, "PR #1 · GitHub")
		h.drain()
		if it := h.item(h.itemOf(w.ID)); it.Tab != 2 {
			t.Errorf("the new window is in tab %d pane %s, want Research", it.Tab, it.Pane)
		}
	})
	t.Run("new_windows float comes before a title filter", func(t *testing.T) {
		h := newHarness(t, setup{config: ruledConfig("float")})
		mustOK(t, h.e.SwitchTab(1))
		w, _ := h.p.OpenWindow(braveID, "PR #1 · GitHub")
		h.drain()
		if id := h.itemOf(w.ID); id != "" || !slices.Contains(floatingIDs(h.view()), floatingID(w.ID)) {
			t.Errorf("the new window should float: item %q", id)
		}
	})
	// The spec's app-pane order: the pane of the app's most recently used
	// window, in the current tab and then other tabs, before the pane the
	// config gives the app, which is where title filters count.
	t.Run("app-pane: the recent pane comes before a title filter", func(t *testing.T) {
		h := newHarness(t, setup{config: ruledConfig("app-pane")})
		mustOK(t, h.e.SwitchTab(2))
		w, _ := h.p.OpenWindow(braveID, "PR #1 · GitHub")
		h.drain()
		if it := h.item(h.itemOf(w.ID)); it.Tab != 2 || it.Pane != h.paneOf("w7") {
			t.Errorf("the new window is in tab %d pane %s, want Research's Brave pane %s", it.Tab, it.Pane, h.paneOf("w7"))
		}
	})
}

func TestAddAppLeavesRemembered(t *testing.T) {
	titles := []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"}
	cfg := string(config.ExampleTOML())
	h := newHarness(t, setup{config: cfg, world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows(titles...))
	}})
	mustOK(t, h.e.SwitchTab(2))
	h.p.QuitApp(braveID)
	h.drain()
	mustOK(t, h.e.SwitchTab(1))
	// From Code, + > Apps > Brave: Brave opens and restores its windows,
	// which are Research's.
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: braveID}))
	h.drain()
	for _, id := range []string{"w6", "w17", "w18"} {
		if it := h.item(id); it.Tab != 2 || it.Status == StatusMissing {
			t.Errorf("%s: tab %d status %s, want Research's window back", id, it.Tab, it.Status)
		}
	}
	if got := h.view().Current; got != 1 {
		t.Fatalf("current: %d, want Code", got)
	}
	// None came to the pane, so Bench opens a new one there.
	h.advance(2 * time.Second)
	w := h.windowID(braveID, platform.DefaultNewWindowTitle)
	if it := h.item(h.itemOf(w)); it.Pane != "p12" {
		t.Errorf("the new Brave window is in %s, want p12", it.Pane)
	}
}

func TestAddAppFloatRuleComesFirst(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("Picture in picture", "Docs"))
	}})
	mustOK(t, h.e.SwitchTab(2))
	h.p.QuitApp(braveID)
	h.drain()
	mustOK(t, h.e.SwitchTab(1))
	// Forget Research's Brave windows, so the launch windows come here.
	for _, id := range []string{"w6", "w17", "w18"} {
		h.e.lock()
		h.e.model.Remove(id)
		h.e.unlock()
	}
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: braveID}))
	h.drain()
	pip := h.windowID(braveID, "Picture in picture")
	if id := h.itemOf(pip); id != "" {
		t.Errorf("picture in picture should float, not be %s in %s", id, h.paneOf(id))
	}
	if it := h.item(h.itemOf(h.windowID(braveID, "Docs"))); it.Pane != "p12" {
		t.Errorf("Docs is in %s, want p12", it.Pane)
	}
}

func TestAddAppIncludesPoppedOutWindows(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.PopOut("w3"))
	h.p.ResetCalls()
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: zedID}))
	if len(h.calls("NewWindow")) != 0 {
		t.Errorf("Zed's popped-out window should be added, not a new one opened")
	}
	if it := h.item("w3"); it.Pane != "p12" || h.e.model.IsFloating("w3") {
		t.Errorf("w3: pane %q", it.Pane)
	}
}

func TestArrangingWhileAnAppQuitsKeepsItsItems(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	// Brave is quitting: its windows close one by one, before it ends.
	for _, title := range []string{"Wails v3 docs", "Pull request #42 · bench"} {
		h.p.SetWindowErr(h.windowID(braveID, title), "Window", platform.ErrNoWindow)
	}
	mustOK(t, h.e.Rearrange())
	h.advance(500 * time.Millisecond)
	h.p.QuitApp(braveID)
	h.advance(2 * time.Second)
	for _, id := range []string{"w6", "w17", "w18"} {
		if _, ok := h.view().Items[id]; !ok {
			t.Errorf("%s was removed, but it should wait for Brave to open again", id)
		}
	}
}

func TestTurningHideOtherTabsOffShowsWhatItHid(t *testing.T) {
	h := newHarness(t, setup{})
	// Brave has windows in Code and Research.
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	docs := h.windowID(braveID, "Wails v3 docs")
	if !h.e.win.IsParked(docs) || !h.p.IsHidden(obsidianID) {
		t.Fatal("Research's windows should be out of sight on Code")
	}
	draft := draftOf(h.view(), "Code", "custom", "stack")
	draft.Settings.HideOtherTabs = false
	mustOK(t, h.e.SaveTab(1, draft))
	if h.e.win.IsParked(docs) || h.p.IsHidden(obsidianID) {
		t.Errorf("turning hide_other_tabs off should show Research's windows: docs parked %v, Obsidian hidden %v",
			h.e.win.IsParked(docs), h.p.IsHidden(obsidianID))
	}
}

func TestStackViewSplitsSideBySide(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(1))
	if h.view().SizeClass != "compact" {
		t.Fatalf("size class: %s", h.view().SizeClass)
	}
	h.e.Shortcut("move-window", "right")
	v := h.view()
	fresh := v.Items["w3"].Pane
	var a, b layout.Rect
	for _, p := range v.Display.Panes {
		switch p.ID {
		case "p11":
			a = p.Rect
		case fresh:
			b = p.Rect
		}
	}
	if fresh == "p11" || b.X <= a.X || b.Y != a.Y {
		t.Errorf("the new pane %s at %v should be to the right of p11 at %v", fresh, b, a)
	}
	// It's side by side on a monitor too.
	mustOK(t, h.e.SetMode("wide"))
	v = h.view()
	for _, p := range v.Display.Panes {
		switch p.ID {
		case "p11":
			a = p.Rect
		case fresh:
			b = p.Rect
		}
	}
	if b.X <= a.X || b.Y != a.Y {
		t.Errorf("wide: %s at %v, p11 at %v", fresh, b, a)
	}
}

func TestUndockedWindowOfAnotherTabIsParked(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	h.advance(3 * time.Second)
	docs := h.windowID(braveID, "Wails v3 docs")
	where := layout.Rect{X: 300, Y: 300, W: 800, H: 600}
	h.p.UserMoveWindow(docs, where)
	h.drain()
	if h.item("w6").Status != StatusUndocked {
		t.Fatalf("w6: %s", h.item("w6").Status)
	}
	mustOK(t, h.e.SwitchTab(1))
	if !h.e.win.IsParked(docs) {
		t.Errorf("Research's undocked window stays over Code at %v", h.frame(docs))
	}
	mustOK(t, h.e.SwitchTab(2))
	if h.frame(docs) != where {
		t.Errorf("back on Research, the undocked window is at %v, want %v", h.frame(docs), where)
	}
}

func TestParkedFileProblemsAreShown(t *testing.T) {
	h := newHarness(t, setup{})
	path := filepath.Join(h.dir, config.ParkedFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "in-the-way"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	if n := h.view().Notices; !hasPrefix(n, "Bench couldn't record the windows it moved out of sight") {
		t.Errorf("notices: %v", n)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	mustOK(t, h.e.Rearrange())
	if n := h.view().Notices; hasPrefix(n, "Bench couldn't record") {
		t.Errorf("the notice should go once the file is written: %v", n)
	}
	if pf, err := os.ReadFile(path); err != nil || !strings.Contains(string(pf), "Wails v3 docs") {
		t.Errorf("parked.json: %s %v", pf, err)
	}
}

func TestOverlappingAppsHiddenAreShownOnQuit(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.ActivateApp(benchID)
	h.drain()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder's window covers Bench, so it should be hidden")
	}
	mustOK(t, h.e.Close())
	if h.p.IsHidden(finderID) {
		t.Error("Finder should be shown again when Bench quits")
	}
}

func TestNoticesFromASourceAreListedOnce(t *testing.T) {
	h := newHarness(t, setup{})
	h.e.SetNotices("shortcuts", []string{"A"})
	h.e.SetNotices("shortcuts", nil)
	h.e.SetNotices("shortcuts", []string{"B"})
	if got := h.view().Notices; !slices.Equal(got, []string{"B"}) {
		t.Errorf("notices: %v, want [B]", got)
	}
}

func TestPoppingOutAWindowThatCantBeMovedKeepsItParked(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	docs := h.windowID(braveID, "Wails v3 docs")
	if !h.e.win.IsParked(docs) {
		t.Fatal("docs should be parked")
	}
	h.p.SetWindowErr(docs, "SetFrame", errors.New("kAXErrorCannotComplete"))
	mustOK(t, h.e.PopOut("w6"))
	if !h.e.win.IsParked(docs) {
		t.Error("the manager forgot a window it couldn't put back")
	}
	if !hasPrefix(h.em.toasts, "Bench couldn't move") {
		t.Errorf("toasts: %q", h.em.toasts)
	}
	// On quit, Bench tries again.
	h.p.SetWindowErr(docs, "SetFrame", nil)
	mustOK(t, h.e.Close())
	if !windows.OnScreen(h.frame(docs), []platform.Screen{monitor}) {
		t.Errorf("docs is still off screen at %v", h.frame(docs))
	}
}

func TestQuitKeepsRecordsOfWindowsItCouldntPutBack(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	docs := h.windowID(braveID, "Wails v3 docs")
	h.p.MoveToOtherDesktop(docs)
	if err := h.e.Close(); !errors.Is(err, platform.ErrOtherDesktop) {
		t.Fatalf("Close: %v, want ErrOtherDesktop", err)
	}
	data, err := os.ReadFile(filepath.Join(h.dir, config.ParkedFile))
	if err != nil || !strings.Contains(string(data), "Wails v3 docs") {
		t.Fatalf("parked.json should still list docs: %s %v", data, err)
	}
	// The next run puts the window back as it starts, once it can.
	h.p.ReturnToDesktop(docs)
	h.drain()
	h.p.ResetCalls()
	h.restart()
	putBack := false
	for _, c := range h.calls("SetFrame") {
		putBack = putBack || c.Window == docs && windows.OnScreen(c.Rect, []platform.Screen{monitor})
	}
	if !putBack {
		t.Errorf("the next run didn't put docs back: %v", h.calls("SetFrame"))
	}
}

func TestStartKeepsRecordsOfWindowsItStillCantPutBack(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	docs := h.windowID(braveID, "Wails v3 docs")
	h.p.MoveToOtherDesktop(docs)
	if err := h.e.Close(); !errors.Is(err, platform.ErrOtherDesktop) {
		t.Fatalf("Close: %v, want ErrOtherDesktop", err)
	}
	// The next run starts while the window is still on the other desktop,
	// and parks Research's other Brave windows: the file must still list
	// docs, and quitting puts it back once it can be reached.
	h.restart()
	data, err := os.ReadFile(filepath.Join(h.dir, config.ParkedFile))
	if err != nil || !strings.Contains(string(data), "Wails v3 docs") || !strings.Contains(string(data), "Pull request #42") {
		t.Fatalf("parked.json should list docs and the windows parked since: %s %v", data, err)
	}
	if !hasPrefix(h.view().Notices, "Bench couldn't put back every window") {
		t.Errorf("notices: %v", h.view().Notices)
	}
	h.p.ReturnToDesktop(docs)
	h.drain()
	mustOK(t, h.e.Close())
	if !windows.OnScreen(h.frame(docs), []platform.Screen{monitor}) {
		t.Errorf("docs is still off screen at %v", h.frame(docs))
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.ParkedFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("parked.json should be gone once everything is back: %v", err)
	}
}

func TestSaveTabKeepsTheConfiguredWebAddress(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebURL, ItemID: "w1", Body: "https://app.slack.com/client/T1/C2"})
	h.drain()
	mustOK(t, h.e.SaveTab(0, draftOf(h.view(), "Chat", "side", "tabs")))
	if got := savedConfig(t, h).Tabs[0].Panes[0].Items[0].Web; got != "https://app.slack.com/client" {
		t.Errorf("config.toml has web = %q, want the configured address", got)
	}
}

func TestWebItemsKeepTheirAddressAcrossRestarts(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebURL, ItemID: "w1", Body: "https://app.slack.com/client/T1/C2"})
	h.drain()
	// Drag Slack onto Outlook's pane edge, then quit and reopen Bench.
	mustOK(t, h.e.MoveItem("w1", 0, "p9", "bottom"))
	h.restart()
	if got := h.p.WebViews()["w1"].View.URL; got != "https://app.slack.com/client" {
		t.Errorf("Slack loads %q after a restart, want the address it was given", got)
	}
	if got := h.item("w1").URL; got != "https://app.slack.com/client/T1/C2" {
		t.Errorf("Slack's current address: %q", got)
	}
	// Reset layout still finds Slack's config pane.
	mustOK(t, h.e.ResetLayout(0))
	leaves := leafIDs(h.tabNamed("Comms"))
	if len(leaves) != 2 || h.paneOf("w1") != leaves[0] || h.paneOf("w2") != leaves[1] {
		t.Errorf("after Reset layout: Slack in %s, Outlook in %s, panes %v", h.paneOf("w1"), h.paneOf("w2"), leaves)
	}
}

func TestResetLayoutBringsBackClosedConfigItems(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w1"))
	h.p.EmitEvent(platform.Event{Kind: platform.WebViewClosed, ItemID: "w1"})
	h.drain()
	if _, ok := h.view().Items["w1"]; ok {
		t.Fatal("closing Slack's window should remove it")
	}
	mustOK(t, h.e.ResetLayout(0))
	tv := h.tabNamed("Comms")
	leaves := layout.Leaves(tv.Root)
	if len(leaves) != 2 || len(leaves[0].Items) != 1 {
		t.Fatalf("Comms after Reset layout: %+v", leaves)
	}
	slack := h.item(leaves[0].Items[0])
	if slack.Kind != KindWeb || slack.URL != "https://app.slack.com/client" {
		t.Errorf("Slack should be back in its pane: %+v", slack)
	}
	if got := h.p.WebViews()[slack.ID].View.Profile; got != "slack" {
		t.Errorf("Slack's profile: %q, want slack, so it's still signed in", got)
	}
}

func TestWebPageAddedToConfigByHandAppears(t *testing.T) {
	data := savedState(t, nil)
	cfg := strings.Replace(string(config.ExampleTOML()),
		`items = [{ web = "https://outlook.office.com/mail/", profile = "outlook", name = "Outlook" }]`,
		`items = [{ web = "https://outlook.office.com/mail/", profile = "outlook", name = "Outlook" }, { web = "https://example.com", name = "Example" }]`, 1)
	h := newHarness(t, setup{config: cfg, state: writeState(data)})
	if got := h.tabNamed("Comms").Windows; got != 3 {
		t.Errorf("Comms has %d windows, want the new web page too", got)
	}
}

func TestPoppedOutWebPageReopensItsWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w2"))
	h.p.ResetCalls()
	h.restart()
	if calls := h.calls("PopOutWebView"); len(calls) != 1 || calls[0].Arg != "w2" {
		t.Errorf("Outlook's window should open again: %v", calls)
	}
	if !h.e.model.IsFloating("w2") {
		t.Error("Outlook should still be out of Bench")
	}
}

func TestPoppedOutTerminalReopensItsWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.PopOut("w5"))
	h.em.mu.Lock()
	h.em.popped = nil
	h.em.mu.Unlock()
	h.restart()
	h.em.mu.Lock()
	popped := slices.Clone(h.em.popped)
	h.em.mu.Unlock()
	if !slices.Equal(popped, []string{"w5"}) {
		t.Errorf("terminal windows opened: %v, want w5's", popped)
	}
	if _, ok := h.terms.started["w5"]; !ok {
		t.Error("the terminal's session should start")
	}
}
