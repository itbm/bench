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

# The server build (the browser preview) needs no cgo, and on Linux Wails
# would otherwise want GTK 4.
SERVER := CGO_ENABLED=0 $(GO)

# Stops a recipe that needs macOS.
REQUIRE_MACOS := if [ "$$(uname -s)" != Darwin ]; then echo "This target needs macOS." >&2; exit 1; fi

.DEFAULT_GOAL := help

.PHONY: help build preview preview-bin test vet e2e check app app-bin icns dist run icon acknowledgements clean

help: ## List the targets.
	@echo "Usage: make <target> [VARIABLE=value]"
	@echo
	@awk 'BEGIN { FS = ":.*## " } /^[a-z][a-z0-9-]*:.*## / { printf "  %-17s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo
	@echo "Variables: SIGN_IDENTITY (\"$(SIGN_IDENTITY)\"), ARCHS ($(ARCHS)), APP_TAGS ($(APP_TAGS)), PORT ($(PORT)), VERSION ($(VERSION)), RACE ($(RACE))."

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

app: app-bin icns ## Build, assemble, and sign bin/Bench.app (macOS).
	@$(REQUIRE_MACOS)
	rm -rf $(APP)
	mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	cp $(DARWIN)/Info.plist $(APP)/Contents/Info.plist
	cp $(BIN)/Bench.icns $(APP)/Contents/Resources/Bench.icns
	cp $(DARWIN)/Credits.rtf $(APP)/Contents/Resources/Credits.rtf
	cp $(BIN)/bench-universal $(APP)/Contents/MacOS/bench
	plutil -lint $(APP)/Contents/Info.plist
	@if [ -z "$(SIGN_IDENTITY)" ]; then \
		echo "Not signing $(APP), because SIGN_IDENTITY is empty."; \
	else \
		echo "Signing $(APP) as \"$(SIGN_IDENTITY)\""; \
		codesign --force --options runtime $(SIGN_TIMESTAMP) \
			--entitlements $(DARWIN)/entitlements.plist \
			--sign "$(SIGN_IDENTITY)" $(APP) || { \
			echo "Couldn't sign $(APP) as \"$(SIGN_IDENTITY)\". Without that certificate, sign ad hoc with make app SIGN_IDENTITY=- (see docs/development.md)." >&2; \
			exit 1; }; \
		codesign --verify --strict --verbose=2 $(APP); \
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

icon: ## Draw build/darwin/icon.png again, after changing its program.
	$(GO) run $(DARWIN)/icon/main.go -o $(DARWIN)/icon.png

# The Acknowledgements window's page, from the licences of what Bench.app
# includes. go test ./build/acknowledgements fails until it's run again
# after a dependency changes.
acknowledgements: ## Write frontend/acknowledgements.html again, after changing dependencies.
	$(GO) run ./build/acknowledgements -o frontend/acknowledgements.html

clean: ## Remove what the build made.
	rm -rf $(BIN) $(DIST) e2e/test-results
