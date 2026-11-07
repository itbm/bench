package layout

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

func TestPresets(t *testing.T) {
	want := []Preset{
		{ID: "single", Label: "Single", Slots: 1, DefaultSizes: []float64{}},
		{ID: "side", Label: "Side by side", Slots: 2, DefaultSizes: []float64{50, 50}},
		{ID: "main-side", Label: "Main and side", Slots: 2, DefaultSizes: []float64{65, 35}},
		{ID: "three", Label: "Three columns", Slots: 3, DefaultSizes: []float64{40, 30, 30}},
		{ID: "top-bottom", Label: "Top and bottom", Slots: 2, DefaultSizes: []float64{65, 35}},
		{ID: "grid", Label: "Grid of four", Slots: 4, DefaultSizes: []float64{50, 50}},
	}
	got := Presets()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Presets() =\n%+v\nwant\n%+v", got, want)
	}
	got[1].DefaultSizes[0] = 1
	got[1].Label = "Changed"
	if again := Presets(); !reflect.DeepEqual(again, want) {
		t.Errorf("changing the result of Presets changed the presets: %+v", again)
	}
}

func TestLookupPreset(t *testing.T) {
	tests := []struct {
		id    string
		label string
		ok    bool
	}{
		{"single", "Single", true},
		{"side", "Side by side", true},
		{"main-side", "Main and side", true},
		{"three", "Three columns", true},
		{"top-bottom", "Top and bottom", true},
		{"grid", "Grid of four", true},
		{"custom", "", false},
		{"", "", false},
		{"Side", "", false},
	}
	for _, tt := range tests {
		p, ok := LookupPreset(tt.id)
		if ok != tt.ok || p.Label != tt.label {
			t.Errorf("LookupPreset(%q) = %q, %v; want %q, %v", tt.id, p.Label, ok, tt.label, tt.ok)
		}
	}
}

func TestBuildShapes(t *testing.T) {
	tests := []struct {
		preset string
		sizes  []float64
		want   *Node
	}{
		{
			preset: PresetSingle,
			want:   NewPane("a", "w1"),
		},
		{
			preset: PresetSide,
			want:   NewSplit("ns3", Row, []float64{50, 50}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetMainSide,
			want:   NewSplit("ns3", Row, []float64{65, 35}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetThree,
			want:   NewSplit("ns4", Row, []float64{40, 30, 30}, NewPane("a", "w1"), NewPane("b", "w2"), NewPane("np3")),
		},
		{
			preset: PresetTopBottom,
			want:   NewSplit("ns3", Column, []float64{65, 35}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetGrid,
			want: NewSplit("ns5", Column, []float64{50, 50},
				NewSplit("ns6", Row, []float64{50, 50}, NewPane("a", "w1"), NewPane("b", "w2")),
				NewSplit("ns7", Row, []float64{50, 50}, NewPane("np3"), NewPane("np4"))),
		},
		{
			preset: PresetSide,
			sizes:  []float64{45, 55},
			want:   NewSplit("ns3", Row, []float64{45, 55}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetThree,
			sizes:  []float64{2, 1, 1},
			want:   NewSplit("ns4", Row, []float64{50, 25, 25}, NewPane("a", "w1"), NewPane("b", "w2"), NewPane("np3")),
		},
		{
			preset: PresetGrid,
			sizes:  []float64{70, 30},
			want: NewSplit("ns5", Column, []float64{70, 30},
				NewSplit("ns6", Row, []float64{50, 50}, NewPane("a", "w1"), NewPane("b", "w2")),
				NewSplit("ns7", Row, []float64{50, 50}, NewPane("np3"), NewPane("np4"))),
		},
		{
			preset: PresetMainSide,
			sizes:  []float64{10, 20, 70},
			want:   NewSplit("ns3", Row, []float64{65, 35}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetMainSide,
			sizes:  []float64{-1, 20},
			want:   NewSplit("ns3", Row, []float64{65, 35}, NewPane("a", "w1"), NewPane("b", "w2")),
		},
		{
			preset: PresetSingle,
			sizes:  []float64{100},
			want:   NewPane("a", "w1"),
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %v", tt.preset, tt.sizes), func(t *testing.T) {
			p, _ := LookupPreset(tt.preset)
			panes := []*Node{NewPane("a", "w1"), NewPane("b", "w2")}[:min(2, p.Slots)]
			// Burn two IDs, so the generated IDs show the order they
			// were asked for in.
			newID := counter()
			newID(TypePane)
			newID(TypePane)
			got, err := Build(tt.preset, panes, tt.sizes, newID)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Build =\n%s\nwant\n%s", describe(got), describe(tt.want))
			}
			if got := ids(Leaves(got))[:len(panes)]; !slices.Equal(got, ids(panes)) {
				t.Errorf("panes in reading order = %v, want %v first", got, ids(panes))
			}
		})
	}
}

func TestBuildUsesGivenPanes(t *testing.T) {
	a := NewPane("a", "w1")
	root := mustBuild(t, PresetSide, nil, a)
	if root.Children[0] != a {
		t.Error("Build copied the pane instead of using it")
	}
}

func TestBuildErrors(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		panes   []*Node
		newID   NewID
		unknown bool
	}{
		{name: "unknown preset", preset: "four", unknown: true, newID: counter()},
		{name: "custom", preset: PresetCustom, unknown: true, newID: counter()},
		{name: "too many panes", preset: PresetSide, panes: []*Node{NewPane("a"), NewPane("b"), NewPane("c")}, newID: counter()},
		{name: "a split as a pane", preset: PresetSide, panes: []*Node{NewSplit("s", Row, nil, NewPane("a"), NewPane("b"))}, newID: counter()},
		{name: "a nil pane", preset: PresetSide, panes: []*Node{nil}, newID: counter()},
		{name: "no ID function for padding", preset: PresetSingle},
		{name: "no ID function for a split", preset: PresetSide, panes: []*Node{NewPane("a"), NewPane("b")}},
		{name: "empty IDs", preset: PresetSide, newID: func(string) string { return "" }},
		{name: "repeated IDs", preset: PresetSide, newID: func(string) string { return "x" }},
		{name: "an item in two panes", preset: PresetSide, panes: []*Node{NewPane("a", "w1"), NewPane("b", "w1")}, newID: counter()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := Build(tt.preset, tt.panes, nil, tt.newID)
			if err == nil {
				t.Fatalf("Build succeeded with %s, want an error", describe(root))
			}
			if got := errors.Is(err, ErrUnknownPreset); got != tt.unknown {
				t.Errorf("errors.Is(%v, ErrUnknownPreset) = %v, want %v", err, got, tt.unknown)
			}
		})
	}
}

// presetIDs returns every preset's ID.
func presetIDs() []string {
	var out []string
	for _, p := range Presets() {
		out = append(out, p.ID)
	}
	return out
}

// randomTree builds a preset filled with random items, numbered from *next,
// and random active items.
func randomTree(t *testing.T, r *rand.Rand, presetID string, next *int) *Node {
	t.Helper()
	p, _ := LookupPreset(presetID)
	panes := make([]*Node, p.Slots)
	for i := range panes {
		var items []string
		for range r.IntN(4) {
			*next++
			items = append(items, fmt.Sprintf("w%d", *next))
		}
		panes[i] = NewPane(fmt.Sprintf("old%d", i+1), items...)
		if len(items) > 0 {
			panes[i].Active = items[r.IntN(len(items))]
		}
	}
	root, err := Build(presetID, panes, nil, counter())
	if err != nil {
		t.Fatalf("Build(%q): %v", presetID, err)
	}
	return root
}

// TestApplyPresetEveryPair replaces every preset with every preset, with
// random items, and checks that nothing is lost, repeated, or reordered.
func TestApplyPresetEveryPair(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, from := range presetIDs() {
		for _, to := range presetIDs() {
			t.Run(from+" to "+to, func(t *testing.T) {
				for range 50 {
					next := 0
					old := randomTree(t, r, from, &next)
					checkApplyPreset(t, old, to)
				}
			})
		}
	}
}

// TestApplyPresetCustom replaces edited trees with more panes than any
// preset.
func TestApplyPresetCustom(t *testing.T) {
	old := NewSplit("s1", Row, []float64{20, 20, 60},
		NewPane("p1", "w1", "w2"),
		NewSplit("s2", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3", "w3")),
		NewSplit("s3", Column, []float64{30, 30, 40}, NewPane("p4", "w4"), NewPane("p5", "w5", "w6"), NewPane("p6", "w7")))
	old.Children[2].Children[1].Active = "w6"
	for _, to := range presetIDs() {
		t.Run(to, func(t *testing.T) { checkApplyPreset(t, old, to) })
	}
}

func TestApplyPresetNil(t *testing.T) {
	root, err := ApplyPreset(nil, PresetSide, nil, counter())
	if err != nil {
		t.Fatalf("ApplyPreset(nil): %v", err)
	}
	want := NewSplit("ns3", Row, []float64{50, 50}, NewPane("np1"), NewPane("np2"))
	if !reflect.DeepEqual(root, want) {
		t.Errorf("ApplyPreset(nil) = %s, want %s", describe(root), describe(want))
	}
}

func TestApplyPresetActive(t *testing.T) {
	// The first old pane landing in the last new pane is empty, so the
	// next one's active item is kept.
	old := NewSplit("s1", Row, []float64{25, 25, 25, 25},
		NewPane("p1", "w1"),
		NewPane("p2"),
		NewPane("p3", "w2", "w3"),
		NewPane("p4", "w4"))
	old.Children[2].Active = "w3"
	got, err := ApplyPreset(old, PresetSide, []float64{30, 70}, counter())
	if err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	want := NewSplit("ns1", Row, []float64{30, 70}, NewPane("p1", "w1"), NewPane("p2", "w2", "w3", "w4"))
	want.Children[1].Active = "w3"
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ApplyPreset =\n%s\nwant\n%s", describe(got), describe(want))
	}
}

func TestApplyPresetErrors(t *testing.T) {
	old := NewSplit("s1", Row, []float64{50, 50}, NewPane("p1", "w1"), NewPane("p2", "w2"))
	if _, err := ApplyPreset(old, PresetCustom, nil, counter()); !errors.Is(err, ErrUnknownPreset) {
		t.Errorf("ApplyPreset(custom) error = %v, want ErrUnknownPreset", err)
	}
	if _, err := ApplyPreset(old, PresetThree, nil, nil); err == nil {
		t.Error("ApplyPreset with no ID function succeeded, want an error")
	}
	bad := NewSplit("s1", Row, []float64{50, 50}, NewPane("p1", "w1"), NewPane("p2", "w1"))
	if _, err := ApplyPreset(bad, PresetSingle, nil, counter()); err == nil {
		t.Error("ApplyPreset with an item in two panes succeeded, want an error")
	}
}

// TestPresetOf checks that every preset's tree has its own shape, and only
// Side by side and Main and side share one; and that trees that have
// changed are matched by their shape, and by their sizes only between
// those two.
func TestPresetOf(t *testing.T) {
	for _, id := range presetIDs() {
		p, _ := LookupPreset(id)
		root := mustBuild(t, id, p.DefaultSizes)
		if got, ok := PresetOf(root); got != id || !ok {
			t.Errorf("PresetOf(the %s tree) = %q, %v", id, got, ok)
		}
		for _, other := range presetIDs() {
			same := other == id || (id == PresetSide || id == PresetMainSide) && (other == PresetSide || other == PresetMainSide)
			if got := HasShape(root, other); got != same {
				t.Errorf("HasShape(the %s tree, %q) = %v, want %v", id, other, got, same)
			}
		}
		if HasShape(root, PresetCustom) {
			t.Errorf("HasShape(the %s tree, custom) = true", id)
		}
	}
	row := func(sizes ...float64) *Node {
		var panes []*Node
		for i := range sizes {
			panes = append(panes, NewPane(fmt.Sprintf("p%d", i+1), fmt.Sprintf("w%d", i+1)))
		}
		return NewSplit("s1", Row, sizes, panes...)
	}
	column := NewSplit("s1", Column, []float64{70, 30}, NewPane("p1", "w1"), NewPane("p2"))
	grid := NewSplit("s1", Column, []float64{40, 60},
		NewSplit("s2", Row, []float64{30, 70}, NewPane("p1", "w1"), NewPane("p2", "w2")),
		NewSplit("s3", Row, []float64{50, 50}, NewPane("p3", "w3"), NewPane("p4")))
	for _, tt := range []struct {
		name string
		root *Node
		want string
	}{
		{"a pane", NewPane("p1", "w1", "w2"), PresetSingle},
		{"a row of two, equal", row(50, 50), PresetSide},
		{"a row of two, nearer 50/50", row(57, 43), PresetSide},
		{"a row of two, nearer 65/35", row(58, 42), PresetMainSide},
		{"a row of two, wider than 65/35", row(80, 20), PresetMainSide},
		{"a row of two, the second wider", row(30, 70), PresetSide},
		{"a row of two, sizes that don't add up to 100", row(2, 1), PresetMainSide},
		{"a row of three", row(20, 20, 60), PresetThree},
		{"a column of two", column, PresetTopBottom},
		{"two rows of two in a column", grid, PresetGrid},
		{"a row of four", row(25, 25, 25, 25), ""},
		{"a column of three", NewSplit("s1", Column, nil, NewPane("p1"), NewPane("p2"), NewPane("p3")), ""},
		{"a row of a pane and a column", NewSplit("s1", Row, nil, NewPane("p1"), NewSplit("s2", Column, nil, NewPane("p2"), NewPane("p3"))), ""},
		{"a column of a row and a pane", NewSplit("s1", Column, nil, NewSplit("s2", Row, nil, NewPane("p1"), NewPane("p2")), NewPane("p3")), ""},
		{"no tree", nil, ""},
	} {
		got, ok := PresetOf(tt.root)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("PresetOf(%s): %s = %q, %v; want %q", tt.name, describe(tt.root), got, ok, tt.want)
		}
	}
}

// checkApplyPreset applies a preset to old and checks the documented
// guarantees.
func checkApplyPreset(t *testing.T, old *Node, to string) {
	t.Helper()
	before := Clone(old)
	got, err := ApplyPreset(old, to, nil, counter())
	if err != nil {
		t.Fatalf("ApplyPreset(%s, %q): %v", describe(old), to, err)
	}
	if !reflect.DeepEqual(old, before) {
		t.Fatalf("ApplyPreset changed its input:\n got %s\nwant %s", describe(old), describe(before))
	}
	if err := Validate(got); err != nil {
		t.Fatalf("ApplyPreset(%s, %q) = %s, which isn't valid: %v", describe(old), to, describe(got), err)
	}
	if !slices.Equal(Items(got), Items(old)) {
		t.Fatalf("items = %v, want %v", Items(got), Items(old))
	}
	p, _ := LookupPreset(to)
	oldPanes, newPanes := Leaves(old), Leaves(got)
	if len(newPanes) != p.Slots {
		t.Fatalf("%d panes, want %d", len(newPanes), p.Slots)
	}
	for i, np := range newPanes {
		if i < len(oldPanes) && np.ID != oldPanes[i].ID {
			t.Errorf("pane %d has ID %q, want the old pane's %q", i+1, np.ID, oldPanes[i].ID)
		}
		var items []string
		active := ""
		for j, op := range oldPanes {
			if min(j, p.Slots-1) != i {
				continue
			}
			items = append(items, op.Items...)
			if active == "" {
				active = op.Active
			}
		}
		if active == "" && len(items) > 0 {
			active = items[0]
		}
		if !slices.Equal(np.Items, items) {
			t.Errorf("pane %d holds %v, want %v", i+1, np.Items, items)
		}
		if np.Active != active {
			t.Errorf("pane %d shows %q, want %q", i+1, np.Active, active)
		}
	}
}
