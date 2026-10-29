package workspace

import (
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// paneView returns a displayed pane, failing the test if it isn't shown.
func (h *harness) paneView(id string) PaneView {
	h.t.Helper()
	for _, p := range h.view().Display.Panes {
		if p.ID == id {
			return p
		}
	}
	h.t.Fatalf("pane %s isn't displayed", id)
	return PaneView{}
}

// saveSetting changes the settings with change and saves them.
func (h *harness) saveSetting(change func(s *SettingsView)) {
	h.t.Helper()
	s := h.view().Settings
	change(&s)
	mustOK(h.t, h.e.SaveSettings(s))
	h.drain()
}

// TestDensityHeaderHeights checks the pane headers' height in each
// density: 40 pt in the standard density, which a wide screen has by
// default, and 32 pt in the compact density, which the compact size class
// has by default; and that the docked windows follow the bodies below
// them.
func TestDensityHeaderHeights(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	check := func(when, density string, header float64) {
		t.Helper()
		v := h.view()
		if v.Density != density || v.TitleBar != density {
			t.Errorf("%s: density %q, title bar %q; want %q", when, v.Density, v.TitleBar, density)
		}
		for _, p := range v.Display.Panes {
			if p.Header != header || p.Body != layout.Body(p.Rect, header) {
				t.Errorf("%s: pane %s header %v, body %v; want %v, %v", when, p.ID, p.Header, p.Body, header, layout.Body(p.Rect, header))
			}
		}
		if got, want := h.frame(zed), h.bodyOf("p11"); got != want {
			t.Errorf("%s: Zed at %v, want the pane body %v", when, got, want)
		}
	}
	check("a wide screen", config.DensityStandard, layout.HeaderHeight)

	h.saveSetting(func(s *SettingsView) { s.Density = config.DensityCompact })
	check("compact by hand", config.DensityCompact, layout.CompactHeaderHeight)

	// Auto is compact in the compact size class, here chosen by hand.
	h.saveSetting(func(s *SettingsView) { s.Density = config.DensityAuto })
	mustOK(t, h.e.SetMode("compact"))
	check("the compact layout", config.DensityCompact, layout.CompactHeaderHeight)

	h.saveSetting(func(s *SettingsView) { s.Density = config.DensityStandard })
	check("standard by hand in the compact layout", config.DensityStandard, layout.HeaderHeight)
}

// TestDensityOnALaptop checks that Auto chooses the compact density on a
// small screen.
func TestDensityOnALaptop(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(1))
	if v := h.view(); v.Density != config.DensityCompact || v.Display.Panes[0].Header != layout.CompactHeaderHeight {
		t.Errorf("laptop: density %q, header %v; want compact, %v", v.Density, v.Display.Panes[0].Header, layout.CompactHeaderHeight)
	}
}

// TestTitleBarKeepsItsLaunchDensity checks that, once the shell has made the
// window's title bar (WindowStart), the View's title bar keeps that density,
// while the panes' headers follow the setting.
func TestTitleBarKeepsItsLaunchDensity(t *testing.T) {
	h := newHarness(t, setup{noStart: true, screen: laptop})
	if start := h.e.WindowStart(); !start.CompactTitleBar {
		t.Error("WindowStart on a laptop: the title bar isn't compact")
	}
	h.start()
	mustOK(t, h.e.SwitchTab(1))
	h.saveSetting(func(s *SettingsView) { s.Density = config.DensityStandard })
	if v := h.view(); v.TitleBar != config.DensityCompact || v.Density != config.DensityStandard || v.Display.Panes[0].Header != layout.HeaderHeight {
		t.Errorf("title bar %q, density %q, header %v; want compact, standard, %v", v.TitleBar, v.Density, v.Display.Panes[0].Header, layout.HeaderHeight)
	}

	h = newHarness(t, setup{noStart: true})
	if start := h.e.WindowStart(); start.CompactTitleBar {
		t.Error("WindowStart on a monitor: the title bar is compact")
	}
	h = newHarness(t, setup{noStart: true, config: "[settings]\ndensity = \"compact\"\n\n[[tab]]\nname = \"Code\"\nlayout = \"single\"\n\n[[tab.pane]]\nitems = [{ app = \"dev.zed.Zed\" }]\n"})
	if start := h.e.WindowStart(); !start.CompactTitleBar {
		t.Error("WindowStart with density compact: the title bar isn't compact")
	}
}

// TestTabsViewPaneHasNoHeader checks that the "tabs" view's one pane, whose
// sub-tabs are in the title bar, has no header and reaches up over the gap
// above it, with its windows docked in the taller body.
func TestTabsViewPaneHasNoHeader(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	v := h.view()
	d := v.Display
	if !d.Compact || len(d.Panes) != 1 {
		t.Fatalf("display: compact %v, %d panes", d.Compact, len(d.Panes))
	}
	p := d.Panes[0]
	frames := layout.Frames(d.Root, d.Area, v.Settings.Gap)
	want := frames[0].Rect
	want.H += want.Y - d.Area.Y
	want.Y = d.Area.Y
	if p.Header != 0 || p.Rect != want || p.Body != layout.Body(want, 0) {
		t.Errorf("compact pane: header %v, rect %v, body %v; want 0, %v, %v", p.Header, p.Rect, p.Body, want, layout.Body(want, 0))
	}
	if p.Body.Y != d.Area.Y+layout.PaneBorder {
		t.Errorf("compact pane's body starts at %v, want just inside its border at the top of the area, %v", p.Body.Y, d.Area.Y+layout.PaneBorder)
	}
	note := h.windowID(obsidianID, "Daily note")
	if got := h.frame(note); got != h.bodyOf(layout.CompactPaneID) {
		t.Errorf("Obsidian at %v, want the compact pane's body %v", got, h.bodyOf(layout.CompactPaneID))
	}
	// pane_headers has nothing to add: the pane has none already.
	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
	if got := h.paneView(layout.CompactPaneID); got.Header != 0 || got.Rect != want {
		t.Errorf("compact pane with pane_headers auto: header %v, rect %v", got.Header, got.Rect)
	}
}

// TestPaneHeadersAuto checks pane_headers = "auto": a pane that shows one
// App window goes without its header, and its window fills the pane; a
// second window brings the header back at once; and the header goes again
// only once the pane has shown one window for headerHideDelay.
func TestPaneHeadersAuto(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	note := h.windowID(obsidianID, "Daily note")
	docs := h.windowID(braveID, "Wails v3 docs")
	noteID, docsID := h.itemOf(note), h.itemOf(docs)
	side, main := h.paneOf(noteID), h.paneOf(docsID)
	if got := h.paneView(side); got.Header != layout.HeaderHeight {
		t.Fatalf("with pane_headers always, Obsidian's pane has header %v", got.Header)
	}

	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
	p := h.paneView(side)
	if p.Header != 0 || p.Body != layout.Body(p.Rect, 0) {
		t.Errorf("one App window: header %v, body %v; want none, %v", p.Header, p.Body, layout.Body(p.Rect, 0))
	}
	if got, want := h.frame(note), h.bodyOf(side); got != want {
		t.Errorf("Obsidian at %v, want the pane's body without a header, %v", got, want)
	}
	// The Brave pane has three windows, so it keeps its header.
	if got := h.paneView(main); got.Header != layout.HeaderHeight {
		t.Errorf("three windows: header %v, want %v", got.Header, layout.HeaderHeight)
	}

	// A second window needs the sub-tabs at once.
	mustOK(t, h.e.MoveItem(docsID, 2, side, "center"))
	h.drain()
	if got := h.paneView(side); got.Header != layout.HeaderHeight {
		t.Errorf("two windows: header %v, want %v", got.Header, layout.HeaderHeight)
	}
	if got, want := h.frame(docs), h.bodyOf(side); got != want {
		t.Errorf("Brave at %v, want the body below the header, %v", got, want)
	}

	// Back to one: the header stays a while, so that windows moving
	// through don't make it come and go.
	mustOK(t, h.e.MoveItem(docsID, 2, main, "center"))
	h.advance(headerHideDelay - 100*time.Millisecond)
	if got := h.paneView(side); got.Header != layout.HeaderHeight {
		t.Errorf("one window again, just now: header %v, want %v", got.Header, layout.HeaderHeight)
	}
	h.advance(200 * time.Millisecond)
	if got := h.paneView(side); got.Header != 0 {
		t.Errorf("one window again, after %v: header %v, want none", headerHideDelay, got.Header)
	}
	if got, want := h.frame(note), h.bodyOf(side); got != want {
		t.Errorf("Obsidian at %v, want the body without a header again, %v", got, want)
	}

	// Always brings every header back.
	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAlways })
	if got := h.paneView(side); got.Header != layout.HeaderHeight {
		t.Errorf("pane_headers always again: header %v", got.Header)
	}
	if got, want := h.frame(note), h.bodyOf(side); got != want {
		t.Errorf("Obsidian at %v, want the body below the header, %v", got, want)
	}
}

// TestPaneHeadersAutoKeepsThemForPagesAndTerminals checks that a pane that
// shows one web page, or one terminal, keeps its header, its only bar.
func TestPaneHeadersAutoKeepsThemForPagesAndTerminals(t *testing.T) {
	h := newHarness(t, setup{})
	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
	for _, p := range h.view().Display.Panes { // Comms: Slack and Outlook, web pages.
		if p.Header != layout.HeaderHeight {
			t.Errorf("Comms pane %s: header %v, want %v", p.ID, p.Header, layout.HeaderHeight)
		}
	}
}

// TestPaneHeaderWaitsForAStatus checks that a status that comes and goes
// at once doesn't bring a pane's header back, and that one that lasts
// does, after headerShowDelay.
func TestPaneHeaderWaitsForAStatus(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
	note := h.windowID(obsidianID, "Daily note")
	side := h.paneOf(h.itemOf(note))
	headers := map[float64]bool{}
	watch := func(d time.Duration) {
		for ; d > 0; d -= 100 * time.Millisecond {
			h.advance(100 * time.Millisecond)
			headers[h.paneView(side).Header] = true
		}
	}

	if err := h.p.SetFullScreen(note, true); err != nil {
		t.Fatal(err)
	}
	watch(headerShowDelay / 2)
	if err := h.p.SetFullScreen(note, false); err != nil {
		t.Fatal(err)
	}
	watch(3 * time.Second)
	if headers[layout.HeaderHeight] {
		t.Error("a status that lasted half a second brought the header back")
	}

	if err := h.p.SetFullScreen(note, true); err != nil {
		t.Fatal(err)
	}
	h.advance(headerShowDelay - 100*time.Millisecond)
	if got := h.paneView(side).Header; got != 0 {
		t.Errorf("full screen for under %v: header %v, want none yet", headerShowDelay, got)
	}
	h.advance(200 * time.Millisecond)
	if got := h.item(h.itemOf(note)).Status; got != StatusFullScreen {
		t.Fatalf("status %q, want full screen", got)
	}
	if got := h.paneView(side).Header; got != layout.HeaderHeight {
		t.Errorf("full screen for %v: header %v, want %v", headerShowDelay, got, layout.HeaderHeight)
	}
}

// TestPaneHeaderNeedsRoomWithoutIt checks that a window that needs more room
// below a pane's header is tried in the taller body without one, and keeps
// the header only if it doesn't fit that either.
func TestPaneHeaderNeedsRoomWithoutIt(t *testing.T) {
	// Research's side pane on the monitor is 1,332 pt tall: its body is
	// 1,289 pt below a header, and 1,330 pt without one.
	const fitsWithout, tooTall = 1310, 1400
	for _, tc := range []struct {
		name      string
		minHeight float64
		header    float64
		status    string
	}{
		{"fits without a header", fitsWithout, 0, StatusReady},
		{"too tall either way", tooTall, layout.HeaderHeight, StatusNeedsRoom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{world: func(p *platform.Fake) {
				standardWorld(p)
				p.CloseWindow(obsidianWindow(p))
				p.AddWindow(obsidianID, "Daily note", platform.WithMinSize(400, tc.minHeight))
			}})
			mustOK(t, h.e.SwitchTab(2))
			note := h.windowID(obsidianID, "Daily note")
			id := h.itemOf(note)
			side := h.paneOf(id)
			if p := h.paneView(side); p.Rect.H != 1332 || h.item(id).Status != StatusNeedsRoom {
				t.Fatalf("with a header: pane height %v, status %q; want 1332, needs-room", p.Rect.H, h.item(id).Status)
			}

			h.saveSetting(func(s *SettingsView) { s.PaneHeaders = config.PaneHeadersAuto })
			h.advance(5 * time.Second)
			if got := h.paneView(side).Header; got != tc.header {
				t.Errorf("header %v, want %v", got, tc.header)
			}
			if got := h.item(id).Status; got != tc.status {
				t.Errorf("status %q, want %q", got, tc.status)
			}
			// A window that doesn't fit either way keeps the header for good,
			// rather than trying again and again.
			calls := len(h.calls("SetFrame"))
			h.advance(5 * time.Second)
			if got := h.paneView(side).Header; got != tc.header {
				t.Errorf("later: header %v, want %v", got, tc.header)
			}
			if got := len(h.calls("SetFrame")); got != calls {
				t.Errorf("later: %d more moves, want none", got-calls)
			}
		})
	}
}

// obsidianWindow returns the ID of Obsidian's window in the standard world.
func obsidianWindow(p *platform.Fake) platform.WindowID {
	app, _ := p.App(obsidianID)
	ws, _ := p.Windows(app)
	return ws[0].ID
}
