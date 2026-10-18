package layout

import (
	"reflect"
	"testing"
)

func TestChooseSizeClass(t *testing.T) {
	tests := []struct {
		mode         Mode
		width, below float64
		want         SizeClass
	}{
		{ModeWide, 1512, 1800, Wide},
		{ModeCompact, 2560, 1800, Compact},
		{ModeAuto, 2560, 1800, Wide},
		{ModeAuto, 1512, 1800, Compact},
		{ModeAuto, 1800, 1800, Wide},
		{ModeAuto, 1799.5, 1800, Compact},
		{"", 1512, 1800, Compact},
		{"other", 2560, 1800, Wide},
	}
	for _, tt := range tests {
		if got := ChooseSizeClass(tt.mode, tt.width, tt.below); got != tt.want {
			t.Errorf("ChooseSizeClass(%q, %v, %v) = %q, want %q", tt.mode, tt.width, tt.below, got, tt.want)
		}
	}
}

// adaptSample returns a row of p1 [w1 w2] (showing w2) and a column of
// p2 [w3] and a row of p3 [w4] and p4 [w5].
func adaptSample() *Node {
	root := NewSplit("s1", Row, []float64{60, 40},
		NewPane("p1", "w1", "w2"),
		NewSplit("s2", Column, []float64{50, 50},
			NewPane("p2", "w3"),
			NewSplit("s3", Row, []float64{50, 50}, NewPane("p3", "w4"), NewPane("p4", "w5"))))
	root.Children[0].Active = "w2"
	return root
}

func TestAdapt(t *testing.T) {
	stacked := adaptSample()
	stacked.Dir = Column
	compact := func(active string) *Node {
		p := NewPane(CompactPaneID, "w1", "w2", "w3", "w4", "w5")
		p.Active = active
		return p
	}
	column := NewSplit("s1", Column, []float64{50, 50}, NewPane("p1", "w1"), NewPane("p2", "w2"))
	tests := []struct {
		name         string
		root         *Node
		setting      SmallScreens
		class        SizeClass
		focused      string
		compactShown string
		want         *Node
	}{
		{name: "wide, stack", root: adaptSample(), setting: Stack, class: Wide, want: adaptSample()},
		{name: "wide, tabs", root: adaptSample(), setting: Tabs, class: Wide, want: adaptSample()},
		{name: "compact, same", root: adaptSample(), setting: Same, class: Compact, want: adaptSample()},
		{name: "compact, unknown setting", root: adaptSample(), setting: "other", class: Compact, want: adaptSample()},
		{name: "compact, no setting: stack, the default", root: adaptSample(), setting: "", class: Compact, want: stacked},
		{name: "wide, no setting", root: adaptSample(), setting: "", class: Wide, want: adaptSample()},
		{name: "compact, stack: only the top row turns", root: adaptSample(), setting: Stack, class: Compact, want: stacked},
		{name: "compact, stack: a column stays", root: column, setting: Stack, class: Compact, want: column},
		{name: "compact, stack: a pane stays", root: NewPane("p1", "w1"), setting: Stack, class: Compact, want: NewPane("p1", "w1")},
		{
			name: "compact, tabs: the shown item", root: adaptSample(), setting: Tabs, class: Compact,
			focused: "p1", compactShown: "w4", want: compact("w4"),
		},
		{
			name: "compact, tabs: the focused pane's item", root: adaptSample(), setting: Tabs, class: Compact,
			focused: "p1", compactShown: "w9", want: compact("w2"),
		},
		{
			name: "compact, tabs: the focused pane's item, with nothing shown", root: adaptSample(), setting: Tabs, class: Compact,
			focused: "p4", want: compact("w5"),
		},
		{
			name: "compact, tabs: the first item", root: adaptSample(), setting: Tabs, class: Compact,
			focused: "p9", want: compact("w1"),
		},
		{
			name: "compact, tabs: a split is focused", root: adaptSample(), setting: Tabs, class: Compact,
			focused: "s2", want: compact("w1"),
		},
		{
			name: "compact, tabs: no items", root: NewSplit("s1", Row, nil, NewPane("p1"), NewPane("p2")), setting: Tabs, class: Compact,
			focused: "p1", compactShown: "w1", want: &Node{Type: TypePane, ID: CompactPaneID},
		},
		{name: "nil", root: nil, setting: Tabs, class: Compact, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := Clone(tt.root)
			got := Adapt(tt.root, tt.setting, tt.class, tt.focused, tt.compactShown)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Adapt =\n%s\nwant\n%s", describe(got), describe(tt.want))
			}
			if !reflect.DeepEqual(tt.root, before) {
				t.Errorf("Adapt changed its input to %s", describe(tt.root))
			}
			if got != nil && tt.root != nil {
				// The result must share nothing with the input.
				got.ID = "changed"
				if leaves := Leaves(got); len(leaves) > 0 && len(leaves[0].Items) > 0 {
					leaves[0].Items[0] = "changed"
				}
				if !reflect.DeepEqual(tt.root, before) {
					t.Errorf("changing Adapt's result changed its input to %s", describe(tt.root))
				}
			}
		})
	}
}
