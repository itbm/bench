#!/bin/bash
# Makes the README's screenshots and animated tour, in docs/images/, from a
# recording of the macOS demo (.github/workflows/macos-demo.yml).
#
# Usage: readme-media.sh <folder>
#
# The folder is the demo's unzipped macos-demo artifact. The script needs
# ffmpeg. It picks the tour's clips by the captions that
# scripts/macos-demo/run.sh writes, so it follows changes to the tour's
# timing, but not to its steps' captions or screenshot names.

set -euo pipefail

DEMO="${1:?Usage: readme-media.sh <folder>}"
OUT="$(cd "$(dirname "$0")/.." && pwd)/docs/images"
mkdir -p "$OUT"

# png <screenshot> <name> [filter]: one of the tour's screenshots, through
# an optional ffmpeg filter, reduced to 256 colours, which makes it a third
# of the size with no visible change.
png() {
	local src
	src="$(find "$DEMO/shots" -name "*-$1.png" | sort | head -1)"
	[ -n "$src" ] || { echo "There's no screenshot called $1." >&2; exit 1; }
	ffmpeg -v error -y -i "$src" \
		-vf "${3:+$3,}split[a][b];[a]palettegen=stats_mode=full[p];[b][p]paletteuse=dither=none" \
		"$OUT/$2.png"
	echo "$OUT/$2.png"
}
png bench-filled code-tab
png tab-notes notes-tab
# The palette, and the title bar it opens below, at 16:9 like the others.
png palette jump-palette "crop=960:540:480:0"
png compact-code compact
# For GitHub's social preview (Settings > General > Social preview), which
# link cards show: 1,280 by 640 pixels, from the top of the Code tab.
png bench-filled social-preview "crop=1920:960:0:0,scale=1280:640:flags=lanczos"

# at <caption>: how many seconds into the recording the step with that
# caption began.
started="$(cat "$DEMO/recording-started")"
at() {
	awk -v s="$started" -v c="$1" '
		{ text = substr($0, length($1) + 2) }
		index(text, c) == 1 { print $1 - s; found = 1; exit }
		END { if (!found) exit 1 }' "$DEMO/captions.txt" ||
		{ echo "There's no step called \"$1\" in captions.txt." >&2; exit 1; }
}

# The clips: a step's caption, when the clip starts in seconds from when
# that step began (negative is before), and how long it is. Sub-tab drags
# show only their end: during a drag, Bench's window covers the docked
# windows, to show where the drop lands, so the panes look empty.
clips=(
	"Bench is open and docks the Code tab|-3.5|5"
	"Bench fills the screen|-2.5|3.5"
	"Control-Option-2: the Notes tab|-0.5|3"
	"Control-Option-3: the Browse tab|-0.5|3"
	"Control-Option-1: back to Code|-0.5|2.5"
	"Control-Option-Shift-Down splits the pane|-0.5|3.5"
	"Dragging iTerm2's sub-tab onto the Notes tab|-6.5|4.5"
	"iTerm2 is in the Notes tab now|-1|2.5"
	"The jump palette finds Zed|-5.5|4.5"
	"Compact: the Code tab stacks its panes|-3.5|3"
	"Dragging VS Code's sub-tab onto the right edge|-7|5.5"
)
inputs=()
filter=""
n=0
for clip in "${clips[@]}"; do
	IFS='|' read -r caption offset length <<<"$clip"
	t="$(at "$caption")"
	start="$(awk -v t="$t" -v o="$offset" 'BEGIN { s = t + o; print (s < 0 ? 0 : s) }')"
	inputs+=(-ss "$start" -t "$length" -i "$DEMO/tour.mp4")
	filter="${filter}[$n:v]setpts=PTS-STARTPTS[c$n];"
	n=$((n + 1))
done
for ((i = 0; i < n; i++)); do filter="${filter}[c$i]"; done

# Joined, played at 1.5 times the speed, at 10 frames a second and 1,280
# pixels wide, with one palette for the whole GIF.
filter="${filter}concat=n=$n:v=1:a=0,setpts=PTS/1.5,fps=10,scale=1280:-2:flags=lanczos,split[a][b];"
filter="${filter}[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle"
ffmpeg -v error -y "${inputs[@]}" -filter_complex "$filter" -loop 0 "$OUT/tour.gif"
echo "$OUT/tour.gif"
