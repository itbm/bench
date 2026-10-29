package workspace

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
)

// persistent reports whether the engine saves state.json: not for bench
// arrange; not while config.toml has problems, so that the arrangement made
// for the user's own config isn't overwritten, unless the model follows a
// config that loaded without them (followsGood); and not while config.toml
// hasn't been saved with a change to the tabs, because state.json must name
// the same tabs (stateHeld).
func (e *Engine) persistent() bool {
	return !e.opts.Headless && !e.stateHeld && (e.writable || e.followsGood)
}

// scheduleSaveLocked saves state.json saveDelay after the last change, but
// no later than saveMaxDelay after the first change that's waiting, so that a
// stream of changes can't put the save off until Bench quits.
func (e *Engine) scheduleSaveLocked() {
	if !e.persistent() || e.closed {
		return
	}
	now := e.clk.Now()
	if due := now.Add(saveMaxDelay); e.saveTimer == nil || due.Before(e.saveDue) {
		e.saveDue = due
	}
	e.startSaveTimerLocked(min(saveDelay, max(e.saveDue.Sub(now), 0)))
}

// scheduleLazySaveLocked saves state.json retitleSaveDelay from now, unless a
// save is already waiting, which it leaves as it is.
func (e *Engine) scheduleLazySaveLocked() {
	if !e.persistent() || e.closed || e.saveTimer != nil {
		return
	}
	e.saveDue = e.clk.Now().Add(retitleSaveDelay)
	e.startSaveTimerLocked(retitleSaveDelay)
}

// startSaveTimerLocked replaces the pending save with one after d.
func (e *Engine) startSaveTimerLocked(d time.Duration) {
	e.saveGen++
	gen := e.saveGen
	if e.saveTimer != nil {
		e.saveTimer.Stop()
	}
	e.saveTimer = e.clk.AfterFunc(d, func() { e.saveNow(gen) })
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
	e.stateSavedLocked(err)
}

// stateSavedLocked shows a state.json save's error in the status bar, or
// clears the last one.
func (e *Engine) stateSavedLocked(err error) {
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
	st.Apps = e.appStatesLocked()
	st.Ran = e.clk.Now()
	st.Screen = e.lastScreen
	st.Dialogs = maps.Clone(e.dialogs)
	for _, c := range e.closedTabs {
		st.Closed = append(st.Closed, *c)
	}
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

// saveConfig writes config.toml, if it can: not while the file has
// problems, and not if it has changed since Bench last read or wrote it, as
// when the user has edited it while Bench runs, which Bench's own copy would
// undo. Then a notice offers to reload the file or overwrite it. It returns
// why it didn't save, for the toast, or "". The caller holds cfgMu.
func (e *Engine) saveConfig(cfg *config.Config, writable bool) (string, error) {
	if !writable {
		return notSavedProblems, nil
	}
	data, err := config.SaveIf(e.cfgPath, cfg, e.cfgSum)
	if errors.Is(err, config.ErrChanged) {
		e.lock()
		defer e.unlock()
		e.outside = outsideKept
		e.touch()
		return notSavedChanged, nil
	}
	if err != nil {
		return "", fmt.Errorf("Bench couldn't save config.toml: %w", err)
	}
	e.cfgSum = config.SumOf(data)
	e.keepGood(data)
	return "", nil
}

// keepGood keeps config.toml's content, which has loaded without problems
// or which Bench has just saved, as the last good config, for Bench to run
// with if the file has problems later.
func (e *Engine) keepGood(data []byte) {
	if e.opts.Headless {
		return
	}
	if err := config.KeepGood(e.goodPath(), data); err != nil {
		e.log.Warn("couldn't keep a copy of config.toml", "error", err)
	}
}

// goodPath is where the last good config is kept.
func (e *Engine) goodPath() string { return filepath.Join(e.dir, config.GoodFile) }

// markFirstRunDone creates the file that says the permissions guide has
// been dismissed.
func (e *Engine) markFirstRunDone() error {
	path := filepath.Join(e.dir, firstRunFile)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return fmt.Errorf("Bench couldn't remember that the guide is closed: %w", err)
	}
	return nil
}
