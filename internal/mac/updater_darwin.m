//go:build darwin && cgo && !server

// updater_darwin.m: Sparkle, which installs new versions of Bench.
//
// make app puts Sparkle.framework in Bench.app only when it's given an
// update key, so Bench loads it at run time, with NSBundle, rather than
// linking it: a Bench.app without it, and a bare binary, run as before.
// Without Sparkle's headers, the few parts of its API that Bench uses are
// declared here as protocols, and its classes are found by name.
//
// Sparkle's standard user driver shows its own windows: the new version
// with its release notes, the download, and Install and Relaunch. Bench's
// delegate takes over only the news of a version that a daily look finds
// (Sparkle's gentle reminders), which goes to Go for the status bar's
// notice, so that Sparkle never interrupts with a window of its own. The
// notice's Install… shows Sparkle's window for that version.
//
// Sparkle runs on the main thread. Go calls these functions from any
// thread; each sends its work to the main thread and returns at once.

#import "internal_darwin.h"

NS_ASSUME_NONNULL_BEGIN

// SPUUpdater: the updater.
@protocol BenchSparkleUpdater <NSObject>
@property (nonatomic) BOOL automaticallyChecksForUpdates;
- (BOOL)startUpdater:(NSError *__autoreleasing *)error;
@end

// SPUStandardUpdaterController: the updater, with Sparkle's standard user
// driver, which shows its windows.
@protocol BenchSparkleController <NSObject>
- (instancetype)initWithStartingUpdater:(BOOL)startUpdater updaterDelegate:(nullable id)updaterDelegate userDriverDelegate:(nullable id)userDriverDelegate;
@property (nonatomic, readonly) id<BenchSparkleUpdater> updater;
- (void)checkForUpdates:(nullable id)sender;
@end

// SUAppcastItem: a version in the appcast.
@protocol BenchSparkleItem <NSObject>
@property (nonatomic, readonly, copy) NSString *displayVersionString;
@end

// Sparkle's error code for a look that found no newer version, which isn't
// a failure (SUNoUpdateError).
static const NSInteger BenchSparkleNoUpdate = 1001;

// BenchUpdaterDelegate is Sparkle's updater delegate and its standard user
// driver's delegate.
@interface BenchUpdaterDelegate : NSObject
@end

@implementation BenchUpdaterDelegate

// check_for_updates decides whether Sparkle looks each day, so Sparkle
// never asks. Info.plist's SUEnableAutomaticChecks says so too.
- (BOOL)updaterShouldPromptForPermissionToCheckForUpdates:(id)updater {
	(void)updater;
	return NO;
}

// A look that failed is only logged, as Go's own look is. One that found
// nothing newer isn't a failure.
- (void)updater:(id)updater didFinishUpdateCycleForUpdateCheck:(NSInteger)updateCheck error:(nullable NSError *)error {
	(void)updater;
	(void)updateCheck;
	if (error == nil || error.code == BenchSparkleNoUpdate) {
		return;
	}
	@autoreleasepool {
		NSString *text = [NSString stringWithFormat:@"%@ (%@ %ld)", error.localizedDescription, error.domain, (long)error.code];
		NSError *cause = error.userInfo[NSUnderlyingErrorKey];
		if ([cause isKindOfClass:[NSError class]]) {
			text = [text stringByAppendingFormat:@": %@", cause.localizedDescription];
		}
		benchUpdateFailed((char *)benchUTF8(text));
	}
}

- (BOOL)supportsGentleScheduledUpdateReminders {
	return YES;
}

// Bench, not Sparkle, shows the news of a version that a daily look found.
// Sparkle still shows what a look that the user asked for finds.
- (BOOL)standardUserDriverShouldHandleShowingScheduledUpdate:(id)update andInImmediateFocus:(BOOL)immediateFocus {
	(void)update;
	(void)immediateFocus;
	return NO;
}

- (void)standardUserDriverWillHandleShowingUpdate:(BOOL)handleShowingUpdate forUpdate:(id<BenchSparkleItem>)update state:(id)state {
	(void)state;
	if (handleShowingUpdate) {
		return;
	}
	@autoreleasepool {
		benchUpdateFound((char *)benchUTF8(update.displayVersionString));
	}
}

- (void)standardUserDriverWillFinishUpdateSession {
	benchUpdateFinished();
}

@end

// The controller and its delegate, which Sparkle holds weakly. Main thread
// only.
static id<BenchSparkleController> benchSparkle = nil;
static BenchUpdaterDelegate *benchSparkleDelegate = nil;

// benchSparkleBundle returns Bench.app's Sparkle.framework, or nil if it
// has none, or no update key to check updates with.
static NSBundle *_Nullable benchSparkleBundle(void) {
	NSBundle *app = [NSBundle mainBundle];
	id key = [app objectForInfoDictionaryKey:@"SUPublicEDKey"];
	if (!benchHasBundleID() || ![key isKindOfClass:[NSString class]] || [key length] == 0) {
		return nil;
	}
	NSString *path = [app.privateFrameworksPath stringByAppendingPathComponent:@"Sparkle.framework"];
	if (path == nil || ![[NSFileManager defaultManager] fileExistsAtPath:path]) {
		return nil;
	}
	return [NSBundle bundleWithPath:path];
}

int benchUpdaterAvailable(void) {
	@autoreleasepool {
		return benchSparkleBundle() != nil ? 1 : 0;
	}
}

void benchUpdaterStart(int automatic) {
	benchOnMain(^{
		if (benchSparkle != nil) {
			return;
		}
		NSBundle *bundle = benchSparkleBundle();
		NSError *error = nil;
		if (bundle == nil || ![bundle loadAndReturnError:&error]) {
			benchUpdateFailed((char *)benchUTF8([NSString stringWithFormat:@"couldn't load Sparkle: %@", error.localizedDescription ?: @"Bench.app has no Sparkle.framework"]));
			return;
		}
		Class controllerClass = NSClassFromString(@"SPUStandardUpdaterController");
		if (controllerClass == Nil) {
			benchUpdateFailed("couldn't find Sparkle's SPUStandardUpdaterController");
			return;
		}
		benchSparkleDelegate = [[BenchUpdaterDelegate alloc] init];
		benchSparkle = [(id<BenchSparkleController>)[controllerClass alloc] initWithStartingUpdater:NO updaterDelegate:benchSparkleDelegate userDriverDelegate:benchSparkleDelegate];
		id<BenchSparkleUpdater> updater = benchSparkle.updater;
		// Before Sparkle starts, so that it doesn't schedule a look that the
		// setting doesn't allow.
		if (updater.automaticallyChecksForUpdates != (automatic != 0)) {
			updater.automaticallyChecksForUpdates = automatic != 0;
		}
		// The updater, not the controller, so that a failure is logged, not
		// shown in an alert at startup.
		if (![updater startUpdater:&error]) {
			benchUpdateFailed((char *)benchUTF8([NSString stringWithFormat:@"couldn't start Sparkle: %@", error.localizedDescription]));
		}
	});
}

void benchUpdaterSetAutomatic(int automatic) {
	benchOnMain(^{
		id<BenchSparkleUpdater> updater = benchSparkle.updater;
		// Setting it, even to what it is, makes Sparkle plan its next look
		// again.
		if (updater != nil && updater.automaticallyChecksForUpdates != (automatic != 0)) {
			updater.automaticallyChecksForUpdates = automatic != 0;
		}
	});
}

void benchUpdaterCheck(void) {
	benchOnMain(^{
		[benchSparkle checkForUpdates:nil];
	});
}

NS_ASSUME_NONNULL_END
