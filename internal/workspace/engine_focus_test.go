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

// followConfig is the default config with follow_focus set.
func followConfig(mode string) string {
	return strings.Replace(string(config.DefaultTOML()), `follow_focus = "switch-tab"`, `follow_focus = "`+mode+`"`, 1)
}

func TestFollowFocusSwitchTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	pkgs := h.windowID(braveID, "Go packages")
	// A link in Code opens in Brave, which brings Research's window forward.
	h.p.FocusWindow(pkgs)
	h.drain()
	v := h.view()
	if v.Current != 2 {
		t.Fatalf("current: %d, want Research", v.Current)
	}
	if active := v.Display.Panes[0].Active; active != "w18" || v.Display.Focus != "p14" {
		t.Errorf("the focused window should be shown: active %s focus %s", active, v.Display.Focus)
	}
	if h.frame(pkgs) != h.bodyOf("p14") || h.p.IsHidden(braveID) {
		t.Errorf("Brave should be docked in Research")
	}
	if !h.p.IsHidden(zedID) {
		t.Errorf("Code's apps should be hidden after following focus")
	}
}

func TestFollowFocusBringHere(t *testing.T) {
	h := newHarness(t, setup{config: followConfig("bring-here")})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusPane("p12"))
	pkgs := h.windowID(braveID, "Go packages")
	h.p.FocusWindow(pkgs)
	h.drain()
	v := h.view()
	if v.Current != 1 || v.Items["w18"].Tab != 1 || v.Items["w18"].Pane != "p12" {
		t.Errorf("bring-here: current %d, w18 in tab %d pane %s", v.Current, v.Items["w18"].Tab, v.Items["w18"].Pane)
	}
	if h.frame(pkgs) != h.bodyOf("p12") {
		t.Errorf("Brave's window should dock in the focused pane")
	}
}

func TestFollowFocusOff(t *testing.T) {
	h := newHarness(t, setup{config: followConfig("off")})
	mustOK(t, h.e.SwitchTab(1))
	h.p.FocusWindow(h.windowID(braveID, "Go packages"))
	h.drain()
	if v := h.view(); v.Current != 1 || v.Items["w18"].Tab != 2 {
		t.Errorf("off: current %d, w18 tab %d", v.Current, v.Items["w18"].Tab)
	}
}

func TestFocusInCurrentTabShowsAndFocusesPane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusPane("p12"))
	code := h.windowID(vscodeID, "index.html — bench-ui")
	h.p.FocusWindow(code)
	h.drain()
	v := h.view()
	if v.Display.Focus != "p11" || v.Display.Panes[0].Active != "w4" {
		t.Errorf("focus %s, active %s", v.Display.Focus, v.Display.Panes[0].Active)
	}
}

func TestOwnFocusIsIgnored(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Bench focuses VS Code, then Zed; the echoes of its own focus calls
	// arrive afterwards and mustn't undo the second.
	mustOK(t, h.e.ShowItem("p11", "w4"))
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.drain()
	if got := h.view().Display.Panes[0].Active; got != "w3" {
		t.Errorf("Bench's own focus event was followed: active %s", got)
	}
	// Later, the user clicks VS Code: that's followed.
	h.clk.Advance(time.Second)
	h.p.FocusWindow(h.windowID(vscodeID, "index.html — bench-ui"))
	h.drain()
	if got := h.view().Display.Panes[0].Active; got != "w4" {
		t.Errorf("the user's focus should be followed: active %s", got)
	}
}

func TestUserMovesDockedWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	// Bench's own moves are ignored.
	h.drain()
	if h.item("w3").Status != StatusReady {
		t.Fatalf("Bench's own moves undocked Zed: %+v", h.item("w3"))
	}
	h.clk.Advance(3 * time.Second)
	where := layout.Rect{X: 300, Y: 300, W: 800, H: 600}
	h.p.UserMoveWindow(zed, where)
	h.drain()
	it := h.item("w3")
	if it.Status != StatusUndocked || it.StatusText != "Moved out of its pane" {
		t.Fatalf("w3: %s %q", it.Status, it.StatusText)
	}
	if h.frame(zed) != where {
		t.Errorf("an undocked window is left where the user put it: %v", h.frame(zed))
	}
	if _, docked := h.e.win.Docked(zed); docked {
		t.Errorf("the manager should forget it")
	}
	if !h.e.win.IsParked(code) {
		t.Errorf("the pane shows Zed's status, so VS Code should be parked")
	}
	mustOK(t, h.e.Rearrange())
	if h.frame(zed) != where {
		t.Errorf("rearranging moved an undocked window")
	}
	mustOK(t, h.e.Redock("w3"))
	if h.item("w3").Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Redock: %s %v", h.item("w3").Status, h.frame(zed))
	}
	mustErr(t, h.e.Redock("w3"), "That window is already docked.")
}

func TestOtherDesktop(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.clk.Advance(3 * time.Second)
	h.p.MoveToOtherDesktop(zed)
	h.drain()
	it := h.item("w3")
	if it.Status != StatusUnavailable || it.StatusText != "On another desktop" {
		t.Fatalf("w3: %s %q", it.Status, it.StatusText)
	}
	if !slices.Contains(layout.Items(h.view().Tabs[1].Root), "w3") {
		t.Errorf("an unavailable item stays")
	}
	h.p.ReturnToDesktop(zed)
	h.drain()
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("back: %s %v", it.Status, h.frame(zed))
	}
}

func TestBenchActivationHidesOverlappingApps(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder starts shown")
	}
	h.p.ActivateApp(benchID)
	h.drain()
	if !h.p.IsHidden(finderID) {
		t.Errorf("Finder's window overlaps Bench, so Finder should be hidden")
	}
	if h.p.IsHidden(zedID) {
		t.Errorf("Zed's windows are docked here")
	}
}

func TestBenchActivationKeepsAppsOutsideBench(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetScreens(monitor, platform.Screen{ID: "2", Frame: layout.Rect{X: 2560, W: 1000, H: 800},
			Visible: layout.Rect{X: 2560, W: 1000, H: 800}})
		app, _ := p.App(finderID)
		ws, _ := p.Windows(app)
		p.UserMoveWindow(ws[0].ID, layout.Rect{X: 2600, Y: 100, W: 300, H: 300})
	}})
	h.p.ActivateApp(benchID)
	h.drain()
	if h.p.IsHidden(finderID) {
		t.Errorf("Finder's window is on the other screen")
	}
}

func TestWebEvents(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "# general"})
	h.p.EmitEvent(platform.Event{Kind: platform.WebURL, ItemID: "w1", Body: "https://app.slack.com/client/T1/C2"})
	h.drain()
	it := h.item("w1")
	if it.Title != "# general" || it.URL != "https://app.slack.com/client/T1/C2" {
		t.Errorf("w1: %+v", it)
	}
	mustOK(t, h.e.SwitchTab(1))
	h.p.EmitEvent(platform.Event{Kind: platform.WebNotification, ItemID: "w2", Title: "Inbox", Body: "New mail"})
	h.drain()
	ns := h.p.Notifications()
	if len(ns) != 1 || ns[0] != (platform.Notification{ItemID: "w2", Title: "Inbox", Body: "New mail"}) {
		t.Errorf("notifications: %+v", ns)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w2"})
	h.drain()
	v := h.view()
	if v.Current != 0 || v.Display.Panes[len(v.Display.Panes)-1].Active != "w2" {
		t.Errorf("clicking the notification should show Outlook: current %d", v.Current)
	}
	// A popped-out web view's window closing removes its item.
	mustOK(t, h.e.PopOut("w2"))
	h.p.EmitEvent(platform.Event{Kind: platform.WebViewClosed, ItemID: "w2"})
	h.drain()
	if _, ok := h.view().Items["w2"]; ok {
		t.Errorf("w2 should be gone")
	}
}

func TestPreviews(t *testing.T) {
	cfg := strings.Replace(string(config.DefaultTOML()), "live_previews = false", "live_previews = true", 1)
	h := newHarness(t, setup{config: cfg})
	anchor := layout.Rect{X: 10, Y: 50, W: 120, H: 30}
	mustOK(t, h.e.ShowPreview("w3", anchor))
	if _, shown := h.p.PreviewShown(); shown {
		t.Errorf("without Screen Recording, no preview")
	}
	h.p.SetScreenRecordingAllowed(true)
	mustOK(t, h.e.ShowPreview("w3", anchor))
	p, shown := h.p.PreviewShown()
	zed := h.windowID(zedID, "bench — switch.go")
	if !shown || p.Target.Window != zed || p.Anchor != anchor || p.Target.Title != "Zed — bench — switch.go" {
		t.Errorf("preview: %+v %v", p, shown)
	}
	mustOK(t, h.e.ShowPreview("w1", anchor))
	if p, _ := h.p.PreviewShown(); p.Target.WebViewID != "w1" {
		t.Errorf("web preview: %+v", p)
	}
	mustOK(t, h.e.HidePreview())
	if _, shown := h.p.PreviewShown(); shown {
		t.Errorf("HidePreview")
	}
	mustOK(t, h.e.ShowPreview("w5", anchor))
	if _, shown := h.p.PreviewShown(); shown {
		t.Errorf("a terminal has no preview")
	}

	off := newHarness(t, setup{})
	off.p.SetScreenRecordingAllowed(true)
	mustOK(t, off.e.ShowPreview("w3", anchor))
	if _, shown := off.p.PreviewShown(); shown {
		t.Errorf("live_previews is off")
	}
	mustOK(t, off.e.RequestScreenRecording())
	if !off.view().Permissions.ScreenRecording {
		t.Errorf("RequestScreenRecording should record the permission")
	}
}
