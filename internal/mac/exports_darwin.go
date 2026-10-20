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

// benchEmitItemEvent reports a Web item's event, or one about Bench's window
// (BenchHidden and BenchShown, with no item). A page that posts
// notifications too often is held back here.
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

//export benchDecideNavigation
func benchDecideNavigation(itemURL, targetURL *C.char, userClicked, signInByClick C.int) C.int {
	return C.int(DecideNavigation(C.GoString(itemURL), C.GoString(targetURL), userClicked != 0, signInByClick != 0))
}

//export benchDecidePopup
func benchDecidePopup(itemURL, targetURL, sourceOrigin *C.char, sourceMainFrame C.int) C.int {
	return C.int(DecidePopup(C.GoString(itemURL), C.GoString(targetURL), C.GoString(sourceOrigin), sourceMainFrame != 0))
}

//export benchDecidePopupNavigation
func benchDecidePopupNavigation(itemURL, startURL, targetURL *C.char, userClicked C.int) C.int {
	return C.int(DecidePopupNavigation(C.GoString(itemURL), C.GoString(startURL), C.GoString(targetURL), userClicked != 0))
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

//export benchNewWindowRank
func benchNewWindowRank(title *C.char) C.int {
	return C.int(newWindowRank(C.GoString(title)))
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

//export benchSignInByClick
func benchSignInByClick(itemURL, targetURL *C.char, userClicked, committed, signInByClick C.int) C.int {
	if SignInByClick(C.GoString(itemURL), C.GoString(targetURL), userClicked != 0, committed != 0, signInByClick != 0) {
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
