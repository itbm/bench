//go:build darwin || server

package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// watchdogWait is how long closing the watchdog waits for it to exit. Tests
// shorten it.
var watchdogWait = 2 * time.Second

// errStillRunning is closing a piped process's error when it hasn't exited
// within watchdogWait.
var errStillRunning = errors.New("hasn't exited yet")

// executable finds this executable, for the watchdog. Tests replace it.
var executable = os.Executable

// watchdogArgs are the arguments that run `bench watchdog` for a support
// folder (see cmd/bench).
func watchdogArgs(dir string) []string {
	return []string{"watchdog", "--dir", dir}
}

// StartWatchdog starts `bench watchdog --dir <dir>` (this executable, which
// must be cmd/bench), with its standard input a pipe that this process holds
// open. If this process ends, even by crashing in native code, the pipe
// closes and the watchdog puts back the windows parked.json still lists.
// Close the returned Closer once the windows are back; closing it waits
// briefly for the watchdog to exit, and returns an error if it hasn't. Run
// starts one when Options.Watchdog is set.
func StartWatchdog(dir string) (io.Closer, error) {
	exe, err := executable()
	if err != nil {
		return nil, fmt.Errorf("starting the watchdog: finding Bench's executable: %w", err)
	}
	p, err := startPiped(exe, watchdogArgs(dir))
	if err != nil {
		// Returning p as it is would make a nil *pipedProcess into an
		// io.Closer that isn't nil, which quitting would then close.
		return nil, err
	}
	return p, nil
}

// pipedProcess is a child process whose standard input is a pipe that this
// process holds open.
type pipedProcess struct {
	pid  int
	pipe *os.File
	done chan struct{} // Closed when the process has exited.
	once sync.Once
}

// startPiped starts path with args and a pipe on its standard input. Its
// standard error is this process's. The pipe isn't passed to any other
// child process, so it closes when this process ends, however that happens.
// The process has a process group of its own, so that the signals a
// terminal sends Bench's process group (Control-C, Control-\, and a hangup
// when the terminal closes) don't reach it: it must outlive Bench.
func startPiped(path string, args []string) (*pipedProcess, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("starting %s: making a pipe: %w", path, err)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = r
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = ownProcessGroup()
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(fmt.Errorf("starting %s: %w", path, err), r.Close(), w.Close())
	}
	// The child has its own copy of the read end; closing this one can't
	// affect it.
	_ = r.Close()
	p := &pipedProcess{pid: cmd.Process.Pid, pipe: w, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// Close closes the pipe, which tells the process to finish, and waits up to
// watchdogWait for it to exit. It returns an error if the process is still
// running then, so that bench arrange knows to keep the folder the watchdog
// works in. It's safe to call more than once; later calls return nil.
func (p *pipedProcess) Close() error {
	var err error
	p.once.Do(func() {
		err = p.pipe.Close()
		select {
		case <-p.done:
		case <-time.After(watchdogWait):
			err = errors.Join(err, fmt.Errorf("process %d %w", p.pid, errStillRunning))
		}
	})
	return err
}
