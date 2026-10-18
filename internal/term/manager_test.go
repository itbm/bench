//go:build darwin || linux

package term

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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
	if len(snap) > SnapshotLimit {
		t.Errorf("len(Snapshot()) = %d, want at most %d", len(snap), SnapshotLimit)
	}
	if !strings.HasSuffix(string(snap), "done-marker\r\n") {
		t.Errorf("Snapshot() ends %q, want the latest output", tail(string(snap), 40))
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
	// overwritten, and the numbers wouldn't run in order.
	lines := strings.Split(r.output("t1"), "\r\n")
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
	if got := m.Snapshot("t1"); got != nil {
		t.Errorf("Snapshot() after Close = %q, want nil", got)
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
