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
	tests := []struct {
		name string
		call func(s *Service) error
		want call
	}{
		{"Ready", func(s *Service) error { _, err := s.Ready(); return err }, call{"Ready", nil}},
		{"SetPaneArea", func(s *Service) error { return s.SetPaneArea(area) }, call{"SetPaneArea", []any{area}}},
		{"SwitchTab", func(s *Service) error { return s.SwitchTab(2) }, call{"SwitchTab", []any{2}}},
		{"StepTab", func(s *Service) error { return s.StepTab(-1) }, call{"StepTab", []any{-1}}},
		{"SetMode", func(s *Service) error { return s.SetMode("compact") }, call{"SetMode", []any{"compact"}}},
		{"FocusPane", func(s *Service) error { return s.FocusPane("p2") }, call{"FocusPane", []any{"p2"}}},
		{"ShowItem", func(s *Service) error { return s.ShowItem("p1", "w3") }, call{"ShowItem", []any{"p1", "w3"}}},
		{"MoveItem", func(s *Service) error { return s.MoveItem("w3", 1, "p4", "right") },
			call{"MoveItem", []any{"w3", 1, "p4", "right"}}},
		{"MoveItemToTab", func(s *Service) error { return s.MoveItemToTab("w3", 0) }, call{"MoveItemToTab", []any{"w3", 0}}},
		{"FocusDirection", func(s *Service) error { return s.FocusDirection("left") }, call{"FocusDirection", []any{"left"}}},
		{"MoveDirection", func(s *Service) error { return s.MoveDirection("down") }, call{"MoveDirection", []any{"down"}}},
		{"CycleSubtab", func(s *Service) error { return s.CycleSubtab(1) }, call{"CycleSubtab", []any{1}}},
		{"AddOptions", func(s *Service) error { _, err := s.AddOptions("p1"); return err }, call{"AddOptions", []any{"p1"}}},
		{"AddToPane", func(s *Service) error { return s.AddToPane("p1", source) }, call{"AddToPane", []any{"p1", source}}},
		{"PopOut", func(s *Service) error { return s.PopOut("w2") }, call{"PopOut", []any{"w2"}}},
		{"Redock", func(s *Service) error { return s.Redock("w2") }, call{"Redock", []any{"w2"}}},
		{"SetSizes", func(s *Service) error { return s.SetSizes("s1", []float64{40, 60}) },
			call{"SetSizes", []any{"s1", []float64{40, 60}}}},
		{"Rearrange", func(s *Service) error { return s.Rearrange() }, call{"Rearrange", nil}},
		{"PreviewTree", func(s *Service) error { _, err := s.PreviewTree(-1, "grid"); return err },
			call{"PreviewTree", []any{-1, "grid"}}},
		{"SaveTab", func(s *Service) error { return s.SaveTab(-1, draft) }, call{"SaveTab", []any{-1, draft}}},
		{"DeleteTab", func(s *Service) error { return s.DeleteTab(3) }, call{"DeleteTab", []any{3}}},
		{"ResetLayout", func(s *Service) error { return s.ResetLayout(1) }, call{"ResetLayout", []any{1}}},
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
		{"DismissFirstRun", func(s *Service) error { return s.DismissFirstRun() }, call{"DismissFirstRun", nil}},
		{"SetDemoScreen", func(s *Service) error { return s.SetDemoScreen("fit", 1280) },
			call{"SetDemoScreen", []any{"fit", 1280.0}}},
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
	tree, err := s.PreviewTree(0, "single")
	if err != nil || tree == nil || tree.ID != "p1" {
		t.Errorf("PreviewTree() = %+v, %v; want the engine's tree", tree, err)
	}
	snap, err := s.TermSnapshot("w1")
	if err != nil || snap != "aGk=" {
		t.Errorf("TermSnapshot() = %q, %v; want the engine's snapshot", snap, err)
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
