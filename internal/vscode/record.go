package vscode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// record is what Start writes to Options.Record: enough to stop the server
// and remove its token if Bench crashes, without ever signalling a process
// that isn't the server, or stopping a server that a Bench still uses.
type record struct {
	// PID is the process Start ran, which leads the server's process
	// group.
	PID int `json:"pid"`
	// Started is when that process started, as the kernel records it, so
	// that a later process given the same ID isn't taken for it. It's
	// empty where Bench can't find out, and then StopRecorded signals
	// nothing.
	Started string `json:"started,omitempty"`
	// Owner is the process that called Start (Bench), and OwnerStarted
	// when it started. StopRecorded leaves the server alone while Owner
	// runs.
	Owner        int    `json:"owner,omitempty"`
	OwnerStarted string `json:"ownerStarted,omitempty"`
	// TokenFile is the connection token's file.
	TokenFile string `json:"tokenFile,omitempty"`
	// TempDir is the folder Start made for the token file, if it made one.
	TempDir string `json:"tempDir,omitempty"`
}

// process is what the kernel says about a process.
type process struct {
	// started is when it started, to tell it from a later process given
	// the same ID.
	started string
	// ended is true if it's ending or has ended, though its parent hasn't
	// collected it yet.
	ended bool
}

// processStarted returns when process pid started, and false if there's no
// such process.
func processStarted(pid int) (string, bool) {
	p, ok := lookUpProcess(pid)
	return p.started, ok
}

// writeRecord records the server in path, readable only by the user, as
// the token file is.
func (s *Server) writeRecord(path string) error {
	pid := s.cmd.Process.Pid
	started, _ := processStarted(pid)
	owner := os.Getpid()
	ownerStarted, _ := processStarted(owner)
	data, err := json.Marshal(record{
		PID: pid, Started: started, Owner: owner, OwnerStarted: ownerStarted,
		TokenFile: s.tokenFile, TempDir: s.tempDir,
	})
	if err != nil {
		return fmt.Errorf("record the server: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("record the server: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("record the server: %w", err), os.Remove(tmp))
	}
	s.record = path
	return nil
}

// removeRecord removes the server's record, if it still names this server.
func (s *Server) removeRecord() error {
	if s.record == "" {
		return nil
	}
	return removeRecordOf(s.record, s.cmd.Process.Pid)
}

// readRecord reads a record.
func readRecord(path string) (record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}, err
	}
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		return record{}, fmt.Errorf("%s isn't a record of a server: %w", path, err)
	}
	return rec, nil
}

// StopRecorded stops the server that the record at path names, which a
// Bench that crashed left running, then removes the server's token file and
// the record. A missing record is fine: a server that Stop stopped has
// none. So is a server whose Bench is still running, which it leaves alone.
//
// It signals the server's process group, SIGTERM and then, after 3 seconds,
// SIGKILL, only if the process Start ran is still there and started when the
// record says. Otherwise the server has gone, and its process ID may now be
// another process's.
func StopRecorded(path string) error {
	rec, err := readRecord(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.Join(fmt.Errorf("stop VS Code for the Web: %w", err), removeRecordFile(path))
	}
	if rec.ownerRunning() {
		return nil
	}
	if rec.running() {
		endGroup(rec.PID)
	}
	return errors.Join(rec.removeToken(), removeRecordOf(path, rec.PID))
}

// removeRecordOf removes the record at path if it still names process pid,
// and not a server started meanwhile.
func removeRecordOf(path string, pid int) error {
	if rec, err := readRecord(path); err != nil || rec.PID != pid {
		return nil // Already gone, or another server's.
	}
	return removeRecordFile(path)
}

// removeRecordFile removes a record.
func removeRecordFile(path string) error {
	if err := removeFile(path); err != nil {
		return fmt.Errorf("remove the VS Code for the Web server's record: %w", err)
	}
	return nil
}

// running reports whether the process that leads the server's group is
// still there, if only as a zombie, whose group may still have members.
func (rec record) running() bool {
	if rec.Started == "" {
		return false
	}
	p, ok := lookUpProcess(rec.PID)
	return ok && p.started == rec.Started
}

// ownerRunning reports whether the Bench that started the server is still
// running, and not ending: a watchdog wakes as its Bench ends.
func (rec record) ownerRunning() bool {
	if rec.OwnerStarted == "" {
		return false
	}
	p, ok := lookUpProcess(rec.Owner)
	return ok && !p.ended && p.started == rec.OwnerStarted
}

// endGroup ends the process group led by pid: SIGTERM, then SIGKILL to
// whatever is left after stopGrace.
func endGroup(pid int) {
	signalGroup(pid, false)
	if waitGroup(pid, stopGrace) {
		return
	}
	signalGroup(pid, true)
	waitGroup(pid, killWait)
}

// waitGroup waits up to limit for the process group led by pid to empty,
// and reports whether it has.
func waitGroup(pid int, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for groupAlive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(groupPoll)
	}
	return true
}

// removeToken removes the token file and the folder that the record names,
// if they're what Start makes, so that a damaged record can't remove
// anything else: a file whose name is a token file's, and an empty folder
// whose name is a token folder's, in the temporary folder, that held the
// token file.
func (rec record) removeToken() error {
	var errs []error
	if rec.TokenFile != "" && strings.HasPrefix(filepath.Base(rec.TokenFile), tokenPrefix) {
		if err := removeFile(rec.TokenFile); err != nil {
			errs = append(errs, fmt.Errorf("remove the connection token file: %w", err))
		}
	}
	if rec.ownTempDir() {
		// Not RemoveAll: the folder held only the token file, so anything
		// else in it isn't Bench's.
		if err := removeFile(rec.TempDir); err != nil {
			errs = append(errs, fmt.Errorf("remove the connection token folder: %w", err))
		}
	}
	return errors.Join(errs...)
}

// ownTempDir reports whether the record's TempDir is a folder Start could
// have made for its token file: named as Start names it, in the temporary
// folder, and holding the token file.
func (rec record) ownTempDir() bool {
	if rec.TempDir == "" || rec.TokenFile == "" {
		return false
	}
	dir := filepath.Clean(rec.TempDir)
	return strings.HasPrefix(filepath.Base(dir), tempDirPrefix) &&
		filepath.Dir(dir) == filepath.Clean(os.TempDir()) &&
		filepath.Dir(filepath.Clean(rec.TokenFile)) == dir
}

// removeFile removes a file that may already have gone.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
