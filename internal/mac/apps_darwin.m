//go:build darwin && cgo && !server

// apps_darwin.m: running apps, launching them, hiding and showing them, the
// default browser, opening files for the user, with NSWorkspace and
// NSRunningApplication, the open panel for choosing an app or program to
// add, and apps' icons.
//
// NSRunningApplication is thread-safe, and NSWorkspace's runningApplications
// may be read from any thread, so these run on the calling thread. Their
// values change only when the main run loop runs.

#import "internal_darwin.h"
#import <CoreServices/CoreServices.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>

#include <stdlib.h>
#include <string.h>
#include <unistd.h>

@implementation BenchAppSnapshot

- (instancetype)initWithApp:(NSRunningApplication *)app event:(int)kind {
	self = [super init];
	if (self) {
		_pid = app.processIdentifier;
		_bundleID = [app.bundleIdentifier copy] ?: @"";
		_name = [app.localizedName copy] ?: @"";
		_regular = benchIsRegular(app);
		switch (kind) {
		case BENCH_EV_APP_TERMINATED:
			_hidden = NO;
			_active = NO;
			break;
		case BENCH_EV_APP_ACTIVATED:
			_hidden = NO;
			_active = YES;
			break;
		default:
			_hidden = app.hidden;
			_active = app.active;
			break;
		}
	}
	return self;
}

@end

BOOL benchIsRegular(NSRunningApplication *app) {
	return app.activationPolicy == NSApplicationActivationPolicyRegular;
}

void benchEmitApp(int kind, BenchAppSnapshot *snapshot) {
	@autoreleasepool {
		BenchAppInfo info;
		memset(&info, 0, sizeof info);
		info.pid = snapshot.pid;
		info.bundleID = (char *)benchUTF8(snapshot.bundleID);
		info.name = (char *)benchUTF8(snapshot.name);
		info.hidden = snapshot.hidden ? 1 : 0;
		info.active = snapshot.active ? 1 : 0;
		info.regular = snapshot.regular ? 1 : 0;
		benchEmitAppEvent(kind, &info);
	}
}

int benchRunningApps(BenchAppInfo **out, int *count) {
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		NSArray<NSRunningApplication *> *apps = [[NSWorkspace sharedWorkspace] runningApplications];
		BenchAppInfo *infos = calloc(apps.count > 0 ? apps.count : 1, sizeof(BenchAppInfo));
		if (infos == NULL) {
			return BENCH_ERR_FAILED;
		}
		pid_t me = getpid();
		int n = 0;
		for (NSRunningApplication *app in apps) {
			if (app.terminated || app.processIdentifier <= 0 || app.processIdentifier == me) {
				continue;
			}
			BenchAppInfo *info = &infos[n++];
			info->pid = app.processIdentifier;
			info->bundleID = benchCopyCString(app.bundleIdentifier ?: @"");
			info->name = benchCopyCString(app.localizedName ?: @"");
			info->hidden = app.hidden ? 1 : 0;
			info->active = app.active ? 1 : 0;
			info->regular = benchIsRegular(app) ? 1 : 0;
		}
		*out = infos;
		*count = n;
	}
	return BENCH_OK;
}

void benchFreeAppInfos(BenchAppInfo *infos, int count) {
	if (infos == NULL) {
		return;
	}
	for (int i = 0; i < count; i++) {
		free(infos[i].bundleID);
		free(infos[i].name);
	}
	free(infos);
}

int benchOwnAppHidden(void) {
	@autoreleasepool {
		// NSRunningApplication is safe to read on any thread, unlike NSApp.
		return [NSRunningApplication currentApplication].hidden ? 1 : 0;
	}
}

int32_t benchResolvePID(int32_t pid, const char *bundleID) {
	@autoreleasepool {
		NSString *wanted = bundleID != NULL ? benchString(bundleID) : @"";
		if (pid > 0) {
			NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
			BOOL same = wanted.length == 0 || (app.bundleIdentifier != nil && [app.bundleIdentifier caseInsensitiveCompare:wanted] == NSOrderedSame);
			if (app != nil && !app.terminated && same) {
				return pid;
			}
		}
		if (wanted.length > 0) {
			for (NSRunningApplication *app in [NSRunningApplication runningApplicationsWithBundleIdentifier:wanted]) {
				if (!app.terminated && app.processIdentifier > 0) {
					return app.processIdentifier;
				}
			}
		}
	}
	return 0;
}

int benchOpenApp(const char *bundleID, const char *url) {
	@autoreleasepool {
		NSString *identifier = benchString(bundleID);
		NSString *address = url != NULL ? benchString(url) : @"";
		// A LaunchServices lookup, which is safe on any thread.
		NSURL *appURL = [[NSWorkspace sharedWorkspace] URLForApplicationWithBundleIdentifier:identifier];
		if (appURL == nil) {
			return BENCH_ERR_NOT_INSTALLED;
		}
		NSURL *target = nil;
		if (address.length > 0) {
			target = [NSURL URLWithString:address];
			if (target == nil || target.scheme.length == 0) {
				return BENCH_ERR_BAD_URL;
			}
		}
		// Opening a running app activates it and sends it a reopen event,
		// which makes most apps open a window if they have none.
		benchOnMainIfRunning(^{
			NSWorkspaceOpenConfiguration *configuration = [NSWorkspaceOpenConfiguration configuration];
			configuration.activates = YES;
			NSWorkspace *workspace = [NSWorkspace sharedWorkspace];
			if (target != nil) {
				[workspace openURLs:@[ target ] withApplicationAtURL:appURL configuration:configuration completionHandler:^(NSRunningApplication *app, NSError *error) {
					(void)app;
					(void)error;
				}];
			} else {
				[workspace openApplicationAtURL:appURL configuration:configuration completionHandler:^(NSRunningApplication *app, NSError *error) {
					(void)app;
					(void)error;
				}];
			}
		});
	}
	return BENCH_OK;
}

char *benchDefaultBrowser(void) {
	@autoreleasepool {
		// Any https address will do: LaunchServices chooses the app by
		// scheme. Like benchOpenApp's, this is a LaunchServices lookup,
		// which is safe on any thread, and so is NSBundle.
		NSURL *probe = [NSURL URLWithString:@"https://example.com/"];
		if (probe == nil) {
			return NULL;
		}
		NSURL *appURL = [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:probe];
		if (appURL == nil) {
			return NULL;
		}
		NSBundle *bundle = [NSBundle bundleWithURL:appURL];
		NSString *identifier = bundle.bundleIdentifier;
		if (identifier.length == 0) {
			return NULL;
		}
		// A browser picker, such as Finicky or Velja, is an agent: it has
		// no windows of its own to dock, and passes links on to a browser.
		for (NSString *key in @[ @"LSUIElement", @"LSBackgroundOnly" ]) {
			id value = [bundle objectForInfoDictionaryKey:key];
			if ([value respondsToSelector:@selector(boolValue)] && [value boolValue]) {
				return NULL;
			}
		}
		// A copy, since identifier goes with the pool.
		return benchCopyCString(identifier);
	}
}

int benchOpenTextFile(const char *path) {
	@autoreleasepool {
		NSString *filePath = benchString(path);
		if (filePath.length == 0) {
			return BENCH_ERR_FAILED;
		}
		NSURL *file = [NSURL fileURLWithPath:filePath];
		NSWorkspace *workspace = [NSWorkspace sharedWorkspace];
		// Like benchOpenApp's, these are LaunchServices lookups, which are
		// safe on any thread. The default text editor is the app that opens
		// plain text, whatever opens the file's own type: a .toml file may
		// have none, or an app that only shows it.
		NSURL *appURL = [workspace URLForApplicationToOpenContentType:UTTypePlainText];
		if (appURL == nil) {
			appURL = [workspace URLForApplicationWithBundleIdentifier:@"com.apple.TextEdit"];
		}
		if (appURL == nil) {
			return BENCH_ERR_NOT_INSTALLED;
		}
		benchOnMainIfRunning(^{
			NSWorkspaceOpenConfiguration *configuration = [NSWorkspaceOpenConfiguration configuration];
			configuration.activates = YES;
			[workspace openURLs:@[ file ] withApplicationAtURL:appURL configuration:configuration completionHandler:^(NSRunningApplication *app, NSError *error) {
				(void)app;
				(void)error;
			}];
		});
	}
	return BENCH_OK;
}

int benchShowInFinder(const char *path) {
	@autoreleasepool {
		NSString *filePath = benchString(path);
		if (filePath.length == 0) {
			return BENCH_ERR_FAILED;
		}
		NSURL *file = [NSURL fileURLWithPath:filePath];
		benchOnMain(^{
			[[NSWorkspace sharedWorkspace] activateFileViewerSelectingURLs:@[ file ]];
		});
	}
	return BENCH_OK;
}

// gChooser is the open panel that benchChooseToAdd shows, while it's open.
// Main thread only.
static NSOpenPanel *gChooser = nil;

int benchChooseToAdd(const char *start) {
	// The panel's answer comes on the main thread, which must be running
	// its run loop for the panel to show at all.
	if (benchStopped() || !benchMainRunning()) {
		return BENCH_ERR_FAILED;
	}
	@autoreleasepool {
		NSString *folder = benchString(start);
		benchOnMain(^{
			NSOpenPanel *panel = [NSOpenPanel openPanel];
			// A window of its own, which takes Bench's appearance, as the
			// drop guide does, rather than macOS's.
			panel.appearance = benchPanelAppearance();
			// An app is a package, which the panel chooses as a file.
			panel.canChooseFiles = YES;
			panel.canChooseDirectories = NO;
			panel.allowsMultipleSelection = NO;
			panel.resolvesAliases = YES;
			panel.treatsFilePackagesAsDirectories = NO;
			panel.prompt = @"Add";
			panel.message = @"Choose an app, or a program or script to run in a terminal.";
			if (folder.length > 0) {
				panel.directoryURL = [NSURL fileURLWithPath:folder isDirectory:YES];
			}
			gChooser = panel;
			benchActivateBench();
			// Not a sheet: begin shows the panel as a window of its own, which
			// stays in front of Bench's window, lowered as it is below the
			// docked windows, rather than going behind them with it.
			[panel beginWithCompletionHandler:^(NSModalResponse result) {
				@autoreleasepool {
					if (gChooser == panel) {
						gChooser = nil;
					}
					NSURL *chosen = result == NSModalResponseOK ? panel.URL : nil;
					// An app that LaunchServices hasn't seen, such as one in a
					// folder of its own, is registered, so that it opens by its
					// bundle ID.
					if (chosen != nil && [chosen.pathExtension caseInsensitiveCompare:@"app"] == NSOrderedSame) {
						LSRegisterURL((__bridge CFURLRef)chosen, false);
					}
					benchChoseToAdd(chosen != nil ? (char *)benchUTF8(chosen.path) : NULL);
				}
			}];
		});
	}
	return BENCH_OK;
}

void benchFrontChooser(void) {
	benchOnMain(^{
		if (gChooser != nil) {
			benchActivateBench();
			[gChooser makeKeyAndOrderFront:nil];
		}
	});
}

int benchHideApp(int32_t pid, int hide) {
	@autoreleasepool {
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		if (app == nil || app.terminated) {
			return BENCH_ERR_NOT_RUNNING;
		}
		// hide and unhide are requests, and on macOS 15 they return NO even
		// when the app does hide or show (seen on a real Mac: every call said
		// NO while the apps hid and showed as asked). So when they say NO,
		// also set the app's AXHidden attribute, which is synchronous and
		// reports real errors, and fail only if that fails too. Always send
		// the request: app.hidden changes only when the main run loop runs,
		// so it can be out of date just after an earlier hide or unhide.
		if (hide ? [app hide] : [app unhide]) {
			return BENCH_OK;
		}
		AXUIElementRef element = AXUIElementCreateApplication(pid);
		if (element == NULL) {
			return BENCH_ERR_FAILED;
		}
		AXUIElementSetMessagingTimeout(element, 1.0f);
		AXError err = AXUIElementSetAttributeValue(element, kAXHiddenAttribute, hide ? kCFBooleanTrue : kCFBooleanFalse);
		CFRelease(element);
		if (err == kAXErrorSuccess) {
			return BENCH_OK;
		}
		if ((hide != 0) == app.hidden) {
			return BENCH_OK; // Already as asked.
		}
		return BENCH_ERR_FAILED;
	}
}

#pragma mark - Icons

// benchIconQueue draws apps' icons one at a time, off the main thread:
// NSWorkspace's icons may be drawn on any thread, into a bitmap of one's
// own, but not by two threads at once.
static dispatch_queue_t benchIconQueue(void) {
	static dispatch_queue_t queue;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		queue = dispatch_queue_create("com.github.itbm.bench.icons", DISPATCH_QUEUE_SERIAL);
	});
	return queue;
}

int benchAppIcon(const char *bundleID, int pixels, void **png, int *length) {
	*png = NULL;
	*length = 0;
	if (pixels <= 0) {
		return BENCH_ERR_FAILED;
	}
	NSString *identifier = benchString(bundleID);
	__block int status = BENCH_ERR_FAILED;
	__block NSData *data = nil;
	dispatch_sync(benchIconQueue(), ^{
		@autoreleasepool {
			// A LaunchServices lookup, which is safe on any thread.
			NSURL *appURL = [[NSWorkspace sharedWorkspace] URLForApplicationWithBundleIdentifier:identifier];
			if (appURL == nil) {
				status = BENCH_ERR_NOT_INSTALLED;
				return;
			}
			NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:appURL.path];
			NSBitmapImageRep *bitmap = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL
			                                                                   pixelsWide:pixels
			                                                                   pixelsHigh:pixels
			                                                                bitsPerSample:8
			                                                              samplesPerPixel:4
			                                                                     hasAlpha:YES
			                                                                     isPlanar:NO
			                                                               colorSpaceName:NSCalibratedRGBColorSpace
			                                                                  bytesPerRow:0
			                                                                 bitsPerPixel:0];
			if (icon == nil || bitmap == nil) {
				return;
			}
			// One point a pixel, so that the icon chooses its image for the
			// pixels it fills, as it would on a Retina screen at half the size.
			bitmap.size = NSMakeSize(pixels, pixels);
			NSGraphicsContext *context = [NSGraphicsContext graphicsContextWithBitmapImageRep:bitmap];
			if (context == nil) {
				return;
			}
			[NSGraphicsContext saveGraphicsState];
			NSGraphicsContext.currentContext = context;
			context.imageInterpolation = NSImageInterpolationHigh;
			[icon drawInRect:NSMakeRect(0, 0, pixels, pixels) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
			[context flushGraphics];
			[NSGraphicsContext restoreGraphicsState];
			data = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
			status = data.length > 0 ? BENCH_OK : BENCH_ERR_FAILED;
		}
	});
	if (status != BENCH_OK) {
		return status;
	}
	void *copy = malloc(data.length);
	if (copy == NULL) {
		return BENCH_ERR_FAILED;
	}
	memcpy(copy, data.bytes, data.length);
	*png = copy;
	*length = (int)data.length;
	return BENCH_OK;
}
