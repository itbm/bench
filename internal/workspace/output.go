package workspace

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/term"
)

// This file keeps terminals' output between launches, while the
// keep_terminal_output setting is on. As Bench quits, it writes each running
// terminal's recent output to terms/<item ID>.out in the support folder,
// readable only by the user, without the escape sequences that would ask
// the terminal something or change its modes (term.Restorable). When
// state.json brings the terminal back, its session starts with that output,
// drawn faint, and a line that says when it's from. The file goes when the
// terminal is closed, or the setting is turned off.

// outputExt is the extension of a terminal's saved output.
const outputExt = ".out"

// outputDir returns the folder that holds terminals' saved output.
func (e *Engine) outputDir() string { return filepath.Join(e.dir, config.TermsDir) }

// outputPath returns where a terminal's output is saved.
func (e *Engine) outputPath(id string) string {
	return filepath.Join(e.outputDir(), id+outputExt)
}

// outputPlan is what Close does with terminals' output: it saves the output
// of the sessions running, keeps the files of the terminals state.json
// brought back whose sessions haven't started, and removes the rest. With
// forget set, as when keep_terminal_output is off, it removes them all.
type outputPlan struct {
	forget bool
	save   []string
	keep   map[string]bool
}

// outputPlanLocked works out what Close does with terminals' output, or
// returns nil to leave it alone: while state.json isn't saved, the items it
// has aren't the model's, and their output isn't the model's terminals'.
func (e *Engine) outputPlanLocked(saving bool) *outputPlan {
	if e.opts.Headless {
		return nil
	}
	if !e.cfg.Settings.KeepTerminalOutput {
		return &outputPlan{forget: true}
	}
	if !saving {
		return nil
	}
	plan := &outputPlan{keep: map[string]bool{}}
	for _, it := range e.model.orderedItems() {
		switch {
		case it.Kind != KindBuiltin:
		case e.termOn[it.ID]:
			plan.save = append(plan.save, it.ID)
			plan.keep[it.ID] = true
		case it.savedOutput:
			plan.keep[it.ID] = true
		}
	}
	return plan
}

// saveOutput carries out an output plan, before the sessions are closed.
// It takes no lock: the terminal host's methods are safe without it.
func (e *Engine) saveOutput(plan *outputPlan) error {
	if plan == nil {
		return nil
	}
	dir := e.outputDir()
	if plan.forget {
		return os.RemoveAll(dir)
	}
	var errs []error
	for _, id := range plan.save {
		out := term.Restorable(e.terms.Snapshot(id).Data, term.SnapshotLimit)
		if err := writeOutput(dir, id, out, e.clk.Now()); err != nil {
			errs = append(errs, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), outputExt)
		if !ok || !plan.keep[id] {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	if len(errs) > 0 {
		return errors.New("Bench couldn't keep the terminals' output: " + errors.Join(errs...).Error())
	}
	return nil
}

// writeOutput saves a terminal's output in dir, which it creates if need
// be, readable only by the user, marked as written at now. Output that's
// empty removes the file instead.
func writeOutput(dir, id string, out []byte, now time.Time) error {
	path := filepath.Join(dir, id+outputExt)
	if len(out) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, id+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(out)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chtimes(path, now, now)
}

// savedOutputLocked returns what a terminal that state.json brought back
// shows before its session's own output, the first time it starts: the
// output Bench kept from when it last quit, faint, then a line that says
// when that was. It returns nil if there's none, or the setting is off.
func (e *Engine) savedOutputLocked(it *Item) []byte {
	if !it.savedOutput {
		return nil
	}
	it.savedOutput = false
	if !e.cfg.Settings.KeepTerminalOutput {
		return nil
	}
	path := e.outputPath(it.ID)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*term.SnapshotLimit {
		return nil
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return restoredPreface(saved, info.ModTime(), e.clk.Now())
}

// restoredPreface returns saved output as a terminal shows it again: faint,
// ending on a line of its own, followed by a line that says when it's from,
// at, and that it's only a record. It's made safe to draw again first, in
// case the file has changed since Bench wrote it.
func restoredPreface(saved []byte, at, now time.Time) []byte {
	out := term.Restorable(saved, term.SnapshotLimit)
	var b bytes.Buffer
	if len(out) > 0 {
		b.Write(term.Faint(out))
		if !bytes.HasSuffix(out, []byte("\n")) {
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\x1b[2;3m" + restoredText(at, now) + "\x1b[0m\r\n")
	return b.Bytes()
}

// restoredText says when a terminal's output from before Bench last quit is
// from: "Restored output from 14:02. The program that wrote it has ended.",
// with the day too, if it's not today.
func restoredText(at, now time.Time) string {
	at, now = at.Local(), now.Local()
	when := at.Format("15:04")
	if y, m, d := at.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
		when += " on " + at.Format("2 January")
	}
	return "Restored output from " + when + ". The program that wrote it has ended."
}

// forgetOutputLocked removes a terminal's saved output, as it's closed.
func (e *Engine) forgetOutputLocked(id string) {
	if err := os.Remove(e.outputPath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.log.Warn("couldn't remove a terminal's saved output", "item", id, "error", err)
	}
}

// forgetAllOutputLocked removes every terminal's saved output, as
// keep_terminal_output is turned off.
func (e *Engine) forgetAllOutputLocked() {
	if e.opts.Headless {
		return
	}
	if err := os.RemoveAll(e.outputDir()); err != nil {
		e.log.Warn("couldn't remove the terminals' saved output", "error", err)
	}
}
