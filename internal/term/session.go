//go:build darwin || linux

package term

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

const (
	// readSize is the most output read from a pty at once.
	readSize = 32 * 1024
	// inputQueue is how many writes can wait for a session to take them.
	inputQueue = 1024
	// drainWait is how long to wait, after the process ends, for the last
	// of its output before reporting that it ended. Output only goes on
	// longer than this if something else still holds the terminal open.
	drainWait = 2 * time.Second
)

// session is one running (or ended) process in a pseudo-terminal.
type session struct {
	id       string
	cb       Callbacks
	replaced *atomic.Bool // set when a later Start with the same ID replaces this session
	cmd      *exec.Cmd
	pty      *os.File
	shell    string // the shell's name, for titles
	startDir string
	home     string

	input    chan []byte
	hint     chan struct{} // asks the title watcher to look now
	stop     chan struct{} // closed when the pty is closed
	exited   chan struct{} // closed when the process has ended
	readDone chan struct{} // closed when the reader has finished

	// emitMu serialises callbacks, so Data, Title, and Exit arrive in
	// order, and nothing arrives after Exit or once the session has been
	// replaced. Methods never take it.
	emitMu sync.Mutex

	mu         sync.Mutex
	out        ring
	osc        oscParser
	shellTitle string // set by the shell with OSC 0 or 2
	oscDir     string // reported by the shell with OSC 7
	title      string // the latest title
	closing    bool   // Close was called
	ptyClosed  bool
	ended      bool // Exit has been reported, or skipped
}

// startSession starts the shell (or command) for opts in a new pty and the
// goroutines that look after it.
func startSession(id string, opts Options, cb Callbacks, titleEvery time.Duration, replaced *atomic.Bool) (*session, error) {
	home := homeDir()
	shell, err := chooseShell(opts.Shell, os.Getenv("SHELL"))
	if err != nil {
		return nil, err
	}
	dir, notice, err := startDir(opts.Dir, home, opts.Command)
	if err != nil {
		return nil, err
	}

	cmd, viaHelper := shellCommand(shell, shellArgs(opts.Command))
	cmd.Dir = dir
	cmd.Env = sessionEnv(environ(), dir, opts.Env)
	cols, rows := opts.size()
	// StartWithSize starts the process in a session of its own, with the
	// pty as its controlling terminal.
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", shell, err)
	}

	s := &session{
		id:       id,
		cb:       cb,
		replaced: replaced,
		cmd:      cmd,
		pty:      f,
		shell:    filepath.Base(shell),
		startDir: dir,
		home:     home,
		input:    make(chan []byte, inputQueue),
		hint:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		exited:   make(chan struct{}),
		readDone: make(chan struct{}),
		out:      newRing(SnapshotLimit),
	}
	if viaHelper {
		// The helper may not have become the shell yet, and its name
		// isn't one to show.
		s.title = s.computeTitle(s.shell, "")
	} else {
		s.title = s.computeTitle(s.foreground())
	}

	go s.read(notice)
	go s.write()
	go s.wait()
	go s.watchTitle(titleEvery)
	return s, nil
}

// read sends output to Callbacks.Data until the pty closes.
func (s *session) read(notice string) {
	defer close(s.readDone)
	if notice != "" {
		s.deliver([]byte(notice))
	}
	buf := make([]byte, readSize)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.deliver(bytes.Clone(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// deliver records a chunk of output for Snapshot, looks for title changes in
// it, and passes it on.
func (s *session) deliver(chunk []byte) {
	s.mu.Lock()
	s.out.Write(chunk)
	titleChanged := false
	s.osc.Feed(chunk, func(code, text string) {
		if s.applyOSC(code, text) {
			titleChanged = true
		}
	})
	s.mu.Unlock()

	s.emitMu.Lock()
	if s.mayReport() {
		s.cb.Data(s.id, chunk)
	}
	s.emitMu.Unlock()

	if titleChanged {
		s.hintTitle()
	}
}

// applyOSC handles one OSC sequence, and reports whether it might change the
// title. s.mu must be held.
func (s *session) applyOSC(code, text string) bool {
	switch code {
	case "0", "2":
		title := cleanTitle(text)
		changed := title != s.shellTitle
		s.shellTitle = title
		return changed
	case "7":
		dir, ok := parseFileURL(text)
		if !ok || dir == s.oscDir {
			return false
		}
		s.oscDir = dir
		return true
	}
	return false
}

// write sends queued input to the pty. Writes can block while the process
// isn't reading, so they happen here rather than in Manager.Write.
func (s *session) write() {
	for {
		select {
		case <-s.stop:
			return
		case data := <-s.input:
			// After an error, carry on draining: the process is
			// ending, and wait reports that.
			_, _ = s.pty.Write(data)
		}
	}
}

// wait waits for the process to end, then closes the pty and reports Exit,
// exactly once.
func (s *session) wait() {
	err := s.cmd.Wait()
	close(s.exited)
	select {
	case <-s.readDone:
	case <-time.After(drainWait):
	}
	s.closePTY()

	s.mu.Lock()
	if s.closing {
		// Ended on purpose, by Close.
		err = nil
	}
	s.mu.Unlock()
	if err != nil {
		err = fmt.Errorf("%s ended: %w", s.shell, err)
	}

	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	report := s.mayReport()
	s.mu.Lock()
	s.ended = true
	s.mu.Unlock()
	if report {
		s.cb.Exit(s.id, err)
	}
}

// watchTitle checks the title every interval, and when the reader sees a
// change, until the process ends or the session closes.
func (s *session) watchTitle(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.exited:
			return
		case <-s.stop:
			return
		case <-t.C:
		case <-s.hint:
		}
		s.updateTitle()
	}
}

// hintTitle asks the title watcher to look now, without waiting.
func (s *session) hintTitle() {
	select {
	case s.hint <- struct{}{}:
	default:
	}
}

// updateTitle works out the title and reports it if it has changed.
func (s *session) updateTitle() {
	name, dir := s.foreground()
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	title := s.computeTitle(name, dir)
	changed := title != s.title
	s.title = title
	s.mu.Unlock()
	if changed && s.mayReport() {
		s.cb.Title(s.id, title)
	}
}

// computeTitle returns the title the shell set, or "<command> — <folder>".
// s.mu must be held, except while the session is being made.
//
// The folder and the command's name come from the program too, through OSC 7
// or the names it gives its folders and processes, so they're cleaned as the
// shell's own title is.
func (s *session) computeTitle(command, procDir string) string {
	if s.shellTitle != "" {
		return s.shellTitle
	}
	dir := s.oscDir
	if dir == "" {
		dir = procDir
	}
	if dir == "" {
		dir = s.startDir
	}
	return cleanTitle(formatTitle(command, abbreviate(dir, s.home)))
}

// foreground returns the name and folder of the process in the foreground
// of the terminal: the leader of its foreground process group. It falls
// back to the shell, and the folder may be unknown.
func (s *session) foreground() (name, dir string) {
	shellPid := s.cmd.Process.Pid
	pids := []int{shellPid}
	if pgrp, ok := s.foregroundGroup(); ok && pgrp != shellPid {
		pids = []int{pgrp, shellPid}
	}
	for _, pid := range pids {
		if n, ok := processName(pid); ok {
			d, _ := processDir(pid)
			return n, d
		}
	}
	return s.shell, ""
}

// foregroundGroup returns the terminal's foreground process group.
func (s *session) foregroundGroup() (int, bool) {
	conn, err := s.pty.SyscallConn()
	if err != nil {
		return 0, false
	}
	pgrp, ioErr := 0, error(nil)
	// Control holds the file open while it runs, so the descriptor can't be
	// closed and reused underneath the ioctl.
	if err := conn.Control(func(fd uintptr) {
		pgrp, ioErr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	}); err != nil || ioErr != nil || pgrp <= 0 {
		return 0, false
	}
	return pgrp, true
}

// send queues input for the process.
func (s *session) send(data []byte) error {
	if !s.running() {
		return ErrEnded
	}
	select {
	case s.input <- bytes.Clone(data):
		return nil
	default:
		return ErrBusy
	}
}

// resize sets the pty's size. The kernel tells the foreground process with
// SIGWINCH.
func (s *session) resize(cols, rows int) error {
	if err := checkSize(cols, rows); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptyClosed {
		// Nothing to resize once the session has ended.
		return nil
	}
	ws := &pty.Winsize{Cols: uint16(clampCells(cols)), Rows: uint16(clampCells(rows))}
	if err := pty.Setsize(s.pty, ws); err != nil {
		return fmt.Errorf("resize terminal: %w", err)
	}
	return nil
}

// running reports whether the process is still going and Close hasn't been
// called.
func (s *session) running() bool {
	select {
	case <-s.exited:
		return false
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closing && !s.ptyClosed
}

// mayReport reports whether callbacks may still be called: not once Exit
// has been, nor once a later Start has replaced the session.
func (s *session) mayReport() bool {
	if s.replaced.Load() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.ended
}

// hangUp ends the session: SIGHUP to the shell and to the foreground process
// group, then the pty is closed. Exit follows when the process ends.
//
// The pty is in blocking mode (creack/pty's Open calls Fd), so while the
// reader is waiting in Read, closing it can't release the descriptor: the
// terminal is only hung up for good once everything using it has let go. A
// process that ignores SIGHUP keeps it open until it ends by itself.
func (s *session) hangUp() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	select {
	case <-s.exited:
	default:
		pgrp, ok := s.foregroundGroup()
		// Process.Signal is safe even if the process has just been
		// reaped; a process group ID can't be reused while the group
		// has members.
		_ = s.cmd.Process.Signal(syscall.SIGHUP)
		if ok && pgrp != s.cmd.Process.Pid {
			_ = syscall.Kill(-pgrp, syscall.SIGHUP)
		}
	}
	s.closePTY()
}

// closePTY closes the pty once. A read in progress keeps the descriptor open
// until it returns, which it does when every process using the terminal has
// closed it.
func (s *session) closePTY() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptyClosed {
		return
	}
	s.ptyClosed = true
	close(s.stop)
	_ = s.pty.Close()
}
