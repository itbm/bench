package mac

import "testing"

func TestDownloadLimiter(t *testing.T) {
	l := gestureLimiter{perEvent: downloadsPerEvent, window: downloadWindow}
	const at = 1000.0
	if l.allow("w1", 0, at) {
		t.Error("allowed a download with no user event")
	}
	for i := range downloadsPerEvent {
		if !l.allow("w1", at, at+float64(i)) {
			t.Fatalf("download %d after a click refused", i+1)
		}
	}
	if l.allow("w1", at, at+10) {
		t.Errorf("allowed more than %d downloads for one click", downloadsPerEvent)
	}
	// Another item has a budget of its own.
	if !l.allow("w2", at, at+1) {
		t.Error("another item's download refused")
	}
	// A new click allows more, but not long after it.
	next := at + 100
	if !l.allow("w1", next, next+1) {
		t.Error("download after a new click refused")
	}
	if l.allow("w1", next, next+downloadWindow.Seconds()+1) {
		t.Error("allowed a download long after the click")
	}
	// An older event, from a popup, shares the newest one's budget.
	for range downloadsPerEvent - 1 {
		l.allow("w1", next-50, next+2)
	}
	if l.allow("w1", next-50, next+3) {
		t.Error("an older event reset the budget")
	}
	// A clock that went backwards allows nothing.
	if l.allow("w3", at, at-1) {
		t.Error("allowed a download before the click")
	}
}

func TestOpenLimiter(t *testing.T) {
	l := gestureLimiter{perEvent: opensPerEvent, window: openWindow}
	const at = 1000.0
	if l.allow("w1", 0, at) {
		t.Error("allowed an open with no user event")
	}
	if !l.allow("w1", at, at+0.1) {
		t.Fatal("the open a click allows refused")
	}
	for i := range 100 {
		if l.allow("w1", at, at+0.2+float64(i)/1000) {
			t.Fatalf("one click opened %d things", i+2)
		}
	}
	// Each key press allows one.
	for i := range 3 {
		key := at + 1 + float64(i)
		if !l.allow("w1", key, key+0.01) {
			t.Errorf("key press %d's open refused", i+1)
		}
		if l.allow("w1", key, key+0.02) {
			t.Errorf("key press %d opened two things", i+1)
		}
	}
	// A redirect long after its click still opens, but not after a minute.
	if !l.allow("w2", at, at+30) {
		t.Error("a slow redirect's open refused")
	}
	if l.allow("w3", at, at+openWindow.Seconds()+1) {
		t.Error("allowed an open long after the click")
	}
}

// TestGestureLimiterGates checks that navigations and popups that would open
// the browser or another app are cancelled once their user event has
// allowed one, and that others aren't counted.
func TestGestureLimiterGates(t *testing.T) {
	l := gestureLimiter{perEvent: opensPerEvent, window: openWindow}
	const at = 1000.0
	if got := l.navigation("w1", NavigateInView, 0, at); got != NavigateInView {
		t.Errorf("a navigation in view = %v, want NavigateInView", got)
	}
	if got := l.navigation("w1", NavigateCancel, 0, at); got != NavigateCancel {
		t.Errorf("a cancelled navigation = %v, want NavigateCancel", got)
	}
	if got := l.popup("w1", PopupWindow, 0, at); got != PopupWindow {
		t.Errorf("a popup window = %v, want PopupWindow", got)
	}
	if got := l.navigation("w1", NavigateInBrowser, 0, at); got != NavigateCancel {
		t.Errorf("a browser open with no user event = %v, want NavigateCancel", got)
	}
	// A loop of target=_blank links to another site, or of mailto: links, in
	// one click.
	if got := l.popup("w1", PopupBrowser, at, at+0.1); got != PopupBrowser {
		t.Fatalf("the first popup for the browser = %v, want PopupBrowser", got)
	}
	for _, next := range []Popup{PopupBrowser, PopupAsk} {
		if got := l.popup("w1", next, at, at+0.2); got != PopupCancel {
			t.Errorf("another popup for the same click = %v, want PopupCancel", got)
		}
	}
	for _, next := range []Navigation{NavigateInBrowser, NavigateAsk} {
		if got := l.navigation("w1", next, at, at+0.3); got != NavigateCancel {
			t.Errorf("another open for the same click = %v, want NavigateCancel", got)
		}
	}
	click := at + 5
	if got := l.navigation("w1", NavigateAsk, click, click+0.1); got != NavigateAsk {
		t.Errorf("a new click's prompt = %v, want NavigateAsk", got)
	}
}
