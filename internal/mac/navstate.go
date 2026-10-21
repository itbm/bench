package mac

import (
	"sync"
	"time"
)

// afterClickWindow is how long after a page that a click brought into view
// comes into view a navigation it starts to another site counts as clicked
// too (see navigationFrom). Client-side redirectors, which send the user on
// with a meta refresh or location.replace as soon as their page loads, as
// link trackers and Google's /url do, do so within a second or two, even on
// a slow connection. A page that sends the user elsewhere later is more
// likely doing it for itself, such as to sign them in again, which mustn't
// go to the browser.
const afterClickWindow = 5 * time.Second

// maxHomeReloads is how many times, while a Web item goes back to its own
// address after a sign-in page sent the user on (see navigator), Bench loads
// the address again when the page in view replaces that load with one of
// its own.
const maxHomeReloads = 3

// navKind is the kind of a navigation of a Web item's main frame, from
// WebKit's WKNavigationType. The values match bridge_darwin.h's
// BENCH_NAV_KIND_ constants.
type navKind int

const (
	// navOther is any navigation but those below: one a page's script or
	// form starts, or Bench's own load.
	navOther navKind = iota
	// navLink is an activated link, which the user or a script clicked (see
	// UserClicked).
	navLink
	// navHistory goes back or forward, or reloads, a form's page too.
	navHistory
)

// navSource is what started a navigation of a Web item's main frame, from
// the navigation's source frame. The values match bridge_darwin.h's
// BENCH_NAV_FROM_ constants.
type navSource int

const (
	// fromBench is Bench's own load of the item's address, which the web
	// view was asked for and which hasn't started.
	fromBench navSource = iota
	// fromNone has no source frame: the web view itself started it.
	fromNone
	// fromPage is the main frame of the item's own web view: its page.
	fromPage
	// fromFrame is a frame inside the page, such as embedded content.
	fromFrame
	// fromWindow is a frame in another web view, such as a popup's page,
	// which can navigate its opener.
	fromWindow
)

// frameGroup groups sources by the frame they're in: the item's main frame,
// a frame inside it, or another window.
func frameGroup(s navSource) navSource {
	if s == fromBench || s == fromNone {
		return fromPage
	}
	return s
}

// mainAction is a navigation of a Web item's main frame that its web view
// asks about.
type mainAction struct {
	target string
	kind   navKind
	from   navSource
	// navigation is the navigation that WebKit asks about it for, by the
	// ID the native side gives it: a new one, or for a redirect, the one
	// under way. It's 0 when WebKit doesn't say.
	navigation uint64
	// sourceOrigin is the security origin of the frame that started it, such
	// as https://app.slack.com, or "" for none or an opaque one.
	sourceOrigin string
	// userEvent is when the user last released a mouse button or pressed a
	// key in the item's web view, or 0, and now is the time now, both in
	// seconds on the native side's clock.
	userEvent float64
	now       float64
}

// navRecord is what the navigator decided about a navigation: how it
// started, and the user event that allows it to open the browser or another
// app (see webOpenLimit), or 0 for none.
type navRecord struct {
	valid bool
	kind  navKind
	from  navSource
	navigationFrom
	event float64
}

// navState is a Web item's navigation state.
type navState struct {
	// signInByClick is whether the page in view is one the user clicked
	// through to on another site (see SignInByClick), and signInInView what
	// it was when that page came into view.
	signInByClick bool
	signInInView  bool
	// opened is whether such a page has sent the user to the browser
	// without a click, which it may do once.
	opened bool
	// returning is whether the item is going back to its own address after
	// that, until that address's page or one the user clicked is in view,
	// and homeReloads how many times Bench has loaded it again since it
	// started going back.
	returning   bool
	homeReloads int
	// homeID is the navigation that loads the item's own address, or 0.
	homeID uint64
	// pending is the last navigation kept in view that hasn't started, and
	// provisional the one under way (started, and neither in view nor
	// failed), with its ID, or 0.
	pending       navRecord
	provisionalID uint64
	provisional   navRecord
	// clickAt is when the page in view came into view, if a navigation that
	// counted as clicked brought it, or 0, and clickEvent the user event
	// that allowed that navigation.
	clickAt    float64
	clickEvent float64
	// page is the address of the page in view, or "".
	page string
}

// navigator follows each Web item's main-frame navigations, and decides
// them, for what DecideNavigation can't tell from one navigation alone. The
// native side tells it about each navigation as WebKit does: when it's
// decided, when it starts, and when its page comes into view or it fails,
// by an ID the native side gives each WKNavigation (0 for none).
//
//   - Clicked. A navigation counts as clicked (see navigationFrom) when the
//     user clicked it, when the item didn't start it, or when a page on the
//     item's site that a click brought into view starts it within
//     afterClickWindow. Its redirects keep what it was, however long its
//     server takes: WebKit asks about each redirect as it asked about the
//     navigation, with its kind and source frame, so a navigation that
//     looks like that while one is under way is taken for one of its
//     redirects.
//   - Clicked through. A page on another site that a navigation that
//     counts as clicked keeps in view (a sign-in page), and a page on
//     another site that going back, forward, or reloading brings back, are
//     pages the user clicked through to (see SignInByClick).
//   - Going back. When such a page sends the user on without a click to a
//     page that's neither the item's site nor a sign-in page, that page
//     opens in the default browser, once (later ones are cancelled), and
//     the item goes back to its own address: the native side loads it.
//     Until that load's page is in view, every navigation but that load
//     and its redirects is cancelled, unless the user clicks it, so that
//     the page in view can't go anywhere else meanwhile. A page in view
//     that replaces the load with a navigation of its own has the item load
//     its address again, up to maxHomeReloads times. If the load fails by
//     itself, the page in view stays, and still goes only where the user
//     clicks. Where WebKit says which navigation a redirect is for, the
//     load's redirects are the item's own, wherever they go, as they are
//     when nothing was clicked through to; otherwise a redirect to another
//     site that's not a sign-in page is cancelled, since it can't be told
//     from a navigation that the page in view starts.
//   - Opens. A navigation that would open the browser or another app,
//     because it counts as clicked, needs a user event to allow it, and
//     each user event allows one (see webOpenLimit): the click itself, the
//     click that a redirect or a redirector's page follows, or for a
//     navigation that something other than the item started, a mouse-up or
//     key-down in the web view within gestureWindow.
//   - Prompts. Each page that comes into view tells the prompt gate, if
//     there is one, whether Bench loaded it, whether the user clicked it,
//     and whether it's on another site than the page before (see
//     promptGate.committed).
//
// Its methods are safe for concurrent use.
type navigator struct {
	mu      sync.Mutex
	items   map[string]*navState
	opens   *gestureLimiter
	prompts *promptGate
}

// webNavigator is the process's navigator.
var webNavigator = navigator{opens: &webOpenLimit, prompts: &webPrompts}

// state returns an item's state, made afresh if it has none. n.mu is held.
func (n *navigator) state(itemID string) *navState {
	if n.items == nil {
		n.items = map[string]*navState{}
	}
	s := n.items[itemID]
	if s == nil {
		s = &navState{}
		n.items[itemID] = s
	}
	return s
}

// forget drops an item's state, and its prompts', when its web view is
// made or closed.
func (n *navigator) forget(itemID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.items, itemID)
	if n.prompts != nil {
		n.prompts.forget(itemID)
	}
}

// loadRequested notes that Bench has asked the item's web view to load the
// item's own address, as navigation nav.
func (n *navigator) loadRequested(itemID string, nav uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.state(itemID).homeID = nav
}

// decide says what the item's web view does with a navigation of its main
// frame (see DecideNavigation), whose own address is itemURL, and whether
// the native side then loads the item's address again.
func (n *navigator) decide(itemID, itemURL string, a mainAction) (Navigation, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.state(itemID)
	r, redirect := s.redirectOf(a)
	// Bench's load of the item's address, or one of its redirects, as
	// WebKit says, is the item's own, whatever page is in view.
	ownLoad := a.navigation != 0 && a.navigation == s.homeID
	switch {
	case ownLoad:
		r = navRecord{valid: true, kind: a.kind, from: fromBench, navigationFrom: navigationFrom{own: true}}
	case !redirect:
		r = s.classify(itemURL, a)
	}
	home := ownLoad || a.from == fromBench || (redirect && s.homeID != 0 && s.provisionalID == s.homeID) ||
		(frameGroup(a.from) == fromPage && sameAddress(itemURL, a.target))
	if s.returning && !r.clicked && !home {
		return NavigateCancel, false
	}
	decision := DecideNavigation(itemURL, a.target, r.navigationFrom, s.signInByClick && !ownLoad)
	switch {
	case decision == NavigateInView:
		before := s.signInByClick
		s.signInByClick = SignInByClick(itemURL, a.target, r.asClick(), false, s.signInByClick)
		if a.kind == navHistory && !onItemSite(itemURL, a.target) {
			s.signInByClick = true
		}
		if s.signInByClick && (!before || r.clicked) {
			s.opened = false // A page clicked through to anew may send the user on once.
		}
		s.pending = r
	case decision == NavigateInBrowser && !r.clicked && !r.afterClick && frameGroup(r.from) == fromPage:
		// A page the user clicked through to sends them on by itself.
		returnHome := false
		if s.opened {
			decision = NavigateCancel
		}
		s.opened = true
		if !s.returning {
			s.returning, s.homeReloads, returnHome = true, 0, true
		}
		return decision, returnHome
	default:
		decision = n.opens.navigation(itemID, decision, r.event, a.now)
	}
	return decision, false
}

// redirectOf returns the record of the navigation under way, if a may be
// one of its redirects: WebKit asks about a redirect with the kind and
// source frame of the navigation it belongs to, and never says it's one. A
// new navigation of the same kind from the same frame looks the same; it's
// from the same page, which most often decides as its redirects would.
// Bench's own load is never a redirect.
func (s *navState) redirectOf(a mainAction) (navRecord, bool) {
	p := s.provisional
	if s.provisionalID == 0 || !p.valid || a.from == fromBench || p.kind != a.kind || frameGroup(p.from) != frameGroup(a.from) {
		return navRecord{}, false
	}
	return p, true
}

// classify works out how a new navigation started (see navigationFrom), and
// the user event that allows it to open the browser or another app.
func (s *navState) classify(itemURL string, a mainAction) navRecord {
	since := -1.0
	if a.userEvent > 0 {
		since = a.now - a.userEvent
	}
	r := navRecord{valid: true, kind: a.kind, from: a.from}
	r.clicked = UserClicked(a.kind == navLink, since)
	switch {
	case a.kind == navHistory, a.from == fromBench, a.from == fromNone:
		r.own = true
	case a.from == fromPage:
		// A page on another site that the item went to by itself, such as
		// its identity provider's, which may send the user on to their
		// organisation's own sign-in server, is the item's; one the user
		// clicked through to isn't.
		onSite := onItemSite(itemURL, a.sourceOrigin)
		r.own = onSite || !s.signInByClick
		r.afterClick = !r.clicked && onSite && s.clickAt > 0 && within(a.now-s.clickAt, afterClickWindow)
	}
	switch {
	case r.clicked:
		r.event = a.userEvent
	case r.afterClick:
		r.event = s.clickEvent
	case !r.own && since >= 0 && within(since, gestureWindow):
		r.event = a.userEvent
	}
	return r
}

// within reports whether seconds is from 0 to d.
func within(seconds float64, d time.Duration) bool {
	return seconds >= 0 && time.Duration(seconds*float64(time.Second)) <= d
}

// started notes that navigation nav has started: the last one kept in view.
func (n *navigator) started(itemID string, nav uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.state(itemID)
	if nav != 0 && nav == s.provisionalID {
		return // Started again, such as in another process after a redirect.
	}
	s.provisionalID, s.provisional, s.pending = nav, s.pending, navRecord{}
}

// committed notes that navigation nav's page, at address, has come into
// view, at now.
func (n *navigator) committed(itemID, itemURL string, nav uint64, address string, now float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.state(itemID)
	var r navRecord
	if nav != 0 && nav == s.provisionalID {
		r = s.provisional
	}
	benchLoad := nav != 0 && nav == s.homeID
	if n.prompts != nil {
		n.prompts.committed(itemID, benchLoad, r.clicked, !samePageSite(s.page, address))
	}
	s.page = address
	if benchLoad {
		// The item's own address, wherever it redirected to, ends a page
		// clicked through to: the item's own sign-in starts without a click.
		s.signInByClick, s.returning, s.homeReloads, s.homeID = false, false, 0, 0
	} else {
		s.signInByClick = SignInByClick(itemURL, address, false, true, s.signInByClick)
		if r.valid && r.kind == navHistory && !onItemSite(itemURL, address) {
			s.signInByClick = true
		}
		if r.clicked {
			s.returning, s.homeReloads = false, 0 // The user went somewhere else.
		}
	}
	s.signInInView = s.signInByClick
	s.clickAt, s.clickEvent = 0, 0
	if r.valid && r.asClick() {
		s.clickAt, s.clickEvent = now, r.event
	}
	s.provisionalID, s.provisional = 0, navRecord{}
}

// failed notes that navigation nav failed before its page came into view:
// replaced is true when it was cancelled, which is how a newer navigation
// replaces it. It reports whether the native side then loads the item's
// address again.
func (n *navigator) failed(itemID string, nav uint64, replaced bool) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.state(itemID)
	returnHome := false
	if nav != 0 && nav == s.homeID {
		s.homeID = 0
		// A load that fails by itself, such as when a redirect of it is
		// cancelled, leaves the item going back: the page in view, which it
		// would have replaced, mustn't go anywhere without a click, nor have
		// the item load its address again and again.
		if replaced && s.returning && !s.replacement(nav).clicked && s.homeReloads < maxHomeReloads {
			s.homeReloads++
			returnHome = true
		}
	}
	if nav != 0 && nav == s.provisionalID {
		s.provisionalID, s.provisional = 0, navRecord{}
	}
	if !replaced {
		// The page in view stays, and so does what it is.
		s.signInByClick = s.signInInView
	}
	return returnHome
}

// replacement returns the record of the navigation that replaced nav: the
// last one kept in view, if it hasn't started, or the one under way.
func (s *navState) replacement(nav uint64) navRecord {
	if s.pending.valid {
		return s.pending
	}
	if s.provisionalID != nav {
		return s.provisional
	}
	return navRecord{}
}

// processEnded notes that the page's process ended: the navigations under
// way, and Bench's load, ended with it, and the page that comes back is
// judged as it comes (going back home, it's a reload, see navigator).
func (n *navigator) processEnded(itemID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.state(itemID)
	s.pending, s.provisionalID, s.provisional = navRecord{}, 0, navRecord{}
	s.homeID, s.returning, s.homeReloads = 0, false, 0
}
