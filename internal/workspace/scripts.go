package workspace

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/control"
)

// This file has what links and scripts ask of the running Bench
// (internal/control, docs/architecture.md, decision 77): FollowLink for
// bench:// links, which can only switch tabs and show Jump to… or every
// tab; RunScript for the bench command's script subcommands, which work
// only while the allow_scripts setting is on; and ScriptList, with the
// socket that answers bench list, which is there only while it's on.

// noticeScripts is the source of the notice that says the socket that
// bench list asks couldn't be made.
const noticeScripts = "scripts"

// The bench:commands that a link's jump and overview send: they open Jump
// to… and every tab, and leave them open if they are already.
const (
	openPaletteCommand = "open-palette"
	showAllTabsCommand = "show-all-tabs"
)

var (
	// errScriptsOff is the refusal of a script while the allow_scripts
	// setting is off.
	errScriptsOff = errors.New("Scripts can't control Bench: turn on Allow scripts to control Bench in Settings ▸ Advanced, then try again.")
	// errDropdownHasKeys refuses bench move-to-tab while the drop-down
	// shows, as it has the keyboard then (decision 72): the window that
	// the user looks at is in no pane, and the focused pane's is out of
	// sight behind it.
	errDropdownHasKeys = errors.New("The drop-down has the keyboard, so there's no shown window to move. Hide the drop-down, then try again.")
)

// At most linkBurst bench:// links in any linkWindow are followed: a link
// that comes when that many have come in the last linkWindow is ignored,
// and only logged, the first in a row of them (linkLimit). Any web page can
// open links, as can a page in another browser that the user left open,
// over and over, and each switches tabs and brings Bench forward
// (docs/architecture.md, decision 77). Scripts, which need the
// allow_scripts setting, aren't limited.
const (
	linkBurst  = 4
	linkWindow = time.Second
)

// linkLimit keeps the times of the latest linkBurst links followed, for
// FollowLink. Its lock is taken without the engine's.
type linkLimit struct {
	mu    sync.Mutex
	times [linkBurst]time.Time
	next  int // Where the next time goes: the oldest's place.
	// ignoring is set from the first link ignored until one is followed,
	// so that a flood logs one line.
	ignoring bool
}

// allow reports whether a link that comes at now is followed, and keeps its
// time if it is. It also reports whether it's the first of a row ignored,
// to log.
func (l *linkLimit) allow(now time.Time) (ok, first bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if oldest := l.times[l.next]; !oldest.IsZero() && now.Sub(oldest) < linkWindow {
		first = !l.ignoring
		l.ignoring = true
		return false, first
	}
	l.times[l.next] = now
	l.next = (l.next + 1) % linkBurst
	l.ignoring = false
	return true, false
}

// scriptSocket is the socket that answers bench list (control.Serve). Its
// lock is taken without the engine's, since closing the socket waits for
// the answers on their way, which take the engine's lock.
type scriptSocket struct {
	mu     sync.Mutex
	server *control.Server
	// closed is set by Engine.Close: the socket doesn't start again after.
	closed bool
}

// FollowLink does what a bench:// link asks (control.ParseLink): it
// switches to a tab, the next or previous one, or the one used before this
// one, as a click on a tab or the shortcuts do, or shows Jump to… or every
// tab, and brings Bench forward. It works whatever the allow_scripts
// setting says: a link can do only what the user's own clicks on tabs
// would, which the config says (docs/architecture.md, decision 77). Any
// web page can open a link, so one that asks for anything else is refused,
// with a toast that names it, and Bench comes forward to show it. It
// returns the refusal, for the log. A link that comes when linkBurst have
// come in the last linkWindow, refused or not, is ignored, without a toast
// or coming forward (linkLimit). A switch is timed as a link's
// (reasonLink, decision 67).
func (e *Engine) FollowLink(link string) error {
	if ok, first := e.links.allow(e.clk.Now()); !ok {
		if first {
			e.log.Warn("ignored links that came too quickly", "link", control.Shown(link), "most", linkBurst, "in", linkWindow)
		}
		return errLinksTooQuick
	}
	req, err := control.ParseLink(link)
	if err != nil {
		e.log.Warn("refused a link", "link", control.Shown(link))
		e.em.BringForward()
		e.showScriptResult("", err.Error())
		return err
	}
	return e.carryOut(req, reasonLink)
}

// errLinksTooQuick is what FollowLink returns for a link that it ignores
// (linkLimit), for the log.
var errLinksTooQuick = errors.New("Bench ignored a link that came too quickly after others.")

// RunScript does what one of the bench command's script subcommands asks,
// given its arguments after the program's name, as another Bench started
// from Terminal forwards them (control.ParseCommand): bench tab, next,
// previous, last, move-to-tab, or launch. It does nothing, and only logs
// why, unless the allow_scripts setting is on: any app of the user's can
// send these, even one that macOS keeps in a sandbox. The tab subcommands
// bring Bench forward on the tab, and launch on what it opened; move-to-tab
// moves the shown window without coming forward, as the send-to-tab
// shortcut does, and is refused while the drop-down shows, as it has the
// keyboard then. A refusal is shown as a toast, as a shortcut's is. A
// switch is timed as a script's (reasonScript, decision 67).
func (e *Engine) RunScript(args []string) error {
	e.lock()
	on := e.cfg.Settings.AllowScripts && !e.closed
	e.unlock()
	if !on {
		e.log.Warn("ignored a script while Allow scripts to control Bench is off", "command", control.Shown(fmt.Sprint(args)))
		return errScriptsOff
	}
	req, err := control.ParseCommand(args)
	if err != nil {
		e.log.Warn("ignored a script that Bench can't read", "command", control.Shown(fmt.Sprint(args)), "error", err)
		return err
	}
	return e.carryOut(req, reasonScript)
}

// carryOut does what a link or a script asks. A switch is timed for
// reason, a link's or a script's, from when the request came, and shows
// the tab as current at once (switchShowing, decision 67). What
// move-to-tab and launch arrange is timed as other, as the shortcut's send
// to a tab and the add dialog are.
func (e *Engine) carryOut(req control.Request, reason string) error {
	at := e.clk.Now()
	e.lock()
	if e.closed {
		e.unlock()
		return nil
	}
	// A link or script takes the place of the tabs that a hold after
	// last_tab's shortcut shows, as Jump to… does.
	e.endHoldLocked(false)
	var err error
	forward := true
	command := ""
	switch req.Action {
	case control.ActionTab, control.ActionNext, control.ActionPrevious, control.ActionLast:
		// The title bar shows the tab as current at once, once the
		// frontend gets events (readied).
		to := -1
		if e.started && e.readied {
			to = e.scriptTargetLocked(req)
		}
		e.unlock()
		err = e.switchShowing(to, at, reason, func() error { return e.scriptSwitchLocked(req) })
		e.em.BringForward()
		e.showScriptResult("", errText(err))
		return err
	case control.ActionJump:
		e.noteJumpLocked()
		command, forward = openPaletteCommand, false
	case control.ActionOverview:
		e.noteJumpLocked()
		command, forward = showAllTabsCommand, false
	case control.ActionMoveToTab:
		forward = false
		err = e.scriptMoveLocked(req.Arg)
	case control.ActionLaunch:
		err = e.launchLocked(req.Arg)
	default:
		err = fmt.Errorf("Bench can't do %s.", control.Shown(req.Action))
	}
	e.unlock()
	// The emitter's commands that open the palette and every tab bring
	// Bench forward themselves.
	if forward {
		e.em.BringForward()
	}
	e.showScriptResult(command, errText(err))
	return err
}

// errText returns an error's message, or "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// scriptSwitchLocked switches tabs for a link's or script's tab, next,
// previous, or last, as a click on a tab, the Tab menu's Next Tab and
// Previous Tab, and Last Used Tab without a hold do.
func (e *Engine) scriptSwitchLocked(req control.Request) error {
	if e.closed {
		return nil
	}
	switch req.Action {
	case control.ActionTab:
		i, err := e.scriptTabLocked(req.Arg)
		if err != nil {
			return err
		}
		e.switchToLocked(i)
	case control.ActionNext, control.ActionPrevious:
		step := 1
		if req.Action == control.ActionPrevious {
			step = -1
		}
		if i := e.stepTargetLocked(step); i >= 0 {
			e.switchToLocked(i)
		}
	case control.ActionLast:
		// Without a hold, whatever keys are down as the script runs.
		i := e.lastTabLocked()
		if i < 0 {
			return errNoLastTab
		}
		e.switchToLocked(i)
	}
	return nil
}

// scriptTargetLocked returns the tab that a link's or script's switch is
// about to switch to, for the title bar to show as current at once, or -1:
// for the current tab, which it arranges again, and for a tab that isn't
// there, which is refused.
func (e *Engine) scriptTargetLocked(req control.Request) int {
	to := -1
	switch req.Action {
	case control.ActionTab:
		to, _ = e.scriptTabLocked(req.Arg)
	case control.ActionNext:
		to = e.stepTargetLocked(1)
	case control.ActionPrevious:
		to = e.stepTargetLocked(-1)
	case control.ActionLast:
		to = e.lastTabLocked()
	}
	if to == e.model.Current {
		return -1
	}
	return to
}

// showScriptResult sends what a link or script shows: the bench:command
// that opens Jump to… or every tab, and a toast, either of which may be "".
// Until the frontend has first asked for the view (Ready), it keeps them,
// the newest of each, for Ready to send: before then, nothing would get
// them, as when a link opens Bench. A switch needs nothing kept: the view
// that the frontend asks for has it.
func (e *Engine) showScriptResult(command, toast string) {
	e.lock()
	if !e.readied {
		if command != "" {
			e.heldCommand = command
		}
		if toast != "" {
			e.heldToast = toast
		}
		e.unlock()
		return
	}
	e.unlock()
	sendScriptResult(e.em, command, toast)
}

// sendScriptResult sends a link's or script's command and toast, either of
// which may be "".
func sendScriptResult(em Emitter, command, toast string) {
	if command != "" {
		em.Command(Command{Name: command})
	}
	if toast != "" {
		em.Toast(toast)
	}
}

// scriptMoveLocked moves the shown window to the tab that arg names, for
// bench move-to-tab, as the send_to_tab_modifiers shortcut does. It's
// refused while the drop-down shows, as it has the keyboard then.
func (e *Engine) scriptMoveLocked(arg string) error {
	if e.dropShownLocked() {
		return errDropdownHasKeys
	}
	i, err := e.scriptTabLocked(arg)
	if err != nil {
		return err
	}
	return e.sendToTabLocked(i)
}

// scriptTabLocked returns the index of the tab that a link's or script's
// argument names (control.FindTab), or why there's none.
func (e *Engine) scriptTabLocked(arg string) (int, error) {
	if i := control.FindTab(e.tabNamesLocked(), arg); i >= 0 {
		return i, nil
	}
	if n, err := strconv.Atoi(arg); err == nil && strconv.Itoa(n) == arg && n > 0 {
		return -1, errNoTab(n - 1)
	}
	return -1, errNoTabNamed(control.Shown(arg))
}

// tabNamesLocked returns the tabs' names, in order.
func (e *Engine) tabNamesLocked() []string {
	names := make([]string, len(e.model.Tabs))
	for i, t := range e.model.Tabs {
		names[i] = t.Name
	}
	return names
}

// launchLocked adds what the launcher called name opens to the current
// tab's focused pane, and shows it, as the add dialog does.
func (e *Engine) launchLocked(name string) error {
	if e.cfg.LauncherIndex(name) < 0 {
		return errNoLauncher(control.Shown(name))
	}
	t := e.model.Tab(e.model.Current)
	if t == nil {
		return errNoTab(e.model.Current)
	}
	p := t.FocusedPane()
	if p == nil {
		return errEmptyPane
	}
	shown, err := e.addLauncherLocked(t, p, name)
	if err != nil {
		return err
	}
	e.finishAddLocked(shown)
	return nil
}

// ScriptList returns what bench list prints: the tabs, their panes, and
// what each holds, with what needs the user. It's refused unless the
// allow_scripts setting is on.
func (e *Engine) ScriptList() (control.List, error) {
	e.lock()
	defer e.unlock()
	if !e.cfg.Settings.AllowScripts || e.closed {
		return control.List{}, errScriptsOff
	}
	return e.scriptListLocked(), nil
}

// scriptListLocked builds ScriptList's List. Items that aren't drawn as
// sub-tabs (hiddenLocked) are left out, as they are from the tabs' counts.
// The totals count an item that's in more than one tab once, as the Dock's
// badge does, while each tab's count has each of its own. The drop-down,
// which is in no tab (decision 72), has a field of its own, and counts in
// the totals.
func (e *Engine) scriptListLocked() control.List {
	m := e.model
	unread := e.unreadLocked()
	counted := map[string]bool{}
	l := control.List{Version: control.ListVersion, Current: m.Current + 1, Tabs: []control.Tab{}}
	for i, t := range m.Tabs {
		tab := control.Tab{Number: i + 1, Name: t.Name, Current: i == m.Current, Colour: t.Colour, Panes: []control.Pane{}}
		// A focus that isn't one of the tab's panes is its first's.
		focused := ""
		if f := t.FocusedPane(); f != nil {
			focused = f.ID
		}
		for _, p := range t.Leaves() {
			pane := control.Pane{ID: p.ID, Focused: p.ID == focused, Items: []control.Item{}}
			for _, id := range p.Items {
				it := m.Items[id]
				if it == nil || e.hiddenLocked(it) {
					continue
				}
				item := e.scriptItemLocked(it, unread[id])
				item.Shown = p.Active == id
				if item.Attention != "" {
					tab.Attention++
				}
				tab.Unread = addUnread(tab.Unread, item.Unread)
				if !counted[id] {
					counted[id] = true
					if item.Attention != "" {
						l.Attention++
					}
					l.Unread = addUnread(l.Unread, item.Unread)
				}
				pane.Items = append(pane.Items, item)
			}
			tab.Panes = append(tab.Panes, pane)
		}
		l.Tabs = append(l.Tabs, tab)
	}
	if d := e.dropdownLocked(); d != nil {
		item := e.scriptItemLocked(d, unread[d.ID])
		item.Shown = e.dropShown
		if item.Attention != "" {
			l.Attention++
		}
		l.Unread = addUnread(l.Unread, item.Unread)
		l.Dropdown = &item
	}
	return l
}

// scriptItemLocked returns an item as ScriptList's List has it, with its
// unread count u, but not whether it's shown, which its pane says. A web
// page says whether it keeps running in the background, and whether Bench
// hasn't loaded it (notLoadedLocked, decision 76).
func (e *Engine) scriptItemLocked(it *Item, u UnreadView) control.Item {
	item := control.Item{
		ID: it.ID, Kind: scriptKind(it), App: it.App, Title: e.shownTitleLocked(it), Attention: it.attention,
		Unread: control.Unread{Count: u.Count, More: u.More, Dot: u.Dot},
	}
	if keepable(it) {
		item.KeepRunning, item.NotLoaded = it.KeepRunning, e.notLoadedLocked(it)
	}
	return item
}

// addUnread returns the total of two unread counts, as UnreadView.add
// does.
func addUnread(a, b control.Unread) control.Unread {
	t := UnreadView{Count: a.Count, More: a.More, Dot: a.Dot}.add(UnreadView{Count: b.Count, More: b.More, Dot: b.Dot})
	return control.Unread{Count: t.Count, More: t.More, Dot: t.Dot}
}

// scriptKind returns an item's kind as bench list says it.
func scriptKind(it *Item) string {
	switch {
	case it.Kind == KindBuiltin:
		return control.KindTerminal
	case it.Kind == KindWeb && it.VSCode != "":
		return control.KindVSCode
	case it.Kind == KindWeb:
		return control.KindWeb
	}
	return control.KindApp
}

// socketRetryWait is how long Bench waits before it tries again to make
// the socket that bench list asks, after it couldn't: once the user has
// moved what the notice says is in the way, the socket comes by itself.
const socketRetryWait = 5 * time.Second

// scriptSocketChangedLocked records whether the socket that answers bench
// list should be there, and reports whether that has changed since it was
// last asked: it's there once Bench has started, while the allow_scripts
// setting is on, but not for bench arrange (Options.Headless).
func (e *Engine) scriptSocketChangedLocked() (changed bool) {
	want := e.started && !e.closed && !e.opts.Headless && e.cfg.Settings.AllowScripts
	changed = want != e.socketWanted
	e.socketWanted = want
	return changed
}

// syncScriptSocket starts or stops the socket that answers bench list, as
// flushState last found that it should be, without the engine's lock. A
// socket that can't be made is logged, and a notice says why, while Bench
// tries again every socketRetryWait, until the setting goes off or the
// socket is made. It reads the wanted state under the socket's lock, so
// that whichever call comes last leaves the socket as it should be.
func (e *Engine) syncScriptSocket() {
	s := &e.sock
	s.mu.Lock()
	defer s.mu.Unlock()
	e.lock()
	want := e.socketWanted && !e.closed
	e.unlock()
	if !want || s.closed {
		if s.server != nil {
			if err := s.server.Close(); err != nil {
				e.log.Warn("couldn't remove Bench's socket", "error", err)
			}
			s.server = nil
		}
		e.setScriptNotice("", false)
		return
	}
	if s.server != nil {
		return
	}
	err := config.CheckDir(e.dir)
	if err == nil {
		s.server, err = control.Serve(e.dir, e.ScriptList)
	}
	if err != nil {
		if e.setScriptNotice("Scripts can't control Bench: "+err.Error()+".", true) {
			e.log.Warn("couldn't make the socket that bench list asks", "error", err)
		}
		return
	}
	e.setScriptNotice("", false)
}

// setScriptNotice shows the notice that says why the socket couldn't be
// made, or takes it away for "", and reports whether it changed. With
// retry, it has syncScriptSocket try again after socketRetryWait, unless
// it will already; without, it stops trying.
func (e *Engine) setScriptNotice(text string, retry bool) bool {
	e.lock()
	defer e.unlock()
	switch {
	case retry && e.socketRetry == nil && !e.closed:
		e.socketRetry = e.clk.AfterFunc(socketRetryWait, func() {
			e.lock()
			e.socketRetry = nil
			e.unlock()
			e.syncScriptSocket()
		})
	case !retry && e.socketRetry != nil:
		e.socketRetry.Stop()
		e.socketRetry = nil
	}
	cur := e.notices[noticeScripts]
	if (text == "" && len(cur) == 0) || (len(cur) == 1 && cur[0].Text == text) {
		return false
	}
	var notices []string
	if text != "" {
		notices = []string{text}
	}
	e.setNoticesLocked(noticeScripts, notices)
	e.touch()
	return true
}

// closeScriptSocket stops the socket for good, as Bench quits.
func (e *Engine) closeScriptSocket() {
	s := &e.sock
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			e.log.Warn("couldn't remove Bench's socket", "error", err)
		}
		s.server = nil
	}
}

// scriptSocketPath returns the socket's path while it's there, or "", for
// tests.
func (e *Engine) scriptSocketPath() string {
	s := &e.sock
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return ""
	}
	return s.server.Path()
}
