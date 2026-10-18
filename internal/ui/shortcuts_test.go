//go:build darwin || server

package ui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/workspace"
)

// defaultShortcuts is the spec's [shortcuts] table.
var defaultShortcuts = shortcuts.Config{
	TabModifiers:        "ctrl+alt",
	NextTab:             "ctrl+alt+]",
	PreviousTab:         "ctrl+alt+[",
	Jump:                "ctrl+alt+space",
	Rearrange:           "ctrl+alt+0",
	FocusPaneModifiers:  "ctrl+alt",
	MoveWindowModifiers: "ctrl+alt+shift",
	NextWindow:          "ctrl+alt+tab",
}

// fakeRegistrar refuses the accelerators in taken, and ones already
// registered, and records the rest.
type fakeRegistrar struct {
	taken     map[string]bool
	callbacks map[string]func()
}

func (r *fakeRegistrar) Register(accelerator string, callback func()) error {
	if r.taken[accelerator] {
		return errors.New("failed to register global shortcut: already in use")
	}
	if r.callbacks == nil {
		r.callbacks = map[string]func(){}
	}
	if r.callbacks[accelerator] != nil {
		return errors.New("global shortcut is already registered")
	}
	r.callbacks[accelerator] = callback
	return nil
}

func (r *fakeRegistrar) Unregister(accelerator string) error {
	if r.callbacks[accelerator] == nil {
		return errors.New("global shortcut is not registered")
	}
	delete(r.callbacks, accelerator)
	return nil
}

func TestRegisterShortcutsRunsActions(t *testing.T) {
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: defaultShortcuts}
	reg := &fakeRegistrar{}
	registerShortcuts(reg, f)

	tests := []struct {
		accelerator string
		want        call
	}{
		{"Ctrl+Option+3", call{"Shortcut", []any{shortcuts.ActionTab, "3"}}},
		{"Ctrl+Option+Space", call{"Shortcut", []any{shortcuts.ActionJump, ""}}},
		{"Ctrl+Option+Shift+Left", call{"Shortcut", []any{shortcuts.ActionMoveWindow, "left"}}},
		{"Ctrl+Option+Shift+Tab", call{"Shortcut", []any{shortcuts.ActionPreviousWindow, ""}}},
	}
	for _, tt := range tests {
		run := reg.callbacks[tt.accelerator]
		if run == nil {
			t.Errorf("%s isn't registered; registered %v", tt.accelerator, keys(reg.callbacks))
			continue
		}
		before := len(f.Calls())
		run()
		if got := f.Calls()[before:]; len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
			t.Errorf("%s ran %v, want %v", tt.accelerator, got, tt.want)
		}
	}
	if got := f.notices[noticeSource]; len(got) != 0 {
		t.Errorf("notices = %q, want none", got)
	}
	if _, ok := f.notices[noticeSource]; !ok {
		t.Error("registerShortcuts didn't set the shortcuts notices, so old ones would stay")
	}
}

func TestShortcutNotices(t *testing.T) {
	tests := []struct {
		name  string
		cfg   shortcuts.Config
		taken []string
		want  []string
	}{
		{
			name:  "input sources",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Space"},
			want: []string{
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥J: set jump in config.toml.",
			},
		},
		{
			name:  "window tools",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Left", "Ctrl+Option+Right"},
			want: []string{
				"Couldn't register ⌃⌥← and ⌃⌥→ (window tools such as Rectangle often use them). " +
					"Try ⌃⌥⌘ with the arrow keys: set focus_pane_modifiers in config.toml.",
			},
		},
		{
			name:  "tabs",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+1", "Ctrl+Option+2", "Ctrl+Option+3"},
			want: []string{
				"Couldn't register ⌃⌥1, ⌃⌥2, and ⌃⌥3 (another app might be using them). " +
					"Try ⌃⌥⌘ with 1 to 9: set tab_modifiers in config.toml.",
			},
		},
		{
			name:  "previous window",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Shift+Tab"},
			want: []string{
				"Couldn't register ⌃⌥⇧Tab (another app might be using it). Try ⌃⌥`: set next_window in config.toml.",
			},
		},
		{
			name:  "suggestion taken too",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Space", "Ctrl+Option+J"},
			want: []string{
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥⌘J: set jump in config.toml.",
			},
		},
		{
			name:  "suggestion used by another field",
			cfg:   withField(defaultShortcuts, "rearrange", "ctrl+alt+j"),
			taken: []string{"Ctrl+Option+Space"},
			want: []string{
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥⌘J: set jump in config.toml.",
			},
		},
		{
			name:  "two fields",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+]", "Ctrl+Option+0"},
			want: []string{
				"Couldn't register ⌃⌥] (another app might be using it). Try ⌃⌥.: set next_tab in config.toml.",
				"Couldn't register ⌃⌥0 (another app might be using it). Try ⌃⌥⌘0: set rearrange in config.toml.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noticesFor(t, tt.cfg, tt.taken...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("notices:\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestShortcutNoticesForConfigProblems(t *testing.T) {
	cfg := withField(defaultShortcuts, "jump", "hyper+x")
	cfg = withField(cfg, "previous_tab", "ctrl+alt+]")
	got := noticesFor(t, cfg)
	if len(got) != 2 {
		t.Fatalf("notices = %q, want one for each field", got)
	}
	for i, field := range []string{"shortcuts.previous_tab", "shortcuts.jump"} {
		prefix := "Couldn't use " + field + " in config.toml: "
		if !strings.HasPrefix(got[i], prefix) || !strings.HasSuffix(got[i], ".") {
			t.Errorf("notice %d = %q, want a sentence starting %q", i, got[i], prefix)
		}
	}
}

func TestListAnd(t *testing.T) {
	tests := []struct {
		words []string
		want  string
	}{
		{nil, ""},
		{[]string{"⌃⌥1"}, "⌃⌥1"},
		{[]string{"⌃⌥1", "⌃⌥2"}, "⌃⌥1 and ⌃⌥2"},
		{[]string{"⌃⌥1", "⌃⌥2", "⌃⌥3"}, "⌃⌥1, ⌃⌥2, and ⌃⌥3"},
	}
	for _, tt := range tests {
		if got := listAnd(tt.words); got != tt.want {
			t.Errorf("listAnd(%q) = %q, want %q", tt.words, got, tt.want)
		}
	}
}

// noticesFor registers cfg's shortcuts with the given accelerators taken,
// and returns the notices.
func noticesFor(t *testing.T, cfg shortcuts.Config, taken ...string) []string {
	t.Helper()
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: cfg}
	reg := &fakeRegistrar{taken: map[string]bool{}}
	for _, a := range taken {
		reg.taken[a] = true
	}
	registerShortcuts(reg, f)
	if got, want := keys(reg.callbacks), freeAccelerators(cfg, reg.taken); !slices.Equal(got, want) {
		t.Errorf("registered %q, want just the free bindings %q: probing left some", got, want)
	}
	return f.notices[noticeSource]
}

// freeAccelerators returns the accelerators of cfg's bindings that aren't
// taken, sorted.
func freeAccelerators(cfg shortcuts.Config, taken map[string]bool) []string {
	bindings, _ := shortcuts.Bindings(cfg)
	var out []string
	for _, b := range bindings {
		if !taken[b.Accelerator] {
			out = append(out, b.Accelerator)
		}
	}
	slices.Sort(out)
	return out
}

func withField(cfg shortcuts.Config, name, value string) shortcuts.Config {
	fields := cfg.Fields()
	fields[name] = value
	return shortcuts.FromFields(fields)
}

func keys(m map[string]func()) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
