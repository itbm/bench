package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// editConfig changes config.toml as the user would in a text editor while
// Bench runs, replacing old with new, which must be there.
func editConfig(t *testing.T, h *harness, old, new string) {
	t.Helper()
	path := filepath.Join(h.dir, config.ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("config.toml has no %q:\n%s", old, data)
	}
	writeConfig(t, h, strings.Replace(string(data), old, new, 1))
}

// writeConfig writes config.toml as the user would while Bench runs.
func writeConfig(t *testing.T, h *harness, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, config.ConfigFile), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile returns a file in Bench's support folder, or "" if it isn't
// there.
func readFile(t *testing.T, h *harness, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// mailRule is a rule the user adds to config.toml while Bench runs.
const mailRule = "\n[[rule]]\napp = \"com.apple.mail\"\nfloat = true\n"

// TestSaveKeepsChangesMadeOutsideBench checks that a save doesn't write over
// config.toml once it has changed outside Bench: the change is applied but
// not saved, and a notice offers to reload the file or overwrite it.
func TestSaveKeepsChangesMadeOutsideBench(t *testing.T) {
	h := newHarness(t, setup{})
	edited := string(config.ExampleTOML()) + mailRule
	writeConfig(t, h, edited)

	s := h.view().Settings
	s.Gap = 12
	mustOK(t, h.e.SaveSettings(s))
	if got, want := h.em.lastToast(), "Applied for now, not saved: config.toml was changed outside Bench"; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if got := h.view().Settings.Gap; got != 12 {
		t.Errorf("gap %v, want the change applied", got)
	}
	if got := readFile(t, h, config.ConfigFile); got != edited {
		t.Errorf("config.toml was written over:\n%s", got)
	}
	if got := readFile(t, h, config.BackupFile); got != "" {
		t.Errorf("config.toml.bak was written:\n%s", got)
	}
	n, ok := noticeWith(h.view().Notices, "config.toml was changed outside Bench")
	if !ok || !slices.Equal(n.Actions, []NoticeAction{actionReload, actionOverwrite}) {
		t.Fatalf("notices %+v, want one with Reload and Overwrite", h.view().Notices)
	}
	if strings.Contains(n.Text, outsideKeptHeld) {
		t.Errorf("settings don't hold state.json: %q", n.Text)
	}

	// Tab properties, and a deleted tab, aren't saved either.
	draft := TabDraft{Name: "Mail", Preset: "single"}
	mustOK(t, h.e.SaveTab(-1, draft))
	if got := h.em.lastToast(); got != appliedText(notSavedChanged) {
		t.Errorf("toast after Tab properties %q", got)
	}
	mustOK(t, h.e.DeleteTab(len(h.view().Tabs)-1))
	if got := h.em.lastToast(); !strings.HasSuffix(got, " Not saved: config.toml was changed outside Bench.") {
		t.Errorf("toast after Delete tab %q", got)
	}
	if got := readFile(t, h, config.ConfigFile); got != edited {
		t.Errorf("config.toml was written over:\n%s", got)
	}
	n, _ = noticeWith(h.view().Notices, "config.toml was changed outside Bench")
	if !strings.HasSuffix(n.Text, outsideKeptHeld) {
		t.Errorf("notice %q, want it to say the arrangement isn't saved", n.Text)
	}
}

// TestOverwriteConfig checks that Overwrite writes Bench's config over a
// config.toml changed outside Bench, keeping that version as
// config.toml.bak, and saves the arrangement that was waiting for it.
func TestOverwriteConfig(t *testing.T) {
	h := newHarness(t, setup{})
	edited := string(config.ExampleTOML()) + mailRule
	writeConfig(t, h, edited)
	mustOK(t, h.e.SaveTab(-1, TabDraft{Name: "Mail", Preset: "single"}))
	h.advance(saveDelay * 2)
	if strings.Contains(readFile(t, h, config.StateFile), `"Mail"`) {
		t.Fatal("state.json was saved with a tab config.toml doesn't have")
	}

	mustOK(t, h.e.OverwriteConfig())
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 4 || cfg.Tabs[3].Name != "Mail" || len(cfg.Rules) != 1 {
		t.Errorf("config.toml: tabs %+v rules %+v, want Bench's", cfg.Tabs, cfg.Rules)
	}
	if got := readFile(t, h, config.BackupFile); got != edited {
		t.Errorf("config.toml.bak:\n%s\nwant the version changed outside Bench", got)
	}
	if _, ok := noticeWith(h.view().Notices, "config.toml was changed outside Bench"); ok {
		t.Errorf("notices %v", texts(h.view().Notices))
	}
	st, err := stateOnDisk(t, h)
	if err != nil || st == nil {
		t.Errorf("state.json = %v, %v; want it saved, fitting config.toml", st, err)
	}
	if got := readFile(t, h, config.GoodFile); got != string(config.Format(cfg)) {
		t.Errorf("config.toml.good:\n%s\nwant what Bench saved", got)
	}

	// Saves work again.
	s := h.view().Settings
	s.Gap = 6
	mustOK(t, h.e.SaveSettings(s))
	if got := h.em.lastToast(); got != "Saved settings" {
		t.Errorf("toast %q", got)
	}
}

// TestReloadConfig checks that reloading config.toml applies what the user
// changed in it while Bench ran, keeping the arrangement: settings, edited,
// added, and removed entries, new and removed tabs, and the tabs' order.
func TestReloadConfig(t *testing.T) {
	h := newHarness(t, setup{})
	// Something arranged in Bench that the file doesn't say.
	mustOK(t, h.e.SetMode(string(layout.ModeWide)))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	brave := h.windowID(braveID, "Wails v3 docs")
	if h.itemOf(code) != "w4" || h.itemOf(brave) == "" {
		t.Fatalf("VS Code's window is %q, Brave's %q", h.itemOf(code), h.itemOf(brave))
	}
	// Finder's window isn't in a tab.
	finder := h.windowID(finderID, "Downloads")
	if h.itemOf(finder) != "" {
		t.Fatalf("Finder's window is in a tab before the reload")
	}
	h.p.ResetCalls()

	text := string(config.ExampleTOML())
	text = strings.Replace(text, "gap = 4 ", "gap = 6 ", 1)
	// Code: VS Code's entry goes, and a terminal comes.
	text = strings.Replace(text, `items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]`, `items = [{ app = "dev.zed.Zed" }]`, 1)
	text = strings.Replace(text, `items = [{ terminal = "~/code" }]`, `items = [{ terminal = "~/code" }, { terminal = "~/notes", name = "Notes" }]`, 1)
	// Comms: Outlook moves.
	text = strings.Replace(text, "https://outlook.office.com/mail/", "https://outlook.office.com/calendar/", 1)
	// Research goes, Files comes, and Code moves first.
	comms, rest, _ := strings.Cut(text, "[[tab]]\nname = \"Code\"")
	codeTab, rest, _ := strings.Cut(rest, "[[tab]]\nname = \"Research\"")
	_, rules, _ := strings.Cut(rest, "[[rule]]")
	settings, commsTab, _ := strings.Cut(comms, "[[tab]]\nname = \"Comms\"")
	text = settings + "[[tab]]\nname = \"Code\"" + codeTab + "[[tab]]\nname = \"comms\"" + commsTab +
		"[[tab]]\nname = \"Files\"\nlayout = \"single\"\n\n  [[tab.pane]]\n  items = [{ app = \"com.apple.finder\" }]\n\n[[rule]]" + rules
	writeConfig(t, h, text)
	if _, problems, err := config.Parse([]byte(text)); err != nil || len(problems) > 0 {
		t.Fatalf("the test's config: %v %v\n%s", err, problems, text)
	}

	mustOK(t, h.e.ReloadConfig())
	h.settle()
	if got := h.em.lastToast(); got != "Reloaded config.toml" {
		t.Errorf("toast %q", got)
	}
	v := h.view()
	var names []string
	for _, tab := range v.Tabs {
		names = append(names, tab.Name)
	}
	if !slices.Equal(names, []string{"Code", "comms", "Files"}) {
		t.Fatalf("tabs %v, want the file's", names)
	}
	if v.Current != 1 {
		t.Errorf("current tab %d, want comms still", v.Current)
	}
	if v.Settings.Gap != 6 || v.Mode != string(layout.ModeWide) {
		t.Errorf("gap %v mode %s, want the file's gap and Bench's mode", v.Settings.Gap, v.Mode)
	}
	if h.itemOf(zed) != "w3" {
		t.Errorf("Zed's window is %q, want it kept in its item", h.itemOf(zed))
	}
	// VS Code's and Brave's windows are no longer in a tab, and are back on
	// screen.
	for _, w := range []platform.WindowID{code, brave} {
		id := h.itemOf(w)
		if id == "" || !slices.Contains(floatingIDs(v), id) {
			t.Errorf("window %d is item %q, want it floating (%v)", w, id, floatingIDs(v))
		}
		if h.e.win.IsParked(w) || !OnScreenForTest(h.frame(w)) {
			t.Errorf("window %d is still out of sight", w)
		}
	}
	// The new terminal is in Code's second pane.
	codePanes := layout.Leaves(v.Tabs[0].Root)
	if len(codePanes) != 2 || len(codePanes[1].Items) != 2 || v.Items[codePanes[1].Items[1]].Title != "Notes" {
		t.Errorf("Code's panes %+v", codePanes)
	}
	// Outlook loads its new address.
	if !h.called("CloseWebView", func(c platform.Call) bool { return c.Arg == "w2" }) {
		t.Errorf("Outlook's view wasn't closed: %v", h.calls("CloseWebView"))
	}
	if !h.called("ShowWebView", func(c platform.Call) bool { return strings.Contains(c.Arg, "calendar") || c.Arg == "w2" }) {
		t.Errorf("Outlook's view wasn't shown again: %v", h.calls("ShowWebView"))
	}
	if v.Items["w2"].URL != "https://outlook.office.com/calendar/" {
		t.Errorf("Outlook: %+v", v.Items["w2"])
	}
	// Finder's window went where its new entry claims it.
	if id := h.itemOf(finder); id == "" || !slices.Contains(layout.Items(v.Tabs[2].Root), id) {
		t.Errorf("Finder's window is %q, want it in the Files tab", id)
	}
	if h.em.shortcutsChanged() != 0 {
		t.Errorf("the shortcuts didn't change")
	}
	// state.json follows the file.
	h.advance(saveDelay * 2)
	st, err := stateOnDisk(t, h)
	if err != nil || st == nil {
		t.Fatalf("state.json = %v, %v; want it saved, fitting config.toml", st, err)
	}
	if got := readFile(t, h, config.GoodFile); got != text {
		t.Errorf("config.toml.good isn't the reloaded file")
	}
}

// TestReloadConfigTakesBackUnsavedChanges checks that Reload uses the file
// over Bench's changes that a save couldn't write, and saves the
// arrangement again once the tabs match the file.
func TestReloadConfigTakesBackUnsavedChanges(t *testing.T) {
	h := newHarness(t, setup{})
	edited := strings.Replace(string(config.ExampleTOML()), "gap = 4 ", "gap = 2 ", 1)
	writeConfig(t, h, edited)
	mustOK(t, h.e.SaveTab(-1, TabDraft{Name: "Mail", Preset: "single"}))
	if len(h.view().Tabs) != 4 {
		t.Fatalf("tabs %d", len(h.view().Tabs))
	}

	mustOK(t, h.e.ReloadConfig())
	h.settle()
	v := h.view()
	if len(v.Tabs) != 3 || v.Settings.Gap != 2 {
		t.Errorf("tabs %d gap %v, want the file's", len(v.Tabs), v.Settings.Gap)
	}
	if v.Current < 0 || v.Current >= len(v.Tabs) {
		t.Errorf("current %d", v.Current)
	}
	if _, ok := noticeWith(v.Notices, "config.toml was changed outside Bench"); ok {
		t.Errorf("notices %v", texts(v.Notices))
	}
	h.advance(saveDelay * 2)
	st, err := stateOnDisk(t, h)
	if err != nil || st == nil {
		t.Errorf("state.json = %v, %v; want it saved, fitting config.toml", st, err)
	}
	if got := readFile(t, h, config.ConfigFile); got != edited {
		t.Errorf("config.toml was written:\n%s", got)
	}
}

// TestReloadConfigWithNewShortcuts checks that the shell hears when a
// reload changes the shortcuts, to register them again.
func TestReloadConfigWithNewShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	editConfig(t, h, `jump = "ctrl+alt+space"`, `jump = "ctrl+alt+j"`)
	mustOK(t, h.e.ReloadConfig())
	h.drain()
	if got := h.em.shortcutsChanged(); got != 1 {
		t.Errorf("ShortcutsChanged %d times, want once", got)
	}
	if got := h.e.ConfigCopy().Shortcuts.Jump; got != "ctrl+alt+j" {
		t.Errorf("jump %q", got)
	}
	if got := h.view().Keys.Jump; got == "" || !strings.Contains(got, "J") {
		t.Errorf("the view's jump key %q", got)
	}
}

// TestReloadConfigWithProblems checks that a reload of a file with problems
// keeps the config Bench has, stops saving, and says so; fixing the file and
// reloading again puts things right.
func TestReloadConfigWithProblems(t *testing.T) {
	h := newHarness(t, setup{})
	before := h.view()
	editConfig(t, h, "gap = 4 ", "gapp = 4 ")
	broken := readFile(t, h, config.ConfigFile)

	mustOK(t, h.e.ReloadConfig())
	v := h.view()
	if v.ConfigWritable {
		t.Errorf("ConfigWritable should be false")
	}
	if len(v.Tabs) != len(before.Tabs) || !reflect.DeepEqual(v.Settings, before.Settings) {
		t.Errorf("tabs %d settings %+v, want Bench's kept", len(v.Tabs), v.Settings)
	}
	if len(v.Notices) < 2 || v.Notices[0].Text != problemsLead(usingOwn, false, true) || !strings.Contains(v.Notices[1].Text, "gapp") {
		t.Fatalf("notices %v", texts(v.Notices))
	}
	if got, want := h.em.lastToast(), "config.toml has problems, so Bench kept the config it had. The status bar lists them."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}

	// Changes are applied, not saved.
	s := v.Settings
	s.Gap = 12
	mustOK(t, h.e.SaveSettings(s))
	if got := h.em.lastToast(); got != "Applied for now, not saved: config.toml has problems" {
		t.Errorf("toast %q", got)
	}
	if got := readFile(t, h, config.ConfigFile); got != broken {
		t.Errorf("config.toml was written")
	}
	// Activation doesn't check a file with problems.
	h.p.ActivateApp(finderID)
	h.p.ActivateApp(benchID)
	h.drain()
	if _, ok := noticeWith(h.view().Notices, outsideSeenText); ok {
		t.Errorf("notices %v", texts(h.view().Notices))
	}

	writeConfig(t, h, strings.Replace(broken, "gapp = 4 ", "gap = 5 ", 1))
	mustOK(t, h.e.ReloadConfig())
	v = h.view()
	if !v.ConfigWritable || len(v.Notices) != 0 || v.Settings.Gap != 5 {
		t.Errorf("writable %v notices %v gap %v", v.ConfigWritable, texts(v.Notices), v.Settings.Gap)
	}
}

// TestReloadConfigWritesAMissingFile checks that reloading a config.toml
// that has gone writes Bench's config there.
// TestReloadConfigWithProblemsAndNoStatusBar checks that the message after
// reloading a file with problems points to where its notices are: the title
// bar, while the status bar is hidden.
func TestReloadConfigWithProblemsAndNoStatusBar(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.ShowStatusBar = false
	mustOK(t, h.e.SaveSettings(s))
	editConfig(t, h, "gap = 4 ", "gapp = 4 ")
	mustOK(t, h.e.ReloadConfig())
	if got, want := h.em.lastToast(), "config.toml has problems, so Bench kept the config it had. The notices in the title bar list them."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if v := h.view(); v.Settings.ShowStatusBar || len(v.Notices) < 2 {
		t.Errorf("status bar shown %v, notices %v; want hidden, and the problems", v.Settings.ShowStatusBar, texts(v.Notices))
	}
}

func TestReloadConfigWritesAMissingFile(t *testing.T) {
	h := newHarness(t, setup{})
	if err := os.Remove(filepath.Join(h.dir, config.ConfigFile)); err != nil {
		t.Fatal(err)
	}
	mustOK(t, h.e.ReloadConfig())
	if cfg := savedConfig(t, h); len(cfg.Tabs) != 3 {
		t.Errorf("config.toml has %d tabs, want Bench's 3", len(cfg.Tabs))
	}
	if !strings.HasPrefix(h.em.lastToast(), "config.toml wasn't there") {
		t.Errorf("toast %q", h.em.lastToast())
	}
}

// TestStartupWithProblemsUsesTheLastGoodConfig checks that a config.toml
// with problems runs with the last config that loaded without them, and the
// arrangement state.json has for it, which Bench goes on saving until a
// change to the tabs, while it leaves config.toml and the last good config
// alone.
func TestStartupWithProblemsUsesTheLastGoodConfig(t *testing.T) {
	var good, state []byte
	{
		h := newHarness(t, setup{})
		mustOK(t, h.e.SetMode(string(layout.ModeCompact)))
		mustOK(t, h.e.Close())
		good = []byte(readFile(t, h, config.GoodFile))
		state = []byte(readFile(t, h, config.StateFile))
		if string(good) != string(config.ExampleTOML()) || len(state) == 0 {
			t.Fatalf("config.toml.good:\n%s\nstate.json:\n%s", good, state)
		}
	}
	bad := strings.Replace(string(config.ExampleTOML()), "gap = 4 ", "gapp = 4 ", 1)
	h := newHarness(t, setup{config: bad, state: func(t *testing.T, dir string) {
		for name, data := range map[string][]byte{config.GoodFile: good, config.StateFile: state} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}})
	v := h.view()
	if v.ConfigWritable || len(v.Tabs) != 3 || v.Tabs[0].Name != "Comms" {
		t.Errorf("writable %v tabs %+v, want the last good config's", v.ConfigWritable, v.Tabs)
	}
	if v.Mode != string(layout.ModeCompact) {
		t.Errorf("mode %s, want state.json's", v.Mode)
	}
	if len(v.Notices) == 0 || v.Notices[0].Text != problemsLead(usingGood, false, true) {
		t.Errorf("notices %v", texts(v.Notices))
	}
	// The arrangement is for the last good config, so Bench goes on saving
	// it.
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	h.advance(2 * saveDelay)
	var st config.State
	mustOK(t, json.Unmarshal([]byte(readFile(t, h, config.StateFile)), &st))
	if len(layout.Leaves(st.Tabs[1].Root)) != 3 || st.Mode != string(layout.ModeCompact) {
		t.Errorf("state.json wasn't saved with the change: %v", treePanes(st.Tabs[1].Root))
	}
	if got := readFile(t, h, config.ConfigFile); got != bad {
		t.Errorf("config.toml was written")
	}
	if got := readFile(t, h, config.GoodFile); got != string(good) {
		t.Errorf("config.toml.good was changed")
	}

	// A change to the tabs, which config.toml can't be saved with, stops
	// it, and the notice says so.
	saved := readFile(t, h, config.StateFile)
	mustOK(t, h.e.SaveTab(-1, TabDraft{Name: "Mail", Preset: "single"}))
	if v := h.view(); v.Notices[0].Text != problemsLead(usingGood, false, false) {
		t.Errorf("notices %v", texts(v.Notices))
	}
	mustOK(t, h.e.SetMode(string(layout.ModeWide)))
	mustOK(t, h.e.Close())
	if got := readFile(t, h, config.StateFile); got != saved {
		t.Errorf("state.json was saved with a tab that the last good config doesn't have")
	}
}

// TestStartupWithProblemsDoesntSaveTheDefaultConfigsArrangement checks that
// Bench doesn't save state.json while it runs with the default config,
// whose tabs aren't the user's, even if state.json has the same tabs.
func TestStartupWithProblemsDoesntSaveTheDefaultConfigsArrangement(t *testing.T) {
	bad := "[[tab]]\nname = \"Research\"\nlayout = \"nope\"\n"
	// state.json has the default config's one tab, Research.
	state := savedState(t, func(h *harness) {
		for len(h.view().Tabs) > 1 {
			mustOK(t, h.e.DeleteTab(0))
		}
	})
	h := newHarness(t, setup{config: bad, state: writeState(state)})
	if v := h.view(); v.Notices[0].Text != problemsLead(usingDefault, false, false) {
		t.Errorf("notices %v", texts(v.Notices))
	}
	mustOK(t, h.e.SetMode(string(layout.ModeWide)))
	mustOK(t, h.e.Close())
	if got := readFile(t, h, config.StateFile); got != string(state) {
		t.Errorf("state.json was saved for the default config")
	}
}

// TestStartupWithProblemsLeavesAnotherConfigsState checks that a
// state.json that doesn't fit the config Bench runs with, while config.toml
// has problems, stays for when they're fixed.
func TestStartupWithProblemsLeavesAnotherConfigsState(t *testing.T) {
	state := savedState(t, nil)
	h := newHarness(t, setup{config: "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n", state: writeState(state)})
	if got := readFile(t, h, config.StateFile); got != string(state) {
		t.Errorf("state.json was changed")
	}
	if got := readFile(t, h, config.BadState); got != "" {
		t.Errorf("state.json was set aside")
	}
}

// TestUseConfigBackup checks that Use backup swaps config.toml, which has
// problems, with config.toml.bak, and runs with it.
func TestUseConfigBackup(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.Gap = 12
	mustOK(t, h.e.SaveSettings(s))
	backup := readFile(t, h, config.BackupFile)
	broken := strings.Replace(readFile(t, h, config.ConfigFile), "gap = 12", "gapp = 12", 1)
	writeConfig(t, h, broken)
	mustErr(t, h.e.UseConfigBackup(), errNoConfigProblems.Error())

	mustOK(t, h.e.ReloadConfig())
	n := h.view().Notices[0]
	if !slices.Contains(n.Actions, actionUseBackup) {
		t.Fatalf("notice %+v, want Use backup", n)
	}
	mustOK(t, h.e.UseConfigBackup())
	v := h.view()
	if !v.ConfigWritable || len(v.Notices) != 0 || v.Settings.Gap != 4 {
		t.Errorf("writable %v notices %v gap %v; want the backup's", v.ConfigWritable, texts(v.Notices), v.Settings.Gap)
	}
	if readFile(t, h, config.ConfigFile) != backup || readFile(t, h, config.BackupFile) != broken {
		t.Errorf("config.toml and config.toml.bak weren't swapped")
	}
	if !strings.HasPrefix(h.em.lastToast(), "Using config.toml.bak.") {
		t.Errorf("toast %q", h.em.lastToast())
	}
}

// TestUseConfigBackupWithProblems checks that a backup with problems isn't
// used, or offered.
func TestUseConfigBackupWithProblems(t *testing.T) {
	bad := "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n"
	h := newHarness(t, setup{config: bad, state: func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, config.BackupFile), []byte("gapp = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}})
	if n := h.view().Notices[0]; slices.Contains(n.Actions, actionUseBackup) {
		t.Errorf("notice %+v offers a backup with problems", n)
	}
	if err := h.e.UseConfigBackup(); err == nil || !strings.HasPrefix(err.Error(), "Bench couldn't use the backup: config.toml.bak has problems too") {
		t.Errorf("UseConfigBackup = %v", err)
	}
	if got := readFile(t, h, config.ConfigFile); got != bad {
		t.Errorf("config.toml was changed")
	}
}

// TestOpenConfig checks that Open config.toml and Show in Finder hand the
// file to the platform, and that a platform that can't open files, as in the
// browser preview, says so.
func TestOpenConfig(t *testing.T) {
	h := newHarness(t, setup{})
	path := filepath.Join(h.dir, config.ConfigFile)
	mustOK(t, h.e.OpenConfig())
	mustOK(t, h.e.ShowConfigInFinder())
	for _, method := range []string{"OpenTextFile", "ShowInFinder"} {
		if !h.called(method, func(c platform.Call) bool { return c.Arg == path }) {
			t.Errorf("%s: %v", method, h.calls(method))
		}
	}
	h.p.SetErr("OpenTextFile", platform.ErrUnsupported)
	h.p.SetErr("ShowInFinder", platform.ErrUnsupported)
	mustErr(t, h.e.OpenConfig(), "This build of Bench can't open files.")
	mustErr(t, h.e.ShowConfigInFinder(), "This build of Bench can't open files.")
	h.p.SetErr("OpenTextFile", platform.ErrNotInstalled)
	mustErr(t, h.e.OpenConfig(), "Bench couldn't open config.toml.")
}

// TestActivationNoticesChangesMadeOutsideBench checks that Bench, when it
// or an app in a tab becomes active, or a dialog opens, notices that
// config.toml has changed and offers to reload it, and drops the notice if
// the file is put back.
func TestActivationNoticesChangesMadeOutsideBench(t *testing.T) {
	h := newHarness(t, setup{})
	original := readFile(t, h, config.ConfigFile)
	activate := func() {
		h.p.ActivateApp(finderID)
		h.p.ActivateApp(benchID)
		h.drain()
	}
	activate()
	if len(h.view().Notices) != 0 {
		t.Fatalf("notices %v", texts(h.view().Notices))
	}
	writeConfig(t, h, original+mailRule)
	activate()
	n, ok := noticeWith(h.view().Notices, outsideSeenText)
	if !ok || !slices.Equal(n.Actions, []NoticeAction{actionReload}) {
		t.Fatalf("notices %+v", h.view().Notices)
	}
	writeConfig(t, h, original)
	activate()
	if len(h.view().Notices) != 0 {
		t.Errorf("notices %v, want none once the file is back", texts(h.view().Notices))
	}

	writeConfig(t, h, original+mailRule)
	activate()
	mustOK(t, h.e.ReloadConfig())
	if len(h.view().Notices) != 0 {
		t.Errorf("notices %v after the reload", texts(h.view().Notices))
	}

	// An app with no window in a tab doesn't check the file, but one with a
	// window in a tab does, as the user may switch to it after editing the
	// file without coming back to Bench.
	writeConfig(t, h, original)
	h.p.ActivateApp(finderID)
	h.drain()
	if _, ok := noticeWith(h.view().Notices, outsideSeenText); ok {
		t.Errorf("Finder's activation checked the file")
	}
	h.p.ActivateApp(zedID)
	h.drain()
	if _, ok := noticeWith(h.view().Notices, outsideSeenText); !ok {
		t.Errorf("Zed's activation didn't check the file: notices %v", texts(h.view().Notices))
	}

	// Opening a menu or dialog checks it too, as the user may have edited
	// it in one of Bench's terminals.
	writeConfig(t, h, original+mailRule)
	h.p.ActivateApp(finderID)
	h.drain()
	if _, ok := noticeWith(h.view().Notices, outsideSeenText); !ok {
		t.Fatalf("the notice went without a check")
	}
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.SetOverlay(false))
	if _, ok := noticeWith(h.view().Notices, outsideSeenText); ok {
		t.Errorf("opening a dialog didn't check the file: notices %v", texts(h.view().Notices))
	}
}

// TestNotSavedWhileConfigHasProblems checks that Settings, Tab properties,
// and Delete tab say their changes weren't saved while config.toml has
// problems, rather than "Saved".
func TestNotSavedWhileConfigHasProblems(t *testing.T) {
	bad := strings.Replace(string(config.ExampleTOML()), "gap = 4 ", "gapp = 4 ", 1)
	h := newHarness(t, setup{config: bad})
	s := h.view().Settings
	s.Gap = 12
	mustOK(t, h.e.SaveSettings(s))
	if got := h.em.lastToast(); got != "Applied for now, not saved: config.toml has problems" {
		t.Errorf("Settings: %q", got)
	}
	mustOK(t, h.e.SaveTab(-1, TabDraft{Name: "Mail", Preset: "single"}))
	if got := h.em.lastToast(); got != "Applied for now, not saved: config.toml has problems" {
		t.Errorf("Tab properties: %q", got)
	}
	mustOK(t, h.e.DeleteTab(len(h.view().Tabs)-1))
	if got := h.em.lastToast(); !strings.HasSuffix(got, " Not saved: config.toml has problems.") {
		t.Errorf("Delete tab: %q", got)
	}
	if got := readFile(t, h, config.ConfigFile); got != bad {
		t.Errorf("config.toml was written")
	}
	mustErr(t, h.e.OverwriteConfig(), errConfigProblems.Error())
}

// TestReloadConfigGivesNewEntriesTheWindowsThatLeft checks that a reload puts
// the windows that leave their tabs where the file's new entries claim them,
// as startup would, rather than opening their apps again, and that a
// terminal whose tab goes carries on as a new entry's for the same folder:
// for an entry moved to another tab, and for a tab renamed along with
// another change to the tabs, which can't be told from a new tab.
func TestReloadConfigGivesNewEntriesTheWindowsThatLeft(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	obsidian := h.windowID(obsidianID, "Daily note")
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if h.itemOf(obsidian) == "" || !h.terms.Running("w5") {
		t.Fatalf("Obsidian's window is %q, Code's terminal running %v", h.itemOf(obsidian), h.terms.Running("w5"))
	}
	floating := floatingIDs(h.view())
	h.p.ResetCalls()

	writeConfig(t, h, exampleWith(t, func(c *config.Config) {
		// Obsidian's entry moves from Research to Comms.
		research := &c.Tabs[2]
		c.Tabs[0].Panes[1].Items = append(c.Tabs[0].Panes[1].Items, research.Panes[1].Items...)
		research.Panes = research.Panes[:1]
		// Code is renamed Dev, and a tab added.
		c.Tabs[1].Name = "Dev"
		c.Tabs = append(c.Tabs, config.Tab{Name: "Notes", Layout: "single"})
	}))
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Dev", "Research", "Notes"}) {
		t.Fatalf("tabs %v", got)
	}
	if id := h.itemOf(obsidian); id == "" || !slices.Contains(layout.Items(v.Tabs[0].Root), id) {
		t.Errorf("Obsidian's window is %q, want it in Comms (%v)", id, layout.Items(v.Tabs[0].Root))
	}
	dev := layout.Items(v.Tabs[1].Root)
	for _, w := range []platform.WindowID{zed, code} {
		if id := h.itemOf(w); id == "" || !slices.Contains(dev, id) {
			t.Errorf("window %d is %q, want it in Dev (%v)", w, id, dev)
		}
	}
	// Dev's terminal is Code's, still running, not a second one.
	if len(dev) != 3 || !slices.Contains(dev, "w5") || !h.terms.Running("w5") {
		t.Errorf("Dev's items %v, Code's terminal running %v", dev, h.terms.Running("w5"))
	}
	if !slices.Equal(floatingIDs(v), floating) {
		t.Errorf("floating %v, want %v", floatingIDs(v), floating)
	}
	for _, c := range h.calls("OpenApp", "NewWindow") {
		if sameApp(c.BundleID, obsidianID) || sameApp(c.BundleID, zedID) || sameApp(c.BundleID, vscodeID) {
			t.Errorf("Bench asked for a window again: %+v", c)
		}
	}
	for _, id := range dev {
		if it := v.Items[id]; it.Kind == KindApp && it.Status != StatusReady {
			t.Errorf("%s is %s", it.App, it.Status)
		}
	}
}
