package layout

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"
)

func TestNormalise(t *testing.T) {
	third := 100.0 / 3
	tests := []struct {
		name  string
		sizes []float64
		n     int
		want  []float64
	}{
		{"already 100", []float64{65, 35}, 2, []float64{65, 35}},
		{"scaled up", []float64{1, 1, 2}, 3, []float64{25, 25, 50}},
		{"scaled down", []float64{90, 90}, 2, []float64{50, 50}},
		{"nil", nil, 3, []float64{third, third, third}},
		{"too short", []float64{100}, 2, []float64{50, 50}},
		{"too long", []float64{50, 25, 25}, 2, []float64{50, 50}},
		{"zero", []float64{0, 100}, 2, []float64{50, 50}},
		{"negative", []float64{-10, 110}, 2, []float64{50, 50}},
		{"NaN", []float64{math.NaN(), 50}, 2, []float64{50, 50}},
		{"infinite", []float64{math.Inf(1), 50}, 2, []float64{50, 50}},
		{"huge", []float64{math.MaxFloat64, math.MaxFloat64}, 2, []float64{50, 50}},
		{"huge and uneven", []float64{math.MaxFloat64, math.MaxFloat64 / 4}, 2, []float64{80, 20}},
		{"too uneven to represent", []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}, 2, []float64{50, 50}},
		{"one", []float64{7}, 1, []float64{100}},
		{"none", nil, 0, nil},
		{"negative count", []float64{1}, -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalise(tt.sizes, tt.n)
			if !near(got, tt.want) {
				t.Errorf("Normalise(%v, %d) = %v, want %v", tt.sizes, tt.n, got, tt.want)
			}
		})
	}
}

func TestNormaliseDoesNotChangeInput(t *testing.T) {
	in := []float64{1, 3}
	Normalise(in, 2)
	if !slices.Equal(in, []float64{1, 3}) {
		t.Errorf("Normalise changed its input to %v", in)
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		name, pane, item string
		ok               bool
		notFound         bool
		wantItems        []string
	}{
		{name: "appends and shows", pane: "p2", item: "w9", ok: true, wantItems: []string{"w3", "w9"}},
		{name: "unknown pane", pane: "p9", item: "w9", notFound: true},
		{name: "a split", pane: "s2", item: "w9"},
		{name: "already in another pane", pane: "p2", item: "w1"},
		{name: "already in the pane", pane: "p1", item: "w1"},
		{name: "no ID", pane: "p1", item: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sample()
			err := Insert(root, tt.pane, tt.item)
			if (err == nil) != tt.ok {
				t.Fatalf("Insert error = %v, want ok %v", err, tt.ok)
			}
			if errors.Is(err, ErrNotFound) != tt.notFound {
				t.Errorf("errors.Is(%v, ErrNotFound) = %v, want %v", err, !tt.notFound, tt.notFound)
			}
			if !tt.ok {
				if !reflect.DeepEqual(root, sample()) {
					t.Errorf("a failed Insert changed the tree: %s", describe(root))
				}
				return
			}
			p, _, _ := Find(root, tt.pane)
			if !slices.Equal(p.Items, tt.wantItems) || p.Active != tt.item {
				t.Errorf("pane = %v showing %q, want %v showing %q", p.Items, p.Active, tt.wantItems, tt.item)
			}
			mustValidate(t, root)
		})
	}
}

func TestTakeItem(t *testing.T) {
	tests := []struct {
		name     string
		tree     func() *Node
		item     string
		ok       bool
		fromPane string
		want     *Node
	}{
		{
			name: "shown item: the next one takes its place",
			tree: func() *Node { p := NewPane("p1", "w1", "w2", "w3"); p.Active = "w2"; return p },
			item: "w2", ok: true, fromPane: "p1",
			want: &Node{Type: TypePane, ID: "p1", Items: []string{"w1", "w3"}, Active: "w3"},
		},
		{
			name: "shown last item: the one before",
			tree: func() *Node { p := NewPane("p1", "w1", "w2", "w3"); p.Active = "w3"; return p },
			item: "w3", ok: true, fromPane: "p1",
			want: &Node{Type: TypePane, ID: "p1", Items: []string{"w1", "w2"}, Active: "w2"},
		},
		{
			name: "shown first item",
			tree: func() *Node { return NewPane("p1", "w1", "w2") },
			item: "w1", ok: true, fromPane: "p1",
			want: &Node{Type: TypePane, ID: "p1", Items: []string{"w2"}, Active: "w2"},
		},
		{
			name: "hidden item: the shown one stays",
			tree: func() *Node { p := NewPane("p1", "w1", "w2", "w3"); p.Active = "w3"; return p },
			item: "w1", ok: true, fromPane: "p1",
			want: &Node{Type: TypePane, ID: "p1", Items: []string{"w2", "w3"}, Active: "w3"},
		},
		{
			name: "the root pane stays, empty",
			tree: func() *Node { return NewPane("p1", "w1") },
			item: "w1", ok: true, fromPane: "p1",
			want: &Node{Type: TypePane, ID: "p1"},
		},
		{
			name: "an emptied pane goes and its split collapses",
			tree: sample,
			item: "w3", ok: true, fromPane: "p2",
			want: NewSplit("s1", Row, []float64{65, 35}, NewPane("p1", "w1", "w2"), NewPane("p3", "w4", "w5")),
		},
		{
			name: "an emptied pane goes from a three-way split",
			tree: func() *Node {
				return NewSplit("s1", Row, []float64{40, 30, 30}, NewPane("p1", "w1"), NewPane("p2", "w2"), NewPane("p3", "w3"))
			},
			item: "w1", ok: true, fromPane: "p1",
			want: &Node{Type: TypeSplit, ID: "s1", Dir: Row, Sizes: []float64{50, 50},
				Children: []*Node{NewPane("p2", "w2"), NewPane("p3", "w3")}},
		},
		{
			name: "the root split collapses to a pane",
			tree: func() *Node {
				return NewSplit("s1", Column, nil, NewPane("p1", "w1"), NewPane("p2", "w2"))
			},
			item: "w2", ok: true, fromPane: "p2",
			want: NewPane("p1", "w1"),
		},
		{
			name: "not in the tree",
			tree: sample,
			item: "w9", ok: false, fromPane: "",
			want: sample(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, from, ok := TakeItem(tt.tree(), tt.item)
			if ok != tt.ok || from != tt.fromPane {
				t.Errorf("TakeItem = %q, %v; want %q, %v", from, ok, tt.fromPane, tt.ok)
			}
			if !treesNear(root, tt.want) {
				t.Errorf("TakeItem tree =\n%s\nwant\n%s", describe(root), describe(tt.want))
			}
			mustValidate(t, root)
		})
	}
}

// treesNear reports whether two trees are equal, allowing float error in
// sizes.
func treesNear(a, b *Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.ID != b.ID || a.Dir != b.Dir || a.Active != b.Active ||
		!slices.Equal(a.Items, b.Items) || !near(a.Sizes, b.Sizes) || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !treesNear(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

func TestRemovePane(t *testing.T) {
	tests := []struct {
		name     string
		tree     func() *Node
		pane     string
		want     *Node
		fails    bool
		notFound bool
	}{
		{
			name: "renormalises the rest",
			tree: func() *Node {
				return NewSplit("s1", Row, []float64{40, 30, 30}, NewPane("p1"), NewPane("p2"), NewPane("p3"))
			},
			pane: "p2",
			want: &Node{Type: TypeSplit, ID: "s1", Dir: Row, Sizes: []float64{400.0 / 7, 300.0 / 7},
				Children: []*Node{NewPane("p1"), NewPane("p3")}},
		},
		{
			name: "collapses a nested split into its child",
			tree: sample,
			pane: "p3",
			want: NewSplit("s1", Row, []float64{65, 35}, NewPane("p1", "w1", "w2"), NewPane("p2", "w3")),
		},
		{
			name: "collapses the root split into its child",
			tree: sample,
			pane: "p1",
			want: NewSplit("s2", Column, []float64{50, 50}, NewPane("p2", "w3"), NewPane("p3", "w4", "w5")),
		},
		{
			name: "collapses into a split",
			tree: func() *Node {
				return NewSplit("s1", Column, []float64{50, 50},
					NewSplit("s2", Row, []float64{50, 50},
						NewPane("p1"),
						NewSplit("s3", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3"))),
					NewPane("p4"))
			},
			pane: "p1",
			want: NewSplit("s1", Column, []float64{50, 50},
				NewSplit("s3", Column, []float64{50, 50}, NewPane("p2"), NewPane("p3")),
				NewPane("p4")),
		},
		{
			name: "the root pane stays",
			tree: func() *Node { return NewPane("p1", "w1") },
			pane: "p1",
			want: NewPane("p1", "w1"),
		},
		{
			name: "removes a pane that still has items",
			tree: func() *Node { return NewSplit("s1", Row, nil, NewPane("p1", "w1"), NewPane("p2", "w2")) },
			pane: "p1",
			want: NewPane("p2", "w2"),
		},
		{name: "unknown pane", tree: sample, pane: "p9", want: sample(), fails: true, notFound: true},
		{name: "a split", tree: sample, pane: "s2", want: sample(), fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := RemovePane(tt.tree(), tt.pane)
			if (err != nil) != tt.fails || errors.Is(err, ErrNotFound) != tt.notFound {
				t.Fatalf("RemovePane error = %v, want fails %v, not found %v", err, tt.fails, tt.notFound)
			}
			if !treesNear(root, tt.want) {
				t.Errorf("RemovePane =\n%s\nwant\n%s", describe(root), describe(tt.want))
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		tree func() *Node
		pane string
		zone Zone
		want *Node
	}{
		{
			name: "right of the root pane",
			tree: func() *Node { return NewPane("p1", "w1") },
			pane: "p1", zone: Right,
			want: NewSplit("ns1", Row, []float64{50, 50}, NewPane("p1", "w1"), NewPane("new", "w9")),
		},
		{
			name: "left of the root pane",
			tree: func() *Node { return NewPane("p1", "w1") },
			pane: "p1", zone: Left,
			want: NewSplit("ns1", Row, []float64{50, 50}, NewPane("new", "w9"), NewPane("p1", "w1")),
		},
		{
			name: "below the root pane",
			tree: func() *Node { return NewPane("p1", "w1") },
			pane: "p1", zone: Bottom,
			want: NewSplit("ns1", Column, []float64{50, 50}, NewPane("p1", "w1"), NewPane("new", "w9")),
		},
		{
			name: "above the root pane",
			tree: func() *Node { return NewPane("p1", "w1") },
			pane: "p1", zone: Top,
			want: NewSplit("ns1", Column, []float64{50, 50}, NewPane("new", "w9"), NewPane("p1", "w1")),
		},
		{
			name: "right, in a row: a sibling sharing the pane's size",
			tree: sample,
			pane: "p1", zone: Right,
			want: NewSplit("s1", Row, []float64{32.5, 32.5, 35},
				NewPane("p1", "w1", "w2"),
				NewPane("new", "w9"),
				NewSplit("s2", Column, []float64{50, 50}, NewPane("p2", "w3"), NewPane("p3", "w4", "w5"))),
		},
		{
			name: "left, in a row: a sibling before the pane",
			tree: sample,
			pane: "p1", zone: Left,
			want: NewSplit("s1", Row, []float64{32.5, 32.5, 35},
				NewPane("new", "w9"),
				NewPane("p1", "w1", "w2"),
				NewSplit("s2", Column, []float64{50, 50}, NewPane("p2", "w3"), NewPane("p3", "w4", "w5"))),
		},
		{
			name: "below, in a column: a sibling",
			tree: sample,
			pane: "p2", zone: Bottom,
			want: NewSplit("s1", Row, []float64{65, 35},
				NewPane("p1", "w1", "w2"),
				NewSplit("s2", Column, []float64{25, 25, 50}, NewPane("p2", "w3"), NewPane("new", "w9"), NewPane("p3", "w4", "w5"))),
		},
		{
			name: "below, in a row: a new split",
			tree: sample,
			pane: "p1", zone: Bottom,
			want: NewSplit("s1", Row, []float64{65, 35},
				NewSplit("ns1", Column, []float64{50, 50}, NewPane("p1", "w1", "w2"), NewPane("new", "w9")),
				NewSplit("s2", Column, []float64{50, 50}, NewPane("p2", "w3"), NewPane("p3", "w4", "w5"))),
		},
		{
			name: "right, in a column: a new split",
			tree: sample,
			pane: "p3", zone: Right,
			want: NewSplit("s1", Row, []float64{65, 35},
				NewPane("p1", "w1", "w2"),
				NewSplit("s2", Column, []float64{50, 50},
					NewPane("p2", "w3"),
					NewSplit("ns1", Row, []float64{50, 50}, NewPane("p3", "w4", "w5"), NewPane("new", "w9")))),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := Split(tt.tree(), tt.pane, tt.zone, NewPane("new", "w9"), counter())
			if err != nil {
				t.Fatalf("Split: %v", err)
			}
			if !treesNear(root, tt.want) {
				t.Errorf("Split =\n%s\nwant\n%s", describe(root), describe(tt.want))
			}
			mustValidate(t, root)
		})
	}
}

func TestSplitErrors(t *testing.T) {
	tests := []struct {
		name     string
		pane     string
		zone     Zone
		fresh    *Node
		newID    NewID
		notFound bool
	}{
		{name: "centre", pane: "p1", zone: Center, fresh: NewPane("new", "w9"), newID: counter()},
		{name: "unknown zone", pane: "p1", zone: "middle", fresh: NewPane("new", "w9"), newID: counter()},
		{name: "unknown pane", pane: "p9", zone: Right, fresh: NewPane("new", "w9"), newID: counter(), notFound: true},
		{name: "a split", pane: "s2", zone: Right, fresh: NewPane("new", "w9"), newID: counter()},
		{name: "nothing to add", pane: "p1", zone: Right, newID: counter()},
		{name: "an ID that's in the tree", pane: "p1", zone: Right, fresh: NewPane("p2", "w9"), newID: counter()},
		{name: "an item that's in the tree", pane: "p1", zone: Right, fresh: NewPane("new", "w3"), newID: counter()},
		{name: "an invalid new node", pane: "p1", zone: Right, fresh: &Node{Type: "box", ID: "new"}, newID: counter()},
		{name: "no ID function", pane: "p1", zone: Bottom, fresh: NewPane("new", "w9")},
		{name: "an empty ID", pane: "p1", zone: Bottom, fresh: NewPane("new", "w9"), newID: func(string) string { return "" }},
		{name: "a used ID", pane: "p1", zone: Bottom, fresh: NewPane("new", "w9"), newID: func(string) string { return "s2" }},
		{name: "the new node's ID", pane: "p1", zone: Bottom, fresh: NewPane("new", "w9"), newID: func(string) string { return "new" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sample()
			got, err := Split(root, tt.pane, tt.zone, tt.fresh, tt.newID)
			if err == nil {
				t.Fatalf("Split succeeded with %s, want an error", describe(got))
			}
			if errors.Is(err, ErrNotFound) != tt.notFound {
				t.Errorf("errors.Is(%v, ErrNotFound) = %v, want %v", err, !tt.notFound, tt.notFound)
			}
			if got != root || !reflect.DeepEqual(root, sample()) {
				t.Errorf("a failed Split changed the tree: %s", describe(got))
			}
		})
	}
}

func TestSplitWithoutIDFunctionInSameDirection(t *testing.T) {
	// A sibling needs no new split, so no ID function is needed.
	root, err := Split(sample(), "p1", Right, NewPane("new", "w9"), nil)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	mustValidate(t, root)
}

func TestSetSizes(t *testing.T) {
	tests := []struct {
		name     string
		split    string
		sizes    []float64
		want     []float64
		fails    bool
		notFound bool
	}{
		{name: "sets", split: "s1", sizes: []float64{40, 60}, want: []float64{40, 60}},
		{name: "normalises", split: "s2", sizes: []float64{1, 3}, want: []float64{25, 75}},
		{name: "wrong length", split: "s1", sizes: []float64{100}, fails: true},
		{name: "zero", split: "s1", sizes: []float64{0, 100}, fails: true},
		{name: "negative", split: "s1", sizes: []float64{-1, 100}, fails: true},
		{name: "NaN", split: "s1", sizes: []float64{math.NaN(), 100}, fails: true},
		{name: "infinite", split: "s1", sizes: []float64{math.Inf(1), 100}, fails: true},
		{name: "a pane", split: "p1", sizes: []float64{50, 50}, fails: true},
		{name: "unknown", split: "s9", sizes: []float64{50, 50}, fails: true, notFound: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := sample()
			err := SetSizes(root, tt.split, tt.sizes)
			if (err != nil) != tt.fails || errors.Is(err, ErrNotFound) != tt.notFound {
				t.Fatalf("SetSizes error = %v, want fails %v, not found %v", err, tt.fails, tt.notFound)
			}
			if tt.fails {
				if !reflect.DeepEqual(root, sample()) {
					t.Errorf("a failed SetSizes changed the tree: %s", describe(root))
				}
				return
			}
			n, _, _ := Find(root, tt.split)
			if !near(n.Sizes, tt.want) {
				t.Errorf("sizes = %v, want %v", n.Sizes, tt.want)
			}
		})
	}
}
