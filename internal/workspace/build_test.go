package workspace

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestStateRoundTrip(t *testing.T) {
	cfg := config.Example()
	m := defaultModel(t)
	m.Bind(m.Items["w3"], win(42, "dev.zed.Zed", "bench — switch.go", 1))
	m.Items["w1"].Title = "# general"
	if _, err := m.Move("w4", 1, "p12", layout.Right); err != nil {
		t.Fatal(err)
	}
	m.Current, m.Mode = 1, layout.ModeCompact
	m.Floating = append(m.Floating, "w99")
	m.Items["w99"] = &Item{ID: "w99", Kind: KindWeb, URL: "https://example.com", Profile: "example.com"}

	st := m.State()
	if err := config.CheckState(st); err != nil {
		t.Fatalf("CheckState: %v", err)
	}
	back := modelFromState(st, cfg, testNamer)
	mustValid(t, back)
	for i := range m.Tabs {
		if !reflect.DeepEqual(back.Tabs[i].Root, m.Tabs[i].Root) || back.Tabs[i].Preset != m.Tabs[i].Preset ||
			back.Tabs[i].SmallScreens != m.Tabs[i].SmallScreens {
			t.Errorf("tab %d differs", i)
		}
	}
	if back.Current != 1 || back.Mode != layout.ModeCompact || back.Seq != m.Seq {
		t.Errorf("current %d mode %s seq %d", back.Current, back.Mode, back.Seq)
	}
	zed := back.Items["w3"]
	if zed.WindowID != 42 || zed.Title != "bench — switch.go" || zed.Created != 1 || back.ItemByWindow(42) != zed {
		t.Errorf("zed: %+v", zed)
	}
	if back.Items["w1"].Title != "# general" || back.Items["w1"].App != "Slack" {
		t.Errorf("slack: %+v", back.Items["w1"])
	}
	if !back.IsFloating("w99") || back.Items["w99"].App != "Example" {
		t.Errorf("floating: %+v", back.Items["w99"])
	}
	// The creation counter carries on from the state.
	back.Bind(back.Items["w4"], win(43, "com.microsoft.VSCode", "x", 2))
	if back.Items["w4"].Created != 2 {
		t.Errorf("created: got %d, want 2", back.Items["w4"].Created)
	}
}

func TestConfigPanes(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "T"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }, { app = "com.brave.Browser", title = "GitHub", url = "https://github.com" }, { web = "https://app.slack.com/client", profile = "slack", name = "Slack" }]
  [[tab.pane]]
  items = [{ terminal = "~/code", command = "htop" }, { vscode = "~/code/bench" }]
`)
	tab := m.Tabs[0]
	// Three more Brave windows in the first pane: still one entry for them.
	for i := 0; i < 3; i++ {
		addAppItem(m, tab, tab.Leaves()[0], "com.brave.Browser", win(uint32ID(i), "com.brave.Browser", "x", i+1))
	}
	got := m.configPanes(tab, 2)
	want := []config.Pane{
		{Items: []config.Item{
			{App: "com.brave.Browser"},
			{App: "com.brave.Browser", Title: "GitHub", URL: "https://github.com"},
			{Web: "https://app.slack.com/client", Profile: "slack", Name: "Slack"},
		}},
		{Items: []config.Item{{Terminal: "~/code", Command: "htop"}, {VSCode: "~/code/bench"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	// With one slot, the second pane's items join the first.
	if got := m.configPanes(tab, 1); len(got) != 1 || len(got[0].Items) != 5 {
		t.Errorf("one slot: %+v", got)
	}
	// Trailing empty panes are left out.
	empty := &Tab{Root: layout.NewSplit("s", layout.Row, nil, layout.NewPane("a", "w1"), layout.NewPane("b"))}
	if got := m.configPanes(empty, 2); len(got) != 1 {
		t.Errorf("trailing empty: %+v", got)
	}
}

func uint32ID(i int) platform.WindowID { return platform.WindowID(100 + i) }

func TestConfigItem(t *testing.T) {
	tests := []struct {
		name string
		it   Item
		want config.Item
		ok   bool
	}{
		{"web keeps a profile that isn't the host", Item{Kind: KindWeb, URL: "https://a.com/x", Profile: "p"},
			config.Item{Web: "https://a.com/x", Profile: "p"}, true},
		{"web drops the host profile", Item{Kind: KindWeb, URL: "https://a.com/x", Profile: "a.com"},
			config.Item{Web: "https://a.com/x"}, true},
		{"web falls back to its first address", Item{Kind: KindWeb, URL: "about:blank", Home: "https://a.com"},
			config.Item{Web: "https://a.com"}, true},
		{"web keeps the address it was given, not the page it's on", Item{Kind: KindWeb, URL: "https://app.slack.com/client/T1/C2", Home: "https://app.slack.com/client", Profile: "slack"},
			config.Item{Web: "https://app.slack.com/client", Profile: "slack"}, true},
		{"web uses the page it's on if it was given no secure address", Item{Kind: KindWeb, URL: "https://a.com/x", Home: "http://a.com"},
			config.Item{Web: "https://a.com/x"}, true},
		{"web with no secure address", Item{Kind: KindWeb, URL: "http://a.com", Home: "http://a.com"},
			config.Item{}, false},
		{"url only with a title", Item{Kind: KindApp, BundleID: "x", OpenURL: "https://x"},
			config.Item{App: "x"}, true},
		{"terminal folder defaults", Item{Kind: KindBuiltin}, config.Item{Terminal: "~"}, true},
	}
	for _, tc := range tests {
		got, _, ok := configItem(&tc.it)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got %+v %v, want %+v %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormaliseAddress(t *testing.T) {
	tests := []struct {
		in, want string
		err      error
	}{
		{"example.com", "https://example.com", nil},
		{"  https://example.com/a  ", "https://example.com/a", nil},
		{"HTTPS://Example.com", "HTTPS://Example.com", nil},
		{"github.com/login?return_to=https://github.com/org", "https://github.com/login?return_to=https://github.com/org", nil},
		// A host on this Mac may be http, which it is if typed without a
		// scheme, as browsers do.
		{"localhost:3000", "http://localhost:3000", nil},
		{"127.0.0.1:8000/docs", "http://127.0.0.1:8000/docs", nil},
		{"[::1]:8080", "http://[::1]:8080", nil},
		{"app.localhost", "https://app.localhost", nil},
		{"http://localhost:3000/a", "http://localhost:3000/a", nil},
		{"https://localhost:8443", "https://localhost:8443", nil},
		{"localhost.example.com", "https://localhost.example.com", nil},
		{"http://example.com", "", errInsecure},
		{"http://localhost.example.com", "", errInsecure},
		{"http://app.localhost:5173", "", errInsecure},
		{"ftp://example.com", "", errNotWebAddress},
		{"https://", "", errNotWebAddress},
		{"not a url at all", "", errNotWebAddress},
		{"", "", errNoAddress},
	}
	for _, tc := range tests {
		got, err := normaliseAddress(tc.in)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("%q: got %q %v, want %q %v", tc.in, got, err, tc.want, tc.err)
		}
	}
	if _, err := normaliseAddress("http://example.com:8080/a"); err == nil || err.Error() != "Bench shows http pages only from this Mac, such as localhost. Try https://example.com:8080." {
		t.Errorf("http message: %v", err)
	}
	if errNotWebAddress.Error() != "That isn't a web address. Try example.com." {
		t.Errorf("message: %q", errNotWebAddress)
	}
}

// TestSecureAddress checks which addresses a Web item can keep in the
// config: https, or http on this Mac, with a scheme.
func TestSecureAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"https://app.slack.com": true, "http://localhost:3000": true, "http://127.0.0.1:8000": true,
		"http://example.com": false, "localhost:3000": false, "example.com": false, "ftp://example.com": false, "": false,
	} {
		if got := secureAddress(addr); got != want {
			t.Errorf("secureAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestResetTab(t *testing.T) {
	m := defaultModel(t)
	cfg := config.Example()
	// Make Code custom: VS Code beside the terminal, Zed split off.
	if _, err := m.Move("w4", 1, "p12", layout.Center); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Move("w3", 1, "p12", layout.Top); err != nil {
		t.Fatal(err)
	}
	extra := addAppItem(m, m.Tabs[1], m.Tabs[1].Leaves()[0], "com.apple.Safari", win(5, "com.apple.Safari", "Apple", 1))
	if err := m.ResetTab(1, cfg.Tabs[1], testNamer); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	code := m.Tabs[1]
	if code.Preset != "main-side" || len(code.Leaves()) != 2 {
		t.Fatalf("preset %s, %d panes", code.Preset, len(code.Leaves()))
	}
	if !reflect.DeepEqual(code.Root.Sizes, []float64{65, 35}) {
		t.Errorf("sizes: %v", code.Root.Sizes)
	}
	first, second := code.Leaves()[0].Items, code.Leaves()[1].Items
	in := func(list []string, id string) bool {
		for _, x := range list {
			if x == id {
				return true
			}
		}
		return false
	}
	if !in(first, "w3") || !in(first, "w4") || !in(second, "w5") {
		t.Errorf("claimed items: %v %v", first, second)
	}
	if !in(first, extra.ID) && !in(second, extra.ID) {
		t.Errorf("unclaimed item lost")
	}
}

func TestSmallScreensDefaultsToStack(t *testing.T) {
	if smallScreensOf("") != layout.Stack || smallScreensOf("tabs") != layout.Tabs {
		t.Error("smallScreensOf")
	}
}

func TestResetTabAddsMissingEntries(t *testing.T) {
	m := defaultModel(t)
	cfg := config.Example()
	// Slack's window was closed, and Code's terminal ended and the Zed and
	// VS Code items went: Reset layout brings back the config's items.
	m.Remove("w1")
	for _, id := range []string{"w3", "w4", "w5"} {
		m.Remove(id)
	}
	if err := m.ResetTab(0, cfg.Tabs[0], testNamer); err != nil {
		t.Fatal(err)
	}
	if err := m.ResetTab(1, cfg.Tabs[1], testNamer); err != nil {
		t.Fatal(err)
	}
	mustValid(t, m)
	comms := m.Tabs[0].Leaves()
	if len(comms[0].Items) != 1 || len(comms[1].Items) != 1 || comms[1].Items[0] != "w2" {
		t.Fatalf("Comms: %v", paneItems(m.Tabs[0]))
	}
	slack := m.Items[comms[0].Items[0]]
	if slack.Kind != KindWeb || slack.Home != "https://app.slack.com/client" || slack.Profile != "slack" || slack.Name != "Slack" {
		t.Errorf("Slack again: %+v", slack)
	}
	code := m.Tabs[1].Leaves()
	var kinds []string
	for _, p := range code {
		for _, id := range p.Items {
			it := m.Items[id]
			kinds = append(kinds, it.Kind+":"+it.BundleID+it.Folder)
		}
	}
	want := []string{"app:" + "dev.zed.Zed", "app:com.microsoft.VSCode", "builtin:~/code"}
	if !reflect.DeepEqual(kinds, want) || len(code[0].Items) != 2 {
		t.Errorf("Code: %v %v, want %v", kinds, paneItems(m.Tabs[1]), want)
	}
	if it := m.Items[code[0].Items[0]]; it.WindowID != 0 || it.Status != StatusMissing {
		t.Errorf("App entries come back as placeholders: %+v", it)
	}

	// Resetting again adds nothing: every entry has its item.
	before := len(m.Items)
	if err := m.ResetTab(1, cfg.Tabs[1], testNamer); err != nil {
		t.Fatal(err)
	}
	if len(m.Items) != before {
		t.Errorf("a second reset added items: %d, want %d", len(m.Items), before)
	}
}

func TestModelFromStateAddsEntriesAddedByHand(t *testing.T) {
	cfg := config.Example()
	m := defaultModel(t)
	// Slack was closed in Bench, so it's gone from the tab but still in the
	// config.
	m.Remove("w1")
	st := m.State()
	for i := range st.Tabs {
		st.Tabs[i].Entries = entryKeys(cfg.Tabs[i])
	}
	// While Bench isn't running, a web page and a second terminal are added
	// to config.toml.
	cfg.Tabs[0].Panes[1].Items = append(cfg.Tabs[0].Panes[1].Items, config.Item{Web: "https://example.com", Name: "Example"})
	cfg.Tabs[1].Panes[1].Items = append(cfg.Tabs[1].Panes[1].Items, config.Item{Terminal: "~/code"})

	got := modelFromState(st, cfg, testNamer)
	mustValid(t, got)
	// Closing Slack removed its pane, so Outlook's is the only one.
	comms := got.Tabs[0].Leaves()
	if len(comms) != 1 || len(comms[0].Items) != 2 || comms[0].Items[0] != "w2" {
		t.Fatalf("Comms: %v, want Outlook and the new page, without Slack", paneItems(got.Tabs[0]))
	}
	if it := got.Items[comms[0].Items[1]]; it.Home != "https://example.com" || it.Name != "Example" {
		t.Errorf("the new web page: %+v", it)
	}
	if terms := got.Tabs[1].Leaves()[1].Items; len(terms) != 2 || got.Items[terms[1]].Folder != "~/code" {
		t.Errorf("the second terminal should be added: %v", paneItems(got.Tabs[1]))
	}

	// A state.json from before entries were saved adds nothing.
	for i := range st.Tabs {
		st.Tabs[i].Entries = nil
	}
	if got := modelFromState(st, cfg, testNamer); len(got.Items) != len(st.Items) {
		t.Errorf("an old state.json gained items: %d, want %d", len(got.Items), len(st.Items))
	}
}

func TestStateKeepsWebHomeAndOwnWindow(t *testing.T) {
	m := defaultModel(t)
	slack := m.Items["w1"]
	slack.URL = "https://app.slack.com/client/T1/C2"
	m.Take("w2")
	m.Floating = append(m.Floating, "w2")
	m.Items["w2"].OwnWindow = true
	got := modelFromState(m.State(), config.Example(), testNamer)
	if it := got.Items["w1"]; it.Home != "https://app.slack.com/client" || it.URL != slack.URL || it.App != "Slack" {
		t.Errorf("Slack after a restart: home %q url %q app %q", it.Home, it.URL, it.App)
	}
	if !got.Items["w2"].OwnWindow {
		t.Error("a popped-out web page should remember its own window")
	}
	// A state.json from before Home was saved uses the URL.
	st := m.State()
	is := st.Items["w1"]
	is.Home = ""
	st.Items["w1"] = is
	if it := modelFromState(st, config.Example(), testNamer).Items["w1"]; it.Home != slack.URL {
		t.Errorf("home from an old file: %q", it.Home)
	}
}

func TestStateDropsAVSCodeAddress(t *testing.T) {
	m, cfg := modelOf(t, `
[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ vscode = "~/code/bench" }]
`)
	// An older Bench kept the address the item loaded, with the server's
	// connection token.
	st := m.State()
	is := st.Items["w1"]
	is.URL = "http://127.0.0.1:9999/?tkn=secret&folder=%2Fcode"
	st.Items["w1"] = is
	got := modelFromState(st, cfg, testNamer)
	if it := got.Items["w1"]; it.URL != "" || it.Home != "" || it.VSCode != "~/code/bench" {
		t.Errorf("VS Code after a restart: url %q home %q folder %q", it.URL, it.Home, it.VSCode)
	}
	if is := got.State().Items["w1"]; is.URL != "" || is.Home != "" {
		t.Errorf("the token would be saved again: url %q home %q", is.URL, is.Home)
	}
}

// TestModelFromStateMatchesTabsByName checks that the model keeps the
// arrangement of each tab that the config still has, in the config's order
// and with its names; builds a new tab from the config, with IDs after the
// state's; and drops a tab the config no longer has, with the items that no
// other tab has.
func TestModelFromStateMatchesTabsByName(t *testing.T) {
	m := defaultModel(t)
	// Zed is in Research as well as Code.
	if _, err := m.AddToTab("w3", 2, "p14"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Move("w4", 1, "p12", layout.Right); err != nil {
		t.Fatal(err)
	}
	m.Current = 1
	st := m.State()
	cfg := config.Example()
	code, research := cfg.Tabs[1], cfg.Tabs[2]
	research.Name = "research"
	notes := config.Tab{Name: "Notes", Layout: "single", Panes: []config.Pane{{Items: []config.Item{{Terminal: "~/notes"}}}}}
	cfg.Tabs = []config.Tab{research, notes, code} // Comms goes.

	got := modelFromState(st, cfg, testNamer)
	mustValid(t, got)
	var names []string
	for _, tab := range got.Tabs {
		names = append(names, tab.Name)
	}
	if !slices.Equal(names, []string{"research", "Notes", "Code"}) || got.Current != 2 {
		t.Fatalf("tabs %v, current %d", names, got.Current)
	}
	if !equalPanes(paneItems(got.Tabs[0]), paneItems(m.Tabs[2])) || !equalPanes(paneItems(got.Tabs[2]), paneItems(m.Tabs[1])) {
		t.Errorf("research %v, Code %v; want their arrangement", paneItems(got.Tabs[0]), paneItems(got.Tabs[2]))
	}
	if items := got.Tabs[1].ItemIDs(); len(items) != 1 || got.Items[items[0]].Folder != "~/notes" || idNumber(t, items[0]) <= st.Seq {
		t.Errorf("Notes: %v, want a terminal with an ID after %d", items, st.Seq)
	}
	// Comms's pages go; Zed stays, in Code and research.
	if got.Items["w1"] != nil || got.Items["w2"] != nil || got.Items["w3"] == nil {
		t.Errorf("items %v", slices.Sorted(maps.Keys(got.Items)))
	}
}
