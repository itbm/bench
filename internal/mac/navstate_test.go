package mac

import (
	"fmt"
	"testing"
)

// navTest drives a navigator as a Web item's web view would, on a clock of
// its own.
type navTest struct {
	t    *testing.T
	n    *navigator
	item string
	// now is the time, event the user's last mouse-up or key-down in the web
	// view (0 for none), and inView the page in view.
	now    float64
	event  float64
	inView string
	lastID uint64
	// opened holds what went to the browser or another app, and home
	// whether the navigator asked for the item's address to load again
	// since it was last cleared.
	opened []string
	home   bool
}

func newNavTest(t *testing.T, item string) *navTest {
	t.Helper()
	h := &navTest{t: t, n: &navigator{opens: &gestureLimiter{perEvent: opensPerEvent, window: openWindow}, prompts: &promptGate{}}, item: item, now: 1000}
	h.commit(h.loadHome(), item)
	return h
}

func (h *navTest) wait(seconds float64) { h.now += seconds }

// click is the user clicking in the page.
func (h *navTest) click() { h.event = h.now }

// ask asks about a navigation of the main frame.
func (h *navTest) ask(target string, kind navKind, from navSource, origin string) Navigation {
	h.t.Helper()
	return h.decide(mainAction{target: target, kind: kind, from: from, sourceOrigin: origin})
}

// redirect asks about a redirect of navigation nav to target, as WebKit
// does where it says which navigation the redirect is for, from no frame
// or the page in view's.
func (h *navTest) redirect(nav uint64, target string, from navSource) Navigation {
	h.t.Helper()
	a := mainAction{target: target, kind: navOther, from: from, navigation: nav}
	if from == fromPage {
		a.sourceOrigin = originOf(h.inView)
	}
	return h.decide(a)
}

// decide asks about a navigation of the main frame now, with the user's
// last event.
func (h *navTest) decide(a mainAction) Navigation {
	h.t.Helper()
	a.userEvent, a.now = h.event, h.now
	d, home := h.n.decide("w1", h.item, a)
	if d == NavigateInBrowser || d == NavigateAsk {
		h.opened = append(h.opened, a.target)
	}
	if home {
		if h.home {
			h.t.Errorf("asked to load the item's address again while it already was")
		}
		h.home = true
	}
	return d
}

// page asks about a navigation that the page in view starts: a link, or a
// script's or a form's.
func (h *navTest) page(target string, kind navKind) Navigation {
	h.t.Helper()
	return h.ask(target, kind, fromPage, originOf(h.inView))
}

func (h *navTest) start() uint64 {
	h.lastID++
	h.n.started("w1", h.lastID)
	return h.lastID
}

func (h *navTest) commit(nav uint64, address string) {
	h.n.committed("w1", h.item, nav, address, h.now)
	h.inView = address
}

func (h *navTest) fail(nav uint64, replaced bool) bool {
	again := h.n.failed("w1", nav, replaced)
	if again {
		h.home = true
	}
	return again
}

// loadHome loads the item's address, as Bench does, and starts it.
func (h *navTest) loadHome() uint64 {
	h.t.Helper()
	h.home = false
	h.lastID++
	nav := h.lastID
	h.n.loadRequested("w1", nav)
	if got := h.ask(h.item, navOther, fromBench, originOf(h.inView)); got != NavigateInView {
		h.t.Fatalf("Bench's load of the item's address = %v, want NavigateInView", got)
	}
	h.n.started("w1", nav)
	return nav
}

// go_ asks about a navigation the page in view starts and, if it stays in
// view, starts it and brings its page into view.
func (h *navTest) go_(target string, kind navKind) Navigation {
	h.t.Helper()
	d := h.page(target, kind)
	if d == NavigateInView {
		h.commit(h.start(), target)
	}
	return d
}

func (h *navTest) want(step string, got, want Navigation) {
	h.t.Helper()
	if got != want {
		h.t.Errorf("%s = %v, want %v", step, got, want)
	}
}

func (h *navTest) wantOpened(n int) {
	h.t.Helper()
	if len(h.opened) != n {
		h.t.Errorf("opened %d things in the browser or other apps (%q), want %d", len(h.opened), h.opened, n)
	}
}

func (h *navTest) signInByClick() bool { return h.n.items["w1"].signInByClick }

// TestNavFramesCantReplaceThePage checks navigations of the item's main
// frame that something other than the item starts: embedded content from
// another site, or a popup's page. They count as clicked, so a page on
// another site goes to the browser, which needs the user's event, rather
// than replacing the item's page.
func TestNavFramesCantReplaceThePage(t *testing.T) {
	const embed = "https://embed.example"
	h := newNavTest(t, slack)
	// The user clicked inside the embed once, a while ago, as WebKit needs
	// before a frame from another site may navigate the page.
	h.click()
	h.wait(60)
	h.want("an embed sending the item to a look-alike", h.ask("https://slack-login.evil.example/", navOther, fromFrame, embed), NavigateCancel)
	h.want("a popup's page setting opener.location", h.ask("https://slack-login.evil.example/", navOther, fromWindow, "https://evil.us.auth0.com"), NavigateCancel)
	h.want("a form in an embed posting to the top frame", h.ask("https://evil.example/post", navOther, fromFrame, embed), NavigateCancel)
	h.want("an embed's page of its own making", h.ask("about:blank", navOther, fromFrame, embed), NavigateCancel)
	h.want("an embed's blob: page", h.ask("blob:https://embed.example/1234", navOther, fromFrame, embed), NavigateCancel)
	h.want("an embed opening Mail", h.ask("mailto:a@example.com", navOther, fromFrame, embed), NavigateCancel)
	h.wantOpened(0)

	// Just after the user's click in the page, it goes to the browser,
	// once.
	h.click()
	h.wait(0.5)
	h.want("an embed's navigation just after a click", h.ask("https://example.com/article", navOther, fromFrame, embed), NavigateInBrowser)
	h.want("another from the same click", h.ask("https://example.com/other", navOther, fromFrame, embed), NavigateCancel)
	h.wantOpened(1)

	// It can send the item to the item's own pages, and a redirect from
	// there to another site belongs to its navigation, not the item's.
	h.wait(10)
	h.want("an embed sending the item to the item's redirector", h.ask("https://app.slack.com/redirect?u=x", navOther, fromFrame, embed), NavigateInView)
	nav := h.start()
	h.want("that page's redirect to another site", h.ask("https://phish.example/", navOther, fromFrame, embed), NavigateCancel)
	h.fail(nav, false)

	// A sign-in page that anyone can have stays, as one clicked through to,
	// so it can't then send the item to any site it likes.
	h.want("an embed sending the item to someone's sign-in page", h.ask("https://evil.okta.com/login", navOther, fromFrame, embed), NavigateInView)
	h.commit(h.start(), "https://evil.okta.com/login")
	if !h.signInByClick() {
		t.Fatal("a sign-in page an embed sent the item to isn't taken for one clicked through to")
	}
	h.want("that page sending the user on", h.page("https://phish.example/", navOther), NavigateInBrowser)
	if !h.home {
		t.Error("the item didn't go back to its own address")
	}
	h.wantOpened(2)
}

// TestNavItemsOwnNavigations checks that what works today still works: the
// item's own page going where it likes, and sign-in flows that the item
// starts, through its organisation's own sign-in servers too.
func TestNavItemsOwnNavigations(t *testing.T) {
	t.Run("the item's page", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.want("the item's page going to another site by itself", h.go_("https://example.com/landing", navOther), NavigateInView)
	})
	t.Run("Microsoft, then the organisation's own sign-in server", func(t *testing.T) {
		h := newNavTest(t, outlook)
		home := h.loadHome()
		// WebKit asks about a redirect with the source frame of the
		// navigation it belongs to: the main frame, or none.
		h.want("Outlook's redirect to Microsoft", h.ask("https://login.microsoftonline.com/common/oauth2/authorize", navOther, fromPage, originOf(h.inView)), NavigateInView)
		h.commit(home, "https://login.microsoftonline.com/common/oauth2/authorize")
		h.want("Microsoft sending the user to their organisation's server", h.go_("https://sts.contoso.com/adfs/ls/?wa=wsignin1.0", navOther), NavigateInView)
		h.want("a step of the organisation's own", h.go_("https://sts.contoso.com/adfs/ls/mfa", navOther), NavigateInView)
		h.want("the organisation's server posting back to Microsoft", h.go_("https://login.microsoftonline.com/login.srf", navOther), NavigateInView)
		h.want("Microsoft sending the user back to Outlook", h.go_("https://outlook.office.com/mail/", navOther), NavigateInView)
		h.wantOpened(0)
		if h.signInByClick() {
			t.Error("Outlook's own sign-in is taken for one the user clicked through to")
		}
	})
	t.Run("the item's redirect, with no source frame", func(t *testing.T) {
		h := newNavTest(t, outlook)
		home := h.loadHome()
		h.want("Outlook's redirect to Microsoft", h.ask("https://login.microsoftonline.com/common", navOther, fromNone, ""), NavigateInView)
		h.want("then to the organisation's server", h.ask("https://sts.contoso.com/adfs/ls/", navOther, fromNone, ""), NavigateInView)
		h.commit(home, "https://sts.contoso.com/adfs/ls/")
		h.want("the organisation's own next step", h.go_("https://mfa.contoso.com/", navOther), NavigateInView)
	})
	t.Run("Okta, then another identity provider", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.want("Slack sending the user to their Okta", h.go_("https://mycompany.okta.com/app/slack/sso/saml", navOther), NavigateInView)
		h.want("Okta sending them on to the organisation's server", h.go_("https://idp.mycompany.example/sso", navOther), NavigateInView)
		h.want("and back to Slack", h.go_("https://app.slack.com/sso/saml", navOther), NavigateInView)
		h.wantOpened(0)
	})
	t.Run("GitHub's sign-in, two-factor step and all", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.want("Slack sending the user to GitHub", h.go_("https://github.com/login/oauth/authorize?client_id=x", navOther), NavigateInView)
		h.want("the sign-in form", h.page("https://github.com/session", navOther), NavigateInView)
		nav := h.start()
		h.want("its redirect to the two-factor step", h.page("https://github.com/sessions/two-factor", navOther), NavigateInView)
		h.commit(nav, "https://github.com/sessions/two-factor")
		h.want("the two-factor form", h.go_("https://github.com/sessions/two-factor", navOther), NavigateInView)
		h.want("back to Slack", h.go_("https://app.slack.com/sso/github", navOther), NavigateInView)
		h.wantOpened(0)
	})
	t.Run("a clicked link that redirects to Google and back", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked Sign in with Google", h.page("https://app.slack.com/signin/google", navLink), NavigateInView)
		nav := h.start()
		h.wait(3)
		h.want("its redirect to Google", h.page("https://accounts.google.com/o/oauth2/auth", navLink), NavigateInView)
		h.commit(nav, "https://accounts.google.com/o/oauth2/auth")
		h.wait(20)
		h.want("Google's own next page", h.go_("https://accounts.google.com/signin/challenge", navOther), NavigateInView)
		h.want("Google sending the user back", h.go_("https://app.slack.com/sso/google/callback", navOther), NavigateInView)
		h.wantOpened(0)
		if h.signInByClick() {
			t.Error("the item's page in view is still taken for a sign-in page clicked through to")
		}
	})
	t.Run("a link the user clicks inside an embed, to the top frame", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked link to another site", h.ask("https://example.com/", navLink, fromFrame, "https://embed.example"), NavigateInBrowser)
		h.wantOpened(1)
	})
}

// TestNavSwipeBackToClickedThroughSignIn checks that going back to a sign-in
// page the user clicked through to, after the item went back to its own
// address, takes it for one clicked through to again.
func TestNavSwipeBackToClickedThroughSignIn(t *testing.T) {
	const tenant = "https://evil.okta.com/login"
	for _, kind := range []string{"back", "reload"} {
		t.Run(kind, func(t *testing.T) {
			h := newNavTest(t, slack)
			h.click()
			h.want("the clicked link to someone's sign-in page", h.go_(tenant, navLink), NavigateInView)
			h.wait(30)
			h.want("that page sending the user on", h.page("https://spam.example/", navOther), NavigateInBrowser)
			h.commit(h.loadHome(), "https://app.slack.com/client")
			if h.signInByClick() {
				t.Fatal("the item's own page is taken for a sign-in page clicked through to")
			}
			h.want("going back to the sign-in page", h.page(tenant, navHistory), NavigateInView)
			h.commit(h.start(), tenant)
			if kind == "reload" {
				h.want("reloading it", h.page(tenant, navHistory), NavigateInView)
				h.commit(h.start(), tenant)
			}
			if !h.signInByClick() {
				t.Fatal("the sign-in page that came back isn't taken for one clicked through to")
			}
			h.home = false
			h.want("it sending the user anywhere", h.page("https://phish.example/", navOther), NavigateInBrowser)
			if !h.home {
				t.Error("the item didn't go back to its own address")
			}
		})
	}
	t.Run("back to the item's own page", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.want("the item's page going elsewhere", h.go_("https://app.slack.com/client/T1", navOther), NavigateInView)
		h.want("going back", h.page("https://app.slack.com/client", navHistory), NavigateInView)
		h.commit(h.start(), "https://app.slack.com/client")
		if h.signInByClick() {
			t.Error("going back on the item's site is taken for clicking through")
		}
		h.want("its page going to another site by itself", h.go_("https://example.com/", navOther), NavigateInView)
	})
	t.Run("a redirect away from the page gone back to", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.want("going back", h.page("https://app.slack.com/old", navHistory), NavigateInView)
		nav := h.start()
		h.want("its redirect to another site", h.page("https://other.example/", navHistory), NavigateInView)
		h.commit(nav, "https://other.example/")
		if !h.signInByClick() {
			t.Error("a page on another site that going back brought isn't taken for one clicked through to")
		}
	})
}

// TestNavSlowAndClientRedirects checks that a clicked link's redirects count
// as clicked however long they take, and so does a redirector's page on the
// item's site just after a click brings it into view.
func TestNavSlowAndClientRedirects(t *testing.T) {
	t.Run("a slow server redirect", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked link to the item's redirector", h.page("https://app.slack.com/out?u=phish", navLink), NavigateInView)
		nav := h.start()
		h.wait(10)
		h.want("its redirect to another site, 10 s later", h.page("https://phish.example/", navLink), NavigateInBrowser)
		h.fail(nav, false)
		h.wantOpened(1)
	})
	t.Run("a client-side redirector", func(t *testing.T) {
		const gmail = "https://mail.google.com/mail/u/0/"
		h := newNavTest(t, gmail)
		h.click()
		h.want("a clicked link through Google's /url", h.go_("https://www.google.com/url?q=https://phish.example/", navLink), NavigateInView)
		h.wait(1)
		h.want("its page sending the user on", h.page("https://phish.example/", navOther), NavigateInBrowser)
		h.want("and again", h.page("https://phish.example/2", navOther), NavigateCancel)
		h.want("on the item's site", h.go_("https://mail.google.com/mail/u/0/#inbox", navOther), NavigateInView)
		h.wait(afterClickWindow.Seconds() + 1)
		h.want("then the item's page going to another site, later", h.go_("https://example.com/", navOther), NavigateInView)
		h.wantOpened(1)
	})
	t.Run("two client-side redirectors", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked link to a redirector", h.go_("https://app.slack.com/track?u=1", navLink), NavigateInView)
		h.wait(1)
		h.want("its page sending the user to another", h.go_("https://slack.com/r?u=1", navOther), NavigateInView)
		h.wait(1)
		h.want("that one's page sending them to another site", h.page("https://phish.example/", navOther), NavigateInBrowser)
		h.wantOpened(1)
	})
	t.Run("a page that sends the user elsewhere later", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked link on the item's site", h.go_("https://app.slack.com/client/T1", navLink), NavigateInView)
		h.wait(afterClickWindow.Seconds() + 1)
		h.want("the page going to another site, later", h.page("https://example.com/", navOther), NavigateInView)
	})
	t.Run("a clicked link that redirects to a sign-in page", func(t *testing.T) {
		h := newNavTest(t, slack)
		h.click()
		h.want("a clicked link on the item's site", h.page("https://app.slack.com/sso/start", navLink), NavigateInView)
		nav := h.start()
		h.wait(5)
		h.want("its redirect to someone's sign-in page", h.page("https://evil.okta.com/sso", navLink), NavigateInView)
		h.commit(nav, "https://evil.okta.com/sso")
		if !h.signInByClick() {
			t.Error("a sign-in page a click redirected to isn't taken for one clicked through to")
		}
	})
}

// TestNavSignInPageCantOpenTabsWithoutEnd follows a sign-in page that the
// user clicked through to as it tries to open the browser over and over,
// sending the user on and then to another sign-in page of its own before
// the item's own address comes into view.
func TestNavSignInPageCantOpenTabsWithoutEnd(t *testing.T) {
	const tenant = "https://evil.okta.com/login"
	h := newNavTest(t, slack)
	h.click()
	h.want("the clicked link to someone's sign-in page", h.go_(tenant, navLink), NavigateInView)
	h.wait(30)
	for i := range 10 {
		h.page(fmt.Sprintf("https://spam.example/%d", i), navOther)
		h.want("another of its sign-in pages, before the item's load", h.page(fmt.Sprintf("https://evil.okta.com/p?%d", i), navOther), NavigateCancel)
	}
	h.wantOpened(1)
	if !h.home {
		t.Fatal("the item didn't go back to its own address")
	}
	// The page replaces the item's load with another of its sign-in pages,
	// as fast as it can: the item loads its address again, a few times.
	for i := range maxHomeReloads + 1 {
		home := h.loadHome()
		h.want("its sign-in page while the item's load is under way", h.page(fmt.Sprintf("https://evil.okta.com/q?%d", i), navOther), NavigateInView)
		again := h.fail(home, true)
		if again != (i < maxHomeReloads) {
			t.Errorf("replacement %d: load the item's address again = %v, want %v", i+1, again, i < maxHomeReloads)
		}
		h.commit(h.start(), fmt.Sprintf("https://evil.okta.com/q?%d", i))
		h.page("https://spam.example/again", navOther)
	}
	h.wantOpened(1)
	// Nothing else it does goes anywhere until the user clicks.
	h.want("its next sign-in page", h.page("https://evil.okta.com/r", navOther), NavigateCancel)
	h.want("going back", h.page("https://app.slack.com/client", navHistory), NavigateInView) // The item's own address.
	h.click()
	h.want("the user's click on a link to another site", h.page("https://example.com/", navLink), NavigateInBrowser)
	h.want("the user's click on the item's site", h.go_("https://app.slack.com/client/T1", navLink), NavigateInView)
	h.wait(afterClickWindow.Seconds() + 1)
	h.want("then the item's own page, by itself", h.go_("https://example.com/landing", navOther), NavigateInView)
	h.wantOpened(2)
}

// TestNavReturnFails checks the item going back to its own address after a
// sign-in page sent the user on, when that load fails: the sign-in page
// stays in view, still taken for one clicked through to, and may go only
// where the user clicks, without sending the user to the browser again or
// having the item load its address again.
func TestNavReturnFails(t *testing.T) {
	h := newNavTest(t, slack)
	h.click()
	h.want("the clicked link to someone's sign-in page", h.go_("https://evil.us.auth0.com/u/login", navLink), NavigateInView)
	h.wait(30)
	h.want("it sending the user on", h.page("https://spam.example/", navOther), NavigateInBrowser)
	home := h.loadHome()
	if h.fail(home, false) {
		t.Error("a load that failed by itself was made again at once")
	}
	if !h.signInByClick() {
		t.Fatal("the sign-in page in view isn't taken for one clicked through to")
	}
	for i := range maxHomeReloads + 2 {
		h.want("it sending the user on again", h.page(fmt.Sprintf("https://spam.example/%d", i), navOther), NavigateCancel)
		h.want("its sign-in page meanwhile", h.page(fmt.Sprintf("https://evil.us.auth0.com/u/other?%d", i), navOther), NavigateCancel)
	}
	if h.home {
		t.Error("the sign-in page had the item go back to its own address again")
	}
	h.click()
	h.want("the user's click on the item's site", h.go_("https://app.slack.com/client", navLink), NavigateInView)
	h.wait(afterClickWindow.Seconds() + 1)
	h.want("the item's page going elsewhere by itself", h.go_("https://example.com/", navOther), NavigateInView)
	h.wantOpened(1)
}

// TestNavReturnRedirectsOffSite follows the item going back to its own
// address when that address redirects to its sign-in server on another
// site that isn't a sign-in page Bench knows, as Jira's does to
// id.atlassian.com. Where WebKit says that the redirect is for Bench's
// load, it's kept in view, as it is when nothing was clicked through to.
// Where it doesn't, the redirect is cancelled, as it can't be told from a
// navigation that the sign-in page in view starts, and that page stays,
// going only where the user clicks.
func TestNavReturnRedirectsOffSite(t *testing.T) {
	const (
		item   = "https://mycompany.atlassian.net/jira/your-work"
		sso    = "https://id.atlassian.com/login?continue=x"
		tenant = "https://evil.okta.com/login"
	)
	clickThrough := func(t *testing.T) *navTest {
		t.Helper()
		h := newNavTest(t, item)
		h.click()
		h.want("the clicked link to someone's sign-in page", h.go_(tenant, navLink), NavigateInView)
		h.wait(30)
		h.want("it sending the user on", h.page("https://spam.example/", navOther), NavigateInBrowser)
		if !h.home {
			t.Fatal("the item didn't go back to its own address")
		}
		return h
	}
	for _, from := range []navSource{fromNone, fromPage} {
		t.Run("WebKit says, from "+from.String(), func(t *testing.T) {
			h := clickThrough(t)
			home := h.loadHome()
			h.want("the load's redirect to its sign-in server", h.redirect(home, sso, from), NavigateInView)
			h.commit(home, sso)
			if h.signInByClick() {
				t.Error("the item's own sign-in page is taken for one clicked through to")
			}
			h.want("its sign-in server sending the user back", h.go_(item, navOther), NavigateInView)
			h.wantOpened(1)
		})
	}
	t.Run("WebKit doesn't say", func(t *testing.T) {
		h := clickThrough(t)
		home := h.loadHome()
		h.want("the load's redirect to its sign-in server", h.ask(sso, navOther, fromNone, ""), NavigateCancel)
		if h.fail(home, false) {
			t.Error("a load whose redirect was cancelled was made again at once")
		}
		for i := range 10 {
			h.want("the sign-in page sending the user on again", h.page(fmt.Sprintf("https://spam.example/%d", i), navOther), NavigateCancel)
		}
		if h.home {
			t.Error("the sign-in page had the item load its address again")
		}
		h.wantOpened(1)
		h.click()
		h.want("the user's click on the item's site", h.go_("https://mycompany.atlassian.net/browse/X-1", navLink), NavigateInView)
	})
}

// TestNavOneClickOneOpen checks that one click lets a page open one thing
// in the browser or another app, however many links it then clicks.
func TestNavOneClickOneOpen(t *testing.T) {
	h := newNavTest(t, slack)
	h.click()
	for i := range 2000 {
		h.page(fmt.Sprintf("mailto:x@evil.example?body=%d", i), navLink)
	}
	h.wantOpened(1)
	h.wait(0.1)
	for i := range 100 {
		h.page(fmt.Sprintf("https://evil.example/%d", i), navLink)
		h.page(fmt.Sprintf("zoommtg://zoom.us/join?confno=%d", i), navLink)
	}
	h.wantOpened(1)
	// Each key press allows one more.
	for i := range 3 {
		h.wait(0.2)
		h.click()
		h.page(fmt.Sprintf("https://evil.example/key%d", i), navLink)
		h.page(fmt.Sprintf("https://evil.example/key%d/again", i), navLink)
	}
	h.wantOpened(4)
	// A script's click long after the user's opens nothing.
	h.wait(30)
	h.want("a script's click", h.page("https://example.com/", navLink), NavigateInView)
	h.want("a script's mailto:", h.page("mailto:a@example.com", navLink), NavigateCancel)
}

// TestNavProcessEnded checks that a page's process ending ends the item
// going back to its address, but not what the page in view is.
func TestNavProcessEnded(t *testing.T) {
	const tenant = "https://evil.okta.com/login"
	h := newNavTest(t, slack)
	h.click()
	h.go_(tenant, navLink)
	h.wait(30)
	h.page("https://spam.example/", navOther)
	h.n.processEnded("w1")
	h.want("the reload after the crash", h.ask(tenant, navHistory, fromNone, ""), NavigateInView)
	h.commit(h.start(), tenant)
	h.home = false
	h.want("the sign-in page sending the user on again", h.page("https://spam.example/2", navOther), NavigateCancel)
	if !h.home {
		t.Error("the item didn't go back to its own address")
	}
	h.wantOpened(1)
}

// TestNavCommitWhileLoadRequested checks that a sign-in page that comes into
// view while Bench's load of the item's address hasn't started yet is still
// one the user clicked through to: the native side passes the page's own
// address, not WebKit's URL, which is still the load's.
func TestNavCommitWhileLoadRequested(t *testing.T) {
	h := newNavTest(t, slack)
	h.click()
	h.want("a clicked link to someone's sign-in page", h.page("https://evil.okta.com/a", navLink), NavigateInView)
	nav := h.start()
	h.n.loadRequested("w1", 99) // Asked for, not decided or started.
	h.commit(nav, "https://evil.okta.com/a")
	if !h.signInByClick() {
		t.Error("the sign-in page that came into view isn't taken for one clicked through to")
	}
}

// TestNavForget checks that a web view made afresh for an item starts
// afresh.
func TestNavForget(t *testing.T) {
	h := newNavTest(t, slack)
	h.click()
	h.go_("https://evil.okta.com/login", navLink)
	h.n.forget("w1")
	if _, ok := h.n.items["w1"]; ok {
		t.Fatal("the item's state is still there")
	}
	h.inView = ""
	h.commit(h.loadHome(), slack)
	if h.signInByClick() {
		t.Error("a new web view starts with a sign-in page clicked through to")
	}
}

// TestNavPromptsStayStopped checks that a page whose dialogs the user
// stopped can't bring them back by reloading itself, or by going anywhere
// without a click: only a click to another site, or Bench loading the
// item's address, does.
func TestNavPromptsStayStopped(t *testing.T) {
	h := newNavTest(t, slack)
	g := h.n.prompts
	g.ended("w1", promptDialog, true, h.now)
	stopped := func(step string, want bool) {
		t.Helper()
		if got := !g.allow("w1", promptDialog, h.now); got != want {
			t.Errorf("after %s, dialogs stopped = %v, want %v", step, got, want)
		}
	}
	h.want("the page reloading itself", h.go_(h.inView, navHistory), NavigateInView)
	stopped("a reload", true)
	h.want("the page going to another of its pages", h.go_("https://app.slack.com/client/T2", navOther), NavigateInView)
	stopped("a page that the page went to", true)
	h.click()
	h.want("a clicked link on the item's site", h.go_("https://app.slack.com/client/T3", navLink), NavigateInView)
	stopped("a click to another page on the site", true)
	h.wait(10)
	h.want("the page going to another site", h.go_("https://evil.example/", navOther), NavigateInView)
	stopped("the page taking the item to another site", true)
	h.commit(h.loadHome(), slack)
	stopped("Bench loading the item's address", false)

	g.ended("w1", promptMedia, true, h.now)
	h.click()
	h.want("a clicked link to a sign-in page", h.go_("https://login.microsoftonline.com/common/oauth2", navLink), NavigateInView)
	if !g.allow("w1", promptMedia, h.now) {
		t.Error("camera requests still stopped after a click to another site")
	}
}

// TestNavPromptsHeldBack checks that a page that brings up prompts in a
// loop is held back, reloading or not, for as long as it keeps asking,
// until the user clicks to another page.
func TestNavPromptsHeldBack(t *testing.T) {
	h := newNavTest(t, slack)
	g := h.n.prompts
	for range promptBurst {
		h.wait(1)
		g.ended("w1", promptDialog, false, h.now)
		h.go_(h.inView, navHistory)
	}
	for i := range 60 {
		h.wait(1)
		if g.allow("w1", promptDialog, h.now) {
			t.Fatalf("a page reloading itself after each dialog isn't held back, %d seconds on", i+1)
		}
		h.go_(h.inView, navHistory)
	}
	h.go_("https://app.slack.com/client/T2", navOther)
	if g.allow("w1", promptDialog, h.now) {
		t.Error("the page went to another of its pages and is no longer held back")
	}
	h.click()
	h.go_("https://app.slack.com/client/T3", navLink)
	if !g.allow("w1", promptDialog, h.now) {
		t.Error("still held back after a clicked link")
	}
}

// TestNavMacWebItemsCheck checks what scripts/macos-webitems/run.sh expects
// on a real Mac, with its test server's two sites: https://localhost:8443,
// the items' own, and https://127.0.0.1:9443, another site.
func TestNavMacWebItemsCheck(t *testing.T) {
	const other = "https://127.0.0.1:9443"
	if u, v := mustParse(t, "https://localhost:8443/a/"), mustParse(t, other+"/a/away"); sameSite(u, v) {
		t.Fatal("the test server's two ports are one site")
	}
	t.Run("a cross-site frame setting top.location after a click", func(t *testing.T) {
		h := newNavTest(t, "https://localhost:8443/a/")
		h.click()
		h.wait(0.5)
		h.want("the frame's navigation", h.ask(other+"/a/away", navOther, fromFrame, other), NavigateInBrowser)
		h.wantOpened(1)
	})
	t.Run("a clicked link to a page that calls location.replace", func(t *testing.T) {
		h := newNavTest(t, "https://localhost:8443/b/")
		h.click()
		h.want("the clicked link", h.go_("https://localhost:8443/b/hop", navLink), NavigateInView)
		h.wait(0.2)
		h.want("its page's location.replace", h.page(other+"/b/away", navOther), NavigateInBrowser)
		h.wantOpened(1)
	})
}
