//go:build darwin && cgo && !server

// window_darwin.m: Bench's own window. It sits one level below normal
// windows, so docked windows stay above it; it can't go into native full
// screen; and it reports where its content area is.
//
// Main thread only, apart from the entry points, which hop there.

#import "internal_darwin.h"

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
