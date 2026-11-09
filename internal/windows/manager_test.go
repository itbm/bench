package windows

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

var epoch = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

var (
	monitor = platform.Screen{
		ID:      "monitor",
		Name:    "Monitor",
		Frame:   layout.Rect{W: 2560, H: 1440},
		Visible: layout.Rect{Y: 25, W: 2560, H: 1415},
		Primary: true,
	}
	laptop = platform.Screen{
		ID:      "laptop",
		Name:    "Laptop",
		Frame:   layout.Rect{W: 1512, H: 982},
		Visible: layout.Rect{Y: 33, W: 1512, H: 949},
		Primary: true,
	}
	// The main-side preset's panes on the monitor, from the spec's layout
	// maths table, as bodies.
	leftBody  = layout.Body(layout.Rect{X: 8, Y: 33, W: 1651, H: 1399}, layout.HeaderHeight)
	rightBody = layout.Body(layout.Rect{X: 1667, Y: 33, W: 885, H: 1399}, layout.HeaderHeight)
	// Where windows start, on screen.
	startFrame = layout.Rect{X: 200, Y: 200, W: 1000, H: 700}
)

const (
	zedID    = "dev.zed.Zed"
	vscodeID = "com.microsoft.VSCode"
	braveID  = "com.brave.Browser"
	finderID = "com.apple.finder"
)

// world is a fake platform with a manager.
type world struct {
	f    *platform.Fake
	clk  *clock.Fake
	m    *Manager
	path string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	clk := clock.NewFake(epoch)
	f := platform.NewFake(clk)
	f.SetScreens(monitor)
	path := filepath.Join(t.TempDir(), "parked.json")
	return &world{f: f, clk: clk, m: New(f, clk, path), path: path}
}

// window adds a window that starts at startFrame (offset by n points, so
// windows don't share a frame by accident).
func (w *world) window(bundleID, title string, n int, opts ...platform.WindowOption) platform.Window {
	frame := startFrame
	frame.X += float64(n * 10)
	return w.f.AddWindow(bundleID, title, append([]platform.WindowOption{platform.WithFrame(frame)}, opts...)...)
}

func (w *world) app(bundleID string) platform.App {
	app, ok := w.f.App(bundleID)
	if !ok {
		panic("no app " + bundleID)
	}
	return app
}

func (w *world) frame(t *testing.T, id platform.WindowID) layout.Rect {
	t.Helper()
	r, ok := w.f.Frame(id)
	if !ok {
		t.Fatalf("window %d doesn't exist", id)
	}
	return r
}

func (w *world) parkedFile(t *testing.T) (ParkedFile, bool) {
	t.Helper()
	if _, err := os.Stat(w.path); errors.Is(err, os.ErrNotExist) {
		return ParkedFile{}, false
	}
	pf, err := ReadParked(w.path)
	if err != nil {
		t.Fatal(err)
	}
	return pf, true
}

func parkedAt(w platform.Window, s platform.Screen, screens ...platform.Screen) layout.Rect {
	if len(screens) == 0 {
		screens = []platform.Screen{s}
	}
	return platform.ParkFrame(screens, s, w.Frame)
}

// offset returns r moved right by dx points.
func offset(r layout.Rect, dx float64) layout.Rect {
	r.X += dx
	return r
}

func noErrors(t *testing.T, res Result) {
	t.Helper()
	if len(res.Errors) > 0 {
		t.Errorf("Apply errors: %v", res.Errors)
	}
}

func callNames(calls []platform.Call) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.String()
	}
	return out
}

func TestApplyStacksAPane(t *testing.T) {
	tests := []struct {
		name  string
		shown int // Index of the shown window.
	}{
		{name: "first window shown", shown: 0},
		{name: "second window shown", shown: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			zed := w.window(zedID, "bench — switch.go", 0)
			code := w.window(vscodeID, "index.html — bench-ui", 1)
			stack := []platform.Window{zed, code}
			shown, other := stack[tt.shown], stack[1-tt.shown]

			res := w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{
				PaneID: "p1", Body: leftBody, Windows: stack, Shown: shown.ID,
			}}})
			noErrors(t, res)

			for _, win := range stack {
				if got := w.frame(t, win.ID); got != leftBody {
					t.Errorf("%s is at %+v, want the body %+v", win.Title, got, leftBody)
				}
				if res.Status[win.ID] != StatusReady {
					t.Errorf("status of %s = %q, want ready", win.Title, res.Status[win.ID])
				}
				if got, ok := w.m.Docked(win.ID); !ok || got != leftBody {
					t.Errorf("Docked(%s) = %+v, %v", win.Title, got, ok)
				}
			}
			if got := w.f.ZOrder()[:2]; !slices.Equal(got, []platform.WindowID{shown.ID, other.ID}) {
				t.Errorf("front to back = %v, want %d then %d", got, shown.ID, other.ID)
			}
			raises := w.f.CallsTo("Raise")
			if len(raises) != 2 || raises[1].Window != shown.ID {
				t.Errorf("raises = %v, want the inactive window, then the shown one", callNames(raises))
			}
			if w.f.ActiveApp() != "" {
				t.Error("Apply activated an app")
			}
		})
	}
}

func TestApplySkipsWhatWouldntChange(t *testing.T) {
	w := newWorld(t)
	a := w.window(braveID, "Wails v3 docs", 0)
	b := w.window(braveID, "Pull request #42 · bench", 1)
	plan := Plan{Screen: monitor, Stacks: []Stack{{Body: rightBody, Windows: []platform.Window{a, b}, Shown: b.ID}}}
	noErrors(t, w.m.Apply(plan))

	// Nudge one window by less than half a point.
	nudged := rightBody
	nudged.X += 0.4
	w.f.UserMoveWindow(a.ID, nudged)
	w.f.ResetCalls()

	noErrors(t, w.m.Apply(plan))
	if got := w.f.CallsTo("SetFrame", "Park", "Unminimise", "Hide", "Unhide"); len(got) != 0 {
		t.Errorf("second Apply made calls %v, want none", callNames(got))
	}
	if got := w.f.CallsTo("Raise"); len(got) != 2 {
		t.Errorf("second Apply raised %d windows, want 2: raising always happens", len(got))
	}
}

func TestApplyParksOtherTabsWindows(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "Wails v3 docs", 0)
	pr := w.window(braveID, "Pull request #42 · bench", 1)
	before := w.frame(t, pr.ID)

	res := w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{docs}, Shown: docs.ID}},
		Park:   []platform.Window{pr},
	})
	noErrors(t, res)
	if got, want := w.frame(t, pr.ID), parkedAt(pr, monitor); got != want {
		t.Errorf("parked window at %+v, want %+v", got, want)
	}
	if _, ok := res.Status[pr.ID]; ok {
		t.Errorf("Status has the parked window, want only stack windows: %v", res.Status)
	}
	if !w.m.IsParked(pr.ID) || w.m.IsParked(docs.ID) {
		t.Errorf("IsParked: pr %v, docs %v; want true, false", w.m.IsParked(pr.ID), w.m.IsParked(docs.ID))
	}
	if _, ok := w.m.Docked(pr.ID); ok {
		t.Error("a parked window is docked")
	}
	pf, ok := w.parkedFile(t)
	want := ParkedWindow{ID: pr.ID, PID: pr.PID, BundleID: braveID, Title: "Pull request #42 · bench", Frame: before, Screen: "monitor"}
	if !ok || !slices.Equal(pf.Windows, []ParkedWindow{want}) {
		t.Errorf("parked file = %+v (exists %v), want %+v", pf, ok, want)
	}

	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{pr}}))
	if got := w.f.CallsTo("Park", "SetFrame"); len(got) != 0 {
		t.Errorf("parking a parked window again made calls %v", callNames(got))
	}
}

func TestApplyHidesAndUnhidesApps(t *testing.T) {
	tests := []struct {
		name      string
		setUp     func(w *world)
		plan      func(w *world) Plan
		wantCalls []string
		wantShown bool
	}{
		{
			name:      "hides an app",
			plan:      func(w *world) Plan { return Plan{Hide: []platform.App{w.app(finderID)}} },
			wantCalls: []string{"Hide"},
		},
		{
			name:  "skips an app that's hidden already",
			setUp: func(w *world) { w.f.Hide(w.app(finderID)) },
			plan:  func(w *world) Plan { return Plan{Hide: []platform.App{w.app(finderID)}} },
		},
		{
			name:      "unhides a hidden app",
			setUp:     func(w *world) { w.f.Hide(w.app(finderID)) },
			plan:      func(w *world) Plan { return Plan{Unhide: []platform.App{w.app(finderID)}} },
			wantCalls: []string{"Unhide"},
			wantShown: true,
		},
		{
			name:      "skips unhiding an app that isn't hidden",
			plan:      func(w *world) Plan { return Plan{Unhide: []platform.App{w.app(finderID)}} },
			wantShown: true,
		},
		{
			name: "unhides an app it hid, whatever the plan says",
			setUp: func(w *world) {
				w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}})
			},
			plan: func(w *world) Plan {
				app := w.app(finderID)
				app.Hidden = false // A stale snapshot.
				return Plan{Unhide: []platform.App{app}}
			},
			wantCalls: []string{"Unhide"},
			wantShown: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.f.AddApp(finderID, "Finder")
			w.window(finderID, "Downloads", 0)
			if tt.setUp != nil {
				tt.setUp(w)
			}
			w.f.ResetCalls()
			noErrors(t, w.m.Apply(tt.plan(w)))
			var got []string
			for _, c := range w.f.CallsTo("Hide", "Unhide") {
				got = append(got, c.Method)
			}
			if !slices.Equal(got, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", got, tt.wantCalls)
			}
			if hidden := w.f.IsHidden(finderID); hidden == tt.wantShown {
				t.Errorf("hidden = %v, want %v", hidden, !tt.wantShown)
			}
		})
	}
}

func TestApplyUnhidesBeforeDocking(t *testing.T) {
	w := newWorld(t)
	zed := w.window(zedID, "bench", 0)
	w.f.Hide(w.app(zedID))
	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Unhide: []platform.App{w.app(zedID)},
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{zed}, Shown: zed.ID}},
	}))
	var got []string
	for _, c := range w.f.Calls() {
		if c.Method == "Unhide" || c.Method == "SetFrame" || c.Method == "Raise" {
			got = append(got, c.Method)
		}
	}
	if want := []string{"Unhide", "SetFrame", "Raise"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestApplyParksAStackShowingBenchsOwnView(t *testing.T) {
	w := newWorld(t)
	zed := w.window(zedID, "bench", 0)
	code := w.window(vscodeID, "bench-ui", 1)
	mini := w.window(vscodeID, "notes", 2, platform.WithMinimised())

	res := w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{
		Body: rightBody, Windows: []platform.Window{zed, code, mini}, Shown: 0,
	}}})
	noErrors(t, res)
	for _, win := range []platform.Window{zed, code} {
		if got, want := w.frame(t, win.ID), parkedAt(win, monitor); got != want {
			t.Errorf("%s at %+v, want parked at %+v", win.Title, got, want)
		}
		if !w.m.IsParked(win.ID) || res.Status[win.ID] != StatusReady {
			t.Errorf("%s: parked %v, status %q", win.Title, w.m.IsParked(win.ID), res.Status[win.ID])
		}
	}
	if st, _ := w.f.WindowState(mini.ID); !st.Minimised || st.Frame != mini.Frame {
		t.Errorf("minimised window = %+v, want left minimised where it was", st)
	}
	if got := w.f.CallsTo("Raise", "Unminimise"); len(got) != 0 {
		t.Errorf("calls %v, want no raising or unminimising", callNames(got))
	}
}

func TestApplyNeedsRoom(t *testing.T) {
	w := newWorld(t)
	big := layout.Rect{W: rightBody.W + 200, H: 900}
	shown := w.window(zedID, "bench", 0, platform.WithMinSize(big.W, big.H))
	inactive := w.window(vscodeID, "bench-ui", 1, platform.WithMinSize(big.W, big.H))
	fine := w.window(braveID, "docs", 2)

	res := w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{
		Body: rightBody, Windows: []platform.Window{shown, inactive, fine}, Shown: shown.ID,
	}}})
	noErrors(t, res)

	wantShown := layout.Rect{X: rightBody.X, Y: rightBody.Y, W: big.W, H: rightBody.H}
	if got := w.frame(t, shown.ID); got != wantShown {
		t.Errorf("shown window at %+v, want the body's top-left at its own size %+v", got, wantShown)
	}
	if res.Status[shown.ID] != StatusNeedsRoom || res.Status[inactive.ID] != StatusNeedsRoom {
		t.Errorf("statuses = %v, want needs-room for both big windows", res.Status)
	}
	if got, ok := w.m.Docked(shown.ID); !ok || got != rightBody {
		t.Errorf("Docked(shown) = %+v, %v; want the body", got, ok)
	}
	got := w.frame(t, inactive.ID)
	if want := platform.ParkFrame([]platform.Screen{monitor}, monitor, got); got != want || !w.m.IsParked(inactive.ID) {
		t.Errorf("inactive big window at %+v, want parked at %+v", got, want)
	}
	if res.Status[fine.ID] != StatusReady || w.frame(t, fine.ID) != rightBody {
		t.Errorf("window that fits: status %q, frame %+v", res.Status[fine.ID], w.frame(t, fine.ID))
	}
	if z := w.f.ZOrder(); z[0] != shown.ID || z[1] != fine.ID {
		t.Errorf("front to back = %v, want shown then the docked inactive window", z)
	}
	for _, c := range w.f.CallsTo("Raise") {
		if c.Window == inactive.ID {
			t.Error("raised the parked window")
		}
	}
}

func TestNeedsRoom(t *testing.T) {
	w := newWorld(t)
	big := layout.Rect{W: rightBody.W + 200, H: 900}
	shown := w.window(zedID, "bench", 0, platform.WithMinSize(big.W, big.H))
	fine := w.window(braveID, "docs", 1)
	if w.m.NeedsRoom(shown.ID, rightBody) {
		t.Error("NeedsRoom before the window was docked = true")
	}
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: rightBody, Windows: []platform.Window{shown, fine}, Shown: shown.ID}}}))

	taller := rightBody
	taller.H += layout.HeaderHeight + layout.HeaderDivider
	wider := rightBody
	wider.W = big.W
	for _, tt := range []struct {
		name string
		id   platform.WindowID
		body layout.Rect
		want bool
	}{
		{"the body it didn't fit", shown.ID, rightBody, true},
		{"a smaller body", shown.ID, layout.Rect{W: rightBody.W - 10, H: rightBody.H - 10}, true},
		// A pane without its header is taller: the window may fit it, which
		// only docking it there can tell.
		{"a taller body", shown.ID, taller, false},
		{"a body as wide as the window", shown.ID, wider, false},
		{"a window that fitted", fine.ID, rightBody, false},
	} {
		if got := w.m.NeedsRoom(tt.id, tt.body); got != tt.want {
			t.Errorf("NeedsRoom for %s = %v, want %v", tt.name, got, tt.want)
		}
	}
	w.m.Forget(shown.ID)
	if w.m.NeedsRoom(shown.ID, rightBody) {
		t.Error("NeedsRoom for a forgotten window = true")
	}
}

func TestApplyLeavesFullScreenWindowsAlone(t *testing.T) {
	w := newWorld(t)
	full := w.window(zedID, "bench", 0, platform.WithFullScreen())
	other := w.window(zedID, "other tab", 1, platform.WithFullScreen())
	code := w.window(vscodeID, "bench-ui", 2)
	before := w.frame(t, full.ID)
	w.f.ResetCalls()

	res := w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{
			{Body: leftBody, Windows: []platform.Window{full, code}, Shown: full.ID},
			{Body: rightBody, Windows: []platform.Window{other}, Shown: 0},
		},
		Park: []platform.Window{other},
	})
	noErrors(t, res)
	if res.Status[full.ID] != StatusFullScreen || res.Status[other.ID] != StatusFullScreen {
		t.Errorf("statuses = %v, want full-screen", res.Status)
	}
	for _, c := range w.f.CallsTo("SetFrame", "Park", "Raise", "Unminimise") {
		if c.Window == full.ID || c.Window == other.ID {
			t.Errorf("touched a full-screen window: %v", c)
		}
	}
	if got := w.frame(t, full.ID); got != before {
		t.Errorf("full-screen window moved to %+v", got)
	}
	// The pane says its shown window is in full screen: the window behind
	// it waits out of sight, rather than covering that.
	if !w.m.IsParked(code.ID) || res.Status[code.ID] != StatusReady {
		t.Errorf("the other window in the stack should be parked: at %+v, status %s", w.frame(t, code.ID), res.Status[code.ID])
	}
	for _, c := range w.f.CallsTo("Raise") {
		if c.Window == code.ID {
			t.Errorf("raised the window behind a full-screen one: %v", c)
		}
	}

	// When the shown window isn't the full-screen one, the stack docks as
	// usual.
	res = w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{full, code}, Shown: code.ID}}})
	noErrors(t, res)
	if w.frame(t, code.ID) != leftBody || res.Status[code.ID] != StatusReady || w.m.IsParked(code.ID) {
		t.Error("the shown window wasn't docked")
	}
}

func TestApplyRestoresMinimisedWindows(t *testing.T) {
	w := newWorld(t)
	shown := w.window(zedID, "bench", 0, platform.WithMinimised())
	inactive := w.window(vscodeID, "bench-ui", 1, platform.WithMinimised())

	res := w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{
		Body: leftBody, Windows: []platform.Window{shown, inactive}, Shown: shown.ID,
		Restore: []platform.WindowID{shown.ID, inactive.ID},
	}}})
	noErrors(t, res)
	for _, win := range []platform.Window{shown, inactive} {
		st, _ := w.f.WindowState(win.ID)
		if st.Minimised || st.Frame != leftBody || res.Status[win.ID] != StatusReady {
			t.Errorf("%s = %+v, status %q; want restored into the body", win.Title, st, res.Status[win.ID])
		}
	}
	if got := w.f.CallsTo("Unminimise"); len(got) != 2 {
		t.Errorf("Unminimise calls = %v, want 2", callNames(got))
	}
	if z := w.f.ZOrder(); z[0] != shown.ID {
		t.Errorf("front window = %d, want %d", z[0], shown.ID)
	}
}

func TestApplyLeavesWindowsTheUserMinimisedInTheDock(t *testing.T) {
	w := newWorld(t)
	shown := w.window(zedID, "bench", 0, platform.WithMinimised())
	inactive := w.window(vscodeID, "bench-ui", 1)
	asked := w.window(braveID, "docs", 2, platform.WithMinimised())
	plan := Plan{Screen: monitor, Stacks: []Stack{{
		Body: leftBody, Windows: []platform.Window{shown, inactive, asked}, Shown: shown.ID,
		Restore: []platform.WindowID{asked.ID},
	}}}

	// The manager didn't minimise the shown window: the user did, perhaps
	// before the engine heard, so it stays in the Dock, untouched, and the
	// other windows are docked as if it weren't there. One that the
	// engine asks to restore comes back.
	res := w.m.Apply(plan)
	noErrors(t, res)
	if st, _ := w.f.WindowState(shown.ID); !st.Minimised || st.Frame != shown.Frame || res.Status[shown.ID] != StatusMinimised {
		t.Errorf("the minimised window is %+v, status %q; want it left in the Dock", st, res.Status[shown.ID])
	}
	for _, c := range w.f.CallsTo("Unminimise", "SetFrame", "Raise", "Park", "Minimise") {
		if c.Window == shown.ID {
			t.Errorf("acted on the minimised window: %v", c)
		}
	}
	if st, _ := w.f.WindowState(asked.ID); st.Minimised || st.Frame != leftBody || res.Status[asked.ID] != StatusReady {
		t.Errorf("the window to restore is %+v, status %q; want it docked", st, res.Status[asked.ID])
	}
	if w.frame(t, inactive.ID) != leftBody || res.Status[inactive.ID] != StatusReady {
		t.Errorf("the other window is at %+v, status %q", w.frame(t, inactive.ID), res.Status[inactive.ID])
	}
}

func TestApplyMissingAndUnavailable(t *testing.T) {
	w := newWorld(t)
	gone := w.window(braveID, "closed", 0)
	away := w.window(braveID, "on another desktop", 1)
	here := w.window(braveID, "docs", 2)
	parkedGone := w.window(braveID, "parked, then closed", 3)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{parkedGone}}))
	w.f.CloseWindow(gone.ID)
	w.f.CloseWindow(parkedGone.ID)
	w.f.MoveToOtherDesktop(away.ID)

	res := w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{gone, away, here}, Shown: gone.ID}},
		Park:   []platform.Window{parkedGone},
	})
	noErrors(t, res)
	want := map[platform.WindowID]string{gone.ID: StatusMissing, away.ID: StatusUnavailable, here.ID: StatusReady}
	for id, st := range want {
		if res.Status[id] != st {
			t.Errorf("status of %d = %q, want %q", id, res.Status[id], st)
		}
	}
	if w.frame(t, here.ID) != leftBody {
		t.Error("the window that's there wasn't docked")
	}
	if w.m.IsParked(parkedGone.ID) {
		t.Error("a parked window that closed is still parked")
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file still exists with nothing parked")
	}
}

func TestApplyCarriesOnAfterErrors(t *testing.T) {
	w := newWorld(t)
	a := w.window(zedID, "a", 0)
	b := w.window(zedID, "b", 1)
	c := w.window(braveID, "c", 2)
	boom := errors.New("boom")
	w.f.SetWindowErr(a.ID, "SetFrame", boom)
	w.f.SetWindowErr(c.ID, "Park", boom)
	w.f.SetErr("Hide", boom)
	w.f.AddApp(finderID, "Finder")

	res := w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{a, b}, Shown: b.ID}},
		Park:   []platform.Window{c},
		Hide:   []platform.App{w.app(finderID)},
	})
	if len(res.Errors) != 3 {
		t.Fatalf("errors = %v, want 3", res.Errors)
	}
	for _, err := range res.Errors {
		if !errors.Is(err, boom) {
			t.Errorf("error %v doesn't wrap the platform's", err)
		}
	}
	if w.frame(t, b.ID) != leftBody || res.Status[b.ID] != StatusReady {
		t.Error("the window after the failure wasn't docked")
	}
	// The app may still make the move, or hide, after an error such as a
	// timeout, so both stay recorded, for putting back.
	if !w.m.IsParked(c.ID) || !w.m.Hid(w.app(finderID)) {
		t.Errorf("after failures, parked %v and hid %v; want both recorded", w.m.IsParked(c.ID), w.m.Hid(w.app(finderID)))
	}
	if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 || pf.Windows[0].ID != c.ID || len(pf.Hidden) != 1 {
		t.Errorf("parked file = %+v, want the window that failed to park and Finder", pf)
	}
	if w.m.Apply(Plan{}).Errors != nil {
		t.Error("errors carried over to the next plan")
	}
}

func TestApplyParksOnBenchsScreenByDefault(t *testing.T) {
	w := newWorld(t)
	w.f.SetScreens(laptop, platform.Screen{ID: "right", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}})
	w.f.SetBenchScreen("right")
	win := w.window(zedID, "bench", 0)
	noErrors(t, w.m.Apply(Plan{Park: []platform.Window{win}}))
	if got, want := w.frame(t, win.ID), (layout.Rect{X: 4071, Y: 1439, W: startFrame.W, H: startFrame.H}); got != want {
		t.Errorf("parked at %+v, want the right-hand screen's corner %+v", got, want)
	}
}

func TestOwnChange(t *testing.T) {
	w := newWorld(t)
	zed := w.window(zedID, "bench", 0)
	code := w.window(vscodeID, "bench-ui", 1)
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{zed}, Shown: zed.ID}},
		Park:   []platform.Window{code},
	}))

	evs := w.f.DrainEvents()
	if len(evs) == 0 {
		t.Fatal("the fake sent no events for Bench's moves")
	}
	for _, ev := range evs {
		if !w.m.OwnChange(ev) {
			t.Errorf("OwnChange(%s of %d) = false for Bench's own move", ev.Kind, ev.Window.ID)
		}
	}

	moved := func(id platform.WindowID, r layout.Rect) platform.Event {
		return platform.Event{Kind: platform.WindowMoved, Window: platform.Window{ID: id, Frame: r}}
	}
	nudged := leftBody
	nudged.X += 1.5
	shifted := leftBody
	shifted.X += 3
	parked := w.frame(t, code.ID)

	tests := []struct {
		name    string
		advance time.Duration
		ev      func() platform.Event
		want    bool
	}{
		{"any move just after setting the frame", 100 * time.Millisecond, func() platform.Event { return moved(zed.ID, shifted) }, true},
		{"a late echo of the frame set", 400 * time.Millisecond, func() platform.Event { return moved(zed.ID, leftBody) }, true},
		{"within 2 points of the frame set", 0, func() platform.Event { return moved(zed.ID, nudged) }, true},
		{"the parked frame", 0, func() platform.Event { return moved(code.ID, parked) }, true},
		{"more than 2 points away", 0, func() platform.Event { return moved(zed.ID, shifted) }, false},
		{"resized by the user", 0, func() platform.Event {
			return platform.Event{Kind: platform.WindowResized, Window: platform.Window{ID: zed.ID, Frame: layout.Rect{X: leftBody.X, Y: leftBody.Y, W: 500, H: 500}}}
		}, false},
		{"a window the manager never moved", 0, func() platform.Event { return moved(999, leftBody) }, false},
		{"not a move", 0, func() platform.Event {
			return platform.Event{Kind: platform.WindowFocused, Window: platform.Window{ID: zed.ID, Frame: leftBody}}
		}, false},
		{"the frame set, more than 2 seconds later", 2 * time.Second, func() platform.Event { return moved(zed.ID, leftBody) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w.clk.Advance(tt.advance)
			if got := w.m.OwnChange(tt.ev()); got != tt.want {
				t.Errorf("OwnChange() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("the user's drag", func(t *testing.T) {
		w.f.DrainEvents()
		w.f.UserMoveWindow(zed.ID, layout.Rect{X: 600, Y: 300, W: 900, H: 700})
		evs := w.f.DrainEvents()
		if len(evs) == 0 {
			t.Fatal("no events for the user's drag")
		}
		for _, ev := range evs {
			if w.m.OwnChange(ev) {
				t.Errorf("OwnChange(%s) = true for the user's drag", ev.Kind)
			}
		}
	})
}

func TestRestoreAll(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	shown := w.window(zedID, "bench", 0)
	parkedA := w.window(braveID, "docs", 1)
	parkedB := w.window(braveID, "pr", 2)
	beforeA, beforeB := w.frame(t, parkedA.ID), w.frame(t, parkedB.ID)
	w.f.Hide(w.app(braveID)) // Hidden by the user: RestoreAll leaves it.
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{shown}, Shown: shown.ID}},
		Park:   []platform.Window{parkedA, parkedB},
		Hide:   []platform.App{w.app(finderID), w.app(braveID)},
	}))
	if pf, ok := w.parkedFile(t); !ok || len(pf.Windows) != 2 || len(pf.Hidden) != 1 {
		t.Fatalf("parked file = %+v, want two windows and Finder", pf)
	}

	if err := w.m.RestoreAll(); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if w.frame(t, parkedA.ID) != beforeA || w.frame(t, parkedB.ID) != beforeB {
		t.Error("parked windows weren't put back where they were")
	}
	if w.frame(t, shown.ID) != leftBody {
		t.Error("RestoreAll moved a docked window")
	}
	if w.f.IsHidden(finderID) {
		t.Error("Finder, which the manager hid, is still hidden")
	}
	if !w.f.IsHidden(braveID) {
		t.Error("RestoreAll showed an app the user hid")
	}
	if w.m.IsParked(parkedA.ID) || len(w.m.Parked()) != 0 {
		t.Error("windows are still parked")
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file still exists")
	}
	if err := w.m.RestoreAll(); err != nil {
		t.Errorf("RestoreAll again: %v", err)
	}
}

// Without Accessibility permission, the manager can show the apps it hid,
// which needs no permission, but not move the windows it parked, which stay
// recorded for RestoreAll.
func TestShowHiddenLeavesParkedWindows(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	shown := w.window(zedID, "bench", 0)
	parked := w.window(braveID, "docs", 1)
	before := w.frame(t, parked.ID)
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{shown}, Shown: shown.ID}},
		Park:   []platform.Window{parked},
		Hide:   []platform.App{w.app(finderID)},
	}))
	// The engine's snapshot of Finder, made before macOS updates its
	// Hidden flag, still says it's hidden.
	stale := w.app(finderID)
	w.f.SetTrusted(false)
	if err := w.m.ShowHidden(); err != nil {
		t.Fatalf("ShowHidden: %v", err)
	}
	if w.f.IsHidden(finderID) || w.m.Hid(w.app(finderID)) {
		t.Error("Finder, which the manager hid, should be shown and forgotten")
	}
	if !w.m.IsParked(parked.ID) || w.frame(t, parked.ID) == before {
		t.Error("the parked window should stay parked")
	}
	pf, ok := w.parkedFile(t)
	if !ok || len(pf.Windows) != 1 || len(pf.Hidden) != 0 {
		t.Errorf("parked file = %+v, want the parked window and no apps", pf)
	}
	// Once the permission is back, Apply hides Finder again, whatever the
	// stale flag says.
	w.f.SetTrusted(true)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{stale}}))
	if !w.f.IsHidden(finderID) || !w.m.Hid(stale) {
		t.Errorf("hidden %v, recorded %v; want both", w.f.IsHidden(finderID), w.m.Hid(stale))
	}
	if err := w.m.RestoreAll(); err != nil {
		t.Fatalf("RestoreAll: %v", err)
	}
	if w.frame(t, parked.ID) != before {
		t.Error("RestoreAll should put the parked window back")
	}
}

func TestRestoreAllCentresWindowsThatWereOffScreen(t *testing.T) {
	w := newWorld(t)
	w.f.SetScreens(laptop, platform.Screen{ID: "monitor", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}})
	onMonitor := w.f.AddWindow(braveID, "docs", platform.WithFrame(layout.Rect{X: 2000, Y: 100, W: 1000, H: 700}))
	noErrors(t, w.m.Apply(Plan{Screen: laptop, Park: []platform.Window{onMonitor}}))

	w.f.SetScreens(laptop) // The monitor is unplugged.
	if err := w.m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	want := layout.Rect{X: 256, Y: 158, W: 1000, H: 700}
	if got := w.frame(t, onMonitor.ID); got != want {
		t.Errorf("window put back at %+v, want centred on the laptop at %+v", got, want)
	}
}

func TestParkedFileTracksParking(t *testing.T) {
	w := newWorld(t)
	a := w.window(braveID, "docs", 0)
	b := w.window(braveID, "pr", 1)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{a, b}}))
	pf, _ := w.parkedFile(t)
	if got := len(pf.Windows); got != 2 {
		t.Fatalf("parked file has %d windows, want 2", got)
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parked file isn't JSON: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(w.path))
	if len(entries) != 1 {
		t.Errorf("support folder holds %d files, want only parked.json (no temporary files)", len(entries))
	}

	// The title changes while it's parked; parking again records it.
	w.f.RetitleWindow(b.ID, "pr — merged")
	b2, _ := w.f.Window(b.ID)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{a, b2}}))
	pf, _ = w.parkedFile(t)
	if pf.Windows[1].Title != "pr — merged" || pf.Windows[1].Frame != offset(startFrame, 10) {
		t.Errorf("record = %+v, want the new title and the frame from before parking", pf.Windows[1])
	}

	// Docking a parked window drops its record.
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{a}, Shown: a.ID}}}))
	pf, _ = w.parkedFile(t)
	if len(pf.Windows) != 1 || pf.Windows[0].ID != b.ID || w.m.IsParked(a.ID) {
		t.Errorf("after docking, parked file = %+v", pf)
	}

	if err := w.m.Unpark(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file wasn't removed when nothing was parked")
	}
}

func TestNoParkedFile(t *testing.T) {
	w := newWorld(t)
	m := New(w.f, w.clk, "")
	win := w.window(braveID, "docs", 0)
	noErrors(t, m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}, Hide: []platform.App{w.app(braveID)}}))
	if !m.IsParked(win.ID) {
		t.Error("window isn't parked")
	}
	if entries, _ := os.ReadDir(filepath.Dir(w.path)); len(entries) != 0 {
		t.Errorf("a manager without a path wrote %d files", len(entries))
	}
	if err := m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, win.ID) != startFrame || w.f.IsHidden(braveID) {
		t.Error("RestoreAll didn't put things back")
	}
}

func TestUnpark(t *testing.T) {
	t.Run("back where it was", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}}))
		if err := w.m.Unpark(win.ID); err != nil {
			t.Fatal(err)
		}
		if w.frame(t, win.ID) != startFrame || w.m.IsParked(win.ID) {
			t.Errorf("window at %+v, parked %v", w.frame(t, win.ID), w.m.IsParked(win.ID))
		}
	})

	t.Run("centred when where it was is off screen", func(t *testing.T) {
		w := newWorld(t)
		docked := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: rightBody, Windows: []platform.Window{docked}, Shown: docked.ID}}}))
		docked, _ = w.f.Window(docked.ID)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docked}}))
		w.f.SetScreens(laptop)
		if err := w.m.Unpark(docked.ID); err != nil {
			t.Fatal(err)
		}
		// The body is taller than the laptop's visible area, so it shrinks.
		want := layout.Rect{X: layout.Round((1512 - rightBody.W) / 2), Y: 33, W: rightBody.W, H: 949}
		if got := w.frame(t, docked.ID); got != want {
			t.Errorf("window at %+v, want %+v", got, want)
		}
	})

	t.Run("not parked", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		w.f.ResetCalls()
		if err := w.m.Unpark(win.ID); err != nil {
			t.Fatal(err)
		}
		if got := w.f.Calls(); len(got) != 0 {
			t.Errorf("calls = %v, want none", callNames(got))
		}
	})

	t.Run("closed", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}}))
		w.f.CloseWindow(win.ID)
		if err := w.m.Unpark(win.ID); err != nil {
			t.Fatal(err)
		}
		if w.m.IsParked(win.ID) {
			t.Error("closed window is still parked")
		}
	})

	t.Run("on another desktop", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}}))
		w.f.MoveToOtherDesktop(win.ID)
		if err := w.m.Unpark(win.ID); !errors.Is(err, platform.ErrOtherDesktop) {
			t.Errorf("Unpark: %v, want ErrOtherDesktop", err)
		}
		if !w.m.IsParked(win.ID) {
			t.Error("a window that couldn't be put back is no longer parked")
		}
	})
}

func TestMoveBack(t *testing.T) {
	t.Run("back where it was", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: rightBody, Windows: []platform.Window{win}, Shown: win.ID}}}))
		w.m.Forget(win.ID)
		if err := w.m.MoveBack(win.ID, startFrame); err != nil {
			t.Fatal(err)
		}
		if got := w.frame(t, win.ID); got != startFrame {
			t.Errorf("window at %+v, want %+v", got, startFrame)
		}
		// Bench moved it, so its move event is Bench's own.
		for _, ev := range w.f.DrainEvents() {
			if ev.Kind == platform.WindowMoved && ev.Window.ID == win.ID && !w.m.OwnChange(ev) {
				t.Errorf("the move back isn't Bench's own: %+v", ev)
			}
		}
	})

	t.Run("left alone", func(t *testing.T) {
		w := newWorld(t)
		there := w.window(braveID, "there", 0)
		small := w.window(braveID, "small", 1, platform.WithMinimised())
		docked := w.window(zedID, "bench", 2)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{docked}, Shown: docked.ID}}}))
		w.f.ResetCalls()
		for id, frame := range map[platform.WindowID]layout.Rect{
			there.ID:  startFrame,                                   // There already.
			small.ID:  offset(startFrame, 300),                      // Minimised.
			docked.ID: layout.Rect{X: 9000, Y: 200, W: 800, H: 600}, // Off every screen.
		} {
			if err := w.m.MoveBack(id, frame); err != nil {
				t.Fatal(err)
			}
		}
		if got := w.f.CallsTo("SetFrame"); len(got) != 0 {
			t.Errorf("SetFrame calls = %v, want none", callNames(got))
		}
		w.f.CloseWindow(there.ID)
		if err := w.m.MoveBack(there.ID, offset(startFrame, 50)); err != nil {
			t.Errorf("a closed window: %v, want nothing", err)
		}
	})
}

func TestForget(t *testing.T) {
	w := newWorld(t)
	docked := w.window(zedID, "bench", 0)
	parked := w.window(braveID, "docs", 1)
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{docked}, Shown: docked.ID}},
		Park:   []platform.Window{parked},
	}))
	at := w.frame(t, parked.ID)

	w.m.Forget(docked.ID)
	if _, ok := w.m.Docked(docked.ID); ok {
		t.Error("a forgotten window is still docked")
	}
	w.f.DrainEvents()
	w.f.UserMoveWindow(docked.ID, offset(leftBody, 1))
	for _, ev := range w.f.DrainEvents() {
		if w.m.OwnChange(ev) {
			t.Error("OwnChange still knows a forgotten window's moves")
		}
	}

	w.m.Forget(parked.ID)
	if w.m.IsParked(parked.ID) {
		t.Error("a forgotten window is still parked")
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file still lists the forgotten window")
	}
	if err := w.m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if got := w.frame(t, parked.ID); got != at {
		t.Errorf("forgotten parked window moved to %+v, want it left at %+v", got, at)
	}
}

func TestRestoreFromFileAfterACrash(t *testing.T) {
	tests := []struct {
		name string
		// crash happens after Bench parks the windows and hides Finder, and
		// before RestoreFromFile.
		crash func(t *testing.T, w *world, wins []platform.Window)
		// want gives where each window should end up, by title.
		want map[string]layout.Rect
	}{
		{
			name:  "windows found by ID",
			crash: func(t *testing.T, w *world, wins []platform.Window) {},
			want:  map[string]layout.Rect{"docs": startFrame, "pr": offset(startFrame, 10)},
		},
		{
			name: "the app restarted, so windows are found by title",
			crash: func(t *testing.T, w *world, wins []platform.Window) {
				parked := map[string]layout.Rect{}
				for _, win := range wins {
					parked[win.Title] = w.frame(t, win.ID)
				}
				w.f.QuitApp(braveID)
				w.f.AddApp(braveID, "Brave")
				// The browser reopens its windows where they were when it
				// quit: parked. And a new one, which isn't Bench's.
				w.f.AddWindow(braveID, "pr", platform.WithFrame(parked["pr"]))
				w.f.AddWindow(braveID, "docs", platform.WithFrame(parked["docs"]))
				w.f.AddWindow(braveID, "unrelated", platform.WithFrame(parked["docs"]))
			},
			want: map[string]layout.Rect{"docs": startFrame, "pr": offset(startFrame, 10), "unrelated": parkedAt(platform.Window{Frame: startFrame}, monitor)},
		},
		{
			name: "the monitor was unplugged, but where they were is on the laptop",
			crash: func(t *testing.T, w *world, wins []platform.Window) {
				w.f.SetScreens(laptop)
			},
			want: map[string]layout.Rect{"docs": startFrame, "pr": offset(startFrame, 10)},
		},
		{
			name: "a window already back on screen is left alone",
			crash: func(t *testing.T, w *world, wins []platform.Window) {
				w.f.UserMoveWindow(wins[0].ID, layout.Rect{X: 50, Y: 50, W: 600, H: 400})
			},
			want: map[string]layout.Rect{"docs": {X: 50, Y: 50, W: 600, H: 400}, "pr": offset(startFrame, 10)},
		},
		{
			name: "a window that closed is skipped",
			crash: func(t *testing.T, w *world, wins []platform.Window) {
				w.f.CloseWindow(wins[0].ID)
			},
			want: map[string]layout.Rect{"pr": offset(startFrame, 10)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.f.AddApp(finderID, "Finder")
			docs := w.window(braveID, "docs", 0)
			pr := w.window(braveID, "pr", 1)
			wins := []platform.Window{docs, pr}
			noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: wins, Hide: []platform.App{w.app(finderID)}}))

			// Bench crashes: the manager is lost, and the file stays.
			w.m = nil
			tt.crash(t, w, wins)

			if err := RestoreFromFile(w.f, w.path); err != nil {
				t.Fatalf("RestoreFromFile: %v", err)
			}
			for _, app := range w.f.RunningApps() {
				ws, _ := w.f.Windows(app)
				for _, win := range ws {
					want, ok := tt.want[win.Title]
					if !ok {
						continue
					}
					if win.Frame != want {
						t.Errorf("%q at %+v, want %+v", win.Title, win.Frame, want)
					}
				}
			}
			if w.f.IsHidden(finderID) {
				t.Error("Finder, which the crashed run hid, is still hidden")
			}
			if _, ok := w.parkedFile(t); ok {
				t.Error("the parked file wasn't removed")
			}

			// A new run starts with nothing parked.
			m := New(w.f, w.clk, w.path)
			if len(m.Parked()) != 0 {
				t.Error("a new manager has parked windows")
			}
		})
	}
}

func TestRestoreFromFileCentresWindowsOffEveryScreen(t *testing.T) {
	w := newWorld(t)
	right := w.f.AddWindow(braveID, "docs", platform.WithFrame(layout.Rect{X: 2000, Y: 300, W: 1000, H: 700}))
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{right}}))
	w.f.SetScreens(laptop) // The monitor is unplugged after the crash.
	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	if got, want := w.frame(t, right.ID), (layout.Rect{X: 256, Y: 158, W: 1000, H: 700}); got != want {
		t.Errorf("window at %+v, want centred on the primary screen at %+v", got, want)
	}
}

func TestRestoreFromFileProblems(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		w := newWorld(t)
		if err := RestoreFromFile(w.f, w.path); err != nil {
			t.Errorf("RestoreFromFile: %v, want nil", err)
		}
	})

	t.Run("bad file", func(t *testing.T) {
		w := newWorld(t)
		if err := os.WriteFile(w.path, []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RestoreFromFile(w.f, w.path); err == nil {
			t.Error("RestoreFromFile of a bad file = nil, want an error")
		}
		if _, ok := w.parkedFile(t); ok {
			t.Error("the bad file wasn't moved out of the way")
		}
		// What it holds may still help someone find their windows.
		if data, err := os.ReadFile(w.path + ".bad"); err != nil || string(data) != "{not json" {
			t.Errorf("parked.json.bad = %q, %v; want the bad file", data, err)
		}
	})

	t.Run("a window on another desktop", func(t *testing.T) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		other := w.window(braveID, "pr", 1)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win, other}}))
		w.f.MoveToOtherDesktop(win.ID)
		err := RestoreFromFile(w.f, w.path)
		if !errors.Is(err, platform.ErrOtherDesktop) {
			t.Errorf("RestoreFromFile: %v, want ErrOtherDesktop", err)
		}
		if w.frame(t, other.ID) != offset(startFrame, 10) {
			t.Error("RestoreFromFile stopped at the first failure")
		}
	})
}

func TestRestoreFrame(t *testing.T) {
	screens := []platform.Screen{laptop, {ID: "right", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}}}
	tests := []struct {
		name     string
		before   layout.Rect
		fallback platform.Screen
		want     layout.Rect
	}{
		{"on the laptop", layout.Rect{X: 100, Y: 100, W: 800, H: 600}, laptop, layout.Rect{X: 100, Y: 100, W: 800, H: 600}},
		{"across both screens", layout.Rect{X: 1200, Y: 100, W: 800, H: 600}, laptop, layout.Rect{X: 1200, Y: 100, W: 800, H: 600}},
		{"mostly off, but 40 points on", layout.Rect{X: -760, Y: 100, W: 800, H: 600}, laptop, layout.Rect{X: -760, Y: 100, W: 800, H: 600}},
		{"under 40 points on", layout.Rect{X: -770, Y: 100, W: 800, H: 600}, laptop, layout.Rect{X: 356, Y: 208, W: 800, H: 600}},
		{"a parked frame", layout.Rect{X: -799, Y: 981, W: 800, H: 600}, laptop, layout.Rect{X: 356, Y: 208, W: 800, H: 600}},
		{"under the menu bar only", layout.Rect{X: 100, Y: 0, W: 800, H: 30}, laptop, layout.Rect{X: 356, Y: 493, W: 800, H: 30}},
		{"a small window, wholly on", layout.Rect{X: 10, Y: 40, W: 20, H: 20}, laptop, layout.Rect{X: 10, Y: 40, W: 20, H: 20}},
		{"too big for the fallback", layout.Rect{X: 9000, Y: 0, W: 2000, H: 1200}, laptop, layout.Rect{X: 0, Y: 33, W: 1512, H: 949}},
		{"centred on another screen", layout.Rect{X: 9000, Y: 0, W: 1000, H: 700}, screens[1], layout.Rect{X: 2292, Y: 383, W: 1000, H: 700}},
		{"no size", layout.Rect{X: 100, Y: 100}, laptop, layout.Rect{X: 756, Y: 508}},
		{"no fallback screen", layout.Rect{X: 9000, Y: 0, W: 1000, H: 700}, platform.Screen{}, layout.Rect{X: 9000, Y: 0, W: 1000, H: 700}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RestoreFrame(tt.before, screens, tt.fallback); got != tt.want {
				t.Errorf("RestoreFrame() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestConcurrentUse runs Apply, OwnChange, and the queries together, for the
// race detector.
func TestConcurrentUse(t *testing.T) {
	w := newWorld(t)
	a := w.window(zedID, "a", 0)
	b := w.window(braveID, "b", 1)
	plans := []Plan{
		{Screen: monitor, Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{a}, Shown: a.ID}}, Park: []platform.Window{b}},
		{Screen: monitor, Stacks: []Stack{{Body: rightBody, Windows: []platform.Window{b}, Shown: b.ID}}, Park: []platform.Window{a}},
	}
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 25 {
				w.m.Apply(plans[(i+j)%2])
				w.m.IsParked(a.ID)
				w.m.Docked(b.ID)
				w.m.Parked()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			select {
			case ev := <-w.f.Events():
				w.m.OwnChange(ev)
			default:
			}
		}
	}()
	wg.Wait()
	if err := w.m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWhenUnminimisingFails(t *testing.T) {
	w := newWorld(t)
	stuck := w.window(zedID, "stuck", 0, platform.WithMinimised())
	closing := w.window(zedID, "closing", 1, platform.WithMinimised())
	boom := errors.New("boom")
	w.f.SetWindowErr(stuck.ID, "Unminimise", boom)
	// The window closes between being read and being restored.
	w.f.SetWindowErr(closing.ID, "Unminimise", platform.ErrNoWindow)

	res := w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{
		Body: leftBody, Windows: []platform.Window{stuck, closing}, Shown: stuck.ID,
		Restore: []platform.WindowID{stuck.ID, closing.ID},
	}}})
	if len(res.Errors) != 1 || !errors.Is(res.Errors[0], boom) {
		t.Errorf("errors = %v, want only boom", res.Errors)
	}
	if res.Status[closing.ID] != StatusMissing {
		t.Errorf("status of the closing window = %q, want missing", res.Status[closing.ID])
	}
	if got := w.frame(t, stuck.ID); got != leftBody || res.Status[stuck.ID] != StatusReady {
		t.Errorf("window that wouldn't restore: frame %+v, status %q; want docked anyway", got, res.Status[stuck.ID])
	}
	for _, c := range w.f.CallsTo("SetFrame", "Raise") {
		if c.Window == closing.ID {
			t.Errorf("acted on the closed window: %v", c)
		}
	}
}

func TestParkedFileErrors(t *testing.T) {
	// The parked file's folder can't be created, because a file is in the
	// way.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-folder")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w := newWorld(t)
	w.m = New(w.f, w.clk, filepath.Join(blocker, "parked.json"))
	win := w.window(braveID, "docs", 0)

	res := w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}})
	if len(res.Errors) == 0 {
		t.Error("Apply didn't report that it couldn't write the parked file")
	}
	if !w.m.IsParked(win.ID) || w.frame(t, win.ID) == startFrame {
		t.Error("a file error stopped the window being parked")
	}

	w.m.Forget(win.ID)
	if res := w.m.Apply(Plan{}); len(res.Errors) != 1 {
		t.Errorf("next Apply errors = %v, want Forget's file error", res.Errors)
	}
	if res := w.m.Apply(Plan{}); len(res.Errors) != 0 {
		t.Errorf("Forget's file error was reported twice: %v", res.Errors)
	}
}

func TestParkedFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "parked.json")
	want := ParkedFile{
		Windows: []ParkedWindow{{ID: 7, PID: 1001, BundleID: braveID, Title: "docs", Frame: startFrame, Screen: "monitor"}},
		Hidden:  []HiddenApp{{PID: 1002, BundleID: finderID, Name: "Finder"}},
	}
	if err := WriteParked(path, want); err != nil {
		t.Fatal(err)
	}
	// The folder it made, and the file, are private, as Bench's other
	// files are.
	for name, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		if info, err := os.Stat(name); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want permissions %v", name, info.Mode().Perm(), err, want)
		}
	}
	got, err := ReadParked(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Windows, want.Windows) || !slices.Equal(got.Hidden, want.Hidden) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	if err := WriteParked(path, ParkedFile{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty file wasn't removed: %v", err)
	}
	if got, err := ReadParked(path); err != nil || !got.Empty() {
		t.Errorf("ReadParked of a missing file = %+v, %v; want empty, nil", got, err)
	}
	if err := WriteParked(path, ParkedFile{}); err != nil {
		t.Errorf("removing a missing file: %v", err)
	}
}

func TestMessages(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{describe(platform.Window{ID: 3, App: "Zed", Title: "bench"}), `Zed's window "bench"`},
		{describe(platform.Window{ID: 3, BundleID: zedID, Title: "bench"}), `dev.zed.Zed's window "bench"`},
		{describe(platform.Window{ID: 3}), "window 3"},
		{appName(platform.App{Name: "Finder", BundleID: finderID}), "Finder"},
		{appName(platform.App{BundleID: finderID}), finderID},
		{appName(platform.App{PID: 42}), "process 42"},
		{appKey(platform.App{PID: 42}), "pid:42"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestApplyDoesntMeasureWindowsThatNeedRoomAgain(t *testing.T) {
	w := newWorld(t)
	big := layout.Rect{W: rightBody.W + 200, H: 900}
	shown := w.window(zedID, "bench", 0, platform.WithMinSize(big.W, big.H))
	inactive := w.window(vscodeID, "bench-ui", 1, platform.WithMinSize(big.W, big.H))
	plan := func(body layout.Rect) Plan {
		return Plan{Screen: monitor, Stacks: []Stack{{
			Body: body, Windows: []platform.Window{shown, inactive}, Shown: shown.ID,
		}}}
	}
	noErrors(t, w.m.Apply(plan(rightBody)))
	parkedAt := w.frame(t, inactive.ID)
	if !w.m.IsParked(inactive.ID) {
		t.Fatal("test set-up: the inactive window wasn't parked")
	}

	t.Run("the same body", func(t *testing.T) {
		w.f.DrainEvents()
		w.f.ResetCalls()
		res := w.m.Apply(plan(rightBody))
		noErrors(t, res)
		if got := w.f.CallsTo("SetFrame", "Park"); len(got) != 0 {
			t.Errorf("calls = %v, want none", callNames(got))
		}
		if evs := w.f.DrainEvents(); len(evs) != 0 {
			t.Errorf("events = %v, want none: nothing should move", evs)
		}
		if res.Status[shown.ID] != StatusNeedsRoom || res.Status[inactive.ID] != StatusNeedsRoom {
			t.Errorf("statuses = %v, want needs-room for both", res.Status)
		}
	})

	t.Run("Bench moves", func(t *testing.T) {
		moved := offset(rightBody, -50)
		w.f.ResetCalls()
		noErrors(t, w.m.Apply(plan(moved)))
		calls := w.f.CallsTo("SetFrame", "Park")
		want := layout.Rect{X: moved.X, Y: moved.Y, W: big.W, H: rightBody.H}
		if len(calls) != 1 || calls[0].Window != shown.ID || calls[0].Rect != want {
			t.Errorf("calls = %v, want only the shown window moved to %+v", callNames(calls), want)
		}
		if got := w.frame(t, inactive.ID); got != parkedAt {
			t.Errorf("inactive window at %+v, want it left parked at %+v", got, parkedAt)
		}
	})

	t.Run("the window changes size", func(t *testing.T) {
		// Its app might have a different minimum size now, so try again.
		resized := parkedAt
		resized.W += 40
		w.f.UserMoveWindow(inactive.ID, resized)
		w.f.ResetCalls()
		noErrors(t, w.m.Apply(plan(rightBody)))
		var tried bool
		for _, c := range w.f.CallsTo("SetFrame") {
			tried = tried || (c.Window == inactive.ID && c.Rect == rightBody)
		}
		if !tried || !w.m.IsParked(inactive.ID) {
			t.Errorf("calls = %v; want the inactive window tried in the body, then parked again", callNames(w.f.Calls()))
		}
	})

	t.Run("the body grows enough", func(t *testing.T) {
		roomy := layout.Rect{X: 8, Y: 74, W: big.W + 100, H: 1357}
		res := w.m.Apply(plan(roomy))
		noErrors(t, res)
		for _, win := range []platform.Window{shown, inactive} {
			if got := w.frame(t, win.ID); got != roomy || res.Status[win.ID] != StatusReady {
				t.Errorf("%s at %+v, status %q; want docked in %+v", win.Title, got, res.Status[win.ID], roomy)
			}
		}
		if w.m.IsParked(inactive.ID) {
			t.Error("the inactive window is still parked")
		}
		if _, ok := w.parkedFile(t); ok {
			t.Error("the parked file still exists")
		}
	})
}

func TestApplyLeavesWindowsThatSizeInSteps(t *testing.T) {
	w := newWorld(t)
	term := w.window("com.apple.Terminal", "zsh", 0, platform.WithSizeStep(7, 15))
	plan := func(body layout.Rect) Plan {
		return Plan{Screen: monitor, Stacks: []Stack{{Body: body, Windows: []platform.Window{term}, Shown: term.ID}}}
	}
	res := w.m.Apply(plan(rightBody))
	noErrors(t, res)
	got := w.frame(t, term.ID)
	if got == rightBody || got.X != rightBody.X || got.Y != rightBody.Y || res.Status[term.ID] != StatusReady {
		t.Fatalf("terminal at %+v, status %q; want a little smaller than the body, and ready", got, res.Status[term.ID])
	}

	w.f.ResetCalls()
	noErrors(t, w.m.Apply(plan(rightBody)))
	if calls := w.f.CallsTo("SetFrame"); len(calls) != 0 {
		t.Errorf("calls = %v, want none for a window where it settled last time", callNames(calls))
	}
	if got, ok := w.m.Docked(term.ID); !ok || got != rightBody {
		t.Errorf("Docked() = %+v, %v; want the body", got, ok)
	}
	if frame, ok := w.m.DockedFrame(term.ID); !ok || frame != got {
		t.Errorf("DockedFrame() = %+v, %v; want where the terminal ended up, %+v", frame, ok, got)
	}

	w.f.ResetCalls()
	moved := offset(rightBody, -50)
	noErrors(t, w.m.Apply(plan(moved)))
	if calls := w.f.CallsTo("SetFrame"); len(calls) != 1 || calls[0].Rect != moved {
		t.Errorf("calls = %v, want one move into the new body", callNames(calls))
	}
}

func TestRestoreAllSkipsAppsThatQuit(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	w.f.AddApp(braveID, "Brave")
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(finderID), w.app(braveID)}}))

	w.f.QuitApp(finderID)
	// Brave restarts, with a new process ID, and something hides it again.
	w.f.QuitApp(braveID)
	w.f.AddApp(braveID, "Brave", platform.AppHidden())
	w.f.ResetCalls()

	if err := w.m.RestoreAll(); err != nil {
		t.Fatalf("RestoreAll: %v, want nil for an app that quit", err)
	}
	calls := w.f.CallsTo("Unhide")
	if len(calls) != 1 || calls[0].BundleID != braveID {
		t.Errorf("Unhide calls = %v, want only Brave's", callNames(calls))
	}
	if w.f.IsHidden(braveID) {
		t.Error("the restarted Brave is still hidden")
	}
}

func TestRestoreFromFileShowsAppsFirst(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "docs", 0)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}}))
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(braveID)}}))
	w.f.ResetCalls()

	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range w.f.CallsTo("Unhide", "SetFrame") {
		got = append(got, c.Method)
	}
	if want := []string{"Unhide", "SetFrame"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if w.frame(t, docs.ID) != startFrame || w.f.IsHidden(braveID) {
		t.Error("Brave's window wasn't put back and shown")
	}
}

func TestRestoreAllKeepsWhatItCouldntPutBack(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	busy := w.window(braveID, "busy", 0)
	away := w.window(braveID, "away", 1)
	fine := w.window(braveID, "fine", 2)
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Park:   []platform.Window{busy, away, fine},
		Hide:   []platform.App{w.app(finderID)},
	}))
	parkedFrame := w.frame(t, busy.ID)
	// Brave is busy and doesn't move one window, another is on another
	// desktop, and Finder doesn't respond.
	timeout := errors.New("kAXErrorCannotComplete")
	w.f.SetWindowErr(busy.ID, "SetFrame", timeout)
	w.f.MoveToOtherDesktop(away.ID)
	w.f.SetErr("Unhide", timeout)

	err := w.m.RestoreAll()
	if !errors.Is(err, timeout) || !errors.Is(err, platform.ErrOtherDesktop) {
		t.Fatalf("RestoreAll: %v, want both failures", err)
	}
	if w.frame(t, fine.ID) != offset(startFrame, 20) {
		t.Error("the window that could be put back wasn't")
	}
	if w.frame(t, busy.ID) != parkedFrame {
		t.Errorf("the busy window moved to %+v", w.frame(t, busy.ID))
	}
	pf, ok := w.parkedFile(t)
	var ids []platform.WindowID
	for _, rec := range pf.Windows {
		ids = append(ids, rec.ID)
	}
	if !ok || !slices.Equal(ids, []platform.WindowID{busy.ID, away.ID}) || len(pf.Hidden) != 1 || pf.Hidden[0].BundleID != finderID {
		t.Fatalf("parked file = %+v (exists %v), want the busy and away windows and Finder", pf, ok)
	}
	if !w.m.IsParked(busy.ID) || !w.m.IsParked(away.ID) || w.m.IsParked(fine.ID) || !w.m.Hid(w.app(finderID)) {
		t.Error("the manager should still know what it couldn't put back")
	}

	// Trying again once they respond puts everything back and removes the
	// file.
	w.f.SetWindowErr(busy.ID, "SetFrame", nil)
	w.f.SetErr("Unhide", nil)
	w.f.ReturnToDesktop(away.ID)
	if err := w.m.RestoreAll(); err != nil {
		t.Fatalf("RestoreAll again: %v", err)
	}
	if w.frame(t, busy.ID) != startFrame || w.frame(t, away.ID) != offset(startFrame, 10) || w.f.IsHidden(finderID) {
		t.Error("the second RestoreAll didn't put everything back")
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file wasn't removed once everything was back")
	}
}

func TestRestoreFromFileKeepsWhatItCouldntPutBack(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	away := w.window(braveID, "away", 0)
	fine := w.window(braveID, "fine", 1)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{away, fine}, Hide: []platform.App{w.app(finderID)}}))
	// Bench crashes while the user is on another desktop, where one of
	// the windows is.
	w.m = nil
	w.f.MoveToOtherDesktop(away.ID)
	w.f.SetErr("Unhide", errors.New("no answer"))

	if err := RestoreFromFile(w.f, w.path); !errors.Is(err, platform.ErrOtherDesktop) {
		t.Fatalf("RestoreFromFile: %v, want ErrOtherDesktop", err)
	}
	if w.frame(t, fine.ID) != offset(startFrame, 10) {
		t.Error("the reachable window wasn't put back")
	}
	pf, ok := w.parkedFile(t)
	if !ok || len(pf.Windows) != 1 || pf.Windows[0].ID != away.ID || pf.Windows[0].Frame != startFrame || len(pf.Hidden) != 1 {
		t.Fatalf("parked file = %+v (exists %v), want only the window on another desktop, and Finder", pf, ok)
	}

	// The next run keeps what's left, so saving the file doesn't lose it,
	// and puts it back on quit.
	w.f.SetErr("Unhide", nil)
	w.f.MoveToOtherDesktop(away.ID)
	m := New(w.f, w.clk, w.path)
	if err := m.RestoreCrashed(); !errors.Is(err, platform.ErrOtherDesktop) {
		t.Fatalf("RestoreCrashed: %v, want ErrOtherDesktop", err)
	}
	if w.f.IsHidden(finderID) {
		t.Error("Finder should be shown now that it answers")
	}
	if !m.IsParked(away.ID) {
		t.Fatal("the new manager doesn't know about the window it couldn't put back")
	}
	other := w.window(braveID, "other", 2)
	noErrors(t, m.Apply(Plan{Screen: monitor, Park: []platform.Window{other}}))
	if pf, _ := w.parkedFile(t); len(pf.Windows) != 2 {
		t.Errorf("parked file = %+v, want the left-over window and the new one", pf)
	}
	w.f.ReturnToDesktop(away.ID)
	if err := m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, away.ID) != startFrame || w.frame(t, other.ID) != offset(startFrame, 20) {
		t.Errorf("RestoreAll put the windows at %+v and %+v", w.frame(t, away.ID), w.frame(t, other.ID))
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file wasn't removed")
	}
}

func TestRestoreFromFileKeepsANewIDForAWindowFoundByTitle(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "docs", 0)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}}))
	parked := w.frame(t, docs.ID)
	w.m = nil
	w.f.QuitApp(braveID)
	w.f.AddApp(braveID, "Brave")
	again := w.f.AddWindow(braveID, "docs", platform.WithFrame(parked))
	w.f.SetWindowErr(again.ID, "SetFrame", errors.New("busy"))

	if err := RestoreFromFile(w.f, w.path); err == nil {
		t.Fatal("RestoreFromFile = nil, want the failure")
	}
	pf, _ := w.parkedFile(t)
	if len(pf.Windows) != 1 || pf.Windows[0].ID != again.ID || pf.Windows[0].Frame != startFrame {
		t.Fatalf("parked file = %+v, want the record under the window's new ID", pf)
	}
	w.f.SetWindowErr(again.ID, "SetFrame", nil)
	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, again.ID) != startFrame {
		t.Errorf("window at %+v, want %+v", w.frame(t, again.ID), startFrame)
	}
}

func TestOwnFocus(t *testing.T) {
	w := newWorld(t)
	zed := w.window(zedID, "bench", 0)
	code := w.window(vscodeID, "bench-ui", 1)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{zed, code}, Shown: zed.ID}}}))
	focused := func(id platform.WindowID) platform.Event {
		return platform.Event{Kind: platform.WindowFocused, Window: platform.Window{ID: id}}
	}
	if !w.m.OwnFocus(focused(code.ID)) || !w.m.OwnFocus(focused(zed.ID)) {
		t.Error("focus events just after raising should be the manager's own")
	}
	if w.m.OwnFocus(platform.Event{Kind: platform.WindowMoved, Window: platform.Window{ID: code.ID}}) {
		t.Error("OwnFocus is only for focus events")
	}
	w.clk.Advance(600 * time.Millisecond)
	if w.m.OwnFocus(focused(code.ID)) {
		t.Error("a focus event long after a raise is the user's")
	}
}

func TestHideRecordsTheApp(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	if err := w.m.Hide(w.app(finderID)); err != nil {
		t.Fatal(err)
	}
	if !w.f.IsHidden(finderID) || !w.m.Hid(w.app(finderID)) {
		t.Fatal("Finder should be hidden, by the manager")
	}
	if pf, _ := w.parkedFile(t); len(pf.Hidden) != 1 {
		t.Errorf("parked file = %+v, want Finder", pf)
	}
	if err := w.m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if w.f.IsHidden(finderID) || w.m.Hid(w.app(finderID)) {
		t.Error("RestoreAll didn't show Finder")
	}
	// A hide that reports a failure can still happen (decision 14), so
	// the app is recorded first and stays recorded.
	w.f.SetErr("Hide", errors.New("no"))
	if err := w.m.Hide(w.app(finderID)); err == nil || !w.m.Hid(w.app(finderID)) {
		t.Errorf("a failed Hide: %v, recorded %v; want an error, and recorded", err, w.m.Hid(w.app(finderID)))
	}
	if pf, _ := w.parkedFile(t); len(pf.Hidden) != 1 {
		t.Errorf("parked file = %+v, want Finder", pf)
	}
}

// TestUnhideShowsAnAppTheManagerHid checks that Unhide shows an app that
// Hide hid, forgets it, and leaves an app it didn't hide alone.
func TestUnhideShowsAnAppTheManagerHid(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	w.f.AddApp(zedID, "Zed")
	if err := w.m.Hide(w.app(finderID)); err != nil {
		t.Fatal(err)
	}
	w.f.Hide(w.app(zedID))
	if err := w.m.Unhide(w.app(finderID)); err != nil {
		t.Fatal(err)
	}
	if w.f.IsHidden(finderID) || w.m.Hid(w.app(finderID)) {
		t.Error("Finder should be shown, and no longer the manager's")
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file still lists Finder")
	}
	if err := w.m.Unhide(w.app(zedID)); err != nil || !w.f.IsHidden(zedID) {
		t.Errorf("Unhide of an app the manager didn't hide: %v, hidden %v; want nil, and hidden", err, w.f.IsHidden(zedID))
	}
}

// TestParkOutsideAPlan checks that Park parks a window as Apply parks one,
// recorded in the parked file first, so that Unpark, RestoreAll, and
// RestoreFromFile put it back where it was.
func TestParkOutsideAPlan(t *testing.T) {
	w := newWorld(t)
	win := w.window(finderID, "Downloads", 0)
	if err := w.m.Park(win, monitor); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, win.ID) != parkedAt(win, monitor) || !w.m.IsParked(win.ID) {
		t.Fatalf("the window is at %+v, parked %v; want parked", w.frame(t, win.ID), w.m.IsParked(win.ID))
	}
	if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 || pf.Windows[0].Frame != startFrame {
		t.Errorf("parked file = %+v, want the window, from %+v", pf, startFrame)
	}
	if err := w.m.Unpark(win.ID); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, win.ID) != startFrame || w.m.IsParked(win.ID) {
		t.Errorf("Unpark: the window is at %+v; want %+v", w.frame(t, win.ID), startFrame)
	}
	// Quitting puts it back.
	if err := w.m.Park(win, platform.Screen{}); err != nil {
		t.Fatal(err)
	}
	if err := w.m.RestoreAll(); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, win.ID) != startFrame {
		t.Errorf("RestoreAll: the window is at %+v; want %+v", w.frame(t, win.ID), startFrame)
	}
	// So does the next start, after a crash.
	if err := w.m.Park(win, monitor); err != nil {
		t.Fatal(err)
	}
	w.m = nil
	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	if w.frame(t, win.ID) != startFrame {
		t.Errorf("after a crash: the window is at %+v; want %+v", w.frame(t, win.ID), startFrame)
	}
}

// TestParkPassesOverWhatIsOutOfSight checks that Park leaves a minimised
// window as it is, and passes over one that has closed, keeping no record.
func TestParkPassesOverWhatIsOutOfSight(t *testing.T) {
	w := newWorld(t)
	small := w.window(finderID, "Minimised", 0, platform.WithMinimised())
	gone := w.window(finderID, "Gone", 1)
	if err := w.f.CloseWindow(gone.ID); err != nil {
		t.Fatal(err)
	}
	w.f.DrainEvents()
	w.f.ResetCalls()
	for _, win := range []platform.Window{small, gone} {
		if err := w.m.Park(win, monitor); err != nil {
			t.Errorf("Park(%q) = %v", win.Title, err)
		}
		if w.m.IsParked(win.ID) {
			t.Errorf("%q was recorded as parked", win.Title)
		}
	}
	if calls := w.f.CallsTo("Park", "Minimise"); len(calls) != 0 {
		t.Errorf("calls %v, want none", calls)
	}
}

func TestHideRecordsTheAppBeforeHiding(t *testing.T) {
	// Hide fails, as when the Accessibility fallback times out, but the
	// app hides anyway; then Bench crashes.
	for _, viaApply := range []bool{false, true} {
		w := newWorld(t)
		w.f.AddApp(finderID, "Finder")
		w.f.SetErr("Hide", errors.New("timed out"))
		if viaApply {
			if res := w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}}); len(res.Errors) != 1 {
				t.Errorf("Apply errors = %v, want the failed hide", res.Errors)
			}
		} else if err := w.m.Hide(w.app(finderID)); err == nil {
			t.Error("Hide = nil, want the platform's error")
		}
		w.f.SetErr("Hide", nil)
		w.f.Hide(w.app(finderID))
		w.m = nil
		if err := RestoreFromFile(w.f, w.path); err != nil {
			t.Fatal(err)
		}
		if w.f.IsHidden(finderID) {
			t.Errorf("through Apply %v: Finder stayed hidden after the crash", viaApply)
		}
	}
}

func TestApplyUnparks(t *testing.T) {
	w := newWorld(t)
	undocked := w.window(braveID, "docs", 0)
	never := w.window(braveID, "pr", 1)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{undocked}}))
	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Unpark: []platform.Window{undocked, never}}))
	if w.frame(t, undocked.ID) != startFrame || w.m.IsParked(undocked.ID) {
		t.Errorf("the parked window wasn't put back: %+v", w.frame(t, undocked.ID))
	}
	for _, c := range w.f.Calls() {
		if c.Window == never.ID {
			t.Errorf("touched a window the manager hadn't parked: %v", c)
		}
	}
	if _, ok := w.parkedFile(t); ok {
		t.Error("the parked file still lists the window")
	}
}

func TestApplyReportsFileErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-folder")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w := newWorld(t)
	w.m = New(w.f, w.clk, filepath.Join(blocker, "parked.json"))
	win := w.window(braveID, "docs", 0)
	if res := w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}}); res.FileErr == nil {
		t.Error("FileErr wasn't set when the parked file couldn't be written")
	}
	// The file is still out of date: each Apply tries again, and says so,
	// without reporting the same error again in Errors.
	if res := w.m.Apply(Plan{Screen: monitor}); res.FileErr == nil || len(res.Errors) != 0 {
		t.Errorf("while the file is out of date: FileErr %v, errors %v", res.FileErr, res.Errors)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if res := w.m.Apply(Plan{Screen: monitor}); res.FileErr != nil {
		t.Errorf("FileErr = %v once the file could be written", res.FileErr)
	}
	if pf, _ := ReadParked(filepath.Join(blocker, "parked.json")); len(pf.Windows) != 1 {
		t.Errorf("the parked file = %+v, want the parked window", pf)
	}
	w.f.SetErr("Hide", errors.New("no"))
	w.f.AddApp(finderID, "Finder")
	res := w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}})
	if res.FileErr != nil || len(res.Errors) != 1 {
		t.Errorf("a failed Hide: FileErr %v, errors %v", res.FileErr, res.Errors)
	}
}

func TestApplyHidesAnAppItShowedAMomentAgo(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	w.window(finderID, "Downloads", 0)
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}}))
	// The engine's snapshot of the app, made before macOS updated its
	// Hidden flag after the next Unhide, still says it's hidden.
	stale := w.app(finderID)
	noErrors(t, w.m.Apply(Plan{Unhide: []platform.App{stale}}))
	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{stale}}))
	if got := w.f.CallsTo("Hide"); len(got) != 1 {
		t.Errorf("Hide calls = %v, want one, whatever the stale flag says", callNames(got))
	}
	if !w.f.IsHidden(finderID) || !w.m.Hid(stale) {
		t.Errorf("hidden %v, recorded %v; want both", w.f.IsHidden(finderID), w.m.Hid(stale))
	}

	// Later, the flag can be trusted again.
	w.clk.Advance(2 * time.Second)
	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}}))
	if got := w.f.CallsTo("Hide"); len(got) != 0 {
		t.Errorf("Hide calls for an app that's hidden = %v, want none", callNames(got))
	}
}

func TestApplyHidesAnAppThatRestoreCrashedShowed(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(finderID)}}))
	// Bench crashes. The next run shows Finder, then reads the running
	// apps before macOS has updated Finder's Hidden flag.
	stale := w.app(finderID)
	m := New(w.f, w.clk, w.path)
	if err := m.RestoreCrashed(); err != nil {
		t.Fatal(err)
	}
	if w.f.IsHidden(finderID) {
		t.Fatal("RestoreCrashed didn't show Finder")
	}
	noErrors(t, m.Apply(Plan{Hide: []platform.App{stale}}))
	if !w.f.IsHidden(finderID) || !m.Hid(stale) {
		t.Errorf("hidden %v, recorded %v; want both, whatever the stale flag says", w.f.IsHidden(finderID), m.Hid(stale))
	}
}

func TestApplyKeepsTheRecordOfAParkThatFailed(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKept bool
	}{
		// On a Mac, an app that takes over a second to answer gives an
		// error, but may still make the move.
		{"the app didn't answer in time", errors.New("the app didn't respond"), true},
		{"the window has closed", platform.ErrNoWindow, false},
		{"the window is on another desktop", platform.ErrOtherDesktop, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			win := w.window(braveID, "docs", 0)
			w.f.SetWindowErr(win.ID, "Park", tt.err)
			res := w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}})
			if len(res.Errors) > 1 {
				t.Errorf("Apply errors = %v", res.Errors)
			}
			if got := w.m.IsParked(win.ID); got != tt.wantKept {
				t.Errorf("IsParked = %v, want %v", got, tt.wantKept)
			}
			pf, _ := w.parkedFile(t)
			if got := len(pf.Windows) == 1; got != tt.wantKept {
				t.Errorf("parked file = %+v, want the window recorded: %v", pf, tt.wantKept)
			}
			if !tt.wantKept {
				return
			}
			// The app makes the move after all, then Bench crashes.
			w.f.SetWindowErr(win.ID, "Park", nil)
			w.f.AppMoveWindow(win.ID, parkedAt(win, monitor))
			w.m = nil
			if err := RestoreFromFile(w.f, w.path); err != nil {
				t.Fatal(err)
			}
			if got := w.frame(t, win.ID); got != startFrame {
				t.Errorf("after the crash, the window is at %+v, want it back at %+v", got, startFrame)
			}
		})
	}
}

func TestPuttingBackAWindowInFullScreen(t *testing.T) {
	park := func(t *testing.T) (*world, platform.Window) {
		w := newWorld(t)
		win := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{win}}))
		// The user brings the parked window forward and makes it full
		// screen, where it reports the screen's frame.
		w.f.SetFullScreen(win.ID, true)
		return w, win
	}
	// leave takes the window out of full screen, where macOS gives it
	// back its parked frame, and checks that the record put it back.
	leave := func(t *testing.T, w *world, win platform.Window, putBack func() error) {
		t.Helper()
		w.f.SetFullScreen(win.ID, false)
		if got, want := w.frame(t, win.ID), parkedAt(win, monitor); got != want {
			t.Fatalf("out of full screen, the window is at %+v, want the parked frame %+v", got, want)
		}
		if err := putBack(); err != nil {
			t.Fatal(err)
		}
		if got := w.frame(t, win.ID); got != startFrame {
			t.Errorf("the window is at %+v, want it back at %+v", got, startFrame)
		}
	}

	t.Run("Unpark and RestoreAll", func(t *testing.T) {
		w, win := park(t)
		if err := w.m.Unpark(win.ID); !errors.Is(err, errFullScreen) || !w.m.IsParked(win.ID) {
			t.Errorf("Unpark: %v, parked %v; want errFullScreen, and still parked", err, w.m.IsParked(win.ID))
		}
		// Apply tries again each time, quietly.
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Unpark: []platform.Window{win}}))
		if err := w.m.RestoreAll(); !errors.Is(err, errFullScreen) {
			t.Errorf("RestoreAll: %v, want errFullScreen", err)
		}
		if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 {
			t.Errorf("parked file = %+v, want the window still recorded", pf)
		}
		leave(t, w, win, w.m.RestoreAll)
	})

	t.Run("after a crash", func(t *testing.T) {
		w, win := park(t)
		w.m = nil
		if err := RestoreFromFile(w.f, w.path); !errors.Is(err, errFullScreen) {
			t.Errorf("RestoreFromFile: %v, want errFullScreen", err)
		}
		if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 {
			t.Errorf("parked file = %+v, want the window still recorded", pf)
		}
		leave(t, w, win, func() error { return RestoreFromFile(w.f, w.path) })
	})
}

func TestRestoreFromFileWaitsForAppsItShows(t *testing.T) {
	// stubSleep replaces the restorer's sleep, calling then on each wait.
	stubSleep := func(t *testing.T, then func(n int)) *int {
		n := 0
		restoreSleep = func(time.Duration) {
			n++
			then(n)
		}
		t.Cleanup(func() { restoreSleep = time.Sleep })
		return &n
	}
	setUp := func(t *testing.T) (*world, platform.Window) {
		w := newWorld(t)
		code := w.window(vscodeID, "tab B", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{code}}))
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{w.app(vscodeID)}}))
		// Bench crashes. To a fresh process, the hidden app's window
		// looks as if it's on another desktop until the app has shown.
		w.m = nil
		w.f.SetWindowErr(code.ID, "Window", platform.ErrOtherDesktop)
		return w, code
	}

	t.Run("the window turns up", func(t *testing.T) {
		w, code := setUp(t)
		waits := stubSleep(t, func(n int) {
			if n == 3 {
				w.f.SetWindowErr(code.ID, "Window", nil)
			}
		})
		if err := RestoreFromFile(w.f, w.path); err != nil {
			t.Fatal(err)
		}
		if got := w.frame(t, code.ID); got != startFrame || *waits != 3 {
			t.Errorf("window at %+v after %d waits, want %+v after 3", got, *waits, startFrame)
		}
	})

	t.Run("the window stays away", func(t *testing.T) {
		w, code := setUp(t)
		waits := stubSleep(t, func(int) {})
		if err := RestoreFromFile(w.f, w.path); !errors.Is(err, platform.ErrOtherDesktop) {
			t.Errorf("RestoreFromFile: %v, want ErrOtherDesktop", err)
		}
		if *waits != showPolls {
			t.Errorf("waited %d times, want %d", *waits, showPolls)
		}
		if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 || pf.Windows[0].ID != code.ID {
			t.Errorf("parked file = %+v, want the window kept", pf)
		}
	})

	t.Run("an app it didn't show isn't waited for", func(t *testing.T) {
		w := newWorld(t)
		docs := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}}))
		w.m = nil
		w.f.MoveToOtherDesktop(docs.ID)
		waits := stubSleep(t, func(int) {})
		if err := RestoreFromFile(w.f, w.path); !errors.Is(err, platform.ErrOtherDesktop) {
			t.Errorf("RestoreFromFile: %v, want ErrOtherDesktop", err)
		}
		if *waits != 0 {
			t.Errorf("waited %d times, want none", *waits)
		}
	})
}

func TestRestoreFromFileKeepsAWindowItCouldntLookUp(t *testing.T) {
	w := newWorld(t)
	inbox := w.window(braveID, "Inbox (3)", 0)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{inbox}}))
	// Bench crashes, the title changes, and looking the window up by ID
	// times out.
	w.m = nil
	w.f.RetitleWindow(inbox.ID, "Inbox (4)")
	timeout := errors.New("the app didn't respond")
	w.f.SetWindowErr(inbox.ID, "Window", timeout)
	if err := RestoreFromFile(w.f, w.path); !errors.Is(err, timeout) {
		t.Errorf("RestoreFromFile: %v, want the lookup's error", err)
	}
	if pf, _ := w.parkedFile(t); len(pf.Windows) != 1 || pf.Windows[0].ID != inbox.ID {
		t.Fatalf("parked file = %+v, want the window kept", pf)
	}
	w.f.SetWindowErr(inbox.ID, "Window", nil)
	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	if got := w.frame(t, inbox.ID); got != startFrame {
		t.Errorf("window at %+v, want it back at %+v", got, startFrame)
	}
}

func TestApplyMinimisesWhenNowhereToParkIsClear(t *testing.T) {
	// A laptop between two taller monitors, which reach below it, and a
	// window wider than the laptop: wherever it's parked below the laptop,
	// part of it shows on a monitor.
	left := platform.Screen{ID: "left", Frame: layout.Rect{X: -2560, Y: -229, W: 2560, H: 1440}, Visible: layout.Rect{X: -2560, Y: -204, W: 2560, H: 1415}}
	right := platform.Screen{ID: "right", Frame: layout.Rect{X: 1512, Y: -229, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: -204, W: 2560, H: 1415}}
	setUp := func(t *testing.T) (*world, platform.Window, platform.Window) {
		w := newWorld(t)
		w.f.SetScreens(laptop, left, right)
		wide := w.f.AddWindow(braveID, "wide", platform.WithFrame(layout.Rect{X: 0, Y: 33, W: 1600, H: 800}))
		narrow := w.f.AddWindow(braveID, "narrow", platform.WithFrame(layout.Rect{X: 100, Y: 100, W: 1000, H: 800}))
		noErrors(t, w.m.Apply(Plan{Screen: laptop, Park: []platform.Window{wide, narrow}}))
		return w, wide, narrow
	}

	t.Run("parking", func(t *testing.T) {
		w, wide, narrow := setUp(t)
		got, _ := w.f.WindowState(wide.ID)
		if !got.Minimised || got.Frame != wide.Frame {
			t.Errorf("the wide window is %+v, want it minimised where it was", got)
		}
		if got := w.frame(t, narrow.ID); got != (layout.Rect{X: 512, Y: 981, W: 1000, H: 800}) {
			t.Errorf("the narrow window is parked at %+v, want below the laptop, clear of the monitors", got)
		}
		pf, _ := w.parkedFile(t)
		if len(pf.Windows) != 2 || !pf.Windows[0].Minimised || pf.Windows[1].Minimised {
			t.Errorf("parked file = %+v, want both, the wide one minimised", pf)
		}
		// The engine asks, to tell this from the user minimising it.
		if !w.m.Minimised(wide.ID) || w.m.Minimised(narrow.ID) {
			t.Errorf("Minimised: wide %v, narrow %v; want true, false", w.m.Minimised(wide.ID), w.m.Minimised(narrow.ID))
		}
		w.f.ResetCalls()
		noErrors(t, w.m.Apply(Plan{Screen: laptop, Park: []platform.Window{wide, narrow}}))
		if got := w.f.CallsTo("Park", "Minimise", "SetFrame"); len(got) != 0 {
			t.Errorf("parking again made calls %v", callNames(got))
		}
	})

	t.Run("Unpark", func(t *testing.T) {
		w, wide, _ := setUp(t)
		if err := w.m.Unpark(wide.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := w.f.WindowState(wide.ID); got.Minimised || got.Frame != wide.Frame || w.m.IsParked(wide.ID) || w.m.Minimised(wide.ID) {
			t.Errorf("after Unpark, the window is %+v, parked %v; want it shown where it was", got, w.m.IsParked(wide.ID))
		}
	})

	t.Run("after a crash", func(t *testing.T) {
		w, wide, narrow := setUp(t)
		w.m = nil
		if err := RestoreFromFile(w.f, w.path); err != nil {
			t.Fatal(err)
		}
		if got, _ := w.f.WindowState(wide.ID); got.Minimised {
			t.Error("the wide window is still minimised")
		}
		if got := w.frame(t, narrow.ID); got != narrow.Frame {
			t.Errorf("the narrow window is at %+v, want %+v", got, narrow.Frame)
		}
	})

	// Before Apply shows a hidden app, it parks only the windows that have
	// a place to park: minimising a hidden app's window animates, and may
	// show the app, so the wide window waits until the app is shown, as
	// it always did (decision 67).
	t.Run("before showing an app", func(t *testing.T) {
		w := newWorld(t)
		w.f.SetScreens(laptop, left, right)
		wide := w.f.AddWindow(braveID, "wide", platform.WithFrame(layout.Rect{X: 0, Y: 33, W: 1600, H: 800}))
		narrow := w.f.AddWindow(braveID, "narrow", platform.WithFrame(layout.Rect{X: 100, Y: 100, W: 1000, H: 800}))
		noErrors(t, w.m.Apply(Plan{Screen: laptop, Hide: []platform.App{w.app(braveID)}}))
		w.f.ResetCalls()
		noErrors(t, w.m.Apply(Plan{Screen: laptop, Unhide: []platform.App{w.app(braveID)}, Park: []platform.Window{wide, narrow}}))
		want := []string{fmt.Sprintf("Park w%d", narrow.ID), "Unhide " + braveID, fmt.Sprintf("Minimise w%d", wide.ID)}
		if got := moveOrder(w.f.Calls()); !slices.Equal(got, want) {
			t.Errorf("calls = %v, want %v", got, want)
		}
		if got, _ := w.f.WindowState(wide.ID); !got.Minimised {
			t.Errorf("the wide window is %+v, want it minimised", got)
		}
	})

	t.Run("an inactive window too big for its pane stays minimised", func(t *testing.T) {
		w := newWorld(t)
		w.f.SetScreens(laptop, left, right)
		body := layout.Rect{X: 8, Y: 41, W: 700, H: 900}
		shown := w.f.AddWindow(zedID, "bench")
		big := w.f.AddWindow(braveID, "big", platform.WithMinSize(1600, 800))
		plan := Plan{Screen: laptop, Stacks: []Stack{{Body: body, Windows: []platform.Window{shown, big}, Shown: shown.ID}}}
		res := w.m.Apply(plan)
		noErrors(t, res)
		if got, _ := w.f.WindowState(big.ID); !got.Minimised || res.Status[big.ID] != StatusNeedsRoom {
			t.Fatalf("the big window is %+v, status %q; want it minimised, needing room", got, res.Status[big.ID])
		}
		w.f.ResetCalls()
		res = w.m.Apply(plan)
		noErrors(t, res)
		if got := w.f.CallsTo("Unminimise", "Minimise", "SetFrame"); len(got) != 0 || res.Status[big.ID] != StatusNeedsRoom {
			t.Errorf("arranging again made calls %v, status %q; want none, needing room", callNames(got), res.Status[big.ID])
		}
	})

	t.Run("a window the user minimised stays minimised", func(t *testing.T) {
		w := newWorld(t)
		docs := w.window(braveID, "docs", 0)
		noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}}))
		w.f.MinimiseWindow(docs.ID)
		if w.m.Minimised(docs.ID) {
			t.Error("Minimised says the manager minimised a window the user did")
		}
		if err := w.m.Unpark(docs.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := w.f.WindowState(docs.ID); !got.Minimised {
			t.Error("Unpark unminimised a window the manager parked, not minimised")
		}
	})
}

func TestApplyWritesTheParkedFileAgainIfItHasGone(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	docs := w.window(braveID, "docs", 0)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}, Hide: []platform.App{w.app(finderID)}}))
	// bench restore, run while Bench runs, puts everything back and
	// removes the file; Bench then parks and hides them again.
	if err := RestoreFromFile(w.f, w.path); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.parkedFile(t); ok {
		t.Fatal("RestoreFromFile left the file")
	}
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}, Hide: []platform.App{w.app(finderID)}}))
	pf, ok := w.parkedFile(t)
	if !ok || len(pf.Windows) != 1 || len(pf.Hidden) != 1 {
		t.Errorf("parked file = %+v (exists %v), want the window and Finder again", pf, ok)
	}
	if w.frame(t, docs.ID) != parkedAt(docs, monitor) || !w.f.IsHidden(finderID) {
		t.Error("the window and Finder weren't put out of sight again")
	}
}

// moveOrder returns the Park, Minimise, Unhide, and SetFrame calls in
// turn, each as its method and window ID or bundle ID, such as "Park w2".
func moveOrder(calls []platform.Call) []string {
	var out []string
	for _, c := range calls {
		switch c.Method {
		case "Park", "Minimise", "SetFrame":
			out = append(out, fmt.Sprintf("%s w%d", c.Method, c.Window))
		case "Unhide":
			out = append(out, "Unhide "+c.BundleID)
		}
	}
	return out
}

// Showing an app shows every window it has where it is, so its windows
// that are to go out of sight are parked first: those of other tabs
// (Park), and those of a pane that shows Bench's own view (Shown == 0).
// Otherwise they would show over the tab, where another tab docked them,
// until the rest of the plan parked them (decision 67).
func TestApplyParksBeforeShowingAnApp(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "Wails v3 docs", 0, platform.WithFrame(leftBody))
	pr := w.window(braveID, "Pull request #42 · bench", 1)
	pkgs := w.window(braveID, "Go packages", 2, platform.WithFrame(rightBody))
	zed := w.window(zedID, "bench", 3)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{w.app(braveID)}}))
	if !w.f.IsHidden(braveID) {
		t.Fatal("Brave wasn't hidden")
	}

	w.f.ResetCalls()
	res := w.m.Apply(Plan{
		Screen: monitor,
		Unhide: []platform.App{w.app(braveID), w.app(zedID)},
		Stacks: []Stack{
			{Body: leftBody, Windows: []platform.Window{pr}, Shown: pr.ID},
			{Body: rightBody, Windows: []platform.Window{pkgs}},
			{Body: rightBody, Windows: []platform.Window{zed}, Shown: zed.ID},
		},
		Park: []platform.Window{docs},
	})
	noErrors(t, res)
	want := []string{
		fmt.Sprintf("Park w%d", docs.ID), fmt.Sprintf("Park w%d", pkgs.ID),
		"Unhide " + braveID,
		fmt.Sprintf("SetFrame w%d", pr.ID), fmt.Sprintf("SetFrame w%d", zed.ID),
	}
	if got := moveOrder(w.f.Calls()); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	for _, win := range []platform.Window{docs, pkgs} {
		if !w.m.IsParked(win.ID) || w.frame(t, win.ID) != parkedAt(win, monitor) {
			t.Errorf("%q: parked %v at %v", win.Title, w.m.IsParked(win.ID), w.frame(t, win.ID))
		}
	}
	// Zed wasn't hidden, so there was nothing to park before showing it.
	// Each window the plan changed counts once: the two parked, and the two
	// docked and raised.
	if res.Touched != 4 {
		t.Errorf("Touched = %d, want 4", res.Touched)
	}
	if got := res.Status[pkgs.ID]; got != StatusReady {
		t.Errorf("the parked stack's window has status %q, want ready", got)
	}
}

// Where the Accessibility API won't move a hidden app's windows, the
// windows are parked once the app is shown, as Apply always did, and the
// failure before isn't reported. One that the platform said it moved, but
// didn't, is parked then too.
func TestApplyParksAfterShowingWhereItCant(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"the move fails", errors.New("the app didn't do it")},
		{"the move is said to be made, but isn't", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			docs := w.window(braveID, "Wails v3 docs", 0, platform.WithFrame(leftBody))
			pr := w.window(braveID, "Pull request #42 · bench", 1)
			pkgs := w.window(braveID, "Go packages", 2, platform.WithFrame(rightBody))
			noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{w.app(braveID)}}))
			w.f.FixHiddenWindows(true, tt.err)

			w.f.ResetCalls()
			res := w.m.Apply(Plan{
				Screen: monitor,
				Unhide: []platform.App{w.app(braveID)},
				Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{pr}, Shown: pr.ID}},
				Park:   []platform.Window{docs, pkgs},
			})
			noErrors(t, res)
			// The first failure leaves the app's other windows for after.
			want := []string{fmt.Sprintf("Park w%d", docs.ID)}
			if tt.err == nil {
				want = append(want, fmt.Sprintf("Park w%d", pkgs.ID))
			}
			want = append(want, "Unhide "+braveID, fmt.Sprintf("SetFrame w%d", pr.ID),
				fmt.Sprintf("Park w%d", docs.ID), fmt.Sprintf("Park w%d", pkgs.ID))
			if got := moveOrder(w.f.Calls()); !slices.Equal(got, want) {
				t.Errorf("calls = %v, want %v", got, want)
			}
			for _, win := range []platform.Window{docs, pkgs} {
				if !w.m.IsParked(win.ID) || w.frame(t, win.ID) != parkedAt(win, monitor) {
					t.Errorf("%q: parked %v at %v", win.Title, w.m.IsParked(win.ID), w.frame(t, win.ID))
				}
			}
		})
	}
}

// A window that can't be read before its app is shown is left for after,
// and isn't forgotten: a hidden app's window may answer only once it's
// shown.
func TestApplyKeepsAWindowItCouldntReadBeforeShowing(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "Wails v3 docs", 0, platform.WithFrame(leftBody))
	pr := w.window(braveID, "Pull request #42 · bench", 1)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{w.app(braveID)}}))
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Unhide: []platform.App{w.app(braveID)}, Park: []platform.Window{docs}}))
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Unpark: []platform.Window{docs}, Hide: []platform.App{w.app(braveID)}}))
	if w.m.IsParked(docs.ID) || !w.f.IsHidden(braveID) {
		t.Fatalf("set-up: docs parked %v, Brave hidden %v", w.m.IsParked(docs.ID), w.f.IsHidden(braveID))
	}
	w.f.SetWindowErr(docs.ID, "Window", errors.New("no answer"))
	w.f.ResetCalls()
	res := w.m.Apply(Plan{
		Screen: monitor,
		Unhide: []platform.App{w.app(braveID)},
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{pr}, Shown: pr.ID}},
		Park:   []platform.Window{docs},
	})
	// After the app is shown, the read fails again, and that's reported,
	// as it always was.
	if len(res.Errors) != 1 {
		t.Errorf("errors = %v, want the one read after showing the app", res.Errors)
	}
	if got := moveOrder(w.f.Calls()); !slices.Equal(got, []string{"Unhide " + braveID, fmt.Sprintf("SetFrame w%d", pr.ID)}) {
		t.Errorf("calls = %v", got)
	}
	w.f.SetWindowErr(docs.ID, "Window", nil)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs}}))
	if !w.m.IsParked(docs.ID) {
		t.Error("the window was forgotten")
	}
}

// A window that has closed, or is on another desktop, before its app is
// shown doesn't stop the app's other windows from being parked first, as
// a failure that may be a slow app does: it isn't slow to answer.
func TestApplyParksPastAGoneWindowBeforeShowing(t *testing.T) {
	for _, tt := range []struct {
		name, method string
		err          error
	}{
		{"it can't be read, as it has closed", "Window", platform.ErrNoWindow},
		{"it can't be read, as it's on another desktop", "Window", platform.ErrOtherDesktop},
		{"it can't be parked, as it has closed", "Park", platform.ErrNoWindow},
		{"it can't be parked, as it's on another desktop", "Park", platform.ErrOtherDesktop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			docs := w.window(braveID, "Wails v3 docs", 0, platform.WithFrame(leftBody))
			pr := w.window(braveID, "Pull request #42 · bench", 1)
			pkgs := w.window(braveID, "Go packages", 2, platform.WithFrame(rightBody))
			noErrors(t, w.m.Apply(Plan{Screen: monitor, Hide: []platform.App{w.app(braveID)}}))
			w.f.SetWindowErr(docs.ID, tt.method, fmt.Errorf("window %d: %w", docs.ID, tt.err))
			w.f.ResetCalls()
			w.m.Apply(Plan{
				Screen: monitor,
				Unhide: []platform.App{w.app(braveID)},
				Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{pr}, Shown: pr.ID}},
				Park:   []platform.Window{docs, pkgs},
			})
			got := moveOrder(w.f.Calls())
			unhide := slices.Index(got, "Unhide "+braveID)
			if park := slices.Index(got, fmt.Sprintf("Park w%d", pkgs.ID)); unhide < 0 || park < 0 || park > unhide {
				t.Errorf("calls = %v, want the other window parked before Brave shows", got)
			}
			if !w.m.IsParked(pkgs.ID) || w.frame(t, pkgs.ID) != parkedAt(pkgs, monitor) {
				t.Errorf("the other window: parked %v at %v", w.m.IsParked(pkgs.ID), w.frame(t, pkgs.ID))
			}
		})
	}
}

// An app that's shown already has nothing parked before it: the plan
// parks other tabs' windows after docking, as it always did.
func TestApplyParksAfterDockingForAnAppThatsShown(t *testing.T) {
	w := newWorld(t)
	docs := w.window(braveID, "Wails v3 docs", 0)
	pr := w.window(braveID, "Pull request #42 · bench", 1)
	w.f.ResetCalls()
	noErrors(t, w.m.Apply(Plan{
		Screen: monitor,
		Unhide: []platform.App{w.app(braveID)},
		Stacks: []Stack{{Body: leftBody, Windows: []platform.Window{pr}, Shown: pr.ID}},
		Park:   []platform.Window{docs},
	}))
	want := []string{fmt.Sprintf("SetFrame w%d", pr.ID), fmt.Sprintf("Park w%d", docs.ID)}
	if got := moveOrder(w.f.Calls()); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}
