package windows

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

const (
	// skipSlack is how close, in points, a window must be to a frame for
	// the manager to treat it as already there and skip moving it.
	skipSlack = 0.5
	// ownSlack is how close, in points, a moved window must be to a frame
	// the manager set for OwnChange to put the move down to the manager.
	ownSlack = 2
	// ownMemory is how long the manager remembers the frames it set.
	ownMemory = 2 * time.Second
	// ownRecent is how soon after setting a window's frame any move or
	// resize of it counts as the manager's own.
	ownRecent = 250 * time.Millisecond
)

// Manager applies plans and remembers what it did, so that it can tell its
// own moves from the user's, and put everything back on quit. Its methods
// are safe for concurrent use.
type Manager struct {
	p    platform.Platform
	clk  clock.Clock
	path string

	mu     sync.Mutex
	docked map[platform.WindowID]placement
	tooBig map[platform.WindowID]tooBig
	moves  map[platform.WindowID][]ownMove
	parked map[platform.WindowID]ParkedWindow
	hidden map[string]platform.App // Apps the manager hid, by appKey.
	// fileErr is a failure to update the parked-windows file outside
	// Apply (in Forget), reported by the next Apply.
	fileErr error
}

// ownMove is a frame the manager set a window to, and when.
type ownMove struct {
	frame layout.Rect
	at    time.Time
}

// placement is where the manager docked a window: the pane body, and the
// frame the window ended up with, which can differ from the body (an app
// that sizes its windows in steps, or one that needs more room).
type placement struct {
	body  layout.Rect
	frame layout.Rect
}

// tooBig records that a window couldn't shrink to a body of a given size,
// and the size it kept. Until the window changes size, it won't fit a body
// that's no bigger, so the manager doesn't try again: trying would move an
// inactive window out from where it's parked and back on every Apply.
type tooBig struct {
	body layout.Rect // Only the width and height matter.
	size layout.Rect // Only the width and height matter.
}

// New returns a manager. parkedPath is where it records parked windows
// (config.ParkedFile in the support folder), so that RestoreFromFile can put
// them back after a crash; "" means don't record. A nil clk means
// clock.Real().
func New(p platform.Platform, clk clock.Clock, parkedPath string) *Manager {
	if clk == nil {
		clk = clock.Real()
	}
	return &Manager{
		p:      p,
		clk:    clk,
		path:   parkedPath,
		docked: map[platform.WindowID]placement{},
		tooBig: map[platform.WindowID]tooBig{},
		moves:  map[platform.WindowID][]ownMove{},
		parked: map[platform.WindowID]ParkedWindow{},
		hidden: map[string]platform.App{},
	}
}

// Apply carries out a plan:
//   - Unhide the apps in Unhide.
//   - For each stack: unminimise minimised windows; leave full-screen
//     windows alone (StatusFullScreen); set every window's frame to the
//     body; read the frame back with Platform.Window, and if a window
//     couldn't shrink to fit, park it if it's inactive, or place it at the
//     body's top-left if it's shown (StatusNeedsRoom); then raise the
//     inactive windows and finally the shown one. With Shown == 0, park
//     every window in the stack.
//   - Park the windows in Park, and hide the apps in Hide.
//   - Skip calls that wouldn't change anything (a window already in its
//     frame, an app already hidden), except that raising always happens.
//
// ErrNoWindow gives StatusMissing, and ErrOtherDesktop StatusUnavailable.
//
// An inactive window parked because it can't shrink is reported as
// StatusNeedsRoom too. Minimised windows that are to be parked (in a stack
// with Shown == 0, or in Park) are left minimised, since they're out of
// sight already, and so are full-screen ones.
//
// Besides windows within 0.5 pt of their frame, Apply doesn't move a window
// that's where it ended up the last time it was docked in the same body
// (such as a terminal that sizes itself in whole characters), or one that
// couldn't shrink to a body at least as big and hasn't changed size since.
// So an inactive window that needs more room stays parked, rather than
// coming back into the body to be measured each time.
func (m *Manager) Apply(plan Plan) Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := &applier{m: m, plan: plan, res: Result{Status: map[platform.WindowID]string{}}}
	if m.fileErr != nil {
		a.fail(m.fileErr)
		m.fileErr = nil
	}
	if a.plan.Screen.Frame.W <= 0 || a.plan.Screen.Frame.H <= 0 {
		a.plan.Screen = m.p.ScreenOfWindow()
	}
	a.unhide()
	for _, s := range a.plan.Stacks {
		a.stack(s)
	}
	for _, w := range a.plan.Park {
		a.parkOther(w)
	}
	a.hide()
	return a.res
}

// applier carries out one plan. The manager's lock is held throughout.
type applier struct {
	m       *Manager
	plan    Plan
	res     Result
	screens []platform.Screen
}

func (a *applier) fail(err error) { a.res.Errors = append(a.res.Errors, err) }

// allScreens returns the platform's screens, asking for them once per plan.
func (a *applier) allScreens() []platform.Screen {
	if a.screens == nil {
		a.screens = a.m.p.Screens()
	}
	return a.screens
}

func (a *applier) unhide() {
	for _, app := range a.plan.Unhide {
		key := appKey(app)
		_, ours := a.m.hidden[key]
		if !ours && !app.Hidden {
			continue
		}
		if err := a.m.p.Unhide(app); err != nil {
			a.fail(fmt.Errorf("showing %s: %w", appName(app), err))
			continue
		}
		if ours {
			delete(a.m.hidden, key)
			a.save()
		}
	}
}

func (a *applier) hide() {
	for _, app := range a.plan.Hide {
		if app.Hidden {
			continue
		}
		if err := a.m.p.Hide(app); err != nil {
			a.fail(fmt.Errorf("hiding %s: %w", appName(app), err))
			continue
		}
		key := appKey(app)
		if _, ours := a.m.hidden[key]; !ours {
			a.m.hidden[key] = app
			a.save()
		}
	}
}

// stack docks one pane's windows.
func (a *applier) stack(s Stack) {
	if s.Shown == 0 {
		for _, w := range s.Windows {
			a.parkInStack(w)
		}
		return
	}
	var inactive []platform.Window
	var shown *platform.Window
	for _, w := range s.Windows {
		cur, ok := a.current(w.ID, true)
		if !ok {
			continue
		}
		if cur.FullScreen {
			a.res.Status[w.ID] = StatusFullScreen
			continue
		}
		isShown := w.ID == s.Shown
		cur, docked := a.dock(cur, s.Body, isShown)
		if !docked {
			continue
		}
		if isShown {
			shown = &cur
		} else {
			inactive = append(inactive, cur)
		}
	}
	for _, w := range inactive {
		a.raise(w)
	}
	if shown != nil {
		a.raise(*shown)
	}
}

// current reads a window's state. If that fails, it records why (as a
// status, when inStack) and returns false.
func (a *applier) current(id platform.WindowID, inStack bool) (platform.Window, bool) {
	w, err := a.m.p.Window(id)
	if err == nil {
		return w, true
	}
	a.windowFailed(id, inStack, fmt.Errorf("reading window %d: %w", id, err))
	return platform.Window{}, false
}

// windowFailed records a failed platform call for a window: ErrNoWindow and
// ErrOtherDesktop as statuses (when inStack), anything else as an error.
// A window that has closed is forgotten.
func (a *applier) windowFailed(id platform.WindowID, inStack bool, err error) {
	switch {
	case errors.Is(err, platform.ErrNoWindow):
		if a.m.forgetLocked(id) {
			a.save()
		}
		if inStack {
			a.res.Status[id] = StatusMissing
		}
	case errors.Is(err, platform.ErrOtherDesktop):
		if inStack {
			a.res.Status[id] = StatusUnavailable
		}
	default:
		a.fail(err)
		if inStack {
			if _, ok := a.res.Status[id]; !ok {
				a.res.Status[id] = StatusReady
			}
		}
	}
}

// dock puts a window in a pane body, and reports the window's state
// afterwards and whether it's in the body (rather than parked, or lost).
//
// It doesn't move a window that's where it ended up the last time it was
// docked in the same body, or one that couldn't shrink to a body this size
// before and hasn't changed size since.
func (a *applier) dock(w platform.Window, body layout.Rect, shown bool) (platform.Window, bool) {
	if w.Minimised {
		if err := a.m.p.Unminimise(w); err != nil {
			a.windowFailed(w.ID, true, fmt.Errorf("restoring %s: %w", describe(w), err))
			if isGone(err) {
				return w, false
			}
		} else {
			w.Minimised = false
		}
	}
	if !a.m.settledLocked(w, body) && !a.m.tooBigLocked(w, body) {
		var ok bool
		if w, ok = a.setFrame(w, body); !ok {
			return w, false
		}
	}
	if fits(w.Frame, body) {
		delete(a.m.tooBig, w.ID)
		a.res.Status[w.ID] = StatusReady
		a.docked(w, body)
		return w, true
	}
	a.m.noteTooBigLocked(w, body)
	a.res.Status[w.ID] = StatusNeedsRoom
	if !shown {
		a.park(w, true)
		return w, false
	}
	topLeft := layout.Rect{X: body.X, Y: body.Y, W: w.Frame.W, H: w.Frame.H}
	var ok bool
	if w, ok = a.setFrame(w, topLeft); !ok {
		return w, false
	}
	a.docked(w, body)
	return w, true
}

// setFrame moves a window to r unless it's already there, and returns its
// state afterwards. It returns false if the window has gone or the move
// failed.
func (a *applier) setFrame(w platform.Window, r layout.Rect) (platform.Window, bool) {
	if near(w.Frame, r, skipSlack) {
		return w, true
	}
	if err := a.m.p.SetFrame(w, r); err != nil {
		a.windowFailed(w.ID, true, fmt.Errorf("moving %s: %w", describe(w), err))
		return w, false
	}
	a.m.noteMoveLocked(w.ID, r)
	after, ok := a.current(w.ID, true)
	if !ok {
		return w, false
	}
	a.m.noteMoveLocked(w.ID, after.Frame)
	return after, true
}

// docked records that a window is docked in body, at its current frame,
// and no longer parked.
func (a *applier) docked(w platform.Window, body layout.Rect) {
	a.m.docked[w.ID] = placement{body: body, frame: w.Frame}
	if _, ok := a.m.parked[w.ID]; ok {
		delete(a.m.parked, w.ID)
		a.save()
	}
}

// settledLocked reports whether a window is where it ended up the last time
// the manager docked it in body.
func (m *Manager) settledLocked(w platform.Window, body layout.Rect) bool {
	p, ok := m.docked[w.ID]
	return ok && near(p.body, body, skipSlack) && near(p.frame, w.Frame, skipSlack)
}

// tooBigLocked reports whether a window couldn't shrink to a body at least
// as big as this one in both directions, and has kept the size it had then.
func (m *Manager) tooBigLocked(w platform.Window, body layout.Rect) bool {
	tb, ok := m.tooBig[w.ID]
	return ok && body.W <= tb.body.W+skipSlack && body.H <= tb.body.H+skipSlack &&
		math.Abs(w.Frame.W-tb.size.W) <= skipSlack && math.Abs(w.Frame.H-tb.size.H) <= skipSlack
}

// noteTooBigLocked records that a window, at its current size, doesn't fit
// body, unless what's recorded already covers it.
func (m *Manager) noteTooBigLocked(w platform.Window, body layout.Rect) {
	if !m.tooBigLocked(w, body) {
		m.tooBig[w.ID] = tooBig{body: body, size: w.Frame}
	}
}

func (a *applier) raise(w platform.Window) {
	if err := a.m.p.Raise(w); err != nil {
		a.windowFailed(w.ID, true, fmt.Errorf("raising %s: %w", describe(w), err))
	}
}

// parkInStack parks a window of a stack that shows a Web or Built-in item.
func (a *applier) parkInStack(w platform.Window) {
	cur, ok := a.current(w.ID, true)
	if !ok {
		return
	}
	if cur.FullScreen {
		a.res.Status[w.ID] = StatusFullScreen
		return
	}
	a.res.Status[w.ID] = StatusReady
	if cur.Minimised {
		return
	}
	a.park(cur, true)
}

// parkOther parks a window that belongs to another tab.
func (a *applier) parkOther(w platform.Window) {
	cur, ok := a.current(w.ID, false)
	if !ok || cur.FullScreen || cur.Minimised {
		return
	}
	a.park(cur, false)
}

// park moves a window to a bottom corner of the plan's screen, recording it
// in the parked-windows file first. A window already parked there is left
// alone.
func (a *applier) park(w platform.Window, inStack bool) {
	m := a.m
	screen := a.plan.Screen
	target := platform.ParkFrame(a.allScreens(), screen, w.Frame)
	rec, had := m.parked[w.ID]
	if !had {
		rec = ParkedWindow{Frame: w.Frame, Screen: screen.ID}
	}
	// Keep the record's title up to date, for finding the window after a
	// crash, but keep the frame from before it was first parked.
	updated := rec
	updated.ID, updated.PID, updated.BundleID, updated.Title = w.ID, w.PID, w.BundleID, w.Title
	if !had || updated != rec {
		m.parked[w.ID] = updated
		a.save()
	}
	if had && near(w.Frame, target, skipSlack) {
		delete(m.docked, w.ID)
		return
	}
	if err := m.p.Park(w, screen); err != nil {
		if !had {
			delete(m.parked, w.ID)
			a.save()
		}
		a.windowFailed(w.ID, inStack, fmt.Errorf("parking %s: %w", describe(w), err))
		return
	}
	m.noteMoveLocked(w.ID, target)
	delete(m.docked, w.ID)
}

// save updates the parked-windows file, recording any failure.
func (a *applier) save() {
	if err := a.m.saveLocked(); err != nil {
		a.fail(err)
	}
}

// OwnChange reports whether a WindowMoved or WindowResized event was caused
// by the manager: the window's frame matches (within 2 pt) a frame the
// manager set or parked it at in the last 2 seconds, or the event arrived
// within 250 ms of the manager setting that window's frame.
func (m *Manager) OwnChange(ev platform.Event) bool {
	if ev.Kind != platform.WindowMoved && ev.Kind != platform.WindowResized {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clk.Now()
	for _, mv := range m.recentMovesLocked(ev.Window.ID, now) {
		if now.Sub(mv.at) <= ownRecent || near(ev.Window.Frame, mv.frame, ownSlack) {
			return true
		}
	}
	return false
}

// noteMoveLocked remembers that the manager set a window's frame.
func (m *Manager) noteMoveLocked(id platform.WindowID, r layout.Rect) {
	now := m.clk.Now()
	m.moves[id] = append(m.recentMovesLocked(id, now), ownMove{frame: r, at: now})
}

// recentMovesLocked returns the frames the manager set a window to in the
// last ownMemory, dropping older ones.
func (m *Manager) recentMovesLocked(id platform.WindowID, now time.Time) []ownMove {
	moves := m.moves[id]
	keep := moves[:0]
	for _, mv := range moves {
		if now.Sub(mv.at) <= ownMemory {
			keep = append(keep, mv)
		}
	}
	if len(keep) == 0 {
		delete(m.moves, id)
		return nil
	}
	m.moves[id] = keep
	return keep
}

// Docked returns the body frame the manager last docked a window in. A
// window that has been parked since isn't docked.
func (m *Manager) Docked(id platform.WindowID) (layout.Rect, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.docked[id]
	return p.body, ok
}

// Forget drops what the manager knows about a window (it closed, or the user
// dragged it out of its pane). A parked window that's forgotten stays where
// it is.
func (m *Manager) Forget(id platform.WindowID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.forgetLocked(id) {
		if err := m.saveLocked(); err != nil {
			m.fileErr = err
		}
	}
}

// forgetLocked drops a window, and reports whether it was parked.
func (m *Manager) forgetLocked(id platform.WindowID) bool {
	delete(m.docked, id)
	delete(m.tooBig, id)
	delete(m.moves, id)
	_, wasParked := m.parked[id]
	delete(m.parked, id)
	return wasParked
}

// IsParked reports whether the manager has parked a window.
func (m *Manager) IsParked(id platform.WindowID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.parked[id]
	return ok
}

// Parked returns the windows the manager has parked, by ID, with where
// they were before.
func (m *Manager) Parked() []ParkedWindow {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fileLocked().Windows
}

// fits reports whether a window's frame fits in body: it isn't bigger in
// either direction.
func fits(frame, body layout.Rect) bool {
	return frame.W <= body.W+skipSlack && frame.H <= body.H+skipSlack
}

// near reports whether two frames are within slack points of each other in
// position and size.
func near(a, b layout.Rect, slack float64) bool {
	return math.Abs(a.X-b.X) <= slack && math.Abs(a.Y-b.Y) <= slack &&
		math.Abs(a.W-b.W) <= slack && math.Abs(a.H-b.H) <= slack
}

// isGone reports whether err means the window can't be reached at all.
func isGone(err error) bool {
	return errors.Is(err, platform.ErrNoWindow) || errors.Is(err, platform.ErrOtherDesktop)
}

// appKey identifies an app: by bundle ID, or by PID if it has none.
func appKey(app platform.App) string {
	if app.BundleID != "" {
		return app.BundleID
	}
	return "pid:" + strconv.Itoa(app.PID)
}

// appName names an app in messages.
func appName(app platform.App) string {
	switch {
	case app.Name != "":
		return app.Name
	case app.BundleID != "":
		return app.BundleID
	}
	return "process " + strconv.Itoa(app.PID)
}

// describe names a window in messages, such as `Zed's window "bench"`.
func describe(w platform.Window) string {
	app := w.App
	if app == "" {
		app = w.BundleID
	}
	if app == "" {
		return fmt.Sprintf("window %d", w.ID)
	}
	return fmt.Sprintf("%s's window %q", app, w.Title)
}
