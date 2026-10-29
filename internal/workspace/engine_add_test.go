package workspace

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestAddOptions(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.AddApp("com.google.Chrome.app.slack", "Slack", platform.AppNotRunning(), platform.AppWebApp("Chrome"),
		platform.AppPath("/Users/sam/Applications/Chrome Apps.localized/Slack.app"))
	h.p.AddApp("com.tinyspeck.slackmacgap", "Slack", platform.AppNotRunning())
	mustOK(t, h.e.SwitchTab(1))
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	var groups []string
	for _, o := range opts {
		if len(groups) == 0 || groups[len(groups)-1] != o.Group {
			groups = append(groups, o.Group)
		}
	}
	if !slices.Equal(groups, []string{"Windows not in a tab", "Windows in other tabs", "New window", "Built in", "Apps", "Web apps"}) {
		t.Fatalf("groups: %v", groups)
	}
	// A web app is listed with its browser, apart from the site's desktop
	// app of the same name.
	var web []AddOption
	for _, o := range opts {
		if o.Group == "Web apps" {
			web = append(web, o)
		}
	}
	if len(web) != 1 || web[0].Label != "Slack" || web[0].Hint != "Web app from Chrome" || web[0].Mono != "Sl" ||
		web[0].Source != (Source{Type: "app", BundleID: "com.google.Chrome.app.slack"}) {
		t.Errorf("web apps: %+v", web)
	}
	finder := h.windowID(finderID, "Downloads")
	if o := opts[0]; o.Label != "Downloads" || o.Hint != "Finder" || o.Mono != "Fn" ||
		o.Source != (Source{Type: "floating", ID: floatingID(finder)}) {
		t.Errorf("floating: %+v", o)
	}
	var labels []string
	for _, o := range opts {
		if o.Group == "New window" || o.Group == "Built in" {
			labels = append(labels, o.Label)
		}
	}
	// The terminal that the focused pane shows is in ~/code, so a new
	// terminal there comes first.
	if !slices.Equal(labels, []string{"New Zed window", "New VS Code window", "New terminal in ~/code", "New terminal", "New terminal in…",
		"New terminal…", "Web page…", "Browse…"}) {
		t.Errorf("labels: %v", labels)
	}
	for _, o := range opts {
		switch o.Label {
		case "New Zed window":
			if o.Source != (Source{Type: "new-window", BundleID: zedID}) {
				t.Errorf("new window: %+v", o.Source)
			}
		case "New terminal":
			if o.Hint != "~" || o.Mono != ">_" || o.AsksForCommand {
				t.Errorf("terminal: %+v", o)
			}
		case "New terminal…":
			if !o.AsksForCommand || o.Mono != ">_" || o.Source != (Source{Type: "terminal"}) {
				t.Errorf("terminal form: %+v", o)
			}
		case "New terminal in ~/code":
			if o.Hint != "Where the terminal here is" || o.Mono != ">_" || o.AsksForCommand || o.ChoosesFolder ||
				o.Source != (Source{Type: "terminal", Folder: "~/code"}) {
				t.Errorf("terminal here: %+v", o)
			}
		case "New terminal in…":
			if !o.ChoosesFolder || o.AsksForCommand || o.Hint != "Choose a folder" || o.Source != (Source{Type: "terminal", Folder: "~/code"}) {
				t.Errorf("terminal in a folder: %+v", o)
			}
		case "Web page…":
			if !o.AsksForURL || o.Hint != "Opens in a Bench web view" || o.Mono != "Wb" {
				t.Errorf("web page: %+v", o)
			}
		case "Browse…":
			if !o.Browses || o.AsksForURL || o.AsksForCommand || o.Source != (Source{Type: "browse"}) ||
				o.Hint != "An app, or a program to run in a terminal" {
				t.Errorf("browse: %+v", o)
			}
		}
	}
	var apps []string
	for _, o := range opts {
		if o.Group == "Apps" {
			apps = append(apps, o.Label)
			if o.Hint != "Open, or use its open windows" || o.Source.Type != "app" {
				t.Errorf("app: %+v", o)
			}
		}
	}
	if !slices.Contains(apps, "Calendar") || !slices.Contains(apps, "Visual Studio Code") || !slices.Contains(apps, "Slack") {
		t.Errorf("apps: %v", apps)
	}
	if _, err := h.e.AddOptions("p99"); err == nil {
		t.Errorf("AddOptions for an unknown pane should fail")
	}
}

func TestAddUnmanagedWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	finder := h.windowID(finderID, "Downloads")
	mustOK(t, h.e.AddToPane("p12", Source{Type: "floating", ID: floatingID(finder)}))
	id := h.itemOf(finder)
	v := h.view()
	if id == "" || v.Items[id].Pane != "p12" || v.Display.Panes[1].Active != id {
		t.Fatalf("finder: item %q", id)
	}
	if len(v.Floating) != 0 {
		t.Errorf("it's in a tab now: %v", floatingIDs(v))
	}
	if h.frame(finder) != h.bodyOf("p12") {
		t.Errorf("Finder should dock in the pane")
	}
	mustErr(t, h.e.AddToPane("p12", Source{Type: "floating", ID: "f999"}), "That window has closed.")
	mustErr(t, h.e.AddToPane("p99", Source{Type: "terminal"}), "There's no such pane.")
	mustErr(t, h.e.AddToPane("p12", Source{Type: "pizza"}), "Bench can't add that.")
}

func TestAddNewWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "new-window", BundleID: zedID}))
	if !h.called("NewWindow", func(c platform.Call) bool { return c.BundleID == zedID }) {
		t.Fatalf("NewWindow wasn't called")
	}
	h.drain()
	w := h.windowID(zedID, platform.DefaultNewWindowTitle)
	id := h.itemOf(w)
	if id == "" || h.paneOf(id) != "p12" {
		t.Errorf("the new window should go to p12, not Zed's pane: item %q in %s", id, h.paneOf(id))
	}
}

func TestAddApp(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Finder is running with a window not in a tab: it's added.
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: finderID}))
	finder := h.windowID(finderID, "Downloads")
	if id := h.itemOf(finder); id == "" || h.paneOf(id) != "p12" {
		t.Errorf("Finder's window: item %q", id)
	}
	// Calendar isn't running: it's opened, and its first window comes here.
	mustOK(t, h.e.AddToPane("p11", Source{Type: "app", BundleID: calendarID}))
	h.drain()
	cal := h.windowID(calendarID, "Calendar")
	if id := h.itemOf(cal); id == "" || h.paneOf(id) != "p11" {
		t.Errorf("Calendar's window: item %q", id)
	}
	// Obsidian is running, but every window is in a tab: a new one opens.
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: obsidianID}))
	h.drain()
	w := h.windowID(obsidianID, platform.DefaultNewWindowTitle)
	if id := h.itemOf(w); id == "" || h.paneOf(id) != "p12" {
		t.Errorf("Obsidian's new window: item %q", id)
	}
}

func TestAddTerminalAndWebPage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "terminal"}))
	v := h.view()
	id := v.Display.Panes[1].Active
	if it := v.Items[id]; it.Kind != KindBuiltin || it.Folder != "~" || it.App != "Terminal" {
		t.Errorf("terminal: %+v", it)
	}
	if opts, ok := h.terms.started[id]; !ok || opts.Dir != "~" {
		t.Errorf("the terminal should start in ~")
	}
	mustOK(t, h.e.AddToPane("p11", Source{Type: "web", URL: "news.ycombinator.com"}))
	v = h.view()
	id = v.Display.Panes[0].Active
	it := v.Items[id]
	if it.Kind != KindWeb || it.URL != "https://news.ycombinator.com" || it.App != "Hacker News" {
		t.Errorf("web: %+v", it)
	}
	if wv := h.p.WebViews()[id]; !wv.Visible || wv.View.Profile != "news.ycombinator.com" {
		t.Errorf("web view: %+v", wv)
	}
	zed := h.windowID(zedID, "bench — switch.go")
	if !h.e.win.IsParked(zed) {
		t.Errorf("Zed should be parked while the web page shows in its pane")
	}
	mustErr(t, h.e.AddToPane("p11", Source{Type: "web", URL: "http://example.com"}),
		"That address isn't a secure web address (https).")
}

func TestPopOutAppItem(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ShowItem("p11", "w4"))
	zed := h.windowID(zedID, "bench — switch.go")
	before := h.frame(zed)
	h.p.ResetCalls()
	mustOK(t, h.e.PopOut("w3"))
	v := h.view()
	if v.Items["w3"].Tab != -1 || !slices.Contains(floatingIDs(v), "w3") {
		t.Fatalf("w3 should be floating: %+v", v.Items["w3"])
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go out of Bench. Add it back with +." {
		t.Errorf("toast: %q", got)
	}
	if _, docked := h.e.win.Docked(zed); docked {
		t.Errorf("Zed's window should be left free-floating")
	}
	_ = before
	// Popped-out windows are left alone: focusing it doesn't switch tabs.
	mustOK(t, h.e.SwitchTab(0))
	h.p.FocusWindow(zed)
	h.drain()
	if h.view().Current != 0 {
		t.Errorf("focusing a popped-out window switched tabs")
	}
	if h.p.IsHidden(zedID) {
		t.Errorf("Zed has a popped-out window, so it shouldn't be hidden")
	}
	// Adding it back docks it again.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "floating", ID: "w3"}))
	if h.paneOf("w3") != "p12" || h.frame(zed) != h.bodyOf("p12") {
		t.Errorf("w3 back: pane %s frame %v", h.paneOf("w3"), h.frame(zed))
	}
}

func TestPopOutParkedWindowIsUnparked(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	zed := h.windowID(zedID, "bench — switch.go")
	if !h.e.win.IsParked(zed) {
		t.Fatal("Zed should be parked behind the terminal")
	}
	mustOK(t, h.e.PopOut("w3"))
	if h.e.win.IsParked(zed) || !OnScreenForTest(h.frame(zed)) {
		t.Errorf("Zed's window should be back on screen: %v", h.frame(zed))
	}
}

// OnScreenForTest reports whether a frame is on the monitor.
func OnScreenForTest(r layout.Rect) bool {
	return platform.Overlaps(r, monitor.Visible) && r.X < monitor.Frame.W-1
}

func TestPopOutWebAndTerminal(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w1"))
	wv := h.p.WebViews()["w1"]
	if !wv.PoppedOut || wv.Title != "Slack" {
		t.Errorf("web view: %+v", wv)
	}
	if it := h.item("w1"); it.Tab != -1 {
		t.Errorf("w1 tab %d", it.Tab)
	}
	// p8 closed up when Slack left it.
	mustOK(t, h.e.AddToPane("p9", Source{Type: "floating", ID: "w1"}))
	if wv := h.p.WebViews()["w1"]; wv.PoppedOut || !wv.Visible {
		t.Errorf("docked again: %+v", wv)
	}
	if !h.called("DockWebView", func(c platform.Call) bool { return c.Arg == "w1" }) {
		t.Errorf("DockWebView wasn't called")
	}

	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.PopOut("w5"))
	if !slices.Equal(h.em.popped, []string{"w5"}) || h.item("w5").Tab != -1 {
		t.Errorf("terminal: popped %v, tab %d", h.em.popped, h.item("w5").Tab)
	}
	mustOK(t, h.e.AddToPane("p11", Source{Type: "floating", ID: "w5"}))
	h.drain()
	if !slices.Equal(h.em.closed, []string{"w5"}) {
		t.Errorf("CloseTerminalWindow: %v", h.em.closed)
	}
	h.em.popErr = errors.New("no window")
	if err := h.e.PopOut("w5"); err == nil || h.item("w5").Tab != 1 {
		t.Errorf("a failed pop-out should leave the item: %v", err)
	}
}

func TestPopOutPlaceholderRemovesIt(t *testing.T) {
	h := newHarness(t, setup{world: func(*platform.Fake) {}})
	mustOK(t, h.e.PopOut("w3"))
	if _, ok := h.view().Items["w3"]; ok {
		t.Errorf("a placeholder has no window to pop out, so it's removed")
	}
	if !strings.HasPrefix(h.em.lastToast(), "Removed Zed") {
		t.Errorf("toast: %q", h.em.lastToast())
	}
}

func TestWindowDestroyedRemovesItem(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	note := h.windowID(obsidianID, "Daily note")
	h.p.CloseWindow(note)
	h.drain()
	v := h.view()
	if _, ok := v.Items["w7"]; ok {
		t.Errorf("w7 should be removed")
	}
	if leaves := leafIDs(v.Tabs[2]); !slices.Equal(leaves, []string{"p14"}) {
		t.Errorf("its pane should close up: %v", leaves)
	}
	finder := h.windowID(finderID, "Downloads")
	h.p.CloseWindow(finder)
	h.drain()
	if len(h.view().Floating) != 0 {
		t.Errorf("a closed unmanaged window should leave Floating")
	}
}

func TestBraveQuitAndReopen(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItem("w17", 2, "p14", "right"))
	before := h.view()
	h.p.QuitApp(braveID)
	h.drain()
	v := h.view()
	for _, id := range []string{"w6", "w17", "w18"} {
		it := v.Items[id]
		if it.Status != StatusMissing || it.StatusText != "Brave isn't running" {
			t.Errorf("%s after quitting: %s %q", id, it.Status, it.StatusText)
		}
	}
	if !slices.Equal(layout.Items(v.Tabs[2].Root), layout.Items(before.Tabs[2].Root)) {
		t.Errorf("quitting changed the tree")
	}
	// Brave opens again, restoring its windows: they match back into place.
	h.p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("Wails v3 docs", "Pull request #42 · bench", "Go packages"))
	h.p.LaunchApp(braveID)
	h.drain()
	for title, id := range map[string]string{"Wails v3 docs": "w6", "Pull request #42 · bench": "w17", "Go packages": "w18"} {
		if got := h.itemOf(h.windowID(braveID, title)); got != id {
			t.Errorf("%q: item %q, want %s", title, got, id)
		}
	}
	pr := h.windowID(braveID, "Pull request #42 · bench")
	if h.frame(pr) != h.bodyOf(h.paneOf("w17")) {
		t.Errorf("w17 should dock in its own pane again")
	}
}

func TestBraveReopensWithNewTitles(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.QuitApp(braveID)
	h.drain()
	h.p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("New Tab", "New Tab", "New Tab"))
	h.p.LaunchApp(braveID)
	h.drain()
	app, _ := h.p.App(braveID)
	ws, _ := h.p.Windows(app)
	for i, id := range []string{"w6", "w17", "w18"} {
		if got := h.itemOf(ws[i].ID); got != id {
			t.Errorf("window %d: item %q, want %s (creation order)", i, got, id)
		}
	}
}

func TestNewWindowRules(t *testing.T) {
	cfg := strings.Replace(string(config.ExampleTOML()), `float = true`, "float = true\n\n[[rule]]\napp = \"com.apple.Safari\"\ntab = \"Research\"", 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	h.p.AddApp(safariID, "Safari")
	h.p.AddApp(benchID, "Bench", platform.AppNotInstalled())
	tests := []struct {
		name     string
		bundleID string
		title    string
		opts     []platform.WindowOption
		pane     string // "" for floating, "-" for ignored
	}{
		{"float rule", braveID, "Picture in picture", nil, ""},
		{"app-pane: most recent in another tab", braveID, "News", nil, "p14"},
		{"app-pane: current tab first", zedID, "main.go", nil, "p11"},
		{"tab rule", safariID, "Apple", nil, "p14"},
		{"dialogs never become items", zedID, "Settings", []platform.WindowOption{platform.WithSubrole(platform.SubroleDialog)}, "-"},
		{"Bench's own windows", benchID, "Bench 2", nil, "-"},
		{"unclaimed app: current focused pane", "com.example.Other", "Other", nil, "p11"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.bundleID == "com.example.Other" {
				h.p.AddApp(tc.bundleID, "Other")
			}
			w, err := h.p.OpenWindow(tc.bundleID, tc.title, tc.opts...)
			mustOK(t, err)
			h.drain()
			id := h.itemOf(w.ID)
			floating := slices.Contains(floatingIDs(h.view()), floatingID(w.ID))
			switch tc.pane {
			case "-":
				if id != "" || floating {
					t.Errorf("should be ignored: item %q floating %v", id, floating)
				}
			case "":
				if id != "" || !floating {
					t.Errorf("should float: item %q floating %v", id, floating)
				}
			default:
				if id == "" || h.paneOf(id) != tc.pane {
					t.Errorf("item %q in %q, want %s", id, h.paneOf(id), tc.pane)
				}
				if h.view().Items[id].Status != StatusReady {
					t.Errorf("status %s", h.view().Items[id].Status)
				}
			}
		})
	}
	// A new window is shown in its pane.
	w, _ := h.p.OpenWindow(zedID, "fresh")
	h.drain()
	if active := h.view().Display.Panes[0].Active; active != h.itemOf(w.ID) {
		t.Errorf("the new window should be shown: active %s", active)
	}
}

func TestNewWindowsSetting(t *testing.T) {
	for _, tc := range []struct{ setting, pane string }{{"focused-pane", "p12"}, {"float", ""}} {
		t.Run(tc.setting, func(t *testing.T) {
			cfg := strings.Replace(string(config.ExampleTOML()), `new_windows = "app-pane"`, `new_windows = "`+tc.setting+`"`, 1)
			h := newHarness(t, setup{config: cfg})
			mustOK(t, h.e.SwitchTab(1))
			mustOK(t, h.e.FocusPane("p12"))
			w, _ := h.p.OpenWindow(zedID, "main.go")
			h.drain()
			id := h.itemOf(w.ID)
			if tc.pane == "" {
				if id != "" || !slices.Contains(floatingIDs(h.view()), floatingID(w.ID)) {
					t.Errorf("should float: item %q", id)
				}
				return
			}
			if id == "" || h.paneOf(id) != tc.pane {
				t.Errorf("item %q in %s", id, h.paneOf(id))
			}
		})
	}
}

func TestNewWindowBindsPlaceholder(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) { p.InstallApp(zedID, "Zed", platform.AppLaunchWindows("hello")) }})
	mustOK(t, h.e.SwitchTab(2))
	// Zed isn't running; the user opens it from the Dock.
	h.p.LaunchApp(zedID)
	h.drain()
	w := h.windowID(zedID, "hello")
	if got := h.itemOf(w); got != "w3" {
		t.Errorf("Zed's window should bind to its placeholder w3, got %q", got)
	}
	if layout.Items(h.view().Tabs[1].Root)[0] != "w3" {
		t.Errorf("the placeholder keeps its place")
	}
}

func TestPendingPlacementExpires(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.SetErr("NewWindow", nil)
	h.e.lock()
	h.e.addPendingLocked(braveID, h.e.model.Tabs[1], h.e.model.Tabs[1].Pane("p12"), true)
	h.e.unlock()
	h.advance(16 * time.Second)
	w, _ := h.p.OpenWindow(braveID, "late")
	h.drain()
	if pane := h.paneOf(h.itemOf(w.ID)); pane == "p12" {
		t.Errorf("an expired placement was used")
	}
}

func TestRetitle(t *testing.T) {
	h := newHarness(t, setup{})
	zed := h.windowID(zedID, "bench — switch.go")
	h.p.RetitleWindow(zed, "bench — model.go")
	finder := h.windowID(finderID, "Downloads")
	h.p.RetitleWindow(finder, "Documents")
	h.drain()
	if got := h.item("w3").Title; got != "bench — model.go" {
		t.Errorf("title: %q", got)
	}
	if got := h.view().Floating[0].Title; got != "Documents" {
		t.Errorf("unmanaged title: %q", got)
	}
}

func TestWindowsGoneWithoutEvents(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Zed's window closes, but the event is lost: arranging finds out, and
	// once Zed is still running a moment later, the item is removed.
	h.p.CloseWindow(h.windowID(zedID, "bench — switch.go"))
	h.p.DrainEvents()
	mustOK(t, h.e.Rearrange())
	if active := h.view().Display.Panes[0].Active; active != "w3" {
		t.Errorf("the pane should still show w3 for a moment: %s", active)
	}
	h.advance(2 * time.Second)
	if _, ok := h.view().Items["w3"]; ok {
		t.Errorf("Zed is running, so its gone window's item should be removed")
	}
	// VS Code quits, and that event is lost too: its item waits for it.
	h.p.QuitApp(vscodeID)
	h.p.DrainEvents()
	mustOK(t, h.e.Rearrange())
	if it := h.item("w4"); it.Status != StatusMissing || it.StatusText != "VS Code isn't running" {
		t.Errorf("w4: %s %q", it.Status, it.StatusText)
	}
}

func TestPopOutUnshownWebView(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Web"
layout = "single"
  [[tab.pane]]
  items = [{ web = "https://example.com", name = "Example" }]
`})
	if _, ok := h.p.WebViews()["w2"]; ok {
		t.Fatal("w2's web view shouldn't exist yet")
	}
	mustOK(t, h.e.PopOut("w2"))
	if wv, ok := h.p.WebViews()["w2"]; !ok || !wv.PoppedOut || wv.View.URL != "https://example.com" {
		t.Errorf("web view: %+v %v", wv, ok)
	}
	h.p.SetErr("PopOutWebView", errBadSource)
	mustOK(t, h.e.AddToPane("p3", Source{Type: "floating", ID: "w2"}))
	if err := h.e.PopOut("w2"); err == nil || !strings.HasPrefix(err.Error(), "Bench couldn't pop out") {
		t.Errorf("a failed pop-out: %v", err)
	}
}

// filteredConfig has one pane for Brave windows about docs and another for
// the rest.
const filteredConfig = `
[[tab]]
name = "Docs"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "Docs", url = "https://go.dev/doc" }]
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
`

func TestNewWindowTakesItsPanesTitleFilter(t *testing.T) {
	h := newHarness(t, setup{config: filteredConfig, world: func(p *platform.Fake) {
		p.AddApp(braveID, "Brave")
		p.AddWindow(braveID, "Docs one")
		p.AddWindow(braveID, "Other")
	}})
	// Docs one, in the pane with the title filter, is the Brave window used
	// last, so a new Brave window joins that pane (new_windows = "app-pane").
	h.advance(time.Second)
	h.p.FocusWindow(h.windowID(braveID, "Docs one"))
	h.drain()
	two, err := h.p.OpenWindow(braveID, "Docs two")
	mustOK(t, err)
	h.drain()
	if got := h.paneOf(h.itemOf(two.ID)); got != h.paneOf("w1") {
		t.Fatalf("the new window should join the pane of the Brave window used last: it's in %s", got)
	}
	// Saving the tab mustn't turn the new item into an entry that claims
	// every Brave window.
	mustOK(t, h.e.SaveTab(0, TabDraft{Name: "Docs", Preset: layout.PresetSide}))
	cfg, problems, err := config.Load(filepath.Join(h.dir, config.ConfigFile))
	if err != nil || len(problems) > 0 {
		t.Fatalf("config.toml: %v %v", err, problems)
	}
	want := [][]config.Item{
		{{App: braveID, Title: "Docs", URL: "https://go.dev/doc"}},
		{{App: braveID}},
	}
	for j, pane := range cfg.Tabs[0].Panes {
		if !slices.Equal(pane.Items, want[j]) {
			t.Errorf("pane %d: %+v, want %+v", j+1, pane.Items, want[j])
		}
	}
}
