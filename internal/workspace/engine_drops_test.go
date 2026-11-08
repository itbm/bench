package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// TestShellPath checks how a dropped file's path is typed into a terminal:
// as Terminal types it, with a backslash before each character that shells
// treat specially, letters from other alphabets as they are, and $'...'
// for a path with a control character, or bytes that aren't UTF-8.
func TestShellPath(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"/Users/demo/notes.txt", "/Users/demo/notes.txt"},
		{"/Users/demo/Q4 report (final).pdf", `/Users/demo/Q4\ report\ \(final\).pdf`},
		{`/tmp/it's "here"`, `/tmp/it\'s\ \"here\"`},
		{"/Users/demo/Pictures/Ünïcödé café.jpg", `/Users/demo/Pictures/Ünïcödé\ café.jpg`},
		{"/tmp/a$b&c;d|e*f?g[h]~i!j#k<l>m{n}o^p`q", "/tmp/a\\$b\\&c\\;d\\|e\\*f\\?g\\[h\\]\\~i\\!j\\#k\\<l\\>m\\{n\\}o\\^p\\`q"},
		{"/tmp/=x", `/tmp/\=x`},
		{`/tmp/back\slash`, `/tmp/back\\slash`},
		{"/tmp/v1.2_final-copy+@50%,x:y", "/tmp/v1.2_final-copy+@50%,x:y"},
		{"/tmp/two\nlines", `$'/tmp/two\nlines'`},
		{"/tmp/tab\there's!", `$'/tmp/tab\there\'s\x21'`},
		{"/tmp/bell\a and é", `$'/tmp/bell\x07 and é'`},
		{"/tmp/bad\xffbyte", `$'/tmp/bad\xffbyte'`},
	} {
		if got := shellPath(c.path); got != c.want {
			t.Errorf("shellPath(%q) = %s, want %s", c.path, got, c.want)
		}
	}
}

// TestShellPathInAShell checks that a shell reads each quoted path as the
// path itself.
func TestShellPathInAShell(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash here")
	}
	for _, path := range []string{
		"/Users/demo/Q4 report (final).pdf", `/tmp/it's "here"`, "/Users/demo/Ünïcödé café.jpg",
		"/tmp/a$b&c;d|e*f?g[h]~i!j#k<l>m{n}o^p`q", "/tmp/=x", `/tmp/back\slash`,
		"/tmp/two\nlines", "/tmp/tab\there's!", "/tmp/bell\a", "/tmp/bad\xffbyte",
	} {
		out, err := exec.Command(bash, "-c", "printf %s "+shellPath(path)).Output()
		if err != nil || string(out) != path {
			t.Errorf("bash read %s as %q (%v), want %q", shellPath(path), out, err, path)
		}
	}
}

// pastes returns what the paste-terminal commands typed, by terminal.
func pastes(h *harness) []Command {
	return h.em.commandsNamed(pasteCommand)
}

// TestDropOnTerminal checks that paths dropped on a terminal are typed into
// it, quoted, with a space after each, by the window that shows it; and
// that a drop on a terminal that has ended, or on something that isn't a
// terminal, is refused.
func TestDropOnTerminal(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	term := h.paneShows("p12")
	h.drain()

	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: term},
		[]string{"/Users/demo/Q4 report.pdf", "notes.txt", "/Users/demo/Pictures/café.jpg", "/Users/demo/Q4 report.pdf"}))
	got := pastes(h)
	want := `/Users/demo/Q4\ report.pdf /Users/demo/Pictures/café.jpg `
	if len(got) != 1 || got[0].ItemID != term || got[0].Text != want {
		t.Fatalf("pastes %+v, want %q typed into %s", got, want, term)
	}
	// A folder or an app is typed as a path too, as Terminal types it.
	dir := t.TempDir()
	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: term}, []string{dir}))
	if got := pastes(h); len(got) != 2 || got[1].Text != shellPath(dir)+" " {
		t.Errorf("a folder: %+v", got)
	}

	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: term}, []string{"notes.txt"}), errDropNothing.Error())
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: "w3"}, []string{"/tmp/a"}), errNoTerminal.Error())
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: "gone"}, []string{"/tmp/a"}), errNoTerminal.Error())

}

// TestDropOnEndedTerminal checks that a drop on a terminal whose command
// has finished, which waits for Return to run it again, is refused.
func TestDropOnEndedTerminal(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	run := h.paneShows(layout.Leaves(h.view().Tabs[0].Root)[1].ID)
	h.terms.end(run)
	h.terms.cb.Exit(run, nil)
	h.clk.Advance(frame)
	if !h.item(run).Ended {
		t.Fatalf("the command terminal should have ended")
	}
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: run}, []string{"/tmp/a"}), errDropEnded.Error())
	if len(pastes(h)) != 0 {
		t.Errorf("nothing should be typed")
	}
}

// TestDropFolderAndAppOnPane checks a drop on a pane's header: a folder
// adds a new terminal in it, and an app is added as the add dialog adds
// it, opening it if it isn't running, while a file is left out, with a
// toast that says so.
func TestDropFolderAndAppOnPane(t *testing.T) {
	h := newHarness(t, setup{})
	dir := t.TempDir()
	site := filepath.Join(dir, "bench-site")
	if err := os.Mkdir(site, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := writeFile(t, filepath.Join(dir, "notes.txt"), "hello", 0o644)
	h.p.InstallApp("com.example.notes", "Notes Plus", platform.AppLaunchWindows("Today"))
	app := writeBundle(t, filepath.Join(dir, "Apps", "Notes Plus.app"), "com.example.notes", "Notes Plus")
	mustOK(t, h.e.SwitchTab(1))
	before := slices.Clone(h.view().Display.Panes[1].Items)

	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: "p12"}, []string{site, notes, app}))
	v := h.view()
	added := slices.DeleteFunc(slices.Clone(v.Display.Panes[1].Items), func(id string) bool { return slices.Contains(before, id) })
	if len(added) != 1 {
		t.Fatalf("p12 should have one new terminal: %v", v.Display.Panes[1].Items)
	}
	id := added[0]
	if it := v.Items[id]; it.Kind != KindBuiltin || it.Folder != site || h.paneShows("p12") != id {
		t.Errorf("the new terminal: %+v, p12 shows %s", it, h.paneShows("p12"))
	}
	if opts, ok := h.terms.started[id]; !ok || opts.Dir != site {
		t.Errorf("the terminal should start in %s: %+v", site, opts)
	}
	if !h.called("OpenApp", func(c platform.Call) bool { return c.BundleID == "com.example.notes" }) {
		t.Errorf("Notes Plus should open: %v", h.calls("OpenApp"))
	}
	if got := h.em.lastToast(); got != "Bench left out 1 file. Drop files on a terminal." {
		t.Errorf("toast %q", got)
	}
	h.settle()
	if w := h.itemOf(h.windowID("com.example.notes", "Today")); w == "" || h.paneOf(w) != "p12" {
		t.Errorf("Notes Plus's window should go in p12: item %q", w)
	}
	// The add dialog knows the app from then on, wherever it is.
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	if !slices.ContainsFunc(opts, func(o AddOption) bool { return o.Group == groupApps && o.Label == "Notes Plus" }) {
		t.Errorf("the add dialog should list Notes Plus")
	}
}

// TestDropOnPaneRefused checks the drops on a pane that add nothing: only
// files, a path that has gone, an app without a bundle ID, too many folders
// at once, and a pane that isn't there. A drop elsewhere says where to
// drop.
func TestDropOnPaneRefused(t *testing.T) {
	h := newHarness(t, setup{})
	dir := t.TempDir()
	notes := writeFile(t, filepath.Join(dir, "notes.txt"), "hello", 0o644)
	other := writeFile(t, filepath.Join(dir, "other.md"), "hello", 0o644)
	mustOK(t, h.e.SwitchTab(1))
	items := len(h.view().Items)
	on := DropTarget{Kind: DropPane, Pane: "p11"}

	mustErr(t, h.e.DropFiles(on, []string{notes, other}), "Drop folders and apps here. Drop files on a terminal.")
	mustErr(t, h.e.DropFiles(on, []string{filepath.Join(dir, "gone")}), errCantBrowse("gone").Error())
	broken := writeBundle(t, filepath.Join(dir, "Broken.app"), "", "Broken")
	mustErr(t, h.e.DropFiles(on, []string{broken, notes}), "Broken isn't an app that Bench can open.")
	var many []string
	for i := range dropLimit + 1 {
		many = append(many, filepath.Join(dir, "f"+strconv.Itoa(i)))
		if err := os.Mkdir(many[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustErr(t, h.e.DropFiles(on, many), "Drop at most 8 folders and apps at once. Bench opened none.")
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, many), "Drop at most 8 folders and apps at once. Bench opened none.")
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: "p99"}, many[:1]), errNoPane.Error())
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropElsewhere}, []string{notes}), "Drop on a terminal, a pane's header, or the + after the tabs.")
	mustErr(t, h.e.DropFiles(DropTarget{}, []string{notes}), errDropElsewhere.Error())
	if got := len(h.view().Items); got != items {
		t.Errorf("%d items, want %d: nothing should be added", got, items)
	}

	// With a folder too, the folder is added, and the rest is left out.
	mustOK(t, h.e.DropFiles(on, []string{notes, many[0], other, filepath.Join(dir, "gone")}))
	if got := h.em.lastToast(); got != "Bench left out 2 files. Drop files on a terminal. "+
		"Bench couldn't open gone. Check that it's still there." {
		t.Errorf("toast %q", got)
	}
	if it := h.item(h.paneShows("p11")); it.Kind != KindBuiltin || it.Folder != many[0] {
		t.Errorf("p11 should show the new terminal: %+v", it)
	}
}

// TestDropOnNewTab checks a drop on the + after the tabs: a new tab, named
// after the first thing dropped, with a pane for each, the folder's
// terminal in the first and the app's windows that weren't in a tab in the
// second; Bench switches to it, and config.toml gets it. Undo takes it
// away, with the terminal, and puts the windows back.
func TestDropOnNewTab(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	dir := t.TempDir()
	site := filepath.Join(dir, "bench-site")
	if err := os.Mkdir(site, 0o755); err != nil {
		t.Fatal(err)
	}
	brave := writeBundle(t, filepath.Join(dir, "Brave Browser.app"), braveID, "Brave Browser")
	docs, pr, packages := h.fid(braveID, "Wails v3 docs"), h.fid(braveID, "Pull request #42 · bench"), h.fid(braveID, "Go packages")
	before := floatingIDs(h.view())
	items := len(h.view().Items)

	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{site, brave}))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "bench-site"}) || v.Current != 2 {
		t.Fatalf("tabs %v, current %d; want a third tab, bench-site, current", got, v.Current)
	}
	tab := v.Tabs[2]
	panes := viewPanes(tab)
	if tab.Preset != layout.PresetSide || len(panes) != 2 || len(panes[0]) != 1 || len(panes[1]) != 3 {
		t.Fatalf("new tab: preset %s, panes %v; want side by side, the terminal, and Brave's three windows", tab.Preset, panes)
	}
	if it := v.Items[panes[0][0]]; it.Kind != KindBuiltin || it.Folder != site {
		t.Errorf("first pane: %+v, want a terminal in %s", it, site)
	}
	for _, f := range []string{docs, pr, packages} {
		if slices.Contains(floatingIDs(v), f) {
			t.Errorf("%s is still not in a tab", f)
		}
	}
	if p := layout.Leaves(tab.Root)[0]; tab.Focus != p.ID {
		t.Errorf("focus %s, want the first pane", tab.Focus)
	}
	if got := h.em.lastToast(); got != "Made a new tab, bench-site." || !h.em.lastHasAction() {
		t.Errorf("toast %q, with Undo %v", got, h.em.lastHasAction())
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "bench-site"}) {
		t.Errorf("config.toml's tabs %v", got)
	}

	undoLast(t, h)
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code"}) {
		t.Fatalf("after Undo: tabs %v", got)
	}
	if got := floatingIDs(v); !sameSet(got, before) {
		t.Errorf("after Undo, not in a tab: %v, want %v", got, before)
	}
	if got := len(v.Items); got != items {
		t.Errorf("after Undo, %d items, want %d: the terminal should go", got, items)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code"}) {
		t.Errorf("config.toml's tabs %v, want the new tab gone", got)
	}
	if got := h.em.lastToast(); got != "Deleted the new tab, bench-site, and put its windows back." {
		t.Errorf("toast %q", got)
	}

	// A second drop gets a numbered name; a folder alone has the single
	// layout, and Undo says only that the tab has gone.
	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{site}))
	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{site}))
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code", "bench-site", "bench-site (1)"}) || v.Tabs[3].Preset != layout.PresetSingle {
		t.Errorf("tabs %v", got)
	}
	undoLast(t, h)
	if got := h.em.lastToast(); got != "Deleted the new tab, bench-site (1)." {
		t.Errorf("toast %q", got)
	}
}

// TestDropTargetOf checks what a drop from Finder is on, from the
// attributes of the element of the page that Wails found under it.
func TestDropTargetOf(t *testing.T) {
	for _, c := range []struct {
		name  string
		attrs map[string]string
		want  DropTarget
	}{
		{"a terminal", map[string]string{"data-file-drop-target": "terminal", "data-drop-item": "w5", "class": "term-host"},
			DropTarget{Kind: DropTerminal, Item: "w5"}},
		{"a pane's header", map[string]string{"data-file-drop-target": "pane", "data-drop-pane": "p12"},
			DropTarget{Kind: DropPane, Pane: "p12"}},
		{"the tab +", map[string]string{"data-file-drop-target": "new-tab", "data-action": "new-tab"}, DropTarget{Kind: DropNewTab}},
		{"the window", map[string]string{"data-file-drop-target": "window"}, DropTarget{Kind: DropElsewhere}},
		{"something unknown", map[string]string{"data-file-drop-target": "sidebar", "data-drop-pane": "p1"},
			DropTarget{Kind: DropElsewhere, Pane: "p1"}},
		{"nothing", nil, DropTarget{Kind: DropElsewhere}},
	} {
		if got := DropTargetOf(c.attrs); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestDemoDropFiles checks the browser preview's pretend drops, which name
// the element they're on by its attributes, and that they're only for the
// preview.
func TestDemoDropFiles(t *testing.T) {
	h := newHarness(t, setup{})
	mustErr(t, h.e.DemoDropFiles([]string{"/tmp/a"}, nil), errNoDemoDrop.Error())

	demo := newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustOK(t, demo.e.SwitchTab(1))
	term := demo.paneShows("p12")
	demo.drain()
	mustOK(t, demo.e.DemoDropFiles([]string{"/tmp/a b"}, map[string]string{"data-file-drop-target": "terminal", "data-drop-item": term}))
	if got := pastes(demo); len(got) != 1 || got[0].ItemID != term || got[0].Text != `/tmp/a\ b ` {
		t.Errorf("pastes %+v", got)
	}
	mustErr(t, demo.e.DemoDropFiles([]string{"/tmp/a"}, nil), errDropElsewhere.Error())
}

// TestDropNothingOpens checks a drop on the + or a pane of only apps that
// don't open: Bench makes no tab, saves nothing in config.toml, keeps no
// Undo, adds nothing, and says why.
func TestDropNothingOpens(t *testing.T) {
	h := newHarness(t, setup{config: fromWindowsConfig})
	dir := t.TempDir()
	// Neither is installed, so neither opens.
	ghost := writeBundle(t, filepath.Join(dir, "Ghost.app"), "com.example.ghost", "Ghost")
	other := writeBundle(t, filepath.Join(dir, "Other.app"), "com.example.other", "Other")
	notes := writeFile(t, filepath.Join(dir, "notes.txt"), "hello", 0o644)
	v := h.view()
	tabs, items, toasts := tabNames(v), len(v.Items), len(h.em.toasts)

	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{ghost}), "Bench couldn't open Ghost.")
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{ghost, notes, other}), "Bench couldn't open Ghost.")
	pane := layout.Leaves(v.Tabs[v.Current].Root)[0].ID
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: pane}, []string{ghost, other}), "Bench couldn't open Ghost.")
	h.drain()
	v = h.view()
	if got := tabNames(v); !slices.Equal(got, tabs) || v.Current != 0 {
		t.Errorf("tabs %v, current %d; want %v as they were", got, v.Current, tabs)
	}
	if got := configTabNames(t, h); !slices.Equal(got, tabs) {
		t.Errorf("config.toml's tabs %v, want %v", got, tabs)
	}
	if v.Undo != nil {
		t.Errorf("Undo %+v, want nothing to take back", v.Undo)
	}
	if len(v.Items) != items || len(h.em.toasts) != toasts {
		t.Errorf("%d items and %d toasts, want %d and %d", len(v.Items), len(h.em.toasts), items, toasts)
	}
}

// TestDropAppOnPaneFocusesIt checks that a drop on a pane that isn't the
// focused one focuses it, as the user chose it: an app that is still to
// open gives no other pane's window the keyboard, so no other app comes in
// front of the Finder, and its window comes to that pane.
func TestDropAppOnPaneFocusesIt(t *testing.T) {
	h := newHarness(t, setup{})
	dir := t.TempDir()
	h.p.InstallApp("com.example.notes", "Notes Plus", platform.AppLaunchWindows("Today"))
	app := writeBundle(t, filepath.Join(dir, "Notes Plus.app"), "com.example.notes", "Notes Plus")
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if f := h.view().Tabs[1].Focus; f != "p11" {
		t.Fatalf("p11 should be focused first, not %s", f)
	}
	h.p.ResetCalls()

	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: "p12"}, []string{app}))
	if f := h.view().Tabs[1].Focus; f != "p12" {
		t.Errorf("focus %s, want p12, where the app was dropped", f)
	}
	if got := h.focusCalls(); slices.Contains(got, h.windowID(zedID, "bench — switch.go")) {
		t.Errorf("Focus calls %v: Zed, in p11, shouldn't get the keyboard", got)
	}
	h.settle()
	if w := h.itemOf(h.windowID("com.example.notes", "Today")); w == "" || h.paneOf(w) != "p12" {
		t.Errorf("Notes Plus's window should go in p12: item %q", w)
	}
}

// TestDropPackage checks that a package, such as an Xcode project, which
// the Finder shows as one document, is a file to Bench: refused on a pane
// or the +, and typed as a path on a terminal.
func TestDropPackage(t *testing.T) {
	h := newHarness(t, setup{})
	dir := t.TempDir()
	project := filepath.Join(dir, "MyApp.xcodeproj")
	library := filepath.Join(dir, "Photos Library.photoslibrary")
	for _, d := range []string{project, library} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustOK(t, h.e.SwitchTab(1))
	tabs := len(h.view().Tabs)
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropNewTab}, []string{project}), errDropFiles.Error())
	mustErr(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: "p11"}, []string{library}), errDropFiles.Error())
	if got := len(h.view().Tabs); got != tabs {
		t.Errorf("%d tabs, want %d", got, tabs)
	}
	term := h.paneShows("p12")
	h.drain()
	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropTerminal, Item: term}, []string{project}))
	if got := pastes(h); len(got) != 1 || got[0].Text != shellPath(project)+" " {
		t.Errorf("pastes %+v", got)
	}
}

// TestDropMessagesFit checks that each message after a drop fits on the
// app's toast, which is one line, in Bench's smallest window, even in the
// title bar while the status bar is hidden: about 64 characters.
func TestDropMessagesFit(t *testing.T) {
	const most = 64
	for _, msg := range []string{
		errDropElsewhere.Error(), errDropNothing.Error(), errDropEnded.Error(), errDropTooMany.Error(),
		errDropFiles.Error(), errNoTerminal.Error(), leftOutText(1), leftOutText(12),
	} {
		if n := utf8.RuneCountInString(msg); n > most {
			t.Errorf("%q has %d characters, more than %d", msg, n, most)
		}
	}
}

// TestDropFromFinderLeavesWhatCoversBench checks that a drop from the
// Finder on a pane or the + that gives a new terminal the keyboard, which
// brings Bench forward, leaves the windows over Bench where they are, the
// Finder's that the drag came from among them, with no toast about them
// (decision 68). Bench coming forward later, or after a drop that gave
// Bench nothing, is the user's doing, which puts them out of sight as
// before.
func TestDropFromFinderLeavesWhatCoversBench(t *testing.T) {
	type c struct {
		name, kind string
		app        bool          // Drop an app that isn't running, which doesn't bring Bench forward.
		late       time.Duration // How long after the drop Bench comes forward.
		hidden     bool          // Whether the Finder is hidden then.
	}
	for _, c := range []c{
		{name: "on a pane", kind: DropPane},
		{name: "on the +", kind: DropNewTab},
		{name: "later", kind: DropPane, late: dropForwardGrace, hidden: true},
		{name: "an app", kind: DropPane, app: true, hidden: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			dir := t.TempDir()
			site := filepath.Join(dir, "bench-site")
			if err := os.Mkdir(site, 0o755); err != nil {
				t.Fatal(err)
			}
			dropped := []string{site}
			if c.app {
				h.p.InstallApp("com.example.notes", "Notes Plus", platform.AppLaunchWindows("Today"))
				dropped = []string{writeBundle(t, filepath.Join(dir, "Notes Plus.app"), "com.example.notes", "Notes Plus")}
			}
			mustOK(t, h.p.ActivateApp(finderID))
			h.drain()
			on := DropTarget{Kind: c.kind}
			if c.kind == DropPane {
				on.Pane = h.view().Display.Focus
			}
			h.p.ResetCalls()

			mustOK(t, h.e.DropFiles(on, dropped))
			h.drain()
			if got := len(h.calls("FocusUI")) > 0; got == c.app {
				t.Fatalf("FocusUI called %v: the drop should give a new terminal the keyboard, and only a terminal", got)
			}
			h.clk.Advance(c.late)
			// macOS makes Bench the active app, as FocusUI asks.
			h.comeForward()
			if h.p.IsHidden(finderID) != c.hidden {
				t.Errorf("the Finder hidden %v, want %v", h.p.IsHidden(finderID), c.hidden)
			}
			if got := len(h.coveredToasts()); (got > 0) != c.hidden {
				t.Errorf("toasts %q", h.coveredToasts())
			}
			if c.hidden {
				return
			}
			// Going to the Finder and back to Bench is the user's doing.
			mustOK(t, h.p.ActivateApp(finderID))
			h.drain()
			h.comeForward()
			if !h.p.IsHidden(finderID) {
				t.Error("coming forward from the Finder later should hide it, as it covers Bench")
			}
		})
	}
}

// TestDropOnPaneWhoseWindowKeepsMoving checks a drop from the Finder on a
// pane whose window its app or a window tool kept moving out of it, whose
// body says so, with Dock again (decision 69): as on its header, a folder
// adds a new terminal, which the pane shows. The window keeps its mark and
// stays where its app put it: only Dock again, Rearrange, or dragging the
// window onto a pane docks it again.
func TestDropOnPaneWhoseWindowKeepsMoving(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	where := keepMoving(t, h, zed)
	site := filepath.Join(t.TempDir(), "bench-site")
	if err := os.Mkdir(site, 0o755); err != nil {
		t.Fatal(err)
	}

	mustOK(t, h.e.DropFiles(DropTarget{Kind: DropPane, Pane: "p11"}, []string{site}))
	h.settle()
	shown := h.item(h.paneShows("p11"))
	if shown.Kind != KindBuiltin || shown.Folder != site || h.view().Display.Focus != "p11" {
		t.Fatalf("p11 shows %+v, focus %s; want a new terminal in %s, focused", shown, h.view().Display.Focus, site)
	}
	if it := h.item("w3"); it.Status != StatusMoving || it.Pane != "p11" {
		t.Errorf("w3 is %s in %s, want still marked, in p11", it.Status, it.Pane)
	}
	if h.frame(zed) != where {
		t.Errorf("Zed's window moved to %v, want left where Zed put it, %v", h.frame(zed), where)
	}
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.settle()
	if it := h.item("w3"); it.Status != StatusMoving || h.frame(zed) != where {
		t.Errorf("shown again, w3 is %s at %v, want still marked, where Zed put it", it.Status, h.frame(zed))
	}
}
