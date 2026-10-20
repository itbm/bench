package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestStartOnCommsHidesOtherTabsApps(t *testing.T) {
	h := newHarness(t, setup{})
	for _, id := range []string{zedID, vscodeID, braveID, obsidianID} {
		if !h.p.IsHidden(id) {
			t.Errorf("%s should be hidden: all its items are in other tabs", id)
		}
	}
	if h.p.IsHidden(finderID) {
		t.Errorf("Finder has no items and hide_other_apps is off")
	}
	wv := h.p.WebViews()
	if !wv["w1"].Visible || !wv["w2"].Visible {
		t.Errorf("Comms's web views should show: %+v", wv)
	}
	if wv["w1"].View.URL != "https://app.slack.com/client" || wv["w1"].View.Profile != "slack" || wv["w1"].View.Name != "Slack" {
		t.Errorf("slack's web view: %+v", wv["w1"].View)
	}
	// Web views are placed at their pane bodies, in content coordinates.
	for _, p := range h.view().Display.Panes {
		if p.ID == "p8" && wv["w1"].Frame != p.Body {
			t.Errorf("slack's frame %v, body %v", wv["w1"].Frame, p.Body)
		}
	}
	if h.terms.starts != 0 {
		t.Errorf("Code's terminal started before Code was shown")
	}
}

func TestSwitchToCodeDocksAndFocuses(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if h.p.IsHidden(zedID) || h.p.IsHidden(vscodeID) {
		t.Errorf("Code's apps should be shown")
	}
	body := h.bodyOf("p11")
	if h.frame(zed) != body || h.frame(code) != body {
		t.Errorf("Zed %v, VS Code %v, body %v: both should fill the body", h.frame(zed), h.frame(code), body)
	}
	z := h.p.ZOrder()
	if slices.Index(z, zed) > slices.Index(z, code) {
		t.Errorf("Zed (shown) should be in front of VS Code: %v", z)
	}
	if !slices.Equal(h.focusCalls(), []platform.WindowID{zed}) {
		t.Errorf("focus: %v, want Zed's window", h.focusCalls())
	}
	wv := h.p.WebViews()
	if wv["w1"].Visible || wv["w2"].Visible {
		t.Errorf("Comms's web views should be hidden")
	}
	if opts, ok := h.terms.started["w5"]; !ok || opts.Dir != "~/code" {
		t.Errorf("the terminal should start in ~/code: %+v %v", opts, ok)
	}
	if got := h.item("w5").Title; got != "zsh — ~/code" {
		t.Errorf("terminal title: %q", got)
	}
	if !h.p.IsHidden(braveID) || !h.p.IsHidden(obsidianID) {
		t.Errorf("Research's apps should stay hidden")
	}
	if h.view().Current != 1 {
		t.Errorf("current: %d", h.view().Current)
	}
}

func TestSwitchToWebPaneDoesNotFocus(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(0))
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(0))
	if len(h.focusCalls()) != 0 {
		t.Errorf("a Web item's pane shouldn't focus a window: %v", h.focusCalls())
	}
}

func TestSwitchingParksWindowsOfAppsInBothTabs(t *testing.T) {
	h := newHarness(t, setup{})
	// Put one Brave window in Code: Brave now has items in both tabs.
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w18", 1))
	mustOK(t, h.e.SwitchTab(1))
	pkgs := h.windowID(braveID, "Go packages")
	docs := h.windowID(braveID, "Wails v3 docs")
	if h.p.IsHidden(braveID) {
		t.Errorf("Brave has an item in Code, so it shouldn't be hidden")
	}
	if !h.e.win.IsParked(docs) {
		t.Errorf("Research's Brave windows should be parked")
	}
	if h.e.win.IsParked(pkgs) {
		t.Errorf("Code's Brave window shouldn't be parked")
	}
	f := h.frame(docs)
	if f.X != monitor.Frame.W-1 || f.Y != monitor.Frame.H-1 {
		t.Errorf("parked frame: %v", f)
	}
	// Back to Research: unparked and docked again.
	mustOK(t, h.e.SwitchTab(2))
	if h.e.win.IsParked(docs) || h.frame(docs) != h.bodyOf("p14") {
		t.Errorf("Research's Brave window wasn't docked again: %v", h.frame(docs))
	}
}

func TestWithoutHideOtherTabs(t *testing.T) {
	cfg := strings.Replace(string(config.DefaultTOML()), "hide_other_tabs = true", "hide_other_tabs = false", 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	if h.p.IsHidden(braveID) || h.p.IsHidden(obsidianID) {
		t.Errorf("without hide_other_tabs nothing is hidden")
	}
	if len(h.e.win.Parked()) != 0 {
		t.Errorf("without hide_other_tabs nothing is parked: %v", h.e.win.Parked())
	}
}

func TestHideOtherApps(t *testing.T) {
	cfg := strings.Replace(string(config.DefaultTOML()), "hide_other_apps = false", "hide_other_apps = true", 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	if !h.p.IsHidden(finderID) {
		t.Errorf("Finder has no items and should be hidden")
	}
	if h.p.IsHidden(benchID) {
		t.Errorf("Bench mustn't hide itself")
	}
}

// launchConfig has a tab with apps that aren't running.
const launchConfig = `
[settings]
launch_timeout = "3s"

[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Plan"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.apple.iCal" }]
  [[tab.pane]]
  items = [{ app = "com.apple.Safari", title = "Apple", url = "https://apple.com" }]
`

func TestSwitchOpensMissingApps(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(time.Second), platform.AppLaunchWindows("Calendar"))
		p.InstallApp(safariID, "Safari", platform.AppLaunchDelay(500*time.Millisecond),
			platform.AppURLWindow("https://apple.com", "Apple"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	opened := h.p.CallsTo("OpenApp")
	if len(opened) != 2 || opened[0].BundleID != calendarID || opened[1].Arg != "https://apple.com" {
		t.Fatalf("OpenApp calls: %v", opened)
	}
	// "Opening Calendar…" until its window arrives.
	if it := h.item("w2"); it.Status != StatusLaunching || it.StatusText != "Opening Calendar…" {
		t.Errorf("w2: %s %q", it.Status, it.StatusText)
	}
	h.advance(600 * time.Millisecond)
	if it := h.item("w3"); it.Status != StatusReady || it.Title != "Apple" {
		t.Errorf("Safari's window: %+v", it)
	}
	if it := h.item("w2"); it.Status != StatusLaunching {
		t.Errorf("Calendar is still opening: %+v", it)
	}
	h.advance(600 * time.Millisecond)
	cal := h.windowID(calendarID, "Calendar")
	if h.itemOf(cal) != "w2" || h.frame(cal) != h.bodyOf(h.paneOf("w2")) {
		t.Errorf("Calendar's window should bind to w2 and dock: item %q frame %v", h.itemOf(cal), h.frame(cal))
	}
}

func TestSwitchWaitArrangesAgainAfterSettling(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchWindows("Calendar"))
		p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://apple.com", "Apple"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	cal := h.windowID(calendarID, "Calendar")
	// The app moves and resizes its own window after launch, as apps do.
	// Bench sees the events, but nobody is holding the mouse, so it isn't
	// the user dragging the window out.
	h.clk.Advance(300 * time.Millisecond)
	mustOK(t, h.p.AppMoveWindow(cal, layout.Rect{X: 5, Y: 30, W: 700, H: 500}))
	h.drain()
	if it := h.item("w2"); it.Status != StatusReady {
		t.Errorf("an app settling isn't the user: %s %q", it.Status, it.StatusText)
	}
	before := len(h.calls("SetFrame"))
	h.advance(800 * time.Millisecond)
	if after := len(h.calls("SetFrame")); after <= before {
		t.Errorf("no arrangement 750 ms after everything was ready")
	}
	if h.frame(cal) != h.bodyOf(h.paneOf("w2")) {
		t.Errorf("Calendar should be docked again: %v", h.frame(cal))
	}
	h.e.lock()
	waiting := h.e.wait != nil
	h.e.unlock()
	if waiting {
		t.Errorf("the wait should be over")
	}
	// Once it has settled, a move is the user's.
	h.advance(3 * time.Second)
	mustOK(t, h.p.UserMoveWindow(cal, layout.Rect{X: 5, Y: 30, W: 700, H: 500}))
	h.drain()
	if it := h.item("w2"); it.Status != StatusUndocked {
		t.Errorf("after settling: %s, want undocked", it.Status)
	}
}

func TestSwitchGivesUpAfterLaunchTimeout(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		// Calendar opens, but never makes a window.
		p.InstallApp(calendarID, "Calendar")
		p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://apple.com", "Apple"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(2 * time.Second)
	if it := h.item("w2"); it.Status != StatusLaunching {
		t.Errorf("before the timeout: %+v", it)
	}
	checks := len(h.calls("Windows"))
	h.advance(1500 * time.Millisecond)
	if it := h.item("w2"); it.Status != StatusMissing || it.StatusText != "Calendar has no window open for this" {
		t.Errorf("after the timeout: %s %q", it.Status, it.StatusText)
	}
	if len(h.calls("Windows")) <= checks {
		t.Errorf("the wait should keep checking for windows")
	}
	checks = len(h.calls("Windows"))
	h.advance(2 * time.Second)
	if got := len(h.calls("Windows")); got != checks {
		t.Errorf("checking after the timeout: %d calls", got-checks)
	}
}

func TestNewerSwitchCancelsWait(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(10*time.Second))
		p.InstallApp(safariID, "Safari", platform.AppLaunchDelay(10*time.Second))
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(300 * time.Millisecond)
	mustOK(t, h.e.SwitchTab(0))
	h.e.lock()
	waiting := h.e.wait
	h.e.unlock()
	if waiting != nil {
		t.Fatalf("the old wait should be cancelled")
	}
	h.p.ResetCalls()
	h.advance(time.Second)
	if n := len(h.calls("Windows")); n != 0 {
		t.Errorf("a cancelled wait kept checking: %d calls", n)
	}
	if it := h.item("w2"); it.Status != StatusMissing {
		t.Errorf("a cancelled wait's item: %+v", it)
	}
}

func TestOpenMissingAppsOff(t *testing.T) {
	cfg := strings.Replace(launchConfig, "[settings]", "[settings]\nopen_missing_apps = false", 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	if n := len(h.calls("OpenApp")); n != 0 {
		t.Errorf("OpenApp called %d times", n)
	}
	if it := h.item("w2"); it.Status != StatusMissing || it.StatusText != "Calendar isn't running" {
		t.Errorf("w2: %s %q", it.Status, it.StatusText)
	}
}

func TestOpenAppFailure(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig})
	h.p.SetErr("OpenApp", errBadSource)
	mustOK(t, h.e.SwitchTab(1))
	if it := h.item("w2"); it.Status != StatusMissing || it.StatusText != "Bench couldn't open Calendar" {
		t.Errorf("w2: %s %q", it.Status, it.StatusText)
	}
	// The user opens Calendar themselves, and it shows no window yet.
	h.p.SetErr("OpenApp", nil)
	mustOK(t, h.p.LaunchApp(calendarID))
	h.drain()
	if it := h.item("w2"); h.itemOf(h.windowID(calendarID, "Calendar")) != "w2" || it.Status != StatusReady {
		t.Errorf("after the user opened Calendar: %s %q", it.Status, it.StatusText)
	}
}

func TestOpenAppFailureClearsWhenTheAppOpens(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar") // It opens with no window.
		p.InstallApp(safariID, "Safari")
	}})
	h.p.SetErr("OpenApp", errBadSource)
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.p.LaunchApp(calendarID))
	h.drain()
	if it := h.item("w2"); it.StatusText != "Calendar has no window open for this" {
		t.Errorf("w2: %s %q, want Calendar running without a window", it.Status, it.StatusText)
	}
}

func TestStepTabWraps(t *testing.T) {
	h := newHarness(t, setup{})
	tests := []struct{ step, want int }{{-1, 2}, {1, 0}, {1, 1}, {1, 2}, {1, 0}}
	for _, tc := range tests {
		mustOK(t, h.e.StepTab(tc.step))
		if got := h.view().Current; got != tc.want {
			t.Errorf("StepTab(%d): current %d, want %d", tc.step, got, tc.want)
		}
	}
	mustErr(t, h.e.SwitchTab(7), "There's no tab 8.")
	mustErr(t, h.e.SwitchTab(-1), "There's no tab 0.")
}

func TestSwitchToCurrentTabRearranges(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.p.SetEmitOwnMoves(false)
	h.p.UserMoveWindow(zed, platform.Window{}.Frame)
	h.p.DrainEvents()
	mustOK(t, h.e.SwitchTab(1))
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("switching to the current tab should arrange it again")
	}
}

func TestCurrentTabSentToShell(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(50 * time.Millisecond)
	h.em.mu.Lock()
	tabs := slices.Clone(h.em.tabs)
	h.em.mu.Unlock()
	if len(tabs) < 2 || tabs[0] != "Comms" || tabs[len(tabs)-1] != "Research" {
		t.Errorf("CurrentTab calls: %v", tabs)
	}
}

func TestWaitedForWindowFillsPlaceholder(t *testing.T) {
	h := newHarness(t, setup{})
	// Zed's window moves to Research, and Code gets a Zed item without a
	// window.
	mustOK(t, h.e.MoveItemToTab("w3", 2))
	h.e.lock()
	zed := h.e.model.Items["w3"]
	placeholder := &Item{ID: h.e.model.NewID(itemPrefix), Kind: KindApp, BundleID: zedID, App: "Zed", Mono: "Zd", Title: "Zed"}
	code := h.e.model.Tabs[1]
	h.e.model.Add(placeholder, code, code.Pane("p11"), 0)
	h.e.unlock()
	if zed.WindowID == 0 {
		t.Fatal("w3 should have a window")
	}
	mustOK(t, h.e.SwitchTab(1))
	if !h.called("OpenApp", func(c platform.Call) bool { return c.BundleID == zedID }) {
		t.Fatal("Zed should be opened for the placeholder")
	}
	// Zed answers with a new window: it fills the item the switch waits
	// for, not the pane of Zed's most recently used window in Research.
	w, _ := h.p.OpenWindow(zedID, "second")
	h.drain()
	if got := h.itemOf(w.ID); got != placeholder.ID {
		t.Errorf("the new window went to %q (pane %s), want the placeholder %s", got, h.paneOf(got), placeholder.ID)
	}
}
