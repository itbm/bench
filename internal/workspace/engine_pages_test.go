package workspace

import (
	"math"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// pageBody returns the body of the displayed pane that shows an item, in
// content coordinates, as a web view's frame is.
func (h *harness) pageBody(id string) layout.Rect {
	h.t.Helper()
	pane := h.paneOf(id)
	for _, p := range h.view().Display.Panes {
		if p.ID == pane {
			return p.Body
		}
	}
	h.t.Fatalf("%s's pane isn't displayed", id)
	return layout.Rect{}
}

// TestPageZoom checks Zoom in, Zoom out, and Actual size: the steps, the
// toasts, the web view's zoom, the ends, and that state.json keeps it for
// the web view made at the next start.
func TestPageZoom(t *testing.T) {
	h := newHarness(t, setup{})
	steps := []struct {
		step  int
		zoom  int
		scale float64
		toast string
	}{
		{1, 110, 1.1, "Zoomed Slack to 110%."},
		{1, 125, 1.25, "Zoomed Slack to 125%."},
		{-1, 110, 1.1, "Zoomed Slack to 110%."},
		{0, 0, 1, "Slack is at actual size."},
		{-1, 90, 0.9, "Zoomed Slack to 90%."},
		{1, 0, 1, "Slack is at actual size."},
	}
	for _, s := range steps {
		mustOK(t, h.e.ZoomPage("w1", s.step))
		if got := h.item("w1").Zoom; got != s.zoom {
			t.Errorf("after step %d: zoom %d%%, want %d%%", s.step, got, s.zoom)
		}
		if got := h.p.WebViews()["w1"].Zoom; got != s.scale {
			t.Errorf("after step %d: the web view's zoom is %v, want %v", s.step, got, s.scale)
		}
		if got := h.em.lastToast(); got != s.toast {
			t.Errorf("after step %d: toast %q, want %q", s.step, got, s.toast)
		}
	}
	if h.item("w2").Zoom != 0 || h.p.WebViews()["w2"].Zoom != 0 {
		t.Errorf("zooming Slack zoomed Outlook")
	}

	// The ends.
	for range len(zoomLevels) {
		_ = h.e.ZoomPage("w1", 1)
	}
	if got := h.item("w1").Zoom; got != 300 {
		t.Errorf("zoomed all the way in: %d%%, want 300%%", got)
	}
	mustErr(t, h.e.ZoomPage("w1", 1), errZoomedIn.Error())
	for range len(zoomLevels) {
		_ = h.e.ZoomPage("w1", -1)
	}
	if got := h.item("w1").Zoom; got != 50 {
		t.Errorf("zoomed all the way out: %d%%, want 50%%", got)
	}
	mustErr(t, h.e.ZoomPage("w1", -1), errZoomedOut.Error())

	// Only web pages zoom.
	mustErr(t, h.e.ZoomPage("w3", 1), errPageOnly.Error())
	mustErr(t, h.e.ZoomPage("nope", 1), errGone.Error())

	// state.json keeps it, and the view made at the next start has it.
	mustOK(t, h.e.ZoomPage("w1", 0))
	mustOK(t, h.e.ZoomPage("w1", 1))
	mustOK(t, h.e.ZoomPage("w1", 1))
	h.settle()
	h.restart()
	if got := h.item("w1").Zoom; got != 125 {
		t.Errorf("after a restart: zoom %d%%, want 125%%", got)
	}
	if got := h.p.WebViews()["w1"].View.Zoom; got != 1.25 {
		t.Errorf("after a restart: the web view was made with zoom %v, want 1.25", got)
	}
	if got := h.p.WebViews()["w2"].View.Zoom; got != 0 {
		t.Errorf("after a restart: Outlook's web view was made with zoom %v, want 0 (actual size)", got)
	}
}

// TestSavedZoom checks the zooms that state.json can give an item: one of
// zoomLevels, the nearest, or 0 for actual size and for nonsense.
func TestSavedZoom(t *testing.T) {
	for _, tt := range []struct{ in, want float64 }{
		{0, 0}, {1, 0}, {1.02, 0}, {-2, 0}, {math.NaN(), 0}, {math.Inf(1), 0},
		{1.25, 1.25}, {1.3, 1.25}, {0.66, 0.67}, {10, 3}, {0.1, 0.5},
	} {
		if got := savedZoom(tt.in); got != tt.want {
			t.Errorf("savedZoom(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestPageReloadBackForward checks Reload, and Back and Forward, which
// follow what the web view says the page can do.
func TestPageReloadBackForward(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.ReloadPage("w1"))
	if got := h.p.WebViews()["w1"].Reloads; got != 1 {
		t.Errorf("reloads = %d, want 1", got)
	}
	mustErr(t, h.e.ReloadPage("w3"), errPageOnly.Error())
	mustErr(t, h.e.ReloadPage("nope"), errGone.Error())

	// A new page has nowhere to go.
	if it := h.item("w1"); it.CanGoBack || it.CanGoForward {
		t.Errorf("a new page can go back %v, forward %v; want neither", it.CanGoBack, it.CanGoForward)
	}
	mustErr(t, h.e.GoBack("w1"), errNoBack.Error())
	mustErr(t, h.e.GoForward("w1"), errNoForward.Error())

	// The web view says when it can go back.
	mustOK(t, h.p.SetWebState("w1", platform.WebState{CanGoBack: true}))
	h.drain()
	if it := h.item("w1"); !it.CanGoBack || it.CanGoForward {
		t.Errorf("after a link: back %v, forward %v; want back only", it.CanGoBack, it.CanGoForward)
	}
	mustOK(t, h.e.GoBack("w1"))
	if wv := h.p.WebViews()["w1"]; wv.Back != 1 || wv.Forward != 0 {
		t.Errorf("went back %d, forward %d times; want back once", wv.Back, wv.Forward)
	}
	mustOK(t, h.p.SetWebState("w1", platform.WebState{CanGoForward: true}))
	h.drain()
	mustOK(t, h.e.GoForward("w1"))
	if wv := h.p.WebViews()["w1"]; wv.Forward != 1 {
		t.Errorf("went forward %d times, want once", wv.Forward)
	}

	// What it can do goes with the web view.
	mustOK(t, h.p.SetWebState("w1", platform.WebState{CanGoBack: true, Microphone: platform.CaptureActive}))
	h.drain()
	h.e.lock()
	h.e.closeWebViewLocked(h.e.model.Items["w1"])
	h.e.unlock()
	if it := h.item("w1"); it.CanGoBack || it.Microphone != "" {
		t.Errorf("after its web view closed, Slack can go back %v, uses the microphone %q", it.CanGoBack, it.Microphone)
	}
}

// TestFindInPage checks the find bar: the web view moves down to make
// room for it, Bench's UI gets the keyboard for its field, the page finds
// text, and closing it gives the page its room and the keyboard back.
func TestFindInPage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.FocusPane(h.paneOf("w1")))
	body := h.pageBody("w1")
	if got := h.p.WebViews()["w1"].Frame; got != body {
		t.Fatalf("Slack's web view is at %v, want its pane's body %v", got, body)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SetFindBar("w1", true))
	if !h.item("w1").Finding {
		t.Errorf("the find bar didn't open")
	}
	want := layout.Rect{X: body.X, Y: body.Y + layout.FindBarHeight, W: body.W, H: body.H - layout.FindBarHeight}
	if got := h.p.WebViews()["w1"].Frame; got != want {
		t.Errorf("with the find bar open, Slack's web view is at %v, want %v", got, want)
	}
	if len(h.calls("FocusUI")) != 1 {
		t.Errorf("opening the find bar didn't give Bench's UI the keyboard: %v", h.calls("FocusUI", "FocusWebView"))
	}
	if got := h.p.WebViews()["w2"].Frame; got != h.pageBody("w2") {
		t.Errorf("Outlook's web view moved: %v", got)
	}

	// Finding, ignoring case, forwards and backwards.
	mustOK(t, h.p.SetPageText("w1", "Huddle in #general"))
	found, err := h.e.FindInPage("w1", "HUDDLE", false)
	if err != nil || !found {
		t.Errorf("FindInPage(HUDDLE) = %v, %v; want a match", found, err)
	}
	found, err = h.e.FindInPage("w1", "standup", true)
	if err != nil || found {
		t.Errorf("FindInPage(standup) = %v, %v; want no match", found, err)
	}
	if wv := h.p.WebViews()["w1"]; wv.Found != "standup" || !wv.Backwards {
		t.Errorf("the page was asked for %q, backwards %v; want standup, backwards", wv.Found, wv.Backwards)
	}
	// Nothing to find finds nothing, without asking the page.
	h.p.ResetCalls()
	if found, err := h.e.FindInPage("w1", "", false); err != nil || found {
		t.Errorf("FindInPage(\"\") = %v, %v; want nothing", found, err)
	}
	if len(h.calls("FindInWebView")) != 0 {
		t.Errorf("an empty find asked the page")
	}
	if _, err := h.e.FindInPage("w3", "x", false); err == nil || err.Error() != errPageOnly.Error() {
		t.Errorf("FindInPage on an app's window: %v, want %q", err, errPageOnly)
	}

	// Rearranging keeps the room.
	mustOK(t, h.e.Rearrange())
	if got := h.p.WebViews()["w1"].Frame; got != want {
		t.Errorf("after rearranging, Slack's web view is at %v, want %v", got, want)
	}

	// Closing it gives the page its room and the keyboard back.
	h.p.ResetCalls()
	mustOK(t, h.e.SetFindBar("w1", false))
	if h.item("w1").Finding {
		t.Errorf("the find bar didn't close")
	}
	if got := h.p.WebViews()["w1"].Frame; got != body {
		t.Errorf("with the find bar closed, Slack's web view is at %v, want %v", got, body)
	}
	if calls := h.calls("FocusWebView"); len(calls) != 1 || calls[0].Arg != "w1" {
		t.Errorf("closing the find bar gave the keyboard to %v, want Slack's page", calls)
	}

	// A popped-out page has no find bar.
	mustOK(t, h.e.SetFindBar("w1", true))
	mustOK(t, h.e.PopOut("w1"))
	if h.item("w1").Finding {
		t.Errorf("the popped-out page still has its find bar")
	}
	mustErr(t, h.e.SetFindBar("w1", true), errFindOwnWindow.Error())
	mustErr(t, h.e.SetFindBar("w3", true), errPageOnly.Error())
}

// TestCaptureMarks checks the camera and microphone marks, on the sub-tab
// and summed up on the tab, and Mute microphone, Turn off camera, and Stop
// sound and video.
func TestCaptureMarks(t *testing.T) {
	h := newHarness(t, setup{})
	if tv := h.view().Tabs[0]; tv.Camera != 0 || tv.Microphone != 0 || tv.CameraLive || tv.MicrophoneLive {
		t.Errorf("before a call, Comms has %+v", tv)
	}
	mustErr(t, h.e.MuteCapture("w1", "microphone", true), "The page isn't using the microphone.")

	// A call starts.
	mustOK(t, h.p.SetWebState("w1", platform.WebState{Camera: platform.CaptureActive, Microphone: platform.CaptureActive}))
	h.drain()
	if it := h.item("w1"); it.Camera != "active" || it.Microphone != "active" {
		t.Errorf("in a call: camera %q, microphone %q; want both active", it.Camera, it.Microphone)
	}
	if tv := h.view().Tabs[0]; tv.Camera != 1 || tv.Microphone != 1 || !tv.CameraLive || !tv.MicrophoneLive {
		t.Errorf("in a call, Comms has %+v; want camera 1, microphone 1, both live", tv)
	}

	// Muting the microphone leaves the camera live.
	mustOK(t, h.e.MuteCapture("w1", "microphone", true))
	h.drain()
	if got := h.em.lastToast(); got != "Muted Slack's microphone." {
		t.Errorf("toast %q", got)
	}
	if it := h.item("w1"); it.Microphone != "muted" || it.Camera != "active" {
		t.Errorf("muted: camera %q, microphone %q", it.Camera, it.Microphone)
	}
	if tv := h.view().Tabs[0]; !tv.CameraLive || tv.MicrophoneLive {
		t.Errorf("with the camera on and the microphone muted, Comms has %+v", tv)
	}
	mustOK(t, h.e.MuteCapture("w1", "camera", true))
	h.drain()
	if got := h.em.lastToast(); got != "Turned off Slack's camera." {
		t.Errorf("toast %q", got)
	}
	if tv := h.view().Tabs[0]; tv.Camera != 1 || tv.Microphone != 1 || tv.CameraLive || tv.MicrophoneLive {
		t.Errorf("all muted, Comms has %+v; want camera 1, microphone 1, neither live", tv)
	}
	mustOK(t, h.e.MuteCapture("w1", "camera", false))
	h.drain()
	if got := h.em.lastToast(); got != "Turned Slack's camera back on." || h.item("w1").Camera != "active" {
		t.Errorf("camera back on: toast %q, camera %q", got, h.item("w1").Camera)
	}

	mustErr(t, h.e.MuteCapture("w2", "camera", true), "The page isn't using the camera.")
	mustErr(t, h.e.MuteCapture("w1", "speaker", true), errBadDevice.Error())
	mustErr(t, h.e.MuteCapture("w3", "camera", true), errPageOnly.Error())

	// Stop sound and video pauses the page's media, whatever it plays.
	mustOK(t, h.e.StopMedia("w2"))
	if got := h.p.WebViews()["w2"].Paused; got != 1 {
		t.Errorf("Outlook paused %d times, want once", got)
	}
	if got := h.em.lastToast(); got != "Paused Outlook's sound and video." {
		t.Errorf("toast %q", got)
	}
	mustErr(t, h.e.StopMedia("w3"), errPageOnly.Error())

	// The call ends.
	mustOK(t, h.p.SetWebState("w1", platform.WebState{}))
	h.drain()
	if tv := h.view().Tabs[0]; tv.Camera != 0 || tv.Microphone != 0 || tv.CameraLive || tv.MicrophoneLive {
		t.Errorf("after the call, Comms has %+v", tv)
	}
}

// TestPageLink checks Copy page link's address: the page in view, or the
// item's own address before one, and none for VS Code for the Web.
func TestPageLink(t *testing.T) {
	h := newHarness(t, setup{})
	link, err := h.e.PageLink("w1")
	if err != nil || link != "https://app.slack.com/client" {
		t.Errorf("before a page: %q, %v; want the item's address", link, err)
	}
	h.showPage("w1", "https://app.slack.com/client/T1/C2", platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true})
	if link, err := h.e.PageLink("w1"); err != nil || link != "https://app.slack.com/client/T1/C2" {
		t.Errorf("after a page: %q, %v; want the page's address", link, err)
	}
	if _, err := h.e.PageLink("w3"); err == nil || err.Error() != errPageOnly.Error() {
		t.Errorf("an app's window: %v, want %q", err, errPageOnly)
	}

	h = newHarness(t, setup{config: vscodeConfig})
	if _, err := h.e.PageLink("w1"); err == nil || err.Error() != errVSCodeLink.Error() {
		t.Errorf("VS Code for the Web: %v, want %q", err, errVSCodeLink)
	}
}

// TestMenuBarPage checks what Pane ▸ Page is told about the focused pane's
// web page.
func TestMenuBarPage(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.FocusPane(h.paneOf("w1")))
	h.advance(50 * time.Millisecond)
	got := h.lastMenuBar().Pane
	if !got.Page || !got.Link || got.CanGoBack || got.Zoom != 0 || !got.CanZoomIn || !got.CanZoomOut || got.Microphone != "" {
		t.Errorf("Slack's Pane menu: %+v", got)
	}
	mustOK(t, h.p.SetWebState("w1", platform.WebState{CanGoBack: true, Microphone: platform.CaptureMuted}))
	h.drain()
	for range len(zoomLevels) {
		_ = h.e.ZoomPage("w1", 1)
	}
	h.advance(50 * time.Millisecond)
	got = h.lastMenuBar().Pane
	if !got.CanGoBack || got.Zoom != 300 || got.CanZoomIn || !got.CanZoomOut || got.Microphone != "muted" {
		t.Errorf("Slack's Pane menu in a call, zoomed in: %+v", got)
	}

	// An app's window has no Page menu.
	mustOK(t, h.e.SwitchTab(1))
	h.advance(50 * time.Millisecond)
	if got := h.lastMenuBar().Pane; got.Page || got.Link {
		t.Errorf("Code's Pane menu: %+v, want no Page", got)
	}
}
