package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

func TestPaneGeometry(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	v := h.view()
	d := v.Display
	if d.Area != paneArea || d.Compact || d.Focus != "p11" || d.SplitsDisabled != "" {
		t.Fatalf("display: area %v compact %v focus %s", d.Area, d.Compact, d.Focus)
	}
	want := layout.Frames(v.Tabs[1].Root, paneArea, 4)
	if len(d.Panes) != len(want) {
		t.Fatalf("panes: %d, want %d", len(d.Panes), len(want))
	}
	for i, p := range d.Panes {
		if p.ID != want[i].ID || p.Rect != want[i].Rect || p.Header != layout.HeaderHeight || p.Body != layout.Body(want[i].Rect, layout.HeaderHeight) {
			t.Errorf("pane %d: %+v, want %+v", i, p, want[i])
		}
	}
	if d.Panes[0].Rect != (layout.Rect{X: 4, Y: 44, W: 1657, H: 1332}) {
		t.Errorf("first pane: %v", d.Panes[0].Rect)
	}
	if len(d.Dividers) != 1 || d.Dividers[0].SplitID != "s13" {
		t.Errorf("dividers: %+v", d.Dividers)
	}
	// A docked window's frame is its pane body offset by the content frame.
	zed := h.windowID(zedID, "bench — switch.go")
	body := d.Panes[0].Body
	wantFrame := layout.Rect{X: body.X + contentFrame.X, Y: body.Y + contentFrame.Y, W: body.W, H: body.H}
	if got := h.frame(zed); got != wantFrame {
		t.Errorf("Zed's frame %v, want %v", got, wantFrame)
	}
}

func TestPaneAreaMovesWindows(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	area := layout.Rect{X: 0, Y: 40, W: 2000, H: 1000}
	mustOK(t, h.e.SetPaneArea(area))
	zed := h.windowID(zedID, "bench — switch.go")
	if h.frame(zed) != h.bodyOf("p11") || h.view().Display.Area != area {
		t.Errorf("Zed wasn't docked in the new area")
	}
}

func TestWebItemShownParksPaneWindows(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Put the terminal in front of Zed and VS Code.
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if !h.e.win.IsParked(zed) || !h.e.win.IsParked(code) {
		t.Errorf("a Built-in item is shown, so the pane's windows should be parked")
	}
	mustOK(t, h.e.ShowItem("p11", "w4"))
	if h.e.win.IsParked(code) || h.frame(code) != h.bodyOf("p11") {
		t.Errorf("VS Code should be docked again")
	}
	if !slices.Contains(h.focusCalls(), code) {
		t.Errorf("ShowItem should focus VS Code")
	}
}

func TestNeedsRoomAndFullScreen(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddWindow(zedID, "huge", platform.WithMinSize(3000, 2000))
	}})
	mustOK(t, h.e.SwitchTab(1))
	huge := h.windowID(zedID, "huge")
	id := h.itemOf(huge)
	mustOK(t, h.e.ShowItem("p11", id))
	it := h.item(id)
	if it.Status != StatusNeedsRoom || !strings.Contains(it.StatusText, "can't shrink") ||
		!strings.Contains(it.StatusText, "compact layout") {
		t.Errorf("needs room: %s %q", it.Status, it.StatusText)
	}
	mustOK(t, h.e.SetMode("compact"))
	if it := h.item(id); strings.Contains(it.StatusText, "compact layout") {
		t.Errorf("in the compact layout, don't suggest it: %q", it.StatusText)
	}
	mustOK(t, h.e.SetMode("auto"))
	zed := h.windowID(zedID, "bench — switch.go")
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.p.SetFullScreen(zed, true)
	h.drain()
	if it := h.item("w3"); it.Status != StatusFullScreen || it.StatusText != "In full screen" {
		t.Errorf("full screen: %s %q", it.Status, it.StatusText)
	}
	h.p.SetFullScreen(zed, false)
	h.drain()
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("back from full screen: %s %v", it.Status, h.frame(zed))
	}
}

func TestOverlayHidesWebViews(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SetOverlay(true))
	if wv := h.p.WebViews(); wv["w1"].Visible || wv["w2"].Visible {
		t.Errorf("raised: web views should be hidden")
	}
	// Arranging while raised doesn't show them.
	mustOK(t, h.e.Rearrange())
	if wv := h.p.WebViews(); wv["w1"].Visible {
		t.Errorf("raised: rearranging showed a web view")
	}
	mustOK(t, h.e.SetOverlay(false))
	if wv := h.p.WebViews(); !wv["w1"].Visible || !wv["w2"].Visible {
		t.Errorf("lowered: web views should show again")
	}
	if h.p.Raised() {
		t.Errorf("lowered: Bench is still raised")
	}
}

func TestTerminals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.TermInput("w5", "ls\r"))
	mustOK(t, h.e.TermResize("w5", 120, 40))
	if h.terms.written["w5"] != "ls\r" || h.terms.sizes["w5"] != [2]int{120, 40} {
		t.Errorf("input %q size %v", h.terms.written["w5"], h.terms.sizes["w5"])
	}
	mustErr(t, h.e.TermInput("w99", "x"), "There's no such terminal.")
	mustErr(t, h.e.TermResize("w5", 0, 0), "That terminal size isn't valid.")
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("$ ls\r\n"), Offset: 120}
	snap, err := h.e.TermSnapshot("w5")
	mustOK(t, err)
	if b, _ := base64.StdEncoding.DecodeString(snap.Data); string(b) != "$ ls\r\n" || snap.Offset != 120 {
		t.Errorf("snapshot: %q ending at %d", b, snap.Offset)
	}
	if _, err := h.e.TermSnapshot("w3"); err == nil {
		t.Errorf("TermSnapshot of an App item should fail")
	}

	// Output is batched per item, at most once a frame, starting where
	// the session says.
	h.terms.cb.Data("w5", 120, []byte("one "))
	h.terms.cb.Data("w5", 124, []byte("two"))
	h.clk.Advance(frame)
	h.em.mu.Lock()
	data := slices.Clone(h.em.data)
	h.em.mu.Unlock()
	if len(data) != 1 || data[0].ID != "w5" || data[0].Offset != 120 {
		t.Fatalf("term:data: %+v", data)
	}
	if b, _ := base64.StdEncoding.DecodeString(data[0].Data); string(b) != "one two" {
		t.Errorf("batched output: %q", b)
	}

	// A view that has drawn output says so.
	mustOK(t, h.e.TermAck("w5", 127))
	if h.terms.acks["w5"] != 127 {
		t.Errorf("acknowledged: %d", h.terms.acks["w5"])
	}

	// A title from the session becomes the item's.
	h.terms.cb.Title("w5", "vim — ~/code", false)
	h.clk.Advance(frame)
	if got := h.item("w5").Title; got != "vim — ~/code" {
		t.Errorf("title: %q", got)
	}
	// A session that exits removes its item, and its pane closes up.
	h.terms.cb.Exit("w5", nil)
	h.clk.Advance(frame)
	v := h.view()
	if _, ok := v.Items["w5"]; ok {
		t.Errorf("the item should be gone")
	}
	if len(layout.Leaves(v.Tabs[1].Root)) != 1 {
		t.Errorf("its pane should close up")
	}
	// The host forgets the ended session, and its output, with the item.
	if h.terms.Running("w5") {
		t.Errorf("the ended session should be closed")
	}
}

// TestTerminalOutputIsSentInSmallEvents checks that a frame's worth of fast
// output goes out in term:data events small enough for Wails to send
// straight away, each saying where its output starts.
func TestTerminalOutputIsSentInSmallEvents(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	out := make([]byte, 6*termChunk+100)
	for i := range out {
		out[i] = byte('a' + i%26)
	}
	h.terms.cb.Data("w5", 1000, out[:20000])
	h.terms.cb.Data("w5", 21000, out[20000:])
	h.clk.Advance(frame)
	h.em.mu.Lock()
	data := slices.Clone(h.em.data)
	h.em.mu.Unlock()
	var got []byte
	for i, d := range data {
		b, _ := base64.StdEncoding.DecodeString(d.Data)
		if len(b) > termChunk {
			t.Errorf("event %d carries %d bytes, want at most %d", i, len(b), termChunk)
		}
		if want := int64(1000 + len(got)); d.Offset != want {
			t.Errorf("event %d starts at %d, want %d", i, d.Offset, want)
		}
		if js, _ := json.Marshal(map[string]any{"name": "term:data", "data": d}); len(js) > 8192 {
			t.Errorf("event %d is %d bytes of JSON, over Wails' 8 KB", i, len(js))
		}
		got = append(got, b...)
	}
	if !bytes.Equal(got, out) {
		t.Errorf("the events carry %d bytes, want the %d output", len(got), len(out))
	}
}

// TestTerminalOutputDoesntWaitForTheEngine checks that output goes out
// while something holds the engine's lock, as an arrange does.
func TestTerminalOutputDoesntWaitForTheEngine(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.e.mu.Lock()
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		h.terms.cb.Data("w5", 0, []byte("echo"))
		// Send the batch now: advancing the clock would also run the
		// engine's own timers, which take the lock.
		h.e.flushTerms()
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Error("output waited for the engine's lock")
	}
	h.e.mu.Unlock()
	<-sent
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	if len(h.em.data) != 1 {
		t.Errorf("term:data: %+v", h.em.data)
	}
}

// commandConfig has a terminal that runs a command, with a name.
const commandConfig = `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ terminal = "~/code/bench", command = "go test ./...", name = "Tests" }]
`

// TestCommandTerminalStaysWhenItEnds checks that a terminal whose command
// finishes keeps its output and item, Return runs the command again, and it
// comes back after a restart.
func TestCommandTerminalStaysWhenItEnds(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	if id == "" || h.terms.starts != 1 || h.terms.started[id].Command != "go test ./..." {
		t.Fatalf("the command terminal didn't start: %q, %d starts", id, h.terms.starts)
	}
	h.terms.snaps[id] = term.Snapshot{Data: []byte("ok  bench/internal/layout\r\n"), Offset: 27}
	h.terms.end(id)
	h.terms.cb.Exit(id, nil)
	h.clk.Advance(frame)

	it := h.item(id)
	if !it.Ended || it.Tab != 0 {
		t.Fatalf("after the command ended: %+v", it)
	}
	if snap, err := h.e.TermSnapshot(id); err != nil || snap.Offset != 27 {
		t.Errorf("its output should still be there: %+v, %v", snap, err)
	}
	// Typing does nothing, and isn't refused, until Return.
	mustOK(t, h.e.TermInput(id, "q"))
	if h.terms.starts != 1 || h.em.lastToast() == errTermEnded.Error() {
		t.Errorf("typing started it %d times", h.terms.starts-1)
	}
	h.restart()
	if it := h.item(id); it.Ended || it.Title != "Tests" {
		t.Errorf("after a restart: %+v", it)
	}
	h.terms.end(id)
	h.terms.cb.Exit(id, errors.New("exit status 1"))
	h.clk.Advance(frame)
	starts := h.terms.starts
	mustOK(t, h.e.TermInput(id, "\r"))
	h.clk.Advance(frame)
	if h.terms.starts != starts+1 || h.item(id).Ended || !h.terms.Running(id) {
		t.Errorf("Return should run the command again: %d starts, %+v", h.terms.starts-starts, h.item(id))
	}
	mustOK(t, h.e.TermInput(id, "x"))
	if h.terms.written[id] != "x" {
		t.Errorf("typing should reach the new session: %q", h.terms.written[id])
	}
}

// TestCommandTerminalRunsAgainWithoutItsSession checks that Return runs an
// ended command again even if the host has forgotten its session.
func TestCommandTerminalRunsAgainWithoutItsSession(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	h.terms.cb.Exit(id, nil)
	h.clk.Advance(frame)
	h.terms.mu.Lock()
	delete(h.terms.started, id)
	h.terms.mu.Unlock()
	mustOK(t, h.e.TermInput(id, "\r"))
	if h.terms.starts != 2 || h.item(id).Ended || !h.terms.Running(id) {
		t.Errorf("Return should run the command again: %d starts, %+v", h.terms.starts, h.item(id))
	}

	// Once Bench is closing, Return runs nothing.
	h.terms.cb.Exit(id, nil)
	h.clk.Advance(frame)
	mustOK(t, h.e.Close())
	h.terms.mu.Lock()
	delete(h.terms.started, id)
	h.terms.mu.Unlock()
	_ = h.e.TermInput(id, "\r")
	if h.terms.starts != 2 {
		t.Errorf("Return ran the command after Close: %d starts", h.terms.starts)
	}
}

// TestTerminalNameStaysUntilTheShellSetsATitle checks that a terminal's
// configured name is its title until the shell sets one.
func TestTerminalNameStaysUntilTheShellSetsATitle(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	steps := []struct {
		title   string
		byShell bool
		want    string
	}{
		{"go — ~/code/bench", false, "Tests"},
		{"vim notes.md", true, "vim notes.md"},
		{"zsh — ~/code/bench", false, "Tests"},
	}
	if got := h.item(id).Title; got != "Tests" {
		t.Errorf("at the start: %q", got)
	}
	for _, s := range steps {
		h.terms.cb.Title(id, s.title, s.byShell)
		h.clk.Advance(frame)
		if got := h.item(id).Title; got != s.want {
			t.Errorf("after %q (by the shell: %t): %q, want %q", s.title, s.byShell, got, s.want)
		}
	}
}

// TestPopOutTerminalThatEndsMeanwhile checks that a terminal that ends while
// its window opens doesn't leave the window open.
func TestPopOutTerminalThatEndsMeanwhile(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.em.onPop = func(id string) {
		h.terms.cb.Exit(id, nil)
		h.clk.Advance(frame)
	}
	mustOK(t, h.e.PopOut("w5"))
	if _, ok := h.view().Items["w5"]; ok {
		t.Fatal("the terminal should be gone")
	}
	if !slices.Equal(h.em.closed, []string{"w5"}) {
		t.Errorf("CloseTerminalWindow: %v", h.em.closed)
	}
}

func TestOpenLink(t *testing.T) {
	var opened []string
	h := newHarness(t, setup{options: func(o *Options) {
		o.OpenLink = func(address string) error { opened = append(opened, address); return nil }
	}})
	mustOK(t, h.e.OpenLink("https://github.com/itbm/bench/pull/42"))
	mustOK(t, h.e.OpenLink("http://localhost:8080/"))
	for _, address := range []string{"file:///etc/passwd", "javascript:alert(1)", "ssh://host", "https://", "not a link"} {
		mustErr(t, h.e.OpenLink(address), "Bench only opens http and https links.")
	}
	if want := []string{"https://github.com/itbm/bench/pull/42", "http://localhost:8080/"}; !slices.Equal(opened, want) {
		t.Errorf("opened %q, want %q", opened, want)
	}

	h = newHarness(t, setup{})
	mustErr(t, h.e.OpenLink("https://example.com/"), "This build of Bench can't open links.")
}

func TestTerminalStartFailure(t *testing.T) {
	h := newHarness(t, setup{})
	h.terms.startErr = errors.New("no pty")
	mustOK(t, h.e.SwitchTab(1))
	it := h.item("w5")
	if it.Status != StatusMissing || !strings.Contains(it.StatusText, "no pty") {
		t.Errorf("w5: %s %q", it.Status, it.StatusText)
	}
}

// vscodeConfig has a VS Code for the Web item.
const vscodeConfig = `
[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ vscode = "~/code/bench" }]
`

// fakeServer is a VSCodeServer, on port 9999 unless port says otherwise.
// Closing done stands for the server ending on its own.
type fakeServer struct {
	port    int
	done    chan struct{}
	stopped atomic.Bool
}

func newFakeServer(port int) *fakeServer {
	return &fakeServer{port: port, done: make(chan struct{})}
}

func (f *fakeServer) URL(folder string) string {
	port := f.port
	if port == 0 {
		port = 9999
	}
	return "http://localhost:" + strconv.Itoa(port) + "/?tkn=t&folder=" + folder
}
func (f *fakeServer) Stop() error           { f.stopped.Store(true); return nil }
func (f *fakeServer) Done() <-chan struct{} { return f.done }

func TestVSCodeForTheWeb(t *testing.T) {
	srv := &fakeServer{}
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { starts++; return srv, nil }
	}})
	h.advance(100 * time.Millisecond)
	wv := h.p.WebViews()["w1"]
	if !wv.Visible || wv.View.URL != "http://localhost:9999/?tkn=t&folder=~/code/bench" {
		t.Errorf("web view: %+v", wv)
	}
	if it := h.item("w1"); it.App != "VS Code" || it.Mono != "VS" || it.Status != StatusReady || it.URL != "" {
		t.Errorf("item: %+v", it)
	}
	mustOK(t, h.e.Rearrange())
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.SetOverlay(false))
	h.advance(100 * time.Millisecond)
	if starts != 1 {
		t.Errorf("the server should start once, started %d times", starts)
	}
	mustOK(t, h.e.Close())
	if !srv.stopped.Load() {
		t.Errorf("Close should stop the server")
	}
}

// TestVSCodeRestartsAfterItStops checks that a server that ends on its own,
// as when VS Code updates itself, is forgotten and stopped, and that the
// item is shown at a new server's address rather than the dead one's.
func TestVSCodeRestartsAfterItStops(t *testing.T) {
	servers := []*fakeServer{newFakeServer(9001), newFakeServer(9002)}
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) {
			srv := servers[starts]
			starts++
			return srv, nil
		}
	}})
	h.advance(100 * time.Millisecond)
	if wv := h.p.WebViews()["w1"]; !wv.Visible || !strings.Contains(wv.View.URL, ":9001/") {
		t.Fatalf("web view: %+v", wv)
	}
	h.advance(vscodeRetry)

	close(servers[0].done)
	waitUntil(t, "the ended server to be stopped", servers[0].stopped.Load)
	h.advance(100 * time.Millisecond)
	if starts != 2 {
		t.Fatalf("the server started %d times, want twice", starts)
	}
	if wv := h.p.WebViews()["w1"]; !wv.Visible || !strings.Contains(wv.View.URL, ":9002/") {
		t.Errorf("web view after the server stopped: %+v, want the new server's address", wv)
	}
	if it := h.item("w1"); it.Status != StatusReady {
		t.Errorf("item after the server stopped: %+v", it)
	}
	mustOK(t, h.e.Close())
	if !servers[1].stopped.Load() {
		t.Errorf("Close should stop the new server")
	}
}

// TestVSCodeStoppedTooSoon checks that a server that ends straight after
// it has started isn't started over and over: the item says so, and the
// server starts again only when the item is shown after vscodeRetry.
func TestVSCodeStoppedTooSoon(t *testing.T) {
	srv, next := newFakeServer(9001), newFakeServer(9002)
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) {
			starts++
			if starts == 1 {
				return srv, nil
			}
			return next, nil
		}
	}})
	h.advance(100 * time.Millisecond)
	close(srv.done)
	waitUntil(t, "the ended server to be stopped", srv.stopped.Load)
	h.advance(100 * time.Millisecond)
	if it := h.item("w1"); starts != 1 || it.Status != StatusMissing || it.StatusText != vscodeStopped {
		t.Errorf("after the server stopped at once: %d starts, item %s %q", starts, it.Status, it.StatusText)
	}
	if wv := h.p.WebViews()["w1"]; wv.Visible {
		t.Errorf("the web view of a server that has stopped is still shown: %+v", wv)
	}
	h.advance(vscodeRetry)
	mustOK(t, h.e.Rearrange())
	h.advance(100 * time.Millisecond)
	if wv := h.p.WebViews()["w1"]; starts != 2 || !wv.Visible || !strings.Contains(wv.View.URL, ":9002/") {
		t.Errorf("after showing it again: %d starts, web view %+v", starts, wv)
	}
}

// TestVSCodeRetriesAfterAFailedStart checks that a start that failed, as
// when another process took the port, is tried again when the item is next
// shown, but not straight away.
func TestVSCodeRetriesAfterAFailedStart(t *testing.T) {
	srv := newFakeServer(0)
	fail := true
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) {
			starts++
			if fail {
				return nil, errors.New("the port is taken")
			}
			return srv, nil
		}
	}})
	h.advance(100 * time.Millisecond)
	if it := h.item("w1"); it.Status != StatusMissing || !strings.Contains(it.StatusText, "the port is taken") {
		t.Errorf("failed start: %+v", it)
	}
	mustOK(t, h.e.Rearrange())
	h.advance(100 * time.Millisecond)
	if starts != 1 {
		t.Errorf("the server started %d times straight after failing, want once", starts)
	}

	fail = false
	h.advance(vscodeRetry)
	mustOK(t, h.e.Rearrange())
	h.advance(100 * time.Millisecond)
	if starts != 2 {
		t.Errorf("the server started %d times, want a second try", starts)
	}
	if it := h.item("w1"); it.Status != StatusReady {
		t.Errorf("item after the second try: %+v", it)
	}
	if wv := h.p.WebViews()["w1"]; !wv.Visible || wv.View.URL != srv.URL("~/code/bench") {
		t.Errorf("web view after the second try: %+v", wv)
	}
}

// TestVSCodeSlowFailureIsShown checks that a start that fails after longer
// than vscodeRetry, as one that times out does, isn't tried again at once:
// the items say why it failed, rather than starting for ever.
func TestVSCodeSlowFailureIsShown(t *testing.T) {
	var clk *aheadClock
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		clk = &aheadClock{Fake: o.Clock.(*clock.Fake)}
		o.Clock = clk
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) {
			starts++
			if starts == 1 {
				clk.ahead.Add(int64(30 * time.Second))
			}
			return nil, errors.New("code serve-web didn't print its address")
		}
	}})
	h.advance(100 * time.Millisecond)
	mustOK(t, h.e.Rearrange())
	h.advance(100 * time.Millisecond)
	if it := h.item("w1"); starts != 1 || it.Status != StatusMissing || !strings.Contains(it.StatusText, "didn't print") {
		t.Errorf("after a slow failure: %d starts, item %s %q", starts, it.Status, it.StatusText)
	}
	h.advance(vscodeRetry)
	mustOK(t, h.e.Rearrange())
	h.advance(100 * time.Millisecond)
	if starts != 2 {
		t.Errorf("the server started %d times, want a second try", starts)
	}
}

// aheadClock is a fake clock whose Now runs ahead of it by ahead, for a
// start that takes a while.
type aheadClock struct {
	*clock.Fake
	ahead atomic.Int64
}

func (c *aheadClock) Now() time.Time { return c.Fake.Now().Add(time.Duration(c.ahead.Load())) }

// TestVSCodeReloadsPoppedOutItems checks that a popped-out VS Code for the
// Web item, which arranging doesn't show, still starts a server that ends
// again, and loads the new server's address in its own window.
func TestVSCodeReloadsPoppedOutItems(t *testing.T) {
	servers := []*fakeServer{newFakeServer(9001), newFakeServer(9002)}
	starts := 0
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) {
			srv := servers[starts]
			starts++
			return srv, nil
		}
	}})
	h.advance(100 * time.Millisecond)
	mustOK(t, h.e.PopOut("w1"))
	h.advance(vscodeRetry)

	close(servers[0].done)
	waitUntil(t, "the ended server to be stopped", servers[0].stopped.Load)
	h.advance(100 * time.Millisecond)
	if starts != 2 {
		t.Fatalf("the server started %d times, want twice", starts)
	}
	if wv := h.p.WebViews()["w1"]; !wv.PoppedOut || !strings.Contains(wv.View.URL, ":9002/") {
		t.Errorf("popped-out web view after the server stopped: %+v, want the new server's address in its own window", wv)
	}
}

// TestVSCodeAddressIsntSaved checks that a VS Code for the Web item's
// address, which holds the server's connection token, isn't kept or saved
// in state.json.
func TestVSCodeAddressIsntSaved(t *testing.T) {
	h := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return newFakeServer(0), nil }
	}})
	h.advance(100 * time.Millisecond)
	h.p.EmitEvent(platform.Event{Kind: platform.WebURL, ItemID: "w1", Body: "http://127.0.0.1:9999/?tkn=secret&folder=%2Fcode"})
	h.settle()
	mustOK(t, h.e.Close())
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") {
		t.Errorf("state.json holds the connection token:\n%s", data)
	}
}

// waitUntil waits, in real time, for something another goroutine does.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestVSCodeUnavailable(t *testing.T) {
	h := newHarness(t, setup{config: vscodeConfig})
	if it := h.item("w1"); it.Status != StatusMissing || it.StatusText == "" {
		t.Errorf("without StartVSCode: %+v", it)
	}
	h2 := newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return nil, errors.New("no code") }
	}})
	h2.advance(100 * time.Millisecond)
	if it := h2.item("w1"); it.Status != StatusMissing || !strings.Contains(it.StatusText, "no code") {
		t.Errorf("failed start: %+v", it)
	}
}

func TestRearrangeToasts(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.Rearrange())
	if got := h.em.lastToast(); got != "Rearranged Code" {
		t.Errorf("toast: %q", got)
	}
}

func TestBenchMovedRedocks(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	moved := contentFrame
	moved.X += 100
	moved.W -= 100
	h.p.SetContentFrame(moved)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.drain()
	zed := h.windowID(zedID, "bench — switch.go")
	if got, body := h.frame(zed), h.bodyOf("p11"); got != body || body.X != h.view().Display.Panes[0].Body.X+100 {
		t.Errorf("Zed %v, body %v", got, body)
	}
}

func TestScreensChangedRearrangesAfterSettling(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ResetCalls()
	h.p.SetScreens(laptop)
	h.p.SetContentFrame(laptop.Visible)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.advance(time.Second)
	if v := h.view(); v.SizeClass != "wide" || v.ScreenWidth != 2560 {
		t.Errorf("before settling: %s %v", v.SizeClass, v.ScreenWidth)
	}
	h.advance(600 * time.Millisecond)
	v := h.view()
	if v.SizeClass != "compact" || v.ScreenWidth != 1512 {
		t.Fatalf("after settling: %s %v", v.SizeClass, v.ScreenWidth)
	}
	if v.Display.Root.Dir != layout.Column {
		t.Errorf("Code stacks its panes: %s", v.Display.Root.Dir)
	}
	if len(h.focusCalls()) != 0 {
		t.Errorf("a screen change mustn't change focus: %v", h.focusCalls())
	}
}

func TestBenchMovedToAnotherScreen(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.SetScreens(monitor, platform.Screen{ID: "laptop", Frame: layout.Rect{X: 2560, W: 1512, H: 982},
		Visible: layout.Rect{X: 2560, Y: 25, W: 1512, H: 957}})
	h.p.SetContentFrame(layout.Rect{X: 2560, Y: 25, W: 1512, H: 957})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.advance(time.Second)
	if h.view().SizeClass != "wide" {
		t.Errorf("the size class waits for macOS to settle")
	}
	h.advance(time.Second)
	if v := h.view(); v.SizeClass != "compact" || v.ScreenWidth != 1512 {
		t.Errorf("after settling: %s %v", v.SizeClass, v.ScreenWidth)
	}
}

func TestSizeReason(t *testing.T) {
	tests := []struct {
		screen platform.Screen
		mode   string
		tab    int
		want   string
	}{
		{monitor, "auto", 1, "Wide layout, chosen for a window 2,560 pt wide"},
		{laptop, "auto", 1, "Compact layout (stack panes), chosen for a window 1,512 pt wide"},
		{laptop, "auto", 0, "Compact layout (one pane, every window as a sub-tab), chosen for a window 1,512 pt wide"},
		{monitor, "compact", 1, "Compact layout (stack panes), set by hand"},
		{laptop, "wide", 1, "Wide layout, set by hand"},
	}
	for _, tc := range tests {
		h := newHarness(t, setup{screen: tc.screen})
		mustOK(t, h.e.SetMode(tc.mode))
		mustOK(t, h.e.SwitchTab(tc.tab))
		if got := h.view().SizeReason; got != tc.want {
			t.Errorf("%s %s tab %d: got %q, want %q", tc.screen.Name, tc.mode, tc.tab, got, tc.want)
		}
	}
	cfg := strings.Replace(string(config.ExampleTOML()), `small_screens = "stack"`, `small_screens = "same"`, 1)
	h := newHarness(t, setup{screen: laptop, config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	if got := h.view().SizeReason; got != "Compact layout (keep the layout), chosen for a window 1,512 pt wide" {
		t.Errorf("same: %q", got)
	}
	mustErr(t, h.e.SetMode("huge"), "That isn't a layout: use auto, wide, or compact.")
}

func TestGroupThousands(t *testing.T) {
	tests := map[float64]string{0: "0", 900: "900", 1512: "1,512", 2560: "2,560", 1234567: "1,234,567", 1799.6: "1,800"}
	for v, want := range tests {
		if got := groupThousands(v); got != want {
			t.Errorf("%v: got %q, want %q", v, got, want)
		}
	}
}

func TestTabsView(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	// Research shows one pane holding every window.
	mustOK(t, h.e.SwitchTab(2))
	d := h.view().Display
	if !d.Compact || d.Focus != layout.CompactPaneID || d.SplitsDisabled != "Splits are hidden in the compact layout" {
		t.Fatalf("display: compact %v focus %s reason %q", d.Compact, d.Focus, d.SplitsDisabled)
	}
	if len(d.Panes) != 1 || d.Panes[0].ID != "compact" || !slices.Equal(d.Panes[0].Items, []string{"w6", "w17", "w18", "w7"}) {
		t.Fatalf("panes: %+v", d.Panes)
	}
	note := h.windowID(obsidianID, "Daily note")
	docs := h.windowID(braveID, "Wails v3 docs")
	body := h.bodyOf("compact")
	if h.frame(note) != body || h.frame(docs) != body {
		t.Errorf("every window shares the one pane: %v %v, body %v", h.frame(note), h.frame(docs), body)
	}
	mustOK(t, h.e.ShowItem("compact", "w7"))
	if d := h.view().Display; d.Panes[0].Active != "w7" {
		t.Errorf("ShowItem(compact): active %s", d.Panes[0].Active)
	}
	z := h.p.ZOrder()
	if slices.Index(z, note) > slices.Index(z, docs) {
		t.Errorf("Obsidian should be in front")
	}
	// Splitting is refused with the reason; moving to a tab still works.
	mustErr(t, h.e.MoveItem("w7", 2, "compact", "right"), "Splits are hidden in the compact layout. Choose Wide to show them.")
	mustErr(t, h.e.MoveItem("w7", 2, "p14", "left"), "Splits are hidden in the compact layout. Choose Wide to show them.")
	mustErr(t, h.e.MoveDirection("left"), "Splits are hidden in the compact layout. Choose Wide to show them.")
	mustOK(t, h.e.MoveItemToTab("w7", 1))
	if h.item("w7").Tab != 1 {
		t.Errorf("MoveItemToTab in the tabs view: tab %d", h.item("w7").Tab)
	}
	// CycleSubtab cycles the compact pane's items.
	mustOK(t, h.e.ShowItem("compact", "w18"))
	mustOK(t, h.e.CycleSubtab(1))
	if got := h.view().Display.Panes[0].Active; got != "w6" {
		t.Errorf("CycleSubtab wraps: %s", got)
	}
	// Back on a big screen, the tree is there: Obsidian's pane closed up
	// when it moved to Code.
	mustOK(t, h.e.SetMode("wide"))
	if d := h.view().Display; d.Compact || len(d.Panes) != 1 || d.Panes[0].ID != "p14" {
		t.Errorf("wide: compact %v, panes %+v", d.Compact, d.Panes)
	}
}

func TestHeadlessArrangeOnce(t *testing.T) {
	h := newHarness(t, setup{noStart: true, options: func(o *Options) { o.Headless = true }})
	h.p.SetContentFrame(layout.Rect{})
	done := make(chan error, 1)
	go func() { done <- h.e.ArrangeOnce(context.Background(), "code") }()
	h.waitFor(done)
	zed := h.windowID(zedID, "bench — switch.go")
	frames := layout.Frames(h.e.View().Tabs[1].Root, monitor.Visible, 4)
	body := layout.Body(frames[0].Rect, layout.HeaderHeight)
	if got := h.frame(zed); got != body {
		t.Errorf("headless: Zed %v, want the body %v of the screen's visible area", got, body)
	}
	if len(h.p.WebViews()) != 0 || h.terms.starts != 0 {
		t.Errorf("headless mode skips web views and terminals")
	}
	if !h.p.IsHidden(braveID) {
		t.Errorf("Research's Brave should be hidden")
	}
	mustOK(t, h.e.Close())
	if fileExists(h.dir + "/state.json") {
		t.Errorf("headless mode saves nothing")
	}
	if h.p.IsHidden(braveID) {
		t.Errorf("Close should show the apps Bench hid")
	}
}

func TestArrangeOnceShowsFirstWindowAndWaits(t *testing.T) {
	h := newHarness(t, setup{noStart: true, config: launchConfig, options: func(o *Options) { o.Headless = true },
		world: func(p *platform.Fake) {
			p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(time.Second), platform.AppLaunchWindows("Calendar"))
			p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://apple.com", "Apple"))
		}})
	done := make(chan error, 1)
	go func() { done <- h.e.ArrangeOnce(context.Background(), "Plan") }()
	h.waitFor(done)
	cal := h.windowID(calendarID, "Calendar")
	if h.itemOf(cal) == "" || h.frame(cal).W == 0 {
		t.Errorf("Calendar should be docked once it opened")
	}
	mustErr(t, h.e.ArrangeOnce(context.Background(), "Nope"), "There's no tab called Nope.")
}

func TestArrangeOnceStopsWithContext(t *testing.T) {
	h := newHarness(t, setup{noStart: true, config: launchConfig, options: func(o *Options) { o.Headless = true },
		world: func(p *platform.Fake) { p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(time.Hour)) }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.e.ArrangeOnce(ctx, "Plan") }()
	h.advance(500 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ArrangeOnce didn't stop")
	}
}

// waitFor moves the clock until a call running in another goroutine
// returns, failing the test on an error.
func (h *harness) waitFor(done chan error) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			mustOK(h.t, err)
			return
		default:
		}
		h.clk.Advance(50 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	h.t.Fatal("the call didn't return")
}

func TestWebViewCallsAreInOrder(t *testing.T) {
	// Comms shows two web views. However often it's done, Bench hides and
	// shows them in the same order, by item ID.
	want := []string{`HideWebView("w1")`, `HideWebView("w2")`, `ShowWebView({5 86 1144 1289} "w1")`, `ShowWebView({1155 86 1400 1289} "w2")`}
	for range 10 {
		h := newHarness(t, setup{})
		h.p.ResetCalls()
		mustOK(t, h.e.SetOverlay(true))
		mustOK(t, h.e.SetOverlay(false))
		var got []string
		for _, c := range h.calls("ShowWebView", "HideWebView") {
			got = append(got, c.String())
		}
		if !slices.Equal(got, want) {
			t.Fatalf("web view calls:\n got %q\nwant %q", got, want)
		}
	}
}

func TestVSCodeUnavailableInAnotherTab(t *testing.T) {
	// Without StartVSCode, a VS Code item says so before it's ever shown.
	cfg := "[[tab]]\nname = \"Home\"\nlayout = \"single\"\n  [[tab.pane]]\n  items = [{ terminal = \"~\" }]\n" + vscodeConfig
	h := newHarness(t, setup{config: cfg})
	if it := h.item("w2"); it.Status != StatusMissing || it.StatusText != vscodeMissing {
		t.Errorf("w2: %s %q", it.Status, it.StatusText)
	}
}
