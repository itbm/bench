package shortcuts

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// defaultConfig is the [shortcuts] table of the spec's default config.
func defaultConfig() Config {
	return Config{
		TabModifiers:        "ctrl+alt",
		NextTab:             "ctrl+alt+]",
		PreviousTab:         "ctrl+alt+[",
		LastTab:             "ctrl+alt+`",
		Jump:                "ctrl+alt+space",
		Rearrange:           "ctrl+alt+0",
		FocusPaneModifiers:  "ctrl+alt",
		MoveWindowModifiers: "ctrl+alt+shift",
		NextWindow:          "ctrl+alt+tab",
		ZoomPane:            "ctrl+alt+return",
	}
}

func TestBindingsDefault(t *testing.T) {
	type row struct{ shortcut, accel, display, action, arg, field string }
	want := []row{
		{"ctrl+alt+1", "Ctrl+Option+1", "⌃⌥1", ActionTab, "1", "shortcuts.tab_modifiers"},
		{"ctrl+alt+2", "Ctrl+Option+2", "⌃⌥2", ActionTab, "2", "shortcuts.tab_modifiers"},
		{"ctrl+alt+3", "Ctrl+Option+3", "⌃⌥3", ActionTab, "3", "shortcuts.tab_modifiers"},
		{"ctrl+alt+4", "Ctrl+Option+4", "⌃⌥4", ActionTab, "4", "shortcuts.tab_modifiers"},
		{"ctrl+alt+5", "Ctrl+Option+5", "⌃⌥5", ActionTab, "5", "shortcuts.tab_modifiers"},
		{"ctrl+alt+6", "Ctrl+Option+6", "⌃⌥6", ActionTab, "6", "shortcuts.tab_modifiers"},
		{"ctrl+alt+7", "Ctrl+Option+7", "⌃⌥7", ActionTab, "7", "shortcuts.tab_modifiers"},
		{"ctrl+alt+8", "Ctrl+Option+8", "⌃⌥8", ActionTab, "8", "shortcuts.tab_modifiers"},
		{"ctrl+alt+9", "Ctrl+Option+9", "⌃⌥9", ActionTab, "9", "shortcuts.tab_modifiers"},
		{"ctrl+alt+]", "Ctrl+Option+]", "⌃⌥]", ActionNextTab, "", "shortcuts.next_tab"},
		{"ctrl+alt+[", "Ctrl+Option+[", "⌃⌥[", ActionPreviousTab, "", "shortcuts.previous_tab"},
		{"ctrl+alt+space", "Ctrl+Option+Space", "⌃⌥Space", ActionJump, "", "shortcuts.jump"},
		{"ctrl+alt+0", "Ctrl+Option+0", "⌃⌥0", ActionRearrange, "", "shortcuts.rearrange"},
		{"ctrl+alt+left", "Ctrl+Option+Left", "⌃⌥←", ActionFocusPane, "left", "shortcuts.focus_pane_modifiers"},
		{"ctrl+alt+right", "Ctrl+Option+Right", "⌃⌥→", ActionFocusPane, "right", "shortcuts.focus_pane_modifiers"},
		{"ctrl+alt+up", "Ctrl+Option+Up", "⌃⌥↑", ActionFocusPane, "up", "shortcuts.focus_pane_modifiers"},
		{"ctrl+alt+down", "Ctrl+Option+Down", "⌃⌥↓", ActionFocusPane, "down", "shortcuts.focus_pane_modifiers"},
		{"ctrl+alt+shift+left", "Ctrl+Option+Shift+Left", "⌃⌥⇧←", ActionMoveWindow, "left", "shortcuts.move_window_modifiers"},
		{"ctrl+alt+shift+right", "Ctrl+Option+Shift+Right", "⌃⌥⇧→", ActionMoveWindow, "right", "shortcuts.move_window_modifiers"},
		{"ctrl+alt+shift+up", "Ctrl+Option+Shift+Up", "⌃⌥⇧↑", ActionMoveWindow, "up", "shortcuts.move_window_modifiers"},
		{"ctrl+alt+shift+down", "Ctrl+Option+Shift+Down", "⌃⌥⇧↓", ActionMoveWindow, "down", "shortcuts.move_window_modifiers"},
		{"ctrl+alt+tab", "Ctrl+Option+Tab", "⌃⌥Tab", ActionNextWindow, "", "shortcuts.next_window"},
		{"ctrl+alt+shift+tab", "Ctrl+Option+Shift+Tab", "⌃⌥⇧Tab", ActionPreviousWindow, "", "shortcuts.next_window"},
		{"ctrl+alt+return", "Ctrl+Option+Return", "⌃⌥↩", ActionZoomPane, "", "shortcuts.zoom_pane"},
		{"ctrl+alt+`", "Ctrl+Option+`", "⌃⌥`", ActionLastTab, "", "shortcuts.last_tab"},
	}
	if len(want) != 9+2+1+1+4+4+2+1+1 {
		t.Fatalf("the test expects %d bindings, want 25", len(want))
	}

	got, errs := Bindings(defaultConfig())
	if len(errs) != 0 {
		t.Fatalf("Bindings(default) errors: %v", errs)
	}
	if len(got) != len(want) {
		t.Fatalf("Bindings(default) returned %d bindings, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if (row{g.Shortcut, g.Accelerator, g.Display, g.Action, g.Arg, g.Field}) != w {
			t.Errorf("binding %d = %+v, want %+v", i, g, w)
		}
		if err := wailsAccepts(g.Accelerator); err != nil {
			t.Errorf("binding %d: Wails wouldn't register %q: %v", i, g.Accelerator, err)
		}
	}
}

// specTOML is the [shortcuts] table from the spec's example config.
const specTOML = `
[shortcuts]
tab_modifiers = "ctrl+alt"          # Plus 1 to 9.
next_tab = "ctrl+alt+]"
previous_tab = "ctrl+alt+["
last_tab = "ctrl+alt+` + "`" + `"             # Back to the tab you were on.
jump = "ctrl+alt+space"
rearrange = "ctrl+alt+0"
focus_pane_modifiers = "ctrl+alt"   # Plus an arrow key.
move_window_modifiers = "ctrl+alt+shift"
next_window = "ctrl+alt+tab"
zoom_pane = "ctrl+alt+return"
`

// TestBindingsFromSpecTOML decodes the spec's [shortcuts] table through
// Config's TOML tags, so that a tag that doesn't match the spec's field
// names fails here.
func TestBindingsFromSpecTOML(t *testing.T) {
	var file struct {
		Shortcuts Config `toml:"shortcuts"`
	}
	dec := toml.NewDecoder(strings.NewReader(specTOML))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		t.Fatalf("decoding the spec's [shortcuts]: %v", err)
	}
	if file.Shortcuts != defaultConfig() {
		t.Fatalf("decoded %+v, want %+v", file.Shortcuts, defaultConfig())
	}
	got, errs := Bindings(file.Shortcuts)
	if len(errs) != 0 {
		t.Errorf("errors: %v", errs)
	}
	if len(got) != 9+2+1+1+4+4+2+1+1 {
		t.Errorf("got %d bindings, want 25", len(got))
	}
}

func TestBindingsEmptyFieldsTurnOff(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
		want   int
		absent []string // Actions that shouldn't appear.
	}{
		{"everything", func(c *Config) { *c = Config{} }, 0,
			[]string{ActionTab, ActionNextTab, ActionPreviousTab, ActionJump, ActionRearrange,
				ActionFocusPane, ActionMoveWindow, ActionNextWindow, ActionPreviousWindow, ActionZoomPane, ActionLastTab}},
		{"tabs", func(c *Config) { c.TabModifiers = "" }, 16, []string{ActionTab}},
		{"next tab", func(c *Config) { c.NextTab = "" }, 24, []string{ActionNextTab}},
		{"previous tab", func(c *Config) { c.PreviousTab = "" }, 24, []string{ActionPreviousTab}},
		{"jump, with spaces", func(c *Config) { c.Jump = "  " }, 24, []string{ActionJump}},
		{"rearrange", func(c *Config) { c.Rearrange = "" }, 24, []string{ActionRearrange}},
		{"focus", func(c *Config) { c.FocusPaneModifiers = "" }, 21, []string{ActionFocusPane}},
		{"move", func(c *Config) { c.MoveWindowModifiers = "" }, 21, []string{ActionMoveWindow}},
		{"windows", func(c *Config) { c.NextWindow = "" }, 23, []string{ActionNextWindow, ActionPreviousWindow}},
		{"zoom", func(c *Config) { c.ZoomPane = "" }, 24, []string{ActionZoomPane}},
		{"last tab", func(c *Config) { c.LastTab = "" }, 24, []string{ActionLastTab}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := defaultConfig()
			tt.change(&c)
			got, errs := Bindings(c)
			if len(errs) != 0 {
				t.Errorf("errors: %v", errs)
			}
			if len(got) != tt.want {
				t.Errorf("got %d bindings, want %d", len(got), tt.want)
			}
			for _, b := range got {
				for _, a := range tt.absent {
					if b.Action == a {
						t.Errorf("binding %+v is still there", b)
					}
				}
			}
		})
	}
}

func TestBindingsErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
		want   int      // Bindings left.
		errs   []string // Each error, in order.
	}{
		{"bad modifier", func(c *Config) { c.Jump = "ctrl+hyper+space" }, 24,
			[]string{`shortcuts.jump: "hyper" isn't a modifier; use ctrl, alt, shift, or cmd`}},
		{"bad key", func(c *Config) { c.NextTab = "ctrl+alt+banana" }, 24,
			[]string{`shortcuts.next_tab: "banana" isn't a key Bench can use`}},
		{"key in a modifiers field", func(c *Config) { c.TabModifiers = "ctrl+alt+1" }, 16,
			[]string{`shortcuts.tab_modifiers: "ctrl+alt+1" includes a key; use modifiers alone`}},
		{"no key", func(c *Config) { c.Rearrange = "ctrl+alt" }, 24,
			[]string{`shortcuts.rearrange: "ctrl+alt" has no key`}},
		{"takes over typing", func(c *Config) { c.FocusPaneModifiers = "shift" }, 21,
			[]string{`shortcuts.focus_pane_modifiers: "shift" needs ctrl, alt, or cmd`}},
		{"next window already has shift", func(c *Config) { c.NextWindow = "ctrl+alt+shift+tab" }, 24,
			[]string{`shortcuts.next_window: "ctrl+alt+shift+tab" already includes shift`}},
		{"same shortcut as a tab", func(c *Config) { c.Jump = "alt+ctrl+1" }, 24,
			[]string{`shortcuts.jump: "ctrl+alt+1" is already the shortcut for shortcuts.tab_modifiers`}},
		{"previous window already taken", func(c *Config) { c.Jump = "ctrl+alt+shift+tab" }, 24,
			[]string{`shortcuts.next_window: "ctrl+alt+shift+tab" (with shift added, for the previous window) ` +
				`is already the shortcut for shortcuts.jump`}},
		{"focus and move the same", func(c *Config) { c.MoveWindowModifiers = "ctrl+option" }, 21,
			[]string{
				`shortcuts.move_window_modifiers: "ctrl+alt+left" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+right" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+up" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+down" is already the shortcut for shortcuts.focus_pane_modifiers`,
			}},
		// last_tab, though listed with the tabs, gives way to the fields
		// that a config had before it.
		{"last tab gives way", func(c *Config) { c.NextWindow = "ctrl+alt+`" }, 24,
			[]string{`shortcuts.last_tab: "ctrl+alt+` + "`" + `" is already the shortcut for shortcuts.next_window`}},
		{"several fields", func(c *Config) {
			c.NextTab = "ctrl+alt+]]"
			c.NextWindow = "tab"
		}, 22, []string{
			`shortcuts.next_tab: "]]" isn't a key Bench can use`,
			`shortcuts.next_window: "tab" needs ctrl, alt, or cmd`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := defaultConfig()
			tt.change(&c)
			got, errs := Bindings(c)
			if len(got) != tt.want {
				t.Errorf("got %d bindings, want %d", len(got), tt.want)
			}
			if len(errs) != len(tt.errs) {
				t.Fatalf("errors = %q, want %d", errs, len(tt.errs))
			}
			for i, err := range errs {
				if !strings.HasPrefix(err.Error(), tt.errs[i]) {
					t.Errorf("error %d = %q, want it to start with %q", i, err, tt.errs[i])
				}
				var fe *FieldError
				if !errors.As(err, &fe) || !strings.HasPrefix(tt.errs[i], fe.Field+": ") {
					t.Errorf("error %d = %#v, want a *FieldError for its field", i, err)
					continue
				}
				if inner := errors.Unwrap(err); inner == nil || fe.Field+": "+inner.Error() != err.Error() {
					t.Errorf("error %d unwraps to %v, want the problem without the field", i, inner)
				}
			}
		})
	}
}

func TestBindingsOtherShortcuts(t *testing.T) {
	c := Config{
		TabModifiers: "cmd+option",
		Jump:         "Control+Option+J",
		NextWindow:   "cmd+f5",
	}
	got, errs := Bindings(c)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	type row struct{ shortcut, accel, display string }
	want := map[string]row{
		ActionTab:            {"alt+cmd+9", "Option+Cmd+9", "⌥⌘9"},
		ActionJump:           {"ctrl+alt+j", "Ctrl+Option+J", "⌃⌥J"},
		ActionNextWindow:     {"cmd+f5", "Cmd+F5", "⌘F5"},
		ActionPreviousWindow: {"shift+cmd+f5", "Shift+Cmd+F5", "⇧⌘F5"},
	}
	last := make(map[string]row)
	for _, b := range got {
		last[b.Action] = row{b.Shortcut, b.Accelerator, b.Display}
		if err := wailsAccepts(b.Accelerator); err != nil {
			t.Errorf("Wails wouldn't register %q: %v", b.Accelerator, err)
		}
	}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("last binding for each action = %+v, want %+v", last, want)
	}
}

func TestFields(t *testing.T) {
	c := defaultConfig()
	m := c.Fields()
	want := map[string]string{
		"tab_modifiers":         "ctrl+alt",
		"next_tab":              "ctrl+alt+]",
		"previous_tab":          "ctrl+alt+[",
		"last_tab":              "ctrl+alt+`",
		"jump":                  "ctrl+alt+space",
		"rearrange":             "ctrl+alt+0",
		"focus_pane_modifiers":  "ctrl+alt",
		"move_window_modifiers": "ctrl+alt+shift",
		"next_window":           "ctrl+alt+tab",
		"zoom_pane":             "ctrl+alt+return",
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("Fields() = %v, want %v", m, want)
	}
	m["unknown"] = "ctrl+alt+u"
	if got := FromFields(m); got != c {
		t.Errorf("FromFields(Fields()) = %+v, want %+v", got, c)
	}
	if got := FromFields(map[string]string{"jump": "ctrl+alt+j"}); got != (Config{Jump: "ctrl+alt+j"}) {
		t.Errorf("FromFields(jump only) = %+v", got)
	}
}
