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
		SendToTabModifiers:  "ctrl+alt+shift",
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
		{"ctrl+alt+shift+1", "Ctrl+Option+Shift+1", "⌃⌥⇧1", ActionSendToTab, "1", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+2", "Ctrl+Option+Shift+2", "⌃⌥⇧2", ActionSendToTab, "2", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+3", "Ctrl+Option+Shift+3", "⌃⌥⇧3", ActionSendToTab, "3", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+4", "Ctrl+Option+Shift+4", "⌃⌥⇧4", ActionSendToTab, "4", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+5", "Ctrl+Option+Shift+5", "⌃⌥⇧5", ActionSendToTab, "5", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+6", "Ctrl+Option+Shift+6", "⌃⌥⇧6", ActionSendToTab, "6", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+7", "Ctrl+Option+Shift+7", "⌃⌥⇧7", ActionSendToTab, "7", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+8", "Ctrl+Option+Shift+8", "⌃⌥⇧8", ActionSendToTab, "8", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+9", "Ctrl+Option+Shift+9", "⌃⌥⇧9", ActionSendToTab, "9", "shortcuts.send_to_tab_modifiers"},
		{"ctrl+alt+shift+`", "Ctrl+Option+Shift+`", "⌃⌥⇧`", ActionSendToLastTab, "", "shortcuts.send_to_tab_modifiers"},
	}
	if len(want) != 9+2+1+1+4+4+2+1+1+9+1 {
		t.Fatalf("the test expects %d bindings, want 35", len(want))
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
send_to_tab_modifiers = "ctrl+alt+shift"  # Plus 1 to 9, or ` + "`" + ` for the last used tab.
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
	if len(got) != 9+2+1+1+4+4+2+1+1+9+1 {
		t.Errorf("got %d bindings, want 35", len(got))
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
				ActionFocusPane, ActionMoveWindow, ActionNextWindow, ActionPreviousWindow, ActionZoomPane, ActionLastTab,
				ActionSendToTab, ActionSendToLastTab}},
		{"tabs", func(c *Config) { c.TabModifiers = "" }, 26, []string{ActionTab}},
		{"next tab", func(c *Config) { c.NextTab = "" }, 34, []string{ActionNextTab}},
		{"previous tab", func(c *Config) { c.PreviousTab = "" }, 34, []string{ActionPreviousTab}},
		{"jump, with spaces", func(c *Config) { c.Jump = "  " }, 34, []string{ActionJump}},
		{"rearrange", func(c *Config) { c.Rearrange = "" }, 34, []string{ActionRearrange}},
		{"focus", func(c *Config) { c.FocusPaneModifiers = "" }, 31, []string{ActionFocusPane}},
		{"move", func(c *Config) { c.MoveWindowModifiers = "" }, 31, []string{ActionMoveWindow}},
		{"windows", func(c *Config) { c.NextWindow = "" }, 33, []string{ActionNextWindow, ActionPreviousWindow}},
		{"zoom", func(c *Config) { c.ZoomPane = "" }, 34, []string{ActionZoomPane}},
		{"last tab", func(c *Config) { c.LastTab = "" }, 34, []string{ActionLastTab}},
		{"send to tab", func(c *Config) { c.SendToTabModifiers = "" }, 25, []string{ActionSendToTab, ActionSendToLastTab}},
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
		{"bad modifier", func(c *Config) { c.Jump = "ctrl+hyper+space" }, 34,
			[]string{`shortcuts.jump: "hyper" isn't a modifier; use ctrl, alt, shift, or cmd`}},
		{"bad key", func(c *Config) { c.NextTab = "ctrl+alt+banana" }, 34,
			[]string{`shortcuts.next_tab: "banana" isn't a key Bench can use`}},
		{"key in a modifiers field", func(c *Config) { c.TabModifiers = "ctrl+alt+1" }, 26,
			[]string{`shortcuts.tab_modifiers: "ctrl+alt+1" includes a key; use modifiers alone`}},
		{"no key", func(c *Config) { c.Rearrange = "ctrl+alt" }, 34,
			[]string{`shortcuts.rearrange: "ctrl+alt" has no key`}},
		{"takes over typing", func(c *Config) { c.FocusPaneModifiers = "shift" }, 31,
			[]string{`shortcuts.focus_pane_modifiers: "shift" needs ctrl, alt, or cmd`}},
		{"next window already has shift", func(c *Config) { c.NextWindow = "ctrl+alt+shift+tab" }, 34,
			[]string{`shortcuts.next_window: "ctrl+alt+shift+tab" already includes shift`}},
		{"same shortcut as a tab", func(c *Config) { c.Jump = "alt+ctrl+1" }, 34,
			[]string{`shortcuts.jump: "ctrl+alt+1" is already the shortcut for shortcuts.tab_modifiers`}},
		{"previous window already taken", func(c *Config) { c.Jump = "ctrl+alt+shift+tab" }, 34,
			[]string{`shortcuts.next_window: "ctrl+alt+shift+tab" (with shift added, for the previous window) ` +
				`is already the shortcut for shortcuts.jump`}},
		{"focus and move the same", func(c *Config) { c.MoveWindowModifiers = "ctrl+option" }, 31,
			[]string{
				`shortcuts.move_window_modifiers: "ctrl+alt+left" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+right" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+up" is already the shortcut for shortcuts.focus_pane_modifiers`,
				`shortcuts.move_window_modifiers: "ctrl+alt+down" is already the shortcut for shortcuts.focus_pane_modifiers`,
			}},
		// last_tab, though listed with the tabs, gives way to the fields
		// that a config had before it.
		{"last tab gives way", func(c *Config) { c.NextWindow = "ctrl+alt+`" }, 33,
			[]string{
				`shortcuts.last_tab: "ctrl+alt+` + "`" + `" is already the shortcut for shortcuts.next_window`,
				`shortcuts.send_to_tab_modifiers: "ctrl+alt+shift+` + "`" + `" is already the shortcut for shortcuts.next_window`,
			}},
		// The keys that arrange the panes are off by default, and keep
		// their place in the table: before last_tab, after zoom_pane.
		{"grow pane before last tab", func(c *Config) { c.GrowPane = "ctrl+alt+`" }, 35,
			[]string{
				`shortcuts.last_tab: "ctrl+alt+` + "`" + `" is already the shortcut for shortcuts.grow_pane`,
			}},
		{"next layout after zoom pane", func(c *Config) { c.NextLayout = "ctrl+alt+return" }, 35,
			[]string{`shortcuts.next_layout: "ctrl+alt+return" is already the shortcut for shortcuts.zoom_pane`}},
		// So is show_all_tabs, which keeps its place after jump.
		{"show all tabs after jump", func(c *Config) { c.ShowAllTabs = "ctrl+alt+space" }, 35,
			[]string{`shortcuts.show_all_tabs: "ctrl+alt+space" is already the shortcut for shortcuts.jump`}},
		{"show all tabs before rearrange", func(c *Config) { c.ShowAllTabs = "ctrl+alt+0" }, 35,
			[]string{`shortcuts.rearrange: "ctrl+alt+0" is already the shortcut for shortcuts.show_all_tabs`}},
		// So does send_to_tab_modifiers, even to add_front_window.
		{"send to tab gives way", func(c *Config) { c.AddFrontWindow = "ctrl+alt+shift+3" }, 35,
			[]string{`shortcuts.send_to_tab_modifiers: "ctrl+alt+shift+3" is already the shortcut for shortcuts.add_front_window`}},
		{"key in send to tab", func(c *Config) { c.SendToTabModifiers = "ctrl+alt+shift+1" }, 25,
			[]string{`shortcuts.send_to_tab_modifiers: "ctrl+alt+shift+1" includes a key; use modifiers alone`}},
		{"several fields", func(c *Config) {
			c.NextTab = "ctrl+alt+]]"
			c.NextWindow = "tab"
		}, 32, []string{
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

// TestBindingsArrangeKeys checks the fields that arrange the current tab's
// panes, which are off by default: each makes one binding, after zoom_pane
// and before last_tab, with its own action.
func TestBindingsArrangeKeys(t *testing.T) {
	c := defaultConfig()
	c.GrowPane = "ctrl+alt+="
	c.ShrinkPane = "ctrl+alt+-"
	c.EqualPanes = "ctrl+alt+shift+="
	c.NextLayout = "ctrl+alt+l"
	c.PreviousLayout = "ctrl+alt+shift+l"
	got, errs := Bindings(c)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	type row struct{ shortcut, accel, display, action, field string }
	want := []row{
		{"ctrl+alt+return", "Ctrl+Option+Return", "⌃⌥↩", ActionZoomPane, "shortcuts.zoom_pane"},
		{"ctrl+alt+=", "Ctrl+Option+=", "⌃⌥=", ActionGrowPane, "shortcuts.grow_pane"},
		{"ctrl+alt+-", "Ctrl+Option+-", "⌃⌥-", ActionShrinkPane, "shortcuts.shrink_pane"},
		{"ctrl+alt+shift+=", "Ctrl+Option+Shift+=", "⌃⌥⇧=", ActionEqualPanes, "shortcuts.equal_panes"},
		{"ctrl+alt+l", "Ctrl+Option+L", "⌃⌥L", ActionNextLayout, "shortcuts.next_layout"},
		{"ctrl+alt+shift+l", "Ctrl+Option+Shift+L", "⌃⌥⇧L", ActionPreviousLayout, "shortcuts.previous_layout"},
		{"ctrl+alt+`", "Ctrl+Option+`", "⌃⌥`", ActionLastTab, "shortcuts.last_tab"},
	}
	at := -1
	for i, b := range got {
		if b.Action == ActionZoomPane {
			at = i
		}
	}
	if at < 0 || at+len(want) > len(got) {
		t.Fatalf("bindings %+v have no zoom_pane, or too little after it", got)
	}
	for i, w := range want {
		g := got[at+i]
		if (row{g.Shortcut, g.Accelerator, g.Display, g.Action, g.Field}) != w {
			t.Errorf("binding %d after zoom_pane = %+v, want %+v", i, g, w)
		}
		if g.Arg != "" {
			t.Errorf("binding %+v has an argument", g)
		}
		if err := wailsAccepts(g.Accelerator); err != nil {
			t.Errorf("Wails wouldn't register %q: %v", g.Accelerator, err)
		}
	}
	if len(got) != 35+5 {
		t.Errorf("got %d bindings, want 40", len(got))
	}
	for _, f := range []string{"grow_pane", "shrink_pane", "equal_panes", "next_layout", "previous_layout"} {
		if Label(f) == "" || AddedKeys(f) != "" {
			t.Errorf("%s: label %q, added keys %q; want a label, and no keys added", f, Label(f), AddedKeys(f))
		}
	}
}

// TestBindingsShowAllTabs checks show_all_tabs, which is off by default:
// it makes one binding, right after jump's.
func TestBindingsShowAllTabs(t *testing.T) {
	c := defaultConfig()
	c.ShowAllTabs = "ctrl+alt+a"
	got, errs := Bindings(c)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(got) != 35+1 {
		t.Fatalf("got %d bindings, want 36", len(got))
	}
	for i, b := range got {
		if b.Action != ActionJump {
			continue
		}
		g := got[i+1]
		want := Binding{Shortcut: "ctrl+alt+a", Accelerator: "Ctrl+Option+A", Display: "⌃⌥A", Action: ActionShowAllTabs, Field: "shortcuts.show_all_tabs"}
		if g != want {
			t.Errorf("binding after jump = %+v, want %+v", g, want)
		}
		if err := wailsAccepts(g.Accelerator); err != nil {
			t.Errorf("Wails wouldn't register %q: %v", g.Accelerator, err)
		}
	}
	if Label("show_all_tabs") != "Show all tabs" || AddedKeys("show_all_tabs") != "" {
		t.Errorf("show_all_tabs: label %q, added keys %q", Label("show_all_tabs"), AddedKeys("show_all_tabs"))
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
		"show_all_tabs":         "",
		"rearrange":             "ctrl+alt+0",
		"focus_pane_modifiers":  "ctrl+alt",
		"move_window_modifiers": "ctrl+alt+shift",
		"send_to_tab_modifiers": "ctrl+alt+shift",
		"next_window":           "ctrl+alt+tab",
		"zoom_pane":             "ctrl+alt+return",
		"grow_pane":             "",
		"shrink_pane":           "",
		"equal_panes":           "",
		"next_layout":           "",
		"previous_layout":       "",
		"add_front_window":      "",
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
