// Package shortcuts turns the [shortcuts] config into global shortcut
// bindings, and formats shortcuts for display.
//
// A shortcut is written in config.toml as modifiers and a key joined with
// "+", such as "ctrl+alt+space". Parse checks one and returns its canonical
// form; Bindings expands the whole [shortcuts] table into the shortcuts Bench
// registers; Display formats a shortcut the way macOS menus do ("⌃⌥Space");
// Suggest proposes another shortcut when one can't be registered; and
// UsesVoiceOverKeys and Reserved say which clash with VoiceOver and macOS.
//
// Every function is safe for concurrent use: the package holds no state
// apart from read-only tables.
package shortcuts

// Config is the [shortcuts] table of config.toml (config.Shortcuts). Each
// field is a shortcut such as "ctrl+alt+space", or modifiers such as
// "ctrl+alt" that combine with keys (1 to 9, the arrow keys, or 1 to 9 and
// `). An empty field turns that shortcut off. ShowAllTabs, and the fields
// that arrange the current tab's panes, from GrowPane to PreviousLayout,
// are off by default.
type Config struct {
	TabModifiers        string `toml:"tab_modifiers" json:"tabModifiers"`
	NextTab             string `toml:"next_tab" json:"nextTab"`
	PreviousTab         string `toml:"previous_tab" json:"previousTab"`
	LastTab             string `toml:"last_tab" json:"lastTab"`
	Jump                string `toml:"jump" json:"jump"`
	ShowAllTabs         string `toml:"show_all_tabs" json:"showAllTabs"`
	Rearrange           string `toml:"rearrange" json:"rearrange"`
	FocusPaneModifiers  string `toml:"focus_pane_modifiers" json:"focusPaneModifiers"`
	MoveWindowModifiers string `toml:"move_window_modifiers" json:"moveWindowModifiers"`
	SendToTabModifiers  string `toml:"send_to_tab_modifiers" json:"sendToTabModifiers"`
	NextWindow          string `toml:"next_window" json:"nextWindow"`
	ZoomPane            string `toml:"zoom_pane" json:"zoomPane"`
	GrowPane            string `toml:"grow_pane" json:"growPane"`
	ShrinkPane          string `toml:"shrink_pane" json:"shrinkPane"`
	EqualPanes          string `toml:"equal_panes" json:"equalPanes"`
	NextLayout          string `toml:"next_layout" json:"nextLayout"`
	PreviousLayout      string `toml:"previous_layout" json:"previousLayout"`
	AddFrontWindow      string `toml:"add_front_window" json:"addFrontWindow"`
}

// Actions a binding can run.
const (
	ActionTab            = "tab"      // Arg: the tab number, 1 to 9.
	ActionNextTab        = "next-tab" // Control+Option+]
	ActionPreviousTab    = "previous-tab"
	ActionLastTab        = "last-tab" // Back to the tab used before this one.
	ActionJump           = "jump"
	ActionShowAllTabs    = "show-all-tabs" // Opens or closes the overview.
	ActionRearrange      = "rearrange"
	ActionFocusPane      = "focus-pane"  // Arg: left, right, up, or down.
	ActionMoveWindow     = "move-window" // Arg: left, right, up, or down.
	ActionNextWindow     = "next-window"
	ActionPreviousWindow = "previous-window" // next_window with Shift added.
	ActionZoomPane       = "zoom-pane"
	// ActionGrowPane and ActionShrinkPane make the focused pane larger or
	// smaller along the split that holds it; ActionEqualPanes gives every
	// split of the current tab equal sizes; and ActionNextLayout and
	// ActionPreviousLayout give the tab the next or previous of the six
	// layouts.
	ActionGrowPane       = "grow-pane"
	ActionShrinkPane     = "shrink-pane"
	ActionEqualPanes     = "equal-panes"
	ActionNextLayout     = "next-layout"
	ActionPreviousLayout = "previous-layout"
	// ActionAddFrontWindow adds the window in front, if it isn't in a
	// tab, to the focused pane.
	ActionAddFrontWindow = "add-front-window"
	// ActionSendToTab moves the focused pane's shown window to a tab's
	// focused pane, as Move to tab does. Arg: the tab number, 1 to 9.
	ActionSendToTab = "send-to-tab"
	// ActionSendToLastTab does the same for the tab used before this
	// one: send_to_tab_modifiers with `.
	ActionSendToLastTab = "send-to-last-tab"
)

// Binding is one global shortcut.
type Binding struct {
	// Shortcut is the canonical config form, such as "ctrl+alt+1".
	Shortcut string
	// Accelerator is the same shortcut in the form Wails v3's global
	// shortcut manager accepts, such as "Ctrl+Option+1".
	Accelerator string
	// Display is the shortcut as macOS shows it, such as "⌃⌥1".
	Display string
	// Action is what the shortcut does: one of the Action constants.
	Action string
	// Arg is the action's argument: a tab number ("1" to "9") for
	// ActionTab and ActionSendToTab, a direction ("left", "right", "up", or "down") for
	// ActionFocusPane and ActionMoveWindow, and "" otherwise.
	Arg string
	// Field is the config field the binding comes from, such as
	// "shortcuts.tab_modifiers", for messages about it.
	Field string
}

// FieldError is a problem with one field of the [shortcuts] table. Its
// message starts with the field, as in `shortcuts.jump: "hyper" isn't a
// modifier; use ctrl, alt, shift, or cmd`.
type FieldError struct {
	// Field is the dotted config path, such as "shortcuts.jump".
	Field string
	Err   error
}

// Error returns the field and the problem.
func (e *FieldError) Error() string { return e.Field + ": " + e.Err.Error() }

// Unwrap returns the problem without the field.
func (e *FieldError) Unwrap() error { return e.Err }

// fieldPrefix starts every field path in messages.
const fieldPrefix = "shortcuts."

// fields lists the [shortcuts] table's fields in config order, by their TOML
// names.
var fields = []struct {
	name string
	ptr  func(*Config) *string
}{
	{"tab_modifiers", func(c *Config) *string { return &c.TabModifiers }},
	{"next_tab", func(c *Config) *string { return &c.NextTab }},
	{"previous_tab", func(c *Config) *string { return &c.PreviousTab }},
	{"last_tab", func(c *Config) *string { return &c.LastTab }},
	{"jump", func(c *Config) *string { return &c.Jump }},
	{"show_all_tabs", func(c *Config) *string { return &c.ShowAllTabs }},
	{"rearrange", func(c *Config) *string { return &c.Rearrange }},
	{"focus_pane_modifiers", func(c *Config) *string { return &c.FocusPaneModifiers }},
	{"move_window_modifiers", func(c *Config) *string { return &c.MoveWindowModifiers }},
	{"send_to_tab_modifiers", func(c *Config) *string { return &c.SendToTabModifiers }},
	{"next_window", func(c *Config) *string { return &c.NextWindow }},
	{"zoom_pane", func(c *Config) *string { return &c.ZoomPane }},
	{"grow_pane", func(c *Config) *string { return &c.GrowPane }},
	{"shrink_pane", func(c *Config) *string { return &c.ShrinkPane }},
	{"equal_panes", func(c *Config) *string { return &c.EqualPanes }},
	{"next_layout", func(c *Config) *string { return &c.NextLayout }},
	{"previous_layout", func(c *Config) *string { return &c.PreviousLayout }},
	{"add_front_window", func(c *Config) *string { return &c.AddFrontWindow }},
}

// FieldNames returns the table's fields in config order, by their TOML
// names, such as "jump".
func FieldNames() []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.name
	}
	return names
}

// labels name what each field's shortcuts do, by the field's TOML name, as
// Settings ▸ Keyboard and the notices about shortcuts name them.
var labels = map[string]string{
	"tab_modifiers":         "Tabs 1 to 9",
	"next_tab":              "Next tab",
	"previous_tab":          "Previous tab",
	"last_tab":              "Last used tab",
	"jump":                  "Jump to…",
	"show_all_tabs":         "Show all tabs",
	"rearrange":             "Rearrange",
	"focus_pane_modifiers":  "Focus a pane",
	"move_window_modifiers": "Move a window",
	"send_to_tab_modifiers": "Send a window to a tab",
	"next_window":           "Next window",
	"zoom_pane":             "Zoom pane",
	"grow_pane":             "Grow pane",
	"shrink_pane":           "Shrink pane",
	"equal_panes":           "Make panes equal",
	"next_layout":           "Next layout",
	"previous_layout":       "Previous layout",
	"add_front_window":      "Add front window to this pane",
}

// Label names what a field's shortcuts do, such as "Jump to…" for "jump",
// or returns "" for a name that isn't a field of the table.
func Label(field string) string { return labels[field] }

// Fields returns the table as a map from TOML field name (such as "jump")
// to its value, including empty fields.
func (c Config) Fields() map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[f.name] = *f.ptr(&c)
	}
	return m
}

// FromFields builds a Config from a map like the one Fields returns. Names
// that aren't fields of the table are ignored, and missing ones stay empty.
func FromFields(m map[string]string) Config {
	var c Config
	for _, f := range fields {
		*f.ptr(&c) = m[f.name]
	}
	return c
}
