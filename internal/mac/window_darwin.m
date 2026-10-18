//go:build darwin && cgo && !server

// window_darwin.m: Bench's own window. It sits one level below normal
// windows, so docked windows stay above it; it can't go into native full
// screen; and it reports where its content area is.
//
// Main thread only, apart from the entry points, which hop there.

#import "internal_darwin.h"

#import <objc/message.h>
#import <objc/runtime.h>

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
static NSMutableArray *gWindowTokens = nil;
static BOOL gRaised = NO;

NSView *benchWebContainerView(void) {
	return gContainer;
}

NSWindow *benchBenchWindow(void) {
	return gWindow;
}

void benchReportContentFrame(void) {
	NSScreen *primary = [NSScreen screens].firstObject;
	double primaryHeight = primary != nil ? primary.frame.size.height : 0;
	NSView *content = gWindow.contentView;
	if (gWindow == nil || content == nil) {
		benchContentFrameUpdated(0, 0, 0, 0, 0, primaryHeight);
		return;
	}
	// The content view's bounds, through window coordinates, to AppKit's
	// screen coordinates. Wails' web view fills the content view, so this
	// is the frame of the UI's content coordinates.
	NSRect inWindow = [content convertRect:content.bounds toView:nil];
	NSRect onScreen = [gWindow convertRectToScreen:inWindow];
	benchContentFrameUpdated(1, onScreen.origin.x, onScreen.origin.y, onScreen.size.width, onScreen.size.height, primaryHeight);
}

void benchDetachWindow(void) {
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	for (id token in gWindowTokens) {
		[center removeObserver:token];
	}
	gWindowTokens = nil;
	[gContainer removeFromSuperview];
	gContainer = nil;
	gWindow = nil;
	benchReportContentFrame();
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

static void benchAttachOnMain(NSWindow *window) {
	if (window == gWindow || benchStopped()) {
		return;
	}
	if (gWindow != nil) {
		benchDetachWindow();
	}
	gWindow = window;

	window.level = gRaised ? NSFloatingWindowLevel : BenchLevel;
	// Docked windows can't follow a window into a full-screen Space, so
	// turn native full screen off; the green button then zooms instead.
	NSWindowCollectionBehavior behaviour = window.collectionBehavior;
	behaviour &= ~(NSWindowCollectionBehaviorFullScreenPrimary | NSWindowCollectionBehaviorFullScreenAuxiliary);
	behaviour |= NSWindowCollectionBehaviorFullScreenNone;
	window.collectionBehavior = behaviour;
	benchAcceptFirstMouse();
	benchPassThroughTitlebar(window);

	NSView *content = window.contentView;
	if (content != nil) {
		BenchWebContainer *container = [[BenchWebContainer alloc] initWithFrame:content.bounds];
		container.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		container.hidden = gRaised;
		// Above every other subview, the UI's web view among them.
		[content addSubview:container positioned:NSWindowAbove relativeTo:nil];
		gContainer = container;
		benchWebAttachContainer(container);
	}

	gWindowTokens = [NSMutableArray array];
	NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
	NSArray<NSNotificationName> *changes = @[
		NSWindowDidMoveNotification,
		NSWindowDidResizeNotification,
		NSWindowDidChangeScreenNotification,
	];
	for (NSNotificationName name in changes) {
		id token = [center addObserverForName:name object:window queue:nil usingBlock:^(NSNotification *note) {
			(void)note;
			benchReportContentFrame();
		}];
		[gWindowTokens addObject:token];
	}
	id closing = [center addObserverForName:NSWindowWillCloseNotification object:window queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		benchDetachWindow();
	}];
	[gWindowTokens addObject:closing];

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

void benchSetRaised(int raised) {
	BOOL up = raised != 0;
	benchOnMain(^{
		gRaised = up;
		if (gWindow == nil) {
			return;
		}
		gWindow.level = up ? NSFloatingWindowLevel : BenchLevel;
		// Hiding the container hides every web view and keeps each one's own
		// hidden state, so lowering again shows just the ones that showed.
		gContainer.hidden = up;
	});
}
