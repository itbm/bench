package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/control"
)

// The script subcommands control the Bench that's running, from Terminal
// or a script, while its Allow scripts to control Bench setting is on
// (docs/architecture.md, decision 77). Each asks the running Bench for its
// list over the socket in its support folder (control.Query), which is
// there only while the setting is on, to check that it is, and what it can
// of what Bench would refuse: a tab that isn't there, and a launcher that
// isn't in config.toml. bench list prints that list; the others forward
// their arguments to the running Bench, as Wails' single-instance lock
// forwards a second Bench's (ui.Forward), and Bench carries them out,
// checking the setting again. Forwarding doesn't wait for Bench, so the
// command can't say what Bench then refuses: Bench shows that itself.

// scriptAbout ends each script subcommand's help.
const scriptAbout = "\n\nIt works only while Bench is running with Allow scripts to control Bench on, in\n" +
	"Settings ▸ Advanced."

// tabAbout says how to name a tab, for the subcommands that take one.
const tabAbout = "\nA name with spaces goes in quotes. A tab's name comes before its number."

var tabCommand = scriptCommand(control.ActionTab, "bench tab <name|number>",
	"Switch the running Bench to a tab.",
	"Switches the running Bench to the tab with that name, matched as Bench matches tab names,\n"+
		"or that number, counting from 1, and brings Bench forward."+tabAbout)

var nextCommand = scriptCommand(control.ActionNext, "bench next",
	"Switch the running Bench to the next tab.",
	"Switches the running Bench to the next tab, after the last one the first, and brings it forward.")

var previousCommand = scriptCommand(control.ActionPrevious, "bench previous",
	"Switch the running Bench to the previous tab.",
	"Switches the running Bench to the previous tab, before the first one the last, and brings it\nforward.")

var lastCommand = scriptCommand(control.ActionLast, "bench last",
	"Switch the running Bench back to the last used tab.",
	"Switches the running Bench back to the tab it was on before this one, as Tab ▸ Last Used Tab\n"+
		"does, and brings it forward.")

var moveToTabCommand = scriptCommand(control.ActionMoveToTab, "bench move-to-tab <name|number>",
	"Move the shown window to another tab.",
	"Moves the window, page, or terminal that the running Bench's focused pane shows to that tab's\n"+
		"focused pane, as the send_to_tab_modifiers shortcut does. Bench stays on its tab. While the\n"+
		"drop-down terminal shows, it has the keyboard, and the command is refused."+tabAbout)

var launchCommand = scriptCommand(control.ActionLaunch, "bench launch <launcher>",
	"Start a launcher in the focused pane.",
	"Starts the launcher with that name, from Settings ▸ Launchers, in the running Bench's focused\n"+
		"pane, as the add dialog does, and brings Bench forward.\nA name with spaces goes in quotes.")

var listCommand = &command{
	name:     "list",
	synopsis: "bench list [--json] [--dir path]",
	summary:  "List the running Bench's tabs, panes, and windows.",
	about: "Prints the running Bench's tabs, with their panes and what each holds, and what needs you:\n" +
		"bells, finished commands, notifications, and unread counts. With --json, it prints them as JSON,\n" +
		"for scripts: the user guide (Control Bench from scripts) explains each field." + scriptAbout,
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		asJSON := fs.Bool("json", false, "print JSON, for scripts")
		dir := dirFlag(fs, "the support folder of the Bench to ask (default: Bench's support folder)")
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			return listBench(c, *dir, *asJSON)
		}
	},
}

// scriptCommand returns a script subcommand that the running Bench carries
// out.
func scriptCommand(action, synopsis, summary, about string) *command {
	return &command{
		name:     action,
		synopsis: synopsis,
		summary:  summary,
		about:    about + scriptAbout,
		flags: func(*flag.FlagSet) func(*cli, []string) error {
			return func(c *cli, args []string) error {
				req, err := control.ParseCommand(append([]string{action, "--"}, args...))
				if err != nil {
					return &usageError{msg: strings.TrimPrefix(err.Error(), "bench "+action+" ")}
				}
				return runScript(req)
			}
		},
	}
}

// scriptControl is how the script subcommands reach the running Bench.
// Tests replace it.
type scriptControl struct {
	// query asks the Bench whose support folder is dir for its list
	// (control.Query).
	query func(dir string) (control.List, error)
	// here says why the script subcommands can't work in this build, or
	// is nil in the macOS app.
	here func() error
	// forward sends a subcommand's arguments to the Bench running with the
	// support folder dir (ui.Forward). In the macOS app it doesn't return
	// once it has.
	forward func(dir string, args []string) error
}

// ctl is how the script subcommands reach the running Bench.
var ctl = scriptControl{query: control.Query, here: scriptsHere, forward: forwardToBench}

// Why a script subcommand can't reach Bench. Nothing answers at the socket
// both while Bench isn't running and while the setting is off, so the
// command says which from config.toml, which Bench saves the setting in.
// It doesn't look for Bench among the running apps, or take Wails' lock to
// see whether Bench holds it: a Bench starting then would take itself for
// a second one, and quit.
var (
	errNotRunning = errors.New("Bench isn't running: open it, then try again")
	errScriptsOff = errors.New("Scripts can't control Bench: open Bench, turn on Allow scripts to control Bench in " +
		"Settings ▸ Advanced, then try again")
	errNoAnswer = errors.New("Bench isn't running, or can't answer scripts: open Bench, then try again. " +
		"If it's open, the message in its status bar says why")
	// errDropdownShown refuses bench move-to-tab while the drop-down
	// shows, as Bench would.
	errDropdownShown = errors.New("the drop-down has the keyboard, so there's no shown window to move: hide the drop-down, then try again")
)

// runScript checks that the running Bench lets scripts control it, and
// what it can of what Bench would refuse, then forwards req to it.
func runScript(req control.Request) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	l, err := askBench(dir, true)
	if err != nil {
		return err
	}
	switch req.Action {
	case control.ActionTab:
		if control.FindTab(l.TabNames(), req.Arg) < 0 {
			return noTab(req.Arg)
		}
	case control.ActionMoveToTab:
		i := control.FindTab(l.TabNames(), req.Arg)
		if i < 0 {
			return noTab(req.Arg)
		}
		if i == l.Current-1 {
			return fmt.Errorf("Bench is on %s already: name another tab", control.Shown(l.Tabs[i].Name))
		}
		// The drop-down has the keyboard while it shows, as when the
		// command is typed in it, and Bench refuses then.
		if l.Dropdown != nil && l.Dropdown.Shown {
			return errDropdownShown
		}
	case control.ActionLaunch:
		// Bench uses config.toml's launchers, or the last good config's.
		if cfg := benchConfig(dir); cfg != nil && cfg.LauncherIndex(req.Arg) < 0 {
			return fmt.Errorf("there's no launcher called %s: Settings ▸ Launchers lists them", control.Shown(req.Arg))
		}
	}
	return ctl.forward(dir, control.CommandArgs(req))
}

// noTab says that there's no tab that arg names, as Bench would.
func noTab(arg string) error {
	if n, err := strconv.Atoi(arg); err == nil && n > 0 && strconv.Itoa(n) == arg {
		return fmt.Errorf("there's no tab %d", n)
	}
	return fmt.Errorf("there's no tab called %s", control.Shown(arg))
}

// askBench asks the Bench whose support folder is dir for its list, and
// says why it can't: Bench isn't running, or doesn't let scripts control
// it. Bench's own support folder (usual) is where the bench command reads
// config.toml too, to say whether the setting is off in it.
func askBench(dir string, usual bool) (control.List, error) {
	l, err := ctl.query(dir)
	if !errors.Is(err, control.ErrNotServing) {
		return l, err
	}
	if !usual {
		return control.List{}, fmt.Errorf("nothing answers at %s: Bench isn't running with that support folder, "+
			"or Allow scripts to control Bench is off in it", filepath.Join(dir, control.SocketFile))
	}
	if err := ctl.here(); err != nil {
		return control.List{}, err
	}
	if cfg := benchConfig(dir); cfg != nil && cfg.Settings.AllowScripts {
		return control.List{}, errNoAnswer
	}
	return control.List{}, errScriptsOff
}

// benchConfig returns the config that Bench would use, in its support
// folder dir: config.toml, or, if that has problems, the last good one. It
// returns nil if there's neither.
func benchConfig(dir string) *config.Config {
	cfg, problems, _, err := config.Read(filepath.Join(dir, config.ConfigFile))
	if err != nil || len(problems) > 0 {
		cfg = config.LoadGood(filepath.Join(dir, config.GoodFile))
	}
	return cfg
}

// listBench prints the running Bench's list: as JSON, or for people.
func listBench(c *cli, dirFlag string, asJSON bool) error {
	dir, err := supportDir(dirFlag)
	if err != nil {
		return err
	}
	l, err := askBench(dir, dirFlag == "")
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(c.stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(l)
	}
	printList(c.stdout, l)
	return nil
}

// printList prints a list for people: each tab, with a * for the current
// one, then each pane's items, with a * for the one it shows, and what
// needs the user, then the drop-down, if there's one, with a * while it
// shows. A long tab name, app name, title, or reason is cut on its own, so
// that what needs the user, at the end, is always there, and a control
// character in one, as a dropped folder's name can have, shows as "?"
// (control.Shown).
func printList(w io.Writer, l control.List) {
	for _, t := range l.Tabs {
		mark := " "
		if t.Current {
			mark = "*"
		}
		fmt.Fprintf(w, "%s %d %s%s\n", mark, t.Number, control.Shown(t.Name), needs(t.Attention, t.Unread))
		for i, p := range t.Panes {
			fmt.Fprintf(w, "    Pane %d\n", i+1)
			for _, it := range p.Items {
				printItem(w, it)
			}
		}
	}
	if l.Dropdown != nil {
		fmt.Fprintln(w, "  Drop-down")
		printItem(w, *l.Dropdown)
	}
}

// printItem prints an item of a list for people, with a * if it's shown,
// and says if it's a web page that Bench hasn't loaded yet.
func printItem(w io.Writer, it control.Item) {
	mark := " "
	if it.Shown {
		mark = "*"
	}
	line := control.Shown(it.App)
	if it.Title != "" && it.Title != it.App {
		line += ": " + control.Shown(it.Title)
	}
	if it.NotLoaded {
		line += " (not loaded yet)"
	}
	if it.Attention != "" {
		line += " (" + control.Shown(it.Attention) + ")"
	}
	if u := it.Unread.Text(); u != "" {
		line += " (" + u + " unread)"
	}
	fmt.Fprintf(w, "    %s %s\n", mark, line)
}

// needs says what in a tab needs the user, after its name, or "".
func needs(attention int, unread control.Unread) string {
	var parts []string
	switch {
	case attention == 1:
		parts = append(parts, "1 needs you")
	case attention > 1:
		parts = append(parts, strconv.Itoa(attention)+" need you")
	}
	if u := unread.Text(); u != "" {
		parts = append(parts, u+" unread")
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}
