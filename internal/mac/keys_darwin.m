//go:build darwin && cgo && !server

// keys_darwin.m: the keys that macOS keeps for itself, which Bench's global
// shortcuts may clash with without macOS saying so. RegisterEventHotKey
// lets Bench take a shortcut that switches input sources, or that VoiceOver
// uses, but the key press may never reach Bench. Go compares them with
// Bench's shortcuts (hotkeys.go, and internal/ui). The modifier keys held
// down now, for the hold after a global shortcut. And a window tool's
// preferences, for its shortcuts, which can take Bench's without macOS
// saying so either (toolprefs.go, decision 69).

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

int benchHeldModifiers(void) {
	// The class property reads the modifier keys' state now, from the
	// window server, wherever the keyboard is. It needs no event monitor,
	// so no permission.
	NSEventModifierFlags flags = [NSEvent modifierFlags];
	int held = 0;
	if (flags & NSEventModifierFlagControl) {
		held |= BENCH_MOD_CONTROL;
	}
	if (flags & NSEventModifierFlagOption) {
		held |= BENCH_MOD_OPTION;
	}
	if (flags & NSEventModifierFlagShift) {
		held |= BENCH_MOD_SHIFT;
	}
	if (flags & NSEventModifierFlagCommand) {
		held |= BENCH_MOD_COMMAND;
	}
	return held;
}

int benchCopyToolPreferences(const char *bundleID, BenchToolPreference **out, int *count) {
	*out = NULL;
	*count = 0;
	@autoreleasepool {
		NSString *domain = benchString(bundleID);
		// The tool writes them, in another process, so read them afresh
		// rather than from this process's cache.
		CFPreferencesAppSynchronize((__bridge CFStringRef)domain);
		id copied = CFBridgingRelease(CFPreferencesCopyMultiple(NULL, (__bridge CFStringRef)domain, kCFPreferencesCurrentUser, kCFPreferencesAnyHost));
		if (![copied isKindOfClass:[NSDictionary class]] || [(NSDictionary *)copied count] == 0) {
			return BENCH_OK;
		}
		NSDictionary *values = (NSDictionary *)copied;
		BenchToolPreference *prefs = calloc(values.count, sizeof(BenchToolPreference));
		if (prefs == NULL) {
			return BENCH_ERR_FAILED;
		}
		int n = 0;
		for (id key in values) {
			if (![key isKindOfClass:[NSString class]]) {
				continue;
			}
			id value = [values objectForKey:key];
			BenchToolPreference *pref = &prefs[n];
			if ([value isKindOfClass:[NSNumber class]]) {
				pref->kind = BENCH_PREF_NUMBER;
				pref->number = [value longLongValue];
			} else if ([value isKindOfClass:[NSString class]]) {
				pref->kind = BENCH_PREF_TEXT;
				pref->text = benchCopyCString(value);
			} else if ([value isKindOfClass:[NSDictionary class]]) {
				// MASShortcut, which Rectangle uses, keeps a shortcut as its
				// key code and modifier flags, and one set to none as an
				// empty dictionary. It reads any dictionary without both as
				// no shortcut, as Bench does (MASDictionaryTransformer).
				NSDictionary *shortcut = (NSDictionary *)value;
				id code = [shortcut objectForKey:@"keyCode"];
				id flags = [shortcut objectForKey:@"modifierFlags"];
				if ([code respondsToSelector:@selector(integerValue)] && [flags respondsToSelector:@selector(integerValue)]) {
					pref->kind = BENCH_PREF_SHORTCUT;
					pref->keyCode = (int)[code integerValue];
					pref->number = (long long)[flags integerValue];
				} else {
					pref->kind = BENCH_PREF_NO_SHORTCUT;
				}
			} else {
				// Data, a date, or a list: under a shortcut's key, MASShortcut
				// reads it as no shortcut, which overrides the tool's default.
				pref->kind = BENCH_PREF_OTHER;
			}
			pref->key = benchCopyCString((NSString *)key);
			n++;
		}
		*out = prefs;
		*count = n;
	}
	return BENCH_OK;
}

void benchFreeToolPreferences(BenchToolPreference *prefs, int count) {
	if (prefs == NULL) {
		return;
	}
	for (int i = 0; i < count; i++) {
		free(prefs[i].key);
		free(prefs[i].text);
	}
	free(prefs);
}
