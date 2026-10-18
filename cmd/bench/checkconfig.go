package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	toml "github.com/pelletier/go-toml/v2"

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

// readConfig reads and parses a config file without writing anything. It
// returns the config, or the problems with it as lines to print: each
// "tab "<name>": <field>: <message>", without the tab outside tabs.
// Problems include those that registering the [shortcuts] table would
// report.
func readConfig(path string) (*config.Config, []string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("there's no config file at %s; Bench writes the default one there when it first runs", path)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the config: %w", err)
	}
	cfg, problems, err := config.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("checking %s: %w", path, err)
	}
	var lines []string
	for _, p := range problems {
		lines = append(lines, p.Error())
	}
	table, ok := shortcutsTable(cfg, data)
	if ok {
		_, errs := shortcuts.Bindings(table)
		for _, err := range errs {
			lines = append(lines, err.Error())
		}
	}
	if len(lines) > 0 {
		return nil, lines, nil
	}
	return cfg, nil, nil
}

// shortcutsTable returns the config's [shortcuts] table. If the config has
// problems, so that there's no cfg, it reads the table from the file on its
// own, so that its problems are reported too; it returns false if it can't.
func shortcutsTable(cfg *config.Config, data []byte) (shortcuts.Config, bool) {
	if cfg != nil {
		return cfg.Shortcuts, true
	}
	file := struct {
		Shortcuts shortcuts.Config `toml:"shortcuts"`
	}{Shortcuts: config.Default().Shortcuts}
	if err := toml.Unmarshal(data, &file); err != nil {
		return shortcuts.Config{}, false
	}
	return file.Shortcuts, true
}

// checkConfig checks a config file and reports what it finds.
func checkConfig(c *cli, flagValue string) error {
	path, err := configPath(flagValue)
	if err != nil {
		return err
	}
	_, problems, err := readConfig(path)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
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
