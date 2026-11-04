package workspace

import (
	"errors"
	"math"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has where Bench's own window opens, and filling the screen with
// it: Bench zooms its window to cover the screen's visible area, below the
// menu bar and beside the Dock, rather than use native full screen, which
// docked windows can't follow it into. It also has the notice while Stage
// Manager is on, which moves windows about as Bench can't.

// fillSlack is how far, in points, each edge of Bench's content area may be
// from the screen's visible area for the window to count as filling it.
// Wails' web view, whose frame is the content frame, sticks out 1 pt above
// and to the right of the window.
const fillSlack = 2

// errCantFill refuses FillScreen where Bench has no window of its own.
var errCantFill = errors.New("This build of Bench has no window to fill the screen with.")

// sourceStageManager is the source of the notice that Stage Manager is on.
const sourceStageManager = "stage-manager"

// stageManagerText is the notice while Stage Manager is on.
const stageManagerText = "Stage Manager is on, so Bench can't keep windows in their panes. Turn it off in Control Centre."

// The size that Bench's window opens at, in points, when it doesn't fill
// the screen and isn't where the user last left it (see openFrame). It's
// narrower than the default compact_below, so Auto makes a window at this
// size compact on any screen (decision 58).
const (
	UsualWidth  = 1440
	UsualHeight = 900
)

// WindowStart is where Bench's window opens: on the screen it was last on,
// if that's still connected, or else the primary screen; at Frame, its
// content area in global coordinates (see openFrame); whether it then fills
// that screen's visible area; whether its title bar is compact (the
// density, on that screen, at that size); and its appearance (the
// appearance setting), which the shell's other windows open with too until
// it hears of another (Emitter.Appearance).
type WindowStart struct {
	Screen          platform.Screen
	Frame           layout.Rect
	Fill            bool
	CompactTitleBar bool
	Appearance      string
}

// WindowStart returns where Bench's window opens. The shell asks once,
// before it opens the window. Its title bar is made then, and keeps the
// density it has until Bench opens again, as the View's TitleBar says.
//
// A window that the user left smaller than its screen, as when they tiled
// it into half the screen, opens where it was, at that size, if that's
// still on the screen. Otherwise it fills the screen with fill_screen on,
// or else opens at its usual size in the middle of the screen.
func (e *Engine) WindowStart() WindowStart {
	e.lock()
	defer e.unlock()
	s := findScreen(e.p.Screens(), e.lastScreen)
	frame, left := openFrame(s, e.lastScreen)
	fill := e.cfg.Settings.FillScreen && !left
	e.filled = fill && !e.opts.Headless && !e.opts.Demo
	e.startWidth = frame.W
	if fill {
		e.startWidth = s.Visible.W
	}
	class := layout.ChooseSizeClass(e.model.Mode, e.startWidth, e.cfg.Settings.CompactBelow)
	e.titleBar = densityFor(e.cfg.Settings.Density, class)
	return WindowStart{
		Screen:          s,
		Frame:           frame,
		Fill:            fill,
		CompactTitleBar: e.titleBar == config.DensityCompact,
		Appearance:      e.cfg.Settings.Appearance,
	}
}

// openFrame returns where Bench's window opens on a screen, s, as its
// content area in global coordinates: where the user left it, as saved in
// state.json, if it didn't fill its screen then, and that screen is s, at
// the same place among the displays, and the window lies inside s's
// visible area, give or take fillSlack on each edge, reporting that it's
// that (left); or else at its usual size, or the visible area's if that's
// smaller, in the middle of the visible area. A screen without a visible
// area, as when there are no screens at all, gives the usual size alone.
func openFrame(s platform.Screen, saved *config.ScreenState) (frame layout.Rect, left bool) {
	visible := s.Visible
	if visible.W <= 0 || visible.H <= 0 {
		return layout.Rect{W: UsualWidth, H: UsualHeight}, false
	}
	if saved != nil && saved.Frame == s.Frame && inside(saved.Window, visible) {
		return saved.Window, true
	}
	w, h := min(UsualWidth, math.Floor(visible.W)), min(UsualHeight, math.Floor(visible.H))
	return layout.Rect{X: visible.X + (visible.W-w)/2, Y: visible.Y + (visible.H-h)/2, W: w, H: h}, false
}

// inside reports whether Bench's content area lies inside a screen's
// visible area, give or take fillSlack on each edge.
func inside(content, visible layout.Rect) bool {
	return content.W > 0 && content.H > 0 &&
		content.X >= visible.X-fillSlack && content.Y >= visible.Y-fillSlack &&
		content.X+content.W <= visible.X+visible.W+fillSlack && content.Y+content.H <= visible.Y+visible.H+fillSlack
}

// FillScreen fills the screen with Bench's window, as View ▸ Fill the
// Screen does: the shell zooms the window, unless it fills the screen
// already. The browser preview refuses.
func (e *Engine) FillScreen() error {
	if e.opts.Demo || e.opts.Headless {
		return errCantFill
	}
	e.lock()
	defer e.unlock()
	e.later(e.em.FillScreen)
	return nil
}

// findScreen returns the screen that was saved, if it's still connected,
// found by its ID, or else its name and frame, or else its name, or else
// its frame; or else the primary screen. Two displays of the same model
// have the same name, so the frame tells them apart. With no screens at all
// it returns a zero Screen.
func findScreen(screens []platform.Screen, saved *config.ScreenState) platform.Screen {
	if saved != nil {
		sameName := func(s platform.Screen) bool { return saved.Name != "" && s.Name == saved.Name }
		for _, same := range []func(platform.Screen) bool{
			func(s platform.Screen) bool { return s.ID == saved.ID },
			func(s platform.Screen) bool { return sameName(s) && s.Frame == saved.Frame },
			sameName,
			func(s platform.Screen) bool { return s.Frame == saved.Frame },
		} {
			for _, s := range screens {
				if same(s) {
					return s
				}
			}
		}
	}
	for _, s := range screens {
		if s.Primary {
			return s
		}
	}
	if len(screens) > 0 {
		return screens[0]
	}
	return platform.Screen{}
}

// fills reports whether Bench's content area covers a screen's visible
// area, give or take fillSlack on each edge.
func fills(content, visible layout.Rect) bool {
	if content.W <= 0 || content.H <= 0 || visible.W <= 0 || visible.H <= 0 {
		return false
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= fillSlack }
	return near(content.X, visible.X) && near(content.Y, visible.Y) &&
		near(content.X+content.W, visible.X+visible.W) && near(content.Y+content.H, visible.Y+visible.H)
}

// noteBenchPlaceLocked records, while the screens are settled, the screen
// that Bench's window is on, s, for it to open there next time; whether the
// window fills that screen, for it to fill the screen again after the
// screens change (refillLocked); and, if it doesn't, where it is, for it to
// open there next time (WindowStart). Without a window of its own, as for
// bench arrange and in the browser preview, there's nothing to record.
func (e *Engine) noteBenchPlaceLocked(s platform.Screen) {
	if e.opts.Headless || e.opts.Demo || s.ID == "" {
		return
	}
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return
	}
	e.filled = fills(content, s.Visible)
	if e.filled {
		content = layout.Rect{}
	}
	e.noteScreenLocked(s, content)
}

// noteScreenLocked records the screen that Bench's window is on, and
// window, its content area, if it doesn't fill the screen, or else zero,
// saving them in state.json when they change.
func (e *Engine) noteScreenLocked(s platform.Screen, window layout.Rect) {
	now := &config.ScreenState{ID: s.ID, Name: s.Name, Frame: s.Frame, Window: window}
	if e.lastScreen != nil && *e.lastScreen == *now {
		return
	}
	e.lastScreen = now
	e.scheduleLazySaveLocked()
}

// refillLocked runs once the screens have settled after a change, with
// e.screen up to date. If Bench's window filled its screen before the
// change, and doesn't fill the screen it's on now, as after a display it
// was on was disconnected, the shell fills it again. That moves the window
// within its screen, which doesn't make Bench wait for the screens again.
// A window the user had made smaller is left as it is. While the window is
// hidden or minimised, zooming it could bring it back, so that waits until
// it's shown (onBenchShown).
func (e *Engine) refillLocked() {
	if e.opts.Demo || e.opts.Headless || e.screen.ID == "" {
		return
	}
	e.refillOnShow = e.benchHidden
	if e.benchHidden {
		return
	}
	if e.filled && !fills(e.p.ContentFrame(), e.screen.Visible) {
		e.noteScreenLocked(e.screen, layout.Rect{})
		e.later(e.em.FillScreen)
		return
	}
	e.noteBenchPlaceLocked(e.screen)
}

// stageManagerNoticeLocked shows the notice while Stage Manager is on, or
// takes it away. Stage Manager moves the windows of apps that aren't in
// front to a strip at the side of the screen, and shrinks the one in
// front, so the windows that Bench docks leave their panes. The platform
// reads it as Bench starts and each time Bench becomes active
// (StageManagerChanged).
func (e *Engine) stageManagerNoticeLocked() {
	if e.opts.Headless || !e.p.StageManager() {
		e.setNoticesLocked(sourceStageManager, nil)
		return
	}
	e.setNoticesLocked(sourceStageManager, []string{stageManagerText})
}
