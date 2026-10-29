//go:build darwin && cgo && !server

// screens_darwin.m: displays, scroll bars, permissions, and facts about the
// process.

#import "internal_darwin.h"

#import <ServiceManagement/ServiceManagement.h>

#include <stdlib.h>
#include <string.h>

void benchCollectScreens(BOOL changed) {
	@autoreleasepool {
		// The first screen is the primary one, with the menu bar; its frame
		// starts at (0, 0) in AppKit coordinates.
		NSArray<NSScreen *> *screens = [NSScreen screens];
		NSUInteger count = screens.count;
		BenchScreenInfo *infos = calloc(count > 0 ? count : 1, sizeof(BenchScreenInfo));
		if (infos == NULL) {
			return;
		}
		for (NSUInteger i = 0; i < count; i++) {
			NSScreen *screen = screens[i];
			NSNumber *number = screen.deviceDescription[@"NSScreenNumber"];
			NSRect frame = screen.frame;
			NSRect visible = screen.visibleFrame;
			BenchScreenInfo *info = &infos[i];
			info->displayID = [number isKindOfClass:[NSNumber class]] ? number.unsignedIntValue : (uint32_t)(i + 1);
			// A copy, freed below: localizedName may make a new string each
			// time, which ARC releases at the end of this statement, taking
			// its UTF-8 buffer with it before Go reads the name.
			info->name = benchCopyCString(screen.localizedName ?: @"");
			info->frameX = frame.origin.x;
			info->frameY = frame.origin.y;
			info->frameW = frame.size.width;
			info->frameH = frame.size.height;
			info->visibleX = visible.origin.x;
			info->visibleY = visible.origin.y;
			info->visibleW = visible.size.width;
			info->visibleH = visible.size.height;
		}
		benchScreensUpdated(infos, (int)count, changed ? 1 : 0);
		for (NSUInteger i = 0; i < count; i++) {
			free(infos[i].name);
		}
		free(infos);
	}
}

void benchRefreshScreens(void) {
	// Go needs the screens at once, so read them here even off the main
	// thread (NSScreen is read-only here, and nothing else uses AppKit
	// yet), then again on the main thread as soon as it runs.
	benchCollectScreens(NO);
	if (![NSThread isMainThread]) {
		dispatch_async(dispatch_get_main_queue(), ^{
			benchCollectScreens(NO);
		});
	}
}

void benchCollectScrollBars(void) {
	// Legacy scroll bars are always shown; overlay ones only while
	// scrolling. macOS prefers legacy ones when System Settings' Show scroll
	// bars is Always, or when it's automatic and a mouse without a trackpad
	// is connected.
	benchScrollBarsUpdated([NSScroller preferredScrollerStyle] == NSScrollerStyleLegacy ? 1 : 0);
}

int benchScreenRecordingAllowed(int request) {
	if (CGPreflightScreenCaptureAccess()) {
		return 1;
	}
	if (request) {
		// Shows the system prompt (or, after the first time, nothing) and
		// returns at once; the answer applies after Bench restarts.
		return CGRequestScreenCaptureAccess() ? 1 : 0;
	}
	return 0;
}

int benchOpenSettingsURL(const char *url) {
	@autoreleasepool {
		NSURL *address = [NSURL URLWithString:benchString(url)];
		if (address == nil) {
			return BENCH_ERR_BAD_URL;
		}
		// Like benchOpenTextFile's, this is a LaunchServices call, safe on
		// any thread, but it runs on the main thread when it can.
		benchOnMainIfRunning(^{
			[[NSWorkspace sharedWorkspace] openURL:address];
		});
	}
	return BENCH_OK;
}

void benchOpenLoginItems(void) {
	benchOnMainIfRunning(^{
		[SMAppService openSystemSettingsLoginItems];
	});
}

int benchLoginItemNeedsApproval(void) {
	// Open at login uses SMAppService only for a real .app bundle (Wails'
	// Autostart), and a bare binary has no main app service to ask about.
	if (!benchHasBundleID()) {
		return 0;
	}
	@autoreleasepool {
		return [SMAppService mainAppService].status == SMAppServiceStatusRequiresApproval ? 1 : 0;
	}
}

int benchOSMajorVersion(void) {
	return (int)[NSProcessInfo processInfo].operatingSystemVersion.majorVersion;
}

int benchHasBundleID(void) {
	// A bare binary can carry an Info.plist (linked into its __TEXT segment)
	// and so have a bundle identifier, but UserNotifications still throws
	// for it: only a real .app bundle counts.
	NSBundle *bundle = [NSBundle mainBundle];
	BOOL isApp = [bundle.bundleURL.pathExtension.lowercaseString isEqualToString:@"app"];
	return bundle.bundleIdentifier.length > 0 && isApp ? 1 : 0;
}
