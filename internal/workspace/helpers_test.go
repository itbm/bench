package workspace

import (
	"slices"
	"testing"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// testNamer names apps from their bundle IDs, as when none is installed.
func testNamer(bundleID string) string { return apps.NameFromBundleID(bundleID) }

// parseConfig parses a config for a test, failing it on problems.
func parseConfig(t *testing.T, text string) *config.Config {
	t.Helper()
	cfg, problems, err := config.Parse([]byte(text))
	if err != nil || len(problems) > 0 {
		t.Fatalf("config.Parse: %v %v", err, problems)
	}
	return cfg
}

// defaultModel builds the model for the default config.
func defaultModel(t *testing.T) *Model {
	t.Helper()
	m, err := modelFromConfig(config.Example(), testNamer)
	if err != nil {
		t.Fatalf("modelFromConfig: %v", err)
	}
	return m
}

// modelOf builds the model for a config.
func modelOf(t *testing.T, text string) (*Model, *config.Config) {
	t.Helper()
	cfg := parseConfig(t, text)
	m, err := modelFromConfig(cfg, testNamer)
	if err != nil {
		t.Fatalf("modelFromConfig: %v", err)
	}
	return m, cfg
}

// paneItems returns each pane's items in a tab, in reading order.
func paneItems(t *Tab) [][]string {
	var out [][]string
	for _, p := range t.Leaves() {
		out = append(out, append([]string{}, p.Items...))
	}
	return out
}

// equalPanes compares the items of each pane.
func equalPanes(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

// mustValid fails the test if any tab's tree isn't valid.
func mustValid(t *testing.T, m *Model) {
	t.Helper()
	for _, tab := range m.Tabs {
		if err := layout.Validate(tab.Root); err != nil {
			t.Fatalf("tab %s: %v", tab.Name, err)
		}
		if tab.Pane(tab.Focus) == nil {
			t.Fatalf("tab %s: focus %q isn't a pane", tab.Name, tab.Focus)
		}
	}
}

// win is a window for tests.
func win(id platform.WindowID, bundleID, title string, created int) platform.Window {
	return platform.Window{ID: id, BundleID: bundleID, Title: title, Subrole: platform.SubroleStandard, Created: created}
}

// addAppItem adds a bound App item to a pane.
func addAppItem(m *Model, t *Tab, p *layout.Node, bundleID string, w platform.Window) *Item {
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindApp, BundleID: bundleID, App: testNamer(bundleID)}
	m.Bind(it, w)
	m.Add(it, t, p, -1)
	return it
}
