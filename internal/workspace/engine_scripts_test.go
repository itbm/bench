package workspace

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/control"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
)

// scriptsConfig returns the example config with allow_scripts on, and the
// launchers of launchersTOML.
func scriptsConfig(t *testing.T) string {
	t.Helper()
	c := exampleConfig(t)
	if !strings.Contains(c, "allow_scripts = false") {
		t.Fatal("the example config has no allow_scripts = false")
	}
	return strings.Replace(c, "allow_scripts = false", "allow_scripts = true", 1) + launchersTOML
}

// socketDir returns a new support folder with a path short enough for a
// socket: t.TempDir's can be too long on macOS.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "bw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// newScriptsHarness is newHarness, with the frontend loaded: it has asked
// for the view (Ready), so links' and scripts' commands and toasts go out
// at once, rather than wait for it.
func newScriptsHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := newHarness(t, s)
	if _, err := h.e.Ready(); err != nil {
		t.Fatal(err)
	}
	return h
}

// TestFollowLinkSwitchesTabs checks bench://tab/<name or number>, next,
// previous, and last: each switches tabs and brings Bench forward, with
// allow_scripts off, as it is by default.
func TestFollowLinkSwitchesTabs(t *testing.T) {
	h := newScriptsHarness(t, setup{})
	if h.view().Settings.AllowScripts {
		t.Fatal("allow_scripts is on in the example config")
	}
	steps := []struct {
		link string
		want int
	}{
		{"bench://tab/Code", 1},
		{"bench://tab/research", 2},
		{"bench://tab/1", 0},
		{"bench://last", 2},
		{"bench://next", 0},
		{"bench://previous", 2},
		{"bench://tab/%43ode/", 1},
	}
	for i, s := range steps {
		mustOK(t, h.e.FollowLink(s.link))
		h.settle()
		if got := h.view().Current; got != s.want {
			t.Errorf("%s: tab %d, want %d", s.link, got, s.want)
		}
		if got := h.em.broughtForward(); got != i+1 {
			t.Errorf("%s: Bench was brought forward %d times, want %d", s.link, got, i+1)
		}
	}
	if got := h.em.lastToast(); got != "" {
		t.Errorf("toast %q", got)
	}
}

// TestFollowLinkShowsJumpAndEveryTab checks bench://jump and
// bench://overview: they open Jump to… and every tab, which bring Bench
// forward themselves.
func TestFollowLinkShowsJumpAndEveryTab(t *testing.T) {
	h := newScriptsHarness(t, setup{})
	mustOK(t, h.e.FollowLink("bench://jump"))
	mustOK(t, h.e.FollowLink("bench://overview"))
	if got := len(h.em.commandsNamed("open-palette")); got != 1 {
		t.Errorf("open-palette sent %d times, want once", got)
	}
	if got := len(h.em.commandsNamed("show-all-tabs")); got != 1 {
		t.Errorf("show-all-tabs sent %d times, want once", got)
	}
	if got := h.view().Current; got != 0 {
		t.Errorf("tab %d, want the first still", got)
	}
}

// TestFollowLinkRefusesTheRest checks that a link can do nothing but
// switch and show: anything else is refused with a toast that names it,
// and Bench comes forward to show it, changing nothing.
func TestFollowLinkRefusesTheRest(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	before := h.view()
	cases := []struct{ link, toast string }{
		{"bench://launch/Claude", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://launch/Claude."},
		{"bench://move-to-tab/3", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://move-to-tab/3."},
		{"bench://close", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://close."},
		{"bench://tab/C#", "In a link, write # as %23, ? as %3F, and % as %25. Bench can't open bench://tab/C#."},
		{"bench://tab/Nope", "There's no tab called Nope."},
		{"bench://tab/9", "There's no tab 9."},
		{"bench://tab/A%0AB", "There's no tab called A?B."},
	}
	for i, c := range cases {
		if err := h.e.FollowLink(c.link); err == nil {
			t.Errorf("%s was followed", c.link)
		}
		if got := h.em.lastToast(); got != c.toast {
			t.Errorf("%s: toast %q, want %q", c.link, got, c.toast)
		}
		if got := h.em.broughtForward(); got != i+1 {
			t.Errorf("%s: brought forward %d times, want %d", c.link, got, i+1)
		}
		// Far enough apart that Bench takes each (linkBurst).
		h.advance(linkWindow / linkBurst)
	}
	h.settle()
	after := h.view()
	if after.Current != before.Current || len(after.Items) != len(before.Items) {
		t.Errorf("a refused link changed something: tab %d, %d items; was tab %d, %d items",
			after.Current, len(after.Items), before.Current, len(before.Items))
	}
}

// TestRunScriptNeedsAllowScripts checks that the script subcommands do
// nothing at all while allow_scripts is off, not even a toast, as any app
// can send them, and work once it's on.
func TestRunScriptNeedsAllowScripts(t *testing.T) {
	h := newScriptsHarness(t, setup{config: exampleConfig(t) + launchersTOML})
	for _, args := range [][]string{{"tab", "Code"}, {"next"}, {"last"}, {"move-to-tab", "Code"}, {"launch", "Claude"}} {
		if err := h.e.RunScript(args); !errors.Is(err, errScriptsOff) {
			t.Errorf("%q with allow_scripts off: %v", args, err)
		}
	}
	h.settle()
	v := h.view()
	if v.Current != 0 || h.em.broughtForward() != 0 || h.em.lastToast() != "" {
		t.Errorf("with allow_scripts off: tab %d, brought forward %d times, toast %q; want nothing done",
			v.Current, h.em.broughtForward(), h.em.lastToast())
	}
	if _, err := h.e.ScriptList(); !errors.Is(err, errScriptsOff) {
		t.Errorf("ScriptList with allow_scripts off: %v", err)
	}

	s := v.Settings
	s.AllowScripts = true
	mustOK(t, h.e.SaveSettings(s))
	h.settle()
	mustOK(t, h.e.RunScript([]string{"tab", "code"}))
	if got := h.view().Current; got != 1 || h.em.broughtForward() != 1 {
		t.Errorf("bench tab code: tab %d, brought forward %d times; want Code, forward once", got, h.em.broughtForward())
	}
	mustOK(t, h.e.RunScript([]string{"next"}))
	mustOK(t, h.e.RunScript([]string{"previous"}))
	mustOK(t, h.e.RunScript([]string{"last"}))
	if got := h.view().Current; got != 2 {
		t.Errorf("after next, previous, and last: tab %d, want Research", got)
	}
	// A subcommand that only links have, or one that's wrong, is refused
	// without a toast: the bench command checks its arguments itself.
	toasts := h.em.toastCount()
	for _, args := range [][]string{{"jump"}, {"tab"}, {"tab", "a", "b"}, {"quit"}} {
		if err := h.e.RunScript(args); err == nil {
			t.Errorf("%q was run", args)
		}
	}
	if got := h.em.toastCount(); got != toasts {
		t.Errorf("%d toasts for wrong subcommands, the last %q", got-toasts, h.em.lastToast())
	}
	// One that Bench can't carry out says why.
	if err := h.e.RunScript([]string{"tab", "Nope"}); err == nil || h.em.lastToast() != "There's no tab called Nope." {
		t.Errorf("bench tab Nope: %v, toast %q", err, h.em.lastToast())
	}
}

// TestRunScriptLastTabWithoutAHold checks that bench last switches at once,
// even with last_tab's modifiers held, which would start a hold for the
// shortcut.
func TestRunScriptLastTabWithoutAHold(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.SwitchTab(1))
	h.p.SetHeldModifiers(held)
	mustOK(t, h.e.RunScript([]string{"last"}))
	h.e.lock()
	holding := h.e.hold != nil
	h.e.unlock()
	if got := h.view().Current; got != 2 || holding {
		t.Errorf("bench last with keys held: tab %d, hold %v; want Research, without a hold", got, holding)
	}
	// And during the shortcut's hold, a script ends it first.
	h.e.Shortcut(shortcuts.ActionLastTab, "")
	h.e.lock()
	holding = h.e.hold != nil
	h.e.unlock()
	if !holding {
		t.Fatal("the shortcut started no hold")
	}
	mustOK(t, h.e.RunScript([]string{"tab", "Comms"}))
	h.e.lock()
	holding = h.e.hold != nil
	h.e.unlock()
	if got := h.view().Current; got != 0 || holding {
		t.Errorf("bench tab Comms during a hold: tab %d, hold %v; want Comms, the hold over", got, holding)
	}
}

// TestRunScriptMovesTheShownWindow checks bench move-to-tab: the focused
// pane's shown window goes to that tab's focused pane, as the send-to-tab
// shortcut does, and Bench stays where it is.
func TestRunScriptMovesTheShownWindow(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	mustOK(t, h.e.RunScript([]string{"move-to-tab", "research"}))
	v := h.view()
	if v.Current != 1 {
		t.Errorf("tab %d; move-to-tab mustn't switch tabs", v.Current)
	}
	if it := v.Items["w3"]; it.Tab != 2 || it.Pane != "p14" {
		t.Errorf("w3: tab %d, pane %s; want Research's focused pane, p14", it.Tab, it.Pane)
	}
	if got := h.em.lastToast(); got != "Moved Zed — bench — switch.go to Research." {
		t.Errorf("toast %q", got)
	}
	if got := h.em.broughtForward(); got != 0 {
		t.Errorf("brought forward %d times; move-to-tab mustn't", got)
	}
	// By number, and refused for the tab it's in.
	mustOK(t, h.e.RunScript([]string{"move-to-tab", "1"}))
	if it := h.item("w4"); it.Tab != 0 {
		t.Errorf("w4 in tab %d, want Comms", it.Tab)
	}
	if err := h.e.RunScript([]string{"move-to-tab", "Code"}); err == nil || h.em.lastToast() != "That window is already in Code." {
		t.Errorf("move-to-tab Code from Code: %v, toast %q", err, h.em.lastToast())
	}
}

// TestRunScriptLaunches checks bench launch: what the launcher opens goes
// into the current tab's focused pane, shown there, and Bench comes
// forward.
func TestRunScriptLaunches(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	h := newScriptsHarness(t, setup{config: scriptsConfig(t), world: launcherWorld})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	mustOK(t, h.e.RunScript([]string{"launch", "claude"}))
	h.settle()
	id := h.paneShows("p11")
	it := h.item(id)
	if it.Kind != KindBuiltin || it.Title != "Claude" || it.Tab != 1 {
		t.Errorf("p11 shows %+v, want the Claude terminal", it)
	}
	if opts := h.startedWith(id); opts.Input != "claude --continue\r" {
		t.Errorf("started with %+v", opts)
	}
	if got := h.em.broughtForward(); got != 1 {
		t.Errorf("brought forward %d times, want once", got)
	}
	if err := h.e.RunScript([]string{"launch", "Nope"}); err == nil || h.em.lastToast() != errNoLauncher("Nope").Error() {
		t.Errorf("launch Nope: %v, toast %q", err, h.em.lastToast())
	}
}

// TestScriptListOverTheSocket checks bench list's socket: it's there, for
// the user alone, while allow_scripts is on, and answers with the tabs,
// their panes, and what they hold. Turning the setting off, and quitting,
// remove it.
func TestScriptListOverTheSocket(t *testing.T) {
	dir := socketDir(t)
	h := newScriptsHarness(t, setup{config: scriptsConfig(t), dir: dir})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	path := filepath.Join(dir, control.SocketFile)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("no socket: %v", err)
	}
	if info.Mode()&fs.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Errorf("the socket's mode is %v, want a socket only the user can open", info.Mode())
	}
	l, err := control.Query(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != 1 || l.Current != 2 || len(l.Tabs) != 3 {
		t.Fatalf("list: version %d, current %d, %d tabs", l.Version, l.Current, len(l.Tabs))
	}
	for i, name := range []string{"Comms", "Code", "Research"} {
		if tab := l.Tabs[i]; tab.Number != i+1 || tab.Name != name || tab.Current != (i == 1) {
			t.Errorf("tab %d: %+v", i, tab)
		}
	}
	code := l.Tabs[1]
	if len(code.Panes) != 2 || code.Panes[0].ID != "p11" || !code.Panes[0].Focused || code.Panes[1].Focused {
		t.Fatalf("Code's panes: %+v", code.Panes)
	}
	items := code.Panes[0].Items
	if len(items) != 2 || items[0].ID != "w3" || items[0].Kind != "app" || items[0].App != "Zed" ||
		items[0].Title != "bench — switch.go" || !items[0].Shown || items[1].Shown {
		t.Errorf("Code's first pane: %+v", items)
	}
	if terminal := code.Panes[1].Items; len(terminal) != 1 || terminal[0].Kind != "terminal" || !terminal[0].Shown {
		t.Errorf("Code's second pane: %+v", terminal)
	}
	if web := l.Tabs[0].Panes[0].Items; len(web) != 1 || web[0].Kind != "web" || web[0].Title == "" {
		t.Errorf("Comms' first pane: %+v", web)
	}
	if l.Attention != 0 || l.Unread != (control.Unread{}) {
		t.Errorf("attention %d, unread %+v; want none", l.Attention, l.Unread)
	}

	// Off: the socket goes, and the list is refused.
	s := h.view().Settings
	s.AllowScripts = false
	mustOK(t, h.e.SaveSettings(s))
	h.settle()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket is still there with allow_scripts off: %v", err)
	}
	if _, err := control.Query(dir); !errors.Is(err, control.ErrNotServing) {
		t.Errorf("Query with allow_scripts off: %v", err)
	}
	// On again, and then quitting removes it.
	s.AllowScripts = true
	mustOK(t, h.e.SaveSettings(s))
	h.settle()
	if _, err := control.Query(dir); err != nil {
		t.Errorf("Query with allow_scripts on again: %v", err)
	}
	mustOK(t, h.e.Close())
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket is still there after quitting: %v", err)
	}
}

// TestScriptListCountsWhatNeedsTheUser checks the attention and unread
// counts in bench list: a terminal that rang its bell out of sight, and a
// page with a count in its title.
func TestScriptListCountsWhatNeedsTheUser(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	terminal := h.paneShows("p12")
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.terms.cb.Alert(terminal, term.Alert{Bell: true})
	h.settle()
	l, err := h.e.ScriptList()
	if err != nil {
		t.Fatal(err)
	}
	code := l.Tabs[1]
	if l.Attention != 1 || code.Attention != 1 || code.Panes[1].Items[0].Attention != bellText {
		t.Errorf("attention: list %d, Code %d, terminal %q; want the bell", l.Attention, code.Attention, code.Panes[1].Items[0].Attention)
	}
	if !l.Tabs[0].Current || l.Current != 1 {
		t.Errorf("current: %d", l.Current)
	}
}

// TestScriptSocketReplacesAStaleOne checks that a socket a crashed Bench
// left is replaced, and that a file that isn't a socket is left alone,
// with a notice that says why scripts can't control Bench, until the
// setting goes off.
func TestScriptSocketReplacesAStaleOne(t *testing.T) {
	dir := socketDir(t)
	path := filepath.Join(dir, control.SocketFile)
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newScriptsHarness(t, setup{config: scriptsConfig(t), dir: dir})
	notice := func() string {
		for _, n := range h.view().Notices {
			if strings.HasPrefix(n.Text, "Scripts can't control Bench") {
				return n.Text
			}
		}
		return ""
	}
	if got := notice(); got != "Scripts can't control Bench: there's a file at "+path+" that isn't Bench's socket: move it, and Bench will try again." {
		t.Errorf("notice %q", got)
	}
	if data, _ := os.ReadFile(path); string(data) != "mine" {
		t.Errorf("the file has %q", data)
	}
	s := h.view().Settings
	s.AllowScripts = false
	mustOK(t, h.e.SaveSettings(s))
	h.settle()
	if got := notice(); got != "" {
		t.Errorf("notice with allow_scripts off: %q", got)
	}

	// The file goes, as the notice asks: on again, the socket comes.
	os.Remove(path)
	s.AllowScripts = true
	mustOK(t, h.e.SaveSettings(s))
	h.settle()
	if _, err := control.Query(dir); err != nil || notice() != "" {
		t.Errorf("Query: %v, notice %q", err, notice())
	}
	if got := h.e.scriptSocketPath(); got != path {
		t.Errorf("socket at %q, want %q", got, path)
	}
}

// TestNoScriptSocketForArrange checks that bench arrange, which has no
// window, makes no socket.
func TestNoScriptSocketForArrange(t *testing.T) {
	dir := socketDir(t)
	h := newScriptsHarness(t, setup{config: scriptsConfig(t), dir: dir, options: func(o *Options) { o.Headless = true }})
	h.settle()
	if got := h.e.scriptSocketPath(); got != "" {
		t.Errorf("socket at %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, control.SocketFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("there's a socket: %v", err)
	}
}

// TestScriptSocketTriesAgain checks that Bench tries again to make the
// socket, as its notice says, once the file in the way has gone, without
// the setting being turned off and on.
func TestScriptSocketTriesAgain(t *testing.T) {
	dir := socketDir(t)
	path := filepath.Join(dir, control.SocketFile)
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newScriptsHarness(t, setup{config: scriptsConfig(t), dir: dir})
	notices := func() int {
		n := 0
		for _, nv := range h.view().Notices {
			if strings.HasPrefix(nv.Text, "Scripts can't control Bench") {
				n++
			}
		}
		return n
	}
	if notices() != 1 {
		t.Fatalf("%d notices, want the one about the file", notices())
	}
	// While the file is there, trying again changes nothing.
	h.advance(3 * socketRetryWait)
	if _, err := control.Query(dir); !errors.Is(err, control.ErrNotServing) || notices() != 1 {
		t.Errorf("with the file there: Query %v, %d notices", err, notices())
	}
	if data, _ := os.ReadFile(path); string(data) != "mine" {
		t.Errorf("the file has %q", data)
	}

	os.Remove(path)
	h.advance(socketRetryWait)
	if _, err := control.Query(dir); err != nil || notices() != 0 {
		t.Errorf("once the file has gone: Query %v, %d notices", err, notices())
	}
	h.e.lock()
	retrying := h.e.socketRetry != nil
	h.e.unlock()
	if retrying {
		t.Error("Bench still tries again with the socket made")
	}
}

// TestScriptListCountsSharedItemsOnce checks the list's totals: an item in
// two tabs that needs the user counts once, as in the Dock's badge, and
// once in each tab's count.
func TestScriptListCountsSharedItemsOnce(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	terminal := h.paneShows("p12")
	mustOK(t, h.e.AddItemToTab(terminal, 2))
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.terms.cb.Alert(terminal, term.Alert{Bell: true})
	h.settle()
	l, err := h.e.ScriptList()
	if err != nil {
		t.Fatal(err)
	}
	if l.Attention != 1 || l.Tabs[1].Attention != 1 || l.Tabs[2].Attention != 1 {
		t.Errorf("attention: list %d, Code %d, Research %d; want 1 each", l.Attention, l.Tabs[1].Attention, l.Tabs[2].Attention)
	}
	h.e.lock()
	badge := h.e.attentionCountLocked()
	h.e.unlock()
	if l.Attention != badge {
		t.Errorf("attention %d, but the Dock's badge says %d", l.Attention, badge)
	}
}

// TestLinkBeforeTheFrontendLoads checks a link that opens Bench: it comes
// before the frontend has loaded, so its Jump to… and its message wait
// until the frontend asks for the view, which has the tab it switched to.
func TestLinkBeforeTheFrontendLoads(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.FollowLink("bench://tab/Code"))
	mustOK(t, h.e.FollowLink("bench://jump"))
	if err := h.e.FollowLink("bench://quit"); err == nil {
		t.Fatal("bench://quit was followed")
	}
	if n, toasts := len(h.em.commandsNamed("open-palette")), h.em.toastCount(); n != 0 || toasts != 0 {
		t.Fatalf("before the frontend loaded: open-palette sent %d times, %d toasts", n, toasts)
	}
	v, err := h.e.Ready()
	if err != nil {
		t.Fatal(err)
	}
	if v.Current != 1 {
		t.Errorf("Ready's view is on tab %d, want Code", v.Current)
	}
	if n := len(h.em.commandsNamed("open-palette")); n != 1 {
		t.Errorf("open-palette sent %d times once loaded, want once", n)
	}
	if got := h.em.lastToast(); !strings.HasSuffix(got, "Bench can't open bench://quit.") {
		t.Errorf("toast %q once loaded", got)
	}
	// Once only: a reload's Ready sends nothing again, and later links
	// show at once.
	if _, err := h.e.Ready(); err != nil {
		t.Fatal(err)
	}
	mustOK(t, h.e.FollowLink("bench://overview"))
	if n, m := len(h.em.commandsNamed("open-palette")), len(h.em.commandsNamed("show-all-tabs")); n != 1 || m != 1 {
		t.Errorf("open-palette sent %d times, show-all-tabs %d; want once each", n, m)
	}
}

// TestFollowLinkIgnoresAFlood checks that Bench follows at most linkBurst
// links in linkWindow, refused ones too, so that a page that opens links
// over and over can't keep switching tabs or bringing Bench forward. The
// links ignored show nothing, and once linkWindow has passed, links work
// again. Scripts aren't limited.
func TestFollowLinkIgnoresAFlood(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	followed := 0
	for range 3 * linkBurst {
		switch err := h.e.FollowLink("bench://next"); {
		case err == nil:
			followed++
		case !errors.Is(err, errLinksTooQuick):
			t.Fatalf("bench://next: %v", err)
		}
	}
	toasts := h.em.toastCount()
	if err := h.e.FollowLink("bench://quit"); !errors.Is(err, errLinksTooQuick) {
		t.Errorf("a refused link in the flood: %v, want it ignored", err)
	}
	if followed != linkBurst || h.view().Current != linkBurst%3 || h.em.broughtForward() != linkBurst {
		t.Errorf("followed %d, tab %d, brought forward %d times; want %d, %d, %d",
			followed, h.view().Current, h.em.broughtForward(), linkBurst, linkBurst%3, linkBurst)
	}
	if got := h.em.toastCount(); got != toasts {
		t.Errorf("%d toasts for ignored links, the last %q", got-toasts, h.em.lastToast())
	}
	// Scripts go on.
	mustOK(t, h.e.RunScript([]string{"tab", "Research"}))
	if got := h.view().Current; got != 2 {
		t.Errorf("bench tab Research in a flood of links: tab %d", got)
	}
	h.advance(linkWindow)
	mustOK(t, h.e.FollowLink("bench://tab/Comms"))
	if got := h.view().Current; got != 0 {
		t.Errorf("a link after the flood: tab %d, want Comms", got)
	}
}

// TestLinkAndScriptSwitchesAreTimed checks that a link's and a script's
// switches are timed with reasons of their own, among the tab switches in
// Report a Problem (decision 67), and show the tab as current at once,
// as a tab's shortcut does: switching, with the tab, before the View that
// has it current. A link to the current tab, which arranges it again,
// sends no switching.
func TestLinkAndScriptSwitchesAreTimed(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	h.settle()
	steps := []struct {
		name   string
		do     func() error
		reason string
		tab    int
	}{
		{"bench://tab/Code", func() error { return h.e.FollowLink("bench://tab/Code") }, reasonLink, 1},
		{"bench://next", func() error { return h.e.FollowLink("bench://next") }, reasonLink, 2},
		{"bench tab Comms", func() error { return h.e.RunScript([]string{"tab", "Comms"}) }, reasonScript, 0},
		{"bench last", func() error { return h.e.RunScript([]string{"last"}) }, reasonScript, 2},
	}
	for _, s := range steps {
		mark := markTimes(h)
		h.em.mu.Lock()
		from, seen := len(h.em.commands), len(h.em.states)
		h.em.mu.Unlock()
		mustOK(t, s.do())
		h.advance(100 * time.Millisecond)
		h.em.mu.Lock()
		got, at, views := slices.Clone(h.em.commands[from:]), slices.Clone(h.em.commandAt[from:]), slices.Clone(h.em.states[seen:])
		h.em.mu.Unlock()
		if !isSwitching(got, s.tab) {
			t.Errorf("%s sent %+v, want switching to %d", s.name, got, s.tab)
		} else if first := slices.IndexFunc(views, func(v View) bool { return v.Current == s.tab }); first < 0 || at[0]-seen > first {
			t.Errorf("%s: switching came after %d new views, want before the first with tab %d", s.name, at[0]-seen, s.tab)
		}
		if reasons := arrangeReasons(h, mark); len(reasons) == 0 || reasons[0] != s.reason {
			t.Errorf("%s: reasons %v, want %s first", s.name, reasons, s.reason)
		}
		if got := h.view().Current; got != s.tab {
			t.Errorf("%s: tab %d, want %d", s.name, got, s.tab)
		}
		h.settle()
	}
	r := h.e.ProblemReport()
	for _, reason := range []string{reasonLink, reasonScript} {
		if !slices.ContainsFunc(r.Switching, func(a ArrangeTiming) bool { return a.Reason == reason && a.Count == 2 }) {
			t.Errorf("Report a Problem's tab switches: %+v, want two of %s", r.Switching, reason)
		}
	}
	h.em.mu.Lock()
	from := len(h.em.commands)
	h.em.mu.Unlock()
	mustOK(t, h.e.FollowLink("bench://tab/Research"))
	if got := h.em.commandsNamed(switchingCommand); len(got) != len(steps) {
		t.Errorf("a link to the current tab sent switching: %+v", h.em.commands[from:])
	}

	// bench move-to-tab moves a window without switching: it's timed as
	// other, as the shortcut's send to a tab is, not among the switches.
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	mark := markTimes(h)
	mustOK(t, h.e.RunScript([]string{"move-to-tab", "Research"}))
	h.advance(100 * time.Millisecond)
	if it := h.item("w3"); it.Tab != 2 {
		t.Fatalf("bench move-to-tab left w3 on tab %d", it.Tab)
	}
	if reasons := arrangeReasons(h, mark); len(reasons) == 0 || slices.ContainsFunc(reasons, isSwitch) {
		t.Errorf("bench move-to-tab: reasons %v, want %s, and no switch", reasons, reasonOther)
	}
}

// TestRunScriptMoveToTabWhileTheDropdownShows checks that bench
// move-to-tab is refused, with a toast that says why, while the drop-down
// shows, as it has the keyboard then (decision 72), and works once it's
// hidden; and that bench list has the drop-down, which is in no tab, with
// whether it shows and what needs the user in it, which counts in the
// totals.
func TestRunScriptMoveToTabWhileTheDropdownShows(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if l, err := h.e.ScriptList(); err != nil || l.Dropdown != nil {
		t.Fatalf("with no drop-down: %+v, %v", l.Dropdown, err)
	}
	mustOK(t, h.e.ToggleDropdown())
	h.drain()
	d := dropdownOf(t, h)
	err := h.e.RunScript([]string{"move-to-tab", "Research"})
	if !errors.Is(err, errDropdownHasKeys) || h.em.lastToast() != errDropdownHasKeys.Error() {
		t.Errorf("move-to-tab with the drop-down shown: %v, toast %q", err, h.em.lastToast())
	}
	if it := h.item("w3"); it.Tab != 1 {
		t.Errorf("w3 went to tab %d", it.Tab)
	}
	l, err := h.e.ScriptList()
	if err != nil {
		t.Fatal(err)
	}
	if dd := l.Dropdown; dd == nil || dd.ID != d.ID || dd.Kind != control.KindTerminal || !dd.Shown {
		t.Errorf("the list's drop-down: %+v, want %s, a terminal, shown", dd, d.ID)
	}
	for _, tab := range l.Tabs {
		for _, p := range tab.Panes {
			for _, it := range p.Items {
				if it.ID == d.ID {
					t.Errorf("the drop-down is in %s's pane %s", tab.Name, p.ID)
				}
			}
		}
	}

	mustOK(t, h.e.HideDropdown())
	h.drain()
	h.terms.cb.Alert(d.ID, term.Alert{Bell: true})
	h.settle()
	if l, _ = h.e.ScriptList(); l.Dropdown == nil || l.Dropdown.Shown || l.Dropdown.Attention == "" || l.Attention != 1 {
		t.Errorf("hidden, with its bell rung: drop-down %+v, attention %d", l.Dropdown, l.Attention)
	}
	mustOK(t, h.e.RunScript([]string{"move-to-tab", "Research"}))
	if it := h.item("w3"); it.Tab != 2 {
		t.Errorf("with the drop-down hidden, w3 is in tab %d, want Research", it.Tab)
	}
}

// TestScriptListHasColoursAndPages checks what bench list says of tabs'
// colours (decision 74), and of web pages: whether each keeps running in
// the background, and whether Bench hasn't loaded it (decision 76).
func TestScriptListHasColoursAndPages(t *testing.T) {
	h := newScriptsHarness(t, setup{config: scriptsConfig(t)})
	l, err := h.e.ScriptList()
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"blue", "green", ""} {
		if got := l.Tabs[i].Colour; got != want {
			t.Errorf("%s's colour: %q, want %q", l.Tabs[i].Name, got, want)
		}
	}

	h = newScriptsHarness(t, setup{config: "[settings]\nallow_scripts = true\n" + keepConfig})
	h.settle()
	l, err = h.e.ScriptList()
	if err != nil {
		t.Fatal(err)
	}
	type page struct{ keep, notLoaded bool }
	got := map[string]page{}
	for _, tab := range l.Tabs {
		for _, p := range tab.Panes {
			for _, it := range p.Items {
				got[it.App] = page{it.KeepRunning, it.NotLoaded}
			}
		}
	}
	want := map[string]page{
		"Slack": {}, "Gmail": {keep: true}, "Outlook": {},
		"Discord": {notLoaded: true}, "Teams": {keep: true}, "Terminal": {},
	}
	if !maps.Equal(got, want) {
		t.Errorf("pages: %+v, want %+v", got, want)
	}
}
