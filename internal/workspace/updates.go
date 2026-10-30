package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/itbm/bench/internal/update"
)

// This file looks for a newer version of Bench while the check_for_updates
// setting is on, which it's not unless the user turns it on.
//
// In a Bench.app that has an updater (Options.Updater, which is Sparkle),
// the updater looks, at most once a day, in the appcast of the latest
// release. Bench turns its daily look on and off with the setting. When it
// finds a newer version, a notice says so, with Install, which shows the
// updater's window for it, and Dismiss. The updater downloads the new
// version, checks its signature, and installs it as Bench quits and
// reopens. Bench ▸ Check for Updates… asks the updater to look at once,
// with or without the setting.
//
// Without an updater, Bench asks GitHub for the latest release itself
// (Options.LatestRelease), at most once a day, and the notice has
// Download, which opens the release's page in the browser, and Dismiss.
// It downloads and installs nothing itself, and Check for Updates… opens
// the latest release's page.
//
// After Dismiss, Bench doesn't announce that version again. state.json
// keeps when Bench last looked itself, and what it or the updater found.

// Updater installs new versions of Bench (Options.Updater). Its methods
// return at once, and the engine calls them without its lock. It tells the
// engine what it finds with FoundUpdate and UpdateFinished.
type Updater interface {
	// SetAutomatic turns the updater's daily look for a new version on or
	// off. The updater keeps when it last looked.
	SetAutomatic(on bool)
	// Check looks for a new version now, and shows the updater's window
	// with what it finds: a new version, to install, or that Bench is up
	// to date. If the updater has found a new version already, it shows
	// that one.
	Check()
}

// errNoUpdater is InstallUpdate's error in a Bench that has no updater.
var errNoUpdater = errors.New("This build of Bench can't install updates. Download the new version from GitHub.")

// updateEvery is how often Bench looks for a newer version.
const updateEvery = 24 * time.Hour

// updateWait is how long Bench waits for GitHub to answer.
const updateWait = 30 * time.Second

// sourceUpdates is the source of the notice of a newer version.
const sourceUpdates = "updates"

// scheduleUpdateCheckLocked looks for a newer version once that's due,
// while check_for_updates is on: at once if Bench hasn't looked in the last
// day, or else a day after it last did. With the setting off, it stops
// looking, and the notice goes. With an updater, it turns the updater's
// daily look on or off instead.
func (e *Engine) scheduleUpdateCheckLocked() {
	if e.updateTimer != nil {
		e.updateTimer.Stop()
		e.updateTimer = nil
	}
	e.updateNoticeLocked()
	if e.opts.Headless || !e.started || e.closed {
		return
	}
	if u := e.opts.Updater; u != nil {
		// The updater looks once a day itself, and only Bench's setting
		// says whether it may.
		on := e.cfg.Settings.CheckForUpdates
		e.later(func() { u.SetAutomatic(on) })
		return
	}
	if !e.cfg.Settings.CheckForUpdates || e.opts.LatestRelease == nil {
		return
	}
	// A clock set back since doesn't put the next look off for longer.
	wait := min(max(e.updates.Checked.Add(updateEvery).Sub(e.clk.Now()), 0), updateEvery)
	e.updateTimer = e.after(wait, func() {
		e.updateTimer = nil
		e.checkUpdatesLocked()
	})
}

// checkUpdatesLocked asks GitHub for the latest release, on a goroutine of
// its own, as it waits for the network, and looks again a day later. A
// look that fails counts, so that Bench asks at most once a day; it's only
// logged.
func (e *Engine) checkUpdatesLocked() {
	e.updates.Checked = e.clk.Now()
	e.scheduleLazySaveLocked()
	latest, ctx := e.opts.LatestRelease, e.ctx
	go func() {
		ctx, cancel := context.WithTimeout(ctx, updateWait)
		defer cancel()
		r, err := latest(ctx)
		e.lock()
		defer e.unlock()
		if e.closed {
			return
		}
		if err != nil {
			e.log.Warn("couldn't look for a new version of Bench", "error", err)
			return
		}
		e.foundReleaseLocked(r)
	}()
	e.scheduleUpdateCheckLocked()
}

// foundReleaseLocked records the latest release, if it's newer than this
// version, for the notice.
func (e *Engine) foundReleaseLocked(r update.Release) {
	if !e.newerLocked(r.Version) {
		r = update.Release{}
	}
	if e.updates.Latest == r.Version && e.updates.URL == r.URL {
		return
	}
	e.updates.Latest, e.updates.URL = r.Version, r.URL
	e.updateNoticeLocked()
	e.changed()
}

// newerLocked reports whether a version is newer than this one. A build
// that isn't a release, such as "dev", has no newer version.
func (e *Engine) newerLocked(version string) bool {
	c, err := update.Compare(version, e.opts.Version)
	return err == nil && c > 0
}

// updateNoticeLocked shows the notice of a newer version, while
// check_for_updates is on and there is one that the user hasn't dismissed,
// or takes it away. With an updater, the notice offers to install it;
// without one, to download it.
func (e *Engine) updateNoticeLocked() {
	u := e.updates
	if !e.cfg.Settings.CheckForUpdates || u.Latest == "" || u.Latest == u.Dismissed || !e.newerLocked(u.Latest) {
		e.setNoticeViewsLocked(sourceUpdates, nil)
		return
	}
	if e.opts.Updater != nil {
		e.setNoticeViewsLocked(sourceUpdates, []NoticeView{{
			Text: "Bench " + u.Latest + " is out.",
			Actions: []NoticeAction{
				{Label: "Install…", Method: "InstallUpdate"},
				{Label: "Dismiss", Method: "DismissUpdate"},
			},
		}})
		return
	}
	address := u.URL
	if address == "" {
		address = update.ReleasesURL
	}
	e.setNoticeViewsLocked(sourceUpdates, []NoticeView{{
		Text: "Bench " + u.Latest + " is out. To update, download it and replace Bench in Applications.",
		Actions: []NoticeAction{
			{Label: "Download", Method: "OpenLink", Arg: address},
			{Label: "Dismiss", Method: "DismissUpdate"},
		},
	}})
}

// DismissUpdate is the notice's Dismiss: the notice of a newer version
// goes, and Bench doesn't announce that version again.
func (e *Engine) DismissUpdate() error {
	e.lock()
	defer e.unlock()
	if e.updates.Latest == "" || e.updates.Dismissed == e.updates.Latest {
		return nil
	}
	e.updates.Dismissed = e.updates.Latest
	e.updateNoticeLocked()
	e.changed()
	return nil
}

// FoundUpdate is the updater's news of a newer version, which its daily
// look found: the notice says so.
func (e *Engine) FoundUpdate(version string) {
	e.lock()
	defer e.unlock()
	if e.closed || e.opts.Updater == nil {
		return
	}
	e.foundReleaseLocked(update.Release{Version: version})
}

// UpdateFinished is the updater's news that it's done with the version it
// found: the user installed it, skipped it, or put it off in the updater's
// window, or it failed. The notice goes. If the user put it off, the
// updater finds it again.
func (e *Engine) UpdateFinished() {
	e.lock()
	defer e.unlock()
	if e.closed || e.opts.Updater == nil || e.updates.Latest == "" {
		return
	}
	e.updates.Latest, e.updates.URL = "", ""
	e.updateNoticeLocked()
	e.changed()
}

// InstallUpdate is the notice's Install…: it shows the updater's window for
// the newer version, which offers to install it.
func (e *Engine) InstallUpdate() error {
	u := e.opts.Updater
	if u == nil {
		return errNoUpdater
	}
	u.Check()
	return nil
}

// CheckForUpdates is Bench ▸ Check for Updates…: the updater looks for a
// newer version at once, and shows what it finds. Without an updater, it
// opens the latest release's page on GitHub.
func (e *Engine) CheckForUpdates() error {
	if u := e.opts.Updater; u != nil {
		u.Check()
		return nil
	}
	return e.openInBrowser(update.ReleasesURL, errCantOpenLinks)
}
