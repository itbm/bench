package config

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Comment columns in the default file, counting from 0.
const (
	settingsCommentColumn  = 30
	shortcutsCommentColumn = 36
)

// maxItemsLine is how long an items line can be before Format puts each item
// on a line of its own.
const maxItemsLine = 100

// Format writes a config as TOML in the layout of the default file: comments
// on settings, [shortcuts], each [[tab]] with its [[tab.pane]] entries and
// inline item tables, then the [[rule]] entries. Format(Default()) is
// DefaultTOML().
//
// Fields that are empty are left out, except a tab's name and layout and a
// pane's items. In particular, an empty small_screens is left out, because
// a tab without it uses stack. A pane with no items is written as
// "items = []". For a config without problems, Parse(Format(c)) returns a
// config equal to c, except that empty lists come back as nil.
func Format(c *Config) []byte {
	if c == nil {
		return nil
	}
	var w writer
	w.settings(c.Settings)
	w.line("")
	w.shortcuts(c.Shortcuts)
	for _, t := range c.Tabs {
		w.line("")
		w.tab(t)
	}
	for _, r := range c.Rules {
		w.line("")
		w.rule(r)
	}
	return w.buf.Bytes()
}

// writer builds a TOML file.
type writer struct {
	buf bytes.Buffer
}

func (w *writer) line(s string) {
	w.buf.WriteString(s)
	w.buf.WriteByte('\n')
}

// keyValue writes "key = value", with a comment at column if there is one.
func (w *writer) keyValue(indent, key, value, comment string, column int) {
	s := indent + key + " = " + value
	if comment != "" {
		pad := max(column-len(s), 2)
		s += strings.Repeat(" ", pad) + "# " + comment
	}
	w.line(s)
}

func (w *writer) settings(s Settings) {
	const col = settingsCommentColumn
	w.line("[settings]")
	w.keyValue("", "compact_below", formatFloat(s.CompactBelow), "Screens narrower than this, in points, are small.", col)
	w.keyValue("", "gap", formatFloat(s.Gap), "Points between panes.", col)
	w.keyValue("", "open_missing_apps", formatBool(s.OpenMissingApps), "Open a tab's apps if they aren't running.", col)
	w.keyValue("", "hide_other_tabs", formatBool(s.HideOtherTabs), "Hide windows that belong to other tabs.", col)
	w.keyValue("", "hide_other_apps", formatBool(s.HideOtherApps), "Also hide every app Bench doesn't manage.", col)
	w.keyValue("", "new_windows", quote(s.NewWindows), "app-pane, focused-pane, or float", col)
	w.keyValue("", "follow_focus", quote(s.FollowFocus), "switch-tab, bring-here, or off", col)
	w.keyValue("", "live_previews", formatBool(s.LivePreviews), "Needs Screen Recording permission.", col)
	w.keyValue("", "launch_timeout", quote(s.LaunchTimeout.String()), "", col)
}

func (w *writer) shortcuts(s Shortcuts) {
	const col = shortcutsCommentColumn
	w.line("[shortcuts]")
	w.keyValue("", "tab_modifiers", quote(s.TabModifiers), "Plus 1 to 9.", col)
	w.keyValue("", "next_tab", quote(s.NextTab), "", col)
	w.keyValue("", "previous_tab", quote(s.PreviousTab), "", col)
	w.keyValue("", "jump", quote(s.Jump), "", col)
	w.keyValue("", "rearrange", quote(s.Rearrange), "", col)
	w.keyValue("", "focus_pane_modifiers", quote(s.FocusPaneModifiers), "Plus an arrow key.", col)
	w.keyValue("", "move_window_modifiers", quote(s.MoveWindowModifiers), "", col)
	w.keyValue("", "next_window", quote(s.NextWindow), "", col)
}

func (w *writer) tab(t Tab) {
	w.line("[[tab]]")
	w.keyValue("", "name", quote(t.Name), "", 0)
	w.keyValue("", "layout", quote(t.Layout), "", 0)
	if len(t.Sizes) > 0 {
		sizes := make([]string, len(t.Sizes))
		for i, s := range t.Sizes {
			sizes[i] = formatFloat(s)
		}
		w.keyValue("", "sizes", "["+strings.Join(sizes, ", ")+"]", "", 0)
	}
	if t.SmallScreens != "" {
		w.keyValue("", "small_screens", quote(t.SmallScreens), "", 0)
	}
	for _, p := range t.Panes {
		w.line("")
		w.line("  [[tab.pane]]")
		w.items(p.Items)
	}
}

// items writes a pane's items as inline tables: on one line if it's short
// enough, otherwise one item to a line.
func (w *writer) items(items []Item) {
	const indent = "  "
	tables := make([]string, len(items))
	for i, it := range items {
		tables[i] = inlineItem(it)
	}
	one := indent + "items = [" + strings.Join(tables, ", ") + "]"
	if len(items) < 2 || len(one) <= maxItemsLine {
		w.line(one)
		return
	}
	w.line(indent + "items = [")
	for _, t := range tables {
		w.line(indent + "  " + t + ",")
	}
	w.line(indent + "]")
}

// inlineItem writes an item as an inline table, with its kind field first.
func inlineItem(it Item) string {
	fields := []struct{ key, value string }{
		{"app", it.App},
		{"web", it.Web},
		{"vscode", it.VSCode},
		{"terminal", it.Terminal},
		{"title", it.Title},
		{"url", it.URL},
		{"profile", it.Profile},
		{"command", it.Command},
		{"name", it.Name},
	}
	var parts []string
	for _, f := range fields {
		if f.value != "" {
			parts = append(parts, f.key+" = "+quote(f.value))
		}
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func (w *writer) rule(r Rule) {
	w.line("[[rule]]")
	w.keyValue("", "app", quote(r.App), "", 0)
	if r.Title != "" {
		w.keyValue("", "title", quote(r.Title), "", 0)
	}
	if r.Float {
		w.keyValue("", "float", "true", "", 0)
	}
	if r.Tab != "" {
		w.keyValue("", "tab", quote(r.Tab), "", 0)
	}
}

// formatFloat writes a number as TOML: whole numbers as integers ("1800"),
// others in the shortest form that reads back the same.
func formatFloat(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	case v == math.Trunc(v) && math.Abs(v) < 1e15:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}

func formatBool(b bool) string {
	return strconv.FormatBool(b)
}

// quote writes a TOML basic string. Invalid UTF-8 becomes U+FFFD, because
// TOML files must be UTF-8.
func quote(s string) string {
	s = strings.ToValidUTF8(s, string(utf8.RuneError))
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
