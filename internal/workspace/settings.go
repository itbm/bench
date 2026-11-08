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

// SaveSettings saves the Settings dialog: it applies [settings], the
// launchers, and the shortcuts that Settings ▸ Keyboard changed, updates the
// config, and saves config.toml when it can. Turning live previews on asks
// macOS for Screen Recording permission, if Bench doesn't have it, turning
// keep_terminal_output off removes the output Bench kept, and changed
// shortcuts are registered again. What Settings previewed (PreviewSettings)
// is then the setting. Turning check_for_updates on looks for a
// new version, if Bench hasn't looked in the last day, and turning it off
// takes the notice of one away (see updates.go).
func (e *Engine) SaveSettings(v SettingsView) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	cfg := e.cfg.Clone()
	if err := applySettings(&cfg.Settings, v); err != nil {
		e.unlock()
		return err
	}
	if v.Launchers != nil {
		launchers, err := launchersOf(v.Launchers, e.appName)
		if err != nil {
			e.unlock()
			return err
		}
		cfg.Launchers = launchers
	}
	if err := applyShortcuts(&cfg.Shortcuts, v.Shortcuts); err != nil {
		e.unlock()
		return err
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	if cfg.Settings.LivePreviews && !e.cfg.Settings.LivePreviews {
		e.askScreenRecordingLocked()
	}
	// Turning hide_other_tabs off shows what it hid.
	showOthers := e.cfg.Settings.HideOtherTabs && !cfg.Settings.HideOtherTabs
	if e.cfg.Settings.KeepTerminalOutput && !cfg.Settings.KeepTerminalOutput {
		e.forgetAllOutputLocked()
	}
	if cfg.Shortcuts != e.cfg.Shortcuts {
		e.later(e.em.ShortcutsChanged)
	}
	e.cfg = cfg
	// What Settings previewed is saved now.
	e.preview = nil
	e.scheduleUpdateCheckLocked()
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
	if v.TerminalFontSize != 0 {
		if !(v.TerminalFontSize >= config.MinTerminalFontSize && v.TerminalFontSize <= config.MaxTerminalFontSize) {
			return fmt.Errorf("The terminals' text size must be from %d to %d points.", config.MinTerminalFontSize, config.MaxTerminalFontSize)
		}
		s.TerminalFontSize = v.TerminalFontSize
	}
	if v.DropdownHeight != 0 {
		if v.DropdownHeight < config.MinDropdownHeight || v.DropdownHeight > config.MaxDropdownHeight {
			return fmt.Errorf("The drop-down's height must be from %d to %d percent of the screen.", config.MinDropdownHeight, config.MaxDropdownHeight)
		}
		s.DropdownHeight = v.DropdownHeight
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
	if v.CoveringWindows != "" {
		s.CoveringWindows = v.CoveringWindows
	}
	if v.MenuBar != "" {
		s.MenuBar = v.MenuBar
	}
	if v.TabShortcutHints != "" {
		s.TabShortcutHints = v.TabShortcutHints
	}
	if v.ScrollBars != "" {
		s.ScrollBars = v.ScrollBars
	}
	if v.Appearance != "" {
		s.Appearance = v.Appearance
	}
	if v.Density != "" {
		s.Density = v.Density
	}
	if v.PaneHeaders != "" {
		s.PaneHeaders = v.PaneHeaders
	}
	s.FillScreen = v.FillScreen
	s.ShowOnTabShortcut = v.ShowOnTabShortcut
	s.UnreadCounts = v.UnreadCounts
	s.CheckForUpdates = v.CheckForUpdates
	s.ConfirmQuitBusyTerminals = v.ConfirmQuitBusyTerminals
	s.ConfirmClose = v.ConfirmClose
	s.ShowStatusBar = v.ShowStatusBar
	s.OpenMissingApps = v.OpenMissingApps
	s.HideOtherTabs = v.HideOtherTabs
	s.HideOtherApps = v.HideOtherApps
	s.DragToAdd = v.DragToAdd
	s.LivePreviews = v.LivePreviews
	s.TerminalOptionAsMeta = v.TerminalOptionAsMeta
	s.NotifyTerminals = v.NotifyTerminals
	s.KeepTerminalOutput = v.KeepTerminalOutput
	s.LaunchTimeout = timeout
	return nil
}
