//go:build darwin || server

package ui

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ErrNotRunning is Forward's error when no Bench is running to send to.
var ErrNotRunning = errors.New("Bench isn't running")

// Forward sends args, a script subcommand's arguments after the program's
// name (control.CommandArgs), to the Bench that's running with the support
// folder dir, as Wails' single-instance lock sends the arguments of a
// second Bench to the first, where secondInstance hands them to the
// engine's RunScript. Wails sends os.Args, so Forward sets them to args
// first, and encrypts them with the support folder's key (forwardKey), as
// the running Bench takes nothing else. In the macOS app it doesn't return
// while a Bench runs: Wails exits, with status 0, once it has sent them. It
// returns ErrNotRunning if no Bench is running, and an error in the browser
// preview, which has no single-instance lock.
func Forward(dir string, args []string) error {
	if !nativeShell {
		return errors.New("the bench command can control only the macOS app")
	}
	key, err := forwardKey(dir)
	if err != nil {
		return fmt.Errorf("reading the key that Bench takes scripts with: %w", err)
	}
	os.Args = append([]string{os.Args[0]}, args...)
	application.New(application.Options{
		Name:   appName,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Assets: application.AssetOptions{
			Handler:        http.NotFoundHandler(),
			DisableLogging: true,
		},
		DisableDefaultSignalHandler: true,
		SingleInstance:              &application.SingleInstanceOptions{UniqueID: UniqueID, EncryptionKey: key},
	})
	// Wails took the lock: no Bench is running. Exiting lets it go.
	return ErrNotRunning
}
