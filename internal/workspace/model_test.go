package workspace

import (
	"errors"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

func TestModelFromDefaultConfig(t *testing.T) {
	m := defaultModel(t)
	mustValid(t, m)
	if got := len(m.Tabs); got != 3 {
		t.Fatalf("tabs: got %d, want 3", got)
	}
	tests := []struct {
		tab    int
		name   string
		preset string
		small  layout.SmallScreens
		panes  [][]string
		focus  string
		sizes  []float64
	}{
		{0, "Comms", "side", layout.Tabs, [][]string{{"w1"}, {"w2"}}, "p8", []float64{45, 55}},
		{1, "Code", "main-side", layout.Stack, [][]string{{"w3", "w4"}, {"w5"}}, "p11", []float64{65, 35}},
		{2, "Research", "side", layout.Tabs, [][]string{{"w6"}, {"w7"}}, "p14", []float64{60, 40}},
	}
	for _, tc := range tests {
		tab := m.Tabs[tc.tab]
		if tab.Name != tc.name || tab.Preset != tc.preset || tab.SmallScreens != tc.small {
			t.Errorf("tab %d: got %s %s %s", tc.tab, tab.Name, tab.Preset, tab.SmallScreens)
		}
		if got := paneItems(tab); !equalPanes(got, tc.panes) {
			t.Errorf("tab %s panes: got %v, want %v", tc.name, got, tc.panes)
		}
		if tab.Focus != tc.focus {
			t.Errorf("tab %s focus: got %s, want %s", tc.name, tab.Focus, tc.focus)
		}
		if !slices.Equal(tab.Root.Sizes, tc.sizes) {
			t.Errorf("tab %s sizes: got %v, want %v", tc.name, tab.Root.Sizes, tc.sizes)
		}
	}
	if m.Seq != 16 {
		t.Errorf("seq: got %d, want 16", m.Seq)
	}
}

func TestModelFromConfigPadsMissingPanes(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "Grid"
layout = "grid"
  [[tab.pane]]
  items = [{ terminal = "~" }]
`)
	mustValid(t, m)
	got := paneItems(m.Tabs[0])
	want := [][]string{{"w1"}, nil, nil, nil}
	if !equalPanes(got, want) {
		t.Fatalf("panes: got %v, want %v", got, want)
	}
}

func TestItemFromConfig(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  Item
	}{
		{"web with name", `{ web = "https://app.slack.com/client", profile = "slack", name = "Slack" }`,
			Item{Kind: KindWeb, App: "Slack", Mono: "Sl", Title: "Slack", Name: "Slack", URL: "https://app.slack.com/client", Profile: "slack"}},
		{"web without name or profile", `{ web = "https://outlook.office.com/mail/" }`,
			Item{Kind: KindWeb, App: "Outlook", Mono: "Ou", Title: "Outlook", URL: "https://outlook.office.com/mail/", Profile: "outlook.office.com"}},
		{"vscode", `{ vscode = "~/code/bench" }`,
			Item{Kind: KindWeb, App: "VS Code", Mono: "VS", Title: "~/code/bench", VSCode: "~/code/bench", Profile: "vscode"}},
		{"terminal", `{ terminal = "~/code" }`,
			Item{Kind: KindBuiltin, App: "Terminal", Mono: ">_", Title: "zsh — ~/code", Folder: "~/code"}},
		{"terminal with name and command", `{ terminal = "~", command = "htop", name = "Top" }`,
			Item{Kind: KindBuiltin, App: "Terminal", Mono: ">_", Title: "Top", Name: "Top", Folder: "~", Command: "htop"}},
		{"app", `{ app = "dev.zed.Zed" }`,
			Item{Kind: KindApp, App: "Zed", Mono: "Zd", Title: "Zed", BundleID: "dev.zed.Zed", Status: StatusMissing}},
		{"app with title and url", `{ app = "com.brave.Browser", title = "GitHub", url = "https://github.com" }`,
			Item{Kind: KindApp, App: "Brave", Mono: "Br", Title: "Brave", BundleID: "com.brave.Browser",
				TitleFilter: "GitHub", OpenURL: "https://github.com", Status: StatusMissing}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := parseConfig(t, "[[tab]]\nname = \"T\"\nlayout = \"single\"\n  [[tab.pane]]\n  items = ["+tc.entry+"]\n")
			got := itemFromConfig(cfg.Tabs[0].Panes[0].Items[0], testNamer)
			want := tc.want
			if want.Kind == KindWeb && want.VSCode == "" {
				want.Home = want.URL
			}
			if want.Status == "" {
				want.Status = StatusReady
			}
			if *got != want {
				t.Errorf("got %+v\nwant %+v", *got, want)
			}
		})
	}
}

func TestMoveToCentre(t *testing.T) {
	m := defaultModel(t)
	code := m.Tabs[1]
	moved, err := m.Move("w4", 1, "p12", layout.Center)
	if err != nil || !moved {
		t.Fatalf("Move: %v %v", moved, err)
	}
	mustValid(t, m)
	if got := paneItems(code); !equalPanes(got, [][]string{{"w3"}, {"w5", "w4"}}) {
		t.Errorf("panes: got %v", got)
	}
	if p := code.Pane("p12"); p.Active != "w4" {
		t.Errorf("active: got %s, want w4", p.Active)
	}
	if code.Focus != "p12" {
		t.Errorf("focus: got %s, want p12", code.Focus)
	}
	if code.Preset != layout.PresetCustom {
		t.Errorf("preset: got %s, want custom", code.Preset)
	}
}

func TestMoveToOwnPaneCentreDoesNothing(t *testing.T) {
	m := defaultModel(t)
	moved, err := m.Move("w3", 1, "p11", layout.Center)
	if err != nil || moved {
		t.Fatalf("Move: got %v %v, want false nil", moved, err)
	}
	if m.Tabs[1].Preset != "main-side" {
		t.Errorf("preset changed to %s", m.Tabs[1].Preset)
	}
}

func TestMoveRefusesSplittingByOnlyItem(t *testing.T) {
	m := defaultModel(t)
	before := layout.Clone(m.Tabs[1].Root)
	for _, zone := range []layout.Zone{layout.Left, layout.Right, layout.Top, layout.Bottom} {
		_, err := m.Move("w5", 1, "p12", zone)
		if !errors.Is(err, errOnlyItem) {
			t.Errorf("zone %s: got %v, want errOnlyItem", zone, err)
		}
	}
	if got := paneItems(m.Tabs[1]); !equalPanes(got, [][]string{{"w3", "w4"}, {"w5"}}) {
		t.Errorf("tree changed: %v (was %v)", got, layout.Items(before))
	}
}

func TestMoveToEdgeSplits(t *testing.T) {
	tests := []struct {
		zone   layout.Zone
		dir    layout.Dir
		before bool
	}{
		{layout.Right, layout.Row, false},
		{layout.Left, layout.Row, true},
		{layout.Top, layout.Column, true},
		{layout.Bottom, layout.Column, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.zone), func(t *testing.T) {
			m := defaultModel(t)
			moved, err := m.Move("w4", 1, "p11", tc.zone)
			if err != nil || !moved {
				t.Fatalf("Move: %v %v", moved, err)
			}
			mustValid(t, m)
			code := m.Tabs[1]
			fresh := layout.PaneOf(code.Root, "w4")
			if fresh == nil || fresh.ID == "p11" || len(fresh.Items) != 1 {
				t.Fatalf("w4 should be alone in a new pane, got %+v", fresh)
			}
			if code.Focus != fresh.ID {
				t.Errorf("focus: got %s, want %s", code.Focus, fresh.ID)
			}
			if len(code.Leaves()) != 3 {
				t.Errorf("panes: got %d, want 3", len(code.Leaves()))
			}
			_, parent, i := layout.Find(code.Root, fresh.ID)
			if parent.Dir != tc.dir {
				t.Errorf("split direction: got %s, want %s", parent.Dir, tc.dir)
			}
			other := parent.Children[1]
			if tc.before {
				other = parent.Children[i+1]
			} else {
				other = parent.Children[i-1]
			}
			if other.ID != "p11" {
				t.Errorf("neighbour: got %s, want p11", other.ID)
			}
			if code.Preset != layout.PresetCustom {
				t.Errorf("preset: got %s", code.Preset)
			}
		})
	}
}

func TestMoveRightIntoRowAddsSibling(t *testing.T) {
	m := defaultModel(t)
	// Code's root is a row, so splitting p11 to the right adds a third child.
	if _, err := m.Move("w4", 1, "p11", layout.Right); err != nil {
		t.Fatal(err)
	}
	root := m.Tabs[1].Root
	if len(root.Children) != 3 || root.Dir != layout.Row {
		t.Fatalf("root: %d children, %s", len(root.Children), root.Dir)
	}
	want := []float64{32.5, 32.5, 35}
	for i, s := range root.Sizes {
		if diff := s - want[i]; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("sizes: got %v, want %v", root.Sizes, want)
		}
	}
}

func TestMoveCollapsesEmptyPane(t *testing.T) {
	m := defaultModel(t)
	// Moving the terminal, p12's only item, into p11 leaves p12 empty.
	if _, err := m.Move("w5", 1, "p11", layout.Center); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	code := m.Tabs[1]
	if !code.Root.IsPane() || code.Root.ID != "p11" {
		t.Fatalf("root: got %+v, want pane p11", code.Root)
	}
	if !slices.Equal(code.Root.Items, []string{"w3", "w4", "w5"}) {
		t.Errorf("items: %v", code.Root.Items)
	}
}

func TestMoveFromOnlyPaneToAnotherPanesEdge(t *testing.T) {
	m := defaultModel(t)
	// w5 is alone in p12; splitting p11 with it removes p12.
	if _, err := m.Move("w5", 1, "p11", layout.Bottom); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	code := m.Tabs[1]
	if code.Pane("p12") != nil {
		t.Errorf("p12 should have closed up")
	}
	if len(code.Leaves()) != 2 {
		t.Errorf("panes: got %d, want 2", len(code.Leaves()))
	}
	if code.Root.Dir != layout.Column {
		t.Errorf("root: got %s, want a column", code.Root.Dir)
	}
}

func TestMoveAcrossTabs(t *testing.T) {
	m := defaultModel(t)
	if _, err := m.Move("w6", 1, "p12", layout.Center); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	if got := paneItems(m.Tabs[2]); !equalPanes(got, [][]string{{"w7"}}) {
		t.Errorf("Research: got %v", got)
	}
	if got := paneItems(m.Tabs[1]); !equalPanes(got, [][]string{{"w3", "w4"}, {"w5", "w6"}}) {
		t.Errorf("Code: got %v", got)
	}
	if m.Tabs[2].Preset != layout.PresetCustom || m.Tabs[1].Preset != layout.PresetCustom {
		t.Errorf("presets: %s %s", m.Tabs[1].Preset, m.Tabs[2].Preset)
	}
}

func TestMoveErrors(t *testing.T) {
	tests := []struct {
		name string
		id   string
		tab  int
		pane string
		zone layout.Zone
		want error
	}{
		{"no tab", "w3", 7, "p11", layout.Center, errNoTab(7)},
		{"no item", "w99", 1, "p11", layout.Center, errGone},
		{"no pane", "w3", 1, "p8", layout.Center, errNoPane},
		{"bad zone", "w3", 1, "p12", "middle", errBadZone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := defaultModel(t)
			_, err := m.Move(tc.id, tc.tab, tc.pane, tc.zone)
			if err == nil || err.Error() != tc.want.Error() {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
	if got := errNoTab(6).Error(); got != "There's no tab 7." {
		t.Errorf("errNoTab: %q", got)
	}
}

func TestMoveToTab(t *testing.T) {
	m := defaultModel(t)
	m.Tabs[2].Focus = "p15"
	moved, err := m.MoveToTab("w3", 2)
	if err != nil || !moved {
		t.Fatalf("MoveToTab: %v %v", moved, err)
	}
	mustValid(t, m)
	if got := paneItems(m.Tabs[2]); !equalPanes(got, [][]string{{"w6"}, {"w7", "w3"}}) {
		t.Errorf("Research: got %v", got)
	}
	if p := m.Tabs[2].Pane("p15"); p.Active != "w3" {
		t.Errorf("active: got %s", p.Active)
	}
	if m.Current != 0 {
		t.Errorf("current changed to %d", m.Current)
	}
	moved, err = m.MoveToTab("w3", 2)
	if err != nil || moved {
		t.Errorf("moving to its own tab: got %v %v, want false nil", moved, err)
	}
}

func TestTakeKeepsLastPane(t *testing.T) {
	m, _ := modelOf(t, "[[tab]]\nname = \"T\"\nlayout = \"single\"\n  [[tab.pane]]\n  items = [{ terminal = \"~\" }]\n")
	tab, removed := m.Take("w1")
	if tab != 0 || removed {
		t.Errorf("Take: got %d %v", tab, removed)
	}
	if !m.Tabs[0].Root.IsPane() || len(m.Tabs[0].Root.Items) != 0 {
		t.Errorf("root: %+v", m.Tabs[0].Root)
	}
	if m.Tabs[0].Preset != "single" {
		t.Errorf("preset: %s", m.Tabs[0].Preset)
	}
}

func TestTakeAndRemove(t *testing.T) {
	m := defaultModel(t)
	m.Floating = []string{"w9"}
	m.Items["w9"] = &Item{ID: "w9", Kind: KindWeb}
	if tab, _ := m.Take("w9"); tab != -1 || m.IsFloating("w9") {
		t.Errorf("Take floating: tab %d, still floating %v", tab, m.IsFloating("w9"))
	}
	if m.Items["w9"] == nil {
		t.Errorf("Take forgot the item")
	}
	if tab, removed := m.Remove("w7"); tab != 2 || !removed {
		t.Errorf("Remove: got %d %v", tab, removed)
	}
	if m.Items["w7"] != nil {
		t.Errorf("Remove kept the item")
	}
	if m.Tabs[2].Preset != layout.PresetCustom {
		t.Errorf("a removed pane should make the tab custom")
	}
	mustValid(t, m)
}

func TestShowAndFocus(t *testing.T) {
	m := defaultModel(t)
	if !m.Show("w4") {
		t.Fatal("Show reported false")
	}
	code := m.Tabs[1]
	if code.Pane("p11").Active != "w4" || code.Focus != "p11" || code.CompactShown != "w4" {
		t.Errorf("after Show: active %s focus %s compact %s", code.Pane("p11").Active, code.Focus, code.CompactShown)
	}
	if m.Items["w4"].Used == 0 {
		t.Errorf("Show should mark the item used")
	}
	m.FocusPane(code, code.Pane("p12"))
	if code.Focus != "p12" || code.CompactShown != "w5" {
		t.Errorf("after FocusPane: focus %s compact %s", code.Focus, code.CompactShown)
	}
	if m.Show("nope") {
		t.Errorf("Show of an unknown item reported true")
	}
}

func TestBindAndUnbind(t *testing.T) {
	m := defaultModel(t)
	zed := m.Items["w3"]
	m.Bind(zed, win(42, "dev.zed.Zed", "bench", 1))
	if m.ItemByWindow(42) != zed || zed.Created != 1 || zed.Title != "bench" || zed.Status != StatusReady {
		t.Fatalf("after Bind: %+v", zed)
	}
	m.Unbind(zed)
	if m.ItemByWindow(42) != nil || zed.WindowID != 0 || zed.Created != 1 || zed.Title != "bench" {
		t.Fatalf("after Unbind: %+v", zed)
	}
}

func TestSetSizes(t *testing.T) {
	m := defaultModel(t)
	ti, err := m.SetSizes("s13", []float64{1, 3})
	if err != nil || ti != 1 {
		t.Fatalf("SetSizes: %d %v", ti, err)
	}
	if !slices.Equal(m.Tabs[1].Root.Sizes, []float64{25, 75}) {
		t.Errorf("sizes: %v", m.Tabs[1].Root.Sizes)
	}
	if m.Tabs[1].Preset != "main-side" {
		t.Errorf("SetSizes changed the preset to %s", m.Tabs[1].Preset)
	}
	for _, tc := range []struct {
		id    string
		sizes []float64
		want  error
	}{
		{"s99", []float64{50, 50}, errNoSplit},
		{"p11", []float64{50, 50}, errNoSplit},
		{"s13", []float64{50}, errBadSizes},
		{"s13", []float64{-1, 50}, errBadSizes},
	} {
		if _, err := m.SetSizes(tc.id, tc.sizes); !errors.Is(err, tc.want) {
			t.Errorf("SetSizes(%s, %v): got %v, want %v", tc.id, tc.sizes, err, tc.want)
		}
	}
}

func TestApplyPresetKeepsItems(t *testing.T) {
	m := defaultModel(t)
	if err := m.ApplyPreset(1, "three", nil); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	if got := paneItems(m.Tabs[1]); !equalPanes(got, [][]string{{"w3", "w4"}, {"w5"}, nil}) {
		t.Errorf("three: %v", got)
	}
	if err := m.ApplyPreset(1, "single", nil); err != nil {
		t.Fatal(err)
	}
	if got := paneItems(m.Tabs[1]); !equalPanes(got, [][]string{{"w3", "w4", "w5"}}) {
		t.Errorf("single: %v", got)
	}
	if m.Tabs[1].Preset != "single" {
		t.Errorf("preset: %s", m.Tabs[1].Preset)
	}
	if err := m.ApplyPreset(1, "custom", nil); err == nil || err.Error() != "There's no layout called custom." {
		t.Errorf("custom: %v", err)
	}
}

func TestAddAndDeleteTab(t *testing.T) {
	m := defaultModel(t)
	tab, err := m.AddTab("Notes", "side", layout.Same)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Tabs) != 4 || len(tab.Leaves()) != 2 || tab.Focus != tab.Leaves()[0].ID {
		t.Fatalf("AddTab: %d tabs, tab %+v", len(m.Tabs), tab)
	}
	mustValid(t, m)

	m.Current = 2
	keep := func(it *Item) bool { return it.Kind != KindApp || it.WindowID != 0 }
	m.Bind(m.Items["w6"], win(9, "com.brave.Browser", "Docs", 1))
	kept, dropped, err := m.DeleteTab(2, keep)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(kept, []string{"w6"}) || !slices.Equal(dropped, []string{"w7"}) {
		t.Errorf("kept %v dropped %v", kept, dropped)
	}
	if !m.IsFloating("w6") || m.Items["w7"] != nil {
		t.Errorf("w6 floating %v, w7 %v", m.IsFloating("w6"), m.Items["w7"])
	}
	if m.Current != 2 || m.Tabs[2].Name != "Notes" {
		t.Errorf("current: %d (%s)", m.Current, m.Tabs[m.Current].Name)
	}
	if _, _, err := m.DeleteTab(0, keep); err != nil {
		t.Fatal(err)
	}
	if m.Current != 1 {
		t.Errorf("deleting an earlier tab: current %d, want 1", m.Current)
	}
	m.DeleteTab(0, keep)
	if _, _, err := m.DeleteTab(0, keep); !errors.Is(err, errLastTab) {
		t.Errorf("deleting the last tab: got %v", err)
	}
}

func TestCycle(t *testing.T) {
	items := []string{"a", "b", "c"}
	tests := []struct {
		current string
		step    int
		want    string
	}{
		{"a", 1, "b"}, {"c", 1, "a"}, {"a", -1, "c"}, {"b", -1, "a"}, {"x", 1, "a"}, {"x", -1, "c"},
	}
	for _, tc := range tests {
		if got := cycle(items, tc.current, tc.step); got != tc.want {
			t.Errorf("cycle(%s, %d): got %s, want %s", tc.current, tc.step, got, tc.want)
		}
	}
	if got := cycle(nil, "", 1); got != "" {
		t.Errorf("empty: %q", got)
	}
}

func TestNewIDsShareOneSequence(t *testing.T) {
	m := newModel()
	got := []string{m.NewID(itemPrefix), m.nodeID(layout.TypePane), m.nodeID(layout.TypeSplit)}
	if !slices.Equal(got, []string{"w1", "p2", "s3"}) {
		t.Errorf("got %v", got)
	}
}

func TestReindexKeepsOneItemPerWindow(t *testing.T) {
	// A state.json edited by hand gives two items the same window: the first
	// in reading order keeps it, and the other waits for one.
	m := defaultModel(t)
	m.Items["w6"].WindowID, m.Items["w6"].Created = 42, 1
	m.Items["w7"].WindowID, m.Items["w7"].Created = 42, 2
	for range 20 {
		m.Items["w7"].WindowID = 42
		m.reindex()
		if got := m.ItemByWindow(42); got == nil || got.ID != "w6" || m.Items["w7"].WindowID != 0 {
			t.Fatalf("window 42: %+v, w7's window %d", got, m.Items["w7"].WindowID)
		}
	}
	if m.created != 2 {
		t.Errorf("created: %d, want 2", m.created)
	}
}
