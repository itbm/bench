package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
)

// tabColours returns the view's tabs' colours, in order.
func tabColours(v View) []string {
	var out []string
	for _, t := range v.Tabs {
		out = append(out, t.Colour)
	}
	return out
}

// configTabColours returns config.toml's tabs' colours, in order.
func configTabColours(t *testing.T, h *harness) []string {
	t.Helper()
	var out []string
	for _, ct := range savedConfig(t, h).Tabs {
		out = append(out, ct.Colour)
	}
	return out
}

// TestTabColoursFromConfig checks that each tab has the colour that its
// config entry gives it, or none, in the View and the menu bar's tabs, and
// still after a restart, as state.json doesn't repeat it.
func TestTabColoursFromConfig(t *testing.T) {
	h := newHarness(t, setup{})
	want := []string{config.ColourBlue, config.ColourGreen, ""}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours %q, want %q", got, want)
	}
	var menu []string
	for _, tab := range h.lastMenuBar().Tabs {
		menu = append(menu, tab.Colour)
	}
	if !slices.Equal(menu, want) {
		t.Errorf("the menu bar's colours %q, want %q", menu, want)
	}
	h.restart()
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours after a restart %q, want %q", got, want)
	}
}

// TestSaveTabColour checks that Tab properties' Save gives a tab a colour,
// changes it, and takes it away, in the View and in config.toml, which has
// no colour line for a tab with none; that a rename that sends the colour
// back keeps it; and that a colour Bench doesn't have is refused, changing
// nothing.
func TestSaveTabColour(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SaveTab(2, TabDraft{Name: "Research", Preset: "custom", SmallScreens: "tabs", Colour: config.ColourPurple}))
	if got := h.em.lastToast(); got != "Saved the changes to Research." {
		t.Errorf("toast %q", got)
	}
	want := []string{config.ColourBlue, config.ColourGreen, config.ColourPurple}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours %q, want %q", got, want)
	}
	if got := configTabColours(t, h); !slices.Equal(got, want) {
		t.Errorf("config.toml's colours %q, want %q", got, want)
	}
	if text := readFile(t, h, config.ConfigFile); !strings.Contains(text, "small_screens = \"tabs\"\ncolour = \"purple\"\n") {
		t.Errorf("config.toml:\n%s", text)
	}

	// No colour.
	mustOK(t, h.e.SaveTab(0, TabDraft{Name: "Comms", Preset: "custom", SmallScreens: "tabs"}))
	want = []string{"", config.ColourGreen, config.ColourPurple}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours %q, want %q", got, want)
	}
	if text := readFile(t, h, config.ConfigFile); strings.Count(text, "colour = ") != 2 {
		t.Errorf("config.toml should have two colours:\n%s", text)
	}

	// A rename, as rename.js sends it, with the tab's own colour.
	mustOK(t, h.e.SaveTab(1, TabDraft{Name: "Coding", Preset: "custom", SmallScreens: "stack", Colour: config.ColourGreen}))
	if tab := h.view().Tabs[1]; tab.Name != "Coding" || tab.Colour != config.ColourGreen {
		t.Errorf("renamed tab %q, colour %q", tab.Name, tab.Colour)
	}

	// Refused.
	mustErr(t, h.e.SaveTab(1, TabDraft{Name: "Coding", Preset: "custom", Colour: "orange"}), "There's no tab colour called orange.")
	mustErr(t, h.e.SaveTab(-1, TabDraft{Name: "Notes", Preset: "single", Colour: "Blue"}), "There's no tab colour called Blue.")
	if got := tabColours(h.view()); !slices.Equal(got, want) || len(h.view().Tabs) != 3 {
		t.Errorf("a refused save changed the colours: %q", got)
	}
}

// TestNewTabColour checks that New tab gives the tab it adds the draft's
// colour, with an empty layout or with windows that are open.
func TestNewTabColour(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SaveTab(-1, TabDraft{Name: "Notes", Preset: "single", Colour: config.ColourPink}))
	v := h.view()
	if tab := v.Tabs[3]; tab.Name != "Notes" || tab.Colour != config.ColourPink {
		t.Errorf("new tab %+v", tab)
	}
	files := h.fid(finderID, "Downloads")
	mustOK(t, h.e.NewTabFromWindows(TabDraft{Name: "Files", Preset: "single", Colour: config.ColourGrey}, []string{files}))
	want := []string{config.ColourBlue, config.ColourGreen, "", config.ColourPink, config.ColourGrey}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours %q, want %q", got, want)
	}
	if got := configTabColours(t, h); !slices.Equal(got, want) {
		t.Errorf("config.toml's colours %q, want %q", got, want)
	}
	// Its Undo takes the tab away, and the others keep their colours.
	mustOK(t, h.e.UndoLatest())
	if got := tabColours(h.view()); !slices.Equal(got, want[:4]) {
		t.Errorf("colours after Undo %q, want %q", got, want[:4])
	}
	mustErr(t, h.e.NewTabFromWindows(TabDraft{Name: "Files", Preset: "single", Colour: "teal"}, []string{files}), "There's no tab colour called teal.")
}

// TestDeletedTabKeepsColour checks that a deleted tab comes back with its
// colour, from Undo, and from Reopen Closed Tab after Bench restarts, as
// state.json keeps closed tabs' config entries.
func TestDeletedTabKeepsColour(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.DeleteTab(0))
	if got := tabColours(h.view()); !slices.Equal(got, []string{config.ColourGreen, ""}) {
		t.Fatalf("colours %q", got)
	}
	mustOK(t, h.e.UndoLatest())
	want := []string{config.ColourBlue, config.ColourGreen, ""}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours after Undo %q, want %q", got, want)
	}
	if got := configTabColours(t, h); !slices.Equal(got, want) {
		t.Errorf("config.toml's colours after Undo %q, want %q", got, want)
	}

	mustOK(t, h.e.DeleteTab(1))
	h.settle()
	h.restart()
	mustOK(t, h.e.ReopenTab("Code"))
	h.settle()
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours after Reopen Closed Tab %q, want %q", got, want)
	}
	if got := configTabColours(t, h); !slices.Equal(got, want) {
		t.Errorf("config.toml's colours after Reopen Closed Tab %q, want %q", got, want)
	}
}

// TestReloadConfigColour checks that a colour changed in config.toml while
// Bench runs applies when the file is reloaded.
func TestReloadConfigColour(t *testing.T) {
	h := newHarness(t, setup{})
	text := strings.Replace(exampleConfig(t), "colour = \"blue\"\n", "colour = \"red\"\n", 1)
	text = strings.Replace(text, "colour = \"green\"\n", "", 1)
	writeConfig(t, h, text)
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	want := []string{config.ColourRed, "", ""}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours after the reload %q, want %q", got, want)
	}
}

// TestSaveTabUndo checks that Undo takes back Tab properties' Save of a
// tab that was there, in the View and in config.toml, with its rule, and
// that the changes before it stay within Undo's reach.
func TestSaveTabUndo(t *testing.T) {
	h := newHarness(t, setup{config: exampleConfig(t) + researchRule})
	mustOK(t, h.e.DeleteTab(0))
	deleted := h.em.lastUndo(t)

	// Research gets a new name, a colour, and the first place.
	mustOK(t, h.e.SaveTab(1, TabDraft{Name: "Reading", Preset: "custom", SmallScreens: "stack", Colour: config.ColourPink, Place: 1}))
	saved := h.em.lastUndo(t)
	if got := h.em.lastToast(); got != "Saved the changes to Reading." {
		t.Errorf("toast %q", got)
	}
	if u := h.view().Undo; u == nil || u.Step != saved || u.Name != "Change Tab Properties" || u.Did != "changed the properties of Reading" {
		t.Errorf("the View's Undo: %+v", u)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Reading", "Code"}) || savedConfig(t, h).Rules[1].Tab != "Reading" {
		t.Fatalf("config.toml: tabs %v, rules %+v", got, savedConfig(t, h).Rules)
	}

	mustOK(t, h.e.UndoLatest())
	h.settle()
	if got := h.em.lastToast(); got != "Undid: changed the properties of Reading." {
		t.Errorf("toast %q", got)
	}
	want := []string{config.ColourGreen, ""}
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Code", "Research"}) {
		t.Errorf("tabs %v", got)
	}
	if got := tabColours(h.view()); !slices.Equal(got, want) {
		t.Errorf("colours %q, want %q", got, want)
	}
	cfg := savedConfig(t, h)
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Code", "Research"}) || cfg.Rules[1].Tab != "Research" {
		t.Errorf("config.toml: tabs %v, rules %+v", got, cfg.Rules)
	}
	if got := configTabColours(t, h); !slices.Equal(got, want) {
		t.Errorf("config.toml's colours %q, want %q", got, want)
	}

	// The tab deleted before it is still within reach.
	if u := h.view().Undo; u == nil || u.Step != deleted {
		t.Fatalf("the View's Undo: %+v, want the delete, %d", u, deleted)
	}
	mustOK(t, h.e.Undo(deleted))
	h.settle()
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs after the second Undo: %v", got)
	}
}

// TestRenameTabUndo checks that a rename in place, which saves the tab's
// layout, small-screen setting, and colour as they are, is named Rename
// Tab for Undo, and that Undo of a change before it takes it back too.
func TestRenameTabUndo(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w3", 1, h.paneOf("w5"), "right"))
	moved := h.em.lastUndo(t)
	mustOK(t, h.e.SaveTab(1, TabDraft{Name: "Coding", Preset: "custom", SmallScreens: "stack", Colour: config.ColourGreen}))
	if u := h.view().Undo; u == nil || u.Name != "Rename Tab" || u.Did != "renamed Code to Coding" {
		t.Errorf("the View's Undo: %+v", u)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	if got := h.em.lastToast(); got != "Renamed Coding back to Code." {
		t.Errorf("toast %q", got)
	}
	if got := configTabNames(t, h); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("config.toml's tabs %v", got)
	}

	// Undo of the move before a rename takes back both.
	mustOK(t, h.e.SaveTab(1, TabDraft{Name: "Coding", Preset: "custom", SmallScreens: "stack", Colour: config.ColourGreen}))
	mustOK(t, h.e.Undo(moved))
	h.settle()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("tabs %v", got)
	}
	if got := configTabColours(t, h); !slices.Equal(got, []string{config.ColourBlue, config.ColourGreen, ""}) {
		t.Errorf("config.toml's colours %q", got)
	}
}

// TestSaveTabUndoWithKeepOverBench checks Undo of Tab properties' Save
// beside Keep over Bench (decision 68), which changes only config.toml's
// rules: Undo of the rule leaves the tab's colour, and Undo of the change
// before it takes back both, in config.toml too.
func TestSaveTabUndoWithKeepOverBench(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SaveTab(0, TabDraft{Name: "Comms", Preset: "custom", SmallScreens: "tabs", Colour: config.ColourPink}))
	saved := h.em.lastUndo(t)
	h.comeForward()
	mustOK(t, h.e.KeepOverBench(finderID))
	kept := h.em.lastUndo(t)
	hasRule := func() bool {
		return slices.ContainsFunc(savedConfig(t, h).Rules, func(r config.Rule) bool { return sameApp(r.App, finderID) })
	}
	if !hasRule() {
		t.Fatal("Keep over Bench should add a rule")
	}

	mustOK(t, h.e.Undo(kept))
	h.settle()
	if hasRule() {
		t.Error("Undo of Keep over Bench should take the rule back")
	}
	pink := []string{config.ColourPink, config.ColourGreen, ""}
	if got := configTabColours(t, h); !slices.Equal(got, pink) {
		t.Errorf("config.toml's colours %q, want %q", got, pink)
	}
	if got := tabColours(h.view()); !slices.Equal(got, pink) {
		t.Errorf("colours %q, want %q", got, pink)
	}

	// Again, and now Undo of the Save takes back the rule after it too.
	h.comeForward()
	mustOK(t, h.e.KeepOverBench(finderID))
	if !hasRule() {
		t.Fatal("Keep over Bench should add the rule again")
	}
	mustOK(t, h.e.Undo(saved))
	h.settle()
	if hasRule() {
		t.Error("Undo of the change before Keep over Bench should take the rule back too")
	}
	was := []string{config.ColourBlue, config.ColourGreen, ""}
	if got := configTabColours(t, h); !slices.Equal(got, was) {
		t.Errorf("config.toml's colours %q, want %q", got, was)
	}
	if got := tabColours(h.view()); !slices.Equal(got, was) {
		t.Errorf("colours %q, want %q", got, was)
	}
}
