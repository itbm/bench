//go:build server

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/workspace"
)

// servicePrefix is how the frontend names Service's methods.
const servicePrefix = "github.com/itbm/bench/internal/ui.Service."

// TestRunServesThePreview runs the preview with a fake engine, as the
// browser does: it loads the UI and the Wails runtime, and calls bound
// methods. Wails' app is a process-wide singleton, so this is the only test
// that runs it.
func TestRunServesThePreview(t *testing.T) {
	if !wailsRuns.CompareAndSwap(false, true) {
		t.Skip("Wails' app can run only once in a process (go test -count above 1)")
	}
	f := newFakeEngine()
	f.view = workspace.View{Current: 1, Mode: "auto", Demo: &workspace.DemoView{Screen: "fit"}}
	engineOpts := make(chan workspace.Options, 1)
	saved := newEngine
	newEngine = func(opts workspace.Options) (engine, error) {
		engineOpts <- opts
		return f, nil
	}
	t.Cleanup(func() { newEngine = saved })
	logs := &syncBuffer{}
	savedLog := logOutput
	logOutput = logs
	t.Cleanup(func() { logOutput = savedLog })

	hangupIgnored := signal.Ignored(syscall.SIGHUP)
	port := freePort(t)
	// Wails would serve on these instead of Options.Port and localhost.
	t.Setenv("WAILS_SERVER_PORT", strconv.Itoa(freePort(t)))
	t.Setenv("WAILS_SERVER_HOST", "0.0.0.0")
	var recovered, locked, unlocked atomic.Int32
	logFile := filepath.Join(t.TempDir(), "Logs", "Bench", "bench.log")
	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Platform: platform.NewDemo(clock.Real()),
			NewTerms: func(cb term.Callbacks) term.Host { return term.NewEcho(cb) },
			Dir:      t.TempDir(),
			Demo:     true,
			Port:     port,
			Version:  "1.2.3",
			LogFile:  logFile,
			Recover: func() {
				if len(engineOpts) > 0 {
					t.Error("Recover ran after the engine started")
				}
				if locked.Load() == 0 {
					t.Error("Recover ran before Run took the support folder's lock")
				}
				recovered.Add(1)
			},
			Lock: func() (func(), error) {
				locked.Add(1)
				return func() { unlocked.Add(1) }, nil
			},
		})
	}()
	base := fmt.Sprintf("http://localhost:%d", port)
	waitForServer(t, base+"/", done)
	got := <-engineOpts
	if n := recovered.Load(); n != 1 {
		t.Errorf("Recover ran %d times, want once", n)
	}

	t.Run("index", func(t *testing.T) {
		body, ctype := get(t, base+"/")
		if !strings.Contains(body, "<title>Bench</title>") || !strings.HasPrefix(ctype, "text/html") {
			t.Errorf("GET / = %s %.80q, want the frontend's index.html", ctype, body)
		}
	})
	t.Run("acknowledgements", func(t *testing.T) {
		body, ctype := get(t, base+acknowledgementsPage)
		if !strings.Contains(body, "<title>Acknowledgements</title>") || !strings.HasPrefix(ctype, "text/html") {
			t.Errorf("GET %s = %s %.80q, want the Acknowledgements page", acknowledgementsPage, ctype, body)
		}
	})
	t.Run("terminal window", func(t *testing.T) {
		body, _ := get(t, base+terminalURL("w4"))
		if !strings.Contains(body, "<title>Bench</title>") {
			t.Errorf("GET %s didn't serve index.html", terminalURL("w4"))
		}
	})
	t.Run("modules", func(t *testing.T) {
		for _, path := range []string{"/js/app.js", "/js/bridge.js", "/vendor/xterm/xterm.mjs", "/styles.css"} {
			if body, _ := get(t, base+path); body == "" {
				t.Errorf("GET %s is empty", path)
			}
		}
	})
	t.Run("runtime", func(t *testing.T) {
		body, ctype := get(t, base+"/wails/runtime.js")
		if len(body) < 1000 || !strings.Contains(ctype, "javascript") {
			t.Errorf("GET /wails/runtime.js = %s, %d bytes; want the Wails runtime", ctype, len(body))
		}
	})
	t.Run("ready", func(t *testing.T) {
		status, body := callBound(t, base, "Ready")
		var view workspace.View
		if err := json.Unmarshal(body, &view); status != http.StatusOK || err != nil {
			t.Fatalf("Ready = %d %s (%v), want the view", status, body, err)
		}
		if view.Current != 1 || view.Demo == nil || view.Demo.Screen != "fit" {
			t.Errorf("Ready = %+v, want the engine's view", view)
		}
	})
	t.Run("action", func(t *testing.T) {
		if status, body := callBound(t, base, "MoveItem", "w3", 1, "p2", "right"); status != http.StatusOK {
			t.Fatalf("MoveItem = %d %s, want success", status, body)
		}
		want := call{"MoveItem", []any{"w3", 1, "p2", "right"}}
		if !slices.ContainsFunc(f.Calls(), func(c call) bool { return c.String() == want.String() }) {
			t.Errorf("engine calls = %v, want %v", f.Calls(), want)
		}
	})
	t.Run("refused", func(t *testing.T) {
		f.mu.Lock()
		f.err = errors.New("A pane can't be split by its only window.")
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.err = nil
			f.mu.Unlock()
		}()
		status, body := callBound(t, base, "MoveDirection", "right")
		var cerr struct{ Message string }
		_ = json.Unmarshal(body, &cerr)
		if status == http.StatusOK || cerr.Message != "A pane can't be split by its only window." {
			t.Errorf("refused MoveDirection = %d %s, want the engine's message", status, body)
		}
		if strings.Contains(logs.String(), "only window") {
			t.Errorf("a refused action was logged as an error:\n%s", logs.String())
		}
	})
	t.Run("other sites", func(t *testing.T) {
		// What an <img> on any web page can send: Wails' transport takes
		// the call from the query.
		binding, _ := json.Marshal(map[string]any{"call-id": "x1", "methodName": servicePrefix + "DeleteTab", "args": []any{1}})
		query := url.Values{"object": {"0"}, "method": {"0"}, "args": {string(binding)}}
		req, _ := http.NewRequest(http.MethodGet, base+"/wails/runtime?"+query.Encode(), nil)
		req.Header.Set("Referer", "https://evil.example/")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		if status := do(t, req); status != http.StatusForbidden {
			t.Errorf("a GET from another site = %d, want 403", status)
		}
		// A page that has rebound its own host name to 127.0.0.1.
		body, _ := json.Marshal(map[string]any{"object": 0, "method": 0, "args": json.RawMessage(binding)})
		req, _ = http.NewRequest(http.MethodPost, base+"/wails/runtime", bytes.NewReader(body))
		req.Host = fmt.Sprintf("evil.example:%d", port)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://"+req.Host)
		if status := do(t, req); status != http.StatusForbidden {
			t.Errorf("a call through DNS rebinding = %d, want 403", status)
		}
		if slices.ContainsFunc(f.Calls(), func(c call) bool { return c.Method == "DeleteTab" }) {
			t.Errorf("another site called DeleteTab: %v", f.Calls())
		}
	})
	t.Run("panic", func(t *testing.T) {
		f.mu.Lock()
		f.panicOn = "Rearrange"
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.panicOn = ""
			f.mu.Unlock()
		}()
		if status, body := callBound(t, base, "Rearrange"); status == http.StatusOK {
			t.Errorf("Rearrange that panicked = %d %s, want an error", status, body)
		}
		if !strings.Contains(logs.String(), "the fake engine panicked in Rearrange") {
			t.Errorf("a panic in a bound method wasn't logged; the log is:\n%s", logs.String())
		}
		// The log file has it too, after the line that starts the launch.
		kept, err := os.ReadFile(logFile)
		if err != nil || !strings.Contains(string(kept), `msg="Bench started" version=1.2.3`) ||
			!strings.Contains(string(kept), "the fake engine panicked in Rearrange") {
			t.Errorf("the log file has %q (%v); want the launch, then the panic", kept, err)
		}
	})
	t.Run("events", func(t *testing.T) { testEvents(t, port, got.Emitter) })
	t.Run("menus", testMenus)

	// Closing the terminal that Bench runs in quits it cleanly, unless the
	// tests run with SIGHUP ignored (under nohup), as Bench then would too.
	if hangupIgnored {
		t.Log("SIGHUP is ignored, so quitting without it")
		application.Get().Quit()
	} else {
		self, _ := os.FindProcess(os.Getpid())
		if err := self.Signal(syscall.SIGHUP); err != nil {
			t.Fatalf("sending SIGHUP: %v", err)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() = %v, want nil after quitting", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run didn't return after SIGHUP")
	}

	if l, u := locked.Load(), unlocked.Load(); l != 1 || u != 1 {
		t.Errorf("took the support folder's lock %d times and let it go %d times, want once each", l, u)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.started || f.closed != 1 {
		t.Errorf("engine started %v, closed %d times; want started and closed once", f.started, f.closed)
	}
	if !got.Demo || got.Emitter == nil || got.LoginItem != nil || got.Platform == nil {
		t.Errorf("engine options = %+v; want the demo, an emitter, a platform, and no login item", got)
	}
	// The engine's warnings always go to the log, without BENCH_DEBUG.
	if got.Log == nil || !got.Log.Enabled(context.Background(), slog.LevelWarn) || got.Log.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("the engine's log = %v; want warnings, without debug messages", got.Log)
	}
}

// wailsRuns records that a test has run Wails' app, which can run only once
// in a process.
var wailsRuns atomic.Bool

// testEvents checks that what the engine sends through its Emitter reaches
// a browser as the events the frontend listens for, with their payloads.
func testEvents(t *testing.T, port int, em workspace.Emitter) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://localhost:%d/wails/events", port), nil)
	if err != nil {
		t.Fatalf("connecting to the preview's events: %v", err)
	}
	defer conn.CloseNow()
	events := make(chan wsEvent, 16)
	go func() {
		for {
			var ev wsEvent
			if err := wsjson.Read(ctx, conn, &ev); err != nil {
				close(events)
				return
			}
			events <- ev
		}
	}()

	// The server registers the connection a moment after accepting it, so
	// keep sending the first event until it arrives.
	waitForEvent(t, events, eventToast, func() { em.Toast("Hello.") })

	tests := []struct {
		name string
		emit func()
		data string // The payload as JSON.
	}{
		{eventState, func() { em.State(workspace.View{Current: 2, Mode: "wide"}) }, ""},
		{eventToast, func() { em.Toast("Moved to Code.") }, `"Moved to Code."`},
		{eventCommand, func() { em.Command(workspace.Command{Name: "toggle-palette"}) }, `{"name":"toggle-palette"}`},
		{eventTermData, func() { em.TermData(workspace.TermData{ID: "w5", Data: "aGk=", Offset: 7}) }, `{"id":"w5","data":"aGk=","offset":7}`},
	}
	for _, tt := range tests {
		tt.emit()
		ev := nextEvent(t, events)
		if ev.Name != tt.name {
			t.Errorf("event %q, want %q", ev.Name, tt.name)
			continue
		}
		if tt.name == eventState {
			var v workspace.View
			if err := json.Unmarshal(ev.Data, &v); err != nil || v.Current != 2 || v.Mode != "wide" {
				t.Errorf("bench:state payload %s (%v), want the view", ev.Data, err)
			}
			continue
		}
		if string(ev.Data) != tt.data {
			t.Errorf("%s payload %s, want %s", tt.name, ev.Data, tt.data)
		}
	}
}

// wsEvent is an event as the preview sends it to a browser.
type wsEvent struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

// waitForEvent calls send until an event called name arrives.
func waitForEvent(t *testing.T, events <-chan wsEvent, name string, send func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	send()
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("the events connection closed")
			}
			if ev.Name == name {
				// Let any repeats arrive, then drop them.
				time.Sleep(200 * time.Millisecond)
				for len(events) > 0 {
					<-events
				}
				return
			}
		case <-tick.C:
			send()
		case <-deadline:
			t.Fatalf("no %s event arrived", name)
		}
	}
}

// nextEvent returns the next event.
func nextEvent(t *testing.T, events <-chan wsEvent) wsEvent {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("the events connection closed")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no event arrived")
	}
	return wsEvent{}
}

// testMenus checks the app menu, which needs Wails' app to exist.
func testMenus(t *testing.T) {
	f := newFakeEngine()
	var commands, commandItems, links []string
	var toasts []string
	var fills, reports int
	a := newAppMenu(menuActions{
		acknowledgements: func() {}, settings: func() {},
		command: func(c workspace.Command) {
			commands = append(commands, c.Name)
			commandItems = append(commandItems, c.ItemID)
		},
		act: func(do func(e engine) error) {
			if err := do(f); err != nil {
				toasts = append(toasts, err.Error())
			}
		},
		fillScreen:    func() { fills++ },
		openLink:      func(address string) { links = append(links, address) },
		reportProblem: func() { reports++ },
	})
	menu := a.root
	var labels []string
	walkMenu(menu, func(item *application.MenuItem) {
		labels = append(labels, item.Label())
		accel := strings.ToLower(item.GetAccelerator())
		for _, key := range []string{"w", "p", "n", "t", "f", "l", ","} {
			if strings.HasSuffix(accel, "+"+key) && strings.Contains(accel, "cmd") && !strings.Contains(accel, "shift") {
				t.Errorf("menu item %q has %s, which Web items rely on", item.Label(), item.GetAccelerator())
			}
		}
	})
	for _, want := range []string{"Bench", "About Bench", "Check for Updates…", "Acknowledgements", "Settings…", "Reload config.toml", "Hide Bench",
		"Hide Others", "Show All", "Quit Bench", "Edit", "Undo", "Redo", "Cut", "Copy", "Paste", "Select All",
		"View", "Show All Tabs", "Show Drop-down", "Layout", "Auto", "Wide", "Compact", "Fill the Screen", "Rearrange", "Tab", "Pane", "Window", "Minimise", "Help"} {
		if !slices.Contains(labels, want) {
			t.Errorf("the app menu has no %q; it has %q", want, labels)
		}
	}
	if got, want := menuLabels(menu), []string{"Bench", "Edit", "View", "Tab", "Pane", "Window", "Help"}; !slices.Equal(got, want) {
		t.Errorf("the app menu = %q, want %q", got, want)
	}
	sub := func(label string) *application.Menu {
		t.Helper()
		item := menu.FindByLabel(label)
		if item == nil || item.GetSubmenu() == nil {
			t.Fatalf("the app menu has no %s menu", label)
		}
		return item.GetSubmenu()
	}
	want := []string{"About Bench", "Check for Updates…", "Acknowledgements", "-", "Settings…", "Reload config.toml", "-", "Hide Bench",
		"Hide Others", "Show All", "-", "Quit Bench"}
	if got := menuLabels(sub("Bench")); !slices.Equal(got, want) {
		t.Errorf("the Bench menu = %q, want %q", got, want)
	}

	// Before the app runs, the menu has no tabs, and nothing in it changes
	// until it starts, as Wails reads it to make the native menu.
	if got := len(a.items.tabs); got != 0 {
		t.Errorf("before the engine says, the Tab menu lists %d tabs", got)
	}
	bar := workspace.MenuBar{
		Mode:    "compact",
		Current: 1,
		LastTab: 2,
		// Notes has no send key, as when add_front_window has ⌃⌥⇧3.
		Tabs: []workspace.MenuBarTab{{Name: "Comms", Keys: "⌃⌥1", SendKeys: "⌃⌥⇧1", Unread: workspace.UnreadView{Count: 15}},
			{Name: "Code", Keys: "⌃⌥2", SendKeys: "⌃⌥⇧2"}, {Name: "Notes"}},
		Keys: workspace.KeysView{Tabs: "⌃⌥", NextTab: "⌃⌥]", PreviousTab: "⌃⌥[", LastTab: "⌃⌥`", Jump: "⌃⌥Space", ShowAllTabs: "⌃⌥O",
			Rearrange: "⌃⌥R", MoveWindow: "⌃⌥⇧", SendToTab: "⌃⌥⇧", ZoomPane: "⌃⌥Z", AddFrontWindow: "⌃⌥A",
			GrowPane: "⌃⌥=", ShrinkPane: "⌃⌥-", EqualPanes: "⌃⌥⇧=", NextLayout: "⌃⌥L", Undo: "⌃⌥U", Dropdown: "⌃⌥D"},
		Pane:       workspace.MenuBarPane{ID: "p2", Item: "w5", CanSplit: true, CanZoom: true},
		Arrange:    workspace.ArrangeView{Grow: true, Equal: true, NextLayout: "Three columns"},
		AddFront:   true,
		Undo:       "Close Tab",
		ClosedItem: "w9",
	}
	a.set(bar)
	if got := len(a.items.tabs); got != 0 {
		t.Errorf("before the app runs, the Tab menu lists %d tabs", got)
	}
	a.start()

	// View ▸ Layout's items are radio items, ticked for the layout mode.
	want = []string{"Show All Tabs (⌃⌥O)", "Show Drop-down (⌃⌥D)", "-", "Layout", "Rearrange (⌃⌥R)", "-", "Fill the Screen"}
	if got := menuLabels(sub("View")); !slices.Equal(got, want) {
		t.Errorf("the View menu = %q, want %q", got, want)
	}
	layouts := a.items.layouts
	for i, mode := range []string{"auto", "wide", "compact"} {
		item := sub("Layout").ItemAt(i)
		if item != layouts[mode] || !item.IsRadio() || item.GetAccelerator() != "" || item.Checked() != (mode == "compact") {
			t.Errorf("%s: %q, radio %v, shortcut %q, ticked %v; want a radio item with no shortcut, ticked for compact",
				mode, item.Label(), item.IsRadio(), item.GetAccelerator(), item.Checked())
		}
	}

	// The Tab menu, with Undo for the latest change, and the tabs and
	// their shortcuts, the current one ticked. No item has a key
	// equivalent: the shortcuts are in the labels.
	want = []string{"Undo Close Tab (⌃⌥U)", "-", "New Tab…", "New Tab from Open Windows…", "-", "Tab Properties…", "Rename…", "-",
		"Next Layout (⌃⌥L)", "Previous Layout", "-", "Move Tab Left", "Move Tab Right", "-",
		"Next Tab (⌃⌥])", "Previous Tab (⌃⌥[)", "Last Used Tab (⌃⌥`)", "-", "Close Tab…", "Delete Tab (Keep Windows)",
		"Reopen Closed Tab", "Reopen Closed Page", "-", "Comms — 15 Unread (⌃⌥1)", "Code (⌃⌥2)", "Notes"}
	if got := menuLabels(sub("Tab")); !slices.Equal(got, want) {
		t.Errorf("the Tab menu = %q, want %q", got, want)
	}
	walkMenu(menu, func(item *application.MenuItem) {
		if item.GetAccelerator() != "" && strings.Contains(item.Label(), "(") {
			t.Errorf("%q has the key equivalent %q as well as a shortcut in its label", item.Label(), item.GetAccelerator())
		}
	})
	for i, item := range a.items.tabs {
		if !item.IsRadio() || item.Checked() != (i == 1) {
			t.Errorf("tab %d: radio %v, ticked %v; want a radio item, ticked for Code", i, item.IsRadio(), item.Checked())
		}
	}
	if !a.items.moveLeft.Enabled() || !a.items.moveRight.Enabled() || a.items.reopen.Enabled() || !a.items.deleteTab.Enabled() {
		t.Error("on the middle of three tabs, with none closed, Move Tab Left and Right and Delete Tab should be on, and Reopen off")
	}
	if !a.items.lastTab.Enabled() {
		t.Error("with a tab used before this one, Last Used Tab should be on")
	}
	// The layouts follow what the engine says: here, there's a next one,
	// but no previous one.
	if !a.items.nextLayout.Enabled() || a.items.previousLayout.Enabled() {
		t.Error("with a next layout and no previous one, Next Layout should be on, and Previous Layout off")
	}
	if !a.items.undo.Enabled() || !a.items.reopenItem.Enabled() {
		t.Error("with a change to undo and a closed page, Undo and Reopen Closed Page should be on")
	}

	// The Pane menu, for the focused pane.
	want = []string{"Jump to… (⌃⌥Space)", "Add to Pane…", "Add Front Window (⌃⌥A)", "-", "Split Right (⌃⌥⇧→)", "Split Down (⌃⌥⇧↓)", "Zoom Pane (⌃⌥Z)", "-",
		"Grow Pane (⌃⌥=)", "Shrink Pane (⌃⌥-)", "Make Panes Equal (⌃⌥⇧=)", "-",
		"Move to Tab", "Pop Out of Bench", "Dock Again", "-", "Page", "-", "Close Window", "Close Pane"}
	if got := menuLabels(sub("Pane")); !slices.Equal(got, want) {
		t.Errorf("the Pane menu = %q, want %q", got, want)
	}
	// Grow Pane, Shrink Pane, and Make Panes Equal follow what the engine
	// says: here, the pane can grow, but not shrink.
	if !a.items.grow.Enabled() || a.items.shrink.Enabled() || !a.items.equal.Enabled() {
		t.Error("for a pane that can grow but not shrink, Grow Pane and Make Panes Equal should be on, and Shrink Pane off")
	}
	// Pane ▸ Page, which is off for a terminal, as for any window that
	// isn't a web page.
	want = []string{"Reload Page", "Back", "Forward", "-", "Zoom In", "Zoom Out", "Actual Size", "-", "Find in Page…", "Copy Page Link", "-",
		"Mute Microphone", "Turn Off Camera", "Stop Sound and Video"}
	if got := menuLabels(sub("Page")); !slices.Equal(got, want) {
		t.Errorf("Pane ▸ Page = %q, want %q", got, want)
	}
	walkMenu(sub("Page"), func(item *application.MenuItem) {
		if item.Enabled() || item.GetAccelerator() != "" {
			t.Errorf("for a terminal, %q: on %v, key equivalent %q; want it off, with none", item.Label(), item.Enabled(), item.GetAccelerator())
		}
	})
	if a.items.page.Enabled() {
		t.Error("for a terminal, Pane ▸ Page should be off")
	}
	// Each tab has its send_to_tab_modifiers shortcut, which moves the
	// window there too, as the engine says: none for a key that another
	// shortcut has.
	want = []string{"Comms (⌃⌥⇧1)", "Code (⌃⌥⇧2)", "Notes", "-", "New Tab"}
	if got := menuLabels(sub("Move to Tab")); !slices.Equal(got, want) {
		t.Errorf("Pane ▸ Move to Tab = %q, want %q", got, want)
	}
	if a.items.moveToTabs[1].Enabled() || !a.items.moveToTabs[0].Enabled() {
		t.Error("Move to Tab should be off for the current tab only")
	}
	if a.items.dockAgain.Enabled() || a.items.closePane.Enabled() || !a.items.closeWindow.Enabled() {
		t.Error("for a docked window, Dock Again and Close Pane should be off, and Close Window on")
	}
	if !a.items.addFront.Enabled() || a.items.backToBench.Enabled() {
		t.Error("with a window in front out of the tabs, and no popped-out window with the keyboard, Add Front Window should be on, and Back to Bench off")
	}

	// The Window menu.
	want = []string{"Minimise", "-", "Back to Bench"}
	if got := menuLabels(sub("Window")); !slices.Equal(got, want) {
		t.Errorf("the Window menu = %q, want %q", got, want)
	}

	// The Help menu, which macOS finds by its title.
	want = []string{"Bench Help", "Troubleshooting", "Keyboard Shortcuts", "-", "Show Welcome Guide", "Recent Messages", "-",
		"Show Bench's Files in Finder", "Report a Problem…"}
	if got := menuLabels(sub("Help")); !slices.Equal(got, want) {
		t.Errorf("the Help menu = %q, want %q", got, want)
	}

	// What the items do: the engine's methods, for the current tab and
	// the focused pane's window, or the frontend's commands.
	click := func(item *application.MenuItem) {
		t.Helper()
		run := a.clicks[item]
		if run == nil {
			t.Fatalf("%q does nothing", item.Label())
		}
		run()
	}
	for _, label := range []string{"Undo Close Tab (⌃⌥U)", "New Tab…", "New Tab from Open Windows…", "Tab Properties…", "Rename…",
		"Next Layout (⌃⌥L)", "Previous Layout", "Close Tab…", "Delete Tab (Keep Windows)", "Reopen Closed Page",
		"Move Tab Left", "Next Tab (⌃⌥])", "Previous Tab (⌃⌥[)", "Last Used Tab (⌃⌥`)", "Comms — 15 Unread (⌃⌥1)"} {
		click(sub("Tab").FindByLabel(label))
	}
	for _, label := range []string{"Jump to… (⌃⌥Space)", "Add to Pane…", "Add Front Window (⌃⌥A)", "Split Right (⌃⌥⇧→)", "Split Down (⌃⌥⇧↓)",
		"Zoom Pane (⌃⌥Z)", "Grow Pane (⌃⌥=)", "Shrink Pane (⌃⌥-)", "Make Panes Equal (⌃⌥⇧=)", "Comms (⌃⌥⇧1)", "New Tab", "Pop Out of Bench", "Dock Again",
		"Close Window", "Close Pane"} {
		click(sub("Pane").FindByLabel(label))
	}
	click(sub("Window").FindByLabel("Back to Bench"))
	for _, label := range []string{"Bench Help", "Troubleshooting", "Keyboard Shortcuts", "Show Welcome Guide",
		"Recent Messages", "Show Bench's Files in Finder", "Report a Problem…"} {
		click(sub("Help").FindByLabel(label))
	}
	click(sub("View").FindByLabel("Show All Tabs (⌃⌥O)"))
	click(sub("View").FindByLabel("Show Drop-down (⌃⌥D)"))
	click(sub("View").FindByLabel("Fill the Screen"))
	click(sub("View").FindByLabel("Rearrange (⌃⌥R)"))
	click(layouts["wide"])
	click(sub("Bench").FindByLabel("Reload config.toml"))
	click(sub("Bench").FindByLabel("Check for Updates…"))
	wantCalls := []string{"UndoLatest[]", "StepLayout[1]", "StepLayout[-1]", "DeleteTab[1]", "ReopenItem[w9]", "MoveCurrentTab[-1]",
		"StepTab[1]", "StepTab[-1]", "LastTab[]", "SwitchTab[0]",
		"AddFrontWindow[]", "MoveItem[w5 1 p2 right]", "MoveItem[w5 1 p2 bottom]", "ToggleZoom[]", "GrowPane[]", "ShrinkPane[]", "EqualPanes[]",
		"MoveItemToTab[w5 0]", "MoveItemToNewTab[w5 false]",
		"PopOut[w5]", "Redock[w5]", "ClosePane[p2]", "BackToBench[]", "ShowConfigInFinder[]", "ToggleDropdown[]", "Rearrange[]", "SetMode[wide]", "ReloadConfig[]", "CheckForUpdates[]"}
	var gotCalls []string
	for _, c := range f.Calls() {
		gotCalls = append(gotCalls, c.String())
	}
	if !slices.Equal(gotCalls, wantCalls) {
		t.Errorf("the items called %q, want %q", gotCalls, wantCalls)
	}
	wantCommands := []string{"new-tab", "new-tab-from-windows", "edit-tab", "rename-tab", "close-tab", "open-palette", "add-to-pane", "close-window",
		"keyboard-shortcuts", "first-run", "recent-messages", "show-all-tabs"}
	if !slices.Equal(commands, wantCommands) {
		t.Errorf("the items sent %q, want %q", commands, wantCommands)
	}
	for _, name := range wantCommands {
		if !windowCommands[name] {
			t.Errorf("%s doesn't bring Bench forward", name)
		}
	}
	if !slices.Equal(links, []string{helpURL, troubleshootingURL}) || reports != 1 || fills != 1 {
		t.Errorf("Help opened %q and reported %d problems, and Fill the Screen filled %d times; "+
			"want the guide, then troubleshooting, one report, and one fill", links, reports, fills)
	}

	// Pane ▸ Page follows what the engine says about the focused pane's
	// web page: in a call, with the microphone muted, zoomed in all the
	// way, and able to go back.
	page := bar
	page.Pane = workspace.MenuBarPane{ID: "p1", Item: "w1", CanSplit: true, Page: true, Link: true, CanGoBack: true,
		Zoom: 300, CanZoomOut: true, Camera: "active", Microphone: "muted"}
	a.set(page)
	on := map[string]bool{"Reload Page": true, "Back": true, "Zoom Out": true, "Actual Size (now 300%)": true, "Find in Page…": true,
		"Copy Page Link": true, "Unmute Microphone": true, "Turn Off Camera": true, "Stop Sound and Video": true}
	walkMenu(sub("Page"), func(item *application.MenuItem) {
		if item.Enabled() != on[item.Label()] {
			t.Errorf("for a page in a call, %q is on %v, want %v", item.Label(), item.Enabled(), on[item.Label()])
		}
	})
	if got := len(menuLabels(sub("Page"))); got != 14 {
		t.Errorf("Pane ▸ Page has %d items, want 14", got)
	}
	f.calls, commands, commandItems = nil, nil, nil
	for label := range on {
		click(sub("Page").FindByLabel(label))
	}
	click(sub("Page").FindByLabel("Zoom In"))
	click(sub("Page").FindByLabel("Forward"))
	gotCalls = nil
	for _, c := range f.Calls() {
		gotCalls = append(gotCalls, c.String())
	}
	slices.Sort(gotCalls)
	wantCalls = []string{"GoBack[w1]", "GoForward[w1]", "MuteCapture[w1 camera true]", "MuteCapture[w1 microphone false]", "ReloadPage[w1]",
		"StopMedia[w1]", "ZoomPage[w1 -1]", "ZoomPage[w1 0]", "ZoomPage[w1 1]"}
	if !slices.Equal(gotCalls, wantCalls) {
		t.Errorf("Pane ▸ Page's items called %q, want %q", gotCalls, wantCalls)
	}
	slices.Sort(commands)
	if !slices.Equal(commands, []string{"copy-page-link", "find-in-page"}) || !slices.Equal(commandItems, []string{"w1", "w1"}) {
		t.Errorf("Pane ▸ Page's items sent %q for %q, want copy-page-link and find-in-page for w1", commands, commandItems)
	}
	if !windowCommands["find-in-page"] || windowCommands["copy-page-link"] {
		t.Error("Find in Page… should bring Bench forward, for its field, and Copy Page Link shouldn't")
	}
	// VS Code for the Web has no link to copy, and a page at actual size
	// has nothing for Actual Size to do.
	page.Pane = workspace.MenuBarPane{ID: "p1", Item: "w1", Page: true, CanZoomIn: true, CanZoomOut: true}
	a.set(page)
	if a.items.copyLink.Enabled() || a.items.actualSize.Enabled() || a.items.actualSize.Label() != "Actual Size" ||
		a.items.microphone.Enabled() || a.items.microphone.Label() != "Mute Microphone" || !a.items.zoomIn.Enabled() {
		t.Errorf("for VS Code for the Web at actual size: Copy Page Link on %v, %q on %v, %q on %v, Zoom In on %v",
			a.items.copyLink.Enabled(), a.items.actualSize.Label(), a.items.actualSize.Enabled(),
			a.items.microphone.Label(), a.items.microphone.Enabled(), a.items.zoomIn.Enabled())
	}

	// In the "tabs" view, which hides the tab's splits, Split Right shows
	// the window side by side in the wide layout, with the keys that do it
	// there too, and Split Down is off.
	compact := bar
	compact.Pane = workspace.MenuBarPane{ID: "compact", Item: "w5", SideBySide: true}
	a.set(compact)
	if got := a.items.split.Label(); got != "Show Side by Side (Wide Layout) (⌃⌥⇧→)" || !a.items.split.Enabled() || a.items.splitDown.Enabled() {
		t.Errorf("in the \"tabs\" view, Split Right reads %q, on %v, and Split Down is on %v; want Show Side by Side (Wide Layout) (⌃⌥⇧→), on, and Split Down off",
			got, a.items.split.Enabled(), a.items.splitDown.Enabled())
	}
	f.calls = nil
	click(a.items.split)
	if got := f.Calls(); len(got) != 1 || got[0].String() != "ShowSideBySide[w5]" {
		t.Errorf("Show Side by Side called %v, want ShowSideBySide[w5]", got)
	}
	a.set(bar)
	if got := a.items.split.Label(); got != "Split Right (⌃⌥⇧→)" {
		t.Errorf("outside the \"tabs\" view, Split Right reads %q", got)
	}

	// The labels follow the engine: with one tab, closing it leaves it,
	// empty, and it can't move, be deleted, or be stepped from.
	one := workspace.MenuBar{Mode: "wide", Tabs: []workspace.MenuBarTab{{Name: "Code"}}, Closed: "Notes", Zoomed: true,
		Pane: workspace.MenuBarPane{ID: "p1", Item: "w1", NoWindow: true, CanZoom: true}, BackToBench: true,
		ClosedItem: "w9", ClosedTerminal: true}
	a.set(one)
	// With nothing to undo, Undo is off and names nothing; Reopen names
	// what it opens again.
	if got := a.items.undo.Label(); got != "Undo" || a.items.undo.Enabled() {
		t.Errorf("with nothing to undo, Undo reads %q, on %v; want Undo, off", got, a.items.undo.Enabled())
	}
	if got := a.items.reopenItem.Label(); got != "Reopen Closed Terminal" || !a.items.reopenItem.Enabled() {
		t.Errorf("with a closed terminal, Reopen reads %q, on %v; want Reopen Closed Terminal, on", got, a.items.reopenItem.Enabled())
	}
	if a.items.addFront.Enabled() || !a.items.backToBench.Enabled() {
		t.Error("with nothing in front out of the tabs, and a popped-out window with the keyboard, Add Front Window should be off, and Back to Bench on")
	}
	// One tab fewer builds the menu again.
	layouts = a.items.layouts
	if got := a.items.closeTab.Label(); got != "Close All Windows…" {
		t.Errorf("with one tab, Close Tab… reads %q, want Close All Windows…", got)
	}
	for _, item := range []*application.MenuItem{a.items.moveLeft, a.items.moveRight, a.items.deleteTab, a.items.next, a.items.previous,
		a.items.lastTab} {
		if item.Enabled() {
			t.Errorf("with one tab, %q should be off", item.Label())
		}
	}
	if got := menuLabels(sub("Tab")); got[len(got)-1] != "Code" || slices.Contains(got, "Notes") {
		t.Errorf("with one tab, the Tab menu = %q, want it to end with Code alone", got)
	}
	if !a.items.reopen.Enabled() || !layouts["wide"].Checked() || layouts["compact"].Checked() {
		t.Error("Reopen Closed Tab should be on, and the layout mode ticked")
	}
	a.set(workspace.MenuBar{Mode: "wide", Tabs: []workspace.MenuBarTab{{Name: "Code"}}})
	if got := a.items.reopenItem.Label(); got != "Reopen Closed Page or Terminal" || a.items.reopenItem.Enabled() {
		t.Errorf("with nothing closed, Reopen reads %q, on %v; want Reopen Closed Page or Terminal, off", got, a.items.reopenItem.Enabled())
	}
	a.set(one)
	if got := a.items.zoom.Label(); got != "Show All Panes" {
		t.Errorf("with a pane zoomed, Zoom Pane reads %q, want Show All Panes", got)
	}
	if got := a.items.popOut.Label(); got != "Remove from Bench" || a.items.closeWindow.Enabled() || a.items.split.Enabled() {
		t.Errorf("for a window that has closed, Pop Out of Bench reads %q, want Remove from Bench, and Close Window and Split Right should be off", got)
	}
	// Close Window names what the pane shows, as its ⋯ menu does.
	for _, tc := range []struct {
		pane workspace.MenuBarPane
		want string
	}{
		{workspace.MenuBarPane{ID: "p2", Item: "w5"}, "Close Window"},
		{workspace.MenuBarPane{ID: "p2", Item: "w5", Page: true}, "Close Page"},
		{workspace.MenuBarPane{ID: "p2", Item: "w5", Terminal: true}, "Close Terminal"},
		{workspace.MenuBarPane{ID: "p2", Item: "w5", Terminal: true, Shared: true}, "Remove from This Tab"},
		// A window that has closed can still leave this tab.
		{workspace.MenuBarPane{ID: "p2", Item: "w5", NoWindow: true, Shared: true}, "Remove from This Tab"},
	} {
		m := a.now()
		m.Pane = tc.pane
		a.set(m)
		if got := a.items.closeWindow.Label(); got != tc.want || !a.items.closeWindow.Enabled() {
			t.Errorf("for %+v, Close Window reads %q (on %v), want %q, on", tc.pane, got, a.items.closeWindow.Enabled(), tc.want)
		}
	}

	// Wails ticks a radio item as it's clicked. If the engine refuses the
	// mode, the menu ticks the one in use again.
	f.err = errors.New("no")
	click(layouts["auto"])
	if !layouts["wide"].Checked() {
		t.Error("the View menu keeps a layout the engine refused ticked")
	}
	if !slices.Contains(toasts, "no") {
		t.Errorf("the engine's refusal wasn't shown: toasts %q", toasts)
	}

	// Settings… opens Settings, which applies to every tab, with Bench's
	// window in front.
	if settingsCommand.Name != "settings" || !windowCommands[settingsCommand.Name] {
		t.Errorf("Settings… sends %q, want settings, which brings Bench forward", settingsCommand.Name)
	}
}

// menuLabels lists a menu's labels, without its submenus' items, with "-"
// for each separator.
func menuLabels(m *application.Menu) []string {
	var labels []string
	for i := 0; ; i++ {
		item := m.ItemAt(i)
		if item == nil {
			return labels
		}
		if item.IsSeparator() {
			labels = append(labels, "-")
			continue
		}
		labels = append(labels, item.Label())
	}
}

func walkMenu(m *application.Menu, fn func(*application.MenuItem)) {
	for i := 0; ; i++ {
		item := m.ItemAt(i)
		if item == nil {
			return
		}
		if item.IsSeparator() {
			continue
		}
		fn(item)
		if sub := item.GetSubmenu(); sub != nil {
			walkMenu(sub, fn)
		}
	}
}

// syncBuffer is a bytes.Buffer that's safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// callBound calls a bound method the way the frontend's runtime does.
func callBound(t *testing.T, base, method string, args ...any) (int, []byte) {
	t.Helper()
	if args == nil {
		args = []any{}
	}
	payload, err := json.Marshal(map[string]any{
		"object": 0, // Call
		"method": 0, // CallBinding
		"args": map[string]any{
			"call-id":    fmt.Sprintf("test-%s-%d", method, time.Now().UnixNano()),
			"methodName": servicePrefix + method,
			"args":       args,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+"/wails/runtime", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("calling %s: %v", method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("calling %s: %v", method, err)
	}
	return resp.StatusCode, body
}

// do sends a request and returns its status.
func do(t *testing.T, req *http.Request) int {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func get(t *testing.T, url string) (body, contentType string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET %s = %s", url, resp.Status)
	}
	return string(data), resp.Header.Get("Content-Type")
}

// waitForServer waits until url answers, or Run fails.
func waitForServer(t *testing.T, url string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("Run returned early: %v", err)
		default:
		}
		if resp, err := http.Get(url); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s didn't answer", url)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
