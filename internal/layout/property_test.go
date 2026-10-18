package layout

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

// TestRandomEdits applies random sequences of Insert, TakeItem, Split,
// SetSizes, ApplyPreset, and moves (TakeItem, then Insert or Split with the
// same item), and checks after every step that the tree is valid, holds
// exactly the items it should, has sizes adding up to 100, and lays out
// sensibly.
func TestRandomEdits(t *testing.T) {
	seeds := uint64(200)
	if testing.Short() {
		seeds = 20
	}
	for seed := range seeds {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			r := rand.New(rand.NewPCG(seed, 7))
			e := newEditor(r)
			for step := range 150 {
				op, err := e.step()
				if err != nil {
					t.Fatalf("step %d: %v\ntree: %s", step, err, describe(e.root))
				}
				if err := Validate(e.root); err != nil {
					t.Fatalf("step %d (%s): %v\ntree: %s", step, op, err, describe(e.root))
				}
				got := Items(e.root)
				if len(got) != len(e.items) {
					t.Fatalf("step %d (%s): items %v, want %v", step, op, got, slices.Sorted(maps.Keys(e.items)))
				}
				for _, item := range got {
					if !e.items[item] {
						t.Fatalf("step %d (%s): unexpected item %q in %v", step, op, item, got)
					}
				}
				if err := checkLaidOut(e.root); err != nil {
					t.Fatalf("step %d (%s): %v\ntree: %s", step, op, err, describe(e.root))
				}
			}
		})
	}
}

// checkLaidOut checks what every edit should leave true of a valid tree:
// each split's sizes add up to 100; frames have no negative sizes; there's
// one divider per neighbouring pair of children; and Adapt gives a valid
// tree without changing its input.
func checkLaidOut(root *Node) error {
	dividers := 0
	var bad error
	walk(root, func(n *Node) {
		if !n.IsSplit() || bad != nil {
			return
		}
		dividers += len(n.Children) - 1
		var sum float64
		for _, s := range n.Sizes {
			sum += s
		}
		if math.Abs(sum-100) > 1e-9 {
			bad = fmt.Errorf("split %s has sizes %v, which add up to %v", n.ID, n.Sizes, sum)
		}
	})
	if bad != nil {
		return bad
	}
	area := Rect{0, 25, 2560, 1415}
	for _, f := range Frames(root, area, 8) {
		if f.Rect.W < 0 || f.Rect.H < 0 {
			return fmt.Errorf("pane %s has frame %v", f.ID, f.Rect)
		}
	}
	if got := len(Dividers(root, area, 8)); got != dividers {
		return fmt.Errorf("%d dividers, want %d", got, dividers)
	}
	before := Clone(root)
	for _, setting := range []SmallScreens{Stack, Tabs, Same} {
		focused := Leaves(root)[0].ID
		adapted := Adapt(root, setting, Compact, focused, "")
		if err := Validate(adapted); err != nil {
			return fmt.Errorf("Adapt(%s) isn't valid: %w", setting, err)
		}
		if !slices.Equal(Items(adapted), Items(root)) {
			return fmt.Errorf("Adapt(%s) has items %v, want %v", setting, Items(adapted), Items(root))
		}
	}
	if !reflect.DeepEqual(root, before) {
		return errors.New("Adapt changed its input")
	}
	return nil
}

// editor holds a tree being edited at random, and the items it should hold.
type editor struct {
	r     *rand.Rand
	root  *Node
	items map[string]bool
	seq   int
}

func newEditor(r *rand.Rand) *editor {
	e := &editor{r: r, items: make(map[string]bool)}
	e.root = NewPane(e.newID(TypePane))
	return e
}

// newID is the editor's NewID.
func (e *editor) newID(nodeType string) string {
	e.seq++
	if nodeType == TypeSplit {
		return fmt.Sprintf("s%d", e.seq)
	}
	return fmt.Sprintf("p%d", e.seq)
}

// newItem returns a fresh item ID and records it.
func (e *editor) newItem() string {
	e.seq++
	id := fmt.Sprintf("w%d", e.seq)
	e.items[id] = true
	return id
}

// randomPane returns a random pane's ID.
func (e *editor) randomPane() string {
	leaves := Leaves(e.root)
	return leaves[e.r.IntN(len(leaves))].ID
}

// step makes one random edit and describes it.
func (e *editor) step() (string, error) {
	switch e.r.IntN(8) {
	case 0, 1:
		pane, item := e.randomPane(), e.newItem()
		if err := Insert(e.root, pane, item); err != nil {
			return "", fmt.Errorf("Insert(%s, %s): %w", pane, item, err)
		}
		return "insert " + item + " into " + pane, nil
	case 2:
		all := Items(e.root)
		if len(all) == 0 {
			return "nothing to take", nil
		}
		item := all[e.r.IntN(len(all))]
		root, _, ok := TakeItem(e.root, item)
		if !ok {
			return "", fmt.Errorf("TakeItem didn't find %s", item)
		}
		e.root = root
		delete(e.items, item)
		return "take " + item, nil
	case 3:
		pane := e.randomPane()
		zone := []Zone{Left, Right, Top, Bottom}[e.r.IntN(4)]
		fresh := NewPane(e.newID(TypePane), e.newItem())
		root, err := Split(e.root, pane, zone, fresh, e.newID)
		if err != nil {
			return "", fmt.Errorf("Split(%s, %s): %w", pane, zone, err)
		}
		e.root = root
		return fmt.Sprintf("split %s %s", pane, zone), nil
	case 4:
		var splits []*Node
		walk(e.root, func(n *Node) {
			if n.IsSplit() {
				splits = append(splits, n)
			}
		})
		if len(splits) == 0 {
			return "no split to size", nil
		}
		s := splits[e.r.IntN(len(splits))]
		sizes := make([]float64, len(s.Children))
		for i := range sizes {
			sizes[i] = 1 + e.r.Float64()*99
		}
		if err := SetSizes(e.root, s.ID, sizes); err != nil {
			return "", fmt.Errorf("SetSizes(%s, %v): %w", s.ID, sizes, err)
		}
		return fmt.Sprintf("size %s %v", s.ID, sizes), nil
	case 6, 7:
		return e.move()
	default:
		presets := Presets()
		p := presets[e.r.IntN(len(presets))]
		before := Items(e.root)
		root, err := ApplyPreset(e.root, p.ID, nil, e.newID)
		if err != nil {
			return "", fmt.Errorf("ApplyPreset(%s): %w", p.ID, err)
		}
		if !slices.Equal(Items(root), before) {
			return "", fmt.Errorf("ApplyPreset(%s) changed the item order from %v to %v", p.ID, before, Items(root))
		}
		e.root = root
		return "apply " + p.ID, nil
	}
}

// move takes a random item out of its pane and puts it back elsewhere: into
// a random pane, or beside one at a random edge, as dragging a sub-tab does.
// Like the workspace, it doesn't split a pane by its only item.
func (e *editor) move() (string, error) {
	all := Items(e.root)
	if len(all) == 0 {
		return "nothing to move", nil
	}
	item := all[e.r.IntN(len(all))]
	target := e.randomPane()
	zone := []Zone{Center, Left, Right, Top, Bottom}[e.r.IntN(5)]
	if from := PaneOf(e.root, item); from.ID == target && (zone == Center || len(from.Items) == 1) {
		return "no move of " + item, nil
	}
	root, _, ok := TakeItem(e.root, item)
	if !ok {
		return "", fmt.Errorf("TakeItem didn't find %s", item)
	}
	e.root = root
	if n, _, _ := Find(e.root, target); !n.IsPane() {
		// Only the item's own pane can go, and then it isn't the target.
		return "", fmt.Errorf("taking %s removed pane %s", item, target)
	}
	if zone == Center {
		if err := Insert(e.root, target, item); err != nil {
			return "", fmt.Errorf("Insert(%s, %s): %w", target, item, err)
		}
		return fmt.Sprintf("move %s into %s", item, target), nil
	}
	root, err := Split(e.root, target, zone, NewPane(e.newID(TypePane), item), e.newID)
	if err != nil {
		return "", fmt.Errorf("Split(%s, %s) with %s: %w", target, zone, item, err)
	}
	e.root = root
	return fmt.Sprintf("move %s to the %s of %s", item, zone, target), nil
}
