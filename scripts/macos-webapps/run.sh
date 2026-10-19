#!/bin/bash
# Checks, on a GitHub macOS runner, that Bench finds and docks the web apps
# that browsers make, for .github/workflows/macos-webapps.yml.
#
# Usage: run.sh <bench binary> <scanapps binary> <output folder>
#
# It makes a web app of the same site in Safari and in each Chromium browser
# that's installed (Chrome, Edge, Brave, and Vivaldi), then checks three
# things for each:
#
#  1. Bench's own scan of installed apps (scanapps, the add dialog's code)
#     finds it, and names the browser that made it.
#  2. Its windows belong to its own process, with its own bundle ID, so that
#     Bench can tell one web app's windows from another's and from the
#     browser's.
#  3. Bench docks it, listed in config.toml by bundle ID in a tab of its
#     own: when the script switches to the tab with Bench's shortcut, its
#     item is ready and its window sits in its pane.
#
# Safari makes web apps only from its File menu, so the script chooses Add to
# Dock with System Events, which the runner's image lets /bin/bash and
# osascript drive. The Chromium browsers make them from the
# WebAppInstallForceList policy, which they read from /Library/Managed
# Preferences, written with sudo before they start.
#
# The output folder gets results.md (also added to the job's summary), a
# screenshot of each step in shots/, steps.log, bench.log, and the scan.

set -u

BENCH="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
SCAN="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"
OUT="$(mkdir -p "$3" && cd "$3" && pwd)"
mkdir -p "$OUT/shots"

SITE="https://squoosh.app/"
APPS="$HOME/Applications"

log() { echo "[$(date +%T)] $*" | tee -a "$OUT/steps.log"; }
shot() { screencapture -x "$OUT/shots/$1.png" || log "screencapture failed"; }
failures=0
# Browsers that aren't installed, so weren't tested.
missing=()
fail() {
	log "FAIL: $*"
	failures=$((failures + 1))
}

# plist_get <Info.plist> <key>: a key's value, or nothing.
plist_get() { /usr/libexec/PlistBuddy -c "Print :$2" "$1" 2>/dev/null; }

# The runner's display starts at 1,024 by 768, too small for four browser
# windows side by side. Try for something roomier, as the demo does.
if brew install jakehilborn/jakehilborn/displayplacer >/dev/null 2>&1; then
	displayplacer list >"$OUT/displays.txt" 2>&1
	id=$(sed -n 's/^Persistent screen id: //p' "$OUT/displays.txt" | head -1)
	for res in 1920x1080 1680x1050 1600x900 1440x900; do
		if grep -q "res:$res" "$OUT/displays.txt" && displayplacer "id:$id res:$res origin:(0,0) degree:0" >>"$OUT/displays.txt" 2>&1; then
			log "Display set to $res"
			break
		fi
	done
	sleep 3
else
	log "couldn't install displayplacer"
fi

# --- Chromium browsers: the policy, then each browser. ---------------------

# name|app|policy domain|web apps folder
browsers=(
	"Chrome|/Applications/Google Chrome.app|com.google.Chrome|Chrome Apps.localized"
	"Edge|/Applications/Microsoft Edge.app|com.microsoft.Edge|Edge Apps.localized"
	"Brave|/Applications/Brave Browser.app|com.brave.Browser|Brave Browser Apps.localized"
	"Vivaldi|/Applications/Vivaldi.app|com.vivaldi.Vivaldi|Vivaldi Apps.localized"
)

sudo mkdir -p "/Library/Managed Preferences"
for b in "${browsers[@]}"; do
	IFS='|' read -r name app domain folder <<<"$b"
	if [ ! -d "$app" ]; then
		log "$name isn't installed, so not testing its web apps"
		missing+=("$name")
		continue
	fi
	policy="/Library/Managed Preferences/$domain.plist"
	sudo tee "$policy" >/dev/null <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>WebAppInstallForceList</key>
	<array>
		<dict>
			<key>url</key>
			<string>$SITE</string>
			<key>default_launch_container</key>
			<string>window</string>
			<key>custom_name</key>
			<string>Bench $name</string>
		</dict>
	</array>
	<key>HideFirstRunExperience</key>
	<true/>
	<key>DefaultBrowserSettingEnabled</key>
	<false/>
</dict>
</plist>
PLIST
	sudo chown root:wheel "$policy"
	sudo chmod 644 "$policy"
	sudo plutil -lint "$policy" >/dev/null || log "the $name policy isn't a valid property list"
done
sudo killall cfprefsd 2>/dev/null || true

for b in "${browsers[@]}"; do
	IFS='|' read -r name app domain folder <<<"$b"
	[ -d "$app" ] || continue
	log "Starting $name"
	open -a "$app" --args --no-first-run --no-default-browser-check
done

# --- Safari: File > Add to Dock. -------------------------------------------

log "Making Safari's web app"
open -a Safari "$SITE"
sleep 8
shot "safari-page"
osascript >"$OUT/safari.txt" 2>&1 <<'OSA'
tell application "Safari" to activate
delay 1
tell application "System Events" to tell process "Safari"
	click menu item "Add to Dock…" of menu "File" of menu bar 1
	delay 3
	try
		click button "Add" of sheet 1 of window 1
	on error
		try
			click button "Add" of window 1
		on error errText
			-- Record what's there, to find the button's real name.
			return "couldn't click Add: " & errText & linefeed & (entire contents of window 1 as text)
		end try
	end try
end tell
return "clicked Add"
OSA
log "Safari: $(head -c 300 "$OUT/safari.txt")"
shot "safari-added"

# --- Wait for the web apps. -------------------------------------------------

# webapps holds "name|bundle id|path" for each web app made.
webapps=()

for i in $(seq 1 40); do
	found=""
	for app in "$APPS"/*.app; do
		[ -d "$app" ] || continue
		id=$(plist_get "$app/Contents/Info.plist" CFBundleIdentifier)
		case "$id" in com.apple.Safari.WebApp.*) found="$app|$id" ;; esac
	done
	[ -n "$found" ] && break
	sleep 3
done
if [ -n "$found" ]; then
	webapps+=("Safari|${found#*|}|${found%%|*}")
	log "Safari made ${found%%|*} (${found#*|})"
else
	fail "Safari made no web app"
fi

for b in "${browsers[@]}"; do
	IFS='|' read -r name app domain folder <<<"$b"
	[ -d "$app" ] || continue
	made="" id=""
	# Policy installs are a low-priority task after the browser starts. The
	# browser copies the web app's bundle into place a file at a time, so
	# wait until its Info.plist names a bundle ID.
	for i in $(seq 1 60); do
		for shim in "$APPS/$folder"/*.app; do
			[ -d "$shim" ] || continue
			id=$(plist_get "$shim/Contents/Info.plist" CFBundleIdentifier)
			[ -n "$id" ] && made="$shim" && break
		done
		[ -n "$made" ] && break
		sleep 3
	done
	if [ -z "$made" ]; then
		fail "$name made no web app in $APPS/$folder"
		continue
	fi
	log "$name made $made ($id, browser $(plist_get "$made/Contents/Info.plist" CrBundleIdentifier))"
	webapps+=("$name|$id|$made")
done

{
	echo "== ~/Applications"
	find "$APPS" -maxdepth 3 -name '*.app' -prune -print 2>/dev/null
	for w in ${webapps[@]+"${webapps[@]}"}; do
		echo "== ${w##*|}/Contents/Info.plist"
		plutil -p "${w##*|}/Contents/Info.plist" 2>&1
	done
} >"$OUT/bundles.txt"

# --- 1. Bench's scan finds them. --------------------------------------------

"$SCAN" >"$OUT/scan.json" 2>"$OUT/scan.err" || fail "scanapps failed: $(cat "$OUT/scan.err")"
scan_row() {
	/usr/bin/python3 - "$OUT/scan.json" "$1" <<'PY'
import json, sys
for a in json.load(open(sys.argv[1])):
    if a["bundleId"].lower() == sys.argv[2].lower():
        print(a.get("webApp", "") + "|" + a["name"])
        break
PY
}

# --- 2. Their windows are their own. ----------------------------------------

# owner <bundle id>: "pid|name|executable|window count|first window's title"
# of the running app with that bundle ID, from NSRunningApplication, and its
# windows from the window server, or nothing. (System Events can't tell
# Chromium web apps apart: each one's process is called app_mode_loader, and
# System Events finds processes by name.)
owner() {
	osascript -l JavaScript - "$1" <<'JXA' 2>>"$OUT/jxa.err"
ObjC.import("AppKit");
ObjC.import("CoreGraphics");
function run(argv) {
	const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(argv[0]);
	if (apps.count === 0) return "";
	const app = apps.objectAtIndex(0), pid = app.processIdentifier;
	const all = ObjC.deepUnwrap(ObjC.castRefToObject(
		$.CGWindowListCopyWindowInfo($.kCGWindowListOptionAll | $.kCGWindowListExcludeDesktopElements, $.kCGNullWindowID))) || [];
	const ws = all.filter(w => w.kCGWindowOwnerPID === pid && w.kCGWindowLayer === 0 &&
		w.kCGWindowBounds.Width > 100 && w.kCGWindowBounds.Height > 100);
	const exe = ObjC.unwrap(app.executableURL.lastPathComponent);
	const title = ws.length ? (ws[0].kCGWindowName || "") : "";
	return [pid, ObjC.unwrap(app.localizedName), exe, ws.length, title].map(v => String(v).replace(/\|/g, "/")).join("|");
}
JXA
}

# frame_of <bundle id>: "x y w h" of the first standard-sized window of the
# running app with that bundle ID, in global top-left coordinates, like
# Bench's.
frame_of() {
	osascript -l JavaScript - "$1" <<'JXA' 2>>"$OUT/jxa.err"
ObjC.import("AppKit");
ObjC.import("CoreGraphics");
function run(argv) {
	const apps = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(argv[0]);
	if (apps.count === 0) return "";
	const pid = apps.objectAtIndex(0).processIdentifier;
	const all = ObjC.deepUnwrap(ObjC.castRefToObject(
		$.CGWindowListCopyWindowInfo($.kCGWindowListOptionOnScreenOnly | $.kCGWindowListExcludeDesktopElements, $.kCGNullWindowID))) || [];
	const w = all.find(w => w.kCGWindowOwnerPID === pid && w.kCGWindowLayer === 0 &&
		w.kCGWindowBounds.Width > 100 && w.kCGWindowBounds.Height > 100);
	if (!w) return "";
	const b = w.kCGWindowBounds;
	return [b.X, b.Y, b.Width, b.Height].join(" ");
}
JXA
}

for w in ${webapps[@]+"${webapps[@]}"}; do
	IFS='|' read -r name id path <<<"$w"
	open "$path"
done
sleep 10

results=()
pids=""
for w in ${webapps[@]+"${webapps[@]}"}; do
	IFS='|' read -r name id path <<<"$w"
	scanned=$(scan_row "$id")
	if [ -z "$scanned" ]; then
		fail "Bench's scan didn't find $name's web app ($id)"
	elif [ "${scanned%%|*}" != "$name" ]; then
		fail "Bench's scan named the browser of $id \"${scanned%%|*}\", not \"$name\""
	fi
	own=""
	for i in $(seq 1 20); do
		own=$(owner "$id")
		[ -n "$own" ] && [ "$(echo "$own" | cut -d'|' -f4)" -gt 0 ] && break
		sleep 2
	done
	pid=${own%%|*}
	if [ -z "$own" ]; then
		fail "no app with $name's web app's bundle ID ($id) is running"
	elif [ "$(echo "$own" | cut -d'|' -f4)" -eq 0 ]; then
		fail "$name's web app's own process ($id) has no windows"
	else
		# Each web app must be a process of its own, so that Bench can tell
		# its windows from other web apps' and the browser's.
		case " $pids " in *" $pid "*) fail "$name's web app shares its process ($pid) with another web app" ;; esac
		pids="$pids $pid"
	fi
	log "$name: scan '$scanned', owner '$own'"
	# Keep each part's fields, even when it's empty, for the results table.
	results+=("$name|$id|${scanned:-|}|${own:-||||}")
done
shot "web-apps-open"

# --- 3. Bench docks them. ---------------------------------------------------

# Each web app gets a tab of its own, with the whole window for its pane:
# Chromium's web app windows are at least 375 points tall, more than a pane
# has in a grid on the runner's display. The script switches to each tab
# with Bench's own shortcut, Control+Option and the tab's number.

export BENCH_HOME="$OUT/home"
mkdir -p "$BENCH_HOME"
touch "$BENCH_HOME/first-run-done"
n=${#webapps[@]}
# The key codes of 1 to 9 on the keyboard.
digits=(18 19 20 21 23 22 26 28 25)
[ "$n" -gt 9 ] && fail "only 9 tabs have shortcuts, so not checking more than 9 web apps" && n=9

# key <key code> [modifier...]: press a key through System Events, so that
# Bench's global shortcuts see it.
key() {
	local code=$1 mods="" m script
	shift
	for m in "$@"; do mods="$mods${mods:+, }$m down"; done
	script="tell application \"System Events\" to key code $code"
	[ -n "$mods" ] && script="$script using {$mods}"
	osascript -e "$script" || log "couldn't press key code $code"
}

# view_get <python expression>: a value from Bench's view.json, where the
# expression sees the view as v.
view_get() {
	/usr/bin/python3 -c 'import json, sys; v = json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "$BENCH_HOME/view.json" "$1" 2>/dev/null
}

# docked <bundle id>: "status|in pane" for the web app's item: its status in
# Bench's view, and whether its window sits in its pane's body, below the
# pane's header: at the body's top-left corner, and no larger than it.
docked() {
	local bx by
	read -r bx by _ _ <<<"$(osascript -e 'tell application "System Events" to tell process "bench" to get {position, size} of window 1' 2>/dev/null | tr -d ' ' | tr ',' ' ')"
	/usr/bin/python3 - "$BENCH_HOME/view.json" "$1" "${bx:-0}" "${by:-0}" "$(frame_of "$1")" <<'PY' 2>/dev/null
import json, sys
view, bundle = json.load(open(sys.argv[1])), sys.argv[2].lower()
ox, oy = float(sys.argv[3]), float(sys.argv[4])
frame = [float(v) for v in sys.argv[5].split()] if sys.argv[5] else None
item = next((i for i in view.get("items", {}).values() if i.get("bundleId", "").lower() == bundle), None)
if item is None:
    print("no item|")
    sys.exit()
inside = "not shown"
for pane in view["display"]["panes"]:
    if pane.get("active") == item["id"]:
        r = pane["body"]
        x, y, w, h = ox + r["x"], oy + r["y"], r["w"], r["h"]
        if frame is None:
            inside = "no window"
        else:
            fx, fy, fw, fh = frame
            ok = abs(fx - x) <= 2 and abs(fy - y) <= 2 and fx + fw <= x + w + 2 and fy + fh <= y + h + 2
            inside = "yes" if ok else "no (window %s, pane body %s)" % (frame, [x, y, w, h])
print(item.get("status", "") + "|" + inside)
PY
}

if [ "$n" -gt 0 ]; then
	{
		echo '[settings]'
		echo 'open_missing_apps = true'
		echo 'launch_timeout = "45s"'
		for k in $(seq 0 $((n - 1))); do
			IFS='|' read -r name id _ <<<"${webapps[$k]}"
			echo
			echo '[[tab]]'
			echo "name = \"$name\""
			echo 'layout = "single"'
			echo
			echo '  [[tab.pane]]'
			echo "  items = [{ app = \"$id\" }]"
		done
	} >"$BENCH_HOME/config.toml"
	cp "$BENCH_HOME/config.toml" "$OUT/config.toml"

	log "Starting Bench"
	BENCH_DEBUG=1 "$BENCH" >"$OUT/bench.log" 2>&1 &
	bench_pid=$!
	for i in $(seq 1 30); do
		sleep 1
		[ -f "$BENCH_HOME/view.json" ] && break
	done

	for k in $(seq 0 $((n - 1))); do
		IFS='|' read -r name id _ <<<"${webapps[$k]}"
		log "Switching to the $name tab"
		key "${digits[$k]}" control option
		state=""
		for i in $(seq 1 30); do
			sleep 1
			[ "$(view_get 'v["current"]')" = "$k" ] || continue
			state=$(docked "$id")
			[ "${state%%|*}" = ready ] && [ "${state#*|}" = yes ] && break
		done
		sleep 2
		state=$(docked "$id")
		shot "bench-$(echo "$name" | tr 'A-Z ' 'a-z-')"
		status=${state%%|*}
		inside=${state#*|}
		if [ "$(view_get 'v["current"]')" != "$k" ]; then
			fail "Bench didn't switch to the $name tab"
		elif [ "$status" != ready ]; then
			fail "Bench's item for $name's web app is \"$status\", not ready"
		elif [ "$inside" != yes ]; then
			fail "$name's web app's window isn't in its pane: $inside"
		fi
		results[$k]="${results[$k]}|$status|$inside"
		log "$name in Bench: $state"
	done

	kill -TERM "$bench_pid" 2>/dev/null
	wait "$bench_pid" 2>/dev/null
	sleep 2
	shot "after-quit"
fi

# --- Results. ---------------------------------------------------------------

{
	echo "## Web apps in Bench"
	echo
	echo "| Browser | Bundle ID | Bench's scan (browser, name) | Owner process (pid, name, executable, windows, title) | Item in Bench | Window in its pane |"
	echo "| --- | --- | --- | --- | --- | --- |"
	for r in ${results[@]+"${results[@]}"}; do
		IFS='|' read -r name id sb sn pid pname exe wins title status inside <<<"$r"
		echo "| $name | \`$id\` | $sb, $sn | $pid, $pname, $exe, $wins, $title | ${status:-} | ${inside:-} |"
	done
	echo
	[ ${#missing[@]} -eq 0 ] || echo "Not tested, because they aren't installed: ${missing[*]}."
	echo
	if [ "$failures" -eq 0 ]; then echo "Every check passed."; else echo "$failures checks failed: see steps.log."; fi
} >"$OUT/results.md"
cat "$OUT/results.md"
[ -n "${GITHUB_STEP_SUMMARY:-}" ] && cat "$OUT/results.md" >>"$GITHUB_STEP_SUMMARY"
[ "$failures" -eq 0 ]
