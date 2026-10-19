//go:build darwin && cgo && !server

// preview_darwin.m: live previews of windows and web views, in a small
// panel under the hovered sub-tab (or above it, near the bottom of Bench's
// window: see previewFrame).
//
// A window's preview comes from ScreenCaptureKit: a stream of just that
// window, small and at 10 frames a second, whose IOSurfaces go straight
// into a layer. A web view's preview is a snapshot taken a few times a
// second. Capturing starts in ShowPreview and stops in HidePreview. The
// panel appears with the first frame, so a capture that fails shows
// nothing, and it's labelled "Live" only because frames are arriving.
//
// Main thread only, apart from the stream's output, which runs on its own
// serial queue and hands frames to the main thread.

#import "internal_darwin.h"

#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <QuartzCore/QuartzCore.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>

#include <math.h>

// The panel's layout, in points, matching preview.go and the prototype's
// .preview: a 1-point border and 8 points of padding around a 232 by 145
// picture, with a caption line below it.
static const CGFloat BenchPreviewWidth = 250;
static const CGFloat BenchPreviewHeight = 186;
static const CGFloat BenchPreviewInset = 9; // Border and padding.
static const CGFloat BenchPreviewImageWidth = 232;
static const CGFloat BenchPreviewImageHeight = 145;
static const CGFloat BenchPreviewCaptionHeight = 15;
static const CGFloat BenchPreviewFontSize = 11.5;

// How often a web view's preview is refreshed, in seconds.
static const NSTimeInterval BenchSnapshotInterval = 0.25;

// BenchStreamOutput receives a window's frames, and hears if its stream
// stops.
@interface BenchStreamOutput : NSObject <SCStreamOutput, SCStreamDelegate>
@property (nonatomic, readonly) NSUInteger generation;
- (instancetype)initWithGeneration:(NSUInteger)generation;
@end

static NSPanel *gPanel = nil;
static CALayer *gImageLayer = nil;
static NSTextField *gTitleLabel = nil;
static NSTextField *gLiveLabel = nil;
static SCStream *gStream = nil;
static BenchStreamOutput *gStreamOutput = nil;
static NSTimer *gSnapshotTimer = nil;
static BOOL gSnapshotInFlight = NO;
static BOOL gPanelShown = NO;
// Every show and hide moves to a new generation, so that late callbacks
// from an earlier preview do nothing.
static NSUInteger gGeneration = 0;

static dispatch_queue_t benchPreviewQueue(void) {
	static dispatch_queue_t queue;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		queue = dispatch_queue_create("com.github.itbm.bench.preview", DISPATCH_QUEUE_SERIAL);
	});
	return queue;
}

static NSColor *benchColour(int red, int green, int blue) {
	return [NSColor colorWithSRGBRed:red / 255.0 green:green / 255.0 blue:blue / 255.0 alpha:1.0];
}

static NSTextField *benchLabel(NSFont *font, NSColor *colour) {
	NSTextField *label = [NSTextField labelWithString:@""];
	label.font = font;
	label.textColor = colour;
	label.lineBreakMode = NSLineBreakByTruncatingTail;
	label.maximumNumberOfLines = 1;
	return label;
}

// benchBuildPanel makes the panel the first time it's needed: borderless,
// never activating Bench, above everything, and letting the mouse through.
static void benchBuildPanel(void) {
	if (gPanel != nil) {
		return;
	}
	NSRect bounds = NSMakeRect(0, 0, BenchPreviewWidth, BenchPreviewHeight);
	NSPanel *panel = [[NSPanel alloc] initWithContentRect:bounds styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel backing:NSBackingStoreBuffered defer:YES];
	panel.level = NSPopUpMenuWindowLevel;
	panel.ignoresMouseEvents = YES;
	panel.opaque = NO;
	panel.backgroundColor = [NSColor clearColor];
	panel.hasShadow = YES;
	panel.releasedWhenClosed = NO;
	panel.hidesOnDeactivate = NO;
	panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorTransient | NSWindowCollectionBehaviorIgnoresCycle | NSWindowCollectionBehaviorFullScreenAuxiliary;

	// The prototype's colours: --bg-raised, --line-strong, --bg-pane,
	// --text-strong, and --text-muted.
	NSView *root = [[NSView alloc] initWithFrame:bounds];
	root.wantsLayer = YES;
	root.layer.backgroundColor = benchColour(0x23, 0x28, 0x33).CGColor;
	root.layer.borderColor = benchColour(0x34, 0x3B, 0x48).CGColor;
	root.layer.borderWidth = 1;
	root.layer.cornerRadius = 10;
	root.layer.masksToBounds = YES;
	panel.contentView = root;

	// The picture is a layer-hosting view (its layer set before wantsLayer),
	// so AppKit leaves the layer's contents alone.
	CGFloat imageY = BenchPreviewHeight - BenchPreviewInset - BenchPreviewImageHeight;
	NSView *imageView = [[NSView alloc] initWithFrame:NSMakeRect(BenchPreviewInset, imageY, BenchPreviewImageWidth, BenchPreviewImageHeight)];
	CALayer *imageLayer = [CALayer layer];
	imageLayer.backgroundColor = benchColour(0x0E, 0x10, 0x14).CGColor;
	imageLayer.cornerRadius = 6;
	imageLayer.masksToBounds = YES;
	imageLayer.contentsGravity = kCAGravityResizeAspect;
	imageView.layer = imageLayer;
	imageView.wantsLayer = YES;
	[root addSubview:imageView];

	NSTextField *live = benchLabel([NSFont systemFontOfSize:BenchPreviewFontSize], benchColour(0xA6, 0xAD, 0xB9));
	live.stringValue = @"Live";
	live.alignment = NSTextAlignmentRight;
	[live sizeToFit];
	CGFloat liveWidth = ceil(live.frame.size.width);
	live.frame = NSMakeRect(BenchPreviewInset + BenchPreviewImageWidth - 2 - liveWidth, BenchPreviewInset, liveWidth, BenchPreviewCaptionHeight);
	NSTextField *title = benchLabel([NSFont systemFontOfSize:BenchPreviewFontSize weight:NSFontWeightMedium], benchColour(0xF1, 0xF3, 0xF6));
	title.frame = NSMakeRect(BenchPreviewInset + 2, BenchPreviewInset, BenchPreviewImageWidth - 4 - liveWidth - 8, BenchPreviewCaptionHeight);
	[root addSubview:title];
	[root addSubview:live];

	gPanel = panel;
	gImageLayer = imageLayer;
	gTitleLabel = title;
	gLiveLabel = live;
}

// benchPreviewPrepare sets the panel up for a new preview, without showing
// it.
static void benchPreviewPrepare(NSString *title, NSRect frame) {
	benchBuildPanel();
	gTitleLabel.stringValue = title;
	gLiveLabel.hidden = YES;
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	gImageLayer.contents = nil;
	[CATransaction commit];
	[gPanel setFrame:frame display:NO];
	gPanelShown = NO;
}

// benchPreviewShowImage puts a frame (an IOSurface or an NSImage) in the
// panel, showing the panel with the first one.
static void benchPreviewShowImage(id contents, NSUInteger generation) {
	if (generation != gGeneration || gPanel == nil) {
		return;
	}
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	gImageLayer.contents = contents;
	[CATransaction commit];
	if (!gPanelShown) {
		gLiveLabel.hidden = NO;
		[gPanel orderFrontRegardless];
		gPanelShown = YES;
	}
}

void benchPreviewStopOnMain(void) {
	gGeneration++;
	if (gStream != nil) {
		SCStream *stream = gStream;
		BenchStreamOutput *output = gStreamOutput;
		gStream = nil;
		gStreamOutput = nil;
		// The block keeps the stream and its output until it has stopped.
		[stream stopCaptureWithCompletionHandler:^(NSError *error) {
			(void)error;
			(void)stream;
			(void)output;
		}];
	}
	[gSnapshotTimer invalidate];
	gSnapshotTimer = nil;
	gSnapshotInFlight = NO;
	if (gPanel != nil) {
		[gPanel orderOut:nil];
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		gImageLayer.contents = nil;
		[CATransaction commit];
	}
	gPanelShown = NO;
}

@implementation BenchStreamOutput

- (instancetype)initWithGeneration:(NSUInteger)generation {
	self = [super init];
	if (self) {
		_generation = generation;
	}
	return self;
}

- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
	(void)stream;
	if (type != SCStreamOutputTypeScreen || !CMSampleBufferIsValid(sampleBuffer)) {
		return;
	}
	// Only complete frames carry a new picture; the others say nothing
	// changed.
	CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, false);
	if (attachments == NULL || CFArrayGetCount(attachments) == 0) {
		return;
	}
	NSDictionary *info = (__bridge NSDictionary *)CFArrayGetValueAtIndex(attachments, 0);
	NSNumber *status = info[SCStreamFrameInfoStatus];
	if (![status isKindOfClass:[NSNumber class]] || status.integerValue != SCFrameStatusComplete) {
		return;
	}
	CVImageBufferRef pixels = CMSampleBufferGetImageBuffer(sampleBuffer);
	if (pixels == NULL) {
		return;
	}
	IOSurfaceRef surface = CVPixelBufferGetIOSurface(pixels);
	if (surface == NULL) {
		return;
	}
	// The layer shows the IOSurface itself, with no copy. Holding it as an
	// object keeps it alive until the main thread has used it.
	id contents = (__bridge id)surface;
	NSUInteger generation = self.generation;
	dispatch_async(dispatch_get_main_queue(), ^{
		benchPreviewShowImage(contents, generation);
	});
}

- (void)stream:(SCStream *)stream didStopWithError:(NSError *)error {
	(void)stream;
	(void)error;
	NSUInteger generation = self.generation;
	dispatch_async(dispatch_get_main_queue(), ^{
		if (generation == gGeneration) {
			benchPreviewStopOnMain();
		}
	});
}

@end

// benchPreviewStartStream starts capturing a window for the current
// preview.
static void benchPreviewStartStream(SCWindow *window, int width, int height, NSUInteger generation) {
	if (generation != gGeneration) {
		return;
	}
	SCContentFilter *filter = [[SCContentFilter alloc] initWithDesktopIndependentWindow:window];
	SCStreamConfiguration *configuration = [[SCStreamConfiguration alloc] init];
	configuration.width = (size_t)width;
	configuration.height = (size_t)height;
	configuration.minimumFrameInterval = CMTimeMake(1, 10); // At most 10 frames a second.
	configuration.showsCursor = NO;
	configuration.pixelFormat = kCVPixelFormatType_32BGRA;
	configuration.queueDepth = 3;
	configuration.scalesToFit = YES;
	configuration.capturesAudio = NO;
	if (@available(macOS 14.0, *)) {
		configuration.preservesAspectRatio = YES;
	}

	BenchStreamOutput *output = [[BenchStreamOutput alloc] initWithGeneration:generation];
	SCStream *stream = [[SCStream alloc] initWithFilter:filter configuration:configuration delegate:output];
	NSError *error = nil;
	if (![stream addStreamOutput:output type:SCStreamOutputTypeScreen sampleHandlerQueue:benchPreviewQueue() error:&error]) {
		return;
	}
	gStream = stream;
	gStreamOutput = output;
	// The block keeps the stream and its output until it has run, so that a
	// stream the preview has let go of meanwhile can still be stopped.
	[stream startCaptureWithCompletionHandler:^(NSError *startError) {
		dispatch_async(dispatch_get_main_queue(), ^{
			if (startError != nil) {
				if (generation == gGeneration) {
					benchPreviewStopOnMain();
				}
				return;
			}
			if (generation == gGeneration && gStream == stream) {
				return;
			}
			// The preview was hidden, or another shown, while the stream was
			// starting. The stop benchPreviewStopOnMain asked for then may have
			// come too soon to take, which would leave the stream capturing,
			// with nothing holding it: stop it again now that it has started.
			[stream stopCaptureWithCompletionHandler:^(NSError *stopError) {
				(void)stopError;
				(void)stream;
				(void)output;
			}];
		});
	}];
}

// benchPreviewSnapshot takes one snapshot of a web view for the current
// preview, unless one is already being taken.
static void benchPreviewSnapshot(NSString *itemID, NSUInteger generation) {
	if (generation != gGeneration || gSnapshotInFlight) {
		return;
	}
	WKWebView *web = benchWebViewForItem(itemID);
	if (web == nil) {
		return;
	}
	WKSnapshotConfiguration *configuration = [[WKSnapshotConfiguration alloc] init];
	configuration.snapshotWidth = @(BenchPreviewImageWidth);
	// Take the page as it is, rather than waiting for the next screen
	// update, which a hidden web view may never have.
	configuration.afterScreenUpdates = NO;
	gSnapshotInFlight = YES;
	[web takeSnapshotWithConfiguration:configuration completionHandler:^(NSImage *image, NSError *error) {
		(void)error;
		if (generation != gGeneration) {
			return;
		}
		gSnapshotInFlight = NO;
		if (image != nil) {
			benchPreviewShowImage(image, generation);
		}
	}];
}

void benchPreviewShowWindow(uint32_t windowID, const char *title, double x, double y, double w, double h, int captureW, int captureH) {
	NSString *name = benchString(title);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		benchPreviewStopOnMain();
		NSUInteger generation = gGeneration;
		benchPreviewPrepare(name, frame);
		[SCShareableContent getShareableContentExcludingDesktopWindows:YES onScreenWindowsOnly:NO completionHandler:^(SCShareableContent *content, NSError *error) {
			(void)error;
			SCWindow *target = nil;
			for (SCWindow *candidate in content.windows) {
				if (candidate.windowID == windowID) {
					target = candidate;
					break;
				}
			}
			if (target == nil) {
				return;
			}
			dispatch_async(dispatch_get_main_queue(), ^{
				benchPreviewStartStream(target, captureW, captureH, generation);
			});
		}];
	});
}

void benchPreviewShowWeb(const char *itemID, const char *title, double x, double y, double w, double h) {
	NSString *identifier = benchString(itemID);
	NSString *name = benchString(title);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		benchPreviewStopOnMain();
		NSUInteger generation = gGeneration;
		benchPreviewPrepare(name, frame);
		benchPreviewSnapshot(identifier, generation);
		NSTimer *timer = [NSTimer timerWithTimeInterval:BenchSnapshotInterval repeats:YES block:^(NSTimer *t) {
			(void)t;
			benchPreviewSnapshot(identifier, generation);
		}];
		// The common modes, so the preview keeps updating while a menu is
		// open or the mouse is tracking.
		[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
		gSnapshotTimer = timer;
	});
}

void benchPreviewHide(void) {
	benchOnMain(^{
		benchPreviewStopOnMain();
	});
}
