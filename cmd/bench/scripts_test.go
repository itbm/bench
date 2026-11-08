//go:build unix

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/control"
)

// scriptList is what a running Bench lists for the script subcommands'
// tests.
func scriptList() control.List {
	return control.List{
		Version: control.ListVersion, Current: 2, Attention: 1, Unread: control.Unread{Count: 3},
		Tabs: []control.Tab{
			{Number: 1, Name: "Comms", Colour: "blue", Unread: control.Unread{Count: 3}, Panes: []control.Pane{
				{ID: "p1", Focused: true, Items: []control.Item{
					{ID: "w1", Kind: control.KindWeb, App: "Slack", Title: "general", Shown: true, Unread: control.Unread{Count: 3}, KeepRunning: true},
					{ID: "w2", Kind: control.KindWeb, App: "Mail", Title: "Mail", NotLoaded: true},
				}},
			}},
			{Number: 2, Name: "Code", Current: true, Attention: 1, Panes: []control.Pane{
				{ID: "p2", Focused: true, Items: []control.Item{
					{ID: "w3", Kind: control.KindApp, App: "Zed", Title: "main.go", Shown: true},
					{ID: "w4", Kind: control.KindApp, App: "VS Code", Title: "index.html"},
				}},
				{ID: "p3", Items: []control.Item{
					{ID: "w5", Kind: control.KindTerminal, App: "Terminal", Title: "zsh", Shown: true, Attention: "Rang the bell"},
				}},
			}},
			{Number: 3, Name: "-x", Panes: []control.Pane{{ID: "p4", Items: []control.Item{}}}},
		},
		Dropdown: &control.Item{ID: "w6", Kind: control.KindTerminal, App: "Terminal", Title: "htop"},
	}
}

// fakeBench replaces how the script subcommands reach Bench: a Bench that
// answers with list, or with err. It returns the arguments forwarded to
// it. The support folder has the example config, with the launchers of
// launchersTOML.
func fakeBench(t *testing.T, list control.List, err error) *[][]string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BENCH_HOME", dir)
	writeScriptsConfig(t, false)
	var sent [][]string
	saved := ctl
	ctl = scriptControl{
		query: func(string) (control.List, error) { return list, err },
		here:  func() error { return nil },
		forward: func(to string, args []string) error {
			if to != dir {
				t.Errorf("forwarded to the Bench of %s, want %s", to, dir)
			}
			sent = append(sent, slices.Clone(args))
			return nil
		},
	}
	t.Cleanup(func() { ctl = saved })
	return &sent
}

// launchersTOML is a launcher for the script subcommands' tests.
const launchersTOML = `
[[launcher]]
name = "Claude"
run = "claude"
`

// writeScriptsConfig writes the example config, with allow_scripts as on says,
// and launchersTOML, to the support folder.
func writeScriptsConfig(t *testing.T, on bool) {
	t.Helper()
	data := strings.Replace(string(config.ExampleTOML()), "allow_scripts = false", "allow_scripts = "+strconv.FormatBool(on), 1)
	if err := os.WriteFile(filepath.Join(os.Getenv("BENCH_HOME"), config.ConfigFile), []byte(data+launchersTOML), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScriptCommandsForwardTheirArguments(t *testing.T) {
	sent := fakeBench(t, scriptList(), nil)
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"tab", "Code"}, []string{"tab", "--", "Code"}},
		{[]string{"tab", "code"}, []string{"tab", "--", "code"}},
		{[]string{"tab", "2"}, []string{"tab", "--", "2"}},
		{[]string{"tab", "--", "-x"}, []string{"tab", "--", "-x"}},
		{[]string{"next"}, []string{"next"}},
		{[]string{"previous"}, []string{"previous"}},
		{[]string{"last"}, []string{"last"}},
		{[]string{"move-to-tab", "Comms"}, []string{"move-to-tab", "--", "Comms"}},
		{[]string{"launch", "Claude"}, []string{"launch", "--", "Claude"}},
		{[]string{"launch", "claude"}, []string{"launch", "--", "claude"}},
	}
	for _, c := range cases {
		*sent = nil
		status, stdout, stderr := runBench(t, c.args...)
		if status != exitOK || stdout != "" || stderr != "" {
			t.Errorf("bench %q = %d %q %q", c.args, status, stdout, stderr)
		}
		if len(*sent) != 1 || !slices.Equal((*sent)[0], c.want) {
			t.Errorf("bench %q forwarded %q, want %q", c.args, *sent, c.want)
		}
	}
}

func TestScriptCommandsCheckTheirArguments(t *testing.T) {
	sent := fakeBench(t, scriptList(), nil)
	cases := []struct {
		args   []string
		status int
		says   string
	}{
		{[]string{"tab"}, exitUsage, "bench tab: needs a tab's name or number.\n\nUsage: bench tab <name|number>"},
		{[]string{"tab", ""}, exitUsage, "bench tab: needs a tab's name or number."},
		{[]string{"tab", "Mail", "and", "chat"}, exitUsage, "bench tab: takes one argument, a tab's name or number: put a name with spaces in quotes."},
		{[]string{"tab", "-x"}, exitUsage, "bench tab: flag provided but not defined: -x."},
		{[]string{"next", "now"}, exitUsage, "bench next: takes nothing after it."},
		{[]string{"launch"}, exitUsage, "bench launch: needs a launcher's name."},
		{[]string{"tab", "Nope"}, exitError, "bench tab: there's no tab called Nope.\n"},
		{[]string{"tab", "9"}, exitError, "bench tab: there's no tab 9.\n"},
		{[]string{"move-to-tab", "Research"}, exitError, "bench move-to-tab: there's no tab called Research.\n"},
		// What Bench would refuse, the command refuses, with status 1, as
		// a script can't see Bench's message.
		{[]string{"move-to-tab", "code"}, exitError, "bench move-to-tab: Bench is on Code already: name another tab.\n"},
		{[]string{"move-to-tab", "2"}, exitError, "bench move-to-tab: Bench is on Code already: name another tab.\n"},
		{[]string{"launch", "Nope"}, exitError, "bench launch: there's no launcher called Nope: Settings ▸ Launchers lists them.\n"},
		{[]string{"launch", "--", "-dev"}, exitError, "bench launch: there's no launcher called -dev"},
	}
	for _, c := range cases {
		status, stdout, stderr := runBench(t, c.args...)
		if status != c.status || stdout != "" || !strings.HasPrefix(stderr, c.says) {
			t.Errorf("bench %q = %d %q %q; want %d and %q", c.args, status, stdout, stderr, c.status, c.says)
		}
	}
	if len(*sent) != 0 {
		t.Errorf("forwarded %q", *sent)
	}
}

// TestMoveToTabWhileTheDropdownShows checks that bench move-to-tab is
// refused, with status 1, while the drop-down shows, as it has the
// keyboard then, as when the command is typed in it: Bench would refuse
// it too. The other commands go on.
func TestMoveToTabWhileTheDropdownShows(t *testing.T) {
	l := scriptList()
	l.Dropdown.Shown = true
	sent := fakeBench(t, l, nil)
	status, stdout, stderr := runBench(t, "move-to-tab", "Comms")
	want := "bench move-to-tab: the drop-down has the keyboard, so there's no shown window to move: hide the drop-down, then try again.\n"
	if status != exitError || stdout != "" || stderr != want {
		t.Errorf("bench move-to-tab Comms = %d %q %q; want 1 and %q", status, stdout, stderr, want)
	}
	if len(*sent) != 0 {
		t.Errorf("forwarded %q", *sent)
	}
	if status, _, stderr := runBench(t, "tab", "Comms"); status != exitOK || len(*sent) != 1 {
		t.Errorf("bench tab Comms with the drop-down shown = %d %q, forwarded %q", status, stderr, *sent)
	}
}

func TestScriptCommandsNeedAllowScripts(t *testing.T) {
	// Nothing answers: Bench isn't running, or runs with the setting off,
	// which config.toml says.
	sent := fakeBench(t, control.List{}, control.ErrNotServing)
	for _, args := range [][]string{{"tab", "Code"}, {"next"}, {"launch", "Claude"}, {"list"}, {"list", "--json"}} {
		status, stdout, stderr := runBench(t, args...)
		want := "bench " + args[0] + ": Scripts can't control Bench: open Bench, turn on Allow scripts to control Bench in Settings ▸ Advanced, then try again.\n"
		if status != exitError || stdout != "" || stderr != want {
			t.Errorf("bench %q = %d %q %q; want 1 and %q", args, status, stdout, stderr, want)
		}
	}
	// With the setting on, Bench isn't running, or can't make its socket,
	// which its notice says: the command doesn't ask for the setting.
	writeScriptsConfig(t, true)
	if status, _, stderr := runBench(t, "next"); status != exitError || stderr != "bench next: Bench isn't running, or can't answer scripts: open Bench, then try again. If it's open, the message in its status bar says why.\n" {
		t.Errorf("setting on: %d %q", status, stderr)
	}
	// config.toml with a problem: the last good config says.
	dir := os.Getenv("BENCH_HOME")
	if err := os.WriteFile(filepath.Join(dir, config.ConfigFile), []byte("allow_scripts = maybe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if status, _, stderr := runBench(t, "next"); status != exitError || !strings.Contains(stderr, "turn on Allow scripts") {
		t.Errorf("config.toml with a problem, no good one: %d %q", status, stderr)
	}
	// What the socket says goes as it is.
	ctl.query = func(string) (control.List, error) {
		return control.List{}, errors.New("Scripts can't control Bench: something else")
	}
	if status, _, stderr := runBench(t, "last"); status != exitError || stderr != "bench last: Scripts can't control Bench: something else.\n" {
		t.Errorf("an error from the socket: %d %q", status, stderr)
	}
	if len(*sent) != 0 {
		t.Errorf("forwarded %q", *sent)
	}
}

func TestListPrintsTheList(t *testing.T) {
	fakeBench(t, scriptList(), nil)
	status, stdout, stderr := runBench(t, "list", "--json")
	if status != exitOK || stderr != "" {
		t.Fatalf("bench list --json = %d %q", status, stderr)
	}
	var got control.List
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("bench list --json printed %q: %v", stdout, err)
	}
	if !reflect.DeepEqual(got, scriptList()) {
		t.Errorf("bench list --json = %+v", got)
	}
	if !strings.Contains(stdout, "\n  \"tabs\": [\n") {
		t.Errorf("bench list --json isn't indented:\n%s", stdout)
	}

	status, stdout, stderr = runBench(t, "list")
	want := "  1 Comms (3 unread)\n" +
		"    Pane 1\n" +
		"    * Slack: general (3 unread)\n" +
		"      Mail (not loaded yet)\n" +
		"* 2 Code (1 needs you)\n" +
		"    Pane 1\n" +
		"    * Zed: main.go\n" +
		"      VS Code: index.html\n" +
		"    Pane 2\n" +
		"    * Terminal: zsh (Rang the bell)\n" +
		"  3 -x\n" +
		"    Pane 1\n" +
		"  Drop-down\n" +
		"      Terminal: htop\n"
	if status != exitOK || stderr != "" || stdout != want {
		t.Errorf("bench list = %d %q\n%s\nwant\n%s", status, stderr, stdout, want)
	}
	// Each field is there, even when it's empty, and a list without a
	// drop-down has it as null.
	for _, field := range []string{`"colour": "blue"`, `"colour": ""`, `"keepRunning": true`, `"notLoaded": true`, `"dropdown": {`} {
		if _, out, _ := runBench(t, "list", "--json"); !strings.Contains(out, field) {
			t.Errorf("bench list --json has no %s:\n%s", field, out)
		}
	}
	none := scriptList()
	none.Dropdown = nil
	ctl.query = func(string) (control.List, error) { return none, nil }
	if _, out, _ := runBench(t, "list", "--json"); !strings.Contains(out, "\n  \"dropdown\": null\n") {
		t.Errorf("bench list --json without a drop-down:\n%s", out)
	}
	if _, stdout, _ := runBench(t, "list"); strings.Contains(stdout, "Drop-down") {
		t.Errorf("bench list without a drop-down:\n%s", stdout)
	}
	if status, _, stderr := runBench(t, "list", "extra"); status != exitUsage || !strings.HasPrefix(stderr, "bench list: unexpected argument \"extra\".") {
		t.Errorf("bench list extra = %d %q", status, stderr)
	}
}

// TestListKeepsWhatNeedsYou checks that a long title is cut on its own, so
// that what needs the user, at the end of its line, still shows.
func TestListKeepsWhatNeedsYou(t *testing.T) {
	long := "npm run build — ~/code/some-project/packages/frontend — 120×40 — and more besides, to be cut"
	l := control.List{Version: 1, Current: 1, Tabs: []control.Tab{{Number: 1, Name: "Code", Current: true, Attention: 1, Panes: []control.Pane{
		{ID: "p1", Focused: true, Items: []control.Item{
			{ID: "w1", Kind: control.KindTerminal, App: "Terminal", Title: long, Shown: true, Attention: "Rang the bell"},
			{ID: "w2", Kind: control.KindWeb, App: "Mail", Title: strings.Repeat("Inbox ", 30), Unread: control.Unread{Count: 4}},
		}},
	}}}}
	fakeBench(t, l, nil)
	status, stdout, stderr := runBench(t, "list")
	if status != exitOK || stderr != "" {
		t.Fatalf("bench list = %d %q", status, stderr)
	}
	lines := strings.Split(stdout, "\n")
	if len(lines) < 4 || !strings.HasSuffix(lines[2], "… (Rang the bell)") || !strings.HasSuffix(lines[3], "… (4 unread)") {
		t.Errorf("bench list:\n%s", stdout)
	}
}

func TestListAsksTheSocket(t *testing.T) {
	// A short folder: t.TempDir's can be too long for a socket on macOS.
	dir, err := os.MkdirTemp("", "bl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	status, _, stderr := runBench(t, "list", "--dir", dir)
	want := "bench list: nothing answers at " + filepath.Join(dir, control.SocketFile) +
		": Bench isn't running with that support folder, or Allow scripts to control Bench is off in it.\n"
	if status != exitError || stderr != want {
		t.Errorf("with nothing there: %d %q; want %q", status, stderr, want)
	}
	s, err := control.Serve(dir, func() (control.List, error) { return scriptList(), nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	status, stdout, stderr := runBench(t, "list", "--json", "--dir", dir)
	var got control.List
	if status != exitOK || json.Unmarshal([]byte(stdout), &got) != nil || !reflect.DeepEqual(got, scriptList()) {
		t.Errorf("bench list --json --dir = %d %q %q", status, stdout, stderr)
	}
}

func TestHelpListsTheScriptCommands(t *testing.T) {
	_, stdout, _ := runBench(t, "help")
	for _, line := range []string{
		"bench tab <name|number>", "bench next", "bench previous", "bench last",
		"bench move-to-tab <name|number>", "bench launch <launcher>", "bench list [--json] [--dir path]",
	} {
		if !strings.Contains(stdout, "  "+line) {
			t.Errorf("bench help has no %q:\n%s", line, stdout)
		}
	}
	_, stdout, _ = runBench(t, "help", "tab")
	if !strings.Contains(stdout, "Allow scripts to control Bench on") {
		t.Errorf("bench help tab doesn't say it needs the setting:\n%s", stdout)
	}
	// Only the subcommands that take a name say how to write one.
	for _, name := range []string{"tab", "move-to-tab", "launch", "next", "previous", "last", "list"} {
		_, stdout, _ := runBench(t, "help", name)
		takes := name == "tab" || name == "move-to-tab" || name == "launch"
		if got := strings.Contains(stdout, "A name with spaces goes in quotes."); got != takes {
			t.Errorf("bench help %s says how to quote a name: %v", name, got)
		}
		if got := strings.Contains(stdout, "A tab's name comes before its number."); got != (name == "tab" || name == "move-to-tab") {
			t.Errorf("bench help %s says a tab's name comes first: %v", name, got)
		}
	}
	// bench last is the last used tab, not the last in the strip.
	if _, stdout, _ := runBench(t, "help"); !strings.Contains(stdout, "Switch the running Bench back to the last used tab.") {
		t.Errorf("bench help says what bench last does as:\n%s", stdout)
	}
}
