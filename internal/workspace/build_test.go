package workspace

import (
	"errors"
	"reflect"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestStateRoundTrip(t *testing.T) {
	cfg := config.Default()
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
	if err := config.CheckState(st, cfg); err != nil {
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
		{"http://example.com", "", errNotHTTPS},
		{"ftp://example.com", "", errNotHTTPS},
		{"https://", "", errNotHTTPS},
		{"", "", errNoAddress},
	}
	for _, tc := range tests {
		got, err := normaliseAddress(tc.in)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("%q: got %q %v, want %q %v", tc.in, got, err, tc.want, tc.err)
		}
	}
	if errNotHTTPS.Error() != "That address isn't a secure web address (https)." {
		t.Errorf("message: %q", errNotHTTPS)
	}
}

func TestResetTab(t *testing.T) {
	m := defaultModel(t)
	cfg := config.Default()
	// Make Code custom: VS Code beside the terminal, Zed split off.
	if _, err := m.Move("w4", 1, "p12", layout.Center); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Move("w3", 1, "p12", layout.Top); err != nil {
		t.Fatal(err)
	}
	extra := addAppItem(m, m.Tabs[1], m.Tabs[1].Leaves()[0], "com.apple.Safari", win(5, "com.apple.Safari", "Apple", 1))
	if err := m.ResetTab(1, cfg.Tabs[1]); err != nil {
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
