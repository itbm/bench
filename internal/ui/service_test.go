//go:build darwin || server

package ui

import (
	"errors"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/workspace"
)

func TestServiceDelegates(t *testing.T) {
	area := layout.Rect{X: 0, Y: 53, W: 1440, H: 815}
	source := workspace.Source{Type: "web", URL: "https://example.com"}
	draft := workspace.TabDraft{Name: "Docs", Preset: "side", SmallScreens: "stack"}
	settings := workspace.SettingsView{CompactBelow: 1600, Gap: 8, LivePreviews: true, LaunchTimeout: "15s"}
	launcher := workspace.LauncherView{Name: "Claude", Kind: "terminal", Folder: "~/code/bench", Command: "claude", StartAgain: true}
	changes := map[string]string{"jump": "ctrl+alt+j"}
	tests := []struct {
		name string
		call func(s *Service) error
		want call
	}{
		{"Ready", func(s *Service) error { _, err := s.Ready(); return err }, call{"Ready", nil}},
		{"SetPaneArea", func(s *Service) error { return s.SetPaneArea(area) }, call{"SetPaneArea", []any{area}}},
		{"SwitchTab", func(s *Service) error { return s.SwitchTab(2) }, call{"SwitchTab", []any{2}}},
		{"StepTab", func(s *Service) error { return s.StepTab(-1) }, call{"StepTab", []any{-1}}},
		{"LastTab", func(s *Service) error { return s.LastTab() }, call{"LastTab", nil}},
		{"SetMode", func(s *Service) error { return s.SetMode("compact") }, call{"SetMode", []any{"compact"}}},
		{"FocusPane", func(s *Service) error { return s.FocusPane("p2") }, call{"FocusPane", []any{"p2"}}},
		{"FocusUI", func(s *Service) error { return s.FocusUI() }, call{"FocusUI", nil}},
		{"ShowItem", func(s *Service) error { return s.ShowItem("p1", "w3") }, call{"ShowItem", []any{"p1", "w3"}}},
		{"MoveItem", func(s *Service) error { return s.MoveItem("w3", 1, "p4", "right") },
			call{"MoveItem", []any{"w3", 1, "p4", "right"}}},
		{"MoveItemAt", func(s *Service) error { return s.MoveItemAt("w3", 1, "p4", 2) },
			call{"MoveItemAt", []any{"w3", 1, "p4", 2}}},
		{"ShowSideBySide", func(s *Service) error { return s.ShowSideBySide("w3") }, call{"ShowSideBySide", []any{"w3"}}},
		{"MoveItemToTab", func(s *Service) error { return s.MoveItemToTab("w3", 0) }, call{"MoveItemToTab", []any{"w3", 0}}},
		{"SendToTab", func(s *Service) error { return s.SendToTab(2) }, call{"SendToTab", []any{2}}},
		{"SendToLastTab", func(s *Service) error { return s.SendToLastTab() }, call{"SendToLastTab", nil}},
		{"AddItemToTab", func(s *Service) error { return s.AddItemToTab("w3", 2) }, call{"AddItemToTab", []any{"w3", 2}}},
		{"RemoveItemFromTab", func(s *Service) error { return s.RemoveItemFromTab("w3", 1) },
			call{"RemoveItemFromTab", []any{"w3", 1}}},
		{"MoveItemToNewTab", func(s *Service) error { return s.MoveItemToNewTab("w3", true) },
			call{"MoveItemToNewTab", []any{"w3", true}}},
		{"RenameItem", func(s *Service) error { return s.RenameItem("w5", "API server") },
			call{"RenameItem", []any{"w5", "API server"}}},
		{"FocusDirection", func(s *Service) error { return s.FocusDirection("left") }, call{"FocusDirection", []any{"left"}}},
		{"MoveDirection", func(s *Service) error { return s.MoveDirection("down") }, call{"MoveDirection", []any{"down"}}},
		{"CycleSubtab", func(s *Service) error { return s.CycleSubtab(1) }, call{"CycleSubtab", []any{1}}},
		{"AddOptions", func(s *Service) error { _, err := s.AddOptions("p1"); return err }, call{"AddOptions", []any{"p1"}}},
		{"AddToPane", func(s *Service) error { return s.AddToPane("p1", source) }, call{"AddToPane", []any{"p1", source}}},
		{"AddWindowsToPane", func(s *Service) error { return s.AddWindowsToPane("p1", []string{"f7", "w3"}) },
			call{"AddWindowsToPane", []any{"p1", []string{"f7", "w3"}}}},
		{"AddWebsite", func(s *Service) error { _, err := s.AddWebsite("p1", "squoosh.app"); return err },
			call{"AddWebsite", []any{"p1", "squoosh.app"}}},
		{"AddWebAppWhenReady", func(s *Service) error { _, err := s.AddWebAppWhenReady("p1", "squoosh.app"); return err },
			call{"AddWebAppWhenReady", []any{"p1", "squoosh.app"}}},
		{"WebAppChoice", func(s *Service) error { _, err := s.WebAppChoice("w1"); return err },
			call{"WebAppChoice", []any{"w1"}}},
		{"UseWebApp", func(s *Service) error { return s.UseWebApp("w1") }, call{"UseWebApp", []any{"w1"}}},
		{"MakeWebApp", func(s *Service) error { _, err := s.MakeWebApp("w2"); return err },
			call{"MakeWebApp", []any{"w2"}}},
		{"UseWebAppWhenReady", func(s *Service) error { _, err := s.UseWebAppWhenReady("w2"); return err },
			call{"UseWebAppWhenReady", []any{"w2"}}},
		{"PopOut", func(s *Service) error { return s.PopOut("w2") }, call{"PopOut", []any{"w2"}}},
		{"Redock", func(s *Service) error { return s.Redock("w2") }, call{"Redock", []any{"w2"}}},
		{"AddFrontWindow", func(s *Service) error { return s.AddFrontWindow() }, call{"AddFrontWindow", nil}},
		{"BackToBench", func(s *Service) error { return s.BackToBench("w2") }, call{"BackToBench", []any{"w2"}}},
		{"OpenItem", func(s *Service) error { return s.OpenItem("w3") }, call{"OpenItem", []any{"w3"}}},
		{"RemoveRemembered", func(s *Service) error { return s.RemoveRemembered("p1", "com.apple.TextEdit") },
			call{"RemoveRemembered", []any{"p1", "com.apple.TextEdit"}}},
		{"ClosePane", func(s *Service) error { return s.ClosePane("p2") }, call{"ClosePane", []any{"p2"}}},
		{"SetSizes", func(s *Service) error { return s.SetSizes("s1", []float64{40, 60}) },
			call{"SetSizes", []any{"s1", []float64{40, 60}}}},
		{"EvenSizes", func(s *Service) error { return s.EvenSizes("s1") }, call{"EvenSizes", []any{"s1"}}},
		{"ZoomPane", func(s *Service) error { return s.ZoomPane("p2") }, call{"ZoomPane", []any{"p2"}}},
		{"ShowAllPanes", func(s *Service) error { return s.ShowAllPanes() }, call{"ShowAllPanes", nil}},
		{"SplitPane", func(s *Service) error { _, err := s.SplitPane("p2", "right"); return err },
			call{"SplitPane", []any{"p2", "right"}}},
		{"CancelSplit", func(s *Service) error { return s.CancelSplit("p9") }, call{"CancelSplit", []any{"p9"}}},
		{"Rearrange", func(s *Service) error { return s.Rearrange() }, call{"Rearrange", nil}},
		{"FillScreen", func(s *Service) error { return s.FillScreen() }, call{"FillScreen", nil}},
		{"PreviewTree", func(s *Service) error { _, err := s.PreviewTree(-1, "grid"); return err },
			call{"PreviewTree", []any{-1, "grid"}}},
		{"SaveTab", func(s *Service) error { return s.SaveTab(-1, draft) }, call{"SaveTab", []any{-1, draft}}},
		{"PreviewTabFromWindows", func(s *Service) error { _, err := s.PreviewTabFromWindows("", []string{"f7"}); return err },
			call{"PreviewTabFromWindows", []any{"", []string{"f7"}}}},
		{"NewTabFromWindows", func(s *Service) error { return s.NewTabFromWindows(draft, []string{"f7", "w3"}) },
			call{"NewTabFromWindows", []any{draft, []string{"f7", "w3"}}}},
		{"SaveSettings", func(s *Service) error { return s.SaveSettings(settings) }, call{"SaveSettings", []any{settings}}},
		{"CheckShortcuts", func(s *Service) error { _, err := s.CheckShortcuts(changes); return err },
			call{"CheckShortcuts", []any{changes}}},
		{"PauseShortcuts", func(s *Service) error { return s.PauseShortcuts(true) }, call{"PauseShortcuts", []any{true}}},
		{"UseSuggestedShortcuts", func(s *Service) error { return s.UseSuggestedShortcuts("jump zoom_pane") },
			call{"UseSuggestedShortcuts", []any{"jump zoom_pane"}}},
		{"SaveLauncher", func(s *Service) error { _, err := s.SaveLauncher(launcher); return err },
			call{"SaveLauncher", []any{launcher}}},
		{"ChooseFolder", func(s *Service) error { _, err := s.ChooseFolder("~/code"); return err },
			call{"ChooseFolder", []any{"~/code"}}},
		{"BrowseToAdd", func(s *Service) error { _, err := s.BrowseToAdd("p1"); return err },
			call{"BrowseToAdd", []any{"p1"}}},
		{"DeleteTab", func(s *Service) error { return s.DeleteTab(3) }, call{"DeleteTab", []any{3}}},
		{"MoveTab", func(s *Service) error { return s.MoveTab(2, 0) }, call{"MoveTab", []any{2, 0}}},
		{"CloseTab", func(s *Service) error { _, err := s.CloseTab(1, true, false); return err },
			call{"CloseTab", []any{1, true, false}}},
		{"CloseItem", func(s *Service) error { _, err := s.CloseItem("w2", false, true); return err },
			call{"CloseItem", []any{"w2", false, true}}},
		{"ResetLayout", func(s *Service) error { return s.ResetLayout(1) }, call{"ResetLayout", []any{1}}},
		{"Undo", func(s *Service) error { return s.Undo(4) }, call{"Undo", []any{4}}},
		{"ReopenTab", func(s *Service) error { return s.ReopenTab("Code") }, call{"ReopenTab", []any{"Code"}}},
		{"TermInput", func(s *Service) error { return s.TermInput("w5", "ls\r") }, call{"TermInput", []any{"w5", "ls\r"}}},
		{"TermResize", func(s *Service) error { return s.TermResize("w5", 120, 40) }, call{"TermResize", []any{"w5", 120, 40}}},
		{"TermSnapshot", func(s *Service) error { _, err := s.TermSnapshot("w5"); return err },
			call{"TermSnapshot", []any{"w5"}}},
		{"SetOverlay", func(s *Service) error { return s.SetOverlay(true) }, call{"SetOverlay", []any{true}}},
		{"ShowPreview", func(s *Service) error { return s.ShowPreview("w2", area) }, call{"ShowPreview", []any{"w2", area}}},
		{"HidePreview", func(s *Service) error { return s.HidePreview() }, call{"HidePreview", nil}},
		{"RequestAccessibility", func(s *Service) error { return s.RequestAccessibility() }, call{"RequestAccessibility", nil}},
		{"RequestScreenRecording", func(s *Service) error { return s.RequestScreenRecording() },
			call{"RequestScreenRecording", nil}},
		{"SetOpenAtLogin", func(s *Service) error { return s.SetOpenAtLogin(true) }, call{"SetOpenAtLogin", []any{true}}},
		{"OpenLoginItems", func(s *Service) error { return s.OpenLoginItems() }, call{"OpenLoginItems", nil}},
		{"DismissFirstRun", func(s *Service) error { return s.DismissFirstRun() }, call{"DismissFirstRun", nil}},
		{"StarterTabs", func(s *Service) error { _, err := s.StarterTabs(); return err }, call{"StarterTabs", nil}},
		{"UseStarterTabs", func(s *Service) error { return s.UseStarterTabs([]string{"Comms", "Code"}) },
			call{"UseStarterTabs", []any{[]string{"Comms", "Code"}}}},
		{"SetDialogSize", func(s *Service) error { return s.SetDialogSize("settings", 640, 520) },
			call{"SetDialogSize", []any{"settings", 640.0, 520.0}}},
		{"SetDemoScreen", func(s *Service) error { return s.SetDemoScreen("fit", 1280) },
			call{"SetDemoScreen", []any{"fit", 1280.0}}},
		{"DemoMinimise", func(s *Service) error { return s.DemoMinimise() }, call{"DemoMinimise", nil}},
		{"TermAck", func(s *Service) error { return s.TermAck("w5", 4096) }, call{"TermAck", []any{"w5", int64(4096)}}},
		{"TermClear", func(s *Service) error { return s.TermClear("w5") }, call{"TermClear", []any{"w5"}}},
		{"OpenLink", func(s *Service) error { return s.OpenLink("https://example.com/") },
			call{"OpenLink", []any{"https://example.com/"}}},
		{"OpenInBrowser", func(s *Service) error { return s.OpenInBrowser("w1") }, call{"OpenInBrowser", []any{"w1"}}},
		{"ReloadPage", func(s *Service) error { return s.ReloadPage("w1") }, call{"ReloadPage", []any{"w1"}}},
		{"GoBack", func(s *Service) error { return s.GoBack("w1") }, call{"GoBack", []any{"w1"}}},
		{"GoForward", func(s *Service) error { return s.GoForward("w1") }, call{"GoForward", []any{"w1"}}},
		{"ZoomPage", func(s *Service) error { return s.ZoomPage("w1", -1) }, call{"ZoomPage", []any{"w1", -1}}},
		{"PageLink", func(s *Service) error { _, err := s.PageLink("w1"); return err }, call{"PageLink", []any{"w1"}}},
		{"SetFindBar", func(s *Service) error { return s.SetFindBar("w1", true) }, call{"SetFindBar", []any{"w1", true}}},
		{"FindInPage", func(s *Service) error { _, err := s.FindInPage("w1", "huddle", true); return err },
			call{"FindInPage", []any{"w1", "huddle", true}}},
		{"MuteCapture", func(s *Service) error { return s.MuteCapture("w1", "microphone", true) },
			call{"MuteCapture", []any{"w1", "microphone", true}}},
		{"StopMedia", func(s *Service) error { return s.StopMedia("w1") }, call{"StopMedia", []any{"w1"}}},
		{"ReloadConfig", func(s *Service) error { return s.ReloadConfig() }, call{"ReloadConfig", nil}},
		{"OverwriteConfig", func(s *Service) error { return s.OverwriteConfig() }, call{"OverwriteConfig", nil}},
		{"UseConfigBackup", func(s *Service) error { return s.UseConfigBackup() }, call{"UseConfigBackup", nil}},
		{"OpenConfig", func(s *Service) error { return s.OpenConfig() }, call{"OpenConfig", nil}},
		{"ShowConfigInFinder", func(s *Service) error { return s.ShowConfigInFinder() }, call{"ShowConfigInFinder", nil}},
		{"RestoreArrangement", func(s *Service) error { return s.RestoreArrangement() }, call{"RestoreArrangement", nil}},
		{"DismissArrangementNotice", func(s *Service) error { return s.DismissArrangementNotice() },
			call{"DismissArrangementNotice", nil}},
		{"DismissUpdate", func(s *Service) error { return s.DismissUpdate() }, call{"DismissUpdate", nil}},
		{"InstallUpdate", func(s *Service) error { return s.InstallUpdate() }, call{"InstallUpdate", nil}},
		{"Quit", func(s *Service) error { return s.Quit(true) }, call{"Quit", []any{true}}},
		{"QuitAndReopen", func(s *Service) error { return s.QuitAndReopen(false, true) },
			call{"QuitAndReopen", []any{false, true}}},
		{"CloseTerminal", func(s *Service) error { return s.CloseTerminal("w5", false) },
			call{"CloseTerminal", []any{"w5", false}}},
	}
	refused := errors.New("a pane can't be split by its only window")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeEngine()
			if err := tt.call(newService(f)); err != nil {
				t.Fatalf("%s: unexpected error %v", tt.name, err)
			}
			if got := f.Calls(); len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
				t.Errorf("engine calls = %v, want [%v]", got, tt.want)
			}

			f = newFakeEngine()
			f.err = refused
			if err := tt.call(newService(f)); !errors.Is(err, refused) {
				t.Errorf("with a refusing engine, error = %v, want %v", err, refused)
			}
		})
	}
}

func TestServiceResults(t *testing.T) {
	f := newFakeEngine()
	f.view = workspace.View{Current: 2, Mode: "wide"}
	s := newService(f)

	view, err := s.Ready()
	if err != nil || view.Current != 2 || view.Mode != "wide" {
		t.Errorf("Ready() = %+v, %v; want the engine's view", view, err)
	}
	opts, err := s.AddOptions("p1")
	if err != nil || len(opts) != 1 || opts[0].Label != "New terminal" {
		t.Errorf("AddOptions() = %+v, %v; want the engine's rows", opts, err)
	}
	site, err := s.AddWebsite("p1", "squoosh.app")
	if err != nil || site.Browser != "Safari" || site.Added != "" {
		t.Errorf("AddWebsite() = %+v, %v; want the engine's result", site, err)
	}
	if ready, err := s.AddWebAppWhenReady("p1", "squoosh.app"); err != nil || !ready {
		t.Errorf("AddWebAppWhenReady() = %v, %v; want the engine's result", ready, err)
	}
	choice, err := s.WebAppChoice("w1")
	if err != nil || choice != (workspace.WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}) {
		t.Errorf("WebAppChoice() = %+v, %v; want the engine's choice", choice, err)
	}
	made, err := s.MakeWebApp("w2")
	if err != nil || made.Browser != "Safari" || made.Steps == "" {
		t.Errorf("MakeWebApp() = %+v, %v; want the engine's result", made, err)
	}
	if ready, err := s.UseWebAppWhenReady("w2"); err != nil || !ready {
		t.Errorf("UseWebAppWhenReady() = %v, %v; want the engine's result", ready, err)
	}
	if id, err := s.SplitPane("p1", "bottom"); err != nil || id != "p9" {
		t.Errorf("SplitPane() = %q, %v; want the engine's new pane", id, err)
	}
	tree, err := s.PreviewTree(0, "single")
	if err != nil || tree == nil || tree.ID != "p1" {
		t.Errorf("PreviewTree() = %+v, %v; want the engine's tree", tree, err)
	}
	preview, err := s.PreviewTabFromWindows("", []string{"f7"})
	if err != nil || preview.Suggested != "side" || preview.Root == nil || preview.Root.ID != "p1" {
		t.Errorf("PreviewTabFromWindows() = %+v, %v; want the engine's preview", preview, err)
	}
	q, err := s.CloseTab(1, false, false)
	if err != nil || !reflect.DeepEqual(q, workspace.CloseQuestion{Ask: true, Tab: "Code", Windows: 2}) {
		t.Errorf("CloseTab() = %+v, %v; want the engine's question", q, err)
	}
	if folder, err := s.ChooseFolder("~"); err != nil || folder != "~/code/bench" {
		t.Errorf("ChooseFolder() = %q, %v; want the engine's folder", folder, err)
	}
	if r, err := s.BrowseToAdd("p1"); err != nil || r != (workspace.BrowseResult{Command: "claude"}) {
		t.Errorf("BrowseToAdd() = %+v, %v; want the engine's result", r, err)
	}
	if name, err := s.SaveLauncher(workspace.LauncherView{Command: "claude"}); err != nil || name != "claude" {
		t.Errorf("SaveLauncher() = %q, %v; want the engine's name", name, err)
	}
	if rows, err := s.CheckShortcuts(map[string]string{"jump": "ctrl+alt+j"}); err != nil || len(rows) != 1 || rows[0].Value != "ctrl+alt+j" {
		t.Errorf("CheckShortcuts() = %+v, %v; want the engine's rows", rows, err)
	}
	if tabs, err := s.StarterTabs(); err != nil || len(tabs) != 1 || tabs[0].Name != "Comms" || tabs[0].Apps[0].Name != "Mail" {
		t.Errorf("StarterTabs() = %+v, %v; want the engine's tabs", tabs, err)
	}
	snap, err := s.TermSnapshot("w1")
	if err != nil || snap.Data != "aGk=" || snap.Offset != 2 {
		t.Errorf("TermSnapshot() = %+v, %v; want the engine's snapshot", snap, err)
	}
	if link, err := s.PageLink("w1"); err != nil || link != "https://app.slack.com/client" {
		t.Errorf("PageLink() = %q, %v; want the engine's link", link, err)
	}
	if found, err := s.FindInPage("w1", "huddle", false); err != nil || !found {
		t.Errorf("FindInPage() = %v, %v; want the engine's answer", found, err)
	}
}

func TestServiceWithoutEngine(t *testing.T) {
	var s Service
	if _, err := s.Ready(); !errors.Is(err, errNotReady) {
		t.Errorf("Ready() error = %v, want errNotReady", err)
	}
	if err := s.SwitchTab(0); !errors.Is(err, errNotReady) {
		t.Errorf("SwitchTab() error = %v, want errNotReady", err)
	}
	if _, err := s.AddWebsite("p1", "squoosh.app"); !errors.Is(err, errNotReady) {
		t.Errorf("AddWebsite() error = %v, want errNotReady", err)
	}
	if _, err := s.AddWebAppWhenReady("p1", "squoosh.app"); !errors.Is(err, errNotReady) {
		t.Errorf("AddWebAppWhenReady() error = %v, want errNotReady", err)
	}
	if _, err := s.WebAppChoice("w1"); !errors.Is(err, errNotReady) {
		t.Errorf("WebAppChoice() error = %v, want errNotReady", err)
	}
	if _, err := s.PageLink("w1"); !errors.Is(err, errNotReady) {
		t.Errorf("PageLink() error = %v, want errNotReady", err)
	}
	if _, err := s.FindInPage("w1", "huddle", false); !errors.Is(err, errNotReady) {
		t.Errorf("FindInPage() error = %v, want errNotReady", err)
	}
	if _, err := s.SplitPane("p1", "right"); !errors.Is(err, errNotReady) {
		t.Errorf("SplitPane() error = %v, want errNotReady", err)
	}
	if _, err := s.CloseTab(0, false, false); !errors.Is(err, errNotReady) {
		t.Errorf("CloseTab() error = %v, want errNotReady", err)
	}
	if _, err := s.CloseItem("w1", false, false); !errors.Is(err, errNotReady) {
		t.Errorf("CloseItem() error = %v, want errNotReady", err)
	}
	if _, err := s.MakeWebApp("w1"); !errors.Is(err, errNotReady) {
		t.Errorf("MakeWebApp() error = %v, want errNotReady", err)
	}
	if _, err := s.UseWebAppWhenReady("w1"); !errors.Is(err, errNotReady) {
		t.Errorf("UseWebAppWhenReady() error = %v, want errNotReady", err)
	}
	if _, err := s.ChooseFolder("~"); !errors.Is(err, errNotReady) {
		t.Errorf("ChooseFolder() error = %v, want errNotReady", err)
	}
	if _, err := s.StarterTabs(); !errors.Is(err, errNotReady) {
		t.Errorf("StarterTabs() error = %v, want errNotReady", err)
	}
	if _, err := s.BrowseToAdd("p1"); !errors.Is(err, errNotReady) {
		t.Errorf("BrowseToAdd() error = %v, want errNotReady", err)
	}
	if _, err := s.SaveLauncher(workspace.LauncherView{}); !errors.Is(err, errNotReady) {
		t.Errorf("SaveLauncher() error = %v, want errNotReady", err)
	}
	if _, err := s.CheckShortcuts(nil); !errors.Is(err, errNotReady) {
		t.Errorf("CheckShortcuts() error = %v, want errNotReady", err)
	}
}

// boundMethodRE matches a method in the "Bound methods" table of
// docs/architecture.md, such as "| `SwitchTab(index int)` |".
var boundMethodRE = regexp.MustCompile("`([A-Z][A-Za-z]*)\\(")

// documentedMethods returns the method names in the "Bound methods" table.
func documentedMethods(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../../docs/architecture.md")
	if err != nil {
		t.Skipf("can't read the architecture notes: %v", err)
	}
	_, section, ok := strings.Cut(string(data), "### Bound methods")
	if !ok {
		t.Fatal("docs/architecture.md has no Bound methods section")
	}
	section, _, _ = strings.Cut(section, "\n### ")
	var names []string
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cell, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " |")
		for _, m := range boundMethodRE.FindAllStringSubmatch(cell, -1) {
			names = append(names, m[1])
		}
	}
	slices.Sort(names)
	return names
}

// TestServiceMatchesTheDocs checks that Service's methods, which are all
// bound, are exactly the documented bound methods.
func TestServiceMatchesTheDocs(t *testing.T) {
	want := documentedMethods(t)
	st := reflect.TypeFor[*Service]()
	var got []string
	for m := range st.Methods() {
		got = append(got, m.Name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("Service methods = %v\nwant the documented bound methods %v", got, want)
	}
}

// TestServiceMatchesTheEngine checks that each bound method has the same
// parameter and result types as the engine's method it delegates to.
func TestServiceMatchesTheEngine(t *testing.T) {
	st := reflect.TypeFor[*Service]()
	et := reflect.TypeFor[*workspace.Engine]()
	for m := range st.Methods() {
		em, ok := et.MethodByName(m.Name)
		if !ok {
			t.Errorf("workspace.Engine has no %s for Service.%s", m.Name, m.Name)
			continue
		}
		if got, want := signature(m.Type), signature(em.Type); got != want {
			t.Errorf("Service.%s is (%s), the engine's is (%s)", m.Name, got, want)
		}
	}
}

// signature describes a method's parameters and results, without the
// receiver.
func signature(ft reflect.Type) string {
	var parts []string
	for i := 1; i < ft.NumIn(); i++ {
		parts = append(parts, ft.In(i).String())
	}
	parts = append(parts, "->")
	for i := range ft.NumOut() {
		parts = append(parts, ft.Out(i).String())
	}
	return strings.Join(parts, ", ")
}
