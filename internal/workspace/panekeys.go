package workspace

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/itbm/bench/internal/layout"
)

// This file has the keys that arrange the current tab's panes from any app
// (decision 59): Grow Pane and Shrink Pane, which make the focused pane
// larger or smaller along the split that holds it; Make Panes Equal, which
// gives every split of the tab equal sizes; and Next Layout and Previous
// Layout, which give the tab the next or previous of the six layouts,
// keeping its windows in order, as Tab properties does. Each has a
// shortcut, off until the user sets it, a row in the jump palette, and an
// item in the Pane menu, or the Tab menu for the layouts. Each says what it
// did in a toast, with Undo. A run of them, one after another, with
// nothing else changing the tab in between, shares one undo step, so that
// Undo puts the tab back as it was before the first.

// growStep is how much Grow Pane and Shrink Pane change the focused pane's
// size by, in percent of the split that holds it.
const growStep = 5

// arrangeRunFor is how long after one of these changes the next shares its
// undo step: as long as the toast with its Undo shows (ACTION_TOAST_MS in
// the frontend's state.js), unless the pointer or the keyboard holds it
// there (holdToast).
const arrangeRunFor = 10 * time.Second

// Refusals of the keys that arrange the panes.
var (
	errArrangeTabsView = errors.New("In the compact layout, this tab shows its windows as one pane. Choose Wide to arrange its panes.")
	errResizeZoomed    = errors.New("This pane is zoomed, so it fills the tab. Show all panes first.")
	errResizeOnePane   = errors.New("This tab has only one pane, so there's nothing to resize.")
	errAlreadyEqual    = errors.New("The panes are the same size already.")
)

// errPaneLimit says that the focused pane can't grow, or shrink, any more
// along its split, which is drawn as a row or a column.
func errPaneLimit(dir layout.Dir, grow bool) error {
	var size string
	switch {
	case dir == layout.Row && grow:
		size = "wide"
	case dir == layout.Row:
		size = "narrow"
	case grow:
		size = "tall"
	default:
		size = "short"
	}
	return fmt.Errorf("This pane is as %s as it can be.", size)
}

// errNoLayoutFits says that no layout has a pane for each of the tab's n
// panes that have windows in them.
func errNoLayoutFits(n int) error {
	return fmt.Errorf("No layout has room for this tab's %d panes. Move windows into fewer panes first.", n)
}

// errOnlyLayout says that the tab's own layout, called label, is the only
// one with a pane for each of its n panes that have windows in them.
func errOnlyLayout(label string, n int) error {
	return fmt.Errorf("%s is the only layout with room for this tab's %d panes.", label, n)
}

// arrangeRun is the last change that the keys that arrange the panes made:
// its undo step, its tab, the tab's tree and preset just after it, when it
// was made, and whether it, or one before it in the run, changed the
// layout.
type arrangeRun struct {
	step   *undoStep
	tab    *Tab
	root   *layout.Node
	preset string
	at     time.Time
	layout bool
}

// GrowPane makes the current tab's focused pane growStep larger along the
// split that holds it, taking the room from the split's other panes, and
// says how they share it, with Undo: the grow_pane shortcut. No pane goes
// below layout.MinSize. It's refused in the "tabs" view, while the pane is
// zoomed, for a tab with one pane, and for a pane as large as it can be.
func (e *Engine) GrowPane() error {
	e.lock()
	defer e.unlock()
	return e.resizeFocusedLocked(growStep)
}

// ShrinkPane makes the focused pane growStep smaller, as GrowPane makes it
// larger, giving the room to the split's other panes: the shrink_pane
// shortcut.
func (e *Engine) ShrinkPane() error {
	e.lock()
	defer e.unlock()
	return e.resizeFocusedLocked(-growStep)
}

func (e *Engine) resizeFocusedLocked(step float64) error {
	t := e.model.CurrentTab()
	split, sizes, err := e.resizeLocked(t, step)
	if err != nil {
		return err
	}
	pane := "the empty pane"
	if p := t.FocusedPane(); len(p.Items) > 0 {
		pane = e.paneNameLocked(p)
	}
	name := undoName{"Grow Pane", "made " + pane + " larger"}
	if step < 0 {
		name = undoName{"Shrink Pane", "made " + pane + " smaller"}
	}
	s := e.beginArrangeLocked(t)
	if err := e.setSizesLocked(split.ID, sizes); err != nil {
		return err
	}
	e.arrangeToastLocked(t, s, false, name, e.sizesTextLocked(split))
	return nil
}

// resizeLocked returns the split that holds tab t's focused pane, and its
// sizes with the pane step percentage points larger, or smaller for a
// negative step (layout.Resize), or why it can't be done.
func (e *Engine) resizeLocked(t *Tab, step float64) (*layout.Node, []float64, error) {
	if t == nil {
		return nil, nil, errNoPane
	}
	if e.isTabsView(t) {
		return nil, nil, errArrangeTabsView
	}
	if e.zoomedLocked() != nil {
		return nil, nil, errResizeZoomed
	}
	p := t.FocusedPane()
	if p == nil {
		return nil, nil, errNoPane
	}
	_, split, i := layout.Find(t.Root, p.ID)
	if split == nil {
		return nil, nil, errResizeOnePane
	}
	sizes, ok := layout.Resize(split.Sizes, i, step)
	if !ok {
		// The stack view draws the tab's top-level row as a column.
		dir := split.Dir
		if split == t.Root && e.stackViewLocked(t) {
			dir = layout.Column
		}
		return nil, nil, errPaneLimit(dir, step > 0)
	}
	return split, sizes, nil
}

// EqualPanes gives every split of the current tab equal sizes, as a
// double-click on each of its dividers would, and says so, with Undo: the
// equal_panes shortcut. A zoomed pane shows every pane again, to show what
// changed. It's refused in the "tabs" view, for a tab with one pane, and
// when the panes have equal sizes already.
func (e *Engine) EqualPanes() error {
	e.lock()
	defer e.unlock()
	t := e.model.CurrentTab()
	splits, err := e.unevenLocked(t)
	if err != nil {
		return err
	}
	zoomed := e.endZoomLocked()
	s := e.beginArrangeLocked(t)
	for _, n := range splits {
		n.Sizes = layout.Normalise(nil, len(n.Children)) // Equal sizes.
	}
	e.arrangeLocked(arrangeOpts{})
	if zoomed {
		e.focusShownLocked()
	}
	e.changed()
	e.arrangeToastLocked(t, s, false, undoName{"Make Panes Equal", "made the panes the same size"}, "Made the panes the same size.")
	return nil
}

// unevenLocked returns tab t's splits whose children don't look equal in
// size already (nearSize), or why there are none to make equal.
func (e *Engine) unevenLocked(t *Tab) ([]*layout.Node, error) {
	if t == nil {
		return nil, errNoPane
	}
	if e.isTabsView(t) {
		return nil, errArrangeTabsView
	}
	if len(t.Leaves()) < 2 {
		return nil, errResizeOnePane
	}
	var out []*layout.Node
	var walk func(n *layout.Node)
	walk = func(n *layout.Node) {
		if !n.IsSplit() {
			return
		}
		even := layout.Normalise(nil, len(n.Children))
		if !slices.EqualFunc(layout.Normalise(n.Sizes, len(n.Children)), even, nearSize) {
			out = append(out, n)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(t.Root)
	if len(out) == 0 {
		return nil, errAlreadyEqual
	}
	return out, nil
}

// StepLayout gives the current tab the next (a positive step) or previous
// (a negative one) of the six layouts, in Tab properties' order, wrapping,
// with the layout's own sizes, and says which, with Undo: the next_layout
// and previous_layout shortcuts. It keeps the windows in order, as Tab
// properties does (layout.ApplyPreset): the first pane's windows go to the
// new first pane, and so on. A layout with too few panes for the tab's
// windows is skipped, so that no two panes' windows end up in one; empty
// panes go first, unless a new window is on its way to one, or the add
// dialog is open for it. A tab whose tree has changed goes on from the
// layout whose shape it still has (layoutOf). A zoomed pane shows every
// pane again, to show the new layout. It's refused in the "tabs" view, and
// when no other layout has room for the tab's panes.
func (e *Engine) StepLayout(step int) error {
	e.lock()
	defer e.unlock()
	t := e.model.CurrentTab()
	p, err := e.nextLayoutLocked(t, step)
	if err != nil {
		return err
	}
	zoomed := e.endZoomLocked()
	shown := e.focusedShownLocked()
	s := e.beginArrangeLocked(t)
	root, err := layout.ApplyPreset(e.withoutEmptyLocked(t), p.ID, nil, e.model.nodeID)
	if err != nil {
		return errNoPreset(p.ID)
	}
	t.Root, t.Preset = root, p.ID
	t.fixFocus()
	e.arrangeLocked(arrangeOpts{})
	// The keyboard follows the focused pane, so if it shows another item
	// now, that item gets it.
	if zoomed || e.focusedShownLocked() != shown {
		e.focusShownLocked()
	}
	e.changed()
	name := undoName{"Next Layout", "changed the layout of " + t.Name}
	if step < 0 {
		name.name = "Previous Layout"
	}
	e.arrangeToastLocked(t, s, true, name, "Layout: "+p.Label)
	return nil
}

// nextLayoutLocked returns the layout that StepLayout(step) would give tab
// t: the next of the six after the tab's own (layoutOf), or the previous,
// wrapping, or the first or last for a tree with no layout's shape, with a
// pane for each of the tab's panes that it keeps (keptPanesLocked); or why
// there's none.
func (e *Engine) nextLayoutLocked(t *Tab, step int) (layout.Preset, error) {
	if t == nil {
		return layout.Preset{}, errNoPane
	}
	if e.isTabsView(t) {
		return layout.Preset{}, errArrangeTabsView
	}
	if step >= 0 {
		step = 1
	} else {
		step = -1
	}
	presets := layout.Presets()
	n := len(presets)
	need := max(1, len(e.keptPanesLocked(t)))
	own := layoutOf(t)
	at := slices.IndexFunc(presets, func(p layout.Preset) bool { return p.ID == own })
	if at < 0 && step < 0 {
		at = n
	}
	for k := 1; k <= n; k++ {
		j := ((at+step*k)%n + n) % n
		if presets[j].Slots < need {
			continue
		}
		if j == at {
			return layout.Preset{}, errOnlyLayout(presets[j].Label, need)
		}
		return presets[j], nil
	}
	return layout.Preset{}, errNoLayoutFits(need)
}

// layoutOf returns the ID of tab t's layout, for Next Layout and Previous
// Layout to go on from: its preset, or for a custom tab the preset it had
// (FromPreset), while its tree has that preset's shape (layout.HasShape),
// or else the layout whose shape the tree has (layout.PresetOf). A tab
// marked custom, because a window moved into it or out of it or between
// its panes, often still has its layout's shape. Without FromPreset, as
// from an older state.json, the sizes tell Side by side from Main and
// side. It returns "" for a tree with no layout's shape.
func layoutOf(t *Tab) string {
	for _, id := range []string{t.Preset, t.FromPreset} {
		if layout.HasShape(t.Root, id) {
			return id
		}
	}
	id, _ := layout.PresetOf(t.Root)
	return id
}

// keptPanesLocked returns the IDs of tab t's panes that a new layout keeps:
// those with items in them, the empty ones that a new window is on its way
// to (awaitedLocked), and the empty one that SplitPane made for the add
// dialog, which is still open to fill it (Engine.split).
func (e *Engine) keptPanesLocked(t *Tab) []string {
	var out []string
	for _, p := range t.Leaves() {
		if len(p.Items) > 0 || e.awaitedLocked(t, p.ID) || e.split.tab == t && e.split.pane == p.ID {
			out = append(out, p.ID)
		}
	}
	return out
}

// withoutEmptyLocked returns a copy of tab t's tree without the panes that
// a new layout doesn't keep (keptPanesLocked), but with one pane at least.
func (e *Engine) withoutEmptyLocked(t *Tab) *layout.Node {
	keep := e.keptPanesLocked(t)
	root := layout.Clone(t.Root)
	for _, p := range layout.Leaves(root) {
		if slices.Contains(keep, p.ID) || len(layout.Leaves(root)) < 2 {
			continue
		}
		if r, err := layout.RemovePane(root, p.ID); err == nil {
			root = r
		}
	}
	return root
}

// beginArrangeLocked returns the undo step for a change that the keys that
// arrange the panes are about to make to tab t: the last such change's, if
// it was to the same tab less than arrangeRunFor ago, no change has kept a
// step since, and nothing has changed the tab's tree or layout since; or
// else a new one (beginUndoLocked).
func (e *Engine) beginArrangeLocked(t *Tab) *undoStep {
	r := e.arranged
	if r.step != nil && r.tab == t && len(e.undo) > 0 && e.undo[len(e.undo)-1] == r.step &&
		e.clk.Now().Sub(r.at) < arrangeRunFor && t.Preset == r.preset && reflect.DeepEqual(t.Root, r.root) {
		return r.step
	}
	e.arranged = arrangeRun{}
	return e.beginUndoLocked()
}

// arrangeToastLocked keeps the undo step of a change that the keys that
// arrange the panes made to tab t, unless it shares the last one's, and
// says what it did, msg, with Undo. newLayout is set for a change of
// layout: Undo's own toast then says that it put the layout back. name
// names the change for Undo away from its toast (UndoLatest). A run keeps
// it while each change in it has the same name, as Grow Pane pressed again
// for the same pane does; a run of different changes is named for all of
// them, as Undo takes them all back: Change Layout if one of them changed
// the layout, or else Resize Panes.
func (e *Engine) arrangeToastLocked(t *Tab, s *undoStep, newLayout bool, name undoName, msg string) {
	shared := e.arranged.step == s
	newLayout = newLayout || shared && e.arranged.layout
	undone := "Put back the sizes of the panes."
	if newLayout {
		undone = "Put back the layout."
	}
	id := s.id
	if shared {
		if s.name != name {
			name = resizePanesName
			if newLayout {
				name = undoName{"Change Layout", "changed the layout of " + t.Name}
			}
		}
		s.name, s.undone = name, undone
		// The View names the latest change.
		e.touch()
	} else {
		id = e.keepUndoLocked(s, name, undone)
	}
	e.arranged = arrangeRun{step: s, tab: t, root: layout.Clone(t.Root), preset: t.Preset, at: e.clk.Now(), layout: newLayout}
	e.undoToastLocked(msg, id)
}

// resizePanesName names a change to the sizes of a tab's panes for Undo away
// from its toast, where no command names it: a run of the keys that arrange
// the panes, such as Grow Pane for one pane and then another, or a drag of a
// docked window's edge.
var resizePanesName = undoName{"Resize Panes", "resized the panes"}

// sizesTextLocked says how a split's children share it, in its order, such
// as "Zed 55% · Terminal 45%", for the toasts of Grow Pane and Shrink Pane.
// The percentages are whole numbers that add up to 100.
func (e *Engine) sizesTextLocked(split *layout.Node) string {
	pct := percents(layout.Normalise(split.Sizes, len(split.Children)))
	parts := make([]string, len(split.Children))
	for i, c := range split.Children {
		parts[i] = e.nodeNameLocked(c) + " " + strconv.Itoa(pct[i]) + "%"
	}
	return strings.Join(parts, " · ")
}

// nodeNameLocked names a child of a split for sizesTextLocked: a pane by
// what it shows (paneNameLocked), and a split by its panes, as "Zed and
// Terminal".
func (e *Engine) nodeNameLocked(n *layout.Node) string {
	if n.IsPane() {
		return e.paneNameLocked(n)
	}
	var names []string
	for _, p := range layout.Leaves(n) {
		names = append(names, e.paneNameLocked(p))
	}
	return joinNames(names)
}

// paneNameLocked names a pane by the item it shows, as drawn: the name the
// user gave it, if any, or else its app's, such as "Zed" or "Terminal". A
// pane that only keeps places for an app's windows is named by the app,
// and one with nothing in it is "Empty pane".
func (e *Engine) paneNameLocked(p *layout.Node) string {
	shown := e.shownItemsLocked(p.Items)
	id := p.Active
	if !slices.Contains(shown, id) {
		id = e.recentOfLocked(shown)
	}
	if id == "" && len(p.Items) > 0 {
		id = p.Items[0]
	}
	it := e.model.Items[id]
	switch {
	case it == nil:
		return "Empty pane"
	case it.Label != "":
		return it.Label
	}
	return it.App
}

// percents rounds sizes that add up to 100 to whole numbers that add up to
// 100 too: each rounds down, and the ones that lost the most round up.
func percents(sizes []float64) []int {
	out := make([]int, len(sizes))
	left := 100
	for i, s := range sizes {
		out[i] = int(math.Floor(s))
		left -= out[i]
	}
	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ra, rb := sizes[a]-math.Floor(sizes[a]), sizes[b]-math.Floor(sizes[b])
		switch {
		case ra > rb:
			return -1
		case ra < rb:
			return 1
		}
		return 0
	})
	for k := 0; k < left && k < len(order); k++ {
		out[order[k]]++
	}
	return out
}

// arrangeViewLocked says what the keys that arrange the panes would do now
// to tab t, the current tab, for the jump palette and the menus, which
// leave out, or turn off, what would be refused.
func (e *Engine) arrangeViewLocked(t *Tab) ArrangeView {
	var a ArrangeView
	if t == nil || e.isTabsView(t) {
		return a
	}
	_, _, err := e.resizeLocked(t, growStep)
	a.Grow = err == nil
	_, _, err = e.resizeLocked(t, -growStep)
	a.Shrink = err == nil
	_, err = e.unevenLocked(t)
	a.Equal = err == nil
	if p, err := e.nextLayoutLocked(t, 1); err == nil {
		a.NextLayout = p.Label
	}
	if p, err := e.nextLayoutLocked(t, -1); err == nil {
		a.PreviousLayout = p.Label
	}
	return a
}
