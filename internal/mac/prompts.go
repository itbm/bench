package mac

import (
	"sync"
	"time"
	"unicode"
)

// Limits on the text of a page's alert or confirmation (see dialogText).
// WebKit passes the page's text on whole, and AppKit lays all of it out on
// the main thread, so a page could freeze Bench with megabytes of it.
const (
	// maxDialogText is the most characters (Unicode code points) the
	// dialog shows.
	maxDialogText = 2000
	// maxDialogRead is the most UTF-16 code units of the page's text that
	// the native side reads: room for maxDialogText characters, each with
	// a few combining marks.
	maxDialogRead = 16 * maxDialogText
	// maxMarks is the most combining marks, or other characters that
	// extend the one before (see extendsCharacter), that the text keeps in
	// a row. Real text uses a few at most, and stacked marks are drawn
	// outside the dialog.
	maxMarks = 8
	// maxCharacter is the most code points that dialogText looks back
	// through for the start of the character that its limit falls in.
	maxCharacter = 32
)

// Characters that say whether those either side of them join into one.
const (
	zeroWidthJoiner    = '\u200D'
	zeroWidthNonJoiner = '\u200C'
)

// dialogText makes the text of a page's alert or confirmation fit to show:
// at most maxDialogText characters, keeping at most maxMarks combining
// marks in a row. Where it cuts the text short, it cuts before the
// character that the limit falls in, as AppKit draws characters: a letter
// and its marks, an emoji sequence joined with zero-width joiners, or a
// flag. A character that starts at the beginning of the text, or more
// than maxCharacter code points before the limit, is cut at the limit
// itself, so that the text can't be longer however it's made. Bytes that
// aren't UTF-8 become U+FFFD.
func dialogText(s string) string {
	kept := make([]rune, 0, min(len(s), maxDialogText+1))
	run := 0
	for _, r := range s {
		if extendsCharacter(r) {
			run++
			if run > maxMarks {
				continue
			}
		} else {
			run = 0
		}
		kept = append(kept, r)
		if len(kept) > maxDialogText {
			break
		}
	}
	if len(kept) <= maxDialogText {
		return string(kept)
	}
	cut := characterStart(kept, maxDialogText)
	if cut <= 0 {
		cut = maxDialogText
	}
	return string(kept[:cut])
}

// characterStart returns where the character that holds text[i] starts
// (see dialogText), or -1 if that's more than maxCharacter code points
// before i.
func characterStart(text []rune, i int) int {
	start := i
	for start > 0 && joinsPrevious(text, start) {
		if i-start >= maxCharacter {
			return -1
		}
		start--
	}
	return start
}

// joinsPrevious reports whether text[i] is part of the same character as
// text[i-1]: a mark or another character that extends the one before, a
// character after a zero-width joiner, or the second of a flag's two
// regional indicators, which pair up from the first in a row.
func joinsPrevious(text []rune, i int) bool {
	prev, r := text[i-1], text[i]
	if extendsCharacter(r) || prev == zeroWidthJoiner {
		return true
	}
	if !isRegionalIndicator(prev) || !isRegionalIndicator(r) {
		return false
	}
	n := 0
	for j := i - 1; j >= 0 && isRegionalIndicator(text[j]); j-- {
		n++
	}
	return n%2 == 1
}

// extendsCharacter reports whether a character is drawn as part of the one
// before it: a combining mark, a zero-width joiner or non-joiner, an emoji
// skin tone, a tag (as in the flags of England, Scotland, and Wales), or a
// half-width katakana sound mark.
func extendsCharacter(r rune) bool {
	switch {
	case unicode.Is(unicode.M, r):
		return true
	case r == zeroWidthJoiner, r == zeroWidthNonJoiner:
		return true
	case 0x1F3FB <= r && r <= 0x1F3FF, 0xE0020 <= r && r <= 0xE007F, r == 0xFF9E, r == 0xFF9F:
		return true
	}
	return false
}

// isRegionalIndicator reports whether r is a regional indicator, two of
// which make a flag.
func isRegionalIndicator(r rune) bool {
	return 0x1F1E6 <= r && r <= 0x1F1FF
}

// asker names the frame that asks for a prompt (see promptSource).
type asker struct {
	// host is the frame's host, or "" for a page with no web address.
	host string
	// embedded is whether the frame is inside a page on another site, and
	// page that page's host, or "" for a page with no web address.
	embedded bool
	page     string
}

// String names the frame: its host, followed, for a frame embedded in a
// page on another site, by that page's.
func (a asker) String() string {
	switch {
	case !a.embedded:
		return a.host
	case a.page == "":
		return a.host + ", embedded in a page with no address"
	}
	return a.host + ", embedded in " + a.page
}

// promptSource names the frame that asks for a prompt, in a page whose
// address is pageURL (the page in view). frameOrigin is the frame's
// security origin, such as https://app.slack.com. A frame inside the page
// is named by its own host, and if it's on another site than the page, as
// embedded in the page: only the main frame is named as the page. It
// reports false for a frame inside the page with no host of its own (a
// data: frame, or a sandboxed one), which mustn't be taken for the page
// around it. The main frame is named by the page's host when its origin
// has none, as a page sandboxed by its server has, and has no host for a
// page with no web address.
func promptSource(pageURL, frameOrigin string, mainFrame bool) (asker, bool) {
	page, pageOK := webPage(pageURL)
	frame, frameOK := parseAddress(frameOrigin)
	frameOK = frameOK && isWeb(frame)
	switch {
	case mainFrame && frameOK:
		return asker{host: hostOf(frame)}, true
	case mainFrame && pageOK:
		return asker{host: hostOf(page)}, true
	case mainFrame:
		return asker{}, true
	case !frameOK:
		return asker{}, false
	case !pageOK:
		return asker{host: hostOf(frame), embedded: true}, true
	case sameSite(page, frame):
		return asker{host: hostOf(frame)}, true
	}
	return asker{host: hostOf(frame), embedded: true, page: hostOf(page)}, true
}

// dialogHeading returns the heading of an alert or confirmation that a
// frame brings up in the page at pageURL, which says who's asking (see
// promptSource): such as "app.slack.com says", "ads.example, embedded in
// app.slack.com, says", or "This page says" for a page with no web
// address. It reports false for a frame inside the page with no host of
// its own: its dialog isn't shown, and it gets its answer at once.
func dialogHeading(pageURL, frameOrigin string, mainFrame bool) (string, bool) {
	source, ok := promptSource(pageURL, frameOrigin, mainFrame)
	switch {
	case !ok:
		return "", false
	case source.host == "":
		return "This page says", true
	case source.embedded:
		return source.String() + ", says", true
	}
	return source.host + " says", true
}

// mediaKind is what a page asks to use. The values match bridge_darwin.h's
// BENCH_MEDIA_ constants.
type mediaKind int

const (
	mediaCamera mediaKind = iota
	mediaMicrophone
	mediaCameraAndMicrophone
)

// mediaRequest is a page's request for the camera or microphone, which a
// Web item's web view, or one of its popups, passes on.
type mediaRequest struct {
	// itemURL is the item's own address, and itemName its name.
	itemURL  string
	itemName string
	// pageURL is the address of the page in view in the web view that
	// asks.
	pageURL string
	// frameOrigin is the security origin of the frame that asks, such as
	// https://app.slack.com, and mainFrame whether it's the page itself.
	frameOrigin string
	mainFrame   bool
	// popup is whether the web view is a popup's, not the item's own.
	popup bool
	media mediaKind
}

// mediaPrompt returns what a prompt for a page's request for the camera or
// microphone says: its question, which names the frame that asks (see
// promptSource), and its details, which name the item. It reports false
// for a request that's refused at once, without asking: one for anything
// else, one from a frame with no host of its own, or one from a frame
// inside the page that isn't on the item's site, such as an advert. The
// page itself may ask, wherever it is: the question names its host.
func mediaPrompt(req mediaRequest) (question, detail string, ok bool) {
	var what string
	switch req.media {
	case mediaCamera:
		what = "camera"
	case mediaMicrophone:
		what = "microphone"
	case mediaCameraAndMicrophone:
		what = "camera and microphone"
	default:
		return "", "", false
	}
	if frame, ok := parseAddress(req.frameOrigin); !ok || !isWeb(frame) {
		return "", "", false
	}
	if !req.mainFrame && !onItemSite(req.itemURL, req.frameOrigin) {
		return "", "", false
	}
	source, ok := promptSource(req.pageURL, req.frameOrigin, req.mainFrame)
	if !ok || source.host == "" {
		return "", "", false
	}
	who := source.String()
	if source.embedded {
		who += ","
	}
	question = "Allow " + who + " to use your " + what + "?"
	switch {
	case req.itemName == "":
		detail = "A web page in Bench is asking."
	case req.popup:
		detail = "A window that " + req.itemName + " opened is asking."
	default:
		detail = "The page in " + req.itemName + " is asking."
	}
	return question, detail, true
}

// promptKind is a kind of prompt that a Web item's page brings up. The
// values match bridge_darwin.h's BENCH_PROMPT_ constants.
type promptKind int

const (
	// promptDialog is an alert or a confirmation.
	promptDialog promptKind = iota
	// promptMedia is a request for the camera or microphone.
	promptMedia
	promptKinds
)

// A page that asks for another dialog within promptRepeat of the last
// prompt closing is offered a way to stop them. Once promptBurst of a Web
// item's prompts of one kind have closed within promptBurstWindow, the
// item's pages get their answers to prompts of that kind at once, with
// nothing shown, until they have asked for none for promptHoldQuiet, the
// user takes the item to another page with a click, or Bench loads the
// item's address: a page that brings up prompts in a loop, even one that
// reloads itself each time, can't keep Bench behind them, since each is
// app-modal, and it stays held back for as long as it keeps asking. A page
// that needs the user's answer asks one thing at a time, and seldom more
// than twice in a few seconds, as with a confirmation and then an alert
// that it's done; by the third, the user has been offered to stop them.
// Dialogs and requests for the camera or microphone are held back apart,
// so that a few confirmations can't refuse a video call.
const (
	promptRepeat      = 5 * time.Second
	promptBurst       = 3
	promptBurstWindow = 10 * time.Second
	promptHoldQuiet   = 10 * time.Second
)

// promptGate decides which prompts each Web item's pages may bring up:
// alerts and confirmations, and requests for the camera or microphone. The
// native side shows one of each at a time, and the navigator tells it
// which pages come into view (see committed). Its methods are safe for
// concurrent use.
type promptGate struct {
	mu    sync.Mutex
	items map[string]*promptState
}

// promptState is what a Web item's pages' prompts have done.
type promptState struct {
	// stopped is whether the user stopped the pages' prompts of each kind.
	stopped [promptKinds]bool
	// ended is when the last prompts of each kind closed, at most
	// promptBurst, oldest first, and held whether promptBurst of them
	// closed within promptBurstWindow. For a kind held back, asked is when
	// it started to be, or when a page last asked for one since.
	ended [promptKinds][]float64
	held  [promptKinds]bool
	asked [promptKinds]float64
	// last is when the last prompt of any kind closed, or 0.
	last float64
}

// webPrompts is the process's prompt gate.
var webPrompts promptGate

// state returns an item's state, made afresh if it has none. g.mu is held.
func (g *promptGate) state(itemID string) *promptState {
	if g.items == nil {
		g.items = map[string]*promptState{}
	}
	s := g.items[itemID]
	if s == nil {
		s = &promptState{}
		g.items[itemID] = s
	}
	return s
}

// allow reports whether an item's page may bring up a prompt of a kind
// now, rather than getting its answer at once: unless the user stopped
// them, or the item's prompts of that kind are held back (see promptBurst).
// now is the time, in seconds on the native side's clock. Asking while
// they're held back keeps them held back.
func (g *promptGate) allow(itemID string, kind promptKind, now float64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if kind < 0 || kind >= promptKinds {
		return false
	}
	s := g.state(itemID)
	if s.held[kind] && !within(now-s.asked[kind], promptHoldQuiet) {
		s.held[kind], s.ended[kind] = false, nil
	}
	if s.held[kind] {
		s.asked[kind] = now
	}
	return !s.stopped[kind] && !s.held[kind]
}

// offerStop reports whether a dialog shown now offers to stop the page's
// dialogs: whether the item's last prompt closed within promptRepeat. now
// is the time, in seconds on the native side's clock.
func (g *promptGate) offerStop(itemID string, now float64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.state(itemID)
	return s.last > 0 && within(now-s.last, promptRepeat)
}

// ended notes that an item's prompt of a kind closed, at now, and whether
// the user stopped the pages' prompts of that kind.
func (g *promptGate) ended(itemID string, kind promptKind, stop bool, now float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.state(itemID)
	s.last = now
	if kind < 0 || kind >= promptKinds {
		return
	}
	if stop {
		s.stopped[kind] = true
	}
	ended := append(s.ended[kind], now)
	if len(ended) > promptBurst {
		ended = ended[len(ended)-promptBurst:]
	}
	s.ended[kind] = ended
	if len(ended) == promptBurst && within(now-ended[0], promptBurstWindow) {
		s.held[kind], s.asked[kind] = true, now
	}
}

// committed notes that a page has come into view in an item's web view.
// Bench's own load of the item's address (benchLoad) lets its pages bring
// up prompts again, as they could when it was made. So does a navigation
// the user clicked, except that one to the same site as the page before
// leaves the prompts that the user stopped as they are. A page that reloads
// itself, or goes anywhere without a click, changes nothing.
func (g *promptGate) committed(itemID string, benchLoad, clicked, otherSite bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.state(itemID)
	switch {
	case benchLoad:
		*s = promptState{}
	case clicked:
		stopped := s.stopped
		*s = promptState{}
		if !otherSite {
			s.stopped = stopped
		}
	}
}

// samePageSite reports whether the pages at two addresses are on the same
// site (see webPage): a page with no web address is on none.
func samePageSite(a, b string) bool {
	pa, ok := webPage(a)
	if !ok {
		return false
	}
	pb, ok := webPage(b)
	return ok && sameSite(pa, pb)
}

// forget drops an item's state, when its web view is made or closed.
func (g *promptGate) forget(itemID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.items, itemID)
}
