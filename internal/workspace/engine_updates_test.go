package workspace

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/update"
)

// fakeReleases stands for GitHub's latest release, counting the looks.
type fakeReleases struct {
	mu    sync.Mutex
	r     update.Release
	err   error
	looks int
}

func (f *fakeReleases) latest(context.Context) (update.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.looks++
	return f.r, f.err
}

func (f *fakeReleases) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.looks
}

// updatesConfig is the example config with check_for_updates on, or off.
func updatesConfig(on bool) string {
	cfg := string(config.ExampleTOML())
	if on {
		cfg = strings.Replace(cfg, "check_for_updates = false", "check_for_updates = true", 1)
	}
	return cfg
}

// updatesHarness starts an engine at version, looking for updates with
// releases if on.
func updatesHarness(t *testing.T, on bool, version string, releases *fakeReleases) *harness {
	t.Helper()
	return newHarness(t, setup{config: updatesConfig(on), options: func(o *Options) {
		o.Version = version
		o.LatestRelease = releases.latest
	}})
}

// updateNotice returns the notice of a newer version, if there's one.
func updateNotice(h *harness) (NoticeView, bool) { return noticeWith(h.view().Notices, "Bench ") }

const releasePage = "https://github.com/itbm/bench/releases/tag/v0.3.0"

// TestCheckForUpdatesIsOptIn checks that Bench doesn't look for a newer
// version unless check_for_updates is on, which it isn't by default.
func TestCheckForUpdatesIsOptIn(t *testing.T) {
	releases := &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}
	h := updatesHarness(t, false, "0.2.0", releases)
	h.clk.Advance(2 * updateEvery)
	time.Sleep(20 * time.Millisecond)
	if n := releases.count(); n != 0 {
		t.Errorf("Bench looked %d times with check_for_updates off, want never", n)
	}
	if _, ok := updateNotice(h); ok || h.view().Settings.CheckForUpdates {
		t.Error("an update notice, or the setting on, by default")
	}
}

// TestCheckForUpdates checks that, with check_for_updates on, Bench looks
// as it starts and then once a day, and that a newer version gets a
// notice with Download, which opens its page, and Dismiss.
func TestCheckForUpdates(t *testing.T) {
	releases := &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}
	h := updatesHarness(t, true, "0.2.0 (4f2c1a9e0b7d)", releases)
	waitFor(t, func() bool { _, ok := updateNotice(h); return ok }, "the update notice")
	n, _ := updateNotice(h)
	if !strings.Contains(n.Text, "0.3.0") {
		t.Errorf("notice %q, want it to name 0.3.0", n.Text)
	}
	want := []NoticeAction{{Label: "Download", Method: "OpenLink", Arg: releasePage}, {Label: "Dismiss", Method: "DismissUpdate"}}
	if !slices.Equal(n.Actions, want) {
		t.Errorf("buttons %+v, want %+v", n.Actions, want)
	}
	if releases.count() != 1 {
		t.Errorf("looked %d times at startup, want once", releases.count())
	}

	// Not again for a day.
	h.clk.Advance(updateEvery - time.Minute)
	time.Sleep(20 * time.Millisecond)
	if releases.count() != 1 {
		t.Errorf("looked %d times within a day, want once", releases.count())
	}
	h.clk.Advance(2 * time.Minute)
	waitFor(t, func() bool { return releases.count() == 2 }, "the next day's look")

	// Dismiss takes the notice away, for that version, and state.json
	// keeps that.
	mustOK(t, h.e.DismissUpdate())
	if _, ok := updateNotice(h); ok {
		t.Error("the notice is still there after Dismiss")
	}
	h.clk.Advance(time.Second)
	st, err := stateOnDisk(t, h)
	mustOK(t, err)
	if st.Updates == nil || st.Updates.Dismissed != "0.3.0" || st.Updates.Latest != "0.3.0" || st.Updates.Checked.IsZero() {
		t.Errorf("state.json's updates = %+v", st.Updates)
	}
	// A newer version still gets a notice.
	releases.mu.Lock()
	releases.r = update.Release{Version: "0.4.0", URL: "https://github.com/itbm/bench/releases/tag/v0.4.0"}
	releases.mu.Unlock()
	h.clk.Advance(updateEvery)
	waitFor(t, func() bool { n, ok := updateNotice(h); return ok && strings.Contains(n.Text, "0.4.0") }, "the notice of 0.4.0")

	// Turning the setting off takes the notice away, and stops looking.
	s := h.view().Settings
	s.CheckForUpdates = false
	mustOK(t, h.e.SaveSettings(s))
	if _, ok := updateNotice(h); ok {
		t.Error("the notice is still there with the setting off")
	}
	looks := releases.count()
	h.clk.Advance(2 * updateEvery)
	time.Sleep(20 * time.Millisecond)
	if releases.count() != looks {
		t.Error("Bench went on looking with the setting off")
	}
}

// TestCheckForUpdatesFromSettings checks that turning check_for_updates on
// in Settings looks at once, if Bench hasn't looked in the last day, and
// that a look within the last day waits for the rest of it.
func TestCheckForUpdatesFromSettings(t *testing.T) {
	releases := &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}
	h := updatesHarness(t, false, "0.2.0", releases)
	s := h.view().Settings
	s.CheckForUpdates = true
	mustOK(t, h.e.SaveSettings(s))
	h.clk.Advance(time.Millisecond)
	waitFor(t, func() bool { _, ok := updateNotice(h); return ok }, "the update notice")

	h = newHarness(t, setup{config: updatesConfig(true), noStart: true, options: func(o *Options) {
		o.Version = "0.2.0"
		o.LatestRelease = releases.latest
	}})
	h.e.updates.Checked = h.clk.Now().Add(-time.Hour)
	before := releases.count()
	h.start()
	time.Sleep(20 * time.Millisecond)
	if releases.count() != before {
		t.Error("Bench looked again within a day of its last look")
	}
	h.clk.Advance(updateEvery - time.Hour + time.Second)
	waitFor(t, func() bool { return releases.count() == before+1 }, "the look a day after the last")
}

// TestNoUpdateNotice checks that there's no notice for the version Bench
// is, or an older one, nor for a build that isn't a release, and that a
// look that fails is only logged, and counts as the day's look.
func TestNoUpdateNotice(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		releases      *fakeReleases
	}{
		{"same version", "0.3.0", &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}},
		{"older release", "0.4.0", &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}},
		{"not a release", "dev", &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}},
		{"no answer", "0.2.0", &fakeReleases{err: errors.New("no network")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := updatesHarness(t, true, tc.version, tc.releases)
			waitFor(t, func() bool { return tc.releases.count() == 1 }, "the look at startup")
			time.Sleep(20 * time.Millisecond)
			if n, ok := updateNotice(h); ok {
				t.Errorf("notice %q, want none", n.Text)
			}
			h.e.lock()
			checked := h.e.updates.Checked
			h.e.unlock()
			if checked.IsZero() || h.clk.Now().Sub(checked) > time.Minute {
				t.Errorf("checked at %v, want as Bench started, by %v", checked, h.clk.Now())
			}
		})
	}
}

// fakeUpdater stands for Sparkle, recording what Bench asks of it.
type fakeUpdater struct {
	mu        sync.Mutex
	automatic []bool
	checks    int
}

func (f *fakeUpdater) SetAutomatic(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.automatic = append(f.automatic, on)
}

func (f *fakeUpdater) Check() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
}

// state returns the settings Bench gave the updater, in order, and how
// many times it asked it to look.
func (f *fakeUpdater) state() ([]bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.automatic), f.checks
}

// updaterHarness starts an engine at version 0.2.0 with an updater, and
// check_for_updates on, or off.
func updaterHarness(t *testing.T, on bool, u *fakeUpdater, releases *fakeReleases) *harness {
	t.Helper()
	return newHarness(t, setup{config: updatesConfig(on), options: func(o *Options) {
		o.Version = "0.2.0 (4f2c1a9e0b7d)"
		o.LatestRelease = releases.latest
		o.Updater = u
	}})
}

// TestUpdaterFollowsTheSetting checks that, with an updater, Bench turns
// the updater's daily look on and off with check_for_updates, and doesn't
// ask GitHub itself.
func TestUpdaterFollowsTheSetting(t *testing.T) {
	u := &fakeUpdater{}
	releases := &fakeReleases{r: update.Release{Version: "0.3.0", URL: releasePage}}
	h := updaterHarness(t, false, u, releases)
	if got, _ := u.state(); !slices.Equal(got, []bool{false}) {
		t.Errorf("the updater's settings %v at startup with the setting off, want [false]", got)
	}
	s := h.view().Settings
	s.CheckForUpdates = true
	mustOK(t, h.e.SaveSettings(s))
	s.CheckForUpdates = false
	mustOK(t, h.e.SaveSettings(s))
	h.clk.Advance(2 * updateEvery)
	time.Sleep(20 * time.Millisecond)
	if got, checks := u.state(); !slices.Equal(got, []bool{false, true, false}) || checks != 0 {
		t.Errorf("the updater's settings %v and %d looks, want [false true false] and none", got, checks)
	}
	if n := releases.count(); n != 0 {
		t.Errorf("Bench asked GitHub %d times with an updater, want never", n)
	}

	// A headless engine, for bench arrange, leaves the updater alone.
	u = &fakeUpdater{}
	newHarness(t, setup{config: updatesConfig(true), options: func(o *Options) {
		o.Updater = u
		o.Headless = true
	}})
	if got, _ := u.state(); len(got) != 0 {
		t.Errorf("a headless engine gave the updater %v, want nothing", got)
	}
}

// TestUpdaterNotice checks the notice of the version the updater found,
// with Install…, which shows the updater's window, and Dismiss, and that
// it goes once the updater is done with that version.
func TestUpdaterNotice(t *testing.T) {
	u := &fakeUpdater{}
	h := updaterHarness(t, true, u, &fakeReleases{})
	h.e.FoundUpdate("0.3.0")
	n, ok := updateNotice(h)
	if !ok || n.Text != "Bench 0.3.0 is out." {
		t.Fatalf("notice %q, %v; want one that names 0.3.0", n.Text, ok)
	}
	want := []NoticeAction{{Label: "Install…", Method: "InstallUpdate"}, {Label: "Dismiss", Method: "DismissUpdate"}}
	if !slices.Equal(n.Actions, want) {
		t.Errorf("buttons %+v, want %+v", n.Actions, want)
	}
	mustOK(t, h.e.InstallUpdate())
	if _, checks := u.state(); checks != 1 {
		t.Errorf("Install… asked the updater to look %d times, want once", checks)
	}
	// The notice stays until the updater is done with the version.
	if _, ok := updateNotice(h); !ok {
		t.Error("the notice went as the updater's window opened")
	}
	h.e.UpdateFinished()
	if _, ok := updateNotice(h); ok {
		t.Error("the notice is still there after the updater finished")
	}

	// Dismiss takes the notice away, and the updater's next look at the
	// same version brings it back no more.
	h.e.FoundUpdate("0.3.0")
	mustOK(t, h.e.DismissUpdate())
	h.e.UpdateFinished()
	h.e.FoundUpdate("0.3.0")
	if _, ok := updateNotice(h); ok {
		t.Error("the notice of 0.3.0 came back after Dismiss")
	}
	h.e.FoundUpdate("0.4.0")
	if _, ok := updateNotice(h); !ok {
		t.Error("no notice of 0.4.0 after the user dismissed 0.3.0")
	}

	// Nor is there a notice of this version, or an older one.
	h.e.UpdateFinished()
	for _, v := range []string{"0.2.0", "0.1.0"} {
		h.e.FoundUpdate(v)
		if n, ok := updateNotice(h); ok {
			t.Errorf("notice %q of %s, want none", n.Text, v)
		}
	}
}

// TestCheckForUpdatesMenu checks Bench ▸ Check for Updates…, which asks
// the updater to look, with the setting off too, or without an updater
// opens the latest release's page.
func TestCheckForUpdatesMenu(t *testing.T) {
	u := &fakeUpdater{}
	h := updaterHarness(t, false, u, &fakeReleases{})
	mustOK(t, h.e.CheckForUpdates())
	if _, checks := u.state(); checks != 1 {
		t.Errorf("Check for Updates… asked the updater to look %d times, want once", checks)
	}

	var opened []string
	h = newHarness(t, setup{options: func(o *Options) {
		o.OpenLink = func(address string) error { opened = append(opened, address); return nil }
	}})
	mustOK(t, h.e.CheckForUpdates())
	if !slices.Equal(opened, []string{update.ReleasesURL}) {
		t.Errorf("Check for Updates… without an updater opened %q, want %q", opened, update.ReleasesURL)
	}
	if err := h.e.InstallUpdate(); err == nil {
		t.Error("InstallUpdate without an updater worked, want an error")
	}
}
