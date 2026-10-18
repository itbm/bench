package workspace

import (
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// claimsConfig lists Brave three times: with a title filter in Code, and
// without one in Research and Notes.
const claimsConfig = `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "GitHub", url = "https://github.com" }]

[[tab]]
name = "Research"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]

[[tab]]
name = "Notes"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]

[[rule]]
app = "com.brave.Browser"
title = "Picture in picture"
float = true

[[rule]]
app = "com.apple.Safari"
tab = "Notes"
`

func TestClaim(t *testing.T) {
	m, cfg := modelOf(t, claimsConfig)
	// Items: w1 Zed, w2 Brave (GitHub), w3 Brave, w4 Obsidian, w5 Brave.
	tests := []struct {
		name     string
		bundleID string
		title    string
		tab      int
		pane     string
		ok       bool
	}{
		{"title filter wins", "com.brave.Browser", "PR #42 · GitHub", 0, layout.Leaves(m.Tabs[0].Root)[1].ID, true},
		{"filter ignores case", "com.brave.Browser", "github issues", 0, layout.Leaves(m.Tabs[0].Root)[1].ID, true},
		{"first pane without a title", "com.brave.Browser", "Go packages", 1, layout.Leaves(m.Tabs[1].Root)[0].ID, true},
		{"bundle IDs ignore case", "COM.BRAVE.BROWSER", "Docs", 1, layout.Leaves(m.Tabs[1].Root)[0].ID, true},
		{"single entry", "dev.zed.Zed", "main.go", 0, layout.Leaves(m.Tabs[0].Root)[0].ID, true},
		{"unclaimed", "com.apple.finder", "Downloads", -1, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ti, p, ok := m.Claim(cfg, tc.bundleID, tc.title)
			if ok != tc.ok || ti != tc.tab || (p != nil && p.ID != tc.pane) {
				id := ""
				if p != nil {
					id = p.ID
				}
				t.Errorf("got %d %s %v, want %d %s %v", ti, id, ok, tc.tab, tc.pane, tc.ok)
			}
		})
	}
}

func TestClaimFallsBackToConfig(t *testing.T) {
	m, cfg := modelOf(t, claimsConfig)
	// Every Brave item has gone: the config still says where Brave goes.
	for _, id := range []string{"w2", "w3", "w5"} {
		m.Remove(id)
	}
	ti, p, ok := m.Claim(cfg, "com.brave.Browser", "Docs")
	if !ok || ti != 1 || p != m.Tabs[1].Leaves()[0] {
		t.Errorf("unfiltered: got %d %v %v", ti, p, ok)
	}
	ti, p, ok = m.FilterClaim(cfg, "com.brave.Browser", "GitHub home")
	if !ok || ti != 0 || p != m.Tabs[0].Leaves()[len(m.Tabs[0].Leaves())-1] {
		t.Errorf("filtered: got %d %v %v", ti, p, ok)
	}
	if _, _, ok := m.FilterClaim(cfg, "com.brave.Browser", "Docs"); ok {
		t.Errorf("FilterClaim should ignore entries without a title")
	}
}

func TestClaimFollowsMovedItems(t *testing.T) {
	m, cfg := modelOf(t, claimsConfig)
	// Move Research's Brave item into Obsidian's pane: claims follow it.
	obsidian := m.Tabs[1].Leaves()[1].ID
	if _, err := m.Move("w3", 1, obsidian, layout.Center); err != nil {
		t.Fatal(err)
	}
	ti, p, ok := m.Claim(cfg, "com.brave.Browser", "Docs")
	if !ok || ti != 1 || p.ID != obsidian {
		t.Errorf("got %d %v %v, want Research's Obsidian pane", ti, p, ok)
	}
}

func TestPlaceholder(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "T"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }, { app = "com.brave.Browser", title = "GitHub" }]
`)
	p := m.Tabs[0].Root
	if got := m.Placeholder(p, "com.brave.Browser", "GitHub · PR"); got == nil || got.ID != "w2" {
		t.Errorf("filtered: got %v, want w2", got)
	}
	if got := m.Placeholder(p, "com.brave.Browser", "Docs"); got == nil || got.ID != "w1" {
		t.Errorf("unfiltered: got %v, want w1", got)
	}
	m.Bind(m.Items["w1"], win(1, "com.brave.Browser", "Docs", 1))
	if got := m.Placeholder(p, "com.brave.Browser", "News"); got != nil {
		t.Errorf("a filter that doesn't fit: got %v, want none", got.ID)
	}
	if got := m.Placeholder(p, "dev.zed.Zed", "x"); got != nil {
		t.Errorf("another app: got %v", got.ID)
	}
}

func TestAfterApp(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "T"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }, { terminal = "~" }, { app = "com.brave.Browser" }, { terminal = "~" }]
`)
	p := m.Tabs[0].Root
	if got := m.afterApp(p, "com.brave.Browser"); got != 3 {
		t.Errorf("after Brave: got %d, want 3", got)
	}
	if got := m.afterApp(p, "dev.zed.Zed"); got != -1 {
		t.Errorf("no Zed: got %d, want -1", got)
	}
}

func TestRecentPane(t *testing.T) {
	m, _ := modelOf(t, claimsConfig)
	research := m.Tabs[1]
	left, right := research.Leaves()[0], research.Leaves()[1]
	if got := m.RecentPane(1, "com.brave.Browser"); got != nil {
		t.Errorf("no windows yet: got %s", got.ID)
	}
	a := addAppItem(m, research, left, "com.brave.Browser", win(1, "com.brave.Browser", "A", 1))
	b := addAppItem(m, research, right, "com.brave.Browser", win(2, "com.brave.Browser", "B", 2))
	if got := m.RecentPane(1, "com.brave.Browser"); got != left {
		t.Errorf("none used: got %s, want the first", got.ID)
	}
	m.Use(b.ID)
	if got := m.RecentPane(1, "com.brave.Browser"); got != right {
		t.Errorf("b used: got %s", got.ID)
	}
	m.Use(a.ID)
	if got := m.RecentPane(1, "com.brave.Browser"); got != left {
		t.Errorf("a used last: got %s", got.ID)
	}
	notes := m.Tabs[2]
	c := addAppItem(m, notes, notes.Root, "com.brave.Browser", win(3, "com.brave.Browser", "C", 3))
	m.Use(c.ID)
	if ti, p := m.RecentPaneElsewhere(0, "com.brave.Browser"); ti != 2 || p != notes.Root {
		t.Errorf("elsewhere: got %d %v", ti, p)
	}
	if ti, p := m.RecentPaneElsewhere(2, "com.brave.Browser"); ti != 1 || p != left {
		t.Errorf("elsewhere but Notes: got %d %v", ti, p)
	}
}

func TestMatchRule(t *testing.T) {
	cfg := parseConfig(t, claimsConfig)
	tests := []struct {
		bundleID, title string
		float           bool
		tab             string
	}{
		{"com.brave.Browser", "Picture in picture", true, ""},
		{"com.brave.Browser", "picture IN picture", true, ""},
		{"com.brave.Browser", "Docs", false, ""},
		{"com.apple.Safari", "Apple", false, "Notes"},
		{"dev.zed.Zed", "x", false, ""},
	}
	for _, tc := range tests {
		float, tab := matchRule(cfg.Rules, tc.bundleID, tc.title)
		if float != tc.float || tab != tc.tab {
			t.Errorf("%s %q: got %v %q, want %v %q", tc.bundleID, tc.title, float, tab, tc.float, tc.tab)
		}
	}
	if float, _ := matchRule([]config.Rule{{App: "x", Tab: "T"}, {App: "x", Float: true}}, "x", ""); !float {
		t.Errorf("a float rule should win over an earlier tab rule")
	}
}

func TestMatchRememberedByTitleThenCreation(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "Research"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
`)
	tab := m.Tabs[0]
	p := tab.Root
	brave := "com.brave.Browser"
	// Three Brave windows, then Brave quits.
	m.Bind(m.Items["w1"], win(10, brave, "Wails v3 docs", 1))
	b := addAppItem(m, tab, p, brave, win(11, brave, "Pull request #42", 2))
	c := addAppItem(m, tab, p, brave, win(12, brave, "Go packages", 3))
	for _, it := range []*Item{m.Items["w1"], b, c} {
		m.Unbind(it)
	}
	// Brave opens again: one window keeps its title, two have new ones.
	ws := []platform.Window{
		win(20, brave, "New Tab", 1),
		win(21, brave, "Go packages", 2),
		win(22, brave, "New Tab", 3),
		win(23, brave, "Extra", 4),
	}
	bound, rest := m.MatchRemembered(ws)
	if len(bound) != 3 || len(rest) != 1 || rest[0].ID != 23 {
		t.Fatalf("bound %d, rest %v", len(bound), rest)
	}
	want := map[string]platform.WindowID{"w1": 20, b.ID: 22, c.ID: 21}
	for id, wid := range want {
		if got := m.Items[id].WindowID; got != wid {
			t.Errorf("%s: got window %d, want %d", id, got, wid)
		}
	}
	if !equalPanes(paneItems(tab), [][]string{{"w1", b.ID, c.ID}}) {
		t.Errorf("order changed: %v", paneItems(tab))
	}
}

func TestMatchRememberedSkipsPlaceholdersAndFilters(t *testing.T) {
	m, _ := modelOf(t, `
[[tab]]
name = "T"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }, { app = "com.brave.Browser", title = "GitHub" }]
`)
	brave := "com.brave.Browser"
	// A placeholder that never had a window isn't matched by creation order.
	if bound, rest := m.MatchRemembered([]platform.Window{win(1, brave, "Docs", 1)}); len(bound) != 0 || len(rest) != 1 {
		t.Fatalf("placeholder matched: %d bound", len(bound))
	}
	// A remembered item with a filter only takes windows that fit it.
	gh := m.Items["w2"]
	m.Bind(gh, win(5, brave, "GitHub · home", 1))
	m.Unbind(gh)
	if bound, _ := m.MatchRemembered([]platform.Window{win(6, brave, "Docs", 2)}); len(bound) != 0 {
		t.Errorf("filtered item took a window that doesn't fit")
	}
	if bound, _ := m.MatchRemembered([]platform.Window{win(7, brave, "GitHub · PR", 3)}); len(bound) != 1 || bound[0] != gh {
		t.Errorf("filtered item didn't take a window that fits")
	}
}
