package workspace

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestPaneGeometry(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	v := h.view()
	d := v.Display
	if d.Area != paneArea || d.Compact || d.Focus != "p11" || d.SplitsDisabled != "" {
		t.Fatalf("display: area %v compact %v focus %s", d.Area, d.Compact, d.Focus)
	}
	want := layout.Frames(v.Tabs[1].Root, paneArea, 8)
	if len(d.Panes) != len(want) {
		t.Fatalf("panes: %d, want %d", len(d.Panes), len(want))
	}
	for i, p := range d.Panes {
		if p.ID != want[i].ID || p.Rect != want[i].Rect || p.Body != layout.Body(want[i].Rect) {
			t.Errorf("pane %d: %+v, want %+v", i, p, want[i])
		}
	}
	if d.Panes[0].Rect != (layout.Rect{X: 8, Y: 48, W: 1651, H: 1324}) {
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
	h.terms.snaps["w5"] = []byte("$ ls\r\n")
	snap, err := h.e.TermSnapshot("w5")
	mustOK(t, err)
	if b, _ := base64.StdEncoding.DecodeString(snap); string(b) != "$ ls\r\n" {
		t.Errorf("snapshot: %q", b)
	}
	if _, err := h.e.TermSnapshot("w3"); err == nil {
		t.Errorf("TermSnapshot of an App item should fail")
	}

	// Output is batched per item, at most once a frame.
	h.terms.cb.Data("w5", []byte("one "))
	h.terms.cb.Data("w5", []byte("two"))
	h.clk.Advance(frame)
	h.em.mu.Lock()
	data := slices.Clone(h.em.data)
	h.em.mu.Unlock()
	if len(data) != 1 || data[0].ID != "w5" {
		t.Fatalf("term:data: %+v", data)
	}
	if b, _ := base64.StdEncoding.DecodeString(data[0].Data); string(b) != "one two" {
		t.Errorf("batched output: %q", b)
	}

	// A title from the session becomes the item's.
	h.terms.cb.Title("w5", "vim — ~/code")
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

// fakeServer is a VSCodeServer.
type fakeServer struct{ stopped bool }

func (f *fakeServer) URL(folder string) string {
	return "http://localhost:9999/?tkn=t&folder=" + folder
}
func (f *fakeServer) Stop() error { f.stopped = true; return nil }

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
	if !srv.stopped {
		t.Errorf("Close should stop the server")
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
		{monitor, "auto", 1, "Wide layout, chosen for a 2,560-pt screen"},
		{laptop, "auto", 1, "Compact layout (stack panes), chosen for a 1,512-pt screen"},
		{laptop, "auto", 0, "Compact layout (one pane, every window as a sub-tab), chosen for a 1,512-pt screen"},
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
	cfg := strings.Replace(string(config.DefaultTOML()), `small_screens = "stack"`, `small_screens = "same"`, 1)
	h := newHarness(t, setup{screen: laptop, config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	if got := h.view().SizeReason; got != "Compact layout (keep the layout), chosen for a 1,512-pt screen" {
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
	if !d.Compact || d.Focus != layout.CompactPaneID || d.SplitsDisabled != "Splits are hidden on small screens" {
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
	mustErr(t, h.e.MoveItem("w7", 2, "compact", "right"), "Splits are hidden on small screens.")
	mustErr(t, h.e.MoveItem("w7", 2, "p14", "left"), "Splits are hidden on small screens.")
	mustErr(t, h.e.MoveDirection("left"), "Splits are hidden on small screens.")
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
	frames := layout.Frames(h.e.View().Tabs[1].Root, monitor.Visible, 8)
	body := layout.Body(frames[0].Rect)
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
	want := []string{`HideWebView("w1")`, `HideWebView("w2")`, `ShowWebView({9 90 1138 1281} "w1")`, `ShowWebView({1157 90 1394 1281} "w2")`}
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
