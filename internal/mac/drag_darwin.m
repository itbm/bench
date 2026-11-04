//go:build darwin && cgo && !server

// drag_darwin.m: the user dragging windows by their title bars, for Go to
// drop them onto Bench's panes (Platform.TrackDrags), Bench's own popped-out
// windows getting and losing the keyboard, and their Back to Bench button.
// Also the left mouse button's state (Platform.MouseHeld), for Go to see
// the user let go of a docked window's edge, whatever the tracker follows.
//
// Other apps' windows: a global monitor of the left mouse button sees the
// press, the drags, and the release, wherever they happen, with no
// permission. At the press, the window under the pointer is looked up in
// the window server's list (CGWindowListCopyWindowInfo), which never waits
// for the app, on a serial queue of Bench's own; as the mouse drags, the
// window's frame is read again, at most 30 times a second however fast the
// mouse reports, and once the window has moved, each read is reported,
// with its frame at the press. While the button is held, a timer checks
// the window and the button ten times a second too, for windows that the
// window server moves without sending drag events, and for a pointer that
// rests. The release reports the drop. A press inside a window that
// doesn't move it, as when selecting text, reports nothing. The
// Accessibility API isn't asked anything, so an app that doesn't answer
// can't hold a drag up. Nor is a drag followed while Bench's window isn't
// on the desktop (Space) the user is on: a window dropped then would go
// into a pane the user can't see.
//
// Bench's own popped-out windows have the identifier "bench.item:" and the
// item's ID. The window server moves them as the user drags their title
// bars, so Bench sees no mouse-up: NSWindowWillMoveNotification starts a
// timer on the main thread that reads the button, the pointer, and the
// window's frame, reports the drag, and reports the drop once the button is
// up. Global monitors don't see Bench's own events, so the two never
// report the same drag.
//
// Main thread, apart from the drag queue's work on other apps' windows.

#import "internal_darwin.h"

#include <math.h>

// How often a drag of another app's window is checked while the button is
// held, and how often a drag of one of Bench's own windows is, in seconds.
static const NSTimeInterval BenchDragHeartbeat = 0.1;
static const NSTimeInterval BenchOwnDragPoll = 1.0 / 30;
// The least time between two reads of the frame of another app's window as
// the mouse drags, which sends far more events than that, whether the
// window moves or not, in seconds.
static const CFAbsoluteTime BenchDragInterval = 1.0 / 30;

static NSString *const BenchPopOutPrefix = @"bench.item:";

// The global monitor and the observer of Bench's own windows starting to
// move, while drags are tracked; the observers of Bench's windows getting
// and losing the keyboard; and the timer that follows a drag of one of
// Bench's own windows. Main thread only.
static id gDragMonitor = nil;
static id gWillMoveToken = nil;
static id gKeyToken = nil;
static id gResignToken = nil;
static NSTimer *gOwnDragTimer = nil;

// BenchDrag is the drag of another app's window under way, if active: the
// window, its frame at the press and now, the pointer, whether the window
// has moved since the press, and when its frame was last read. Drag queue
// only.
typedef struct {
	BOOL active;
	BOOL moving;
	uint32_t windowID;
	pid_t pid;
	CGRect start;
	CGRect frame;
	CGPoint pointer;
	CFAbsoluteTime checked;
} BenchDrag;

static BenchDrag gDrag;
static dispatch_source_t gHeartbeat = nil;

// benchDragQueue is the serial queue for the window lookups of other apps'
// drags.
static dispatch_queue_t benchDragQueue(void) {
	static dispatch_queue_t queue;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		queue = dispatch_queue_create("com.github.itbm.bench.drag", DISPATCH_QUEUE_SERIAL);
	});
	return queue;
}

#pragma mark - Other apps' windows

// benchBoundsOf reads a window's bounds from a window list entry. Quartz's
// global coordinates have a top-left origin, like the Accessibility API's.
static BOOL benchBoundsOf(NSDictionary *info, CGRect *out) {
	NSDictionary *bounds = info[(__bridge NSString *)kCGWindowBounds];
	if (![bounds isKindOfClass:[NSDictionary class]]) {
		return NO;
	}
	return CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)bounds, out);
}

// benchWindowUnder finds the window that a press at a point went to: the
// frontmost on-screen window there. It's another app's normal-level window
// (layer 0), or nothing to drag: Bench's own, the menu bar, the Dock, or a
// panel above normal windows. Drag queue only.
static BOOL benchWindowUnder(CGPoint point, uint32_t *windowID, pid_t *pid, CGRect *frame) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID);
	if (list == NULL) {
		return NO;
	}
	NSArray *windows = (__bridge_transfer NSArray *)list;
	for (NSDictionary *info in windows) {
		if (![info isKindOfClass:[NSDictionary class]]) {
			continue;
		}
		CGRect bounds;
		if (!benchBoundsOf(info, &bounds) || !CGRectContainsPoint(bounds, point)) {
			continue;
		}
		NSNumber *alpha = info[(__bridge NSString *)kCGWindowAlpha];
		if ([alpha isKindOfClass:[NSNumber class]] && alpha.doubleValue <= 0) {
			continue;
		}
		NSNumber *layer = info[(__bridge NSString *)kCGWindowLayer];
		NSNumber *number = info[(__bridge NSString *)kCGWindowNumber];
		NSNumber *owner = info[(__bridge NSString *)kCGWindowOwnerPID];
		if (![layer isKindOfClass:[NSNumber class]] || ![number isKindOfClass:[NSNumber class]] || ![owner isKindOfClass:[NSNumber class]]) {
			return NO;
		}
		if (layer.intValue != 0 || owner.intValue == getpid()) {
			return NO;
		}
		*windowID = number.unsignedIntValue;
		*pid = owner.intValue;
		*frame = bounds;
		return YES;
	}
	return NO;
}

// benchWindowFrame reads a window's frame now, from the window server. Any
// thread.
static BOOL benchWindowFrame(uint32_t windowID, CGRect *frame) {
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionIncludingWindow, windowID);
	if (list == NULL) {
		return NO;
	}
	NSArray *windows = (__bridge_transfer NSArray *)list;
	for (NSDictionary *info in windows) {
		NSNumber *number = [info isKindOfClass:[NSDictionary class]] ? info[(__bridge NSString *)kCGWindowNumber] : nil;
		if ([number isKindOfClass:[NSNumber class]] && number.unsignedIntValue == windowID) {
			return benchBoundsOf(info, frame);
		}
	}
	return NO;
}

// benchPointerNow returns where the pointer is, in global coordinates.
static CGPoint benchPointerNow(void) {
	CGEventRef probe = CGEventCreate(NULL);
	if (probe == NULL) {
		return CGPointZero;
	}
	CGPoint p = CGEventGetLocation(probe);
	CFRelease(probe);
	return p;
}

// benchReportDrag reports the drag of another app's window to Go. Drag
// queue only.
static void benchReportDrag(int kind) {
	benchEmitDragEvent(kind, gDrag.windowID, gDrag.pid, "",
		gDrag.start.origin.x, gDrag.start.origin.y, gDrag.start.size.width, gDrag.start.size.height,
		gDrag.frame.origin.x, gDrag.frame.origin.y, gDrag.frame.size.width, gDrag.frame.size.height,
		gDrag.pointer.x, gDrag.pointer.y);
}

// benchDragMoved is below: the timer that checks a drag calls it.
static void benchDragMoved(CGPoint pointer);

// benchStopHeartbeat stops the timer that checks a drag. Drag queue only.
static void benchStopHeartbeat(void) {
	if (gHeartbeat != nil) {
		dispatch_source_cancel(gHeartbeat);
		gHeartbeat = nil;
	}
}

// benchEndDrag forgets the drag of another app's window. Drag queue only.
static void benchEndDrag(void) {
	benchStopHeartbeat();
	gDrag = (BenchDrag){0};
}

// benchDragReleased reports the drop of a window that moved, and forgets
// the drag. Drag queue only.
static void benchDragReleased(CGPoint pointer) {
	if (gDrag.active && gDrag.moving) {
		CGRect frame;
		if (benchWindowFrame(gDrag.windowID, &frame)) {
			gDrag.frame = frame;
		}
		gDrag.pointer = pointer;
		benchReportDrag(BENCH_EV_WINDOW_DROPPED);
	}
	benchEndDrag();
}

// benchDragHeartbeat checks a drag a few times a second while the button is
// held: some apps' windows are moved by the window server, which sends no
// drag events, and a resting pointer sends none either. It reports the drag
// once the window has moved, and again while the pointer rests, for Go to
// tell that from a drag that has ended without its release being seen. A
// button that's up is that release. Drag queue only.
static void benchDragHeartbeat(void) {
	if (!gDrag.active) {
		benchStopHeartbeat();
		return;
	}
	CGPoint pointer = benchPointerNow();
	if (!CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft)) {
		benchDragReleased(pointer);
		return;
	}
	benchDragMoved(pointer);
}

// benchStartHeartbeat starts the timer that checks a drag. Drag queue
// only.
static void benchStartHeartbeat(void) {
	if (gHeartbeat != nil) {
		return;
	}
	dispatch_source_t timer = dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER, 0, 0, benchDragQueue());
	uint64_t every = (uint64_t)(BenchDragHeartbeat * NSEC_PER_SEC);
	dispatch_source_set_timer(timer, dispatch_time(DISPATCH_TIME_NOW, (int64_t)every), every, every / 10);
	dispatch_source_set_event_handler(timer, ^{
		benchDragHeartbeat();
	});
	dispatch_resume(timer);
	gHeartbeat = timer;
}

// benchBenchHere reports whether Bench's window is on screen, on the
// desktop (Space) the user is on, for a window to be dropped onto its
// panes. Main thread only.
static BOOL benchBenchHere(void) {
	NSWindow *window = benchBenchWindow();
	return window != nil && window.isVisible && window.isOnActiveSpace;
}

// benchDragPressed starts following a press, on the window it went to,
// if Bench's window is here (benchBenchHere, which the main thread asked
// at the press). Drag queue only.
static void benchDragPressed(CGPoint pointer, BOOL here) {
	benchEndDrag();
	if (!here) {
		return;
	}
	uint32_t windowID = 0;
	pid_t pid = 0;
	CGRect frame;
	if (!benchWindowUnder(pointer, &windowID, &pid, &frame)) {
		return;
	}
	gDrag.active = YES;
	gDrag.windowID = windowID;
	gDrag.pid = pid;
	gDrag.start = frame;
	gDrag.frame = frame;
	gDrag.pointer = pointer;
	benchStartHeartbeat();
}

// benchDragMoved follows the mouse dragging: once the window has moved
// since the press, the drag is reported. The window server is asked at
// most every BenchDragInterval, as a press that selects text or moves a
// slider drags as fast as the mouse reports, and the heartbeat catches a
// move that comes in between. Drag queue only.
static void benchDragMoved(CGPoint pointer) {
	if (!gDrag.active) {
		return;
	}
	gDrag.pointer = pointer;
	CFAbsoluteTime now = CFAbsoluteTimeGetCurrent();
	if (now - gDrag.checked < BenchDragInterval) {
		return;
	}
	gDrag.checked = now;
	CGRect frame;
	if (!benchWindowFrame(gDrag.windowID, &frame)) {
		benchEndDrag();
		return;
	}
	gDrag.frame = frame;
	if (!gDrag.moving) {
		if (CGRectEqualToRect(frame, gDrag.start)) {
			return;
		}
		gDrag.moving = YES;
	}
	benchReportDrag(BENCH_EV_WINDOW_DRAGGED);
}

#pragma mark - Bench's own windows

// benchPopOutItem returns the item of one of Bench's popped-out windows, or
// nil for any other window.
static NSString *_Nullable benchPopOutItem(NSWindow *_Nullable window) {
	NSString *identifier = window.identifier;
	if (![identifier hasPrefix:BenchPopOutPrefix]) {
		return nil;
	}
	return [identifier substringFromIndex:BenchPopOutPrefix.length];
}

// benchPrimaryHeight returns the primary display's height, for AppKit's
// coordinates to be turned into global ones.
static CGFloat benchPrimaryHeight(void) {
	NSScreen *primary = NSScreen.screens.firstObject;
	return primary != nil ? primary.frame.size.height : 0;
}

// benchGlobalRect turns a rectangle in AppKit's coordinates into global
// ones.
static CGRect benchGlobalRect(NSRect r, CGFloat primaryHeight) {
	return CGRectMake(r.origin.x, primaryHeight - (r.origin.y + r.size.height), r.size.width, r.size.height);
}

// benchStopOwnDrag stops following a drag of one of Bench's own windows.
// Main thread only.
static void benchStopOwnDrag(void) {
	[gOwnDragTimer invalidate];
	gOwnDragTimer = nil;
}

// benchOwnFrame reads the frame of one of Bench's own windows, in global
// coordinates, from the window server, which moves it while the user drags
// its title bar, ahead of AppKit hearing where it went; or, failing that,
// from AppKit. Main thread only.
static CGRect benchOwnFrame(NSWindow *window, CGFloat primaryHeight) {
	CGRect frame;
	if (window.windowNumber > 0 && benchWindowFrame((uint32_t)window.windowNumber, &frame)) {
		return frame;
	}
	return benchGlobalRect(window.frame, primaryHeight);
}

// benchFollowOwnDrag follows the user dragging one of Bench's popped-out
// windows by its title bar, from when it's about to move until the button
// is up, reporting the drag once the window has moved, and then the drop.
// Main thread only.
static void benchFollowOwnDrag(NSWindow *window, NSString *itemID) {
	benchStopOwnDrag();
	if (!benchBenchHere()) {
		return;
	}
	CGFloat primary = benchPrimaryHeight();
	CGRect start = benchOwnFrame(window, primary);
	__weak NSWindow *weakWindow = window;
	__block BOOL moved = NO;
	void (^report)(int, CGRect) = ^(int kind, CGRect frame) {
		NSPoint mouse = [NSEvent mouseLocation];
		benchEmitDragEvent(kind, 0, 0, (char *)benchUTF8(itemID),
			start.origin.x, start.origin.y, start.size.width, start.size.height,
			frame.origin.x, frame.origin.y, frame.size.width, frame.size.height,
			mouse.x, primary - mouse.y);
	};
	NSTimer *timer = [NSTimer timerWithTimeInterval:BenchOwnDragPoll repeats:YES block:^(NSTimer *t) {
		(void)t;
		NSWindow *w = weakWindow;
		if (w == nil || benchStopped()) {
			benchStopOwnDrag();
			return;
		}
		@autoreleasepool {
			CGRect frame = benchOwnFrame(w, primary);
			if (([NSEvent pressedMouseButtons] & 1) == 0) {
				if (moved) {
					report(BENCH_EV_WINDOW_DROPPED, frame);
				}
				benchStopOwnDrag();
				return;
			}
			if (!moved && CGRectEqualToRect(frame, start)) {
				return;
			}
			moved = YES;
			report(BENCH_EV_WINDOW_DRAGGED, frame);
		}
	}];
	// The common modes, so that the timer fires while AppKit tracks the
	// mouse.
	[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
	gOwnDragTimer = timer;
}

void benchMarkPopOut(NSWindow *window, NSString *itemID) {
	window.identifier = [BenchPopOutPrefix stringByAppendingString:itemID];
}

void benchTagPopOut(void *nsWindow, const char *itemID) {
	if (nsWindow == NULL) {
		return;
	}
	void *pointer = nsWindow;
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		// The window may have closed, and gone, since Go read its pointer.
		NSWindow *window = benchLiveWindow(pointer);
		if (benchStopped() || window == nil || identifier.length == 0) {
			return;
		}
		benchMarkPopOut(window, identifier);
		// It may have the keyboard already, as it's tagged once it's shown.
		if (window.isKeyWindow) {
			benchEmitItem(BENCH_EV_POP_OUT_FOCUSED, identifier, nil, nil);
		}
	});
}

void benchWatchPopOuts(void) {
	if (gKeyToken != nil) {
		return;
	}
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	gKeyToken = [center addObserverForName:NSWindowDidBecomeKeyNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		NSString *item = benchPopOutItem(note.object);
		if (item != nil && !benchStopped()) {
			benchEmitItem(BENCH_EV_POP_OUT_FOCUSED, item, nil, nil);
		}
	}];
	gResignToken = [center addObserverForName:NSWindowDidResignKeyNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		NSString *item = benchPopOutItem(note.object);
		if (item != nil && !benchStopped()) {
			benchEmitItem(BENCH_EV_POP_OUT_UNFOCUSED, item, nil, nil);
		}
	}];
}

// benchUntrackOnMain stops tracking drags. Main thread only.
static void benchUntrackOnMain(void) {
	if (gDragMonitor != nil) {
		[NSEvent removeMonitor:gDragMonitor];
		gDragMonitor = nil;
	}
	if (gWillMoveToken != nil) {
		[[NSNotificationCenter defaultCenter] removeObserver:gWillMoveToken];
		gWillMoveToken = nil;
	}
	benchStopOwnDrag();
	dispatch_async(benchDragQueue(), ^{
		benchEndDrag();
	});
}

void benchUnwatchPopOuts(void) {
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	if (gKeyToken != nil) {
		[center removeObserver:gKeyToken];
		gKeyToken = nil;
	}
	if (gResignToken != nil) {
		[center removeObserver:gResignToken];
		gResignToken = nil;
	}
	benchUntrackOnMain();
}

// benchTrackOnMain starts tracking drags. Main thread only.
static void benchTrackOnMain(void) {
	if (gDragMonitor == nil) {
		NSEventMask mask = NSEventMaskLeftMouseDown | NSEventMaskLeftMouseDragged | NSEventMaskLeftMouseUp;
		gDragMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:mask handler:^(NSEvent *event) {
			CGEventRef cg = event.CGEvent;
			CGPoint pointer = cg != NULL ? CGEventGetLocation(cg) : benchPointerNow();
			NSEventType type = event.type;
			// The monitor runs on the main thread, where Bench's window can
			// be asked where it is.
			BOOL here = type == NSEventTypeLeftMouseDown && benchBenchHere();
			dispatch_async(benchDragQueue(), ^{
				if (benchStopped()) {
					return;
				}
				switch (type) {
				case NSEventTypeLeftMouseDown:
					benchDragPressed(pointer, here);
					break;
				case NSEventTypeLeftMouseDragged:
					benchDragMoved(pointer);
					break;
				case NSEventTypeLeftMouseUp:
					benchDragReleased(pointer);
					break;
				default:
					break;
				}
			});
		}];
	}
	if (gWillMoveToken == nil) {
		gWillMoveToken = [[NSNotificationCenter defaultCenter] addObserverForName:NSWindowWillMoveNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
			NSWindow *window = note.object;
			NSString *item = benchPopOutItem(window);
			// Only the user's drag holds the button down: Bench moving the
			// window itself, as it centres a new one, doesn't.
			if (item != nil && !benchStopped() && ([NSEvent pressedMouseButtons] & 1) != 0) {
				benchFollowOwnDrag(window, item);
			}
		}];
	}
}

int benchMouseHeld(void) {
	// The button's state now, from the window server, as the drag
	// heartbeat reads it. It needs no event monitor, so no permission.
	return CGEventSourceButtonState(kCGEventSourceStateCombinedSessionState, kCGMouseButtonLeft) ? 1 : 0;
}

void benchTrackDrags(int on) {
	benchOnMain(^{
		if (on && !benchStopped()) {
			benchTrackOnMain();
		} else {
			benchUntrackOnMain();
		}
	});
}

#pragma mark - Back to Bench

// BenchBackButton is the Back to Bench button in a popped-out web page's
// title bar, which asks Go to put the page back in Bench.
@interface BenchBackButton : NSButton
@property (nonatomic, copy) NSString *itemID;
@end

@implementation BenchBackButton

- (void)pressBack:(id)sender {
	(void)sender;
	benchEmitItem(BENCH_EV_DOCK_REQUESTED, self.itemID, nil, nil);
}

@end

void benchAddBackToBench(NSWindow *window, NSString *itemID) {
	BenchBackButton *button = [BenchBackButton buttonWithTitle:@"Back to Bench" target:nil action:nil];
	button.itemID = itemID;
	button.target = button;
	button.action = @selector(pressBack:);
	button.bezelStyle = NSBezelStyleRounded;
	button.controlSize = NSControlSizeSmall;
	button.font = [NSFont systemFontOfSize:[NSFont systemFontSizeForControlSize:NSControlSizeSmall]];
	button.toolTip = @"Put this page back in Bench, in the focused pane";
	[button sizeToFit];

	// The title bar is about 28 points tall: the button sits in its middle,
	// with a little room before the window's edge.
	const CGFloat height = 28, inset = 8;
	NSSize size = button.frame.size;
	NSView *holder = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, size.width + inset, height)];
	button.frame = NSMakeRect(0, floor((height - size.height) / 2), size.width, size.height);
	// AppKit sizes the holder to the title bar, which a subtitle makes
	// taller: the button stays in its middle.
	button.autoresizingMask = NSViewMinYMargin | NSViewMaxYMargin;
	[holder addSubview:button];

	NSTitlebarAccessoryViewController *accessory = [[NSTitlebarAccessoryViewController alloc] init];
	accessory.view = holder;
	accessory.layoutAttribute = NSLayoutAttributeTrailing;
	[window addTitlebarAccessoryViewController:accessory];
}
