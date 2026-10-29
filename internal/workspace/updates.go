package workspace

import (
	"context"
	"time"

	"github.com/itbm/bench/internal/update"
)

// This file looks for a newer version of Bench while the check_for_updates
// setting is on, which it's not unless the user turns it on: at most once
// a day, it asks GitHub for the latest release (Options.LatestRelease),
// and if that's newer than this version, a notice says so, with Download,
// which opens the release's page in the browser, and Dismiss, after which
// Bench doesn't announce that version again. Bench downloads and installs
// nothing itself. state.json keeps when it last looked, and what it found.

// updateEvery is how often Bench looks for a newer version.
const updateEvery = 24 * time.Hour

// updateWait is how long Bench waits for GitHub to answer.
const updateWait = 30 * time.Second

// sourceUpdates is the source of the notice of a newer version.
const sourceUpdates = "updates"

// scheduleUpdateCheckLocked looks for a newer version once that's due,
// while check_for_updates is on: at once if Bench hasn't looked in the last
// day, or else a day after it last did. With the setting off, it stops
// looking, and the notice goes.
func (e *Engine) scheduleUpdateCheckLocked() {
	if e.updateTimer != nil {
		e.updateTimer.Stop()
		e.updateTimer = nil
	}
	e.updateNoticeLocked()
	if !e.cfg.Settings.CheckForUpdates || e.opts.LatestRelease == nil || e.opts.Headless || !e.started || e.closed {
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
// or takes it away.
func (e *Engine) updateNoticeLocked() {
	u := e.updates
	if !e.cfg.Settings.CheckForUpdates || u.Latest == "" || u.Latest == u.Dismissed || !e.newerLocked(u.Latest) {
		e.setNoticeViewsLocked(sourceUpdates, nil)
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
