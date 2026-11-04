package workspace

import (
	"slices"
	"strconv"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Model is Bench's arrangement: tabs with their trees, the items in them, and
// the items popped out of Bench. An item is in one pane of one tab, or in one
// pane of each of several tabs (see AddToTab), or floating. It's pure state:
// its methods never call the platform, so they can be tested on their own.
// It isn't safe for concurrent use; the engine guards it with its lock.
type Model struct {
	Tabs []*Tab
	// Items holds every item in a tree or in Floating, by ID.
	Items map[string]*Item
	// Floating holds the items popped out of Bench, in the order they left.
	Floating []string
	Current  int
	Mode     layout.Mode
	// Seq is the last number used in an item, pane, or split ID.
	Seq int

	// created counts windows bound to items, for matching windows to items
	// by creation order after an app restarts.
	created int
	// used counts uses (an item shown or focused), for finding an app's most
	// recently used window.
	used int
	// windows maps each bound window to its item.
	windows map[platform.WindowID]string
	// hidden reports whether an item is out of sight (see
	// Engine.hiddenLocked), for choosing what a pane shows when its shown
	// item leaves it. It's nil for a model that hides nothing.
	hidden func(*Item) bool
}

// Tab is a named workspace with a layout tree.
type Tab struct {
	Name string
	// Preset is a preset ID, or layout.PresetCustom once items have moved.
	Preset string
	// FromPreset is the preset that a custom tab had before it became
	// custom, or "": Next Layout and Previous Layout go on from it while
	// the tree still has its shape (see layoutOf).
	FromPreset   string
	SmallScreens layout.SmallScreens
	Root         *layout.Node
	// Focus is the focused pane's ID.
	Focus string
	// CompactShown is the item the "tabs" small-screen view shows.
	CompactShown string
}

// Item is something a pane can show: an App item (another app's window), a
// Web item (a web view Bench owns), or a Built-in item (a terminal).
type Item struct {
	ID   string
	Kind string // KindWeb, KindApp, or KindBuiltin.
	// App is the app's name, such as "Zed", "Slack", or "Terminal".
	App      string
	BundleID string
	Mono     string
	Title    string

	// WindowID is an App item's window, or 0 for a placeholder: an item
	// whose app isn't running or whose window has gone.
	WindowID platform.WindowID
	// Created orders App items by when they were bound to a window. It's
	// 0 for an item that has never had one.
	Created int
	// TitleFilter and OpenURL are an App item's title and url from the
	// config.
	TitleFilter string
	OpenURL     string

	// Name is the config's label for a Web or Built-in item.
	Name string
	// Label is the user's own name for a Web or Built-in item (Rename…),
	// which its sub-tab shows instead of Title, the title its page or shell
	// gives it, until the user clears it. state.json keeps it.
	Label string
	// URL is a Web item's address now; Home is the address it was given.
	URL     string
	Home    string
	Profile string
	// Zoom is a Web item's page zoom, one of zoomLevels, or 0 for actual
	// size (ZoomPage). state.json keeps it.
	Zoom float64
	// VSCode is the folder of a VS Code for the Web item.
	VSCode string

	// Folder and Command are a terminal's. Run is typed into a terminal's
	// shell as it starts (see term.Options.Input), with Return if
	// StartAgain is set, or for its first session if runNow is: a launcher's
	// command, or one from the add dialog's New terminal… form. Without
	// StartAgain, a session started after Bench opens again only types it,
	// for the user to run.
	Folder     string
	Command    string
	Run        string
	StartAgain bool
	runNow     bool
	// LastFolder is the folder a terminal's shell was last in, as it
	// reported it, with ~ for the home folder, or "" while it's in the
	// folder it started in. A plain shell, one with neither Command nor
	// Run, starts there again after Bench opens again, so state.json keeps
	// it for one (see plainShell). Folder stays as it is: it's what
	// matches the item to its config entry.
	LastFolder string
	// savedOutput is set for a terminal that state.json brought back, until
	// its session first starts: it starts with the output that Bench kept
	// from when it last quit, if there is any (see output.go). A session
	// started again, as a command's is, carries on after it.
	savedOutput bool

	// Status and StatusText are the last status worked out for a bound
	// App item, or for a Web or Built-in item.
	Status     string
	StatusText string
	// Undocked is set when the user dragged the item's window out of its
	// pane.
	Undocked bool
	// minimised is set for an App item in a tab whose window the user
	// minimised, until they show it again (see minimise.go). Arranging
	// leaves the window in the Dock meanwhile. It isn't saved: the startup
	// scan sets it again for a window that's still minimised.
	minimised bool
	// restore is set when the user asks to see an App item's window, as by
	// clicking its sub-tab: the next arrangement that docks it brings it
	// back from the Dock if it's minimised (see showAgainLocked).
	restore bool
	// OwnWindow is set for a Web or Built-in item that has a window of its
	// own, after popping out.
	OwnWindow bool
	// Used is the use counter's value when the item was last shown or
	// focused.
	Used int
	// pid is the process of an App item's window, as last seen.
	pid int
	// gone is set for an App item whose window Bench couldn't find while
	// its app was running, until Bench has checked again (the app may be
	// quitting, closing its windows as it goes).
	gone bool
	// closing is set for an App item whose window Bench has asked, or is
	// about to ask, to close (see close.go), until it has closed or Bench
	// has seen that it's still open.
	closing bool
	// ended is set for a terminal whose command has finished, until it
	// runs again, and failed with it if the command ended with an error.
	ended  bool
	failed bool
	// attention says why an item in a tab needs the user, for the mark on
	// its sub-tab and tab, until it's shown: its program rang the bell, or
	// it or its page posted a notification, or its command finished, while
	// the user couldn't see it (see attention.go). It isn't saved.
	// notified is when Bench last posted a notification for a terminal.
	attention string
	notified  time.Time
	// page, host, site, offSite, and notSecure describe the page a Web
	// item shows, as its web view last reported it (see platform.Page): its
	// address, its host and the host's site, whether it's on another site
	// than the item's own address (as a page with no web address, and no
	// host, is), and whether it isn't secure. They aren't saved: until the
	// view reports a page, after a restart too, the item has no page, and
	// no host. URL, which is saved, can then still be the last page from
	// before, while the view loads the item's own address.
	page      string
	host      string
	site      string
	offSite   bool
	notSecure bool
	// web is what a Web item's page can do and is doing, as its web view
	// last reported it (WebStateChanged): whether it can go back or
	// forward, and how it uses the camera and microphone. finding is set
	// while its find bar is open (SetFindBar). Neither is saved.
	web     platform.WebState
	finding bool
	// titled is set once a Web item's web view has reported its page's
	// title (WebTitle), until the view closes. Until then, Title is the
	// one from the last time Bench ran, whose unread count can be out of
	// date (see unreadLocked). It isn't saved.
	titled bool
	// pageIcon numbers a Web item's page icon, as its web view last
	// reported it (PageIconChanged), for the icon's address: 0 while it
	// has none. It isn't saved.
	pageIcon int
}

// newModel returns an empty model.
func newModel() *Model {
	return &Model{
		Items:   map[string]*Item{},
		Mode:    layout.ModeAuto,
		windows: map[platform.WindowID]string{},
	}
}

// Prefixes of generated IDs.
const (
	itemPrefix  = "w"
	panePrefix  = "p"
	splitPrefix = "s"
)

// NewID returns a fresh ID with the given prefix ("w", "p", or "s"). Items,
// panes, and splits share one sequence.
func (m *Model) NewID(prefix string) string {
	m.Seq++
	return prefix + strconv.Itoa(m.Seq)
}

// nodeID is a layout.NewID for the model's sequence.
func (m *Model) nodeID(nodeType string) string {
	if nodeType == layout.TypeSplit {
		return m.NewID(splitPrefix)
	}
	return m.NewID(panePrefix)
}

// Tab returns the tab at index i, or nil.
func (m *Model) Tab(i int) *Tab {
	if i < 0 || i >= len(m.Tabs) {
		return nil
	}
	return m.Tabs[i]
}

// CurrentTab returns the current tab.
func (m *Model) CurrentTab() *Tab { return m.Tab(m.Current) }

// IndexOf returns a tab's index, or -1.
func (m *Model) IndexOf(t *Tab) int { return slices.Index(m.Tabs, t) }

// Where returns the index of the tab holding an item and its pane, or -1
// and nil for an item that's floating or unknown. An item can be in more
// than one tab (see AddToTab): Where then returns its place in the current
// tab if it's there, or else in the first tab holding it.
func (m *Model) Where(id string) (int, *layout.Node) {
	if p := m.PaneIn(m.Current, id); p != nil {
		return m.Current, p
	}
	for i, t := range m.Tabs {
		if p := layout.PaneOf(t.Root, id); p != nil {
			return i, p
		}
	}
	return -1, nil
}

// PaneIn returns the pane of tab ti holding an item, or nil.
func (m *Model) PaneIn(ti int, id string) *layout.Node {
	t := m.Tab(ti)
	if t == nil {
		return nil
	}
	return layout.PaneOf(t.Root, id)
}

// TabsOf returns the indexes of the tabs holding an item, in order: more
// than one for an item shown in several tabs, and none for one that's
// floating or unknown.
func (m *Model) TabsOf(id string) []int {
	var out []int
	for i, t := range m.Tabs {
		if layout.PaneOf(t.Root, id) != nil {
			out = append(out, i)
		}
	}
	return out
}

// IsFloating reports whether an item has been popped out of Bench.
func (m *Model) IsFloating(id string) bool { return slices.Contains(m.Floating, id) }

// Pane returns the pane with the given ID, or nil.
func (t *Tab) Pane(id string) *layout.Node {
	n, _, _ := layout.Find(t.Root, id)
	if !n.IsPane() {
		return nil
	}
	return n
}

// Leaves returns the tab's panes in reading order.
func (t *Tab) Leaves() []*layout.Node { return layout.Leaves(t.Root) }

// ItemIDs returns the tab's items in reading order.
func (t *Tab) ItemIDs() []string { return layout.Items(t.Root) }

// FocusedPane returns the focused pane, or the first pane if the focus
// isn't one of the tab's panes.
func (t *Tab) FocusedPane() *layout.Node {
	if p := t.Pane(t.Focus); p != nil {
		return p
	}
	leaves := t.Leaves()
	if len(leaves) == 0 {
		return nil
	}
	return leaves[0]
}

// fixFocus points Focus at a pane that exists.
func (t *Tab) fixFocus() {
	if p := t.FocusedPane(); p != nil {
		t.Focus = p.ID
	}
}

// markCustom records that the tab's tree no longer follows its preset, and
// remembers the preset (FromPreset).
func (t *Tab) markCustom() {
	if _, ok := layout.LookupPreset(t.Preset); ok {
		t.FromPreset = t.Preset
	}
	t.Preset = layout.PresetCustom
}

// Take removes an item from its pane in every tab, or from Floating,
// keeping it in Items. A pane left empty is removed and its parent closes
// up, unless it's the last pane in its tab. It returns the index of the
// tab the item was in, as Where gives it (-1 if it was floating or
// unknown), and whether a pane was removed, in which case that pane's tab
// is marked custom.
func (m *Model) Take(id string) (tab int, removedPane bool) {
	if i := slices.Index(m.Floating, id); i >= 0 {
		m.Floating = slices.Delete(m.Floating, i, i+1)
		return -1, false
	}
	tab, _ = m.Where(id)
	for _, ti := range m.TabsOf(id) {
		if m.takeFrom(id, ti) {
			removedPane = true
		}
	}
	return tab, removedPane
}

// takeFrom removes an item from its pane in tab ti, leaving it in any other
// tab, as Take does. It reports whether a pane was removed.
func (m *Model) takeFrom(id string, ti int) bool {
	t := m.Tab(ti)
	if t == nil || layout.PaneOf(t.Root, id) == nil {
		return false
	}
	before := len(t.Leaves())
	keep := m.keepShown(t, id)
	t.Root, _, _ = layout.TakeItem(t.Root, id)
	removed := len(t.Leaves()) < before
	if removed {
		t.markCustom()
	}
	keep()
	t.fixFocus()
	return removed
}

// keepShown is for taking item id out of its pane in tab t: it returns a
// function to call afterwards, which chooses what the pane, and the "tabs"
// view, show in its place, as if hidden items weren't there. For a hidden
// item, which they didn't show, that's what they showed instead: the most
// recently used of the others, so nothing changes on screen. For an item
// they showed, the pane shows its nearest neighbour that isn't hidden, and
// the "tabs" view whatever it shows by default. A hidden item never comes
// to show by itself when its window comes back.
func (m *Model) keepShown(t *Tab, id string) func() {
	p := layout.PaneOf(t.Root, id)
	if p == nil {
		return func() {}
	}
	paneID, active := p.ID, p.Active == id
	at := slices.Index(p.Items, id)
	hid := m.isHidden(id)
	compact := t.CompactShown == id
	instead := ""
	if compact && hid {
		instead = m.recentShown(t.ItemIDs(), id)
	}
	return func() {
		if compact {
			t.CompactShown = instead
		}
		q := t.Pane(paneID)
		if !active || q == nil {
			return
		}
		next := ""
		if hid {
			next = m.recentShown(q.Items, "")
		} else {
			next = m.nearestShown(q.Items, at)
		}
		if next != "" {
			q.Active = next
		}
	}
}

// isHidden reports whether an item is out of sight (see hidden).
func (m *Model) isHidden(id string) bool {
	return m.hidden != nil && m.hidden(m.Items[id])
}

// recentShown returns the most recently used item of a list that isn't
// hidden, other than skip, or "".
func (m *Model) recentShown(ids []string, skip string) string {
	best, used := "", -1
	for _, id := range ids {
		if it := m.Items[id]; it != nil && id != skip && !m.isHidden(id) && it.Used > used {
			best, used = id, it.Used
		}
	}
	return best
}

// nearestShown returns the item of a list that isn't hidden nearest to
// index i, looking from i on first, as a pane shows the next item when its
// shown one leaves, or "".
func (m *Model) nearestShown(ids []string, i int) string {
	for k := max(i, 0); k < len(ids); k++ {
		if !m.isHidden(ids[k]) {
			return ids[k]
		}
	}
	for k := min(i, len(ids)) - 1; k >= 0; k-- {
		if !m.isHidden(ids[k]) {
			return ids[k]
		}
	}
	return ""
}

// leave takes an item out of one place: its pane in tab ti, or Floating
// for ti -1.
func (m *Model) leave(id string, ti int) {
	if ti < 0 {
		m.Take(id)
		return
	}
	m.takeFrom(id, ti)
}

// Float puts an item that's in no tab, and that the model has forgotten,
// back in Floating, with its window.
func (m *Model) Float(it *Item) {
	m.Items[it.ID] = it
	if !m.IsFloating(it.ID) {
		m.Floating = append(m.Floating, it.ID)
	}
	if it.WindowID != 0 {
		m.windows[it.WindowID] = it.ID
	}
}

// Remove takes an item out (see Take) and forgets it.
func (m *Model) Remove(id string) (tab int, removedPane bool) {
	tab, removedPane = m.Take(id)
	if it := m.Items[id]; it != nil && it.WindowID != 0 {
		delete(m.windows, it.WindowID)
	}
	delete(m.Items, id)
	return tab, removedPane
}

// Replace puts a new item in place of one in every tab holding it: in the
// same pane, at the same place among its items, and as the pane's active
// item and the "tabs" view's item where the old one was. The old item is
// forgotten. It reports false, changing nothing, for an item that's in no
// tab.
func (m *Model) Replace(oldID string, it *Item) bool {
	tabs := m.TabsOf(oldID)
	if len(tabs) == 0 {
		return false
	}
	for _, ti := range tabs {
		t := m.Tabs[ti]
		p := layout.PaneOf(t.Root, oldID)
		p.Items[slices.Index(p.Items, oldID)] = it.ID
		if p.Active == oldID {
			p.Active = it.ID
		}
		if t.CompactShown == oldID {
			t.CompactShown = it.ID
		}
	}
	if old := m.Items[oldID]; old != nil {
		it.Used = old.Used
		if old.WindowID != 0 {
			delete(m.windows, old.WindowID)
		}
	}
	delete(m.Items, oldID)
	m.Items[it.ID] = it
	if it.WindowID != 0 {
		m.windows[it.WindowID] = it.ID
	}
	return true
}

// insertAt puts an item into a pane at index at, or at the end if at is out
// of range, without showing it.
func insertAt(p *layout.Node, id string, at int) {
	if at < 0 || at > len(p.Items) {
		at = len(p.Items)
	}
	p.Items = slices.Insert(p.Items, at, id)
	if p.Active == "" {
		p.Active = id
	}
}

// Add puts a new item into a pane of a tab at index at (-1 for the end).
func (m *Model) Add(it *Item, t *Tab, p *layout.Node, at int) {
	m.Items[it.ID] = it
	if it.WindowID != 0 {
		m.windows[it.WindowID] = it.ID
	}
	insertAt(p, it.ID, at)
	if t.Focus == "" {
		t.Focus = p.ID
	}
}

// Show makes an item its pane's active item, focuses the pane, makes it
// the item the "tabs" view shows, and marks it used, in the tab Where
// gives. It reports false for an item that isn't in a tab.
func (m *Model) Show(id string) bool {
	ti, _ := m.Where(id)
	return m.ShowIn(id, ti)
}

// ShowIn is Show for an item's place in tab ti. It reports false if the
// item isn't in that tab.
func (m *Model) ShowIn(id string, ti int) bool {
	p := m.PaneIn(ti, id)
	if p == nil {
		return false
	}
	t := m.Tabs[ti]
	p.Active = id
	t.Focus = p.ID
	t.CompactShown = id
	m.Use(id)
	return true
}

// ShowInPane makes an item the active item of its pane in tab ti, without
// focusing the pane, and marks it used.
func (m *Model) ShowInPane(id string, ti int) {
	p := m.PaneIn(ti, id)
	if p == nil {
		return
	}
	p.Active = id
	if t := m.Tabs[ti]; t.Focus == p.ID {
		t.CompactShown = id
	}
	m.Use(id)
}

// Use marks an item as the most recently used.
func (m *Model) Use(id string) {
	if it := m.Items[id]; it != nil {
		m.used++
		it.Used = m.used
	}
}

// FocusPane focuses a pane of a tab. The "tabs" view then shows the pane's
// active item.
func (m *Model) FocusPane(t *Tab, p *layout.Node) {
	t.Focus = p.ID
	if p.Active != "" {
		t.CompactShown = p.Active
		m.Use(p.Active)
	}
}

// ItemByWindow returns the item bound to a window, or nil.
func (m *Model) ItemByWindow(id platform.WindowID) *Item {
	if id == 0 {
		return nil
	}
	return m.Items[m.windows[id]]
}

// Bind binds an App item to a window, as the next window created.
func (m *Model) Bind(it *Item, w platform.Window) {
	if it.WindowID != 0 {
		delete(m.windows, it.WindowID)
	}
	m.created++
	it.WindowID = w.ID
	it.Created = m.created
	it.Title = w.Title
	it.pid = w.PID
	it.Undocked = false
	it.minimised, it.restore = false, false
	it.gone = false
	it.Status, it.StatusText = StatusReady, ""
	m.windows[w.ID] = it.ID
}

// Unbind makes an App item a placeholder again, keeping its title and
// creation order so that its app's windows can match it when the app opens
// again.
func (m *Model) Unbind(it *Item) {
	if it.WindowID != 0 {
		delete(m.windows, it.WindowID)
	}
	it.WindowID = 0
	it.pid = 0
	it.Undocked = false
	it.minimised, it.restore = false, false
	it.gone = false
	it.Status, it.StatusText = "", ""
}

// reindex rebuilds the window index and the counters from the items. If two
// items name the same window, as in a state.json edited by hand, the one
// first in tab and reading order keeps it and the other becomes a
// placeholder.
func (m *Model) reindex() {
	m.windows = map[platform.WindowID]string{}
	for _, it := range m.orderedItems() {
		m.created = max(m.created, it.Created)
		if it.WindowID == 0 {
			continue
		}
		if _, taken := m.windows[it.WindowID]; taken {
			it.WindowID = 0
			continue
		}
		m.windows[it.WindowID] = it.ID
	}
}
