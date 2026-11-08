package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// keepConfig has pages kept running in the background in two tabs, each
// behind a page that's shown, one that isn't, and one that the user has
// set not to keep running (decision 76).
const keepConfig = `
[[tab]]
name = "Comms"
layout = "side"
  [[tab.pane]]
  items = [
    { web = "https://app.slack.com/client", name = "Slack" },
    { web = "https://mail.google.com/mail/", name = "Gmail", keep_running = true },
  ]
  [[tab.pane]]
  items = [{ web = "https://outlook.office.com/mail/", name = "Outlook", keep_running = false }]

[[tab]]
name = "Chat"
layout = "single"
  [[tab.pane]]
  items = [
    { web = "https://discord.com/app", name = "Discord" },
    { web = "https://teams.microsoft.com/", name = "Teams", keep_running = true },
  ]

[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~/code" }]
`

// pageID returns the ID of the Web item with a name.
func (h *harness) pageID(name string) string {
	h.t.Helper()
	for id, it := range h.view().Items {
		if it.Kind == KindWeb && it.App == name {
			return id
		}
	}
	h.t.Fatalf("there's no page named %s", name)
	return ""
}

// loads returns the items that LoadWebView was called for, in order.
func (h *harness) loads() []string {
	var out []string
	for _, c := range h.calls("LoadWebView") {
		out = append(out, c.Arg)
	}
	return out
}

// keepRunningEntries returns each web entry's keep_running in config.toml,
// by its name: "on", "off", or "" while it's missing.
func keepRunningEntries(t *testing.T, h *harness) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, ct := range savedConfig(t, h).Tabs {
		for _, cp := range ct.Panes {
			for _, ci := range cp.Items {
				if ci.Web == "" {
					continue
				}
				switch {
				case ci.KeepRunning == nil:
					out[ci.Name] = ""
				case *ci.KeepRunning:
					out[ci.Name] = "on"
				default:
					out[ci.Name] = "off"
				}
			}
		}
	}
	return out
}

// TestKeepRunningLoadsAtStart checks that pages kept running in the
// background load, hidden, after the first arrangement, one at a time with
// a gap, those in the current tab first; that a page that isn't kept
// running still loads only once it's shown; and which pages the view says
// aren't loaded.
func TestKeepRunningLoadsAtStart(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig, noStart: true})
	slack, gmail, outlook := h.pageID("Slack"), h.pageID("Gmail"), h.pageID("Outlook")
	discord, teams := h.pageID("Discord"), h.pageID("Teams")

	// Nothing loads before the first arrangement, however long it takes.
	h.advance(10 * preloadDelay)
	if got := h.p.WebViewsMade(); len(got) != 0 {
		t.Fatalf("before Start, web views made: %v", got)
	}

	// Start shows the current tab's pages at once, and loads none in the
	// background yet.
	h.e.Start()
	h.advance(preloadDelay - frame)
	if got, want := h.p.WebViewsMade(), []string{slack, outlook}; !slices.Equal(got, want) {
		t.Errorf("after the first arrangement, web views made: %v, want %v", got, want)
	}
	if got := h.loads(); len(got) != 0 {
		t.Errorf("before preloadDelay, loaded %v", got)
	}
	v := h.view()
	for id, want := range map[string]bool{slack: false, gmail: false, outlook: false, discord: true, teams: false} {
		if got := v.Items[id].NotLoaded; got != want {
			t.Errorf("%s: notLoaded %v, want %v (a page waiting to load in the background isn't counted)", v.Items[id].App, got, want)
		}
	}
	for id, want := range map[string]bool{slack: false, gmail: true, outlook: false, discord: false, teams: true} {
		if got := v.Items[id].KeepRunning; got != want {
			t.Errorf("%s: keepRunning %v, want %v", v.Items[id].App, got, want)
		}
	}

	// Gmail, in the current tab, loads first, hidden, the size of the
	// pane area, and Teams only after the gap.
	h.advance(frame)
	if got, want := h.loads(), []string{gmail}; !slices.Equal(got, want) {
		t.Fatalf("at preloadDelay, loaded %v, want %v", got, want)
	}
	wv := h.p.WebViews()[gmail]
	if !wv.Loaded || wv.Visible || wv.Frame != h.e.area || wv.View.URL != "https://mail.google.com/mail/" {
		t.Errorf("Gmail's web view: %+v, want it loaded, hidden, at the pane area %v", wv, h.e.area)
	}
	h.advance(preloadGap - frame)
	if got := h.loads(); len(got) != 1 {
		t.Errorf("before the gap ended, loaded %v", got)
	}
	h.advance(frame)
	if got, want := h.loads(), []string{gmail, teams}; !slices.Equal(got, want) {
		t.Errorf("after the gap, loaded %v, want %v", got, want)
	}
	h.settle()
	if got, want := h.p.WebViewsMade(), []string{slack, outlook, gmail, teams}; !slices.Equal(got, want) {
		t.Errorf("web views made: %v, want %v: Discord isn't kept running", got, want)
	}
	if v := h.view(); !v.Items[discord].NotLoaded || v.Items[gmail].NotLoaded || v.Items[teams].NotLoaded {
		t.Errorf("notLoaded: Discord %v, Gmail %v, Teams %v; want only Discord's", v.Items[discord].NotLoaded,
			v.Items[gmail].NotLoaded, v.Items[teams].NotLoaded)
	}

	// Showing a page that was loaded in the background shows the same web
	// view; showing one that wasn't makes it then.
	mustOK(t, h.e.ShowItem(h.paneOf(gmail), gmail))
	if wv := h.p.WebViews()[gmail]; !wv.Visible || wv.Shows != 1 {
		t.Errorf("Gmail shown: %+v", wv)
	}
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if got, want := h.p.WebViewsMade(), []string{slack, outlook, gmail, teams, discord}; !slices.Equal(got, want) {
		t.Errorf("after showing Discord, web views made: %v, want %v", got, want)
	}
	if h.view().Items[discord].NotLoaded {
		t.Error("Discord is shown, but the view says it isn't loaded")
	}
	if got := len(h.loads()); got != 2 {
		t.Errorf("LoadWebView was called %d times, want 2", got)
	}
}

// TestKeepRunningLoadFails checks that a page that couldn't load in the
// background isn't tried again, and says it isn't loaded.
func TestKeepRunningLoadFails(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig, noStart: true})
	gmail, teams := h.pageID("Gmail"), h.pageID("Teams")
	h.p.SetErr("LoadWebView", errBadSource)
	h.start()
	h.advance(preloadDelay + 2*preloadGap)
	if got, want := h.loads(), []string{gmail, teams}; !slices.Equal(got, want) {
		t.Errorf("loads tried: %v, want %v, each once", got, want)
	}
	if v := h.view(); !v.Items[gmail].NotLoaded || !v.Items[teams].NotLoaded {
		t.Errorf("notLoaded: Gmail %v, Teams %v; want both", v.Items[gmail].NotLoaded, v.Items[teams].NotLoaded)
	}
}

// notifyFrom has a page post a notification, as Slack's does for a new
// message, and lets the engine settle.
func (h *harness) notifyFrom(id string) {
	h.t.Helper()
	h.p.EmitEvent(platform.Event{Kind: platform.WebNotification, ItemID: id, Title: "Ann", Body: "Lunch?"})
	h.settle()
}

// activateBench makes Bench the active app.
func (h *harness) activateBench() {
	h.t.Helper()
	mustOK(h.t, h.p.ActivateApp(benchID))
	h.settle()
}

// TestNotificationKeepsAPageRunning checks that a page's first
// notification turns Keep Running in the Background on, saves it, and says
// so once, with Undo, as a change that Undo can take back from Tab ▸ Undo
// too; that Undo turns it off as the user's choice, which later
// notifications leave alone; and that a page that the user has set not to
// keep running stays so.
func TestNotificationKeepsAPageRunning(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	h.activateBench()
	slack, outlook := h.pageID("Slack"), h.pageID("Outlook")
	toasts := len(h.em.toastList())

	h.notifyFrom(slack)
	if !h.view().Items[slack].KeepRunning {
		t.Error("after Slack's first notification, Keep Running in the Background is off")
	}
	got := h.em.toastList()[toasts:]
	if want := []string{"Slack will keep running in the background, so it can notify you."}; !slices.Equal(got, want) {
		t.Errorf("toasts %q, want %q", got, want)
	}
	undo := h.view().Undo
	if undo == nil || undo.Name != "Keep Running in the Background" || undo.Did != "kept Slack running in the background" {
		t.Fatalf("Tab ▸ Undo names %+v, want the notification's change", undo)
	}
	if a := h.em.lastAction(); a != (ToastAction{Label: "Undo", Method: "Undo", Arg: undo.Step}) {
		t.Errorf("the toast's button: %+v, want Undo of step %d", a, undo.Step)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "on" || got["Gmail"] != "on" || got["Outlook"] != "off" || got["Teams"] != "on" || got["Discord"] != "" {
		t.Errorf("config.toml's keep_running: %v", got)
	}
	if mb := h.menuBarNow(); mb.Pane.Item == slack && !mb.Pane.KeepRunning {
		t.Error("the menu bar's Pane ▸ Page doesn't tick Keep Running in the Background")
	}
	// Slack's web view, shown already, keeps running while it's hidden.
	if wv := h.p.WebViews()[slack]; !wv.KeepRunning {
		t.Errorf("Slack's web view: %+v, want it kept running", wv)
	}

	// It says so once.
	toasts = len(h.em.toastList())
	h.notifyFrom(slack)
	if got := h.em.toastList()[toasts:]; len(got) != 0 {
		t.Errorf("a second notification toasted %q", got)
	}

	// The toast's Undo turns it off, and saves that.
	mustOK(t, h.e.Undo(undo.Step))
	if h.view().Items[slack].KeepRunning {
		t.Error("after Undo, Keep Running in the Background is on")
	}
	if got, want := h.em.lastToast(), "Slack will load only when you show it, from the next time Bench opens."; got != want {
		t.Errorf("Undo's toast %q, want %q", got, want)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "off" {
		t.Errorf("after Undo, Slack's keep_running is %q, want off", got["Slack"])
	}
	if u := h.view().Undo; u != nil {
		t.Errorf("after Undo, Tab ▸ Undo still names %+v", u)
	}
	// Undo doesn't unload the page, but it no longer keeps running while
	// it's hidden.
	if wv, ok := h.p.WebViews()[slack]; !ok || wv.KeepRunning || slices.Contains(h.calls("CloseWebView"), platform.Call{Method: "CloseWebView", Arg: slack}) {
		t.Errorf("after Undo, Slack's web view: %+v, %v; want it open, not kept running", wv, ok)
	}

	// Later notifications leave the user's choice alone, as they do for
	// Outlook, which config.toml says not to keep running.
	toasts = len(h.em.toastList())
	h.notifyFrom(slack)
	h.notifyFrom(outlook)
	if v := h.view(); v.Items[slack].KeepRunning || v.Items[outlook].KeepRunning {
		t.Errorf("after more notifications: Slack %v, Outlook %v; want both off", v.Items[slack].KeepRunning, v.Items[outlook].KeepRunning)
	}
	if got := h.em.toastList()[toasts:]; len(got) != 0 {
		t.Errorf("notifications after Undo toasted %q", got)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "off" || got["Outlook"] != "off" {
		t.Errorf("config.toml's keep_running: %v", got)
	}
}

// TestNotificationUndoLatest checks that Tab ▸ Undo, away from the toast,
// takes back what a page's first notification did, and leaves the
// arrangement alone: a tab the user has gone to since stays shown.
func TestNotificationUndoLatest(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	h.activateBench()
	slack := h.pageID("Slack")
	h.notifyFrom(slack)
	mustOK(t, h.e.SwitchTab(1))
	h.settle()

	mustOK(t, h.e.UndoLatest())
	if got, want := h.em.lastToast(), "Undid: kept Slack running in the background."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	v := h.view()
	if v.Items[slack].KeepRunning || v.Current != 1 {
		t.Errorf("after Undo: Slack keepRunning %v, current tab %d; want off, and tab 1 still shown", v.Items[slack].KeepRunning, v.Current)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "off" {
		t.Errorf("Slack's keep_running is %q, want off", got["Slack"])
	}
	mustErr(t, h.e.UndoLatest(), errNothingToUndo.Error())
}

// TestNotificationToastWaitsForBench checks that the toast for a page's
// first notification, which most often comes while the user is in another
// app, waits for Bench to be the active app, with its Undo, while the
// setting is saved at once.
func TestNotificationToastWaitsForBench(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	slack := h.pageID("Slack")
	toasts := len(h.em.toastList())
	h.notifyFrom(slack)
	if got := h.em.toastList()[toasts:]; len(got) != 0 {
		t.Errorf("with Bench in the background, toasted %q", got)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "on" {
		t.Errorf("Slack's keep_running is %q, want on", got["Slack"])
	}
	h.activateBench()
	// As Bench comes forward, it hides Zed, which covers it, and says so
	// after (decision 68): that toast is the one left in view, and the
	// recent messages keep both, with their buttons.
	want := []string{"Slack will keep running in the background, so it can notify you.", "Hid Zed because it covered Bench."}
	if got := h.em.toastList()[toasts:]; !slices.Equal(got, want) {
		t.Errorf("once Bench is active, toasted %q, want %q", got, want)
	}
	undo := h.view().Undo
	h.em.mu.Lock()
	a := h.em.actions[toasts]
	h.em.mu.Unlock()
	if undo == nil || a != (ToastAction{Label: "Undo", Method: "Undo", Arg: undo.Step}) {
		t.Errorf("the toast's button: %+v, Tab ▸ Undo %+v", a, undo)
	}
	// Once only.
	toasts = len(h.em.toastList())
	mustOK(t, h.p.ActivateApp(zedID))
	h.settle()
	h.activateBench()
	if got := h.em.toastList()[toasts:]; len(got) != 0 {
		t.Errorf("Bench active again: toasted %q", got)
	}
}

// TestKeepRunningSurvivesUndo checks that Undo of a change to config.toml's
// tabs from before a page's keep_running was set, such as a tab's rename,
// keeps the page's keep_running as it is now, across a restart too.
func TestKeepRunningSurvivesUndo(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	h.activateBench()
	slack, discord := h.pageID("Slack"), h.pageID("Discord")
	mustOK(t, h.e.SaveTab(2, TabDraft{Name: "Coding", Preset: layout.PresetSingle}))
	h.settle()
	h.notifyFrom(slack)
	mustOK(t, h.e.Undo(h.view().Undo.Step))
	mustOK(t, h.e.SetKeepRunning(discord, true))

	// Tab ▸ Undo Rename Tab.
	mustOK(t, h.e.UndoLatest())
	if got := h.em.lastToast(); !strings.HasPrefix(got, "Undid: renamed Code") {
		t.Fatalf("toast %q, want the rename undone", got)
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "off" || got["Discord"] != "on" {
		t.Errorf("after Undo Rename Tab, config.toml's keep_running: Slack %q, Discord %q; want off and on", got["Slack"], got["Discord"])
	}
	if ct := savedConfig(t, h).Tabs[2]; ct.Name != "Code" {
		t.Errorf("tab 2 is %q, want Code again", ct.Name)
	}

	// After a restart, Slack's next notification leaves it off, and
	// Discord loads in the background.
	h.restart()
	h.activateBench()
	toasts := len(h.em.toastList())
	h.notifyFrom(slack)
	if v := h.view().Items; v[slack].KeepRunning || !v[discord].KeepRunning {
		t.Errorf("after a restart: Slack %v, Discord %v; want off and on", v[slack].KeepRunning, v[discord].KeepRunning)
	}
	if got := h.em.toastList()[toasts:]; len(got) != 0 {
		t.Errorf("after a restart, Slack's notification toasted %q", got)
	}
}

// TestSetKeepRunningEndsTheNotificationsUndo checks that the user's own
// choice in the menu takes the place of the change a notification made, so
// that Undo can't take back their choice.
func TestSetKeepRunningEndsTheNotificationsUndo(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	h.activateBench()
	slack := h.pageID("Slack")
	mustOK(t, h.e.SaveTab(2, TabDraft{Name: "Coding", Preset: layout.PresetSingle}))
	h.settle()
	h.notifyFrom(slack)
	mustOK(t, h.e.SetKeepRunning(slack, false))
	mustOK(t, h.e.SetKeepRunning(slack, true))
	if u := h.view().Undo; u == nil || u.Name != "Rename Tab" {
		t.Fatalf("Tab ▸ Undo names %+v, want Rename Tab", u)
	}
	mustOK(t, h.e.UndoLatest())
	if !h.view().Items[slack].KeepRunning {
		t.Error("Undo Rename Tab turned off what the user turned on")
	}
	if got := keepRunningEntries(t, h); got["Slack"] != "on" {
		t.Errorf("Slack's keep_running is %q, want on", got["Slack"])
	}
}

// TestSetKeepRunning checks the ⋯ menu's Keep Running in the Background:
// on, a page that isn't loaded loads soon, and off, it stays loaded but
// isn't loaded at the next start; and what it refuses.
func TestSetKeepRunning(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	discord, gmail := h.pageID("Discord"), h.pageID("Gmail")
	h.advance(preloadDelay + preloadGap)
	loaded := len(h.loads())

	mustOK(t, h.e.SetKeepRunning(discord, true))
	if got, want := h.em.lastToast(), "Discord will keep running in the background."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if got := keepRunningEntries(t, h); got["Discord"] != "on" {
		t.Errorf("Discord's keep_running is %q, want on", got["Discord"])
	}
	h.advance(preloadGap)
	if got := h.loads()[loaded:]; !slices.Equal(got, []string{discord}) {
		t.Errorf("after turning it on, loaded %v, want Discord", got)
	}
	if h.view().Items[discord].NotLoaded {
		t.Error("Discord has loaded, but the view says it isn't")
	}
	if wv := h.p.WebViews()[discord]; !wv.KeepRunning {
		t.Errorf("Discord's web view: %+v, want it kept running", wv)
	}

	// Off, Gmail stays loaded until Bench quits.
	mustOK(t, h.e.SetKeepRunning(gmail, false))
	if got, want := h.em.lastToast(), "Gmail will load only when you show it, from the next time Bench opens."; got != want {
		t.Errorf("toast %q, want %q", got, want)
	}
	if got := keepRunningEntries(t, h); got["Gmail"] != "off" {
		t.Errorf("Gmail's keep_running is %q, want off", got["Gmail"])
	}
	if v := h.view().Items[gmail]; v.KeepRunning || v.NotLoaded {
		t.Errorf("Gmail: keepRunning %v, notLoaded %v; want it off and still loaded", v.KeepRunning, v.NotLoaded)
	}
	if wv, ok := h.p.WebViews()[gmail]; !ok || wv.KeepRunning {
		t.Errorf("Gmail's web view: %+v, %v; want it open, no longer kept running", wv, ok)
	}

	// Only web pages in tabs.
	var terminal string
	for id, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			terminal = id
		}
	}
	mustErr(t, h.e.SetKeepRunning(terminal, true), errKeepRunningPage.Error())
	mustErr(t, h.e.SetKeepRunning("nope", true), errGone.Error())
	mustOK(t, h.e.PopOut(discord))
	mustErr(t, h.e.SetKeepRunning(discord, false), errKeepRunningTab.Error())
}

// TestKeepRunningRefusesVSCode checks that VS Code for the Web, whose page
// needs its server, can't keep running in the background, and isn't said
// to be not loaded.
func TestKeepRunningRefusesVSCode(t *testing.T) {
	startVSCode := func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return &fakeServer{}, nil }
	}
	h := newHarness(t, setup{config: vscodeConfig, options: startVSCode})
	h.advance(100 * time.Millisecond)
	var id string
	for i, it := range h.view().Items {
		if it.Kind == KindWeb {
			id = i
		}
	}
	mustErr(t, h.e.SetKeepRunning(id, true), errKeepRunningPage.Error())
	if h.view().Items[id].NotLoaded {
		t.Error("VS Code for the Web is said to be not loaded")
	}
}

// TestKeepRunningPageAddedInBench checks that a page with no entry in
// config.toml keeps the setting in state.json, and loads in the
// background when Bench starts again.
func TestKeepRunningPageAddedInBench(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig})
	h.activateBench()
	before, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	_, err := h.e.AddWebsite(h.view().Display.Focus, "localhost:3000")
	mustOK(t, err)
	h.settle()
	id := h.webItems("http://localhost:3000")[0].ID
	h.p.EmitEvent(platform.Event{Kind: platform.WebNotification, ItemID: id, Title: "Build", Body: "Done"})
	h.settle()
	if got := h.em.lastToast(); !strings.HasSuffix(got, " will keep running in the background, so it can notify you.") {
		t.Errorf("toast %q", got)
	}
	if after, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile)); string(after) != string(before) {
		t.Errorf("config.toml changed for a page it doesn't list:\n%s", after)
	}

	// Next time, in another tab, it loads in the background.
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	h.p.ResetCalls()
	h.restart()
	if !h.view().Items[id].KeepRunning {
		t.Fatal("after a restart, the page isn't kept running")
	}
	h.advance(preloadDelay + 2*preloadGap)
	if got := h.loads(); !slices.Contains(got, id) {
		t.Errorf("after a restart, loaded %v, want %s among them", got, id)
	}
}

// TestKeepRunningHeadless checks that bench arrange loads no pages and
// keeps none running.
func TestKeepRunningHeadless(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig, options: func(o *Options) { o.Headless = true }})
	slack := h.pageID("Slack")
	h.advance(preloadDelay + 3*preloadGap)
	h.p.EmitEvent(platform.Event{Kind: platform.WebNotification, ItemID: slack, Title: "Ann", Body: "Lunch?"})
	h.settle()
	if got := h.loads(); len(got) != 0 {
		t.Errorf("headless, loaded %v", got)
	}
	if v := h.view().Items[slack]; v.KeepRunning || v.NotLoaded {
		t.Errorf("headless, Slack: keepRunning %v, notLoaded %v", v.KeepRunning, v.NotLoaded)
	}
}

// TestKeepRunningLoadsAreNotArrangements checks that loading pages in the
// background arranges nothing: no window moves, and the times that Report
// a Problem gives (decision 67) are only those of what arranged. A hidden
// web view is Bench's own, not a window, so Bench coming forward puts
// away only the windows that cover it (decision 68), and leaves the pages
// loaded and hidden.
func TestKeepRunningLoadsAreNotArrangements(t *testing.T) {
	h := newHarness(t, setup{config: keepConfig, noStart: true})
	gmail, teams := h.pageID("Gmail"), h.pageID("Teams")
	h.e.Start()
	h.advance(preloadDelay - frame)
	mark := markTimes(h)
	moves := len(h.calls("SetFrame", "Park", "Unhide", "Hide", "ShowWebView", "HideWebView"))
	h.advance(frame + preloadGap)
	h.settle()
	if got, want := h.loads(), []string{gmail, teams}; !slices.Equal(got, want) {
		t.Fatalf("loaded %v, want %v", got, want)
	}
	if got := timesSince(h, mark); len(got) != 0 {
		t.Errorf("loading pages in the background was timed as arranging: %+v", got)
	}
	if got := h.calls("SetFrame", "Park", "Unhide", "Hide", "ShowWebView", "HideWebView"); len(got) != moves {
		t.Errorf("loading pages in the background moved windows: %+v", got[moves:])
	}

	h.activateBench()
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Zed because it covered Bench."}) {
		t.Errorf("as Bench came forward, toasts: %q, want Zed's alone", got)
	}
	for _, id := range []string{gmail, teams} {
		if wv := h.p.WebViews()[id]; !wv.Loaded || wv.Visible || !wv.KeepRunning {
			t.Errorf("%s's web view after Bench came forward: %+v, want it loaded, hidden, and kept running", id, wv)
		}
	}
}
