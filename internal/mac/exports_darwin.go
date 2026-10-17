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
	p.events.push(platform.Event{Kind: k, Window: windowFromC(info)})
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

//export benchEmitItemEvent
func benchEmitItemEvent(kind C.int, itemID, title, body *C.char) {
	p := current.Load()
	k, ok := eventKindFor(int(kind))
	if p == nil || !ok {
		return
	}
	p.events.push(platform.Event{
		Kind:   k,
		ItemID: C.GoString(itemID),
		Title:  clip(C.GoString(title), maxTitle),
		Body:   clip(C.GoString(body), maxBody),
	})
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
func benchDecideNavigation(itemURL, targetURL *C.char, userClicked C.int) C.int {
	return C.int(DecideNavigation(C.GoString(itemURL), C.GoString(targetURL), userClicked != 0))
}

//export benchDecidePopup
func benchDecidePopup(itemURL, targetURL *C.char) C.int {
	return C.int(DecidePopup(C.GoString(itemURL), C.GoString(targetURL)))
}

// windowFromC copies a native window description.
func windowFromC(info *C.BenchWindowInfo) platform.Window {
	return platform.Window{
		ID:         platform.WindowID(info.windowID),
		PID:        int(info.pid),
		BundleID:   C.GoString(info.bundleID),
		App:        C.GoString(info.appName),
		Title:      C.GoString(info.title),
		Subrole:    C.GoString(info.subrole),
		Frame:      layout.Rect{X: float64(info.x), Y: float64(info.y), W: float64(info.w), H: float64(info.h)},
		Minimised:  info.minimised != 0,
		FullScreen: info.fullScreen != 0,
		Focused:    info.focused != 0,
		Created:    int(info.created),
	}
}

// appFromC copies a native app description.
func appFromC(info *C.BenchAppInfo) platform.App {
	return platform.App{
		PID:      int(info.pid),
		BundleID: C.GoString(info.bundleID),
		Name:     C.GoString(info.name),
		Hidden:   info.hidden != 0,
		Active:   info.active != 0,
		Regular:  info.regular != 0,
	}
}
