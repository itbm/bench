//go:build darwin && cgo && !server

// web_darwin.m: Web items' web views. Each is a WKWebView in Bench's window,
// above the UI's own web view, with a persistent data store per profile, a
// navigation policy that keeps the item on its site, popups for sign-in,
// and a bridge for the page's notifications.
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

@class BenchWebItem;

// BenchScriptHandler forwards the notification script's messages to its
// item. It holds the item weakly: the user content controller keeps its
// handlers, and the item owns the web view that owns the controller.
@interface BenchScriptHandler : NSObject <WKScriptMessageHandler>
@property (nonatomic, weak) BenchWebItem *item;
@end

// BenchPopup is a window that a Web item's page opened, for sign-in: a real
// window, so window.opener works.
@interface BenchPopup : NSObject <WKUIDelegate, NSWindowDelegate>
@property (nonatomic, weak) BenchWebItem *owner;
@property (nonatomic, strong) NSWindow *window;
@property (nonatomic, strong) WKWebView *webView;
@property (nonatomic) BOOL observing;
- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner;
- (void)show;
- (void)close;
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
- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix;
- (void)loadAddress:(NSString *)address;
- (void)returnFromPopOut;
- (void)tearDown;
- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features;
@end

#pragma mark - Script messages

@implementation BenchScriptHandler

- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
	(void)userContentController;
	BenchWebItem *item = self.item;
	if (item == nil || ![message.name isEqualToString:BenchNotifyHandler]) {
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

- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner {
	self = [super init];
	if (self) {
		_owner = owner;
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
		_webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		_window.contentView = _webView;
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		_observing = YES;
	}
	return self;
}

- (void)dealloc {
	if (_observing) {
		[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
	}
}

- (void)show {
	[self.window center];
	[self.window makeKeyAndOrderFront:nil];
}

- (void)close {
	[self.window close];
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchKVOContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
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
		self.observing = NO;
	}
	self.webView.UIDelegate = nil;
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
	if (self.observing) {
		[self.webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
		self.observing = NO;
	}
	[self.contentController removeScriptMessageHandlerForName:BenchNotifyHandler];
	[self.webView stopLoading];
	self.webView.navigationDelegate = nil;
	self.webView.UIDelegate = nil;
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
	(void)webView;
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
	switch (decision) {
	case BENCH_NAV_IN_VIEW:
		decisionHandler(WKNavigationActionPolicyAllow);
		break;
	case BENCH_NAV_IN_BROWSER:
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		decisionHandler(WKNavigationActionPolicyCancel);
		break;
	default:
		decisionHandler(WKNavigationActionPolicyCancel);
		break;
	}
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
	// The page's process crashed or was ended: load it again.
	[webView reload];
}

#pragma mark UI

- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features {
	NSURL *url = action.request.URL;
	int decision = benchDecidePopup((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(url.absoluteString));
	if (decision == BENCH_POPUP_BROWSER) {
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		return nil;
	}
	if (decision != BENCH_POPUP_WINDOW) {
		return nil;
	}
	BenchPopup *popup = [[BenchPopup alloc] initWithConfiguration:configuration features:features owner:self];
	[self.popups addObject:popup];
	[popup show];
	return popup.webView;
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	(void)webView;
	return [self popupWithConfiguration:configuration action:navigationAction features:windowFeatures];
}

- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completionHandler {
	(void)frame;
	NSWindow *host = webView.window;
	if (host == nil) {
		completionHandler();
		return;
	}
	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = message;
	[alert addButtonWithTitle:@"OK"];
	[alert beginSheetModalForWindow:host completionHandler:^(NSModalResponse response) {
		(void)response;
		completionHandler();
	}];
}

- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))completionHandler {
	(void)frame;
	NSWindow *host = webView.window;
	if (host == nil) {
		completionHandler(NO);
		return;
	}
	NSAlert *alert = [[NSAlert alloc] init];
	alert.messageText = message;
	[alert addButtonWithTitle:@"OK"];
	[alert addButtonWithTitle:@"Cancel"];
	[alert beginSheetModalForWindow:host completionHandler:^(NSModalResponse response) {
		completionHandler(response == NSAlertFirstButtonReturn);
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
		[web removeFromSuperview];
		web.frame = window.contentView.bounds;
		web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		web.hidden = NO;
		[window.contentView addSubview:web];
		item.popOutWindow = window;
		item.visible = NO;

		[window center];
		[window makeKeyAndOrderFront:nil];
		[NSApp activate];
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
