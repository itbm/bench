package workspace

import (
	"strings"
	"testing"
)

func TestSaveTabRefusesAGapWiderThanTheLimit(t *testing.T) {
	// A gap wider than the pane area would put the panes, and the windows
	// docked in them, off screen.
	h := newHarness(t, setup{})
	d := draftOf(h.view(), "Comms", "side", "stack")
	d.Settings.Gap = 8000
	if err := h.e.SaveTab(0, d); err == nil || !strings.Contains(err.Error(), "100 points") {
		t.Fatalf("SaveTab with a gap of 8000 points = %v, want an error about the gap", err)
	}
	if got := h.view().Settings.Gap; got != 8 {
		t.Errorf("gap after the refused save = %v, want 8", got)
	}
	if got := savedConfig(t, h).Settings.Gap; got != 8 {
		t.Errorf("config.toml's gap = %v, want 8", got)
	}
}
