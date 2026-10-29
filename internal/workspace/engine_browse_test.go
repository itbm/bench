package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// writeBundle writes an app bundle at path, with an Info.plist that has id
// as its bundle ID, if it isn't empty, and name as its name.
func writeBundle(t *testing.T, path, id, name string) string {
	t.Helper()
	keys := "<key>CFBundleName</key><string>" + name + "</string>"
	if id != "" {
		keys += "<key>CFBundleIdentifier</key><string>" + id + "</string>"
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>` + keys + `</dict></plist>
`
	contents := filepath.Join(path, "Contents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeFile writes a file with contents and mode, and the folders it's in.
func writeFile(t *testing.T, path, contents string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// browseStarts returns the folders that the open panel started in.
func (h *harness) browseStarts() []string {
	var starts []string
	for _, c := range h.calls("ChooseToAdd") {
		starts = append(starts, c.Arg)
	}
	return starts
}

// TestBrowseAddsAnApp checks Browse… for an app outside the folders Bench
// scans: Bench is lowered before the panel shows, so that the panel stays
// in front; the app's window goes into the pane; and the add dialog lists
// the app from then on, with its name.
func TestBrowseAddsAnApp(t *testing.T) {
	h := newHarness(t, setup{})
	path := writeBundle(t, filepath.Join(t.TempDir(), "Tools", "Tool.app"), "com.example.tool", "Tool")
	// It's running, but no scan finds it.
	h.p.AddApp("com.example.tool", "Tool", platform.AppNotInstalled())
	h.p.AddWindow("com.example.tool", "Tool window")
	h.drain()
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SetOverlay(true))
	h.p.ResetCalls()
	h.p.SetChosen(path)

	got, err := h.e.BrowseToAdd("p12")
	mustOK(t, err)
	if got != (BrowseResult{Added: "Tool"}) {
		t.Errorf("got %+v, want Tool added", got)
	}
	calls := h.calls("SetRaised", "ChooseToAdd")
	if len(calls) < 2 || calls[0].Method != "SetRaised" || calls[0].Arg != "false" ||
		calls[1].Method != "ChooseToAdd" || calls[1].Arg != "/Applications" {
		t.Errorf("Bench should be lowered before the panel shows, in /Applications: %v", calls)
	}
	h.settle()
	w := h.windowID("com.example.tool", "Tool window")
	if id := h.itemOf(w); id == "" || h.paneOf(id) != "p12" || h.paneShows("p12") != id {
		t.Errorf("the app's window should be shown in p12: item %q", id)
	}

	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	found := false
	for _, o := range opts {
		if o.Group == groupApps && o.Label == "Tool" {
			found = o.Source == (Source{Type: "app", BundleID: "com.example.tool"})
		}
	}
	if !found {
		t.Errorf("the add dialog should list Tool under Apps: %+v", opts)
	}

	// The panel starts where the app was next time.
	h.p.SetChosen("")
	mustOK(t, errOf(h.e.BrowseToAdd("p12")))
	if starts := h.browseStarts(); len(starts) != 2 || starts[1] != filepath.Dir(path) {
		t.Errorf("starts: %v, want %s second", starts, filepath.Dir(path))
	}
}

// TestBrowseForAnAppThatOpens checks that an app that isn't running is
// opened, as the dialog's Apps rows open one, and that a bundle without a
// bundle ID is refused.
func TestBrowseForAnAppThatOpens(t *testing.T) {
	h := newHarness(t, setup{})
	dir := t.TempDir()
	h.p.InstallApp("com.example.notes", "Notes Plus", platform.AppLaunchWindows("Today"))
	mustOK(t, h.e.SwitchTab(1))
	h.p.SetChosen(writeBundle(t, filepath.Join(dir, "Notes Plus.app"), "com.example.notes", "Notes Plus"))
	got, err := h.e.BrowseToAdd("p12")
	mustOK(t, err)
	if got.Added != "Notes Plus" || !h.called("OpenApp", func(c platform.Call) bool { return c.BundleID == "com.example.notes" }) {
		t.Errorf("got %+v; the app should open: %v", got, h.calls("OpenApp"))
	}
	h.settle()
	if id := h.itemOf(h.windowID("com.example.notes", "Today")); id == "" || h.paneOf(id) != "p12" {
		t.Errorf("the app's window should go in p12: item %q", id)
	}

	h.p.SetChosen(writeBundle(t, filepath.Join(dir, "Broken.app"), "", "Broken"))
	mustErr(t, errOf(h.e.BrowseToAdd("p12")), "Broken isn't an app that Bench can open.")
}

// TestBrowseAddsNothingOnAnotherTab checks that an app isn't added when the
// tab changed while the user chose it.
func TestBrowseAddsNothingOnAnotherTab(t *testing.T) {
	h := newHarness(t, setup{})
	path := writeBundle(t, filepath.Join(t.TempDir(), "Calendar.app"), calendarID, "Calendar")
	mustOK(t, h.e.SwitchTab(1))
	before := len(h.view().Items)
	h.p.ResetCalls()
	mustErr(t, errOf(h.e.addBrowsedApp("p12", 0, path)),
		"Bench didn't add Calendar, as the tab changed while you chose it. Add it again from this tab.")
	if len(h.view().Items) != before || len(h.calls("OpenApp")) != 0 {
		t.Errorf("nothing should be added or opened: %v", h.calls("OpenApp"))
	}
}

// TestBrowseCancelledOrRefused checks what the panel's answers say: a
// cancel returns nothing, a second panel brings the first forward, and the
// browser preview has no panel. An unknown pane shows no panel.
func TestBrowseCancelledOrRefused(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	got, err := h.e.BrowseToAdd("p12")
	if err != nil || got != (BrowseResult{}) {
		t.Errorf("cancelled: %+v, %v", got, err)
	}
	for _, c := range []struct {
		err  error
		want string
	}{
		{platform.ErrBusy, "The panel for choosing an app or program is open already. Choose one there, or click Cancel."},
		{platform.ErrUnsupported, "The browser preview can't look for apps or programs on this computer. In the Mac app, Browse… finds one on your Mac."},
		{errors.New("no window"), "Bench couldn't show the panel for choosing an app or program."},
	} {
		h.p.SetErr("ChooseToAdd", c.err)
		mustErr(t, errOf(h.e.BrowseToAdd("p12")), c.want)
	}
	h.p.SetErr("ChooseToAdd", nil)

	h.p.ResetCalls()
	mustErr(t, errOf(h.e.BrowseToAdd("p99")), "There's no such pane.")
	if len(h.calls("ChooseToAdd")) != 0 {
		t.Errorf("no panel should show for an unknown pane")
	}

	h.p.SetChosen(filepath.Join(t.TempDir(), "gone"))
	mustErr(t, errOf(h.e.BrowseToAdd("p12")), "Bench couldn't open gone. Check that it's still there.")
}

// TestBrowseForAProgram checks what Browse… fills in the New terminal…
// form with: the file's name, and as the command, a program's name when the user's shell finds it by
// that name, or else its path, quoted for the shell, starting in its
// folder; and for a script that can't run itself, the program that runs
// it, from its #! line or its extension.
func TestBrowseForAProgram(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	tool := writeFile(t, filepath.Join(home, "bin", "tool"), "#!/bin/sh\n", 0o755)
	other := writeFile(t, filepath.Join(home, "other", "tool"), "#!/bin/sh\n", 0o755)
	spaced := writeFile(t, filepath.Join(home, "My Tools", "it's here"), "\x7fELF", 0o755)
	outside := writeFile(t, filepath.Join(t.TempDir(), "run"), "#!/bin/sh\n", 0o755)
	h.terms.mu.Lock()
	h.terms.paths = map[string]string{"tool": tool}
	h.terms.mu.Unlock()

	for _, c := range []struct {
		name, path string
		want       BrowseResult
	}{
		{"on the shell's path", tool, BrowseResult{Command: "tool", Name: "tool"}},
		{"another of the same name", other, BrowseResult{Command: "~/other/tool", Name: "tool", Folder: "~/other"}},
		{"with spaces and quotes", spaced, BrowseResult{Command: `~/'My Tools/it'\''s here'`, Name: "it's here", Folder: "~/My Tools"}},
		{"outside home", outside, BrowseResult{Command: outside, Name: "run", Folder: filepath.Dir(outside)}},
		{"env script", writeFile(t, filepath.Join(home, "s", "build"), "#!/usr/bin/env bash\necho hi\n", 0o644),
			BrowseResult{Command: "bash ~/s/build", Name: "build", Folder: "~/s"}},
		{"env -S script", writeFile(t, filepath.Join(home, "s", "serve.ts"), "#!/usr/bin/env -S deno run\n", 0o644),
			BrowseResult{Command: "deno ~/s/serve.ts", Name: "serve.ts", Folder: "~/s"}},
		{"script by path", writeFile(t, filepath.Join(home, "s", "z"), "#! /bin/zsh -f\n", 0o644),
			BrowseResult{Command: "/bin/zsh ~/s/z", Name: "z", Folder: "~/s"}},
		{"Python by extension", writeFile(t, filepath.Join(home, "s", "Do It.py"), "print(1)\n", 0o644),
			BrowseResult{Command: "python3 ~/'s/Do It.py'", Name: "Do It.py", Folder: "~/s"}},
		{"shell script by extension", writeFile(t, filepath.Join(home, "s", "x.SH"), "echo\n", 0o644),
			BrowseResult{Command: "sh ~/s/x.SH", Name: "x.SH", Folder: "~/s"}},
	} {
		h.p.SetChosen(c.path)
		got, err := h.e.BrowseToAdd("p12")
		if err != nil || got != c.want {
			t.Errorf("%s: got %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	h.terms.mu.Lock()
	looked := slices.Clone(h.terms.looked)
	h.terms.mu.Unlock()
	if !slices.Equal(looked, []string{"tool", "tool", "it's here", "run"}) {
		t.Errorf("looked for %v, want only the programs that can run themselves", looked)
	}
	if starts := h.browseStarts(); starts[len(starts)-1] != filepath.Join(home, "s") {
		t.Errorf("the panel should start where the last file was: %v", starts)
	}

	h.p.SetChosen(writeFile(t, filepath.Join(home, "notes.txt"), "hello\n", 0o644))
	mustErr(t, errOf(h.e.BrowseToAdd("p12")), "notes.txt isn't an app, or a program or script that a terminal can run.")
	h.p.SetChosen(writeFile(t, filepath.Join(home, "empty.sh"), "#!\n", 0o644))
	mustErr(t, errOf(h.e.BrowseToAdd("p12")), "empty.sh isn't an app, or a program or script that a terminal can run.")
	h.p.SetChosen(filepath.Join(home, "My Tools"))
	mustErr(t, errOf(h.e.BrowseToAdd("p12")), "My Tools isn't an app, or a program or script that a terminal can run.")
}

// TestShellWord checks how Browse… writes a path for the shell.
func TestShellWord(t *testing.T) {
	for in, want := range map[string]string{
		"tool":             "tool",
		"/usr/local/bin/x": "/usr/local/bin/x",
		"~/bin/tool":       "~/bin/tool",
		"~/My Apps/x":      "~/'My Apps/x'",
		"~":                "'~'",
		"a$b":              "'a$b'",
		"it's":             `'it'\''s'`,
		"":                 "''",
	} {
		if got := shellWord(in); got != want {
			t.Errorf("shellWord(%q) = %q, want %q", in, got, want)
		}
	}
}
