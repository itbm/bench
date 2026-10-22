#!/bin/bash
# Records a tour of Bench on a GitHub macOS runner, with real apps, for
# .github/workflows/macos-demo.yml.
#
# Usage: run.sh <bench binary> <output folder>
#
# The runner's image grants Accessibility, Screen Recording, and synthetic
# input to /bin/bash and the runner's own processes, and a binary started
# from a job step inherits them. So Bench runs as the real app, ffmpeg
# records the screen, and cliclick presses Bench's real global shortcuts and
# drags with the real mouse.
#
# The output folder gets tour.mp4 (the recording), shots/ (a screenshot after
# each step), steps.log, bench.log, and diagnostics.txt.

set -u

BENCH="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
OUT="$(mkdir -p "$2" && cd "$2" && pwd)"
HERE="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$OUT/shots"

export BENCH_HOME="$OUT/home"
mkdir -p "$BENCH_HOME"
cp "$HERE/config.toml" "$BENCH_HOME/config.toml"
touch "$BENCH_HOME/first-run-done"

shots=0
log() { echo "[$(date +%T)] $*" | tee -a "$OUT/steps.log"; }
shot() {
	shots=$((shots + 1))
	screencapture -x "$OUT/shots/$(printf %02d "$shots")-$1.png" || log "screencapture failed"
}

# Input, through cliclick. Moves are eased so they're visible in the video.
move() { cliclick -e 400 "m:$1,$2"; }
click() { move "$1" "$2"; sleep 0.2; cliclick "c:$1,$2"; }
# key <key code> [modifier...]: press a key the way the keyboard does, through
# System Events, so Bench's global hot keys see it. (cliclick can only press
# the keypad's digits, which have other key codes.) Key codes: 1 is 18, 2 is
# 19, 3 is 20, Tab 48, Space 49, Return 36, Q 12, down arrow 125.
key() {
	local code=$1 mods=""
	shift
	for m in "$@"; do mods="$mods${mods:+, }$m down"; done
	local script="tell application \"System Events\" to key code $code"
	[ -n "$mods" ] && script="$script using {$mods}"
	osascript -e "$script" || log "couldn't press key code $code"
}
# drag x1 y1 x2 y2: press, move in small steps (a press on a sub-tab
# becomes a drag once the pointer moves), release.
drag() {
	local x1=$1 y1=$2 x2=$3 y2=$4 i args=()
	move "$x1" "$y1"
	sleep 0.3
	args+=("dd:$x1,$y1" "w:150" "dm:$((x1 + 6)),$((y1 + 4))" "w:100")
	for i in $(seq 1 12); do
		args+=("dm:$((x1 + (x2 - x1) * i / 24)),$((y1 + (y2 - y1) * i / 24))" "w:35")
	done
	cliclick "${args[@]}"
	shot "${5:-drag}-halfway"
	args=()
	for i in $(seq 13 24); do
		args+=("dm:$((x1 + (x2 - x1) * i / 24)),$((y1 + (y2 - y1) * i / 24))" "w:35")
	done
	args+=("w:500" "du:$x2,$y2")
	cliclick "${args[@]}"
}

# windows <process name>: the names of the process's windows, front first,
# from System Events.
windows() {
	osascript -e "tell application \"System Events\" to get name of windows of process \"$1\"" 2>/dev/null
}

# frame <process name>: "x y w h" of the process's front window, in global
# top-left coordinates, from System Events (the Accessibility API).
frame() {
	osascript -e "tell application \"System Events\" to tell process \"$1\" to get {position, size} of window 1" 2>/dev/null | tr -d ' ' | tr ',' ' '
}

# visible: "x y w h" of the main screen without the menu bar and the Dock.
visible() {
	osascript -l JavaScript -e '
		ObjC.import("AppKit");
		const s = $.NSScreen.mainScreen, f = s.frame, v = s.visibleFrame;
		[v.origin.x, f.size.height - (v.origin.y + v.size.height), v.size.width, v.size.height].map(Math.round).join(" ");'
}

# setbench x y w h: move and size Bench's window, as the user could.
setbench() {
	osascript -e "tell application \"System Events\" to tell process \"bench\"
		set position of window 1 to {$1, $2}
		set size of window 1 to {$3, $4}
	end tell" >/dev/null 2>&1 || log "couldn't move Bench's window"
}

# tab_of <bundle id>: the name of the tab holding an item of that app, from
# Bench's state.json (Bench writes it half a second after a change).
tab_of() {
	/usr/bin/python3 - "$BENCH_HOME/state.json" "$1" <<'PY' 2>/dev/null
import json, sys
state, bundle = json.load(open(sys.argv[1])), sys.argv[2]
ids = {k for k, v in state.get("items", {}).items() if v.get("bundleId") == bundle}
def items(node):
    if not node:
        return []
    if node.get("type") == "pane":
        return node.get("items") or []
    return [i for c in node.get("children") or [] for i in items(c)]
for tab in state.get("tabs", []):
    if ids & set(items(tab.get("root"))):
        print(tab["name"])
        break
PY
}

# pane_of <bundle id>: "x y w h index" for the displayed pane holding an item
# of that app: its rectangle in global coordinates, and the item's place among
# its sub-tabs. It reads the view Bench writes when BENCH_DEBUG is set, and
# Bench's window origin (its content fills the window).
pane_of() {
	local ox oy _w _h
	read -r ox oy _w _h <<<"$(frame bench)"
	/usr/bin/python3 - "$BENCH_HOME/view.json" "$1" "$ox" "$oy" <<'PY' 2>/dev/null
import json, sys
view, bundle = json.load(open(sys.argv[1])), sys.argv[2]
ox, oy = float(sys.argv[3]), float(sys.argv[4])
items = view.get("items", {})
for pane in view["display"]["panes"]:
    for i, item in enumerate(pane.get("items") or []):
        if items.get(item, {}).get("bundleId") == bundle:
            r = pane["rect"]
            print(int(ox + r["x"]), int(oy + r["y"]), int(r["w"]), int(r["h"]), i)
            sys.exit(0)
PY
}

# first_subtab <bundle id>: "x y" of the first sub-tab in the pane holding an
# item of that app: 27 pt in from the pane's left edge (its border, the
# header's padding, and into the monogram), 21 pt down (the middle of the
# 40 pt header, below the border). It fails, printing nothing, if pane_of
# can't find the pane: empty values count as 0 in arithmetic, which would
# put the point on the Apple menu.
first_subtab() {
	local px py pw ph pi
	read -r px py pw ph pi <<<"$(pane_of "$1")"
	[ -n "$px" ] && [ -n "$py" ] || return 1
	echo "$((px + 27)) $((py + 21))"
}

# click_subtab <bundle id>: click the first sub-tab in the pane holding an
# item of that app, or log and skip the click if there's no such pane.
click_subtab() {
	local pos sx sy
	if ! pos="$(first_subtab "$1")"; then
		log "couldn't find the pane holding $1, so not clicking its sub-tab"
		return 1
	fi
	read -r sx sy <<<"$pos"
	click "$sx" "$sy"
}

# drag_subtab <bundle id> x y [name]: drag the first sub-tab in the pane
# holding an item of that app to (x, y), or log and skip the drag if there's
# no such pane.
drag_subtab() {
	local pos sx sy
	if ! pos="$(first_subtab "$1")"; then
		log "couldn't find the pane holding $1, so not dragging its sub-tab"
		return 1
	fi
	read -r sx sy <<<"$pos"
	log "Dragging $1's sub-tab from $sx $sy to $2 $3"
	drag "$sx" "$sy" "$2" "$3" "${4:-drag}"
}

# mode_now: the layout mode Bench shows (auto, wide, or compact), from the
# view it writes when BENCH_DEBUG is set.
mode_now() {
	/usr/bin/python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("mode", ""))' "$BENCH_HOME/view.json" 2>/dev/null
}

# current_tab: the name of the tab Bench shows, from its view.
current_tab() {
	/usr/bin/python3 -c 'import json, sys; v = json.load(open(sys.argv[1])); print(v["tabs"][v["current"]]["name"])' "$BENCH_HOME/view.json" 2>/dev/null
}

# palette_run <text>: open the jump palette with its shortcut, type, and
# choose the first match.
palette_run() {
	key 49 control option
	sleep 1.2
	cliclick -w 90 "t:$1"
	sleep 0.8
	key 36
	sleep 1.5
}

# activate: click an empty part of Bench's title bar, as a person would
# before dragging in a window that isn't active.
activate() {
	read -r ax ay aw ah <<<"$(frame bench)"
	click $((ax + aw / 2)) $((ay + 12))
	sleep 0.6
}

step() {
	log "$1"
	echo "$(date +%s) $1" >>"$OUT/captions.txt"
	sleep "${2:-2}"
	shot "$3"
}

# The runner's display starts at 1,024 by 768. Try for something roomier.
if brew install jakehilborn/jakehilborn/displayplacer >/dev/null 2>&1; then
	displayplacer list >"$OUT/displays.txt" 2>&1
	id=$(sed -n 's/^Persistent screen id: //p' "$OUT/displays.txt" | head -1)
	for res in 1920x1080 1680x1050 1600x900 1440x900 1280x800; do
		if grep -q "res:$res" "$OUT/displays.txt" && displayplacer "id:$id res:$res origin:(0,0) degree:0" >>"$OUT/displays.txt" 2>&1; then
			log "Display set to $res"
			break
		fi
	done
	sleep 3
else
	log "couldn't install displayplacer"
fi

{
	echo "== macOS"; sw_vers
	echo "== SIP"; csrutil status
	echo "== Screen"; visible; system_profiler SPDisplaysDataType | grep -E "Resolution|Display Type" || true
	echo "== Apps"; ls -d /Applications/*.app
	echo "== ffmpeg devices"; ffmpeg -hide_banner -f avfoundation -list_devices true -i "" 2>&1 | grep -E "\[[0-9]+\]" || true
} >"$OUT/diagnostics.txt" 2>&1

# Make the recording look like a Mac in use, since the README shows it: dark
# mode, the Dock hidden, no trust, update, or AI prompts in the editors, and
# quiet shells. The runner is thrown away after the job, so none of this
# needs undoing.
osascript -e 'tell application "System Events" to tell appearance preferences to set dark mode to true' || log "couldn't turn on dark mode"
{ defaults write com.apple.dock autohide -bool true && killall Dock; } || log "couldn't hide the Dock"
mkdir -p "$HOME/.config/zed"
cat >"$HOME/.config/zed/settings.json" <<'JSON'
{
  "session": { "trust_all_worktrees": true },
  "auto_update": false
}
JSON
code_user="$HOME/Library/Application Support/Code/User"
mkdir -p "$code_user"
cat >"$code_user/settings.json" <<'JSON'
{
  "chat.disableAIFeatures": true,
  "extensions.ignoreRecommendations": true,
  "security.workspace.trust.enabled": false,
  "telemetry.telemetryLevel": "off",
  "update.mode": "none",
  "workbench.secondarySideBar.defaultVisibility": "hidden",
  "workbench.startupEditor": "none",
  "workbench.tips.enabled": false
}
JSON
# iTerm2 and Bench's terminal both run a login shell, which is bash here.
touch "$HOME/.hushlogin"
cat >>"$HOME/.bash_profile" <<'SH'

# Added by Bench's macOS demo.
export BASH_SILENCE_DEPRECATION_WARNING=1
PS1='\w $ '
cd ~/bench-demo 2>/dev/null
[ "$TERM_PROGRAM" = iTerm.app ] && git log --oneline -5
SH

# Some work for the apps to show: a small Go project with a short history.
proj="$HOME/bench-demo"
commit() { git -C "$proj" add -A && git -C "$proj" -c user.name="Bench demo" -c user.email=demo@example.com commit -qm "$1"; }
mkdir -p "$proj/cmd/bench" "$proj/internal/layout"
git -C "$proj" init -q
cat >"$proj/internal/layout/layout.go" <<'GO'
// Package layout describes how a tab divides its space into panes.
package layout

// A Split divides its space between its children, in a row or a column.
type Split struct {
	Row      bool
	Sizes    []float64 // Percentages, one per child.
	Children []Node
}

// A Pane holds windows as sub-tabs, and shows one at a time.
type Pane struct {
	Items []string
	Shown int
}

// A Node is a Split or a Pane.
type Node interface{ node() }

func (Split) node() {}
func (Pane) node()  {}

// Panes lists a layout's panes in reading order.
func Panes(n Node) []Pane {
	switch n := n.(type) {
	case Pane:
		return []Pane{n}
	case Split:
		var panes []Pane
		for _, c := range n.Children {
			panes = append(panes, Panes(c)...)
		}
		return panes
	}
	return nil
}
GO
commit "Add the layout types"
cat >"$proj/cmd/bench/main.go" <<'GO'
package main

import (
	"fmt"
	"os"
)

// Bench gathers any set of apps into one window with tabs.
func main() {
	tabs := []string{"Code", "Notes", "Browse"}
	for i, name := range tabs {
		fmt.Printf("Control-Option-%d  %s\n", i+1, name)
	}
	if len(os.Args) > 1 {
		fmt.Println("Switching to", os.Args[1])
	}
}
GO
commit "Print the tabs and their shortcuts"
cat >"$proj/notes.txt" <<'TXT'
Release checklist

- Bump the version in Info.plist
- Tag the release and push the tag
- Check that the download opens on a clean Mac
- Write the release notes

Bench docks this TextEdit window in its Notes tab.
TXT
commit "Add the release checklist"
printf '# bench-demo\n\nA small project for Bench'"'"'s recorded tour.\n' >"$proj/README.md"
commit "Add a README"

log "Opening the apps"
zed_cli=/Applications/Zed.app/Contents/MacOS/cli
if [ -x "$zed_cli" ]; then
	"$zed_cli" "$proj" "$proj/cmd/bench/main.go" >/dev/null 2>&1 &
else
	open -a /Applications/Zed.app "$proj" 2>>"$OUT/steps.log" || log "couldn't open Zed"
fi
"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" --disable-workspace-trust --skip-welcome --skip-release-notes "$proj" "$proj/internal/layout/layout.go" >/dev/null 2>&1 &
open -a /Applications/iTerm.app 2>>"$OUT/steps.log" || log "couldn't open iTerm2"
open -a TextEdit "$proj/notes.txt"
open -a Safari "https://go.dev/"
sleep 25
shot "apps-open"

log "Recording"
screen_index=$(ffmpeg -hide_banner -f avfoundation -list_devices true -i "" 2>&1 | sed -n 's/.*\[\([0-9]*\)\] Capture screen 0.*/\1/p' | head -1)
ffmpeg -hide_banner -loglevel warning -f avfoundation -capture_cursor 1 -capture_mouse_clicks 1 -framerate 20 \
	-i "${screen_index:-1}:none" -vf "scale='min(1920,iw)':-2" -c:v libx264 -preset veryfast -crf 26 -pix_fmt yuv420p \
	-y "$OUT/tour.mp4" </dev/null >"$OUT/ffmpeg.log" 2>&1 &
ffmpeg_pid=$!
date +%s >"$OUT/recording-started"
sleep 3

log "Starting Bench"
BENCH_DEBUG=1 "$BENCH" >"$OUT/bench.log" 2>&1 &
bench_pid=$!
for _ in $(seq 1 60); do
	[ -n "$(frame bench)" ] && break
	sleep 0.5
done
step "Bench is open and docks the Code tab" 8 "bench-open"

read -r vx vy vw vh <<<"$(visible)"
log "Visible area: $vx $vy $vw $vh"
setbench "$vx" "$vy" "$vw" "$vh"
step "Bench fills the screen; the docked windows follow" 4 "bench-filled"

read -r bx by bw bh <<<"$(frame bench)"
log "Bench: $bx $by $bw $bh"
title_y=$((by + 26))
read -r zx zy zw zh <<<"$(frame Zed)"
read -r ix iy iw ih <<<"$(frame iTerm2)"
log "Zed: $zx $zy $zw $zh; iTerm2: $ix $iy $iw $ih"

click_subtab dev.zed.Zed
step "Clicking the first pane's sub-tab focuses that pane" 1 "pane-focused"
key 48 control option
step "Control-Option-Tab shows the next window in the pane: VS Code" 2 "next-subtab"
key 48 control option
step "And back to Zed" 2 "back-to-zed"

key 19 control option
step "Control-Option-2: the Notes tab, with TextEdit and a web page" 4 "tab-notes"
key 20 control option
step "Control-Option-3: the Browse tab, with Safari" 4 "tab-browse"
key 18 control option
step "Control-Option-1: back to Code" 4 "tab-code"

click_subtab dev.zed.Zed
sleep 1
key 125 control option shift
step "Control-Option-Shift-Down splits the pane: Zed moves into a new pane below" 4 "split-down"

drag_subtab com.googlecode.iterm2 $((bx + 211)) "$title_y" "drag-background"
sleep 2
log "Check: a drag onto the Notes tab with Bench in the background: iTerm2 is now in $(tab_of com.googlecode.iterm2)"
if [ "$(tab_of com.googlecode.iterm2)" != "Notes" ]; then
	log "Activating Bench and trying again"
	activate
	drag_subtab com.googlecode.iterm2 $((bx + 211)) "$title_y" "drag-active"
	sleep 2
	log "Check: the same drag with Bench active: iTerm2 is now in $(tab_of com.googlecode.iterm2)"
fi
step "Dragging iTerm2's sub-tab onto the Notes tab moves its window there" 3 "drag-to-tab"
key 19 control option
step "iTerm2 is in the Notes tab now" 4 "iterm-in-notes"

key 49 control option
sleep 1.5
cliclick -w 120 "t:zed"
sleep 1
shot "palette"
key 36
step "The jump palette finds Zed and switches to its tab" 4 "palette-chosen"

# Bench is in the background here (the palette focused Zed), and the
# Compact control sits in the window's top band, where macOS keeps its title
# bar: the click must still reach Bench.
click $((bx + bw - 250)) "$title_y"
sleep 1.5
log "Check: a click on Compact with Bench in the background: mode is now $(mode_now)"
[ "$(mode_now)" = compact ] || palette_run "compact"
step "Compact: the Code tab stacks its panes" 3 "compact-code"
key 19 control option
step "Compact: the Notes tab shows one pane with every window as a sub-tab" 4 "compact-notes"
key 18 control option
sleep 2
palette_run "auto"
step "Auto again: wide" 3 "auto"

click $((bx + 308)) "$title_y"
sleep 1.5
log "Check: a click on the Browse tab: the current tab is now $(current_tab)"
step "Clicking a tab in the title bar switches to it" 3 "click-browse"
click $((bx + 120)) "$title_y"
sleep 1.5
log "Check: a click on the Code tab: the current tab is now $(current_tab)"
step "And back to Code" 3 "click-code"

read -r px py pw ph pi <<<"$(pane_of dev.zed.Zed)"
log "Zed's pane: $px $py $pw $ph"
if [ -n "$px" ]; then
	drag_subtab com.microsoft.VSCode $((px + pw - pw / 10)) $((py + ph / 2)) "drag-edge"
else
	log "couldn't find Zed's pane, so not dragging VS Code's sub-tab onto it"
fi
sleep 1.5
read -r ex ey _ _ _ <<<"$(pane_of com.microsoft.VSCode)"
read -r zx zy _ _ _ <<<"$(pane_of dev.zed.Zed)"
beside=false
if [ -n "$ex" ] && [ -n "$zx" ] && [ "$ex" -gt "$zx" ] && [ "$ey" -eq "$zy" ]; then beside=true; fi
log "Check: a drag onto the right edge of Zed's pane: VS Code's pane is beside Zed's: $beside"
step "Dragging VS Code's sub-tab onto the right edge of Zed's pane shows them side by side" 4 "drag-split"

setbench $((vx + 80)) $((vy + 40)) $((vw * 3 / 4)) $((vh * 4 / 5))
step "Resizing Bench re-docks every window" 4 "resized"
read -r bx by bw bh <<<"$(frame bench)"
drag $((bx + bw / 2)) $((by + 26)) $((bx + bw / 2 + 160)) $((by + 26 + 60))
step "Moving Bench's window: the docked windows move with it" 4 "moved"

# Bench ▸ Acknowledgements opens its window in front of Bench's, even
# while the jump palette holds Bench's window above the docked apps, and
# Command-W closes it. The app menu is the menu bar's second item.
activate
key 49 control option
sleep 1.2
osascript -e 'tell application "System Events" to tell process "bench" to click menu item "Acknowledgements" of menu 1 of menu bar item 2 of menu bar 1' >/dev/null ||
	log "couldn't choose Bench ▸ Acknowledgements"
sleep 1.5
log "Check: Bench ▸ Acknowledgements with the palette open: Bench's windows, front first: $(windows bench)"
step "Bench ▸ Acknowledgements lists the licences, in front of Bench's window" 3 "acknowledgements"
key 13 command
sleep 1
log "Check: Command-W in Acknowledgements: Bench's windows are now: $(windows bench)"
key 53
sleep 1

read -r bx by bw bh <<<"$(frame bench)"
click $((bx + bw / 2)) $((by + 26))
key 12 command
step "Quitting Bench puts every window back" 6 "after-quit"
for app in TextEdit Safari iTerm2; do
	log "$app visible after quitting: $(osascript -e "tell application \"System Events\" to get visible of process \"$app\"" 2>&1)"
done

kill -INT "$ffmpeg_pid" 2>/dev/null
wait "$ffmpeg_pid" 2>/dev/null
kill "$bench_pid" 2>/dev/null
log "Done"
