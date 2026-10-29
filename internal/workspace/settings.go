package workspace

import (
	"errors"
	"fmt"
	"time"

	"github.com/itbm/bench/internal/config"
)

// This file has the Settings dialog's method. Its settings, the [settings]
// table, apply to every tab; each tab's own properties are Tab properties'
// (editor.go).

// SaveSettings saves the Settings dialog: it applies [settings], updates the
// config, and saves config.toml when it can. Turning live previews on asks
// macOS for Screen Recording permission, if Bench doesn't have it.
func (e *Engine) SaveSettings(v SettingsView) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	cfg := e.cfg.Clone()
	if err := applySettings(&cfg.Settings, v); err != nil {
		e.unlock()
		return err
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	if cfg.Settings.LivePreviews && !e.cfg.Settings.LivePreviews {
		e.screenRec = e.p.ScreenRecordingAllowed(true)
	}
	// Turning hide_other_tabs off shows what it hid.
	showOthers := e.cfg.Settings.HideOtherTabs && !cfg.Settings.HideOtherTabs
	e.cfg = cfg
	e.arrangeLocked(arrangeOpts{showOtherTabs: showOthers})
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.unlock()

	// The tabs haven't changed, so state.json needn't wait for config.toml
	// (holdStateLocked). But if it's waiting already, because config.toml
	// couldn't be saved with an earlier change to the tabs, saving it now
	// lets state.json be saved too.
	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	if notSaved != "" {
		e.em.Toast(appliedText(notSaved))
		return nil
	}
	e.em.Toast("Saved settings")
	return nil
}

// applySettings copies the Settings dialog's settings into [settings].
func applySettings(s *config.Settings, v SettingsView) error {
	timeout := s.LaunchTimeout
	if v.LaunchTimeout != "" {
		d, err := time.ParseDuration(v.LaunchTimeout)
		if err != nil || d <= 0 {
			return errors.New("The launch timeout must be a length of time, such as 15s.")
		}
		timeout = config.Duration{Duration: d}
	}
	if v.CompactBelow > 0 {
		s.CompactBelow = v.CompactBelow
	}
	if v.Gap >= 0 {
		s.Gap = v.Gap
	}
	if v.NewWindows != "" {
		s.NewWindows = v.NewWindows
	}
	if v.FollowFocus != "" {
		s.FollowFocus = v.FollowFocus
	}
	if v.MenuBar != "" {
		s.MenuBar = v.MenuBar
	}
	s.FillScreen = v.FillScreen
	s.ShowOnTabShortcut = v.ShowOnTabShortcut
	s.OpenMissingApps = v.OpenMissingApps
	s.HideOtherTabs = v.HideOtherTabs
	s.HideOtherApps = v.HideOtherApps
	s.LivePreviews = v.LivePreviews
	s.LaunchTimeout = timeout
	return nil
}
