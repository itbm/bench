package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/vscode"
	"github.com/itbm/bench/internal/windows"
)

// errNeedsMacOS is the error from the macOS commands in other builds.
var errNeedsMacOS = errors.New("this command needs macOS: it moves other apps' windows (build Bench on a Mac, with cgo)")

// lockFile is the lock file in a support folder. Every process that acts on
// the folder's parked.json and vscode.json holds an flock(2) lock on it
// while it does: the app and bench arrange for as long as they run, and
// bench restore and the watchdog while they put windows back. So they take
// turns, and none puts back or overwrites what another has just recorded.
// The watchdog takes it only once Bench has gone.
const lockFile = "bench.lock"

// errLocked means another process holds a lock.
var errLocked = errors.New("another process holds the lock")

// errFolderBusy is bench restore's error while another process holds the
// support folder's lock.
var errFolderBusy = errors.New("Bench or bench arrange is running, and it puts these windows back itself as it quits " +
	"(or its watchdog is putting them back now): quit it, then try again")

var arrangeCommand = &command{
	name:     "arrange",
	synopsis: "bench arrange <tab> [--config path]",
	summary:  "Arrange a tab across the screen without Bench's window (macOS).",
	about: "Arranges one tab of config.toml across the screen's visible area, without Bench's window:\n" +
		"it opens the tab's apps if they aren't running, waits for their windows, docks each pane's\n" +
		"first window in the pane, hides apps whose windows all belong to other tabs, and parks the\n" +
		"other tabs' windows of apps that also have windows in this one. Press Control-C to put\n" +
		"everything back. Web and terminal items are skipped. It refuses while Bench, or another\n" +
		"bench arrange, is running, and Bench can't open until it has finished.\n" +
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
		"and shows the apps it hid. Bench does this itself when it next starts. It refuses while Bench,\n" +
		"or bench arrange, runs from the same support folder.",
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

// putFunc puts back what a parked-windows file lists
// (windows.RestoreFromFileReport), and says what it put back.
type putFunc func(path string) (windows.Restored, error)

// restoreParked puts back what parked.json in dir lists, with put, and
// returns what put says it put back. It does nothing if the file is missing
// or empty. A file that can't be read is still given to put, which sets it
// aside as parked.json.bad. The caller holds the folder's lock (lockDir).
func restoreParked(dir string, put putFunc) (windows.Restored, error) {
	path := filepath.Join(dir, config.ParkedFile)
	f, err := windows.ReadParked(path)
	if err == nil && f.Empty() {
		return windows.Restored{}, nil
	}
	done, err := put(path)
	if err != nil {
		return done, fmt.Errorf("putting windows back: %w", err)
	}
	return done, nil
}

// restoreNow is bench restore: it puts back what parked.json in dir lists,
// with put, unless another process holds the folder's lock. That's a Bench
// or bench arrange that's running, which puts the windows back itself, and
// whose records putting them back would delete, or a watchdog that's
// putting them back already.
func restoreNow(dir string, put putFunc) (windows.Restored, error) {
	unlock, err := lockDir(dir, nil)
	if errors.Is(err, errLocked) {
		return windows.Restored{}, errFolderBusy
	}
	if err != nil {
		return windows.Restored{}, err
	}
	defer unlock()
	return restoreParked(dir, put)
}

// waitAndRestore is the watchdog: it waits until stdin closes, which happens
// when Bench quits or crashes, then takes the folder's lock, waiting for any
// process that holds it (wait is called first if one does), puts back what
// parked.json in dir still lists, with put, and stops the VS Code for the
// Web server that vscode.json records. After a clean quit there's nothing
// left to do. A Bench that starts again meanwhile, and so takes the lock
// first, puts back what its last run left itself, and the watchdog waits
// for it to quit. If the lock can't be taken at all, it still puts the
// windows back, as nothing else may.
func waitAndRestore(stdin io.Reader, dir string, put putFunc, wait func()) error {
	// Any error reading means the pipe has gone, as at its end.
	_, _ = io.Copy(io.Discard, stdin)
	unlock, lockErr := lockDir(dir, wait)
	if lockErr == nil {
		defer unlock()
	}
	_, err := restoreParked(dir, put)
	return errors.Join(lockErr, err, stopVSCode(dir))
}

// lockSupportDir creates the support folder dir if it's missing, readable
// only by the user, as Bench's other files are, and takes its lock for the
// app or bench arrange, which hold it for as long as they run. If a
// watchdog or bench restore holds it, it says so on w, and waits for it to
// finish.
func lockSupportDir(w io.Writer, dir string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the support folder: %w", err)
	}
	return lockDir(dir, func() {
		fmt.Fprintf(w, "bench: waiting while another process puts back windows from %s…\n", dir)
	})
}

// quitSignals are the signals that make bench arrange put everything back
// and exit: Control-C, kill's default, and the terminal closing. SIGHUP is
// left out if it's ignored, as nohup starts a command: listening for it
// would stop it being ignored, and such a command is meant to outlive its
// terminal.
func quitSignals() []os.Signal {
	sigs := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if !signal.Ignored(syscall.SIGHUP) {
		sigs = append(sigs, syscall.SIGHUP)
	}
	return sigs
}

// finishArrange removes bench arrange's temporary support folder once
// nothing in it is left to put back: once parked.json is empty and the
// watchdog, which works in the folder, has exited (watchdogDone). Otherwise
// parked.json is the only record of the windows still parked, so it keeps
// the folder, whether temporary or not, and says on w how to put them back.
func finishArrange(w io.Writer, setup arrangeSetup, watchdogDone bool) error {
	left, err := windows.ReadParked(filepath.Join(setup.Dir, config.ParkedFile))
	if watchdogDone && err == nil && left.Empty() {
		if setup.Temporary {
			return os.RemoveAll(setup.Dir)
		}
		return nil
	}
	fmt.Fprintf(w, "bench arrange: windows may still be parked; to put them back, run: bench restore --dir %s\n",
		shellQuote(setup.Dir))
	return nil
}

// shellQuote quotes s for a shell, if it needs it.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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

// restoredSummary says what bench restore put back, and what had gone
// since, such as "Put back 1 window and 1 app; 2 windows had closed." It's
// empty if the file listed nothing.
func restoredSummary(r windows.Restored) string {
	var back, gone []string
	if r.Windows > 0 {
		back = append(back, count(r.Windows, "window", "windows"))
	}
	if r.Apps > 0 {
		back = append(back, count(r.Apps, "app", "apps"))
	}
	if r.Closed > 0 {
		gone = append(gone, count(r.Closed, "window", "windows")+" had closed")
	}
	if r.Quit > 0 {
		gone = append(gone, count(r.Quit, "app", "apps")+" had quit")
	}
	if len(back) == 0 && len(gone) == 0 {
		return ""
	}
	s := "Put back nothing"
	if len(back) > 0 {
		s = "Put back " + strings.Join(back, " and ")
	}
	if len(gone) > 0 {
		s += "; " + strings.Join(gone, " and ")
	}
	return s + "."
}
