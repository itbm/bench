package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

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

// goodState is a state that fits Example().
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
		Screen:   &ScreenState{ID: "2", Name: "Studio Display", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}},
		Dialogs:  map[string]DialogSize{"settings": {W: 640, H: 520}},
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
	got, aside, err := LoadState(path, Example())
	if err != nil || aside != nil {
		t.Fatal(aside, err)
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
	s, aside, err := LoadState(filepath.Join(dir, StateFile), Example())
	if s != nil || aside != nil || err != nil {
		t.Errorf("LoadState = %v, %v, %v; want nothing", s, aside, err)
	}
	if _, err := os.Stat(filepath.Join(dir, BadState)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("LoadState made %s: %v", BadState, err)
	}
}

func TestLoadStateErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LoadState(dir, Example()); err == nil {
		t.Error("LoadState of a folder didn't fail")
	}
	path := filepath.Join(dir, StateFile)
	writeState(t, path, goodState())
	if _, _, err := LoadState(path, nil); err == nil {
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
		{name: "no tree", change: func(s *State) { s.Tabs[1].Root = nil }},
		{name: "tree item missing from items", change: func(s *State) { delete(s.Items, "w4") }},
		{name: "no items at all", change: func(s *State) { s.Items = nil }},
		{name: "item twice in one tree", change: func(s *State) { s.Tabs[2].Root.Items = append(s.Tabs[2].Root.Items, "w6") }},
		{name: "floating item missing", change: func(s *State) { s.Floating = append(s.Floating, "w99") }},
		{name: "floating item in a tab", change: func(s *State) { s.Floating = append(s.Floating, "w3") }},
		{name: "floating item in two tabs", change: func(s *State) {
			s.Tabs[2].Root.Items = append(s.Tabs[2].Root.Items, "w3")
			s.Floating = append(s.Floating, "w3")
		}},
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

			got, aside, err := LoadState(path, Example())
			if got != nil || err != nil {
				t.Fatalf("LoadState = %+v, %v; want no state", got, err)
			}
			// It names the tabs of a file that parses.
			var want []string
			if tc.change != nil {
				want = []string{"Comms", "Code", "Research"}
			}
			if aside == nil || aside.Reason == nil || !reflect.DeepEqual(aside.Tabs, want) {
				t.Errorf("set aside %+v, want the reason and tabs %v", aside, want)
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

func TestLoadStateKeepsAnItemInSeveralTabs(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), StateFile)
	s := goodState()
	s.Tabs[2].Root.Items = append(s.Tabs[2].Root.Items, "w3")
	s.Tabs[0].Root.Children[1].Items = append(s.Tabs[0].Root.Children[1].Items, "w3")
	writeState(t, path, s)

	got, _, err := LoadState(path, Example())
	if err != nil || got == nil {
		t.Fatalf("LoadState = %v, %v", got, err)
	}
	if !reflect.DeepEqual(got.Tabs, s.Tabs) || got.Items["w3"] != s.Items["w3"] {
		t.Errorf("LoadState changed the state: %+v", got)
	}
}

func TestCheckStateReasons(t *testing.T) {
	if err := CheckState(nil); err == nil {
		t.Error("CheckState(nil) didn't fail")
	}
	s := goodState()
	s.Version = 2
	if err := CheckState(s); err == nil || err.Error() != "it's version 2, not 1" {
		t.Errorf("CheckState = %v", err)
	}
	// A config's tabs don't matter.
	s = goodState()
	s.Tabs[2].Name = "Reading"
	if err := CheckState(s); err != nil {
		t.Errorf("CheckState of a renamed tab = %v", err)
	}
	s = goodState()
	delete(s.Items, "w4")
	if err := CheckState(s); err == nil || !strings.Contains(err.Error(), `item "w4", in tab "Code", isn't in items`) {
		t.Errorf("CheckState = %v", err)
	}
	requireLayout(t)
	s = goodState()
	s.Tabs[2].Root.ID = "p1"
	if err := CheckState(s); err == nil || !strings.Contains(err.Error(), `ID "p1" is in tab "Comms" and tab "Research"`) {
		t.Errorf("CheckState = %v", err)
	}
	s = goodState()
	s.Tabs[2].Root.ID = layout.CompactPaneID
	if err := CheckState(s); err == nil || !strings.Contains(err.Error(), `tab "Research" has a pane called "compact", which is reserved`) {
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
	// A pane that shows nothing shows its first item, w3.
	s.Tabs[1].Root.Children[0].Active = ""
	s.Items["w99"] = ItemState{Kind: "app", Title: "Orphan"}
	s.Seq = 3 // Lower than IDs in use.
	s.Dialogs["palette"] = DialogSize{W: 0, H: 300}
	s.Dialogs["guide"] = DialogSize{W: 500, H: -1}
	writeState(t, path, s)

	got, _, err := LoadState(path, Example())
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
			got, _, err := LoadState(path, Example())
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

func TestMatchTabs(t *testing.T) {
	tests := []struct {
		name     string
		had, now []string
		want     []int
	}{
		{"same", []string{"Comms", "Code"}, []string{"Comms", "Code"}, []int{0, 1}},
		{"case", []string{"Comms", "Code"}, []string{"comms", "CODE"}, []int{0, 1}},
		{"reordered", []string{"Comms", "Code", "Research"}, []string{"Research", "Comms", "Code"}, []int{2, 0, 1}},
		{"added", []string{"Comms", "Code"}, []string{"Comms", "Notes", "Code"}, []int{0, -1, 1}},
		{"removed", []string{"Comms", "Code", "Research"}, []string{"Comms", "Research"}, []int{0, 2}},
		{"renamed", []string{"Comms", "Code", "Research"}, []string{"Comms", "Dev", "Research"}, []int{0, 1, 2}},
		{"renamed only tab", []string{"Code"}, []string{"Dev"}, []int{0}},
		{"two renamed", []string{"Comms", "Code"}, []string{"Mail", "Dev"}, []int{-1, -1}},
		{"renamed and added", []string{"Comms", "Code"}, []string{"Comms", "Dev", "Notes"}, []int{0, -1, -1}},
		{"renamed and reordered", []string{"Comms", "Code", "Research"}, []string{"Code", "Comms", "Files"}, []int{1, 0, -1}},
		{"renamed and removed", []string{"Comms", "Code", "Research"}, []string{"Comms", "Dev"}, []int{0, -1}},
		{"repeated name", []string{"Code", "code"}, []string{"Code"}, []int{0}},
		{"none before", nil, []string{"Code"}, []int{-1}},
	}
	for _, tc := range tests {
		if got := MatchTabs(tc.had, tc.now); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: MatchTabs(%q, %q) = %v, want %v", tc.name, tc.had, tc.now, got, tc.want)
		}
	}
	if got := Dropped([]int{2, -1, 0}, 4); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Errorf("Dropped = %v, want [1 3]", got)
	}
}

// TestLoadStateKeepsTabsByName checks that a state.json whose tabs differ
// from the config's loads, and that a copy is set aside only if it has a
// tab that the config has no tab for.
func TestLoadStateKeepsTabsByName(t *testing.T) {
	requireLayout(t)
	tests := []struct {
		name    string
		change  func(s *State)
		dropped []string
	}{
		{name: "fewer tabs", change: func(s *State) { s.Tabs = s.Tabs[:2] }},
		{name: "renamed tab", change: func(s *State) { s.Tabs[2].Name = "Reading" }},
		{name: "tab name in other case", change: func(s *State) { s.Tabs[2].Name = "research" }},
		{name: "tabs out of order", change: func(s *State) { s.Tabs[0], s.Tabs[1] = s.Tabs[1], s.Tabs[0] }},
		{name: "more tabs", change: func(s *State) {
			s.Tabs = append(s.Tabs, TabState{Name: "More", Preset: "single", Root: pane("p9"), Focus: "p9"})
		}, dropped: []string{"More"}},
		{name: "renamed and fewer", change: func(s *State) {
			s.Tabs[1].Name = "Dev"
			s.Tabs = s.Tabs[:2]
		}, dropped: []string{"Dev"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, StateFile)
			s := goodState()
			tc.change(s)
			writeState(t, path, s)
			data := readFile(t, path)

			got, aside, err := LoadState(path, Example())
			if err != nil || got == nil || !reflect.DeepEqual(got.Tabs, s.Tabs) {
				t.Fatalf("LoadState = %+v, %v; want the state as saved", got, err)
			}
			if readFile(t, path) != data {
				t.Errorf("state.json was changed")
			}
			bad := filepath.Join(dir, BadState)
			if tc.dropped == nil {
				if aside != nil {
					t.Errorf("set aside %+v", aside)
				}
				if _, err := os.Stat(bad); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("a copy was set aside: %v", err)
				}
				return
			}
			if aside == nil || aside.Reason != nil || !reflect.DeepEqual(aside.Tabs, tc.dropped) {
				t.Errorf("set aside %+v, want tabs %v", aside, tc.dropped)
			}
			if readFile(t, bad) != data {
				t.Errorf("state.json.bad isn't a copy of state.json")
			}
		})
	}
}

// TestLoadStateKeepsThreeCopies checks that each file set aside moves the
// ones before along, keeping the last three.
func TestLoadStateKeepsThreeCopies(t *testing.T) {
	requireLayout(t)
	dir := t.TempDir()
	path := filepath.Join(dir, StateFile)
	var files []string
	for i := range 4 {
		s := goodState()
		s.Tabs = append(s.Tabs, TabState{Name: "More" + strconv.Itoa(i), Root: pane("p9")})
		if i%2 == 1 {
			s.Version = 0 // Set aside whole.
		}
		writeState(t, path, s)
		files = append(files, readFile(t, path))
		if _, _, err := LoadState(path, Example()); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{
		BadState: files[3], BadState + ".1": files[2], BadState + ".2": files[1],
	} {
		if got := readFile(t, filepath.Join(dir, name)); got != want {
			t.Errorf("%s:\n%s\nwant\n%s", name, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, BadState+".3")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a fourth copy was kept: %v", err)
	}

	// A copy for PeekState's file moves them along too.
	s := goodState()
	writeState(t, path, s)
	if err := SetAsideCopy(path); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, BadState)) != readFile(t, path) || readFile(t, filepath.Join(dir, BadState+".2")) != files[2] {
		t.Errorf("SetAsideCopy didn't keep a copy and move the others along")
	}
}

// closedMail is a closed tab, Mail, as Reopen closed tab keeps it.
func closedMail() ClosedTab {
	return ClosedTab{
		Tab: TabState{Name: "Mail", Preset: "side", Root: row("s30", []float64{50, 50}, pane("p31", "w32"), pane("p33", "w34")), Focus: "p31"},
		Items: map[string]ItemState{
			"w32": {Kind: "web", Title: "Inbox", Created: 32, Name: "Mail", URL: "https://mail.example.com/"},
			"w34": {Kind: "app", BundleID: "com.apple.Notes", WindowID: 140, Title: "Notes", Created: 34},
		},
		Config: Tab{Name: "Mail", Layout: "side", Panes: []Pane{{Items: []Item{{Web: "https://mail.example.com/", Name: "Mail"}}}, {}}},
		Rules:  []Rule{{App: "com.apple.Notes", Tab: "Mail"}},
		Index:  1,
		Closed: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC),
	}
}

func TestSaveAndLoadClosedTabs(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), StateFile)
	want := goodState()
	want.Closed = []ClosedTab{closedMail()}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	if data := readFile(t, path); !strings.Contains(data, "\"closed\": [") {
		t.Errorf("state.json has no closed tabs:\n%s", data)
	}
	got, aside, err := LoadState(path, Example())
	if err != nil || aside != nil {
		t.Fatal(aside, err)
	}
	// A closed tab's IDs stay in use, for when it comes back.
	want.Seq = 34
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadState = %+v\nwant %+v", got, want)
	}
}

// TestLoadStateTidiesClosedTabs checks that a closed tab that couldn't
// come back is dropped, rather than the whole state, and that a closed tab
// loses items that aren't in its tree.
func TestLoadStateTidiesClosedTabs(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), StateFile)
	s := goodState()
	noKind := closedMail()
	noKind.Tab.Name = "No kind"
	noKind.Items = map[string]ItemState{"w32": {Title: "Inbox"}, "w34": noKind.Items["w34"]}
	missing := closedMail()
	missing.Tab.Name = "Missing"
	missing.Items = map[string]ItemState{"w32": missing.Items["w32"]}
	unnamed := closedMail()
	unnamed.Tab.Name = ""
	extra := closedMail()
	extra.Items["w99"] = ItemState{Kind: "app", Title: "Orphan"}
	s.Closed = []ClosedTab{noKind, missing, unnamed, extra}
	writeState(t, path, s)

	got, aside, err := LoadState(path, Example())
	if err != nil || aside != nil || got == nil {
		t.Fatalf("LoadState = %v, %v, %v", got, aside, err)
	}
	if !reflect.DeepEqual(got.Closed, []ClosedTab{closedMail()}) {
		t.Errorf("Closed = %+v, want Mail alone, without w99", got.Closed)
	}
	if got.Seq != 34 {
		t.Errorf("Seq = %d, want 34, from Mail's items", got.Seq)
	}

	// None left is none.
	s.Closed = []ClosedTab{noKind}
	writeState(t, path, s)
	got, _, err = LoadState(path, Example())
	if err != nil || got.Closed != nil {
		t.Errorf("Closed = %+v, %v; want nil", got.Closed, err)
	}
}
