package config

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	// Each case changes the example config and lists where the problems
	// are. Example() has tabs Comms (side), Code (main-side), and Research
	// (side), and one rule.
	tests := []struct {
		name   string
		change func(c *Config)
		want   []where
		// msg, if set, must be in the first problem's message.
		msg string
	}{
		{"default", func(c *Config) {}, nil, ""},

		// Settings.
		{"compact_below zero", func(c *Config) { c.Settings.CompactBelow = 0 }, []where{{"", "settings.compact_below"}}, "positive"},
		{"compact_below negative", func(c *Config) { c.Settings.CompactBelow = -1 }, []where{{"", "settings.compact_below"}}, ""},
		{"compact_below NaN", func(c *Config) { c.Settings.CompactBelow = math.NaN() }, []where{{"", "settings.compact_below"}}, ""},
		{"compact_below infinite", func(c *Config) { c.Settings.CompactBelow = math.Inf(1) }, []where{{"", "settings.compact_below"}}, ""},
		{"gap zero", func(c *Config) { c.Settings.Gap = 0 }, nil, ""},
		{"gap negative", func(c *Config) { c.Settings.Gap = -1 }, []where{{"", "settings.gap"}}, "from 0 to 100"},
		{"gap NaN", func(c *Config) { c.Settings.Gap = math.NaN() }, []where{{"", "settings.gap"}}, ""},
		{"gap at the limit", func(c *Config) { c.Settings.Gap = MaxGap }, nil, ""},
		{"gap over the limit", func(c *Config) { c.Settings.Gap = 8000 }, []where{{"", "settings.gap"}}, "from 0 to 100"},
		{"gap infinite", func(c *Config) { c.Settings.Gap = math.Inf(1) }, []where{{"", "settings.gap"}}, ""},
		{"terminal_font_size smallest", func(c *Config) { c.Settings.TerminalFontSize = MinTerminalFontSize }, nil, ""},
		{"terminal_font_size largest", func(c *Config) { c.Settings.TerminalFontSize = MaxTerminalFontSize }, nil, ""},
		{"terminal_font_size half a point", func(c *Config) { c.Settings.TerminalFontSize = 13.5 }, nil, ""},
		{"terminal_font_size too small", func(c *Config) { c.Settings.TerminalFontSize = 8.5 }, []where{{"", "settings.terminal_font_size"}}, "from 9 to 24"},
		{"terminal_font_size too large", func(c *Config) { c.Settings.TerminalFontSize = 25 }, []where{{"", "settings.terminal_font_size"}}, "from 9 to 24"},
		{"terminal_font_size zero", func(c *Config) { c.Settings.TerminalFontSize = 0 }, []where{{"", "settings.terminal_font_size"}}, ""},
		{"terminal_font_size NaN", func(c *Config) { c.Settings.TerminalFontSize = math.NaN() }, []where{{"", "settings.terminal_font_size"}}, ""},
		{"dropdown_height smallest", func(c *Config) { c.Settings.DropdownHeight = MinDropdownHeight }, nil, ""},
		{"dropdown_height largest", func(c *Config) { c.Settings.DropdownHeight = MaxDropdownHeight }, nil, ""},
		{"dropdown_height too small", func(c *Config) { c.Settings.DropdownHeight = 19 }, []where{{"", "settings.dropdown_height"}}, "from 20 to 90 percent"},
		{"dropdown_height too large", func(c *Config) { c.Settings.DropdownHeight = 91 }, []where{{"", "settings.dropdown_height"}}, "from 20 to 90 percent"},
		{"dropdown_height zero", func(c *Config) { c.Settings.DropdownHeight = 0 }, []where{{"", "settings.dropdown_height"}}, ""},
		{"menu_bar unknown", func(c *Config) { c.Settings.MenuBar = "text" }, []where{{"", "settings.menu_bar"}}, `name, icon, or hidden, not "text"`},
		{"menu_bar empty", func(c *Config) { c.Settings.MenuBar = "" }, []where{{"", "settings.menu_bar"}}, ""},
		{"menu_bar icon", func(c *Config) { c.Settings.MenuBar = MenuBarIcon }, nil, ""},
		{"menu_bar hidden", func(c *Config) { c.Settings.MenuBar = MenuBarHidden }, nil, ""},
		{"tab_shortcut_hints unknown", func(c *Config) { c.Settings.TabShortcutHints = "compact" }, []where{{"", "settings.tab_shortcut_hints"}}, `auto, always, or never, not "compact"`},
		{"tab_shortcut_hints empty", func(c *Config) { c.Settings.TabShortcutHints = "" }, []where{{"", "settings.tab_shortcut_hints"}}, ""},
		{"tab_shortcut_hints always", func(c *Config) { c.Settings.TabShortcutHints = TabHintsAlways }, nil, ""},
		{"tab_shortcut_hints never", func(c *Config) { c.Settings.TabShortcutHints = TabHintsNever }, nil, ""},
		{"scroll_bars unknown", func(c *Config) { c.Settings.ScrollBars = "legacy" }, []where{{"", "settings.scroll_bars"}}, `auto, always, or never, not "legacy"`},
		{"scroll_bars empty", func(c *Config) { c.Settings.ScrollBars = "" }, []where{{"", "settings.scroll_bars"}}, ""},
		{"scroll_bars always", func(c *Config) { c.Settings.ScrollBars = ScrollBarsAlways }, nil, ""},
		{"scroll_bars never", func(c *Config) { c.Settings.ScrollBars = ScrollBarsNever }, nil, ""},
		{"appearance unknown", func(c *Config) { c.Settings.Appearance = "auto" }, []where{{"", "settings.appearance"}}, `system, light, or dark, not "auto"`},
		{"appearance empty", func(c *Config) { c.Settings.Appearance = "" }, []where{{"", "settings.appearance"}}, ""},
		{"appearance system", func(c *Config) { c.Settings.Appearance = AppearanceSystem }, nil, ""},
		{"appearance light", func(c *Config) { c.Settings.Appearance = AppearanceLight }, nil, ""},
		{"density unknown", func(c *Config) { c.Settings.Density = "dense" }, []where{{"", "settings.density"}}, `auto, standard, or compact, not "dense"`},
		{"density empty", func(c *Config) { c.Settings.Density = "" }, []where{{"", "settings.density"}}, ""},
		{"density standard", func(c *Config) { c.Settings.Density = DensityStandard }, nil, ""},
		{"density compact", func(c *Config) { c.Settings.Density = DensityCompact }, nil, ""},
		{"pane_headers unknown", func(c *Config) { c.Settings.PaneHeaders = "never" }, []where{{"", "settings.pane_headers"}}, `always or auto, not "never"`},
		{"pane_headers empty", func(c *Config) { c.Settings.PaneHeaders = "" }, []where{{"", "settings.pane_headers"}}, ""},
		{"pane_headers auto", func(c *Config) { c.Settings.PaneHeaders = PaneHeadersAuto }, nil, ""},
		{"new_windows unknown", func(c *Config) { c.Settings.NewWindows = "tab" }, []where{{"", "settings.new_windows"}}, `not "tab"`},
		{"new_windows empty", func(c *Config) { c.Settings.NewWindows = "" }, []where{{"", "settings.new_windows"}}, ""},
		{"new_windows focused-pane", func(c *Config) { c.Settings.NewWindows = NewWindowsFocusedPane }, nil, ""},
		{"new_windows float", func(c *Config) { c.Settings.NewWindows = NewWindowsFloat }, nil, ""},
		{"covering_windows unknown", func(c *Config) { c.Settings.CoveringWindows = "park" }, []where{{"", "settings.covering_windows"}}, `hide or leave, not "park"`},
		{"covering_windows empty", func(c *Config) { c.Settings.CoveringWindows = "" }, []where{{"", "settings.covering_windows"}}, ""},
		{"covering_windows leave", func(c *Config) { c.Settings.CoveringWindows = CoveringLeave }, nil, ""},
		{"follow_focus unknown", func(c *Config) { c.Settings.FollowFocus = "follow" }, []where{{"", "settings.follow_focus"}}, "switch-tab, bring-here, or off"},
		{"follow_focus bring-here", func(c *Config) { c.Settings.FollowFocus = FollowBringHere }, nil, ""},
		{"follow_focus off", func(c *Config) { c.Settings.FollowFocus = FollowOff }, nil, ""},
		{"launch_timeout zero", func(c *Config) { c.Settings.LaunchTimeout.Duration = 0 }, []where{{"", "settings.launch_timeout"}}, "positive"},
		{"launch_timeout negative", func(c *Config) { c.Settings.LaunchTimeout.Duration = -time.Second }, []where{{"", "settings.launch_timeout"}}, ""},
		{"shortcuts aren't checked", func(c *Config) { c.Shortcuts.Jump = "not a shortcut" }, nil, ""},

		// Tabs.
		{"no tabs", func(c *Config) { c.Tabs, c.Rules = nil, nil }, []where{{"", "tab"}}, "at least one"},
		{"empty name", func(c *Config) { c.Tabs[1].Name = "" }, []where{{"tab 2", "name"}}, "needs a name"},
		{"blank name", func(c *Config) { c.Tabs[1].Name = "  " }, []where{{"tab 2", "name"}}, ""},
		{"duplicate name", func(c *Config) { c.Tabs[2].Name = "Code" }, []where{{"Code", "name"}}, "tab 2"},
		{"duplicate name ignoring case", func(c *Config) { c.Tabs[2].Name = "cODE" }, []where{{"cODE", "name"}}, "unique, ignoring case"},
		{"unknown layout", func(c *Config) { c.Tabs[0].Layout = "columns" }, []where{{"Comms", "layout"}}, `"columns" isn't a preset`},
		{"empty layout", func(c *Config) { c.Tabs[0].Layout = "" }, []where{{"Comms", "layout"}}, "needs a preset"},
		{"custom layout", func(c *Config) { c.Tabs[0].Layout = "custom" }, []where{{"Comms", "layout"}}, `"custom"`},
		{"unknown layout with bad sizes", func(c *Config) {
			c.Tabs[0].Layout = "columns"
			c.Tabs[0].Sizes = []float64{50, -1}
		}, []where{{"Comms", "layout"}, {"Comms", "sizes[2]"}}, ""},
		{"unknown small_screens", func(c *Config) { c.Tabs[0].SmallScreens = "wrap" }, []where{{"Comms", "small_screens"}}, "stack, tabs, or same"},
		{"empty small_screens", func(c *Config) { c.Tabs[0].SmallScreens = "" }, nil, ""},
		{"small_screens same", func(c *Config) { c.Tabs[0].SmallScreens = "same" }, nil, ""},
		{"no colour", func(c *Config) { c.Tabs[0].Colour = "" }, nil, ""},
		{"colour grey", func(c *Config) { c.Tabs[2].Colour = ColourGrey }, nil, ""},
		{"unknown colour", func(c *Config) { c.Tabs[0].Colour = "orange" }, []where{{"Comms", "colour"}},
			`"orange" isn't a tab colour; use red, yellow, green, cyan, blue, purple, pink, or grey, or leave colour out for none`},
		{"American grey", func(c *Config) { c.Tabs[0].Colour = "gray" }, []where{{"Comms", "colour"}}, `"gray" isn't a tab colour; did you mean "grey"?`},
		{"American grey in capitals", func(c *Config) { c.Tabs[0].Colour = "Gray" }, []where{{"Comms", "colour"}}, `"Gray" isn't a tab colour; did you mean "grey"?`},
		{"misspelt colour", func(c *Config) { c.Tabs[0].Colour = "purpel" }, []where{{"Comms", "colour"}}, `"purpel" isn't a tab colour; did you mean "purple"?`},
		{"colour in capitals", func(c *Config) { c.Tabs[1].Colour = "Blue" }, []where{{"Code", "colour"}}, `"Blue" needs to be in lower case: "blue"`},
		{"colour with spaces", func(c *Config) { c.Tabs[1].Colour = " blue" }, []where{{"Code", "colour"}}, "isn't a tab colour"},
		{"too many panes", func(c *Config) {
			c.Tabs[0].Panes = append(c.Tabs[0].Panes, Pane{Items: []Item{{App: "md.obsidian"}}})
		}, []where{{"Comms", "pane[3]"}}, `"side" layout has 2 panes, but this tab has 3`},
		{"fewer panes than the preset", func(c *Config) { c.Tabs[0].Panes = c.Tabs[0].Panes[:1] }, nil, ""},
		{"no panes", func(c *Config) { c.Tabs[0].Panes = nil }, nil, ""},
		{"single with two panes", func(c *Config) {
			c.Tabs[0].Layout = "single"
			c.Tabs[0].Sizes = nil
		}, []where{{"Comms", "pane[2]"}}, "1 pane,"},
		{"grid with four panes", func(c *Config) {
			c.Tabs[0].Layout = "grid"
			c.Tabs[0].Sizes = []float64{30, 70}
			c.Tabs[0].Panes = append(c.Tabs[0].Panes, Pane{}, Pane{Items: []Item{{Terminal: "~"}}})
		}, nil, ""},
		{"grid with five panes", func(c *Config) {
			c.Tabs[0].Layout = "grid"
			c.Tabs[0].Sizes = nil
			c.Tabs[0].Panes = append(c.Tabs[0].Panes, Pane{}, Pane{}, Pane{})
		}, []where{{"Comms", "pane[5]"}}, ""},
		{"sizes that don't add up to 100", func(c *Config) { c.Tabs[0].Sizes = []float64{1, 3} }, nil, ""},
		{"no sizes", func(c *Config) { c.Tabs[0].Sizes = nil }, nil, ""},
		{"too many sizes", func(c *Config) { c.Tabs[0].Sizes = []float64{30, 30, 40} }, []where{{"Comms", "sizes"}}, "needs 2 numbers, not 3"},
		{"too few sizes", func(c *Config) { c.Tabs[0].Sizes = []float64{100} }, []where{{"Comms", "sizes"}}, ""},
		{"zero size", func(c *Config) { c.Tabs[0].Sizes = []float64{100, 0} }, []where{{"Comms", "sizes[2]"}}, "positive"},
		{"negative and NaN sizes", func(c *Config) { c.Tabs[0].Sizes = []float64{-5, math.NaN()} }, []where{{"Comms", "sizes[1]"}, {"Comms", "sizes[2]"}}, ""},
		{"wrong count and a bad size", func(c *Config) { c.Tabs[0].Sizes = []float64{-5} }, []where{{"Comms", "sizes"}, {"Comms", "sizes[1]"}}, ""},
		{"sizes on single", func(c *Config) {
			c.Tabs[0].Layout = "single"
			c.Tabs[0].Panes = c.Tabs[0].Panes[:1]
		}, []where{{"Comms", "sizes"}}, "no split"},
		{"three with three sizes", func(c *Config) {
			c.Tabs[0].Layout = "three"
			c.Tabs[0].Sizes = []float64{1, 1, 1}
		}, nil, ""},
		{"grid with four sizes", func(c *Config) {
			c.Tabs[0].Layout = "grid"
			c.Tabs[0].Sizes = []float64{25, 25, 25, 25}
		}, []where{{"Comms", "sizes"}}, "splits into 2"},
		{"top-bottom", func(c *Config) { c.Tabs[1].Layout = "top-bottom" }, nil, ""},

		// Items.
		{"item with no kind", func(c *Config) { c.Tabs[1].Panes[0].Items[1] = Item{Name: "VS Code"} }, []where{{"Code", "pane[1].items[2]"}}, "needs one of app, web, vscode, or terminal"},
		{"item with two kinds", func(c *Config) { c.Tabs[1].Panes[0].Items[0].Web = "https://zed.dev" }, []where{{"Code", "pane[1].items[1]"}}, "has app and web, but needs exactly one"},
		{"item with three kinds", func(c *Config) {
			c.Tabs[1].Panes[1].Items[0].App = "x"
			c.Tabs[1].Panes[1].Items[0].VSCode = "~/code"
		}, []where{{"Code", "pane[2].items[1]"}}, "has app, vscode, and terminal"},
		{"blank app", func(c *Config) { c.Tabs[1].Panes[0].Items[0].App = " " }, []where{{"Code", "pane[1].items[1].app"}}, "bundle ID"},
		{"title on a web item", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Title = "Slack" }, []where{{"Comms", "pane[1].items[1].title"}}, "only applies to app items"},
		{"url on a terminal item", func(c *Config) { c.Tabs[1].Panes[1].Items[0].URL = "https://x.y" }, []where{{"Code", "pane[2].items[1].url"}}, "only applies to app items"},
		{"title on an app item", func(c *Config) { c.Tabs[2].Panes[0].Items[0].Title = "Docs" }, nil, ""},
		{"url and title on an app item", func(c *Config) {
			c.Tabs[2].Panes[0].Items[0].Title = "Docs"
			c.Tabs[2].Panes[0].Items[0].URL = "https://docs.example.com"
		}, nil, ""},
		{"url without title", func(c *Config) { c.Tabs[2].Panes[0].Items[0].URL = "https://docs.example.com" }, []where{{"Research", "pane[1].items[1].url"}}, "needs title too"},
		{"url with an app's own scheme", func(c *Config) {
			c.Tabs[2].Panes[0].Items[0].Title = "Roadmap"
			c.Tabs[2].Panes[0].Items[0].URL = "notion://www.notion.so/roadmap"
		}, nil, ""},
		{"url without a scheme", func(c *Config) {
			c.Tabs[2].Panes[0].Items[0].Title = "Roadmap"
			c.Tabs[2].Panes[0].Items[0].URL = "notion.so/roadmap"
		}, []where{{"Research", "pane[1].items[1].url"}}, "has no scheme"},
		{"url that doesn't parse", func(c *Config) {
			c.Tabs[2].Panes[0].Items[0].Title = "Docs"
			c.Tabs[2].Panes[0].Items[0].URL = "https://exa mple.com/"
		}, []where{{"Research", "pane[1].items[1].url"}}, "isn't a link"},
		{"profile on an app item", func(c *Config) { c.Tabs[2].Panes[1].Items[0].Profile = "notes" }, []where{{"Research", "pane[2].items[1].profile"}}, "only applies to web items"},
		{"profile on a vscode item", func(c *Config) {
			c.Tabs[1].Panes[0].Items[1] = Item{VSCode: "~/code/bench", Profile: "code"}
		}, []where{{"Code", "pane[1].items[2].profile"}}, "only applies to web items"},
		{"profile on a terminal item", func(c *Config) { c.Tabs[1].Panes[1].Items[0].Profile = "x" }, []where{{"Code", "pane[2].items[1].profile"}}, ""},
		{"command on a terminal item", func(c *Config) { c.Tabs[1].Panes[1].Items[0].Command = "htop" }, nil, ""},
		{"command on an app item", func(c *Config) { c.Tabs[1].Panes[0].Items[0].Command = "zed" }, []where{{"Code", "pane[1].items[1].command"}}, "only applies to terminal items"},
		{"command on a web item", func(c *Config) { c.Tabs[0].Panes[1].Items[0].Command = "x" }, []where{{"Comms", "pane[2].items[1].command"}}, ""},
		{"name on an app item", func(c *Config) { c.Tabs[1].Panes[0].Items[0].Name = "Zed" }, []where{{"Code", "pane[1].items[1].name"}}, "only applies to web, vscode, and terminal items"},
		{"name on vscode and terminal items", func(c *Config) {
			c.Tabs[1].Panes[0].Items[1] = Item{VSCode: "~/code/bench", Name: "Bench"}
			c.Tabs[1].Panes[1].Items[0].Name = "Shell"
		}, nil, ""},
		{"run on a terminal item", func(c *Config) {
			c.Tabs[1].Panes[1].Items[0].Run = "claude --continue"
			c.Tabs[1].Panes[1].Items[0].StartAgain = StartAgainOf(false)
		}, nil, ""},
		{"run on an app item", func(c *Config) { c.Tabs[1].Panes[0].Items[0].Run = "zed" }, []where{{"Code", "pane[1].items[1].run"}}, "only applies to terminal items"},
		{"run with command", func(c *Config) {
			c.Tabs[1].Panes[1].Items[0].Command = "htop"
			c.Tabs[1].Panes[1].Items[0].Run = "htop"
		}, []where{{"Code", "pane[2].items[1].run"}}, "can't go with command"},
		{"run on two lines", func(c *Config) { c.Tabs[1].Panes[1].Items[0].Run = "cd x\nmake" }, []where{{"Code", "pane[2].items[1].run"}}, "one line of text"},
		{"run with a tab", func(c *Config) { c.Tabs[1].Panes[1].Items[0].Run = "make\tdev" }, []where{{"Code", "pane[2].items[1].run"}}, "without tabs"},
		{"start_again without run", func(c *Config) { c.Tabs[1].Panes[1].Items[0].StartAgain = StartAgainOf(false) }, []where{{"Code", "pane[2].items[1].start_again"}}, "only applies to terminal items with run"},
		{"several wrong fields", func(c *Config) {
			c.Tabs[0].Panes[0].Items[0].Title = "x"
			c.Tabs[0].Panes[0].Items[0].Command = "y"
		}, []where{{"Comms", "pane[1].items[1].title"}, {"Comms", "pane[1].items[1].command"}}, ""},
		{"web over http", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://app.slack.com" }, []where{{"Comms", "pane[1].items[1].web"}}, `"http://app.slack.com" isn't an https address; http is only for this Mac`},
		{"web over ftp", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "ftp://files.example.com" }, []where{{"Comms", "pane[1].items[1].web"}}, `"ftp://files.example.com" isn't an https address`},
		{"web over http on localhost", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://localhost:3000" }, nil, ""},
		{"web over http on 127.0.0.1", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://127.0.0.1:8000/docs" }, nil, ""},
		{"web over http on ::1", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://[::1]:8080" }, nil, ""},
		{"web over http on a name under localhost", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://app.localhost:5173" }, []where{{"Comms", "pane[1].items[1].web"}}, "http is only for this Mac"},
		{"web over http on a name like localhost", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "http://localhost.example.com" }, []where{{"Comms", "pane[1].items[1].web"}}, "http is only for this Mac"},
		{"web without a scheme", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "app.slack.com" }, []where{{"Comms", "pane[1].items[1].web"}}, ""},
		{"web without a host", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "https:///client" }, []where{{"Comms", "pane[1].items[1].web"}}, "no host"},
		{"web that doesn't parse", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "https://exa mple.com/%zz" }, []where{{"Comms", "pane[1].items[1].web"}}, ""},
		{"web in capitals", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = "HTTPS://app.slack.com" }, nil, ""},
		{"blank web", func(c *Config) { c.Tabs[0].Panes[0].Items[0].Web = " " }, []where{{"Comms", "pane[1].items[1].web"}}, "needs an https address"},
		{"blank vscode folder", func(c *Config) { c.Tabs[1].Panes[0].Items[1] = Item{VSCode: " "} }, []where{{"Code", "pane[1].items[2].vscode"}}, "needs a folder"},
		{"blank terminal folder", func(c *Config) { c.Tabs[1].Panes[1].Items[0].Terminal = "\t" }, []where{{"Code", "pane[2].items[1].terminal"}}, "starting folder"},
		{"problem in a tab with no name", func(c *Config) {
			c.Tabs[2].Name = ""
			c.Rules = nil
			c.Tabs[2].Panes[0].Items[0].Name = "Brave"
		}, []where{{"tab 3", "name"}, {"tab 3", "pane[1].items[1].name"}}, ""},
		{"items in extra panes are checked", func(c *Config) {
			c.Tabs[0].Panes = append(c.Tabs[0].Panes, Pane{Items: []Item{{}}})
		}, []where{{"Comms", "pane[3]"}, {"Comms", "pane[3].items[1]"}}, ""},

		// Rules.
		{"rule without app", func(c *Config) { c.Rules[0].App = "" }, []where{{"", "rule[1].app"}}, "bundle ID"},
		{"rule with float and tab", func(c *Config) { c.Rules[0].Tab = "Research" }, []where{{"", "rule[1]"}}, "both float = true and tab"},
		{"rule with neither float nor tab", func(c *Config) { c.Rules[0].Float = false }, []where{{"", "rule[1]"}}, "needs float = true, or a tab"},
		{"rule naming a missing tab", func(c *Config) {
			c.Rules = append(c.Rules, Rule{App: "com.brave.Browser", Tab: "Reading"})
		}, []where{{"", "rule[2].tab"}}, `no tab called "Reading"`},
		{"rule naming a tab", func(c *Config) { c.Rules = append(c.Rules, Rule{App: "com.brave.Browser", Tab: "Research"}) }, nil, ""},
		{"rule naming a tab in other case", func(c *Config) { c.Rules = append(c.Rules, Rule{App: "com.brave.Browser", Tab: "research"}) }, nil, ""},
		{"rule with only an app", func(c *Config) { c.Rules = []Rule{{App: "x"}, {}} }, []where{{"", "rule[1]"}, {"", "rule[2].app"}, {"", "rule[2]"}}, ""},

		// Launchers.
		{"terminal launcher", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Claude", Folder: "~/code/bench", Run: "claude --continue"}}
		}, nil, ""},
		{"terminal launcher that doesn't start again", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Deploy", Run: "./deploy.sh", StartAgain: StartAgainOf(false)}}
		}, nil, ""},
		{"shell launcher", func(c *Config) { c.Launchers = []Launcher{{Name: "Shell"}} }, nil, ""},
		{"app launcher on a folder", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Bench in Zed", App: "dev.zed.Zed", Folder: "~/code/bench"}}
		}, nil, ""},
		{"app launcher on a link", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Notes", App: "md.obsidian", URL: "obsidian://open?vault=Notes", Title: "Notes"}}
		}, nil, ""},
		{"launcher without a name", func(c *Config) { c.Launchers = []Launcher{{Run: "claude"}} }, []where{{"", "launcher[1].name"}}, "needs a name"},
		{"launcher name on two lines", func(c *Config) { c.Launchers = []Launcher{{Name: "a\nb"}} }, []where{{"", "launcher[1].name"}}, "one line"},
		{"duplicate launcher name ignoring case", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Claude", Run: "claude"}, {Name: "claude", Run: "claude -c"}}
		}, []where{{"", "launcher[2].name"}}, "launcher 1 is already called \"Claude\""},
		{"launcher run on two lines", func(c *Config) { c.Launchers = []Launcher{{Name: "x", Run: "a\rb"}} }, []where{{"", "launcher[1].run"}}, "one line of text"},
		{"title and url on a terminal launcher", func(c *Config) {
			c.Launchers = []Launcher{{Name: "x", Run: "x", Title: "x", URL: "https://x.y"}}
		}, []where{{"", "launcher[1].title"}, {"", "launcher[1].url"}}, "only applies to app launchers"},
		{"start_again on a terminal launcher without run", func(c *Config) {
			c.Launchers = []Launcher{{Name: "Shell", StartAgain: StartAgainOf(false)}}
		}, []where{{"", "launcher[1].start_again"}}, "only applies to terminal launchers with run"},
		{"run and start_again on an app launcher", func(c *Config) {
			c.Launchers = []Launcher{{Name: "x", App: "dev.zed.Zed", Folder: "~", Run: "zed", StartAgain: StartAgainOf(false)}}
		}, []where{{"", "launcher[1].run"}, {"", "launcher[1].start_again"}}, "only applies to terminal launchers"},
		{"blank app on a launcher", func(c *Config) { c.Launchers = []Launcher{{Name: "x", App: " ", Folder: "~"}} }, []where{{"", "launcher[1].app"}}, "bundle ID"},
		{"app launcher with folder and url", func(c *Config) {
			c.Launchers = []Launcher{{Name: "x", App: "dev.zed.Zed", Folder: "~", URL: "file:///tmp", Title: "tmp"}}
		}, []where{{"", "launcher[1]"}}, "both folder and url"},
		{"app launcher with nothing to open", func(c *Config) { c.Launchers = []Launcher{{Name: "x", App: "dev.zed.Zed"}} }, []where{{"", "launcher[1]"}}, "needs a folder or a url"},
		{"app launcher's url without title", func(c *Config) {
			c.Launchers = []Launcher{{Name: "x", App: "md.obsidian", URL: "obsidian://open"}}
		}, []where{{"", "launcher[1].url"}}, "needs title too"},
		{"app launcher's url without a scheme", func(c *Config) {
			c.Launchers = []Launcher{{Name: "x", App: "md.obsidian", URL: "obsidian.md", Title: "x"}}
		}, []where{{"", "launcher[1].url"}}, "has no scheme"},

		// Several at once, in order: settings, launchers, tabs, rules.
		{"everything", func(c *Config) {
			c.Settings.Gap = -1
			c.Settings.FollowFocus = ""
			c.Tabs[0].Layout = "nope"
			c.Tabs[1].Name = "comms"
			c.Tabs[1].Panes[0].Items[0].Web = "ftp://x"
			c.Rules[0].Float = false
			c.Launchers = []Launcher{{Run: "x"}}
		}, []where{
			{"", "settings.gap"},
			{"", "settings.follow_focus"},
			{"", "launcher[1].name"},
			{"Comms", "layout"},
			{"comms", "name"},
			{"comms", "pane[1].items[1]"},
			{"", "rule[1]"},
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Example()
			tc.change(c)
			problems := c.Validate()
			checkWheres(t, problems, tc.want)
			if tc.msg != "" && len(problems) > 0 && !strings.Contains(problems[0].Message, tc.msg) {
				t.Errorf("message %q doesn't contain %q", problems[0].Message, tc.msg)
			}
		})
	}
}

func TestProblemError(t *testing.T) {
	tests := []struct {
		p    Problem
		want string
	}{
		{Problem{Tab: "Code", Field: "pane[1].items[2].web", Message: "needs an https address"}, `tab "Code": pane[1].items[2].web: needs an https address`},
		{Problem{Field: "settings.gap", Message: "must be 0 or more points"}, "settings.gap: must be 0 or more points"},
		{Problem{Message: "line 3, column 7: expected '=' after key"}, "line 3, column 7: expected '=' after key"},
		{Problem{Tab: "tab 2", Message: "line 9, column 1: oops"}, `tab "tab 2": line 9, column 1: oops`},
	}
	for _, tc := range tests {
		if got := tc.p.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}

func TestProblemsError(t *testing.T) {
	one := Problems{{Field: "settings.gap", Message: "must be 0 or more points"}}
	if got, want := one.Error(), "settings.gap: must be 0 or more points"; got != want {
		t.Errorf("one problem: %q, want %q", got, want)
	}
	two := append(one, Problem{Tab: "Code", Field: "layout", Message: "needs a preset"})
	if got, want := two.Error(), `2 problems: settings.gap: must be 0 or more points; tab "Code": layout: needs a preset`; got != want {
		t.Errorf("two problems: %q, want %q", got, want)
	}
}

func TestTabIndex(t *testing.T) {
	c := Example()
	c.Tabs = append(c.Tabs, Tab{Name: "research"}) // Invalid, but TabIndex doesn't care.
	tests := []struct {
		name string
		want int
	}{
		{"Comms", 0},
		{"Code", 1},
		{"Research", 2},
		{"research", 3}, // An exact match wins.
		{"CODE", 1},
		{"Nope", -1},
		{"", -1},
	}
	for _, tc := range tests {
		if got := c.TabIndex(tc.name); got != tc.want {
			t.Errorf("TabIndex(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := (*Config)(nil).TabIndex("Code"); got != -1 {
		t.Errorf("TabIndex on a nil config = %d, want -1", got)
	}
}

func TestValidateNil(t *testing.T) {
	if problems := (*Config)(nil).Validate(); len(problems) != 1 || problems[0].Message == "" {
		t.Errorf("Validate on a nil config = %v, want one problem", problems)
	}
}

func TestClone(t *testing.T) {
	a := Example()
	a.Tabs[0].Panes[1].Items = []Item{}
	a.Launchers = []Launcher{{Name: "Deploy", Run: "./deploy.sh", StartAgain: StartAgainOf(false)}}
	a.Tabs[1].Panes[1].Items[0] = Item{Terminal: "~", Run: "make", StartAgain: StartAgainOf(false)}
	b := a.Clone()
	b.Tabs[0].Name = "x"
	b.Tabs[0].Sizes[0] = 1
	b.Tabs[1].Panes[0].Items[0].App = "x"
	b.Tabs[1].Panes = append(b.Tabs[1].Panes, Pane{})
	b.Rules[0].App = "x"
	b.Launchers[0].Name = "x"
	*b.Launchers[0].StartAgain = true
	*b.Tabs[1].Panes[1].Items[0].StartAgain = true
	b.Settings.Gap = 99
	b.Shortcuts.Jump = "x"
	if want := Example(); a.Tabs[0].Name != want.Tabs[0].Name || a.Tabs[0].Sizes[0] != 45 ||
		a.Tabs[1].Panes[0].Items[0].App != "dev.zed.Zed" || len(a.Tabs[1].Panes) != 2 ||
		a.Rules[0].App != want.Rules[0].App || a.Settings.Gap != want.Settings.Gap || a.Shortcuts.Jump != want.Shortcuts.Jump ||
		a.Launchers[0].Name != "Deploy" || a.Launchers[0].StartsAgain() || a.Tabs[1].Panes[1].Items[0].StartsAgain() {
		t.Error("changing a clone changed the original")
	}
	if c := a.Clone(); c.Tabs[0].Panes[1].Items == nil || c.Tabs[0].Sizes == nil || (&Config{}).Clone().Tabs != nil {
		t.Error("Clone doesn't keep empty and nil lists as they are")
	}
	if (*Config)(nil).Clone() != nil {
		t.Error("cloning nil isn't nil")
	}
}

// TestLocalHost checks which hosts are this Mac, where a web item may use
// http, as browsers count them secure. A name under localhost isn't, as
// macOS doesn't promise to keep it on the Mac.
func TestLocalHost(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost": true, "LOCALHOST": true, "localhost.": true,
		"127.0.0.1": true, "127.1.2.3": true, "::1": true,
		"app.localhost": false, "localhost.example.com": false, "example.com": false, "10.0.0.1": false, "192.168.1.2": false, "": false,
	} {
		if got := LocalHost(host); got != want {
			t.Errorf("LocalHost(%q) = %v, want %v", host, got, want)
		}
	}
}
