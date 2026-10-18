//go:build server

package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/ui"
)

// defaultPort is the browser preview's port.
const defaultPort = 8080

const appAbout = "Runs the browser preview: Bench's UI at http://localhost:<port>, backed by a demo of the\n" +
	"prototype's apps and pretend terminals, with placeholders for window contents. It keeps its\n" +
	"files in $BENCH_HOME, or the Preview folder in Bench's support folder, so that it never\n" +
	"touches the app's config.toml or state.json."

func appFlags(fs *flag.FlagSet) func(*cli, []string) error {
	port := fs.Int("port", defaultPort, "the port to serve the preview on")
	dir := dirFlag(fs, "the preview's support folder (default: $BENCH_HOME, or Preview in Bench's support folder)")
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
		fmt.Fprintf(c.stdout, "Bench's browser preview is at http://localhost:%d, with its files in %s. "+
			"Press Control-C to stop it.\n", *port, d)
		return ui.Run(ui.Options{
			Platform: platform.NewDemo(clock.Real()),
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
