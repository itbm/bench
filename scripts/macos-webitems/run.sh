#!/bin/bash
# Checks, on a GitHub macOS runner, how Bench's Web items deal with pages
# that try to put another site in place of the item's page, or to keep Bench
# behind their dialogs, for .github/workflows/macos-webitems.yml. Bench's
# rules for these depend on what WebKit tells it about each navigation and
# dialog (the frame that started a navigation, its type, redirects), which
# only a real Mac shows.
#
# Usage: run.sh <bench binary> <server binary> <output folder>
#
# The server is scripts/macos-webitems/server. It serves the test pages over
# https on two sites: the items' own, https://localhost:8443, and another,
# https://127.0.0.1:9443. Bench counts each local host and port as a site of
# its own, and WebKit counts the two hosts as two sites. A Web item's address
# must be https, so the script makes a throwaway certificate authority, and a
# certificate for both hosts, with openssl, and trusts the authority
# system-wide.
#
# Bench, started with BENCH_DEBUG=1, gets a Web item in a tab of its own for
# each check:
#
#  a. A page whose frame, from the other site, sets top.location to the
#     other site just after the user clicks in it (once, with cliclick).
#  b. A link that the user clicks to a page on the item's site that sends
#     them on to the other site with location.replace.
#  c. A page that brings up an alert of 1,999 characters, then one more
#     with 2,000,000 combining marks (U+0301).
#  d. A page that brings up an alert, then reloads itself, again and again.
#
# For a and b, the item must stay on its site, and the other site's page
# must open in the default browser: Bench's view.json (the item's url, host,
# and offSite), Bench's log of its navigation decisions, and the server's
# log of the pages that the web view fetched and those the browser opened
# must all say so. For c, Bench must keep answering System Events while the
# alert shows, and the alert's text must be short. For d, the second alert
# must offer its stop checkbox, and once that's ticked, no alert may show
# for 10 seconds, while the page keeps reloading and asking. If no alert
# offers it, or ticking it doesn't take, that fails, and the same 10 seconds
# check instead that Bench holds the page's dialogs back by itself.
#
# The runner's image grants Accessibility and Screen Recording to /bin/bash
# and its children, so cliclick can click in the pages, and System Events
# can press Bench's shortcuts, and read and press its dialogs.
#
# The output folder gets results.md (also added to the job's summary), a
# screenshot of each step in shots/, steps.log, bench.log, decisions.txt
# (each item's navigation decisions from bench.log), server.log,
# requests.jsonl (the server's log of each request), diagnostics.txt, the
# certificates (not their keys) in certs/, and Bench's support folder in
# home/.

set -u

BENCH="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
SERVER="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"
OUT="$(mkdir -p "$3" && cd "$3" && pwd)"
mkdir -p "$OUT/shots" "$OUT/certs"
# Files that stay out of the artifact: the certificates' private keys, and
# the script that reads Bench's dialogs.
WORK="$(mktemp -d)"

ITEM="https://localhost:8443"
OTHER="https://127.0.0.1:9443"
# The other site's host, as Bench's log names it.
OTHER_HOST="127.0.0.1:9443"
REQUESTS="$OUT/requests.jsonl"
: >"$REQUESTS"

export BENCH_HOME="$OUT/home"
mkdir -p "$BENCH_HOME"
touch "$BENCH_HOME/first-run-done"

# log goes to standard error, so that it doesn't mix with what functions
# print for the caller.
log() { echo "[$(date +%T)] $*" | tee -a "$OUT/steps.log" >&2; }
shots=0
shot() {
	shots=$((shots + 1))
	screencapture -x "$OUT/shots/$(printf %02d "$shots")-$1.png" || log "screencapture failed"
}

failures=0
# results holds "check|result|what the script saw" for each row of the
# results table.
results=()
# record <check> <yes or no> <what the script saw>: notes that a check
# passed (yes) or failed.
record() {
	local seen
	seen=$(printf '%s' "$3" | tr '|\n' '/ ')
	if [ "$2" = yes ]; then
		results+=("$1|pass|$seen")
		log "pass: $1: $3"
	else
		results+=("$1|FAIL|$seen")
		failures=$((failures + 1))
		log "FAIL: $1: $3"
	fi
}

# now: the time in seconds since 1970, to the millisecond, as the server's
# log has it.
now() { perl -MTime::HiRes=time -e 'printf "%.3f\n", time'; }

# since <time>: how many seconds have passed since a time from now, to a
# tenth of a second.
since() { awk -v a="$1" -v b="$(now)" 'BEGIN { printf "%.1f", b - a }'; }

# with_timeout <seconds> <command> [argument...]: runs a command, and stops
# it if it takes longer than that, which fails.
with_timeout() {
	local secs=$1 pid watcher status
	shift
	"$@" &
	pid=$!
	(sleep "$secs" && kill -9 "$pid") >/dev/null 2>&1 &
	watcher=$!
	wait "$pid" 2>/dev/null
	status=$?
	kill "$watcher" 2>/dev/null
	wait "$watcher" 2>/dev/null
	return "$status"
}

# wait_for <seconds> <command> [argument...]: runs a command every half
# second until it succeeds, for up to that long.
wait_for() {
	local end=$((SECONDS + $1))
	shift
	while :; do
		"$@" && return 0
		[ "$SECONDS" -lt "$end" ] || return 1
		sleep 0.5
	done
}

# key <key code> [modifier...]: presses a key through System Events, so
# that Bench's global shortcuts see it. Key codes: 1 is 18, 2 is 19, 3 is
# 20, 4 is 21.
key() {
	local code=$1 mods="" m script
	shift
	for m in "$@"; do mods="$mods${mods:+, }$m down"; done
	script="tell application \"System Events\" to key code $code"
	[ -n "$mods" ] && script="$script using {$mods}"
	osascript -e "$script" >/dev/null || log "couldn't press key code $code"
}
digits=(18 19 20 21)

# bench_frame: "x y w h" of Bench's window, in global top-left coordinates,
# from System Events: its standard window, not a dialog. Its content fills
# the window.
bench_frame() {
	with_timeout 10 osascript \
		-e 'tell application "System Events"' \
		-e 'tell process "bench"' \
		-e '	try' \
		-e '		set w to first window whose subrole is "AXStandardWindow"' \
		-e '	on error' \
		-e '		set w to window 1' \
		-e '	end try' \
		-e '	get {position, size} of w' \
		-e 'end tell' \
		-e 'end tell' 2>/dev/null | tr -d ' ' | tr ',' ' '
}

# activate_bench: brings Bench forward. A page's dialog shows only while
# Bench is active, and a click must land on Bench's window, which sits
# below other apps' windows.
activate_bench() {
	osascript -e 'tell application "System Events" to set frontmost of process "bench" to true' >/dev/null 2>&1 || log "couldn't bring Bench forward"
	sleep 1
}

# view_get <python expression>: a value from Bench's view.json, where the
# expression sees the view as v.
view_get() {
	/usr/bin/python3 -c 'import json, sys; v = json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "$BENCH_HOME/view.json" "$1" 2>/dev/null
}

# page_of <item ID>: "url|host|offSite" of the page that a Web item shows,
# from view.json, where offSite is true or false.
page_of() {
	/usr/bin/python3 - "$BENCH_HOME/view.json" "$1" <<'PY' 2>/dev/null
import json, sys
item = json.load(open(sys.argv[1]))["items"].get(sys.argv[2], {})
print("|".join([item.get("url", ""), item.get("host", ""), "true" if item.get("offSite") else "false"]))
PY
}

# switch_tab <index>: switches to a tab with Bench's shortcut, waits until
# Bench shows it, and prints the ID of the item in its pane, or fails.
switch_tab() {
	key "${digits[$1]}" control option
	for _ in $(seq 1 30); do
		if [ "$(view_get 'v["current"]')" = "$1" ]; then
			view_get 'v["display"]["panes"][0]["active"]'
			return 0
		fi
		sleep 0.5
	done
	return 1
}

# pane_centre: "x y" of the middle of the current tab's pane, where its Web
# item's page is, on the screen; or it fails.
pane_centre() {
	local bx by
	read -r bx by _ _ <<<"$(bench_frame)"
	[ -n "$bx" ] && [ -n "$by" ] || return 1
	/usr/bin/python3 - "$BENCH_HOME/view.json" "$bx" "$by" <<'PY' 2>/dev/null
import json, sys
body = json.load(open(sys.argv[1]))["display"]["panes"][0]["body"]
print(int(float(sys.argv[2]) + body["x"] + body["w"] / 2), int(float(sys.argv[3]) + body["y"] + body["h"] / 2))
PY
}

# requests <since> <python condition>: the server's log entries for
# requests since a time (see now) for which the condition, on an entry e,
# holds, one per line: "time site path query fresh-or-from-page
# Sec-Fetch-Site". A fresh request is one that no page asked for: the
# default browser opening an address that Bench passed it, or Bench's own
# load of an item's address (see the server's isFresh).
requests() {
	/usr/bin/python3 - "$REQUESTS" "$1" "$2" <<'PY' 2>/dev/null
import json, sys
since, cond = float(sys.argv[2]), sys.argv[3]
for line in open(sys.argv[1], errors="replace"):
    try:
        e = json.loads(line)
    except ValueError:
        continue
    if "path" in e and e["t"] >= since and eval(cond, {}, {"e": e}):
        print(e["t"], e["site"], e["path"], e.get("query") or "-", "fresh" if e.get("fresh") else "from-page", e.get("fetchSite") or "-")
PY
}

# count <since> <python condition>: how many such requests there are.
count() { requests "$1" "$2" | wc -l | tr -d ' '; }

# has_request <since> <python condition>: whether there's such a request.
has_request() { [ "$(count "$1" "$2")" -gt 0 ]; }

# beacon_values <since> <event> <field>: a field's values in the beacons of
# an event that the pages sent since a time, one per line.
beacon_values() {
	requests "$1" 'e["path"] == "/beacon" and ("&" + e.get("query", "") + "&").find("&e='"$2"'&") >= 0' |
		awk '{ print $4 }' | tr '&' '\n' | sed -n "s/^$3=//p"
}

# log_lines: how many lines bench.log has, to look only at those after.
log_lines() { wc -l <"$OUT/bench.log" | tr -d ' '; }

# decisions <item ID> <lines>: Bench's logged decisions about the item's
# navigations and new windows, after the first lines of bench.log, one per
# line: "what|decision|host|from|kind".
decisions() {
	/usr/bin/python3 - "$OUT/bench.log" "$1" "$2" <<'PY' 2>/dev/null
import re, sys
pair = re.compile(r'(\w+)=("(?:[^"\\]|\\.)*"|\S*)')
for n, line in enumerate(open(sys.argv[1], errors="replace")):
    if n < int(sys.argv[3]) or 'msg="web ' not in line:
        continue
    f = {k: v[1:-1] if v.startswith('"') else v for k, v in pair.findall(line)}
    if f.get("item") == sys.argv[2]:
        print("|".join(f.get(k, "") for k in ("msg", "decision", "host", "from", "kind")))
PY
}

# has_decision <item ID> <lines> <decision> <host>: whether Bench logged
# that decision about a navigation of the item's page to that host.
has_decision() { decisions "$1" "$2" | grep -q "^web navigation|$3|$4|"; }

# Bench's dialogs, read and pressed through System Events.
cat >"$WORK/dialogs.js" <<'JXA'
// Reads and presses Bench's dialogs through System Events, for run.sh. A
// dialog is a window of Bench's with buttons but no close button, other
// than a standard window, such as a page's alert.
//
//   dialogs.js list         Prints a JSON array that describes each dialog:
//                           its subrole, the length of each of its texts in
//                           UTF-16 code units, with how it starts, its
//                           buttons' titles, and its checkboxes' titles,
//                           with whether each is ticked.
//   dialogs.js press <title>  Presses that button of the first dialog.
//   dialogs.js tick         Ticks the first dialog's checkbox, and prints
//                           whether it's ticked.
function run(argv) {
	const se = Application("System Events");
	const get = f => { try { return f(); } catch (e) { return null; } };
	const found = se.processes.whose({ name: "bench" });
	if (found.length === 0) return "[]";
	const dialogs = [];
	for (const w of found[0].windows()) {
		const subrole = get(() => w.subrole());
		const buttons = get(() => w.buttons()) || [];
		if (subrole === null || subrole === "AXStandardWindow" || buttons.length === 0) continue;
		if (buttons.some(b => get(() => b.subrole()) === "AXCloseButton")) continue;
		dialogs.push({ window: w, subrole: subrole });
	}
	const parts = d => {
		const texts = [], buttons = [], checkboxes = [];
		for (const el of d.window.entireContents()) {
			const role = get(() => el.role());
			const title = () => String(get(() => el.title()) || get(() => el.name()) || "");
			if (role === "AXStaticText" || role === "AXTextArea" || role === "AXTextField") {
				texts.push(String(get(() => el.value()) || ""));
			} else if (role === "AXButton") {
				buttons.push({ el: el, title: title() });
			} else if (role === "AXCheckBox") {
				checkboxes.push({ el: el, title: title(), ticked: Number(get(() => el.value())) === 1 });
			}
		}
		return { texts: texts, buttons: buttons, checkboxes: checkboxes };
	};
	const command = argv[0] || "list";
	if (command === "list") {
		return JSON.stringify(dialogs.map(d => {
			const p = parts(d);
			return {
				subrole: d.subrole,
				texts: p.texts.map(t => ({ length: t.length, start: t.slice(0, 40) })),
				buttons: p.buttons.map(b => b.title),
				checkboxes: p.checkboxes.map(c => ({ title: c.title, ticked: c.ticked })),
			};
		}));
	}
	if (dialogs.length === 0) throw new Error("Bench shows no dialog");
	const p = parts(dialogs[0]);
	if (command === "press") {
		const button = p.buttons.find(b => b.title === argv[1]);
		if (!button) throw new Error("the dialog has no " + argv[1] + " button");
		se.click(button.el);
		return "pressed " + argv[1];
	}
	if (command === "tick") {
		if (p.checkboxes.length === 0) throw new Error("the dialog has no checkbox");
		const box = p.checkboxes[0];
		if (!box.ticked) se.click(box.el);
		return Number(get(() => box.el.value())) === 1 ? "ticked" : "not ticked";
	}
	throw new Error("unknown command " + command);
}
JXA

# dialogs [list | press <title> | tick]: see dialogs.js. Bench must answer
# within 10 seconds.
dialogs() { with_timeout 10 osascript -l JavaScript "$WORK/dialogs.js" "$@" 2>>"$OUT/jxa.err"; }

# wait_dialog <seconds>: waits for Bench to show a dialog, for up to that
# long, and prints dialogs' description of them; or it fails.
wait_dialog() {
	local end=$((SECONDS + $1)) d
	while :; do
		d=$(dialogs list)
		case "$d" in "["*"{"*)
			echo "$d"
			return 0
			;;
		esac
		[ "$SECONDS" -lt "$end" ] || return 1
		sleep 0.5
	done
}

# json_get <JSON> <python expression>: a value from JSON, which the
# expression sees as d.
json_get() {
	/usr/bin/python3 -c 'import json, sys; d = json.loads(sys.argv[1]); print(eval(sys.argv[2]))' "$1" "$2" 2>/dev/null
}

# bench_answers: whether System Events can read Bench's windows within 5
# seconds, which it can't while Bench's main thread is busy.
bench_answers() {
	with_timeout 5 osascript -e 'tell application "System Events" to count windows of process "bench"' >/dev/null 2>&1
}

# The default browser, which Bench opens other sites' pages with.
BROWSER_ID=""
BROWSER_PATH=""

# quit_browser: quits the default browser, if it's running, so that its
# windows don't cover Bench's, where the script clicks.
quit_browser() {
	[ -n "$BROWSER_ID" ] || return 0
	osascript -e "if application id \"$BROWSER_ID\" is running then tell application id \"$BROWSER_ID\" to quit" >/dev/null 2>&1
	if ! wait_for 10 browser_stopped; then
		log "the default browser didn't quit; stopping it"
		pkill -9 -f "$BROWSER_PATH/Contents/MacOS/" 2>/dev/null
		sleep 1
	fi
}
browser_stopped() { [ "$(osascript -e "application id \"$BROWSER_ID\" is running" 2>/dev/null)" != true ]; }

# finish: writes results.md, adds it to the job's summary, and exits,
# failing if a check failed.
finish() {
	[ -n "${bench_pid:-}" ] && kill -TERM "$bench_pid" 2>/dev/null && wait "$bench_pid" 2>/dev/null
	[ -n "${server_pid:-}" ] && kill -TERM "$server_pid" 2>/dev/null && wait "$server_pid" 2>/dev/null
	quit_browser
	for id in ${items[@]+"${items[@]}"}; do
		echo "== $id"
		decisions "$id" 0
	done >"$OUT/decisions.txt"
	rm -rf "$WORK"
	{
		echo "## Web items on a real Mac"
		echo
		echo "| Check | Result | What the script saw |"
		echo "| --- | --- | --- |"
		for r in ${results[@]+"${results[@]}"}; do
			IFS='|' read -r check result seen <<<"$r"
			echo "| $check | $result | $seen |"
		done
		echo
		if [ "$failures" -eq 0 ]; then echo "Every check passed."; else echo "$failures of ${#results[@]} checks failed: see steps.log."; fi
	} >"$OUT/results.md"
	cat "$OUT/results.md"
	[ -n "${GITHUB_STEP_SUMMARY:-}" ] && cat "$OUT/results.md" >>"$GITHUB_STEP_SUMMARY"
	[ "$failures" -eq 0 ]
	exit
}

# items holds each scenario's item ID, for decisions.txt.
items=()

{
	echo "== macOS"
	sw_vers
	echo "== Screen"
	system_profiler SPDisplaysDataType | grep -E "Resolution|Display Type" || true
} >"$OUT/diagnostics.txt" 2>&1

# --- The certificates and the server. ----------------------------------------

log "Making the certificates"
cat >"$WORK/openssl.cnf" <<'CNF'
[req]
distinguished_name = dn
prompt = no

[dn]
CN = Bench web items check CA

[ca]
basicConstraints = critical, CA:TRUE
keyUsage = critical, keyCertSign, cRLSign
subjectKeyIdentifier = hash

[leaf]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:localhost, IP:127.0.0.1
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF
# macOS trusts a server's certificate only if it names the host in
# subjectAltName, allows serverAuth, and lasts at most 825 days.
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 7 -config "$WORK/openssl.cnf" -extensions ca \
	-keyout "$WORK/ca-key.pem" -out "$WORK/ca.pem" >>"$OUT/diagnostics.txt" 2>&1 &&
	openssl req -new -newkey rsa:2048 -nodes -sha256 -config "$WORK/openssl.cnf" -subj "/CN=localhost" \
		-keyout "$WORK/localhost-key.pem" -out "$WORK/localhost.csr" >>"$OUT/diagnostics.txt" 2>&1 &&
	openssl x509 -req -sha256 -days 7 -in "$WORK/localhost.csr" -CA "$WORK/ca.pem" -CAkey "$WORK/ca-key.pem" \
		-CAcreateserial -extfile "$WORK/openssl.cnf" -extensions leaf -out "$WORK/localhost.pem" >>"$OUT/diagnostics.txt" 2>&1
made=$?
cp "$WORK/ca.pem" "$WORK/localhost.pem" "$OUT/certs/" 2>/dev/null
if [ "$made" -ne 0 ]; then
	record "Setup: make the certificates" no "openssl failed: see diagnostics.txt"
	finish
fi

# Trust the authority for every app, WebKit's web views and the default
# browser too. Changing the Mac's trust settings asks for an
# administrator's password, even with sudo, unless the authorization rule
# for it allows anyone, and nobody is there to type one. The runner is
# thrown away after the job, so none of this needs undoing.
{
	echo "== security add-trusted-cert"
	sudo security authorizationdb write com.apple.trust-settings.admin allow
	sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$WORK/ca.pem"
	echo "== security verify-cert"
} >>"$OUT/diagnostics.txt" 2>&1
if security verify-cert -L -c "$WORK/localhost.pem" -p ssl -s localhost >>"$OUT/diagnostics.txt" 2>&1; then
	record "Setup: macOS trusts the test server's certificate" yes "security verify-cert succeeded for localhost"
else
	record "Setup: macOS trusts the test server's certificate" no "security verify-cert failed: see diagnostics.txt"
	finish
fi

log "Starting the test server"
"$SERVER" -cert "$WORK/localhost.pem" -key "$WORK/localhost-key.pem" -log "$REQUESTS" >"$OUT/server.log" 2>&1 &
server_pid=$!
ready() { curl -sS --noproxy '*' --cacert "$WORK/ca.pem" -o /dev/null -w '%{http_code}' "$1/ready" 2>>"$OUT/server.log" | grep -q 204; }
if wait_for 20 ready "$ITEM" && wait_for 5 ready "$OTHER"; then
	record "Setup: the test server answers on both sites" yes "$ITEM and $OTHER"
else
	record "Setup: the test server answers on both sites" no "see server.log"
	finish
fi

# The default browser must open the test server's pages too, so that the
# server's log shows which pages Bench sent there.
browser=$(osascript -l JavaScript -e '
	ObjC.import("AppKit");
	const app = $.NSWorkspace.sharedWorkspace.URLForApplicationToOpenURL($.NSURL.URLWithString("https://127.0.0.1/"));
	app.isNil() ? "" : ObjC.unwrap($.NSBundle.bundleWithURL(app).bundleIdentifier) + "|" + ObjC.unwrap(app.path);' 2>>"$OUT/jxa.err")
BROWSER_ID=${browser%%|*}
BROWSER_PATH=${browser#*|}
echo "== Default browser: $browser" >>"$OUT/diagnostics.txt"
log "The default browser is ${browser:-unknown}"
t0=$(now)
open "$ITEM/ping"
if wait_for 45 has_request "$t0" 'e["path"] == "/ping" and e["fresh"]'; then
	record "Setup: the default browser opens the test server's pages" yes "$BROWSER_ID fetched /ping in $(since "$t0") s"
else
	record "Setup: the default browser opens the test server's pages" no "$BROWSER_ID didn't fetch /ping in 45 s, so the checks below can't tell what it opened: see server.log and requests.jsonl"
fi
shot "browser-ping"
quit_browser

# --- Bench. -------------------------------------------------------------------

# Each check's item has a tab of its own. Other apps' windows float, and
# don't switch tabs, so that the default browser's windows stay out of
# Bench's panes.
cat >"$BENCH_HOME/config.toml" <<TOML
[settings]
open_missing_apps = false
new_windows = "float"
follow_focus = "off"
live_previews = false

[[tab]]
name = "Frame"
layout = "single"

  [[tab.pane]]
  items = [{ web = "$ITEM/a/", name = "Frame" }]

[[tab]]
name = "Redirector"
layout = "single"

  [[tab.pane]]
  items = [{ web = "$ITEM/b/", name = "Redirector" }]

[[tab]]
name = "Long alert"
layout = "single"

  [[tab.pane]]
  items = [{ web = "$ITEM/c/", name = "Long alert" }]

[[tab]]
name = "Alert loop"
layout = "single"

  [[tab.pane]]
  items = [{ web = "$ITEM/d/", name = "Alert loop" }]
TOML

log "Starting Bench"
BENCH_DEBUG=1 "$BENCH" >"$OUT/bench.log" 2>&1 &
bench_pid=$!
has_window() { [ -n "$(bench_frame)" ]; }
if ! wait_for 30 test -f "$BENCH_HOME/view.json" || ! wait_for 30 has_window; then
	record "Setup: Bench starts" no "no view.json or no window in 60 s: see bench.log"
	finish
fi
sleep 3
activate_bench
shot "bench-open"

# --- a. A frame from another site sets top.location. -------------------------

check_frame() {
	local item mark t0 pos x y found page url host off seen fresh fromPage
	log "a: a frame from another site sets top.location just after a click in it"
	activate_bench
	if ! item=$(switch_tab 0); then
		record "a. Frame: the item's tab shows" no "Bench didn't switch to the Frame tab"
		return
	fi
	items+=("$item")
	if ! wait_for 30 has_request 0 'e["path"] == "/a/frame" and e["site"] == "other"'; then
		record "a. Frame: the page and its frame load" no "the web view didn't fetch the frame, /a/frame, from $OTHER"
		return
	fi
	sleep 2
	shot "a-loaded"
	mark=$(log_lines)
	t0=$(now)
	found=no
	for _ in 1 2 3; do
		if ! pos=$(pane_centre); then
			log "couldn't find the Frame tab's pane"
			break
		fi
		read -r x y <<<"$pos"
		log "Clicking in the frame at $x, $y"
		cliclick "c:$x,$y"
		if wait_for 3 has_request "$t0" 'e["path"] == "/beacon" and "e=a-click" in e.get("query", "")'; then
			found=yes
			break
		fi
	done
	if [ "$found" != yes ]; then
		record "a. Frame: a click lands in the frame" no "the frame didn't report a click: see shots"
		return
	fi
	# The frame sets top.location half a second after the click.
	wait_for 15 has_decision "$item" "$mark" in-browser "$OTHER_HOST"
	wait_for 30 has_request "$t0" 'e["path"] == "/a/away" and e["fresh"]'
	sleep 3
	shot "a-after-click"
	quit_browser
	activate_bench
	shot "a-item"

	seen=$(decisions "$item" "$mark" | grep "|$OTHER_HOST|" | tr '\n' ';')
	if has_decision "$item" "$mark" in-browser "$OTHER_HOST" && ! has_decision "$item" "$mark" in-view "$OTHER_HOST"; then
		record "a. Frame: Bench sends the frame's navigation to the browser" yes "$seen"
	else
		record "a. Frame: Bench sends the frame's navigation to the browser" no "decisions about $OTHER_HOST: ${seen:-none}"
	fi
	page=$(page_of "$item")
	IFS='|' read -r url host off <<<"$page"
	if [ "${url#"$ITEM"/}" != "$url" ] && [ "$host" = localhost ] && [ "$off" = false ]; then
		record "a. Frame: the item stays on its site" yes "url $url, host $host, offSite $off"
	else
		record "a. Frame: the item stays on its site" no "url $url, host ${host:-none}, offSite $off"
	fi
	fresh=$(count "$t0" 'e["path"] == "/a/away" and e["fresh"]')
	fromPage=$(count "$t0" 'e["path"] == "/a/away" and not e["fresh"]')
	if [ "$fresh" -gt 0 ] && [ "$fromPage" -eq 0 ]; then
		record "a. Frame: the other site's page opens in the default browser" yes "requests for /a/away: $fresh from the browser, none from the web view"
	else
		record "a. Frame: the other site's page opens in the default browser" no "requests for /a/away: $fresh from the browser, $fromPage from the web view"
	fi
}
check_frame

# --- b. A clicked link to a page that calls location.replace. ----------------

check_redirector() {
	local item mark t0 pos x y found page url host off seen fresh fromPage
	log "b: a clicked link to a page on the item's site that sends the user on with location.replace"
	activate_bench
	if ! item=$(switch_tab 1); then
		record "b. Redirector: the item's tab shows" no "Bench didn't switch to the Redirector tab"
		return
	fi
	items+=("$item")
	if ! wait_for 30 has_request 0 'e["path"] == "/b/"'; then
		record "b. Redirector: the page loads" no "the web view didn't fetch /b/"
		return
	fi
	sleep 2
	shot "b-loaded"
	mark=$(log_lines)
	t0=$(now)
	found=no
	for _ in 1 2 3; do
		if ! pos=$(pane_centre); then
			log "couldn't find the Redirector tab's pane"
			break
		fi
		read -r x y <<<"$pos"
		log "Clicking the link at $x, $y"
		cliclick "c:$x,$y"
		if wait_for 3 has_request "$t0" 'e["path"] == "/b/hop"'; then
			found=yes
			break
		fi
	done
	if [ "$found" != yes ]; then
		record "b. Redirector: a click follows the link" no "the web view didn't fetch /b/hop: see shots"
		return
	fi
	wait_for 15 has_decision "$item" "$mark" in-browser "$OTHER_HOST"
	wait_for 30 has_request "$t0" 'e["path"] == "/b/away" and e["fresh"]'
	sleep 3
	shot "b-after-click"
	quit_browser
	activate_bench
	shot "b-item"

	seen=$(decisions "$item" "$mark" | tr '\n' ';')
	if has_decision "$item" "$mark" in-browser "$OTHER_HOST" && ! has_decision "$item" "$mark" in-view "$OTHER_HOST"; then
		record "b. Redirector: Bench sends the redirector's navigation to the browser" yes "$seen"
	else
		record "b. Redirector: Bench sends the redirector's navigation to the browser" no "decisions: ${seen:-none}"
	fi
	page=$(page_of "$item")
	IFS='|' read -r url host off <<<"$page"
	if [ "$url" = "$ITEM/b/hop" ] && [ "$host" = localhost ] && [ "$off" = false ]; then
		record "b. Redirector: the item stays on its site" yes "url $url, host $host, offSite $off"
	else
		record "b. Redirector: the item stays on its site" no "url $url (want $ITEM/b/hop), host ${host:-none}, offSite $off"
	fi
	fresh=$(count "$t0" 'e["path"] == "/b/away" and e["fresh"]')
	fromPage=$(count "$t0" 'e["path"] == "/b/away" and not e["fresh"]')
	if [ "$fresh" -gt 0 ] && [ "$fromPage" -eq 0 ]; then
		record "b. Redirector: the other site's page opens in the default browser" yes "requests for /b/away: $fresh from the browser, none from the web view"
	else
		record "b. Redirector: the other site's page opens in the default browser" no "requests for /b/away: $fresh from the browser, $fromPage from the web view"
	fi
}
check_redirector

# --- c. An alert with 2,000,000 combining marks. -----------------------------

check_long_alert() {
	local item t0 dialog heading longest sent s ok times answered page
	log "c: an alert of 1,999 characters, then one with 2,000,000 combining marks"
	activate_bench
	if ! item=$(switch_tab 2); then
		record "c. Long alert: the item's tab shows" no "Bench didn't switch to the Long alert tab"
		return
	fi
	items+=("$item")
	t0=$(now)
	if ! dialog=$(wait_dialog 30); then
		shot "c-no-alert"
		if bench_answers; then
			record "c. Long alert: the alert shows" no "no dialog in 30 s, though Bench answers System Events"
		else
			record "c. Long alert: Bench keeps answering" no "Bench doesn't answer System Events, and showed no dialog in 30 s"
		fi
		return
	fi
	shot "c-alert"
	log "c: the alert: $dialog"
	sent=$(beacon_values 0 c-alert length | tail -1)
	heading=$(json_get "$dialog" 'any(t["start"] == "localhost says" for t in d[0]["texts"])')
	longest=$(json_get "$dialog" 'max([t["length"] for t in d[0]["texts"]] or [0])')
	if [ "$heading" = True ] && [ "${longest:-0}" -gt 0 ] && [ "${longest:-0}" -le 2100 ]; then
		record "c. Long alert: the alert's text is short" yes "headed \"localhost says\"; its longest text has $longest UTF-16 code units, of the page's ${sent:-?}"
	else
		record "c. Long alert: the alert's text is short" no "headed \"localhost says\": $heading; its longest text has ${longest:-?} UTF-16 code units, of the page's ${sent:-?}"
	fi
	ok=yes
	times=""
	for _ in 1 2 3; do
		s=$(now)
		if bench_answers; then
			times="$times $(since "$s") s"
		else
			ok=no
			times="$times no answer in 5 s"
		fi
		sleep 1
	done
	record "c. Long alert: Bench answers System Events while the alert shows" "$ok" "three reads of its windows:$times"
	dialogs press OK >/dev/null
	if wait_for 10 has_request "$t0" '"e=c-answered" in e.get("query", "")' && bench_answers; then
		answered=$(beacon_values "$t0" c-answered ms | tail -1)
		record "c. Long alert: OK closes the alert, and Bench still answers" yes "the page's alert() returned after ${answered:-?} ms"
	else
		record "c. Long alert: OK closes the alert, and Bench still answers" no "the page didn't report its alert closing, or Bench doesn't answer System Events"
	fi
	sleep 1
	shot "c-after"
	page=$(page_of "$item")
	log "c: the item's page: $page"
}
check_long_alert

# --- d. A page that brings up an alert and reloads, again and again. ---------

check_alert_loop() {
	local item d alerts boxes offered stopped_by ticked ticked_on not_ticked w0 end seen reloads answered slowest page
	log "d: a page that brings up an alert, then reloads itself, again and again"
	activate_bench
	if ! item=$(switch_tab 3); then
		record "d. Alert loop: the item's tab shows" no "Bench didn't switch to the Alert loop tab"
		return
	fi
	items+=("$item")
	if ! d=$(wait_dialog 30); then
		shot "d-no-alert"
		record "d. Alert loop: the first alert shows" no "no dialog in 30 s"
		return
	fi
	alerts=1
	offered=""
	# stopped_by is how the alerts stopped: "checkbox" once its stop
	# checkbox is ticked, or "hold" when Bench held them back by itself.
	stopped_by=""
	ticked_on=""
	not_ticked=""
	shot "d-alert-1"
	log "d: alert 1: $d"
	dialogs press OK >/dev/null
	while [ "$alerts" -lt 6 ]; do
		if ! d=$(wait_dialog 10); then
			stopped_by=hold
			break
		fi
		alerts=$((alerts + 1))
		shot "d-alert-$alerts"
		log "d: alert $alerts: $d"
		boxes=$(json_get "$d" 'len(d[0]["checkboxes"])')
		if [ "${boxes:-0}" -gt 0 ]; then
			offered=${offered:-$alerts}
			ticked=$(dialogs tick)
			shot "d-alert-$alerts-ticked"
			dialogs press OK >/dev/null
			if [ "$ticked" = ticked ]; then
				ticked_on="alert $alerts ($(json_get "$d" 'd[0]["checkboxes"][0]["title"]'))"
				stopped_by=checkbox
				break
			fi
			not_ticked="${not_ticked:+$not_ticked, }alert $alerts: ${ticked:-no answer}"
			continue
		fi
		dialogs press OK >/dev/null
	done
	if [ "$offered" = 2 ]; then
		record "d. Alert loop: the second alert offers to stop the page's dialogs" yes "it has the checkbox"
	else
		record "d. Alert loop: the second alert offers to stop the page's dialogs" no "the first alert with a checkbox was ${offered:-none} of $alerts"
	fi
	if [ -n "$ticked_on" ]; then
		record "d. Alert loop: the stop checkbox can be ticked" yes "ticked on $ticked_on"
	else
		record "d. Alert loop: the stop checkbox can be ticked" no "${not_ticked:-no alert offered it}"
	fi
	if [ -z "$stopped_by" ]; then
		record "d. Alert loop: no alerts for 10 s once stopped" no "the alerts didn't stop after $alerts of them"
		record "d. Alert loop: Bench holds back a burst of alerts" no "the alerts didn't stop after $alerts of them"
		return
	fi
	# Bench's hold stands in for a stop that was never made, and doesn't
	# show whether one lasts while the page reloads: check the hold apart.
	if [ "$stopped_by" = hold ]; then
		record "d. Alert loop: no alerts for 10 s once stopped" no "the stop was never applied: the alerts stopped only because Bench held them back, after $alerts alerts"
	fi
	log "d: the alerts stopped ($stopped_by) after $alerts of them; watching for 10 s"
	# The page reports how long the last alert shown was open as soon as
	# it closes: leave that out.
	sleep 1
	w0=$(now)
	end=$((SECONDS + 10))
	seen=0
	while [ "$SECONDS" -lt "$end" ]; do
		d=$(dialogs list)
		case "$d" in "["*"{"*)
			seen=$((seen + 1))
			shot "d-came-back-$seen"
			dialogs press OK >/dev/null
			;;
		esac
		sleep 0.5
	done
	shot "d-after"
	reloads=$(count "$w0" 'e["path"] == "/d/"')
	answered=$(beacon_values "$w0" d-answered ms | wc -l | tr -d ' ')
	slowest=$(beacon_values "$w0" d-answered ms | sort -n | tail -1)
	if [ "$stopped_by" = checkbox ] && [ "$seen" -eq 0 ]; then
		record "d. Alert loop: no alerts for 10 s once stopped" yes "stopped by its checkbox on $ticked_on"
	elif [ "$stopped_by" = checkbox ]; then
		record "d. Alert loop: no alerts for 10 s once stopped" no "$seen alerts came back after its checkbox on $ticked_on"
	elif [ "$seen" -eq 0 ]; then
		record "d. Alert loop: Bench holds back a burst of alerts" yes "no alerts for 10 s after $alerts of them"
	else
		record "d. Alert loop: Bench holds back a burst of alerts" no "$seen alerts came back in the 10 s after the first $alerts"
	fi
	# Without this, no alerts could just mean that the page stopped asking.
	if [ "$reloads" -ge 5 ] && [ "$answered" -ge 5 ] && [ "${slowest:-0}" -lt 1000 ]; then
		record "d. Alert loop: the page keeps asking, and Bench answers at once" yes "in those 10 s, the page reloaded $reloads times, and $answered of its alert() calls returned, the slowest in ${slowest:-?} ms"
	else
		record "d. Alert loop: the page keeps asking, and Bench answers at once" no "in those 10 s, the page reloaded $reloads times, and $answered of its alert() calls returned, the slowest in ${slowest:-?} ms"
	fi
	page=$(page_of "$item")
	log "d: the item's page: $page"
}
check_alert_loop

finish
