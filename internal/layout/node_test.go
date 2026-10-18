package layout

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// sample returns row 65, 35 of p1 [w1 w2] and a column 50, 50 of p2 [w3]
// and p3 [w4 w5].
func sample() *Node {
	return NewSplit("s1", Row, []float64{65, 35},
		NewPane("p1", "w1", "w2"),
		NewSplit("s2", Column, []float64{50, 50}, NewPane("p2", "w3"), NewPane("p3", "w4", "w5")))
}

func TestNewPane(t *testing.T) {
	items := []string{"w1", "w2"}
	p := NewPane("p1", items...)
	want := &Node{Type: TypePane, ID: "p1", Items: []string{"w1", "w2"}, Active: "w1"}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("NewPane = %+v, want %+v", p, want)
	}
	items[0] = "changed"
	if p.Items[0] != "w1" {
		t.Error("NewPane kept the caller's slice")
	}
	if empty := NewPane("p2"); empty.Items != nil || empty.Active != "" {
		t.Errorf("NewPane with no items = %+v, want no items and nothing shown", empty)
	}
}

func TestNewSplit(t *testing.T) {
	a, b := NewPane("p1"), NewPane("p2")
	tests := []struct {
		name  string
		sizes []float64
		want  []float64
	}{
		{"given", []float64{65, 35}, []float64{65, 35}},
		{"scaled", []float64{1, 3}, []float64{25, 75}},
		{"nil", nil, []float64{50, 50}},
		{"wrong length", []float64{100}, []float64{50, 50}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSplit("s1", Row, tt.sizes, a, b)
			if s.Type != TypeSplit || s.ID != "s1" || s.Dir != Row || !near(s.Sizes, tt.want) {
				t.Errorf("NewSplit = %+v, want sizes %v", s, tt.want)
			}
			if len(s.Children) != 2 || s.Children[0] != a || s.Children[1] != b {
				t.Errorf("NewSplit children = %v", s.Children)
			}
		})
	}
}

func TestClone(t *testing.T) {
	if Clone(nil) != nil {
		t.Error("Clone(nil) isn't nil")
	}
	root := sample()
	c := Clone(root)
	if !reflect.DeepEqual(c, root) {
		t.Fatalf("Clone = %s, want %s", describe(c), describe(root))
	}
	c.Sizes[0] = 1
	c.Children[0].Items[0] = "changed"
	c.Children[1].Children[0].ID = "changed"
	c.Children[1].Children = append(c.Children[1].Children, NewPane("p9"))
	if !reflect.DeepEqual(root, sample()) {
		t.Errorf("changing a clone changed the original: %s", describe(root))
	}
}

func TestLeavesAndItems(t *testing.T) {
	tests := []struct {
		name  string
		tree  *Node
		panes []string
		items []string
	}{
		{"nil", nil, []string{}, nil},
		{"pane", NewPane("p1", "w1"), []string{"p1"}, []string{"w1"}},
		{"nested", sample(), []string{"p1", "p2", "p3"}, []string{"w1", "w2", "w3", "w4", "w5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ids(Leaves(tt.tree)); !slices.Equal(got, tt.panes) {
				t.Errorf("Leaves = %v, want %v", got, tt.panes)
			}
			if got := Items(tt.tree); !slices.Equal(got, tt.items) {
				t.Errorf("Items = %v, want %v", got, tt.items)
			}
		})
	}
}

func TestFind(t *testing.T) {
	root := sample()
	tests := []struct {
		id     string
		found  bool
		parent string
		index  int
	}{
		{"s1", true, "", -1},
		{"p1", true, "s1", 0},
		{"s2", true, "s1", 1},
		{"p2", true, "s2", 0},
		{"p3", true, "s2", 1},
		{"p9", false, "", -1},
		{"", false, "", -1},
	}
	for _, tt := range tests {
		node, parent, index := Find(root, tt.id)
		if (node != nil) != tt.found || index != tt.index {
			t.Errorf("Find(%q) = %v, %v, %d; want found %v, index %d", tt.id, node, parent, index, tt.found, tt.index)
			continue
		}
		if node != nil && node.ID != tt.id {
			t.Errorf("Find(%q) found %q", tt.id, node.ID)
		}
		if got := ""; parent != nil {
			got = parent.ID
			if got != tt.parent {
				t.Errorf("Find(%q) parent = %q, want %q", tt.id, got, tt.parent)
			}
		} else if tt.parent != "" {
			t.Errorf("Find(%q) parent = nil, want %q", tt.id, tt.parent)
		}
	}
	if node, parent, index := Find(nil, "p1"); node != nil || parent != nil || index != -1 {
		t.Errorf("Find(nil) = %v, %v, %d", node, parent, index)
	}
}

func TestPaneOf(t *testing.T) {
	root := sample()
	tests := []struct{ item, pane string }{
		{"w1", "p1"},
		{"w2", "p1"},
		{"w3", "p2"},
		{"w5", "p3"},
		{"w9", ""},
	}
	for _, tt := range tests {
		got := ""
		if p := PaneOf(root, tt.item); p != nil {
			got = p.ID
		}
		if got != tt.pane {
			t.Errorf("PaneOf(%q) = %q, want %q", tt.item, got, tt.pane)
		}
	}
}

// TestJSONShape checks that a tree has the prototype's JSON shape and
// survives a round trip.
func TestJSONShape(t *testing.T) {
	root := NewSplit("s1", Row, []float64{50, 50}, NewPane("p1", "w1"), NewPane("p2"))
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"type":"split","id":"s1","dir":"row","sizes":[50,50],"children":[` +
		`{"type":"pane","id":"p1","items":["w1"],"active":"w1"},{"type":"pane","id":"p2"}]}`
	if string(data) != want {
		t.Errorf("JSON =\n%s\nwant\n%s", data, want)
	}
	var back Node
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(&back, root) {
		t.Errorf("round trip = %s, want %s", describe(&back), describe(root))
	}
}
