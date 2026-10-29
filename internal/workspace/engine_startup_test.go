package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/windows"
)

// The example config's items, in order: w1 Slack, w2 Outlook, w3 Zed, w4 VS
// Code, w5 the terminal, w6 Brave, w7 Obsidian. Panes: p8 and p9 in Comms,
// p11 and p12 in Code, p14 and p15 in Research.

func TestStartupBuildsItemsFromConfig(t *testing.T) {
	h := newHarness(t, setup{world: func(*platform.Fake) {}})
	v := h.view()
	if len(v.Tabs) != 3 || v.Current != 0 {
		t.Fatalf("tabs %d, current %d", len(v.Tabs), v.Current)
	}
	tests := []struct {
		id, kind, app, mono, title, status, text, pane string
	}{
		{"w1", KindWeb, "Slack", "Sl", "Slack", StatusReady, "", "p8"},
		{"w2", KindWeb, "Outlook", "Ou", "Outlook", StatusReady, "", "p9"},
		{"w3", KindApp, "Zed", "Zd", "Zed", StatusMissing, "Zed isn't running", "p11"},
		{"w4", KindApp, "VS Code", "VS", "VS Code", StatusMissing, "VS Code isn't running", "p11"},
		{"w5", KindBuiltin, "Terminal", ">_", "zsh — ~/code", StatusReady, "", "p12"},
		{"w6", KindApp, "Brave", "Br", "Brave", StatusMissing, "Brave isn't running", "p14"},
		{"w7", KindApp, "Obsidian", "Ob", "Obsidian", StatusMissing, "Obsidian isn't running", "p15"},
	}
	for _, tc := range tests {
		it := v.Items[tc.id]
		got := []string{it.Kind, it.App, it.Mono, it.Title, it.Status, it.StatusText, it.Pane}
		want := []string{tc.kind, tc.app, tc.mono, tc.title, tc.status, tc.text, tc.pane}
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %q, want %q", tc.id, got, want)
		}
	}
	if v.Items["w1"].URL != "https://app.slack.com/client" || v.Items["w5"].Folder != "~/code" {
		t.Errorf("url %q folder %q", v.Items["w1"].URL, v.Items["w5"].Folder)
	}
	if !v.ConfigWritable || len(v.Notices) != 0 {
		t.Errorf("writable %v notices %v", v.ConfigWritable, texts(v.Notices))
	}
}

// TestFirstRunWritesTheDefaultBrowser checks that on first run, Bench
// writes a config with one tab holding the default browser, and no app
// that the Mac may not have.
func TestFirstRunWritesTheDefaultBrowser(t *testing.T) {
	for _, tc := range []struct {
		name, browser, want string
	}{
		{"Chrome", "com.google.Chrome", "com.google.Chrome"},
		{"no default browser", "", "com.apple.Safari"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{firstRun: true, world: func(p *platform.Fake) { p.SetDefaultBrowser(tc.browser) }})
			data, err := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
			if err != nil {
				t.Fatalf("the default config wasn't written: %v", err)
			}
			if got, want := string(data), string(config.DefaultTOMLFor(tc.want)); got != want {
				t.Errorf("config.toml =\n%s\nwant\n%s", got, want)
			}
			v := h.view()
			if len(v.Tabs) != 1 || v.Tabs[0].Name != "Research" || len(v.Items) != 1 || !v.ConfigWritable {
				t.Errorf("tabs %+v, items %+v, writable %v; want Research with one item", v.Tabs, v.Items, v.ConfigWritable)
			}
			for _, it := range h.e.model.Items {
				if it.BundleID != tc.want {
					t.Errorf("item %s is %s, want %s", it.ID, it.BundleID, tc.want)
				}
			}
		})
	}
}

// TestFirstRunInThePreview checks that the browser preview, whose apps are
// all there, starts with the example config.
func TestFirstRunInThePreview(t *testing.T) {
	h := newHarness(t, setup{firstRun: true, options: func(o *Options) { o.Demo = true }})
	data, err := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	if err != nil || string(data) != string(config.ExampleTOML()) {
		t.Errorf("config.toml = %v\n%s\nwant the example config", err, data)
	}
	if v := h.view(); len(v.Tabs) != 3 || v.Tabs[0].Name != "Comms" {
		t.Errorf("tabs %+v, want the example's three", v.Tabs)
	}
}

// TestStartupMakesAPrivateSupportFolder checks that the support folder, if
// it's missing, is made readable only by the user.
func TestStartupMakesAPrivateSupportFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Bench")
	newHarness(t, setup{options: func(o *Options) { o.Dir = dir }})
	if info, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	} else if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("the support folder was made %#o, want 0700", perm)
	}
}

// TestConfigProblemsUseDefaults checks that Bench runs with the default
// config when config.toml has problems and no config has worked before, and
// says so, with buttons for fixing the file.
func TestConfigProblemsUseDefaults(t *testing.T) {
	bad := "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n"
	h := newHarness(t, setup{config: bad})
	v := h.view()
	if v.ConfigWritable {
		t.Errorf("ConfigWritable should be false")
	}
	if len(v.Tabs) != 1 || v.Tabs[0].Name != "Research" {
		t.Errorf("should run with the default config, got %d tabs", len(v.Tabs))
	}
	if len(v.Notices) < 2 || v.Notices[0].Text != problemsLead(usingDefault, false, false) {
		t.Fatalf("notices: %v", texts(v.Notices))
	}
	if n := v.Notices[1].Text; !strings.HasPrefix(n, "config.toml: ") || !strings.Contains(n, "Mine") {
		t.Errorf("notices: %v", texts(v.Notices))
	}
	want := []NoticeAction{actionOpenConfig, actionShowConfig, actionReload}
	if got := v.Notices[0].Actions; !slices.Equal(got, want) {
		t.Errorf("actions %+v, want %+v (no backup to use)", got, want)
	}
	if got := h.e.Problems(); len(got) != len(v.Notices)-1 {
		t.Errorf("Problems: %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	if string(data) != bad {
		t.Errorf("config.toml was changed")
	}
	// Changes aren't saved over the user's state.json.
	mustOK(t, h.e.SetMode(string(layout.ModeCompact)))
	h.advance(saveDelay * 2)
	if _, err := os.Stat(filepath.Join(h.dir, config.StateFile)); err == nil {
		t.Errorf("state.json was written while config.toml has problems")
	}
}

func TestStartupScanMatchesWindowsByClaims(t *testing.T) {
	h := newHarness(t, setup{})
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	docs := h.windowID(braveID, "Wails v3 docs")
	pr := h.windowID(braveID, "Pull request #42 · bench")
	pkgs := h.windowID(braveID, "Go packages")
	note := h.windowID(obsidianID, "Daily note")
	tests := []struct {
		window platform.WindowID
		item   string
	}{
		{zed, "w3"}, {code, "w4"}, {docs, "w6"}, {pr, "w17"}, {pkgs, "w18"}, {note, "w7"},
	}
	for _, tc := range tests {
		if got := h.itemOf(tc.window); got != tc.item {
			t.Errorf("window %d: item %q, want %s", tc.window, got, tc.item)
		}
	}
	v := h.view()
	research := v.Tabs[2]
	if got := layout.Leaves(research.Root)[0].Items; !slices.Equal(got, []string{"w6", "w17", "w18"}) {
		t.Errorf("Research's first pane: %v (later windows go right after the entry's item)", got)
	}
	if v.Items["w3"].Title != "bench — switch.go" || v.Items["w3"].Status != StatusReady {
		t.Errorf("zed: %+v", v.Items["w3"])
	}
	// Finder isn't claimed, so it's listed as not in a tab.
	finder := h.windowID(finderID, "Downloads")
	ids := floatingIDs(v)
	if !slices.Equal(ids, []string{floatingID(finder)}) {
		t.Errorf("floating: %v", ids)
	}
	if f := v.Floating[0]; f.App != "Finder" || f.Mono != "Fn" || f.Title != "Downloads" || f.Tab != -1 || f.Kind != KindApp {
		t.Errorf("finder: %+v", f)
	}
	// Bench's own window is never listed.
	for _, f := range v.Floating {
		if f.BundleID == benchID {
			t.Errorf("Bench's own window is listed")
		}
	}
}

func TestStartupScanSkipsDialogsAndAgents(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		p.AddApp(zedID, "Zed")
		p.AddWindow(zedID, "Settings", platform.WithSubrole(platform.SubroleDialog))
		p.AddApp("com.example.agent", "Agent", platform.AppNotRegular())
		p.AddWindow("com.example.agent", "Panel")
	}})
	v := h.view()
	if len(v.Floating) != 0 {
		t.Errorf("floating: %v", floatingIDs(v))
	}
	if v.Items["w3"].Status != StatusMissing {
		t.Errorf("a dialog bound to Zed's item: %+v", v.Items["w3"])
	}
}

// savedState starts an engine on the standard world, changes it, and returns
// the state.json it saved, for another engine to start from.
func savedState(t *testing.T, change func(h *harness)) []byte {
	t.Helper()
	h := newHarness(t, setup{})
	if change != nil {
		change(h)
	}
	mustOK(t, h.e.Close())
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	return data
}

// writeState returns a setup step that writes state.json.
func writeState(data []byte) func(t *testing.T, dir string) {
	return func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, config.StateFile), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStateRestoresArrangement(t *testing.T) {
	data := savedState(t, func(h *harness) {
		mustOK(t, h.e.SwitchTab(2))
		mustOK(t, h.e.MoveItem("w17", 2, "p15", "center"))
		mustOK(t, h.e.SetMode("wide"))
	})
	h := newHarness(t, setup{state: writeState(data)})
	v := h.view()
	if v.Current != 2 || v.Mode != "wide" {
		t.Errorf("current %d mode %s", v.Current, v.Mode)
	}
	if v.Tabs[2].Preset != layout.PresetCustom || v.Items["w17"].Pane != "p15" {
		t.Errorf("preset %s, w17 in %s", v.Tabs[2].Preset, v.Items["w17"].Pane)
	}
	// Bench restarted while the apps kept running: the same windows match
	// by window ID.
	if got := h.itemOf(h.windowID(braveID, "Pull request #42 · bench")); got != "w17" {
		t.Errorf("w17's window: %q", got)
	}
	// IDs carry on after the state's.
	h.e.lock()
	seq := h.e.model.Seq
	h.e.unlock()
	if seq < 18 {
		t.Errorf("seq: %d", seq)
	}
}

func TestRestartMatchesByTitleThenCreationOrder(t *testing.T) {
	data := savedState(t, nil)
	// Brave restarted while Bench was closed: new window IDs, one new title.
	h := newHarness(t, setup{state: writeState(data), world: func(p *platform.Fake) {
		standardWorld(p)
		p.QuitApp(braveID)
		p.AddApp(braveID, "Brave")
		p.AddWindow(braveID, "New Tab")
		p.AddWindow(braveID, "Go packages")
		p.AddWindow(braveID, "New Tab 2")
		p.AddWindow(braveID, "Another")
	}})
	pkgs := h.windowID(braveID, "Go packages")
	first := h.windowID(braveID, "New Tab")
	third := h.windowID(braveID, "New Tab 2")
	another := h.windowID(braveID, "Another")
	if got := h.itemOf(pkgs); got != "w18" {
		t.Errorf("by title: %q, want w18", got)
	}
	if got := h.itemOf(first); got != "w6" {
		t.Errorf("by creation order: %q, want w6", got)
	}
	if got := h.itemOf(third); got != "w17" {
		t.Errorf("by creation order: %q, want w17", got)
	}
	// The fourth is new: claims put it in Brave's pane.
	it := h.itemOf(another)
	if it == "" || h.paneOf(it) != "p14" {
		t.Errorf("the extra window: item %q", it)
	}
}

func TestStartupKeepsWindowsOnAnotherDesktop(t *testing.T) {
	data := savedState(t, nil)
	h := newHarness(t, setup{state: writeState(data), world: func(p *platform.Fake) {
		standardWorld(p)
		app, _ := p.App(obsidianID)
		ws, _ := p.Windows(app)
		p.MoveToOtherDesktop(ws[0].ID)
	}})
	it := h.item("w7")
	if it.Status != StatusUnavailable || it.StatusText != "On another desktop" {
		t.Errorf("w7: %s %q", it.Status, it.StatusText)
	}
}

func TestNewRestoresCrashedParkedWindows(t *testing.T) {
	var parked platform.WindowID
	h := newHarness(t, setup{noStart: true, world: func(p *platform.Fake) {
		standardWorld(p)
		w := p.AddWindow(safariID, "Stranded", platform.WithFrame(layout.Rect{X: 2559, Y: 1439, W: 800, H: 600}))
		parked = w.ID
	}, state: func(t *testing.T, dir string) {
		f := windows.ParkedFile{Windows: []windows.ParkedWindow{{ID: 0, BundleID: safariID, Title: "Stranded",
			Frame: layout.Rect{X: 200, Y: 200, W: 800, H: 600}}}}
		if err := windows.WriteParked(filepath.Join(dir, config.ParkedFile), f); err != nil {
			t.Fatal(err)
		}
	}})
	if got := h.frame(parked); got != (layout.Rect{X: 200, Y: 200, W: 800, H: 600}) {
		t.Errorf("stranded window: %v", got)
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.ParkedFile)); err == nil {
		t.Errorf("parked.json is still there")
	}
}

func TestWithoutAccessibility(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	v := h.view()
	if v.Permissions.Accessibility || !slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Fatalf("permissions %+v notices %v", v.Permissions, texts(v.Notices))
	}
	if v.Items["w3"].Status != StatusMissing {
		t.Errorf("w3 shouldn't have a window yet")
	}
	h.advance(4 * trustEvery)
	if n := len(h.calls("Trusted")); n < 4 {
		t.Errorf("Trusted calls: %d, want one every 2 s", n)
	}
	// Permission arrives: Bench scans and arranges.
	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	v = h.view()
	if !v.Permissions.Accessibility || slices.Contains(texts(v.Notices), accessibilityNotice) {
		t.Errorf("still untrusted: %+v %v", v.Permissions, texts(v.Notices))
	}
	if v.Items["w3"].Status != StatusReady {
		t.Errorf("w3 after permission: %+v", v.Items["w3"])
	}
	if !h.p.IsHidden(zedID) {
		t.Errorf("Zed belongs to another tab and should be hidden")
	}
}

func TestRequestAccessibility(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	h.p.ResetCalls()
	mustOK(t, h.e.RequestAccessibility())
	if !h.called("Trusted", func(c platform.Call) bool { return c.Arg == "true" }) || h.p.TrustPrompts() != 1 {
		t.Errorf("RequestAccessibility should call Trusted(true)")
	}
	h.p.SetTrusted(true)
	mustOK(t, h.e.RequestAccessibility())
	if v := h.view(); !v.Permissions.Accessibility || v.Items["w3"].Status != StatusReady {
		t.Errorf("after granting: %+v %+v", v.Permissions, v.Items["w3"])
	}
}

func TestFirstRunAndLogin(t *testing.T) {
	login := &fakeLogin{}
	h := newHarness(t, setup{options: func(o *Options) { o.LoginItem = login }})
	if !h.view().FirstRun {
		t.Errorf("FirstRun should start true")
	}
	mustOK(t, h.e.DismissFirstRun())
	if h.view().FirstRun {
		t.Errorf("FirstRun after dismissing")
	}
	if info, err := os.Stat(filepath.Join(h.dir, "first-run-done")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("first-run-done: %v, %v; want it readable only by the user", info.Mode().Perm(), err)
	}
	mustOK(t, h.e.SetOpenAtLogin(true))
	if !login.on || !h.view().OpenAtLogin {
		t.Errorf("open at login: %v %v", login.on, h.view().OpenAtLogin)
	}
	mustOK(t, h.e.SetOpenAtLogin(false))
	if login.on || h.view().OpenAtLogin {
		t.Errorf("open at login off: %v %v", login.on, h.view().OpenAtLogin)
	}
	h2 := newHarness(t, setup{state: func(t *testing.T, dir string) {
		os.WriteFile(filepath.Join(dir, "first-run-done"), nil, 0o644)
	}})
	if h2.view().FirstRun {
		t.Errorf("FirstRun with first-run-done present")
	}
	mustErr(t, h2.e.SetOpenAtLogin(true), "Open at login isn't available here.")
}

// fakeLogin is a LoginItem.
type fakeLogin struct{ on bool }

func (f *fakeLogin) IsEnabled() (bool, error) { return f.on, nil }
func (f *fakeLogin) Enable() error            { f.on = true; return nil }
func (f *fakeLogin) Disable() error           { f.on = false; return nil }

func TestDemo(t *testing.T) {
	h := newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	v := h.view()
	if v.Demo == nil {
		t.Fatal("View.Demo should be set")
	}
	mustOK(t, h.e.SetDemoScreen(platform.DemoLaptop, 0))
	v = h.view()
	if v.Demo.Screen != platform.DemoLaptop || v.ScreenWidth != 1512 || v.SizeClass != "compact" {
		t.Errorf("laptop: %+v width %v class %s", v.Demo, v.ScreenWidth, v.SizeClass)
	}
	mustOK(t, h.e.SetDemoScreen(platform.DemoFit, 2000))
	if v := h.view(); v.ScreenWidth != 2000 || v.SizeClass != "wide" {
		t.Errorf("fit: width %v class %s", v.ScreenWidth, v.SizeClass)
	}
	h2 := newHarness(t, setup{})
	if h2.view().Demo != nil {
		t.Errorf("View.Demo outside the preview")
	}
	if err := h2.e.SetDemoScreen("laptop", 0); err == nil {
		t.Errorf("SetDemoScreen should need the demo platform")
	}
}

func TestKeysAndSettings(t *testing.T) {
	h := newHarness(t, setup{config: strings.Replace(string(config.ExampleTOML()),
		`jump = "ctrl+alt+space"`, `jump = ""`, 1)})
	v := h.view()
	want := KeysView{Tabs: "⌃⌥", NextTab: "⌃⌥]", PreviousTab: "⌃⌥[", Jump: "", Rearrange: "⌃⌥0",
		FocusPane: "⌃⌥", MoveWindow: "⌃⌥⇧", NextWindow: "⌃⌥Tab", ZoomPane: "⌃⌥↩"}
	if v.Keys != want {
		t.Errorf("keys: got %+v\nwant %+v", v.Keys, want)
	}
	if v.Tabs[0].Shortcut != "⌃⌥1" || v.Tabs[2].Shortcut != "⌃⌥3" {
		t.Errorf("tab shortcuts: %q %q", v.Tabs[0].Shortcut, v.Tabs[2].Shortcut)
	}
	s := v.Settings
	if !s.FillScreen || s.CompactBelow != 1800 || s.Gap != 4 || !s.ShowStatusBar || s.TabShortcutHints != "auto" ||
		!s.OpenMissingApps || !s.HideOtherTabs || s.HideOtherApps ||
		s.NewWindows != "app-pane" || s.FollowFocus != "switch-tab" || s.LivePreviews || s.LaunchTimeout != "15s" {
		t.Errorf("settings: %+v", s)
	}
	if info := h.e.ConfigCopy(); info.LaunchTimeout != 15*time.Second || info.Path != filepath.Join(h.dir, "config.toml") {
		t.Errorf("ConfigCopy: %+v", info)
	}
}

func TestReadyLowersBench(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SetOverlay(true))
	if !h.p.Raised() {
		t.Fatal("SetOverlay(true) should raise Bench")
	}
	v, err := h.e.Ready()
	mustOK(t, err)
	if h.p.Raised() {
		t.Errorf("Ready should lower Bench")
	}
	if len(v.Tabs) != 3 {
		t.Errorf("Ready's view: %d tabs", len(v.Tabs))
	}
}
