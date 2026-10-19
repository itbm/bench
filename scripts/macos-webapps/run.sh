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
#  3. Bench docks it, listed in config.toml by bundle ID: its item is ready
#     and its window sits in its pane.
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
	made=""
	# Policy installs are a low-priority task after the browser starts.
	for i in $(seq 1 60); do
		for shim in "$APPS/$folder"/*.app; do
			[ -d "$shim" ] && made="$shim" && break
		done
		[ -n "$made" ] && break
		sleep 3
	done
	if [ -z "$made" ]; then
		fail "$name made no web app in $APPS/$folder"
		continue
	fi
	id=$(plist_get "$made/Contents/Info.plist" CFBundleIdentifier)
	log "$name made $made ($id, browser $(plist_get "$made/Contents/Info.plist" CrBundleIdentifier))"
	webapps+=("$name|$id|$made")
done

{
	echo "== ~/Applications"
	find "$APPS" -maxdepth 3 -name '*.app' -prune -print 2>/dev/null
	for w in "${webapps[@]}"; do
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

# owner <bundle id>: "pid|process name|window count|first window's title" of
# the running process with that bundle ID, from System Events (the
# Accessibility API), or nothing.
owner() {
	osascript -l JavaScript - "$1" <<'JXA' 2>/dev/null
function run(argv) {
	const ps = Application("System Events").processes.whose({ bundleIdentifier: argv[0] })();
	if (ps.length === 0) return "";
	const p = ps[0], ws = p.windows();
	return [p.unixId(), p.name(), ws.length, ws.length ? ws[0].name() : ""].join("|");
}
JXA
}

for w in "${webapps[@]}"; do
	IFS='|' read -r name id path <<<"$w"
	open "$path"
done
sleep 10

results=()
for w in "${webapps[@]}"; do
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
		[ -n "$own" ] && [ "$(echo "$own" | cut -d'|' -f3)" -gt 0 ] && break
		sleep 2
	done
	if [ -z "$own" ]; then
		fail "no process with $name's web app's bundle ID ($id) is running"
	elif [ "$(echo "$own" | cut -d'|' -f3)" -eq 0 ]; then
		fail "$name's web app's own process ($id) has no windows"
	fi
	log "$name: scan '$scanned', owner '$own'"
	results+=("$name|$id|$scanned|$own")
done
shot "web-apps-open"

# --- 3. Bench docks them. ---------------------------------------------------

export BENCH_HOME="$OUT/home"
mkdir -p "$BENCH_HOME"
touch "$BENCH_HOME/first-run-done"
n=${#webapps[@]}
case $n in
0) layout="" ;;
1) layout=single ;;
2) layout=side ;;
3) layout=three ;;
*) layout=grid ;;
esac
if [ -n "$layout" ]; then
	slots=$n
	[ "$n" -gt 4 ] && slots=4
	{
		echo '[settings]'
		# Wide on any display the runner gets, so that each pane shows.
		echo 'compact_below = 800'
		echo 'open_missing_apps = true'
		echo 'launch_timeout = "45s"'
		echo
		echo '[[tab]]'
		echo 'name = "Web apps"'
		echo "layout = \"$layout\""
		for s in $(seq 0 $((slots - 1))); do
			items=""
			for j in $(seq "$s" "$slots" $((n - 1))); do
				IFS='|' read -r _ id _ <<<"${webapps[$j]}"
				items="$items${items:+, }{ app = \"$id\" }"
			done
			echo
			echo '  [[tab.pane]]'
			echo "  items = [$items]"
		done
	} >"$BENCH_HOME/config.toml"
	cp "$BENCH_HOME/config.toml" "$OUT/config.toml"

	log "Starting Bench"
	BENCH_DEBUG=1 "$BENCH" >"$OUT/bench.log" 2>&1 &
	bench_pid=$!

	# docked <bundle id>: "status|in pane" for the web app's item: its status
	# in Bench's view, and whether its window lies inside its pane, if the
	# pane shows it.
	docked() {
		local own pid bx by
		read -r bx by _ _ <<<"$(osascript -e 'tell application "System Events" to tell process "bench" to get {position, size} of window 1' 2>/dev/null | tr -d ' ' | tr ',' ' ')"
		own=$(osascript -l JavaScript - "$1" <<'JXA' 2>/dev/null
function run(argv) {
	const ps = Application("System Events").processes.whose({ bundleIdentifier: argv[0] })();
	if (ps.length === 0 || ps[0].windows().length === 0) return "";
	const w = ps[0].windows()[0], p = w.position(), s = w.size();
	return [p[0], p[1], s[0], s[1]].join(" ");
}
JXA
)
		/usr/bin/python3 - "$BENCH_HOME/view.json" "$1" "${bx:-0}" "${by:-0}" "$own" <<'PY' 2>/dev/null
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
        r = pane["rect"]
        x, y, w, h = ox + r["x"], oy + r["y"], r["w"], r["h"]
        if frame is None:
            inside = "no window"
        else:
            fx, fy, fw, fh = frame
            ok = fx >= x - 2 and fy >= y - 2 and fx + fw <= x + w + 2 and fy + fh <= y + h + 2
            inside = "yes" if ok else "no (window %s, pane %s)" % (frame, [x, y, w, h])
print(item.get("status", "") + "|" + inside)
PY
	}

	for i in $(seq 1 30); do
		sleep 2
		[ -f "$BENCH_HOME/view.json" ] || continue
		ready=0
		for w in "${webapps[@]}"; do
			IFS='|' read -r _ id _ <<<"$w"
			[ "$(docked "$id" | cut -d'|' -f1)" = ready ] && ready=$((ready + 1))
		done
		[ "$ready" -eq "$n" ] && break
	done
	sleep 3
	shot "bench-docked"

	for k in "${!results[@]}"; do
		IFS='|' read -r name id _ <<<"${results[$k]}"
		state=$(docked "$id")
		status=${state%%|*}
		inside=${state#*|}
		if [ "$status" != ready ]; then
			fail "Bench's item for $name's web app is \"$status\", not ready"
		elif [ "$inside" != yes ] && [ "$inside" != "not shown" ]; then
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
	echo "| Browser | Bundle ID | Bench's scan (browser, name) | Owner process (pid, name, windows, title) | Item in Bench | Window in its pane |"
	echo "| --- | --- | --- | --- | --- | --- |"
	for r in "${results[@]}"; do
		IFS='|' read -r name id sb sn pid pname wins title status inside <<<"$r"
		echo "| $name | \`$id\` | $sb, $sn | $pid, $pname, $wins, $title | ${status:-} | ${inside:-} |"
	done
	echo
	if [ "$failures" -eq 0 ]; then echo "Every check passed."; else echo "$failures checks failed: see steps.log."; fi
} >"$OUT/results.md"
cat "$OUT/results.md"
[ -n "${GITHUB_STEP_SUMMARY:-}" ] && cat "$OUT/results.md" >>"$GITHUB_STEP_SUMMARY"
[ "$failures" -eq 0 ]
