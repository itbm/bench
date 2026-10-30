//go:build darwin && cgo && !server

package mac

// This file is Bench's side of Sparkle (updater_darwin.m). cgo copies the
// preamble of a file with //export into _cgo_export.h, so it may only
// declare things.

/*
#include "bridge_darwin.h"
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"
)

// Updater installs new versions of Bench with Sparkle, which make app puts
// in Bench.app when it's given an update key (see
// docs/development.md#updates). It implements workspace.Updater. Make it
// with NewUpdater; there's one per process. Its methods are safe for
// concurrent use and return at once: Sparkle runs on the main thread.
type Updater struct {
	mu       sync.Mutex
	started  bool
	on       bool // Whether Sparkle may look each day.
	found    func(version string)
	finished func()
	failed   func(err error)
	// news holds Sparkle's news, in order, for a goroutine that passes it
	// on.
	news chan func()
}

// newsQueue is how much of Sparkle's news can wait to be passed on. Sparkle
// has a few things to say a day.
const newsQueue = 16

// theUpdater is the Updater that Sparkle's news goes to, once it has
// started.
var theUpdater atomic.Pointer[Updater]

// NewUpdater returns Bench.app's updater, or nil if Bench.app has no
// Sparkle: a build without an update key, or a binary outside Bench.app.
func NewUpdater() *Updater {
	if C.benchUpdaterAvailable() == 0 {
		return nil
	}
	return &Updater{}
}

// Start loads and starts Sparkle, once the app is running. found hears of
// each newer version that a daily look finds, finished that Sparkle is done
// with it, and failed of what went wrong, such as a look that failed. They
// may be called on any goroutine, and must not block.
func (u *Updater) Start(found func(version string), finished func(), failed func(err error)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.started {
		return
	}
	u.started = true
	u.found, u.finished, u.failed = found, finished, failed
	u.news = make(chan func(), newsQueue)
	go func() {
		for f := range u.news {
			f()
		}
	}()
	theUpdater.Store(u)
	C.benchUpdaterStart(cBool(u.on))
}

// SetAutomatic turns Sparkle's daily look for a new version on or off.
// Before Start, Start applies it.
func (u *Updater) SetAutomatic(on bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.on = on
	if u.started {
		C.benchUpdaterSetAutomatic(cBool(on))
	}
}

// Check looks for a new version now, and shows Sparkle's window with what
// it finds. If Sparkle has found one already, it shows that one. While
// Sparkle is busy, as it is for a few seconds after it starts and while it
// makes its daily look, Check waits for it, for up to 30 seconds.
func (u *Updater) Check() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.started {
		C.benchUpdaterCheck()
	}
}

// tell passes on Sparkle's news, after what came before it, without
// waiting: the native side calls on the main thread, and the engine takes
// its lock.
func (u *Updater) tell(f func()) {
	select {
	case u.news <- f:
	default:
		go f()
	}
}

//export benchUpdateFound
func benchUpdateFound(version *C.char) {
	if u := theUpdater.Load(); u != nil && u.found != nil {
		v := C.GoString(version)
		u.tell(func() { u.found(v) })
	}
}

//export benchUpdateFinished
func benchUpdateFinished() {
	if u := theUpdater.Load(); u != nil && u.finished != nil {
		u.tell(u.finished)
	}
}

//export benchUpdateFailed
func benchUpdateFailed(text *C.char) {
	if u := theUpdater.Load(); u != nil && u.failed != nil {
		err := errors.New(C.GoString(text))
		u.tell(func() { u.failed(err) })
	}
}
