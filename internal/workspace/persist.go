package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itbm/bench/internal/config"
)

// persistent reports whether the engine saves state.json: not for bench
// arrange, and not while config.toml has problems, so that the arrangement
// made for the user's own config isn't overwritten.
func (e *Engine) persistent() bool { return !e.opts.Headless && e.writable }

// scheduleSaveLocked saves state.json saveDelay after the last change.
func (e *Engine) scheduleSaveLocked() {
	if !e.persistent() || e.closed {
		return
	}
	e.saveGen++
	gen := e.saveGen
	if e.saveTimer != nil {
		e.saveTimer.Stop()
	}
	e.saveTimer = e.clk.AfterFunc(saveDelay, func() { e.saveNow(gen) })
}

// saveNow saves state.json, unless a later change has asked for a later
// save. The file is written without the lock held.
func (e *Engine) saveNow(gen int) {
	e.lock()
	if e.closed || gen != e.saveGen {
		e.unlock()
		return
	}
	e.saveTimer = nil
	st, seq := e.snapshotLocked()
	e.unlock()

	err := e.writeState(st, seq)

	e.lock()
	defer e.unlock()
	if e.closed {
		return
	}
	var notices []string
	if err != nil {
		notices = []string{err.Error()}
	}
	if len(notices) > 0 || len(e.notices[sourceState]) > 0 {
		e.setNoticesLocked(sourceState, notices)
		e.touch()
	}
}

// snapshotLocked returns the model as state.json, numbered so that writes
// can tell an older snapshot from a newer one.
func (e *Engine) snapshotLocked() (*config.State, int) {
	e.stateSeq++
	st := e.model.State()
	for i := range st.Tabs {
		if i < len(e.cfg.Tabs) && strings.EqualFold(e.cfg.Tabs[i].Name, st.Tabs[i].Name) {
			st.Tabs[i].Entries = entryKeys(e.cfg.Tabs[i])
		}
	}
	return st, e.stateSeq
}

// writeState writes a snapshot to state.json, unless a newer one has been
// written already (a save timer's write can finish after Close's).
func (e *Engine) writeState(st *config.State, seq int) error {
	e.saveMu.Lock()
	defer e.saveMu.Unlock()
	if seq <= e.writtenSeq {
		return nil
	}
	if err := config.SaveState(e.statePath, st); err != nil {
		return fmt.Errorf("Bench couldn't save the arrangement: %w", err)
	}
	e.writtenSeq = seq
	return nil
}

// saveConfig writes config.toml, if it can be written.
func (e *Engine) saveConfig(cfg *config.Config, writable bool) error {
	if !writable {
		return nil
	}
	if err := config.Save(e.cfgPath, cfg); err != nil {
		return fmt.Errorf("Bench couldn't save config.toml: %w", err)
	}
	return nil
}

// markFirstRunDone creates the file that says the permissions guide has
// been dismissed.
func (e *Engine) markFirstRunDone() error {
	path := filepath.Join(e.dir, firstRunFile)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return fmt.Errorf("Bench couldn't remember that the guide is closed: %w", err)
	}
	return nil
}
