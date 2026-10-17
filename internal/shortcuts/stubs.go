// Package shortcuts turns the [shortcuts] config into global shortcut
// bindings, and formats shortcuts for display.
package shortcuts

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// Config is the [shortcuts] table of config.toml (config.Shortcuts). Each
// field is a shortcut such as "ctrl+alt+space", or modifiers such as
// "ctrl+alt" that combine with keys (1 to 9, or the arrow keys).
type Config struct {
	TabModifiers        string `toml:"tab_modifiers" json:"tabModifiers"`
	NextTab             string `toml:"next_tab" json:"nextTab"`
	PreviousTab         string `toml:"previous_tab" json:"previousTab"`
	Jump                string `toml:"jump" json:"jump"`
	Rearrange           string `toml:"rearrange" json:"rearrange"`
	FocusPaneModifiers  string `toml:"focus_pane_modifiers" json:"focusPaneModifiers"`
	MoveWindowModifiers string `toml:"move_window_modifiers" json:"moveWindowModifiers"`
	NextWindow          string `toml:"next_window" json:"nextWindow"`
}

// Actions a binding can run.
const (
	ActionTab            = "tab"      // Arg: the tab number, 1 to 9.
	ActionNextTab        = "next-tab" // Control+Option+]
	ActionPreviousTab    = "previous-tab"
	ActionJump           = "jump"
	ActionRearrange      = "rearrange"
	ActionFocusPane      = "focus-pane"  // Arg: left, right, up, or down.
	ActionMoveWindow     = "move-window" // Arg: left, right, up, or down.
	ActionNextWindow     = "next-window"
	ActionPreviousWindow = "previous-window" // next_window with Shift added.
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
	Action  string
	Arg     string
}

// Parse checks a shortcut such as "ctrl+alt+space", "ctrl+alt+]", or
// "ctrl+alt+shift+left" and returns its canonical form: lower case,
// modifiers in the order ctrl, alt, shift, cmd ("option" and "opt" mean alt,
// "control" means ctrl, "command" means cmd). With modifiersOnly, it accepts
// modifiers alone, such as "ctrl+alt".
func Parse(s string, modifiersOnly bool) (string, error) { panic("not implemented") }

// Bindings expands the [shortcuts] config into every binding: tabs 1 to 9,
// next and previous tab, jump, rearrange, focus and move in four directions,
// and next and previous window. It returns the bindings that parse and an
// error for each that doesn't, naming the field.
func Bindings(s Config) ([]Binding, []error) { panic("not implemented") }

// Display formats a canonical shortcut the way macOS does: ⌃ for ctrl, ⌥ for
// alt, ⇧ for shift, ⌘ for cmd, then the key ("Space", "Tab", "←", "]", "1").
func Display(shortcut string) string { panic("not implemented") }

// Suggest returns another shortcut to try when one can't be registered,
// such as "ctrl+alt+j" for "ctrl+alt+space".
func Suggest(shortcut string) string { panic("not implemented") }
