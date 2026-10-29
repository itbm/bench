// Package layout turns a tab's layout tree into pane rectangles.
//
// It's pure Go with no dependencies outside the standard library. Everything
// is in points, with a top-left origin and y increasing downwards.
package layout

// Rect is a rectangle in points.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Dir is the direction a split divides its space in.
type Dir string

const (
	Row    Dir = "row"    // Children side by side, left to right.
	Column Dir = "column" // Children stacked, top to bottom.
)

// Node types.
const (
	TypeSplit = "split"
	TypePane  = "pane"
)

// CompactPaneID is the ID of the single pane shown by the "tabs" small-screen
// setting. It isn't part of any stored tree.
const CompactPaneID = "compact"

// Node is a split or a pane in a tab's layout tree.
//
// A split has a direction, a size for each child as a percentage, and two or
// more children. A pane holds an ordered list of item IDs and shows one of
// them (Active). The JSON shape matches the prototype's state and the
// bench:state event.
type Node struct {
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	Dir      Dir       `json:"dir,omitempty"`
	Sizes    []float64 `json:"sizes,omitempty"`
	Children []*Node   `json:"children,omitempty"`
	Items    []string  `json:"items,omitempty"`
	Active   string    `json:"active,omitempty"`
}

// IsPane reports whether n is a pane.
func (n *Node) IsPane() bool { return n != nil && n.Type == TypePane }

// IsSplit reports whether n is a split.
func (n *Node) IsSplit() bool { return n != nil && n.Type == TypeSplit }

// NewID returns a fresh, unique ID for a node of the given type (TypePane or
// TypeSplit). Callers own the ID scheme; the workspace uses "p1", "s2", ...
type NewID func(nodeType string) string

// Zone is where a dragged item lands on a pane.
type Zone string

const (
	Center Zone = "center"
	Left   Zone = "left"
	Right  Zone = "right"
	Top    Zone = "top"
	Bottom Zone = "bottom"
)

// Direction is a keyboard direction, for focusing and moving between panes.
type Direction string

const (
	DirLeft  Direction = "left"
	DirRight Direction = "right"
	DirUp    Direction = "up"
	DirDown  Direction = "down"
)

// SizeClass is the screen's size class.
type SizeClass string

const (
	Wide    SizeClass = "wide"
	Compact SizeClass = "compact"
)

// Mode is the user's layout choice in the title bar.
type Mode string

const (
	ModeAuto    Mode = "auto"
	ModeWide    Mode = "wide"
	ModeCompact Mode = "compact"
)

// SmallScreens is a tab's small-screen setting.
type SmallScreens string

const (
	Stack SmallScreens = "stack" // The top-level row becomes a column.
	Tabs  SmallScreens = "tabs"  // One pane holding every item as a sub-tab.
	Same  SmallScreens = "same"  // No change.
)

// Preset is a starting tree for a tab.
type Preset struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	Slots        int       `json:"slots"`
	DefaultSizes []float64 `json:"defaultSizes"`
}

// Preset IDs, in the order Tab properties shows them.
const (
	PresetSingle    = "single"
	PresetSide      = "side"
	PresetMainSide  = "main-side"
	PresetThree     = "three"
	PresetTopBottom = "top-bottom"
	PresetGrid      = "grid"
	// PresetCustom marks a tab whose tree has been edited by moving items.
	// It isn't a real preset and can't be built.
	PresetCustom = "custom"
)

// PaneRect is a pane's rectangle, by pane ID.
type PaneRect struct {
	ID   string `json:"id"`
	Rect Rect   `json:"rect"`
}

// DividerRect is the gap between two neighbouring children of a split, where
// the user can drag to resize them.
type DividerRect struct {
	SplitID string    `json:"splitId"`
	Index   int       `json:"index"` // Between child Index and child Index+1.
	Dir     Dir       `json:"dir"`
	Rect    Rect      `json:"rect"`      // The gap itself, at least 1 pt across.
	Split   Rect      `json:"splitRect"` // The split's whole frame, for turning a drag into sizes.
	Sizes   []float64 `json:"sizes"`     // The split's current sizes, as percentages.
}

// Pane chrome, in points. A pane's body (where its item is shown) is the pane
// rectangle inset by its border, below the header and the header's divider.
// A header is HeaderHeight tall, or CompactHeaderHeight in the compact
// density. A pane can also have no header, and then has no divider either:
// the "tabs" view's pane, whose sub-tabs are in Bench's title bar, and a pane
// that shows one App window, whose own title bar stands in for it.
const (
	PaneBorder          = 1
	HeaderHeight        = 40
	CompactHeaderHeight = 32
	HeaderDivider       = 1
)
