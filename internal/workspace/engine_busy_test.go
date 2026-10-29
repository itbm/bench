package workspace

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// busyConfig has terminals in two tabs: claude in Code, and in Web, one
// that runs npm run dev and a shell, each named as its sub-tab is titled.
const busyConfig = `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ terminal = "~/code", name = "claude" }]

[[tab]]
name = "Web"
layout = "side"
  [[tab.pane]]
  items = [{ terminal = "~/code/web", command = "npm run dev", name = "npm run dev" }]
  [[tab.pane]]
  items = [{ terminal = "~/code/web", name = "Shell" }]
`

// busyHarness starts an engine with busyConfig, with settings before the
// tabs, and every terminal's session started. It returns the terminals'
// item IDs by sub-tab title.
func busyHarness(t *testing.T, settings string) (*harness, map[string]string) {
	t.Helper()
	h := newHarness(t, setup{config: settings + busyConfig})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	ids := map[string]string{}
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			ids[it.Title] = it.ID
		}
	}
	for _, title := range []string{"claude", "npm run dev", "Shell"} {
		id := ids[title]
		if _, started := h.terms.started[id]; id == "" || !started {
			t.Fatalf("the %q terminal isn't running: terminals %v", title, ids)
		}
	}
	return h, ids
}

// commandsNamed returns the commands sent with a name.
func (r *recEmitter) commandsNamed(name string) []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Command
	for _, c := range r.commands {
		if c.Name == name {
			out = append(out, c)
		}
	}
	return out
}

// savedSettings returns config.toml's [settings] as saved.
func savedSettings(t *testing.T, h *harness) config.Settings {
	t.Helper()
	cfg, problems, err := config.Load(filepath.Join(h.dir, config.ConfigFile))
	if err != nil || len(problems) > 0 {
		t.Fatalf("loading config.toml: %v %v", err, problems)
	}
	return cfg.Settings
}

func TestAskToQuitWithNothingRunning(t *testing.T) {
	h, _ := busyHarness(t, "")
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false with no terminal running anything, want true")
	}
	if quit, answered := h.e.AskToQuitNow(); !quit || !answered {
		t.Errorf("AskToQuitNow() = %v, %v; want true, true", quit, answered)
	}
	if got := h.em.commandsNamed(confirmQuitCommand); len(got) != 0 {
		t.Errorf("asked before quitting: %+v", got)
	}
}

// TestAskToQuitNamesBusyTerminals checks that Bench asks before quitting
// while terminals are running something, naming each by its sub-tab title
// and tab, in the tabs' order, then those in windows of their own.
func TestAskToQuitNamesBusyTerminals(t *testing.T) {
	h, ids := busyHarness(t, "")
	h.terms.setBusy(ids["claude"], "claude")
	h.terms.setBusy(ids["npm run dev"], "npm run dev")
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.PopOut(ids["Shell"]))
	h.terms.setBusy(ids["Shell"], "tail")
	h.drain()

	if h.e.AskToQuit() {
		t.Fatal("AskToQuit() = true while terminals are running something, want false")
	}
	want := Command{Name: confirmQuitCommand, Terminals: []BusyTerminal{
		{ID: ids["claude"], Title: "claude", Tab: "Code", Running: "claude"},
		{ID: ids["npm run dev"], Title: "npm run dev", Tab: "Web", Running: "npm run dev"},
		{ID: ids["Shell"], Title: "Shell", Running: "tail"},
	}}
	got := h.em.commandsNamed(confirmQuitCommand)
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("commands = %+v\nwant %+v", got, want)
	}
	if h.em.quitCount() != 0 {
		t.Error("Bench quit without waiting for the user's answer")
	}

	// A terminal whose program has finished is no longer asked about.
	h.terms.setBusy(ids["claude"], "")
	h.terms.setBusy(ids["Shell"], "")
	h.terms.end(ids["npm run dev"])
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false once nothing is running, want true")
	}
}

// TestAskToQuitNowOnTheMainThread checks that the main thread's question
// sends the command without waiting, and isn't answered while the engine's
// lock is held, as the engine may be waiting for the main thread then.
func TestAskToQuitNowOnTheMainThread(t *testing.T) {
	h, ids := busyHarness(t, "")
	h.terms.setBusy(ids["claude"], "claude")
	if quit, answered := h.e.AskToQuitNow(); quit || !answered {
		t.Errorf("AskToQuitNow() = %v, %v; want false, true", quit, answered)
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(h.em.commandsNamed(confirmQuitCommand)) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := h.em.commandsNamed(confirmQuitCommand); len(got) != 1 {
		t.Errorf("commands = %+v, want one confirm-quit", got)
	}

	h.e.lock()
	quit, answered := h.e.AskToQuitNow()
	h.e.unlock()
	if answered {
		t.Errorf("AskToQuitNow() = %v, %v while the engine's lock is held; want it unanswered", quit, answered)
	}
}

func TestAskToQuitWithTheSettingOff(t *testing.T) {
	h, ids := busyHarness(t, "[settings]\nconfirm_quit_busy_terminals = false\n")
	h.terms.setBusy(ids["claude"], "claude")
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false with confirm_quit_busy_terminals off, want true")
	}
	mustOK(t, h.e.PopOut(ids["claude"]))
	if !h.e.AskToCloseTerminal(ids["claude"]) {
		t.Error("AskToCloseTerminal() = false with confirm_quit_busy_terminals off, want true")
	}
	if got := h.em.commandsNamed(confirmQuitCommand); len(got) != 0 {
		t.Errorf("asked before quitting: %+v", got)
	}
}

// TestAskToQuitWhileLoggingOut checks that Bench never stops a log out,
// restart, or shut down to ask, whether macOS said so as it began or says
// so as it asks Bench to quit.
func TestAskToQuitWhileLoggingOut(t *testing.T) {
	h, ids := busyHarness(t, "")
	h.terms.setBusy(ids["claude"], "claude")
	h.p.SetLoggingOut(true)
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false while the platform says the user is logging out, want true")
	}
	h.p.SetLoggingOut(false)
	h.p.EmitEvent(platform.Event{Kind: platform.LoggingOut})
	h.drain()
	h.advance(4 * time.Minute)
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false four minutes after macOS said the user is logging out, want true")
	}
	// The user cancelled the log out.
	h.advance(2 * time.Minute)
	if h.e.AskToQuit() {
		t.Error("AskToQuit() = true long after a log out, want false")
	}
}

// TestQuitAfterAsking checks the question's Quit: Bench quits, and with
// Don't ask again, it saves confirm_quit_busy_terminals off first.
func TestQuitAfterAsking(t *testing.T) {
	h, ids := busyHarness(t, "")
	h.terms.setBusy(ids["claude"], "claude")
	mustOK(t, h.e.Quit(false))
	if h.em.quitCount() != 1 {
		t.Errorf("Quit(false) quit %d times, want once", h.em.quitCount())
	}
	if !savedSettings(t, h).ConfirmQuitBusyTerminals {
		t.Error("Quit(false) turned confirm_quit_busy_terminals off")
	}

	mustOK(t, h.e.Quit(true))
	if h.em.quitCount() != 2 {
		t.Errorf("Quit(true) didn't quit: %d quits", h.em.quitCount())
	}
	if savedSettings(t, h).ConfirmQuitBusyTerminals {
		t.Error("Quit(true) didn't save confirm_quit_busy_terminals off")
	}
	if !strings.Contains(readFile(t, h, config.ConfigFile), "confirm_quit_busy_terminals = false") {
		t.Error("config.toml doesn't say confirm_quit_busy_terminals = false")
	}
	if h.view().Settings.ConfirmQuitBusyTerminals {
		t.Error("Settings doesn't show the setting off")
	}
	if !h.e.AskToQuit() {
		t.Error("AskToQuit() = false after Don't ask again, want true")
	}
}

// TestAskToCloseATerminalsWindow checks that closing the window of a
// popped-out terminal that's running something asks first, in that window.
func TestAskToCloseATerminalsWindow(t *testing.T) {
	h, ids := busyHarness(t, "")
	id := ids["claude"]
	mustOK(t, h.e.PopOut(id))
	if !h.e.AskToCloseTerminal(id) {
		t.Error("AskToCloseTerminal() = false with nothing running, want true")
	}
	h.terms.setBusy(id, "claude")
	if h.e.AskToCloseTerminal(id) {
		t.Fatal("AskToCloseTerminal() = true while the terminal is running something, want false")
	}
	want := Command{Name: confirmCloseCommand, ItemID: id, Terminals: []BusyTerminal{
		{ID: id, Title: "claude", Running: "claude"},
	}}
	got := h.em.commandsNamed(confirmCloseCommand)
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("commands = %+v\nwant %+v", got, want)
	}
	if _, ok := h.view().Items[id]; !ok {
		t.Error("asking removed the terminal")
	}
	// An item that has gone, or isn't a terminal, closes without asking.
	if !h.e.AskToCloseTerminal("w99") {
		t.Error("AskToCloseTerminal() = false for an item that has gone")
	}
}

// TestCloseTerminalAfterAsking checks the closing question's Close: the
// terminal ends and goes, and its window closes; with Don't ask again,
// confirm_quit_busy_terminals is saved off, and a toast says so.
func TestCloseTerminalAfterAsking(t *testing.T) {
	h, ids := busyHarness(t, "")
	id := ids["claude"]
	mustOK(t, h.e.PopOut(id))
	h.terms.setBusy(id, "claude")
	mustOK(t, h.e.CloseTerminal(id, false))
	h.drain()
	if _, ok := h.view().Items[id]; ok {
		t.Error("the terminal is still there")
	}
	if h.terms.Running(id) {
		t.Error("the terminal's session is still running")
	}
	h.em.mu.Lock()
	closed := slices.Clone(h.em.closed)
	h.em.mu.Unlock()
	if !slices.Equal(closed, []string{id}) {
		t.Errorf("CloseTerminalWindow: %v, want [%s]", closed, id)
	}
	if !savedSettings(t, h).ConfirmQuitBusyTerminals {
		t.Error("CloseTerminal(false) turned confirm_quit_busy_terminals off")
	}
	mustErr(t, h.e.CloseTerminal(id, false), errGone.Error())

	// Don't ask again, for a terminal in a tab.
	shell := ids["Shell"]
	mustOK(t, h.e.CloseTerminal(shell, true))
	h.drain()
	if _, ok := h.view().Items[shell]; ok {
		t.Error("the terminal in a tab is still there")
	}
	if savedSettings(t, h).ConfirmQuitBusyTerminals {
		t.Error("CloseTerminal(true) didn't save confirm_quit_busy_terminals off")
	}
	if got := h.em.lastToast(); got != stopAskingText {
		t.Errorf("toast = %q, want %q", got, stopAskingText)
	}
}
