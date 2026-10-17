package windows

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// minOnScreen is how much of a window, in points each way, must be inside a
// screen's visible area for it to count as on that screen. A window smaller
// than this must be wholly inside.
const minOnScreen = 40

// Unpark moves a parked window back on screen (to where it was before it was
// parked, or else centred on the screen's visible area), for windows that
// leave Bench (pop out, a deleted tab).
//
// "Where it was" counts only if it's still on a screen; otherwise the window
// is centred on the visible area of the screen it was parked on (or the
// primary screen, if that's gone). Unpark does nothing for a window that
// isn't parked, and forgets a parked window that has closed.
func (m *Manager) Unpark(id platform.WindowID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.parked[id]; !ok {
		return nil
	}
	err := m.unparkLocked(id, m.p.Screens())
	if saveErr := m.saveLocked(); saveErr != nil {
		err = errors.Join(err, saveErr)
	}
	return err
}

// unparkLocked puts one parked window back and drops its record. A window
// that can't be reached, other than one that has closed, keeps its record.
func (m *Manager) unparkLocked(id platform.WindowID, screens []platform.Screen) error {
	rec := m.parked[id]
	w, err := m.p.Window(id)
	if errors.Is(err, platform.ErrNoWindow) {
		m.forgetLocked(id)
		return nil
	}
	if err != nil {
		return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
	}
	fallback := screenByID(screens, rec.Screen)
	target := RestoreFrame(rec.Frame, screens, fallback)
	if !near(w.Frame, target, skipSlack) {
		if err := m.p.SetFrame(w, target); err != nil {
			return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
		}
		m.noteMoveLocked(id, target)
	}
	delete(m.parked, id)
	delete(m.docked, id)
	return nil
}

// RestoreAll puts every parked window back on screen, unhides the apps the
// manager hid, and clears the parked-windows file. Bench calls it on quit.
// It carries on past failures and returns them together.
//
// It shows the apps first, so that their windows can be found. An app the
// manager hid that has quit since is skipped.
func (m *Manager) RestoreAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	if len(m.hidden) > 0 {
		running := m.p.RunningApps()
		for _, key := range sortedKeys(m.hidden) {
			hid := m.hidden[key]
			app, ok := findRunning(running, hid.BundleID, hid.PID)
			if !ok {
				continue
			}
			// Unhide even if app.Hidden is false: macOS updates it only
			// when its main run loop runs, so just after Hide it can still
			// say the app is shown.
			if err := m.p.Unhide(app); err != nil {
				errs = append(errs, fmt.Errorf("showing %s: %w", appName(app), err))
			}
		}
	}
	if len(m.parked) > 0 {
		screens := m.p.Screens()
		ids := make([]platform.WindowID, 0, len(m.parked))
		for id := range m.parked {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			if err := m.unparkLocked(id, screens); err != nil {
				errs = append(errs, err)
			}
		}
	}
	m.parked = map[platform.WindowID]ParkedWindow{}
	m.hidden = map[string]platform.App{}
	m.fileErr = nil
	if err := m.saveLocked(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// findRunning returns the running app that matches one Bench hid: the same
// process, if it's still running, or else the first app with the same
// bundle ID.
func findRunning(apps []platform.App, bundleID string, pid int) (platform.App, bool) {
	for _, app := range apps {
		if pid != 0 && app.PID == pid && (bundleID == "" || app.BundleID == bundleID) {
			return app, true
		}
	}
	if bundleID == "" {
		return platform.App{}, false
	}
	for _, app := range apps {
		if app.BundleID == bundleID {
			return app, true
		}
	}
	return platform.App{}, false
}

// RestoreFromFile puts back the windows recorded in a parked-windows file
// left by a run that crashed, then removes the file. A missing file is fine.
//
// It first shows the apps the file says Bench hid, if they're still hidden.
// Then it finds each window by ID, or else by bundle ID and exact title (an
// app that restarted gives its windows new IDs), and puts it back where it
// was, or centred on the primary screen's visible area if that's off every
// screen. Windows that are on screen already are left alone. It carries on
// past failures and returns them together; a file that can't be parsed is
// removed too.
func RestoreFromFile(p platform.Platform, path string) error {
	f, err := ReadParked(path)
	if err != nil {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("removing the parked-windows file: %w", rmErr))
		}
		return err
	}
	if f.Empty() {
		return removeParked(path)
	}
	r := &restorer{p: p, used: map[platform.WindowID]bool{}}
	screens := p.Screens()
	primary := primaryScreen(screens)
	var errs []error
	for _, h := range f.Hidden {
		if err := r.show(h); err != nil {
			errs = append(errs, err)
		}
	}
	for _, rec := range f.Windows {
		if err := r.restore(rec, screens, primary); err != nil {
			errs = append(errs, err)
		}
	}
	if err := removeParked(path); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func removeParked(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the parked-windows file: %w", err)
	}
	return nil
}

// restorer puts back the windows from one parked-windows file.
type restorer struct {
	p    platform.Platform
	apps []platform.App
	// used holds the windows already put back, so two records with the
	// same title find different windows.
	used map[platform.WindowID]bool
}

func (r *restorer) restore(rec ParkedWindow, screens []platform.Screen, primary platform.Screen) error {
	w, ok, err := r.find(rec, screens)
	if err != nil {
		return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
	}
	if !ok {
		return nil
	}
	r.used[w.ID] = true
	if OnScreen(w.Frame, screens) {
		return nil
	}
	target := RestoreFrame(rec.Frame, screens, primary)
	if near(w.Frame, target, skipSlack) {
		return nil
	}
	if err := r.p.SetFrame(w, target); err != nil {
		return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
	}
	return nil
}

// find looks for a recorded window: by ID, if a window with that ID belongs
// to the same app, or else by bundle ID and exact title, preferring a window
// that's off screen. It returns false if there's no such window.
func (r *restorer) find(rec ParkedWindow, screens []platform.Screen) (platform.Window, bool, error) {
	w, err := r.p.Window(rec.ID)
	switch {
	case err == nil && w.BundleID == rec.BundleID && !r.used[w.ID]:
		return w, true, nil
	case errors.Is(err, platform.ErrOtherDesktop):
		return platform.Window{}, false, err
	}
	var candidates []platform.Window
	for _, app := range r.runningApps() {
		if app.BundleID != rec.BundleID {
			continue
		}
		ws, err := r.p.Windows(app)
		if err != nil {
			return platform.Window{}, false, err
		}
		for _, w := range ws {
			if w.Title == rec.Title && !r.used[w.ID] {
				candidates = append(candidates, w)
			}
		}
	}
	for _, w := range candidates {
		if !OnScreen(w.Frame, screens) {
			return w, true, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], true, nil
	}
	return platform.Window{}, false, nil
}

func (r *restorer) runningApps() []platform.App {
	if r.apps == nil {
		r.apps = r.p.RunningApps()
	}
	return r.apps
}

// show unhides an app the crashed run hid, if it's running and hidden.
func (r *restorer) show(h HiddenApp) error {
	for _, app := range r.runningApps() {
		match := app.BundleID == h.BundleID
		if h.BundleID == "" {
			match = app.PID == h.PID
		}
		if !match || !app.Hidden {
			continue
		}
		if err := r.p.Unhide(app); err != nil {
			return fmt.Errorf("showing %s: %w", appName(app), err)
		}
	}
	return nil
}

// RestoreFrame returns where to put a window back: at before, if that's on
// a screen (see OnScreen), or else centred on fallback's visible area at
// before's size (shrunk to fit if it's bigger), on whole points. With no
// fallback screen (no screens at all), it returns before.
func RestoreFrame(before layout.Rect, screens []platform.Screen, fallback platform.Screen) layout.Rect {
	if OnScreen(before, screens) {
		return before
	}
	v := visibleArea(fallback)
	if v.W <= 0 || v.H <= 0 {
		return before
	}
	w, h := min(before.W, v.W), min(before.H, v.H)
	return layout.Rect{
		X: layout.Round(v.X + (v.W-w)/2),
		Y: layout.Round(v.Y + (v.H-h)/2),
		W: w,
		H: h,
	}
}

// OnScreen reports whether enough of a frame is inside some screen's visible
// area for the user to see and reach it: at least 40 points each way, or
// all of it if it's smaller.
func OnScreen(r layout.Rect, screens []platform.Screen) bool {
	if r.W <= 0 || r.H <= 0 {
		return false
	}
	needW, needH := min(r.W, minOnScreen), min(r.H, minOnScreen)
	for _, s := range screens {
		in := platform.Intersect(r, visibleArea(s))
		if in.W >= needW && in.H >= needH {
			return true
		}
	}
	return false
}

// visibleArea returns a screen's visible area, or its whole frame if it has
// none.
func visibleArea(s platform.Screen) layout.Rect {
	if s.Visible.W > 0 && s.Visible.H > 0 {
		return s.Visible
	}
	return s.Frame
}

// screenByID returns the screen with an ID, or else the primary screen.
func screenByID(screens []platform.Screen, id string) platform.Screen {
	for _, s := range screens {
		if id != "" && s.ID == id {
			return s
		}
	}
	return primaryScreen(screens)
}

// primaryScreen returns the primary screen, or the first, or a zero Screen.
func primaryScreen(screens []platform.Screen) platform.Screen {
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

func describeParked(rec ParkedWindow) string {
	return describe(platform.Window{ID: rec.ID, BundleID: rec.BundleID, Title: rec.Title})
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
