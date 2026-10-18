package mac

import (
	"net"
	"net/url"
	"strings"
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

// DecideNavigation says what a Web item's web view does when its main frame
// navigates from the item's own address, itemURL, to targetURL. userClicked
// is true when the user clicked a link.
//
// Pages on the item's site (the same registrable domain, or a related one,
// such as office.com and office365.com) stay in the view, and so do the
// sign-in pages of the identity providers Bench knows (see isSignIn). So
// does any page reached without a click, such as a redirect, because
// sign-in flows redirect through other sites. A link to another site that
// the user clicks opens in the default browser: a web view has no address
// bar, so a page from another site mustn't look like the item's own.
//
// Addresses for other apps need a click, so that pages can't launch apps
// unasked. mailto: and tel: then open in their app; addresses for file
// servers and screen sharing (smb:, afp:, vnc:, and the like), which macOS
// would connect to, never open, nor do local files; and the user is asked
// about any other app first (see appSchemeFor).
func DecideNavigation(itemURL, targetURL string, userClicked bool) Navigation {
	target, ok := parseAddress(targetURL)
	if !ok {
		return NavigateCancel
	}
	switch target.Scheme {
	case "http", "https":
	case "about", "data", "blob", "javascript":
		// The page's own content, not another site.
		return NavigateInView
	default:
		return appNavigation(target.Scheme, userClicked)
	}
	if !userClicked || isSignIn(target) {
		return NavigateInView
	}
	item, ok := parseAddress(itemURL)
	if !ok || sameSite(item, target) {
		return NavigateInView
	}
	return NavigateInBrowser
}

// DecidePopup says what a Web item's web view does when a page opens a new
// window for targetURL. Web items don't let pages open windows without a
// click, so the user asked for every window that gets here.
//
// A blank window (which sign-in libraries open first, then fill), a page on
// the item's site, or a sign-in page gets a real popup window, so that
// window.opener works. Where a popup goes next is up to
// DecidePopupNavigation. Other web pages open in the default browser.
// Addresses for other apps are handled as clicked links are (see
// DecideNavigation). Anything else, such as a javascript: or data:
// address, opens nothing.
func DecidePopup(itemURL, targetURL string) Popup {
	if strings.TrimSpace(targetURL) == "" {
		return PopupWindow
	}
	target, ok := parseAddress(targetURL)
	if !ok {
		return PopupCancel
	}
	switch target.Scheme {
	case "http", "https":
	case "about":
		if target.Opaque == "blank" {
			return PopupWindow
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
		return DecideNavigation(itemURL, targetURL, userClicked)
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

// isWeb reports whether a parsed address is an http or https one.
func isWeb(u *url.URL) bool {
	return u.Scheme == "http" || u.Scheme == "https"
}

// parseAddress parses an absolute address, lower-casing its scheme. It
// reports false for anything without a scheme, and for http and https
// addresses without a host.
func parseAddress(address string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Scheme == "" {
		return nil, false
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() == "" {
		return nil, false
	}
	return u, true
}

// sameSite reports whether two web addresses are on the same site, or on
// related sites (see relatedSites).
func sameSite(a, b *url.URL) bool {
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

// siteOf returns an address's site: its registrable domain, such as
// slack.com for app.slack.com, or bbc.co.uk for www.bbc.co.uk. Local hosts
// and IP addresses are sites of their own on each port, so a local server
// such as VS Code for the Web doesn't share a site with every other local
// server.
func siteOf(u *url.URL) string {
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" {
		return ""
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		if port := u.Port(); port != "" {
			return host + ":" + port
		}
		return host
	}
	labels := strings.Split(host, ".")
	n := 2
	if len(labels) >= 3 && publicSuffixes[strings.Join(labels[len(labels)-2:], ".")] {
		n = 3
	}
	return strings.Join(labels[len(labels)-n:], ".")
}

// publicSuffixes are two-label suffixes under which each name is a site of
// its own: country domains such as co.uk, and hosting services that give
// every customer a subdomain, such as github.io. It's a short list of the
// common ones, not the whole Public Suffix List.
var publicSuffixes = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true, "me.uk": true,
	"ltd.uk": true, "plc.uk": true, "net.uk": true, "sch.uk": true, "nhs.uk": true,
	"com.au": true, "net.au": true, "org.au": true, "edu.au": true, "gov.au": true,
	"co.nz": true, "org.nz": true, "net.nz": true, "govt.nz": true,
	"co.jp": true, "ne.jp": true, "or.jp": true, "ac.jp": true, "go.jp": true,
	"co.in": true, "co.kr": true, "co.za": true, "co.il": true,
	"com.br": true, "com.cn": true, "com.hk": true, "com.sg": true, "com.mx": true,
	"com.tr": true, "com.tw": true, "com.ar": true,

	"github.io": true, "gitlab.io": true, "herokuapp.com": true, "vercel.app": true,
	"netlify.app": true, "pages.dev": true, "workers.dev": true, "web.app": true,
	"firebaseapp.com": true, "appspot.com": true, "azurewebsites.net": true,
	"cloudfront.net": true, "blogspot.com": true, "onrender.com": true,
	"fly.dev": true, "ngrok.io": true, "ngrok-free.app": true, "trycloudflare.com": true,
}

// relatedSites groups sites that belong to one web app, so that moving
// between them isn't leaving the item. The value names the group.
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

// signInSites are sites whose every page is part of signing in.
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
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if signInHosts[host] || signInSites[siteOf(u)] {
		return true
	}
	for _, prefix := range signInPaths[host] {
		if u.Path == prefix || strings.HasPrefix(u.Path, prefix+"/") {
			return true
		}
	}
	return false
}
