package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/itbm/bench/internal/layout"
)

// Item kinds in state.json. They match the workspace package's KindWeb,
// KindApp, and KindBuiltin.
const (
	stateKindWeb     = "web"
	stateKindApp     = "app"
	stateKindBuiltin = "builtin"
)

// LoadState reads state.json and checks it against the config with
// CheckState: it must parse, have version StateVersion, name the config's
// tabs in the same order, and have valid trees whose items all exist in
// Items. If it doesn't, the file is renamed to path plus ".bad"
// (state.json.bad for state.json) and LoadState returns (nil, nil). A
// missing file also returns (nil, nil). The error is for files that can't
// be read or set aside.
//
// It also tidies what it can without losing the arrangement: a Current
// outside the tabs becomes 0, an unknown Mode becomes auto, an unknown
// Preset becomes custom, a Focus that isn't one of the tab's panes (or
// layout.CompactPaneID) becomes its first pane, a CompactShown that isn't
// in the tab is cleared, items that are in no tree and not floating are
// dropped, and Seq is raised to at least the number at the end of every
// item, pane, and split ID (such as 12 for "w12"), so that new IDs can't
// repeat one in use.
func LoadState(path string, cfg *Config) (*State, error) {
	if cfg == nil {
		return nil, errors.New("loading the state: there's no config to check it against")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the state: %w", err)
	}
	var s State
	err = json.Unmarshal(data, &s)
	if err != nil {
		err = fmt.Errorf("it doesn't parse: %w", err)
	} else {
		err = CheckState(&s, cfg)
	}
	if err != nil {
		if rerr := os.Rename(path, path+".bad"); rerr != nil {
			return nil, fmt.Errorf("setting aside the state, because %v: %w", err, rerr)
		}
		return nil, nil
	}
	tidyState(&s)
	return &s, nil
}

// CheckState reports why a state doesn't fit a config, or returns nil: it
// must have version StateVersion and name the config's tabs in the same
// order, each with a valid tree (see layout.Validate); no pane or split ID
// can be in two tabs, and no pane can be called layout.CompactPaneID; every
// item in a tree or floating must be in Items, with a known kind (web, app,
// or builtin); and no item can be in a tab twice, or both floating and in a
// tab. An item can be in several tabs, once in each.
func CheckState(s *State, cfg *Config) error {
	if s == nil || cfg == nil {
		return errors.New("there's no state or no config to check")
	}
	if s.Version != StateVersion {
		return fmt.Errorf("it's version %d, not %d", s.Version, StateVersion)
	}
	if len(s.Tabs) != len(cfg.Tabs) {
		return fmt.Errorf("it has %d tabs, but the config has %d", len(s.Tabs), len(cfg.Tabs))
	}
	for i, t := range s.Tabs {
		if t.Name != cfg.Tabs[i].Name {
			return fmt.Errorf("tab %d is %q, but it's %q in the config", i+1, t.Name, cfg.Tabs[i].Name)
		}
	}

	seen := map[string]string{} // Item ID to where it is.
	place := func(id, where string) error {
		prev, ok := seen[id]
		if ok && (prev == where || where == "floating") {
			return fmt.Errorf("item %q is in %s and %s", id, prev, where)
		}
		seen[id] = where
		item, ok := s.Items[id]
		if !ok {
			return fmt.Errorf("item %q, in %s, isn't in items", id, where)
		}
		switch item.Kind {
		case stateKindWeb, stateKindApp, stateKindBuiltin:
		default:
			return fmt.Errorf("item %q has an unknown kind, %q", id, item.Kind)
		}
		return nil
	}
	for _, t := range s.Tabs {
		if t.Root == nil {
			return fmt.Errorf("tab %q has no tree", t.Name)
		}
		if err := layout.Validate(t.Root); err != nil {
			return fmt.Errorf("tab %q's tree isn't valid: %w", t.Name, err)
		}
	}
	if err := checkNodeIDs(s); err != nil {
		return err
	}
	for _, t := range s.Tabs {
		where := fmt.Sprintf("tab %q", t.Name)
		for _, id := range treeItems(t.Root) {
			if err := place(id, where); err != nil {
				return err
			}
		}
	}
	for _, id := range s.Floating {
		if err := place(id, "floating"); err != nil {
			return err
		}
	}
	return nil
}

// checkNodeIDs checks that no pane or split ID is in two tabs, because
// Bench names panes and splits by ID alone, and that no stored pane uses
// layout.CompactPaneID, which is the tabs view's own pane. layout.Validate
// has already checked the IDs within each tree.
func checkNodeIDs(s *State) error {
	tabOf := map[string]string{} // Node ID to the tab holding it.
	for _, t := range s.Tabs {
		var err error
		walkNodes(t.Root, func(n *layout.Node) {
			switch other, seen := tabOf[n.ID]; {
			case err != nil:
			case n.ID == layout.CompactPaneID:
				err = fmt.Errorf("tab %q has a pane called %q, which is reserved", t.Name, n.ID)
			case seen:
				err = fmt.Errorf("ID %q is in tab %q and tab %q", n.ID, other, t.Name)
			default:
				tabOf[n.ID] = t.Name
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// tidyState fixes what can be fixed in a state that passed CheckState.
func tidyState(s *State) {
	if s.Current < 0 || s.Current >= len(s.Tabs) {
		s.Current = 0
	}
	switch layout.Mode(s.Mode) {
	case layout.ModeAuto, layout.ModeWide, layout.ModeCompact:
	default:
		s.Mode = string(layout.ModeAuto)
	}
	used := map[string]bool{}
	for i := range s.Tabs {
		t := &s.Tabs[i]
		if _, ok := presetShapes[t.Preset]; !ok {
			t.Preset = layout.PresetCustom
		}
		panes := treePanes(t.Root)
		if len(panes) > 0 && !contains(panes, t.Focus) && t.Focus != layout.CompactPaneID {
			t.Focus = panes[0]
		}
		items := treeItems(t.Root)
		if !contains(items, t.CompactShown) {
			t.CompactShown = ""
		}
		for _, id := range items {
			used[id] = true
		}
	}
	for _, id := range s.Floating {
		used[id] = true
	}
	for id := range s.Items {
		if !used[id] {
			delete(s.Items, id)
		}
	}
	if s.Items == nil {
		s.Items = map[string]ItemState{}
	}
	raiseSeq(s)
}

// raiseSeq makes Seq at least the number at the end of every ID in use.
func raiseSeq(s *State) {
	raise := func(id string) {
		if n, ok := idNumber(id); ok && n > s.Seq {
			s.Seq = n
		}
	}
	for id := range s.Items {
		raise(id)
	}
	for _, t := range s.Tabs {
		walkNodes(t.Root, func(n *layout.Node) { raise(n.ID) })
	}
}

// treeItems returns the item IDs in a tree, in reading order. It doesn't
// assume the tree is valid.
func treeItems(n *layout.Node) []string {
	var out []string
	walkPanes(n, func(p *layout.Node) { out = append(out, p.Items...) })
	return out
}

// treePanes returns the pane IDs in a tree, in reading order.
func treePanes(n *layout.Node) []string {
	var out []string
	walkPanes(n, func(p *layout.Node) { out = append(out, p.ID) })
	return out
}

func walkPanes(n *layout.Node, f func(*layout.Node)) {
	walkNodes(n, func(n *layout.Node) {
		if n.Type == layout.TypePane {
			f(n)
		}
	})
}

// walkNodes calls f for every node in a tree, parents first. It doesn't
// assume the tree is valid, but it mustn't be given a tree that holds
// itself, which layout.Validate rejects and JSON can't express.
func walkNodes(n *layout.Node, f func(*layout.Node)) {
	if n == nil {
		return
	}
	f(n)
	if n.Type == layout.TypePane {
		return
	}
	for _, c := range n.Children {
		walkNodes(c, f)
	}
}

// idNumber returns the number at the end of a generated ID, such as 12 for
// "w12".
func idNumber(id string) (int, bool) {
	i := len(id)
	for i > 0 && id[i-1] >= '0' && id[i-1] <= '9' {
		i--
	}
	if i == len(id) {
		return 0, false
	}
	n, err := strconv.Atoi(id[i:])
	if err != nil {
		return 0, false
	}
	return n, true
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// SaveState writes state.json atomically, creating the folder if needed.
func SaveState(path string, s *State) error {
	if s == nil {
		return errors.New("saving the state: there's no state to save")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("saving the state: %w", err)
	}
	data = append(data, '\n')
	perm := os.FileMode(filePerm)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, data, perm); err != nil {
		return fmt.Errorf("saving the state: %w", err)
	}
	return nil
}
