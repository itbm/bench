//go:build server

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/mac"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/ui"
)

// defaultPort is the browser preview's port.
const defaultPort = 8080

const appAbout = "Runs the browser preview: Bench's UI at http://localhost:<port>, backed by a demo of the\n" +
	"prototype's apps and pretend terminals, with placeholders for window contents. It keeps its\n" +
	"files in the Preview folder in Bench's support folder ($BENCH_HOME, if that's set), so that\n" +
	"it never touches the app's config.toml, state.json, or parked.json."

func appFlags(fs *flag.FlagSet) func(*cli, []string) error {
	port := fs.Int("port", defaultPort, "the port to serve the preview on")
	dir := dirFlag(fs, "the preview's support folder (default: Preview in Bench's support folder)")
	return func(c *cli, args []string) error {
		if len(args) > 0 {
			return usageErrorf("unexpected argument %q; see bench help for the commands", args[0])
		}
		if *port < 1 || *port > 65535 {
			return usageErrorf("--port must be from 1 to 65535")
		}
		d, err := previewDir(*dir)
		if err != nil {
			return err
		}
		if err := checkPort(*port); err != nil {
			return err
		}
		// ui.Run unsets these, so that Wails serves where this says.
		for _, name := range ui.ServerEnv {
			if value, ok := os.LookupEnv(name); ok {
				fmt.Fprintf(c.stderr, "bench: ignoring %s=%s: the preview serves on localhost, on --port.\n", name, value)
			}
		}
		fmt.Fprintf(c.stdout, "Bench's browser preview is at http://localhost:%d, with its files in %s. "+
			"Press Control-C to stop it.\n", *port, d)
		demo := platform.NewDemo(clock.Real())
		// The demo's web views show placeholders, not pages: each reports
		// the address it's given as the page in view, as internal/mac's
		// pure-Go PageInfo describes it, so that its pane's header shows the
		// page's host, as the app's does.
		demo.SetWebPages(func(address string) platform.Page { return mac.PageInfo(address, address, true) })
		return ui.Run(ui.Options{
			Version:  versionText(),
			Platform: demo,
			NewTerms: func(cb term.Callbacks) term.Host { return term.NewEcho(cb) },
			Dir:      d,
			Demo:     true,
			Port:     *port,
		})
	}
}

// checkPort reports a port that's in use before saying the preview is
// there. Wails listens on it a moment later.
func checkPort(port int) error {
	l, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("can't serve the preview on port %d (try another with --port): %w", port, err)
	}
	return l.Close()
}

func arrange(*cli, string, string) error { return errNeedsMacOS }
func restore(*cli, string) error         { return errNeedsMacOS }
func watchdog(*cli, string) error        { return errNeedsMacOS }

// scriptsHere says that the script subcommands control only the macOS
// app.
func scriptsHere() error { return errScriptsNeedMacOS }

// forwardToBench can't forward here: only the macOS app has the
// single-instance lock that the arguments go through.
func forwardToBench(string, []string) error { return errScriptsNeedMacOS }
