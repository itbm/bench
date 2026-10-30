package workspace

import (
	"fmt"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file has the welcome guide's starting set-up: a few tabs that it
// suggests, built only from apps that are installed, for the user to take,
// leaving out any of them, or to start with the one tab that the first
// config has (decision 28).

// StarterTab is a tab that the welcome guide suggests: its name, and its
// windows, one for each pane, in reading order.
type StarterTab struct {
	Name string       `json:"name"`
	Apps []StarterApp `json:"apps"`
	// preset and small are its layout and small-screen setting.
	preset, small string
}

// StarterApp is one of a suggested tab's windows: an installed app's, or
// one of Bench's own terminals.
type StarterApp struct {
	// BundleID is the app's bundle ID, or "" for a terminal.
	BundleID string `json:"bundleId,omitempty"`
	Name     string `json:"name"`
	Mono     string `json:"mono"`
	// Icon is the address of the app's icon, or "" (see icons.go).
	Icon string `json:"icon,omitempty"`
}

// starterTerminal stands for one of Bench's terminals among a starter
// tab's apps.
const starterTerminal = ""

// starterTabs are the tabs that the welcome guide may suggest, in order:
// each pane takes the first of its apps that's installed, by bundle ID,
// and that no tab has already. A tab with no app, other than a terminal,
// isn't suggested, nor is one whose name a tab has. preset is its layout
// with two panes; with one, it's single.
var starterTabs = []struct {
	name, preset, small string
	panes               [][]string
}{
	{"Comms", layout.PresetSide, string(layout.Tabs), [][]string{
		{"com.tinyspeck.slackmacgap", "com.microsoft.teams2", "com.microsoft.teams", "com.hnc.Discord"},
		{"com.microsoft.Outlook", "com.apple.mail"},
	}},
	{"Code", layout.PresetMainSide, "", [][]string{
		{"com.microsoft.VSCode", "com.todesktop.230313mzl4w4u92", "dev.zed.Zed", "com.apple.dt.Xcode"},
		{starterTerminal},
	}},
	{"Plan", layout.PresetSide, string(layout.Tabs), [][]string{
		{"md.obsidian", "notion.id", "com.apple.Notes"},
		{"com.flexibits.fantastical2.mac", "com.apple.iCal"},
	}},
}

// StarterTabs returns the tabs that the welcome guide suggests for a
// starting set-up. The scan of the installed apps reads folders, so it's
// made without the lock.
func (e *Engine) StarterTabs() ([]StarterTab, error) {
	installed := e.p.InstalledApps()
	e.lock()
	defer e.unlock()
	e.setInstalledLocked(installed)
	return e.starterTabsLocked(), nil
}

// starterTabsLocked returns the tabs to suggest, from the installed apps
// as last scanned.
func (e *Engine) starterTabsLocked() []StarterTab {
	taken := map[string]bool{}
	for _, it := range e.model.Items {
		if it.Kind == KindApp && len(e.model.TabsOf(it.ID)) > 0 {
			taken[strings.ToLower(it.BundleID)] = true
		}
	}
	var out []StarterTab
	for _, s := range starterTabs {
		if slices.ContainsFunc(e.model.Tabs, func(t *Tab) bool { return strings.EqualFold(t.Name, s.name) }) {
			continue
		}
		t := StarterTab{Name: s.name, preset: s.preset, small: s.small}
		hasApp := false
		for _, pane := range s.panes {
			for _, id := range pane {
				if id == starterTerminal {
					t.Apps = append(t.Apps, StarterApp{Name: terminalApp, Mono: apps.Mono(terminalApp)})
					break
				}
				info, ok := e.installed[strings.ToLower(id)]
				if !ok || info.WebApp != "" || taken[strings.ToLower(id)] {
					continue
				}
				name := apps.DisplayName(info.Name)
				t.Apps = append(t.Apps, StarterApp{BundleID: info.BundleID, Name: name, Mono: apps.Mono(name),
					Icon: e.appIconLocked(info.BundleID)})
				hasApp = true
				break
			}
		}
		if hasApp {
			out = append(out, t)
		}
	}
	return out
}

// configTab returns a suggested tab as a [[tab]] entry: one pane for each
// of its windows.
func (t StarterTab) configTab() config.Tab {
	ct := config.Tab{Name: t.Name, Layout: layout.PresetSingle}
	if len(t.Apps) > 1 {
		ct.Layout, ct.SmallScreens = t.preset, t.small
	}
	for _, a := range t.Apps {
		item := config.Item{App: a.BundleID}
		if a.BundleID == starterTerminal {
			item = config.Item{Terminal: defaultFolder}
		}
		ct.Panes = append(ct.Panes, config.Pane{Items: []config.Item{item}})
	}
	return ct
}

// UseStarterTabs is the welcome guide's starting set-up: it adds the
// suggested tabs that names gives, after the others, as StarterTabs
// suggests them now, and saves config.toml as Tab properties saves a new
// tab. Bench stays on the current tab; the new tabs' apps open as the user
// switches to them. A name that Bench doesn't suggest any more is refused,
// and nothing is added.
func (e *Engine) UseStarterTabs(names []string) error {
	if len(names) == 0 {
		return nil
	}
	installed := e.p.InstalledApps()
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	e.setInstalledLocked(installed)
	suggested := e.starterTabsLocked()
	old := e.cfg
	cfg := old.Clone()
	var added []string
	for _, name := range names {
		i := slices.IndexFunc(suggested, func(t StarterTab) bool { return strings.EqualFold(t.Name, name) })
		if i < 0 {
			e.unlock()
			return fmt.Errorf("Bench doesn't suggest a tab called %s any more.", name)
		}
		if slices.Contains(added, suggested[i].Name) {
			continue
		}
		cfg.Tabs = append(cfg.Tabs, suggested[i].configTab())
		added = append(added, suggested[i].Name)
	}
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	e.cfg = cfg
	e.applyConfigLocked(old, cfg, true)
	e.forgetUndoLocked()
	e.changed()
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	msg := "Added " + joinNames(added) + "."
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.Toast(msg)
	return nil
}
