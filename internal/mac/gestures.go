package mac

import (
	"sync"
	"time"
)

// A Web item's page may start downloadsPerEvent downloads after each mouse-up
// or key-down in its web view (or one of its popups), within downloadWindow
// of it, and none without one: so a page can't fill the Downloads folder by
// itself, but a click that saves a few files, or a server that takes a while
// to answer, still works.
const (
	downloadsPerEvent = 5
	downloadWindow    = 60 * time.Second
)

// A Web item's page may send the user to the default browser or another app
// opensPerEvent times for each mouse-up or key-down in its web view (or one
// of its popups) that allows it, within openWindow of it: so one click
// can't open any number of the browser's tabs, or of Mail's windows, while a
// clicked link whose server takes a while to redirect still opens. What
// allows an open is up to the caller: the click on a link, or the click
// that a redirect or a redirector's page follows (see navigator).
const (
	opensPerEvent = 1
	openWindow    = 60 * time.Second
)

// gestureLimiter holds back what a page does that the user didn't ask for:
// at most perEvent of it for each of the user's events, within window of
// the event. Its methods are safe for concurrent use.
type gestureLimiter struct {
	perEvent int
	window   time.Duration

	mu    sync.Mutex
	items map[string]*gestureBudget
}

// gestureBudget is what an item has done since the user's last event in it,
// at.
type gestureBudget struct {
	at   float64
	used int
}

// webDownloadLimit is the process's download limiter, and webOpenLimit its
// limiter for opening the browser and other apps.
var (
	webDownloadLimit = gestureLimiter{perEvent: downloadsPerEvent, window: downloadWindow}
	webOpenLimit     = gestureLimiter{perEvent: opensPerEvent, window: openWindow}
)

// allow reports whether an item's page may do one more thing now, and if so
// counts it. userEvent is when the user pressed a key or released a mouse
// button in the web view asking, which allows it, or 0 for never, and now
// is the time now, both in seconds on the native side's clock.
func (l *gestureLimiter) allow(itemID string, userEvent, now float64) bool {
	age := time.Duration((now - userEvent) * float64(time.Second))
	if userEvent <= 0 || age < 0 || age > l.window {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.items == nil {
		l.items = map[string]*gestureBudget{}
	}
	b := l.items[itemID]
	if b == nil {
		b = &gestureBudget{}
		l.items[itemID] = b
	}
	// An older event, in a popup say, shares the newest one's budget.
	if userEvent > b.at {
		b.at, b.used = userEvent, 0
	}
	if b.used >= l.perEvent {
		return false
	}
	b.used++
	return true
}

// navigation holds back a navigation that would open the default browser or
// another app (see Navigation) once the user event that allows it,
// userEvent, has allowed as many as it may: it's cancelled instead.
func (l *gestureLimiter) navigation(itemID string, n Navigation, userEvent, now float64) Navigation {
	if (n == NavigateInBrowser || n == NavigateAsk) && !l.allow(itemID, userEvent, now) {
		return NavigateCancel
	}
	return n
}

// popup holds back a popup that would open the default browser or another
// app (see Popup), as navigation does.
func (l *gestureLimiter) popup(itemID string, p Popup, userEvent, now float64) Popup {
	if (p == PopupBrowser || p == PopupAsk) && !l.allow(itemID, userEvent, now) {
		return PopupCancel
	}
	return p
}
