#!/bin/bash
# Checks on a GitHub macOS runner that Sparkle updates Bench.app, for
# .github/workflows/macos-update.yml. Run it from the repository's root.
#
# Usage: run.sh <output folder>
#
# It makes an update key for the run, builds an old Bench.app (9.9.0) and a
# new one (9.9.1), both signed ad hoc and with Sparkle and the key, as
# make app does for a release, and serves the new one's Bench.dmg, with a
# signed appcast that build/appcast writes, on 127.0.0.1. Then it opens the
# old one with check_for_updates on, and checks that:
#
#  1. Sparkle's first look finds 9.9.1, and Bench's status bar has the
#     notice, with Install… and no Dismiss, not a window of Sparkle's;
#  2. Bench ▸ Check for Updates… shows Sparkle's window for 9.9.1, with no
#     tick box to install updates automatically;
#  3. Install Update downloads it, checks its signature, and offers to
#     install it;
#  4. Install and Relaunch quits Bench, puts 9.9.1 in its place, and opens
#     it.
#
# The runner's image grants Accessibility to /bin/bash, so osascript can
# press the buttons of Sparkle's windows through System Events.
#
# The output folder gets shots/ (a screenshot after each step), steps.log,
# build.log, bench.log, appcast.xml, server.log, and sparkle.log, Sparkle's
# messages from the system log. It exits with an error if a check fails.

set -u

OUT="$(mkdir -p "$1" && cd "$1" && pwd)"
REPO="$(pwd)"
mkdir -p "$OUT/shots"
WORK="$(mktemp -d)"
OLD=9.9.0
NEW=9.9.1
PORT=8765
FEED="http://127.0.0.1:$PORT/appcast.xml"
ID=io.github.itbm.bench
# Where the old Bench is installed, and the new one should be.
APP="$HOME/Applications/Bench.app"

shots=0
failed=0
log() { echo "[$(date +%T)] $*" | tee -a "$OUT/steps.log"; }
shot() {
	shots=$((shots + 1))
	screencapture -x "$OUT/shots/$(printf %02d "$shots")-$1.png" || log "screencapture failed"
}
fail() {
	log "FAIL: $*"
	failed=1
}

# names_of <role>: the names of the controls with that role, such as
# AXButton, in Bench's windows, which Sparkle's are among, one per line.
names_of() {
	osascript -l JavaScript - "$1" <<'JS' 2>/dev/null
function run(argv) {
	const p = Application("System Events").processes.byName("bench");
	const names = [];
	for (const w of p.windows()) {
		for (const e of w.entireContents()) {
			try { if (e.role() === argv[0] && e.name()) names.push(w.name() + ": " + e.name()); } catch (err) {}
		}
	}
	return names.join("\n");
}
JS
}

# button_names: the names of the buttons in Bench's windows.
button_names() { names_of AXButton; }

# press <button name>: press the first button with that name in Bench's
# windows. System Events can report an error for a press that worked, as
# the button's window changes or closes under it, so the steps after a
# press check what it did.
press() {
	osascript -l JavaScript - "$1" <<'JS' 2>&1
function run(argv) {
	const p = Application("System Events").processes.byName("bench");
	let button = null, window = "";
	for (const w of p.windows()) {
		for (const e of w.entireContents()) {
			try {
				if (e.role() === "AXButton" && e.name() === argv[0]) {
					button = e;
					window = w.name();
					break;
				}
			} catch (err) {}
		}
		if (button) break;
	}
	if (!button) throw new Error("no button " + argv[0]);
	try {
		button.click();
	} catch (err) {
		return "pressed " + argv[0] + " in " + window + "; System Events said: " + err.message;
	}
	return "pressed " + argv[0] + " in " + window;
}
JS
}

# wait_for_button <name> <seconds>: wait until a button with that name is
# in Bench's windows.
wait_for_button() {
	for _ in $(seq 1 "$2"); do
		button_names | grep -q ": $1\$" && return 0
		sleep 1
	done
	return 1
}

# notices: the notices in the view that Bench writes when BENCH_DEBUG is
# set, one per line: the text, then the labels of its buttons.
notices() {
	/usr/bin/python3 -c 'import json, sys; print("\n".join(" | ".join([n.get("text", "")] + [a.get("label", "") for a in n.get("actions") or []]) for n in json.load(open(sys.argv[1])).get("notices") or []))' \
		"$BENCH_HOME/view.json" 2>/dev/null
}

# app_pid: the newest process of an app called bench, not counting its
# watchdog, which runs the same binary with arguments.
app_pid() { ps -axo pid=,command= | awk '/Contents\/MacOS\/bench$/ { pid = $1 } END { print pid }'; }

# version_of <app>: the app's CFBundleShortVersionString.
version_of() { plutil -extract CFBundleShortVersionString raw "$1/Contents/Info.plist" 2>/dev/null; }

finish() {
	log "Sparkle's messages from the system log"
	/usr/bin/log show --last 15m --style compact --predicate 'subsystem == "org.sparkle-project.Sparkle" OR process == "Autoupdate" OR process == "Updater"' \
		>"$OUT/sparkle.log" 2>&1 || true
	pkill -x bench 2>/dev/null
	[ -n "${server_pid:-}" ] && kill "$server_pid" 2>/dev/null
	if [ "$failed" = 0 ]; then log "Done: every check passed"; else log "Done: a check failed"; fi
	exit "$failed"
}

log "Making an update key for this run"
private="$(openssl rand -base64 32)"
public="$(printf '%s\n' "$private" | go run ./build/appcast -print-public-key)" || { fail "couldn't read the key"; finish; }
log "Public key: $public"

build() {
	log "Building Bench $1"
	make app SIGN_IDENTITY=- ARCHS=arm64 VERSION="$1" SPARKLE_PUBLIC_KEY="$public" SPARKLE_FEED="$FEED" >>"$OUT/build.log" 2>&1 ||
		{ fail "couldn't build Bench $1; see build.log"; finish; }
}

build "$NEW"
make dist >>"$OUT/build.log" 2>&1 || { fail "couldn't package Bench $NEW"; finish; }
mkdir -p "$WORK/server"
cp dist/Bench.dmg "$WORK/server/Bench.dmg"

log "Signing Bench.dmg and writing the appcast"
signature="$(printf '%s\n' "$private" | bin/sparkle/bin/sign_update --ed-key-file - -p "$WORK/server/Bench.dmg")" ||
	{ fail "sign_update couldn't sign Bench.dmg"; finish; }
printf '## Bench %s\n\nAn update that the macOS update check serves.\n' "$NEW" >"$WORK/notes.md"
go run ./build/appcast -version "$NEW" -min-system "$(plutil -extract LSMinimumSystemVersion raw build/darwin/Info.plist)" \
	-url "http://127.0.0.1:$PORT/Bench.dmg" -link "http://127.0.0.1:$PORT/" \
	-archive "$WORK/server/Bench.dmg" -signature "$signature" -public-key "$public" \
	-notes "$WORK/notes.md" -o "$WORK/server/appcast.xml" || { fail "build/appcast refused the update"; finish; }
printf '%s\n' "$private" | bin/sparkle/bin/sign_update --ed-key-file - "$WORK/server/appcast.xml" >/dev/null ||
	{ fail "sign_update couldn't sign the appcast"; finish; }
cp "$WORK/server/appcast.xml" "$OUT/appcast.xml"
(cd "$WORK/server" && exec /usr/bin/python3 -m http.server "$PORT" --bind 127.0.0.1) >"$OUT/server.log" 2>&1 &
server_pid=$!

build "$OLD"
rm -rf "$APP"
mkdir -p "$(dirname "$APP")"
ditto bin/Bench.app "$APP"
log "Installed Bench $(version_of "$APP") at $APP"

# Sparkle keeps its settings and when it last looked in Bench's defaults.
# Without them, it looks as Bench opens.
defaults delete "$ID" >/dev/null 2>&1 || true
export BENCH_HOME="$WORK/home"
mkdir -p "$BENCH_HOME"
cp "$REPO/scripts/macos-update/config.toml" "$BENCH_HOME/config.toml"
touch "$BENCH_HOME/first-run-done"

log "Opening Bench $OLD"
BENCH_DEBUG=1 "$APP/Contents/MacOS/bench" >"$OUT/bench.log" 2>&1 &
for _ in $(seq 1 60); do
	notices | grep -q "Bench $NEW is out" && break
	sleep 1
done
# Bench's window shows a moment after Sparkle's first look. Sparkle mustn't
# show a window of its own meanwhile.
sleep 5
shot "notice"
log "Notices: $(notices | tr '\n' '|')"
log "Buttons: $(button_names | tr '\n' '|')"
# The notice has Install… and no Dismiss, as Sparkle waits for an answer in
# its own window before it looks again.
if notices | grep -qx "Bench $NEW is out\. | Install…"; then
	log "Check 1 passed: the status bar says that $NEW is out, with Install…"
else
	fail "check 1: no notice of $NEW with Install… alone"
fi
if button_names | grep -q ": Install Update\$"; then
	fail "check 1: Sparkle showed its window for a look that Bench didn't ask for"
fi

log "Choosing Bench ▸ Check for Updates…"
osascript -e 'tell application "System Events" to tell process "bench"
	set frontmost to true
	click menu item "Check for Updates…" of menu 1 of menu bar item 2 of menu bar 1
end tell' >/dev/null 2>&1 || log "couldn't choose Bench ▸ Check for Updates…"
if wait_for_button "Install Update" 30; then
	sleep 2
	shot "sparkle-window"
	log "Check 2 passed: Sparkle's window offers $NEW"
	# Info.plist's SUAllowsAutomaticUpdates leaves out the tick box that
	# would install updates without the status bar's notice.
	boxes="$(names_of AXCheckBox)"
	log "Tick boxes: $(printf '%s' "$boxes" | tr '\n' '|')"
	if printf '%s' "$boxes" | grep -qi "automatically"; then
		fail "check 2: Sparkle's window offers to install updates automatically"
	fi
else
	shot "no-sparkle-window"
	fail "check 2: no Install Update; buttons: $(button_names | tr '\n' '|')"
	finish
fi

log "$(press "Install Update")"
if wait_for_button "Install and Relaunch" 120; then
	sleep 1
	shot "ready-to-install"
	log "Check 3 passed: Sparkle downloaded $NEW, checked it, and offers to install it"
else
	shot "not-ready"
	fail "check 3: no Install and Relaunch; buttons: $(button_names | tr '\n' '|')"
	finish
fi

# Sparkle opens the new Bench as macOS opens apps, without this script's
# BENCH_HOME, so the usual support folder gets the same config.
support="$HOME/Library/Application Support/Bench"
mkdir -p "$support"
cp "$BENCH_HOME/config.toml" "$support/config.toml"
touch "$support/first-run-done"

old_pid="$(app_pid)"
log "$(press "Install and Relaunch")"
for _ in $(seq 1 90); do
	pid="$(app_pid)"
	[ "$(version_of "$APP")" = "$NEW" ] && [ -n "$pid" ] && [ "$pid" != "$old_pid" ] && break
	sleep 1
done
sleep 5
shot "relaunched"
got="$(version_of "$APP")"
ran="$("$APP/Contents/MacOS/bench" version 2>&1)"
log "Installed: $got; bench version: $ran; running: $(ps -axo pid=,command= | grep '[M]acOS/bench' | tr '\n' ' ')"
if [ "$got" = "$NEW" ] && case "$ran" in "bench $NEW"*) true ;; *) false ;; esac; then
	log "Check 4 passed: Bench $NEW replaced $OLD"
else
	fail "check 4: $APP is $got, and says \"$ran\""
fi
pid="$(app_pid)"
if [ -n "$pid" ] && [ "$pid" != "$old_pid" ]; then
	log "Check 4 passed: Sparkle opened the new Bench"
else
	fail "check 4: the new Bench isn't running"
fi
codesign --verify --deep --strict --verbose=2 "$APP" >>"$OUT/steps.log" 2>&1 || fail "check 4: the new Bench.app's signature isn't intact"
finish
