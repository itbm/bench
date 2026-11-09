package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// approve is what Claude Code says when it needs permission to run a
// command.
const approve = "approve Bash: npm test"

// agentHarness starts the standard world with Bench active, showing the
// Comms tab, so that the Code tab's terminal, w5, is out of sight.
func agentHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := attentionHarness(t, s)
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	return h
}

// report reports what a terminal's agent says about itself, and lets the
// engine apply it.
func (h *harness) report(id string, s term.Status) {
	h.terms.cb.Status(id, s)
	h.advance(2 * frame)
}

// agentSays returns a report of state, with kind and msg.
func agentSays(state, kind, msg string) term.Status {
	return term.Status{State: state, Kind: kind, Msg: msg, Progress: -1}
}

func TestAgentEventIsOneAlertWithItsWords(t *testing.T) {
	h := agentHarness(t, setup{})
	// Claude Code reports that it needs permission, rings the bell, and
	// asks for a notification, all at once, in any order. The name and
	// label it gives itself are its own, not the terminal's.
	h.terms.cb.Alert("w5", term.Alert{Bell: true})
	h.terms.cb.Status("w5", term.Status{State: term.StateBlocked, Kind: term.KindPermission, App: "claude-code",
		Title: "Not the terminal", Msg: approve, Progress: -1})
	h.terms.cb.Alert("w5", term.Alert{Title: "Claude Code", Body: "Claude needs your permission"})
	h.advance(2 * frame)

	v := h.view()
	it := v.Items["w5"]
	if it.Agent != agentWaiting || it.AgentKind != term.KindPermission || it.AgentText != approve {
		t.Errorf("agent %q, kind %q, text %q", it.Agent, it.AgentKind, it.AgentText)
	}
	if it.Attention != "Needs your permission: "+approve {
		t.Errorf("attention: %q", it.Attention)
	}
	if tab := v.Tabs[1]; tab.Attention != 1 || tab.Waiting != 1 || tab.Working != 0 {
		t.Errorf("tab: attention %d, waiting %d, working %d", tab.Attention, tab.Waiting, tab.Working)
	}
	if h.p.Badge() != 1 {
		t.Errorf("badge: %d", h.p.Badge())
	}
	want := platform.Notification{ItemID: "w5", Title: "zsh — ~/code", Subtitle: "Code", Body: "Needs your permission: " + approve}
	if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
		t.Fatalf("notifications: %+v", n)
	}

	// A bell long after keeps the agent's words, and posts nothing.
	h.advance(notifyGap)
	h.alert("w5", term.Alert{Bell: true})
	if got := h.item("w5").Attention; got != "Needs your permission: "+approve {
		t.Errorf("a later bell changed the mark: %q", got)
	}
	if n := h.p.Notifications(); len(n) != 1 {
		t.Errorf("a later bell posted: %+v", n)
	}
	// But a notification the agent asks for later is a new event.
	h.alert("w5", term.Alert{Body: "Still waiting"})
	if got := h.item("w5").Attention; got != "Notification: Still waiting" || len(h.p.Notifications()) != 2 {
		t.Errorf("a later notification: attention %q, %d notifications", got, len(h.p.Notifications()))
	}
}

func TestAgentAfterAnAlertIsStillNotified(t *testing.T) {
	bell := term.Alert{Bell: true}
	wordsOff := strings.Replace(exampleConfig(t), "agent_words = true", "agent_words = false", 1)
	for _, tt := range []struct {
		name   string
		config string
		alert  term.Alert
		body   string
		report term.Status
		line   string
		mark   string
	}{
		{"bell", "", bell, "Rang the bell.", agentSays(term.StateBlocked, term.KindPermission, approve),
			"Needs your permission: " + approve, "Needs your permission: " + approve},
		{"notification", "", term.Alert{Title: "Claude Code", Body: "Claude needs your permission"}, "Claude Code: Claude needs your permission",
			agentSays(term.StateBlocked, term.KindPermission, approve), "Needs your permission: " + approve, "Needs your permission: " + approve},
		// A bell says less than the agent's line, even without its words.
		{"bell, then blocked without words", "", bell, "Rang the bell.", agentSays(term.StateBlocked, "", ""), agentNeedsText, agentNeedsText},
		{"bell, agent_words off", wordsOff, bell, "Rang the bell.", agentSays(term.StateBlocked, term.KindPermission, approve),
			agentPermissionText, "Needs your permission: " + approve},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := agentHarness(t, setup{config: tt.config})
			// The agent rings the bell, or asks for a notification, just
			// before it reports, so that the alert is in the batch before.
			// The alert has posted already, but the agent's line follows.
			h.alert("w5", tt.alert)
			h.report("w5", tt.report)
			n := h.p.Notifications()
			if len(n) != 2 || n[0].Body != tt.body || n[1].Body != tt.line {
				t.Fatalf("notifications: %+v", n)
			}
			if got := h.item("w5").Attention; got != tt.mark {
				t.Errorf("attention: %q", got)
			}
			// That's once: notifyGap holds the next one back.
			h.report("w5", agentSays(term.StateBlocked, term.KindPermission, "approve Bash: rm -rf build"))
			if n := h.p.Notifications(); len(n) != 2 {
				t.Errorf("another report: %+v", n)
			}
		})
	}

	// An alert longer than agentEcho before is another event, and
	// notifyGap holds the agent's notification back.
	h := agentHarness(t, setup{})
	h.alert("w5", term.Alert{Bell: true})
	h.advance(agentEcho)
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	if n := h.p.Notifications(); len(n) != 1 || n[0].Body != "Rang the bell." {
		t.Errorf("notifications: %+v", n)
	}
	if got := h.item("w5").Attention; got != "Needs your permission: "+approve {
		t.Errorf("attention: %q", got)
	}

	// After a notification, an agent's notification without its words
	// would say less than the notification did, so notifyGap holds it
	// back: for an agent that sends no words, such as Codex, and while the
	// agent_words setting is off.
	for _, tt := range []struct {
		name   string
		config string
		alert  term.Alert
		report term.Status
		mark   string
	}{
		{"no words", exampleConfig(t), term.Alert{Title: "Codex", Body: "Codex wants to run npm test"},
			agentSays(term.StateBlocked, "", ""), agentNeedsText},
		{"agent_words off", wordsOff,
			term.Alert{Title: "Claude Code", Body: "Claude needs your permission"},
			agentSays(term.StateBlocked, term.KindPermission, approve), "Needs your permission: " + approve},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := agentHarness(t, setup{config: tt.config})
			h.alert("w5", tt.alert)
			h.report("w5", tt.report)
			if n := h.p.Notifications(); len(n) != 1 || n[0].Body != noteText(tt.alert.Title, tt.alert.Body) {
				t.Errorf("notifications: %+v", n)
			}
			if got := h.item("w5").Attention; got != tt.mark {
				t.Errorf("attention: %q", got)
			}
		})
	}
}

func TestAgentTakesItsWordsFromItsNotification(t *testing.T) {
	h := agentHarness(t, setup{})
	// Codex says only what it needs, and asks for a notification that
	// says why.
	h.terms.cb.Status("w5", agentSays(term.StateBlocked, term.KindPermission, ""))
	h.terms.cb.Alert("w5", term.Alert{Title: "Codex", Body: "Approval requested: rm -rf build"})
	h.advance(2 * frame)
	it := h.item("w5")
	if it.AgentText != "Approval requested: rm -rf build" || it.Attention != "Needs your permission: Approval requested: rm -rf build" {
		t.Errorf("text %q, attention %q", it.AgentText, it.Attention)
	}
	want := platform.Notification{ItemID: "w5", Title: "zsh — ~/code", Subtitle: "Code", Body: "Needs your permission: Approval requested: rm -rf build"}
	if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
		t.Fatalf("notifications: %+v", n)
	}

	// A notification with only a title gives no words.
	h.advance(notifyGap)
	h.terms.cb.Status("w5", agentSays(term.StateDone, "", ""))
	h.terms.cb.Alert("w5", term.Alert{Title: "Codex"})
	h.advance(2 * frame)
	if it := h.item("w5"); it.Agent != agentDone || it.AgentText != "" || it.Attention != agentDoneText {
		t.Errorf("agent %q, text %q, attention %q", it.Agent, it.AgentText, it.Attention)
	}
	if n := h.p.Notifications(); len(n) != 2 || n[1].Body != agentDoneText {
		t.Errorf("notifications: %+v", n)
	}
}

func TestAgentWordsCanBeLeftOutOfNotifications(t *testing.T) {
	h := agentHarness(t, setup{config: strings.Replace(exampleConfig(t), "agent_words = true", "agent_words = false", 1)})
	if h.view().Settings.AgentWords {
		t.Fatal("the view should say the words are left out")
	}
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	if n := h.p.Notifications(); len(n) != 1 || n[0].Body != "Needs your permission" {
		t.Errorf("notifications: %+v", n)
	}
	// The mark has them.
	if it := h.item("w5"); it.Attention != "Needs your permission: "+approve || it.AgentText != approve {
		t.Errorf("attention %q, text %q", it.Attention, it.AgentText)
	}
}

func TestAgentKindTexts(t *testing.T) {
	tests := []struct {
		state, kind, words string
		withWords          bool
		want               string
	}{
		{term.StateBlocked, term.KindPermission, approve, true, "Needs your permission: " + approve},
		{term.StateBlocked, term.KindPermission, approve, false, "Needs your permission"},
		{term.StateBlocked, term.KindQuestion, "", true, "Has a question for you"},
		{term.StateBlocked, term.KindAuth, "GitHub", true, "Needs you to sign in: GitHub"},
		{term.StateBlocked, "", "Look at this", true, "Needs you: Look at this"},
		{term.StateDone, "", "Tests passed", true, "Finished: Tests passed"},
		{term.StateError, "", "", true, "Stopped with an error"},
	}
	for _, tt := range tests {
		if got := agentLine(tt.state, tt.kind, tt.words, tt.withWords); got != tt.want {
			t.Errorf("agentLine(%q, %q, %q, %v) = %q, want %q", tt.state, tt.kind, tt.words, tt.withWords, got, tt.want)
		}
	}
}

func TestAgentStatusCanBeTurnedOff(t *testing.T) {
	h := agentHarness(t, setup{config: strings.Replace(exampleConfig(t), "agent_status = true", "agent_status = false", 1)})
	if h.terms.agentStatusOn() {
		t.Error("the terminals' host should have agent status off")
	}
	// Reports change nothing, so the bell works as it always has.
	h.terms.cb.Status("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	h.terms.cb.Alert("w5", term.Alert{Bell: true})
	h.advance(2 * frame)
	v := h.view()
	if it := v.Items["w5"]; it.Agent != "" || it.AgentText != "" || it.Attention != bellText {
		t.Errorf("agent %q, text %q, attention %q", it.Agent, it.AgentText, it.Attention)
	}
	if tab := v.Tabs[1]; tab.Waiting != 0 || tab.Attention != 1 {
		t.Errorf("tab: waiting %d, attention %d", tab.Waiting, tab.Attention)
	}
	if n := h.p.Notifications(); len(n) != 1 || n[0].Body != "Rang the bell." {
		t.Errorf("notifications: %+v", n)
	}

	// Turning it on tells the host, and reports count from then on.
	h.saveSetting(func(s *SettingsView) { s.AgentStatus = true })
	if !h.terms.agentStatusOn() {
		t.Error("the host should have agent status on")
	}
	h.report("w5", agentSays(term.StateBlocked, term.KindQuestion, "Which branch?"))
	if it := h.item("w5"); it.Agent != agentWaiting || it.Attention != "Has a question for you: Which branch?" {
		t.Errorf("agent %q, attention %q", it.Agent, it.Attention)
	}

	// Turning it off forgets every report, and the mark of an agent that
	// waits.
	h.saveSetting(func(s *SettingsView) { s.AgentStatus = false })
	if h.terms.agentStatusOn() {
		t.Error("the host should have agent status off again")
	}
	v = h.view()
	if it := v.Items["w5"]; it.Agent != "" || it.Attention != "" || v.Tabs[1].Attention != 0 {
		t.Errorf("agent %q, attention %q, tab %d", it.Agent, it.Attention, v.Tabs[1].Attention)
	}
	h.saveSetting(func(s *SettingsView) { s.AgentStatus = true })
	if it := h.item("w5"); it.Agent != "" {
		t.Errorf("a report came back: %q", it.Agent)
	}
	if !savedConfig(t, h).Settings.AgentStatus {
		t.Error("config.toml should have agent status on")
	}
}

func TestAgentStatusFollowsConfigFile(t *testing.T) {
	h := agentHarness(t, setup{})
	h.report("w5", agentSays(term.StateWorking, "", ""))
	editConfig(t, h, "agent_status = true", "agent_status = false")
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	if h.terms.agentStatusOn() || h.item("w5").Agent != "" {
		t.Errorf("after turning it off in config.toml: host on %v, agent %q", h.terms.agentStatusOn(), h.item("w5").Agent)
	}
	editConfig(t, h, "agent_status = false", "agent_status = true")
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	if !h.terms.agentStatusOn() {
		t.Error("after turning it on in config.toml, the host should have it on")
	}
}

func TestAgentInViewIsntMarked(t *testing.T) {
	h := attentionHarness(t, setup{})
	// An agent that needs the user shows as waiting, and its tab counts
	// it, but it isn't marked, and nothing is posted.
	h.terms.cb.Status("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	h.terms.cb.Alert("w5", term.Alert{Bell: true})
	h.advance(2 * frame)
	v := h.view()
	if it := v.Items["w5"]; it.Agent != agentWaiting || it.AgentText != approve || it.Attention != "" {
		t.Errorf("agent %q, text %q, attention %q", it.Agent, it.AgentText, it.Attention)
	}
	if tab := v.Tabs[1]; tab.Attention != 1 || tab.Waiting != 1 {
		t.Errorf("tab: attention %d, waiting %d", tab.Attention, tab.Waiting)
	}
	if h.p.Badge() != 0 || len(h.p.Notifications()) != 0 {
		t.Errorf("badge %d, notifications %+v", h.p.Badge(), h.p.Notifications())
	}

	// One that finishes, or stops, in view shows nothing: the user saw it.
	for _, state := range []string{term.StateDone, term.StateError} {
		h.report("w5", agentSays(state, "", "Tests passed"))
		v = h.view()
		if it := v.Items["w5"]; it.Agent != "" || it.AgentText != "" || it.Attention != "" {
			t.Errorf("%s: agent %q, text %q, attention %q", state, it.Agent, it.AgentText, it.Attention)
		}
		if tab := v.Tabs[1]; tab.Attention != 0 || tab.Done != 0 || tab.Errored != 0 {
			t.Errorf("%s: tab %+v", state, tab)
		}
	}
	if n := h.p.Notifications(); len(n) != 0 {
		t.Errorf("notifications: %+v", n)
	}

	// One that works shows as working, and its tab counts it.
	h.report("w5", agentSays(term.StateWorking, "", "Running tests"))
	v = h.view()
	if it := v.Items["w5"]; it.Agent != agentWorking || it.AgentText != "Running tests" || it.AgentKind != "" {
		t.Errorf("agent %q, text %q, kind %q", it.Agent, it.AgentText, it.AgentKind)
	}
	if tab := v.Tabs[1]; tab.Working != 1 || tab.Attention != 0 {
		t.Errorf("tab: working %d, attention %d", tab.Working, tab.Attention)
	}
	// And an idle one shows nothing.
	h.report("w5", agentSays(term.StateIdle, "", "Ready"))
	if it := h.item("w5"); it.Agent != "" || it.AgentText != "" {
		t.Errorf("idle: agent %q, text %q", it.Agent, it.AgentText)
	}
}

func TestAgentDoneShowsUntilSeen(t *testing.T) {
	for _, tt := range []struct{ state, view, why string }{
		{term.StateDone, agentDone, "Finished: Tests passed"},
		{term.StateError, agentError, "Stopped with an error: Tests passed"},
	} {
		t.Run(tt.state, func(t *testing.T) {
			h := agentHarness(t, setup{})
			h.report("w5", agentSays(tt.state, "", "Tests passed"))
			v := h.view()
			if it := v.Items["w5"]; it.Agent != tt.view || it.AgentText != "Tests passed" || it.Attention != tt.why {
				t.Errorf("agent %q, text %q, attention %q", it.Agent, it.AgentText, it.Attention)
			}
			tab := v.Tabs[1]
			if tab.Attention != 1 || tab.Done+tab.Errored != 1 || tab.Done != boolInt(tt.state == term.StateDone) {
				t.Errorf("tab: %+v", tab)
			}
			if n := h.p.Notifications(); len(n) != 1 || n[0].Body != tt.why {
				t.Errorf("notifications: %+v", n)
			}
			// It stays while the user looks elsewhere, and the program
			// ends.
			h.report("w5", agentSays(term.StateEnded, "", ""))
			h.settle()
			if it := h.item("w5"); it.Agent != tt.view {
				t.Errorf("after the program ended: %q", it.Agent)
			}
			// Looking at it takes it off.
			mustOK(t, h.e.SwitchTab(1))
			h.settle()
			v = h.view()
			if it := v.Items["w5"]; it.Agent != "" || it.AgentText != "" || it.Attention != "" {
				t.Errorf("after showing it: agent %q, text %q, attention %q", it.Agent, it.AgentText, it.Attention)
			}
			if tab := v.Tabs[1]; tab.Done != 0 || tab.Errored != 0 || tab.Attention != 0 {
				t.Errorf("after showing it: tab %+v", tab)
			}
		})
	}
}

// boolInt returns 1 for true and 0 for false.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestAgentThatCarriesOnIsntWaiting(t *testing.T) {
	h := agentHarness(t, setup{})
	// The user answers in another way, such as from their phone: the agent
	// works again, and its mark goes.
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	h.report("w5", agentSays(term.StateWorking, "", ""))
	v := h.view()
	if it := v.Items["w5"]; it.Agent != agentWorking || it.Attention != "" || v.Tabs[1].Attention != 0 || v.Tabs[1].Working != 1 {
		t.Errorf("working: agent %q, attention %q, tab %+v", it.Agent, it.Attention, v.Tabs[1])
	}
	// So does a waiting agent's mark when the agent is idle, or clears
	// its report.
	for _, state := range []string{term.StateIdle, term.StateClear} {
		h.report("w5", agentSays(term.StateBlocked, "", "Look at this"))
		if got := h.item("w5").Attention; got != "Needs you: Look at this" {
			t.Fatalf("attention: %q", got)
		}
		h.report("w5", agentSays(state, "", ""))
		if it := h.item("w5"); it.Agent != "" || it.Attention != "" {
			t.Errorf("%s: agent %q, attention %q", state, it.Agent, it.Attention)
		}
	}
	// But a finished agent's mark stays until the user sees it, when it
	// clears its report.
	h.report("w5", agentSays(term.StateDone, "", "Tests passed"))
	h.report("w5", agentSays(term.StateClear, "", ""))
	if it := h.item("w5"); it.Agent != "" || it.Attention != "Finished: Tests passed" {
		t.Errorf("done, then clear: agent %q, attention %q", it.Agent, it.Attention)
	}
}

func TestAgentThatCarriesOnAtOnceIsNotified(t *testing.T) {
	// codex is the notification that an agent without words, such as
	// Codex, asks for as it finishes, with what it did.
	codex := term.Alert{Title: "Codex", Body: "Fixed the failing test"}
	for _, tt := range []struct {
		name        string
		first, then term.Status
		alert       *term.Alert
		agent, why  string
	}{
		{"done, then working", agentSays(term.StateDone, "", "Tests passed"), agentSays(term.StateWorking, "", ""), nil, agentWorking, "Finished: Tests passed"},
		{"error, then idle", agentSays(term.StateError, "", "rsync failed"), agentSays(term.StateIdle, "", ""), nil, "", "Stopped with an error: rsync failed"},
		{"done, then clear", agentSays(term.StateDone, "", "Tests passed"), agentSays(term.StateClear, "", ""), nil, "", "Finished: Tests passed"},
		{"done and its notification, then working", agentSays(term.StateDone, "", ""), agentSays(term.StateWorking, "", ""), &codex, agentWorking, "Finished: Fixed the failing test"},
		{"error and its notification, then idle", agentSays(term.StateError, "", ""), agentSays(term.StateIdle, "", ""), &codex, "", "Stopped with an error: Fixed the failing test"},
		{"done and its notification, then clear", agentSays(term.StateDone, "", ""), agentSays(term.StateClear, "", ""), &codex, "", "Finished: Fixed the failing test"},
	} {
		for _, batch := range []string{"same batch", "separate batches"} {
			t.Run(tt.name+", "+batch, func(t *testing.T) {
				h := agentHarness(t, setup{})
				// The agent finishes, and carries on at once, as with a
				// prompt that waits. It keeps its mark, and its
				// notification goes, whatever the batches. An agent
				// without words asks for a notification as it finishes,
				// which gives them, although the agent's next report is
				// applied before it.
				h.terms.cb.Status("w5", tt.first)
				if tt.alert != nil {
					h.terms.cb.Alert("w5", *tt.alert)
				}
				if batch == "separate batches" {
					h.advance(2 * frame)
				}
				h.report("w5", tt.then)
				if n := h.p.Notifications(); len(n) != 1 || n[0].Body != tt.why {
					t.Errorf("notifications: %+v", n)
				}
				if it := h.item("w5"); it.Agent != tt.agent || it.Attention != tt.why || h.p.Badge() != 1 {
					t.Errorf("agent %q, attention %q, badge %d", it.Agent, it.Attention, h.p.Badge())
				}
				// The View keeps the event for VoiceOver, with the same
				// line.
				if it := h.item("w5"); it.AgentEvent != 1 || it.AgentEventLine != tt.why {
					t.Errorf("event %d, line %q", it.AgentEvent, it.AgentEventLine)
				}
			})
		}
	}

	// An agent that needed the user, and carries on at once, has no mark,
	// so it gets no notification either.
	h := agentHarness(t, setup{})
	h.terms.cb.Status("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	h.report("w5", agentSays(term.StateWorking, "", ""))
	if it := h.item("w5"); it.Agent != agentWorking || it.Attention != "" || len(h.p.Notifications()) != 0 {
		t.Errorf("agent %q, attention %q, notifications %+v", it.Agent, it.Attention, h.p.Notifications())
	}
}

func TestAgentEventsAreCounted(t *testing.T) {
	permission := "Needs your permission: " + approve
	for _, seen := range []bool{false, true} {
		name := "out of sight"
		if seen {
			name = "in view"
		}
		t.Run(name, func(t *testing.T) {
			h := agentHarness(t, setup{})
			if seen {
				mustOK(t, h.e.SwitchTab(1))
				h.settle()
			}
			// event checks the View's count of events and the latest
			// one's line, after one more event if raised.
			n := 0
			event := func(what string, raised bool, line string) {
				t.Helper()
				if raised {
					n++
				}
				if it := h.item("w5"); it.AgentEvent != n || it.AgentEventLine != line {
					t.Errorf("%s: event %d, line %q; want %d, %q", what, it.AgentEvent, it.AgentEventLine, n, line)
				}
			}
			event("at first", false, "")
			h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
			event("blocked", true, permission)
			h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
			event("the same report again", false, permission)
			// Work is no event, and the last one's line stays.
			h.report("w5", agentSays(term.StateWorking, "", "Running the tests"))
			event("working", false, permission)
			// A finished agent that the user sees is forgotten at once,
			// but its event counts.
			h.report("w5", agentSays(term.StateDone, "", "Tests passed"))
			event("done", true, "Finished: Tests passed")
			h.report("w5", agentSays(term.StateError, "", "rsync failed"))
			event("error", true, "Stopped with an error: rsync failed")
			if !seen {
				h.report("w5", agentSays(term.StateError, "", "rsync failed"))
				event("the same error again", false, "Stopped with an error: rsync failed")
			}
			// Words that come from a notification just after a report
			// without words, in the next batch, are no new event, but the
			// line has them.
			h.report("w5", agentSays(term.StateBlocked, term.KindPermission, ""))
			event("blocked without words", true, agentPermissionText)
			h.alert("w5", term.Alert{Title: "Codex", Body: "Approval requested: npm test"})
			event("its notification", false, "Needs your permission: Approval requested: npm test")
			// A bell later is no event either, nor is clearing the report.
			h.alert("w5", term.Alert{Bell: true})
			event("a bell", false, "Needs your permission: Approval requested: npm test")
			h.report("w5", agentSays(term.StateClear, "", ""))
			event("clear", false, "Needs your permission: Approval requested: npm test")

			// With agent status off, the View has neither, and turning it
			// on again gives the same count, which never goes back.
			h.saveSetting(func(s *SettingsView) { s.AgentStatus = false })
			if it := h.item("w5"); it.AgentEvent != 0 || it.AgentEventLine != "" {
				t.Errorf("agent status off: event %d, line %q", it.AgentEvent, it.AgentEventLine)
			}
			h.saveSetting(func(s *SettingsView) { s.AgentStatus = true })
			event("agent status on again", false, "Needs your permission: Approval requested: npm test")
		})
	}
}

func TestAgentSameReportIsTheSameEvent(t *testing.T) {
	h := agentHarness(t, setup{})
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	// The user looks, and then looks away, while the agent still waits.
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if it := h.item("w5"); it.Agent != agentWaiting || it.Attention != "" {
		t.Fatalf("seen: agent %q, attention %q", it.Agent, it.Attention)
	}
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.advance(notifyGap)
	// The same report again is no new event.
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	if it := h.item("w5"); it.Attention != "" || len(h.p.Notifications()) != 1 {
		t.Errorf("the same report: attention %q, notifications %+v", it.Attention, h.p.Notifications())
	}
	// Nor is the bell that came with it.
	h.alert("w5", term.Alert{Bell: true})
	if it := h.item("w5"); it.Attention != "" || len(h.p.Notifications()) != 1 {
		t.Errorf("its bell: attention %q, notifications %+v", it.Attention, h.p.Notifications())
	}
	// Another one is.
	h.advance(notifyGap)
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, "approve Bash: rm -rf build"))
	if it := h.item("w5"); it.Attention != "Needs your permission: approve Bash: rm -rf build" || len(h.p.Notifications()) != 2 {
		t.Errorf("another report: attention %q, notifications %+v", it.Attention, h.p.Notifications())
	}
}

func TestAgentGoesWithItsProgram(t *testing.T) {
	h := agentHarness(t, setup{})
	// A program that worked or waited, and has gone, as the shell has
	// the terminal again, leaves nothing.
	for _, s := range []term.Status{agentSays(term.StateWorking, "", ""), agentSays(term.StateBlocked, "", approve), agentSays(term.StateIdle, "", "")} {
		h.report("w5", s)
		h.report("w5", agentSays(term.StateEnded, "", ""))
		if it := h.item("w5"); it.Agent != "" || it.Attention != "" {
			t.Errorf("%s, then ended: agent %q, attention %q", s.State, it.Agent, it.Attention)
		}
	}
}

func TestAgentInACommandEndsWithIt(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	// Obsidian isn't in a tab, so the terminal is out of sight.
	mustOK(t, h.p.ActivateApp(obsidianID))
	h.settle()
	id := h.builtinID()
	exit := func() {
		h.terms.end(id)
		h.terms.cb.Exit(id, nil)
		h.advance(2 * frame)
	}
	rerun := func() {
		mustOK(t, h.e.TermInput(id, "\r"))
		h.advance(2 * frame)
	}

	h.report(id, agentSays(term.StateBlocked, term.KindPermission, approve))
	exit()
	if it := h.item(id); it.Agent != "" || it.Attention != finishedText {
		t.Errorf("waiting, then exit: agent %q, attention %q", it.Agent, it.Attention)
	}

	// A finished agent stays when its program ends, until the command
	// runs again.
	rerun()
	h.report(id, agentSays(term.StateDone, "", "Tests passed"))
	exit()
	if it := h.item(id); it.Agent != agentDone || it.AgentText != "Tests passed" || !it.Ended {
		t.Errorf("done, then exit: %+v", it)
	}
	rerun()
	if it := h.item(id); it.Agent != "" || it.Ended {
		t.Errorf("after running again: %+v", it)
	}
}

func TestAgentInACommandIsNotifiedAsItEnds(t *testing.T) {
	for _, tt := range []struct {
		state, agent, why, mark string
		err                     error
	}{
		{term.StateDone, agentDone, "Finished: Tests passed", finishedText, nil},
		{term.StateError, agentError, "Stopped with an error: Tests passed", failedText, errors.New("exit status 1")},
	} {
		for _, batch := range []string{"same batch", "separate batches"} {
			t.Run(tt.state+", "+batch, func(t *testing.T) {
				h := newHarness(t, setup{config: commandConfig})
				// Obsidian isn't in a tab, so the terminal is out of sight.
				mustOK(t, h.p.ActivateApp(obsidianID))
				h.settle()
				id := h.builtinID()
				// The program reports just before it ends, as the spec
				// asks, so its report and its end are often in the same
				// batch.
				h.terms.cb.Status(id, agentSays(tt.state, "", "Tests passed"))
				if batch == "separate batches" {
					h.advance(2 * frame)
				}
				h.terms.end(id)
				h.terms.cb.Exit(id, tt.err)
				h.advance(2 * frame)
				// The agent's notification says more than the command's.
				want := platform.Notification{ItemID: id, Title: "Tests", Subtitle: "Code", Body: tt.why}
				if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
					t.Errorf("notifications: %+v", n)
				}
				if it := h.item(id); it.Agent != tt.agent || it.AgentText != "Tests passed" || !it.Ended || it.Attention != tt.mark {
					t.Errorf("agent %q, text %q, ended %v, attention %q", it.Agent, it.AgentText, it.Ended, it.Attention)
				}
			})
		}
	}
}

func TestAgentInAShellIsNotifiedAsItEnds(t *testing.T) {
	for _, tt := range []struct {
		name        string
		first, then term.Status
		why         string
	}{
		{"done", agentSays(term.StateDone, "", "Tests passed"), term.Status{}, "Finished: Tests passed"},
		{"error", agentSays(term.StateError, "", "rsync failed"), term.Status{}, "Stopped with an error: rsync failed"},
		{"done, then working", agentSays(term.StateDone, "", "Tests passed"), agentSays(term.StateWorking, "", ""), "Finished: Tests passed"},
	} {
		for _, batch := range []string{"same batch", "separate batches"} {
			t.Run(tt.name+", "+batch, func(t *testing.T) {
				h := agentHarness(t, setup{})
				// A shell with no command ends just after its agent's
				// report, as with "codex exec …; exit", which Bench then
				// removes with its item.
				h.terms.cb.Status("w5", tt.first)
				if tt.then.State != "" {
					h.terms.cb.Status("w5", tt.then)
				}
				if batch == "separate batches" {
					h.advance(2 * frame)
				}
				h.terms.end("w5")
				h.terms.cb.Exit("w5", nil)
				h.advance(2 * frame)
				want := platform.Notification{ItemID: "w5", Title: "zsh — ~/code", Subtitle: "Code", Body: tt.why}
				if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
					t.Errorf("notifications: %+v", n)
				}
				if _, ok := h.view().Items["w5"]; ok {
					t.Error("the item should be gone")
				}
			})
		}
	}
}

func TestAgentReportsArentSaved(t *testing.T) {
	h := agentHarness(t, setup{})
	h.report("w5", agentSays(term.StateBlocked, term.KindPermission, approve))
	h.settle()
	mustOK(t, h.e.Close())
	if state := readFile(t, h, config.StateFile); state == "" || strings.Contains(state, approve) || strings.Contains(state, "Needs your") {
		t.Errorf("state.json holds the agent's report:\n%s", state)
	}
	h.restart()
	if it := h.item("w5"); it.Agent != "" || it.AgentText != "" {
		t.Errorf("after a restart: agent %q, text %q", it.Agent, it.AgentText)
	}
}

func TestTermAttachAndDetach(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.terms.snaps["w5"] = term.Snapshot{Data: []byte("hi"), Offset: 2}
	snap, err := h.e.TermAttach("w5", term.ViewMain)
	if err != nil || snap.Data != "aGk=" || snap.Offset != 2 {
		t.Errorf("TermAttach = %+v, %v; want the snapshot", snap, err)
	}
	if got := h.terms.drawnBy("w5"); !slices.Equal(got, []string{term.ViewMain}) {
		t.Errorf("drawn by %v", got)
	}
	if _, err := h.e.TermAttach("w3", term.ViewMain); !errors.Is(err, errNoTerminal) {
		t.Errorf("TermAttach of an App item: %v", err)
	}
	mustOK(t, h.e.TermDetach("w5", term.ViewMain))
	if got := h.terms.drawnBy("w5"); len(got) != 0 {
		t.Errorf("after TermDetach: drawn by %v", got)
	}
	// TermDetach of a terminal that has gone isn't refused.
	mustOK(t, h.e.TermDetach("w99", term.ViewMain))

	// The main window forgets its views as it loads again.
	_, _ = h.e.TermAttach("w5", term.ViewMain)
	_, _ = h.e.TermAttach("w5", term.ViewOwn)
	if _, err := h.e.Ready(); err != nil {
		t.Fatal(err)
	}
	if got := h.terms.drawnBy("w5"); !slices.Equal(got, []string{term.ViewOwn}) {
		t.Errorf("after Ready: drawn by %v", got)
	}
	mustOK(t, h.e.TermDetach("w5", term.ViewOwn))

	// A terminal's own window no longer draws it once Bench closes the
	// window, as the terminal comes back into a tab.
	pane := h.item("w5").Pane
	mustOK(t, h.e.PopOut("w5"))
	_, _ = h.e.TermAttach("w5", term.ViewOwn)
	var other string
	for _, p := range h.view().Display.Panes {
		if p.ID != pane {
			other = p.ID
			break
		}
	}
	mustOK(t, h.e.AddToPane(other, Source{Type: "floating", ID: "w5"}))
	h.drain()
	if got := h.terms.drawnBy("w5"); len(got) != 0 {
		t.Errorf("after the terminal came back: drawn by %v", got)
	}
	// Nor once the user closes it.
	mustOK(t, h.e.PopOut("w5"))
	_, _ = h.e.TermAttach("w5", term.ViewOwn)
	h.e.TerminalWindowClosed("w5")
	h.drain()
	if got := h.terms.drawnBy("w5"); len(got) != 0 {
		t.Errorf("after its window closed: drawn by %v", got)
	}
}
