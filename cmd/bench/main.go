// Command bench runs Bench, a macOS app that gathers any set of apps into one
// window with tabs. With no arguments it runs the app; its subcommands are
// for development and recovery:
//
//	bench                      Run Bench (the browser preview in a -tags server build).
//	bench check-config         Check config.toml and report its problems.
//	bench layout               Print every tab's pane frames for a screen size, as JSON.
//	bench arrange <tab>        Arrange a tab across the screen without Bench's window (macOS).
//	bench restore              Put back windows that a crashed Bench left parked (macOS).
//	bench watchdog             Wait for Bench to quit, then put back what it left parked (macOS).
//	bench version              Print the version.
//	bench help [command]       Explain a command.
//
// Errors go to standard error. The exit status is 0 on success, 1 for an
// error, and 2 for a usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/term"
)

// version is the release, set with -ldflags "-X main.version=1.0.0".
var version = "dev"

// Exit statuses.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	term.Version = version // Terminals see it as TERM_PROGRAM_VERSION.
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// cli is where a command writes.
type cli struct {
	stdout, stderr io.Writer
}

// command is a subcommand.
type command struct {
	name     string
	synopsis string // Such as "bench layout --width points --height points [flags]".
	summary  string // One line for the command list.
	about    string // A paragraph for `bench help <command>`.
	// flags adds the command's flags to fs and returns a function that runs
	// the command with its other arguments, once fs has parsed them.
	flags func(fs *flag.FlagSet) func(c *cli, args []string) error
}

// commands lists the subcommands in the order help shows them. The app
// itself runs when there's no subcommand.
var commands = []*command{
	checkConfigCommand,
	layoutCommand,
	arrangeCommand,
	restoreCommand,
	watchdogCommand,
	versionCommand,
}

// appCommand runs the app, or in the server build, the browser preview.
var appCommand = &command{
	name:     "bench",
	synopsis: "bench [flags]",
	summary:  "Run Bench.",
	about:    appAbout,
	flags:    appFlags,
}

var versionCommand = &command{
	name:     "version",
	synopsis: "bench version",
	summary:  "Print the version.",
	about:    "Prints Bench's version and, if it's known, the commit it was built from.",
	flags: func(*flag.FlagSet) func(*cli, []string) error {
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			fmt.Fprintln(c.stdout, "bench", versionText())
			return nil
		}
	},
}

// run runs Bench with the command-line arguments (without the program name)
// and returns the exit status.
func run(args []string, stdout, stderr io.Writer) int {
	c := &cli{stdout: stdout, stderr: stderr}
	switch {
	case len(args) == 1 && slices.Contains([]string{"-h", "-help", "--help"}, args[0]):
		return c.help(nil)
	case len(args) == 0 || strings.HasPrefix(args[0], "-"):
		return c.runCommand(appCommand, appArgs(args))
	}
	name, rest := args[0], args[1:]
	if name == "help" {
		return c.help(rest)
	}
	cmd := lookup(name)
	if cmd == nil {
		fmt.Fprintf(stderr, "bench: there's no command %q.\n\n", name)
		c.printCommands(stderr)
		return exitUsage
	}
	return c.runCommand(cmd, rest)
}

// appArgs drops the process serial number that macOS can pass to an app
// that Finder opens, such as -psn_0_12345.
func appArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-psn_") {
			out = append(out, a)
		}
	}
	return out
}

// lookup returns the subcommand called name, or nil.
func lookup(name string) *command {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd
		}
	}
	return nil
}

// runCommand parses a command's flags and runs it.
func (c *cli) runCommand(cmd *command, args []string) int {
	fs := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	runFn := cmd.flags(fs)
	rest, err := parseArgs(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		c.printUsage(c.stdout, cmd, fs)
		return exitOK
	}
	if err != nil {
		err = &usageError{msg: err.Error()}
	} else {
		err = runFn(c, rest)
	}
	return c.report(cmd, fs, err)
}

// report prints an error and returns the exit status for it.
func (c *cli) report(cmd *command, fs *flag.FlagSet, err error) int {
	var usage *usageError
	var exit exitStatus
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &exit):
		return int(exit)
	case errors.As(err, &usage):
		fmt.Fprintf(c.stderr, "%s: %v\n\n", prefix(cmd), sentence(err.Error()))
		c.printUsage(c.stderr, cmd, fs)
		return exitUsage
	default:
		fmt.Fprintf(c.stderr, "%s: %v\n", prefix(cmd), sentence(err.Error()))
		return exitError
	}
}

// prefix starts an error message about a command.
func prefix(cmd *command) string {
	if cmd == appCommand {
		return "bench"
	}
	return "bench " + cmd.name
}

// help explains Bench's commands, or one of them.
func (c *cli) help(args []string) int {
	switch {
	case len(args) == 0:
		c.printCommands(c.stdout)
		return exitOK
	case len(args) > 1:
		fmt.Fprintln(c.stderr, "bench help: name one command, such as bench help layout.")
		return exitUsage
	}
	cmd := lookup(args[0])
	if args[0] == "bench" {
		cmd = appCommand
	}
	if cmd == nil {
		fmt.Fprintf(c.stderr, "bench help: there's no command %q.\n\n", args[0])
		c.printCommands(c.stderr)
		return exitUsage
	}
	fs := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	cmd.flags(fs)
	c.printUsage(c.stdout, cmd, fs)
	return exitOK
}

// printCommands lists the commands.
func (c *cli) printCommands(w io.Writer) {
	fmt.Fprintln(w, "Bench gathers any set of apps into one window with tabs.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	rows := [][2]string{{appCommand.synopsis, appCommand.summary}}
	for _, cmd := range commands {
		rows = append(rows, [2]string{cmd.synopsis, cmd.summary})
	}
	rows = append(rows, [2]string{"bench help [command]", "Explain a command."})
	width := 0
	for _, r := range rows {
		width = max(width, len(r[0]))
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-*s  %s\n", width, r[0], r[1])
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Add -h to any command for its flags.")
}

// printUsage explains one command and its flags.
func (c *cli) printUsage(w io.Writer, cmd *command, fs *flag.FlagSet) {
	fmt.Fprintf(w, "Usage: %s\n\n%s\n", cmd.synopsis, cmd.about)
	hasFlags := false
	fs.VisitAll(func(*flag.Flag) { hasFlags = true })
	if hasFlags {
		fmt.Fprintln(w, "\nFlags:")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
}

// parseArgs parses flags wherever they are among args, so that
// `bench arrange Code --config x.toml` works, and returns the other
// arguments. Everything after "--" is an argument.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		left := fs.Args()
		// flag stops at the first argument, or just after "--".
		if consumed := len(args) - len(left); consumed > 0 && args[consumed-1] == "--" {
			return append(rest, left...), nil
		}
		if len(left) == 0 {
			return rest, nil
		}
		rest = append(rest, left[0])
		args = left[1:]
	}
}

// usageError is a mistake in how a command was run.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// usageErrorf returns a usageError with a formatted message.
func usageErrorf(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// unexpectedArgs is the usage error for a command that takes no arguments.
func unexpectedArgs(args []string) error {
	return usageErrorf("unexpected argument %q", args[0])
}

// exitStatus is an error that ends a command with a status, having already
// said why.
type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// sentence ends text with a full stop if it doesn't end with one.
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasSuffix(text, ".") {
		return text
	}
	return text + "."
}

// versionText is the version, with the commit if the build recorded one.
func versionText() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = ", modified"
			}
		}
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if revision == "" {
		return version
	}
	return fmt.Sprintf("%s (%s%s)", version, revision, modified)
}
