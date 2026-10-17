package layout

import (
	"errors"
	"fmt"
	"slices"
)

// ErrUnknownPreset is wrapped by the errors Build and ApplyPreset return for
// a preset ID they don't know, including PresetCustom.
var ErrUnknownPreset = errors.New("unknown preset")

// presets are the six presets in editor order. Every preset but single is a
// split whose top-level direction is topDir.
var presets = []struct {
	Preset
	topDir Dir
}{
	{Preset{ID: PresetSingle, Label: "Single", Slots: 1}, ""},
	{Preset{ID: PresetSide, Label: "Side by side", Slots: 2, DefaultSizes: []float64{50, 50}}, Row},
	{Preset{ID: PresetMainSide, Label: "Main and side", Slots: 2, DefaultSizes: []float64{65, 35}}, Row},
	{Preset{ID: PresetThree, Label: "Three columns", Slots: 3, DefaultSizes: []float64{40, 30, 30}}, Row},
	{Preset{ID: PresetTopBottom, Label: "Top and bottom", Slots: 2, DefaultSizes: []float64{65, 35}}, Column},
	{Preset{ID: PresetGrid, Label: "Grid of four", Slots: 4, DefaultSizes: []float64{50, 50}}, Column},
}

// copyPreset returns p with its own DefaultSizes. Single has an empty,
// non-nil slice, so it's [] in JSON.
func copyPreset(p Preset) Preset {
	p.DefaultSizes = append([]float64{}, p.DefaultSizes...)
	return p
}

// Presets returns the six presets in editor order.
func Presets() []Preset {
	out := make([]Preset, len(presets))
	for i, p := range presets {
		out[i] = copyPreset(p.Preset)
	}
	return out
}

// LookupPreset returns the preset with the given ID.
func LookupPreset(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return copyPreset(p.Preset), true
		}
	}
	return Preset{}, false
}

// topDirOf returns the direction of a preset's top-level split.
func topDirOf(id string) Dir {
	for _, p := range presets {
		if p.ID == id {
			return p.topDir
		}
	}
	return ""
}

// Build returns the tree for a preset, placing panes in reading order and
// padding with new empty panes. Sizes apply to the top-level split; nil or a
// wrong length means the preset's defaults, as do sizes that aren't all
// positive numbers. It's an error to pass more panes than the preset has, or
// an unknown preset.
//
// The tree holds the given panes themselves, not copies. Build asks newID for
// the padding panes' IDs first, then for the splits' IDs, outermost first and
// then in reading order. It's an error if the result isn't a valid tree (see
// Validate), for example because two panes hold the same item.
func Build(presetID string, panes []*Node, sizes []float64, newID NewID) (*Node, error) {
	root, err := build(presetID, panes, sizes, newID)
	if err != nil {
		return nil, fmt.Errorf("build preset %q: %w", presetID, err)
	}
	return root, nil
}

// build is Build without the error context.
func build(presetID string, panes []*Node, sizes []float64, newID NewID) (*Node, error) {
	p, ok := LookupPreset(presetID)
	if !ok {
		return nil, ErrUnknownPreset
	}
	if len(panes) > p.Slots {
		return nil, fmt.Errorf("the preset has %d panes, but got %d", p.Slots, len(panes))
	}
	for i, n := range panes {
		if !n.IsPane() {
			return nil, fmt.Errorf("pane %d isn't a pane", i+1)
		}
	}
	ids := idMaker{newID: newID}
	slots := slices.Clone(panes)
	for len(slots) < p.Slots {
		slots = append(slots, NewPane(ids.next(TypePane)))
	}
	if len(sizes) != len(p.DefaultSizes) || !allPositive(sizes) {
		sizes = p.DefaultSizes
	}
	root := shape(presetID, slots, sizes, &ids)
	if ids.err != nil {
		return nil, ids.err
	}
	if err := Validate(root); err != nil {
		return nil, err
	}
	return root, nil
}

// shape arranges a preset's panes into its tree.
func shape(presetID string, slots []*Node, sizes []float64, ids *idMaker) *Node {
	switch presetID {
	case PresetSingle:
		return slots[0]
	case PresetGrid:
		outer := ids.next(TypeSplit)
		top := NewSplit(ids.next(TypeSplit), Row, []float64{50, 50}, slots[0], slots[1])
		bottom := NewSplit(ids.next(TypeSplit), Row, []float64{50, 50}, slots[2], slots[3])
		return NewSplit(outer, Column, sizes, top, bottom)
	default:
		return NewSplit(ids.next(TypeSplit), topDirOf(presetID), sizes, slots...)
	}
}

// idMaker calls a NewID and remembers the first problem, so a tree can be
// put together before checking for errors.
type idMaker struct {
	newID NewID
	err   error
}

// next returns a new ID of the given type, or "" after a problem.
func (m *idMaker) next(nodeType string) string {
	if m.err != nil {
		return ""
	}
	if m.newID == nil {
		m.err = fmt.Errorf("a new %s needs an ID, but there's no ID function", nodeType)
		return ""
	}
	id := m.newID(nodeType)
	if id == "" {
		m.err = fmt.Errorf("the ID function returned an empty %s ID", nodeType)
	}
	return id
}

// ApplyPreset replaces a tree with a preset, keeping every item in order:
// old pane 1's items go into new pane 1, and so on, and the extra panes'
// items join the last new pane. New panes reuse the old panes' IDs in reading
// order where they can, so focus survives. Each new pane keeps the active
// item of the first old pane that lands in it: the first one showing an
// item, so that an empty old pane doesn't lose the next one's. A new pane
// with no active item shows its first item. Sizes are as for Build.
//
// Old isn't changed, and may be nil. ApplyPreset asks newID for the IDs of
// the new panes that have no old pane, in reading order, then Build asks it
// for the splits' IDs.
func ApplyPreset(old *Node, presetID string, sizes []float64, newID NewID) (*Node, error) {
	p, ok := LookupPreset(presetID)
	if !ok {
		return nil, fmt.Errorf("apply preset %q: %w", presetID, ErrUnknownPreset)
	}
	leaves := Leaves(old)
	ids := idMaker{newID: newID}
	panes := make([]*Node, p.Slots)
	for i := range panes {
		var id string
		if i < len(leaves) {
			id = leaves[i].ID
		} else {
			id = ids.next(TypePane)
		}
		panes[i] = &Node{Type: TypePane, ID: id}
	}
	if ids.err != nil {
		return nil, fmt.Errorf("apply preset %q: %w", presetID, ids.err)
	}
	for j, leaf := range leaves {
		target := panes[min(j, p.Slots-1)]
		target.Items = append(target.Items, leaf.Items...)
		if target.Active == "" && leaf.Active != "" && slices.Contains(leaf.Items, leaf.Active) {
			target.Active = leaf.Active
		}
	}
	for _, pane := range panes {
		if pane.Active == "" && len(pane.Items) > 0 {
			pane.Active = pane.Items[0]
		}
	}
	root, err := build(presetID, panes, sizes, newID)
	if err != nil {
		return nil, fmt.Errorf("apply preset %q: %w", presetID, err)
	}
	return root, nil
}
