//go:build server

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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
	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Platform: platform.NewDemo(clock.Real()),
			NewTerms: func(cb term.Callbacks) term.Host { return term.NewEcho(cb) },
			Dir:      t.TempDir(),
			Demo:     true,
			Port:     port,
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
	menu := appMenu(menuActions{settings: func() {}, fillScreen: func() {}, rearrange: func() {}})
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
	for _, want := range []string{"Bench", "About Bench", "Settings…", "Hide Bench", "Hide Others", "Show All",
		"Quit Bench", "Edit", "Undo", "Redo", "Cut", "Copy", "Paste", "Select All", "View", "Fill the Screen",
		"Rearrange", "Window", "Minimise"} {
		if !slices.Contains(labels, want) {
			t.Errorf("the app menu has no %q; it has %q", want, labels)
		}
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
