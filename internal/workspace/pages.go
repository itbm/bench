package workspace

import (
	"fmt"
	"math"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has a Web item's page controls, which its menus, the jump
// palette, and Pane ▸ Page offer: Reload, Back and Forward, the page's zoom,
// its link, find in page, and what its page does with the camera,
// microphone, and sound (decision 50).

// zoomLevels are the page zooms that Zoom in and Zoom out step through, as
// browsers' are, from 50% to 300%.
var zoomLevels = []float64{0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3}

// pageZoom returns an item's zoom as a scale: 1 for 0, actual size.
func pageZoom(zoom float64) float64 {
	if zoom <= 0 {
		return 1
	}
	return zoom
}

// zoomPercent returns a zoom in percent, for the View, or 0 for actual
// size, which the View leaves out.
func zoomPercent(zoom float64) int {
	p := int(math.Round(pageZoom(zoom) * 100))
	if p == 100 {
		return 0
	}
	return p
}

// savedZoom returns a zoom that state.json keeps as one Bench can show:
// the nearest of zoomLevels, or 0 for actual size, as for a file that has
// none, or a zoom that isn't a number.
func savedZoom(zoom float64) float64 {
	if zoom <= 0 || math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return 0
	}
	best := 1.0
	for _, l := range zoomLevels {
		if math.Abs(l-zoom) < math.Abs(best-zoom) {
			best = l
		}
	}
	if best == 1 {
		return 0
	}
	return best
}

// zoomStep returns the zoom after Zoom in (step 1), Zoom out (-1), or
// Actual size (0), with 0 for actual size, and reports false at the end of
// zoomLevels, where it would stay as it is.
func zoomStep(zoom float64, step int) (float64, bool) {
	z := pageZoom(zoom)
	next := 0.0
	switch {
	case step == 0:
		return 0, true
	case step > 0:
		for _, l := range zoomLevels {
			if l > z+0.001 {
				next = l
				break
			}
		}
	default:
		for i := len(zoomLevels) - 1; i >= 0; i-- {
			if zoomLevels[i] < z-0.001 {
				next = zoomLevels[i]
				break
			}
		}
	}
	if next == 0 {
		return zoom, false
	}
	if next == 1 {
		next = 0
	}
	return next, true
}

// webFrame returns where a Web item's view goes in a pane's body: all of
// it, or below its find bar while that's open.
func webFrame(it *Item, body layout.Rect) layout.Rect {
	if !it.finding {
		return body
	}
	bar := math.Min(layout.FindBarHeight, body.H)
	return layout.Rect{X: body.X, Y: body.Y + bar, W: body.W, H: body.H - bar}
}

// webViewOf returns a Web item whose view exists, for a page control, or
// why there's none: it has closed, it isn't a Web item, or its view hasn't
// been made yet, as for VS Code for the Web while its server starts.
func (e *Engine) webViewOf(itemID string) (*Item, error) {
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		return nil, errGone
	case it.Kind != KindWeb:
		return nil, errPageOnly
	case !e.webMade[it.ID] || e.opts.Headless:
		return nil, errPageNotOpen
	}
	return it, nil
}

// ReloadPage loads a Web item's page again, as a browser's Reload does: the
// page in view, or the item's own address if no page has come into view.
func (e *Engine) ReloadPage(itemID string) error {
	e.lock()
	defer e.unlock()
	it, err := e.webViewOf(itemID)
	if err != nil {
		return err
	}
	if err := e.p.ReloadWebView(it.ID); err != nil {
		return errPageFailed
	}
	return nil
}

// GoBack goes back to the page a Web item showed before, as a browser's
// Back does, and as a swipe to the right does in the page. It's refused
// while the view says there's none (WebStateChanged).
func (e *Engine) GoBack(itemID string) error {
	return e.navigate(itemID, -1)
}

// GoForward goes forward again, after GoBack.
func (e *Engine) GoForward(itemID string) error {
	return e.navigate(itemID, 1)
}

// navigate goes back (step -1) or forward (1) in a Web item's history.
func (e *Engine) navigate(itemID string, step int) error {
	e.lock()
	defer e.unlock()
	it, err := e.webViewOf(itemID)
	if err != nil {
		return err
	}
	if step < 0 && !it.web.CanGoBack {
		return errNoBack
	}
	if step > 0 && !it.web.CanGoForward {
		return errNoForward
	}
	if err := e.p.NavigateWebView(it.ID, step); err != nil {
		return errPageFailed
	}
	return nil
}

// ZoomPage zooms a Web item's page in (step 1) or out (-1) to the next of
// zoomLevels, or back to actual size (0), and says so in a toast. The item
// keeps its zoom, in state.json too, and its view takes it whenever it's
// made. A page that isn't open yet keeps the zoom for when it is.
func (e *Engine) ZoomPage(itemID string, step int) error {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		return errGone
	case it.Kind != KindWeb:
		return errPageOnly
	}
	zoom, ok := zoomStep(it.Zoom, step)
	if !ok && step > 0 {
		return errZoomedIn
	}
	if !ok {
		return errZoomedOut
	}
	if zoom != it.Zoom {
		it.Zoom = zoom
		if e.webMade[it.ID] && !e.opts.Headless {
			if err := e.p.ZoomWebView(it.ID, pageZoom(zoom)); err != nil {
				e.log.Warn("couldn't zoom a page", "item", it.ID, "error", err)
			}
		}
		e.changed()
	}
	if zoom == 0 {
		e.toastLocked(it.App + " is at actual size.")
	} else {
		e.toastLocked(fmt.Sprintf("Zoomed %s to %d%%.", it.App, zoomPercent(zoom)))
	}
	return nil
}

// PageLink returns the address of the page a Web item shows, for Copy page
// link, which the frontend puts on the clipboard: the address that Open in
// browser opens (see OpenInBrowser). It's refused for VS Code for the Web,
// whose address holds the server's connection token, and for a page with
// no web address.
func (e *Engine) PageLink(itemID string) (string, error) {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		return "", errGone
	case it.Kind != KindWeb:
		return "", errPageOnly
	case it.VSCode != "":
		return "", errVSCodeLink
	}
	address := browserAddress(it.page)
	if address == "" && it.page == "" {
		address, _ = e.webAddressLocked(it)
		address = browserAddress(address)
	}
	if address == "" {
		return "", errNoPageLink
	}
	return address, nil
}

// SetFindBar opens or closes a Web item's find bar, which the frontend
// draws at the top of its pane's body, above its web view, which moves
// down to make room (webFrame). Opening it gives Bench's own UI the
// keyboard, for the bar's field; closing it gives the page the keyboard
// back, if its pane is focused and shows it. A page in a window of its own
// has no bar.
func (e *Engine) SetFindBar(itemID string, open bool) error {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		return errGone
	case it.Kind != KindWeb:
		return errPageOnly
	case open && it.OwnWindow:
		return errFindOwnWindow
	}
	if it.finding != open {
		it.finding = open
		e.arrangeWebLocked(e.displayLocked())
		e.touch()
	}
	if open {
		e.newFocusLocked()
		if !e.opts.Headless {
			e.p.FocusUI()
		}
		return nil
	}
	if g, ok := e.displayLocked().focusedGeom(); ok && g.node.Active == it.ID {
		e.newFocusLocked()
		e.focusWebLocked(it)
	}
	return nil
}

// FindInPage finds text in a Web item's page, as find in page does in a
// browser: the next match after the one selected, or the one before it if
// backwards, ignoring case and going round at the end. WebKit selects the
// match and scrolls to it. It reports whether the page has one. It waits
// for the page's answer without the engine's lock.
func (e *Engine) FindInPage(itemID, text string, backwards bool) (bool, error) {
	e.lock()
	it, err := e.webViewOf(itemID)
	e.unlock()
	if err != nil || text == "" {
		return false, err
	}
	found, err := e.p.FindInWebView(it.ID, text, backwards)
	if err != nil {
		e.log.Warn("couldn't find in a page", "item", it.ID, "error", err)
		return false, errFindFailed
	}
	return found, nil
}

// MuteCapture mutes a Web item's page's microphone, or turns its camera
// off, with mute, or turns it back on, without it: device is "camera" or
// "microphone". The page stays in its call, and can't turn it back on
// itself. It's refused for a device the page isn't using. The view reports
// the change (WebStateChanged), for the mark on the sub-tab; a toast says
// so now.
func (e *Engine) MuteCapture(itemID, device string, mute bool) error {
	e.lock()
	defer e.unlock()
	it, err := e.webViewOf(itemID)
	if err != nil {
		return err
	}
	var using platform.CaptureState
	switch platform.CaptureDevice(device) {
	case platform.Camera:
		using = it.web.Camera
	case platform.Microphone:
		using = it.web.Microphone
	default:
		return errBadDevice
	}
	if using == platform.CaptureNone {
		return errNotCapturing(device)
	}
	state := platform.CaptureActive
	if mute {
		state = platform.CaptureMuted
	}
	if err := e.p.SetCaptureState(it.ID, platform.CaptureDevice(device), state); err != nil {
		return errPageFailed
	}
	e.toastLocked(captureToast(it.App, platform.CaptureDevice(device), mute))
	return nil
}

// captureToast says what MuteCapture did to an app's page.
func captureToast(app string, device platform.CaptureDevice, mute bool) string {
	switch {
	case device == platform.Camera && mute:
		return "Turned off " + app + "'s camera."
	case device == platform.Camera:
		return "Turned " + app + "'s camera back on."
	case mute:
		return "Muted " + app + "'s microphone."
	}
	return "Turned " + app + "'s microphone back on."
}

// StopMedia pauses every sound and video that a Web item's page plays, and
// its popups' pages, as when a video or a call carries on out of sight. The
// page can play them again. Bench can't tell whether a page plays sound,
// so a page's menu always offers it.
func (e *Engine) StopMedia(itemID string) error {
	e.lock()
	defer e.unlock()
	it, err := e.webViewOf(itemID)
	if err != nil {
		return err
	}
	if err := e.p.PauseWebViewMedia(it.ID); err != nil {
		return errPageFailed
	}
	e.toastLocked("Paused " + it.App + "'s sound and video.")
	return nil
}
