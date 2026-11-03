package windows

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// minOnScreen is how much of a window, in points each way, must be inside a
// screen's visible area for it to count as on that screen. A window smaller
// than this must be wholly inside.
const minOnScreen = 40

const (
	// showPoll is how long RestoreFromFile waits before it looks again for
	// a window of an app it has just shown, and showPolls how many times
	// it waits in all: 1.5 seconds.
	showPoll  = 100 * time.Millisecond
	showPolls = 15
)

// restoreSleep waits, for RestoreFromFile. Tests replace it.
var restoreSleep = time.Sleep

// errFullScreen means a parked window is in native full screen. It reports
// the screen's frame there, so it looks as if it's back on screen, but it
// goes back to where it was parked when it leaves full screen, and macOS
// doesn't let Bench move it until then.
var errFullScreen = errors.New("it's in native full screen")

// Unpark moves a parked window back on screen (to where it was before it was
// parked, or else centred on the screen's visible area), for windows that
// leave Bench (pop out, a deleted tab).
//
// "Where it was" counts only if it's still on a screen; otherwise the window
// is centred on the visible area of the screen it was parked on (or the
// primary screen, if that's gone). A window that's back on screen already
// is left where it is, and one that the manager minimised instead of parking
// is unminimised. Unpark does nothing for a window that isn't parked, and
// forgets a parked window that has closed. A window it couldn't put back,
// including one in native full screen, stays parked, so that RestoreAll
// tries again.
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

// MoveBack moves a window that has left Bench back to a frame it had before
// Bench docked it, as when Undo takes back a tab made of windows that were
// open. It does so only if that frame is on a screen (see OnScreen), and
// the window isn't minimised, in native full screen, or there already. It
// doesn't dock, park, or keep the window, and leaves one that has closed
// alone.
func (m *Manager) MoveBack(id platform.WindowID, frame layout.Rect) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !OnScreen(frame, m.p.Screens()) {
		return nil
	}
	w, err := m.p.Window(id)
	if errors.Is(err, platform.ErrNoWindow) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("moving back window %d: %w", id, err)
	}
	if w.Minimised || w.FullScreen || near(w.Frame, frame, skipSlack) {
		return nil
	}
	if err := m.p.SetFrame(w, frame); err != nil {
		return fmt.Errorf("moving back %s: %w", describe(w), err)
	}
	m.noteMoveLocked(id, frame)
	return nil
}

// unparkLocked puts one parked window back and drops its record. A window
// that can't be reached, other than one that has closed, keeps its record,
// and so do one that couldn't be moved and one in native full screen
// (errFullScreen). A window the manager minimised instead of parking is
// unminimised. A window that's on screen already (an app moved it, or a run
// that crashed couldn't record that it was put back) is left where it is.
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
	if w.FullScreen {
		return fmt.Errorf("putting back %s: %w", describeParked(rec), errFullScreen)
	}
	if rec.Minimised && w.Minimised {
		if err := m.p.Unminimise(w); err != nil {
			return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
		}
	}
	if !OnScreen(w.Frame, screens) {
		fallback := screenByID(screens, rec.Screen)
		target := RestoreFrame(rec.Frame, screens, fallback)
		if !near(w.Frame, target, skipSlack) {
			if err := m.p.SetFrame(w, target); err != nil {
				return fmt.Errorf("putting back %s: %w", describeParked(rec), err)
			}
			m.noteMoveLocked(id, target)
		}
	}
	delete(m.parked, id)
	delete(m.docked, id)
	return nil
}

// RestoreAll puts every parked window back on screen and unhides the apps
// the manager hid. Bench calls it on quit. It carries on past failures and
// returns them together.
//
// It shows the apps first, so that their windows can be found. An app the
// manager hid that has quit since is forgotten. What it couldn't put back (a
// window on another desktop or in native full screen, a window or app that
// didn't respond) stays in the parked-windows file, for the watchdog or the
// next run to put back; the file is removed once nothing is left in it.
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
				delete(m.hidden, key)
				continue
			}
			// Unhide even if app.Hidden is false: macOS updates it only
			// when its main run loop runs, so just after Hide it can still
			// say the app is shown.
			if err := m.p.Unhide(app); err != nil {
				errs = append(errs, fmt.Errorf("showing %s: %w", appName(app), err))
				continue
			}
			delete(m.hidden, key)
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
	m.fileErr = nil
	if err := m.saveLocked(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// RestoreCrashed puts back what a run that crashed (or couldn't put
// everything back as it quit) left in the manager's parked-windows file, as
// RestoreFromFile does. What it can't put back stays in the file and is
// kept by the manager too, so that saving the file doesn't lose it and
// RestoreAll tries again. The apps it shows count as shown by the manager,
// so that Apply hides them again even if their Hidden flag hasn't caught
// up. Call it before Apply.
func (m *Manager) RestoreCrashed() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.path == "" {
		return nil
	}
	left, _, shown, err := restoreFile(m.p, m.path)
	now := m.clk.Now()
	for _, key := range shown {
		m.unhid[key] = now
	}
	for _, rec := range left.Windows {
		if _, ok := m.parked[rec.ID]; !ok {
			m.parked[rec.ID] = rec
		}
	}
	for _, h := range left.Hidden {
		app := platform.App{PID: h.PID, BundleID: h.BundleID, Name: h.Name}
		if _, ok := m.hidden[appKey(app)]; !ok {
			m.hidden[appKey(app)] = app
		}
	}
	if saveErr := m.saveLocked(); saveErr != nil {
		err = errors.Join(err, saveErr)
	}
	return err
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
// left by a run that crashed. A missing file is fine.
//
// It first shows the apps the file says Bench hid, if they're still hidden.
// Then it finds each window by ID, or else by bundle ID and exact title (an
// app that restarted gives its windows new IDs), and puts it back where it
// was, or centred on the primary screen's visible area if that's off every
// screen, unminimising one that Bench minimised instead of parking. Windows
// that are on screen already are left alone, and windows that have closed
// are dropped. It carries on past failures and returns them together.
//
// A window of an app it has just shown can look as if it's on another
// desktop until the app has finished showing, so it looks for such a window
// again, for up to 1.5 seconds in all.
//
// What it couldn't put back (a window on another desktop or in native full
// screen, or a window or app that didn't respond) is written back to the
// file, for the next try; the file is removed when nothing is left. A file
// that can't be read is set aside, as path plus ".bad" (parked.json.bad),
// and nothing is put back from it.
func RestoreFromFile(p platform.Platform, path string) error {
	_, err := RestoreFromFileReport(p, path)
	return err
}

// Restored says what RestoreFromFileReport did with the records in a
// parked-windows file.
type Restored struct {
	// Windows is how many recorded windows are back on screen, whether it
	// moved them or found them there, and Closed how many had closed.
	Windows, Closed int
	// Apps is how many recorded apps are showing, whether it unhid them or
	// found them showing, and Quit how many had quit.
	Apps, Quit int
}

// RestoreFromFileReport is RestoreFromFile, also saying what it did, so
// that bench restore reports what it put back rather than what the file
// listed.
func RestoreFromFileReport(p platform.Platform, path string) (Restored, error) {
	_, done, _, err := restoreFile(p, path)
	return done, err
}

// restoreFile is RestoreFromFileReport, also returning what's left in the
// file and the apps it showed, by appKey.
func restoreFile(p platform.Platform, path string) (ParkedFile, Restored, []string, error) {
	f, err := ReadParked(path)
	if err != nil {
		// Keep what it holds, which a person could still use to find the
		// windows, but out of the way of the next run.
		if mvErr := os.Rename(path, path+".bad"); mvErr != nil {
			err = errors.Join(err, fmt.Errorf("setting aside the parked-windows file: %w", mvErr))
		}
		return ParkedFile{}, Restored{}, nil, err
	}
	if f.Empty() {
		return ParkedFile{}, Restored{}, nil, removeParked(path)
	}
	r := &restorer{p: p, used: map[platform.WindowID]bool{}, shown: map[string]bool{}}
	screens := p.Screens()
	primary := primaryScreen(screens)
	var errs []error
	var left ParkedFile
	for _, h := range f.Hidden {
		if err := r.show(h); err != nil {
			errs = append(errs, err)
			left.Hidden = append(left.Hidden, h)
		}
	}
	for _, rec := range f.Windows {
		if kept, err := r.restore(rec, screens, primary); err != nil {
			errs = append(errs, err)
			left.Windows = append(left.Windows, kept)
		}
	}
	if err := WriteParked(path, left); err != nil {
		errs = append(errs, err)
	}
	return left, r.done, sortedKeys(r.shown), errors.Join(errs...)
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
	// shown holds the apps that show unhid, by appKey.
	shown map[string]bool
	// polls is how many times the restorer has waited for them.
	polls int
	// done counts what it has put back, and what had gone.
	done Restored
}

// restore puts back one recorded window. If it can't, it returns the record
// to keep, with the window's ID now if it was found by title.
func (r *restorer) restore(rec ParkedWindow, screens []platform.Screen, primary platform.Screen) (ParkedWindow, error) {
	w, ok, err := r.find(rec, screens)
	for errors.Is(err, platform.ErrOtherDesktop) && r.waitForShown(rec) {
		w, ok, err = r.find(rec, screens)
	}
	if err != nil {
		return rec, fmt.Errorf("putting back %s: %w", describeParked(rec), err)
	}
	if !ok {
		r.done.Closed++
		return rec, nil
	}
	r.used[w.ID] = true
	if w.FullScreen {
		rec.ID, rec.PID = w.ID, w.PID
		return rec, fmt.Errorf("putting back %s: %w", describeParked(rec), errFullScreen)
	}
	if rec.Minimised && w.Minimised {
		if err := r.p.Unminimise(w); err != nil {
			rec.ID, rec.PID = w.ID, w.PID
			return rec, fmt.Errorf("putting back %s: %w", describeParked(rec), err)
		}
	}
	if OnScreen(w.Frame, screens) {
		r.done.Windows++
		return rec, nil
	}
	target := RestoreFrame(rec.Frame, screens, primary)
	if near(w.Frame, target, skipSlack) {
		r.done.Windows++
		return rec, nil
	}
	if err := r.p.SetFrame(w, target); err != nil {
		rec.ID, rec.PID = w.ID, w.PID
		return rec, fmt.Errorf("putting back %s: %w", describeParked(rec), err)
	}
	r.done.Windows++
	return rec, nil
}

// find looks for a recorded window: by ID, if a window with that ID belongs
// to the same app, or else by bundle ID and exact title, preferring a window
// that's off screen. It returns false if there's no such window, and an
// error if it can't tell: the window is on another desktop, or looking it up
// by ID failed (such as an app that didn't answer in time) and no window has
// its title, which may have changed while it was parked.
func (r *restorer) find(rec ParkedWindow, screens []platform.Screen) (platform.Window, bool, error) {
	w, err := r.p.Window(rec.ID)
	switch {
	case err == nil && w.BundleID == rec.BundleID && !r.used[w.ID]:
		return w, true, nil
	case errors.Is(err, platform.ErrOtherDesktop):
		return platform.Window{}, false, err
	case errors.Is(err, platform.ErrNoWindow):
		err = nil
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
	return platform.Window{}, false, err
}

// waitForShown waits a moment and reports true if rec's window belongs to an
// app that show unhid, unless the restorer has waited for such apps for 1.5
// seconds already. macOS shows an app after Unhide returns, and a process
// that hasn't seen the app's windows before, such as the watchdog, finds
// them only in the app's window list, which can leave a hidden app's out.
func (r *restorer) waitForShown(rec ParkedWindow) bool {
	if !r.shown[appKey(platform.App{PID: rec.PID, BundleID: rec.BundleID})] || r.polls >= showPolls {
		return false
	}
	r.polls++
	restoreSleep(showPoll)
	return true
}

func (r *restorer) runningApps() []platform.App {
	if r.apps == nil {
		r.apps = r.p.RunningApps()
	}
	return r.apps
}

// show unhides an app the crashed run hid, if it's running and hidden.
func (r *restorer) show(h HiddenApp) error {
	running := false
	for _, app := range r.runningApps() {
		match := app.BundleID == h.BundleID
		if h.BundleID == "" {
			match = app.PID == h.PID
		}
		if !match {
			continue
		}
		running = true
		if !app.Hidden {
			continue
		}
		if err := r.p.Unhide(app); err != nil {
			return fmt.Errorf("showing %s: %w", appName(app), err)
		}
		r.shown[appKey(app)] = true
	}
	if running {
		r.done.Apps++
	} else {
		r.done.Quit++
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
