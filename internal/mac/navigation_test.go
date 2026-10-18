package mac

import (
	"net/url"
	"strings"
	"testing"
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
