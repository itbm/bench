# Bench's build. Run `make help` to list the targets.
#
# It's written for the GNU Make 3.81 that comes with macOS. The app targets
# (app, icns, run) need macOS with the Xcode command-line tools; the others
# work on any OS. See docs/development.md.

GO ?= go
BIN := bin
DARWIN := build/darwin
APP := $(BIN)/Bench.app

# The version comes from build/darwin/Info.plist, so that the app and
# `bench version` agree. If it can't be read, the binary keeps its own
# default ("dev") rather than an empty version.
ifeq ($(origin VERSION),undefined)
VERSION := $(shell sed -n '/<key>CFBundleShortVersionString<\/key>/{n;s/.*<string>\(.*\)<\/string>.*/\1/p;}' $(DARWIN)/Info.plist)
endif
LDFLAGS := $(if $(VERSION),-X main.version=$(VERSION))

# Extra flags for the app's go test, such as RACE=-race. check sets it, as
# CI does. The browser preview's tests run without it: the race detector
# needs cgo, which the preview's build turns off.
RACE ?=

# The browser preview's port.
PORT ?= 8080

# Bench.app's architectures. It's a universal binary unless you name one.
ARCHS ?= arm64 amd64
ARCH_BINS = $(foreach arch,$(ARCHS),$(BIN)/darwin-$(arch)/bench)
MACOS_MIN := 14.0
# The production tag turns off Wails' web inspector. Build with
# APP_TAGS= to keep it.
APP_TAGS ?= production

# The certificate that `make app` signs Bench.app with. macOS ties the
# Accessibility permission to the app's signature, so sign every
# development build with the same certificate: with a new ad hoc signature
# each time, macOS forgets the permission and asks again. "Apple
# Development" matches the certificate Xcode creates for your Apple ID; if
# you have more than one, give its full name or SHA-1 hash, as
# `security find-identity -v -p codesigning` lists them. Use "-" to sign ad
# hoc (no certificate needed, but the permission resets on every build), or
# leave it empty to skip signing.
SIGN_IDENTITY ?= Apple Development

# codesign's timestamp flag. Notarisation needs a secure timestamp from
# Apple, so a release signed with a Developer ID certificate sets
# SIGN_TIMESTAMP=--timestamp. Development builds skip it: it needs the
# network, and slows every build.
SIGN_TIMESTAMP ?= --timestamp=none

# Where `make dist` puts the files for a release.
DIST := dist

# Sparkle, the framework that installs new versions of Bench. make sparkle
# downloads it, with its tools, to $(SPARKLE). The checksum is the one in
# the Package.swift of Sparkle's tag for this version, so that a download
# that someone changed fails. See docs/development.md#updates.
SPARKLE_VERSION := 2.10.1
SPARKLE_SHA256 := 4c2e48a3e50ebcc2c88a58a4c7264a1a99c3c31857ce754357afb9cdb323022a
SPARKLE_URL := https://github.com/sparkle-project/Sparkle/releases/download/$(SPARKLE_VERSION)/Sparkle-for-Swift-Package-Manager.zip
SPARKLE := $(BIN)/sparkle
# Where make app puts Sparkle in Bench.app.
APP_SPARKLE := $(APP)/Contents/Frameworks/Sparkle.framework

# The EdDSA public key that updates are signed with, as Sparkle's
# generate_keys prints it. With a key, make app puts Sparkle in Bench.app
# and the key in its Info.plist, and Bench can install new versions. Without
# one, Bench.app has no Sparkle, and Bench only says when there's a new
# version. Once a release has a key, every later release needs the same
# key: Sparkle refuses an update that has none, or another.
SPARKLE_PUBLIC_KEY ?=

# Where Bench looks for new versions, in place of Info.plist's SUFeedURL.
# The update check on a macOS runner sets it; releases leave it empty.
SPARKLE_FEED ?=

# The server build (the browser preview) needs no cgo, and on Linux Wails
# would otherwise want GTK 4.
SERVER := CGO_ENABLED=0 $(GO)

# Stops a recipe that needs macOS.
REQUIRE_MACOS := if [ "$$(uname -s)" != Darwin ]; then echo "This target needs macOS." >&2; exit 1; fi

.DEFAULT_GOAL := help

.PHONY: help build preview preview-bin test vet e2e check app app-bin icns sparkle dist run icon acknowledgements clean

help: ## List the targets.
	@echo "Usage: make <target> [VARIABLE=value]"
	@echo
	@awk 'BEGIN { FS = ":.*## " } /^[a-z][a-z0-9-]*:.*## / { printf "  %-17s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo
	@echo "Variables: SIGN_IDENTITY (\"$(SIGN_IDENTITY)\"), ARCHS ($(ARCHS)), APP_TAGS ($(APP_TAGS)), PORT ($(PORT)), VERSION ($(VERSION)), RACE ($(RACE)), SPARKLE_PUBLIC_KEY (\"$(SPARKLE_PUBLIC_KEY)\")."

build: ## Build bin/bench: the app on macOS, the subcommands anywhere.
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/bench ./cmd/bench

preview: ## Run the browser preview at http://localhost:8080 (or PORT).
	$(SERVER) run -tags server -ldflags "$(LDFLAGS)" ./cmd/bench --port $(PORT)

preview-bin: ## Build the browser preview as bin/bench-preview.
	$(SERVER) build -tags server -ldflags "$(LDFLAGS)" -o $(BIN)/bench-preview ./cmd/bench

test: ## Run the Go tests, for the app and for the browser preview.
	$(GO) test $(RACE) ./...
	$(SERVER) test -tags server ./...

vet: ## Check formatting, and run go vet on both builds.
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "These files need gofmt -w:"; echo "$$files"; exit 1; fi
	$(GO) vet ./...
	$(SERVER) vet -tags server ./...

# The end-to-end tests in e2e/app run the preview that preview-bin built.
e2e: e2e/node_modules/.installed preview-bin ## Run the browser tests (needs Node 22).
	BENCH_PREVIEW="$(CURDIR)/$(BIN)/bench-preview" sh scripts/e2e.sh

# Playwright and its Chromium, from e2e/package-lock.json. The target is a
# stamp, made only once both commands succeed: npm ci makes e2e/node_modules
# afresh before Chromium downloads, so if the download fails, the folder
# would look up to date and make would never try again.
e2e/node_modules/.installed: e2e/package.json e2e/package-lock.json
	cd e2e && npm ci && npx playwright install chromium
	touch $@

# check runs the tests with the race detector, as the Linux CI job does.
check: RACE := -race
check: vet test e2e ## Run every check that the Linux CI job runs.

# With SPARKLE_PUBLIC_KEY, Bench.app gets Sparkle, without the XPC services
# that only a sandboxed app needs, and Info.plist gets the key. Sparkle's
# helpers are signed first, then the framework, then the app: codesign's
# --deep would sign them all alike. Under the hardened runtime, macOS loads
# a framework only if it's signed by the app's team, and an ad hoc
# signature has no team, so an ad hoc Bench.app with Sparkle is signed with
# library validation turned off (see build/darwin/README.md).
app: app-bin icns $(if $(SPARKLE_PUBLIC_KEY),$(SPARKLE)/.installed-$(SPARKLE_VERSION)) ## Build, assemble, and sign bin/Bench.app (macOS).
	@$(REQUIRE_MACOS)
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	cp $(DARWIN)/Info.plist $(APP)/Contents/Info.plist
	$(if $(VERSION),plutil -replace CFBundleShortVersionString -string "$(VERSION)" $(APP)/Contents/Info.plist)
	$(if $(VERSION),plutil -replace CFBundleVersion -string "$(VERSION)" $(APP)/Contents/Info.plist)
	$(if $(SPARKLE_FEED),plutil -replace SUFeedURL -string "$(SPARKLE_FEED)" $(APP)/Contents/Info.plist)
	cp $(BIN)/Bench.icns $(APP)/Contents/Resources/Bench.icns
	cp $(DARWIN)/Credits.rtf $(APP)/Contents/Resources/Credits.rtf
	cp $(BIN)/bench-universal $(APP)/Contents/MacOS/bench
	cp $(DARWIN)/entitlements.plist $(BIN)/entitlements.plist
	@if [ -n "$(SPARKLE_PUBLIC_KEY)" ]; then \
		echo "Adding Sparkle $(SPARKLE_VERSION), for updates signed with $(SPARKLE_PUBLIC_KEY)"; \
		mkdir -p $(APP)/Contents/Frameworks && \
		ditto $(SPARKLE)/Sparkle.framework $(APP_SPARKLE) && \
		rm -rf $(APP_SPARKLE)/Versions/B/XPCServices $(APP_SPARKLE)/XPCServices && \
		plutil -insert SUPublicEDKey -string "$(SPARKLE_PUBLIC_KEY)" $(APP)/Contents/Info.plist || exit 1; \
		if [ "$(SIGN_IDENTITY)" = - ]; then \
			/usr/libexec/PlistBuddy -c "Add :com.apple.security.cs.disable-library-validation bool true" $(BIN)/entitlements.plist || exit 1; \
		fi; \
	fi
	plutil -lint $(APP)/Contents/Info.plist
	@if [ -z "$(SIGN_IDENTITY)" ]; then \
		echo "Not signing $(APP), because SIGN_IDENTITY is empty."; \
	else \
		echo "Signing $(APP) as \"$(SIGN_IDENTITY)\""; \
		sign() { code="$$1"; shift; \
			codesign --force --options runtime $(SIGN_TIMESTAMP) "$$@" --sign "$(SIGN_IDENTITY)" "$$code" || { \
			echo "Couldn't sign $$code as \"$(SIGN_IDENTITY)\". Without that certificate, sign ad hoc with make app SIGN_IDENTITY=- (see docs/development.md)." >&2; \
			exit 1; }; }; \
		if [ -d $(APP_SPARKLE) ]; then \
			sign $(APP_SPARKLE)/Versions/B/Autoupdate; \
			sign $(APP_SPARKLE)/Versions/B/Updater.app; \
			sign $(APP_SPARKLE); \
		fi; \
		sign $(APP) --entitlements $(BIN)/entitlements.plist; \
		codesign --verify --deep --strict --verbose=2 $(APP); \
	fi

# The app's binary, built for each of ARCHS and joined with lipo.
app-bin:
	@$(REQUIRE_MACOS)
	@for arch in $(ARCHS); do \
		echo "Building bench for $$arch"; \
		CGO_ENABLED=1 GOOS=darwin GOARCH=$$arch MACOSX_DEPLOYMENT_TARGET=$(MACOS_MIN) \
			$(GO) build -tags "$(APP_TAGS)" -ldflags "$(LDFLAGS)" \
			-o $(BIN)/darwin-$$arch/bench ./cmd/bench || exit 1; \
	done
	$(if $(word 2,$(ARCHS)),lipo -create $(ARCH_BINS) -output,cp $(ARCH_BINS)) $(BIN)/bench-universal
	lipo -info $(BIN)/bench-universal

icns: $(BIN)/Bench.icns

# The app icon, from build/darwin/icon.png, at every size macOS uses.
$(BIN)/Bench.icns: $(DARWIN)/icon.png
	@$(REQUIRE_MACOS)
	rm -rf $(BIN)/Bench.iconset
	mkdir -p $(BIN)/Bench.iconset
	@for size in 16 32 128 256 512; do \
		sips -z $$size $$size $< --out $(BIN)/Bench.iconset/icon_$${size}x$${size}.png >/dev/null && \
		sips -z $$((size * 2)) $$((size * 2)) $< --out $(BIN)/Bench.iconset/icon_$${size}x$${size}@2x.png >/dev/null || exit 1; \
	done
	iconutil -c icns -o $@ $(BIN)/Bench.iconset
	rm -rf $(BIN)/Bench.iconset

sparkle: $(SPARKLE)/.installed-$(SPARKLE_VERSION) ## Download Sparkle and its tools, such as generate_keys, to bin/sparkle (macOS).

# Sparkle's framework and its tools, from the download that SPARKLE_SHA256
# names. The target is a stamp, made only once the download is checked and
# unpacked, so that a failed download is tried again, and a new
# SPARKLE_VERSION downloads afresh. Sparkle's Swift Package Manager download
# holds an XCFramework; Bench.app takes the macOS framework from it.
$(SPARKLE)/.installed-$(SPARKLE_VERSION):
	@$(REQUIRE_MACOS)
	rm -rf $(SPARKLE)
	mkdir -p $(SPARKLE)
	curl -fsSL --retry 3 -o $(SPARKLE)/Sparkle.zip "$(SPARKLE_URL)"
	echo "$(SPARKLE_SHA256)  $(SPARKLE)/Sparkle.zip" | shasum -a 256 -c -
	ditto -x -k $(SPARKLE)/Sparkle.zip $(SPARKLE)/package
	rm $(SPARKLE)/Sparkle.zip
	@framework="$$(ls -d $(SPARKLE)/package/Sparkle.xcframework/macos-*/Sparkle.framework 2>/dev/null | head -1)"; \
		if [ -z "$$framework" ]; then echo "Sparkle's download has no macOS framework." >&2; exit 1; fi; \
		echo "ditto $$framework $(SPARKLE)/Sparkle.framework"; \
		ditto "$$framework" $(SPARKLE)/Sparkle.framework
	ln -s package/bin $(SPARKLE)/bin
	$(SPARKLE)/bin/sign_update --help >/dev/null
	touch $@

# The files a release offers: Bench.dmg, a disk image to drag Bench into
# Applications from, and Bench.zip. dist packages bin/Bench.app as it is,
# without building it, so that a release can notarise Bench.app between
# `make app` and `make dist`. hdiutil sometimes fails with "Resource busy"
# on CI runners, so it gets three tries.
dist: ## Package bin/Bench.app as dist/Bench.dmg and dist/Bench.zip (macOS).
	@$(REQUIRE_MACOS)
	@if [ ! -d $(APP) ]; then echo "There's no $(APP). Run make app first." >&2; exit 1; fi
	rm -rf $(DIST) $(BIN)/dmg
	mkdir -p $(DIST) $(BIN)/dmg
	ditto -c -k --sequesterRsrc --keepParent $(APP) $(DIST)/Bench.zip
	ditto $(APP) $(BIN)/dmg/Bench.app
	ln -s /Applications $(BIN)/dmg/Applications
	@for try in 1 2 3; do \
		hdiutil create -volname Bench -srcfolder $(BIN)/dmg -fs HFS+ -format UDZO -ov $(DIST)/Bench.dmg && exit 0; \
		echo "hdiutil failed; trying again in 5 seconds." >&2; sleep 5; \
	done; exit 1
	rm -rf $(BIN)/dmg

# open brings a running Bench forward instead of starting the new build, and
# rebuilding would replace the bundle under it, so run stops before building
# if one of your bench processes is running: the app, or a command such as
# bench arrange. Other users' don't count, since open never reaches them.
run: ## Build Bench.app, then open it (macOS).
	@$(REQUIRE_MACOS)
	@if pgrep -x -U "$$(id -u)" bench >/dev/null; then \
		echo "Bench is running. Quit it first: open would bring it forward instead of starting the new build." >&2; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory app
	open $(APP)

icon: ## Draw the app icon and the menu bar item's icon again, after changing their programs.
	$(GO) run $(DARWIN)/icon/main.go -o $(DARWIN)/icon.png
	$(GO) run $(DARWIN)/menubar/main.go -o internal/ui/menubar.png

# The Acknowledgements window's page, from the licences of what Bench.app
# includes. go test ./build/acknowledgements fails until it's run again
# after a dependency changes.
acknowledgements: ## Write frontend/acknowledgements.html again, after changing dependencies.
	$(GO) run ./build/acknowledgements -o frontend/acknowledgements.html

clean: ## Remove what the build made.
	rm -rf $(BIN) $(DIST) e2e/test-results
