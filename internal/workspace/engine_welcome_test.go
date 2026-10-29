package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// withVersion gives the engine Bench's version.
func withVersion(v string) func(o *Options) {
	return func(o *Options) { o.Version = v }
}

// recordVersion writes the version that last had Accessibility permission
// into the support folder.
func recordVersion(v string) func(t *testing.T, dir string) {
	return func(t *testing.T, dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, trustedVersionFile), []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// recordedVersion returns the version that the support folder records as
// the last to have Accessibility permission, or "".
func recordedVersion(t *testing.T, h *harness) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, trustedVersionFile))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	mustOK(t, err)
	return strings.TrimSpace(string(data))
}

// waitFor waits up to two seconds for ok, which a goroutine of the
// engine's makes true, and fails the test if it doesn't.
func waitFor(t *testing.T, ok func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRequestAccessibilityOpensSystemSettings checks that Turn on
// Accessibility opens System Settings at Accessibility's list as well as
// asking macOS, which asks only once, but only while Bench doesn't have it.
func TestRequestAccessibilityOpensSystemSettings(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	h.p.ResetCalls()
	mustOK(t, h.e.RequestAccessibility())
	if h.p.TrustPrompts() != 1 {
		t.Errorf("TrustPrompts = %d, want macOS asked once", h.p.TrustPrompts())
	}
	if got := h.p.SettingsOpened(); !slices.Equal(got, []platform.SettingsPane{platform.SettingsAccessibility}) {
		t.Errorf("System Settings opened at %v, want accessibility", got)
	}
	h.p.SetTrusted(true)
	h.p.ResetCalls()
	mustOK(t, h.e.RequestAccessibility())
	if got := h.p.SettingsOpened(); len(got) != 0 {
		t.Errorf("System Settings opened at %v once Bench has the permission, want nothing", got)
	}
}

// TestTrustedVersionIsRecorded checks that Bench records its version in
// the support folder while it has Accessibility permission, readable only
// by the user, and once the permission arrives.
func TestTrustedVersionIsRecorded(t *testing.T) {
	h := newHarness(t, setup{options: withVersion("0.2.0 (4f2c1a9e0b7d)")})
	if got := recordedVersion(t, h); got != "0.2.0 (4f2c1a9e0b7d)" {
		t.Errorf("recorded %q, want this version", got)
	}
	if info, err := os.Stat(filepath.Join(h.dir, trustedVersionFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("%s: %v; want it readable only by the user", trustedVersionFile, err)
	}

	h = newHarness(t, setup{untrusted: true, options: withVersion("0.2.0")})
	if got := recordedVersion(t, h); got != "" {
		t.Errorf("recorded %q without the permission, want nothing", got)
	}
	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	if got := recordedVersion(t, h); got != "0.2.0" {
		t.Errorf("recorded %q once the permission came, want 0.2.0", got)
	}
}

// TestUpdatedWithoutAccessibility checks that after an update that macOS
// has stopped applying Accessibility permission to, the notice says how to
// put it right, with a button that asks for it, and that it doesn't for a
// first start, the same version, or an older one.
func TestUpdatedWithoutAccessibility(t *testing.T) {
	for _, tc := range []struct {
		name, was, now string
		updated        bool
	}{
		{"updated", "0.2.0 (4f2c1a9e0b7d)", "0.3.0 (a1b2c3d4e5f6)", true},
		{"a new build", "dev (4f2c1a9e0b7d)", "dev (a1b2c3d4e5f6)", true},
		{"first start", "", "0.3.0", false},
		{"same version", "0.3.0", "0.3.0", false},
		{"older version", "0.3.0", "0.2.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := setup{untrusted: true, options: withVersion(tc.now)}
			if tc.was != "" {
				s.state = recordVersion(tc.was)
			}
			h := newHarness(t, s)
			v := h.view()
			if v.Permissions.AccessibilityUpdated != tc.updated {
				t.Errorf("AccessibilityUpdated = %v, want %v", v.Permissions.AccessibilityUpdated, tc.updated)
			}
			want := accessibilityNotice
			if tc.updated {
				want = accessibilityUpdatedNotice
			}
			n, ok := noticeWith(v.Notices, want)
			if !ok {
				t.Fatalf("notices %q, want %q", texts(v.Notices), want)
			}
			if len(n.Actions) != 1 || n.Actions[0].Method != "RequestAccessibility" || n.Actions[0].Label != "Turn on Accessibility" {
				t.Errorf("the notice's buttons are %+v, want Turn on Accessibility", n.Actions)
			}

			// Once the permission is back, the notice goes, and Bench
			// records this version.
			h.p.SetTrusted(true)
			mustOK(t, h.e.RequestAccessibility())
			v = h.view()
			if v.Permissions.AccessibilityUpdated || slices.Contains(texts(v.Notices), want) {
				t.Errorf("after the permission came: %+v, notices %q", v.Permissions, texts(v.Notices))
			}
			if got := recordedVersion(t, h); got != tc.now {
				t.Errorf("recorded %q, want %q", got, tc.now)
			}
		})
	}
}

// TestScreenRecordingOffersToReopen checks that asking for Screen
// Recording permission opens System Settings at its list as well as
// asking macOS, and that the view then offers to quit and reopen Bench,
// since macOS applies it only as Bench opens; but not when Bench had it
// as it opened.
func TestScreenRecordingOffersToReopen(t *testing.T) {
	h := newHarness(t, setup{})
	if v := h.view(); v.Permissions.ScreenRecording || v.Permissions.ScreenRecordingReopen {
		t.Fatalf("permissions %+v, want neither", v.Permissions)
	}
	mustOK(t, h.e.RequestScreenRecording())
	if h.p.ScreenRecordingRequests() != 1 {
		t.Errorf("ScreenRecordingRequests = %d, want 1", h.p.ScreenRecordingRequests())
	}
	if got := h.p.SettingsOpened(); !slices.Equal(got, []platform.SettingsPane{platform.SettingsScreenRecording}) {
		t.Errorf("System Settings opened at %v, want screen-recording", got)
	}
	if !h.view().Permissions.ScreenRecordingReopen {
		t.Error("ScreenRecordingReopen = false after asking, want true")
	}
	// The permission shows as granted once Bench comes forward again.
	h.p.SetScreenRecordingAllowed(true)
	h.p.EmitEvent(platform.Event{Kind: platform.AppActivated, App: h.bench})
	h.drain()
	if v := h.view(); !v.Permissions.ScreenRecording || !v.Permissions.ScreenRecordingReopen {
		t.Errorf("permissions %+v once granted, want both", v.Permissions)
	}

	h = newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetScreenRecordingAllowed(true)
	}})
	mustOK(t, h.e.RequestScreenRecording())
	if v := h.view(); !v.Permissions.ScreenRecording || v.Permissions.ScreenRecordingReopen {
		t.Errorf("permissions %+v with the permission from the start, want no reopen", v.Permissions)
	}
	if got := h.p.SettingsOpened(); len(got) != 0 {
		t.Errorf("System Settings opened at %v, want nothing", got)
	}
}

// TestLivePreviewsOpenScreenRecording checks that turning live previews on
// in Settings opens System Settings at Screen Recording's list too, while
// Bench doesn't have the permission.
func TestLivePreviewsOpenScreenRecording(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.LivePreviews = true
	mustOK(t, h.e.SaveSettings(s))
	if got := h.p.SettingsOpened(); !slices.Equal(got, []platform.SettingsPane{platform.SettingsScreenRecording}) {
		t.Errorf("System Settings opened at %v, want screen-recording", got)
	}
	if !h.view().Permissions.ScreenRecordingReopen {
		t.Error("ScreenRecordingReopen = false, want true")
	}
}

// TestQuitAndReopen checks that Quit and Reopen Bench quits and opens Bench
// again at once with nothing running, asks first, as quitting does, while a
// terminal is running something, and goes ahead once the user is sure.
func TestQuitAndReopen(t *testing.T) {
	h, ids := busyHarness(t, "")
	mustOK(t, h.e.QuitAndReopen(false, false))
	if h.em.relaunchCount() != 1 {
		t.Errorf("Relaunch calls = %d, want 1", h.em.relaunchCount())
	}

	h, ids = busyHarness(t, "")
	h.terms.setBusy(ids["claude"], "claude")
	mustOK(t, h.e.QuitAndReopen(false, false))
	got := h.em.commandsNamed(confirmQuitCommand)
	if len(got) != 1 || !got[0].Reopen || len(got[0].Terminals) != 1 {
		t.Fatalf("commands %+v, want one confirm-quit for reopening, naming claude", got)
	}
	if h.em.relaunchCount() != 0 {
		t.Error("Bench reopened without waiting for the user's answer")
	}
	mustOK(t, h.e.QuitAndReopen(true, true))
	if h.em.relaunchCount() != 1 {
		t.Errorf("Relaunch calls = %d once sure, want 1", h.em.relaunchCount())
	}
	if savedConfig(t, h).Settings.ConfirmQuitBusyTerminals {
		t.Error("Don't ask again didn't turn confirm_quit_busy_terminals off")
	}

	h = newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustErr(t, h.e.QuitAndReopen(true, false), errCantReopen.Error())
}

// approvalLogin is a LoginItem that macOS doesn't enable until the user
// allows it in System Settings.
type approvalLogin struct{ on, allowed bool }

func (f *approvalLogin) IsEnabled() (bool, error) { return f.on && f.allowed, nil }
func (f *approvalLogin) Enable() error            { f.on = true; return nil }
func (f *approvalLogin) Disable() error           { f.on = false; return nil }

// TestOpenAtLoginNeedsApproval checks that when macOS needs the user to
// allow Bench's login item, turning Open at login on says so, opens Login
// Items, and the view says so until it's allowed, which Bench notices as
// it comes forward.
func TestOpenAtLoginNeedsApproval(t *testing.T) {
	login := &approvalLogin{}
	h := newHarness(t, setup{options: func(o *Options) { o.LoginItem = login }})
	h.p.SetLoginItemNeedsApproval(true)
	mustErr(t, h.e.SetOpenAtLogin(true), loginApprovalText)
	if got := h.p.SettingsOpened(); !slices.Equal(got, []platform.SettingsPane{platform.SettingsLoginItems}) {
		t.Errorf("System Settings opened at %v, want login-items", got)
	}
	if v := h.view(); v.OpenAtLogin || !v.LoginApproval {
		t.Errorf("OpenAtLogin %v, LoginApproval %v; want false, true", v.OpenAtLogin, v.LoginApproval)
	}
	mustOK(t, h.e.OpenLoginItems())
	if got := h.p.SettingsOpened(); len(got) != 2 || got[1] != platform.SettingsLoginItems {
		t.Errorf("OpenLoginItems opened %v", got)
	}

	// The user allows it, and comes back to Bench.
	login.allowed = true
	h.p.SetLoginItemNeedsApproval(false)
	h.p.EmitEvent(platform.Event{Kind: platform.AppActivated, App: h.bench})
	h.drain()
	waitFor(t, func() bool { v := h.view(); return v.OpenAtLogin && !v.LoginApproval }, "Open at login to be on once allowed")
}

// TestLoginApprovalAtStart checks that Bench says at startup that macOS
// needs the user to allow its login item.
func TestLoginApprovalAtStart(t *testing.T) {
	login := &approvalLogin{on: true}
	h := newHarness(t, setup{
		world:   func(p *platform.Fake) { standardWorld(p); p.SetLoginItemNeedsApproval(true) },
		options: func(o *Options) { o.LoginItem = login },
	})
	if v := h.view(); v.OpenAtLogin || !v.LoginApproval {
		t.Errorf("OpenAtLogin %v, LoginApproval %v; want false, true", v.OpenAtLogin, v.LoginApproval)
	}
	mustErr(t, newHarness(t, setup{}).e.OpenLoginItems(), errNoLoginItem.Error())
}

// starterWorld has the apps a new Mac might: Safari, Mail, Notes, and
// Calendar from macOS, and Slack and VS Code, and Teams as a second chat
// app.
func starterWorld(p *platform.Fake) {
	p.InstallApp(safariID, "Safari", platform.AppLaunchWindows("Apple"))
	p.InstallApp("com.apple.mail", "Mail", platform.AppLaunchWindows("Inbox"))
	p.InstallApp("com.apple.Notes", "Notes")
	p.InstallApp(calendarID, "Calendar")
	p.InstallApp("com.tinyspeck.slackmacgap", "Slack", platform.AppLaunchWindows("Slack"))
	p.InstallApp("com.microsoft.teams2", "Microsoft Teams")
	p.InstallApp(vscodeID, "Visual Studio Code")
}

// TestStarterTabs checks that the welcome guide suggests tabs built only
// from apps that are installed, leaving out apps that a tab has already,
// and tabs whose name one has.
func TestStarterTabs(t *testing.T) {
	h := newHarness(t, setup{firstRun: true, world: func(p *platform.Fake) {
		starterWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	got, err := h.e.StarterTabs()
	mustOK(t, err)
	want := []StarterTab{
		{Name: "Comms", Apps: []StarterApp{{BundleID: "com.tinyspeck.slackmacgap", Name: "Slack", Mono: "Sl"}, {BundleID: "com.apple.mail", Name: "Mail", Mono: "Ma"}}},
		{Name: "Code", Apps: []StarterApp{{BundleID: vscodeID, Name: "Visual Studio Code", Mono: "VS"}, {Name: "Terminal", Mono: ">_"}}},
		{Name: "Plan", Apps: []StarterApp{{BundleID: "com.apple.Notes", Name: "Notes", Mono: "No"}, {BundleID: calendarID, Name: "Calendar", Mono: "Ca"}}},
	}
	if !reflect.DeepEqual(names(got), names(want)) {
		t.Fatalf("StarterTabs = %+v\nwant %+v", got, want)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i].Apps, want[i].Apps) {
			t.Errorf("%s's apps = %+v, want %+v", want[i].Name, got[i].Apps, want[i].Apps)
		}
	}

	// A tab named Plan, and Slack in a tab, change what's suggested.
	h = newHarness(t, setup{world: starterWorld, config: `
[[tab]]
name = "Research"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.apple.Safari" }]

[[tab]]
name = "plan"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.tinyspeck.slackmacgap" }]
`})
	got, err = h.e.StarterTabs()
	mustOK(t, err)
	if !reflect.DeepEqual(names(got), []string{"Comms", "Code"}) || got[0].Apps[0].Name != "Microsoft Teams" {
		t.Errorf("StarterTabs = %+v, want Comms with Teams, and Code", got)
	}
}

// tabApps describes each tab by its name and its items' apps, whether or
// not they have windows yet.
func tabApps(h *harness) []string {
	h.e.lock()
	defer h.e.unlock()
	var out []string
	for _, t := range h.e.model.Tabs {
		var apps []string
		for _, id := range t.ItemIDs() {
			apps = append(apps, h.e.model.Items[id].App)
		}
		out = append(out, t.Name+": "+strings.Join(apps, ", "))
	}
	return out
}

// names returns the starter tabs' names.
func names(tabs []StarterTab) []string {
	var out []string
	for _, t := range tabs {
		out = append(out, t.Name)
	}
	return out
}

// TestUseStarterTabs checks that the starting set-up adds the tabs chosen
// after the first, with their apps and a terminal, saves them in
// config.toml, and stays on the current tab.
func TestUseStarterTabs(t *testing.T) {
	h := newHarness(t, setup{firstRun: true, world: func(p *platform.Fake) {
		starterWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustErr(t, h.e.UseStarterTabs([]string{"Comms", "Games"}), "Bench doesn't suggest a tab called Games any more.")
	if n := len(h.view().Tabs); n != 1 {
		t.Fatalf("a refused set-up added tabs: %d", n)
	}
	mustOK(t, h.e.UseStarterTabs([]string{"Comms", "code"}))
	v := h.view()
	if got, want := tabApps(h), []string{"Research: Safari", "Comms: Slack, Mail", "Code: Visual Studio Code, Terminal"}; !slices.Equal(got, want) {
		t.Errorf("tabs = %q, want %q", got, want)
	}
	if v.Current != 0 {
		t.Errorf("current tab %d, want Research still", v.Current)
	}
	if h.em.lastToast() != "Added Comms and Code." {
		t.Errorf("toast %q", h.em.lastToast())
	}
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 3 {
		t.Fatalf("config.toml has %d tabs, want 3", len(cfg.Tabs))
	}
	comms, code := cfg.Tabs[1], cfg.Tabs[2]
	if comms.Layout != "side" || comms.SmallScreens != "tabs" || len(comms.Panes) != 2 || comms.Panes[1].Items[0].App != "com.apple.mail" {
		t.Errorf("Comms in config.toml: %+v", comms)
	}
	if code.Layout != "main-side" || code.Panes[0].Items[0].App != vscodeID || code.Panes[1].Items[0] != (config.Item{Terminal: "~"}) {
		t.Errorf("Code in config.toml: %+v", code)
	}
	if _, err := stateOnDisk(t, h); err != nil {
		t.Errorf("state.json doesn't follow config.toml: %v", err)
	}
	// Switching to a new tab opens its apps.
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if !h.p.IsRunning("com.tinyspeck.slackmacgap") || !h.p.IsRunning("com.apple.mail") {
		t.Error("switching to Comms didn't open its apps")
	}
	// Nothing to add does nothing.
	mustOK(t, h.e.UseStarterTabs(nil))
	if n := len(h.view().Tabs); n != 3 {
		t.Errorf("%d tabs after adding nothing, want 3", n)
	}
}
