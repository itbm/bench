package workspace

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// Bundle IDs of the test world's apps.
const (
	benchID    = "com.itbm.bench"
	zedID      = "dev.zed.Zed"
	vscodeID   = "com.microsoft.VSCode"
	braveID    = "com.brave.Browser"
	obsidianID = "md.obsidian"
	finderID   = "com.apple.finder"
	calendarID = "com.apple.iCal"
	safariID   = "com.apple.Safari"
)

// Screens for the tests.
var (
	monitor = platform.Screen{ID: "monitor", Name: "Monitor", Primary: true,
		Frame: layout.Rect{W: 2560, H: 1440}, Visible: layout.Rect{Y: 25, W: 2560, H: 1415}}
	laptop = platform.Screen{ID: "laptop", Name: "Laptop", Primary: true,
		Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}}
	// contentFrame is Bench's window content area on the monitor, and
	// paneArea the pane area inside it, in content coordinates.
	contentFrame = layout.Rect{X: 0, Y: 25, W: 2560, H: 1415}
	paneArea     = layout.Rect{X: 0, Y: 40, W: 2560, H: 1340}
)

// recEmitter records what the engine sends.
type recEmitter struct {
	mu     sync.Mutex
	states []View
	toasts []string
	// actions holds each toast's button, by its place in toasts: the zero
	// ToastAction for a toast without one; seconds holds its second
	// button, or nil.
	actions  []ToastAction
	seconds  []*ToastAction
	commands []Command
	// commandAt holds, for each command, how many states came before it.
	commandAt []int
	data      []TermData
	popped    []string
	closed    []string
	bars      []MenuBar
	// drops holds each ShowDropdown and HideDropdown call, as "show w12
	// 40" or "hide w12", and dropErr is ShowDropdown's error. windows
	// holds the CloseTerminalWindow and ShowDropdown calls together, in
	// turn, as "close w12" and "show w12".
	drops   []string
	dropErr error
	windows []string
	// shortcuts counts the ShortcutsChanged calls, fills the FillScreen
	// calls, forwards the BringForward calls, quits the Quit calls, and
	// relaunches the Relaunch calls.
	shortcuts  int
	fills      int
	forwards   int
	quits      int
	relaunches int
	popErr     error
	// onPop runs as a terminal's window opens, without the lock, and
	// onCommand as a command is sent, without mu.
	onPop     func(id string)
	onCommand func(c Command)
	// pauses holds each PauseShortcuts call's argument, and checked the
	// tables CheckShortcuts was asked about; check, if set, answers it.
	pauses  []bool
	checked []map[string]string
	check   func(fields map[string]string) []ShortcutView
	// looks holds each Appearance call's argument.
	looks []string
	// moves holds each MoveToScreen call.
	moves []benchMove
}

// benchMove is a MoveToScreen call: where the shell was to move Bench's
// window, and whether it was to fill the screen there.
type benchMove struct {
	content layout.Rect
	fill    bool
}

func (r *recEmitter) State(v View) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states = append(r.states, v)
}

func (r *recEmitter) Toast(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toasts = append(r.toasts, msg)
	r.actions = append(r.actions, ToastAction{})
	r.seconds = append(r.seconds, nil)
}

func (r *recEmitter) ActionToast(t Toast) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toasts = append(r.toasts, t.Text)
	r.actions = append(r.actions, t.Action)
	r.seconds = append(r.seconds, t.Second)
}

// lastSecond returns the latest toast's second button, or nil.
func (r *recEmitter) lastSecond() *ToastAction {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.seconds) == 0 {
		return nil
	}
	return r.seconds[len(r.seconds)-1]
}

// lastUndo returns the change that the latest toast's Undo takes back, or
// fails the test if it has no Undo.
func (r *recEmitter) lastUndo(t *testing.T) int {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.actions) == 0 || r.actions[len(r.actions)-1].Method != "Undo" {
		t.Fatalf("the latest toast, %q, has no Undo", r.toasts[len(r.toasts)-1])
	}
	a := r.actions[len(r.actions)-1]
	if a.Label != "Undo" {
		t.Errorf("the Undo button reads %q", a.Label)
	}
	return a.Arg
}

// lastHasAction reports whether the latest toast has a button.
func (r *recEmitter) lastHasAction() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.actions) > 0 && r.actions[len(r.actions)-1].Method != ""
}

func (r *recEmitter) Command(c Command) {
	r.mu.Lock()
	r.commands = append(r.commands, c)
	r.commandAt = append(r.commandAt, len(r.states))
	on := r.onCommand
	r.mu.Unlock()
	if on != nil {
		on(c)
	}
}

func (r *recEmitter) TermData(d TermData) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, d)
}

func (r *recEmitter) PopOutTerminal(id, title string) error {
	if r.onPop != nil {
		r.onPop(id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.popErr != nil {
		return r.popErr
	}
	r.popped = append(r.popped, id)
	return nil
}

func (r *recEmitter) CloseTerminalWindow(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = append(r.closed, id)
	r.windows = append(r.windows, "close "+id)
}

func (r *recEmitter) ShowDropdown(id, title string, height int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dropErr != nil {
		return r.dropErr
	}
	r.drops = append(r.drops, fmt.Sprintf("show %s %d", id, height))
	r.windows = append(r.windows, "show "+id)
	return nil
}

func (r *recEmitter) HideDropdown(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drops = append(r.drops, "hide "+id)
}

// windowCalls returns the CloseTerminalWindow and ShowDropdown calls, in
// turn.
func (r *recEmitter) windowCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.windows)
}

// dropdownCalls returns the ShowDropdown and HideDropdown calls, in turn,
// and forgets them.
func (r *recEmitter) dropdownCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.drops
	r.drops = nil
	return out
}

func (r *recEmitter) MenuBar(m MenuBar) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bars = append(r.bars, m)
}

func (r *recEmitter) BringForward() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forwards++
}

// menuBars returns what the menu bar item was told to show, in turn.
func (r *recEmitter) menuBars() []MenuBar {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.bars)
}

// broughtForward returns how many times the shell was asked to bring Bench
// forward.
func (r *recEmitter) broughtForward() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.forwards
}

func (r *recEmitter) ShortcutsChanged() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shortcuts++
}

func (r *recEmitter) CheckShortcuts(fields map[string]string) []ShortcutView {
	r.mu.Lock()
	r.checked = append(r.checked, maps.Clone(fields))
	check := r.check
	r.mu.Unlock()
	if check == nil {
		return nil
	}
	return check(fields)
}

func (r *recEmitter) PauseShortcuts(paused bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pauses = append(r.pauses, paused)
}

// paused returns each PauseShortcuts call's argument, in turn.
func (r *recEmitter) paused() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.pauses)
}

func (r *recEmitter) FillScreen() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fills++
}

func (r *recEmitter) MoveToScreen(content layout.Rect, fill bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.moves = append(r.moves, benchMove{content, fill})
}

// benchMoves returns the MoveToScreen calls so far.
func (r *recEmitter) benchMoves() []benchMove {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.moves)
}

func (r *recEmitter) Quit() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quits++
}

func (r *recEmitter) Appearance(appearance string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.looks = append(r.looks, appearance)
}

// appearances returns each Appearance call's argument, in turn.
func (r *recEmitter) appearances() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.looks)
}

func (r *recEmitter) Relaunch() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.relaunches++
}

// relaunchCount returns how many times the shell was asked to quit and
// open Bench again.
func (r *recEmitter) relaunchCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.relaunches
}

// quitCount returns how many times the shell was asked to quit.
func (r *recEmitter) quitCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.quits
}

// filled returns how many times the shell was asked to fill the screen.
func (r *recEmitter) filled() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fills
}

// shortcutsChanged returns how many times the shortcuts were said to have
// changed.
func (r *recEmitter) shortcutsChanged() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shortcuts
}

// lastToast returns the latest toast, or "".
func (r *recEmitter) lastToast() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.toasts) == 0 {
		return ""
	}
	return r.toasts[len(r.toasts)-1]
}

// stateCount returns how many bench:state events were sent.
func (r *recEmitter) stateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.states)
}

// fakeTerms is a terminal host that runs nothing.
type fakeTerms struct {
	mu        sync.Mutex
	cb        term.Callbacks
	started   map[string]term.Options
	starts    int
	written   map[string]string
	sizes     map[string][2]int
	snaps     map[string]term.Snapshot
	titles    map[string]string
	ended     map[string]bool
	acks      map[string]int64
	busy      map[string]string // what each busy session runs
	paths     map[string]string // where LookPath finds each program
	looked    []string          // the names LookPath was asked for
	startErr  error
	closedAll bool
	// lateSnaps counts the snapshots taken after CloseAll, whose output
	// would be gone.
	lateSnaps int
	// views records the views that draw each terminal (Attach), and
	// agentOff is set while agent status is off (SetAgentStatus).
	views    map[string]map[string]bool
	agentOff bool
}

func newFakeTerms() *fakeTerms {
	return &fakeTerms{started: map[string]term.Options{}, written: map[string]string{},
		sizes: map[string][2]int{}, snaps: map[string]term.Snapshot{}, titles: map[string]string{},
		ended: map[string]bool{}, acks: map[string]int64{}, busy: map[string]string{}, paths: map[string]string{},
		views: map[string]map[string]bool{}}
}

// end makes a session's process end, as Manager does: it stays, but isn't
// running, until a Start replaces it. Exit is the test's to report.
func (f *fakeTerms) end(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ended[id] = true
}

func (f *fakeTerms) Start(id string, opts term.Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	if _, ok := f.started[id]; ok && !f.ended[id] {
		return nil
	}
	f.starts++
	f.started[id] = opts
	delete(f.ended, id)
	f.titles[id] = "zsh — " + opts.Dir
	return nil
}

func (f *fakeTerms) Running(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.started[id]
	return ok && !f.ended[id]
}

// setBusy makes a session busy running what, or not busy for "".
func (f *fakeTerms) setBusy(id, what string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if what == "" {
		delete(f.busy, id)
		return
	}
	f.busy[id] = what
}

func (f *fakeTerms) Busy(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	what, busy := f.busy[id]
	if _, ok := f.started[id]; !ok || !busy || f.ended[id] {
		return "", false
	}
	return what, true
}

func (f *fakeTerms) Write(id string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.started[id]; !ok {
		return fmt.Errorf("write: %w", term.ErrNoSession)
	}
	if f.ended[id] {
		return fmt.Errorf("write: %w", term.ErrEnded)
	}
	f.written[id] += string(data)
	return nil
}

func (f *fakeTerms) Resize(id string, cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.started[id]; !ok {
		return fmt.Errorf("resize: %w", term.ErrNoSession)
	}
	if cols <= 0 || rows <= 0 {
		return errors.New("bad size")
	}
	f.sizes[id] = [2]int{cols, rows}
	return nil
}

func (f *fakeTerms) Title(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.titles[id]
}

func (f *fakeTerms) Snapshot(id string) term.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closedAll {
		f.lateSnaps++
	}
	return f.snaps[id]
}

func (f *fakeTerms) Attach(id, view string) term.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.views[id] == nil {
		f.views[id] = map[string]bool{}
	}
	f.views[id][view] = true
	return f.snaps[id]
}

func (f *fakeTerms) Detach(id, view string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.views[id], view)
}

func (f *fakeTerms) DetachView(view string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.views {
		delete(v, view)
	}
}

// drawnBy returns the views that draw a terminal, in order.
func (f *fakeTerms) drawnBy(id string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(maps.Keys(f.views[id]))
}

func (f *fakeTerms) SetAgentStatus(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentOff = !on
}

// agentStatusOn reports whether agent status is on.
func (f *fakeTerms) agentStatusOn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.agentOff
}

func (f *fakeTerms) Clear(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snaps[id] = term.Snapshot{Offset: f.snaps[id].Offset}
}

func (f *fakeTerms) Close(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.started, id)
}

func (f *fakeTerms) CloseAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closedAll = true
}

func (f *fakeTerms) Ack(id string, offset int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks[id] = max(f.acks[id], offset)
}

func (f *fakeTerms) LookPath(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.looked = append(f.looked, name)
	return f.paths[name]
}

// setup describes a test's world.
type setup struct {
	// config is config.toml; "" means the example config, unless firstRun.
	config string
	// firstRun leaves config.toml out, for the engine to write.
	firstRun bool
	// state is written as state.json before the engine starts.
	state func(t *testing.T, dir string)
	// world sets up apps and windows; nil means standardWorld.
	world func(p *platform.Fake)
	// options changes the engine's options.
	options func(o *Options)
	// untrusted starts without Accessibility permission.
	untrusted bool
	// noStart leaves Start to the test.
	noStart bool
	// screen is the screen; the zero Screen means the monitor.
	screen platform.Screen
	// dir is the support folder; "" means a new one from t.TempDir, whose
	// path can be too long for a socket on macOS.
	dir string
}

// harness runs an engine against a Fake platform and a Fake clock. Events
// are fed to the engine by drain, not by its loop.
type harness struct {
	t     *testing.T
	clk   *clock.Fake
	p     *platform.Fake
	em    *recEmitter
	terms *fakeTerms
	e     *Engine
	dir   string
	bench platform.App
}

// standardWorld is the prototype's world: Zed, VS Code, three Brave windows,
// Obsidian, and Finder running; Calendar and Safari installed; and Bench's
// own window.
func standardWorld(p *platform.Fake) {
	p.InstallApp(zedID, "Zed")
	p.AddApp(zedID, "Zed")
	p.AddWindow(zedID, "bench — switch.go")
	p.InstallApp(vscodeID, "Visual Studio Code")
	p.AddApp(vscodeID, "VS Code")
	p.AddWindow(vscodeID, "index.html — bench-ui")
	p.AddApp(braveID, "Brave")
	for _, title := range []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"} {
		p.AddWindow(braveID, title)
	}
	p.AddApp(obsidianID, "Obsidian")
	p.AddWindow(obsidianID, "Daily note")
	p.AddApp(finderID, "Finder")
	p.AddWindow(finderID, "Downloads", platform.WithFrame(layout.Rect{X: 100, Y: 100, W: 600, H: 400}))
	p.InstallApp(calendarID, "Calendar", platform.AppLaunchWindows("Calendar"))
	p.InstallApp(safariID, "Safari", platform.AppLaunchWindows("Apple"))
}

// newHarness starts an engine for a test.
// testCompactBelow is compact_below in the example config that the
// engine's tests use: 1,800 pt, the default before decision 58's lower
// one, so that the laptop screen (1,512 pt) gets the compact layout when
// Bench fills it, and the tests of that layout can use it. The tests of the
// default itself say so (TestDefaultMacBookIsSideBySide).
const testCompactBelow = 1800

// exampleConfig returns the example config.toml with compact_below set to
// testCompactBelow.
func exampleConfig(t *testing.T) string {
	t.Helper()
	from := fmt.Sprintf("compact_below = %d ", config.DefaultCompactBelow)
	example := string(config.ExampleTOML())
	if !strings.Contains(example, from) {
		t.Fatalf("the example config has no %q", from)
	}
	return strings.Replace(example, from, fmt.Sprintf("compact_below = %d ", testCompactBelow), 1)
}

func newHarness(t *testing.T, s setup) *harness {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	p := platform.NewFake(clk)
	screen := s.screen
	if screen.ID == "" {
		screen = monitor
	}
	p.SetScreens(screen)
	content := screen.Visible
	p.SetContentFrame(content)
	h := &harness{t: t, clk: clk, p: p, em: &recEmitter{}, terms: newFakeTerms(), dir: s.dir}
	if h.dir == "" {
		h.dir = t.TempDir()
	}
	h.bench = p.AddApp(benchID, "Bench", platform.AppNotInstalled())
	p.AddWindow(benchID, "Bench")
	p.SetOwnApp(benchID)
	if s.world == nil {
		standardWorld(p)
	} else {
		s.world(p)
	}
	p.SetTrusted(!s.untrusted)
	p.DrainEvents()
	if s.config == "" && !s.firstRun {
		s.config = exampleConfig(t)
	}
	if s.config != "" {
		if err := os.WriteFile(filepath.Join(h.dir, config.ConfigFile), []byte(s.config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if s.state != nil {
		s.state(t, h.dir)
	}
	opts := Options{
		Platform: p, Clock: clk, Emitter: h.em, Dir: h.dir,
		NewTerms: func(cb term.Callbacks) term.Host { h.terms.cb = cb; return h.terms },
	}
	if s.options != nil {
		s.options(&opts)
	}
	e, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.manual = true
	e.ownPID = h.bench.PID
	h.e = e
	t.Cleanup(func() { _ = e.Close() })
	area := paneArea
	area.W, area.H = content.W, content.H-75
	if err := e.SetPaneArea(area); err != nil {
		t.Fatal(err)
	}
	if !s.noStart {
		h.start()
	}
	return h
}

// start starts the engine and lets it settle.
func (h *harness) start() {
	h.e.Start()
	h.settle()
}

// drain feeds the platform's waiting events to the engine, until there are
// none.
func (h *harness) drain() {
	for range 100 {
		evs := h.p.DrainEvents()
		if len(evs) == 0 {
			return
		}
		for _, ev := range evs {
			h.e.handle(ev)
		}
	}
	h.t.Fatal("events kept coming")
}

// advance moves the clock forward in small steps, feeding events between
// them as the engine's loop would.
func (h *harness) advance(d time.Duration) {
	const step = 10 * time.Millisecond
	h.drain()
	for d > 0 {
		s := min(step, d)
		h.clk.Advance(s)
		h.drain()
		d -= s
	}
}

// settle lets every wait finish: long enough for a launch timeout.
func (h *harness) settle() { h.advance(2 * time.Second) }

// view returns the current view.
func (h *harness) view() View { return h.e.View() }

// item returns an item's view, failing the test if there's none.
func (h *harness) item(id string) ItemView {
	h.t.Helper()
	it, ok := h.view().Items[id]
	if !ok {
		h.t.Fatalf("no item %s", id)
	}
	return it
}

// itemOf returns the item showing a window, or "".
func (h *harness) itemOf(w platform.WindowID) string {
	h.e.lock()
	defer h.e.unlock()
	if it := h.e.model.ItemByWindow(w); it != nil {
		return it.ID
	}
	return ""
}

// windowID returns the ID of an app's window with a title.
func (h *harness) windowID(bundleID, title string) platform.WindowID {
	h.t.Helper()
	app, _ := h.p.App(bundleID)
	ws, err := h.p.Windows(app)
	if err != nil {
		h.t.Fatalf("Windows(%s): %v", bundleID, err)
	}
	for _, w := range ws {
		if w.Title == title {
			return w.ID
		}
	}
	h.t.Fatalf("no %s window %q", bundleID, title)
	return 0
}

// frame returns a window's frame.
func (h *harness) frame(id platform.WindowID) layout.Rect {
	r, _ := h.p.Frame(id)
	return r
}

// bodyOf returns a displayed pane's body in global coordinates.
func (h *harness) bodyOf(paneID string) layout.Rect {
	h.t.Helper()
	for _, p := range h.view().Display.Panes {
		if p.ID == paneID {
			b := p.Body
			cf := h.p.ContentFrame()
			b.X += cf.X
			b.Y += cf.Y
			return b
		}
	}
	h.t.Fatalf("pane %s isn't displayed", paneID)
	return layout.Rect{}
}

// calls returns the calls to methods since the last reset.
func (h *harness) calls(methods ...string) []platform.Call { return h.p.CallsTo(methods...) }

// called reports whether a method was called with a window or bundle ID.
func (h *harness) called(method string, match func(platform.Call) bool) bool {
	return slices.ContainsFunc(h.calls(method), match)
}

// focusCalls returns the windows Bench focused.
func (h *harness) focusCalls() []platform.WindowID {
	var out []platform.WindowID
	for _, c := range h.calls("Focus") {
		out = append(out, c.Window)
	}
	return out
}

// mustErr checks that err is a refusal with the given text.
func mustErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Errorf("got error %v, want %q", err, want)
	}
}

// mustOK fails the test on an error.
func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// paneOf returns the pane holding an item in the view.
func (h *harness) paneOf(id string) string { return h.item(id).Pane }

// tabNamed returns a tab's view.
func (h *harness) tabNamed(name string) TabView {
	h.t.Helper()
	for _, tv := range h.view().Tabs {
		if tv.Name == name {
			return tv
		}
	}
	h.t.Fatalf("no tab %s", name)
	return TabView{}
}

// leafIDs returns a tab's panes in reading order.
func leafIDs(tv TabView) []string {
	var out []string
	for _, p := range layout.Leaves(tv.Root) {
		out = append(out, p.ID)
	}
	return out
}

// floatingIDs returns the IDs in View.Floating.
func floatingIDs(v View) []string {
	var out []string
	for _, f := range v.Floating {
		out = append(out, f.ID)
	}
	return out
}

// hasPrefix reports whether any string has a prefix.
func hasPrefix(list []string, prefix string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

// texts returns the notices' texts.
func texts(notices []NoticeView) []string {
	out := []string{}
	for _, n := range notices {
		out = append(out, n.Text)
	}
	return out
}

// noticeWith returns the notice whose text starts with prefix, if there's
// one.
func noticeWith(notices []NoticeView, prefix string) (NoticeView, bool) {
	for _, n := range notices {
		if strings.HasPrefix(n.Text, prefix) {
			return n, true
		}
	}
	return NoticeView{}, false
}
