package layout_test

import (
	"fmt"

	"github.com/itbm/bench/internal/layout"
)

func ExampleFrames() {
	root := layout.NewSplit("s1", layout.Row, []float64{65, 35},
		layout.NewPane("p1", "w1"),
		layout.NewPane("p2", "w2"))
	for _, f := range layout.Frames(root, layout.Rect{X: 0, Y: 25, W: 2560, H: 1415}, 8) {
		fmt.Println(f.ID, f.Rect)
	}
	// Output:
	// p1 {8 33 1651 1399}
	// p2 {1667 33 885 1399}
}

func ExampleApplyPreset() {
	seq := 0
	newID := func(nodeType string) string {
		seq++
		if nodeType == layout.TypeSplit {
			return fmt.Sprint("s", seq)
		}
		return fmt.Sprint("p", seq)
	}
	old := layout.NewSplit("s1", layout.Row, []float64{40, 30, 30},
		layout.NewPane("p1", "w1"),
		layout.NewPane("p2", "w2"),
		layout.NewPane("p3", "w3", "w4"))
	root, err := layout.ApplyPreset(old, layout.PresetSide, nil, newID)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range layout.Leaves(root) {
		fmt.Println(p.ID, p.Items, p.Active)
	}
	// Output:
	// p1 [w1] w1
	// p2 [w2 w3 w4] w2
}
