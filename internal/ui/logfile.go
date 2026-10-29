//go:build darwin || server

package ui

import (
	"os"
	"path/filepath"
	"sync"
)

// logFileLimit is the most, in bytes, that the log file holds before it
// starts again. Tests lower it.
var logFileLimit int64 = 1 << 20

// logFile is Bench's log file, which keeps the log from one launch to the
// next, for reporting a problem: standard error is lost when Bench opens
// from the Finder or at login. Only the user can read it, or its folder.
// Once it would grow past logFileLimit, it's moved aside, as <name>.1, in
// place of the one moved aside before, and starts again, so that the two
// together hold no more than twice that. Until hold is called, the file is
// only added to, never moved aside.
type logFile struct {
	mu   sync.Mutex
	path string
	f    *os.File // Nil if it couldn't be opened again.
	size int64
	held bool
}

// openLogFile opens the log file at path, to add to it, making it and its
// folder, readable only by the user, if they don't exist.
func openLogFile(path string) (*logFile, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	l := &logFile{path: path}
	if err := l.openLocked(); err != nil {
		return nil, err
	}
	return l, nil
}

// openLocked opens the file, to add to it, readable only by the user, even
// if it was made otherwise.
func (l *logFile) openLocked() error {
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.size = f, info.Size()
	return nil
}

// hold makes the file this process's to move aside once it's full: Bench
// calls it once it knows that it's the only Bench running. A second Bench,
// which only brings the first forward and exits, mustn't move the file from
// under the first, which would then write to the file moved aside. It opens
// the file again, so as to count its size as it is now: the Bench that ran
// before may have written to it, or moved it aside, meanwhile.
func (l *logFile) hold() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		l.f.Close()
		l.f = nil
	}
	l.held = l.openLocked() == nil
}

// Write adds p to the file, first moving the file aside if p would take it
// past logFileLimit and the file is held. It never fails, so that a log
// file that can't be written doesn't stop the log reaching standard error:
// what it can't write is dropped.
func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil && l.held && l.size > 0 && l.size+int64(len(p)) > logFileLimit {
		l.f.Close()
		l.f = nil
		_ = os.Rename(l.path, l.path+".1")
		_ = l.openLocked()
	}
	if l.f != nil {
		n, _ := l.f.Write(p)
		l.size += int64(n)
	}
	return len(p), nil
}
