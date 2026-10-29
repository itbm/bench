package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// exampleWith returns the example config.toml, changed.
func exampleWith(t *testing.T, change func(c *config.Config)) string {
	t.Helper()
	c := config.Example()
	change(c)
	if problems := c.Validate(); len(problems) > 0 {
		t.Fatalf("the test's config: %v", problems)
	}
	return string(config.Format(c))
}

// treePanes returns the items of each pane of a tree, in reading order.
func treePanes(root *layout.Node) [][]string {
	var out [][]string
	for _, p := range layout.Leaves(root) {
		out = append(out, slices.Clone(p.Items))
	}
	return out
}

// tabNames returns the view's tab names.
func tabNames(v View) []string {
	var out []string
	for _, tab := range v.Tabs {
		out = append(out, tab.Name)
	}
	return out
}

// splitCode returns a state.json in which VS Code has a pane of its own in
// Code, the current tab, and the Code tab it saved.
func splitCode(t *testing.T) ([]byte, config.TabState) {
	t.Helper()
	data := savedState(t, func(h *harness) {
		mustOK(t, h.e.SwitchTab(1))
		mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	})
	var st config.State
	mustOK(t, json.Unmarshal(data, &st))
	if len(layout.Leaves(st.Tabs[1].Root)) != 3 {
		t.Fatalf("Code: %+v", treePanes(st.Tabs[1].Root))
	}
	return data, st.Tabs[1]
}

// TestStartupKeepsArrangementWhenTabsChange checks that tabs reordered,
// added, and renamed in case in config.toml keep their arrangement, with
// nothing set aside.
func TestStartupKeepsArrangementWhenTabsChange(t *testing.T) {
	data, code := splitCode(t)
	text := exampleWith(t, func(c *config.Config) {
		comms, code, research := c.Tabs[0], c.Tabs[1], c.Tabs[2]
		code.Name = "code"
		notes := config.Tab{Name: "Notes", Layout: "single", Panes: []config.Pane{{Items: []config.Item{{Terminal: "~/notes"}}}}}
		c.Tabs = []config.Tab{research, notes, comms, code}
	})
	h := newHarness(t, setup{config: text, state: writeState(data)})
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Research", "Notes", "Comms", "code"}) {
		t.Fatalf("tabs %v, want the file's", got)
	}
	if v.Current != 3 {
		t.Errorf("current tab %d, want code, still", v.Current)
	}
	if got := treePanes(v.Tabs[3].Root); !equalPanes(got, treePanes(code.Root)) || v.Tabs[3].Preset != layout.PresetCustom {
		t.Errorf("code's panes %v (%s), want %v", got, v.Tabs[3].Preset, treePanes(code.Root))
	}
	if got := h.itemOf(h.windowID(vscodeID, "index.html — bench-ui")); got != "w4" {
		t.Errorf("VS Code's window is %q, want w4", got)
	}
	// Notes is new, built from the file, with IDs after the state's.
	notes := layout.Items(v.Tabs[1].Root)
	if len(notes) != 1 || v.Items[notes[0]].Kind != KindBuiltin || idNumber(t, notes[0]) <= 18 {
		t.Errorf("Notes: %v", notes)
	}
	if len(v.Notices) != 0 {
		t.Errorf("notices %v", texts(v.Notices))
	}
	if got := readFile(t, h, config.BadState); got != "" {
		t.Errorf("state.json was set aside")
	}
	h.advance(2 * saveDelay)
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("state.json doesn't follow config.toml: %v", err)
	}
}

// idNumber returns the number at the end of an ID.
func idNumber(t *testing.T, id string) int {
	t.Helper()
	n := 0
	for _, r := range strings.TrimLeft(id, "wps") {
		n = n*10 + int(r-'0')
	}
	return n
}

// TestStartupKeepsARenamedTab checks that a tab renamed in config.toml,
// with nothing else about the tabs changed, keeps its arrangement.
func TestStartupKeepsARenamedTab(t *testing.T) {
	data, code := splitCode(t)
	text := exampleWith(t, func(c *config.Config) { c.Tabs[1].Name = "Dev" })
	h := newHarness(t, setup{config: text, state: writeState(data)})
	v := h.view()
	if v.Tabs[1].Name != "Dev" || !equalPanes(treePanes(v.Tabs[1].Root), treePanes(code.Root)) {
		t.Errorf("tab %q, panes %v; want Code's arrangement", v.Tabs[1].Name, treePanes(v.Tabs[1].Root))
	}
	if len(v.Notices) != 0 || readFile(t, h, config.BadState) != "" {
		t.Errorf("notices %v, or state.json set aside", texts(v.Notices))
	}
}

// TestStartupSaysWhichArrangementItCouldntKeep checks that a tab whose name
// config.toml no longer has loses its arrangement, with a notice that names
// it, and a copy of state.json set aside; and that once a tab has its name
// again, Restore previous arrangement puts it back, windows and all.
func TestStartupSaysWhichArrangementItCouldntKeep(t *testing.T) {
	data, code := splitCode(t)
	// Code is renamed Dev, and a tab added, so Dev can't be told from new.
	text := exampleWith(t, func(c *config.Config) {
		c.Tabs[1].Name = "Dev"
		c.Tabs = append(c.Tabs, config.Tab{Name: "Notes", Layout: "single"})
	})
	h := newHarness(t, setup{config: text, state: writeState(data)})
	v := h.view()
	if len(layout.Leaves(v.Tabs[1].Root)) != 2 || v.Tabs[1].Preset != "main-side" {
		t.Fatalf("Dev: %v, want the file's layout", treePanes(v.Tabs[1].Root))
	}
	// Dev's VS Code entry claimed VS Code's window.
	vscode := h.windowID(vscodeID, "index.html — bench-ui")
	if id := h.itemOf(vscode); id == "w4" || !slices.Contains(layout.Items(v.Tabs[1].Root), id) {
		t.Errorf("VS Code's window is %q, want it in Dev, by its entry", id)
	}
	devTerm := layout.Leaves(v.Tabs[1].Root)[1].Items[0]
	floating := floatingIDs(v) // Finder's window.
	if v.Current != 1 {
		t.Errorf("current %d, want the tab in Code's place", v.Current)
	}
	n, ok := noticeWith(v.Notices, "When Bench started, config.toml had no tab called Code, so Bench couldn't keep its arrangement.")
	if !ok || !strings.HasSuffix(n.Text, " To get it back, name a tab Code again, then choose Restore previous arrangement here.") ||
		!slices.Equal(n.Actions, []NoticeAction{actionDismissArrangement}) {
		t.Fatalf("notices %+v", v.Notices)
	}
	if readFile(t, h, config.BadState) != string(data) || readFile(t, h, config.StateFile) == "" {
		t.Errorf("state.json.bad should be a copy of state.json, which stays")
	}
	mustErr(t, h.e.RestoreArrangement(), errNoTabToRestore.Error())

	// Dev is called Code again, in config.toml: as the only change to the
	// tabs, it's a rename, which keeps Dev's arrangement.
	editConfig(t, h, `name = "Dev"`, `name = "Code"`)
	mustOK(t, h.e.ReloadConfig())
	v = h.view()
	if v.Tabs[1].Name != "Code" || h.itemOf(vscode) == "w4" {
		t.Fatalf("tab %q, VS Code's window in %q", v.Tabs[1].Name, h.itemOf(vscode))
	}
	n, _ = noticeWith(v.Notices, "When Bench started, config.toml had no tab called Code")
	if !slices.Equal(n.Actions, []NoticeAction{actionRestoreArrangement, actionDismissArrangement}) || strings.Contains(n.Text, "To get it back") {
		t.Fatalf("notice %+v, want it to offer Restore previous arrangement", n)
	}

	if !h.terms.Running(devTerm) {
		t.Fatalf("Dev's terminal isn't running")
	}
	mustOK(t, h.e.RestoreArrangement())
	h.settle()
	v = h.view()
	// Dev's terminal, for the same entry as the arrangement's, takes its
	// place, and goes on running.
	restored := withItem(treePanes(code.Root), "w5", devTerm)
	if got := treePanes(v.Tabs[1].Root); !equalPanes(got, restored) || v.Tabs[1].Preset != layout.PresetCustom {
		t.Errorf("Code's panes %v, want %v", got, restored)
	}
	// The windows go back to the arrangement's items, and Dev's own items
	// go: none of them is left floating.
	if got := h.itemOf(vscode); got != "w4" {
		t.Errorf("VS Code's window is %q, want w4", got)
	}
	if got := h.itemOf(h.windowID(zedID, "bench — switch.go")); got != "w3" {
		t.Errorf("Zed's window is %q, want w3", got)
	}
	if !slices.Equal(floatingIDs(v), floating) || !h.terms.Running(devTerm) {
		t.Errorf("floating %v, Dev's terminal running %v", floatingIDs(v), h.terms.Running(devTerm))
	}
	if _, ok := noticeWith(v.Notices, "When Bench started"); ok {
		t.Errorf("notices %v", texts(v.Notices))
	}
	if got := h.em.lastToast(); got != "Restored the arrangement of Code" {
		t.Errorf("toast %q", got)
	}
	mustErr(t, h.e.RestoreArrangement(), errNothingToRestore.Error())
	h.advance(2 * saveDelay)
	if st, err := stateOnDisk(t, h); err != nil || !equalPanes(treePanes(st.Tabs[1].Root), restored) {
		t.Errorf("state.json: %v", err)
	}
}

// withItem returns panes with one item's ID replaced by another's.
func withItem(panes [][]string, old, new string) [][]string {
	for _, p := range panes {
		if i := slices.Index(p, old); i >= 0 {
			p[i] = new
		}
	}
	return panes
}

// TestRestoreArrangementAfterRenamingInBench checks that renaming a tab in
// Tab properties to the name of a tab whose arrangement Bench couldn't keep
// lets Restore previous arrangement put it back, and that Dismiss takes the
// notice away instead.
func TestRestoreArrangementAfterRenamingInBench(t *testing.T) {
	data, code := splitCode(t)
	text := exampleWith(t, func(c *config.Config) {
		c.Tabs[1].Name = "Dev"
		c.Tabs = append(c.Tabs, config.Tab{Name: "Notes", Layout: "single"})
	})
	h := newHarness(t, setup{config: text, state: writeState(data)})
	devTerm := layout.Leaves(h.view().Tabs[1].Root)[1].Items[0]
	mustOK(t, h.e.SaveTab(1, TabDraft{Name: "code", Preset: "main-side", SmallScreens: "stack"}))
	n, _ := noticeWith(h.view().Notices, "When Bench started, config.toml had no tab called Code")
	if !slices.Contains(n.Actions, actionRestoreArrangement) {
		t.Fatalf("notice %+v, want it to offer Restore previous arrangement", n)
	}
	mustOK(t, h.e.RestoreArrangement())
	v := h.view()
	if v.Tabs[1].Name != "code" || !equalPanes(treePanes(v.Tabs[1].Root), withItem(treePanes(code.Root), "w5", devTerm)) {
		t.Errorf("tab %q, panes %v; want Code's arrangement", v.Tabs[1].Name, treePanes(v.Tabs[1].Root))
	}
	h.advance(2 * saveDelay)
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("state.json: %v", err)
	}

	h = newHarness(t, setup{config: text, state: writeState(data)})
	mustOK(t, h.e.DismissArrangementNotice())
	if v := h.view(); len(v.Notices) != 0 {
		t.Errorf("notices %v", texts(v.Notices))
	}
	mustErr(t, h.e.RestoreArrangement(), errNothingToRestore.Error())
}

// TestStartupSetsAsideStateItCantUse checks that a state.json that can't be
// used is set aside whole, with a notice that says why and names its tabs,
// which Dismiss takes away.
func TestStartupSetsAsideStateItCantUse(t *testing.T) {
	data := savedState(t, nil)
	newer := strings.Replace(string(data), `"version": 1`, `"version": 2`, 1)
	h := newHarness(t, setup{state: writeState([]byte(newer))})
	v := h.view()
	want := "Bench couldn't restore the arrangement of Comms, Code, and Research, because state.json can't be used: " +
		"it's version 2, not 1. They start as the config lays them out. The file is kept as state.json.bad, but it can't be restored."
	n, ok := noticeWith(v.Notices, "Bench couldn't restore")
	if !ok || n.Text != want || !slices.Equal(n.Actions, []NoticeAction{actionDismissArrangement}) {
		t.Fatalf("notices %+v", v.Notices)
	}
	if readFile(t, h, config.BadState) != newer {
		t.Errorf("state.json.bad isn't the file")
	}
	mustErr(t, h.e.RestoreArrangement(), errNothingToRestore.Error())
	mustOK(t, h.e.DismissArrangementNotice())
	if v := h.view(); len(v.Notices) != 0 {
		t.Errorf("notices %v", texts(v.Notices))
	}

	// A file that doesn't parse names no tabs.
	h = newHarness(t, setup{state: writeState([]byte("{not json"))})
	if n := h.view().Notices; len(n) != 1 || !strings.HasPrefix(n[0].Text, "Bench couldn't restore the arrangement, because state.json can't be used: it doesn't parse: ") ||
		!strings.Contains(n[0].Text, ". The tabs start as the config lays them out.") {
		t.Errorf("notices %v", texts(n))
	}
}

// TestStartupWithProblemsSaysLaterWhatItCouldntKeep checks that, while
// Bench runs with another config because config.toml has problems, the
// arrangement of tabs that config doesn't have isn't set aside or reported
// until config.toml is fixed, when Bench is about to save over it; and that
// the tabs can then be restored from it.
func TestStartupWithProblemsSaysLaterWhatItCouldntKeep(t *testing.T) {
	data, code := splitCode(t)
	bad := strings.Replace(string(config.ExampleTOML()), "gap = 8 ", "gapp = 8 ", 1)
	h := newHarness(t, setup{config: bad, state: writeState(data)})
	v := h.view()
	// The default config's tab is Research, with the browser, which isn't
	// the user's Research.
	if got := tabNames(v); !slices.Equal(got, []string{"Research"}) || v.Tabs[0].Preset != "single" {
		t.Fatalf("tabs %v, want the default config's", got)
	}
	if _, ok := noticeWith(v.Notices, "Bench started with another config"); ok || readFile(t, h, config.BadState) != "" {
		t.Errorf("notices %v, or state.json set aside, while config.toml has problems", texts(v.Notices))
	}

	writeConfig(t, h, string(config.ExampleTOML()))
	mustOK(t, h.e.ReloadConfig())
	v = h.view()
	// The default config has a Research tab too, but it isn't the user's.
	n, ok := noticeWith(v.Notices, "Bench started with another config while config.toml had problems, so it couldn't keep the arrangement of Comms, Code, and Research.")
	if !ok || !slices.Equal(n.Actions, []NoticeAction{actionRestoreArrangement, actionDismissArrangement}) {
		t.Fatalf("notices %+v", v.Notices)
	}
	if readFile(t, h, config.BadState) != string(data) {
		t.Errorf("state.json.bad isn't the state.json Bench started with")
	}
	floating := floatingIDs(v) // Finder's window, and the default config's browser.
	codeTerm := layout.Leaves(v.Tabs[1].Root)[1].Items[0]
	mustOK(t, h.e.RestoreArrangement())
	h.settle()
	v = h.view()
	if got, want := treePanes(v.Tabs[1].Root), withItem(treePanes(code.Root), "w5", codeTerm); !equalPanes(got, want) {
		t.Errorf("Code's panes %v, want %v", got, want)
	}
	if got := h.itemOf(h.windowID(vscodeID, "index.html — bench-ui")); got != "w4" {
		t.Errorf("VS Code's window is %q, want w4", got)
	}
	if got := h.em.lastToast(); got != "Restored the arrangement of Comms, Code, and Research" {
		t.Errorf("toast %q", got)
	}
	if !slices.Equal(floatingIDs(v), floating) {
		t.Errorf("floating %v, want %v", floatingIDs(v), floating)
	}
}

// TestReloadConfigKeepsARenamedTab checks that a tab renamed in config.toml
// while Bench runs, with nothing else about the tabs changed, keeps its
// arrangement when the file is reloaded.
func TestReloadConfigKeepsARenamedTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	before := treePanes(h.view().Tabs[1].Root)
	editConfig(t, h, `name = "Code"`, `name = "Dev"`)
	mustOK(t, h.e.ReloadConfig())
	v := h.view()
	if v.Tabs[1].Name != "Dev" || !equalPanes(treePanes(v.Tabs[1].Root), before) {
		t.Errorf("tab %q, panes %v; want Code's arrangement %v", v.Tabs[1].Name, treePanes(v.Tabs[1].Root), before)
	}
}

// TestReloadConfigSaysWhichArrangementItCouldntKeep checks that a reload of
// config.toml that deletes a tab, here one renamed along with another change
// to the tabs, says so, sets aside a copy of state.json first, and lets
// Restore previous arrangement put the tab back once a tab has its name
// again, with its windows and its terminal, which goes on running.
func TestReloadConfigSaysWhichArrangementItCouldntKeep(t *testing.T) {
	h := newHarness(t, setup{})
	floating := floatingIDs(h.view())
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "right"))
	code := treePanes(h.view().Tabs[1].Root)
	if !equalPanes(code, [][]string{{"w3"}, {"w5"}, {"w4"}}) {
		t.Fatalf("Code: %v", code)
	}
	if !h.terms.Running("w5") {
		t.Fatalf("Code's terminal isn't running")
	}
	h.advance(2 * saveDelay)
	saved := readFile(t, h, config.StateFile)

	writeConfig(t, h, exampleWith(t, func(c *config.Config) {
		c.Tabs[1].Name = "Dev"
		c.Tabs = append(c.Tabs, config.Tab{Name: "Notes", Layout: "single"})
	}))
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Dev", "Research", "Notes"}) || len(layout.Leaves(v.Tabs[1].Root)) != 2 {
		t.Fatalf("tabs %v, Dev %v; want the file's", got, treePanes(v.Tabs[1].Root))
	}
	n, ok := noticeWith(v.Notices, "The config.toml that Bench reloaded had no tab called Code, so Bench couldn't keep its arrangement.")
	if !ok || !strings.HasSuffix(n.Text, " To get it back, name a tab Code again, then choose Restore previous arrangement here.") ||
		!slices.Equal(n.Actions, []NoticeAction{actionDismissArrangement}) {
		t.Fatalf("notices %+v", v.Notices)
	}
	if readFile(t, h, config.BadState) != saved {
		t.Errorf("state.json.bad isn't the state.json from before the reload")
	}
	h.advance(2 * saveDelay)
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("state.json doesn't follow config.toml: %v", err)
	}

	// Dev is called Code again, as the only change to the tabs: it's a
	// rename, which keeps Dev's arrangement, and Code's can be restored.
	editConfig(t, h, `name = "Dev"`, `name = "Code"`)
	mustOK(t, h.e.ReloadConfig())
	n, _ = noticeWith(h.view().Notices, "The config.toml that Bench reloaded had no tab called Code")
	if !slices.Equal(n.Actions, []NoticeAction{actionRestoreArrangement, actionDismissArrangement}) {
		t.Fatalf("notice %+v, want it to offer Restore previous arrangement", n)
	}
	mustOK(t, h.e.RestoreArrangement())
	h.settle()
	v = h.view()
	if got := treePanes(v.Tabs[1].Root); !equalPanes(got, code) || v.Tabs[1].Preset != layout.PresetCustom {
		t.Errorf("Code's panes %v, want %v", got, code)
	}
	if h.itemOf(h.windowID(zedID, "bench — switch.go")) != "w3" || h.itemOf(h.windowID(vscodeID, "index.html — bench-ui")) != "w4" {
		t.Errorf("Zed's window is %q, VS Code's %q", h.itemOf(h.windowID(zedID, "bench — switch.go")), h.itemOf(h.windowID(vscodeID, "index.html — bench-ui")))
	}
	if !h.terms.Running("w5") || !slices.Equal(floatingIDs(v), floating) || len(v.Notices) != 0 {
		t.Errorf("terminal running %v, floating %v, notices %v", h.terms.Running("w5"), floatingIDs(v), texts(v.Notices))
	}
}

// TestReloadConfigRestoresWindowsThatLeftWithTheTab checks that a tab that
// a reload deleted gets back, when it's restored, the windows, pages, and
// terminals that left it for "Not in a tab".
func TestReloadConfigRestoresWindowsThatLeftWithTheTab(t *testing.T) {
	h := newHarness(t, setup{})
	floating := floatingIDs(h.view())
	research := treePanes(h.view().Tabs[2].Root)
	writeConfig(t, h, exampleWith(t, func(c *config.Config) { c.Tabs = c.Tabs[:2] }))
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	v := h.view()
	if len(v.Tabs) != 2 || len(floatingIDs(v)) != len(floating)+4 {
		t.Fatalf("tabs %v, floating %v; want Research's windows not in a tab", tabNames(v), floatingIDs(v))
	}
	if _, ok := noticeWith(v.Notices, "The config.toml that Bench reloaded had no tab called Research"); !ok {
		t.Fatalf("notices %v", texts(v.Notices))
	}
	writeConfig(t, h, string(config.ExampleTOML()))
	mustOK(t, h.e.ReloadConfig())
	mustOK(t, h.e.RestoreArrangement())
	h.settle()
	v = h.view()
	if got := treePanes(v.Tabs[2].Root); !equalPanes(got, research) || !slices.Equal(floatingIDs(v), floating) {
		t.Errorf("Research's panes %v, want %v; floating %v", got, research, floatingIDs(v))
	}
}

// TestReloadConfigWritesAMissingFileAfterProblems checks that, after Bench
// started with another config because config.toml had problems, removing
// the file and reloading it, which writes Bench's config there, says what
// Bench couldn't keep of state.json, having set it aside first.
func TestReloadConfigWritesAMissingFileAfterProblems(t *testing.T) {
	data, _ := splitCode(t)
	bad := strings.Replace(string(config.ExampleTOML()), "gap = 8 ", "gapp = 8 ", 1)
	h := newHarness(t, setup{config: bad, state: writeState(data)})
	if err := os.Remove(filepath.Join(h.dir, config.ConfigFile)); err != nil {
		t.Fatal(err)
	}
	mustOK(t, h.e.ReloadConfig())
	v := h.view()
	if !v.ConfigWritable {
		t.Errorf("ConfigWritable should be true")
	}
	if _, ok := noticeWith(v.Notices, "Bench started with another config while config.toml had problems, so it couldn't keep the arrangement of Comms, Code, and Research."); !ok {
		t.Errorf("notices %v", texts(v.Notices))
	}
	if readFile(t, h, config.BadState) != string(data) {
		t.Errorf("state.json.bad isn't the state.json Bench started with")
	}
}

// TestStartupWithProblemsSetsAsideStateItCantUseLater checks that a
// state.json that can't be used is left alone while config.toml has
// problems, and set aside whole once the file is fixed, with the notice.
func TestStartupWithProblemsSetsAsideStateItCantUseLater(t *testing.T) {
	data := savedState(t, nil)
	newer := strings.Replace(string(data), `"version": 1`, `"version": 2`, 1)
	bad := strings.Replace(string(config.ExampleTOML()), "gap = 8 ", "gapp = 8 ", 1)
	h := newHarness(t, setup{config: bad, state: writeState([]byte(newer))})
	if _, ok := noticeWith(h.view().Notices, "Bench couldn't restore"); ok {
		t.Errorf("notices %v while config.toml has problems", texts(h.view().Notices))
	}
	if readFile(t, h, config.StateFile) != newer || readFile(t, h, config.BadState) != "" {
		t.Errorf("state.json was changed or set aside while config.toml has problems")
	}

	writeConfig(t, h, string(config.ExampleTOML()))
	mustOK(t, h.e.ReloadConfig())
	want := "Bench couldn't restore the arrangement of Comms, Code, and Research, because state.json can't be used: it's version 2, not 1."
	if _, ok := noticeWith(h.view().Notices, want); !ok {
		t.Errorf("notices %v", texts(h.view().Notices))
	}
	if readFile(t, h, config.BadState) != newer {
		t.Errorf("state.json.bad isn't the state.json Bench couldn't use")
	}
	h.advance(2 * saveDelay)
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("state.json: %v", err)
	}
}

// TestReloadConfigDoesntKeepTheDefaultConfigsTabs checks that a reload of
// a fixed config.toml says nothing of the default config's tabs that it
// drops, which Bench ran with while the file had problems: they aren't the
// user's.
func TestReloadConfigDoesntKeepTheDefaultConfigsTabs(t *testing.T) {
	h := newHarness(t, setup{config: "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n"})
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Research"}) {
		t.Fatalf("tabs %v, want the default config's", got)
	}
	writeConfig(t, h, exampleWith(t, func(c *config.Config) { c.Tabs = c.Tabs[:2] }))
	mustOK(t, h.e.ReloadConfig())
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Comms", "Code"}) || len(v.Notices) != 0 {
		t.Errorf("tabs %v, notices %v", got, texts(v.Notices))
	}
	if readFile(t, h, config.BadState) != "" {
		t.Errorf("state.json was set aside")
	}
}
