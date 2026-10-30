//go:build darwin && cgo && !server

// keys_darwin.m: the keys that macOS keeps for itself, which Bench's global
// shortcuts may clash with without macOS saying so. RegisterEventHotKey
// lets Bench take a shortcut that switches input sources, or that VoiceOver
// uses, but the key press may never reach Bench. Go compares them with
// Bench's shortcuts (hotkeys.go, and internal/ui).

#import "internal_darwin.h"

#import <Carbon/Carbon.h>

// The input sources' symbolic hot keys in com.apple.symbolichotkeys:
// Select the previous input source, and Select next source in Input menu.
static NSString *const BenchPreviousSourceKey = @"60";
static NSString *const BenchNextSourceKey = @"61";

static void *BenchVoiceOverContext = &BenchVoiceOverContext;

// BenchVoiceOverObserver hears NSWorkspace's voiceOverEnabled change.
@interface BenchVoiceOverObserver : NSObject
@end

@implementation BenchVoiceOverObserver

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchVoiceOverContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	// KVO may tell on any thread.
	benchOnMain(^{
		benchCollectSystemKeys();
	});
}

@end

// Main thread only.
static BenchVoiceOverObserver *gVoiceOverObserver = nil;
static id gInputSourcesToken = nil;

// benchHotKeyFrom reads one of the symbolic hot keys. One that the user has
// never changed may be missing, and then has macOS's default, which is
// turned on: Space with defaultFlags.
static BenchHotKeyInfo benchHotKeyFrom(NSDictionary *hotKeys, NSString *key, int defaultFlags) {
	BenchHotKeyInfo info = {1, kVK_Space, defaultFlags};
	id entry = hotKeys[key];
	if (![entry isKindOfClass:[NSDictionary class]]) {
		return info;
	}
	id enabled = entry[@"enabled"];
	if ([enabled respondsToSelector:@selector(boolValue)]) {
		info.enabled = [enabled boolValue] ? 1 : 0;
	}
	id value = entry[@"value"];
	id parameters = [value isKindOfClass:[NSDictionary class]] ? value[@"parameters"] : nil;
	// The parameters are the character (or 65535), the virtual key code,
	// and the modifier flags.
	if ([parameters isKindOfClass:[NSArray class]] && [parameters count] >= 3) {
		id code = parameters[1];
		id flags = parameters[2];
		if ([code respondsToSelector:@selector(intValue)] && [flags respondsToSelector:@selector(intValue)]) {
			info.keyCode = [code intValue];
			info.flags = [flags intValue];
		}
	}
	return info;
}

// benchSelectableInputSources returns how many keyboard input sources the
// user can switch between: the keyboard layouts and input methods that are
// turned on and can be chosen. Main thread only.
static int benchSelectableInputSources(void) {
	NSDictionary *filter = @{
		(__bridge NSString *)kTISPropertyInputSourceCategory: (__bridge NSString *)kTISCategoryKeyboardInputSource,
		(__bridge NSString *)kTISPropertyInputSourceIsSelectCapable: @YES,
	};
	CFArrayRef sources = TISCreateInputSourceList((__bridge CFDictionaryRef)filter, false);
	if (sources == NULL) {
		return 0;
	}
	int count = (int)CFArrayGetCount(sources);
	CFRelease(sources);
	return count;
}

void benchCollectSystemKeys(void) {
	@autoreleasepool {
		// Another process, System Settings, writes these, so read them
		// afresh rather than from this process's cache.
		CFPreferencesAppSynchronize(CFSTR("com.apple.symbolichotkeys"));
		id hotKeys = CFBridgingRelease(CFPreferencesCopyAppValue(CFSTR("AppleSymbolicHotKeys"), CFSTR("com.apple.symbolichotkeys")));
		if (![hotKeys isKindOfClass:[NSDictionary class]]) {
			hotKeys = @{};
		}
		BenchHotKeyInfo keys[2] = {
			benchHotKeyFrom(hotKeys, BenchPreviousSourceKey, (int)NSEventModifierFlagControl),
			benchHotKeyFrom(hotKeys, BenchNextSourceKey, (int)(NSEventModifierFlagControl | NSEventModifierFlagOption)),
		};
		int voiceOver = [[NSWorkspace sharedWorkspace] isVoiceOverEnabled] ? 1 : 0;
		benchSystemKeysUpdated(keys, 2, benchSelectableInputSources(), voiceOver);
	}
}

void benchWatchSystemKeys(void) {
	if (gVoiceOverObserver == nil) {
		gVoiceOverObserver = [[BenchVoiceOverObserver alloc] init];
		[[NSWorkspace sharedWorkspace] addObserver:gVoiceOverObserver forKeyPath:@"voiceOverEnabled" options:0 context:BenchVoiceOverContext];
	}
	// Turning an input source on or off can make the input sources'
	// shortcuts start or stop doing something.
	if (gInputSourcesToken == nil) {
		gInputSourcesToken = [[NSDistributedNotificationCenter defaultCenter] addObserverForName:(__bridge NSString *)kTISNotifyEnabledKeyboardInputSourcesChanged object:nil queue:nil usingBlock:^(NSNotification *note) {
			(void)note;
			benchOnMain(^{
				benchCollectSystemKeys();
			});
		}];
	}
}

void benchUnwatchSystemKeys(void) {
	if (gVoiceOverObserver != nil) {
		[[NSWorkspace sharedWorkspace] removeObserver:gVoiceOverObserver forKeyPath:@"voiceOverEnabled" context:BenchVoiceOverContext];
		gVoiceOverObserver = nil;
	}
	if (gInputSourcesToken != nil) {
		[[NSDistributedNotificationCenter defaultCenter] removeObserver:gInputSourcesToken];
		gInputSourcesToken = nil;
	}
}
