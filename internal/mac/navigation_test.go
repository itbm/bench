package mac

import (
	"net/url"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

const (
	slack   = "https://app.slack.com/client"
	outlook = "https://outlook.office.com/mail/"
	vscode  = "http://localhost:51234/?tkn=abc&folder=/Users/me/code"
)

func TestDecideNavigation(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		target  string
		clicked bool
		want    Navigation
	}{
		{"same page", slack, "https://app.slack.com/client/T1/C2", true, NavigateInView},
		{"same site, other host, clicked", slack, "https://files.slack.com/files-pri/T1/x.png", true, NavigateInView},
		{"same site, plain http", slack, "http://slack.com/", true, NavigateInView},
		{"related site, clicked", outlook, "https://outlook.office365.com/mail/inbox", true, NavigateInView},
		{"related new domain, clicked", outlook, "https://outlook.cloud.microsoft/mail/", true, NavigateInView},
		{"other site, clicked", slack, "https://example.com/article", true, NavigateInBrowser},
		{"other site, http, clicked", slack, "http://example.com/", true, NavigateInBrowser},
		{"other site, redirect", slack, "https://example.com/article", false, NavigateInView},
		{"corporate sign-in by redirect", slack, "https://mycompany.okta.com/app/slack/sso/saml", false, NavigateInView},
		{"Google sign-in, clicked", slack, "https://accounts.google.com/o/oauth2/auth?client_id=x", true, NavigateInView},
		{"Microsoft sign-in, clicked", outlook, "https://login.microsoftonline.com/common/oauth2/authorize", true, NavigateInView},
		{"unknown host labelled sign-in, clicked", slack, "https://sso.example.org/start", true, NavigateInBrowser},
		{"look-alike sign-in, clicked", slack, "https://login.evil.example/slack", true, NavigateInBrowser},
		{"look-alike sign-in, redirect", slack, "https://login.evil.example/slack", false, NavigateInView},
		{"GitHub sign-in path, clicked", slack, "https://github.com/login/oauth/authorize?client_id=x", true, NavigateInView},
		{"GitHub other path, clicked", slack, "https://github.com/itbm/bench", true, NavigateInBrowser},
		{"GitHub path that only starts like sign-in", slack, "https://github.com/loginx", true, NavigateInBrowser},
		{"about:blank", slack, "about:blank", false, NavigateInView},
		{"about:srcdoc", slack, "about:srcdoc", true, NavigateInView},
		{"data address", slack, "data:text/html,hello", false, NavigateInView},
		{"blob address", slack, "blob:https://app.slack.com/1234", false, NavigateInView},
		{"mailto, clicked", outlook, "mailto:someone@example.com", true, NavigateInBrowser},
		{"mailto, not clicked", outlook, "mailto:someone@example.com", false, NavigateCancel},
		{"app link, clicked", slack, "zoommtg://zoom.us/join?confno=1", true, NavigateAsk},
		{"app link, not clicked", slack, "slack://open?team=T1", false, NavigateCancel},
		{"upper-case scheme", slack, "MAILTO:someone@example.com", true, NavigateInBrowser},
		{"file server, clicked", slack, "smb://evil.example/share", true, NavigateCancel},
		{"screen sharing, clicked", slack, "vnc://evil.example", true, NavigateCancel},
		{"FTP, clicked", slack, "ftp://evil.example/x", true, NavigateCancel},
		{"system settings, clicked", slack, "x-apple.systempreferences:com.apple.preference.security", true, NavigateAsk},
		{"local file, clicked", slack, "file:///etc/hosts", true, NavigateCancel},
		{"local file, not clicked", slack, "file:///etc/hosts", false, NavigateCancel},
		{"empty target", slack, "", true, NavigateCancel},
		{"relative target", slack, "/client/T1", true, NavigateCancel},
		{"http without a host", slack, "https:///path", true, NavigateCancel},
		{"unparseable target", slack, "https://exa mple.com/%zz", true, NavigateCancel},
		{"VS Code, same server", vscode, "http://localhost:51234/?folder=/tmp", true, NavigateInView},
		{"VS Code, another local server, clicked", vscode, "http://localhost:3000/", true, NavigateInBrowser},
		{"VS Code, 127.0.0.1 is another site", vscode, "http://127.0.0.1:51234/", true, NavigateInBrowser},
		{"item without an address", "", "https://example.com/", true, NavigateInView},
		{"two-part suffix, same site", "https://www.bbc.co.uk/news", "https://sport.bbc.co.uk/", true, NavigateInView},
		{"two-part suffix, other site", "https://www.bbc.co.uk/news", "https://www.itv.co.uk/", true, NavigateInBrowser},
		{"hosting suffix, other customer", "https://alice.github.io/", "https://bob.github.io/", true, NavigateInBrowser},
		{"trailing dot in host", slack, "https://files.slack.com./x", true, NavigateInView},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideNavigation(tt.item, tt.target, tt.clicked, false); got != tt.want {
				t.Errorf("DecideNavigation(%q, %q, %v) = %v, want %v", tt.item, tt.target, tt.clicked, got, tt.want)
			}
		})
	}
}

func TestDecidePopup(t *testing.T) {
	tests := []struct {
		name   string
		item   string
		target string
		want   Popup
	}{
		{"blank window", slack, "", PopupWindow},
		{"about:blank", slack, "about:blank", PopupWindow},
		{"same site", slack, "https://app.slack.com/huddle/T1", PopupWindow},
		{"related site", outlook, "https://outlook.office365.com/mail/deeplink/compose", PopupWindow},
		{"sign-in page", slack, "https://accounts.google.com/o/oauth2/v2/auth", PopupWindow},
		{"corporate sign-in", outlook, "https://login.microsoftonline.com/common/oauth2/v2.0/authorize", PopupWindow},
		{"other site", slack, "https://example.com/article", PopupBrowser},
		{"link redirector", slack, "https://slack-redir.net/link?url=https%3A%2F%2Fexample.com", PopupBrowser},
		{"look-alike sign-in", slack, "https://login.evil.example/", PopupBrowser},
		{"look-alike Microsoft sign-in", outlook, "https://login.microsoftonline-secure.example/", PopupBrowser},
		{"mailto", outlook, "mailto:someone@example.com", PopupBrowser},
		{"tel", slack, "tel:+441234567890", PopupBrowser},
		{"file server", slack, "smb://evil.example/share", PopupCancel},
		{"app link", slack, "zoommtg://zoom.us/join?confno=1", PopupAsk},
		{"about:srcdoc", slack, "about:srcdoc", PopupCancel},
		{"javascript", slack, "javascript:alert(1)", PopupCancel},
		{"data", slack, "data:text/html,hi", PopupCancel},
		{"file", slack, "file:///etc/hosts", PopupCancel},
		{"relative", slack, "/path", PopupCancel},
		{"item without an address", "", "https://example.com/", PopupBrowser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecidePopup(tt.item, tt.target, "", true); got != tt.want {
				t.Errorf("DecidePopup(%q, %q) = %v, want %v", tt.item, tt.target, got, tt.want)
			}
		})
	}
}

func TestSiteOf(t *testing.T) {
	tests := []struct{ address, want string }{
		{"https://app.slack.com/client", "slack.com"},
		{"https://slack.com", "slack.com"},
		{"https://APP.Slack.COM/", "slack.com"},
		{"https://a.b.c.example.com/", "example.com"},
		{"https://www.bbc.co.uk/", "bbc.co.uk"},
		{"https://co.uk/", "co.uk"},
		{"https://alice.github.io/x", "alice.github.io"},
		{"https://outlook.cloud.microsoft/", "cloud.microsoft"},
		{"http://localhost:8080/", "localhost:8080"},
		{"http://localhost/", "localhost"},
		{"http://app.localhost:3000/", "app.localhost:3000"},
		{"http://127.0.0.1:9000/", "127.0.0.1:9000"},
		{"http://[::1]:9000/", "::1:9000"},
		{"http://intranet/", "intranet"},
		{"https://example.com.:443/", "example.com"},
	}
	for _, tt := range tests {
		u, err := url.Parse(tt.address)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tt.address, err)
		}
		if got := siteOf(u); got != tt.want {
			t.Errorf("siteOf(%q) = %q, want %q", tt.address, got, tt.want)
		}
	}
}

// TestNavigationNeedsAClickToLeave checks the policy's safety rules over
// many addresses: without a click, a page can't send the user to the default
// browser or another app, and local files never load, clicked or not.
func TestNavigationNeedsAClickToLeave(t *testing.T) {
	items := []string{slack, outlook, vscode, "", "about:blank"}
	targets := []string{
		"https://example.com/", "http://example.com/x", "https://accounts.google.com/",
		"mailto:a@example.com", "tel:+441234567890", "slack://open", "zoommtg://zoom.us/join",
		"file:///etc/hosts", "FILE:///etc/hosts", "javascript:void(0)", "about:blank",
		"data:text/html,hi", "blob:https://app.slack.com/1", "https://app.slack.com/client",
		"", "/relative", "not an address", "http://localhost:3000/",
	}
	for _, item := range items {
		for _, target := range targets {
			if got := DecideNavigation(item, target, false, false); got == NavigateInBrowser || got == NavigateAsk {
				t.Errorf("DecideNavigation(%q, %q, false) = %v; only a click may leave the view", item, target, got)
			}
			if strings.HasPrefix(strings.ToLower(target), "file:") {
				for _, clicked := range []bool{false, true} {
					if got := DecideNavigation(item, target, clicked, false); got != NavigateCancel {
						t.Errorf("DecideNavigation(%q, %q, %v) = %v, want NavigateCancel", item, target, clicked, got)
					}
				}
				if got := DecidePopup(item, target, "", true); got != PopupCancel {
					t.Errorf("DecidePopup(%q, %q) = %v, want PopupCancel", item, target, got)
				}
			}
		}
	}
}

// TestDecideNavigationMore covers addresses the main table leaves out.
func TestDecideNavigationMore(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		target  string
		clicked bool
		want    Navigation
	}{
		{"javascript address", slack, "javascript:void(0)", true, NavigateInView},
		{"upper-case web scheme, same site", slack, "HTTPS://APP.SLACK.COM/client", true, NavigateInView},
		{"upper-case web scheme, other site", slack, "HTTPS://EXAMPLE.COM/", true, NavigateInBrowser},
		{"VS Code on IPv6 loopback, same port", "http://[::1]:51234/", "http://[::1]:51234/?folder=/tmp", true, NavigateInView},
		{"VS Code on IPv6 loopback, other port", "http://[::1]:51234/", "http://[::1]:3000/", true, NavigateInBrowser},
		{"sign-in redirect from VS Code", vscode, "https://login.microsoftonline.com/common/oauth2/authorize", false, NavigateInView},
		{"form posted to another site", outlook, "https://example.com/submit", false, NavigateInView},
		{"tel link, clicked", slack, "tel:+441234567890", true, NavigateInBrowser},
		{"Google's user content is another site", "https://mail.google.com/", "https://x.googleusercontent.com/", true, NavigateInBrowser},
		{"SSH, clicked", slack, "ssh://evil.example", true, NavigateAsk},
		{"SSH, not clicked", slack, "ssh://evil.example", false, NavigateCancel},
		{"stray % in the path", slack, "https://app.slack.com/sale/50%off", true, NavigateInView},
		{"stray % in the fragment", slack, "https://app.slack.com/client#q=100%", true, NavigateInView},
		{"stray % at the end", slack, "https://app.slack.com/%", false, NavigateInView},
		{"stray % on another site, clicked", slack, "https://example.com/50%off", true, NavigateInBrowser},
		{"stray % in the host", slack, "https://app%zz.slack.com/", false, NavigateCancel},
		{"stray % with no host", slack, "https:///50%off", false, NavigateCancel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideNavigation(tt.item, tt.target, tt.clicked, false); got != tt.want {
				t.Errorf("DecideNavigation(%q, %q, %v) = %v, want %v", tt.item, tt.target, tt.clicked, got, tt.want)
			}
		})
	}
}

func TestUserClicked(t *testing.T) {
	tests := []struct {
		name           string
		linkActivated  bool
		sinceUserEvent float64
		want           bool
	}{
		{"a link just clicked", true, 0.05, true},
		{"a link at the edge of the window", true, gestureWindow.Seconds(), true},
		{"a link a script clicked long after", true, 30, false},
		{"a link with no user event ever", true, -1, false},
		{"a script's navigation just after a click", false, 0.05, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserClicked(tt.linkActivated, tt.sinceUserEvent); got != tt.want {
				t.Errorf("UserClicked(%v, %v) = %v, want %v", tt.linkActivated, tt.sinceUserEvent, got, tt.want)
			}
		})
	}
}

// TestSignInByClick follows a link someone posted to a sign-in page of their
// own, on an identity provider anyone can use: it may stay in the view, but
// what it sends the user to next mustn't.
func TestSignInByClick(t *testing.T) {
	const tenant = "https://evil.us.auth0.com/login"
	steps := []struct {
		name    string
		target  string
		clicked bool
		// Whether the page comes into view. One that the page in view
		// replaces with another before it does, or that a server answers
		// with nothing to show, doesn't.
		commits bool
		want    Navigation
		after   bool
	}{
		{"the clicked link to the sign-in page", tenant, true, true, NavigateInView, true},
		{"a page of its own", "https://evil.us.auth0.com/u/login/identifier", false, true, NavigateInView, true},
		{"another sign-in page", "https://accounts.google.com/o/oauth2/auth", false, true, NavigateInView, true},
		{"its page in the page", "about:blank", false, true, NavigateInView, true},
		{"a redirect to another site", "https://phish.example/", false, false, NavigateInBrowser, true},
		{"the item's site, replaced at once", "https://app.slack.com/", false, false, NavigateInView, true},
		{"then another site", "https://phish.example/", false, false, NavigateInBrowser, true},
		{"a redirect through the item's site", "https://app.slack.com/sso/callback", false, false, NavigateInView, true},
		{"then the item's page", "https://app.slack.com/client", false, true, NavigateInView, false},
		{"then a redirect to another site", "https://example.com/", false, true, NavigateInView, false},
	}
	signInByClick := false
	for _, step := range steps {
		got := DecideNavigation(slack, step.target, step.clicked, signInByClick)
		if got != step.want {
			t.Errorf("%s: DecideNavigation(%q, %v, %v) = %v, want %v", step.name, step.target, step.clicked, signInByClick, got, step.want)
		}
		if got == NavigateInView {
			signInByClick = SignInByClick(slack, step.target, step.clicked, false, signInByClick)
			if step.commits {
				signInByClick = SignInByClick(slack, step.target, false, true, signInByClick)
			}
		}
		if signInByClick != step.after {
			t.Errorf("%s: signInByClick = %v, want %v", step.name, signInByClick, step.after)
		}
	}
}

func TestSignInByClickStarts(t *testing.T) {
	tests := []struct {
		name      string
		item      string
		target    string
		clicked   bool
		committed bool
		before    bool
		want      bool
	}{
		{"a clicked link to a sign-in page", slack, "https://mycompany.okta.com/app", true, false, false, true},
		{"a redirect to a sign-in page", slack, "https://mycompany.okta.com/app", false, false, false, false},
		{"a clicked link on the item's site", slack, "https://app.slack.com/client", true, false, false, false},
		{"a clicked link to the item's own sign-in", "https://github.com/notifications", "https://github.com/login", true, false, false, false},
		{"an item without an address", "", "https://mycompany.okta.com/app", true, false, false, false},
		{"a sign-in page coming into view", slack, "https://mycompany.okta.com/app", false, true, false, false},
		{"the item's site, decided", slack, "https://app.slack.com/client", false, false, true, true},
		{"the item's site, in view", slack, "https://app.slack.com/client", false, true, true, false},
		{"another site, in view", slack, "https://example.com/", false, true, true, true},
		{"the page's own content, in view", slack, "about:blank", false, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SignInByClick(tt.item, tt.target, tt.clicked, tt.committed, tt.before); got != tt.want {
				t.Errorf("SignInByClick(%q, %q, %v, %v, %v) = %v, want %v", tt.item, tt.target, tt.clicked, tt.committed, tt.before, got, tt.want)
			}
		})
	}
}

// TestDecidePopupBlankFromFrames checks which frames may open a blank popup,
// which the frame that opens it can fill with anything.
func TestDecidePopupBlankFromFrames(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		origin    string
		mainFrame bool
		want      Popup
	}{
		{"the main frame", "", "https://app.slack.com", true, PopupWindow},
		{"the main frame, about:blank", "about:blank", "https://app.slack.com", true, PopupWindow},
		{"a frame on the item's site", "", "https://files.slack.com", false, PopupWindow},
		{"a frame from another site", "", "https://ads.example", false, PopupCancel},
		{"a frame from another site, about:blank", "about:blank", "https://ads.example", false, PopupCancel},
		{"a frame with an opaque origin", "", "", false, PopupCancel},
		{"a frame on a sign-in site", "", "https://evil.okta.com", false, PopupCancel},
		{"a frame from another site, the item's page", "https://app.slack.com/huddle", "https://ads.example", false, PopupWindow},
		{"a frame from another site, another site's page", "https://example.com/", "https://ads.example", false, PopupBrowser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecidePopup(slack, tt.target, tt.origin, tt.mainFrame); got != tt.want {
				t.Errorf("DecidePopup(slack, %q, %q, %v) = %v, want %v", tt.target, tt.origin, tt.mainFrame, got, tt.want)
			}
		})
	}
}

// TestDecidePopupMore covers popups the main table leaves out.
func TestDecidePopupMore(t *testing.T) {
	tests := []struct {
		name   string
		item   string
		target string
		want   Popup
	}{
		{"blank with a fragment", slack, "about:blank#x", PopupWindow},
		{"blob address", slack, "blob:https://app.slack.com/1", PopupCancel},
		{"spaces only", slack, "   ", PopupWindow},
		{"upper-case same site", slack, "HTTPS://Files.Slack.com/x", PopupWindow},
		{"VS Code, same server", vscode, "http://localhost:51234/?folder=/tmp", PopupWindow},
		{"VS Code, another local server", vscode, "http://localhost:3000/", PopupBrowser},
		{"stray % on the item's site", slack, "https://app.slack.com/sale/50%off", PopupWindow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecidePopup(tt.item, tt.target, "", true); got != tt.want {
				t.Errorf("DecidePopup(%q, %q) = %v, want %v", tt.item, tt.target, got, tt.want)
			}
		})
	}
}

func TestDecidePopupNavigation(t *testing.T) {
	const (
		safeLinks = "https://nam12.safelinks.protection.outlook.com/?url=https%3A%2F%2Fexample.com"
		googleURL = "https://www.google.com/url?q=https://example.com"
		gmail     = "https://mail.google.com/mail/u/0/"
	)
	tests := []struct {
		name    string
		item    string
		start   string
		target  string
		clicked bool
		want    Navigation
	}{
		{"redirector's own page", outlook, safeLinks, safeLinks, false, NavigateInView},
		{"Safe Links redirect to another site", outlook, safeLinks, "https://example.com/article", false, NavigateInBrowser},
		{"Google redirect to another site", gmail, googleURL, "https://example.com/", false, NavigateInBrowser},
		{"blank popup sent to another site", slack, "", "https://evil.example/fake-login", false, NavigateInBrowser},
		{"blank popup sent to a look-alike sign-in", slack, "", "https://login.evil.example/", false, NavigateInBrowser},
		{"blank popup's own blank page", slack, "", "about:blank", false, NavigateInView},
		{"blank popup sent to sign-in", slack, "", "https://accounts.google.com/o/oauth2/v2/auth", false, NavigateInView},
		{"blank popup sent to the item's site", slack, "", "https://app.slack.com/huddle", false, NavigateInView},
		{"sign-in popup through another identity provider", slack, "https://accounts.google.com/o/saml2", "https://idp.corp.example/sso", false, NavigateInView},
		{"sign-in popup back to the item", slack, "https://login.microsoftonline.com/common", "https://app.slack.com/sso/callback", false, NavigateInView},
		{"same-site popup, other site clicked", slack, "https://app.slack.com/huddle", "https://example.com/", true, NavigateInBrowser},
		{"same-site popup, same site clicked", slack, "https://app.slack.com/huddle", "https://files.slack.com/x", true, NavigateInView},
		{"popup, mailto clicked", outlook, "https://outlook.office.com/mail/deeplink", "mailto:a@example.com", true, NavigateInBrowser},
		{"popup, app link clicked", slack, "https://app.slack.com/huddle", "zoommtg://zoom.us/join", true, NavigateAsk},
		{"popup, app link not clicked", slack, "https://app.slack.com/huddle", "zoommtg://zoom.us/join", false, NavigateCancel},
		{"sign-in popup, app link not clicked", slack, "https://accounts.google.com/", "zoommtg://zoom.us/join", false, NavigateCancel},
		{"popup, file", slack, "", "file:///etc/hosts", true, NavigateCancel},
		{"popup, unparseable", slack, "", "https://exa mple.com/%zz", false, NavigateCancel},
		{"popup, stray % on the item's site", slack, "https://app.slack.com/huddle", "https://app.slack.com/sale/50%off", false, NavigateInView},
		{"item without an address", "", "", "https://example.com/", false, NavigateInView},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecidePopupNavigation(tt.item, tt.start, tt.target, tt.clicked); got != tt.want {
				t.Errorf("DecidePopupNavigation(%q, %q, %q, %v) = %v, want %v", tt.item, tt.start, tt.target, tt.clicked, got, tt.want)
			}
		})
	}
}

// TestPopupsOpenedForRedirectorsLeave checks the case the popup policy is
// for: a link on the item's site that redirects elsewhere opens a popup
// (DecidePopup), whose redirect then goes to the default browser.
func TestPopupsOpenedForRedirectorsLeave(t *testing.T) {
	const safeLinks = "https://nam12.safelinks.protection.outlook.com/?url=https%3A%2F%2Fexample.com"
	if got := DecidePopup(outlook, safeLinks, "", true); got != PopupWindow {
		t.Fatalf("DecidePopup(Outlook, Safe Links) = %v, want PopupWindow", got)
	}
	if got := DecidePopupNavigation(outlook, safeLinks, "https://example.com/", false); got != NavigateInBrowser {
		t.Errorf("the redirect = %v, want NavigateInBrowser", got)
	}
}

func TestAppSchemes(t *testing.T) {
	tests := []struct {
		scheme string
		want   appScheme
	}{
		{"mailto", appSchemeOpen},
		{"tel", appSchemeOpen},
		{"file", appSchemeBlock},
		{"smb", appSchemeBlock},
		{"afp", appSchemeBlock},
		{"nfs", appSchemeBlock},
		{"ftp", appSchemeBlock},
		{"vnc", appSchemeBlock},
		{"zoommtg", appSchemeAsk},
		{"vscode", appSchemeAsk},
		{"ssh", appSchemeAsk},
		{"x-apple.systempreferences", appSchemeAsk},
	}
	for _, tt := range tests {
		if got := appSchemeFor(tt.scheme); got != tt.want {
			t.Errorf("appSchemeFor(%q) = %v, want %v", tt.scheme, got, tt.want)
		}
	}
}

func TestNotificationAllowed(t *testing.T) {
	tests := []struct {
		name      string
		item      string
		origin    string
		mainFrame bool
		want      bool
	}{
		{"the item's page", slack, "https://app.slack.com", true, true},
		{"the item's site, another host", slack, "https://files.slack.com", true, true},
		{"a related site", outlook, "https://outlook.office365.com", true, true},
		{"VS Code's server", vscode, "http://localhost:51234", true, true},
		{"VS Code, another local server", vscode, "http://localhost:3000", true, false},
		{"an embedded frame", slack, "https://app.slack.com", false, false},
		{"another site", outlook, "https://evil.example", true, false},
		{"another site's frame", slack, "https://ads.example", false, false},
		{"no origin", slack, "", true, false},
		{"an item without an address", "", "https://app.slack.com", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notificationAllowed(tt.item, tt.origin, tt.mainFrame); got != tt.want {
				t.Errorf("notificationAllowed(%q, %q, %v) = %v, want %v", tt.item, tt.origin, tt.mainFrame, got, tt.want)
			}
		})
	}
}

func TestPageInfo(t *testing.T) {
	vscodeHome := "http://127.0.0.1:51234/?tkn=abc&folder=/Users/me/code"
	tests := []struct {
		name   string
		item   string
		page   string
		secure bool
		want   platform.Page
	}{
		{"the item's page", slack, "https://app.slack.com/client/T1/C2", true,
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true}},
		{"the item's site, another host", slack, "https://files.slack.com/files-pri/T1/x.png", true,
			platform.Page{Host: "files.slack.com", Site: "slack.com", Secure: true}},
		{"a related site", outlook, "https://outlook.office365.com/mail/", true,
			platform.Page{Host: "outlook.office365.com", Site: "office365.com", Secure: true}},
		{"another site", slack, "https://evil.example/slack", true,
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true}},
		{"a sign-in page", outlook, "https://login.microsoftonline.com/common/oauth2/authorize", true,
			platform.Page{Host: "login.microsoftonline.com", Site: "microsoftonline.com", Secure: true, OffSite: true}},
		{"anyone's tenant of a sign-in site", slack, "https://evil.okta.com/app/slack/sso/saml", true,
			platform.Page{Host: "evil.okta.com", Site: "okta.com", Secure: true, OffSite: true}},
		{"a two-part suffix", "https://www.bbc.co.uk/news", "https://sport.bbc.co.uk/football", true,
			platform.Page{Host: "sport.bbc.co.uk", Site: "bbc.co.uk", Secure: true}},
		{"another customer of a shared domain", "https://alice.github.io/", "https://bob.github.io/", true,
			platform.Page{Host: "bob.github.io", Site: "bob.github.io", Secure: true, OffSite: true}},
		{"a user name that looks like the item's host", slack, "https://app.slack.com@evil.example/client", true,
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true}},
		{"a user name and password", slack, "https://app.slack.com:x@evil.example/", true,
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true}},
		{"upper case and a trailing dot", slack, "https://APP.Slack.com./client", true,
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true}},
		{"a port", slack, "https://app.slack.com:8443/client", true,
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true}},
		{"plain http", slack, "http://slack.com/", true,
			platform.Page{Host: "slack.com", Site: "slack.com"}},
		{"plain http on another site", slack, "http://example.com/", true,
			platform.Page{Host: "example.com", Site: "example.com", OffSite: true}},
		{"https with content that isn't secure", slack, "https://app.slack.com/client", false,
			platform.Page{Host: "app.slack.com", Site: "slack.com"}},
		{"an IP address", slack, "https://203.0.113.7/login", true,
			platform.Page{Host: "203.0.113.7", Site: "203.0.113.7", Secure: true, OffSite: true}},
		{"an IP address over http", slack, "http://203.0.113.7:8080/", false,
			platform.Page{Host: "203.0.113.7", Site: "203.0.113.7", OffSite: true}},
		{"an IPv6 address", slack, "https://[2001:db8::1]/", true,
			platform.Page{Host: "2001:db8::1", Site: "2001:db8::1", Secure: true, OffSite: true}},
		{"VS Code's server", vscodeHome, "http://127.0.0.1:51234/?folder=/tmp", false,
			platform.Page{Host: "127.0.0.1", Site: "127.0.0.1", Secure: true}},
		{"VS Code, another local server", vscodeHome, "http://127.0.0.1:3000/", false,
			platform.Page{Host: "127.0.0.1", Site: "127.0.0.1", Secure: true, OffSite: true}},
		{"localhost", vscode, "http://localhost:51234/", false,
			platform.Page{Host: "localhost", Site: "localhost", Secure: true}},
		{"a name under localhost", slack, "http://app.localhost:3000/", false,
			platform.Page{Host: "app.localhost", Site: "app.localhost", Secure: true, OffSite: true}},
		{"IPv6 loopback", slack, "http://[::1]:8080/", false,
			platform.Page{Host: "::1", Site: "::1", Secure: true, OffSite: true}},
		{"a name that only starts like localhost", slack, "http://localhost.evil.example/", false,
			platform.Page{Host: "localhost.evil.example", Site: "evil.example", OffSite: true}},
		// A blob: page is its origin's, as browsers show it.
		{"a blob: page on the item's site", slack, "blob:https://app.slack.com/1234", true,
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true}},
		{"a blob: page on another site", slack, "blob:https://evil.us.auth0.com/6f1c2a9e-0b7d-4d3e-9a51-2c8f4e7b1d60", true,
			platform.Page{Host: "evil.us.auth0.com", Site: "auth0.com", Secure: true, OffSite: true}},
		{"a blob: page with content that isn't secure", slack, "blob:https://app.slack.com/1234", false,
			platform.Page{Host: "app.slack.com", Site: "slack.com"}},
		{"a blob: page over http, on the item's site", slack, "blob:http://slack.com/1234", true,
			platform.Page{Host: "slack.com", Site: "slack.com"}},
		{"a blob: page over http, on another site", slack, "blob:http://evil.example/1234", true,
			platform.Page{Host: "evil.example", Site: "evil.example", OffSite: true}},
		{"a blob: page on the Mac itself", vscode, "blob:http://localhost:51234/1234", false,
			platform.Page{Host: "localhost", Site: "localhost", Secure: true}},
		{"a blob: page with a user name", slack, "blob:https://app.slack.com@evil.example/1234", true,
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true}},
		{"an upper-case blob: page", slack, "BLOB:HTTPS://Evil.Example/1234", true,
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true}},
		// A page with no web address is on another site, with no host.
		{"about:blank", slack, "about:blank", true, platform.Page{OffSite: true, Secure: true}},
		{"about:blank, not secure", slack, "about:blank", false, platform.Page{OffSite: true}},
		{"about:srcdoc", slack, "about:srcdoc", false, platform.Page{OffSite: true}},
		{"a data: page", slack, "data:text/html,<p>https://app.slack.com</p>", true, platform.Page{OffSite: true, Secure: true}},
		{"a data: page, not secure", slack, "data:text/html,hi", false, platform.Page{OffSite: true}},
		{"a blob: page with an opaque origin", slack, "blob:null/1234", true, platform.Page{OffSite: true, Secure: true}},
		{"a blob: page of a data: page", slack, "blob:data:text/html,hi", false, platform.Page{OffSite: true}},
		{"a blob: page with no host", slack, "blob:https:///1234", false, platform.Page{OffSite: true}},
		{"a blob: page with nothing inside", slack, "blob:", false, platform.Page{OffSite: true}},
		{"a blob: page inside a blob: page", slack, "blob:blob:https://app.slack.com/1234", true, platform.Page{OffSite: true, Secure: true}},
		{"an item without an address, about:blank", "", "about:blank", false, platform.Page{OffSite: true}},
		{"an unparseable address", slack, "https://exa mple.com/%zz", true, platform.Page{OffSite: true, Secure: true}},
		{"no address", slack, "", true, platform.Page{}},
		{"spaces only", slack, "  ", true, platform.Page{}},
		{"an item without an address", "", "https://app.slack.com/client", true,
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PageInfo(tt.item, tt.page, tt.secure); got != tt.want {
				t.Errorf("PageInfo(%q, %q, %v) = %+v, want %+v", tt.item, tt.page, tt.secure, got, tt.want)
			}
		})
	}
}

func TestPageSubtitle(t *testing.T) {
	tests := []struct {
		page   string
		secure bool
		want   string
	}{
		{"https://app.slack.com/client", true, "app.slack.com"},
		{"https://app.slack.com/client", false, "Not secure — app.slack.com"},
		{"http://example.com/", true, "Not secure — example.com"},
		{"https://app.slack.com@evil.example/", true, "evil.example"},
		{"http://127.0.0.1:51234/", false, "127.0.0.1"},
		{"blob:https://evil.us.auth0.com/1234", true, "evil.us.auth0.com"},
		{"blob:http://evil.example/1234", true, "Not secure — evil.example"},
		{"about:blank", true, "Page with no address"},
		{"about:blank", false, "Not secure — page with no address"},
		{"data:text/html,hi", false, "Not secure — page with no address"},
		{"blob:null/1234", true, "Page with no address"},
		{"", true, ""},
	}
	for _, tt := range tests {
		if got := pageSubtitle(PageInfo("", tt.page, tt.secure)); got != tt.want {
			t.Errorf("pageSubtitle(%q, %v) = %q, want %q", tt.page, tt.secure, got, tt.want)
		}
	}
}
