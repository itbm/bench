package workspace

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/windows"
)

func TestStateSavedAfterChanges(t *testing.T) {
	h := newHarness(t, setup{})
	path := filepath.Join(h.dir, config.StateFile)
	os.Remove(path)
	mustOK(t, h.e.SwitchTab(1))
	h.advance(400 * time.Millisecond)
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "center"))
	h.advance(400 * time.Millisecond)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("state.json should wait 500 ms after the last change")
	}
	h.advance(200 * time.Millisecond)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	var st config.State
	mustOK(t, json.Unmarshal(data, &st))
	if st.Current != 1 || st.Tabs[1].Preset != "custom" || st.Items["w3"].WindowID == 0 {
		t.Errorf("state: current %d preset %s", st.Current, st.Tabs[1].Preset)
	}
}

func TestCloseRestoresAndSaves(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.MoveItem("w5", 1, "p11", "center"))
	zed := h.windowID(zedID, "bench — switch.go")
	if !h.e.win.IsParked(zed) {
		t.Fatal("Zed should be parked")
	}
	os.Remove(filepath.Join(h.dir, config.StateFile))
	mustOK(t, h.e.Close())
	if h.e.win.IsParked(zed) || h.p.IsHidden(braveID) {
		t.Errorf("Close should put windows back and show hidden apps")
	}
	if !h.terms.closedAll {
		t.Errorf("Close should end the terminals")
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.StateFile)); err != nil {
		t.Errorf("Close should save state.json: %v", err)
	}
	mustOK(t, h.e.Close())
	if _, err := os.Stat(filepath.Join(h.dir, config.ParkedFile)); err == nil {
		t.Errorf("parked.json should be gone")
	}
	// Nothing happens after Close.
	if err := h.e.ArrangeOnce(t.Context(), "Code"); err == nil {
		t.Errorf("ArrangeOnce after Close should fail")
	}
}

func TestStateIsCoalesced(t *testing.T) {
	h := newHarness(t, setup{})
	h.advance(100 * time.Millisecond)
	before := h.em.stateCount()
	for i := 0; i < 5; i++ {
		mustOK(t, h.e.FocusPane("p9"))
		mustOK(t, h.e.FocusPane("p8"))
	}
	if h.em.stateCount() != before {
		t.Errorf("bench:state was sent while the lock was held or before a frame")
	}
	h.clk.Advance(frame)
	if got := h.em.stateCount(); got != before+1 {
		t.Errorf("one bench:state a frame: got %d", got-before)
	}
	h.clk.Advance(frame)
	if got := h.em.stateCount(); got != before+1 {
		t.Errorf("no change, no bench:state: got %d", got-before)
	}
	last := h.em.states[len(h.em.states)-1]
	if last.Tabs[0].Focus != "p8" {
		t.Errorf("the last state: focus %s", last.Tabs[0].Focus)
	}
}

func TestShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	tests := []struct {
		action, arg string
		check       func() bool
		what        string
	}{
		{shortcuts.ActionTab, "2", func() bool { return h.view().Current == 1 }, "tab 2"},
		{shortcuts.ActionNextTab, "", func() bool { return h.view().Current == 2 }, "next tab"},
		{shortcuts.ActionNextTab, "", func() bool { return h.view().Current == 0 }, "next tab wraps"},
		{shortcuts.ActionPreviousTab, "", func() bool { return h.view().Current == 2 }, "previous tab"},
		{shortcuts.ActionTab, "2", func() bool { return h.view().Current == 1 }, "tab 2 again"},
		{shortcuts.ActionFocusPane, "right", func() bool { return h.view().Display.Focus == "p12" }, "focus right"},
		{shortcuts.ActionFocusPane, "left", func() bool { return h.view().Display.Focus == "p11" }, "focus left"},
		{shortcuts.ActionNextWindow, "", func() bool { return h.view().Display.Panes[0].Active == "w4" }, "next window"},
		{shortcuts.ActionPreviousWindow, "", func() bool { return h.view().Display.Panes[0].Active == "w3" }, "previous window"},
		{shortcuts.ActionMoveWindow, "right", func() bool { return h.item("w3").Pane == "p12" }, "move right"},
		{shortcuts.ActionRearrange, "", func() bool { return h.em.lastToast() == "Rearranged Code" }, "rearrange"},
		{shortcuts.ActionTab, "9", func() bool { return h.em.lastToast() == "There's no tab 9." }, "tab 9 toasts"},
		{shortcuts.ActionFocusPane, "left", func() bool { return h.view().Display.Focus == "p11" }, "focus left again"},
		{shortcuts.ActionMoveWindow, "down", func() bool { return h.em.lastToast() == "There's nothing to move it next to." },
			"refusals toast"},
	}
	for _, tc := range tests {
		h.e.Shortcut(tc.action, tc.arg)
		if !tc.check() {
			t.Errorf("%s: didn't happen", tc.what)
		}
	}
	h.em.mu.Lock()
	h.em.commands = nil
	h.em.mu.Unlock()
	h.e.Shortcut(shortcuts.ActionJump, "")
	if !reflect.DeepEqual(h.em.commands, []Command{{Name: "toggle-palette"}}) {
		t.Errorf("jump: %v", h.em.commands)
	}
}

func TestSetNotices(t *testing.T) {
	h := newHarness(t, setup{})
	h.e.SetNotices("shortcuts", []string{"Control+Option+Space is taken."})
	h.e.SetNotices("other", []string{"Another."})
	if got := texts(h.view().Notices); !slices.Equal(got, []string{"Control+Option+Space is taken.", "Another."}) {
		t.Errorf("notices: %v", got)
	}
	h.e.SetNotices("shortcuts", []string{"One.", "Two."})
	if got := texts(h.view().Notices); !slices.Equal(got, []string{"One.", "Two.", "Another."}) {
		t.Errorf("replaced by source: %v", got)
	}
	h.e.SetNotices("shortcuts", nil)
	if got := texts(h.view().Notices); !slices.Equal(got, []string{"Another."}) {
		t.Errorf("cleared: %v", got)
	}
}

func TestStageManagerNotice(t *testing.T) {
	// On as Bench starts: the notice says so at once.
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetStageManager(true)
	}})
	if _, ok := noticeWith(h.view().Notices, "Stage Manager is on"); !ok {
		t.Fatalf("no Stage Manager notice at start: %v", texts(h.view().Notices))
	}
	if n, _ := noticeWith(h.view().Notices, "Stage Manager"); n.Text != stageManagerText || len(n.Actions) != 0 {
		t.Errorf("notice %+v, want %q with no buttons", n, stageManagerText)
	}
	// Turned off, as the platform finds when Bench becomes active.
	h.p.SetStageManager(false)
	h.p.EmitEvent(platform.Event{Kind: platform.StageManagerChanged})
	h.drain()
	if _, ok := noticeWith(h.view().Notices, "Stage Manager"); ok {
		t.Errorf("the notice stayed after Stage Manager went off: %v", texts(h.view().Notices))
	}
	// And on again.
	h.p.SetStageManager(true)
	h.p.EmitEvent(platform.Event{Kind: platform.StageManagerChanged})
	h.drain()
	if _, ok := noticeWith(h.view().Notices, "Stage Manager is on"); !ok {
		t.Errorf("no notice once Stage Manager came on: %v", texts(h.view().Notices))
	}
}

func TestNoStageManagerNoticeWhileItsOff(t *testing.T) {
	h := newHarness(t, setup{})
	if _, ok := noticeWith(h.view().Notices, "Stage Manager"); ok {
		t.Errorf("a Stage Manager notice while it's off: %v", texts(h.view().Notices))
	}
}

func TestEventLoop(t *testing.T) {
	clk := clock.Real()
	p := platform.NewFake(clk)
	p.SetScreens(monitor)
	standardWorld(p)
	p.DrainEvents()
	em := &recEmitter{}
	e, err := New(Options{Platform: p, Clock: clk, Emitter: em, Dir: t.TempDir(),
		NewTerms: func(cb term.Callbacks) term.Host { return newFakeTerms() }})
	mustOK(t, err)
	e.Start()
	defer e.Close()
	_, err = p.OpenWindow(finderID, "Documents")
	mustOK(t, err)
	// app-pane puts a new window of an app with no items in the focused pane.
	added := func() bool {
		for _, it := range e.View().Items {
			if it.Title == "Documents" {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !added() {
		time.Sleep(5 * time.Millisecond)
	}
	if !added() {
		t.Fatalf("the loop didn't handle WindowCreated")
	}
	for time.Now().Before(deadline) && em.stateCount() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if em.stateCount() == 0 {
		t.Errorf("no bench:state was sent")
	}
	mustOK(t, e.Close())
}

func TestUserFacingErrors(t *testing.T) {
	h := newHarness(t, setup{})
	tests := []struct {
		err  error
		want string
	}{
		{h.e.SwitchTab(6), "There's no tab 7."},
		{h.e.MoveItemToTab("w99", 1), "That window has closed."},
		{h.e.AddToPane("p8", Source{Type: "web", URL: "http://example.com"}), "Bench shows http pages only from this Mac, such as localhost. Try https://example.com."},
		{h.e.PopOut("w99"), "That window has closed."},
		{h.e.Redock("w99"), "That window has closed."},
		{h.e.FocusPane("p99"), "There's no such pane."},
	}
	for _, tc := range tests {
		mustErr(t, tc.err, tc.want)
	}
}

// TestDialogSizesAreRemembered checks that a dialog's size, which the user
// sets by resizing it, is in the View, saved in state.json, and back after a
// restart, and that sizes for no dialog, or that aren't sizes, are refused.
func TestDialogSizesAreRemembered(t *testing.T) {
	h := newHarness(t, setup{})
	if got := h.view().Dialogs; got != nil {
		t.Fatalf("with nothing resized, dialogs = %v, want none", got)
	}
	mustOK(t, h.e.SetDialogSize("settings", 640.4, 519.6))
	mustOK(t, h.e.SetDialogSize("palette", 720, 360))
	want := map[string]DialogSize{"settings": {W: 640, H: 520}, "palette": {W: 720, H: 360}}
	if got := h.view().Dialogs; !maps.Equal(got, want) {
		t.Errorf("dialogs = %v, want %v", got, want)
	}

	mustErr(t, h.e.SetDialogSize("notices", 400, 300), "There's no such dialog.")
	for _, size := range [][2]float64{{0, 300}, {400, -1}, {math.NaN(), 300}, {400, math.Inf(1)}, {30000, 300}} {
		mustErr(t, h.e.SetDialogSize("editor", size[0], size[1]), "That isn't a size for a dialog.")
	}

	h.advance(retitleSaveDelay)
	st, _, err := config.LoadState(filepath.Join(h.dir, config.StateFile), savedConfig(t, h))
	mustOK(t, err)
	saved := map[string]config.DialogSize{"settings": {W: 640, H: 520}, "palette": {W: 720, H: 360}}
	if !maps.Equal(st.Dialogs, saved) {
		t.Errorf("state.json's dialogs = %v, want %v", st.Dialogs, saved)
	}
	h.restart()
	if got := h.view().Dialogs; !maps.Equal(got, want) {
		t.Errorf("after a restart, dialogs = %v, want %v", got, want)
	}
}

func TestCloseWithoutAccessibilityKeepsParkedFile(t *testing.T) {
	// A crashed run left a window parked, and this run never gets
	// Accessibility permission: parked.json must survive for the next run
	// (or the watchdog) to put the window back.
	parked := windows.ParkedFile{Windows: []windows.ParkedWindow{
		{ID: 99, BundleID: braveID, Title: "Wails v3 docs", Frame: layout.Rect{X: 10, Y: 30, W: 800, H: 600}},
	}}
	path := ""
	h := newHarness(t, setup{untrusted: true, state: func(t *testing.T, dir string) {
		path = filepath.Join(dir, config.ParkedFile)
		mustOK(t, windows.WriteParked(path, parked))
	}})
	mustOK(t, h.e.Close())
	got, err := windows.ReadParked(path)
	if err != nil || len(got.Windows) != 1 || got.Windows[0].ID != 99 {
		t.Errorf("parked.json after Close: %+v, %v", got, err)
	}
}

func TestOlderStateSnapshotNeverOverwritesNewer(t *testing.T) {
	h := newHarness(t, setup{})
	path := filepath.Join(h.dir, config.StateFile)
	h.e.lock()
	older, olderSeq := h.e.snapshotLocked()
	h.e.model.Current = 2
	newer, newerSeq := h.e.snapshotLocked()
	h.e.unlock()
	// The newer snapshot (Close's) is written first, then a save timer's
	// older one finishes.
	mustOK(t, h.e.writeState(newer, newerSeq))
	mustOK(t, h.e.writeState(older, olderSeq))
	st, _, err := config.LoadState(path, h.e.cfg)
	if err != nil || st == nil || st.Current != 2 {
		t.Fatalf("state.json: %+v, %v; want the newer snapshot", st, err)
	}
}

// TestConcurrentUse runs bound methods, platform events through the real
// loop, and timers at once, so that the race detector and a deadline can
// catch lock mistakes.
func TestConcurrentUse(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	p := platform.NewFake(clk)
	p.SetScreens(monitor)
	standardWorld(p)
	p.DrainEvents()
	terms := newFakeTerms()
	e, err := New(Options{Platform: p, Clock: clk, Emitter: &recEmitter{}, Dir: t.TempDir(),
		NewTerms: func(cb term.Callbacks) term.Host { terms.cb = cb; return terms }})
	mustOK(t, err)
	e.Start()
	mustOK(t, e.SetPaneArea(paneArea))

	const rounds = 100
	var wg sync.WaitGroup
	run := func(f func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				f(i)
			}
		}()
	}
	run(func(i int) { _ = e.SwitchTab(i % 3) })
	run(func(i int) { _ = e.CycleSubtab(1); _ = e.FocusDirection("right"); _ = e.MoveDirection("left") })
	run(func(i int) { _ = e.SetOverlay(i%2 == 0); _ = e.View() })
	run(func(i int) {
		_, _ = e.AddOptions("p8")
		_, _ = e.PreviewTree(0, layout.PresetGrid)
		_ = e.ShowPreview("w1", layout.Rect{})
	})
	run(func(i int) {
		w, err := p.OpenWindow(finderID, fmt.Sprintf("Folder %d", i))
		if err == nil && i%2 == 0 {
			_ = p.CloseWindow(w.ID)
		}
	})
	run(func(i int) {
		if terms.cb.Data != nil {
			terms.cb.Data("w5", int64(i), []byte("x"))
		}
		clk.Advance(5 * time.Millisecond)
	})
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(20 * time.Second):
		t.Fatal("the engine deadlocked")
	}
	mustOK(t, e.Close())
	for _, tab := range e.View().Tabs {
		if err := layout.Validate(tab.Root); err != nil {
			t.Errorf("%s: %v", tab.Name, err)
		}
	}
}
