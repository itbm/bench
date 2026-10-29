//go:build darwin && cgo && !server

// window_darwin.m: Bench's own window. It sits one level below normal
// windows, so docked windows stay above it, and above them while it's
// raised, Bench is active, and none of Bench's other windows has the
// keyboard; it can't go into native full screen, but zooms to fill the
// screen, at launch too; it
// reports where its content area is, and when it's hidden and shown again;
// and it decides which view in it has the keyboard focus.
//
// Main thread only, apart from the entry points, which hop there, and
// benchBenchPressHeld.

#import "internal_darwin.h"

#import <objc/message.h>
#import <objc/runtime.h>

#include <stdatomic.h>

// The level Bench's window normally sits at: just below normal windows. A
// macro rather than a constant, since NSNormalWindowLevel isn't a constant
// expression in every SDK.
#define BenchLevel (NSNormalWindowLevel - 1)

// BenchWebContainer holds the Web items' web views, above the UI's own web
// view. It's flipped, so its coordinates are content coordinates (top-left
// origin, y down) and a web view's frame is its pane body as Go sends it.
// Clicks that miss every web view pass through to the UI below.
@interface BenchWebContainer : NSView
@end

@implementation BenchWebContainer

- (BOOL)isFlipped {
	return YES;
}

- (NSView *)hitTest:(NSPoint)point {
	NSView *hit = [super hitTest:point];
	return hit == self ? nil : hit;
}

@end

static NSWindow *gWindow = nil;
static BenchWebContainer *gContainer = nil;
// Wails' web view, which shows Bench's UI. Wails owns it.
static __weak WKWebView *gUIWebView = nil;
static NSMutableArray *gWindowTokens = nil;
static id gMouseMonitor = nil;
// Whether a menu, dialog, or drag is open in the UI (benchSetRaised). The
// window floats above the docked windows only while Bench is also active,
// and none of its other windows has the keyboard (benchLevelNow).
static BOOL gRaised = NO;
// Whether BenchHidden was the last of BenchHidden and BenchShown sent, and
// whether the window has been on screen since it was attached (before
// that, its not being on screen is Wails still opening it, not hiding it).
static BOOL gReportedHidden = NO;
static BOOL gSeenVisible = NO;
// Whether Bench's app is hiding or hidden, from just before it hides (its
// windows go before NSApp.hidden is set) until it's unhidden, and whether
// that counts as Bench's window being hidden (see benchUpdateHidden).
static BOOL gAppHidden = NO;
static BOOL gHideCounts = NO;
// When another app last hid, from the workspace's notifications, and how
// many times Bench's app has started hiding, so that the late check of a
// hide (see benchWillHide) is about the hide it was made for.
static id gOtherHideToken = nil;
static CFAbsoluteTime gOtherAppHid = 0;
static NSUInteger gHideSerial = 0;

// Another app's Hide Others hides every other app at about the same time:
// Bench hiding while it isn't active counts as Hide Others if another app
// hides within this many seconds of it.
static const CFAbsoluteTime BenchHideTogether = 1;

// The keyboard focus around raised mode, while the web views are hidden.
// gRestoreFocus is the web view that had the focus when Bench was raised,
// which gets it back when Bench is lowered, unless benchFocusView asked for
// a view meanwhile (gFocusAsked): then that view, gPendingFocus, gets it
// (or the UI keeps it, for nil). gShownFocus is the one of those that was
// still hidden when Bench was lowered (Go shows the web views again only
// after lowering Bench), and gShownFocusSince when: it gets the focus as
// it's shown (benchFocusShownView), unless the focus moves on first.
static __weak NSView *gRestoreFocus = nil;
static __weak NSView *gPendingFocus = nil;
static BOOL gFocusAsked = NO;
static __weak NSView *gShownFocus = nil;
static CFAbsoluteTime gShownFocusSince = 0;

// Go shows the web views straight after lowering Bench, so a web view that
// isn't shown within this many seconds of it isn't being shown for that
// (its tab was left, say), and mustn't take the focus when it's shown later.
static const CFAbsoluteTime BenchShownFocusWait = 1;

// The left mouse-down count (CGEventSourceCounterForEventType) of the last
// press that began in one of Bench's windows, with bit 32 set, or 0 before
// the first. Written on the main thread, read on the Accessibility queue.
static _Atomic(uint64_t) gBenchPress = 0;

// How much later than a press's event the session's last mouse-down may
// seem to be and still be that press (see benchNotePress), in seconds:
// room for rounding, and far less than the time between two presses.
static const NSTimeInterval BenchPressSlack = 0.05;

NSView *benchWebContainerView(void) {
	return gContainer;
}

NSWindow *benchBenchWindow(void) {
	return gWindow;
}

WKWebView *benchUIWebView(void) {
	return gUIWebView;
}

// benchFindUIWebView finds Wails' web view in window: the window's webView
// property, or else the first web view in its content view.
static WKWebView *benchFindUIWebView(NSWindow *window) {
	if ([window respondsToSelector:NSSelectorFromString(@"webView")]) {
		id view = [window valueForKey:@"webView"];
		if ([view isKindOfClass:[WKWebView class]]) {
			return (WKWebView *)view;
		}
	}
	for (NSView *view in window.contentView.subviews) {
		if ([view isKindOfClass:[WKWebView class]]) {
			return (WKWebView *)view;
		}
	}
	return nil;
}

// benchReferenceView is the view whose coordinates are the UI's content
// coordinates: Wails' web view. Wails makes the content view 1 pt narrower
// and shorter than the web view, which sticks out above it, so the content
// view's own bounds would put docked windows and web views 1 pt low.
static NSView *_Nullable benchReferenceView(void) {
	WKWebView *ui = gUIWebView;
	if (ui != nil && ui.window == gWindow) {
		return ui;
	}
	return gWindow.contentView;
}

void benchReportContentFrame(void) {
	NSScreen *primary = [NSScreen screens].firstObject;
	double primaryHeight = primary != nil ? primary.frame.size.height : 0;
	NSView *reference = benchReferenceView();
	if (gWindow == nil || reference == nil) {
		benchContentFrameUpdated(0, 0, 0, 0, 0, primaryHeight);
		return;
	}
	// The reference view's bounds, through window coordinates, to AppKit's
	// screen coordinates.
	NSRect inWindow = [reference convertRect:reference.bounds toView:nil];
	NSRect onScreen = [gWindow convertRectToScreen:inWindow];
	benchContentFrameUpdated(1, onScreen.origin.x, onScreen.origin.y, onScreen.size.width, onScreen.size.height, primaryHeight);
}

void benchDetachWindow(void) {
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	for (id token in gWindowTokens) {
		[center removeObserver:token];
	}
	gWindowTokens = nil;
	if (gOtherHideToken != nil) {
		[[[NSWorkspace sharedWorkspace] notificationCenter] removeObserver:gOtherHideToken];
		gOtherHideToken = nil;
	}
	if (gMouseMonitor != nil) {
		[NSEvent removeMonitor:gMouseMonitor];
		gMouseMonitor = nil;
	}
	[gContainer removeFromSuperview];
	gContainer = nil;
	gUIWebView = nil;
	gWindow = nil;
	gReportedHidden = NO;
	gSeenVisible = NO;
	gAppHidden = NO;
	gHideCounts = NO;
	gRestoreFocus = nil;
	gPendingFocus = nil;
	gFocusAsked = NO;
	gShownFocus = nil;
	benchReportContentFrame();
}

#pragma mark - Activating and the level

void benchActivateBench(void) {
	if ([NSApp isActive]) {
		return;
	}
	// macOS 14's activate is cooperative: it does nothing while another app
	// is active, unless that app yields. Bench comes forward this way only
	// for something the user did (a shortcut, a click on a notification, a
	// pane chosen from the keyboard), so it activates as Wails' Focus does.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	[NSApp activateIgnoringOtherApps:YES];
#pragma clang diagnostic pop
}

// benchOtherWindow reports whether a window is another of Bench's ordinary
// windows, such as Acknowledgements, a popped-out terminal, or a page's
// popup, rather than Bench's own window or a sheet on it.
static BOOL benchOtherWindow(NSWindow *window) {
	return window != nil && window != gWindow && window.sheetParent != gWindow && window.level == NSNormalWindowLevel;
}

// benchOtherWindowKey reports whether another of Bench's ordinary windows
// has the keyboard, or an alert or panel over one of them does: an alert
// takes the keyboard but leaves the window it's about as the main window.
static BOOL benchOtherWindowKey(void) {
	NSWindow *key = NSApp.keyWindow;
	if (key == nil || key == gWindow || key.sheetParent == gWindow) {
		return NO;
	}
	return benchOtherWindow(key) || benchOtherWindow(NSApp.mainWindow);
}

// benchLevelNow is the level for Bench's window: above the docked windows
// while it's raised and Bench is the active app, and otherwise just below
// normal windows. Raised but not active (the user switched to another app
// while a menu or dialog was open, or the first-run guide sent them to
// System Settings), Bench would float over every other app's windows; and
// while another of Bench's windows has the keyboard, it would float over
// that window.
static NSWindowLevel benchLevelNow(void) {
	return gRaised && [NSApp isActive] && !benchOtherWindowKey() ? NSFloatingWindowLevel : BenchLevel;
}

// benchActiveChanged follows Bench becoming active or not: it puts the
// window at its level, and shows the dialogs that pages left waiting for
// Bench to come forward. The web views stay hidden while Bench is raised,
// whether or not it's active.
static void benchActiveChanged(void) {
	if (gWindow == nil) {
		return;
	}
	gWindow.level = benchLevelNow();
	if ([NSApp isActive]) {
		benchWebShowWaitingDialogs();
	} else {
		// The user went to another app before the web view waiting for the
		// focus (see benchLowerFocus) was shown: it's not to take it later.
		gShownFocus = nil;
	}
}

#pragma mark - Hidden and shown

// benchUpdateHidden reports Bench's window becoming hidden (Bench > Hide,
// minimised, or closed, which Wails turns into hiding the window) or shown
// again, once per change.
//
// Bench's app being hidden counts unless another app's Hide Others hid it
// (see benchWillHide). While the app is hidden, its window being off screen
// doesn't count either.
static void benchUpdateHidden(void) {
	if (gWindow == nil) {
		return;
	}
	if (gWindow.visible) {
		gSeenVisible = YES;
	}
	BOOL hidden;
	if (gAppHidden || [NSApp isHidden]) {
		hidden = gHideCounts;
	} else {
		hidden = gWindow.miniaturized || (gSeenVisible && !gWindow.visible);
	}
	if (hidden == gReportedHidden) {
		return;
	}
	gReportedHidden = hidden;
	benchEmitItem(hidden ? BENCH_EV_BENCH_HIDDEN : BENCH_EV_BENCH_SHOWN, @"", nil, nil);
}

// benchWillHide decides whether Bench's app starting to hide counts as
// Bench's window being hidden. It does if Bench is the active app: the user
// hid Bench. Hidden while another app is active, it's either that app's
// Hide Others, which leaves that app's windows on screen, and those may be
// docked in Bench (Go hiding Bench's docked windows with it would hide the
// very app the user chose), or the user choosing Hide from Bench's Dock
// menu. Hide Others hides the other apps too, so Bench waits a moment, and
// counts the hide if no other app hid with it. (A window that was minimised
// or closed already stays hidden.)
static void benchWillHide(void) {
	gAppHidden = YES;
	gHideSerial++;
	if ([NSApp isActive] || gReportedHidden) {
		gHideCounts = YES;
		return;
	}
	gHideCounts = NO;
	NSUInteger serial = gHideSerial;
	CFAbsoluteTime hid = CFAbsoluteTimeGetCurrent();
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(BenchHideTogether * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		if (gWindow == nil || serial != gHideSerial || !gAppHidden || gOtherAppHid >= hid - BenchHideTogether) {
			return;
		}
		gHideCounts = YES;
		benchUpdateHidden();
	});
}

#pragma mark - Keyboard focus

// benchIsFocusedIn reports whether the keyboard focus in Bench's window is
// in view (view itself or a view inside it).
static BOOL benchIsFocusedIn(NSView *_Nullable view) {
	NSResponder *responder = gWindow.firstResponder;
	return view != nil && [responder isKindOfClass:[NSView class]] && [(NSView *)responder isDescendantOf:(NSView *)view];
}

// benchMakeFirstResponder gives view, or the UI's web view for nil, the
// keyboard focus in Bench's window, if it's in the window and not hidden.
static void benchMakeFirstResponder(NSView *_Nullable view) {
	NSView *target = view != nil ? view : gUIWebView;
	if (gWindow == nil || target == nil || target.window != gWindow || target.hiddenOrHasHiddenAncestor) {
		return;
	}
	[gWindow makeFirstResponder:target];
}

void benchFocusView(NSView *view) {
	if (gWindow == nil) {
		return;
	}
	gShownFocus = nil;
	benchActivateBench();
	[gWindow makeKeyAndOrderFront:nil];
	if (gRaised) {
		gFocusAsked = YES;
		gPendingFocus = view;
		if (view != nil && gContainer != nil && [view isDescendantOf:gContainer]) {
			return; // Hidden while Bench is raised: it gets the focus when Bench is lowered.
		}
	}
	benchMakeFirstResponder(view);
}

void benchYieldFocusFrom(NSView *view) {
	if (gShownFocus == view) {
		gShownFocus = nil; // It isn't to be shown after all.
	}
	if (gWindow != nil && view.window == gWindow && benchIsFocusedIn(view)) {
		benchMakeFirstResponder(nil);
	}
}

// benchRaiseFocus moves the keyboard focus from a web view to the UI as
// Bench is raised: the web views are about to be hidden, and the menu or
// dialog that's opening needs the keys. The web view gets it back when Bench
// is lowered (benchLowerFocus).
static void benchRaiseFocus(void) {
	gFocusAsked = NO;
	gPendingFocus = nil;
	gRestoreFocus = nil;
	gShownFocus = nil;
	if (gContainer == nil || !benchIsFocusedIn(gContainer)) {
		return;
	}
	NSView *view = (NSView *)gWindow.firstResponder;
	while (view.superview != nil && view.superview != gContainer) {
		view = view.superview;
	}
	gRestoreFocus = view;
	benchMakeFirstResponder(nil);
}

// benchUIHasFocus reports whether the UI's web view, or nothing, has the
// keyboard focus in Bench's window.
static BOOL benchUIHasFocus(void) {
	NSResponder *responder = gWindow.firstResponder;
	return responder == nil || responder == gWindow || benchIsFocusedIn(gUIWebView);
}

// benchLowerFocus gives the keyboard focus, as Bench is lowered, to the web
// view asked for while it was raised, or else back to the web view that had
// it before, if the UI still has it. A web view that's still hidden gets it
// when it's shown, if the UI still has it then.
static void benchLowerFocus(void) {
	BOOL asked = gFocusAsked;
	NSView *target = asked ? gPendingFocus : gRestoreFocus;
	gFocusAsked = NO;
	gPendingFocus = nil;
	gRestoreFocus = nil;
	if (target == nil) {
		return;
	}
	if (!asked && !benchUIHasFocus()) {
		return;
	}
	if (target.hiddenOrHasHiddenAncestor) {
		gShownFocus = target;
		gShownFocusSince = CFAbsoluteTimeGetCurrent();
		return;
	}
	benchMakeFirstResponder(target);
}

void benchFocusShownView(NSView *view) {
	NSView *target = gShownFocus;
	if (target == nil || target != view || gRaised) {
		return;
	}
	gShownFocus = nil;
	if (CFAbsoluteTimeGetCurrent() - gShownFocusSince <= BenchShownFocusWait && benchUIHasFocus()) {
		benchMakeFirstResponder(view);
	}
}

#pragma mark - Presses

// benchNotePress records that a press began in one of Bench's windows (see
// benchBenchPressHeld) and, for a press in a Web item's web view, reports
// it: the page gets the click, not Bench's UI, so this is how the item's
// pane gets focused. It runs for every left mouse-down in Bench, before
// the window gets it.
//
// The count is the session's when AppKit hands the event over, which can be
// a while after the press if the main thread was busy. If a newer
// mouse-down has reached the session since, the count is that press's,
// which may have begun in another app's window, so the press isn't
// recorded: claiming that one for Bench would make its drag look like
// Bench's own. Bench's own newer press is recorded when its event comes.
static void benchNotePress(NSEvent *event) {
	uint32_t count = CGEventSourceCounterForEventType(kCGEventSourceStateCombinedSessionState, kCGEventLeftMouseDown);
	// Both times are in seconds since the system started (not counting sleep).
	NSTimeInterval since = CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateCombinedSessionState, kCGEventLeftMouseDown);
	NSTimeInterval lastDown = [NSProcessInfo processInfo].systemUptime - since;
	if (lastDown <= event.timestamp + BenchPressSlack) {
		atomic_store(&gBenchPress, ((uint64_t)1 << 32) | count);
	}
	if (gWindow == nil || event.window != gWindow) {
		return;
	}
	NSString *itemID = benchWebItemAtPoint(gWindow, event.locationInWindow);
	if (itemID != nil) {
		benchEmitItem(BENCH_EV_WEB_FOCUSED, itemID, nil, nil);
	}
}

BOOL benchBenchPressHeld(void) {
	uint64_t press = atomic_load(&gBenchPress);
	if (press == 0) {
		return NO;
	}
	// A press that began elsewhere since has counted another mouse-down,
	// even if Bench never saw its own press end (the window server keeps the
	// mouse-up of a window drag).
	uint32_t count = CGEventSourceCounterForEventType(kCGEventSourceStateCombinedSessionState, kCGEventLeftMouseDown);
	return (uint32_t)press == count;
}

// benchAttachOnMain makes window Bench's window.
// benchAcceptsFirstMouse answers YES, so that a click on Bench's window
// while another app is active both activates Bench and acts, as it does in
// Electron apps with acceptFirstMouse. Bench sits under the docked windows,
// whose apps are usually the active ones, so without this every click on its
// tabs and sub-tabs, and every drag, would have to be made twice.
static BOOL benchAcceptsFirstMouse(id self, SEL _cmd, NSEvent *event) {
	(void)self;
	(void)_cmd;
	(void)event;
	return YES;
}

// benchAcceptFirstMouse makes every web view in Bench's process accept the
// first click: the UI's own (Wails uses a plain WKWebView) and the Web items'.
static void benchAcceptFirstMouse(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		Class cls = [WKWebView class];
		SEL sel = @selector(acceptsFirstMouse:);
		Method existing = class_getInstanceMethod(cls, sel);
		const char *types = existing != NULL ? method_getTypeEncoding(existing) : "B@:@";
		class_replaceMethod(cls, sel, (IMP)benchAcceptsFirstMouse, types);
	});
}

// Bench's tab strip, and its Auto, Wide, and Compact control, sit in the
// top 28 pt of its window, where macOS keeps the (invisible) title bar. When
// Bench wasn't active, the title bar took the first click there to activate
// the window, so the click did nothing (seen on a real Mac). So Bench's own
// title bar views answer hit tests only over the traffic lights, and every
// other click goes to the UI's web view, which accepts first clicks. Moving
// the window and double-click zoom come from the UI, through Wails.

// benchTitlebarHitTest is hitTest: for Bench's title bar views: the usual
// answer over a button (the traffic lights), otherwise nil, so the hit test
// goes on to the content below.
static NSView *benchTitlebarHitTest(NSView *self, SEL _cmd, NSPoint point) {
	struct objc_super sup = {self, class_getSuperclass(object_getClass(self))};
	NSView *hit = ((NSView * (*)(struct objc_super *, SEL, NSPoint)) objc_msgSendSuper)(&sup, _cmd, point);
	for (NSView *v = hit; v != nil && v != self; v = v.superview) {
		if ([v isKindOfClass:[NSButton class]]) {
			return hit;
		}
	}
	return nil;
}

// benchPassThrough gives one view (not its whole class) benchTitlebarHitTest,
// by moving it to a subclass made for the purpose.
static void benchPassThrough(NSView *view) {
	if (view == nil) {
		return;
	}
	Class base = object_getClass(view);
	NSString *baseName = NSStringFromClass(base);
	if ([baseName hasPrefix:@"BenchPassThrough_"]) {
		return;
	}
	NSString *name = [@"BenchPassThrough_" stringByAppendingString:baseName];
	Class sub = NSClassFromString(name);
	if (sub == Nil) {
		sub = objc_allocateClassPair(base, name.UTF8String, 0);
		if (sub == Nil) {
			return;
		}
		Method m = class_getInstanceMethod(base, @selector(hitTest:));
		class_addMethod(sub, @selector(hitTest:), (IMP)benchTitlebarHitTest, method_getTypeEncoding(m));
		objc_registerClassPair(sub);
	}
	object_setClass(view, sub);
}

// benchPassThroughTitlebar applies benchPassThrough to the title bar view
// holding the traffic lights and the container holding it.
static void benchPassThroughTitlebar(NSWindow *window) {
	NSView *titlebar = [window standardWindowButton:NSWindowCloseButton].superview;
	if (titlebar == nil || titlebar == window.contentView) {
		return;
	}
	benchPassThrough(titlebar);
	NSView *container = titlebar.superview;
	if (container != nil && container != window.contentView && container != window.contentView.superview) {
		benchPassThrough(container);
	}
}

// benchObserve calls handler on the main thread whenever the default
// notification centre posts name about object (any object for nil), until
// benchDetachWindow.
static void benchObserve(NSNotificationName name, id _Nullable object, void (^handler)(void)) {
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	id token = [center addObserverForName:name object:object queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		handler();
	}];
	[gWindowTokens addObject:token];
}

static void benchAttachOnMain(NSWindow *window) {
	if (window == gWindow || benchStopped()) {
		return;
	}
	if (gWindow != nil) {
		benchDetachWindow();
	}
	gWindow = window;
	gUIWebView = benchFindUIWebView(window);
	gReportedHidden = NO;
	gSeenVisible = window.visible;
	gAppHidden = NO;
	gHideCounts = NO;

	window.level = benchLevelNow();
	// Docked windows can't follow a window into a full-screen Space, so
	// turn native full screen off; the green button then zooms instead.
	// AppKit treats a window that isn't at the normal level as transient
	// (hidden by Mission Control, and not kept on one desktop) unless it's
	// marked as managed, and Bench's window never is at the normal level:
	// so mark it, to keep it on its desktop with the windows docked in it.
	NSWindowCollectionBehavior behaviour = window.collectionBehavior;
	behaviour &= ~(NSWindowCollectionBehaviorFullScreenPrimary | NSWindowCollectionBehaviorFullScreenAuxiliary |
	               NSWindowCollectionBehaviorTransient | NSWindowCollectionBehaviorStationary |
	               NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorMoveToActiveSpace);
	behaviour |= NSWindowCollectionBehaviorManaged | NSWindowCollectionBehaviorFullScreenNone;
	window.collectionBehavior = behaviour;
	// Bench is usually inactive, while the docked app has the keyboard, and
	// macOS shows tooltips only in the active app unless the window allows
	// them: the tooltips of Bench's tabs, sub-tabs, and buttons explain them.
	window.allowsToolTipsWhenApplicationIsInactive = YES;
	benchAcceptFirstMouse();
	benchPassThroughTitlebar(window);

	NSView *content = window.contentView;
	if (content != nil) {
		// Exactly over the UI's web view (see benchReferenceView), resizing
		// as it does, so that the container's coordinates are the UI's.
		NSView *reference = benchReferenceView();
		NSRect frame = reference != nil && reference != content ? [content convertRect:reference.bounds fromView:reference] : content.bounds;
		BenchWebContainer *container = [[BenchWebContainer alloc] initWithFrame:frame];
		container.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		container.hidden = gRaised;
		// Above every other subview, the UI's web view among them.
		[content addSubview:container positioned:NSWindowAbove relativeTo:nil];
		gContainer = container;
		benchWebAttachContainer(container);
	}

	gWindowTokens = [NSMutableArray array];
	for (NSNotificationName name in @[ NSWindowDidMoveNotification, NSWindowDidResizeNotification, NSWindowDidChangeScreenNotification ]) {
		benchObserve(name, window, ^{
			benchReportContentFrame();
		});
	}
	// Minimised and back, closed (Wails hides the window instead) and shown
	// again, and Bench > Hide and back.
	for (NSNotificationName name in @[ NSWindowDidMiniaturizeNotification, NSWindowDidDeminiaturizeNotification, NSWindowDidChangeOcclusionStateNotification ]) {
		benchObserve(name, window, ^{
			benchUpdateHidden();
		});
	}
	benchObserve(NSApplicationWillHideNotification, nil, ^{
		benchWillHide();
	});
	// On the main queue, like everything else that reads gOtherAppHid.
	gOtherHideToken = [[[NSWorkspace sharedWorkspace] notificationCenter] addObserverForName:NSWorkspaceDidHideApplicationNotification object:nil queue:[NSOperationQueue mainQueue] usingBlock:^(NSNotification *note) {
		NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
		if ([app isKindOfClass:[NSRunningApplication class]] && app.processIdentifier != getpid()) {
			gOtherAppHid = CFAbsoluteTimeGetCurrent();
		}
	}];
	benchObserve(NSApplicationDidHideNotification, nil, ^{
		benchUpdateHidden();
	});
	benchObserve(NSApplicationDidUnhideNotification, nil, ^{
		// Once AppKit has finished putting the windows back, so that a window
		// not on screen yet isn't taken for one that was closed.
		dispatch_async(dispatch_get_main_queue(), ^{
			gAppHidden = NO;
			benchUpdateHidden();
		});
	});
	for (NSNotificationName name in @[ NSApplicationDidBecomeActiveNotification, NSApplicationDidResignActiveNotification ]) {
		benchObserve(name, nil, ^{
			benchActiveChanged();
		});
	}
	// Any window, so as to follow another of Bench's windows taking the
	// keyboard or giving it back (see benchLevelNow).
	for (NSNotificationName name in @[ NSWindowDidBecomeKeyNotification, NSWindowDidResignKeyNotification ]) {
		benchObserve(name, nil, ^{
			if (gWindow != nil) {
				gWindow.level = benchLevelNow();
			}
		});
	}
	benchObserve(NSWindowWillCloseNotification, window, ^{
		benchDetachWindow();
	});
	gMouseMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown handler:^NSEvent *(NSEvent *event) {
		benchNotePress(event);
		return event;
	}];

	benchReportContentFrame();
}

void benchAttachWindow(void *nsWindow) {
	if (nsWindow == NULL) {
		return;
	}
	void *pointer = nsWindow;
	benchOnMain(^{
		id object = (__bridge id)pointer;
		if ([object isKindOfClass:[NSWindow class]]) {
			benchAttachOnMain((NSWindow *)object);
		}
	});
}

void benchFillWindow(void *nsWindow) {
	if (nsWindow == NULL) {
		return;
	}
	void *pointer = nsWindow;
	benchOnMain(^{
		id object = (__bridge id)pointer;
		if (![object isKindOfClass:[NSWindow class]]) {
			return;
		}
		// Wails' window class animates zoom:, from the frame the window has
		// now, and a frame set while it animates stops it short. So this is
		// NSWindow's own zoom:, which goes straight to the standard frame,
		// the screen's visible area, and keeps the frame the window had as
		// the one that the green button gives back. Zooming a window that
		// fills the screen would shrink it, so a zoomed one is left alone.
		struct objc_super sup = {object, [NSWindow class]};
		BOOL zoomed = ((BOOL (*)(struct objc_super *, SEL))objc_msgSendSuper)(&sup, @selector(isZoomed));
		if (!zoomed) {
			((void (*)(struct objc_super *, SEL, id))objc_msgSendSuper)(&sup, @selector(zoom:), nil);
		}
	});
}

void benchSetRaised(int raised) {
	BOOL up = raised != 0;
	benchOnMain(^{
		BOOL was = gRaised;
		gRaised = up;
		if (gWindow == nil) {
			return;
		}
		if (up && !was) {
			benchRaiseFocus();
		}
		gWindow.level = benchLevelNow();
		// Hiding the container hides every web view and keeps each one's own
		// hidden state, so lowering again shows just the ones that showed.
		gContainer.hidden = up;
		if (!up && was) {
			benchLowerFocus();
			benchWebShowWaitingDialogs();
		}
	});
}

void benchFocusUI(void) {
	benchOnMain(^{
		if (!benchStopped()) {
			benchFocusView(nil);
		}
	});
}
