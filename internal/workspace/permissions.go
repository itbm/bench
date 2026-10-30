package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/update"
)

// This file has what Bench does about macOS's permissions besides asking
// for them: it takes the user to the switch in System Settings, since
// macOS asks only once; it tells the user when an update has made macOS
// stop applying Accessibility permission; it offers to quit and reopen
// Bench for Screen Recording permission, which macOS applies only as Bench
// opens; and it says when macOS needs the user to allow Bench's login
// item.

// trustedVersionFile records the version of Bench that last had
// Accessibility permission. Releases aren't notarised yet, so macOS takes
// each new version for a new app: System Settings still shows Bench turned
// on, but macOS doesn't apply it, until the user removes Bench from the
// list and turns it on again.
const trustedVersionFile = "accessibility-version"

// maxVersionText is the most of trustedVersionFile that Bench reads: more
// than any version's text.
const maxVersionText = 200

// readTrustedVersion returns the version that trustedVersionFile records,
// or "".
func (e *Engine) readTrustedVersion() string {
	if e.opts.Headless || e.opts.Demo {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(e.dir, trustedVersionFile))
	if err != nil || len(data) > maxVersionText {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// noteTrustedLocked records Bench's version as the one that last had
// Accessibility permission, unless it's recorded already.
func (e *Engine) noteTrustedLocked() {
	v := e.opts.Version
	if e.opts.Headless || e.opts.Demo || v == "" || e.trustedVersion == v {
		return
	}
	if err := os.WriteFile(filepath.Join(e.dir, trustedVersionFile), []byte(v+"\n"), 0o600); err != nil {
		e.log.Warn("couldn't record the version of Bench that has Accessibility permission", "error", err)
		return
	}
	e.trustedVersion = v
}

// updatedLocked reports whether Bench lacks Accessibility permission that
// an earlier version of it had: one that the support folder records, which
// isn't newer than this one. A build that isn't a release, such as "dev",
// counts if its text differs, as each build is new to macOS.
func (e *Engine) updatedLocked() bool {
	was, now := e.trustedVersion, e.opts.Version
	if e.trusted || was == "" || now == "" || was == now {
		return false
	}
	c, err := update.Compare(now, was)
	return err != nil || c >= 0
}

// accessibilityNoticeLocked is the status bar's notice while Bench doesn't
// have Accessibility permission, with a button that asks for it and opens
// System Settings at its list.
func (e *Engine) accessibilityNoticeLocked() NoticeView {
	text := accessibilityNotice
	if e.updatedLocked() {
		text = accessibilityUpdatedNotice
	}
	return NoticeView{Text: text, Actions: []NoticeAction{{Label: "Turn on Accessibility", Method: "RequestAccessibility"}}}
}

// openSettingsLocked opens System Settings at a pane, logging a failure:
// the button that asked has asked macOS too, or says what to do.
func (e *Engine) openSettingsLocked(pane platform.SettingsPane) {
	if err := e.p.OpenSettings(pane); err != nil {
		e.log.Warn("couldn't open System Settings", "pane", pane, "error", err)
	}
}

// askScreenRecordingLocked asks macOS for Screen Recording permission, and
// while Bench doesn't have it, opens System Settings at its list, where the
// user turns it on after the first time. Once it's asked, the view offers
// to quit and reopen Bench, which is when macOS applies it.
func (e *Engine) askScreenRecordingLocked() {
	e.screenRec = e.p.ScreenRecordingAllowed(true)
	if e.screenRec || e.opts.Demo || e.opts.Headless {
		return
	}
	e.screenRecAsked = true
	e.openSettingsLocked(platform.SettingsScreenRecording)
}

// permissionsLocked returns the view's permissions.
func (e *Engine) permissionsLocked() PermissionsView {
	return PermissionsView{
		Accessibility:         e.trusted,
		AccessibilityUpdated:  e.updatedLocked(),
		ScreenRecording:       e.screenRec,
		ScreenRecordingReopen: !e.screenRecAtStart && (e.screenRec || e.screenRecAsked),
	}
}

// OpenLoginItems opens System Settings ▸ General ▸ Login Items, where the
// user allows Bench's login item once macOS needs them to.
func (e *Engine) OpenLoginItems() error {
	if e.opts.LoginItem == nil {
		return errNoLoginItem
	}
	e.lock()
	defer e.unlock()
	if err := e.p.OpenSettings(platform.SettingsLoginItems); err != nil {
		return errCantOpenSettings
	}
	return nil
}

// recheckLogin reads whether Bench opens at login, and whether macOS needs
// the user to allow it, again, as Bench comes forward while macOS needs
// that: the user may have allowed it in System Settings meanwhile. Wails
// reads the login item's state with a command, so this runs on a goroutine
// of its own, without the lock.
func (e *Engine) recheckLogin() {
	li := e.opts.LoginItem
	if li == nil {
		return
	}
	enabled, err := li.IsEnabled()
	if err != nil {
		return
	}
	needs := !enabled && e.p.LoginItemNeedsApproval()
	e.lock()
	defer e.unlock()
	if e.openAtLogin != enabled || e.loginApproval != needs {
		e.openAtLogin, e.loginApproval = enabled, needs
		e.touch()
	}
}
