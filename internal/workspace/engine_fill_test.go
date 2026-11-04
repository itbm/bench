package workspace

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Two displays for the tests of filling the screen: the monitor, which is
// primary, and a studio display to its right.
var (
	studio = platform.Screen{ID: "studio", Name: "Studio Display",
		Frame: layout.Rect{X: 2560, W: 1512, H: 982}, Visible: layout.Rect{X: 2560, Y: 25, W: 1512, H: 957}}
	// smallFrame is Bench's content area at its usual size, on the monitor.
	smallFrame = layout.Rect{X: 560, Y: 295, W: 1440, H: 872}
)

// moveBench puts Bench's window's content area at r, and says so, as
// macOS would.
func (h *harness) moveBench(r layout.Rect) {
	h.p.SetContentFrame(r)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.drain()
}

// changeScreens replaces the displays, and moves Bench's window's content
// area to r, as macOS does when a display is plugged in or out, and lets the
// screens settle.
func (h *harness) changeScreens(r layout.Rect, screens ...platform.Screen) {
	h.p.SetScreens(screens...)
	h.p.SetContentFrame(r)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.settle()
}

func TestFindScreen(t *testing.T) {
	side := platform.Screen{ID: "7", Name: "DELL U2723QE", Frame: layout.Rect{X: 2560, W: 2560, H: 1440}}
	primary := platform.Screen{ID: "1", Name: "Built-in Retina Display", Primary: true, Frame: layout.Rect{W: 1512, H: 982}}
	twin := platform.Screen{ID: "9", Name: "DELL U2723QE", Frame: layout.Rect{X: -2560, W: 2560, H: 1440}}
	screens := []platform.Screen{primary, side, twin}
	tests := []struct {
		name  string
		saved *config.ScreenState
		want  string
	}{
		{"nothing saved", nil, "1"},
		{"by ID", &config.ScreenState{ID: "9", Name: "DELL U2723QE", Frame: twin.Frame}, "9"},
		// macOS can give a display a new ID after a restart.
		{"by name", &config.ScreenState{ID: "4", Name: "DELL U2723QE", Frame: layout.Rect{X: 9000}}, "7"},
		// Two displays of the same model have the same name: the frame
		// picks the one Bench was on.
		{"by name and frame", &config.ScreenState{ID: "4", Name: "DELL U2723QE", Frame: twin.Frame}, "9"},
		{"by frame", &config.ScreenState{ID: "4", Name: "Projector", Frame: twin.Frame}, "9"},
		{"gone", &config.ScreenState{ID: "4", Name: "Projector", Frame: layout.Rect{X: 9000, W: 800, H: 600}}, "1"},
	}
	for _, tc := range tests {
		if got := findScreen(screens, tc.saved); got.ID != tc.want {
			t.Errorf("%s: found %q, want %q", tc.name, got.ID, tc.want)
		}
	}
	if got := findScreen([]platform.Screen{side, twin}, nil); got.ID != "7" {
		t.Errorf("with no primary screen: found %q, want the first, 7", got.ID)
	}
	if got := findScreen(nil, &config.ScreenState{ID: "7"}); got.ID != "" {
		t.Errorf("with no screens: found %q, want none", got.ID)
	}
}

func TestFills(t *testing.T) {
	visible := monitor.Visible
	tests := []struct {
		name    string
		content layout.Rect
		want    bool
	}{
		{"exactly", visible, true},
		// Wails' web view sticks out 1 pt above and to the right.
		{"web view", layout.Rect{Y: 24, W: 2561, H: 1416}, true},
		{"2 pt short", layout.Rect{X: 2, Y: 25, W: 2556, H: 1413}, true},
		{"3 pt short", layout.Rect{Y: 25, W: 2557, H: 1415}, false},
		{"usual size", smallFrame, false},
		{"tiled to the left half", layout.Rect{Y: 25, W: 1280, H: 1415}, false},
		{"no window", layout.Rect{}, false},
	}
	for _, tc := range tests {
		if got := fills(tc.content, visible); got != tc.want {
			t.Errorf("%s: fills(%v) = %v, want %v", tc.name, tc.content, got, tc.want)
		}
	}
	if fills(visible, layout.Rect{}) {
		t.Error("a window fills a screen with no visible area")
	}
}

func TestWindowStart(t *testing.T) {
	h := newHarness(t, setup{})
	centred := layout.Rect{X: 560, Y: 282.5, W: 1440, H: 900}
	if got := h.e.WindowStart(); got.Screen.ID != "monitor" || !got.Fill || got.Frame != centred {
		t.Errorf("with nothing saved: %+v, want the primary screen, filled, from %v", got, centred)
	}

	h = newHarness(t, setup{config: strings.Replace(string(config.ExampleTOML()),
		"fill_screen = true", "fill_screen = false", 1)})
	// The usual size is narrower than compact_below, so even on the
	// monitor, the window opens with the compact layout's title bar.
	if got := h.e.WindowStart(); got.Fill || got.Frame != centred || !got.CompactTitleBar {
		t.Errorf("with fill_screen = false: %+v, want not filled, from %v, with the compact layout's title bar", got, centred)
	}
}

// TestOpenFrame checks where Bench's window opens on a screen: where the
// user left it, if it's still on that screen, or else at its usual size in
// the middle of the screen's visible area, or no bigger than it.
func TestOpenFrame(t *testing.T) {
	half := layout.Rect{Y: 25, W: 1280, H: 1415}
	saved := func(window layout.Rect) *config.ScreenState {
		return &config.ScreenState{ID: "monitor", Name: "Monitor", Frame: monitor.Frame, Window: window}
	}
	small := platform.Screen{ID: "small", Frame: layout.Rect{W: 1280.5, H: 800}, Visible: layout.Rect{Y: 25, W: 1280.5, H: 775}}
	moved := monitor
	moved.Frame.X, moved.Visible.X = -2560, -2560
	tests := []struct {
		name   string
		screen platform.Screen
		saved  *config.ScreenState
		want   layout.Rect
		left   bool
	}{
		{"nothing saved", monitor, nil, layout.Rect{X: 560, Y: 282.5, W: 1440, H: 900}, false},
		{"filled last time", monitor, saved(layout.Rect{}), layout.Rect{X: 560, Y: 282.5, W: 1440, H: 900}, false},
		{"tiled into the left half", monitor, saved(half), half, true},
		// Wails' web view sticks out 1 pt above the window.
		{"at the top", monitor, saved(layout.Rect{Y: 24, W: 1281, H: 1416}), layout.Rect{Y: 24, W: 1281, H: 1416}, true},
		{"off the screen", monitor, saved(layout.Rect{X: 2000, Y: 25, W: 1280, H: 1415}), layout.Rect{X: 560, Y: 282.5, W: 1440, H: 900}, false},
		// The display is somewhere else among the displays now, so the
		// saved place is somewhere else on it.
		{"display moved", moved, saved(half), layout.Rect{X: -2000, Y: 282.5, W: 1440, H: 900}, false},
		// A display smaller than the window's usual size.
		{"small screen", small, nil, layout.Rect{X: 0.25, Y: 25, W: 1280, H: 775}, false},
		{"no screen", platform.Screen{}, saved(half), layout.Rect{W: 1440, H: 900}, false},
	}
	for _, tc := range tests {
		got, left := openFrame(tc.screen, tc.saved)
		if got != tc.want || left != tc.left {
			t.Errorf("%s: %v, where it was left %v; want %v, %v", tc.name, got, left, tc.want, tc.left)
		}
	}
}

// TestBenchOpensWhereTheUserLeftIt checks that state.json records where
// Bench's window was when it didn't fill its screen, as when the user tiled
// it into half the screen, and that Bench opens there next time, without
// filling the screen, though fill_screen is on, and fills it again once
// the window filled the screen when Bench quit.
func TestBenchOpensWhereTheUserLeftIt(t *testing.T) {
	h := newHarness(t, setup{})
	half := layout.Rect{Y: 25, W: 1280, H: 1415}
	h.moveBench(half)
	h.settle()
	h.advance(retitleSaveDelay)
	st, _, err := config.LoadState(filepath.Join(h.dir, config.StateFile), savedConfig(t, h))
	mustOK(t, err)
	if st.Screen == nil || st.Screen.ID != "monitor" || st.Screen.Window != half {
		t.Fatalf("state.json's screen = %+v, want the monitor, with the window at %v", st.Screen, half)
	}

	h.restart()
	got := h.e.WindowStart()
	if got.Screen.ID != "monitor" || got.Frame != half || got.Fill {
		t.Errorf("after a restart: %+v, want the monitor, at %v, not filled", got, half)
	}
	// The window is narrow, so its title bar is the compact layout's.
	if !got.CompactTitleBar {
		t.Error("a window 1,280 pt wide opens with the wide layout's title bar")
	}

	// The user fills the screen: the place is forgotten.
	h.moveBench(monitor.Visible)
	h.settle()
	h.advance(retitleSaveDelay)
	st, _, err = config.LoadState(filepath.Join(h.dir, config.StateFile), savedConfig(t, h))
	mustOK(t, err)
	if st.Screen == nil || st.Screen.Window != (layout.Rect{}) {
		t.Fatalf("state.json's screen = %+v, want no window place for a window that fills the screen", st.Screen)
	}
	h.restart()
	if got := h.e.WindowStart(); !got.Fill || got.CompactTitleBar {
		t.Errorf("after filling the screen: %+v, want it filled, with the wide layout's title bar", got)
	}
}

// TestBenchOpensOnTheScreenItWasLastOn checks that state.json records the
// display that Bench's window was on, and that Bench opens there next time,
// or on the primary display if it's gone.
func TestBenchOpensOnTheScreenItWasLastOn(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.SetScreens(monitor, studio)
	h.moveBench(studio.Visible)
	h.settle()
	h.advance(retitleSaveDelay)
	st, _, err := config.LoadState(filepath.Join(h.dir, config.StateFile), savedConfig(t, h))
	mustOK(t, err)
	want := config.ScreenState{ID: "studio", Name: "Studio Display", Frame: studio.Frame}
	if st.Screen == nil || *st.Screen != want {
		t.Fatalf("state.json's screen = %+v, want %+v", st.Screen, want)
	}

	h.restart()
	if got := h.e.WindowStart(); got.Screen.ID != "studio" || !got.Fill {
		t.Errorf("after a restart: %+v, want the studio display, filled", got)
	}
	// The display is unplugged while Bench isn't running.
	h.p.SetScreens(monitor)
	if got := h.e.WindowStart(); got.Screen.ID != "monitor" {
		t.Errorf("with the studio display gone: %q, want the primary screen", got.Screen.ID)
	}
	if h.em.filled() != 0 {
		t.Errorf("Bench asked to fill the screen %d times, want none", h.em.filled())
	}
}

// TestBenchFillsTheScreenAgainAfterTheScreensChange checks that Bench's
// window fills the screen again when macOS moves it, as when the display
// it filled is unplugged, but only once, and not when it didn't fill the
// screen before.
func TestBenchFillsTheScreenAgainAfterTheScreensChange(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.SetScreens(monitor, studio)
	h.moveBench(studio.Visible)
	h.settle()
	if h.em.filled() != 0 {
		t.Fatalf("Bench asked to fill a screen it fills")
	}

	// The studio display is unplugged, and macOS moves Bench's window onto
	// the monitor, at the same size.
	h.changeScreens(layout.Rect{X: 100, Y: 25, W: 1512, H: 957}, monitor)
	if got := h.em.filled(); got != 1 {
		t.Fatalf("after the display went: asked to fill the screen %d times, want once", got)
	}
	// The shell zooms the window.
	h.moveBench(monitor.Visible)
	h.settle()
	if got := h.em.filled(); got != 1 {
		t.Errorf("after zooming: asked to fill the screen %d times, want once", got)
	}
	if !h.e.filled {
		t.Error("Bench doesn't know that its window fills the screen")
	}

	// The studio display is plugged in again; Bench stays on the monitor,
	// which it still fills.
	h.changeScreens(monitor.Visible, monitor, studio)
	if got := h.em.filled(); got != 1 {
		t.Errorf("after the display came back: asked to fill the screen %d times, want once", got)
	}

	// The user makes the window smaller, then the resolution changes.
	h.moveBench(smallFrame)
	scaled := platform.Screen{ID: "monitor", Name: "Monitor", Primary: true,
		Frame: layout.Rect{W: 1920, H: 1080}, Visible: layout.Rect{Y: 25, W: 1920, H: 1055}}
	h.changeScreens(layout.Rect{X: 240, Y: 120, W: 1440, H: 872}, scaled, studio)
	if got := h.em.filled(); got != 1 {
		t.Errorf("a window the user made smaller: asked to fill the screen %d times, want once", got)
	}
}

// TestBenchFillsTheScreenAgainAfterAResolutionChange checks that a window
// that filled the screen fills it again when the display's resolution
// changes, though it stays on the same display, and that moves while the
// screens settle don't count as the user making it smaller.
func TestBenchFillsTheScreenAgainAfterAResolutionChange(t *testing.T) {
	h := newHarness(t, setup{})
	h.moveBench(monitor.Visible)
	scaled := platform.Screen{ID: "monitor", Name: "Monitor", Primary: true,
		Frame: layout.Rect{W: 3008, H: 1692}, Visible: layout.Rect{Y: 25, W: 3008, H: 1667}}
	h.p.SetScreens(scaled)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.advance(500 * time.Millisecond)
	// macOS moves the window as the screens settle.
	h.moveBench(layout.Rect{X: 10, Y: 30, W: 2560, H: 1415})
	h.settle()
	if got := h.em.filled(); got != 1 {
		t.Errorf("asked to fill the screen %d times, want once", got)
	}
}

// TestBenchFillsTheScreenAgainWhenTheDockChanges checks that a window that
// filled the screen fills its new visible area when the Dock grows, though
// the display stays the same, and that Bench's window moving while the
// screens settle doesn't count as the user making it smaller.
func TestBenchFillsTheScreenAgainWhenTheDockChanges(t *testing.T) {
	h := newHarness(t, setup{})
	h.moveBench(monitor.Visible)
	docked := monitor
	docked.Visible = layout.Rect{Y: 25, W: 2560, H: 1340}
	h.p.SetScreens(docked)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.advance(500 * time.Millisecond)
	h.moveBench(monitor.Visible)
	if got := h.em.filled(); got != 0 {
		t.Errorf("asked to fill the screen %d times before it settled, want none", got)
	}
	h.settle()
	if got := h.em.filled(); got != 1 {
		t.Errorf("asked to fill the screen %d times, want once", got)
	}
}

func TestFillScreen(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.FillScreen())
	if got := h.em.filled(); got != 1 {
		t.Errorf("FillScreen asked the shell %d times, want once", got)
	}

	for name, o := range map[string]func(*Options){
		"preview":  func(o *Options) { o.Demo = true },
		"headless": func(o *Options) { o.Headless = true },
	} {
		h := newHarness(t, setup{options: o})
		mustErr(t, h.e.FillScreen(), "This build of Bench has no window to fill the screen with.")
		h.moveBench(layout.Rect{X: 100, Y: 100, W: 1440, H: 900})
		h.changeScreens(layout.Rect{X: 100, Y: 100, W: 1440, H: 900}, laptop)
		if h.em.filled() != 0 {
			t.Errorf("%s: asked the shell to fill the screen", name)
		}
		if h.e.lastScreen != nil {
			t.Errorf("%s: recorded the screen %+v", name, h.e.lastScreen)
		}
	}
}

// TestBenchFillsTheScreenAgainOnceShown checks that a display change while
// Bench's window is hidden or minimised fills the screen again only once the
// window is back, since zooming it could bring it back.
func TestBenchFillsTheScreenAgainOnceShown(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.SetScreens(monitor, studio)
	h.moveBench(studio.Visible)
	h.settle()
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.changeScreens(layout.Rect{X: 100, Y: 25, W: 1512, H: 957}, monitor)
	// macOS moves the hidden window again, on the same screen.
	h.moveBench(layout.Rect{X: 120, Y: 25, W: 1512, H: 957})
	h.settle()
	if got := h.em.filled(); got != 0 {
		t.Fatalf("asked to fill the screen %d times while Bench was hidden, want none", got)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if got := h.em.filled(); got != 1 {
		t.Errorf("asked to fill the screen %d times once Bench was shown, want once", got)
	}
	h.moveBench(monitor.Visible)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.settle()
	if got := h.em.filled(); got != 1 {
		t.Errorf("hiding and showing Bench again: asked to fill the screen %d times, want once", got)
	}
}
