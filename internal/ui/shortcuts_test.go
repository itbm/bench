//go:build darwin || server

package ui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/platform"
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
	(&shortcutSet{reg: reg, e: f}).register()

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
	report, ok := f.lastReport()
	if !ok {
		t.Fatal("register didn't report, so old notices would stay")
	}
	if len(report.Notices) != 0 {
		t.Errorf("notices = %+v, want none", report.Notices)
	}
	if len(report.Fields) != len(shortcuts.FieldNames()) {
		t.Errorf("%d fields reported, want every field", len(report.Fields))
	}
	for _, v := range report.Fields {
		if v.Problem != "" || v.Suggestion != "" {
			t.Errorf("%s: %+v, want it to work", v.Field, v)
		}
	}
}

// TestShortcutSetRegistersAgain checks that the shortcuts are registered
// again for a changed [shortcuts] table, as after config.toml is reloaded:
// the old ones are released, and the new ones run their actions.
func TestShortcutSetRegistersAgain(t *testing.T) {
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: defaultShortcuts}
	reg := &fakeRegistrar{}
	set := &shortcutSet{reg: reg, e: f}
	set.register()
	if reg.callbacks["Ctrl+Option+Space"] == nil {
		t.Fatalf("the jump shortcut isn't registered: %v", keys(reg.callbacks))
	}
	before := len(reg.callbacks)

	changed := defaultShortcuts
	changed.Jump = "ctrl+alt+j"
	f.cfg = workspace.ConfigInfo{Shortcuts: changed}
	set.register()
	if reg.callbacks["Ctrl+Option+Space"] != nil {
		t.Error("the old jump shortcut is still registered")
	}
	run := reg.callbacks["Ctrl+Option+J"]
	if run == nil {
		t.Fatalf("the new jump shortcut isn't registered: %v", keys(reg.callbacks))
	}
	if len(reg.callbacks) != before {
		t.Errorf("%d shortcuts registered, want %d: the others were registered again", len(reg.callbacks), before)
	}
	run()
	if got := f.Calls(); len(got) != 1 || !reflect.DeepEqual(got[0], call{"Shortcut", []any{shortcuts.ActionJump, ""}}) {
		t.Errorf("the new shortcut ran %v", got)
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
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥J instead.",
			},
		},
		{
			name:  "window tools",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Left", "Ctrl+Option+Right"},
			want: []string{
				"Couldn't register ⌃⌥← and ⌃⌥→ (window tools such as Rectangle often use them). " +
					"Try ⌃⌥⌘ with the arrow keys instead.",
			},
		},
		{
			name:  "tabs",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+1", "Ctrl+Option+2", "Ctrl+Option+3"},
			want: []string{
				"Couldn't register ⌃⌥1, ⌃⌥2, and ⌃⌥3 (another app might be using them). " +
					"Try ⌃⌥⌘ with 1 to 9 instead.",
			},
		},
		{
			name:  "previous window",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Shift+Tab"},
			want: []string{
				"Couldn't register ⌃⌥⇧Tab (another app might be using it). Try ⌃⌥` instead.",
			},
		},
		{
			name:  "suggestion taken too",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+Space", "Ctrl+Option+J"},
			want: []string{
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥⌘J instead.",
			},
		},
		{
			name:  "suggestion used by another field",
			cfg:   withField(defaultShortcuts, "rearrange", "ctrl+alt+j"),
			taken: []string{"Ctrl+Option+Space"},
			want: []string{
				"Couldn't register ⌃⌥Space (macOS uses it to switch input sources). Try ⌃⌥⌘J instead.",
			},
		},
		{
			name:  "two fields",
			cfg:   defaultShortcuts,
			taken: []string{"Ctrl+Option+]", "Ctrl+Option+0"},
			want: []string{
				"Couldn't register ⌃⌥] (another app might be using it). Try ⌃⌥. instead.",
				"Couldn't register ⌃⌥0 (another app might be using it). Try ⌃⌥⌘0 instead.",
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
	(&shortcutSet{reg: reg, e: f}).register()
	if got, want := keys(reg.callbacks), freeAccelerators(cfg, reg.taken); !slices.Equal(got, want) {
		t.Errorf("registered %q, want just the free bindings %q: probing left some", got, want)
	}
	report, _ := f.lastReport()
	return noticeTexts(report.Notices)
}

// noticeTexts returns the notices' texts.
func noticeTexts(notices []workspace.NoticeView) []string {
	var out []string
	for _, n := range notices {
		out = append(out, n.Text)
	}
	return out
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

// reportFor registers cfg's shortcuts with the given accelerators taken and
// the given keys that macOS keeps for itself, and returns the report.
func reportFor(t *testing.T, cfg shortcuts.Config, sys platform.SystemKeys, taken ...string) workspace.ShortcutReport {
	t.Helper()
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: cfg}
	reg := &fakeRegistrar{taken: map[string]bool{}}
	for _, a := range taken {
		reg.taken[a] = true
	}
	(&shortcutSet{reg: reg, e: f, system: func() platform.SystemKeys { return sys }}).register()
	report, ok := f.lastReport()
	if !ok {
		t.Fatal("register didn't report")
	}
	return report
}

// fieldOf returns a field's row from a report.
func fieldOf(t *testing.T, rows []workspace.ShortcutView, field string) workspace.ShortcutView {
	t.Helper()
	for _, v := range rows {
		if v.Field == field {
			return v
		}
	}
	t.Fatalf("no row for %s in %+v", field, rows)
	return workspace.ShortcutView{}
}

// TestShortcutNoticeActions checks the buttons of a notice about a
// shortcut that couldn't be registered: Use this instead, for the field,
// and Keyboard settings; and that the field's row says why and suggests the
// same.
func TestShortcutNoticeActions(t *testing.T) {
	report := reportFor(t, defaultShortcuts, platform.SystemKeys{}, "Ctrl+Option+Space")
	if len(report.Notices) != 1 {
		t.Fatalf("notices %+v", report.Notices)
	}
	want := []workspace.NoticeAction{
		{Label: "Use this instead", Method: "UseSuggestedShortcuts", Arg: "jump"},
		{Label: "Keyboard settings", Command: "keyboard-settings"},
	}
	if got := report.Notices[0].Actions; !reflect.DeepEqual(got, want) {
		t.Errorf("actions %+v, want %+v", got, want)
	}
	jump := fieldOf(t, report.Fields, "jump")
	if jump.Problem != "Bench couldn't register ⌃⌥Space: macOS uses it to switch input sources." ||
		jump.Suggestion != "ctrl+alt+j" || jump.SuggestionDisplay != "⌃⌥J" || jump.Display != "⌃⌥Space" || jump.Label != "Jump to…" {
		t.Errorf("jump: %+v", jump)
	}

	// Without a suggestion, there's only Keyboard settings.
	bindings, _ := shortcuts.Bindings(defaultShortcuts)
	notices := shortcutNotices([]fieldCheck{{
		view:    workspace.ShortcutView{Field: "jump", Value: "ctrl+alt+space"},
		problem: problemTaken,
		group:   bindingsOf(bindings, "jump"),
	}})
	if len(notices) != 1 || !reflect.DeepEqual(notices[0].Actions, want[1:]) ||
		notices[0].Text != "Couldn't register ⌃⌥Space (macOS uses it to switch input sources)." {
		t.Errorf("notices without a suggestion %+v", notices)
	}
}

// TestShortcutsClashWithInputSources checks that a shortcut that macOS
// uses to switch input sources, which RegisterEventHotKey takes without a
// word, is reported, with another to try.
func TestShortcutsClashWithInputSources(t *testing.T) {
	report := reportFor(t, defaultShortcuts, platform.SystemKeys{InputSources: []string{"ctrl+space", "ctrl+alt+space"}})
	if got := noticeTexts(report.Notices); !slices.Equal(got, []string{
		"macOS uses ⌃⌥Space to switch input sources, so Bench won't see it. Try ⌃⌥J instead.",
	}) {
		t.Errorf("notices %q", got)
	}
	if got := report.Notices[0].Actions[0]; got.Method != "UseSuggestedShortcuts" || got.Arg != "jump" {
		t.Errorf("action %+v", got)
	}
	jump := fieldOf(t, report.Fields, "jump")
	if jump.Problem != "macOS uses ⌃⌥Space to switch input sources, so Bench won't see it." || jump.Suggestion != "ctrl+alt+j" {
		t.Errorf("jump: %+v", jump)
	}
	if next := fieldOf(t, report.Fields, "next_tab"); next.Problem != "" {
		t.Errorf("next_tab: %+v", next)
	}
}

// TestShortcutsClashWithVoiceOver checks that, while VoiceOver is on, the
// shortcuts with ⌃⌥ are reported in one notice, with ⌃⌘ in place of ⌃⌥
// suggested for each field, but never a shortcut that macOS keeps for
// itself (⌃⌘Space) or another field's.
func TestShortcutsClashWithVoiceOver(t *testing.T) {
	cfg := withField(withField(defaultShortcuts, "zoom_pane", "ctrl+alt+return"), "last_tab", "ctrl+alt+`")
	report := reportFor(t, cfg, platform.SystemKeys{VoiceOver: true})
	if len(report.Notices) != 1 {
		t.Fatalf("notices %q, want one", noticeTexts(report.Notices))
	}
	n := report.Notices[0]
	if want := "VoiceOver is on, and it uses ⌃⌥ for its own commands, so Bench's shortcuts with ⌃⌥ may not work. " +
		"Try ⌃⌘ in place of ⌃⌥ (and ⌃⌘J for Jump to…)."; n.Text != want {
		t.Errorf("notice %q\nwant %q", n.Text, want)
	}
	all := "tab_modifiers next_tab previous_tab last_tab jump rearrange focus_pane_modifiers move_window_modifiers next_window zoom_pane"
	if n.Actions[0].Arg != all {
		t.Errorf("Use this instead changes %q, want %q", n.Actions[0].Arg, all)
	}
	want := map[string]string{
		"tab_modifiers":         "ctrl+cmd",
		"next_tab":              "ctrl+cmd+]",
		"previous_tab":          "ctrl+cmd+[",
		"last_tab":              "ctrl+cmd+`",
		"jump":                  "ctrl+cmd+j",
		"rearrange":             "ctrl+cmd+0",
		"focus_pane_modifiers":  "ctrl+cmd",
		"move_window_modifiers": "ctrl+shift+cmd",
		"next_window":           "ctrl+cmd+tab",
		"zoom_pane":             "ctrl+cmd+return",
	}
	suggested := map[string]string{}
	for _, v := range report.Fields {
		if v.Value == "" {
			// A field that's off, as add_front_window is by default,
			// has no shortcut to clash.
			if v.Problem != "" {
				t.Errorf("%s is off, but has a problem: %s", v.Field, v.Problem)
			}
			continue
		}
		if v.Problem == "" {
			t.Errorf("%s has no problem", v.Field)
		}
		suggested[v.Field] = v.Suggestion
	}
	if !reflect.DeepEqual(suggested, want) {
		t.Errorf("suggestions %v\nwant %v", suggested, want)
	}
	// Together, the suggestions make no problem.
	if _, errs := shortcuts.Bindings(shortcuts.FromFields(suggested)); len(errs) > 0 {
		t.Errorf("the suggestions clash: %v", errs)
	}

	// Without ⌃⌥, there's nothing to say.
	report = reportFor(t, shortcuts.FromFields(suggested), platform.SystemKeys{VoiceOver: true})
	if len(report.Notices) != 0 {
		t.Errorf("notices %q", noticeTexts(report.Notices))
	}
}

// TestShortcutsReservedByMacOS checks that a shortcut that macOS usually
// keeps for itself, such as ⌘Space for Spotlight, is reported, since Bench
// can register it but may never see it.
func TestShortcutsReservedByMacOS(t *testing.T) {
	report := reportFor(t, withField(defaultShortcuts, "jump", "cmd+space"), platform.SystemKeys{})
	if got := noticeTexts(report.Notices); !slices.Equal(got, []string{
		"macOS usually keeps ⌘Space for itself, so Bench may not see it. Try ⌘J instead.",
	}) {
		t.Errorf("notices %q", got)
	}
}

// TestShortcutNoticesForTableProblemsSuggest checks that a field that
// another field's shortcut makes unusable gets a suggestion, and a field
// that isn't a shortcut gets its default.
func TestShortcutNoticesForTableProblemsSuggest(t *testing.T) {
	cfg := withField(defaultShortcuts, "jump", "hyper+x")
	cfg = withField(cfg, "previous_tab", "ctrl+alt+]")
	report := reportFor(t, cfg, platform.SystemKeys{})
	prev := fieldOf(t, report.Fields, "previous_tab")
	if prev.Problem != "⌃⌥] is already the shortcut for Next tab." || prev.Suggestion != "ctrl+alt+." {
		t.Errorf("previous_tab: %+v", prev)
	}
	if jump := fieldOf(t, report.Fields, "jump"); jump.Suggestion != "ctrl+alt+space" || jump.Problem == "" {
		t.Errorf("jump: %+v", jump)
	}
	for _, n := range report.Notices {
		if !strings.HasSuffix(n.Text, " instead.") || n.Actions[0].Method != "UseSuggestedShortcuts" {
			t.Errorf("notice %+v", n)
		}
	}
}

// TestCheckShortcuts checks Settings ▸ Keyboard's check of a draft: the
// shortcuts that Bench holds count as free, one that another app has
// doesn't, nothing stays registered but Bench's own, and a draft that gives
// two fields the same shortcut says so.
func TestCheckShortcuts(t *testing.T) {
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: defaultShortcuts}
	reg := &fakeRegistrar{taken: map[string]bool{"Ctrl+Option+K": true}}
	set := &shortcutSet{reg: reg, e: f}
	set.register()
	held := keys(reg.callbacks)

	fields := defaultShortcuts.Fields()
	fields["jump"] = "ctrl+alt+k"
	rows := set.check(fields)
	if got := keys(reg.callbacks); !slices.Equal(got, held) {
		t.Errorf("registered after the check %q, want just Bench's own %q", got, held)
	}
	jump := fieldOf(t, rows, "jump")
	if !strings.HasPrefix(jump.Problem, "Bench couldn't register ⌃⌥K") || jump.Suggestion != "ctrl+alt+cmd+k" {
		t.Errorf("jump: %+v", jump)
	}
	for _, v := range rows {
		if v.Field != "jump" && v.Problem != "" {
			t.Errorf("%s: %+v, want it free, as Bench holds it", v.Field, v)
		}
	}

	fields["jump"] = "ctrl+alt+]"
	rows = set.check(fields)
	if jump := fieldOf(t, rows, "jump"); jump.Problem != "⌃⌥] is already the shortcut for Next tab." {
		t.Errorf("jump the same as next_tab: %+v", jump)
	}
	if got := len(f.reports); got != 1 {
		t.Errorf("check reported %d times; want only register's report", got)
	}
}

// TestPauseShortcuts checks that pausing releases every shortcut, that
// registering while paused waits for the pause to end, and that ending it
// registers the shortcuts again.
func TestPauseShortcuts(t *testing.T) {
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: defaultShortcuts}
	reg := &fakeRegistrar{}
	set := &shortcutSet{reg: reg, e: f}
	set.register()
	before := keys(reg.callbacks)
	if len(before) == 0 {
		t.Fatal("nothing registered")
	}
	set.pause(true)
	if len(reg.callbacks) != 0 {
		t.Errorf("registered while paused %q", keys(reg.callbacks))
	}
	set.register()
	if len(reg.callbacks) != 0 {
		t.Errorf("register while paused registered %q", keys(reg.callbacks))
	}
	reports := len(f.reports)
	set.pause(false)
	if got := keys(reg.callbacks); !slices.Equal(got, before) {
		t.Errorf("registered after the pause %q, want %q", got, before)
	}
	if len(f.reports) != reports+1 {
		t.Errorf("ending the pause didn't report")
	}
	set.pause(false)
	if len(f.reports) != reports+1 {
		t.Errorf("ending the pause twice registered again")
	}
}

// TestPreviewRegistrar checks that the preview's shortcuts are checked as
// the app's are, but that it takes every shortcut, as it registers none.
func TestPreviewRegistrar(t *testing.T) {
	f := newFakeEngine()
	f.cfg = workspace.ConfigInfo{Shortcuts: withField(defaultShortcuts, "previous_tab", "ctrl+alt+]")}
	set := &shortcutSet{reg: previewRegistrar{}, e: f}
	set.register()
	report, _ := f.lastReport()
	if got := noticeTexts(report.Notices); len(got) != 1 || !strings.HasPrefix(got[0], "Couldn't use shortcuts.previous_tab in config.toml: ") {
		t.Errorf("notices %q", got)
	}
	em := newEmitter(&fakeSink{}, &fakeShell{})
	if rows := em.CheckShortcuts(defaultShortcuts.Fields()); rows != nil {
		t.Errorf("an emitter without shortcuts checked them: %+v", rows)
	}
	em.keys = set
	if rows := em.CheckShortcuts(defaultShortcuts.Fields()); len(rows) != len(shortcuts.FieldNames()) {
		t.Errorf("the emitter's check: %+v", rows)
	}
}
