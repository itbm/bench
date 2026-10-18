package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/vscode"
	"github.com/itbm/bench/internal/windows"
)

// errNeedsMacOS is the error from the macOS commands in other builds.
var errNeedsMacOS = errors.New("this command needs macOS: it moves other apps' windows (build Bench on a Mac, with cgo)")

var arrangeCommand = &command{
	name:     "arrange",
	synopsis: "bench arrange <tab> [--config path]",
	summary:  "Arrange a tab across the screen without Bench's window (macOS).",
	about: "Arranges one tab of config.toml across the screen's visible area, without Bench's window:\n" +
		"it opens the tab's apps if they aren't running, waits for their windows, docks each pane's\n" +
		"first window in the pane, hides apps whose windows all belong to other tabs, and parks the\n" +
		"other tabs' windows of apps that also have windows in this one. Press Control-C to put\n" +
		"everything back. Web and terminal items are skipped. It refuses while Bench is running.\n" +
		"Needs Accessibility permission.",
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		path := configFlag(fs)
		return func(c *cli, args []string) error {
			if len(args) != 1 {
				return usageErrorf("name one tab, such as bench arrange Code")
			}
			return arrange(c, args[0], *path)
		}
	},
}

var restoreCommand = &command{
	name:     "restore",
	synopsis: "bench restore [--dir path]",
	summary:  "Put back windows that a crashed Bench left parked (macOS).",
	about: "Puts back the windows recorded in parked.json, which a Bench that crashed left off screen,\n" +
		"and shows the apps it hid. Bench does this itself when it next starts.",
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		dir := dirFlag(fs, "the support folder holding parked.json (default: Bench's support folder)")
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			d, err := supportDir(*dir)
			if err != nil {
				return err
			}
			return restore(c, d)
		}
	},
}

var watchdogCommand = &command{
	name:     "watchdog",
	synopsis: "bench watchdog --dir path",
	summary:  "Wait for Bench to quit, then put back what it left parked (macOS).",
	about: "Bench starts this itself. It waits until its standard input closes, which happens when\n" +
		"Bench quits or crashes, then puts back any windows that parked.json still lists.",
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		dir := dirFlag(fs, "the support folder holding parked.json (required)")
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			if *dir == "" {
				return usageErrorf("give the support folder with --dir")
			}
			return watchdog(c, *dir)
		}
	},
}

// arrangeSetup is where `bench arrange` runs the engine from.
type arrangeSetup struct {
	// Dir is the support folder for the engine: the config's own folder if
	// the config is called config.toml, or a temporary copy.
	Dir string
	// Temporary is true if Dir is a temporary folder to remove afterwards.
	Temporary bool
}

// prepareArrange checks the config and the tab for `bench arrange`, and
// works out the folder the engine runs from, which must hold the config as
// config.toml. A config with problems is an error here: the app would run
// with the default config instead, which isn't what arrange is asked for.
// Problems with its [shortcuts] table are only a warning, as they are for
// the app.
func prepareArrange(c *cli, tab, flagValue string) (arrangeSetup, error) {
	path, err := configPath(flagValue)
	if err != nil {
		return arrangeSetup{}, err
	}
	cfg, err := usableConfig(c, "arrange", path)
	if err != nil {
		return arrangeSetup{}, err
	}
	if cfg.TabIndex(tab) < 0 {
		names := make([]string, len(cfg.Tabs))
		for i, t := range cfg.Tabs {
			names[i] = t.Name
		}
		return arrangeSetup{}, fmt.Errorf("there's no tab called %q; the tabs are %s", tab, strings.Join(names, ", "))
	}
	if filepath.Base(path) == config.ConfigFile {
		return arrangeSetup{Dir: filepath.Dir(path)}, nil
	}
	return copyConfig(path)
}

// copyConfig copies a config file into a new temporary folder as
// config.toml.
func copyConfig(path string) (arrangeSetup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return arrangeSetup{}, fmt.Errorf("reading the config: %w", err)
	}
	dir, err := os.MkdirTemp("", "bench-arrange-")
	if err != nil {
		return arrangeSetup{}, fmt.Errorf("making a folder for the config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, config.ConfigFile), data, 0o600); err != nil {
		return arrangeSetup{}, errors.Join(fmt.Errorf("copying the config: %w", err), os.RemoveAll(dir))
	}
	return arrangeSetup{Dir: dir, Temporary: true}, nil
}

// restoreParked puts back what parked.json in dir lists, with put, and
// returns what the file listed. It does nothing if the file is missing or
// empty. A file that can't be read is still given to put, which removes it.
func restoreParked(dir string, put func(path string) error) (windows.ParkedFile, error) {
	path := filepath.Join(dir, config.ParkedFile)
	f, err := windows.ReadParked(path)
	if err == nil && f.Empty() {
		return f, nil
	}
	if err := put(path); err != nil {
		return f, fmt.Errorf("putting windows back: %w", err)
	}
	return f, nil
}

// waitAndRestore is the watchdog: it waits until stdin closes, which happens
// when Bench quits or crashes, then puts back what parked.json in dir still
// lists, with put, and stops the VS Code for the Web server that
// vscode.json records. After a clean quit there's nothing left to do.
func waitAndRestore(stdin io.Reader, dir string, put func(path string) error) error {
	// Any error reading means the pipe has gone, as at its end.
	_, _ = io.Copy(io.Discard, stdin)
	_, err := restoreParked(dir, put)
	return errors.Join(err, stopVSCode(dir))
}

// stopVSCode stops the VS Code for the Web server that a Bench which crashed
// left running, as vscode.json in dir records it.
func stopVSCode(dir string) error {
	return vscode.StopRecorded(filepath.Join(dir, config.VSCodeFile))
}

// vscodeOptions are the options the app starts the VS Code for the Web
// server with: its record, and its token file, in the support folder dir.
// The token isn't in the temporary folder, which macOS clears of files that
// haven't been used for a few days, while the server rereads the token
// whenever it starts VS Code again.
func vscodeOptions(dir string) vscode.Options {
	return vscode.Options{TokenDir: dir, Record: filepath.Join(dir, config.VSCodeFile)}
}

// whileLooping runs work on a new goroutine while loop runs on this one,
// and returns once both have finished. loop must return once its stop
// channel is closed, which happens when work returns. bench arrange runs the
// main thread's run loop this way (mac.RunMainLoop) while it arranges:
// macOS updates its list of running apps, and reports apps' windows, only
// while that runs.
func whileLooping(loop func(stop <-chan struct{}), work func()) {
	stop := make(chan struct{})
	go func() {
		defer close(stop)
		work()
	}()
	loop(stop)
	<-stop
}

// parkedSummary says what a parked-windows file listed, such as
// "3 windows and 1 app".
func parkedSummary(f windows.ParkedFile) string {
	var parts []string
	if n := len(f.Windows); n > 0 {
		parts = append(parts, count(n, "window", "windows"))
	}
	if n := len(f.Hidden); n > 0 {
		parts = append(parts, count(n, "app", "apps"))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, " and ")
}
