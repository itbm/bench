package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// stateOnDisk reads state.json and reports why it doesn't fit config.toml as
// saved, as LoadState would at the next start, without setting it aside.
func stateOnDisk(t *testing.T, h *harness) (*config.State, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	var st config.State
	mustOK(t, json.Unmarshal(data, &st))
	return &st, config.CheckState(&st, savedConfig(t, h))
}

func TestEditorSavesStateWithConfig(t *testing.T) {
	h := newHarness(t, setup{})
	// A rename that state.json missed would have it set aside at the next
	// start, so it's saved at once, not 500 ms later.
	mustOK(t, h.e.SaveTab(1, draftOf(h.view(), "Dev", "main-side", "stack")))
	st, err := stateOnDisk(t, h)
	if err != nil || st.Tabs[1].Name != "Dev" {
		t.Fatalf("straight after the save, state.json doesn't fit config.toml: %v", err)
	}
	mustOK(t, h.e.SaveTab(-1, draftOf(h.view(), "Notes", "single", "stack")))
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("after adding a tab: %v", err)
	}
	mustOK(t, h.e.DeleteTab(3))
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("after deleting a tab: %v", err)
	}
}

func TestEditorHoldsStateWhileConfigCantBeSaved(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SaveTab(1, draftOf(h.view(), "Dev", "main-side", "stack")))
	// A folder in the way of config.toml.bak stops config.toml being saved.
	bak := filepath.Join(h.dir, config.BackupFile)
	mustOK(t, os.RemoveAll(bak))
	mustOK(t, os.MkdirAll(filepath.Join(bak, "in-the-way"), 0o755))
	if err := h.e.SaveTab(1, draftOf(h.view(), "Build", "main-side", "stack")); err == nil {
		t.Fatal("SaveTab should report that config.toml couldn't be saved")
	}
	if !hasPrefix(h.view().Notices, "Bench couldn't save config.toml, so it isn't saving the arrangement") {
		t.Errorf("notices: %v", h.view().Notices)
	}
	// Later changes, and closing, leave state.json as config.toml's.
	mustOK(t, h.e.SwitchTab(2))
	h.advance(2 * time.Second)
	mustOK(t, h.e.Close())
	st, err := stateOnDisk(t, h)
	if err != nil || st.Tabs[1].Name != "Dev" {
		t.Fatalf("state.json no longer fits config.toml: %v", err)
	}

	// Once config.toml can be saved again, state.json follows it.
	h = newHarness(t, setup{})
	mustOK(t, os.RemoveAll(filepath.Join(h.dir, config.BackupFile)))
	mustOK(t, os.MkdirAll(filepath.Join(h.dir, config.BackupFile, "in-the-way"), 0o755))
	draft := draftOf(h.view(), "Build", "main-side", "stack")
	if err := h.e.SaveTab(1, draft); err == nil {
		t.Fatal("SaveTab should report that config.toml couldn't be saved")
	}
	mustOK(t, os.RemoveAll(filepath.Join(h.dir, config.BackupFile)))
	mustOK(t, h.e.SaveTab(1, draft))
	if st, err := stateOnDisk(t, h); err != nil || st.Tabs[1].Name != "Build" {
		t.Errorf("after config.toml was saved again: %v", err)
	}
	if hasPrefix(h.view().Notices, "Bench couldn't save config.toml") {
		t.Errorf("the notice should go: %v", h.view().Notices)
	}
	mustOK(t, h.e.MoveItem("w4", 1, "p12", "center"))
	h.advance(time.Second)
	if st, _ := stateOnDisk(t, h); st.Tabs[1].Preset != "custom" {
		t.Errorf("state.json should be saved after changes again: %+v", st.Tabs[1])
	}
}

func TestCloseWaitsForEditorSave(t *testing.T) {
	h := newHarness(t, setup{})
	// With config.toml a link to a pipe, the editor's save waits in its
	// read of the old file until the test writes to the pipe.
	path := filepath.Join(h.dir, config.ConfigFile)
	old, err := os.ReadFile(path)
	mustOK(t, err)
	pipe := filepath.Join(h.dir, "pipe")
	mustOK(t, syscall.Mkfifo(pipe, 0o644))
	mustOK(t, os.Remove(path))
	mustOK(t, os.Symlink(pipe, path))
	draft := draftOf(h.view(), "Dev", "main-side", "stack")
	saved := make(chan error, 1)
	go func() { saved <- h.e.SaveTab(1, draft) }()
	// Opening the pipe returns once the save is reading it.
	w, err := os.OpenFile(pipe, os.O_WRONLY, 0)
	mustOK(t, err)
	closed := make(chan error, 1)
	go func() { closed <- h.e.Close() }()
	early := false
	select {
	case <-closed:
		early = true
	case <-time.After(100 * time.Millisecond):
	}
	_, err = w.Write(old)
	mustOK(t, err)
	mustOK(t, w.Close())
	mustOK(t, <-saved)
	if early {
		t.Fatal("Close returned while the editor was saving config.toml, before state.json")
	}
	mustOK(t, <-closed)
	if st, err := stateOnDisk(t, h); err != nil || st.Tabs[1].Name != "Dev" {
		t.Errorf("state.json doesn't fit config.toml: %v", err)
	}
}

func TestStackViewSplitsSideBySideAfterSourceClosesUp(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(1))
	// Code is row[column[p11, w4's pane], p12].
	mustOK(t, h.e.MoveItem("w4", 1, "p11", "bottom"))
	// Dropping w4 on p11's right edge closes up the column, which brings
	// p11 into the top-level row, shown as a column.
	mustOK(t, h.e.MoveItem("w4", 1, "p11", "right"))
	v := h.view()
	fresh := v.Items["w4"].Pane
	var a, b layout.Rect
	for _, p := range v.Display.Panes {
		switch p.ID {
		case "p11":
			a = p.Rect
		case fresh:
			b = p.Rect
		}
	}
	if fresh == "p11" || b.X <= a.X || b.Y != a.Y {
		t.Errorf("the new pane %s at %v should be to the right of p11 at %v", fresh, b, a)
	}
}

func TestMoveToTabFocusesItemLeftBehind(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// p11 shows Zed, with VS Code behind it.
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItemToTab("w3", 0))
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if focus := h.focusCalls(); !slices.Contains(focus, code) {
		t.Errorf("VS Code, left showing in the focused pane, should be focused: %v", focus)
	}
	// Moving an item the focused pane doesn't show changes no focus.
	mustOK(t, h.e.FocusPane("p11"))
	h.p.ResetCalls()
	mustOK(t, h.e.MoveItemToTab("w5", 0))
	if focus := h.focusCalls(); len(focus) != 0 {
		t.Errorf("nothing should be focused: %v", focus)
	}
}

func TestPreviewOfWindowNotInATab(t *testing.T) {
	cfg := strings.Replace(string(config.ExampleTOML()), "live_previews = false", "live_previews = true", 1)
	h := newHarness(t, setup{config: cfg})
	h.p.SetScreenRecordingAllowed(true)
	finder := h.windowID(finderID, "Downloads")
	anchor := layout.Rect{X: 10, Y: 50, W: 120, H: 30}
	mustOK(t, h.e.ShowPreview(floatingID(finder), anchor))
	p, shown := h.p.PreviewShown()
	if !shown || p.Target.Window != finder || p.Target.Title != "Finder — Downloads" || p.Anchor != anchor {
		t.Errorf("preview: %+v %v", p, shown)
	}
	mustOK(t, h.e.HidePreview())
	mustOK(t, h.e.ShowPreview("f999", anchor))
	if _, shown := h.p.PreviewShown(); shown {
		t.Errorf("a window that has gone has no preview")
	}
}
