package workspace

import (
	"reflect"
	"slices"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// noPreview is a preview that previews nothing.
var noPreview = SettingsPreview{Gap: -1}

// TestPreviewSettingsDrawsWithoutArranging checks that Settings' preview of
// the density and the space between panes changes the panes as the View
// draws them, and the View's density, but moves no window, changes no
// setting, and saves nothing. A preview of nothing puts the View back.
func TestPreviewSettingsDrawsWithoutArranging(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	zed := h.windowID(zedID, "bench — switch.go")
	before := h.view()
	at := h.frame(zed)
	h.p.ResetCalls()

	mustOK(t, h.e.PreviewSettings(SettingsPreview{Density: config.DensityCompact, Gap: 8}))
	h.settle()
	v := h.view()
	if v.Density != config.DensityCompact || v.TitleBar != config.DensityCompact {
		t.Errorf("density %q, title bar %q; want compact", v.Density, v.TitleBar)
	}
	frames := layout.Frames(v.Display.Root, v.Display.Area, 8)
	if len(frames) != len(v.Display.Panes) || len(frames) < 2 {
		t.Fatalf("%d panes drawn, want %d, at least 2", len(v.Display.Panes), len(frames))
	}
	for i, p := range v.Display.Panes {
		if p.Rect != frames[i].Rect || p.Header != layout.CompactHeaderHeight || p.Body != layout.Body(p.Rect, layout.CompactHeaderHeight) {
			t.Errorf("pane %s: %v, header %v, body %v; want %v with a compact header", p.ID, p.Rect, p.Header, p.Body, frames[i].Rect)
		}
	}
	if got, want := v.Display.Dividers, layout.Dividers(v.Display.Root, v.Display.Area, 8); !reflect.DeepEqual(got, want) {
		t.Errorf("dividers %v, want %v", got, want)
	}
	// The setting, the docked window, and config.toml stay as they were.
	if !reflect.DeepEqual(v.Settings, before.Settings) {
		t.Errorf("settings %+v, want %+v", v.Settings, before.Settings)
	}
	if got := h.frame(zed); got != at {
		t.Errorf("Zed moved to %v, want it at %v", got, at)
	}
	if calls := h.calls("SetFrame"); len(calls) != 0 {
		t.Errorf("the preview moved windows: %v", calls)
	}
	if cfg := savedConfig(t, h); cfg.Settings.Gap != 4 || cfg.Settings.Density != config.DensityAuto {
		t.Errorf("config.toml's gap %v and density %q, want 4 and auto", cfg.Settings.Gap, cfg.Settings.Density)
	}

	// Arranging meanwhile, as a window event does, docks to the saved
	// settings' bodies, not the preview's.
	mustOK(t, h.e.Rearrange())
	h.settle()
	if got := h.frame(zed); got != at {
		t.Errorf("Zed after Rearrange at %v, want it still at %v", got, at)
	}

	mustOK(t, h.e.PreviewSettings(noPreview))
	h.settle()
	after := h.view()
	if !reflect.DeepEqual(after.Display, before.Display) || after.Density != before.Density || after.TitleBar != before.TitleBar {
		t.Errorf("after the preview: display %+v, density %q; want %+v, %q", after.Display, after.Density, before.Display, before.Density)
	}
}

// TestPreviewSettingsGapZero checks that a gap of 0 can be previewed, as
// config.toml can set one: only a gap below 0 keeps the setting.
func TestPreviewSettingsGapZero(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PreviewSettings(SettingsPreview{Gap: 0}))
	if p := h.view().Display.Panes[0]; p.Rect.X != 0 {
		t.Errorf("first pane with a gap of 0 at %v, want x 0", p.Rect)
	}
	mustOK(t, h.e.PreviewSettings(SettingsPreview{Gap: -5}))
	if p := h.view().Display.Panes[0]; p.Rect.X != 4 {
		t.Errorf("first pane with no gap previewed at %v, want x 4", p.Rect)
	}
}

// TestPreviewSettingsAppearance checks that the previewed appearance
// reaches the shell, for each of Bench's windows, and the View's
// appearance, which the frontend's colours follow, while the setting stays
// as it was; and that it goes back when the preview stops.
func TestPreviewSettingsAppearance(t *testing.T) {
	h := newHarness(t, setup{})
	h.settle()
	mustOK(t, h.e.PreviewSettings(SettingsPreview{Appearance: config.AppearanceLight, Gap: -1}))
	h.settle()
	v := h.view()
	if v.Appearance != config.AppearanceLight || v.Settings.Appearance != config.AppearanceSystem {
		t.Errorf("appearance %q, setting %q; want light, system", v.Appearance, v.Settings.Appearance)
	}
	// The same preview again tells the shell nothing new.
	mustOK(t, h.e.PreviewSettings(SettingsPreview{Appearance: config.AppearanceLight, Gap: -1}))
	h.settle()
	mustOK(t, h.e.PreviewSettings(noPreview))
	h.settle()
	if got := h.view().Appearance; got != config.AppearanceSystem {
		t.Errorf("appearance after the preview %q, want system", got)
	}
	want := []string{config.AppearanceSystem, config.AppearanceLight, config.AppearanceSystem}
	if got := h.em.appearances(); !reflect.DeepEqual(got, want) {
		t.Errorf("appearances %q, want %q", got, want)
	}
}

// TestPreviewSettingsEnds checks what stops the preview: SaveSettings,
// which makes the settings saved what the View shows; lowering Bench, as
// when the last dialog closes; and Ready, as when the page loads again.
// A refused save keeps it, as Settings stays open.
func TestPreviewSettingsEnds(t *testing.T) {
	h := newHarness(t, setup{})
	preview := SettingsPreview{Appearance: config.AppearanceDark, Density: config.DensityCompact, Gap: 8}

	// Saved: the View shows the settings saved, not the preview.
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.PreviewSettings(preview))
	bad := h.view().Settings
	bad.NewWindows = "sideways"
	if err := h.e.SaveSettings(bad); err == nil {
		t.Fatal("a bad setting was saved")
	}
	if v := h.view(); v.Appearance != config.AppearanceDark || v.Display.Panes[0].Rect.X != 8 {
		t.Errorf("after a refused save: appearance %q, first pane %v; want the preview", v.Appearance, v.Display.Panes[0].Rect)
	}
	s := h.view().Settings
	s.Gap = 2
	mustOK(t, h.e.SaveSettings(s))
	v := h.view()
	if v.Appearance != config.AppearanceSystem || v.Density != config.DensityStandard || v.Display.Panes[0].Rect.X != 2 {
		t.Errorf("after saving: appearance %q, density %q, first pane %v; want system, standard, x 2", v.Appearance, v.Density, v.Display.Panes[0].Rect)
	}

	// Lowered.
	mustOK(t, h.e.PreviewSettings(preview))
	mustOK(t, h.e.SetOverlay(false))
	if v := h.view(); v.Appearance != config.AppearanceSystem || v.Display.Panes[0].Rect.X != 2 {
		t.Errorf("after lowering: appearance %q, first pane %v; want the settings", v.Appearance, v.Display.Panes[0].Rect)
	}

	// The page loaded again.
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.PreviewSettings(preview))
	v, err := h.e.Ready()
	mustOK(t, err)
	if v.Appearance != config.AppearanceSystem || v.Density != config.DensityStandard {
		t.Errorf("Ready: appearance %q, density %q; want the settings", v.Appearance, v.Density)
	}
}

// TestPreviewSettingsTitleBarInTheApp checks that, in the app, the View's
// title bar keeps the density it had at launch while the panes preview
// another, as it does for a saved density.
func TestPreviewSettingsTitleBarInTheApp(t *testing.T) {
	h := newHarness(t, setup{noStart: true})
	h.e.WindowStart()
	h.start()
	mustOK(t, h.e.PreviewSettings(SettingsPreview{Density: config.DensityCompact, Gap: -1}))
	if v := h.view(); v.TitleBar != config.DensityStandard || v.Density != config.DensityCompact {
		t.Errorf("title bar %q, density %q; want standard, compact", v.TitleBar, v.Density)
	}
}

// TestPreviewSettingsRefusals checks that a preview the settings can't
// have is refused, and changes nothing.
func TestPreviewSettingsRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	before := h.view()
	mustErr(t, h.e.PreviewSettings(SettingsPreview{Appearance: "sepia", Gap: -1}), errNoSuchAppearance.Error())
	mustErr(t, h.e.PreviewSettings(SettingsPreview{Density: "roomy", Gap: -1}), errNoSuchDensity.Error())
	mustErr(t, h.e.PreviewSettings(SettingsPreview{Gap: config.MaxGap + 1}), errGapTooWide.Error())
	after := h.view()
	if !reflect.DeepEqual(after.Display, before.Display) || after.Appearance != before.Appearance || after.Density != before.Density {
		t.Errorf("a refused preview changed the View")
	}
}

// TestPreviewSettingsWithPaneKeysAndEdges checks that Settings' preview of
// the space between panes and the density doesn't fight the keys that
// arrange the panes (decision 59) or a drag of a docked window's edge
// (decision 62). Each changes the splits' sizes, which the View draws with
// the gap and the headers previewed, while the windows dock to the bodies
// that the saved settings give, as with no preview. Ending the preview
// then moves no window.
func TestPreviewSettingsWithPaneKeysAndEdges(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeZed(t, h)
	split := h.tabNamed("Code").Root.ID
	mustOK(t, h.e.SetOverlay(true))
	preview := SettingsPreview{Density: config.DensityCompact, Gap: 8}
	mustOK(t, h.e.PreviewSettings(preview))
	h.settle()

	// saved returns p11's body as the saved settings place it, which the
	// View draws without the preview.
	saved := func() layout.Rect {
		t.Helper()
		mustOK(t, h.e.PreviewSettings(noPreview))
		b := h.bodyOf("p11")
		mustOK(t, h.e.PreviewSettings(preview))
		return b
	}
	// drawn checks that the View draws the panes with the preview.
	drawn := func(when string) {
		t.Helper()
		v := h.view()
		frames := layout.Frames(v.Display.Root, v.Display.Area, 8)
		if len(frames) != len(v.Display.Panes) {
			t.Fatalf("%s: %d panes drawn, want %d", when, len(v.Display.Panes), len(frames))
		}
		for i, p := range v.Display.Panes {
			if p.Rect != frames[i].Rect || p.Header != layout.CompactHeaderHeight {
				t.Errorf("%s: pane %s drawn at %v with a header of %v, want %v with a compact header", when, p.ID, p.Rect, p.Header, frames[i].Rect)
			}
		}
	}

	mustOK(t, h.e.FocusPane("p11"))
	mustOK(t, h.e.GrowPane())
	h.settle()
	if got := h.sizesOf("Code", split); !slices.Equal(got, []float64{70, 30}) {
		t.Errorf("after Grow Pane, sizes %v, want [70 30]", got)
	}
	drawn("after Grow Pane")
	if got, want := h.frame(zed), saved(); got != want {
		t.Errorf("after Grow Pane, Zed is at %v, want docked in p11's saved body at %v", got, want)
	}
	if got, drawnAt := h.frame(zed), h.bodyOf("p11"); got == drawnAt {
		t.Errorf("after Grow Pane, Zed is docked in the body that the preview draws, %v", drawnAt)
	}

	f := h.frame(zed)
	f.W += 200
	h.dragEdge(zed, f)
	got := h.sizesOf("Code", split)
	if got[0] <= 70 || !near(got[0]+got[1], 100) {
		t.Errorf("after the drag, sizes %v, want the first larger than 70", got)
	}
	wantToast(t, h, "Resized the panes.")
	drawn("after the drag")
	body := saved()
	if !near(body.X+body.W, f.X+f.W) {
		t.Errorf("p11's saved body ends at %v, want %v, where the user let go of Zed's edge", body.X+body.W, f.X+f.W)
	}
	if got := h.frame(zed); got != body {
		t.Errorf("after the drag, Zed is at %v, want docked in p11's saved body at %v", got, body)
	}

	// Discard: the View draws the saved settings, and no window moves.
	h.p.ResetCalls()
	mustOK(t, h.e.PreviewSettings(noPreview))
	h.settle()
	if calls := h.calls("SetFrame"); len(calls) != 0 {
		t.Errorf("ending the preview moved windows: %v", calls)
	}
	if got := h.frame(zed); got != h.bodyOf("p11") {
		t.Errorf("after the preview, Zed is at %v, want docked in p11 at %v", got, h.bodyOf("p11"))
	}
}
