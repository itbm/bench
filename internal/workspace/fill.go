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

// WindowStart is where Bench's window opens: on the screen it was last on,
// if that's still connected, or else the primary screen; whether it fills
// that screen's visible area (fill_screen); whether its title bar is
// compact (the density, on that screen); and its appearance (the
// appearance setting), which the shell's other windows open with too until
// it hears of another (Emitter.Appearance).
type WindowStart struct {
	Screen          platform.Screen
	Fill            bool
	CompactTitleBar bool
	Appearance      string
}

// WindowStart returns where Bench's window opens. The shell asks once,
// before it opens the window. Its title bar is made then, and keeps the
// density it has until Bench opens again, as the View's TitleBar says.
func (e *Engine) WindowStart() WindowStart {
	e.lock()
	defer e.unlock()
	s := findScreen(e.p.Screens(), e.lastScreen)
	class := layout.ChooseSizeClass(e.model.Mode, s.Frame.W, e.cfg.Settings.CompactBelow)
	e.titleBar = densityFor(e.cfg.Settings.Density, class)
	return WindowStart{
		Screen:          s,
		Fill:            e.cfg.Settings.FillScreen,
		CompactTitleBar: e.titleBar == config.DensityCompact,
		Appearance:      e.cfg.Settings.Appearance,
	}
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
// that Bench's window is on, s, for it to open there next time, and whether
// the window fills that screen, for it to fill the screen again after the
// screens change (refillLocked). Without a window of its own, as for bench
// arrange and in the browser preview, there's nothing to record.
func (e *Engine) noteBenchPlaceLocked(s platform.Screen) {
	if e.opts.Headless || e.opts.Demo || s.ID == "" {
		return
	}
	content := e.p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return
	}
	e.filled = fills(content, s.Visible)
	e.noteScreenLocked(s)
}

// noteScreenLocked records the screen that Bench's window is on, saving it
// in state.json when it changes.
func (e *Engine) noteScreenLocked(s platform.Screen) {
	now := &config.ScreenState{ID: s.ID, Name: s.Name, Frame: s.Frame}
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
		e.noteScreenLocked(e.screen)
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
