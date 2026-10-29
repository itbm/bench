package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// followConfig is the default config with follow_focus set.
func followConfig(mode string) string {
	return strings.Replace(string(config.ExampleTOML()), `follow_focus = "switch-tab"`, `follow_focus = "`+mode+`"`, 1)
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
	h.advance(time.Second)
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

func TestAppMovesDockedWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.drain()
	h.clk.Advance(3 * time.Second)
	body := h.bodyOf("p11")
	// Zed resizes its own window, as Electron apps do after being resized.
	// Nobody holds the mouse, so it isn't the user dragging it out.
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.drain()
	if it := h.item("w3"); it.Status != StatusReady {
		t.Fatalf("an app moving its own window isn't the user: %s %q", it.Status, it.StatusText)
	}
	h.advance(redockDelay)
	if h.frame(zed) != body {
		t.Errorf("Bench should put the window back in its pane: %v, want %v", h.frame(zed), body)
	}
}

func TestAppThatKeepsMovingItsWindowWins(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	h.drain()
	h.clk.Advance(3 * time.Second)
	where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
	for i := 0; i < redockLimit; i++ {
		mustOK(t, h.p.AppMoveWindow(zed, where))
		h.drain()
		h.advance(redockDelay)
		if h.frame(zed) != h.bodyOf("p11") {
			t.Fatalf("try %d: Bench should put the window back: %v", i+1, h.frame(zed))
		}
		h.clk.Advance(time.Second)
	}
	before := len(h.calls("SetFrame"))
	mustOK(t, h.p.AppMoveWindow(zed, where))
	h.drain()
	h.advance(redockDelay)
	if after := len(h.calls("SetFrame")); after != before || h.frame(zed) != where {
		t.Errorf("after %d tries, Bench should stop fighting the app: %d more SetFrame calls, frame %v", redockLimit, after-before, h.frame(zed))
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Errorf("the item isn't undocked by an app: %s", it.Status)
	}
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

// While Bench's window is hidden, Bench can still become active, when the
// user clicks another of its windows, such as Acknowledgements. Apps over
// the hidden window stay until the window is back.
func TestBenchActivationWhileHiddenKeepsApps(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.ActivateApp(benchID)
	h.drain()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder was hidden while Bench's window was hidden")
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if !h.p.IsHidden(finderID) {
		t.Errorf("Finder's window overlaps Bench, so Finder should be hidden once Bench's window is back")
	}
}

// Showing Bench's window while another app is active leaves that app's
// windows: Bench hides them only when it becomes active.
func TestBenchShownWhileInactiveKeepsApps(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ActivateApp(benchID)
	h.drain()
	h.p.ActivateApp(finderID)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if h.p.IsHidden(finderID) {
		t.Errorf("Finder is the active app, so it shouldn't be hidden")
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
	cfg := strings.Replace(string(config.ExampleTOML()), "live_previews = false", "live_previews = true", 1)
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

// The jump shortcut, pressed while typing in a docked app, brings Bench
// forward for the palette, which takes the keyboard from the app. Closing
// the palette without choosing anything gives the keyboard back to the
// app's window; choosing a row that focuses something doesn't.
func TestClosingTheJumpPaletteGivesTheKeyboardBack(t *testing.T) {
	// press presses the jump shortcut. The shell brings Bench forward for the
	// palette, which raises it.
	press := func(t *testing.T, h *harness) {
		h.e.Shortcut(shortcuts.ActionJump, "")
		mustOK(t, h.p.ActivateApp(benchID))
		h.drain()
		mustOK(t, h.e.SetOverlay(true))
		h.p.ResetCalls()
	}
	open := func(t *testing.T) (*harness, platform.WindowID) {
		h := newHarness(t, setup{})
		mustOK(t, h.e.SwitchTab(1))
		h.settle()
		if h.p.ActiveApp() != zedID {
			t.Fatalf("Zed should have the keyboard, not %q", h.p.ActiveApp())
		}
		press(t, h)
		return h, h.windowID(zedID, "bench — switch.go")
	}
	refocused := func(h *harness, zed platform.WindowID) bool {
		return slices.Contains(h.focusCalls(), zed)
	}

	t.Run("escape", func(t *testing.T) {
		h, zed := open(t)
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if got := h.focusCalls(); !slices.Equal(got, []platform.WindowID{zed}) || h.p.ActiveApp() != zedID {
			t.Errorf("focus calls %v, active %q; want Zed's window to get the keyboard back", got, h.p.ActiveApp())
		}
	})

	t.Run("the shortcut again", func(t *testing.T) {
		h, zed := open(t)
		h.e.Shortcut(shortcuts.ActionJump, "")
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if !refocused(h, zed) {
			t.Errorf("focus calls %v; want Zed's window", h.focusCalls())
		}
	})

	t.Run("a tab chosen", func(t *testing.T) {
		// The palette lowers Bench as it closes, and the switch may reach Go
		// after that.
		h, zed := open(t)
		mustOK(t, h.e.SetOverlay(false))
		h.advance(50 * time.Millisecond)
		mustOK(t, h.e.SwitchTab(2))
		h.advance(time.Second)
		if refocused(h, zed) {
			t.Errorf("focus calls %v: Zed's window took the keyboard from Research", h.focusCalls())
		}
	})

	t.Run("a dialog opened from the palette", func(t *testing.T) {
		// Settings… closes the palette and opens Settings, which raises Bench
		// again: Zed gets the keyboard back when Settings closes.
		h, zed := open(t)
		mustOK(t, h.e.SetOverlay(false))
		mustOK(t, h.e.SetOverlay(true))
		h.advance(time.Second)
		if refocused(h, zed) {
			t.Fatalf("focus calls %v while Settings is open", h.focusCalls())
		}
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if !refocused(h, zed) {
			t.Errorf("focus calls %v; want Zed's window once Settings closes", h.focusCalls())
		}
	})

	t.Run("another app", func(t *testing.T) {
		h, zed := open(t)
		mustOK(t, h.p.ActivateApp(finderID))
		h.drain()
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if refocused(h, zed) {
			t.Errorf("focus calls %v: Zed's window took the keyboard from Finder", h.focusCalls())
		}
	})

	// Another of Zed's windows has the keyboard, not the one in the pane:
	// Escape mustn't send what the user types next to a different document.
	for _, tc := range []struct {
		name string
		open func(t *testing.T, h *harness) platform.WindowID
	}{
		{"a dialog", func(t *testing.T, h *harness) platform.WindowID {
			w, err := h.p.OpenWindow(zedID, "Settings", platform.WithSubrole(platform.SubroleDialog))
			mustOK(t, err)
			return w.ID
		}},
		{"a window not in a tab", func(t *testing.T, h *harness) platform.WindowID {
			w, err := h.p.OpenWindow(zedID, "main.go")
			mustOK(t, err)
			h.drain()
			mustOK(t, h.e.PopOut(h.itemOf(w.ID)))
			mustOK(t, h.e.ShowItem("p11", h.itemOf(h.windowID(zedID, "bench — switch.go"))))
			return w.ID
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			mustOK(t, h.e.SwitchTab(1))
			h.settle()
			zed := h.windowID(zedID, "bench — switch.go")
			other := tc.open(t, h)
			mustOK(t, h.p.FocusWindow(other))
			h.drain()
			d := h.view().Display
			if i := slices.IndexFunc(d.Panes, func(p PaneView) bool { return p.ID == d.Focus }); i < 0 || d.Panes[i].Active != h.itemOf(zed) || h.p.ActiveApp() != zedID {
				t.Fatalf("the focused pane should still show Zed's first window, with Zed frontmost: %+v", d)
			}
			press(t, h)
			mustOK(t, h.e.SetOverlay(false))
			h.advance(time.Second)
			if refocused(h, zed) {
				t.Errorf("focus calls %v: Zed's window in the pane took the keyboard", h.focusCalls())
			}
		})
	}

	t.Run("a pane clicked", func(t *testing.T) {
		h, zed := open(t)
		mustOK(t, h.e.SetOverlay(false))
		mustOK(t, h.e.FocusPane("p12"))
		h.advance(time.Second)
		if refocused(h, zed) {
			t.Errorf("focus calls %v: Zed's window took the keyboard from the other pane", h.focusCalls())
		}
	})

	t.Run("palette opened in Bench", func(t *testing.T) {
		// Opened with Jump to…, not the shortcut: Bench had the keyboard.
		h, zed := open(t)
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		h.p.ResetCalls()
		mustOK(t, h.e.SetOverlay(true))
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if refocused(h, zed) {
			t.Errorf("focus calls %v", h.focusCalls())
		}
	})

	t.Run("a web page", func(t *testing.T) {
		// Comms shows web pages: their views get the keyboard back by
		// themselves (decision 15).
		h := newHarness(t, setup{})
		h.e.Shortcut(shortcuts.ActionJump, "")
		mustOK(t, h.p.ActivateApp(benchID))
		h.drain()
		mustOK(t, h.e.SetOverlay(true))
		h.p.ResetCalls()
		mustOK(t, h.e.SetOverlay(false))
		h.advance(time.Second)
		if calls := h.calls("Focus", "FocusWebView", "FocusUI"); len(calls) != 0 {
			t.Errorf("calls %v", calls)
		}
	})
}
