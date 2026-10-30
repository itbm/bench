package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/term"
)

// restartTerms restarts Bench with a new terminal host, as the next run
// has, and returns the old one.
func (h *harness) restartTerms() *fakeTerms {
	h.t.Helper()
	old := h.terms
	h.terms = newFakeTerms()
	h.restart()
	return old
}

// outputFile returns the path of a terminal's saved output.
func (h *harness) outputFile(id string) string {
	return filepath.Join(h.dir, config.TermsDir, id+".out")
}

func TestPlainShellStartsAgainWhereItWasLast(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	if opts := h.terms.started["w5"]; opts.Dir != "~/code" || opts.Fallback != "" {
		t.Fatalf("the first start: %+v", opts)
	}
	h.terms.cb.Folder("w5", "~/code/bench")
	h.advance(2 * frame)
	if it := h.item("w5"); it.Folder != "~/code" || it.Here != "~/code/bench" {
		t.Errorf("after cd: folder %q, here %q", it.Folder, it.Here)
	}

	// The add dialog offers a new terminal there, first.
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	for _, o := range opts {
		if o.Group == groupBuiltin {
			if o.Label != "New terminal in ~/code/bench" || o.Source != (Source{Type: sourceTerminal, Folder: "~/code/bench"}) {
				t.Errorf("the first Built in row: %+v", o)
			}
			break
		}
	}

	// After a restart, its shell starts there, or in its own folder if
	// that one has gone; the config entry still matches it by its own.
	h.restartTerms()
	if opts := h.terms.started["w5"]; opts.Dir != "~/code/bench" || opts.Fallback != "~/code" {
		t.Errorf("after a restart: %+v", opts)
	}
	if it := h.item("w5"); it.Folder != "~/code" || it.Here != "~/code/bench" {
		t.Errorf("after a restart: folder %q, here %q", it.Folder, it.Here)
	}
	st, _, err := config.PeekState(filepath.Join(h.dir, config.StateFile))
	mustOK(t, err)
	if s := st.Items["w5"]; s.Folder != "~/code" || s.LastFolder != "~/code/bench" {
		t.Errorf("state.json: folder %q, last folder %q", s.Folder, s.LastFolder)
	}
}

func TestCommandTerminalStartsAgainInItsOwnFolder(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	h.terms.cb.Folder(id, "/tmp")
	h.advance(2 * frame)
	if got := h.item(id).Here; got != "/tmp" {
		t.Errorf("here: %q", got)
	}
	h.restartTerms()
	if opts := h.terms.started[id]; opts.Dir != "~/code/bench" || opts.Fallback != "" {
		t.Errorf("after a restart: %+v", opts)
	}
	if got := h.item(id).Here; got != "~/code/bench" {
		t.Errorf("here after a restart: %q", got)
	}
	st, _, err := config.PeekState(filepath.Join(h.dir, config.StateFile))
	mustOK(t, err)
	if s := st.Items[id]; s.LastFolder != "" {
		t.Errorf("state.json keeps a last folder for a command: %q", s.LastFolder)
	}
}

func TestTerminalOutputIsKeptBetweenLaunches(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// Its output asks the terminal where its cursor is, and sets a title,
	// which isn't kept.
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("\x1b]0;zsh\x07$ ls\r\nnotes.md\r\n\x1b[6n$ "), Offset: 40}
	// The output of a terminal that's gone isn't kept.
	mustOK(t, os.MkdirAll(filepath.Join(h.dir, config.TermsDir), 0o700))
	mustOK(t, os.WriteFile(h.outputFile("w99"), []byte("old"), 0o600))
	saved := h.clk.Now()
	old := h.restartTerms()
	if old.lateSnaps != 0 {
		t.Errorf("%d snapshots were taken after the sessions closed", old.lateSnaps)
	}
	data, err := os.ReadFile(h.outputFile("w5"))
	mustOK(t, err)
	if string(data) != "$ ls\r\nnotes.md\r\n$ " {
		t.Errorf("saved output: %q", data)
	}
	if info, err := os.Stat(h.outputFile("w5")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file's permissions: %v, %v", info.Mode(), err)
	}
	if _, err := os.Stat(h.outputFile("w99")); !os.IsNotExist(err) {
		t.Errorf("the gone terminal's output is still there: %v", err)
	}

	// The terminal starts with it, faint, and a line that says when it's
	// from.
	preface := string(h.terms.started["w5"].Preface)
	if want := string(restoredPreface(data, saved, h.clk.Now())); preface != want {
		t.Errorf("preface: %q, want %q", preface, want)
	}
	note := "Restored output from " + saved.Local().Format("15:04") + ". The program that wrote it has ended."
	if !strings.Contains(preface, "notes.md") || !strings.Contains(preface, note) {
		t.Errorf("preface %q doesn't have the output and %q", preface, note)
	}

	// Closing the terminal forgets its output.
	_, err = h.e.CloseItem("w5", true, false)
	mustOK(t, err)
	if _, err := os.Stat(h.outputFile("w5")); !os.IsNotExist(err) {
		t.Errorf("the closed terminal's output is still there: %v", err)
	}
}

func TestClearedTerminalOutputIsntKept(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("$ export TOKEN=secret\r\n$ "), Offset: 27}
	h.restartTerms()
	if _, err := os.Stat(h.outputFile("w5")); err != nil {
		t.Fatalf("the output wasn't kept: %v", err)
	}

	// Clear forgets the output, so that a view drawn later doesn't show
	// it, and Bench no longer keeps it.
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("$ export TOKEN=secret\r\n$ "), Offset: 27}
	mustOK(t, h.e.TermClear("w5"))
	if snap, err := h.e.TermSnapshot("w5"); err != nil || snap.Data != "" || snap.Offset != 27 {
		t.Errorf("TermSnapshot after Clear: %+v, %v", snap, err)
	}
	h.restartTerms()
	if _, err := os.Stat(h.outputFile("w5")); !os.IsNotExist(err) {
		t.Errorf("the cleared output is still kept: %v", err)
	}
	if preface := h.terms.started["w5"].Preface; preface != nil {
		t.Errorf("preface after Clear: %q", preface)
	}
}

func TestTerminalOutputWaitsForItsTerminalToStart(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("$ make\r\nok\r\n")}
	mustOK(t, h.e.SwitchTab(0))
	h.restartTerms()
	// Its tab isn't shown, so its terminal hasn't started: its output stays
	// for when it does, through another restart.
	if _, ok := h.terms.started["w5"]; ok {
		t.Fatal("the terminal started in a tab that isn't shown")
	}
	h.restartTerms()
	mustOK(t, h.e.SwitchTab(1))
	if preface := h.terms.started["w5"].Preface; !bytes.Contains(preface, []byte("ok\r\n")) {
		t.Errorf("preface: %q", preface)
	}
}

func TestCommandRunAgainDoesntShowItsOldOutputAgain(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := ""
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	h.terms.snaps[id] = term.Snapshot{Data: []byte("PASS\r\n")}
	h.restartTerms()
	if preface := h.terms.started[id].Preface; !bytes.Contains(preface, []byte("PASS")) {
		t.Fatalf("preface: %q", preface)
	}
	h.terms.end(id)
	h.terms.cb.Exit(id, nil)
	h.advance(2 * frame)
	mustOK(t, h.e.TermInput(id, "\r"))
	if preface := h.terms.started[id].Preface; preface != nil {
		t.Errorf("running it again showed the old output again: %q", preface)
	}
}

func TestTurningKeepTerminalOutputOffForgetsIt(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("secret\r\n")}
	h.restartTerms()
	if _, err := os.Stat(h.outputFile("w5")); err != nil {
		t.Fatalf("the output wasn't kept: %v", err)
	}
	v := h.view().Settings
	if !v.KeepTerminalOutput {
		t.Fatal("keep_terminal_output should be on by default")
	}
	v.KeepTerminalOutput = false
	mustOK(t, h.e.SaveSettings(v))
	if _, err := os.Stat(filepath.Join(h.dir, config.TermsDir)); !os.IsNotExist(err) {
		t.Errorf("the output is still there: %v", err)
	}
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("more secrets\r\n")}
	h.restartTerms()
	if _, err := os.Stat(filepath.Join(h.dir, config.TermsDir)); !os.IsNotExist(err) {
		t.Errorf("output was kept with the setting off: %v", err)
	}
	if preface := h.terms.started["w5"].Preface; preface != nil {
		t.Errorf("preface with the setting off: %q", preface)
	}
}

func TestNewTerminalDoesntShowSavedOutput(t *testing.T) {
	h := newHarness(t, setup{})
	// A file left from another arrangement, under the ID the next item
	// gets, as after state.json was lost.
	h.e.lock()
	next := itemPrefix + strconv.Itoa(h.e.model.Seq+1)
	h.e.unlock()
	mustOK(t, os.MkdirAll(filepath.Join(h.dir, config.TermsDir), 0o700))
	mustOK(t, os.WriteFile(h.outputFile(next), []byte("someone else's\r\n"), 0o600))
	mustOK(t, h.e.AddToPane(h.view().Display.Panes[0].ID, Source{Type: sourceTerminal}))
	if _, ok := h.terms.started[next]; !ok {
		t.Fatalf("the new terminal isn't %s: %v", next, h.terms.started)
	}
	if preface := h.terms.started[next].Preface; preface != nil {
		t.Errorf("a new terminal showed saved output: %q", preface)
	}
}

func TestRestoredText(t *testing.T) {
	now := mustTime(t, "2026-10-17T15:30:00")
	tests := []struct{ at, want string }{
		{"2026-10-17T14:02:00", "Restored output from 14:02. The program that wrote it has ended."},
		{"2026-10-16T23:59:00", "Restored output from 23:59 on 16 October. The program that wrote it has ended."},
	}
	for _, tt := range tests {
		if got := restoredText(mustTime(t, tt.at), now); got != tt.want {
			t.Errorf("restoredText(%s) = %q, want %q", tt.at, got, tt.want)
		}
	}
}

func TestRestoredPreface(t *testing.T) {
	at := mustTime(t, "2026-10-17T14:02:00")
	note := "\x1b[2;3mRestored output from 14:02. The program that wrote it has ended.\x1b[0m\r\n"
	tests := []struct{ saved, want string }{
		{"", note},
		{"done\r\n", "\x1b[0;2mdone\r\n\x1b[0m\x1b(B\x0f" + note},
		{"$ ", "\x1b[0;2m$ \x1b[0m\x1b(B\x0f\r\n" + note},
		// A file that has changed since Bench wrote it is made safe too.
		{"a\x1b[6nb\r\n", "\x1b[0;2mab\r\n\x1b[0m\x1b(B\x0f" + note},
	}
	for _, tt := range tests {
		if got := string(restoredPreface([]byte(tt.saved), at, at)); got != tt.want {
			t.Errorf("restoredPreface(%q) = %q, want %q", tt.saved, got, tt.want)
		}
	}
}

// mustTime parses a local time, such as "2026-10-17T14:02:00".
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	return at
}
