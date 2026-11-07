package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// These tests check decision 67: Bench times each arrangement of the
// current tab, from what asked for it to the last frame set, and keeps the
// latest times for Report a Problem and the log; a switch parks what is to
// go out of sight before it shows an app; and a tab's shortcut shows the
// new tab as current at once.

// ringOf returns the times a ring keeps, oldest first.
func ringOf(r *arrangeTimes) []arrangeTime {
	out := make([]arrangeTime, 0, r.n)
	for i := range r.n {
		out = append(out, r.ring[(r.next-r.n+i+timedArranges)%timedArranges])
	}
	return out
}

// timesMark is how many times each of the engine's rings keeps, the tab
// switches' and the others', to tell the times kept after it (timesSince).
type timesMark struct{ switches, others int }

// markTimes returns how many times the engine keeps now.
func markTimes(h *harness) timesMark {
	h.e.lock()
	defer h.e.unlock()
	return timesMark{h.e.switches.n, h.e.arranges.n}
}

// timesSince returns the times the engine kept after mark: the tab
// switches', oldest first, then the other arrangements'.
func timesSince(h *harness, mark timesMark) []arrangeTime {
	h.e.lock()
	defer h.e.unlock()
	return append(ringOf(&h.e.switches)[mark.switches:], ringOf(&h.e.arranges)[mark.others:]...)
}

// newArrangeTime returns the first time the engine kept after mark,
// failing the test if it kept none.
func newArrangeTime(h *harness, mark timesMark) arrangeTime {
	h.t.Helper()
	ts := timesSince(h, mark)
	if len(ts) == 0 {
		h.t.Fatal("no new arrangement was timed")
	}
	return ts[0]
}

// arrangeReasons returns the reasons of the times kept after mark: the tab
// switches' first.
func arrangeReasons(h *harness, mark timesMark) []string {
	var out []string
	for _, t := range timesSince(h, mark) {
		out = append(out, t.reason)
	}
	return out
}

// TestArrangeTimesKeepsTheLatest checks the ring of times: it keeps the
// latest timedArranges, in place of the oldest, and sums them up by
// reason, with the median, the worst, and the most windows changed.
func TestArrangeTimesKeepsTheLatest(t *testing.T) {
	var r arrangeTimes
	if got := r.summary(); len(got) != 0 {
		t.Errorf("an empty ring sums up as %+v", got)
	}
	// The oldest ten, which the ring lets go of, are the slowest.
	for i := range 10 {
		r.add(arrangeTime{reason: "old", took: time.Hour, windows: 100 + i})
	}
	for i := range 30 {
		r.add(arrangeTime{reason: reasonTab, took: time.Duration(i+1) * time.Millisecond, windows: i % 5})
	}
	for _, ms := range []int{7, 3, 5, 1} {
		r.add(arrangeTime{reason: "bench-moved", took: time.Duration(ms) * time.Millisecond, windows: 2})
	}
	for range 16 {
		r.add(arrangeTime{reason: reasonRearrange, took: 4 * time.Millisecond})
	}
	if r.n != timedArranges {
		t.Errorf("the ring holds %d, want %d", r.n, timedArranges)
	}
	want := []ArrangeTiming{
		{Reason: reasonTab, Count: 30, Median: 15500 * time.Microsecond, Worst: 30 * time.Millisecond, Windows: 4},
		{Reason: reasonRearrange, Count: 16, Median: 4 * time.Millisecond, Worst: 4 * time.Millisecond},
		{Reason: "bench-moved", Count: 4, Median: 4 * time.Millisecond, Worst: 7 * time.Millisecond, Windows: 2},
	}
	if got := r.summary(); !slices.Equal(got, want) {
		t.Errorf("summary:\n got %+v\nwant %+v", got, want)
	}

	// An odd count has a middle one; reasons with as many are in order of
	// name.
	var s arrangeTimes
	for _, ms := range []int{9, 2, 4} {
		s.add(arrangeTime{reason: "window-moved", took: time.Duration(ms) * time.Millisecond, windows: 1})
		s.add(arrangeTime{reason: "apps-opened", took: time.Duration(ms) * time.Millisecond, windows: 3})
	}
	want = []ArrangeTiming{
		{Reason: "apps-opened", Count: 3, Median: 4 * time.Millisecond, Worst: 9 * time.Millisecond, Windows: 3},
		{Reason: "window-moved", Count: 3, Median: 4 * time.Millisecond, Worst: 9 * time.Millisecond, Windows: 1},
	}
	if got := s.summary(); !slices.Equal(got, want) {
		t.Errorf("summary:\n got %+v\nwant %+v", got, want)
	}
}

// TestArrangeTimingReasons checks the reason that each kind of request
// gives the arrangement it makes, and the windows it counts.
func TestArrangeTimingReasons(t *testing.T) {
	h := newHarness(t, setup{})
	before := markTimes(h)

	mustOK(t, h.e.SwitchTab(1))
	if got := newArrangeTime(h, before); got.reason != reasonTab || got.windows == 0 {
		t.Errorf("SwitchTab's time: %+v, want reason tab and the windows it changed", got)
	}
	// What only reads the state, as the View, isn't timed.
	n := markTimes(h)
	_ = h.view()
	_ = h.e.ProblemReport()
	if got := timesSince(h, n); len(got) != 0 {
		t.Errorf("reading the view kept %d times", len(got))
	}

	cases := []struct {
		name   string
		do     func()
		reason string
	}{
		{"StepTab", func() { mustOK(t, h.e.StepTab(1)) }, reasonTab},
		{"a tab's shortcut", func() { h.e.Shortcut(shortcuts.ActionTab, "2") }, reasonTabShortcut},
		{"next_tab's shortcut", func() { h.e.Shortcut(shortcuts.ActionNextTab, "") }, reasonTabShortcut},
		{"Rearrange", func() { mustOK(t, h.e.Rearrange()) }, reasonRearrange},
		{"rearrange's shortcut", func() { h.e.Shortcut(shortcuts.ActionRearrange, "") }, reasonRearrange},
		{"a new pane area", func() {
			area := paneArea
			area.W, area.H = contentFrame.W-40, contentFrame.H-75
			mustOK(t, h.e.SetPaneArea(area))
		}, reasonPaneArea},
		{"Bench's window moved", func() {
			moved := contentFrame
			moved.X += 100
			moved.W -= 100
			h.p.SetContentFrame(moved)
			h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
			h.drain()
		}, string(platform.BenchMoved)},
		{"another change", func() { mustOK(t, h.e.SetMode("compact")) }, reasonOther},
	}
	for _, c := range cases {
		n := markTimes(h)
		c.do()
		if got := arrangeReasons(h, n); len(got) == 0 || got[0] != c.reason {
			t.Errorf("%s: reasons %v, want %s first", c.name, got, c.reason)
		}
	}

	// The end of the wait for the screens to settle has its own reason,
	// and the screens' change, which only starts the wait, isn't timed.
	n = markTimes(h)
	h.p.SetScreens(laptop)
	h.p.SetContentFrame(laptop.Visible)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.drain()
	if got := arrangeReasons(h, n); len(got) != 0 {
		t.Errorf("the screens' change itself: %v, want nothing timed yet", got)
	}
	h.advance(2 * time.Second)
	if got := arrangeReasons(h, n); !slices.Contains(got, reasonSettle) {
		t.Errorf("after the screens settled: %v, want %s", got, reasonSettle)
	}
}

// TestArrangeTimingWaitsForApps checks that a switch that waits for the
// apps it opened times the arrangement at the end of the wait as one of
// apps-opened.
func TestArrangeTimingWaitsForApps(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(time.Second), platform.AppLaunchWindows("Calendar"))
		p.InstallApp(safariID, "Safari", platform.AppLaunchWindows("Apple"))
	}})
	n := markTimes(h)
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	got := arrangeReasons(h, n)
	if len(got) < 2 || got[0] != reasonTab || !slices.Contains(got[1:], reasonAppsOpened) {
		t.Errorf("reasons %v, want tab and then apps-opened", got)
	}
}

// waitClock is a fake clock whose Now runs ahead of it by ahead, and which
// says on read when Now is first read after armed is set, so that a test
// knows when a request has noted the time it was asked for.
type waitClock struct {
	*clock.Fake
	ahead atomic.Int64
	armed atomic.Bool
	read  chan struct{}
}

func (c *waitClock) Now() time.Time {
	if c.armed.CompareAndSwap(true, false) {
		c.read <- struct{}{}
	}
	return c.Fake.Now().Add(time.Duration(c.ahead.Load()))
}

// TestArrangeTimingCountsTheWaitForTheLock checks that an arrangement is
// timed from when it was asked for, so that the time it waited for the
// engine's lock, as another request held it, counts, and that the time is
// how long it took by the clock.
func TestArrangeTimingCountsTheWaitForTheLock(t *testing.T) {
	var clk *waitClock
	h := newHarness(t, setup{options: func(o *Options) {
		clk = &waitClock{Fake: o.Clock.(*clock.Fake), read: make(chan struct{}, 1)}
		o.Clock = clk
	}})
	n := markTimes(h)
	const wait = 250 * time.Millisecond
	h.e.lock()
	clk.armed.Store(true)
	var wg sync.WaitGroup
	var err error
	wg.Go(func() { err = h.e.SwitchTab(1) })
	<-clk.read
	clk.ahead.Add(int64(wait))
	h.e.unlock()
	wg.Wait()
	mustOK(t, err)
	if got := newArrangeTime(h, n); got.reason != reasonTab || got.took != wait {
		t.Errorf("the switch's time: %+v, want %s for the wait", got, wait)
	}
}

// TestArrangeTimingLogsOneLine checks the debug line that each timed
// arrangement logs, with BENCH_DEBUG.
func TestArrangeTimingLogsOneLine(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	h := newHarness(t, setup{options: func(o *Options) {
		o.Log = slog.New(slog.NewTextHandler(&lockedWriter{w: &buf, mu: &mu}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}})
	mu.Lock()
	buf.Reset()
	mu.Unlock()
	n := markTimes(h)
	mustOK(t, h.e.SwitchTab(1))
	last := newArrangeTime(h, n)
	mu.Lock()
	text := buf.String()
	mu.Unlock()
	var lines []string
	for line := range strings.Lines(text) {
		if strings.Contains(line, "msg=arranged") {
			lines = append(lines, line)
		}
	}
	want := fmt.Sprintf("msg=arranged reason=tab ms=0 windows=%d\n", last.windows)
	if len(lines) != 1 || !strings.HasSuffix(lines[0], want) {
		t.Errorf("logged %q, want one line ending %q", lines, want)
	}
}

// lockedWriter writes to w with mu held, as a logger may write from more
// than one goroutine.
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// TestProblemReportHasArrangeTimes checks that Report a Problem gets the
// summed-up times, with the tab switches' apart from the others', so that
// dragging Bench's window, which arranges once a frame, doesn't push the
// switches out.
func TestProblemReportHasArrangeTimes(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(2))
	h.e.Shortcut(shortcuts.ActionTab, "1")
	// Under a second of a drag, at 60 frames a second.
	for i := range 60 {
		moved := contentFrame
		moved.X += float64(i + 1)
		moved.W -= float64(i + 1)
		h.p.SetContentFrame(moved)
		h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
		h.drain()
	}
	r := h.e.ProblemReport()
	count := func(times []ArrangeTiming, reason string) int {
		if i := slices.IndexFunc(times, func(a ArrangeTiming) bool { return a.Reason == reason }); i >= 0 {
			return times[i].Count
		}
		return 0
	}
	if count(r.Switching, reasonTab) != 2 || count(r.Switching, reasonTabShortcut) != 1 || r.Switching[0].Windows == 0 {
		t.Errorf("the report's switches: %+v, want two for tab and one for tab-shortcut", r.Switching)
	}
	if count(r.Arranging, string(platform.BenchMoved)) != timedArranges {
		t.Errorf("the report's other times: %+v, want the latest %d for bench-moved", r.Arranging, timedArranges)
	}
	for _, a := range r.Arranging {
		if isSwitch(a.Reason) {
			t.Errorf("a switch's times with the others: %+v", a)
		}
	}
}

// TestSwitchParksBeforeShowingAnApp checks that a switch to a tab with a
// window of a hidden app parks that app's windows of other tabs before it
// shows the app, so that they don't show over the tab for a moment (the
// flash), and that they end up parked.
func TestSwitchParksBeforeShowingAnApp(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	docs := h.windowID(braveID, "Wails v3 docs")
	pr := h.windowID(braveID, "Pull request #42 · bench")
	pkgs := h.windowID(braveID, "Go packages")
	// Research shows one Brave window, with its other two in sub-tabs of
	// the same pane; Go packages goes to Code.
	mustOK(t, h.e.MoveItemToTab(h.itemOf(pkgs), 1))
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	if !h.p.IsHidden(braveID) {
		t.Fatal("Brave should be hidden on Comms")
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	calls := h.calls("Park", "SetFrame", "Unhide")
	unhide := slices.IndexFunc(calls, func(c platform.Call) bool { return c.Method == "Unhide" && c.BundleID == braveID })
	if unhide < 0 {
		t.Fatalf("Brave wasn't shown: %v", calls)
	}
	for _, w := range []platform.WindowID{docs, pr} {
		park := slices.IndexFunc(calls, func(c platform.Call) bool { return c.Method == "Park" && c.Window == w })
		if park < 0 || park > unhide {
			t.Errorf("window %d parked at %d, Brave shown at %d: %v", w, park, unhide, calls)
		}
		if !parkedNow(h, w) {
			t.Errorf("window %d isn't parked: %v", w, h.frame(w))
		}
	}
	if f := h.frame(pkgs); f != h.bodyOf(h.paneOf(h.itemOf(pkgs))) {
		t.Errorf("Go packages at %v, want its pane's body", f)
	}
}

// parkedNow says if a window is parked, and in the corner where Bench
// parks windows, rather than only noted as parked.
func parkedNow(h *harness, w platform.WindowID) bool {
	f := h.frame(w)
	return h.e.win.IsParked(w) && f.X == monitor.Frame.W-1 && f.Y == monitor.Frame.H-1
}

// TestSwitchShowsAnAppItCantParkFirst checks the fallback: with an app
// whose windows don't move while it's hidden, as some apps' windows may
// not, whether the platform says so or not, the switch shows the app and
// then parks them, as before.
func TestSwitchShowsAnAppItCantParkFirst(t *testing.T) {
	for _, err := range []error{nil, errors.New("the window didn't move")} {
		t.Run(fmt.Sprint(err), func(t *testing.T) {
			h := newHarness(t, setup{})
			mustOK(t, h.e.SwitchTab(2))
			docs := h.windowID(braveID, "Wails v3 docs")
			pkgs := h.windowID(braveID, "Go packages")
			mustOK(t, h.e.MoveItemToTab(h.itemOf(pkgs), 1))
			mustOK(t, h.e.SwitchTab(0))
			h.settle()
			h.p.FixHiddenWindows(true, err)
			h.p.ResetCalls()
			mustOK(t, h.e.SwitchTab(1))
			if !parkedNow(h, docs) {
				t.Errorf("Wails v3 docs isn't parked: %v", h.frame(docs))
			}
			calls := h.calls("Park", "Unhide")
			unhide := slices.IndexFunc(calls, func(c platform.Call) bool { return c.Method == "Unhide" && c.BundleID == braveID })
			if unhide < 0 || !slices.ContainsFunc(calls[unhide:], func(c platform.Call) bool { return c.Method == "Park" && c.Window == docs }) {
				t.Errorf("Wails v3 docs wasn't parked after Brave was shown: %v", calls)
			}
			for _, id := range []string{h.itemOf(docs), h.itemOf(pkgs)} {
				if it := h.item(id); it.Status != StatusReady {
					t.Errorf("%s: %s %q", it.Title, it.Status, it.StatusText)
				}
			}
			if got := h.view().Notices; len(got) != 0 {
				t.Errorf("notices: %+v", got)
			}
		})
	}
}

// TestTabShortcutShowsTheTabAtOnce checks the commands that a tab's
// shortcut sends around its switch, for the title bar: switching, with the
// tab, before the View that has it current, and none for the current tab,
// a tab that isn't there, or last_tab's shortcut, which switches as its
// keys are let go of.
func TestTabShortcutShowsTheTabAtOnce(t *testing.T) {
	h := newHarness(t, setup{})
	commands := func() []Command {
		h.em.mu.Lock()
		defer h.em.mu.Unlock()
		return slices.Clone(h.em.commands)
	}
	before, seen := len(commands()), h.em.stateCount()
	h.e.Shortcut(shortcuts.ActionTab, "3")
	got := commands()[before:]
	if !isSwitching(got, 2) {
		t.Fatalf("tab 3's shortcut sent %+v, want switching to 2", got)
	}
	h.advance(100 * time.Millisecond)
	h.em.mu.Lock()
	at, views := h.em.commandAt[before], slices.Clone(h.em.states[seen:])
	h.em.mu.Unlock()
	if at != seen || len(views) == 0 || views[0].Current != 2 {
		t.Errorf("switching came after %d new views, want before the %d with the new tab", at-seen, len(views))
	}
	if v := h.view(); v.Current != 2 {
		t.Errorf("current %d, want 2", v.Current)
	}

	for _, c := range []struct{ action, arg string }{
		{shortcuts.ActionTab, "3"},
		{shortcuts.ActionTab, "9"},
	} {
		before := len(commands())
		h.e.Shortcut(c.action, c.arg)
		h.settle()
		if got := commands()[before:]; slices.ContainsFunc(got, func(c Command) bool { return c.Name == switchingCommand }) {
			t.Errorf("%s %q sent %+v", c.action, c.arg, got)
		}
	}

	before = len(commands())
	h.e.Shortcut(shortcuts.ActionPreviousTab, "")
	if got := commands()[before:]; !isSwitching(got, h.view().Current) {
		t.Errorf("previous_tab's shortcut sent %+v, want switching to %d", got, h.view().Current)
	}

	// last_tab's shortcut with its keys let go of already switches at
	// once, and shows the tab as the others do.
	last := h.view().LastTab
	before = len(commands())
	h.e.Shortcut(shortcuts.ActionLastTab, "")
	if got := commands()[before:]; !isSwitching(got, last) || h.view().Current != last {
		t.Errorf("last_tab's shortcut sent %+v, current %d; want switching to %d", got, h.view().Current, last)
	}
}

// TestHoldLetGoShowsTheTabAtOnce checks that the switch a hold after
// last_tab's shortcut makes as its keys are let go of, a quick tap's or
// one that showed the tabs, is a tab shortcut's: the title bar shows the
// tab at once, before the View that has it current, and it's timed as
// tab-shortcut, from when Bench saw the keys let go of.
func TestHoldLetGoShowsTheTabAtOnce(t *testing.T) {
	for _, hold := range []time.Duration{60 * time.Millisecond, holdDelay + 100*time.Millisecond} {
		t.Run(fmt.Sprint(hold), func(t *testing.T) {
			h := recentHarness(t, setup{})
			h.p.SetHeldModifiers(held)
			before := len(h.em.commandsNamed(switchingCommand))
			h.press()
			h.advance(hold)
			if got := h.em.commandsNamed(switchingCommand); len(got) != before {
				t.Fatalf("the hold sent %+v before the keys were let go of", got[before:])
			}
			mark, seen := markTimes(h), h.em.stateCount()
			h.e.lock()
			from := len(h.em.commands)
			h.e.unlock()
			h.letGo()
			h.advance(100 * time.Millisecond)
			if v := h.view(); v.Current != 2 || v.Hold != nil {
				t.Fatalf("after letting go: current %d, hold %+v; want Research, and no hold", v.Current, v.Hold)
			}
			h.em.mu.Lock()
			got, at, views := slices.Clone(h.em.commands[from:]), slices.Clone(h.em.commandAt[from:]), slices.Clone(h.em.states[seen:])
			h.em.mu.Unlock()
			i := slices.IndexFunc(got, func(c Command) bool { return c.Name == switchingCommand })
			if i < 0 || got[i].Tab != 2 || slices.ContainsFunc(got, func(c Command) bool { return c.Name == switchRefusedCommand }) {
				t.Fatalf("letting go sent %+v, want switching to Research", got)
			}
			first := slices.IndexFunc(views, func(v View) bool { return v.Current == 2 })
			if first < 0 || at[i]-seen > first {
				t.Errorf("switching came after %d new views, want before the first with Research, the %dth", at[i]-seen, first)
			}
			if reasons := arrangeReasons(h, mark); len(reasons) == 0 || reasons[0] != reasonTabShortcut || slices.Contains(reasons, "hold") {
				t.Errorf("reasons %v, want tab-shortcut", reasons)
			}
		})
	}
}

// TestTabShortcutPutsTheTitleBarBack checks that a tab's shortcut puts the
// title bar back as the View has it when its switch doesn't end on the tab
// it showed: here a click on Research comes in between the hold of the
// lock that finds the next tab and the one that switches, so next_tab
// goes on from Research, to Comms.
func TestTabShortcutPutsTheTitleBarBack(t *testing.T) {
	h := newHarness(t, setup{})
	clicked := false
	h.em.onCommand = func(c Command) {
		if c.Name == switchingCommand && !clicked {
			clicked = true
			mustOK(t, h.e.SwitchTab(2))
		}
	}
	h.e.Shortcut(shortcuts.ActionNextTab, "")
	h.em.mu.Lock()
	got := slices.Clone(h.em.commands)
	h.em.mu.Unlock()
	if v := h.view(); v.Current != 0 {
		t.Fatalf("current %d, want Comms, the tab after Research", v.Current)
	}
	want := []Command{{Name: switchingCommand, Tab: 1}, {Name: switchRefusedCommand}}
	if !slices.EqualFunc(got, want, func(a, b Command) bool { return reflect.DeepEqual(a, b) }) {
		t.Errorf("commands %+v, want %+v", got, want)
	}

	// A refusal puts it back too: here the tab is deleted in between.
	h.em.mu.Lock()
	from := len(h.em.commands)
	h.em.mu.Unlock()
	h.em.onCommand = func(c Command) {
		if c.Name == switchingCommand {
			mustOK(t, h.e.DeleteTab(2))
		}
	}
	h.e.Shortcut(shortcuts.ActionTab, "3")
	h.em.mu.Lock()
	got = slices.Clone(h.em.commands[from:])
	h.em.mu.Unlock()
	want = []Command{{Name: switchingCommand, Tab: 2}, {Name: switchRefusedCommand}}
	if !slices.EqualFunc(got, want, func(a, b Command) bool { return reflect.DeepEqual(a, b) }) {
		t.Errorf("after the tab was deleted: commands %+v, want %+v", got, want)
	}
}

// isSwitching says if commands are only switching, to a tab.
func isSwitching(commands []Command, tab int) bool {
	return len(commands) == 1 && reflect.DeepEqual(commands[0], Command{Name: switchingCommand, Tab: tab})
}
