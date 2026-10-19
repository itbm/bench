//go:build darwin || linux

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// wait is how long tests wait for a shell to do something.
const wait = 10 * time.Second

// newTestManager returns a manager that checks titles often, and closes its
// sessions when the test ends.
func newTestManager(t *testing.T) (*Manager, *recorder) {
	t.Helper()
	if !isExecutable("/bin/sh") {
		t.Skip("/bin/sh isn't available")
	}
	r := newRecorder()
	m := NewManager(r.callbacks())
	m.titleEvery = 20 * time.Millisecond
	t.Cleanup(m.CloseAll)
	return m, r
}

// shOptions runs /bin/sh in a folder of its own.
func shOptions(t *testing.T) Options {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return Options{Shell: "/bin/sh", Dir: dir}
}

// shName is the process name /bin/sh runs as: "sh" on Linux, but on macOS
// /bin/sh starts another shell (usually bash) in its place, and that's what a
// title shows.
func shName(t *testing.T) string {
	t.Helper()
	// A compound command keeps the shell running, rather than letting it
	// exec sleep in its place.
	cmd := exec.Command("/bin/sh", "-c", "sleep 5; exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting /bin/sh: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	// Give macOS's /bin/sh time to start the real shell in its place.
	time.Sleep(300 * time.Millisecond)
	name, ok := processName(cmd.Process.Pid)
	if !ok || name == "" {
		t.Fatal("couldn't find the name of /bin/sh's process")
	}
	return name
}

func startSh(t *testing.T, m *Manager, id string, opts Options) {
	t.Helper()
	if err := m.Start(id, opts); err != nil {
		t.Fatalf("Start(%q) = %v", id, err)
	}
}

func write(t *testing.T, m *Manager, id, s string) {
	t.Helper()
	if err := m.Write(id, []byte(s)); err != nil {
		t.Fatalf("Write(%q, %q) = %v", id, s, err)
	}
}

func waitExit(t *testing.T, r *recorder, id string) error {
	t.Helper()
	waitFor(t, wait, "Exit for "+id, func() bool { return r.exitCount(id) > 0 })
	return r.exitList(id)[0]
}

func TestManagerOutputAndInput(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	if !m.Running("t1") {
		t.Fatal("Running() = false just after Start")
	}
	// The shell works out 40+2, so the answer can't be the echo of
	// what's typed.
	write(t, m, "t1", "echo hello-$((40+2))\n")
	waitOutput(t, r, "t1", "hello-42", wait)
}

func TestManagerStartTwiceDoesNothing(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	startSh(t, m, "t1", opts)
	write(t, m, "t1", "MARK=kept\n")
	startSh(t, m, "t1", opts)
	write(t, m, "t1", "echo value-$MARK\n")
	waitOutput(t, r, "t1", "value-kept", wait)
}

func TestManagerEnvironment(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Env = []string{"BENCH_EXTRA=yes"}
	opts.Command = `printf '%s|%s|%s|%s|%s\n' "$TERM" "$COLORTERM" "$TERM_PROGRAM" "$BENCH_EXTRA" "$(pwd)"`
	startSh(t, m, "t1", opts)
	if err := waitExit(t, r, "t1"); err != nil {
		t.Fatalf("Exit error = %v, want nil", err)
	}
	want := "xterm-256color|truecolor|Bench|yes|" + opts.Dir
	if out := r.output("t1"); !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to contain %q", out, want)
	}
}

func TestManagerMissingFolder(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Dir = filepath.Join(opts.Dir, "missing")
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", "isn't a folder, so this terminal starts in", wait)
}

func TestManagerMissingFolderForCommand(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	ran := filepath.Join(opts.Dir, "ran")
	opts.Dir = filepath.Join(opts.Dir, "missing")
	opts.Command = "touch " + ran
	err := m.Start("t1", opts)
	if err == nil || !strings.Contains(err.Error(), "missing isn't a folder") {
		t.Fatalf("Start() = %v, want an error saying the folder is missing", err)
	}
	if m.Running("t1") {
		t.Error("Running() = true after Start failed")
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(ran); err == nil {
		t.Error("the command ran, though its folder is missing")
	}
	if n := r.exitCount("t1"); n != 0 {
		t.Errorf("Exit called %d times, want none", n)
	}
}

func TestManagerHangUpWhenBenchIgnoresIt(t *testing.T) {
	// Ignore SIGHUP, as nohup does.
	if !signal.Ignored(syscall.SIGHUP) {
		signal.Ignore(syscall.SIGHUP)
		t.Cleanup(func() {
			// Neither signal.Reset nor listening and then stopping
			// puts back the default action: either leaves SIGHUP
			// ignored, and later tests' shells would inherit that. So
			// the test binary listens for it until it ends, which
			// gives the shells the default action.
			signal.Notify(make(chan os.Signal, 1), syscall.SIGHUP)
		})
	}
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = "kill -HUP $$; echo still-running"
	startSh(t, m, "t1", opts)
	if got, want := m.Title("t1"), formatTitle("sh", abbreviate(opts.Dir, homeDir())); got != want {
		t.Errorf("Title() = %q at the start, want %q", got, want)
	}
	err := waitExit(t, r, "t1")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.Sys().(syscall.WaitStatus).Signal() != syscall.SIGHUP {
		t.Errorf("Exit error = %v, want the shell ended by SIGHUP", err)
	}
	if out := r.output("t1"); strings.Contains(out, "still-running") {
		t.Errorf("output = %q, want the shell ended before it printed", out)
	}
}

func TestManagerTitleIsCleaned(t *testing.T) {
	m, _ := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	// The folder's name has an escape sequence and a new line in it.
	write(t, m, "t1", `printf '\033]7;file://h/tmp/%%1b%%5b31mRED%%0aX\007'`+"\n")
	want := formatTitle(shName(t), "/tmp/[31mREDX")
	waitFor(t, wait, "title "+quote(want), func() bool { return m.Title("t1") == want })
}

func TestComputeTitleLimitsLength(t *testing.T) {
	s := &session{home: "/Users/me", startDir: "/Users/me/" + strings.Repeat("a", 2*maxTitle)}
	got := s.computeTitle("sh\x07", "")
	if n := utf8.RuneCountInString(got); n != maxTitle {
		t.Errorf("title has %d runes, want %d", n, maxTitle)
	}
	if want := "sh" + titleSeparator + "~/aaa"; !strings.HasPrefix(got, want) {
		t.Errorf("title = %q, want it to start %q", got, want)
	}
}

func TestManagerResize(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	if err := m.Resize("t1", 123, 45); err != nil {
		t.Fatalf("Resize() = %v", err)
	}
	write(t, m, "t1", "stty size\n")
	waitOutput(t, r, "t1", "45 123", wait)

	if err := m.Resize("t1", 0, 10); err == nil {
		t.Error("Resize(0, 10) = nil, want an error")
	}
	if err := m.Resize("nope", 80, 24); !errors.Is(err, ErrNoSession) {
		t.Errorf("Resize(unknown) = %v, want ErrNoSession", err)
	}
}

func TestManagerStartingSize(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Cols, opts.Rows = 100, 30
	opts.Command = "stty size"
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	if out := r.output("t1"); !strings.Contains(out, "30 100") {
		t.Errorf("output = %q, want the size 30 100", out)
	}
}

func TestManagerTitle(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	startSh(t, m, "t1", opts)
	folder := abbreviate(opts.Dir, homeDir())
	shellTitle := formatTitle(shName(t), folder)
	// Until the first check of the foreground process, the title uses the
	// configured shell's name.
	if got, first := m.Title("t1"), formatTitle("sh", folder); got != first && got != shellTitle {
		t.Errorf("Title() = %q at the start, want %q or %q", got, first, shellTitle)
	}

	write(t, m, "t1", `printf '\033]0;my title\007'`+"\n")
	waitFor(t, wait, "the title the shell set", func() bool { return m.Title("t1") == "my title" })
	waitFor(t, wait, "a Title callback", func() bool { return slices.Contains(r.titleList("t1"), "my title") })

	// An empty title goes back to the command and folder.
	write(t, m, "t1", `printf '\033]2;\033\\'`+"\n")
	waitFor(t, wait, "the title to go back to "+shellTitle, func() bool { return m.Title("t1") == shellTitle })
	waitFor(t, wait, "a Title callback", func() bool { return slices.Contains(r.titleList("t1"), shellTitle) })

	// Only the title the shell set is reported as the shell's.
	titles, byShell := r.titleList("t1"), r.byShellList("t1")
	for i, title := range titles {
		if want := title == "my title"; byShell[i] != want {
			t.Errorf("Title callback %q has byShell %t, want %t", title, byShell[i], want)
		}
	}
}

func TestManagerForegroundCommandTitle(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	startSh(t, m, "t1", opts)
	folder := abbreviate(opts.Dir, homeDir())

	write(t, m, "t1", "sleep 30\n")
	want := formatTitle("sleep", folder)
	waitFor(t, wait, "title "+quote(want), func() bool { return m.Title("t1") == want })
	waitFor(t, wait, "a Title callback", func() bool { return slices.Contains(r.titleList("t1"), want) })

	write(t, m, "t1", "\x03") // Control-C ends sleep.
	back := formatTitle(shName(t), folder)
	waitFor(t, wait, "title "+quote(back), func() bool { return m.Title("t1") == back })
}

func TestManagerFolderFollowsCd(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	sub := filepath.Join(opts.Dir, "sub folder")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	startSh(t, m, "t1", opts)
	// /bin/sh doesn't send OSC 7, so the folder comes from the process.
	write(t, m, "t1", "cd 'sub folder'\n")
	want := formatTitle(shName(t), abbreviate(sub, homeDir()))
	waitFor(t, wait, "title "+quote(want), func() bool { return m.Title("t1") == want })
	waitFor(t, wait, "a Title callback", func() bool { return slices.Contains(r.titleList("t1"), want) })
}

func TestManagerFolderFromOSC7(t *testing.T) {
	m, _ := newTestManager(t)
	opts := shOptions(t)
	startSh(t, m, "t1", opts)
	write(t, m, "t1", `printf '\033]7;file://host/somewhere/else\007'`+"\n")
	want := formatTitle(shName(t), "/somewhere/else")
	waitFor(t, wait, "title "+quote(want), func() bool { return m.Title("t1") == want })
}

func TestManagerSnapshotIsBounded(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = `head -c 300000 /dev/zero | tr '\0' x; echo; echo done-marker`
	startSh(t, m, "t1", opts)
	if err := waitExit(t, r, "t1"); err != nil {
		t.Fatalf("Exit error = %v, want nil", err)
	}
	if n := len(r.output("t1")); n < 300000 {
		t.Errorf("Data delivered %d bytes, want at least 300000", n)
	}
	snap := m.Snapshot("t1")
	if len(snap.Data) > SnapshotLimit {
		t.Errorf("len(Snapshot()) = %d, want at most %d", len(snap.Data), SnapshotLimit)
	}
	if !strings.HasSuffix(string(snap.Data), "done-marker\r\n") {
		t.Errorf("Snapshot() ends %q, want the latest output", tail(string(snap.Data), 40))
	}
	if end, ok := r.offset("t1"); snap.Offset != end || !ok {
		t.Errorf("Snapshot().Offset = %d, want %d, where the output has got to (offsets follow on: %t)", snap.Offset, end, ok)
	}
	if m.Running("t1") {
		t.Error("Running() = true after the command ended")
	}
	r.checkTidy(t, "t1")
}

func TestManagerDataSlicesAreFresh(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = "seq 1 30000"
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	// If the reader reused its buffer, earlier chunks would have been
	// overwritten, and the numbers wouldn't run in order. The terminal
	// driver turns each line feed into a carriage return and a line feed,
	// but macOS's can send the carriage return twice when its output queue
	// is full, as it can be when the reader falls behind, so lines are
	// compared without them.
	lines := strings.Split(r.output("t1"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}
	first := slices.Index(lines, "1")
	if first < 0 {
		t.Fatalf("output has no line \"1\": %q", tail(r.output("t1"), 80))
	}
	for i := 1; i <= 30000; i++ {
		if got := lines[first+i-1]; got != strconv.Itoa(i) {
			t.Fatalf("line %d = %q, want %d", i, got, i)
		}
	}
}

func TestManagerExitStatus(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = "exit 3"
	startSh(t, m, "t1", opts)
	err := waitExit(t, r, "t1")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Errorf("Exit error = %v, want exit status 3", err)
	}
	if err := m.Write("t1", []byte("x")); !errors.Is(err, ErrEnded) {
		t.Errorf("Write() after exit = %v, want ErrEnded", err)
	}
	if err := m.Resize("t1", 80, 24); err != nil {
		t.Errorf("Resize() after exit = %v, want nil", err)
	}
	time.Sleep(100 * time.Millisecond)
	r.checkTidy(t, "t1")
}

func TestManagerStartReplacesEnded(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = "exit 0"
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	opts.Command = "echo started-again"
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", "started-again", wait)
}

func TestManagerOffsetsCarryOn(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = "echo first-session"
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	end, _ := r.offset("t1")
	if end == 0 || m.Snapshot("t1").Offset != end {
		t.Fatalf("Snapshot().Offset = %d after the first session, want %d", m.Snapshot("t1").Offset, end)
	}

	// The session that replaces it numbers its output from where the
	// first left off, so a view that showed the first one's output
	// doesn't take the new output for what it has already drawn.
	opts.Command = "echo second-session"
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", "second-session", wait)
	if next, ok := r.offset("t1"); !ok || next <= end {
		t.Errorf("output ends at %d after the second session, and offsets follow on: %t; want past %d, following on", next, ok, end)
	}
	snap := m.Snapshot("t1")
	if next, _ := r.offset("t1"); snap.Offset != next || strings.Contains(string(snap.Data), "first-session") {
		t.Errorf("Snapshot() = %q ending at %d, want only the second session's output, ending at %d", snap.Data, snap.Offset, next)
	}
}

func TestManagerWaitsForViews(t *testing.T) {
	m, r := newTestManager(t)
	m.flowWait = time.Minute
	opts := shOptions(t)
	opts.Command = `head -c 1000000 /dev/zero | tr '\0' x; echo; echo done-marker`
	startSh(t, m, "t1", opts)

	// No view has drawn anything, so once the session is a window ahead,
	// it stops reading.
	waitFor(t, wait, "a window of output", func() bool { n, _ := r.offset("t1"); return n >= flowWindow })
	time.Sleep(200 * time.Millisecond)
	if n, _ := r.offset("t1"); n >= flowWindow+readSize {
		t.Fatalf("the session sent %d bytes that no view had drawn, want less than %d", n, flowWindow+readSize)
	}

	// Each time a view says it has drawn what it got, more follows,
	// until the end.
	waitFor(t, wait, "all the output", func() bool {
		n, _ := r.offset("t1")
		m.Ack("t1", n)
		return strings.HasSuffix(r.output("t1"), "done-marker\r\n")
	})
	if _, ok := r.offset("t1"); !ok {
		t.Error("offsets don't follow on from one chunk to the next")
	}
	if n := strings.Count(r.output("t1"), "x"); n != 1000000 {
		t.Errorf("the output has %d x's, want 1000000", n)
	}
}

func TestManagerCarriesOnWithoutViews(t *testing.T) {
	m, r := newTestManager(t)
	m.flowWait = 500 * time.Millisecond
	opts := shOptions(t)
	opts.Command = `head -c 4000000 /dev/zero | tr '\0' x; echo; echo done-marker`
	start := time.Now()
	startSh(t, m, "t1", opts)
	// No view acknowledges anything, as for a terminal that isn't shown,
	// and the output still all arrives. The session waits for its views
	// once, not once for each window of output, which would take over 7 s.
	waitOutput(t, r, "t1", "done-marker", wait)
	waitExit(t, r, "t1")
	if took := time.Since(start); took > 4*time.Second {
		t.Errorf("the output took %v, want the session to stop waiting for views that don't answer", took)
	}
}

func TestSessionWaitsForViewsAgainOnceOneAnswers(t *testing.T) {
	s := &session{
		acked: make(chan struct{}, 1), flowWait: 20 * time.Millisecond,
		stop: make(chan struct{}), exited: make(chan struct{}),
	}
	s.offset = flowWindow
	s.waitForViews() // No view answers, so this runs out.
	// The views didn't answer, so the session doesn't wait for them.
	s.offset += flowWindow
	start := time.Now()
	s.waitForViews()
	if took := time.Since(start); took >= s.flowWait {
		t.Errorf("waitForViews took %v with no view answering, want no wait", took)
	}

	// A view answers, but is still a window behind, so the session waits
	// for it again, until it catches up.
	s.flowWait = time.Minute
	s.ack(s.offset - flowWindow)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.waitForViews()
	}()
	select {
	case <-done:
		t.Fatal("waitForViews returned with the views a window behind")
	case <-time.After(100 * time.Millisecond):
	}
	s.ack(s.end())
	select {
	case <-done:
	case <-time.After(wait):
		t.Error("waitForViews still waits once the views have caught up")
	}
}

func TestManagerClose(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	write(t, m, "t1", "echo ready\n")
	waitOutput(t, r, "t1", "ready", wait)

	m.Close("t1")
	if err := waitExit(t, r, "t1"); err != nil {
		t.Errorf("Exit error after Close = %v, want nil", err)
	}
	if m.Running("t1") {
		t.Error("Running() = true after Close")
	}
	if err := m.Write("t1", []byte("x")); !errors.Is(err, ErrNoSession) {
		t.Errorf("Write() after Close = %v, want ErrNoSession", err)
	}
	if got := m.Title("t1"); got != "" {
		t.Errorf("Title() after Close = %q, want \"\"", got)
	}
	if got := m.Snapshot("t1"); got.Data != nil || got.Offset != 0 {
		t.Errorf("Snapshot() after Close = %+v, want nothing", got)
	}
	m.Close("t1") // Closing again does nothing.
	time.Sleep(100 * time.Millisecond)
	r.checkTidy(t, "t1")
}

func TestManagerCloseHangsUpForegroundCommand(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	startSh(t, m, "t1", opts)
	write(t, m, "t1", "sleep 30\n")
	want := formatTitle("sleep", abbreviate(opts.Dir, homeDir()))
	waitFor(t, wait, "sleep to start", func() bool { return m.Title("t1") == want })
	pgrp, ok := m.get("t1").foregroundGroup()
	if !ok {
		t.Fatal("can't find the foreground process group")
	}

	m.Close("t1")
	waitExit(t, r, "t1")
	if runtime.GOOS == "linux" {
		waitFor(t, wait, "sleep to end", func() bool { return !processAlive(pgrp) })
	}
}

// ignoresHangUp is a command that ignores SIGHUP, so that it outlives Close
// for a second, then prints something.
const ignoresHangUp = "trap '' HUP; echo ready; sleep 1; echo late-output"

func TestManagerCloseWhenHangUpIsIgnored(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	opts.Command = ignoresHangUp
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", "ready", wait)
	m.Close("t1")
	// Exit still comes, once the process ends by itself.
	if err := waitExit(t, r, "t1"); err != nil {
		t.Errorf("Exit error = %v, want nil after Close", err)
	}
	r.checkTidy(t, "t1")
}

func TestManagerRestartSilencesOldSession(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	old := opts
	old.Command = ignoresHangUp
	startSh(t, m, "t1", old)
	waitOutput(t, r, "t1", "ready", wait)

	// Close and start again with the same ID, as a restart would. The old
	// process carries on for a second, but nothing it does is reported.
	m.Close("t1")
	startSh(t, m, "t1", opts)
	write(t, m, "t1", "echo new-$((1+1))\n")
	waitOutput(t, r, "t1", "new-2", wait)
	time.Sleep(1500 * time.Millisecond)

	if out := r.output("t1"); strings.Contains(out, "late-output") {
		t.Errorf("output = %q, want nothing from the old session after the restart", tail(out, 80))
	}
	if n := r.exitCount("t1"); n != 0 {
		t.Errorf("Exit called %d times, want none: the new session is running", n)
	}
	if !m.Running("t1") {
		t.Error("Running() = false, want the new session")
	}
}

func TestManagerCloseAll(t *testing.T) {
	m, r := newTestManager(t)
	ids := []string{"a", "b", "c"}
	for _, id := range ids {
		startSh(t, m, id, shOptions(t))
		write(t, m, id, "echo ready-"+id+"\n")
	}
	for _, id := range ids {
		waitOutput(t, r, id, "ready-"+id, wait)
	}
	m.CloseAll()
	for _, id := range ids {
		waitExit(t, r, id)
		if m.Running(id) {
			t.Errorf("Running(%q) = true after CloseAll", id)
		}
	}
	r.checkTidy(t, ids...)
}

func TestManagerStartErrors(t *testing.T) {
	m, _ := newTestManager(t)
	opts := shOptions(t)
	opts.Shell = filepath.Join(opts.Dir, "no-such-shell")
	if err := m.Start("t1", opts); err == nil {
		t.Fatal("Start() with a missing shell = nil, want an error")
	}
	if m.Running("t1") {
		t.Error("Running() = true after Start failed")
	}
	if err := m.Write("t1", []byte("x")); !errors.Is(err, ErrNoSession) {
		t.Errorf("Write() = %v, want ErrNoSession", err)
	}
}

func TestManagerConcurrentUse(t *testing.T) {
	m, r := newTestManager(t)
	const sessions = 4
	var wg sync.WaitGroup
	for i := range sessions {
		id := fmt.Sprintf("t%d", i)
		opts := shOptions(t)
		for range 3 {
			wg.Go(func() {
				// Several goroutines start the same session at once;
				// only one shell starts.
				if err := m.Start(id, opts); err != nil {
					t.Errorf("Start(%q) = %v", id, err)
					return
				}
				for j := range 20 {
					_ = m.Write(id, []byte("echo "+strconv.Itoa(j)+"\n"))
					_ = m.Resize(id, 80+j, 24+j)
					_ = m.Title(id)
					_ = m.Snapshot(id)
					_ = m.Running(id)
				}
			})
		}
	}
	wg.Wait()
	for i := range sessions {
		id := fmt.Sprintf("t%d", i)
		waitOutput(t, r, id, "19", wait)
	}
	wg.Go(func() { m.Close("t0") })
	wg.Go(func() { m.Close("t0") })
	wg.Go(m.CloseAll)
	wg.Wait()
	for i := range sessions {
		id := fmt.Sprintf("t%d", i)
		waitExit(t, r, id)
		r.checkTidy(t, id)
	}
}

// processAlive reports whether pid is a live process (not gone, and not a
// zombie waiting to be reaped).
func processAlive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the command name, which is in parentheses.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i < 0 || i+2 >= len(s) || s[i+2] != 'Z'
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
