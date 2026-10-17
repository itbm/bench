package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

func pane(id string, items ...string) *layout.Node {
	n := &layout.Node{Type: layout.TypePane, ID: id, Items: items}
	if len(items) > 0 {
		n.Active = items[0]
	}
	return n
}

func row(id string, sizes []float64, children ...*layout.Node) *layout.Node {
	return &layout.Node{Type: layout.TypeSplit, ID: id, Dir: layout.Row, Sizes: sizes, Children: children}
}

// goodState is a state that fits Default().
func goodState() *State {
	return &State{
		Version: StateVersion,
		Current: 1,
		Mode:    "wide",
		Seq:     12,
		Tabs: []TabState{
			{Name: "Comms", Preset: "side", Root: row("s1", []float64{45, 55}, pane("p1", "w1"), pane("p2", "w2")), Focus: "p1"},
			{Name: "Code", Preset: "custom", Root: row("s2", []float64{65, 35}, pane("p3", "w3", "w4"), pane("p4", "w5")), Focus: "p4", CompactShown: "w4"},
			{Name: "Research", Preset: "side", Root: pane("p5", "w6", "w7"), Focus: "p5"},
		},
		Items: map[string]ItemState{
			"w1": {Kind: "web", Title: "Slack", Created: 1, Name: "Slack", URL: "https://app.slack.com/client", Profile: "slack"},
			"w2": {Kind: "web", Title: "Outlook", Created: 2, Name: "Outlook", URL: "https://outlook.office.com/mail/", Profile: "outlook"},
			"w3": {Kind: "app", BundleID: "dev.zed.Zed", WindowID: 101, Title: "bench", Created: 3},
			"w4": {Kind: "app", BundleID: "com.microsoft.VSCode", WindowID: 102, Title: "ui", Created: 4},
			"w5": {Kind: "builtin", Title: "zsh", Created: 5, Folder: "~/code"},
			"w6": {Kind: "app", BundleID: "com.brave.Browser", WindowID: 103, Title: "Docs", Created: 6},
			"w7": {Kind: "app", BundleID: "md.obsidian", WindowID: 104, Title: "Notes", Created: 7},
			"w8": {Kind: "app", BundleID: "com.brave.Browser", WindowID: 105, Title: "Popped", Created: 8},
		},
		Floating: []string{"w8"},
	}
}

func writeState(t *testing.T, path string, s *State) {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}

func TestSaveAndLoadState(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), "sub", StateFile)
	want := goodState()
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path, Default())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadState = %+v\nwant %+v", got, want)
	}
	checkNoTempFiles(t, filepath.Dir(path))
}

func TestSaveStateWritesIndentedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), StateFile)
	if err := SaveState(path, goodState()); err != nil {
		t.Fatal(err)
	}
	data := readFile(t, path)
	if !strings.HasPrefix(data, "{\n  \"version\": 1,") || !strings.HasSuffix(data, "}\n") {
		t.Errorf("state.json starts %.40q and ends %q", data, data[len(data)-3:])
	}
	var s State
	if err := json.Unmarshal([]byte(data), &s); err != nil || !reflect.DeepEqual(&s, goodState()) {
		t.Errorf("state.json reads back as %+v, %v", s, err)
	}
	if err := SaveState(path, nil); err == nil {
		t.Error("SaveState(nil) didn't fail")
	}
}

func TestLoadStateWithoutAFile(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadState(filepath.Join(dir, StateFile), Default())
	if s != nil || err != nil {
		t.Errorf("LoadState = %v, %v; want nil, nil", s, err)
	}
	if _, err := os.Stat(filepath.Join(dir, BadState)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("LoadState made %s: %v", BadState, err)
	}
}

func TestLoadStateErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadState(dir, Default()); err == nil {
		t.Error("LoadState of a folder didn't fail")
	}
	path := filepath.Join(dir, StateFile)
	writeState(t, path, goodState())
	if _, err := LoadState(path, nil); err == nil {
		t.Error("LoadState without a config didn't fail")
	}
}

func TestLoadStateSetsAsideStateThatDoesntFit(t *testing.T) {
	tests := []struct {
		name string
		// Either data, or a change to goodState().
		data   string
		change func(s *State)
		// needsLayout is set when only layout.Validate can find the
		// problem.
		needsLayout bool
	}{
		{name: "not JSON", data: "{not json"},
		{name: "empty file", data: ""},
		{name: "wrong JSON shape", data: `{"version": 1, "tabs": {"name": "Comms"}}`},
		{name: "old version", change: func(s *State) { s.Version = 0 }},
		{name: "newer version", change: func(s *State) { s.Version = StateVersion + 1 }},
		{name: "fewer tabs", change: func(s *State) { s.Tabs = s.Tabs[:2] }},
		{name: "more tabs", change: func(s *State) { s.Tabs = append(s.Tabs, TabState{Name: "More", Root: pane("p9")}) }},
		{name: "renamed tab", change: func(s *State) { s.Tabs[2].Name = "Reading" }},
		{name: "tab name in other case", change: func(s *State) { s.Tabs[2].Name = "research" }},
		{name: "tabs out of order", change: func(s *State) { s.Tabs[0], s.Tabs[1] = s.Tabs[1], s.Tabs[0] }},
		{name: "no tree", change: func(s *State) { s.Tabs[1].Root = nil }},
		{name: "tree item missing from items", change: func(s *State) { delete(s.Items, "w4") }},
		{name: "no items at all", change: func(s *State) { s.Items = nil }},
		{name: "item in two tabs", change: func(s *State) { s.Tabs[2].Root.Items = append(s.Tabs[2].Root.Items, "w1") }},
		{name: "item twice in one tree", change: func(s *State) { s.Tabs[2].Root.Items = append(s.Tabs[2].Root.Items, "w6") }},
		{name: "floating item missing", change: func(s *State) { s.Floating = append(s.Floating, "w99") }},
		{name: "floating item in a tab", change: func(s *State) { s.Floating = append(s.Floating, "w3") }},
		{name: "unknown item kind", change: func(s *State) {
			it := s.Items["w5"]
			it.Kind = "terminal"
			s.Items["w5"] = it
		}},
		{name: "nil child", change: func(s *State) { s.Tabs[0].Root.Children[1] = nil }, needsLayout: true},
		{name: "split with one child", change: func(s *State) {
			s.Tabs[0].Root = row("s1", []float64{100}, pane("p1", "w1", "w2"))
		}, needsLayout: true},
		{name: "duplicate pane IDs", change: func(s *State) { s.Tabs[1].Root.Children[1].ID = "p3" }, needsLayout: true},
		{name: "active item not in pane", change: func(s *State) { s.Tabs[2].Root.Active = "w1" }, needsLayout: true},
		{name: "pane ID in two tabs", change: func(s *State) { s.Tabs[2].Root.ID = "p1" }, needsLayout: true},
		{name: "split ID in two tabs", change: func(s *State) { s.Tabs[1].Root.ID = "s1" }, needsLayout: true},
		{name: "pane and split with one ID in two tabs", change: func(s *State) { s.Tabs[2].Root.ID = "s2" }, needsLayout: true},
		{name: "pane called compact", change: func(s *State) { s.Tabs[2].Root.ID = layout.CompactPaneID }, needsLayout: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsLayout {
				requireLayout(t)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, StateFile)
			data := tc.data
			if tc.change != nil {
				s := goodState()
				tc.change(s)
				b, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				data = string(b)
			}
			writeFile(t, path, data)

			got, err := LoadState(path, Default())
			if got != nil || err != nil {
				t.Fatalf("LoadState = %+v, %v; want nil, nil", got, err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("state.json is still there: %v", err)
			}
			if bad := readFile(t, filepath.Join(dir, BadState)); bad != data {
				t.Errorf("state.json.bad = %q, want the old state.json", bad)
			}
		})
	}
}

func TestCheckStateReasons(t *testing.T) {
	if err := CheckState(nil, Default()); err == nil {
		t.Error("CheckState(nil, …) didn't fail")
	}
	s := goodState()
	s.Tabs[2].Name = "Reading"
	err := CheckState(s, Default())
	if err == nil || !strings.Contains(err.Error(), `tab 3 is "Reading", but it's "Research" in the config`) {
		t.Errorf("CheckState = %v", err)
	}
	s = goodState()
	delete(s.Items, "w4")
	if err := CheckState(s, Default()); err == nil || !strings.Contains(err.Error(), `item "w4", in tab "Code", isn't in items`) {
		t.Errorf("CheckState = %v", err)
	}
	requireLayout(t)
	s = goodState()
	s.Tabs[2].Root.ID = "p1"
	if err := CheckState(s, Default()); err == nil || !strings.Contains(err.Error(), `ID "p1" is in tab "Comms" and tab "Research"`) {
		t.Errorf("CheckState = %v", err)
	}
	s = goodState()
	s.Tabs[2].Root.ID = layout.CompactPaneID
	if err := CheckState(s, Default()); err == nil || !strings.Contains(err.Error(), `tab "Research" has a pane called "compact", which is reserved`) {
		t.Errorf("CheckState = %v", err)
	}
}

func TestLoadStateTidies(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), StateFile)
	s := goodState()
	s.Current = 7
	s.Mode = "huge"
	s.Tabs[0].Preset = "spiral"
	s.Tabs[0].Focus = "p9"
	s.Tabs[2].Focus = "compact"   // The tabs view's pane, which is kept.
	s.Tabs[1].CompactShown = "w1" // In another tab.
	s.Items["w99"] = ItemState{Kind: "app", Title: "Orphan"}
	s.Seq = 3 // Lower than IDs in use.
	writeState(t, path, s)

	got, err := LoadState(path, Default())
	if err != nil || got == nil {
		t.Fatalf("LoadState = %v, %v", got, err)
	}
	want := goodState()
	want.Current = 0
	want.Mode = "auto"
	want.Tabs[0].Preset = "custom"
	want.Tabs[0].Focus = "p1"
	want.Tabs[1].CompactShown = ""
	want.Tabs[2].Focus = "compact"
	want.Seq = 8 // From w8, the highest ID left once w99 is dropped.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadState = %+v\nwant %+v", got, want)
	}
}

func TestLoadStateRaisesSeq(t *testing.T) {
	requireLayout(t)
	tests := []struct {
		name   string
		change func(s *State)
		want   int
	}{
		{"already high enough", func(s *State) {}, 12},
		{"from an item", func(s *State) { s.Seq = 0 }, 8},
		{"from a pane", func(s *State) {
			s.Seq = 0
			s.Tabs[2].Root.ID = "p40"
		}, 40},
		{"from a split", func(s *State) {
			s.Seq = 1
			s.Tabs[1].Root.ID = "s31"
		}, 31},
		{"from a floating item", func(s *State) {
			s.Seq = 1
			s.Items["w50"] = s.Items["w8"]
			delete(s.Items, "w8")
			s.Floating = []string{"w50"}
		}, 50},
		{"IDs without numbers", func(s *State) {
			s.Seq = 2
			s.Tabs = []TabState{
				{Name: "Comms", Root: pane("a")},
				{Name: "Code", Root: pane("b")},
				{Name: "Research", Root: pane("c", "item")},
			}
			s.Items = map[string]ItemState{"item": {Kind: "app"}}
			s.Floating = nil
		}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), StateFile)
			s := goodState()
			tc.change(s)
			writeState(t, path, s)
			got, err := LoadState(path, Default())
			if err != nil || got == nil {
				t.Fatalf("LoadState = %v, %v", got, err)
			}
			if got.Seq != tc.want {
				t.Errorf("Seq = %d, want %d", got.Seq, tc.want)
			}
		})
	}
}

func TestIDNumber(t *testing.T) {
	tests := []struct {
		id   string
		want int
		ok   bool
	}{
		{"w12", 12, true},
		{"p1", 1, true},
		{"s007", 7, true},
		{"42", 42, true},
		{"compact", 0, false},
		{"", 0, false},
		{"w99999999999999999999", 0, false},
	}
	for _, tc := range tests {
		if got, ok := idNumber(tc.id); got != tc.want || ok != tc.ok {
			t.Errorf("idNumber(%q) = %d, %v; want %d, %v", tc.id, got, ok, tc.want, tc.ok)
		}
	}
}
