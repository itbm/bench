//go:build darwin || server

package ui

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/workspace"
)

// TestLogFile checks the log file: made with its folder, readable only by
// the user, added to from one launch to the next, and moved aside once it
// would grow past its limit, but only once it's held.
func TestLogFile(t *testing.T) {
	saved := logFileLimit
	logFileLimit = 100
	t.Cleanup(func() { logFileLimit = saved })
	dir := filepath.Join(t.TempDir(), "Logs", "Bench")
	path := filepath.Join(dir, "bench.log")

	l, err := openLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l.hold()
	line := strings.Repeat("a", 39) + "\n"
	for range 2 {
		if n, err := l.Write([]byte(line)); n != len(line) || err != nil {
			t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(line))
		}
	}
	for _, p := range []string{dir, path} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s has mode %v; want it private to the user", p, perm)
		}
	}

	// Opened again, as by a second Bench that only brings the first
	// forward, it's added to, past the limit, without being moved aside.
	second, err := openLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err == nil {
		t.Fatal("a log file that isn't held was moved aside")
	}
	if now, _ := os.ReadFile(path); string(now) != line+line+line {
		t.Errorf("bench.log = %q, want three lines", now)
	}

	// Opened again and held, as by the next launch, it's moved aside and
	// starts again once it would grow past the limit.
	l, err = openLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l.hold()
	if _, err := l.Write([]byte(strings.Repeat("b", 39) + "\n")); err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(path + ".1")
	now, _ := os.ReadFile(path)
	if string(old) != line+line+line || string(now) != strings.Repeat("b", 39)+"\n" {
		t.Errorf("after the limit: bench.log.1 = %q, bench.log = %q; want the first three lines, then the fourth", old, now)
	}

	// Held, it counts the file as it is then, so that what was written
	// meanwhile counts too.
	l, err = openLogFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	l.hold()
	if _, err := l.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if old, _ := os.ReadFile(path + ".1"); string(old) != line+line {
		t.Errorf("bench.log.1 = %q; want the two lines written before hold", old)
	}

	// A file made readable by others is made private again.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openLogFile(path); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("bench.log has mode %v after opening; want 0600", info.Mode().Perm())
	}
}

// TestLogStartsWithRun checks that only a Bench that runs says in the log
// file that it started: a second Bench, which Wails ends before run once it
// has brought the first forward, adds Wails' messages but no launch.
func TestLogStartsWithRun(t *testing.T) {
	saved := logOutput
	t.Cleanup(func() { logOutput = saved })
	logOutput = &bytes.Buffer{}
	path := filepath.Join(t.TempDir(), "bench.log")
	r := &runner{opts: Options{LogFile: path, Version: "1.2.3"}}
	r.keepLog()
	if _, err := io.WriteString(logOutput, "from Wails\n"); err != nil {
		t.Fatal(err)
	}
	if kept, _ := os.ReadFile(path); string(kept) != "from Wails\n" {
		t.Errorf("before run, bench.log = %q; want only Wails' message", kept)
	}
	r.holdLog()
	if kept, _ := os.ReadFile(path); !strings.Contains(string(kept), `msg="Bench started" version=1.2.3`) {
		t.Errorf("once Bench runs, bench.log = %q; want the launch", kept)
	}
}

// TestNoticesLogged checks that each notice is logged once, as it first
// appears, and again if it comes back.
func TestNoticesLogged(t *testing.T) {
	var buf bytes.Buffer
	em := newEmitter(&fakeSink{}, &fakeShell{})
	em.log = slog.New(slog.NewTextHandler(&buf, nil))
	notice := func(texts ...string) workspace.View {
		var v workspace.View
		for _, text := range texts {
			v.Notices = append(v.Notices, workspace.NoticeView{Text: text})
		}
		return v
	}
	em.State(notice("Accessibility is off."))
	em.State(notice("Accessibility is off.", "config.toml has problems."))
	em.State(notice())
	em.State(notice("Accessibility is off."))
	if got := strings.Count(buf.String(), "Accessibility is off."); got != 2 {
		t.Errorf("a notice that came back was logged %d times, want 2:\n%s", got, buf.String())
	}
	if got := strings.Count(buf.String(), "config.toml has problems."); got != 1 || !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("a new notice was logged %d times, want once as a warning:\n%s", got, buf.String())
	}
}
