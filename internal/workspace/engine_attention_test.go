package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// attentionHarness starts the standard world with Bench active, showing
// the Code tab, whose terminal, w5, is in ~/code.
func attentionHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := newHarness(t, s)
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.p.ActivateApp(benchID))
	h.settle()
	return h
}

// alert reports an alert from a terminal's session, and lets the engine
// apply it.
func (h *harness) alert(id string, a term.Alert) {
	h.terms.cb.Alert(id, a)
	h.advance(2 * frame)
}

func TestTerminalBellMarksItsSubtabAndTab(t *testing.T) {
	h := attentionHarness(t, setup{})

	// A bell from the terminal the user can see marks nothing.
	h.alert("w5", term.Alert{Bell: true})
	if got := h.item("w5").Attention; got != "" || len(h.p.Notifications()) != 0 {
		t.Fatalf("a bell in view: attention %q, notifications %+v", got, h.p.Notifications())
	}

	// In another tab, it marks the terminal and its tab, counts on the
	// Dock icon, and posts a notification that says where it is.
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.alert("w5", term.Alert{Bell: true})
	v := h.view()
	if got := v.Items["w5"].Attention; got != "Rang the bell" {
		t.Errorf("attention: %q", got)
	}
	if v.Tabs[1].Attention != 1 || v.Tabs[0].Attention != 0 {
		t.Errorf("tab marks: %d, %d", v.Tabs[0].Attention, v.Tabs[1].Attention)
	}
	if h.p.Badge() != 1 {
		t.Errorf("badge: %d", h.p.Badge())
	}
	want := platform.Notification{ItemID: "w5", Title: "zsh — ~/code", Subtitle: "Code", Body: "Rang the bell."}
	if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
		t.Fatalf("notifications: %+v", n)
	}

	// Another bell doesn't post again while the terminal is marked.
	h.advance(notifyGap)
	h.alert("w5", term.Alert{Bell: true})
	if n := h.p.Notifications(); len(n) != 1 {
		t.Errorf("a second bell posted again: %+v", n)
	}

	// A notification its program asks for posts, and says so.
	h.alert("w5", term.Alert{Title: "Claude Code", Body: "Claude needs your permission"})
	if got := h.item("w5").Attention; got != "Notification: Claude Code: Claude needs your permission" {
		t.Errorf("attention: %q", got)
	}
	n := h.p.Notifications()
	if len(n) != 2 || n[1].Body != "Claude Code: Claude needs your permission" || n[1].Title != "zsh — ~/code" {
		t.Fatalf("notifications: %+v", n)
	}
	// But not again within notifyGap, though the mark says why.
	h.alert("w5", term.Alert{Body: "Done"})
	if got := h.item("w5").Attention; got != "Notification: Done" || len(h.p.Notifications()) != 2 {
		t.Errorf("within the gap: attention %q, %d notifications", got, len(h.p.Notifications()))
	}
	h.advance(notifyGap)
	h.alert("w5", term.Alert{Body: "Done again"})
	if len(h.p.Notifications()) != 3 {
		t.Errorf("after the gap: %d notifications", len(h.p.Notifications()))
	}

	// Showing it takes the mark off, and the badge.
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	v = h.view()
	if v.Items["w5"].Attention != "" || v.Tabs[1].Attention != 0 || h.p.Badge() != 0 {
		t.Errorf("after showing it: attention %q, tab %d, badge %d", v.Items["w5"].Attention, v.Tabs[1].Attention, h.p.Badge())
	}
}

func TestAttentionWhileBenchIsInTheBackground(t *testing.T) {
	h := attentionHarness(t, setup{})
	// The terminal is shown, but Bench isn't active.
	mustOK(t, h.p.ActivateApp(obsidianID))
	h.settle()
	h.alert("w5", term.Alert{Body: "Turn complete"})
	if got := h.item("w5").Attention; got != "Notification: Turn complete" {
		t.Fatalf("attention: %q", got)
	}
	if n := h.p.Notifications(); len(n) != 1 || n[0].Body != "Turn complete" {
		t.Errorf("notifications: %+v", n)
	}
	// Coming back to Bench takes the mark off what it shows.
	mustOK(t, h.p.ActivateApp(benchID))
	h.settle()
	if got := h.item("w5").Attention; got != "" || h.p.Badge() != 0 {
		t.Errorf("after coming back: attention %q, badge %d", got, h.p.Badge())
	}
}

func TestAttentionWhileBenchIsHidden(t *testing.T) {
	h := attentionHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.settle()
	h.alert("w5", term.Alert{Bell: true})
	if got := h.item("w5").Attention; got != bellText {
		t.Fatalf("attention: %q", got)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.settle()
	if got := h.item("w5").Attention; got != "" {
		t.Errorf("after Bench came back: %q", got)
	}
}

func TestTerminalNotificationsCanBeTurnedOff(t *testing.T) {
	h := attentionHarness(t, setup{config: strings.Replace(exampleConfig(t), "notify_terminals = true", "notify_terminals = false", 1)})
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.alert("w5", term.Alert{Body: "Done"})
	if got := h.item("w5").Attention; got != "Notification: Done" {
		t.Errorf("the mark stays: %q", got)
	}
	if n := h.p.Notifications(); len(n) != 0 {
		t.Errorf("notifications: %+v", n)
	}
	if h.view().Settings.NotifyTerminals {
		t.Error("the view should say notifications are off")
	}
}

// builtinID returns the ID of the one terminal in the view.
func (h *harness) builtinID() string {
	h.t.Helper()
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			return it.ID
		}
	}
	h.t.Fatal("there's no terminal")
	return ""
}

func TestCommandThatFinishesOutOfSightMarksItsTerminal(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	// Obsidian isn't in a tab.
	mustOK(t, h.p.ActivateApp(obsidianID))
	h.settle()
	id := h.builtinID()
	h.terms.end(id)
	h.terms.cb.Exit(id, errors.New("exit status 1"))
	h.advance(2 * frame)
	if got := h.item(id).Attention; got != "Its command ended with an error" {
		t.Errorf("attention: %q", got)
	}
	if n := h.p.Notifications(); len(n) != 1 || n[0].Title != "Tests" || n[0].Body != "Its command ended with an error." {
		t.Errorf("notifications: %+v", n)
	}
}

func TestAppDockedInTheTabCountsAsSeeingItsTerminal(t *testing.T) {
	h := newHarness(t, setup{config: commandConfig})
	id := h.builtinID()
	// The user types in Zed, docked beside the terminal, which is in view:
	// its bell marks nothing, and posts nothing.
	mustOK(t, h.p.ActivateApp(zedID))
	h.settle()
	h.alert(id, term.Alert{Bell: true})
	if got := h.item(id).Attention; got != "" || len(h.p.Notifications()) != 0 {
		t.Fatalf("a bell beside Zed: attention %q, notifications %+v", got, h.p.Notifications())
	}

	// From an app that isn't in the tab, it's marked; coming back to Zed
	// takes the mark off, and the badge.
	mustOK(t, h.p.ActivateApp(obsidianID))
	h.settle()
	h.alert(id, term.Alert{Bell: true})
	if got := h.item(id).Attention; got != bellText || len(h.p.Notifications()) != 1 || h.p.Badge() != 1 {
		t.Fatalf("a bell from Obsidian: attention %q, notifications %+v, badge %d", got, h.p.Notifications(), h.p.Badge())
	}
	mustOK(t, h.p.ActivateApp(zedID))
	h.settle()
	if got := h.item(id).Attention; got != "" || h.p.Badge() != 0 {
		t.Errorf("back in Zed: attention %q, badge %d", got, h.p.Badge())
	}
}

func TestWebNotificationMarksAPageOutOfSight(t *testing.T) {
	h := attentionHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebNotification, ItemID: "w1", Title: "Ann", Body: "Lunch?"})
	h.settle()
	v := h.view()
	if got := v.Items["w1"].Attention; got != "Notification: Ann: Lunch?" || v.Tabs[0].Attention != 1 {
		t.Errorf("attention: %q, tab %d", got, v.Tabs[0].Attention)
	}
	if n := h.p.Notifications(); len(n) != 1 || n[0] != (platform.Notification{ItemID: "w1", Title: "Ann", Body: "Lunch?"}) {
		t.Errorf("notifications: %+v", n)
	}

	// Clicking the notification shows the page, which takes the mark off.
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w1"})
	h.settle()
	v = h.view()
	if v.Current != 0 || v.Items["w1"].Attention != "" || h.p.Badge() != 0 {
		t.Errorf("after the click: tab %d, attention %q, badge %d", v.Current, v.Items["w1"].Attention, h.p.Badge())
	}
}

func TestClickingATerminalNotificationShowsIt(t *testing.T) {
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.alert("w5", term.Alert{Body: "Done"})

	// While Bench is hidden, the click brings it back, on the terminal's
	// tab.
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.settle()
	forwards := h.em.broughtForward()
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w5"})
	h.settle()
	if v := h.view(); v.Current != 1 || h.em.broughtForward() != forwards+1 {
		t.Errorf("after the click: tab %d, brought forward %d times", v.Current, h.em.broughtForward()-forwards)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.settle()
	if got := h.item("w5").Attention; got != "" {
		t.Errorf("attention: %q", got)
	}

	// A terminal in a window of its own has its window brought forward.
	mustOK(t, h.e.PopOut("w5"))
	h.settle()
	h.em.mu.Lock()
	popped := len(h.em.popped)
	h.em.mu.Unlock()
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w5"})
	h.settle()
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	if len(h.em.popped) != popped+1 || h.em.popped[len(h.em.popped)-1] != "w5" {
		t.Errorf("popped out: %v", h.em.popped)
	}
}

func TestPoppedOutTerminalIsntMarked(t *testing.T) {
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.PopOut("w5"))
	mustOK(t, h.p.ActivateApp(obsidianID))
	h.settle()
	// It has no sub-tab to mark, but posts a notification while Bench
	// is in the background.
	h.alert("w5", term.Alert{Body: "Done"})
	if got := h.item("w5").Attention; got != "" {
		t.Errorf("attention: %q", got)
	}
	if n := h.p.Notifications(); len(n) != 1 || n[0].Subtitle != "" {
		t.Errorf("notifications: %+v", n)
	}
}

func TestMarkComesOffAnItemThatLeavesItsTab(t *testing.T) {
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	h.alert("w5", term.Alert{Bell: true})
	mustOK(t, h.e.PopOut("w5"))
	h.settle()
	if got := h.item("w5").Attention; got != "" || h.p.Badge() != 0 {
		t.Errorf("after popping out: attention %q, badge %d", got, h.p.Badge())
	}
	if calls := h.calls("SetBadge"); !slices.EqualFunc(calls, []platform.Call{{Arg: "1"}, {Arg: "0"}},
		func(a, b platform.Call) bool { return a.Arg == b.Arg }) {
		t.Errorf("SetBadge calls: %+v", calls)
	}
}
