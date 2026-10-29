package config

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"unicode"

	"github.com/itbm/bench/internal/layout"
)

// presetShape is what validation needs to know about a preset: how many panes
// it has, and how many children its top-level split has (0 for a single
// pane, which has no split).
type presetShape struct {
	panes, children int
}

// presetShapes mirrors the spec's presets table and layout.Presets. It's kept
// here so that checking a config doesn't depend on building trees; a test
// checks that the two agree.
var presetShapes = map[string]presetShape{
	layout.PresetSingle:    {panes: 1, children: 0},
	layout.PresetSide:      {panes: 2, children: 2},
	layout.PresetMainSide:  {panes: 2, children: 2},
	layout.PresetThree:     {panes: 3, children: 3},
	layout.PresetTopBottom: {panes: 2, children: 2},
	layout.PresetGrid:      {panes: 4, children: 2},
}

// presetList is the presets in editor order, for messages.
const presetList = "single, side, main-side, three, top-bottom, or grid"

// Item kinds, by the field that sets them, in the order messages list them.
const (
	keyApp      = "app"
	keyWeb      = "web"
	keyVSCode   = "vscode"
	keyTerminal = "terminal"
)

var kindKeys = []string{keyApp, keyWeb, keyVSCode, keyTerminal}

// The smallest and largest terminal_font_size settings, in points.
const (
	MinTerminalFontSize = 9
	MaxTerminalFontSize = 24
)

// MaxGap is the largest gap setting, in points. Panes are laid out with the
// gap around and between them, so a gap wider than the window would put
// them, and the windows docked in them, off screen, where they'd stay
// after Bench quits.
const MaxGap = 100

// itemRef is an item's place in a config, counting from 0.
type itemRef struct{ tab, pane, item int }

// presence records which of an item's kind fields (app, web, vscode,
// terminal) a file sets, even to "". The Item struct can't tell an empty
// field from a missing one, so Parse supplies this.
type presence map[itemRef][]string

// Validate checks a config and returns every problem: no tabs; an unknown
// preset ("custom" isn't allowed) or small-screen setting; more panes than
// the preset has; sizes that don't have one positive number per child of
// the preset's top-level split (they needn't add up to 100); an item with
// none, or more than one, of app, web, vscode, and terminal, or with an empty
// one; fields that don't apply to an item's kind (title and url are for app
// items, url only with title, profile for web items, command for terminal
// items, and name for web, vscode, and terminal items); a web address that
// isn't https with a host; an empty or duplicate tab name (ignoring case); a
// rule with no app, with both or neither of float and tab, or naming a tab
// that doesn't exist; an unknown menu_bar, tab_shortcut_hints,
// scroll_bars, density, pane_headers, new_windows, or follow_focus value; a
// non-positive compact_below; a gap below 0 or above MaxGap; a
// terminal_font_size outside MinTerminalFontSize to MaxTerminalFontSize; an
// app item's url that has no scheme; a non-positive launch_timeout; a
// terminal item with both command and run, or start_again without run; a
// run that isn't one line of text; and a launcher with no name, a name that
// another has (ignoring case), or fields that don't apply to its kind (see
// launcher). It doesn't check shortcuts: the shortcuts package reports
// those when Bench registers them. A nil config is one problem.
func (c *Config) Validate() []Problem {
	if c == nil {
		return []Problem{{Message: "there's no config"}}
	}
	return validate(c, nil)
}

func validate(c *Config, present presence) []Problem {
	var v validator
	v.settings(c.Settings)
	for i, l := range c.Launchers {
		v.launcher(c, i, l)
	}
	if len(c.Tabs) == 0 {
		v.add("", "tab", "there are no tabs; add at least one [[tab]]")
	}
	for i := range c.Tabs {
		v.tab(c, i, present)
	}
	for i, r := range c.Rules {
		v.rule(c, i, r)
	}
	return v.problems
}

// validator collects problems.
type validator struct {
	problems []Problem
}

func (v *validator) add(tab, field, format string, args ...any) {
	v.problems = append(v.problems, Problem{Tab: tab, Field: field, Message: fmt.Sprintf(format, args...)})
}

func (v *validator) settings(s Settings) {
	if !(s.CompactBelow > 0) || math.IsInf(s.CompactBelow, 0) {
		v.add("", "settings.compact_below", "must be a positive number of points")
	}
	if !(s.Gap >= 0 && s.Gap <= MaxGap) {
		v.add("", "settings.gap", "must be from 0 to %d points", MaxGap)
	}
	if !(s.TerminalFontSize >= MinTerminalFontSize && s.TerminalFontSize <= MaxTerminalFontSize) {
		v.add("", "settings.terminal_font_size", "must be from %d to %d points", MinTerminalFontSize, MaxTerminalFontSize)
	}
	switch s.MenuBar {
	case MenuBarName, MenuBarIcon, MenuBarHidden:
	default:
		v.add("", "settings.menu_bar", "must be name, icon, or hidden, not %q", s.MenuBar)
	}
	switch s.TabShortcutHints {
	case TabHintsAuto, TabHintsAlways, TabHintsNever:
	default:
		v.add("", "settings.tab_shortcut_hints", "must be auto, always, or never, not %q", s.TabShortcutHints)
	}
	switch s.ScrollBars {
	case ScrollBarsAuto, ScrollBarsAlways, ScrollBarsNever:
	default:
		v.add("", "settings.scroll_bars", "must be auto, always, or never, not %q", s.ScrollBars)
	}
	switch s.Density {
	case DensityAuto, DensityStandard, DensityCompact:
	default:
		v.add("", "settings.density", "must be auto, standard, or compact, not %q", s.Density)
	}
	switch s.PaneHeaders {
	case PaneHeadersAlways, PaneHeadersAuto:
	default:
		v.add("", "settings.pane_headers", "must be always or auto, not %q", s.PaneHeaders)
	}
	switch s.NewWindows {
	case NewWindowsAppPane, NewWindowsFocusedPane, NewWindowsFloat:
	default:
		v.add("", "settings.new_windows", "must be app-pane, focused-pane, or float, not %q", s.NewWindows)
	}
	switch s.FollowFocus {
	case FollowSwitchTab, FollowBringHere, FollowOff:
	default:
		v.add("", "settings.follow_focus", "must be switch-tab, bring-here, or off, not %q", s.FollowFocus)
	}
	if s.LaunchTimeout.Duration <= 0 {
		v.add("", "settings.launch_timeout", "must be a positive duration, such as \"15s\"")
	}
}

// tabLabel names a tab in problems: its name, or "tab 2" (counting from 1)
// if it has none.
func tabLabel(name string, index int) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return fmt.Sprintf("tab %d", index+1)
}

func (v *validator) tab(c *Config, i int, present presence) {
	t := c.Tabs[i]
	label := tabLabel(t.Name, i)

	if strings.TrimSpace(t.Name) == "" {
		v.add(label, "name", "needs a name")
	} else {
		for j := 0; j < i; j++ {
			if strings.EqualFold(c.Tabs[j].Name, t.Name) {
				v.add(label, "name", "tab %d is already called %q; tab names must be unique, ignoring case", j+1, c.Tabs[j].Name)
				break
			}
		}
	}

	shape, known := presetShapes[t.Layout]
	switch {
	case known:
	case t.Layout == "":
		v.add(label, "layout", "needs a preset: %s", presetList)
	case t.Layout == layout.PresetCustom:
		v.add(label, "layout", "%q can't be used in the config; use %s", t.Layout, presetList)
	default:
		v.add(label, "layout", "%q isn't a preset; use %s", t.Layout, presetList)
	}

	switch layout.SmallScreens(t.SmallScreens) {
	case "", layout.Stack, layout.Tabs, layout.Same:
	default:
		v.add(label, "small_screens", "%q isn't a small-screen setting; use stack, tabs, or same", t.SmallScreens)
	}

	if known {
		v.sizes(label, t, shape)
		if len(t.Panes) > shape.panes {
			v.add(label, fmt.Sprintf("pane[%d]", shape.panes+1),
				"the %q layout has %d %s, but this tab has %d [[tab.pane]] entries",
				t.Layout, shape.panes, plural(shape.panes, "pane", "panes"), len(t.Panes))
		}
	} else {
		v.positiveSizes(label, t.Sizes)
	}

	for p, pane := range t.Panes {
		for k, it := range pane.Items {
			ref := itemRef{tab: i, pane: p, item: k}
			kinds := itemKinds(it)
			if present != nil {
				if keys, ok := present[ref]; ok {
					kinds = keys
				}
			}
			v.item(label, fmt.Sprintf("pane[%d].items[%d]", p+1, k+1), it, kinds)
		}
	}
}

func (v *validator) sizes(label string, t Tab, shape presetShape) {
	if len(t.Sizes) == 0 {
		return
	}
	switch {
	case shape.children == 0:
		v.add(label, "sizes", "the %q layout has no split, so it takes no sizes", t.Layout)
		return
	case len(t.Sizes) != shape.children:
		v.add(label, "sizes", "the %q layout splits into %d, so sizes needs %d numbers, not %d",
			t.Layout, shape.children, shape.children, len(t.Sizes))
	}
	v.positiveSizes(label, t.Sizes)
}

func (v *validator) positiveSizes(label string, sizes []float64) {
	for k, s := range sizes {
		if !(s > 0) || math.IsInf(s, 0) {
			v.add(label, fmt.Sprintf("sizes[%d]", k+1), "must be a positive number")
		}
	}
}

// itemKinds returns the kind fields an item sets.
func itemKinds(it Item) []string {
	var kinds []string
	for _, k := range kindKeys {
		if itemField(it, k) != "" {
			kinds = append(kinds, k)
		}
	}
	return kinds
}

// itemField returns one of an item's kind fields.
func itemField(it Item, key string) string {
	switch key {
	case keyApp:
		return it.App
	case keyWeb:
		return it.Web
	case keyVSCode:
		return it.VSCode
	case keyTerminal:
		return it.Terminal
	}
	return ""
}

func (v *validator) item(label, path string, it Item, kinds []string) {
	switch len(kinds) {
	case 0:
		v.add(label, path, "needs one of app, web, vscode, or terminal")
		return
	case 1:
	default:
		v.add(label, path, "has %s, but needs exactly one of app, web, vscode, or terminal", listAnd(kinds))
		return
	}
	kind := kinds[0]
	field := func(name string) string { return path + "." + name }

	value := itemField(it, kind)
	switch kind {
	case keyApp:
		if strings.TrimSpace(value) == "" {
			v.add(label, field(kind), "needs a bundle ID, such as \"dev.zed.Zed\"")
		}
	case keyWeb:
		if msg := checkHTTPS(value); msg != "" {
			v.add(label, field(kind), "%s", msg)
		}
	case keyVSCode:
		if strings.TrimSpace(value) == "" {
			v.add(label, field(kind), "needs a folder, such as \"~/code\"")
		}
	case keyTerminal:
		if strings.TrimSpace(value) == "" {
			v.add(label, field(kind), "needs a starting folder, such as \"~\"")
		}
	}

	if kind != keyApp {
		if it.Title != "" {
			v.add(label, field("title"), "only applies to app items")
		}
		if it.URL != "" {
			v.add(label, field("url"), "only applies to app items")
		}
	} else {
		if it.URL != "" && it.Title == "" {
			v.add(label, field("url"), "needs title too: Bench opens this link when no window matches the title")
		} else if msg := checkLink(it.URL); msg != "" {
			v.add(label, field("url"), "%s", msg)
		}
		if it.Name != "" {
			v.add(label, field("name"), "only applies to web, vscode, and terminal items")
		}
	}
	if kind != keyWeb && it.Profile != "" {
		v.add(label, field("profile"), "only applies to web items")
	}
	if kind != keyTerminal && it.Command != "" {
		v.add(label, field("command"), "only applies to terminal items")
	}
	switch {
	case it.Run == "":
	case kind != keyTerminal:
		v.add(label, field("run"), "only applies to terminal items")
	case it.Command != "":
		v.add(label, field("run"), "can't go with command; use one of them: command runs instead of your shell, and run is typed into it")
	default:
		if msg := checkRun(it.Run); msg != "" {
			v.add(label, field("run"), "%s", msg)
		}
	}
	if it.StartAgain != nil && (kind != keyTerminal || it.Run == "") {
		v.add(label, field("start_again"), "only applies to terminal items with run")
	}
}

// launcher checks a [[launcher]] entry: it needs a name, unique ignoring
// case. A terminal launcher's run must be one line of text. An app
// launcher, one with app, needs a folder or a url to open, not both, and a
// url needs a title, as an app item's does; run and start_again are only
// for terminal launchers, and title and url only for app launchers.
func (v *validator) launcher(c *Config, i int, l Launcher) {
	path := fmt.Sprintf("launcher[%d]", i+1)
	field := func(name string) string { return path + "." + name }
	switch {
	case strings.TrimSpace(l.Name) == "":
		v.add("", field("name"), "needs a name, which the add dialog and Jump to… show")
	case strings.ContainsAny(l.Name, "\r\n"):
		v.add("", field("name"), "must be one line")
	default:
		for j := 0; j < i; j++ {
			if strings.EqualFold(c.Launchers[j].Name, l.Name) {
				v.add("", field("name"), "launcher %d is already called %q; launcher names must be unique, ignoring case", j+1, c.Launchers[j].Name)
				break
			}
		}
	}
	if !l.IsApp() {
		if l.Title != "" {
			v.add("", field("title"), "only applies to app launchers")
		}
		if l.URL != "" {
			v.add("", field("url"), "only applies to app launchers")
		}
		if msg := checkRun(l.Run); msg != "" {
			v.add("", field("run"), "%s", msg)
		}
		return
	}
	if strings.TrimSpace(l.App) == "" {
		v.add("", field("app"), "needs a bundle ID, such as \"dev.zed.Zed\"")
	}
	if l.Run != "" {
		v.add("", field("run"), "only applies to terminal launchers")
	}
	if l.StartAgain != nil {
		v.add("", field("start_again"), "only applies to terminal launchers")
	}
	switch {
	case l.Folder != "" && l.URL != "":
		v.add("", path, "has both folder and url; use one of them")
	case strings.TrimSpace(l.Folder) == "" && l.URL == "":
		v.add("", path, "needs a folder or a url to open in the app")
	case l.URL != "" && l.Title == "":
		v.add("", field("url"), "needs title too: Bench docks the app's window whose title has it")
	default:
		if msg := checkLink(l.URL); msg != "" {
			v.add("", field("url"), "%s", msg)
		}
	}
}

// checkRun returns why a command can't be typed into a shell, or "": it's
// typed as it is, so a line break or a tab would act as the key would.
func checkRun(run string) string {
	if strings.ContainsFunc(run, unicode.IsControl) {
		return "must be one line of text, without tabs"
	}
	return ""
}

// checkHTTPS returns why a web item's address isn't usable, or "".
func checkHTTPS(addr string) string {
	if strings.TrimSpace(addr) == "" {
		return "needs an https address, such as \"https://app.slack.com\""
	}
	u, err := url.Parse(addr)
	if err != nil || u.Scheme != "https" {
		return fmt.Sprintf("%q isn't an https address", addr)
	}
	if u.Hostname() == "" {
		return fmt.Sprintf("%q has no host name", addr)
	}
	return ""
}

// checkLink returns why an app item's link can't be opened, or "". macOS
// opens a link in the app only if it has a scheme, such as https: or
// notion:.
func checkLink(link string) string {
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return fmt.Sprintf("%q isn't a link", link)
	}
	if u.Scheme == "" {
		return fmt.Sprintf("%q has no scheme; start it with one, such as https://", link)
	}
	return ""
}

func (v *validator) rule(c *Config, i int, r Rule) {
	path := fmt.Sprintf("rule[%d]", i+1)
	if strings.TrimSpace(r.App) == "" {
		v.add("", path+".app", "needs a bundle ID, such as \"com.brave.Browser\"")
	}
	switch {
	case r.Float && r.Tab != "":
		v.add("", path, "has both float = true and tab; use one of them")
	case !r.Float && r.Tab == "":
		v.add("", path, "needs float = true, or a tab for the app's new windows")
	case r.Tab != "" && c.TabIndex(r.Tab) < 0:
		v.add("", path+".tab", "there's no tab called %q", r.Tab)
	}
}

// listAnd joins words as "a and b" or "a, b, and c".
func listAnd(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	}
	return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
