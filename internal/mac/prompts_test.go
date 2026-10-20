package mac

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// marksInARow returns the longest run of characters in s that extend the
// one before (see extendsCharacter).
func marksInARow(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if extendsCharacter(r) {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

// TestDialogTextAttack checks that a page can't get more than the limit
// into a dialog with one long character, as rangeOfComposedCharacterSequences
// would let it: a letter followed by twenty million combining accents.
func TestDialogTextAttack(t *testing.T) {
	attack := strings.Repeat("x", maxDialogText-1) + "a" + strings.Repeat("\u0301", 20e6)
	start := time.Now()
	got := dialogText(attack)
	if took := time.Since(start); took > time.Second {
		t.Errorf("dialogText took %v", took)
	}
	// The cut falls in the accented a, so it goes before it.
	if want := strings.Repeat("x", maxDialogText-1); got != want {
		t.Errorf("dialogText(attack) has %d characters, ending %q, want %d x's",
			utf8.RuneCountInString(got), got[max(0, len(got)-10):], maxDialogText-1)
	}

	// The accents alone fit, once there are only a few of them.
	got = dialogText("a" + strings.Repeat("\u0301", 1e6) + " b")
	if want := "a" + strings.Repeat("\u0301", maxMarks) + " b"; got != want {
		t.Errorf("an a with a million accents became %d characters, want %d", utf8.RuneCountInString(got), utf8.RuneCountInString(want))
	}

	// Each letter keeps a few of its marks, however many it has.
	zalgo := strings.Repeat("h"+strings.Repeat("\u0336\u0317", 15), 100)
	got = dialogText(zalgo)
	if n, m := strings.Count(got, "h"), marksInARow(got); n != 100 || m != maxMarks {
		t.Errorf("100 letters with 30 marks each became %d letters with up to %d marks, want 100 with %d", n, m, maxMarks)
	}

	// Text that's one character from the start is cut at the limit.
	for name, s := range map[string]string{
		"joined emoji":   "\U0001F468" + strings.Repeat("\u200D\U0001F468", 5000),
		"joined letters": "a" + strings.Repeat("\u200Da\u0301", 5000),
	} {
		got := dialogText(s)
		if n := utf8.RuneCountInString(got); n != maxDialogText {
			t.Errorf("%s: cut to %d characters, want %d", name, n, maxDialogText)
		}
	}
}

func TestDialogText(t *testing.T) {
	long := strings.Repeat("a", maxDialogText)
	family := "\U0001F468\u200D\U0001F469\u200D\U0001F467" // A family: man, woman, girl.
	flag := "\U0001F1EC\U0001F1E7"                         // The UK's flag.
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"short", "Delete this message?", "Delete this message?"},
		{"lines", "One\nTwo", "One\nTwo"},
		{"exactly the limit", long, long},
		{"one over", long + "b", long},
		{"accents kept", "e\u0301te\u0301", "e\u0301te\u0301"},
		{"marks in a row", "a" + strings.Repeat("\u0300", 20) + "b", "a" + strings.Repeat("\u0300", maxMarks) + "b"},
		{"each run counted", "a" + strings.Repeat("\u0300", 9) + "b" + strings.Repeat("\u0301", 9),
			"a" + strings.Repeat("\u0300", maxMarks) + "b" + strings.Repeat("\u0301", maxMarks)},
		{"a skin tone", "\U0001F44B\U0001F3FD", "\U0001F44B\U0001F3FD"},
		{"not UTF-8", "a\xffb", "a\uFFFDb"},

		// At the limit, the cut goes before the character it falls in.
		{"accent at the limit", long[1:] + "e\u0301x", long[1:]},
		{"emoji at the limit", long[3:] + family, long[3:]},
		{"after a joined emoji", long[5:] + family + "x", long[5:] + family},
		{"flag at the limit", long[1:] + flag, long[1:]},
		{"after a flag", long[2:] + flag + "x", long[2:] + flag},
		{"flags paired from the first", long[3:] + flag + flag, long[3:] + flag},
		{"emoji over the limit", strings.Repeat("\U0001F600", maxDialogText+1), strings.Repeat("\U0001F600", maxDialogText)},
	}
	for _, tt := range tests {
		if got := dialogText(tt.in); got != tt.want {
			t.Errorf("%s: dialogText(%q…) = %q…, want %q… (%d characters, want %d)", tt.name,
				clip(tt.in, 12), clip(got, 12), clip(tt.want, 12), utf8.RuneCountInString(got), utf8.RuneCountInString(tt.want))
		}
	}

	// A character longer than maxCharacter is cut at the limit itself.
	chain := strings.Repeat("\U0001F468\u200D", 40)
	in := strings.Repeat("a", maxDialogText-50) + chain + "x"
	got := dialogText(in)
	if n := utf8.RuneCountInString(got); n != maxDialogText {
		t.Errorf("a long joined character: cut to %d characters, want %d", n, maxDialogText)
	}
}

func TestDialogHeading(t *testing.T) {
	const page = "https://app.slack.com/client/T1"
	tests := []struct {
		name        string
		page, frame string
		mainFrame   bool
		want        string
		shown       bool
	}{
		{"the page", page, "https://app.slack.com", true, "app.slack.com says", true},
		{"a page on another site", "https://evil.example/", "https://evil.example", true, "evil.example says", true},
		{"a sandboxed page", page, "", true, "app.slack.com says", true},
		{"a page with no address", "data:text/html,hi", "", true, "This page says", true},
		{"a blob: page", "blob:https://app.slack.com/1234", "", true, "app.slack.com says", true},
		{"a frame on the page's site", page, "https://files.slack.com", false, "files.slack.com says", true},
		{"a frame on another site", page, "https://ads.example:8443", false, "ads.example, embedded in app.slack.com, says", true},
		{"a frame in a page with no address", "about:blank", "https://ads.example", false, "ads.example, embedded in a page with no address, says", true},
		{"a frame on the item's site in another site's page", "https://login.example.com/", "https://app.slack.com", false, "app.slack.com, embedded in login.example.com, says", true},
		{"a data: frame", page, "", false, "", false},
		{"a frame with a data: origin", page, "data:", false, "", false},
	}
	for _, tt := range tests {
		got, shown := dialogHeading(tt.page, tt.frame, tt.mainFrame)
		if got != tt.want || shown != tt.shown {
			t.Errorf("%s: dialogHeading = %q, %v, want %q, %v", tt.name, got, shown, tt.want, tt.shown)
		}
	}
}

func TestMediaPrompt(t *testing.T) {
	const item, page = "https://app.slack.com/client", "https://app.slack.com/client/T1"
	base := mediaRequest{itemURL: item, itemName: "Slack", pageURL: page, frameOrigin: "https://app.slack.com", mainFrame: true, media: mediaCamera}
	tests := []struct {
		name             string
		change           func(*mediaRequest)
		question, detail string
		ok               bool
	}{
		{"the page", func(*mediaRequest) {}, "Allow app.slack.com to use your camera?", "The page in Slack is asking.", true},
		{"the microphone", func(r *mediaRequest) { r.media = mediaMicrophone }, "Allow app.slack.com to use your microphone?", "The page in Slack is asking.", true},
		{"both", func(r *mediaRequest) { r.media = mediaCameraAndMicrophone }, "Allow app.slack.com to use your camera and microphone?", "The page in Slack is asking.", true},
		{"anything else", func(r *mediaRequest) { r.media = 7 }, "", "", false},
		{"a page on another site", func(r *mediaRequest) {
			r.pageURL, r.frameOrigin = "https://evil.example/", "https://evil.example"
		}, "Allow evil.example to use your camera?", "The page in Slack is asking.", true},
		{"a frame on the item's site", func(r *mediaRequest) {
			r.frameOrigin, r.mainFrame = "https://huddles.slack.com", false
		}, "Allow huddles.slack.com to use your camera?", "The page in Slack is asking.", true},
		{"a frame on the item's site in another site's page", func(r *mediaRequest) {
			r.pageURL, r.frameOrigin, r.mainFrame = "https://login.example.com/", "https://app.slack.com", false
		}, "Allow app.slack.com, embedded in login.example.com, to use your camera?", "The page in Slack is asking.", true},
		{"a frame on another site", func(r *mediaRequest) {
			r.frameOrigin, r.mainFrame = "https://ads.example", false
		}, "", "", false},
		{"a frame with no host", func(r *mediaRequest) { r.frameOrigin, r.mainFrame = "", false }, "", "", false},
		{"a page with no host", func(r *mediaRequest) { r.pageURL, r.frameOrigin = "data:text/html,hi", "" }, "", "", false},
		{"a popup", func(r *mediaRequest) {
			r.popup, r.pageURL, r.frameOrigin = true, "https://login.microsoftonline.com/x", "https://login.microsoftonline.com"
		}, "Allow login.microsoftonline.com to use your camera?", "A window that Slack opened is asking.", true},
		{"no name", func(r *mediaRequest) { r.itemName = "" }, "Allow app.slack.com to use your camera?", "A web page in Bench is asking.", true},
	}
	for _, tt := range tests {
		req := base
		tt.change(&req)
		question, detail, ok := mediaPrompt(req)
		if question != tt.question || detail != tt.detail || ok != tt.ok {
			t.Errorf("%s: mediaPrompt = %q, %q, %v, want %q, %q, %v", tt.name, question, detail, ok, tt.question, tt.detail, tt.ok)
		}
	}
}

func TestPromptGate(t *testing.T) {
	var g promptGate
	const at = 1000.0
	if !g.allow("w1", promptDialog, at+30) || !g.allow("w1", promptMedia, at+30) {
		t.Fatal("a new item's prompts refused")
	}
	if g.offerStop("w1", at) {
		t.Error("the first dialog offers to stop them")
	}
	g.ended("w1", promptDialog, false, at)
	if !g.offerStop("w1", at+4) {
		t.Error("a dialog 4 seconds after the last doesn't offer to stop them")
	}
	if g.offerStop("w1", at+6) {
		t.Error("a dialog 6 seconds after the last offers to stop them")
	}

	// Stopping one kind leaves the other.
	g.ended("w1", promptDialog, true, at+20)
	if g.allow("w1", promptDialog, at+30) {
		t.Error("dialogs allowed after the user stopped them")
	}
	if !g.allow("w1", promptMedia, at+30) {
		t.Error("stopping dialogs stopped camera requests")
	}
	if g.allow("w1", promptKinds, at+30) || g.allow("w1", -1, at+30) {
		t.Error("allowed a prompt of no kind")
	}

	// A page that reloads itself, or goes anywhere without a click, keeps
	// them stopped; so does a click to another page on its site.
	g.committed("w1", false, false, false)
	g.committed("w1", false, false, true)
	g.committed("w1", false, true, false)
	if g.allow("w1", promptDialog, at+30) {
		t.Error("dialogs allowed again by a page that came into view without a click to another site")
	}
	// A click to another site, or Bench's own load, lets them ask again.
	g.committed("w1", false, true, true)
	if !g.allow("w1", promptDialog, at+30) {
		t.Error("dialogs still stopped after a click to another site")
	}
	g.ended("w2", promptMedia, true, at)
	g.committed("w2", true, false, false)
	if !g.allow("w2", promptMedia, at+30) {
		t.Error("camera requests still stopped after Bench loaded the item's address")
	}

	// Another item has prompts of its own.
	g.ended("w3", promptDialog, true, at)
	if !g.allow("w4", promptDialog, at+30) {
		t.Error("stopping one item's dialogs stopped another's")
	}
	g.forget("w3")
	if !g.allow("w3", promptDialog, at+30) {
		t.Error("a forgotten item's dialogs still stopped")
	}
}

// TestPromptBurst checks that once promptBurst prompts of a kind have
// closed within promptBurstWindow, the item's pages get their answers to
// prompts of that kind at once, while they keep asking, until they've
// asked for none for promptHoldQuiet, the user clicks to another page, or
// Bench loads the item's address.
func TestPromptBurst(t *testing.T) {
	var g promptGate
	const at = 1000.0
	// Prompts spread out are all shown.
	gap := promptBurstWindow.Seconds()/(promptBurst-1) + 0.1
	for i := range 10 {
		g.ended("w1", promptDialog, false, at+float64(i)*gap)
		if !g.allow("w1", promptDialog, at+float64(i)*gap+0.1) {
			t.Fatalf("prompt %d held back, coming one every %.1f seconds", i+2, gap)
		}
	}

	// A page that asks again as soon as each closes is held back by the
	// third of a kind, and only for that kind.
	next := at + 100
	g.ended("w2", promptDialog, false, next)
	g.ended("w2", promptMedia, false, next+1)
	g.ended("w2", promptDialog, false, next+3)
	if !g.allow("w2", promptDialog, next+3) || !g.allow("w2", promptMedia, next+3) {
		t.Fatal("held back after two prompts of a kind")
	}
	next += promptBurstWindow.Seconds()
	g.ended("w2", promptDialog, false, next)
	if g.allow("w2", promptDialog, next) {
		t.Error("dialogs allowed after three within the window")
	}
	if !g.allow("w2", promptMedia, next) {
		t.Error("three dialogs held back camera requests")
	}
	// For as long as the page keeps asking, however long that is, or
	// wherever it goes by itself.
	for range 100 {
		next += promptHoldQuiet.Seconds() - 1
		if g.allow("w2", promptDialog, next) {
			t.Fatal("dialogs allowed to a page that keeps asking")
		}
	}
	g.committed("w2", false, false, true)
	if g.allow("w2", promptDialog, next) {
		t.Error("dialogs allowed again after a page came into view without a click")
	}
	// Once it has asked for none for a while, it may ask again.
	next += promptHoldQuiet.Seconds() + 1
	if !g.allow("w2", promptDialog, next) {
		t.Errorf("dialogs still held back %v after the page last asked", promptHoldQuiet+time.Second)
	}
	// And it starts counting afresh.
	g.ended("w2", promptDialog, false, next+1)
	g.ended("w2", promptDialog, false, next+2)
	if !g.allow("w2", promptDialog, next+2) {
		t.Error("held back after two dialogs since it was let go")
	}
	g.ended("w2", promptDialog, false, next+3)
	if g.allow("w2", promptDialog, next+3) {
		t.Error("dialogs allowed after three more within the window")
	}
	// A click to another page, on the same site, lets them come again.
	g.committed("w2", false, true, false)
	if !g.allow("w2", promptDialog, next+3) {
		t.Error("dialogs still held back after a click to another page")
	}
	// So does Bench's load of the item's address.
	g.ended("w3", promptMedia, false, next)
	g.ended("w3", promptMedia, false, next+1)
	g.ended("w3", promptMedia, false, next+2)
	if g.allow("w3", promptMedia, next+2) {
		t.Fatal("camera requests allowed after three within the window")
	}
	g.committed("w3", true, false, false)
	if !g.allow("w3", promptMedia, next+2) {
		t.Error("camera requests still held back after Bench loaded the item's address")
	}
	// What the user stopped stays stopped, held back or not.
	g.ended("w4", promptDialog, false, next)
	g.ended("w4", promptDialog, true, next+1)
	g.ended("w4", promptDialog, false, next+2)
	if g.allow("w4", promptDialog, next+60) {
		t.Error("dialogs that the user stopped allowed once no longer held back")
	}
}
