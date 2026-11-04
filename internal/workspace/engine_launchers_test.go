package workspace

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// launchersTOML adds launchers to the example config: two terminals and
// two apps' windows.
const launchersTOML = `
[[launcher]]
name = "Claude"
folder = "~/code/bench"
run = "claude --continue"

[[launcher]]
name = "Deploy"
run = "make deploy"
start_again = false

[[launcher]]
name = "Bench in Zed"
app = "dev.zed.Zed"
folder = "~/code/bench"

[[launcher]]
name = "Notes"
app = "com.example.Editor"
folder = "~/notes"
`

// editorID is an editor that opens ~/notes in a window of its own.
const editorID = "com.example.Editor"

// launcherWorld is the standard world with an editor that isn't running,
// and opens ~/notes (in /Users/me) in a window called "notes — todo.md".
func launcherWorld(p *platform.Fake) {
	standardWorld(p)
	p.InstallApp(editorID, "Editor", platform.AppURLWindow("file:///Users/me/notes", "notes — todo.md"))
}

// newLauncherHarness starts an engine with launchersTOML, in the Code tab,
// with /Users/me as the home folder.
func newLauncherHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("HOME", "/Users/me")
	h := newHarness(t, setup{config: exampleConfig(t) + launchersTOML, world: launcherWorld})
	mustOK(t, h.e.SwitchTab(1))
	return h
}

// startedWith returns how a terminal's session was started, failing the
// test if it wasn't.
func (h *harness) startedWith(id string) term.Options {
	h.t.Helper()
	h.terms.mu.Lock()
	defer h.terms.mu.Unlock()
	opts, ok := h.terms.started[id]
	if !ok {
		h.t.Fatalf("terminal %s didn't start", id)
	}
	return opts
}

// forgetSessions forgets the terminals' sessions, as quitting Bench ends
// them, so that the next run starts them again.
func (f *fakeTerms) forgetSessions() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = map[string]term.Options{}
}

// paneShows returns the item that a pane shows.
func (h *harness) paneShows(paneID string) string {
	h.t.Helper()
	for _, p := range h.view().Display.Panes {
		if p.ID == paneID {
			return p.Active
		}
	}
	h.t.Fatalf("pane %s isn't displayed", paneID)
	return ""
}

// TestAddTerminalWithCommand checks the New terminal… form: the terminal
// starts the user's shell in the folder and types the command into it,
// pressing Return, with the name as its label; after a restart it runs the
// command again, unless it isn't to start again, when it only types it.
func TestAddTerminalWithCommand(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: "terminal", Folder: " ~/code/bench ", Command: " claude --continue ", Name: "Claude"}))
	claude := h.paneShows("p12")
	if opts := h.startedWith(claude); !reflect.DeepEqual(opts, term.Options{Dir: "~/code/bench", Input: "claude --continue\r"}) {
		t.Errorf("started with %+v, want the command typed into the shell", opts)
	}
	if it := h.item(claude); it.Title != "Claude" || it.Folder != "~/code/bench" {
		t.Errorf("terminal: %+v", it)
	}
	no := false
	mustOK(t, h.e.AddToPane("p12", Source{Type: "terminal", Command: "make deploy", StartAgain: &no}))
	deploy := h.paneShows("p12")
	if opts := h.startedWith(deploy); opts.Input != "make deploy\r" || opts.Dir != "~" {
		t.Errorf("started with %+v, want the command run now, whether or not it starts again", opts)
	}

	h.terms.forgetSessions()
	h.restart()
	mustOK(t, h.e.SwitchTab(1))
	if opts := h.startedWith(claude); opts.Input != "claude --continue\r" {
		t.Errorf("after a restart: %+v, want the command run again", opts)
	}
	if opts := h.startedWith(deploy); opts.Input != "make deploy" {
		t.Errorf("after a restart: %+v, want the command typed, not run", opts)
	}
	if it := h.item(claude); it.Title != "Claude" {
		t.Errorf("after a restart: %+v, want its name kept", it)
	}
}

// TestRunEntriesInConfig checks config.toml's run and start_again on a
// terminal entry, and that a terminal whose shell ends goes, as a plain
// one does, rather than staying as one whose command ended.
func TestRunEntriesInConfig(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), `items = [{ terminal = "~/code" }]`,
		`items = [{ terminal = "~/code", run = "npm run dev" }, { terminal = "~", run = "htop", start_again = false }]`, 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(1))
	pane := h.view().Display.Panes[1]
	if len(pane.Items) != 2 {
		t.Fatalf("pane %s has %v, want the two terminals", pane.ID, pane.Items)
	}
	dev, top := pane.Items[0], pane.Items[1]
	mustOK(t, h.e.ShowItem(pane.ID, dev))
	if opts := h.startedWith(dev); opts.Input != "npm run dev\r" || opts.Command != "" {
		t.Errorf("%s started with %+v, want npm run dev typed and run", dev, opts)
	}
	mustOK(t, h.e.ShowItem(pane.ID, top))
	if opts := h.startedWith(top); opts.Input != "htop" {
		t.Errorf("%s started with %+v, want htop typed, not run", top, opts)
	}

	// Saving the tab keeps the entries as they are.
	mustOK(t, h.e.SaveTab(1, draftOf(h.view(), "Code", "main-side", "stack")))
	no := false
	want := []config.Item{{Terminal: "~/code", Run: "npm run dev"}, {Terminal: "~", Run: "htop", StartAgain: &no}}
	if got := savedConfig(t, h).Tabs[1].Panes[1].Items; !reflect.DeepEqual(got, want) {
		t.Errorf("saved entries %+v, want %+v", got, want)
	}

	h.terms.cb.Exit(dev, nil)
	h.advance(frame)
	if _, ok := h.view().Items[dev]; ok {
		t.Errorf("%s's shell ended, so it should have gone", dev)
	}
}

// TestLaunchersComeFirst checks that the add dialog lists the launchers
// first, and that the view has them for Jump to… and Settings.
func TestLaunchersComeFirst(t *testing.T) {
	h := newLauncherHarness(t)
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	if len(opts) < 4 {
		t.Fatalf("options: %+v", opts)
	}
	want := []AddOption{
		{Group: "Launchers", Mono: "Cl", Label: "Claude", Hint: "claude --continue · ~/code/bench", Source: Source{Type: "launcher", ID: "Claude"}},
		{Group: "Launchers", Mono: "De", Label: "Deploy", Hint: "make deploy · ~", Source: Source{Type: "launcher", ID: "Deploy"}},
		{Group: "Launchers", Mono: "Zd", Icon: appIcon(zedID), Label: "Bench in Zed", Hint: "Zed · ~/code/bench", Source: Source{Type: "launcher", ID: "Bench in Zed"}},
		{Group: "Launchers", Mono: "Ed", Icon: appIcon(editorID), Label: "Notes", Hint: "Editor · ~/notes", Source: Source{Type: "launcher", ID: "Notes"}},
	}
	if !reflect.DeepEqual(opts[:4], want) {
		t.Errorf("launchers:\n%+v\nwant\n%+v", opts[:4], want)
	}
	if opts[4].Group != "Windows not in a tab" {
		t.Errorf("after the launchers: %+v", opts[4])
	}
	ls := h.view().Settings.Launchers
	if len(ls) != 4 {
		t.Fatalf("view's launchers: %+v", ls)
	}
	if l := ls[1]; l.Kind != "terminal" || l.Command != "make deploy" || l.StartAgain || l.Folder != "" {
		t.Errorf("Deploy: %+v", l)
	}
	if l := ls[0]; !l.StartAgain {
		t.Errorf("Claude starts again when start_again is missing: %+v", l)
	}
	if l := ls[2]; l.Kind != "app" || l.BundleID != zedID || l.App != "Zed" || l.Folder != "~/code/bench" {
		t.Errorf("Bench in Zed: %+v", l)
	}
}

// TestAddTerminalLauncher checks that a terminal launcher adds a new
// terminal each time, which types its command into the shell, with its
// name as the label.
func TestAddTerminalLauncher(t *testing.T) {
	h := newLauncherHarness(t)
	mustOK(t, h.e.AddToPane("p12", Source{Type: "launcher", ID: "claude"}))
	first := h.paneShows("p12")
	if opts := h.startedWith(first); !reflect.DeepEqual(opts, term.Options{Dir: "~/code/bench", Input: "claude --continue\r"}) {
		t.Errorf("started with %+v", opts)
	}
	if it := h.item(first); it.Title != "Claude" {
		t.Errorf("title %q, want the launcher's name", it.Title)
	}
	mustOK(t, h.e.AddToPane("p11", Source{Type: "launcher", ID: "Deploy"}))
	deploy := h.paneShows("p11")
	if deploy == first || h.startedWith(deploy).Input != "make deploy\r" {
		t.Errorf("Deploy runs its command as it's added: %+v", h.startedWith(deploy))
	}
	mustErr(t, h.e.AddToPane("p12", Source{Type: "launcher", ID: "Nope"}),
		"There's no launcher called Nope. Settings ▸ Launchers lists them.")

	// A launcher's terminal that's running something is busy, as Bench
	// quits.
	h.terms.setBusy(first, "claude")
	busy := h.e.busyTerminalsForTest()
	if len(busy) != 1 || busy[0].ID != first || busy[0].Running != "claude" || busy[0].Title != "Claude" {
		t.Errorf("busy terminals: %+v", busy)
	}
}

// busyTerminalsForTest returns the terminals that are running something.
func (e *Engine) busyTerminalsForTest() []BusyTerminal {
	e.lock()
	defer e.unlock()
	return e.busyTerminalsLocked()
}

// TestAddAppLauncher checks an app launcher: a window of its app with its
// title that's open already is shown, and otherwise the app opens the
// launcher's folder and its window comes to the pane.
func TestAddAppLauncher(t *testing.T) {
	h := newLauncherHarness(t)
	// Zed's "bench — switch.go" is in the tab already.
	h.p.ResetCalls()
	mustOK(t, h.e.AddToPane("p12", Source{Type: "launcher", ID: "Bench in Zed"}))
	if h.paneShows("p11") != "w3" || len(h.calls("OpenApp")) != 0 {
		t.Errorf("Zed's window should be shown in its pane, not opened again: p11 shows %s, calls %v",
			h.paneShows("p11"), h.calls("OpenApp"))
	}

	mustOK(t, h.e.AddToPane("p12", Source{Type: "launcher", ID: "Notes"}))
	if !h.called("OpenApp", func(c platform.Call) bool { return c.BundleID == editorID && c.Arg == "file:///Users/me/notes" }) {
		t.Fatalf("the editor should open the folder: %v", h.calls("OpenApp"))
	}
	h.settle()
	notes := h.windowID(editorID, "notes — todo.md")
	id := h.itemOf(notes)
	if id == "" || h.paneOf(id) != "p12" {
		t.Fatalf("the editor's window should be in p12: item %q", id)
	}
	if h.frame(notes) != h.bodyOf("p12") {
		t.Errorf("the editor's window should dock in p12")
	}
	h.e.lock()
	it := *h.e.model.Items[id]
	h.e.unlock()
	if it.TitleFilter != "notes" || it.OpenURL != "file:///Users/me/notes" {
		t.Errorf("item: filter %q url %q, want the launcher's", it.TitleFilter, it.OpenURL)
	}

	// Chosen again, it shows the same window.
	mustOK(t, h.e.ShowItem("p12", "w5"))
	h.p.ResetCalls()
	mustOK(t, h.e.AddToPane("p12", Source{Type: "launcher", ID: "Notes"}))
	if h.paneShows("p12") != id || len(h.calls("OpenApp")) != 0 {
		t.Errorf("p12 shows %s, want %s; calls %v", h.paneShows("p12"), id, h.calls("OpenApp"))
	}
}

// TestAppLauncherThatCantOpen checks that an app launcher whose app isn't
// installed leaves nothing behind.
func TestAppLauncherThatCantOpen(t *testing.T) {
	h := newLauncherHarness(t)
	before := len(h.view().Items)
	savedLauncher(t, h.e, LauncherView{Name: "Gone", Kind: "app", BundleID: "com.example.Gone", URL: "https://example.com", Title: "Example"})
	err := h.e.AddToPane("p12", Source{Type: "launcher", ID: "Gone"})
	if err == nil || !strings.Contains(err.Error(), "isn't installed") {
		t.Errorf("got %v, want the app isn't installed", err)
	}
	if got := len(h.view().Items); got != before {
		t.Errorf("%d items, want %d: the launcher's item should go", got, before)
	}
}

// TestSaveLauncher checks Save as launcher: it adds the launcher to
// config.toml, names one without a name after its command, and refuses a
// name that's taken.
func TestSaveLauncher(t *testing.T) {
	h := newHarness(t, setup{})
	savedLauncher(t, h.e, LauncherView{Name: " Claude ", Kind: "terminal", Folder: "~/code/bench", Command: "claude", StartAgain: true})
	if got := h.em.lastToast(); got != "Saved launcher Claude" {
		t.Errorf("toast %q", got)
	}
	var names []string
	for _, v := range []LauncherView{{Command: "FOO=1 codex resume --last"}, {Command: "/usr/local/bin/codex"}, {Folder: "~/code/site/"}, {}} {
		names = append(names, savedLauncher(t, h.e, v))
	}
	if want := []string{"codex", "codex (1)", "site", "Terminal"}; !reflect.DeepEqual(names, want) {
		t.Errorf("SaveLauncher named them %q, want %q", names, want)
	}
	no := false
	want := []config.Launcher{
		{Name: "Claude", Folder: "~/code/bench", Run: "claude"},
		{Name: "codex", Run: "FOO=1 codex resume --last", StartAgain: &no},
		{Name: "codex (1)", Run: "/usr/local/bin/codex", StartAgain: &no},
		{Name: "site", Folder: "~/code/site/"},
		{Name: "Terminal"},
	}
	if got := savedConfig(t, h).Launchers; !reflect.DeepEqual(got, want) {
		t.Errorf("config.toml's launchers:\n%+v\nwant\n%+v", got, want)
	}
	if got := len(h.view().Settings.Launchers); got != 5 {
		t.Errorf("the view has %d launchers, want 5", got)
	}

	// A name that differs only in case is refused, naming the launcher
	// that has it.
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "claude", Command: "claude"})),
		"There's a launcher called Claude already. Give this one another name.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Tabs", Command: "a\tb"})),
		"The command of Tabs must be one line, without tabs.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Docs", Kind: "app", BundleID: braveID, URL: "https://docs.example.com"})),
		"Give Docs a window title too: Bench docks the Brave window whose title has it.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Docs", Kind: "app", BundleID: braveID})),
		"Give Docs a folder or a link to open in Brave.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Docs", Kind: "app", BundleID: braveID, Folder: "~", URL: "https://a.example"})),
		"Give Docs a folder or a link to open in Brave, not both.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Docs", Kind: "app", BundleID: braveID, URL: "docs.example.com", Title: "Docs"})),
		"The link of Docs needs a scheme, such as https://.")
	mustErr(t, errOf(h.e.SaveLauncher(LauncherView{Name: "Odd", Kind: "window"})), "A launcher opens a terminal or an app.")
	if got := len(savedConfig(t, h).Launchers); got != 5 {
		t.Errorf("%d launchers saved, want the refused ones left out", got)
	}
}

// TestSaveLauncherKeepsChangesMadeOutsideBench checks that Save as launcher
// doesn't write over config.toml once it has changed outside Bench.
func TestSaveLauncherKeepsChangesMadeOutsideBench(t *testing.T) {
	h := newHarness(t, setup{})
	edited := exampleConfig(t) + mailRule
	writeConfig(t, h, edited)
	savedLauncher(t, h.e, LauncherView{Name: "Claude", Command: "claude"})
	if got, want := h.em.lastToast(), "Applied for now, not saved: config.toml was changed outside Bench"; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if got := readFile(t, h, config.ConfigFile); got != edited {
		t.Errorf("config.toml was written over:\n%s", got)
	}
	if ls := h.view().Settings.Launchers; len(ls) != 1 || ls[0].Name != "Claude" {
		t.Errorf("launchers %+v, want the new one applied", ls)
	}
}

// TestSettingsEditLaunchers checks Settings ▸ Launchers: SaveSettings
// replaces the launchers with the dialog's, keeps them if it has none, and
// refuses ones config.toml couldn't have.
func TestSettingsEditLaunchers(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	h := newHarness(t, setup{config: exampleConfig(t) + launchersTOML, world: launcherWorld})
	s := h.view().Settings
	s.Launchers = slices.Clone(s.Launchers)
	s.Launchers[0].Command = "claude"
	s.Launchers[0].StartAgain = false
	s.Launchers = slices.Delete(s.Launchers, 1, 2) // Deploy
	mustOK(t, h.e.SaveSettings(s))
	no := false
	want := []config.Launcher{
		{Name: "Claude", Folder: "~/code/bench", Run: "claude", StartAgain: &no},
		{Name: "Bench in Zed", App: zedID, Folder: "~/code/bench"},
		{Name: "Notes", App: editorID, Folder: "~/notes"},
	}
	if got := savedConfig(t, h).Launchers; !reflect.DeepEqual(got, want) {
		t.Errorf("config.toml's launchers:\n%+v\nwant\n%+v", got, want)
	}

	// Settings without launchers keeps them.
	s = h.view().Settings
	s.Launchers = nil
	s.Gap = 6
	mustOK(t, h.e.SaveSettings(s))
	if got := savedConfig(t, h).Launchers; !reflect.DeepEqual(got, want) {
		t.Errorf("launchers %+v, want them kept", got)
	}

	s = h.view().Settings
	s.Launchers = append(slices.Clone(s.Launchers), LauncherView{Name: "CLAUDE", Kind: "terminal"})
	mustErr(t, h.e.SaveSettings(s), "There's a launcher called Claude already. Give this one another name.")

	// Launchers without a name are named as Save as launcher names them,
	// numbered if another launcher has the name.
	n := len(s.Launchers)
	s.Launchers[n-1] = LauncherView{Kind: "terminal", Command: "claude --resume"}
	s.Launchers = append(s.Launchers, LauncherView{Kind: "terminal", Folder: "~/code/site"},
		LauncherView{Kind: "app", BundleID: zedID, Folder: "~/code"}, LauncherView{Name: "site", Kind: "terminal"})
	mustOK(t, h.e.SaveSettings(s))
	var names []string
	for _, l := range savedConfig(t, h).Launchers[n-1:] {
		names = append(names, l.Name)
	}
	if want := []string{"claude (1)", "site (1)", "Zed", "site"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Settings named them %q, want %q", names, want)
	}

	// An empty list removes them all.
	s = h.view().Settings
	s.Launchers = []LauncherView{}
	mustOK(t, h.e.SaveSettings(s))
	if got := savedConfig(t, h).Launchers; got != nil {
		t.Errorf("launchers %+v, want none", got)
	}
	if strings.Contains(readFile(t, h, config.ConfigFile), "[[launcher]]") {
		t.Errorf("config.toml still has launchers")
	}
}

// TestReloadPicksUpLaunchers checks that launchers added to config.toml
// outside Bench are in the lists once it's reloaded.
func TestReloadPicksUpLaunchers(t *testing.T) {
	h := newHarness(t, setup{})
	writeConfig(t, h, exampleConfig(t)+launchersTOML)
	mustOK(t, h.e.ReloadConfig())
	opts, err := h.e.AddOptions(h.view().Display.Panes[0].ID)
	mustOK(t, err)
	if opts[0].Label != "Claude" || opts[0].Group != "Launchers" {
		t.Errorf("first option %+v, want the Claude launcher", opts[0])
	}
	if got := len(h.view().Settings.Launchers); got != 4 {
		t.Errorf("%d launchers, want 4", got)
	}
}

// TestChooseFolder checks the folder picker: it starts in the form's
// folder, as a full path, and gives back the folder written with ~. The
// browser preview has none.
func TestChooseFolder(t *testing.T) {
	t.Setenv("HOME", "/Users/me")
	var asked []string
	answer, fail := "/Users/me/code/bench", error(nil)
	h := newHarness(t, setup{options: func(o *Options) {
		o.ChooseFolder = func(start string) (string, error) {
			asked = append(asked, start)
			return answer, fail
		}
	}})
	got, err := h.e.ChooseFolder(" ~/code ")
	mustOK(t, err)
	if got != "~/code/bench" || !slices.Equal(asked, []string{"/Users/me/code"}) {
		t.Errorf("got %q, asked %v", got, asked)
	}
	answer = ""
	if got, err := h.e.ChooseFolder(""); err != nil || got != "" || asked[1] != "/Users/me" {
		t.Errorf("cancelled: %q, %v; asked %v", got, err, asked)
	}
	answer = "/Volumes/Work"
	if got, _ := h.e.ChooseFolder("/Volumes"); got != "/Volumes/Work" {
		t.Errorf("a folder outside home: %q", got)
	}
	fail = errors.New("no window")
	mustErr(t, func() error { _, err := h.e.ChooseFolder("~"); return err }(),
		"Bench couldn't show the folder picker. Type the folder instead.")

	preview := newHarness(t, setup{})
	_, err = preview.e.ChooseFolder("~")
	mustErr(t, err, "This build of Bench can't show a folder picker. Type the folder instead, such as ~/code/bench.")
}

// savedLauncher returns the name that SaveLauncher saved a launcher with,
// failing the test if it refused it.
func savedLauncher(t *testing.T, e *Engine, v LauncherView) string {
	t.Helper()
	name, err := e.SaveLauncher(v)
	mustOK(t, err)
	return name
}

// errOf returns the error of a call that returns a value too.
func errOf[T any](_ T, err error) error {
	return err
}
