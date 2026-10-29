package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/layout"
)

// Item kinds in state.json. They match the workspace package's KindWeb,
// KindApp, and KindBuiltin.
const (
	stateKindWeb     = "web"
	stateKindApp     = "app"
	stateKindBuiltin = "builtin"
)

// LoadState reads state.json, checks it with CheckState, and tidies it
// (see below). A missing file returns nothing.
//
// Its tabs needn't be the config's: MatchTabs pairs them by name, and
// Bench keeps the arrangement of each tab the config still has. A file that
// doesn't pass CheckState is set aside whole: it's renamed to path plus
// ".bad" (state.json.bad for state.json), and LoadState returns no state. A
// file that passes, but has tabs the config has no tab for, is returned,
// and a copy of it is set aside, because Bench drops those tabs'
// arrangement when it next saves. Either way, the SetAside says what was
// lost, and why. The copies set aside before move along to
// state.json.bad.1 and state.json.bad.2, and the oldest goes, so the last
// three are kept. The error is for files that can't be read or set aside:
// a state whose copy couldn't be kept is still returned with it.
//
// It also tidies what it can without losing the arrangement: a Current
// outside the tabs becomes 0, an unknown Mode becomes auto, an unknown
// Preset becomes custom, a Focus that isn't one of the tab's panes (or
// layout.CompactPaneID) becomes its first pane, a pane with items but no
// Active shows its first item, a CompactShown that isn't in the tab is
// cleared, items that are in no tree and not floating are dropped, and so
// are dialog sizes that aren't positive, and Seq is raised to at least the
// number at the end of every item, pane, and split ID (such as 12 for
// "w12"), so that new IDs can't repeat one in use.
func LoadState(path string, cfg *Config) (*State, *SetAside, error) {
	if cfg == nil {
		return nil, nil, errors.New("loading the state: there's no config to check it against")
	}
	s, bad, err := readState(path)
	switch {
	case err != nil:
		return nil, nil, err
	case bad != nil:
		if err := setAside(path, true); err != nil {
			return nil, nil, fmt.Errorf("setting aside the state, because %v: %w", bad, err)
		}
		return nil, wholeAside(s, bad), nil
	case s == nil:
		return nil, nil, nil
	}
	had := s.TabNames()
	dropped := Dropped(MatchTabs(had, cfg.TabNames()), len(had))
	if len(dropped) == 0 {
		return s, nil, nil
	}
	aside := &SetAside{}
	for _, i := range dropped {
		aside.Tabs = append(aside.Tabs, had[i])
	}
	if err := setAside(path, false); err != nil {
		return s, aside, fmt.Errorf("keeping a copy of the state: %w", err)
	}
	return s, aside, nil
}

// SetAside says what LoadState set aside, and why.
type SetAside struct {
	// Tabs names the saved tabs whose arrangement Bench can't keep: those
	// the config has no tab for, or all of them if the whole file was set
	// aside, as far as it could read them.
	Tabs []string
	// Reason is why the whole file was set aside, such as that it doesn't
	// parse, or nil if only a copy was, because the config has none of
	// Tabs.
	Reason error
}

// wholeAside says that state.json, read as s if it parsed, was set aside
// whole, because of bad.
func wholeAside(s *State, bad error) *SetAside {
	aside := &SetAside{Reason: bad}
	if s != nil {
		aside.Tabs = s.TabNames()
	}
	return aside
}

// PeekState is LoadState, but sets nothing aside: for a state.json that
// doesn't pass CheckState, it returns no state, and a SetAside that says
// why, and leaves the file where it is. It's for when the config isn't
// config.toml's, as while config.toml has problems: state.json may fit
// config.toml once they're fixed, so Bench mustn't save over it meanwhile
// either. Once it's about to, it sets the file aside (SetAsideWhole or
// SetAsideCopy).
func PeekState(path string) (*State, *SetAside, error) {
	s, bad, err := readState(path)
	switch {
	case err != nil:
		return nil, nil, err
	case bad != nil:
		return nil, wholeAside(s, bad), nil
	}
	return s, nil, nil
}

// SetAsideCopy keeps a copy of state.json as state.json.bad, as LoadState
// does for a file with tabs that the config has no tab for, moving the
// earlier copies along: for a file that PeekState read, or whose tabs a
// reload of config.toml drops, once Bench is about to save over it.
func SetAsideCopy(path string) error {
	return setAside(path, false)
}

// SetAsideWhole moves state.json to state.json.bad, as LoadState does for
// a file that doesn't pass CheckState, moving the earlier copies along:
// for a file that PeekState couldn't use, once Bench is about to save over
// it.
func SetAsideWhole(path string) error {
	return setAside(path, true)
}

// keptBad is how many set-aside copies of state.json Bench keeps:
// state.json.bad, state.json.bad.1, and state.json.bad.2.
const keptBad = 3

// setAside moves the file at path to path plus ".bad", or copies it there
// if move isn't set. First, the copies set aside before move along:
// state.json.bad.1 to state.json.bad.2, replacing it, and state.json.bad to
// state.json.bad.1.
func setAside(path string, move bool) error {
	bad := path + ".bad"
	for i := keptBad - 1; i > 0; i-- {
		from := bad
		if i > 1 {
			from += "." + strconv.Itoa(i-1)
		}
		if err := os.Rename(from, bad+"."+strconv.Itoa(i)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if move {
		return os.Rename(path, bad)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	perm := os.FileMode(filePerm)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	return writeFileAtomic(bad, data, perm)
}

// readState reads state.json, and checks and tidies it. If it doesn't pass
// CheckState, readState returns why as bad, with the state as read if it
// parsed. A missing file returns nothing.
func readState(path string) (s *State, bad error, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the state: %w", err)
	}
	s = &State{}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("it doesn't parse: %w", err), nil
	}
	if err := CheckState(s); err != nil {
		return s, err, nil
	}
	tidyState(s)
	return s, nil, nil
}

// TabNames returns the names of the state's tabs, in order.
func (s *State) TabNames() []string {
	out := make([]string, len(s.Tabs))
	for i, t := range s.Tabs {
		out[i] = t.Name
	}
	return out
}

// MatchTabs pairs a list of tabs with the list they had before, such as
// the config's tabs now with those that state.json names, so that each tab
// keeps its arrangement. It returns, for each tab in now, the index in had
// of the tab it continues, or -1 for a new tab. A tab continues the one
// with the same name, ignoring case, wherever it is. If the lists differ
// only in one name, in the same place, that tab continues the one there:
// it was renamed.
func MatchTabs(had, now []string) []int {
	from := make([]int, len(now))
	taken := make([]bool, len(had))
	for i, name := range now {
		from[i] = -1
		for j, h := range had {
			if !taken[j] && strings.EqualFold(h, name) {
				from[i], taken[j] = j, true
				break
			}
		}
	}
	if len(had) != len(now) {
		return from
	}
	renamed := -1
	for i, j := range from {
		switch {
		case j == i:
		case j == -1 && renamed == -1:
			renamed = i
		default:
			return from
		}
	}
	if renamed >= 0 {
		from[renamed] = renamed
	}
	return from
}

// Dropped returns the indexes of the tabs, of had of them before, that no
// tab continues, by MatchTabs's from.
func Dropped(from []int, had int) []int {
	kept := make([]bool, had)
	for _, j := range from {
		if j >= 0 {
			kept[j] = true
		}
	}
	var out []int
	for j, k := range kept {
		if !k {
			out = append(out, j)
		}
	}
	return out
}

// CheckState reports why a state can't be used, or returns nil: it must
// have version StateVersion, and each tab a valid tree (see
// layout.Validate); no pane or split ID can be in two tabs, and no pane can
// be called layout.CompactPaneID; every item in a tree or floating must be
// in Items, with a known kind (web, app, or builtin); and no item can be in
// a tab twice, or both floating and in a tab. An item can be in several
// tabs, once in each. Its tabs needn't be a config's: see MatchTabs.
func CheckState(s *State) error {
	if s == nil {
		return errors.New("there's no state to check")
	}
	if s.Version != StateVersion {
		return fmt.Errorf("it's version %d, not %d", s.Version, StateVersion)
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
		// A pane that shows nothing would have all its windows parked.
		walkPanes(t.Root, func(p *layout.Node) {
			if p.Active == "" && len(p.Items) > 0 {
				p.Active = p.Items[0]
			}
		})
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
	for kind, size := range s.Dialogs {
		if !(size.W > 0 && size.H > 0) {
			delete(s.Dialogs, kind)
		}
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
