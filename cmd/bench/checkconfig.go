package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/shortcuts"
)

var checkConfigCommand = &command{
	name:     "check-config",
	synopsis: "bench check-config [--config path]",
	summary:  "Check config.toml and report its problems.",
	about: "Reads config.toml and checks it as Bench does when it starts, including the [shortcuts]\n" +
		"table. It prints each problem with its tab and field, and exits with status 1 if there are any.",
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		path := configFlag(fs)
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			return checkConfig(c, *path)
		}
	},
}

// configReport is what readConfig finds in a config file. Problems are
// lines to print: each "tab "<name>": <field>: <message>", without the tab
// outside tabs.
type configReport struct {
	// Config is the config, or nil if it has Problems.
	Config *config.Config
	// Problems are the config's problems, other than its shortcuts'. With
	// any of them, the app runs with the default config instead.
	Problems []string
	// Shortcuts are the problems that registering the [shortcuts] table
	// would report. The app runs with them, reporting them in the status
	// bar.
	Shortcuts []string
}

// all returns every problem, the config's then its shortcuts'.
func (r configReport) all() []string {
	return append(slices.Clone(r.Problems), r.Shortcuts...)
}

// readConfig reads and parses a config file without writing anything, and
// reports its problems.
func readConfig(path string) (configReport, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return configReport{}, fmt.Errorf("there's no config file at %s; Bench writes the default one there when it first runs", path)
	}
	if err != nil {
		return configReport{}, fmt.Errorf("reading the config: %w", err)
	}
	cfg, problems, err := config.Parse(data)
	if err != nil {
		return configReport{}, fmt.Errorf("checking %s: %w", path, err)
	}
	var r configReport
	for _, p := range problems {
		r.Problems = append(r.Problems, p.Error())
	}
	if table, ok := shortcutsTable(cfg, data); ok {
		_, errs := shortcuts.Bindings(table)
		for _, err := range errs {
			r.Shortcuts = append(r.Shortcuts, err.Error())
		}
	}
	if len(r.Problems) == 0 {
		r.Config = cfg
	}
	return r, nil
}

// usableConfig reads a config file for a command that lays out its tabs
// (layout and arrange). It refuses one with problems, since the app would
// run with the default config instead, and prints them. Problems with the
// [shortcuts] table don't stop it, since the app runs with them and the
// command doesn't use shortcuts; it prints them as a warning.
func usableConfig(c *cli, command, path string) (*config.Config, error) {
	r, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	if len(r.Problems) > 0 {
		all := r.all()
		for _, line := range all {
			fmt.Fprintln(c.stderr, line)
		}
		return nil, fmt.Errorf("%s has %s; run bench check-config", path, count(len(all), "problem", "problems"))
	}
	if len(r.Shortcuts) > 0 {
		for _, line := range r.Shortcuts {
			fmt.Fprintln(c.stderr, line)
		}
		fmt.Fprintf(c.stderr, "%s has %s in [shortcuts], which bench %s doesn't use.\n",
			path, count(len(r.Shortcuts), "problem", "problems"), command)
	}
	return r.Config, nil
}

// shortcutsTable returns the config's [shortcuts] table. If the config has
// problems, so that there's no cfg, it reads the table from the file on its
// own, as Parse reads it (config.ShortcutsOf), so that its problems are
// reported too; it returns false if it can't. A newer shortcut that Parse
// would turn off then isn't reported as a clash.
func shortcutsTable(cfg *config.Config, data []byte) (shortcuts.Config, bool) {
	if cfg != nil {
		return cfg.Shortcuts, true
	}
	return config.ShortcutsOf(data)
}

// checkConfig checks a config file and reports what it finds.
func checkConfig(c *cli, flagValue string) error {
	path, err := configPath(flagValue)
	if err != nil {
		return err
	}
	r, err := readConfig(path)
	if err != nil {
		return err
	}
	if problems := r.all(); len(problems) > 0 {
		for _, line := range problems {
			fmt.Fprintln(c.stdout, line)
		}
		fmt.Fprintf(c.stderr, "%s has %s.\n", path, count(len(problems), "problem", "problems"))
		return exitStatus(exitError)
	}
	fmt.Fprintf(c.stdout, "%s is fine.\n", path)
	return nil
}

// count returns "1 problem" or "2 problems".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
