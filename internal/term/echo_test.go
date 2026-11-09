package term

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const echoWait = 5 * time.Second

func newTestEcho(t *testing.T) (*Echo, *recorder) {
	t.Helper()
	r := newRecorder()
	e := NewEcho(r.callbacks())
	t.Cleanup(e.CloseAll)
	return e, r
}

func startEcho(t *testing.T, e *Echo, id string, opts Options) {
	t.Helper()
	if err := e.Start(id, opts); err != nil {
		t.Fatalf("Start(%q) = %v", id, err)
	}
}

func typeInto(t *testing.T, e *Echo, id string, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := e.Write(id, []byte(k)); err != nil {
			t.Fatalf("Write(%q, %q) = %v", id, k, err)
		}
	}
}

func TestEchoPromptAndTitle(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{Dir: "~/code/bench"})
	waitOutput(t, r, "t1", "~/code/bench $ ", echoWait)
	if !strings.HasPrefix(r.output("t1"), echoBanner) {
		t.Errorf("output = %q, want it to start with the banner", r.output("t1"))
	}
	if got, want := e.Title("t1"), "zsh — ~/code/bench"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if !e.Running("t1") {
		t.Error("Running() = false after Start")
	}
}

func TestEchoFolder(t *testing.T) {
	home := homeDir()
	tests := []struct {
		dir  string
		want string
	}{
		{"", "~"},
		{"~", "~"},
		{"~/code/", "~/code"},
		{"code", "~/code"},
		{"/opt/work", "/opt/work"},
	}
	if home != "" && home != "/" {
		tests = append(tests, struct{ dir, want string }{home + "/code/bench", "~/code/bench"})
	}
	for _, tt := range tests {
		if got := echoFolder(tt.dir, home); got != tt.want {
			t.Errorf("echoFolder(%q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

func TestEchoTyping(t *testing.T) {
	const prompt = "~ $ "
	tests := []struct {
		name string
		keys []string
		want string // the output after the banner and first prompt
	}{
		{"echoes characters", []string{"l", "s"}, "ls"},
		{"backspace", []string{"lx", "\x7f", "s"}, "lx\b \bs"},
		{"backspace on an empty line", []string{"\x7f", "a"}, "a"},
		{"enter on an empty line", []string{"\r"}, "\r\n" + prompt},
		{"help", []string{"help\r"}, "help\r\n" + echoHelp + prompt},
		{"ls", []string{"ls\r"}, "ls\r\n" + echoListing + prompt},
		{"pwd", []string{"pwd\r"}, "pwd\r\n" + pretendHome + "\r\n" + prompt},
		{"echo", []string{`echo hello   "big world"` + "\r"}, `echo hello   "big world"` + "\r\nhello big world\r\n" + prompt},
		{"clear", []string{"clear\r"}, "clear\r\n" + clearScreen + prompt},
		{"unknown command", []string{"make all\r"},
			"make all\r\nmake: the browser preview can't run commands. Type help to see what it can do.\r\n" + prompt},
		{"control-C", []string{"abc", "\x03"}, "abc^C\r\n" + prompt},
		{"control-U", []string{"abc", "\x15"}, "abc\b \b\b \b\b \b"},
		{"control-L", []string{"ab", "\x0c"}, "ab" + clearScreen + prompt + "ab"},
		{"arrow keys are ignored", []string{"a", "\x1b[A", "\x1bOB", "\x1b[1;5C", "b"}, "ab"},
		{"escape split across writes", []string{"a", "\x1b", "[", "D", "b"}, "ab"},
		{"tab is ignored", []string{"a\tb"}, "ab"},
		{"CR LF is one enter", []string{"pwd\r\n"}, "pwd\r\n" + pretendHome + "\r\n" + prompt},
		{"LF alone is enter", []string{"pwd\n"}, "pwd\r\n" + pretendHome + "\r\n" + prompt},
		{"UTF-8 split across writes", []string{"\xc3", "\xa9"}, "é"},
		{"backspace removes a whole character", []string{"é", "\x7f"}, "é\b \b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, r := newTestEcho(t)
			startEcho(t, e, "t1", Options{})
			start := echoBanner + prompt
			waitOutput(t, r, "t1", start, echoWait)
			typeInto(t, e, "t1", tt.keys...)
			want := start + tt.want
			waitFor(t, echoWait, fmt.Sprintf("output %q", want), func() bool {
				return len(r.output("t1")) >= len(want)
			})
			// Give stray output a moment to show itself.
			time.Sleep(20 * time.Millisecond)
			if got := r.output("t1"); got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestEchoCommand(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{Dir: "~/code/bench", Command: "echo hi"})
	want := echoBanner + "~/code/bench $ echo hi\r\nhi\r\n~/code/bench $ "
	waitOutput(t, r, "t1", want, echoWait)
}

func TestEchoInput(t *testing.T) {
	e, r := newTestEcho(t)
	// Input is typed at the first prompt, as if the user had typed it.
	startEcho(t, e, "t1", Options{Dir: "~/code/bench", Input: "echo hi\r"})
	want := echoBanner + "~/code/bench $ echo hi\r\nhi\r\n~/code/bench $ "
	waitOutput(t, r, "t1", want, echoWait)

	// Without Return, it waits at the prompt for the user to run it.
	startEcho(t, e, "t2", Options{Input: "echo later"})
	waitOutput(t, r, "t2", "~ $ echo later", echoWait)
	typeInto(t, e, "t2", "\r")
	waitOutput(t, r, "t2", "echo later\r\nlater\r\n~ $ ", echoWait)
}

func TestEchoChangesFolder(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{Dir: "~/code"})
	tests := []struct{ cd, want string }{
		{"cd bench", "~/code/bench"},
		{"cd ..", "~/code"},
		{"cd /tmp", "/tmp"},
		{"cd", "~"},
		{"cd ~/notes", "~/notes"},
	}
	for _, tt := range tests {
		typeInto(t, e, "t1", tt.cd+"\r")
		waitFor(t, echoWait, "the prompt in "+tt.want, func() bool {
			return strings.HasSuffix(r.output("t1"), tt.cd+"\r\n"+tt.want+" $ ")
		})
		if got, want := e.Title("t1"), "zsh — "+tt.want; got != want {
			t.Errorf("after %q, Title() = %q, want %q", tt.cd, got, want)
		}
	}
	waitFor(t, echoWait, "Folder callbacks", func() bool { return len(r.folderList("t1")) == len(tests) })
	want := []string{"~/code/bench", "~/code", "/tmp", "~", "~/notes"}
	if got := r.folderList("t1"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("folders = %q, want %q", got, want)
	}
	if titles := r.titleList("t1"); len(titles) != len(tests) {
		t.Errorf("titles = %q, want one for each cd", titles)
	}
}

func TestEchoBellAndNotify(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	typeInto(t, e, "t1", "bell 0\r")
	waitFor(t, echoWait, "the bell", func() bool { return len(r.alertList("t1")) == 1 })
	typeInto(t, e, "t1", "notify 0 Tests passed\r")
	waitFor(t, echoWait, "the notification", func() bool { return len(r.alertList("t1")) == 2 })
	if got, want := r.alertList("t1"), []Alert{{Bell: true}, {Body: "Tests passed"}}; got[0] != want[0] || got[1] != want[1] {
		t.Errorf("alerts = %+v, want %+v", got, want)
	}
	waitOutput(t, r, "t1", "\x1b]9;Tests passed\a", echoWait)
	// Closing the session drops one that isn't due yet.
	typeInto(t, e, "t1", "notify 60\r")
	e.Close("t1")
	waitFor(t, echoWait, "Exit", func() bool { return r.exitCount("t1") == 1 })
	r.checkTidy(t, "t1")
}

func TestEchoAgent(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	tests := []struct {
		typed string
		want  Status
	}{
		{"agent 0 working", Status{State: StateWorking, Progress: -1}},
		{"agent 0 blocked permission approve Bash: npm test", Status{State: StateBlocked, Kind: KindPermission, Msg: "approve Bash: npm test", Progress: -1}},
		{"agent 0 blocked question Which database?", Status{State: StateBlocked, Kind: KindQuestion, Msg: "Which database?", Progress: -1}},
		{"agent 0 blocked auth", Status{State: StateBlocked, Kind: KindAuth, Progress: -1}},
		// A kind is taken only if it's one of the protocol's, and only
		// after blocked.
		{"agent 0 blocked approve the plan", Status{State: StateBlocked, Msg: "approve the plan", Progress: -1}},
		{"agent 0 done permission granted", Status{State: StateDone, Msg: "permission granted", Progress: -1}},
		{"agent 0 error Tests failed", Status{State: StateError, Msg: "Tests failed", Progress: -1}},
		{"agent 0 idle", Status{State: StateIdle, Progress: -1}},
		{"agent 0 clear", Status{State: StateClear, Progress: -1}},
	}
	for i, tt := range tests {
		typeInto(t, e, "t1", tt.typed+"\r")
		waitFor(t, echoWait, "the report for "+quote(tt.typed), func() bool { return len(r.statusList("t1")) > i })
		if got := r.statusList("t1")[i]; got != tt.want {
			t.Errorf("%q reported %+v, want %+v", tt.typed, got, tt.want)
		}
	}
	// The report is printed, as a program would print it.
	waitOutput(t, r, "t1", "\x1b]7501;state=blocked:kind=permission:msg="+b64("approve Bash: npm test")+"\x1b\\", echoWait)

	// With agent status off, the report is printed, but not passed on.
	e.SetAgentStatus(false)
	typeInto(t, e, "t1", "agent 0 working while off\r")
	waitOutput(t, r, "t1", b64("while off"), echoWait)
	if got := r.statusList("t1"); len(got) != len(tests) {
		t.Errorf("statuses = %+v, want none more with agent status off", got[len(tests):])
	}
	e.SetAgentStatus(true)

	// Without a state, agent says how to use it.
	typeInto(t, e, "t1", "agent sleeping\r")
	waitOutput(t, r, "t1", "agent sleeping\r\n"+echoAgentHelp, echoWait)

	// Closing the session drops a report that isn't due yet.
	typeInto(t, e, "t1", "agent 60 done\r")
	e.Close("t1")
	waitFor(t, echoWait, "Exit", func() bool { return r.exitCount("t1") == 1 })
	r.checkTidy(t, "t1")
}

func TestEchoAgentReportsInOrder(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	// Reports go in the order they're due, and those due at once in the
	// order they were typed.
	typeInto(t, e, "t1", "agent 1 done\ragent 0 working\ragent 0 blocked\ragent 0 idle\r")
	waitFor(t, echoWait, "four reports", func() bool { return len(r.statusList("t1")) == 4 })
	if got, want := states(r, "t1"), []string{StateWorking, StateBlocked, StateIdle, StateDone}; !slices.Equal(got, want) {
		t.Errorf("statuses = %q, want %q", got, want)
	}
}

func TestEchoAttach(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	waitOutput(t, r, "t1", "~ $ ", echoWait)
	if got, want := e.Attach("t1", ViewMain), e.Snapshot("t1"); string(got.Data) != string(want.Data) || got.Offset != want.Offset {
		t.Errorf("Attach() = %q at %d, want the snapshot, %q at %d", got.Data, got.Offset, want.Data, want.Offset)
	}
	e.Detach("t1", ViewMain)
	e.DetachView(ViewMain)
	if got := e.Attach("none", ViewOwn); got.Data != nil || got.Offset != 0 {
		t.Errorf("Attach() with no session = %+v, want nothing", got)
	}
}

func TestEchoPreface(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{Preface: []byte("from before\r\n")})
	waitOutput(t, r, "t1", "~ $ ", echoWait)
	if got, want := r.output("t1"), "from before\r\n"+echoBanner; !strings.HasPrefix(got, want) {
		t.Errorf("output = %q, want it to start with the preface and the banner", got)
	}
}

func TestEchoNeverBusy(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{Command: "npm run dev"})
	waitOutput(t, r, "t1", "npm run dev\r\n", echoWait)
	if what, busy := e.Busy("t1"); busy {
		t.Errorf("Busy() = %q, true; want false: a pretend session runs nothing", what)
	}
}

func TestEchoSnapshot(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	typeInto(t, e, "t1", "echo snap\r")
	waitOutput(t, r, "t1", "snap\r\n~ $ ", echoWait)
	snap := e.Snapshot("t1")
	if got, want := string(snap.Data), r.output("t1"); got != want {
		t.Errorf("Snapshot() = %q, want everything printed, %q", got, want)
	}
	if end, ok := r.offset("t1"); snap.Offset != end || !ok {
		t.Errorf("Snapshot().Offset = %d, want %d, where the output has got to (offsets follow on: %t)", snap.Offset, end, ok)
	}

	long := strings.Repeat("x", 1000)
	for range 400 {
		typeInto(t, e, "t1", "echo "+long+"\r")
	}
	waitFor(t, echoWait, "lots of output", func() bool { return len(r.output("t1")) > 2*SnapshotLimit })
	if n := len(e.Snapshot("t1").Data); n > SnapshotLimit {
		t.Errorf("len(Snapshot()) = %d, want at most %d", n, SnapshotLimit)
	}
}

func TestEchoClear(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	typeInto(t, e, "t1", "echo secret\r")
	waitOutput(t, r, "t1", "secret\r\n~ $ ", echoWait)
	end, _ := r.offset("t1")
	e.Clear("t1")
	if snap := e.Snapshot("t1"); len(snap.Data) != 0 || snap.Offset != end {
		t.Errorf("Snapshot() after Clear = %q at %d, want nothing at %d", snap.Data, snap.Offset, end)
	}
	typeInto(t, e, "t1", "echo after\r")
	waitOutput(t, r, "t1", "after\r\n~ $ ", echoWait)
	if got, want := string(e.Snapshot("t1").Data), "echo after\r\nafter\r\n~ $ "; got != want {
		t.Errorf("Snapshot() = %q, want %q", got, want)
	}
}

func TestEchoResize(t *testing.T) {
	e, _ := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	if err := e.Resize("t1", 120, 40); err != nil {
		t.Errorf("Resize() = %v", err)
	}
	if err := e.Resize("t1", -1, 40); err == nil {
		t.Error("Resize(-1, 40) = nil, want an error")
	}
	if err := e.Resize("nope", 80, 24); !errors.Is(err, ErrNoSession) {
		t.Errorf("Resize(unknown) = %v, want ErrNoSession", err)
	}
}

func TestEchoStartTwice(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	startEcho(t, e, "t1", Options{Dir: "/elsewhere"})
	waitOutput(t, r, "t1", "~ $ ", echoWait)
	time.Sleep(20 * time.Millisecond)
	if n := strings.Count(r.output("t1"), echoBanner); n != 1 {
		t.Errorf("banner printed %d times, want once", n)
	}
	if got := e.Title("t1"); got != "zsh — ~" {
		t.Errorf("Title() = %q, want the first session's", got)
	}
}

func TestEchoClose(t *testing.T) {
	e, r := newTestEcho(t)
	startEcho(t, e, "t1", Options{})
	waitOutput(t, r, "t1", "~ $ ", echoWait)
	e.Close("t1")
	waitFor(t, echoWait, "Exit", func() bool { return r.exitCount("t1") > 0 })
	if err := r.exitList("t1")[0]; err != nil {
		t.Errorf("Exit error = %v, want nil", err)
	}
	if e.Running("t1") {
		t.Error("Running() = true after Close")
	}
	if err := e.Write("t1", []byte("x")); !errors.Is(err, ErrNoSession) {
		t.Errorf("Write() after Close = %v, want ErrNoSession", err)
	}
	if e.Title("t1") != "" || e.Snapshot("t1").Data != nil {
		t.Error("Title() or Snapshot() still answer after Close")
	}
	e.Close("t1")
	time.Sleep(20 * time.Millisecond)
	r.checkTidy(t, "t1")
}

func TestEchoCloseAll(t *testing.T) {
	e, r := newTestEcho(t)
	ids := []string{"a", "b", "c"}
	for _, id := range ids {
		startEcho(t, e, id, Options{})
	}
	e.CloseAll()
	for _, id := range ids {
		waitFor(t, echoWait, "Exit for "+id, func() bool { return r.exitCount(id) > 0 })
		if e.Running(id) {
			t.Errorf("Running(%q) = true after CloseAll", id)
		}
	}
	r.checkTidy(t, ids...)
}

func TestEchoConcurrentUse(t *testing.T) {
	e, r := newTestEcho(t)
	var wg sync.WaitGroup
	for i := range 4 {
		id := fmt.Sprintf("t%d", i)
		for range 3 {
			wg.Go(func() {
				_ = e.Start(id, Options{})
				for j := range 50 {
					_ = e.Write(id, []byte(fmt.Sprintf("echo %d\r", j)))
					_ = e.Resize(id, 80, 24)
					_ = e.Title(id)
					_ = e.Snapshot(id)
					_ = e.Attach(id, ViewMain)
					e.Detach(id, ViewMain)
					e.SetAgentStatus(j%2 == 0)
					_ = e.Running(id)
				}
			})
		}
	}
	wg.Wait()
	wg.Go(func() { e.Close("t0") })
	wg.Go(e.CloseAll)
	wg.Wait()
	for i := range 4 {
		id := fmt.Sprintf("t%d", i)
		waitFor(t, echoWait, "Exit for "+id, func() bool { return r.exitCount(id) > 0 })
		r.checkTidy(t, id)
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"echo hi", []string{"echo", "hi"}},
		{"echo  'a  b' \"c d\"", []string{"echo", "a  b", "c d"}},
		{"echo ''", []string{"echo", ""}},
		{"echo it's", []string{"echo", "its"}},
	}
	for _, tt := range tests {
		got := splitWords(tt.in)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") || len(got) != len(tt.want) {
			t.Errorf("splitWords(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
