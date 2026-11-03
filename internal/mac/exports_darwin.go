//go:build darwin && cgo && !server

package mac

// This file holds the Go functions that the native side calls. cgo copies
// the preamble of a file with //export into _cgo_export.h, so it may only
// declare things, not define them.

/*
#include "bridge_darwin.h"
*/
import "C"

import (
	"time"
	"unsafe"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// The native side calls these from the main thread and from its
// Accessibility queue. None of them blocks or takes a lock that's held for
// longer than a moment: each makes an Event and hands it to the event
// queue, or stores a value atomically.

//export benchEmitWindowEvent
func benchEmitWindowEvent(kind C.int, info *C.BenchWindowInfo) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || info == nil || !ok {
		return
	}
	p.events.push(platform.Event{Kind: k, Window: windowFromC(info), MouseDown: info.mouseDown != 0})
}

//export benchEmitAppEvent
func benchEmitAppEvent(kind C.int, info *C.BenchAppInfo) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || info == nil || !ok {
		return
	}
	p.events.push(platform.Event{Kind: k, App: appFromC(info)})
}

// benchEmitItemEvent reports a Web item's event, or one with no item: about
// Bench's window (BenchHidden and BenchShown), the user logging out
// (LoggingOut), the Mac waking (Woke, ScreensWoke, SessionActive, and
// ScreenUnlocked), or the user going to Bench's desktop (DesktopChanged).
// A page that posts notifications too often is held back here.
//
//export benchEmitItemEvent
func benchEmitItemEvent(kind C.int, itemID, title, body *C.char) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || !ok {
		return
	}
	id := C.GoString(itemID)
	if k == platform.WebNotification && !p.notifyLimit.allow(id) {
		return
	}
	p.events.push(platform.Event{
		Kind:   k,
		ItemID: id,
		Title:  clip(C.GoString(title), maxTitle),
		Body:   clip(C.GoString(body), maxBody),
	})
}

// benchEmitPageEvent reports the page a Web item's view shows, once it has
// come into view, or changed: its address, and whether everything it has
// loaded came over secure connections. itemURL is the item's own address,
// whose site the page's is compared with (see PageInfo).
//
//export benchEmitPageEvent
func benchEmitPageEvent(kind C.int, itemID, itemURL, pageURL *C.char, onlySecureContent C.int) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || !ok {
		return
	}
	address := C.GoString(pageURL)
	p.events.push(platform.Event{
		Kind:   k,
		ItemID: C.GoString(itemID),
		Body:   clip(address, maxBody),
		Page:   PageInfo(C.GoString(itemURL), address, onlySecureContent != 0),
	})
}

// benchEmitWebState reports what a Web item's page can do and is doing now:
// whether its view can go back or forward, and how it uses the camera and
// microphone, as WebKit's capture states.
//
//export benchEmitWebState
func benchEmitWebState(itemID *C.char, canGoBack, canGoForward, camera, microphone C.int) {
	p := current.Load()
	if p == nil {
		return
	}
	p.events.push(platform.Event{
		Kind:   platform.WebStateChanged,
		ItemID: C.GoString(itemID),
		Web:    webState(canGoBack != 0, canGoForward != 0, int(camera), int(microphone)),
	})
}

// benchPageIconLinks passes on the <link> elements of the page that a Web
// item's view has loaded, as JSON, for its icon, which Go fetches in the
// background.
//
//export benchPageIconLinks
func benchPageIconLinks(itemID, pageURL, links *C.char) {
	p := current.Load()
	if p == nil {
		return
	}
	p.foundIconLinks(C.GoString(itemID), C.GoString(pageURL), C.GoString(links))
}

// benchEmitDragEvent reports a drag or a drop of another app's window, or
// of one of Bench's popped-out windows (itemID), in global coordinates.
//
//export benchEmitDragEvent
func benchEmitDragEvent(kind C.int, windowID C.uint32_t, pid C.int32_t, itemID *C.char, startX, startY, startW, startH, x, y, w, h, pointerX, pointerY C.double) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || !ok {
		return
	}
	ev := platform.Event{
		Kind:    k,
		ItemID:  C.GoString(itemID),
		Start:   layout.Rect{X: float64(startX), Y: float64(startY), W: float64(startW), H: float64(startH)},
		Pointer: platform.Point{X: float64(pointerX), Y: float64(pointerY)},
		Window:  platform.Window{Frame: layout.Rect{X: float64(x), Y: float64(y), W: float64(w), H: float64(h)}},
	}
	if ev.ItemID == "" {
		ev.Window.ID, ev.Window.PID = platform.WindowID(windowID), int(pid)
	}
	p.events.push(ev)
}

// benchPageSubtitle returns a malloc'd subtitle for a window that shows a
// page (see pageSubtitle), which the caller frees.
//
//export benchPageSubtitle
func benchPageSubtitle(pageURL *C.char, onlySecureContent C.int) *C.char {
	return C.CString(pageSubtitle(PageInfo("", C.GoString(pageURL), onlySecureContent != 0)))
}

//export benchScreensUpdated
func benchScreensUpdated(screens *C.BenchScreenInfo, count C.int, changed C.int) {
	p := current.Load()
	if p == nil {
		return
	}
	raw := make([]rawScreen, 0, int(count))
	if screens != nil && count > 0 {
		for _, s := range unsafe.Slice(screens, int(count)) {
			raw = append(raw, rawScreen{
				DisplayID: uint32(s.displayID),
				Name:      C.GoString(s.name),
				Frame:     layout.Rect{X: float64(s.frameX), Y: float64(s.frameY), W: float64(s.frameW), H: float64(s.frameH)},
				Visible:   layout.Rect{X: float64(s.visibleX), Y: float64(s.visibleY), W: float64(s.visibleW), H: float64(s.visibleH)},
			})
		}
	}
	converted := screensFromRaw(raw)
	p.screens.Store(&converted)
	if changed != 0 {
		p.events.push(platform.Event{Kind: platform.ScreensChanged})
	}
}

//export benchContentFrameUpdated
func benchContentFrameUpdated(attached C.int, x, y, w, h, primaryH C.double) {
	p := current.Load()
	if p == nil {
		return
	}
	frame := layout.Rect{}
	if attached != 0 {
		appKit := layout.Rect{X: float64(x), Y: float64(y), W: float64(w), H: float64(h)}
		frame = GlobalFromAppKit(appKit, float64(primaryH))
	}
	p.content.Store(&frame)
	p.moved.trigger()
}

// benchScrollBarsUpdated stores whether macOS shows scroll bars all the
// time, and sends ScrollBarsChanged if that has changed: at start, when it
// differs from false, which ScrollBarsShown reports until then.
//
//export benchScrollBarsUpdated
func benchScrollBarsUpdated(shown C.int) {
	p := current.Load()
	if p == nil {
		return
	}
	if p.scrollBars.Swap(shown != 0) != (shown != 0) {
		p.events.push(platform.Event{Kind: platform.ScrollBarsChanged})
	}
}

// benchStageManagerUpdated stores whether Stage Manager is on, and sends
// StageManagerChanged if that has changed: at start, when it's on, as
// StageManager reports false until then.
//
//export benchStageManagerUpdated
func benchStageManagerUpdated(on C.int) {
	p := current.Load()
	if p == nil {
		return
	}
	if p.stageManager.Swap(on != 0) != (on != 0) {
		p.events.push(platform.Event{Kind: platform.StageManagerChanged})
	}
}

// benchSystemKeysUpdated stores the keys that macOS keeps for itself: the
// input sources' hot keys, for count of them, while sources, the number of
// input sources the user can switch between, is more than one; and
// VoiceOver's, while it's on. It sends SystemKeysChanged if they have
// changed: at start, when they differ from none, which SystemKeys reports
// until then.
//
//export benchSystemKeysUpdated
func benchSystemKeysUpdated(keys *C.BenchHotKeyInfo, count C.int, sources C.int, voiceOver C.int) {
	p := current.Load()
	if p == nil {
		return
	}
	raw := make([]rawHotKey, 0, int(count))
	if keys != nil && count > 0 {
		for _, k := range unsafe.Slice(keys, int(count)) {
			raw = append(raw, rawHotKey{Enabled: k.enabled != 0, KeyCode: int(k.keyCode), Flags: int(k.flags)})
		}
	}
	next := systemKeysFrom(raw, int(sources), voiceOver != 0)
	if old := p.systemKeys.Swap(&next); !sameSystemKeys(*old, next) {
		p.events.push(platform.Event{Kind: platform.SystemKeysChanged})
	}
}

// benchWillPowerOff records that macOS has said that the user is logging
// out, restarting, or shutting down, for LoggingOut. The LoggingOut event
// that goes with it comes through the Accessibility queue.
//
//export benchWillPowerOff
func benchWillPowerOff() {
	if p := current.Load(); p != nil {
		p.poweringOff.Store(time.Now().UnixNano())
	}
}

// benchNavDecide decides a navigation of a Web item's main frame (see
// navigator), logs the decision (see logDecision), and sets *returnHome to
// whether the native side then loads the item's address again. navigation
// is the navigation that WebKit asks about it for, or 0 if it doesn't say.
//
//export benchNavDecide
func benchNavDecide(itemID, itemURL *C.char, navigation C.uint64_t, targetURL *C.char, kind, source C.int, sourceOrigin *C.char, userEvent, now C.double, returnHome *C.int) C.int {
	id := C.GoString(itemID)
	action := mainAction{
		target:       C.GoString(targetURL),
		kind:         navKind(kind),
		from:         navSource(source),
		navigation:   uint64(navigation),
		sourceOrigin: C.GoString(sourceOrigin),
		userEvent:    float64(userEvent),
		now:          float64(now),
	}
	decision, home := webNavigator.decide(id, C.GoString(itemURL), action)
	logDecision(webLog.Load(), "web navigation", id, action.target, decision.String(),
		"kind", action.kind.String(), "from", action.from.String(), "navigation", action.navigation, "returnHome", home)
	if returnHome != nil {
		*returnHome = 0
		if home {
			*returnHome = 1
		}
	}
	return C.int(decision)
}

//export benchNavLoadRequested
func benchNavLoadRequested(itemID *C.char, navigation C.uint64_t) {
	webNavigator.loadRequested(C.GoString(itemID), uint64(navigation))
}

//export benchNavStarted
func benchNavStarted(itemID *C.char, navigation C.uint64_t) {
	webNavigator.started(C.GoString(itemID), uint64(navigation))
}

//export benchNavCommitted
func benchNavCommitted(itemID, itemURL *C.char, navigation C.uint64_t, pageURL *C.char, now C.double) {
	webNavigator.committed(C.GoString(itemID), C.GoString(itemURL), uint64(navigation), C.GoString(pageURL), float64(now))
}

// benchNavFailed notes a navigation that failed before its page came into
// view, and reports whether the native side then loads the item's address
// again.
//
//export benchNavFailed
func benchNavFailed(itemID *C.char, navigation C.uint64_t, replaced C.int) C.int {
	if webNavigator.failed(C.GoString(itemID), uint64(navigation), replaced != 0) {
		return 1
	}
	return 0
}

//export benchNavProcessEnded
func benchNavProcessEnded(itemID *C.char) {
	webNavigator.processEnded(C.GoString(itemID))
}

//export benchNavForget
func benchNavForget(itemID *C.char) {
	webNavigator.forget(C.GoString(itemID))
}

// benchDecidePopup decides a window that a Web item's page opens (see
// DecidePopup), and holds back one that would open the browser or another
// app once the user event that allows it, userEvent, has allowed one (see
// webOpenLimit). It logs the decision (see logDecision).
//
//export benchDecidePopup
func benchDecidePopup(itemID, itemURL, targetURL, sourceOrigin *C.char, sourceInItem C.int, userEvent, now C.double) C.int {
	id, target := C.GoString(itemID), C.GoString(targetURL)
	popup := DecidePopup(C.GoString(itemURL), target, C.GoString(sourceOrigin), sourceInItem != 0)
	popup = webOpenLimit.popup(id, popup, float64(userEvent), float64(now))
	logDecision(webLog.Load(), "web popup", id, target, popup.String(), "inItem", sourceInItem != 0)
	return C.int(popup)
}

// benchDecidePopupNavigation decides a navigation of a popup's main frame
// (see DecidePopupNavigation), holding back opens as benchDecidePopup does,
// and logs the decision.
//
//export benchDecidePopupNavigation
func benchDecidePopupNavigation(itemID, itemURL, startURL, targetURL *C.char, userClicked C.int, userEvent, now C.double) C.int {
	id, target := C.GoString(itemID), C.GoString(targetURL)
	decision := DecidePopupNavigation(C.GoString(itemURL), C.GoString(startURL), target, userClicked != 0)
	decision = webOpenLimit.navigation(id, decision, float64(userEvent), float64(now))
	logDecision(webLog.Load(), "web popup navigation", id, target, decision.String(), "clicked", userClicked != 0)
	return C.int(decision)
}

//export benchDecideResponse
func benchDecideResponse(itemURL, responseURL *C.char, mainFrame, canShow C.int, contentDisposition *C.char) C.int {
	return C.int(DecideResponse(C.GoString(itemURL), C.GoString(responseURL), mainFrame != 0, canShow != 0, C.GoString(contentDisposition)))
}

// benchDownloadPath returns a malloc'd path for a download, which the caller
// frees, or NULL if there's none.
//
//export benchDownloadPath
func benchDownloadPath(dir, suggested *C.char) *C.char {
	path := webDownloads.path(C.GoString(dir), C.GoString(suggested))
	if path == "" {
		return nil
	}
	return C.CString(path)
}

// benchDownloadEnded frees a path that benchDownloadPath gave out, once its
// download has finished or failed.
//
//export benchDownloadEnded
func benchDownloadEnded(path *C.char) {
	webDownloads.release(C.GoString(path))
}

// benchChoseToAdd passes on the path chosen in the open panel that
// ChooseToAdd shows, or NULL if the user cancelled.
//
//export benchChoseToAdd
func benchChoseToAdd(path *C.char) {
	if p := current.Load(); p != nil {
		p.choseToAdd(C.GoString(path))
	}
}

// benchWebFound hands FindInWebView WebKit's answer to its request: whether
// the page has a match.
//
//export benchWebFound
func benchWebFound(request C.uint64_t, found C.int) {
	if p := current.Load(); p != nil {
		p.finds.answer(uint64(request), found != 0)
	}
}

//export benchNewWindowRank
func benchNewWindowRank(title *C.char) C.int {
	return C.int(newWindowRank(C.GoString(title)))
}

//export benchCloseWindowRank
func benchCloseWindowRank(title *C.char) C.int {
	return C.int(closeWindowRank(C.GoString(title)))
}

//export benchNotificationAllowed
func benchNotificationAllowed(itemURL, origin *C.char, mainFrame C.int) C.int {
	if notificationAllowed(C.GoString(itemURL), C.GoString(origin), mainFrame != 0) {
		return 1
	}
	return 0
}

//export benchUserClicked
func benchUserClicked(linkActivated C.int, sinceUserEvent C.double) C.int {
	if UserClicked(linkActivated != 0, float64(sinceUserEvent)) {
		return 1
	}
	return 0
}

// benchAllowDownload reports whether a Web item's page may start a download
// now (see downloadLimiter), and if so counts it.
//
//export benchAllowDownload
func benchAllowDownload(itemID *C.char, userEvent, now C.double) C.int {
	if webDownloadLimit.allow(C.GoString(itemID), float64(userEvent), float64(now)) {
		return 1
	}
	return 0
}

// benchDialogText returns a page's alert or confirmation text, length
// bytes of UTF-8, made fit to show (see dialogText), malloc'd for the
// caller to free.
//
//export benchDialogText
func benchDialogText(text *C.char, length C.int) *C.char {
	return C.CString(dialogText(C.GoStringN(text, length)))
}

// benchDialogHeading returns the heading of an alert or confirmation that
// a frame brings up (see dialogHeading), malloc'd for the caller to free,
// or NULL for a dialog that gets its answer at once, without being shown.
//
//export benchDialogHeading
func benchDialogHeading(pageURL, frameOrigin *C.char, mainFrame C.int) *C.char {
	heading, ok := dialogHeading(C.GoString(pageURL), C.GoString(frameOrigin), mainFrame != 0)
	if !ok {
		return nil
	}
	return C.CString(heading)
}

// benchMediaPrompt returns the question that a prompt for a page's request
// for the camera or microphone asks (see mediaPrompt), and sets *detail to
// its details, both malloc'd for the caller to free; or it returns NULL,
// and sets *detail to NULL, for a request that's refused at once.
//
//export benchMediaPrompt
func benchMediaPrompt(itemURL, itemName, pageURL, frameOrigin *C.char, mainFrame, popup, media C.int, detail **C.char) *C.char {
	question, text, ok := mediaPrompt(mediaRequest{
		itemURL:     C.GoString(itemURL),
		itemName:    C.GoString(itemName),
		pageURL:     C.GoString(pageURL),
		frameOrigin: C.GoString(frameOrigin),
		mainFrame:   mainFrame != 0,
		popup:       popup != 0,
		media:       mediaKind(media),
	})
	if detail != nil {
		*detail = nil
		if ok {
			*detail = C.CString(text)
		}
	}
	if !ok {
		return nil
	}
	return C.CString(question)
}

// benchPromptAllowed reports whether a Web item's page may bring up a
// prompt of a kind now (see promptGate), rather than getting its answer at
// once.
//
//export benchPromptAllowed
func benchPromptAllowed(itemID *C.char, kind C.int, now C.double) C.int {
	if webPrompts.allow(C.GoString(itemID), promptKind(kind), float64(now)) {
		return 1
	}
	return 0
}

// benchPromptOfferStop reports whether a dialog shown now offers to stop
// the page's dialogs.
//
//export benchPromptOfferStop
func benchPromptOfferStop(itemID *C.char, now C.double) C.int {
	if webPrompts.offerStop(C.GoString(itemID), float64(now)) {
		return 1
	}
	return 0
}

// benchPromptEnded notes that a Web item's prompt of a kind closed, and
// whether the user stopped the page's prompts of that kind.
//
//export benchPromptEnded
func benchPromptEnded(itemID *C.char, kind, stop C.int, now C.double) {
	webPrompts.ended(C.GoString(itemID), promptKind(kind), stop != 0, float64(now))
}

// windowFromC copies a native window description. The title and the app's
// name are clipped, as web pages' titles are, since every view, save, and
// event carries them.
func windowFromC(info *C.BenchWindowInfo) platform.Window {
	return platform.Window{
		ID:         platform.WindowID(info.windowID),
		PID:        int(info.pid),
		BundleID:   C.GoString(info.bundleID),
		App:        goClipped(info.appName, maxTitle),
		Title:      goClipped(info.title, maxTitle),
		Subrole:    C.GoString(info.subrole),
		Frame:      layout.Rect{X: float64(info.x), Y: float64(info.y), W: float64(info.w), H: float64(info.h)},
		Minimised:  info.minimised != 0,
		FullScreen: info.fullScreen != 0,
		Focused:    info.focused != 0,
		Created:    int(info.created),
	}
}

// appFromC copies a native app description, with its name clipped.
func appFromC(info *C.BenchAppInfo) platform.App {
	return platform.App{
		PID:      int(info.pid),
		BundleID: C.GoString(info.bundleID),
		Name:     goClipped(info.name, maxTitle),
		Hidden:   info.hidden != 0,
		Active:   info.active != 0,
		Regular:  info.regular != 0,
	}
}
