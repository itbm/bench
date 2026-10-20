package mac

import (
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/itbm/bench/internal/platform"
)

// Navigation is what a Web item's web view does with a navigation of its
// main frame.
type Navigation int

const (
	// NavigateInView loads the page in the web view.
	NavigateInView Navigation = iota
	// NavigateInBrowser cancels the navigation and opens the address with
	// the user's default browser (or, for addresses such as mailto:, the
	// app that handles them).
	NavigateInBrowser
	// NavigateCancel cancels the navigation.
	NavigateCancel
	// NavigateAsk cancels the navigation and asks the user whether to open
	// the address with the app that handles it, naming the app.
	NavigateAsk
)

// Popup is what a Web item's web view does when a page opens a new window
// (window.open, or a link with target="_blank").
type Popup int

const (
	// PopupWindow opens a real popup window, so window.opener works, as
	// sign-in flows need.
	PopupWindow Popup = iota
	// PopupBrowser opens the address with the user's default browser.
	PopupBrowser
	// PopupCancel opens nothing.
	PopupCancel
	// PopupAsk opens no window, and asks the user whether to open the
	// address with the app that handles it, naming the app.
	PopupAsk
)

// gestureWindow is how soon after the user's last mouse-up or key-down in a
// web view a link activated there counts as clicked by the user. WebKit
// reports a link that a script clicks (a.click()) just as it reports one the
// user clicks, so the user's own event is what tells them apart. A real
// click activates its link at once.
const gestureWindow = 2 * time.Second

// UserClicked reports whether the user clicked a link that a web view
// activated (linkActivated, WebKit's WKNavigationTypeLinkActivated):
// whether there was a mouse-up or key-down in that web view sinceUserEvent
// seconds ago, within gestureWindow. A negative sinceUserEvent means none.
func UserClicked(linkActivated bool, sinceUserEvent float64) bool {
	if !linkActivated || sinceUserEvent < 0 {
		return false
	}
	return time.Duration(sinceUserEvent*float64(time.Second)) <= gestureWindow
}

// navigationFrom says how a navigation of a Web item's main frame started,
// for DecideNavigation. The navigator works it out (see navigator).
type navigationFrom struct {
	// clicked is whether the user clicked: a link they clicked (see
	// UserClicked), or a redirect of a navigation they clicked, however long
	// its server takes.
	clicked bool
	// own is whether the item started it: Bench, loading the item's address,
	// or the web view itself, with no source frame; the item's page in its
	// main frame, while that page is on the item's site or is one the item
	// went to by itself; or going back, forward, or reloading. A frame
	// inside the page, such as embedded content from another site, a window
	// the page opened, and a page on another site that the user clicked
	// through to, don't start the item's own navigations.
	own bool
	// afterClick is whether a page on the item's site that a click brought
	// into view started it, just after that (see afterClickWindow): a
	// client-side redirector, such as a link tracker's page.
	afterClick bool
}

// asClick reports whether a navigation counts as clicked when it goes to
// another site: one the user clicked, or one the item didn't start, or a
// redirector's just after a click.
func (f navigationFrom) asClick() bool {
	return f.clicked || !f.own || f.afterClick
}

// DecideNavigation says what a Web item's web view does when its main frame
// navigates from the item's own address, itemURL, to targetURL. from says
// how the navigation started. signInByClick is true when the page in view is
// one the user clicked through to on another site, which only a sign-in page
// is (see SignInByClick).
//
// Pages on the item's site (the same registrable domain, or a related one,
// such as office.com and office365.com) stay in the view, and so do the
// sign-in pages of the identity providers Bench knows (see isSignIn). So
// does any other page that the item goes to by itself, such as by a
// redirect, because sign-in flows redirect through other sites. A link to
// another site that the user clicks opens in the default browser: a web view
// has no address bar, so a page from another site mustn't look like the
// item's own. So does a page on another site that something other than the
// item sends it to (see navigationFrom): embedded content from another site,
// or a window the page opened, mustn't be able to put any site it likes in
// place of the item's page. The page's own content, such as about:blank or a
// blob: page, stays in the view only when the item started it, for the same
// reason.
//
// A sign-in page that the user clicked through to, such as one in a link
// someone posted, may belong to anyone: many identity providers give anyone
// a sign-in page of their own. So from there, a page reached without a
// click that's neither the item's site nor a sign-in page opens in the
// default browser too, rather than in a view that looks like the item.
//
// Addresses for other apps need the user's click, so that pages can't launch
// apps unasked. mailto: and tel: then open in their app; addresses for file
// servers and screen sharing (smb:, afp:, vnc:, and the like), which macOS
// would connect to, never open, nor do local files; and the user is asked
// about any other app first (see appSchemeFor).
func DecideNavigation(itemURL, targetURL string, from navigationFrom, signInByClick bool) Navigation {
	target, ok := parseAddress(targetURL)
	if !ok {
		return NavigateCancel
	}
	switch target.Scheme {
	case "http", "https":
	case "about", "data", "blob", "javascript":
		// The page's own content, not another site.
		if !from.own {
			return NavigateCancel
		}
		return NavigateInView
	default:
		return appNavigation(target.Scheme, from.clicked)
	}
	if isSignIn(target) {
		return NavigateInView
	}
	item, ok := parseAddress(itemURL)
	if !ok || sameSite(item, target) || (!from.asClick() && !signInByClick) {
		return NavigateInView
	}
	return NavigateInBrowser
}

// SignInByClick says whether the page in a Web item's web view is one the
// user clicked through to on another site (see DecideNavigation), as its
// main frame goes to targetURL. asClick is whether the navigation counts as
// clicked (see navigationFrom), and signInByClick whether the page in view
// was one before. committed is false when DecideNavigation has just kept a
// navigation to targetURL in the view, and true once targetURL's page has
// replaced the one in view.
//
// A click to another site's sign-in page starts it as soon as the
// navigation is decided, so that the redirects that follow are judged by
// it, and so does anything else that counts as a click, such as embedded
// content from another site sending the item there. Only a page on the
// item's site, once it's in view, ends it: a navigation that's decided but
// never shows its page, such as one that the page in view replaces at once
// with another, mustn't end it while that page stays. Other pages keep it
// as it was: those in the page itself, and pages reached from there, which
// are sign-in pages. A navigation that fails before its page shows, other
// than by being replaced, puts it back as it was when the page in view came
// into view, which the navigator keeps. The navigator also starts it for a
// page on another site that going back, forward, or reloading brings back
// (see navigator).
func SignInByClick(itemURL, targetURL string, asClick, committed, signInByClick bool) bool {
	target, ok := parseAddress(targetURL)
	if !ok || !isWeb(target) {
		return signInByClick
	}
	item, ok := parseAddress(itemURL)
	if !ok {
		return false
	}
	if sameSite(item, target) {
		return signInByClick && !committed
	}
	return signInByClick || (!committed && asClick && isSignIn(target))
}

// DecidePopup says what a Web item's web view does when a page opens a new
// window for targetURL. Web items don't let pages open windows without a
// click, so the user asked for every window that gets here. sourceOrigin is
// the security origin of the frame that asked, such as
// https://app.slack.com, and sourceInItem whether that frame is in the
// item's own web view, rather than in a popup.
//
// A blank window (which sign-in libraries open first, then fill), a page on
// the item's site, or a sign-in page gets a real popup window, so that
// window.opener works. Where a popup goes next is up to
// DecidePopupNavigation. Other web pages open in the default browser.
// Addresses for other apps are handled as clicked links are (see
// DecideNavigation). Anything else, such as a javascript: or data:
// address, opens nothing.
//
// Only a frame in the item's own web view whose page is on the item's site
// may open a blank window: the frame that opens one can write anything into
// it, and a window with no address shows no site. Embedded content from
// another site, such as an advert, a page on another site in the item's
// view, such as someone's sign-in page that the user clicked through to,
// or a page in a popup, could make it look like a sign-in page, or like
// one of Bench's own windows.
func DecidePopup(itemURL, targetURL, sourceOrigin string, sourceInItem bool) Popup {
	if strings.TrimSpace(targetURL) == "" {
		return blankPopup(itemURL, sourceOrigin, sourceInItem)
	}
	target, ok := parseAddress(targetURL)
	if !ok {
		return PopupCancel
	}
	switch target.Scheme {
	case "http", "https":
	case "about":
		if target.Opaque == "blank" {
			return blankPopup(itemURL, sourceOrigin, sourceInItem)
		}
		return PopupCancel
	case "data", "blob", "javascript":
		return PopupCancel
	default:
		switch appSchemeFor(target.Scheme) {
		case appSchemeOpen:
			return PopupBrowser
		case appSchemeAsk:
			return PopupAsk
		default:
			return PopupCancel
		}
	}
	if isSignIn(target) {
		return PopupWindow
	}
	if item, ok := parseAddress(itemURL); ok && sameSite(item, target) {
		return PopupWindow
	}
	return PopupBrowser
}

// blankPopup decides a blank popup, by the frame that asked for it: its
// origin, and whether it's in the item's own web view (see DecidePopup).
func blankPopup(itemURL, sourceOrigin string, sourceInItem bool) Popup {
	if !sourceInItem {
		return PopupCancel
	}
	source, ok := parseAddress(sourceOrigin)
	if !ok || !isWeb(source) {
		return PopupCancel
	}
	if item, ok := parseAddress(itemURL); ok && sameSite(item, source) {
		return PopupWindow
	}
	return PopupCancel
}

// DecidePopupNavigation says what a popup window (one a Web item's page
// opened, see DecidePopup) does when its main frame navigates to targetURL.
// startURL is the first page the popup showed: the address it was opened
// for, or else the first one it went to, or "" while it's blank.
//
// A popup that started on a sign-in page follows the sign-in flow wherever
// it goes, through identity providers that Bench doesn't know too. Any
// other popup stays on the item's site and sign-in pages: another site,
// clicked or not, opens in the default browser. That's what sends links
// through redirectors on the item's site, such as Outlook's Safe Links or
// Google's /url, and blank windows that a page sends elsewhere, to the
// browser rather than into a window that looks like Bench's own. Addresses
// for other apps are handled as in DecideNavigation.
func DecidePopupNavigation(itemURL, startURL, targetURL string, userClicked bool) Navigation {
	target, ok := parseAddress(targetURL)
	if !ok {
		return NavigateCancel
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return DecideNavigation(itemURL, targetURL, navigationFrom{clicked: userClicked, own: true}, false)
	}
	if start, ok := parseAddress(startURL); ok && isWeb(start) && isSignIn(start) {
		return NavigateInView
	}
	if isSignIn(target) {
		return NavigateInView
	}
	if item, ok := parseAddress(itemURL); !ok || sameSite(item, target) {
		return NavigateInView
	}
	return NavigateInBrowser
}

// appScheme is what a web view does with a clicked address for another app.
type appScheme int

const (
	// appSchemeAsk asks the user first, naming the app that would open it.
	appSchemeAsk appScheme = iota
	// appSchemeOpen opens it with its app at once.
	appSchemeOpen
	// appSchemeBlock never opens it.
	appSchemeBlock
)

// openSchemes are addresses for other apps that open at once when clicked,
// as in every browser: writing an email, and calling a number (which
// FaceTime asks about itself).
var openSchemes = map[string]bool{"mailto": true, "tel": true}

// blockedSchemes are addresses that never open from a web page: local
// files, and file servers and screen sharing, which Finder and Screen
// Sharing would connect to (and offer the user's password to) at once.
var blockedSchemes = map[string]bool{
	"file": true, "smb": true, "cifs": true, "afp": true, "nfs": true,
	"ftp": true, "ftps": true, "sftp": true, "vnc": true,
}

// appSchemeFor says what a web view does with a clicked address of the given
// scheme (lower case), which isn't a web address.
func appSchemeFor(scheme string) appScheme {
	switch {
	case blockedSchemes[scheme]:
		return appSchemeBlock
	case openSchemes[scheme]:
		return appSchemeOpen
	default:
		return appSchemeAsk
	}
}

// appNavigation decides a main-frame navigation to an address for another
// app.
func appNavigation(scheme string, userClicked bool) Navigation {
	action := appSchemeFor(scheme)
	switch {
	case action == appSchemeBlock || !userClicked:
		return NavigateCancel
	case action == appSchemeOpen:
		return NavigateInBrowser
	default:
		return NavigateAsk
	}
}

// notificationAllowed reports whether a page may post a notification for
// its Web item: only the item's main frame may, on the item's site, so that
// embedded content (ads, other apps' panels) and pages the item has been
// sent to can't post notifications that look like the item's. origin is the
// page's security origin, such as https://app.slack.com.
func notificationAllowed(itemURL, origin string, mainFrame bool) bool {
	if !mainFrame {
		return false
	}
	page, ok := parseAddress(origin)
	if !ok || !isWeb(page) {
		return false
	}
	item, ok := parseAddress(itemURL)
	return ok && sameSite(item, page)
}

// PageInfo describes the page at address, which a Web item whose own
// address is itemURL shows, for a WebURL event. onlySecureContent is
// WebKit's hasOnlySecureContent for the page: whether everything it has
// loaded came over secure connections.
//
// The host is the parsed address's, never a part of its text: in
// https://app.slack.com@evil.example/, app.slack.com is a user name, and
// the host is evil.example. A blob: page, which a page makes of content it
// has, belongs to the origin inside its address, as browsers show it: the
// host of blob:https://evil.example/1234 is evil.example, and the page is on
// a site, and secure, as a page of that origin would be; but WebKit's
// hasOnlySecureContent is false for any address that isn't https, so in
// practice only a local blob: page is secure.
//
// Any other page, such as about:blank, a data: page, or a blob: page whose
// address has no http or https origin inside, has no web address to show.
// A page can still move to one, and fill it with anything, so it's off
// site, with no host, and secure only if all its content is. Only an empty
// address, for no page at all, gives an empty Page.
//
// An https page is secure if all its content is. So is a page on the Mac
// itself, at localhost or a loopback address, as browsers count it: VS
// Code for the Web's server is one. Any other http page isn't. The page is
// off site when it isn't on the item's site, or a related one (see
// sameSite), even if it's a sign-in page: anyone can have a sign-in page
// with some identity providers, so the user should see whose it is.
func PageInfo(itemURL, address string, onlySecureContent bool) platform.Page {
	if strings.TrimSpace(address) == "" {
		return platform.Page{}
	}
	u, ok := parseAddress(address)
	if ok && u.Scheme == "blob" {
		u, ok = parseAddress(u.Opaque)
	}
	if !ok || !isWeb(u) {
		return platform.Page{OffSite: true, Secure: onlySecureContent}
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	site := siteOf(u)
	if port := u.Port(); port != "" {
		site = strings.TrimSuffix(site, ":"+port) // A local host's site has its port.
	}
	page := platform.Page{Host: host, Site: site}
	page.Secure = isLocal(host) || (u.Scheme == "https" && onlySecureContent)
	if item, ok := parseAddress(itemURL); ok && isWeb(item) {
		page.OffSite = !sameSite(item, u)
	}
	return page
}

// pageSubtitle is the subtitle of a window that shows a page (see
// PageInfo): its host, or "Page with no address" for a page with no web
// address, after "Not secure — " if it isn't secure; or "" for no page.
func pageSubtitle(page platform.Page) string {
	switch {
	case page.Host != "" && !page.Secure:
		return "Not secure — " + page.Host
	case page.Host != "":
		return page.Host
	case !page.OffSite:
		return ""
	case !page.Secure:
		return "Not secure — page with no address"
	}
	return "Page with no address"
}

// isLocal reports whether a host, in lower case, is the Mac itself:
// localhost, a name under it, or a loopback address.
func isLocal(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isWeb reports whether a parsed address is an http or https one.
func isWeb(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

// parseAddress parses an absolute address, lower-casing its scheme. It
// reports false for anything without a scheme, and for http and https
// addresses without a host.
//
// WebKit passes on a % that doesn't start an escape, as in /sale/50%off or
// #q=100%, as it is, and url.Parse refuses it. So when an address doesn't
// parse, each such % is escaped (as %25) and it's parsed again, but only for
// a host without one: a host with a % in it isn't one WebKit would go to.
func parseAddress(address string) (*url.URL, bool) {
	address = strings.TrimSpace(address)
	u, err := url.Parse(address)
	repaired := false
	if err != nil {
		u, err = url.Parse(escapeStrayPercents(address))
		repaired = true
	}
	if err != nil || u.Scheme == "" {
		return nil, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if repaired && strings.Contains(u.Host, "%") {
		return nil, false
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() == "" {
		return nil, false
	}
	return u, true
}

// escapeStrayPercents escapes each % in s that isn't followed by two hex
// digits.
func escapeStrayPercents(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && (i+2 >= len(s) || !isHex(s[i+1]) || !isHex(s[i+2])) {
			b.WriteString("%25")
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// isHex reports whether c is a hex digit.
func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// sameSite reports whether two web addresses are on the same site, or on
// related sites (see relatedSites). A page that anyone can make, on a host
// that also serves an app (see userContent), is on the same site only as
// another such page of the same kind.
func sameSite(a, b *url.URL) bool {
	ka, ua := userContentKey(a)
	kb, ub := userContentKey(b)
	if ua || ub {
		return ua && ub && ka == kb
	}
	sa, sb := siteOf(a), siteOf(b)
	if sa == "" || sb == "" {
		return false
	}
	if sa == sb {
		return true
	}
	ga, gb := relatedSites[sa], relatedSites[sb]
	return ga != "" && ga == gb
}

// onItemSite reports whether the page at address is on the site of the Web
// item whose own address is itemURL, as PageInfo counts it: a blob: page is
// its origin's, a page with no web address is on no site, and for an item
// without a web address of its own, every page is on its site. address may
// be a security origin, such as https://app.slack.com.
func onItemSite(itemURL, address string) bool {
	item, ok := parseAddress(itemURL)
	if !ok || !isWeb(item) {
		return true
	}
	page, ok := webPage(address)
	return ok && sameSite(item, page)
}

// webPage parses the address of a page as a web address, as PageInfo
// counts it: a blob: page is its origin's. It reports false for a page
// with no web address.
func webPage(address string) (*url.URL, bool) {
	u, ok := parseAddress(address)
	if ok && u.Scheme == "blob" {
		u, ok = parseAddress(u.Opaque)
	}
	return u, ok && isWeb(u)
}

// sameAddress reports whether two web addresses name the same page, as
// WebKit writes them: the same scheme, host, port, path (where an empty one
// is /), and query.
func sameAddress(a, b string) bool {
	ua, ok := parseAddress(a)
	if !ok || !isWeb(ua) {
		return false
	}
	ub, ok := parseAddress(b)
	if !ok || !isWeb(ub) {
		return false
	}
	path := func(u *url.URL) string {
		if u.EscapedPath() == "" {
			return "/"
		}
		return u.EscapedPath()
	}
	return ua.Scheme == ub.Scheme && hostOf(ua) == hostOf(ub) && ua.Port() == ub.Port() &&
		path(ua) == path(ub) && ua.RawQuery == ub.RawQuery
}

// hostOf returns a parsed address's host in lower case, without a trailing
// dot.
func hostOf(u *url.URL) string {
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
}

// siteOf returns an address's site, which the host chip emphasises, and
// which sameSite compares:
//
//   - Its registrable domain on the Public Suffix List, such as slack.com
//     for app.slack.com, bbc.co.uk for www.bbc.co.uk, or alice.github.io
//     for alice.github.io, since github.io gives each customer a name of
//     its own. A host that is a public suffix itself, such as co.uk, is its
//     own site.
//   - Under a domain that gives each customer a name of its own but isn't
//     on the list (see tenantDomains), the customer's name, such as
//     mycompany.atlassian.net.
//   - For a host that serves pages anyone can make (see userContent), such
//     as sites.google.com, the whole host.
//   - Local hosts and IP addresses are sites of their own on each port, so
//     a local server such as VS Code for the Web doesn't share a site with
//     every other local server.
func siteOf(u *url.URL) string {
	host := hostOf(u)
	if host == "" {
		return ""
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		if port := u.Port(); port != "" {
			return host + ":" + port
		}
		return host
	}
	if _, ok := userContentKey(u); ok {
		return host
	}
	if tenant := tenantOf(host); tenant != "" {
		return tenant
	}
	site, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host // A public suffix itself, or a name with an empty label.
	}
	return site
}

// tenantDomains are domains where each customer has a name of its own, such
// as mycompany.atlassian.net, and anyone can be a customer, but which the
// Public Suffix List doesn't list: each name below one is a site of its own,
// so that another customer's pages aren't the item's own. They don't include
// domains whose own apps move between names in normal use, such as
// slack.com, where Slack moves between app.slack.com and each workspace's
// name, or google.com.
var tenantDomains = map[string]bool{
	"atlassian.net":    true, // Jira and Confluence.
	"service-now.com":  true,
	"zendesk.com":      true,
	"freshdesk.com":    true,
	"freshservice.com": true,
	"sharepoint.com":   true,

	// Identity providers that give each customer a sign-in page of its own,
	// so that the host chip emphasises whose it is.
	"okta.com":             true,
	"oktapreview.com":      true,
	"okta-emea.com":        true,
	"onelogin.com":         true,
	"auth0.com":            true,
	"us.auth0.com":         true,
	"eu.auth0.com":         true,
	"au.auth0.com":         true,
	"jp.auth0.com":         true,
	"cloudflareaccess.com": true,
}

// tenantOf returns a host's customer name, with the longest of the
// tenantDomains it's under, such as mycompany.atlassian.net for
// wiki.mycompany.atlassian.net, or "" for a host under none of them.
func tenantOf(host string) string {
	labels := strings.Split(host, ".")
	for i := 1; i < len(labels); i++ {
		if tenantDomains[strings.Join(labels[i:], ".")] {
			return strings.Join(labels[i-1:], ".")
		}
	}
	return ""
}

// userContentHosts are hosts on an app's site, or a related one, that serve
// pages anyone can make and share: a page there is on a site of its own, not
// the app's, so that it can't pass for the item's own page. Microsoft Forms
// and Sway, Google Sites, and Apps Script's web apps.
var userContentHosts = map[string]bool{
	"forms.office.com":      true,
	"forms.cloud.microsoft": true,
	"sway.office.com":       true,
	"sway.cloud.microsoft":  true,
	"sites.google.com":      true,
	"script.google.com":     true,
}

// userContentKey reports whether a web address is a page that anyone can
// make on a host that also serves an app (see userContentHosts), and if so
// returns what kind of page it is, which sameSite compares: its host, and
// for Google Docs, whether it's a form or a document published to the web
// (see googleDocsKind).
func userContentKey(u *url.URL) (string, bool) {
	if !isWeb(u) {
		return "", false
	}
	host := hostOf(u)
	if userContentHosts[host] {
		return host, true
	}
	if host == "docs.google.com" {
		if kind := googleDocsKind(u.Path); kind != "" {
			return host + " " + kind, true
		}
	}
	return "", false
}

// googleDocsKind says whether a path on docs.google.com is a page that anyone
// can make, rather than Google's own editor: "forms" for a form, "published"
// for a document, sheet, or presentation published to the web (whose
// address has /d/e/ where an editor's has /d/), or "".
func googleDocsKind(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "a" {
		parts = parts[2:] // A Google Workspace domain's address: /a/example.com/forms/...
	}
	if len(parts) == 0 {
		return ""
	}
	switch parts[0] {
	case "forms":
		return "forms"
	case "document", "spreadsheets", "presentation":
		for i := 1; i+1 < len(parts); i++ {
			if parts[i] == "d" && parts[i+1] == "e" {
				return "published"
			}
		}
	}
	return ""
}

// relatedSites groups sites that belong to one web app, so that moving
// between them isn't leaving the item. The value names the group. Hosts in
// them that serve pages anyone can make are sites of their own (see
// userContentHosts).
var relatedSites = map[string]string{
	// Outlook on the web and Microsoft 365.
	"office.com":      "microsoft-365",
	"office365.com":   "microsoft-365",
	"cloud.microsoft": "microsoft-365",
	"outlook.com":     "microsoft-365",
	"live.com":        "microsoft-365",
	"office.net":      "microsoft-365",

	// Slack.
	"slack.com":      "slack",
	"slack-edge.com": "slack",

	// Google's apps. Not googleusercontent.com, where Google serves content
	// that anyone can upload.
	"google.com": "google",
}

// signInSites are domains whose every page, on any name under them, is part
// of signing in.
var signInSites = map[string]bool{
	"microsoftonline.com":  true,
	"okta.com":             true,
	"oktapreview.com":      true,
	"okta-emea.com":        true,
	"auth0.com":            true,
	"onelogin.com":         true,
	"duosecurity.com":      true,
	"pingidentity.com":     true,
	"pingone.com":          true,
	"jumpcloud.com":        true,
	"cloudflareaccess.com": true,
}

// signInHosts are hosts used for signing in on sites that also serve other
// pages.
var signInHosts = map[string]bool{
	"accounts.google.com":  true,
	"accounts.youtube.com": true,
	"login.microsoft.com":  true,
	"login.live.com":       true,
	"login.windows.net":    true,
	"account.live.com":     true,
	"appleid.apple.com":    true,
	"idmsa.apple.com":      true,
}

// signInPaths are path prefixes for signing in on hosts that serve other
// pages too.
var signInPaths = map[string][]string{
	"github.com": {"/login", "/session"},
	"gitlab.com": {"/users/sign_in", "/oauth"},
}

// isSignIn reports whether a web address is a sign-in page of an identity
// provider Bench knows, which stays in the item (or opens as a popup)
// rather than going to the browser. Only these lists count, not names that
// look like sign-in pages, such as login.example.com: anyone can make such
// a name, and a page in a web view, which has no address bar, would look
// like the item's own.
func isSignIn(u *url.URL) bool {
	host := hostOf(u)
	if signInHosts[host] || underAny(host, signInSites) {
		return true
	}
	for _, prefix := range signInPaths[host] {
		if u.Path == prefix || strings.HasPrefix(u.Path, prefix+"/") {
			return true
		}
	}
	return false
}

// underAny reports whether host is one of domains, or a name under one.
func underAny(host string, domains map[string]bool) bool {
	for name := host; name != ""; {
		if domains[name] {
			return true
		}
		_, rest, ok := strings.Cut(name, ".")
		if !ok {
			return false
		}
		name = rest
	}
	return false
}
