package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
)

// dropdownHarness starts the standard world on the Code tab, whose
// terminal, w5, is in a pane of its own beside Zed's, and makes w5 the
// drop-down.
func dropdownHarness(t *testing.T, s setup) *harness {
	t.Helper()
	h := attentionHarness(t, s)
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	return h
}

// dropdownOf returns the view's drop-down, failing the test if there's none.
func dropdownOf(t *testing.T, h *harness) DropdownView {
	t.Helper()
	d := h.view().Dropdown
	if d == nil {
		t.Fatal("there's no drop-down")
	}
	return *d
}

func TestUseAsDropdown(t *testing.T) {
	h := attentionHarness(t, setup{})
	if h.view().Dropdown != nil {
		t.Fatal("a drop-down from the start")
	}
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()

	// The terminal leaves its pane, is out of the tabs, and shows at once,
	// at the default height, without a popped-out window.
	if d := dropdownOf(t, h); d.ID != "w5" || !d.Shown || d.InPage {
		t.Errorf("drop-down: %+v", d)
	}
	if it := h.item("w5"); it.Tab != -1 {
		t.Errorf("w5 is in tab %d", it.Tab)
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
	if len(h.em.popped) != 0 {
		t.Errorf("popped out: %v", h.em.popped)
	}
	if got := h.em.lastToast(); !strings.Contains(got, "is the drop-down now") || !strings.Contains(got, "View ▸ Show Drop-down") {
		t.Errorf("toast: %q", got)
	}

	// The shortcut hides it and shows it again, without bringing Bench
	// forward; its session goes on meanwhile.
	forwards := h.em.broughtForward()
	h.e.Shortcut(shortcuts.ActionDropdown, "")
	h.drain()
	if d := dropdownOf(t, h); d.Shown {
		t.Error("the shortcut didn't hide it")
	}
	if !h.terms.Running("w5") {
		t.Error("its session ended as it hid")
	}
	h.e.Shortcut(shortcuts.ActionDropdown, "")
	h.drain()
	if d := dropdownOf(t, h); !d.Shown {
		t.Error("the shortcut didn't show it again")
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide w5", "show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
	if h.em.broughtForward() != forwards {
		t.Error("the shortcut brought Bench forward")
	}
	// Choosing it again shows it, and changes nothing else.
	mustOK(t, h.e.HideDropdown())
	mustOK(t, h.e.UseAsDropdown("w5"))
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide w5", "show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
}

func TestUseAsDropdownOnlyForTerminals(t *testing.T) {
	h := newHarness(t, setup{})
	if err := h.e.UseAsDropdown("w1"); !errors.Is(err, errNotTerminal) {
		t.Errorf("a web page: %v", err)
	}
	if err := h.e.UseAsDropdown("w99"); !errors.Is(err, errGone) {
		t.Errorf("an unknown item: %v", err)
	}
	if h.view().Dropdown != nil || len(h.em.dropdownCalls()) != 0 {
		t.Error("a drop-down all the same")
	}
}

func TestUseAsDropdownWithTheShortcutOn(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), "\ndropdown = \"\"", "\ndropdown = \"ctrl+alt+d\"", 1)
	h := attentionHarness(t, setup{config: cfg})
	mustOK(t, h.e.UseAsDropdown("w5"))
	if got := h.em.lastToast(); !strings.Contains(got, "Press ⌃⌥D to show or hide it.") {
		t.Errorf("toast: %q", got)
	}
	if keys := h.view().Keys.Dropdown; keys != "⌃⌥D" {
		t.Errorf("keys: %q", keys)
	}
	h.advance(2 * frame)
	bars := h.em.menuBars()
	if d := bars[len(bars)-1].Dropdown; d.Shortcut != "Ctrl+Option+D" || !d.Shown || d.Label() != "Hide Drop-down" {
		t.Errorf("menu bar: %+v, %q", d, d.Label())
	}
}

func TestToggleDropdownMakesOne(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.ToggleDropdown())
	h.drain()
	d := dropdownOf(t, h)
	if !d.Shown {
		t.Error("the new drop-down doesn't show")
	}
	it := h.item(d.ID)
	if it.Kind != KindBuiltin || it.Tab != -1 || it.Folder != defaultFolder {
		t.Errorf("the new drop-down: %+v", it)
	}
	if opts, ok := h.terms.started[d.ID]; !ok || opts.Dir != defaultFolder {
		t.Errorf("its session: %v, %+v", ok, opts)
	}
	if !slices.ContainsFunc(h.view().Floating, func(f ItemView) bool { return f.ID == d.ID }) {
		t.Error("it isn't listed as out of the tabs")
	}
}

func TestEscapeHidesTheDropdownAtThePrompt(t *testing.T) {
	h := dropdownHarness(t, setup{})
	h.em.dropdownCalls()

	// While a program runs, Escape is the program's, at once.
	h.terms.setBusy("w5", "vim")
	mustOK(t, h.e.TermInput("w5", escapeKey))
	if !dropdownOf(t, h).Shown || h.terms.written["w5"] != escapeKey {
		t.Fatalf("Escape in vim: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	h.advance(time.Second)
	if !dropdownOf(t, h).Shown {
		t.Fatal("Escape in vim hid the drop-down")
	}
	// At the prompt, it waits a moment for another key, and with none, it
	// hides the drop-down, and isn't typed.
	h.terms.setBusy("w5", "")
	mustOK(t, h.e.TermInput("w5", escapeKey))
	h.advance(escapeWait - 10*time.Millisecond)
	if !dropdownOf(t, h).Shown || h.terms.written["w5"] != escapeKey {
		t.Fatalf("Escape at the prompt, at once: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	h.advance(10 * time.Millisecond)
	if dropdownOf(t, h).Shown || h.terms.written["w5"] != escapeKey {
		t.Errorf("Escape at the prompt: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide w5"}) {
		t.Errorf("shell calls: %q", got)
	}
	// An Escape that comes once it has hidden, as a second press can, is
	// typed.
	mustOK(t, h.e.TermInput("w5", escapeKey))
	if dropdownOf(t, h).Shown || h.terms.written["w5"] != escapeKey+escapeKey {
		t.Errorf("Escape while hidden: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("shell calls: %q", got)
	}
	// Escape with more, as Option sends with Use Option as Meta key, is
	// typed.
	mustOK(t, h.e.ToggleDropdown())
	h.terms.written["w5"] = ""
	mustOK(t, h.e.TermInput("w5", escapeKey+"b"))
	if !dropdownOf(t, h).Shown || h.terms.written["w5"] != escapeKey+"b" {
		t.Errorf("Option-B: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	// So is Escape in another terminal.
	other := addTerminal(t, h)
	mustOK(t, h.e.TermInput(other, escapeKey))
	if !dropdownOf(t, h).Shown || h.terms.written[other] != escapeKey {
		t.Errorf("Escape in %s: shown %v, written %q", other, dropdownOf(t, h).Shown, h.terms.written[other])
	}
}

func TestEscapeThenAKeyIsTyped(t *testing.T) {
	// The shell's Escape keys, such as Escape B, which moves back a word,
	// and Escape ., which types the last argument: a key straight after
	// Escape is typed after it, and the drop-down stays.
	h := dropdownHarness(t, setup{})
	h.em.dropdownCalls()
	mustOK(t, h.e.TermInput("w5", "echo a b"))
	mustOK(t, h.e.TermInput("w5", escapeKey))
	h.advance(escapeWait / 2)
	mustOK(t, h.e.TermInput("w5", "b"))
	if got := h.terms.written["w5"]; got != "echo a b"+escapeKey+"b" {
		t.Errorf("written %q", got)
	}
	h.advance(time.Second)
	if !dropdownOf(t, h).Shown || len(h.em.dropdownCalls()) != 0 {
		t.Errorf("Escape B hid the drop-down")
	}
	// Escape twice hides it at once, and neither is typed.
	h.terms.written["w5"] = ""
	mustOK(t, h.e.TermInput("w5", escapeKey))
	mustOK(t, h.e.TermInput("w5", escapeKey))
	if dropdownOf(t, h).Shown || h.terms.written["w5"] != "" {
		t.Errorf("Escape twice: shown %v, written %q", dropdownOf(t, h).Shown, h.terms.written["w5"])
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide w5"}) {
		t.Errorf("shell calls: %q", got)
	}
	// An Escape waiting as the drop-down hides some other way, here with
	// the shortcut, is forgotten: it doesn't hide the drop-down once it
	// shows again.
	mustOK(t, h.e.ToggleDropdown())
	mustOK(t, h.e.TermInput("w5", escapeKey))
	mustOK(t, h.e.ToggleDropdown())
	mustOK(t, h.e.ToggleDropdown())
	h.advance(time.Second)
	if !dropdownOf(t, h).Shown {
		t.Error("an Escape from before hid the drop-down")
	}
}

func TestEscapeInTheDropdownDoesntWaitForTheEngine(t *testing.T) {
	// An arrange that holds the engine's lock, as one waiting for an app
	// that doesn't answer can for a second, doesn't hold up an Escape, or
	// the keys after it: not for a program, nor at the prompt.
	h := dropdownHarness(t, setup{})
	typed := func(id, data string) bool {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- h.e.TermInput(id, data) }()
		select {
		case err := <-done:
			mustOK(t, err)
			return true
		case <-time.After(2 * time.Second):
			return false
		}
	}
	h.terms.setBusy("w5", "claude")
	h.e.lock()
	if !typed("w5", escapeKey) || !typed("w5", "x") {
		h.e.unlock()
		t.Fatal("Escape in a program waited for the engine's lock")
	}
	h.terms.setBusy("w5", "")
	if !typed("w5", escapeKey) || !typed("w5", "b") {
		h.e.unlock()
		t.Fatal("Escape at the prompt waited for the engine's lock")
	}
	h.e.unlock()
	if got := h.terms.written["w5"]; got != escapeKey+"x"+escapeKey+"b" {
		t.Errorf("written %q", got)
	}
}

func TestEscapeInATerminalThatWasTheDropdown(t *testing.T) {
	// Once the drop-down is back in a pane, Escape there is only typed,
	// without the engine's lock.
	for _, back := range []struct {
		name string
		do   func(h *harness) error
	}{
		{"Move into This Pane", func(h *harness) error {
			return h.e.AddToPane(h.view().Display.Focus, Source{Type: sourceFloating, ID: "w5"})
		}},
		{"Back to Bench", func(h *harness) error { return h.e.BackToBench("w5") }},
	} {
		t.Run(back.name, func(t *testing.T) {
			h := dropdownHarness(t, setup{})
			mustOK(t, back.do(h))
			h.drain()
			if keys, _ := h.e.dropKeys.Load().(string); keys != "" || h.e.dropShown {
				t.Errorf("the drop-down's keys: %q, shown %v", keys, h.e.dropShown)
			}
			h.e.lock()
			done := make(chan error, 1)
			go func() { done <- h.e.TermInput("w5", escapeKey) }()
			select {
			case err := <-done:
				h.e.unlock()
				mustOK(t, err)
			case <-time.After(2 * time.Second):
				h.e.unlock()
				t.Fatal("Escape waited for the engine's lock")
			}
			if got := h.terms.written["w5"]; got != escapeKey {
				t.Errorf("written %q", got)
			}
		})
	}
}

func TestDropdownMovesBackIntoAPane(t *testing.T) {
	h := dropdownHarness(t, setup{})
	pane := h.view().Display.Focus
	mustOK(t, h.e.AddToPane(pane, Source{Type: sourceFloating, ID: "w5"}))
	h.drain()
	if d := h.view().Dropdown; d != nil {
		t.Errorf("still the drop-down: %+v", d)
	}
	if it := h.item("w5"); it.Tab != 1 {
		t.Errorf("w5 is in tab %d", it.Tab)
	}
	if !slices.Equal(h.em.closed, []string{"w5"}) {
		t.Errorf("its window should close: %v", h.em.closed)
	}
	if !h.terms.Running("w5") {
		t.Error("its session ended")
	}
	// The shortcut makes a new one then.
	mustOK(t, h.e.ToggleDropdown())
	if d := dropdownOf(t, h); d.ID == "w5" {
		t.Error("the shortcut showed w5 from its pane")
	}
}

func TestDropdownBackToBench(t *testing.T) {
	h := dropdownHarness(t, setup{})
	forwards := h.em.broughtForward()
	mustOK(t, h.e.BackToBench("w5"))
	h.drain()
	if h.view().Dropdown != nil || h.item("w5").Tab != 1 || !slices.Equal(h.em.closed, []string{"w5"}) {
		t.Errorf("Back to Bench: drop-down %+v, tab %d, closed %v", h.view().Dropdown, h.item("w5").Tab, h.em.closed)
	}
	if h.em.broughtForward() == forwards {
		t.Error("Back to Bench should bring Bench forward")
	}
}

// addTerminal adds a terminal in ~/notes to the focused pane, and returns
// it.
func addTerminal(t *testing.T, h *harness) string {
	t.Helper()
	mustOK(t, h.e.AddToPane(h.view().Display.Focus, Source{Type: sourceTerminal, Folder: "~/notes"}))
	for id, it := range h.view().Items {
		if it.Kind == KindBuiltin && it.Folder == "~/notes" {
			return id
		}
	}
	t.Fatal("no new terminal")
	return ""
}

func TestUseAnotherTerminalAsDropdown(t *testing.T) {
	h := dropdownHarness(t, setup{})
	pane := h.view().Display.Focus
	added := addTerminal(t, h)
	h.em.dropdownCalls()
	mustOK(t, h.e.UseAsDropdown(added))
	h.drain()

	// The new one is the drop-down, and the old one goes back into Bench,
	// in the pane that the new one left, shown there.
	if d := dropdownOf(t, h); d.ID != added || !d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	if it := h.item("w5"); it.Tab != 1 || !paneHas(h, pane, "w5") {
		t.Errorf("w5: tab %d, panes %+v", it.Tab, h.view().Display.Panes)
	}
	if !paneShows(h, pane, "w5") {
		t.Errorf("pane %s should show w5", pane)
	}
	if !slices.Equal(h.em.closed, []string{"w5"}) {
		t.Errorf("w5's window should close: %v", h.em.closed)
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"show " + added + " 40"}) {
		t.Errorf("shell calls: %q", got)
	}
}

// paneHas reports whether a displayed pane has an item.
func paneHas(h *harness, paneID, itemID string) bool {
	for _, p := range h.view().Display.Panes {
		if p.ID == paneID {
			return slices.Contains(p.Items, itemID)
		}
	}
	return false
}

// paneShows reports whether a displayed pane shows an item.
func paneShows(h *harness, paneID, itemID string) bool {
	for _, p := range h.view().Display.Panes {
		if p.ID == paneID {
			return p.Active == itemID
		}
	}
	return false
}

func TestPoppedOutTerminalAsDropdown(t *testing.T) {
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.PopOut("w5"))
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	if d := dropdownOf(t, h); d.ID != "w5" || !d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	// Its own window closes, before the drop-down's opens, so that closing
	// it can't close the drop-down's.
	if got := h.em.windowCalls(); !slices.Equal(got, []string{"close w5", "show w5"}) {
		t.Errorf("window calls: %q", got)
	}
	n := 0
	for _, f := range h.view().Floating {
		if f.ID == "w5" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("w5 is listed %d times out of the tabs", n)
	}
}

func TestDropdownKeptBetweenLaunches(t *testing.T) {
	h := dropdownHarness(t, setup{})
	mustOK(t, h.e.Close())
	if st := reopenReadState(t, h.dir); st.Dropdown != "w5" || !slices.Contains(st.Floating, "w5") || !st.Items["w5"].OwnWindow {
		t.Fatalf("state.json: dropdown %q, floating %v, item %+v", st.Dropdown, st.Floating, st.Items["w5"])
	}
	h.em.dropdownCalls()
	popped := len(h.em.popped)
	h.restartTerms()

	// It's still the drop-down, out of sight, with its session running,
	// and no window until it shows.
	if d := dropdownOf(t, h); d.ID != "w5" || d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	if !h.terms.Running("w5") {
		t.Error("its session didn't start")
	}
	if len(h.em.popped) != popped || len(h.em.dropdownCalls()) != 0 {
		t.Errorf("a window opened: popped %v", h.em.popped)
	}
	mustOK(t, h.e.ToggleDropdown())
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
}

func TestDropdownBellWhileHidden(t *testing.T) {
	h := dropdownHarness(t, setup{})

	// While it shows, a bell marks nothing, even from another app.
	mustOK(t, h.p.ActivateApp(zedID))
	h.settle()
	h.alert("w5", term.Alert{Bell: true})
	if got := h.item("w5").Attention; got != "" || len(h.p.Notifications()) != 0 {
		t.Fatalf("a bell in view: attention %q, notifications %+v", got, h.p.Notifications())
	}

	// Hidden, a bell marks it, which the menu bar item shows, counts on
	// the Dock icon, and posts a notification that names the drop-down.
	mustOK(t, h.e.HideDropdown())
	h.alert("w5", term.Alert{Bell: true})
	if got := h.item("w5").Attention; got != bellText {
		t.Errorf("attention: %q", got)
	}
	bars := h.em.menuBars()
	if d := bars[len(bars)-1].Dropdown; d.Attention != bellText || d.Shown || d.Label() != "Show Drop-down — Needs Attention" {
		t.Errorf("menu bar: %+v, %q", d, d.Label())
	}
	if h.p.Badge() != 1 {
		t.Errorf("badge: %d", h.p.Badge())
	}
	want := platform.Notification{ItemID: "w5", Title: "zsh — ~/code", Subtitle: "Drop-down", Body: "Rang the bell."}
	if n := h.p.Notifications(); len(n) != 1 || n[0] != want {
		t.Fatalf("notifications: %+v", n)
	}

	// The mark stays while Bench comes forward and switches tabs, and
	// goes once the drop-down shows.
	mustOK(t, h.p.ActivateApp(benchID))
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	if got := h.item("w5").Attention; got != bellText {
		t.Errorf("attention after a switch: %q", got)
	}
	h.em.dropdownCalls()
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w5"})
	h.settle()
	if got := h.item("w5").Attention; got != "" || !dropdownOf(t, h).Shown {
		t.Errorf("after the notification's click: attention %q, shown %v", got, dropdownOf(t, h).Shown)
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
	bars = h.em.menuBars()
	if d := bars[len(bars)-1].Dropdown; d.Attention != "" || d.Label() != "Hide Drop-down" {
		t.Errorf("menu bar: %+v", d)
	}
	if h.p.Badge() != 0 {
		t.Errorf("badge: %d", h.p.Badge())
	}
}

func TestDropdownEnds(t *testing.T) {
	h := dropdownHarness(t, setup{})
	// The user closes its window, from ⌘W: it goes, as a popped-out
	// terminal does.
	h.e.TerminalWindowClosed("w5")
	h.drain()
	if h.view().Dropdown != nil {
		t.Error("still a drop-down")
	}
	if _, ok := h.view().Items["w5"]; ok {
		t.Error("w5 is still there")
	}
	if st := h.e.model.State(); st.Dropdown != "" {
		t.Errorf("state: %q", st.Dropdown)
	}
}

func TestDropdownHeightSetting(t *testing.T) {
	h := dropdownHarness(t, setup{})
	s := h.view().Settings
	if s.DropdownHeight != 40 {
		t.Fatalf("default height: %d", s.DropdownHeight)
	}
	for _, bad := range []int{19, 91, -5} {
		s.DropdownHeight = bad
		if err := h.e.SaveSettings(s); err == nil || !strings.Contains(err.Error(), "from 20 to 90 percent") {
			t.Errorf("height %d: %v", bad, err)
		}
	}
	s.DropdownHeight = 60
	mustOK(t, h.e.SaveSettings(s))
	if got := savedConfig(t, h).Settings.DropdownHeight; got != 60 {
		t.Errorf("config.toml: %d", got)
	}
	h.em.dropdownCalls()
	mustOK(t, h.e.HideDropdown())
	mustOK(t, h.e.ToggleDropdown())
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide w5", "show w5 60"}) {
		t.Errorf("shell calls: %q", got)
	}
	// Zero, as from an older page, keeps it.
	s.DropdownHeight = 0
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().Settings.DropdownHeight; got != 60 {
		t.Errorf("after a save without it: %d", got)
	}
}

func TestDropdownThatCantShow(t *testing.T) {
	h := attentionHarness(t, setup{})
	h.em.dropErr = errors.New("no window")
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	if d := dropdownOf(t, h); d.Shown {
		t.Error("it shows")
	}
	if got := h.em.lastToast(); got != "Bench couldn't show the drop-down: no window." {
		t.Errorf("toast: %q", got)
	}
}

func TestUndoLeavesTheDropdownAlone(t *testing.T) {
	// Use as Drop-down isn't a change that Undo takes back: Undo of a
	// change from before it leaves the drop-down as it is.
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.MoveTab(2, 0))
	h.drain()
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	h.em.dropdownCalls()
	closed := len(h.em.closed)
	mustOK(t, h.e.UndoLatest())
	h.drain()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("tabs %v: Undo didn't move Research back", got)
	}
	if d := dropdownOf(t, h); d.ID != "w5" || !d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	if it := h.item("w5"); it.Tab != -1 || len(it.Tabs) != 0 {
		t.Errorf("w5 is back in tab %d", it.Tab)
	}
	if got := h.em.dropdownCalls(); len(got) != 0 || len(h.em.closed) != closed {
		t.Errorf("shell calls %q, closed %v", got, h.em.closed)
	}
	if st := h.e.model.State(); st.Dropdown != "w5" || !st.Items["w5"].OwnWindow {
		t.Errorf("state: dropdown %q, w5 %+v", st.Dropdown, st.Items["w5"])
	}
}

func TestUndoAfterAnotherTerminalBecameTheDropdown(t *testing.T) {
	// w5 is the drop-down, the tabs change, and another terminal becomes
	// the drop-down, which puts w5 back in a pane. Undo of the change
	// leaves both as they are: the new drop-down stays the drop-down, and
	// w5 stays in its pane, not out of the tabs with no window.
	h := dropdownHarness(t, setup{})
	pane := h.view().Display.Focus
	added := addTerminal(t, h)
	mustOK(t, h.e.MoveTab(2, 0))
	h.drain()
	mustOK(t, h.e.UseAsDropdown(added))
	h.drain()
	h.em.dropdownCalls()
	mustOK(t, h.e.UndoLatest())
	h.drain()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research"}) {
		t.Errorf("tabs %v: Undo didn't move Research back", got)
	}
	if d := dropdownOf(t, h); d.ID != added || !d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	if it := h.item(added); it.Tab != -1 {
		t.Errorf("%s is back in tab %d", added, it.Tab)
	}
	if it := h.item("w5"); it.Tab != 1 || !paneHas(h, pane, "w5") {
		t.Errorf("w5: tab %d, panes %+v", it.Tab, h.view().Display.Panes)
	}
	if slices.ContainsFunc(h.view().Floating, func(f ItemView) bool { return f.ID == "w5" }) {
		t.Error("w5 is out of the tabs")
	}
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("shell calls %q", got)
	}
}

func TestNewTabFromWindowsLeavesTheDropdown(t *testing.T) {
	// The drop-down is out of the tabs, but it isn't one of the windows
	// that a tab can be made of: a page that sends it anyway, such as one
	// from before it became the drop-down, can't take it.
	h := dropdownHarness(t, setup{})
	finder := h.fid(finderID, "Downloads")
	preview, err := h.e.PreviewTabFromWindows("", []string{"w5", finder})
	mustOK(t, err)
	if preview.Apps != 1 || slices.Contains(layout.Items(preview.Root), "w5") {
		t.Errorf("preview: %d apps, items %v", preview.Apps, layout.Items(preview.Root))
	}
	mustOK(t, h.e.NewTabFromWindows(TabDraft{}, []string{"w5", finder}))
	h.drain()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Comms", "Code", "Research", "Finder"}) {
		t.Errorf("tabs %v", got)
	}
	if d := dropdownOf(t, h); d.ID != "w5" || !d.Shown {
		t.Errorf("drop-down: %+v", d)
	}
	if it := h.item("w5"); it.Tab != -1 {
		t.Errorf("w5 is in tab %d", it.Tab)
	}
	mustErr(t, h.e.NewTabFromWindows(TabDraft{}, []string{"w5"}), errWindowsGone.Error())
}

func TestShowingTheDropdownFromThePaletteKeepsTheKeyboard(t *testing.T) {
	// Zed has the keyboard, and the jump shortcut opens the palette over
	// it; the palette's Show Drop-down lowers Bench and shows the
	// drop-down. Zed doesn't get the keyboard back from the drop-down, as
	// it would if the palette had closed with nothing chosen.
	h := attentionHarness(t, setup{})
	mustOK(t, h.e.UseAsDropdown("w5"))
	mustOK(t, h.e.HideDropdown())
	mustOK(t, h.p.ActivateApp(zedID))
	h.settle()
	h.e.Shortcut(shortcuts.ActionJump, "")
	mustOK(t, h.p.ActivateApp(benchID))
	h.drain()
	mustOK(t, h.e.SetOverlay(true))
	h.p.ResetCalls()
	h.em.dropdownCalls()

	mustOK(t, h.e.SetOverlay(false))
	mustOK(t, h.e.ToggleDropdown())
	h.advance(time.Second)
	if got := h.focusCalls(); len(got) != 0 {
		t.Errorf("focus calls %v: Zed took the keyboard from the drop-down", got)
	}
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"show w5 40"}) {
		t.Errorf("shell calls: %q", got)
	}
}

// dropScreen returns the display that the drop-down last showed on.
func (h *harness) dropScreen() string {
	h.e.lock()
	defer h.e.unlock()
	return h.e.dropScreen
}

// TestDropdownFollowsBenchsDisplay checks the drop-down against Bench's
// move back to the display that the user chose (decisions 70 and 72). The
// shell puts the drop-down at the top of the display that Bench's window
// is on. One that shows on the monitor as it's unplugged shows again once
// the screens settle, on the built-in display, where macOS has put Bench.
// When the monitor comes back and Bench goes back to it, the drop-down
// that shows stays where it is, and the next show is on the monitor. A
// hidden drop-down isn't shown as a display goes.
func TestDropdownFollowsBenchsDisplay(t *testing.T) {
	h := returnHarness(t, setup{})
	mustOK(t, h.e.ToggleDropdown())
	h.drain()
	id := dropdownOf(t, h).ID
	show := "show " + id + " 40"
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{show}) {
		t.Fatalf("shell calls: %q", got)
	}
	if got := h.dropScreen(); got != dell.ID {
		t.Errorf("the drop-down showed on %q, want the monitor, where Bench is", got)
	}

	h.unplugDell()
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{show}) {
		t.Errorf("after the monitor went: shell calls %q, want the drop-down shown again", got)
	}
	if d := dropdownOf(t, h); !d.Shown {
		t.Error("after the monitor went, the drop-down doesn't show")
	}
	if got := h.dropScreen(); got != builtIn.ID {
		t.Errorf("after the monitor went, the drop-down is on %q, want the built-in display", got)
	}

	// The monitor comes back, and Bench goes back to it: the drop-down
	// that shows stays on the built-in display.
	h.p.PlugScreen(dell)
	h.settle()
	if got := h.em.benchMoves(); len(got) != 1 {
		t.Fatalf("after the monitor came back: moves %+v, want Bench to go back", got)
	}
	h.followMove()
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("as Bench went back: shell calls %q, want none", got)
	}
	// Hidden and shown again, it's on the monitor.
	mustOK(t, h.e.ToggleDropdown())
	mustOK(t, h.e.ToggleDropdown())
	h.drain()
	if got := h.em.dropdownCalls(); !slices.Equal(got, []string{"hide " + id, show}) {
		t.Errorf("hidden and shown: shell calls %q", got)
	}
	if got := h.dropScreen(); got != dell.ID {
		t.Errorf("after Bench went back, the drop-down showed on %q, want the monitor", got)
	}

	// Hidden, it stays hidden as the monitor goes.
	mustOK(t, h.e.HideDropdown())
	h.drain()
	h.em.dropdownCalls()
	h.unplugDell()
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("a hidden drop-down as the monitor went: shell calls %q, want none", got)
	}
	if d := dropdownOf(t, h); d.Shown {
		t.Error("the hidden drop-down shows after the monitor went")
	}
}

// TestComingForwardLeavesTheDropdown checks the drop-down against Bench
// putting away what covers it as it comes forward (decisions 68 and 72).
// The drop-down's panel is one of Bench's own windows, over Bench's
// content area while it shows, and doesn't cover Bench: Bench hides
// Finder, whose window covers it, but not itself, parks none of its own
// windows, and leaves the drop-down showing.
func TestComingForwardLeavesTheDropdown(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	panel := h.p.AddWindow(benchID, "Drop-down", platform.WithFrame(layout.Rect{W: monitor.Visible.W, H: 400}))
	h.drain()
	h.em.dropdownCalls()

	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Error("Finder's window covers Bench, so Finder should be hidden")
	}
	if h.p.IsHidden(benchID) {
		t.Error("Bench hid itself as it came forward")
	}
	if h.e.win.IsParked(panel.ID) || h.frame(panel.ID) != (layout.Rect{W: monitor.Visible.W, H: 400}) {
		t.Errorf("the drop-down's panel moved: parked %v, at %v", h.e.win.IsParked(panel.ID), h.frame(panel.ID))
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	if d := dropdownOf(t, h); !d.Shown {
		t.Error("the drop-down doesn't show after Bench came forward")
	}
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("shell calls: %q, want none", got)
	}
	if _, ok := h.view().Items[h.itemOf(panel.ID)]; ok {
		t.Error("the drop-down's panel became an item")
	}
}

// TestShowingTheDropdownIsntAnArrangement checks the drop-down against the
// timing of arrangements (decisions 67 and 72). Showing and hiding it,
// from its shortcut or a menu, moves no window in a tab, so it keeps no
// time, and isn't a tab shortcut's. Making a terminal the drop-down takes
// it out of its pane, which is a change made in Bench's menus. The
// drop-down terminal is in no tab: a tab switch while it shows leaves it
// showing, and parks only App windows, before it shows their apps.
func TestShowingTheDropdownIsntAnArrangement(t *testing.T) {
	h := attentionHarness(t, setup{})
	mark := markTimes(h)
	mustOK(t, h.e.UseAsDropdown("w5"))
	h.drain()
	if got := arrangeReasons(h, mark); !slices.Equal(got, []string{reasonOther}) {
		t.Errorf("Use as Drop-down: reasons %v, want %s", got, reasonOther)
	}

	mark = markTimes(h)
	h.e.Shortcut(shortcuts.ActionDropdown, "")
	h.e.Shortcut(shortcuts.ActionDropdown, "")
	mustOK(t, h.e.HideDropdown())
	mustOK(t, h.e.ToggleDropdown())
	h.advance(time.Second)
	if got := timesSince(h, mark); len(got) != 0 {
		t.Errorf("showing and hiding the drop-down kept times %+v, want none", got)
	}

	h.em.dropdownCalls()
	h.p.ResetCalls()
	mark = markTimes(h)
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if got := arrangeReasons(h, mark); len(got) == 0 || got[0] != reasonTab {
		t.Errorf("the switch: reasons %v, want %s", got, reasonTab)
	}
	if d := dropdownOf(t, h); !d.Shown {
		t.Error("the switch hid the drop-down")
	}
	if got := h.em.dropdownCalls(); len(got) != 0 {
		t.Errorf("the switch: shell calls %q, want none", got)
	}
	for _, c := range h.calls("Park") {
		if it := h.itemOf(c.Window); it == "w5" || (it != "" && h.item(it).Kind != KindApp) {
			t.Errorf("the switch parked %+v, which isn't an App window", c)
		}
	}
}

// TestWindowToolNoticeNamesTheDropdownShortcut checks the dropdown
// shortcut against the window tools' (decisions 69 and 72): ⌃⌥D is
// Rectangle's First Third in its recommended set, so the notice about
// Rectangle names Show or hide the drop-down too, after Bench's other
// shortcuts, as the dropdown shortcut binds after them, and Settings ▸
// Keyboard knows Rectangle's ⌃⌥D (WindowToolKeys).
func TestWindowToolNoticeNamesTheDropdownShortcut(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), "\ndropdown = \"\"", "\ndropdown = \"ctrl+alt+d\"", 1)
	h := newHarness(t, setup{config: cfg, world: withRectangle(recommended)})
	notices := toolNotices(h)
	if len(notices) != 1 {
		t.Fatalf("notices: %+v", h.view().Notices)
	}
	if want := "and Show or hide the drop-down (⌃⌥D). macOS doesn't warn"; !strings.Contains(notices[0].Text, want) ||
		!strings.Contains(notices[0].Text, "Zoom pane (⌃⌥↩), and Show") {
		t.Errorf("notice:\n%s\nwant it to end its list with %q", notices[0].Text, want)
	}
	keys := h.e.WindowToolKeys()
	if len(keys) != 1 || !slices.Contains(keys[0].Keys, "ctrl+alt+d") {
		t.Errorf("WindowToolKeys = %+v, want Rectangle's ⌃⌥D among them", keys)
	}
}
