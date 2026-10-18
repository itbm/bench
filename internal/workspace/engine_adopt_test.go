package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestConfigPlaceholderDoesNotBeatRules(t *testing.T) {
	// Calendar is listed in Code, and a rule sends its windows to Research.
	cfg := strings.Replace(string(config.DefaultTOML()),
		`items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]`,
		`items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }, { app = "com.apple.iCal" }]`, 1)
	cfg = strings.Replace(cfg, `float = true`, "float = true\n\n[[rule]]\napp = \"com.apple.iCal\"\ntab = \"Research\"", 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(2))
	// From Research, the user opens Calendar from the Dock. Its window is
	// called "Calendar", as is Code's placeholder, which never had a window.
	h.p.LaunchApp(calendarID)
	h.drain()
	id := h.itemOf(h.windowID(calendarID, "Calendar"))
	if id == "" || h.item(id).Tab != 2 {
		t.Errorf("Calendar's window is item %q in tab %d, want Research, as the rule says", id, h.item(id).Tab)
	}
}

// lateID is an app that opens its window a while after it launches, as
// Electron apps do.
const lateID = "com.example.Late"

func TestAddAppWaitsForALateWindow(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(lateID, "Late")
	}})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: lateID}))
	h.advance(2 * time.Second)
	if n := len(h.calls("NewWindow")); n != 0 {
		t.Fatalf("Bench opened %d windows before the app's own came", n)
	}
	w, err := h.p.OpenWindow(lateID, "Late")
	mustOK(t, err)
	h.advance(2 * time.Second)
	if n := len(h.calls("NewWindow")); n != 0 {
		t.Errorf("Bench opened %d windows, although the app's own went to the pane", n)
	}
	if id := h.itemOf(w.ID); id == "" || h.paneOf(id) != "p12" {
		t.Errorf("the app's window is item %q, want it in p12", id)
	}
}

func TestAddAppOpensAWindowAfterTheLaunchTimeout(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(lateID, "Late")
	}})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "app", BundleID: lateID}))
	h.advance(14 * time.Second)
	if n := len(h.calls("NewWindow")); n != 0 {
		t.Fatalf("Bench opened %d windows before the launch timeout", n)
	}
	h.advance(2 * time.Second)
	if n := len(h.calls("NewWindow")); n != 1 {
		t.Fatalf("Bench opened %d windows after the launch timeout, want 1", n)
	}
	id := h.itemOf(h.windowID(lateID, platform.DefaultNewWindowTitle))
	if id == "" || h.paneOf(id) != "p12" {
		t.Errorf("the new window is item %q, want it in p12", id)
	}
}

// gmailConfig has a Brave item with a title filter and a url in Mail.
const gmailConfig = `
[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Mail"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "Gmail", url = "https://mail.google.com" }]
  [[tab.pane]]
  items = [{ terminal = "~/code" }]
`

func TestSwitchWaitsForAWindowsTitle(t *testing.T) {
	h := newHarness(t, setup{config: gmailConfig, world: func(p *platform.Fake) {
		// Brave's window for the url is first titled with the host.
		p.InstallApp(braveID, "Brave", platform.AppURLWindow("https://mail.google.com", "mail.google.com"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	w := h.windowID(braveID, "mail.google.com")
	if got := h.itemOf(w); got == "w2" {
		t.Fatal("the window shouldn't suit the Gmail item yet")
	}
	// The page loads, and its title arrives.
	mustOK(t, h.p.RetitleWindow(w, "Inbox - Gmail"))
	h.drain()
	if got := h.itemOf(w); got != "w2" {
		t.Fatalf("the window is item %q, want the Gmail item w2", got)
	}
	if got := layout.Items(h.tabNamed("Mail").Root); !slices.Equal(got, []string{"w2", "w3"}) {
		t.Errorf("Mail's items: %v, want the Gmail item and the terminal only", got)
	}
	h.settle()
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if calls := h.calls("OpenApp"); len(calls) != 0 {
		t.Errorf("switching back opened Brave again: %v", calls)
	}
}

func TestConfigEditedByHandWithASavedState(t *testing.T) {
	data := savedState(t, nil)
	// With Bench quit, the user renames Slack, changes Outlook's address,
	// and removes Zed from Code.
	cfg := strings.NewReplacer(
		`profile = "slack", name = "Slack"`, `profile = "slack", name = "Team chat"`,
		`web = "https://outlook.office.com/mail/"`, `web = "https://outlook.office.com/owa/"`,
		`items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]`, `items = [{ app = "com.microsoft.VSCode" }]`,
	).Replace(string(config.DefaultTOML()))
	h := newHarness(t, setup{config: cfg, state: writeState(data)})
	v := h.view()
	if it := v.Items["w1"]; it.App != "Team chat" {
		t.Errorf("Slack is called %q, want Team chat", it.App)
	}
	if got := h.tabNamed("Comms").Windows; got != 2 {
		t.Errorf("Comms has %d items, want Slack and one Outlook", got)
	}
	if wv := h.p.WebViews()["w2"]; wv.View.URL != "https://outlook.office.com/owa/" {
		t.Errorf("Outlook loads %q, want its new address", wv.View.URL)
	}
	if _, ok := v.Items["w3"]; ok {
		t.Error("Zed's item should be gone with its entry")
	}
	if id := h.itemOf(h.windowID(zedID, "bench — switch.go")); id != "" {
		t.Errorf("Zed's window is item %s, want it left alone", id)
	}
}

func TestClosingAPoppedOutTerminalsWindowRemovesIt(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// A terminal in a tab has no window of its own to close.
	h.e.TerminalWindowClosed("w5")
	if _, ok := h.view().Items["w5"]; !ok {
		t.Fatal("a docked terminal went")
	}
	mustOK(t, h.e.PopOut("w5"))
	h.e.TerminalWindowClosed("w5")
	h.drain()
	if _, ok := h.view().Items["w5"]; ok {
		t.Error("closing the terminal's window should remove it")
	}
	if h.terms.Running("w5") {
		t.Error("closing the terminal's window should end its session")
	}
	h.em.mu.Lock()
	h.em.popped = nil
	h.em.mu.Unlock()
	h.restart()
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	if len(h.em.popped) != 0 {
		t.Errorf("terminal windows opened again at the next start: %v", h.em.popped)
	}
}

func TestPoppedOutVSCodeReopensItsWindow(t *testing.T) {
	startVSCode := func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return &fakeServer{}, nil }
	}
	h := newHarness(t, setup{config: vscodeConfig, options: startVSCode})
	h.advance(100 * time.Millisecond)
	mustOK(t, h.e.PopOut("w1"))
	mustOK(t, h.e.Close())
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	mustOK(t, err)

	// At the next start, the server isn't running until a while after
	// Start.
	h = newHarness(t, setup{config: vscodeConfig, state: writeState(data), options: startVSCode})
	h.advance(100 * time.Millisecond)
	if calls := h.calls("PopOutWebView"); len(calls) != 1 || calls[0].Arg != "w1" {
		t.Errorf("VS Code's window should open again once its server has started: %v", calls)
	}
	if wv := h.p.WebViews()["w1"]; !wv.PoppedOut || wv.View.URL != "http://localhost:9999/?tkn=t&folder=~/code/bench" {
		t.Errorf("VS Code's web view: %+v", wv)
	}
	mustOK(t, h.e.Close())
	st, err := config.LoadState(filepath.Join(h.dir, config.StateFile), parseConfig(t, vscodeConfig))
	if err != nil || st == nil || !st.Items["w1"].OwnWindow {
		t.Errorf("state.json should still record VS Code's window: %v", err)
	}
}
