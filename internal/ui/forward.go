//go:build darwin || server

package ui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ErrNotRunning is Forward's error when no Bench is running to send to.
var ErrNotRunning = errors.New("Bench isn't running")

// Forward sends args, a script subcommand's arguments after the program's
// name (control.CommandArgs), to the Bench that's running with the support
// folder dir, as Wails' single-instance lock sends the arguments of a second
// Bench to the first, where secondInstance hands them to the engine's
// RunScript. Wails sends os.Args, so Forward sets them to args first, and
// encrypts them with the support folder's key (forwardKey), as the running
// Bench takes nothing else, with a stamp (newStamp), as the running Bench
// takes each subcommand once, and only soon after it was sent (stampGuard).
// In the macOS app it doesn't return while a Bench runs: Wails exits, with
// status 0, once it has sent them. It returns ErrNotRunning if no Bench is
// running, and an error in the browser preview, which has no single-instance
// lock.
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
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: UniqueID, EncryptionKey: key, AdditionalData: newStamp(time.Now()),
		},
	})
	// Wails took the lock: no Bench is running. Exiting lets it go.
	return ErrNotRunning
}

// A script subcommand's stamp. Any app of the user's can read the
// distributed notification that carries a subcommand, and post it again,
// and the key can't stop that: what it posts is still encrypted with it.
// So Forward stamps each subcommand with when it sent it and an ID of its
// own, in SingleInstanceOptions.AdditionalData, which Wails encrypts with
// the arguments, so that nothing can change it. The running Bench carries
// out only a subcommand with a stamp that's fresh, and whose ID it hasn't
// had before (stampGuard). Links and the arguments of a Bench opened again
// have no stamp, and need none: any app can open a link, or open Bench.
const (
	// stampSent is when Forward sent the subcommand, in nanoseconds since
	// 1970, in decimal.
	stampSent = "sent"
	// stampID is a random ID, new for each subcommand.
	stampID = "id"
)

// stampFresh is how long a stamp is fresh for, before and after now: the
// time a subcommand takes to arrive, with some to spare. Forward and the
// running Bench run on the same Mac, so they have the same clock.
const stampFresh = 10 * time.Second

// newStamp returns a stamp for a subcommand that Forward sends at now.
func newStamp(now time.Time) map[string]string {
	return map[string]string{stampSent: strconv.FormatInt(now.UnixNano(), 10), stampID: rand.Text()}
}

// Why stampGuard refuses a subcommand.
var (
	errNoStamp    = errors.New("it has no stamp")
	errStampOld   = fmt.Errorf("it wasn't sent within %d seconds of coming", int(stampFresh/time.Second))
	errStampTaken = errors.New("it came before")
)

// stampGuard takes each stamped subcommand once, while it's fresh. Its
// zero value is ready to use.
type stampGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time // The IDs taken, with when they came.
}

// take checks the stamp of a subcommand that came at now, and remembers
// its ID. It refuses one without a stamp, one sent more than stampFresh
// before or after now, and one whose ID it has had. It forgets an ID once
// it has had it for twice stampFresh, by when no copy of the subcommand
// can be fresh.
func (g *stampGuard) take(stamp map[string]string, now time.Time) error {
	sent, err := strconv.ParseInt(stamp[stampSent], 10, 64)
	id := stamp[stampID]
	if err != nil || id == "" {
		return errNoStamp
	}
	if age := now.Sub(time.Unix(0, sent)); age > stampFresh || age < -stampFresh {
		return errStampOld
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for seen, at := range g.seen {
		if now.Sub(at) > 2*stampFresh {
			delete(g.seen, seen)
		}
	}
	if _, ok := g.seen[id]; ok {
		return errStampTaken
	}
	if g.seen == nil {
		g.seen = map[string]time.Time{}
	}
	g.seen[id] = now
	return nil
}
