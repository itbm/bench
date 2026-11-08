//go:build !server && !(darwin && cgo)

package main

import (
	"errors"
	"flag"
)

const appAbout = "Runs Bench, which needs macOS. Elsewhere, run the browser preview instead:\n" +
	"  " + previewCommand

// previewCommand runs the browser preview from a checkout. Wails needs GTK
// for its Linux webview when cgo is on, so it's off.
const previewCommand = "CGO_ENABLED=0 go run -tags server ./cmd/bench"

// errNoApp says how to run the preview instead of the app.
var errNoApp = errors.New("Bench's app needs macOS, built with cgo. For the browser preview, run " +
	previewCommand + " instead")

func appFlags(*flag.FlagSet) func(*cli, []string) error {
	return func(c *cli, args []string) error {
		if len(args) > 0 {
			return usageErrorf("unexpected argument %q; see bench help for the commands", args[0])
		}
		return errNoApp
	}
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
