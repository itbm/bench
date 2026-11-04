package workspace

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/windows"
)

// These tests follow the "done when" criteria of the spec's milestones, as
// far as they can be checked without a Mac.

// terminalID is macOS Terminal, used as an App item in milestone 3.
const terminalID = "com.apple.Terminal"

// arrangeConfig is milestone 3's test config: Terminal as an App item in
// place of the built-in terminal, and a Brave window in Code as well as in
// Research, so that Research's Brave windows are parked rather than hidden.
const arrangeConfig = `
[settings]
compact_below = 1800 # The laptop is compact, as in the example config.

[[tab]]
name = "Code"
layout = "main-side"
small_screens = "stack"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]
  [[tab.pane]]
  items = [{ app = "com.apple.Terminal" }, { app = "com.brave.Browser", title = "localhost" }]

[[tab]]
name = "Research"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
`

// arrangeWorld is milestone 3's world: every app is running with a window.
func arrangeWorld(p *platform.Fake) {
	p.AddApp(zedID, "Zed")
	p.AddWindow(zedID, "bench — switch.go")
	p.AddApp(vscodeID, "VS Code")
	p.AddWindow(vscodeID, "index.html — bench-ui")
	p.AddApp(terminalID, "Terminal")
	p.AddWindow(terminalID, "zsh — 80×24")
	p.AddApp(braveID, "Brave")
	p.AddWindow(braveID, "localhost:3000")
	p.AddWindow(braveID, "Wails v3 docs")
	p.AddApp(obsidianID, "Obsidian")
	p.AddWindow(obsidianID, "Daily note")
}

// Milestone 3: bench arrange Code docks Zed, VS Code, and Terminal correctly
// on a monitor and a laptop, keeps VS Code stacked behind Zed in the same
// frame, parks a Brave window that belongs to another tab, and puts
// everything back on quit.
func TestMilestone3BenchArrange(t *testing.T) {
	tests := []struct {
		name   string
		screen platform.Screen
		class  layout.SizeClass
		header float64 // The density's: compact on the laptop.
	}{
		{"monitor", monitor, layout.Wide, layout.HeaderHeight},
		{"laptop", laptop, layout.Compact, layout.CompactHeaderHeight},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{noStart: true, config: arrangeConfig, world: arrangeWorld, screen: tc.screen,
				options: func(o *Options) { o.Headless = true }})
			h.p.SetContentFrame(layout.Rect{})
			docs := h.windowID(braveID, "Wails v3 docs")
			docsBefore := h.frame(docs)

			done := make(chan error, 1)
			go func() { done <- h.e.ArrangeOnce(context.Background(), "Code") }()
			h.waitFor(done)

			root := layout.Adapt(h.view().Tabs[0].Root, layout.Stack, tc.class, "", "")
			frames := layout.Frames(root, tc.screen.Visible, 4)
			main, side := layout.Body(frames[0].Rect, tc.header), layout.Body(frames[1].Rect, tc.header)
			zed := h.windowID(zedID, "bench — switch.go")
			code := h.windowID(vscodeID, "index.html — bench-ui")
			term := h.windowID(terminalID, "zsh — 80×24")
			local := h.windowID(braveID, "localhost:3000")
			for _, w := range []struct {
				name string
				id   platform.WindowID
				want layout.Rect
			}{
				{"Zed", zed, main}, {"VS Code", code, main}, {"Terminal", term, side}, {"Brave localhost", local, side},
			} {
				if got := h.frame(w.id); got != w.want {
					t.Errorf("%s: %v, want the pane body %v", w.name, got, w.want)
				}
			}
			z := h.p.ZOrder()
			if slices.Index(z, zed) > slices.Index(z, code) {
				t.Errorf("VS Code should be stacked behind Zed: z-order %v", z)
			}
			if slices.Index(z, term) > slices.Index(z, local) {
				t.Errorf("each pane shows its first window: z-order %v", z)
			}
			if !h.e.win.IsParked(docs) || windows.OnScreen(h.frame(docs), h.p.Screens()) {
				t.Errorf("Research's Brave window should be parked: %v", h.frame(docs))
			}
			if !h.p.IsHidden(obsidianID) {
				t.Errorf("Obsidian has items only in Research, so it should be hidden")
			}

			mustOK(t, h.e.Close())
			if h.frame(docs) != docsBefore || h.p.IsHidden(obsidianID) {
				t.Errorf("quitting should put everything back: Brave %v (was %v), Obsidian hidden %v",
					h.frame(docs), docsBefore, h.p.IsHidden(obsidianID))
			}
		})
	}
}

// restart closes the engine and starts another on the same platform, clock,
// and support folder, as when the user quits and reopens Bench while the
// other apps keep running.
func (h *harness) restart() {
	h.t.Helper()
	// The old run handles its own events first; the new one mustn't see
	// them.
	h.drain()
	mustOK(h.t, h.e.Close())
	e, err := New(Options{
		Platform: h.p, Clock: h.clk, Emitter: h.em, Dir: h.dir,
		NewTerms: func(cb term.Callbacks) term.Host { h.terms.cb = cb; return h.terms },
	})
	if err != nil {
		h.t.Fatalf("New: %v", err)
	}
	e.manual = true
	e.ownPID = h.bench.PID
	h.e = e
	h.t.Cleanup(func() { _ = e.Close() })
	area := paneArea
	content := h.p.ContentFrame()
	area.W, area.H = content.W, content.H-75
	mustOK(h.t, e.SetPaneArea(area))
	h.start()
}

// Milestone 5: drag one of three Brave windows to its pane's right edge and
// see the two side by side; move it to another tab by dropping it on the
// tab; and quit and reopen Bench with the arrangement intact.
func TestMilestone5MoveAndReopen(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	pr := h.windowID(braveID, "Pull request #42 · bench")
	docs := h.windowID(braveID, "Wails v3 docs")

	// Drag w17 (the pull request) onto p14's right edge.
	mustOK(t, h.e.MoveItem("w17", 2, "p14", "right"))
	newPane := h.paneOf("w17")
	if newPane == "p14" {
		t.Fatalf("w17 should be in a new pane")
	}
	left, right := h.bodyOf("p14"), h.bodyOf(newPane)
	if h.frame(docs) != left || h.frame(pr) != right || right.X <= left.X {
		t.Errorf("side by side: docs %v (pane %v), pull request %v (pane %v)", h.frame(docs), left, h.frame(pr), right)
	}

	// Drop the docs window on the Code tab.
	mustOK(t, h.e.MoveItemToTab("w6", 1))
	if v := h.view(); v.Current != 2 || v.Items["w6"].Tab != 1 {
		t.Errorf("moving to a tab doesn't switch: current %d, w6 in tab %d", v.Current, v.Items["w6"].Tab)
	}
	if !h.e.win.IsParked(docs) {
		t.Errorf("w6 now belongs to Code, so it should be parked while Research is shown")
	}

	before := h.view()
	beforeFrames := map[platform.WindowID]layout.Rect{pr: h.frame(pr)}
	h.restart()
	after := h.view()
	if after.Current != before.Current {
		t.Errorf("current tab: %d, want %d", after.Current, before.Current)
	}
	for i := range before.Tabs {
		if !layoutEqual(before.Tabs[i].Root, after.Tabs[i].Root) || before.Tabs[i].Preset != after.Tabs[i].Preset {
			t.Errorf("%s changed when Bench reopened", before.Tabs[i].Name)
		}
	}
	for id, w := range map[string]platform.WindowID{"w17": pr, "w6": docs} {
		if got := h.itemOf(w); got != id {
			t.Errorf("window %d: item %q, want %s", w, got, id)
		}
	}
	if h.frame(pr) != beforeFrames[pr] {
		t.Errorf("the pull request moved: %v, was %v", h.frame(pr), beforeFrames[pr])
	}
	if !h.e.win.IsParked(docs) {
		t.Errorf("w6 is still Code's, so it should be parked again")
	}
	mustOK(t, h.e.SwitchTab(1))
	if h.frame(docs) != h.bodyOf(h.paneOf("w6")) {
		t.Errorf("w6 should be docked in Code: %v", h.frame(docs))
	}
}

// layoutEqual compares two trees.
func layoutEqual(a, b *layout.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.ID != b.ID || a.Dir != b.Dir || a.Active != b.Active ||
		!slices.Equal(a.Sizes, b.Sizes) || !slices.Equal(a.Items, b.Items) || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !layoutEqual(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

// Milestone 8: unplugging the monitor moves the current tab to its compact
// layout within 2 seconds, without changing focus.
func TestMilestone8UnplugMonitor(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.FocusPane("p12"))
	h.advance(time.Second)
	h.p.ResetCalls()

	h.p.SetScreens(laptop)
	h.p.SetContentFrame(laptop.Visible)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	// Bench's window shrinks to the laptop, and the frontend reports its
	// new pane area.
	area := paneArea
	area.W, area.H = laptop.Visible.W, laptop.Visible.H-75
	mustOK(t, h.e.SetPaneArea(area))
	h.advance(2 * time.Second)

	v := h.view()
	if v.SizeClass != string(layout.Compact) || v.Display.Root.Dir != layout.Column {
		t.Fatalf("after 2 s: %s, %s", v.SizeClass, v.Display.Root.Dir)
	}
	zed := h.windowID(zedID, "bench — switch.go")
	if body := h.bodyOf("p11"); h.frame(zed) != body || body.W != laptop.Visible.W-2*4-2 {
		t.Errorf("Zed %v, want the stacked pane's body %v", h.frame(zed), body)
	}
	if got := h.focusCalls(); len(got) != 0 {
		t.Errorf("focus changed: %v", got)
	}
	if v.Display.Focus != "p12" {
		t.Errorf("the focused pane changed: %s", v.Display.Focus)
	}
}
