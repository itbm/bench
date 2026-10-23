package workspace

import (
	"reflect"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/config"
)

// savedWithEntries returns the default model as state.json, with each
// tab's entry keys as the default config gives them.
func savedWithEntries(t *testing.T) *config.State {
	t.Helper()
	cfg := config.Example()
	st := defaultModel(t).State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	return st
}

func TestModelFromStateTakesEditsByHand(t *testing.T) {
	st := savedWithEntries(t)
	// While Bench isn't running, the user renames Slack, changes Outlook's
	// address, removes Zed, changes the terminal's command, and gives
	// Research's Brave entry a title filter and a url.
	cfg := config.Example()
	cfg.Tabs[0].Panes[0].Items[0].Name = "Team chat"
	cfg.Tabs[0].Panes[1].Items[0].Web = "https://outlook.office.com/owa/"
	cfg.Tabs[1].Panes[0].Items = cfg.Tabs[1].Panes[0].Items[1:]
	cfg.Tabs[1].Panes[1].Items[0].Command = "make test"
	cfg.Tabs[2].Panes[0].Items[0].Title = "Docs"
	cfg.Tabs[2].Panes[0].Items[0].URL = "https://go.dev/doc"

	m := modelFromState(st, cfg, testNamer)
	mustValid(t, m)
	if it := m.Items["w1"]; it.Name != "Team chat" || it.App != "Team chat" || it.Title != "Team chat" || it.Profile != "slack" {
		t.Errorf("Slack after renaming: name %q app %q title %q profile %q", it.Name, it.App, it.Title, it.Profile)
	}
	if it := m.Items["w2"]; it.Home != "https://outlook.office.com/owa/" || it.URL != it.Home || it.Profile != "outlook" {
		t.Errorf("Outlook after its address changed: home %q url %q profile %q", it.Home, it.URL, it.Profile)
	}
	if got := paneItems(m.Tabs[0]); !equalPanes(got, [][]string{{"w1"}, {"w2"}}) {
		t.Errorf("Comms: %v, want Slack and Outlook, without a second Outlook", got)
	}
	if _, ok := m.Items["w3"]; ok || !equalPanes(paneItems(m.Tabs[1]), [][]string{{"w4"}, {"w5"}}) {
		t.Errorf("Code: %v, want Zed gone", paneItems(m.Tabs[1]))
	}
	if it := m.Items["w5"]; it.Command != "make test" || it.Folder != "~/code" {
		t.Errorf("the terminal: folder %q command %q", it.Folder, it.Command)
	}
	// Brave's entry with a title filter is a new one: it has a new
	// placeholder.
	if _, ok := m.Items["w6"]; ok {
		t.Error("Brave's item without a title filter should go")
	}
	if p := m.Tabs[2].Leaves()[0]; len(p.Items) != 1 {
		t.Errorf("Research's first pane: %v, want Brave's new placeholder", p.Items)
	} else if it := m.Items[p.Items[0]]; it.TitleFilter != "Docs" || it.OpenURL != "https://go.dev/doc" {
		t.Errorf("Brave: title filter %q url %q", it.TitleFilter, it.OpenURL)
	}

	// The layout editor's next save writes the edits, not the old values.
	for i, ct := range cfg.Tabs {
		if got := m.configPanes(m.Tabs[i], len(ct.Panes)); !reflect.DeepEqual(got, ct.Panes) {
			t.Errorf("tab %s would be saved as %+v, want %+v", ct.Name, got, ct.Panes)
		}
	}
}

func TestModelFromStateKeepsTheArrangementThroughEdits(t *testing.T) {
	m := defaultModel(t)
	// In Bench, Outlook moved into Slack's pane, and the terminal was
	// closed.
	if _, err := m.Move("w2", 0, "p8", "center"); err != nil {
		t.Fatal(err)
	}
	m.Remove("w5")
	cfg := config.Example()
	st := m.State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	// Outlook's address and the terminal's name change by hand.
	cfg.Tabs[0].Panes[1].Items[0].Web = "https://outlook.office.com/owa/"
	cfg.Tabs[1].Panes[1].Items[0].Name = "Shell"

	got := modelFromState(st, cfg, testNamer)
	mustValid(t, got)
	comms := paneItems(got.Tabs[0])
	if len(comms) != 1 || !slices.Equal(comms[0], []string{"w1", "w2"}) {
		t.Errorf("Comms: %v, want Outlook still beside Slack in one pane", comms)
	}
	if got.Items["w2"].Home != "https://outlook.office.com/owa/" {
		t.Errorf("Outlook's address: %q", got.Items["w2"].Home)
	}
	// The terminal was closed in Bench and only renamed by hand: it stays
	// closed.
	if items := got.Tabs[1].ItemIDs(); !slices.Equal(items, []string{"w3", "w4"}) {
		t.Errorf("Code: %v, want the terminal still closed", items)
	}
}

func TestModelFromStatePrefersAWebItemsGivenAddress(t *testing.T) {
	m, cfg := modelOf(t, `
[[tab]]
name = "Docs"
layout = "single"
  [[tab.pane]]
  items = [{ web = "https://a.example.com" }, { web = "https://b.example.com" }]
`)
	// The first page has gone to the second's address.
	m.Items["w1"].URL = "https://b.example.com"
	st := m.State()
	st.Tabs[0].Entries = entryKeys(cfg.Tabs[0])
	// The user swaps the entries by hand.
	ci := cfg.Tabs[0].Panes[0].Items
	ci[0], ci[1] = ci[1], ci[0]

	got := modelFromState(st, cfg, testNamer)
	if items := got.Tabs[0].ItemIDs(); !slices.Equal(items, []string{"w1", "w2"}) {
		t.Errorf("items: %v, want the same two", items)
	}
	if a, b := got.Items["w1"].Home, got.Items["w2"].Home; a != "https://a.example.com" || b != "https://b.example.com" {
		t.Errorf("addresses: %q and %q, want each page's own", a, b)
	}
}

func TestPairEntries(t *testing.T) {
	tests := []struct {
		name        string
		saved, keys []string
		from, gone  []string
	}{
		{"unchanged", []string{"web\x00a", "app\x00z\x00"}, []string{"web\x00a", "app\x00z\x00"},
			[]string{"web\x00a", "app\x00z\x00"}, nil},
		{"edited in place", []string{"web\x00a", "web\x00b", "terminal\x00~\x00"}, []string{"web\x00a", "web\x00c", "terminal\x00~\x00"},
			[]string{"web\x00a", "web\x00b", "terminal\x00~\x00"}, nil},
		{"removed and added", []string{"web\x00a", "web\x00b"}, []string{"web\x00b", "terminal\x00~\x00"},
			[]string{"web\x00b", ""}, []string{"web\x00a"}},
		{"another app isn't an edit", []string{"app\x00x\x00"}, []string{"app\x00y\x00"},
			[]string{""}, []string{"app\x00x\x00"}},
		{"nor is an app's new title filter", []string{"app\x00x\x00"}, []string{"app\x00x\x00Docs"},
			[]string{""}, []string{"app\x00x\x00"}},
		{"an entry moved", []string{"web\x00a", "web\x00b"}, []string{"web\x00b", "web\x00a"},
			[]string{"web\x00b", ""}, []string{"web\x00a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			from, gone := pairEntries(tc.saved, tc.keys)
			if !slices.Equal(from, tc.from) || !slices.Equal(gone, tc.gone) {
				t.Errorf("from %q gone %q, want %q %q", from, gone, tc.from, tc.gone)
			}
		})
	}
}

func TestResetTabKeepsAnItemInItsOwnEntrysPane(t *testing.T) {
	m, cfg := modelOf(t, `
[[tab]]
name = "Research"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "GitHub" }]
  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "Docs" }]
`)
	brave := "com.brave.Browser"
	m.Bind(m.Items["w1"], win(1, brave, "GitHub · home", 1))
	// The Docs window now shows GitHub's docs: its title contains the
	// first entry's filter too.
	m.Bind(m.Items["w2"], win(2, brave, "GitHub Docs", 2))
	if err := m.ResetTab(0, cfg.Tabs[0], testNamer); err != nil {
		t.Fatal(err)
	}
	if got := paneItems(m.Tabs[0]); !equalPanes(got, [][]string{{"w1"}, {"w2"}}) {
		t.Errorf("after Reset layout: %v, want the Docs window still in the Docs pane", got)
	}
}

func TestModelFromStateKeepsAPaneEmptiedByHand(t *testing.T) {
	st := savedWithEntries(t)
	// The user replaces Obsidian, alone in Research's second pane, with a
	// web page.
	cfg := config.Example()
	cfg.Tabs[2].Panes[1].Items = []config.Item{{Web: "https://example.com"}}

	m := modelFromState(st, cfg, testNamer)
	mustValid(t, m)
	panes := paneItems(m.Tabs[2])
	if len(panes) != 2 || !slices.Equal(panes[0], []string{"w6"}) || len(panes[1]) != 1 {
		t.Fatalf("Research: %v, want Brave, then the web page in the second pane", panes)
	}
	if it := m.Items[panes[1][0]]; it.Home != "https://example.com" {
		t.Errorf("the second pane holds %+v, want the web page", it)
	}

	// With the pane's entries all removed, the pane stays, empty, as the
	// config has it.
	cfg.Tabs[2].Panes[1].Items = nil
	m = modelFromState(st, cfg, testNamer)
	mustValid(t, m)
	if panes := paneItems(m.Tabs[2]); len(panes) != 2 || len(panes[1]) != 0 {
		t.Errorf("Research: %v, want Brave, then an empty pane", panes)
	}
}

func TestModelFromStateLeavesAPageThatWentToAnEntrysAddress(t *testing.T) {
	m := defaultModel(t)
	// In Bench, Outlook was closed, and a page added in Comms went to
	// Outlook's address.
	m.Remove("w2")
	other := &Item{ID: m.NewID(itemPrefix), Kind: KindWeb, App: "Example", Title: "Example",
		Home: "https://example.com", URL: "https://outlook.office.com/mail/", Profile: "example.com"}
	m.Add(other, m.Tabs[0], m.Tabs[0].Leaves()[0], -1)
	cfg := config.Example()
	st := m.State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	// Outlook's entry is renamed by hand.
	cfg.Tabs[0].Panes[1].Items[0].Name = "Mail"

	got := modelFromState(st, cfg, testNamer)
	if it := got.Items[other.ID]; it.Home != "https://example.com" || it.Profile != "example.com" || it.Name != "" {
		t.Errorf("the other page took Outlook's entry: home %q profile %q name %q", it.Home, it.Profile, it.Name)
	}
}

func TestModelFromStateUsesAPageAddedInBenchForAnEditedEntry(t *testing.T) {
	m := defaultModel(t)
	// In Bench, Slack was closed, and another page added.
	m.Remove("w1")
	page := &Item{ID: m.NewID(itemPrefix), Kind: KindWeb, App: "Example", Title: "Example",
		Home: "https://example.com", URL: "https://example.com", Profile: "example.com"}
	m.Add(page, m.Tabs[0], m.Tabs[0].Leaves()[0], -1)
	cfg := config.Example()
	st := m.State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	// Slack's entry is changed by hand to that page's address.
	cfg.Tabs[0].Panes[0].Items[0] = config.Item{Web: "https://example.com", Name: "Example"}

	got := modelFromState(st, cfg, testNamer)
	if items := got.Tabs[0].ItemIDs(); len(items) != 2 {
		t.Errorf("Comms: %v, want Outlook and the one page", paneItems(got.Tabs[0]))
	}
	if it := got.Items[page.ID]; it == nil || it.Name != "Example" {
		t.Errorf("the page should take the entry: %+v", it)
	}
}

func TestModelFromStateReleasesWindowsAnAppEntrysNewTitleDoesntFit(t *testing.T) {
	m := defaultModel(t)
	m.Bind(m.Items["w6"], win(1, "com.brave.Browser", "Reddit", 1))
	cfg := config.Example()
	st := m.State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	// Research's Brave entry is given a title filter by hand.
	cfg.Tabs[2].Panes[0].Items[0].Title = "Docs"

	got := modelFromState(st, cfg, testNamer)
	mustValid(t, got)
	if _, ok := got.Items["w6"]; ok {
		t.Error("the Reddit window's item should go: the entry no longer claims it")
	}
	if got.ItemByWindow(1) != nil {
		t.Error("the Reddit window should be left for the startup scan")
	}
	p := got.Tabs[2].Leaves()[0]
	if len(p.Items) != 1 || got.Items[p.Items[0]].TitleFilter != "Docs" || got.Items[p.Items[0]].WindowID != 0 {
		t.Errorf("Research's first pane: %v, want a placeholder for Brave's Docs window", p.Items)
	}
}

// slackInTwoTabs returns the default model with Slack (w1) shown in Code
// too, and the config with Code's entries as the layout editor would save
// them, listing Slack as well.
func slackInTwoTabs(t *testing.T) (*Model, *config.Config) {
	t.Helper()
	m := defaultModel(t)
	if _, err := m.AddToTab("w1", 1, ""); err != nil {
		t.Fatal(err)
	}
	cfg := config.Example()
	cfg.Tabs[1].Panes = m.configPanes(m.Tabs[1], len(cfg.Tabs[1].Panes))
	if len(cfg.Validate()) > 0 {
		t.Fatalf("config: %v", cfg.Validate())
	}
	return m, cfg
}

func TestModelFromStateKeepsAnItemInTheTabsThatStillListIt(t *testing.T) {
	for _, current := range []int{0, 1} {
		m, cfg := slackInTwoTabs(t)
		m.Current = current
		st := m.State()
		for i := range st.Tabs {
			st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
		}
		// Slack's entry is removed from Comms by hand, but Code still
		// lists it.
		cfg.Tabs[0].Panes[0].Items = nil

		got := modelFromState(st, cfg, testNamer)
		mustValid(t, got)
		if got.Items["w1"] == nil {
			t.Fatalf("current tab %d: Slack's item is gone, but Code still lists it", current)
		}
		if tabs := got.TabsOf("w1"); !slices.Equal(tabs, []int{1}) {
			t.Errorf("current tab %d: Slack is in tabs %v, want only Code", current, tabs)
		}
		for _, tab := range got.Tabs {
			for _, id := range tab.ItemIDs() {
				if got.Items[id] == nil {
					t.Errorf("current tab %d: %s lists %s, which isn't an item", current, tab.Name, id)
				}
			}
		}
	}
}

func TestModelFromStateTakesAnEditInEitherTabOfAnItemInTwo(t *testing.T) {
	for _, edited := range []int{0, 1} {
		m, cfg := slackInTwoTabs(t)
		st := m.State()
		for i := range st.Tabs {
			st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
		}
		// Slack is renamed by hand in one of its tabs' entries.
		for _, cp := range cfg.Tabs[edited].Panes {
			for j, ci := range cp.Items {
				if ci.Web == "https://app.slack.com/client" {
					cp.Items[j].Name = "Team chat"
				}
			}
		}

		got := modelFromState(st, cfg, testNamer)
		if it := got.Items["w1"]; it.Name != "Team chat" || it.Title != "Team chat" {
			t.Errorf("renamed in %s: name %q title %q, want the new name", cfg.Tabs[edited].Name, it.Name, it.Title)
		}
	}
}
