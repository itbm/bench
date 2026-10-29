//go:build darwin && cgo && !server

// apps_darwin.m: running apps, launching them, hiding and showing them, the
// default browser, and opening files for the user, with NSWorkspace and
// NSRunningApplication.
//
// NSRunningApplication is thread-safe, and NSWorkspace's runningApplications
// may be read from any thread, so these run on the calling thread. Their
// values change only when the main run loop runs.

#import "internal_darwin.h"
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
