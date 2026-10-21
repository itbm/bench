package mac

import (
	"log/slog"
	"net"
	"strconv"
	"sync/atomic"
)

// webLog is where Web items' web views log what they decide about each
// navigation and new window (see logDecision), or nil for nowhere. Bench
// sets it when BENCH_DEBUG is set (see Platform.SetDebugLog), so that a
// script on a real Mac, such as scripts/macos-webitems, can tell what
// WebKit asked and what Bench did.
var webLog atomic.Pointer[slog.Logger]

// logDecision logs, at debug level, what a Web item's web view decided
// about a navigation or a new window (msg) for targetURL: the item, the
// decision, and the target's scheme and host (see logTarget), followed by
// attrs. It logs nothing with a nil log.
func logDecision(log *slog.Logger, msg, itemID, targetURL, decision string, attrs ...any) {
	if log == nil {
		return
	}
	scheme, host := logTarget(targetURL)
	log.Debug(msg, append([]any{"item", itemID, "decision", decision, "scheme", scheme, "host", host}, attrs...)...)
}

// logTarget returns what the log says of an address: its scheme, and for
// a web address its host, with the port if it has one. A blob: page's host
// is its origin's (see webPage). Nothing else of an address goes in the
// log: a web address's user name, path, query, and fragment can hold
// tokens and private names, and so can the whole of a mailto: or tel:
// address, so those have the scheme alone. An address that doesn't parse,
// or none, as for a blank window, has neither.
func logTarget(address string) (scheme, host string) {
	u, ok := parseAddress(address)
	if !ok {
		return "", ""
	}
	page, ok := webPage(address)
	if !ok {
		return u.Scheme, ""
	}
	host = hostOf(page)
	if port := page.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	return u.Scheme, host
}

// String names a navigation decision for the log.
func (n Navigation) String() string {
	switch n {
	case NavigateInView:
		return "in-view"
	case NavigateInBrowser:
		return "in-browser"
	case NavigateCancel:
		return "cancel"
	case NavigateAsk:
		return "ask"
	}
	return "navigation-" + strconv.Itoa(int(n))
}

// String names a popup decision for the log.
func (p Popup) String() string {
	switch p {
	case PopupWindow:
		return "window"
	case PopupBrowser:
		return "in-browser"
	case PopupCancel:
		return "cancel"
	case PopupAsk:
		return "ask"
	}
	return "popup-" + strconv.Itoa(int(p))
}

// String names a navigation's kind for the log.
func (k navKind) String() string {
	switch k {
	case navOther:
		return "other"
	case navLink:
		return "link"
	case navHistory:
		return "history"
	}
	return "kind-" + strconv.Itoa(int(k))
}

// String names what started a navigation for the log.
func (s navSource) String() string {
	switch s {
	case fromBench:
		return "bench"
	case fromNone:
		return "none"
	case fromPage:
		return "page"
	case fromFrame:
		return "frame"
	case fromWindow:
		return "window"
	}
	return "source-" + strconv.Itoa(int(s))
}
