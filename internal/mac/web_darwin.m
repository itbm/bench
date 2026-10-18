//go:build darwin && cgo && !server

// web_darwin.m: Web items' web views. Each is a WKWebView in Bench's window,
// above the UI's own web view, with a persistent data store per profile, a
// navigation policy that keeps the item on its site, popups for sign-in,
// downloads, and a bridge for the page's notifications.
//
// Main thread only, apart from the entry points, which hop there.

#import "internal_darwin.h"

// The name of the script message handler that the notification script
// posts to.
static NSString *const BenchNotifyHandler = @"benchNotify";

// The script added to every Web item's pages at document start. Web Push
// isn't available in WKWebView, so it replaces window.Notification, and
// service workers' showNotification, with versions that forward the title
// and body to Bench. Pages see permission as granted.
static NSString *const BenchNotificationScript =
	@"(() => {\n"
	@"  const handler = window.webkit?.messageHandlers?.benchNotify;\n"
	@"  if (!handler || window.Notification?.benchBridge) { return; }\n"
	@"  const post = (title, options) => {\n"
	@"    try {\n"
	@"      handler.postMessage({ title: String(title ?? ''), body: String(options?.body ?? '') });\n"
	@"    } catch (e) {}\n"
	@"  };\n"
	@"  class BenchNotification extends EventTarget {\n"
	@"    constructor(title, options = {}) {\n"
	@"      super();\n"
	@"      this.title = String(title);\n"
	@"      this.body = String(options.body ?? '');\n"
	@"      this.tag = String(options.tag ?? '');\n"
	@"      this.icon = String(options.icon ?? '');\n"
	@"      this.data = options.data ?? null;\n"
	@"      this.onclick = null;\n"
	@"      this.onshow = null;\n"
	@"      this.onclose = null;\n"
	@"      this.onerror = null;\n"
	@"      post(this.title, options);\n"
	@"      setTimeout(() => this.benchFire('show'), 0);\n"
	@"    }\n"
	@"    benchFire(type) {\n"
	@"      const event = new Event(type);\n"
	@"      const listener = this['on' + type];\n"
	@"      if (typeof listener === 'function') { listener.call(this, event); }\n"
	@"      this.dispatchEvent(event);\n"
	@"    }\n"
	@"    close() { this.benchFire('close'); }\n"
	@"    static get permission() { return 'granted'; }\n"
	@"    static requestPermission(callback) {\n"
	@"      if (typeof callback === 'function') { callback('granted'); }\n"
	@"      return Promise.resolve('granted');\n"
	@"    }\n"
	@"  }\n"
	@"  BenchNotification.benchBridge = true;\n"
	@"  BenchNotification.maxActions = 0;\n"
	@"  Object.defineProperty(window, 'Notification', { value: BenchNotification, writable: true, configurable: true });\n"
	@"  if (window.ServiceWorkerRegistration?.prototype) {\n"
	@"    ServiceWorkerRegistration.prototype.showNotification = function (title, options) {\n"
	@"      post(title, options);\n"
	@"      return Promise.resolve();\n"
	@"    };\n"
	@"    ServiceWorkerRegistration.prototype.getNotifications = function () { return Promise.resolve([]); };\n"
	@"  }\n"
	@"  const permissions = navigator.permissions;\n"
	@"  if (permissions?.query) {\n"
	@"    const query = permissions.query.bind(permissions);\n"
	@"    permissions.query = (descriptor) => descriptor?.name === 'notifications'\n"
	@"      ? Promise.resolve({ name: 'notifications', state: 'granted', onchange: null,\n"
	@"          addEventListener() {}, removeEventListener() {} })\n"
	@"      : query(descriptor);\n"
	@"  }\n"
	@"})();\n";

static void *BenchKVOContext = &BenchKVOContext;

// The longest message a page's alert or confirmation shows, in characters.
static const NSUInteger BenchDialogMaxLength = 2000;

// A page that asks for another dialog within this many seconds of the last
// one closing is offered a way to stop them, so that one asking in a loop
// can't keep Bench behind dialogs.
static const CFAbsoluteTime BenchDialogRepeat = 5;

// A page whose process crashes again within this many seconds of Bench
// reloading it after the last crash is reloaded only after a delay, which
// starts at BenchCrashDelay and doubles with each crash, up to
// BenchCrashMaxDelay, so that a page that crashes every time it loads
// doesn't reload without end. The window is measured from the reload, not
// the crash, so that waiting the longest delay doesn't start it all again.
static const CFAbsoluteTime BenchCrashWindow = 60;
static const NSTimeInterval BenchCrashDelay = 2;
static const NSTimeInterval BenchCrashMaxDelay = 60;

@class BenchWebItem;

// BenchScriptHandler forwards the notification script's messages to its
// item. It holds the item weakly: the user content controller keeps its
// handlers, and the item owns the web view that owns the controller.
@interface BenchScriptHandler : NSObject <WKScriptMessageHandler>
@property (nonatomic, weak) BenchWebItem *item;
@end

// BenchPopup is a window that a Web item's page opened, for sign-in or a
// page on the item's site: a real window, so window.opener works.
@interface BenchPopup : NSObject <WKUIDelegate, WKNavigationDelegate, NSWindowDelegate>
@property (nonatomic, weak) BenchWebItem *owner;
@property (nonatomic, strong) NSWindow *window;
@property (nonatomic, strong) WKWebView *webView;
// The first page it showed, for its navigation policy (see
// DecidePopupNavigation): the address it was opened for, or the first one it
// went to after opening blank.
@property (nonatomic, copy) NSString *startURL;
// Whether it has shown a page other than a blank one.
@property (nonatomic) BOOL committed;
@property (nonatomic) BOOL observing;
- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner startURL:(NSString *)startURL;
- (void)show;
- (void)close;
@end

// BenchDialog is a page's alert or confirmation, waiting to be shown.
@interface BenchDialog : NSObject
@property (nonatomic, copy) NSString *message;
// The host of the frame that asked, so the dialog says who's asking.
@property (nonatomic, copy) NSString *host;
@property (nonatomic) BOOL confirm;
// Called once, with whether the user chose OK.
@property (nonatomic, copy) void (^done)(BOOL accepted);
@end

@implementation BenchDialog
@end

// BenchWebItem is a Web item's web view and everything around it.
@interface BenchWebItem : NSObject <WKNavigationDelegate, WKUIDelegate, NSWindowDelegate>
@property (nonatomic, copy) NSString *itemID;
@property (nonatomic, copy) NSString *profileUUID;
// The address Go last asked for: the item's own site, for the navigation
// policy, even after the page moves on.
@property (nonatomic, copy) NSString *requestedURL;
@property (nonatomic, strong) WKWebView *webView;
@property (nonatomic, strong) WKUserContentController *contentController;
@property (nonatomic, strong) BenchScriptHandler *scriptHandler;
@property (nonatomic, strong) NSMutableArray<BenchPopup *> *popups;
// The window it's popped out into, or nil.
@property (nonatomic, strong) NSWindow *popOutWindow;
@property (nonatomic) BOOL closingPopOut;
// Where Go last showed it, in content coordinates, and whether it's shown.
@property (nonatomic) NSRect frame;
@property (nonatomic) BOOL visible;
@property (nonatomic) BOOL observing;
// The page's dialog that waits for the page to be seen, the one showing,
// when the last one closed, and whether the user stopped the page's
// dialogs (until it goes to another page).
@property (nonatomic, strong, nullable) BenchDialog *waitingDialog;
@property (nonatomic) BOOL showingDialog;
@property (nonatomic) CFAbsoluteTime lastDialogEnded;
@property (nonatomic) BOOL dialogsStopped;
// Whether the page's process crashed and the page is still to be loaded
// again, whether that's waiting for a delay, how many times it has crashed
// in a row (each within BenchCrashWindow of being reloaded after the last),
// when it last did, and when Bench last reloaded it after a crash (0 for
// not since it was loaded).
@property (nonatomic) BOOL needsReload;
@property (nonatomic) BOOL reloadScheduled;
@property (nonatomic) NSUInteger crashCount;
@property (nonatomic) CFAbsoluteTime lastCrash;
@property (nonatomic) CFAbsoluteTime crashReloaded;
- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix;
- (void)loadAddress:(NSString *)address;
- (void)returnFromPopOut;
- (void)tearDown;
- (void)showWaitingDialog;
- (void)reloadAfterCrash;
- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features;
@end

#pragma mark - Helpers

// benchIsWebURL reports whether an address is an http or https one.
static BOOL benchIsWebURL(NSURL *_Nullable url) {
	NSString *scheme = url.scheme.lowercaseString;
	return [scheme isEqualToString:@"http"] || [scheme isEqualToString:@"https"];
}

// benchOriginString writes a security origin as an address, such as
// https://app.slack.com, or returns @"" for an origin without a host.
static NSString *benchOriginString(WKSecurityOrigin *_Nullable origin) {
	NSString *protocol = origin.protocol ?: @"";
	NSString *host = origin.host ?: @"";
	if (protocol.length == 0 || host.length == 0) {
		return @"";
	}
	if ([host containsString:@":"]) {
		host = [NSString stringWithFormat:@"[%@]", host]; // IPv6.
	}
	if (origin.port > 0) {
		return [NSString stringWithFormat:@"%@://%@:%ld", protocol, host, (long)origin.port];
	}
	return [NSString stringWithFormat:@"%@://%@", protocol, host];
}

// benchSourceHost names the site that started a navigation, for asking the
// user about it.
static NSString *benchSourceHost(WKNavigationAction *action, WKWebView *webView) {
	NSString *host = action.sourceFrame.securityOrigin.host;
	if (host.length == 0) {
		host = webView.URL.host;
	}
	return host ?: @"";
}

// benchOpenAfterAsking asks the user whether to open an address for another
// app, naming the app and the site that asked, and opens it if they agree.
// It asks nothing for an address that no app on this Mac opens. The alert
// is app-modal, so it shows above docked windows, and it waits for the
// page's click to finish first.
static void benchOpenAfterAsking(NSURL *_Nullable url, NSString *source) {
	if (url == nil) {
		return;
	}
	NSURL *app = [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:url];
	if (app == nil) {
		return;
	}
	NSString *appName = [[NSFileManager defaultManager] displayNameAtPath:app.path];
	NSString *from = source.length > 0 ? source : @"This page";
	NSString *scheme = url.scheme ?: @"";
	dispatch_async(dispatch_get_main_queue(), ^{
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = [NSString stringWithFormat:@"Open this link in %@?", appName];
		alert.informativeText = [NSString stringWithFormat:@"%@ wants to open a %@: link.", from, scheme];
		[alert addButtonWithTitle:@"Open"];
		[alert addButtonWithTitle:@"Cancel"];
		if ([alert runModal] == NSAlertFirstButtonReturn) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
	});
}

// benchApplyNavigation carries out a navigation decision that Go made.
static void benchApplyNavigation(int decision, NSURL *_Nullable url, NSString *source, void (^decisionHandler)(WKNavigationActionPolicy)) {
	switch (decision) {
	case BENCH_NAV_IN_VIEW:
		decisionHandler(WKNavigationActionPolicyAllow);
		break;
	case BENCH_NAV_IN_BROWSER:
		decisionHandler(WKNavigationActionPolicyCancel);
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		break;
	case BENCH_NAV_ASK:
		decisionHandler(WKNavigationActionPolicyCancel);
		benchOpenAfterAsking(url, source);
		break;
	default:
		decisionHandler(WKNavigationActionPolicyCancel);
		break;
	}
}

#pragma mark - Downloads

// BenchDownloads saves Web items' downloads to the Downloads folder.
@interface BenchDownloads : NSObject <WKDownloadDelegate>
// Where each download in progress is going: the path Go chose for it.
@property (nonatomic, strong) NSMapTable<WKDownload *, NSString *> *destinations;
@end

// benchQuarantine marks a downloaded file as coming from the web, as
// browsers do, so that Gatekeeper checks it before it runs, unless it's
// marked already.
static void benchQuarantine(NSURL *file, NSURL *_Nullable data, NSURL *_Nullable origin) {
	id existing = nil;
	if ([file getResourceValue:&existing forKey:NSURLQuarantinePropertiesKey error:NULL] && existing != nil) {
		return;
	}
	NSMutableDictionary *properties = [NSMutableDictionary dictionary];
	properties[(__bridge NSString *)kLSQuarantineAgentNameKey] = @"Bench";
	properties[(__bridge NSString *)kLSQuarantineTypeKey] = (__bridge NSString *)kLSQuarantineTypeWebDownload;
	if (data != nil) {
		properties[(__bridge NSString *)kLSQuarantineDataURLKey] = data;
	}
	if (origin != nil) {
		properties[(__bridge NSString *)kLSQuarantineOriginURLKey] = origin;
	}
	[file setResourceValue:properties forKey:NSURLQuarantinePropertiesKey error:NULL];
}

@implementation BenchDownloads

- (instancetype)init {
	self = [super init];
	if (self) {
		_destinations = [NSMapTable strongToStrongObjectsMapTable];
	}
	return self;
}

- (void)download:(WKDownload *)download decideDestinationUsingResponse:(NSURLResponse *)response suggestedFilename:(NSString *)suggestedFilename completionHandler:(void (^)(NSURL *_Nullable destination))completionHandler {
	(void)response;
	NSURL *folder = [[NSFileManager defaultManager] URLsForDirectory:NSDownloadsDirectory inDomains:NSUserDomainMask].firstObject;
	char *path = folder != nil ? benchDownloadPath((char *)benchUTF8(folder.path), (char *)benchUTF8(suggestedFilename)) : NULL;
	NSString *chosen = path != NULL ? benchString(path) : @"";
	free(path);
	if (chosen.length == 0) {
		completionHandler(nil); // Cancels it.
		return;
	}
	[self.destinations setObject:chosen forKey:download];
	completionHandler([NSURL fileURLWithPath:chosen]);
}

- (void)downloadDidFinish:(WKDownload *)download {
	NSString *path = [self.destinations objectForKey:download];
	[self.destinations removeObjectForKey:download];
	if (path == nil) {
		return;
	}
	benchDownloadEnded((char *)benchUTF8(path));
	benchQuarantine([NSURL fileURLWithPath:path], download.originalRequest.URL, download.webView.URL);
	// Bounces the Downloads stack in the Dock, as browsers do.
	[[NSDistributedNotificationCenter defaultCenter] postNotificationName:@"com.apple.DownloadFileFinished" object:path];
}

- (void)download:(WKDownload *)download didFailWithError:(NSError *)error resumeData:(NSData *)resumeData {
	(void)error;
	(void)resumeData;
	NSString *path = [self.destinations objectForKey:download];
	[self.destinations removeObjectForKey:download];
	if (path != nil) {
		benchDownloadEnded((char *)benchUTF8(path));
	}
}

@end

// benchDownloads returns the downloads' delegate, which a download holds
// weakly.
static BenchDownloads *benchDownloads(void) {
	static BenchDownloads *downloads = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		downloads = [[BenchDownloads alloc] init];
	});
	return downloads;
}

// benchResponsePolicy shows a response, or downloads it (see
// DecideResponse). itemURL is the Web item's own address.
static WKNavigationResponsePolicy benchResponsePolicy(WKNavigationResponse *navigationResponse, NSString *itemURL) {
	NSString *disposition = @"";
	NSURLResponse *response = navigationResponse.response;
	if ([response isKindOfClass:[NSHTTPURLResponse class]]) {
		disposition = [(NSHTTPURLResponse *)response valueForHTTPHeaderField:@"Content-Disposition"] ?: @"";
	}
	NSString *address = response.URL.absoluteString ?: @"";
	int decision = benchDecideResponse((char *)benchUTF8(itemURL), (char *)benchUTF8(address), navigationResponse.forMainFrame ? 1 : 0,
	                                   navigationResponse.canShowMIMEType ? 1 : 0, (char *)benchUTF8(disposition));
	return decision == BENCH_RESPONSE_DOWNLOAD ? WKNavigationResponsePolicyDownload : WKNavigationResponsePolicyAllow;
}

// benchDownloadLinkPolicy decides a link with a download attribute by the
// frame it's in, as benchResponsePolicy decides an attachment: the main
// frame, and frames on the item's site, may download. Embedded content from
// another site, such as an advert, can't save files by itself, so its
// download links do nothing.
static WKNavigationActionPolicy benchDownloadLinkPolicy(WKNavigationAction *navigationAction, NSString *itemURL) {
	WKFrameInfo *source = navigationAction.sourceFrame;
	NSString *origin = benchOriginString(source.securityOrigin);
	int decision = benchDecideResponse((char *)benchUTF8(itemURL), (char *)benchUTF8(origin), source.isMainFrame ? 1 : 0, 0, (char *)"attachment");
	return decision == BENCH_RESPONSE_DOWNLOAD ? WKNavigationActionPolicyDownload : WKNavigationActionPolicyCancel;
}

#pragma mark - Script messages

@implementation BenchScriptHandler

- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
	(void)userContentController;
	BenchWebItem *item = self.item;
	if (item == nil || ![message.name isEqualToString:BenchNotifyHandler]) {
		return;
	}
	// Only the item's own page may post, from its main frame on the item's
	// site: popups share the item's content controller, and every frame in
	// a page, embedded content from other sites among them, can reach the
	// handler.
	if (message.webView != item.webView) {
		return;
	}
	WKFrameInfo *frame = message.frameInfo;
	NSString *origin = benchOriginString(frame.securityOrigin);
	if (!benchNotificationAllowed((char *)benchUTF8(item.requestedURL), (char *)benchUTF8(origin), frame.isMainFrame ? 1 : 0)) {
		return;
	}
	NSDictionary *body = [message.body isKindOfClass:[NSDictionary class]] ? message.body : nil;
	id title = body[@"title"];
	id text = body[@"body"];
	NSString *titleText = [title isKindOfClass:[NSString class]] ? title : @"";
	NSString *bodyText = [text isKindOfClass:[NSString class]] ? text : @"";
	if (titleText.length == 0 && bodyText.length == 0) {
		return;
	}
	benchEmitItem(BENCH_EV_WEB_NOTIFICATION, item.itemID, titleText, bodyText);
}

@end

#pragma mark - Popups

@implementation BenchPopup

- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner startURL:(NSString *)startURL {
	self = [super init];
	if (self) {
		_owner = owner;
		_startURL = [startURL copy];
		CGFloat width = features.width != nil ? features.width.doubleValue : 520;
		CGFloat height = features.height != nil ? features.height.doubleValue : 640;
		width = MIN(MAX(width, 320), 1600);
		height = MIN(MAX(height, 320), 1200);
		NSRect contentRect = NSMakeRect(0, 0, width, height);
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
		_window = [[NSWindow alloc] initWithContentRect:contentRect styleMask:style backing:NSBackingStoreBuffered defer:NO];
		// Under ARC the popup owns the window; AppKit mustn't release it too.
		_window.releasedWhenClosed = NO;
		_window.delegate = self;
		// The configuration WebKit passes must be used as it is, so the new
		// page is connected to its opener.
		_webView = [[WKWebView alloc] initWithFrame:contentRect configuration:configuration];
		_webView.UIDelegate = self;
		_webView.navigationDelegate = self;
		_webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		_window.contentView = _webView;
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"URL" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		_observing = YES;
	}
	return self;
}

- (void)dealloc {
	if (_observing) {
		[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[_webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
	}
}

- (void)show {
	[self.window center];
	[self.window makeKeyAndOrderFront:nil];
}

- (void)close {
	[self.window close];
}

// closeSoon closes the popup once WebKit has finished the call it's in.
- (void)closeSoon {
	BenchPopup *popup = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		[popup close];
	});
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchKVOContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	if ([keyPath isEqualToString:@"URL"]) {
		// A popup has no address bar: its subtitle says which site it shows.
		self.window.subtitle = self.webView.URL.host ?: @"";
		return;
	}
	NSString *title = self.webView.title;
	if (title.length > 0) {
		self.window.title = title;
	}
}

- (void)windowWillClose:(NSNotification *)notification {
	(void)notification;
	if (self.observing) {
		[self.webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
		self.observing = NO;
	}
	self.webView.UIDelegate = nil;
	self.webView.navigationDelegate = nil;
	[self.webView stopLoading];
	self.window.delegate = nil;
	// Let go of the popup (and its window) once AppKit has finished closing
	// the window, not in the middle of it.
	BenchPopup *popup = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		[popup.owner.popups removeObject:popup];
	});
}

- (void)webViewDidClose:(WKWebView *)webView {
	(void)webView;
	[self.window close];
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	(void)webView;
	return [self.owner popupWithConfiguration:configuration action:navigationAction features:windowFeatures];
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	NSString *itemURL = self.owner.requestedURL ?: @"";
	if (navigationAction.shouldPerformDownload) {
		decisionHandler(benchDownloadLinkPolicy(navigationAction, itemURL));
		return;
	}
	WKFrameInfo *target = navigationAction.targetFrame;
	if (target == nil || !target.isMainFrame) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	NSURL *url = navigationAction.request.URL;
	BOOL clicked = navigationAction.navigationType == WKNavigationTypeLinkActivated;
	int decision = benchDecidePopupNavigation((char *)benchUTF8(itemURL), (char *)benchUTF8(self.startURL), (char *)benchUTF8(url.absoluteString), clicked ? 1 : 0);
	if (decision == BENCH_NAV_IN_VIEW) {
		NSURL *start = [NSURL URLWithString:self.startURL];
		if (!benchIsWebURL(start) && benchIsWebURL(url)) {
			self.startURL = url.absoluteString ?: @"";
		}
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), decisionHandler);
	if ((decision == BENCH_NAV_IN_BROWSER || decision == BENCH_NAV_ASK) && (!self.committed || !clicked)) {
		// It was opened only to go there, as a redirector's popup is (some
		// show a page of their own first, which then sends itself on):
		// there's nothing to leave on screen. A link the user clicked in a
		// page that stays keeps its popup.
		[self closeSoon];
	}
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)navigationResponse decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	(void)webView;
	decisionHandler(benchResponsePolicy(navigationResponse, self.owner.requestedURL ?: @""));
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	(void)navigation;
	NSURL *url = webView.URL;
	if (url != nil && ![url.absoluteString isEqualToString:@"about:blank"]) {
		self.committed = YES;
	}
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)navigationAction didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationAction;
	download.delegate = benchDownloads();
	if (!self.committed) {
		[self closeSoon];
	}
}

- (void)webView:(WKWebView *)webView navigationResponse:(WKNavigationResponse *)navigationResponse didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationResponse;
	download.delegate = benchDownloads();
	if (!self.committed) {
		[self closeSoon];
	}
}

@end

#pragma mark - Web items

@implementation BenchWebItem

- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix {
	self = [super init];
	if (self) {
		_itemID = [itemID copy];
		_profileUUID = [profileUUID copy];
		_requestedURL = @"";
		_popups = [NSMutableArray array];

		WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
		if (@available(macOS 14.0, *)) {
			// A persistent store per profile, found again by the same UUID
			// after Bench restarts, so logins last and stay separate.
			NSUUID *identifier = [[NSUUID alloc] initWithUUIDString:profileUUID];
			if (identifier != nil) {
				configuration.websiteDataStore = [WKWebsiteDataStore dataStoreForIdentifier:identifier];
			}
		}
		if (suffix.length > 0) {
			configuration.applicationNameForUserAgent = suffix;
		}
		// Pages open windows only when the user clicks, as in browsers, so
		// that a page or anything embedded in it can't open popups, or send
		// addresses to other apps, by itself. Popups inherit this.
		configuration.preferences.javaScriptCanOpenWindowsAutomatically = NO;

		_contentController = [[WKUserContentController alloc] init];
		WKUserScript *script = [[WKUserScript alloc] initWithSource:BenchNotificationScript injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
		[_contentController addUserScript:script];
		_scriptHandler = [[BenchScriptHandler alloc] init];
		_scriptHandler.item = self;
		[_contentController addScriptMessageHandler:_scriptHandler name:BenchNotifyHandler];
		configuration.userContentController = _contentController;

		_webView = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 800, 600) configuration:configuration];
		_webView.navigationDelegate = self;
		_webView.UIDelegate = self;
		_webView.allowsBackForwardNavigationGestures = YES;
		_webView.hidden = YES;
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"URL" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		_observing = YES;
	}
	return self;
}

- (void)dealloc {
	// A web view mustn't outlive its observers' registrations.
	if (_observing) {
		[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[_webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
	}
}

- (void)loadAddress:(NSString *)address {
	self.requestedURL = address;
	// Loading starts a new process anyway, and a new page starts counting
	// crashes afresh.
	self.needsReload = NO;
	self.crashReloaded = 0;
	NSURL *url = [NSURL URLWithString:address];
	if (url == nil || url.scheme.length == 0) {
		url = [NSURL URLWithString:@"about:blank"];
	}
	[self.webView loadRequest:[NSURLRequest requestWithURL:url]];
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchKVOContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	if ([keyPath isEqualToString:@"title"]) {
		NSString *title = self.webView.title;
		if (title.length == 0) {
			return;
		}
		if (self.popOutWindow != nil) {
			self.popOutWindow.title = title;
		}
		benchEmitItem(BENCH_EV_WEB_TITLE, self.itemID, title, nil);
	} else if ([keyPath isEqualToString:@"URL"]) {
		NSString *address = self.webView.URL.absoluteString;
		if (address.length > 0) {
			benchEmitItem(BENCH_EV_WEB_URL, self.itemID, nil, address);
		}
	}
}

- (void)returnFromPopOut {
	NSWindow *window = self.popOutWindow;
	if (window == nil) {
		return;
	}
	window.delegate = nil;
	self.popOutWindow = nil;
	WKWebView *web = self.webView;
	[web removeFromSuperview];
	web.autoresizingMask = NSViewNotSizable;
	web.hidden = YES;
	self.visible = NO;
	NSView *container = benchWebContainerView();
	if (container != nil) {
		[container addSubview:web];
		web.frame = self.frame;
	}
	// Keep the window until AppKit has finished closing it.
	dispatch_async(dispatch_get_main_queue(), ^{
		(void)window;
	});
}

- (void)tearDown {
	benchYieldFocusFrom(self.webView);
	if (self.observing) {
		[self.webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
		self.observing = NO;
	}
	[self.contentController removeScriptMessageHandlerForName:BenchNotifyHandler];
	[self.webView stopLoading];
	self.webView.navigationDelegate = nil;
	self.webView.UIDelegate = nil;
	self.needsReload = NO;
	// WebKit needs every dialog's answer, even from a page that's going.
	BenchDialog *dialog = self.waitingDialog;
	self.waitingDialog = nil;
	if (dialog != nil) {
		dialog.done(NO);
	}
	for (BenchPopup *popup in [self.popups copy]) {
		[popup close];
	}
	if (self.popOutWindow != nil) {
		NSWindow *window = self.popOutWindow;
		self.closingPopOut = YES;
		[self returnFromPopOut];
		[window close];
		self.closingPopOut = NO;
	}
	[self.webView removeFromSuperview];
}

#pragma mark Navigation

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	// A link with a download attribute.
	if (navigationAction.shouldPerformDownload) {
		decisionHandler(benchDownloadLinkPolicy(navigationAction, self.requestedURL));
		return;
	}
	// Frames inside the page (embedded content) always load. A navigation
	// without a target frame asks for a new window, which the UI delegate
	// decides.
	WKFrameInfo *target = navigationAction.targetFrame;
	if (target == nil || !target.isMainFrame) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	NSURL *url = navigationAction.request.URL;
	BOOL clicked = navigationAction.navigationType == WKNavigationTypeLinkActivated;
	int decision = benchDecideNavigation((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(url.absoluteString), clicked ? 1 : 0);
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), decisionHandler);
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)navigationResponse decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	(void)webView;
	decisionHandler(benchResponsePolicy(navigationResponse, self.requestedURL));
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)navigationAction didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationAction;
	download.delegate = benchDownloads();
}

- (void)webView:(WKWebView *)webView navigationResponse:(WKNavigationResponse *)navigationResponse didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationResponse;
	download.delegate = benchDownloads();
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	(void)webView;
	(void)navigation;
	// A new page may ask for dialogs again.
	self.dialogsStopped = NO;
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
	(void)webView;
	// The page's process crashed or was ended: load it again. With this
	// method, WebKit leaves reloading to Bench, which mustn't do it without
	// limit (see reloadAfterCrash).
	CFAbsoluteTime now = CFAbsoluteTimeGetCurrent();
	BOOL again = self.crashReloaded > 0 && now - self.crashReloaded < BenchCrashWindow;
	self.crashCount = again ? self.crashCount + 1 : 1;
	self.lastCrash = now;
	self.needsReload = YES;
	[self reloadAfterCrash];
}

// canBeSeen reports whether the page is on screen, in Bench's window or its
// own, as far as Bench can tell.
- (BOOL)canBeSeen {
	NSWindow *window = self.popOutWindow;
	if (window != nil) {
		return window.visible && !window.miniaturized;
	}
	return self.visible && benchWebContainerView() != nil;
}

// reloadAfterCrash loads the page again after its process crashed, once the
// page can be seen: a hidden page doesn't need a process, and one that
// crashes over and over in a tab the user isn't looking at shouldn't keep
// starting new ones. The first crash in a while reloads at once, and each
// one after that waits longer (see BenchCrashWindow), counted from the
// crash, so that a page shown after being hidden for that long reloads at
// once.
- (void)reloadAfterCrash {
	if (!self.needsReload || self.reloadScheduled || ![self canBeSeen]) {
		return;
	}
	NSTimeInterval delay = 0;
	if (self.crashCount > 1) {
		NSUInteger doublings = MIN(self.crashCount - 2, (NSUInteger)8);
		delay = MIN(BenchCrashMaxDelay, BenchCrashDelay * (double)((NSUInteger)1 << doublings));
		delay = MAX(0.0, delay - (CFAbsoluteTimeGetCurrent() - self.lastCrash));
	}
	self.reloadScheduled = YES;
	__weak BenchWebItem *weakSelf = self;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		BenchWebItem *item = weakSelf;
		if (item == nil) {
			return;
		}
		item.reloadScheduled = NO;
		// Hidden meanwhile, it reloads when it's shown again.
		if (!item.needsReload || ![item canBeSeen]) {
			return;
		}
		item.needsReload = NO;
		item.crashReloaded = CFAbsoluteTimeGetCurrent();
		[item.webView reload];
	});
}

#pragma mark UI

- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features {
	NSURL *url = action.request.URL;
	NSString *address = url.absoluteString ?: @"";
	int decision = benchDecidePopup((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(address));
	switch (decision) {
	case BENCH_POPUP_WINDOW:
		break;
	case BENCH_POPUP_BROWSER:
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		return nil;
	case BENCH_POPUP_ASK:
		benchOpenAfterAsking(url, benchSourceHost(action, self.webView));
		return nil;
	default:
		return nil;
	}
	BenchPopup *popup = [[BenchPopup alloc] initWithConfiguration:configuration features:features owner:self startURL:address];
	[self.popups addObject:popup];
	[popup show];
	return popup.webView;
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	(void)webView;
	return [self popupWithConfiguration:configuration action:navigationAction features:windowFeatures];
}

// canShowDialog reports whether the page can be seen, with Bench active, so
// that a dialog shown now is seen to be about it.
- (BOOL)canShowDialog {
	WKWebView *web = self.webView;
	return [NSApp isActive] && web.window != nil && web.window.visible && !web.hiddenOrHasHiddenAncestor;
}

// askWithMessage keeps a page's alert or confirmation until the page can be
// seen (see showWaitingDialog). A page whose dialogs the user stopped gets
// its answer at once: OK for an alert, Cancel for a confirmation.
- (void)askWithMessage:(NSString *)message frame:(WKFrameInfo *)frame confirm:(BOOL)confirm done:(void (^)(BOOL accepted))done {
	if (self.dialogsStopped || self.waitingDialog != nil || self.showingDialog) {
		done(NO);
		return;
	}
	NSString *text = message ?: @"";
	if (text.length > BenchDialogMaxLength) {
		text = [text substringWithRange:[text rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, BenchDialogMaxLength)]];
	}
	BenchDialog *dialog = [[BenchDialog alloc] init];
	dialog.message = text;
	dialog.host = frame.securityOrigin.host ?: @"";
	dialog.confirm = confirm;
	dialog.done = done;
	self.waitingDialog = dialog;
	[self showWaitingDialog];
}

// showWaitingDialog shows the page's waiting dialog, if the page can be seen.
// It's app-modal rather than a sheet: Bench's window sits below the docked
// windows, which would cover a sheet's buttons. It names the site asking,
// and offers to stop a page that asks again and again.
- (void)showWaitingDialog {
	BenchDialog *dialog = self.waitingDialog;
	if (dialog == nil || self.showingDialog || ![self canShowDialog]) {
		return;
	}
	self.waitingDialog = nil;
	self.showingDialog = YES;
	BOOL offerStop = CFAbsoluteTimeGetCurrent() - self.lastDialogEnded < BenchDialogRepeat;
	BenchWebItem *item = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = dialog.host.length > 0 ? [NSString stringWithFormat:@"%@ says", dialog.host] : @"This page says";
		alert.informativeText = dialog.message;
		[alert addButtonWithTitle:@"OK"];
		if (dialog.confirm) {
			[alert addButtonWithTitle:@"Cancel"];
		}
		if (offerStop) {
			alert.showsSuppressionButton = YES;
			alert.suppressionButton.title = @"Don't allow more dialogs from this page";
		}
		NSModalResponse response = [alert runModal];
		item.showingDialog = NO;
		item.lastDialogEnded = CFAbsoluteTimeGetCurrent();
		if (offerStop && alert.suppressionButton.state == NSControlStateValueOn) {
			item.dialogsStopped = YES;
		}
		dialog.done(response == NSAlertFirstButtonReturn);
	});
}

- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completionHandler {
	(void)webView;
	[self askWithMessage:message frame:frame confirm:NO done:^(BOOL accepted) {
		(void)accepted;
		completionHandler();
	}];
}

- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))completionHandler {
	(void)webView;
	[self askWithMessage:message frame:frame confirm:YES done:^(BOOL accepted) {
		completionHandler(accepted);
	}];
}

- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
	(void)webView;
	(void)frame;
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	panel.canChooseFiles = YES;
	panel.canChooseDirectories = parameters.allowsDirectories;
	panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
	[panel beginWithCompletionHandler:^(NSModalResponse result) {
		completionHandler(result == NSModalResponseOK ? panel.URLs : nil);
	}];
}

#pragma mark Popped out

// A page in its own window can't show a dialog while the window is
// minimised or behind another app's, and isn't reloaded after a crash while
// the window is minimised or Bench is hidden: it does both when the window
// is back.
- (void)windowDidDeminiaturize:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

// Bench being unhidden puts the window back on screen without making it
// key.
- (void)windowDidChangeOcclusionState:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

- (void)windowDidBecomeKey:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

- (void)windowWillClose:(NSNotification *)notification {
	if (notification.object != self.popOutWindow || self.closingPopOut) {
		return;
	}
	// The user closed the window: keep the web view, hidden in Bench's
	// window, and tell Go.
	[self returnFromPopOut];
	benchEmitItem(BENCH_EV_WEB_VIEW_CLOSED, self.itemID, nil, nil);
}

@end

#pragma mark - Entry points

static NSMutableDictionary<NSString *, BenchWebItem *> *gWebItems = nil;

static NSMutableDictionary<NSString *, BenchWebItem *> *benchWebItems(void) {
	if (gWebItems == nil) {
		gWebItems = [NSMutableDictionary dictionary];
	}
	return gWebItems;
}

WKWebView *benchWebViewForItem(NSString *itemID) {
	return benchWebItems()[itemID].webView;
}

NSString *benchWebItemHost(NSString *itemID) {
	return benchWebItems()[itemID].webView.URL.host;
}

NSString *benchWebItemAtPoint(NSWindow *window, NSPoint locationInWindow) {
	NSView *container = benchWebContainerView();
	if (container == nil || container.window != window || container.hiddenOrHasHiddenAncestor) {
		return nil;
	}
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		WKWebView *web = item.webView;
		if (item.popOutWindow != nil || web.superview != container || web.hidden) {
			continue;
		}
		NSPoint point = [web convertPoint:locationInWindow fromView:nil];
		if (NSPointInRect(point, web.bounds)) {
			return item.itemID;
		}
	}
	return nil;
}

void benchWebShowWaitingDialogs(void) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		[item showWaitingDialog];
	}
}

void benchWebAttachContainer(NSView *container) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		if (item.popOutWindow != nil) {
			continue;
		}
		[item.webView removeFromSuperview];
		item.webView.autoresizingMask = NSViewNotSizable;
		[container addSubview:item.webView];
		item.webView.frame = item.frame;
		item.webView.hidden = !item.visible;
		[item reloadAfterCrash];
	}
}

void benchWebCloseAll(void) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		[item tearDown];
	}
	[benchWebItems() removeAllObjects];
}

void benchWebShow(const char *itemID, const char *url, const char *profileUUID, const char *userAgentSuffix, double x, double y, double w, double h) {
	NSString *identifier = benchString(itemID);
	NSString *address = benchString(url);
	NSString *profile = benchString(profileUUID);
	NSString *suffix = benchString(userAgentSuffix);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		NSMutableDictionary<NSString *, BenchWebItem *> *items = benchWebItems();
		BenchWebItem *item = items[identifier];
		if (item != nil && ![item.profileUUID isEqualToString:profile]) {
			// A new profile needs a new data store, so a new web view.
			[item tearDown];
			[items removeObjectForKey:identifier];
			item = nil;
		}
		if (item == nil) {
			item = [[BenchWebItem alloc] initWithID:identifier profileUUID:profile userAgentSuffix:suffix];
			items[identifier] = item;
			[item loadAddress:address];
		} else if (![item.requestedURL isEqualToString:address] && ![item.webView.URL.absoluteString isEqualToString:address]) {
			// A new address loads, but not the page's own address (as a WebURL
			// event reported it), which would reload the page every time it's
			// shown.
			[item loadAddress:address];
		}
		item.frame = frame;
		item.visible = YES;
		if (item.popOutWindow != nil) {
			return; // It stays in its own window until docked.
		}
		NSView *container = benchWebContainerView();
		if (container == nil) {
			return; // Shown once Bench's window is attached.
		}
		WKWebView *web = item.webView;
		if (web.superview != container) {
			[web removeFromSuperview];
			[container addSubview:web];
		}
		web.autoresizingMask = NSViewNotSizable;
		web.frame = frame;
		web.hidden = NO;
		benchFocusShownView(web);
		[item showWaitingDialog];
		[item reloadAfterCrash];
	});
}

void benchWebHide(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		item.visible = NO;
		if (item.popOutWindow == nil) {
			// Hiding a view with the keyboard focus passes it to an arbitrary
			// next view: give it to the UI instead.
			benchYieldFocusFrom(item.webView);
			item.webView.hidden = YES;
		}
	});
}

void benchWebClose(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		[item tearDown];
		[benchWebItems() removeObjectForKey:identifier];
	});
}

void benchWebFocus(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		NSWindow *window = item.popOutWindow;
		if (window != nil) {
			benchActivateBench();
			[window makeKeyAndOrderFront:nil];
			[window makeFirstResponder:item.webView];
			[item showWaitingDialog];
			return;
		}
		benchFocusView(item.webView);
	});
}

void benchWebPopOut(const char *itemID, const char *title) {
	NSString *identifier = benchString(itemID);
	NSString *name = benchString(title);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		if (item.popOutWindow != nil) {
			[item.popOutWindow makeKeyAndOrderFront:nil];
			[item showWaitingDialog];
			return;
		}
		NSSize size = item.frame.size;
		if (size.width < 400 || size.height < 300) {
			size = NSMakeSize(1000, 700);
		}
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
		NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, size.width, size.height) styleMask:style backing:NSBackingStoreBuffered defer:NO];
		window.releasedWhenClosed = NO;
		NSString *pageTitle = item.webView.title;
		window.title = name.length > 0 ? name : (pageTitle ?: @"");
		window.delegate = item;

		WKWebView *web = item.webView;
		benchYieldFocusFrom(web);
		[web removeFromSuperview];
		web.frame = window.contentView.bounds;
		web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		web.hidden = NO;
		[window.contentView addSubview:web];
		item.popOutWindow = window;
		item.visible = NO;

		[window center];
		[window makeKeyAndOrderFront:nil];
		[window makeFirstResponder:web];
		[NSApp activate];
		[item showWaitingDialog];
		[item reloadAfterCrash];
	});
}

void benchWebDock(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		NSWindow *window = item.popOutWindow;
		if (window == nil) {
			return;
		}
		// returnFromPopOut clears the window's delegate first, so closing it
		// here doesn't report WebViewClosed.
		item.closingPopOut = YES;
		[item returnFromPopOut];
		[window close];
		item.closingPopOut = NO;
	});
}
