//go:build darwin || linux

package term

import (
	"bytes"
	"encoding/hex"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// testSession returns a session without a process, for deliver to give
// output to. What it types back waits in its input (see typed). No view
// draws it, and agent status is on.
func testSession() (*session, *recorder) {
	r := newRecorder()
	s := &session{
		id:       "t1",
		cb:       r.callbacks(),
		replaced: new(atomic.Bool),
		views:    newViews(),
		statusOn: new(atomic.Bool),
		input:    make(chan []byte, inputQueue),
		hint:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		exited:   make(chan struct{}),
		out:      newRing(SnapshotLimit),
	}
	s.statusOn.Store(true)
	return s, r
}

// typed returns what a session has typed back to its program, and takes it
// from its input.
func typed(s *session) string {
	var b strings.Builder
	for {
		select {
		case p := <-s.input:
			b.Write(p)
		default:
			return b.String()
		}
	}
}

func TestSessionAnswersProbe(t *testing.T) {
	for _, end := range []string{belEnd, stEnd} {
		for _, drawn := range []bool{false, true} {
			// Claude Code sends DA1 after its probe: whichever answer
			// comes first tells it whether the terminal reads OSC 7501.
			out := "\x1b]7501;?" + end + "\x1b[c"
			want := statusReply + end
			if !drawn {
				want += da1Reply
			}
			// The same answers however the output is split across reads.
			for i := 0; i <= len(out); i++ {
				s, _ := testSession()
				if drawn {
					s.views.add(ViewMain)
				}
				s.deliver([]byte(out[:i]), true)
				s.deliver([]byte(out[i:]), true)
				if got := typed(s); got != want {
					t.Errorf("%q split at %d, drawn by a view %t: typed %q, want %q", out, i, drawn, got, want)
				}
			}
		}
	}
}

func TestSessionAnswersBeforePassingOutputOn(t *testing.T) {
	s, _ := testSession()
	s.views.add(ViewMain)
	var atData string
	s.cb.Data = func(string, int64, []byte) { atData = typed(s) }
	s.deliver([]byte("\x1b]7501;?\a\x1b[c"), true)
	if want := statusReply + belEnd; atData != want {
		t.Errorf("typed %q by the time the output was passed on, want %q, so that the view's answer to DA1 can't come first", atData, want)
	}
}

func TestSessionAnswersOneProbeARead(t *testing.T) {
	s, _ := testSession()
	s.deliver([]byte("\x1b]7501;?\a\x1b]7501;?\a\x1b]7501;?\x1b\\"), true)
	if got, want := typed(s), statusReply+belEnd; got != want {
		t.Errorf("typed %q for three probes in one read, want one answer, %q", got, want)
	}
	s.deliver([]byte("\x1b]7501;?\x1b\\"), true)
	if got, want := typed(s), statusReply+stEnd; got != want {
		t.Errorf("typed %q for a probe in the next read, want %q", got, want)
	}
}

func TestSessionAgentStatusOff(t *testing.T) {
	s, r := testSession()
	s.statusOn.Store(false)
	s.deliver([]byte("\x1b]7501;?\a\x1b[c\x1b]7501;state=working\a\x1bc"), true)
	// DA1 is answered whether agent status is on or not.
	if got := typed(s); got != da1Reply {
		t.Errorf("typed %q with agent status off, want only the answer to DA1, %q", got, da1Reply)
	}
	if got := r.statusList("t1"); len(got) != 0 {
		t.Errorf("statuses = %+v with agent status off, want none", got)
	}
}

func TestSessionAnswersDA1OnlyWithoutAView(t *testing.T) {
	tests := []struct{ out, want string }{
		{"\x1b[c", da1Reply},
		{"\x1b[0c", da1Reply},
		{"\x1b[0;1c", da1Reply},
		{"\x1b[c\x1b[c", da1Reply + da1Reply},
		{"\x1b[\x07c", da1Reply},
		{"\x1b[>c", ""},
		{"\x1b[=c", ""},
		{"\x1b[?1;2c", ""},
		{"\x1b[1c", ""},
		{"\x1b[ c", ""},
		{"text c", ""},
	}
	for _, tt := range tests {
		s, _ := testSession()
		s.deliver([]byte(tt.out), true)
		if got := typed(s); got != tt.want {
			t.Errorf("%q with no view: typed %q, want %q", tt.out, got, tt.want)
		}
		s.views.add(ViewMain)
		s.deliver([]byte(tt.out), true)
		if got := typed(s); got != "" {
			t.Errorf("%q with a view: typed %q, want nothing", tt.out, got)
		}
	}
}

func TestSessionReportsStatusBeforeAlerts(t *testing.T) {
	s, r := testSession()
	// The bell comes before the report, but the report goes first, so the
	// bell and notification count as part of it.
	s.deliver([]byte("\a\x1b]7501;state=blocked:kind=permission:msg="+b64("approve Bash: npm test")+"\x1b\\"+
		"\x1b]9;Claude needs your permission\a"), true)
	if got, want := r.reportList("t1"), []string{"Status blocked", "Alert", "Alert"}; !slices.Equal(got, want) {
		t.Errorf("callbacks = %q, want %q", got, want)
	}
	want := Status{State: StateBlocked, Kind: KindPermission, Msg: "approve Bash: npm test", Progress: -1}
	if got := r.statusList("t1"); len(got) != 1 || got[0] != want {
		t.Errorf("statuses = %+v, want %+v", got, want)
	}
	if got, want := r.alertList("t1"), []Alert{{Bell: true}, {Body: "Claude needs your permission"}}; !slices.Equal(got, want) {
		t.Errorf("alerts = %+v, want %+v", got, want)
	}
}

func TestSessionReportsTheRootRecordOnly(t *testing.T) {
	s, r := testSession()
	s.deliver([]byte("\x1b]7501;state=working:id=a3f9c2\a"+
		"\x1b]7501;state=clear:id=a3f9c2\a"+
		"\x1b]7501;state=sleeping\a"+
		"\x1b]7501;state=done:msg=QQ=\a"+
		"\x1b]7501;state=working:app=claude-code\a"), true)
	want := []Status{{State: StateWorking, App: "claude-code", Progress: -1}}
	if got := r.statusList("t1"); !slices.Equal(got, want) {
		t.Errorf("statuses = %+v, want %+v", got, want)
	}
}

func TestSessionReportsStatusSplitAcrossReads(t *testing.T) {
	out := "\x1b]7501;state=blocked:kind=question:msg=" + b64("Which database?") + "\x1b\\"
	want := []Status{{State: StateBlocked, Kind: KindQuestion, Msg: "Which database?", Progress: -1}}
	for i := 1; i < len(out); i++ {
		s, r := testSession()
		s.deliver([]byte(out[:i]), true)
		s.deliver([]byte(out[i:]), true)
		if got := r.statusList("t1"); !slices.Equal(got, want) {
			t.Errorf("split at %d: statuses = %+v, want %+v", i, got, want)
		}
	}
}

func TestSessionFullResetClearsStatus(t *testing.T) {
	s, r := testSession()
	s.deliver([]byte("\x1b]7501;state=done\a\x1bc"), true)
	want := []Status{{State: StateDone, Progress: -1}, {State: StateClear, Progress: -1}}
	if got := r.statusList("t1"); !slices.Equal(got, want) {
		t.Errorf("statuses = %+v, want %+v", got, want)
	}
}

func TestSessionKeepsTheLatestStatuses(t *testing.T) {
	s, r := testSession()
	var out strings.Builder
	for range maxStatuses + 4 {
		out.WriteString("\x1b]7501;state=working\a")
	}
	out.WriteString("\x1b]7501;state=done\a")
	s.deliver([]byte(out.String()), true)
	got := r.statusList("t1")
	if len(got) != maxStatuses || got[len(got)-1].State != StateDone {
		t.Errorf("statuses = %+v, want the latest %d, ending with done", got, maxStatuses)
	}
}

func TestSessionAnswersNothingInRestoredOutput(t *testing.T) {
	out := "a\x1b]7501;?\a\x1b[c\x1b]7501;state=blocked\x1b\\\x1bcb"
	// A preface is made with Restorable, which takes out all that asks or
	// reports, so that it can't ask or report again.
	s, r := testSession()
	s.deliver(Restorable([]byte(out), SnapshotLimit), true)
	if got := typed(s); got != "" {
		t.Errorf("typed %q for restored output, want nothing", got)
	}
	if got := r.statusList("t1"); len(got) != 0 {
		t.Errorf("statuses = %+v for restored output, want none", got)
	}
	// And output that isn't the program's asks and reports nothing, even
	// if it holds what would.
	s, r = testSession()
	s.deliver([]byte(out), false)
	if got := typed(s); got != "" {
		t.Errorf("typed %q for output that isn't the program's, want nothing", got)
	}
	if got := r.statusList("t1"); len(got) != 0 {
		t.Errorf("statuses = %+v for output that isn't the program's, want none", got)
	}
}

func TestAttachDividesTheOutput(t *testing.T) {
	// However a view's Attach and the output fall, each request for device
	// attributes is either in the view's snapshot, which the view replays
	// without answering, and the session answers it, or after it, and the
	// session leaves it to the view.
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("with one CPU, Attach can't come while the output goes")
	}
	// The session's input (inputQueue) holds an answer to each request, so
	// that none is lost. So many come after the kth that the view attaches
	// before they end.
	const queries = 500
	midway := 0
	for i := range 200 {
		s, _ := testSession()
		m := &Manager{sessions: map[string]*session{"t1": s}, views: map[string]*views{"t1": s.views}}
		// The view attaches once k requests have gone, while the rest go.
		// k is never queries, so started always closes.
		k := i%(queries-1) + 1
		started := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for j := range queries {
				if j == k {
					close(started)
				}
				s.deliver([]byte("\x1b[c"), true)
			}
		}()
		<-started
		snap := m.Attach("t1", ViewMain)
		<-done
		inSnapshot := strings.Count(string(snap.Data), "\x1b[c")
		if inSnapshot > 0 && inSnapshot < queries {
			midway++
		}
		if answered := strings.Count(typed(s), da1Reply); answered != inSnapshot {
			t.Fatalf("the session answered %d requests, want %d: those in the snapshot", answered, inSnapshot)
		}
	}
	if midway == 0 {
		t.Fatal("no Attach came in the middle of the output, so the test shows nothing")
	}
}

// readAnswers returns a command that prints out, then reads what's typed
// back to it, up to n bytes, or what comes until it has waited a second
// for more, and prints that in hex between "got:" and ":end".
func readAnswers(out string, n int) string {
	return `stty raw -echo min 0 time 10; printf '` + out + `'; printf 'got:'; ` +
		`dd bs=1 count=` + strconv.Itoa(n) + ` 2>/dev/null | od -An -tx1 | tr -d ' \n'; printf ':end'`
}

// probe is a probe for OSC 7501, then a request for device attributes, as
// printf writes them.
const probe = `\033]7501;?\033\\\033[c`

func TestManagerAnswersProbeAndDA1WithoutAView(t *testing.T) {
	m, r := newTestManager(t)
	want := statusReply + stEnd + da1Reply
	opts := shOptions(t)
	opts.Command = readAnswers(probe, len(want))
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", ":end", wait)
	if got, w := r.output("t1"), "got:"+hex.EncodeToString([]byte(want))+":end"; !strings.Contains(got, w) {
		t.Errorf("output = %q, want %q", got, w)
	}
}

func TestManagerLeavesDA1ToTheView(t *testing.T) {
	m, r := newTestManager(t)
	// The view attaches before the session starts, as a view of a command
	// that runs again does.
	m.Attach("t1", ViewMain)
	want := statusReply + stEnd
	opts := shOptions(t)
	// One byte more than the probe's answer would be DA1's: none comes.
	opts.Command = readAnswers(probe, len(want)+1)
	startSh(t, m, "t1", opts)
	waitOutput(t, r, "t1", ":end", wait)
	if got, w := r.output("t1"), "got:"+hex.EncodeToString([]byte(want))+":end"; !strings.Contains(got, w) {
		t.Errorf("output = %q, want %q", got, w)
	}
}

func TestManagerAgentStatusOff(t *testing.T) {
	m, r := newTestManager(t)
	m.SetAgentStatus(false)
	opts := shOptions(t)
	// One byte more than DA1's answer would be the probe's: none comes.
	opts.Command = readAnswers(probe, len(da1Reply)+1) + `; printf '\033]7501;state=working\007'`
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	if got, w := r.output("t1"), "got:"+hex.EncodeToString([]byte(da1Reply))+":end"; !strings.Contains(got, w) {
		t.Errorf("output = %q, want %q", got, w)
	}
	if got := r.statusList("t1"); len(got) != 0 {
		t.Errorf("statuses = %+v with agent status off, want none", got)
	}
}

func TestManagerReportsStatusBeforeAlerts(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	// What Claude Code sends when it needs permission, and is set up to
	// ring the bell and send a notification too.
	write(t, m, "t1", `printf '\033]7501;state=blocked:app=claude-code:kind=permission:msg=YXBwcm92ZSBCYXNoOiBucG0gdGVzdA==\007\007\033]9;Claude needs your permission\007'`+"\n")
	waitFor(t, wait, "the status and alerts", func() bool { return len(r.reportList("t1")) >= 3 })
	if got, want := r.reportList("t1"), []string{"Status blocked", "Alert", "Alert"}; !slices.Equal(got, want) {
		t.Errorf("callbacks = %q, want %q", got, want)
	}
	want := Status{State: StateBlocked, Kind: KindPermission, App: "claude-code", Msg: "approve Bash: npm test", Progress: -1}
	if got := r.statusList("t1"); len(got) != 1 || got[0] != want {
		t.Errorf("statuses = %+v, want %+v", got, want)
	}
}

func TestManagerReportsProgramEnd(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	waitStates := func(want ...string) {
		t.Helper()
		waitFor(t, wait, "statuses "+strings.Join(want, ", "), func() bool { return len(states(r, "t1")) >= len(want) })
		// Give a status that shouldn't come time to show itself.
		time.Sleep(20 * m.titleEvery)
		if got := states(r, "t1"); !slices.Equal(got, want) {
			t.Fatalf("statuses = %q, want %q", got, want)
		}
	}

	// A program that the shell runs reports that it's working, then ends
	// without saying more, as one that crashes would.
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; sleep 0.2'`+"\n")
	waitStates(StateWorking, StateEnded)

	// One that's done when it ends stays done.
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=done\007"; sleep 0.2'`+"\n")
	waitStates(StateWorking, StateEnded, StateDone)

	// A status that the shell reports itself lasts until a program that
	// the shell runs after it ends.
	write(t, m, "t1", `printf '\033]7501;state=working\007'`+"\n")
	waitStates(StateWorking, StateEnded, StateDone, StateWorking)
	write(t, m, "t1", "sleep 0.2\n")
	want := []string{StateWorking, StateEnded, StateDone, StateWorking, StateEnded}
	waitStates(want...)

	// A program that reports twice, then ends at once. The shell can have
	// the terminal back before the second report is read, and that report
	// mustn't hide that the program has gone. Which comes first is a race,
	// so the program runs a number of times. If Bench sees that the
	// program has gone before it reads the second report, that report
	// counts as the shell's own, and comes after the end.
	for range 10 {
		write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; sleep 0.1; printf "\033]7501;state=working\007"'`+"\n")
		n := len(want)
		waitFor(t, wait, "statuses "+strings.Join(want, ", ")+", and three more", func() bool { return len(states(r, "t1")) >= n+3 })
		got := states(r, "t1")[n : n+3]
		if !slices.Equal(got, []string{StateWorking, StateWorking, StateEnded}) && !slices.Equal(got, []string{StateWorking, StateEnded, StateWorking}) {
			t.Fatalf("statuses for a program that reports twice, then ends = %q, want working, working, ended, or working, ended, working", got)
		}
		want = append(want, got...)
	}
	waitStates(want...)

	// In a shell that the shell starts, the program's end is reported once
	// the inner shell has the terminal back, while that shell still runs.
	write(t, m, "t1", "sh -i\n")
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=blocked:kind=permission\007"; sleep 0.2'`+"\n")
	want = append(want, StateBlocked, StateEnded)
	waitStates(want...)

	// There too, the end of a program that reports twice, then ends at
	// once, is reported. The inner shell, which started the program, can
	// have the terminal back before the second report is read. Each run
	// ends with a report of done from the inner shell itself, so that a
	// late report that counted as the inner shell's own doesn't last into
	// the next run.
	for range 10 {
		write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; sleep 0.1; printf "\033]7501;state=working\007"'`+"\n")
		n := len(want)
		waitFor(t, wait, "statuses "+strings.Join(want, ", ")+", and three more", func() bool { return len(states(r, "t1")) >= n+3 })
		got := states(r, "t1")[n : n+3]
		if !slices.Equal(got, []string{StateWorking, StateWorking, StateEnded}) && !slices.Equal(got, []string{StateWorking, StateEnded, StateWorking}) {
			t.Fatalf("statuses for a program in sh -i that reports twice, then ends = %q, want working, working, ended, or working, ended, working", got)
		}
		want = append(want, got...)
		write(t, m, "t1", `printf '\033]7501;state=done\007'`+"\n")
		want = append(want, StateDone)
		waitFor(t, wait, "statuses "+strings.Join(want, ", "), func() bool { return len(states(r, "t1")) >= len(want) })
	}
	waitStates(want...)

	// A program that the inner shell starts as soon as another has gone,
	// as in "a; b", keeps its status while it runs, even if Bench hasn't
	// yet seen the first go.
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; sleep 0.2'; sh -c 'printf "\033]7501;state=blocked\007"; read line'`+"\n")
	n := len(want)
	waitFor(t, wait, "a status from the second program", func() bool { return slices.Contains(states(r, "t1")[n:], StateBlocked) })
	time.Sleep(20 * m.titleEvery)
	if got := states(r, "t1")[n:]; !slices.Equal(got, []string{StateWorking, StateBlocked}) && !slices.Equal(got, []string{StateWorking, StateEnded, StateBlocked}) {
		t.Fatalf("statuses while the second of two programs runs = %q, want working, blocked, or working, ended, blocked", got)
	}
	write(t, m, "t1", "\n")
	waitFor(t, wait, "the end of the second program", func() bool {
		got := states(r, "t1")
		return got[len(got)-1] == StateEnded && slices.Contains(got[n:], StateBlocked)
	})
}

func TestManagerKeepsStatusWhileAChildHasTheTerminal(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	noEnd := func(while string) {
		t.Helper()
		// Give an end that shouldn't come time to show itself.
		time.Sleep(20 * m.titleEvery)
		if got, want := states(r, "t1"), []string{StateWorking}; !slices.Equal(got, want) {
			t.Fatalf("statuses %s = %q, want %q", while, got, want)
		}
	}

	// A program reports that it's working, then runs an interactive shell
	// that takes the terminal and gives it to programs of its own. Then it
	// runs one that has the terminal at its prompt, as vim's :sh does, and
	// lines are typed there. The program runs all the while, and it ends
	// once a line is typed. Most of the programs that the shells run are
	// /bin/echo, not the shell's own echo, so each prints a line as it
	// ends. Output makes Bench look at the terminal at once, so it can look
	// after one of them has ended, but before the shell that ran it has
	// taken the terminal back.
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; sleep 0.3; sh -ic "sleep 0.3; for i in \$(seq 400); do /bin/echo \$i; done"; echo "ran-$((40+2))"; PS1="in""ner> " sh -i; echo "over-$((40+2))"; read line'`+"\n")
	waitOutput(t, r, "t1", "ran-42", wait)
	noEnd("once the program's shell has run programs of its own")
	waitOutput(t, r, "t1", "inner> ", wait)
	noEnd("while the program's shell has the terminal at its prompt")
	for i := range 5 {
		n := strconv.Itoa(i)
		write(t, m, "t1", "for i in $(seq 80); do /bin/echo $i; done; /bin/echo typed-$((40+"+n+"))\n")
		waitOutput(t, r, "t1", "typed-4"+n, wait)
	}
	noEnd("once lines have been typed at the program's shell's prompt")
	write(t, m, "t1", "exit\n")
	waitOutput(t, r, "t1", "over-42", wait)
	noEnd("once the program has the terminal back")
	write(t, m, "t1", "\n")
	waitFor(t, wait, "the end of the program", func() bool { return slices.Contains(states(r, "t1"), StateEnded) })
	if got, want := states(r, "t1"), []string{StateWorking, StateEnded}; !slices.Equal(got, want) {
		t.Errorf("statuses = %q, want %q", got, want)
	}
}

func TestManagerKeepsReportsWhileAChildHasTheTerminal(t *testing.T) {
	m, r := newTestManager(t)
	startSh(t, m, "t1", shOptions(t))
	// A program reports that it's working, then runs an interactive shell
	// that takes the terminal. While that shell has it, the program
	// reports again, from a job of its own. The report is the program's,
	// so once the shell gives the terminal back, the program's status
	// stays while it runs, and ends with it.
	write(t, m, "t1", `sh -c 'printf "\033]7501;state=working\007"; (sleep 0.8; printf "\033]7501;state=blocked\007") & sleep 0.3; PS1="in""ner> " sh -i; echo "over-$((40+2))"; read line'`+"\n")
	waitOutput(t, r, "t1", "inner> ", wait)
	waitFor(t, wait, "the report while the program's shell has the terminal", func() bool {
		return slices.Contains(states(r, "t1"), StateBlocked)
	})
	write(t, m, "t1", "exit\n")
	waitOutput(t, r, "t1", "over-42", wait)
	// Give an end that shouldn't come time to show itself.
	time.Sleep(20 * m.titleEvery)
	if got, want := states(r, "t1"), []string{StateWorking, StateBlocked}; !slices.Equal(got, want) {
		t.Fatalf("statuses once the program has the terminal back = %q, want %q", got, want)
	}
	write(t, m, "t1", "\n")
	waitFor(t, wait, "the end of the program", func() bool { return slices.Contains(states(r, "t1"), StateEnded) })
	if got, want := states(r, "t1"), []string{StateWorking, StateBlocked, StateEnded}; !slices.Equal(got, want) {
		t.Errorf("statuses = %q, want %q", got, want)
	}
}

func TestManagerCommandStatusEndsWithExit(t *testing.T) {
	m, r := newTestManager(t)
	opts := shOptions(t)
	// A command's session reports its end with Exit, and nothing else.
	opts.Command = `printf '\033]7501;state=working\007'; sleep 0.2`
	startSh(t, m, "t1", opts)
	waitExit(t, r, "t1")
	if got, want := states(r, "t1"), []string{StateWorking}; !slices.Equal(got, want) {
		t.Errorf("statuses = %q, want %q", got, want)
	}
	r.checkTidy(t, "t1")
}

// viewsOf returns the manager's record of the views of id's sessions, or
// nil if it has none.
func viewsOf(m *Manager, id string) *views {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.views[id]
}

func TestManagerViews(t *testing.T) {
	m, r := newTestManager(t)
	// A view can attach before its session starts: there's nothing to
	// replay yet.
	if snap := m.Attach("t1", ViewMain); snap.Data != nil || snap.Offset != 0 {
		t.Errorf("Attach() with no session = %+v, want nothing", snap)
	}
	startSh(t, m, "t1", shOptions(t))
	if v := viewsOf(m, "t1"); v == nil || m.get("t1").views != v || !v.drawn() {
		t.Fatal("the session doesn't share the record of the view that attached before it started")
	}

	// Attach returns the snapshot, as Snapshot does.
	write(t, m, "t1", "echo ready-$((40+2))\n")
	waitOutput(t, r, "t1", "ready-42", wait)
	waitFor(t, wait, "the output so far in the snapshot", func() bool {
		end, _ := r.offset("t1")
		return m.Snapshot("t1").Offset == end
	})
	if got, want := m.Attach("t1", ViewOwn), m.Snapshot("t1"); !bytes.Equal(got.Data, want.Data) || got.Offset != want.Offset {
		t.Errorf("Attach() = %q at %d, want the snapshot, %q at %d", got.Data, got.Offset, want.Data, want.Offset)
	}

	// Each view goes by itself.
	m.Detach("t1", ViewMain)
	if !viewsOf(m, "t1").drawn() {
		t.Error("no view draws the terminal once one of two has gone")
	}
	m.DetachView(ViewOwn)
	if viewsOf(m, "t1").drawn() {
		t.Error("a view draws the terminal once both have gone")
	}

	// The record outlasts the session while a view draws its terminal, so
	// that a session started again knows it.
	m.Attach("t1", ViewMain)
	m.Close("t1")
	v := viewsOf(m, "t1")
	if v == nil || !v.drawn() {
		t.Fatal("the record of the view went with the session")
	}
	startSh(t, m, "t1", shOptions(t))
	if m.get("t1").views != v {
		t.Error("the new session doesn't share the record of its view")
	}

	// Once neither a session nor a view is left, the record goes.
	m.Close("t1")
	m.Detach("t1", ViewMain)
	m.Attach("t2", ViewMain)
	m.DetachView(ViewMain)
	if viewsOf(m, "t1") != nil || viewsOf(m, "t2") != nil {
		t.Error("records of views are kept for terminals with neither a session nor a view")
	}
}
