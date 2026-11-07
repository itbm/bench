package workspace

import "github.com/itbm/bench/internal/config"

// This file has Settings' preview: while Settings is open, the appearance,
// density, and space between panes that it has chosen show at once, before
// they're saved (decision 66). The preview changes only what the View
// draws, and the appearance that the shell gives Bench's windows. The
// docked windows stay where the saved settings put them, behind a raised
// Bench, until Save applies the settings and arranges them again, so a
// choice tried and put back moves no other app's window.

// SettingsPreview is what Settings previews (PreviewSettings): the
// appearance, density, and gap that it has chosen and not saved yet. An
// empty Appearance or Density, or a Gap below 0, is the saved setting;
// with all three, nothing is previewed.
type SettingsPreview struct {
	Appearance string  `json:"appearance"`
	Density    string  `json:"density"`
	Gap        float64 `json:"gap"`
}

// PreviewSettings shows Settings' choices of appearance, density, and gap
// in the View, and gives Bench's windows the appearance, until Settings
// saves them or stops: a preview with none of them stops it, as do
// SaveSettings, SetOverlay(false), and Ready. Nothing is saved or
// arranged. A value that the setting can't have is refused.
func (e *Engine) PreviewSettings(p SettingsPreview) error {
	switch p.Appearance {
	case "", config.AppearanceSystem, config.AppearanceLight, config.AppearanceDark:
	default:
		return errNoSuchAppearance
	}
	switch p.Density {
	case "", config.DensityAuto, config.DensityStandard, config.DensityCompact:
	default:
		return errNoSuchDensity
	}
	if p.Gap > config.MaxGap {
		return errGapTooWide
	}
	if p.Gap < 0 {
		p.Gap = -1
	}
	e.lock()
	defer e.unlock()
	var next *SettingsPreview
	if p.Appearance != "" || p.Density != "" || p.Gap >= 0 {
		next = &p
	}
	if (next == nil && e.preview == nil) || (next != nil && e.preview != nil && *next == *e.preview) {
		return nil
	}
	e.preview = next
	e.changed()
	return nil
}

// endPreviewLocked stops Settings' preview, if there's one.
func (e *Engine) endPreviewLocked() {
	if e.preview == nil {
		return
	}
	e.preview = nil
	e.changed()
}

// lookLocked returns the appearance that Bench's windows have: the one
// that Settings previews, or else the setting.
func (e *Engine) lookLocked() string {
	if e.preview != nil && e.preview.Appearance != "" {
		return e.preview.Appearance
	}
	return e.cfg.Settings.Appearance
}

// shownDensityLocked returns the density in use as the View draws it: for
// the density that Settings previews, or else the setting.
func (e *Engine) shownDensityLocked() string {
	if e.preview != nil && e.preview.Density != "" {
		return densityFor(e.preview.Density, e.sizeClassLocked())
	}
	return e.densityLocked()
}

// shownTitleBarLocked returns the title bar's density as the View draws
// it. In the app, it keeps the density it had at launch (titleBarLocked);
// without a native title bar, it follows the density previewed too.
func (e *Engine) shownTitleBarLocked() string {
	if e.titleBar != "" {
		return e.titleBar
	}
	return e.shownDensityLocked()
}

// shownDisplayLocked returns the current tab as the View draws it: as
// arranged, or with the gap and the pane headers' density that Settings
// previews.
func (e *Engine) shownDisplayLocked() display {
	gap := e.cfg.Settings.Gap
	if e.preview != nil && e.preview.Gap >= 0 {
		gap = e.preview.Gap
	}
	return e.displayWithLocked(gap, e.shownDensityLocked())
}
