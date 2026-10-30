//go:build darwin && cgo && !server

// guide_darwin.m: the drop guide, which shows where a window dragged onto
// one of Bench's panes would go (Platform.ShowDropGuide). It's a panel over
// the pane, above every window, which never activates Bench and lets the
// mouse through, like the preview's. Go lays out what it draws (guide.go):
// the part of the pane the window would take, the targets, the one the
// pointer is on, and a label that says what a drop does.
//
// Main thread only.

#import "internal_darwin.h"

#include <math.h>

static const CGFloat BenchGuideRadius = 8;
static const CGFloat BenchGuideFontSize = 12;

// The UI's tokens that the guide draws with, dark and light, which follow
// the panel's appearance: Bench's appearance setting
// (benchPanelAppearance).
static NSColor *benchGuideAccent(void) {
	return benchThemeColour(0xF2A33A, 1, 0x9C5504, 1); // --accent
}

static NSColor *benchGuideAccentPressed(void) {
	return benchThemeColour(0xDD922F, 1, 0x7A4203, 1); // --accent-pressed
}

static NSColor *benchGuideAccentWash(void) {
	return benchThemeColour(0xF2A33A, 0.16, 0xC97814, 0.14); // --accent-wash
}

static NSColor *benchGuideOnAccent(void) {
	return benchThemeColour(0x121419, 1, 0xFFFFFF, 1); // --on-accent
}

static NSColor *benchGuideRaised(void) {
	return benchThemeColour(0x232833, 0.94, 0xFFFFFF, 0.94); // --bg-raised
}

static NSColor *benchGuideLine(void) {
	return benchThemeColour(0x343B48, 1, 0xC4C9D1, 1); // --line-strong
}

static NSColor *benchGuideText(void) {
	return benchThemeColour(0xF1F3F6, 1, 0x0D1015, 1); // --text-strong
}

// BenchGuideView draws the drop guide's marks, in its own coordinates, which
// are the panel's.
@interface BenchGuideView : NSView
@property (nonatomic, copy) NSData *marks;
@property (nonatomic, copy) NSString *label;
@end

@implementation BenchGuideView

- (BOOL)isOpaque {
	return NO;
}

// drawArea draws the part of the pane the window would take, as the
// prototype's .drop-indicator does inside Bench: --accent-wash, with a
// 2-point --accent border.
- (void)drawArea:(NSRect)r {
	NSRect inner = NSInsetRect(r, 1, 1);
	NSBezierPath *path = [NSBezierPath bezierPathWithRoundedRect:inner xRadius:BenchGuideRadius yRadius:BenchGuideRadius];
	[benchGuideAccentWash() setFill];
	[path fill];
	path.lineWidth = 2;
	[benchGuideAccent() setStroke];
	[path stroke];
}

// drawMark draws a target: a raised square with a picture of the pane in
// it, the side it stands for filled in, or the whole of it for the middle.
// The target the pointer is on is drawn in the accent colour.
- (void)drawMark:(NSRect)r side:(int)side on:(BOOL)on {
	NSBezierPath *box = [NSBezierPath bezierPathWithRoundedRect:r xRadius:BenchGuideRadius yRadius:BenchGuideRadius];
	[(on ? benchGuideAccent() : benchGuideRaised()) setFill];
	[box fill];
	box.lineWidth = 1;
	[(on ? benchGuideAccentPressed() : benchGuideLine()) setStroke];
	[box stroke];

	// Resolved in the appearance it's drawn in, so that its alpha can
	// change.
	NSColor *ink = [(on ? benchGuideOnAccent() : benchGuideText()) colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
	NSRect pane = NSInsetRect(r, 10, 10);
	NSRect part = pane;
	switch (side) {
	case BENCH_GUIDE_LEFT:
		part.size.width /= 2;
		break;
	case BENCH_GUIDE_RIGHT:
		part.size.width /= 2;
		part.origin.x += part.size.width;
		break;
	case BENCH_GUIDE_TOP:
		// AppKit's y goes up, so the top half starts halfway up.
		part.size.height /= 2;
		part.origin.y += part.size.height;
		break;
	case BENCH_GUIDE_BOTTOM:
		part.size.height /= 2;
		break;
	default:
		break;
	}
	[[ink colorWithAlphaComponent:0.45] setFill];
	[[NSBezierPath bezierPathWithRoundedRect:part xRadius:2 yRadius:2] fill];
	NSBezierPath *outline = [NSBezierPath bezierPathWithRoundedRect:NSInsetRect(pane, 0.75, 0.75) xRadius:3 yRadius:3];
	outline.lineWidth = 1.5;
	[ink setStroke];
	[outline stroke];
}

// drawLabel draws the label in a raised pill, centred in r, as wide as the
// text needs, up to r's width.
- (void)drawLabel:(NSRect)r {
	if (self.label.length == 0) {
		return;
	}
	NSMutableParagraphStyle *style = [[NSMutableParagraphStyle alloc] init];
	style.alignment = NSTextAlignmentCenter;
	style.lineBreakMode = NSLineBreakByTruncatingTail;
	NSDictionary *attributes = @{
		NSFontAttributeName: [NSFont systemFontOfSize:BenchGuideFontSize weight:NSFontWeightMedium],
		NSForegroundColorAttributeName: benchGuideText(),
		NSParagraphStyleAttributeName: style,
	};
	NSSize size = [self.label sizeWithAttributes:attributes];
	CGFloat width = fmin(ceil(size.width) + 20, r.size.width);
	NSRect pill = NSMakeRect(r.origin.x + floor((r.size.width - width) / 2), r.origin.y, width, r.size.height);
	NSBezierPath *path = [NSBezierPath bezierPathWithRoundedRect:pill xRadius:r.size.height / 2 yRadius:r.size.height / 2];
	[benchGuideRaised() setFill];
	[path fill];
	path.lineWidth = 1;
	[benchGuideLine() setStroke];
	[path stroke];
	CGFloat textHeight = ceil(size.height);
	NSRect text = NSMakeRect(pill.origin.x + 10, pill.origin.y + floor((pill.size.height - textHeight) / 2), pill.size.width - 20, textHeight);
	[self.label drawWithRect:text options:NSStringDrawingUsesLineFragmentOrigin | NSStringDrawingTruncatesLastVisibleLine attributes:attributes];
}

- (void)drawRect:(NSRect)dirtyRect {
	(void)dirtyRect;
	[[NSColor clearColor] setFill];
	NSRectFill(self.bounds);
	const BenchGuideMark *marks = self.marks.bytes;
	NSUInteger count = self.marks.length / sizeof(BenchGuideMark);
	for (NSUInteger i = 0; i < count; i++) {
		BenchGuideMark m = marks[i];
		NSRect r = NSMakeRect(m.x, m.y, m.w, m.h);
		switch (m.kind) {
		case BENCH_GUIDE_AREA:
			[self drawArea:r];
			break;
		case BENCH_GUIDE_MARK:
		case BENCH_GUIDE_MARK_ON:
			[self drawMark:r side:m.side on:m.kind == BENCH_GUIDE_MARK_ON];
			break;
		case BENCH_GUIDE_LABEL:
			[self drawLabel:r];
			break;
		default:
			break;
		}
	}
}

@end

static NSPanel *gGuidePanel = nil;
static BenchGuideView *gGuideView = nil;

// benchBuildGuidePanel makes the panel the first time it's needed:
// borderless, never activating Bench, above everything, on every desktop,
// and letting the mouse through, so that the drag and the drop go on to
// the window being dragged.
static void benchBuildGuidePanel(void) {
	if (gGuidePanel != nil) {
		return;
	}
	NSRect bounds = NSMakeRect(0, 0, 100, 100);
	NSPanel *panel = [[NSPanel alloc] initWithContentRect:bounds styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel backing:NSBackingStoreBuffered defer:YES];
	panel.level = NSPopUpMenuWindowLevel;
	panel.ignoresMouseEvents = YES;
	panel.opaque = NO;
	panel.backgroundColor = [NSColor clearColor];
	panel.hasShadow = NO;
	panel.releasedWhenClosed = NO;
	panel.hidesOnDeactivate = NO;
	panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorTransient | NSWindowCollectionBehaviorIgnoresCycle | NSWindowCollectionBehaviorFullScreenAuxiliary;

	BenchGuideView *view = [[BenchGuideView alloc] initWithFrame:bounds];
	view.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	panel.contentView = view;

	gGuidePanel = panel;
	gGuideView = view;
}

void benchDropGuideStopOnMain(void) {
	if (gGuidePanel != nil) {
		[gGuidePanel orderOut:nil];
	}
}

void benchShowDropGuide(double x, double y, double w, double h, const BenchGuideMark *marks, int count, const char *label) {
	// Go's memory is copied before this returns.
	NSData *copied = [NSData dataWithBytes:marks length:(count > 0 && marks != NULL) ? (NSUInteger)count * sizeof(BenchGuideMark) : 0];
	NSString *text = benchString(label);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		benchBuildGuidePanel();
		// It looks as Bench's windows do, by the appearance setting.
		gGuidePanel.appearance = benchPanelAppearance();
		gGuideView.marks = copied;
		gGuideView.label = text;
		[gGuidePanel setFrame:frame display:NO];
		gGuideView.needsDisplay = YES;
		[gGuidePanel orderFrontRegardless];
	});
}

void benchHideDropGuide(void) {
	benchOnMain(^{
		benchDropGuideStopOnMain();
	});
}
