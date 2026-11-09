package workspace

import (
	"time"

	"github.com/itbm/bench/internal/term"
)

// This file has agent status (decision 78): what a program in a terminal,
// such as Claude Code or Codex, reports about itself with OSC 7501 (see
// term.Status), while the agent_status setting is on. The terminal's
// sub-tab and tab show whether its agent works, needs the user, has
// finished, or stopped with an error. When it needs the user, finishes, or
// stops while the user can't see it, the terminal is marked as needing the
// user, with the agent's own words, as for a bell (see attention.go), and
// Bench posts a notification. None of it is saved.

// agentEcho is how long after an agent reports that it needs the user, has
// finished, or stopped with an error, a bell or a notification from its
// terminal is part of the same event. Agents often ring the bell, or ask
// for a notification, as they report, for terminals that don't know OSC
// 7501. Such an alert marks nothing more and posts nothing, so that one
// event gets one mark and one notification, with the agent's words. An
// alert just before the report, in an earlier batch, has posted its own
// notification already. The agent's notification then follows it, once,
// after a bell, or after a notification if it has the agent's words (see
// postAgentNoteLocked).
const agentEcho = 2 * time.Second

// What an agent's state means to the user, for its mark and its
// notification. The frontend says the same (agentLine in util.js).
const (
	agentNeedsText      = "Needs you"
	agentPermissionText = "Needs your permission"
	agentQuestionText   = "Has a question for you"
	agentAuthText       = "Needs you to sign in"
	agentDoneText       = "Finished"
	agentErrorText      = "Stopped with an error"
)

// The states of an agent in the View (ItemView.Agent).
const (
	agentWorking = "working"
	agentWaiting = "waiting" // blocked: it needs the user
	agentDone    = "done"
	agentError   = "error"
)

// agentStatus is what the agent in a terminal last reported (Item.agent).
// The zero value is none.
type agentStatus struct {
	// state is term.StateIdle, StateWorking, StateBlocked, StateDone, or
	// StateError, or "" for none.
	state string
	// kind is what a blocked agent needs, such as term.KindPermission, or
	// "" if it didn't say.
	kind string
	// msg is the msg of the agent's report. words is what the agent says,
	// for its mark and the View: msg, or if it had none, the text of the
	// notification it asked for with its report (see agentEchoLocked).
	msg, words string
	// at is when the agent last reported that it needs the user, has
	// finished, or stopped with an error.
	at time.Time
	// marked is set while the item's attention holds the agent's words.
	marked bool
	// notify is set while the agent's notification waits to be posted, at
	// the end of the batch of terminal events, or before then if its
	// session ends (see postAgentNoteLocked).
	notify bool
}

// agentText returns what an agent's state means to the user, such as
// "Needs your permission" for a blocked agent that needs permission, or ""
// for an agent that works, is idle, or has said nothing.
func agentText(state, kind string) string {
	switch state {
	case term.StateBlocked:
		switch kind {
		case term.KindPermission:
			return agentPermissionText
		case term.KindQuestion:
			return agentQuestionText
		case term.KindAuth:
			return agentAuthText
		}
		return agentNeedsText
	case term.StateDone:
		return agentDoneText
	case term.StateError:
		return agentErrorText
	}
	return ""
}

// agentLine returns what an agent's state means to the user, followed by
// its own words if withWords is set and it has any, such as "Needs your
// permission: approve Bash: npm test". The mark always has the words; a
// notification has them while the agent_words setting is on.
func agentLine(state, kind, words string, withWords bool) string {
	text := agentText(state, kind)
	if withWords && words != "" {
		return text + ": " + words
	}
	return text
}

// agentView returns an agent's state as the View has it (ItemView.Agent):
// working, waiting, done, or error, or "" for an agent that's idle or has
// said nothing.
func agentView(state string) string {
	switch state {
	case term.StateWorking:
		return agentWorking
	case term.StateBlocked:
		return agentWaiting
	case term.StateDone:
		return agentDone
	case term.StateError:
		return agentError
	}
	return ""
}

// agentShownLocked returns an item's agent's state as the View has it, or
// "" while the agent_status setting is off.
func (e *Engine) agentShownLocked(it *Item) string {
	if !e.cfg.Settings.AgentStatus {
		return ""
	}
	return agentView(it.agent.state)
}

// agentReportLocked applies what a terminal's agent reported, while the
// agent_status setting is on. The View changes, but nothing is saved.
func (e *Engine) agentReportLocked(it *Item, s term.Status) {
	if !e.cfg.Settings.AgentStatus {
		return
	}
	switch s.State {
	case term.StateClear:
		e.clearAgentLocked(it)
	case term.StateEnded:
		e.agentEndedLocked(it)
	case term.StateIdle, term.StateWorking:
		e.keepFinishedNoteLocked(it)
		e.unmarkWaitingLocked(it)
		it.agent = agentStatus{state: s.State, msg: s.Msg, words: s.Msg, marked: it.agent.marked}
		e.touch()
	case term.StateBlocked, term.StateDone, term.StateError:
		e.agentNeedsLocked(it, s)
	}
}

// agentNeedsLocked applies an agent's report that it needs the user, has
// finished, or stopped with an error. The same report again is the same
// event, which changes nothing more. A new one raises the item's count of
// events, with its line, seen or not, for VoiceOver (ItemView.AgentEvent).
// While the user can see the terminal, nothing is marked or posted: an
// agent that needs them shows as waiting, and one that finished or
// stopped shows nothing, as they saw it happen. Bench forgets the latter
// at once, but keeps its report for agentEcho (Item.agentSeen): until the
// agent reports that it works or is idle, the same report again within
// that time is still the same event. Otherwise the terminal is marked,
// with the agent's words, and its notification waits for the end of the
// batch of terminal events, for a notification that the agent may have
// asked for with the report, whose text becomes the words if the agent had
// none (see agentEchoLocked).
func (e *Engine) agentNeedsLocked(it *Item, s term.Status) {
	now := e.clk.Now()
	kind := ""
	if s.State == term.StateBlocked {
		kind = s.Kind
	}
	if a := it.agent; a.state == s.State && a.kind == kind && a.msg == s.Msg {
		it.agent.at = now
		return
	}
	if a := it.agentSeen; it.agent.state == "" && a.state == s.State && a.msg == s.Msg && now.Sub(a.at) < agentEcho {
		it.agentSeen.at = now
		return
	}
	// The new report's notification takes the place of the one that a
	// finished agent left, as its mark does.
	it.agentNote, it.agentSeen = agentStatus{}, agentStatus{}
	e.unmarkWaitingLocked(it)
	it.agent = agentStatus{state: s.State, kind: kind, msg: s.Msg, words: s.Msg, at: now, marked: it.agent.marked}
	why := agentLine(s.State, kind, s.Msg, true)
	it.agentEvent++
	it.agentEventLine = why
	e.touch()
	if unseen, _ := e.markLocked(it, why); !unseen {
		if s.State != term.StateBlocked {
			it.agent, it.agentSeen = agentStatus{}, it.agent
		}
		return
	}
	it.agent.marked = it.attention == why
	if !it.agent.notify {
		it.agent.notify = true
		e.agentNotes = append(e.agentNotes, it.ID)
	}
}

// agentEchoLocked reports whether an alert from a terminal, a bell or a
// notification, is part of what its agent reported within agentEcho:
// that it needs the user, has finished, or stopped with an error. That
// includes the report of an agent that finished or stopped, and then
// carried on or quit in the same batch, while its notification waits
// (Item.agentNote), as the alert came with that report. So does the report
// of an agent that finished or stopped while the user could see it, which
// Bench forgot at once, if it's still the latest event (Item.agentSeen).
// If the agent had no words, a notification's text becomes them, on the
// mark while it has the agent's line, in the agent's notification if
// that's still to be posted, and in the line of the event, which is that
// report's, without a new event. Its body is what the agent says; its
// title is only the name it gives itself.
func (e *Engine) agentEchoLocked(it *Item, a term.Alert) bool {
	ag, marked := &it.agent, it.agent.marked
	switch n := &it.agentNote; {
	case n.notify:
		ag, marked = n, it.attention == agentLine(n.state, n.kind, n.words, true)
	case it.agentSeen.state != "":
		ag, marked = &it.agentSeen, false
	}
	switch ag.state {
	case term.StateBlocked, term.StateDone, term.StateError:
	default:
		return false
	}
	if e.clk.Now().Sub(ag.at) >= agentEcho {
		return false
	}
	if ag.words == "" && !a.Bell && a.Body != "" {
		ag.words = a.Body
		line := agentLine(ag.state, ag.kind, ag.words, true)
		if marked {
			it.attention = line
		}
		it.agentEventLine = line
		e.touch()
	}
	return true
}

// postAgentNotesLocked posts the notifications that agents' reports in a
// batch of terminal events left waiting (see agentNeedsLocked).
func (e *Engine) postAgentNotesLocked() {
	ids := e.agentNotes
	e.agentNotes = nil
	for _, id := range ids {
		if it := e.model.Items[id]; it != nil {
			e.postAgentNoteLocked(it)
		}
	}
}

// postAgentNoteLocked posts the notification that a terminal's agent left
// waiting, if it has one, with what the agent needs, and its words while
// the agent_words setting is on. If the agent finished or stopped, and
// then carried on or quit in the same batch, it's the notification of that
// report (Item.agentNote). A bell or a notification that Bench posted for
// the terminal less than agentEcho before says less than the agent's
// notification: a bell says only that, and a notification doesn't have
// the agent's words. So the agent's notification still goes, although
// notifyGap hasn't passed, after a bell, and after a notification if it
// has the words. Without them, it would say less than the notification
// did. That happens once: a program still gets at most two notifications
// within notifyGap.
func (e *Engine) postAgentNoteLocked(it *Item) {
	a := it.agent
	switch {
	case it.agentNote.notify:
		a, it.agentNote = it.agentNote, agentStatus{}
	case a.notify:
		it.agent.notify = false
	default:
		return
	}
	withWords := e.cfg.Settings.AgentWords && a.words != ""
	if (withWords || it.alertBell) && !it.alertNoted.IsZero() && e.clk.Now().Sub(it.alertNoted) < agentEcho {
		it.notified = time.Time{}
	}
	e.notifyTerminalLocked(it, agentLine(a.state, a.kind, a.words, withWords))
}

// keepFinishedNoteLocked keeps the report of an agent that finished or
// stopped with an error, while its notification waits, before Bench
// forgets the agent or its next report takes its place, as when it
// carries on or quits at once (Item.agentNote). The mark of such an agent
// stays, so its notification goes too, at the end of the batch of terminal
// events, or as its session ends, with the words of a notification that
// the agent asked for with its report (see agentEchoLocked). A waiting
// agent's notification is dropped, as its mark is (see
// unmarkWaitingLocked).
func (e *Engine) keepFinishedNoteLocked(it *Item) {
	switch it.agent.state {
	case term.StateDone, term.StateError:
		if it.agent.notify {
			it.agentNote = it.agent
		}
	}
}

// unmarkWaitingLocked takes the mark off a terminal whose agent needed the
// user, now that it no longer does. The mark of an agent that finished or
// stopped stays until the user sees it.
func (e *Engine) unmarkWaitingLocked(it *Item) {
	if it.agent.marked && it.agent.state == term.StateBlocked {
		it.attention = ""
		it.agent.marked = false
		e.touch()
	}
}

// clearAgentLocked forgets what a terminal's agent reported, as when it
// clears its report, or the terminal runs a new program.
func (e *Engine) clearAgentLocked(it *Item) {
	if it.agent == (agentStatus{}) {
		return
	}
	e.keepFinishedNoteLocked(it)
	e.unmarkWaitingLocked(it)
	it.agent = agentStatus{}
	e.touch()
}

// agentEndedLocked follows the end of the program that reported, as the
// protocol asks: an agent that worked, needed the user, or was idle has
// gone, but one that finished or stopped with an error shows that until
// the user has seen it.
func (e *Engine) agentEndedLocked(it *Item) {
	switch it.agent.state {
	case term.StateDone, term.StateError:
		return
	}
	e.clearAgentLocked(it)
}

// agentSeenLocked forgets that a terminal's agent finished or stopped with
// an error once the user can see the terminal, and reports whether it did.
func (e *Engine) agentSeenLocked(it *Item) bool {
	switch it.agent.state {
	case term.StateDone, term.StateError:
		if e.seenLocked(it) {
			it.agent = agentStatus{}
			return true
		}
	}
	return false
}

// setAgentStatusLocked follows a change to the agent_status setting. The
// terminals' host answers agents' status checks, and passes on their
// reports, only while it's on; turning it off forgets every report, and
// takes the marks of agents that need the user off.
func (e *Engine) setAgentStatusLocked(was, on bool) {
	if was == on {
		return
	}
	e.terms.SetAgentStatus(on)
	if !on {
		for _, it := range e.model.Items {
			e.clearAgentLocked(it)
		}
		// A terminal that has gone carries its agent too, so the next
		// View doesn't list it.
		e.goneTerminals = nil
	}
}
