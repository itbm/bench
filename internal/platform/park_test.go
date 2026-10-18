package platform

import (
	"testing"

	"github.com/itbm/bench/internal/layout"
)

func screen(id string, x, y, w, h float64) Screen {
	return Screen{ID: id, Name: id, Frame: layout.Rect{X: x, Y: y, W: w, H: h}, Visible: layout.Rect{X: x, Y: y + 25, W: w, H: h - 25}}
}

func TestParkFrame(t *testing.T) {
	size := layout.Rect{W: 800, H: 600}
	main := screen("main", 0, 0, 2560, 1440)
	bottomRight := layout.Rect{X: 2559, Y: 1439, W: 800, H: 600}
	bottomLeft := layout.Rect{X: -799, Y: 1439, W: 800, H: 600}

	tests := []struct {
		name    string
		screens []Screen
		target  Screen
		size    layout.Rect
		want    layout.Rect
	}{
		{
			name:    "one screen",
			screens: []Screen{main},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "no screens listed",
			screens: nil,
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "size's position is ignored",
			screens: []Screen{main},
			target:  main,
			size:    layout.Rect{X: 300, Y: 200, W: 800, H: 600},
			want:    bottomRight,
		},
		{
			name:    "screen to the right, same height",
			screens: []Screen{main, screen("right", 2560, 0, 1512, 1440)},
			target:  main,
			size:    size,
			want:    bottomLeft,
		},
		{
			name:    "screen to the right, shorter, so it doesn't reach the corner",
			screens: []Screen{main, screen("right", 2560, 0, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen to the left, same height",
			screens: []Screen{screen("left", -1512, 0, 1512, 1440), main},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "parking on the right-hand screen of a pair",
			screens: []Screen{main, screen("right", 2560, 0, 1512, 1440)},
			target:  screen("right", 2560, 0, 1512, 1440),
			size:    size,
			want:    layout.Rect{X: 4071, Y: 1439, W: 800, H: 600},
		},
		{
			name:    "screen below, full width",
			screens: []Screen{main, screen("below", 0, 1440, 2560, 1440)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen below, centred",
			screens: []Screen{main, screen("below", 524, 1440, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen below, under the right-hand corner",
			screens: []Screen{main, screen("below", 1500, 1440, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomLeft,
		},
		{
			name:    "screen below, under the left-hand corner",
			screens: []Screen{main, screen("below", -500, 1440, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen at the bottom-right corner",
			screens: []Screen{main, screen("corner", 2560, 1440, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomLeft,
		},
		{
			name:    "screen at the bottom-left corner",
			screens: []Screen{main, screen("corner", -1512, 1440, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen at the top-right corner",
			screens: []Screen{main, screen("corner", 2560, -982, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name:    "screen at the top-left corner",
			screens: []Screen{main, screen("corner", -1512, -982, 1512, 982)},
			target:  main,
			size:    size,
			want:    bottomRight,
		},
		{
			name: "screens at both bottom corners",
			screens: []Screen{
				screen("left", -1512, 1440, 1512, 982),
				main,
				screen("right", 2560, 1440, 1512, 982),
			},
			target: main,
			size:   size,
			want:   bottomRight,
		},
		{
			name:    "parking on a screen above another, both corners over it",
			screens: []Screen{main, screen("above", 400, -1117, 1728, 1117)},
			target:  screen("above", 400, -1117, 1728, 1117),
			size:    size,
			want:    layout.Rect{X: 2127, Y: -1, W: 800, H: 600},
		},
		{
			name:    "parking on a screen above another, only the right-hand corner over it",
			screens: []Screen{main, screen("above", -1000, -1117, 1728, 1117)},
			target:  screen("above", -1000, -1117, 1728, 1117),
			size:    size,
			want:    layout.Rect{X: -1799, Y: -1, W: 800, H: 600},
		},
		{
			name:    "parking on a screen above another, beside it",
			screens: []Screen{main, screen("above", 2560, -1117, 1728, 1117)},
			target:  screen("above", 2560, -1117, 1728, 1117),
			size:    size,
			want:    layout.Rect{X: 4287, Y: -1, W: 800, H: 600},
		},
		{
			name: "screens without IDs are told apart by frame",
			screens: []Screen{
				{Frame: layout.Rect{W: 2560, H: 1440}},
				{Frame: layout.Rect{X: 2560, W: 1512, H: 1440}},
			},
			target: Screen{Frame: layout.Rect{W: 2560, H: 1440}},
			size:   size,
			want:   bottomLeft,
		},
		{
			name:    "a target without an ID is found among screens with IDs",
			screens: []Screen{main, screen("right", 2560, 0, 1512, 1440)},
			target:  Screen{Frame: main.Frame, Visible: main.Visible},
			size:    size,
			want:    bottomLeft,
		},
		{
			// The listed frame overlaps both corners, so main must be
			// recognised by its ID to leave the bottom-left corner free.
			name:    "a target whose frame is out of date is found by ID",
			screens: []Screen{main, screen("right", 2560, 0, 1512, 1440)},
			target:  screen("main", 0, 0, 2560, 1200),
			size:    size,
			want:    layout.Rect{X: -799, Y: 1199, W: 800, H: 600},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParkFrame(tt.screens, tt.target, tt.size)
			if got != tt.want {
				t.Errorf("ParkFrame() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParkFrameLeavesALine(t *testing.T) {
	main := screen("main", 0, 0, 1512, 982)
	for _, others := range [][]Screen{nil, {screen("right", 1512, 0, 1512, 982)}} {
		got := ParkFrame(append([]Screen{main}, others...), main, layout.Rect{W: 640, H: 480})
		on := Intersect(got, main.Frame)
		if on.W != ParkLine || on.H != ParkLine {
			t.Errorf("parked at %+v, leaving %+v on screen, want a %d-point square", got, on, ParkLine)
		}
	}
}

func TestOverlapsAndIntersect(t *testing.T) {
	tests := []struct {
		name string
		a, b layout.Rect
		want layout.Rect
	}{
		{"apart", layout.Rect{W: 10, H: 10}, layout.Rect{X: 20, W: 10, H: 10}, layout.Rect{}},
		{"touching edges", layout.Rect{W: 10, H: 10}, layout.Rect{X: 10, W: 10, H: 10}, layout.Rect{}},
		{"overlapping", layout.Rect{W: 10, H: 10}, layout.Rect{X: 5, Y: 5, W: 10, H: 10}, layout.Rect{X: 5, Y: 5, W: 5, H: 5}},
		{"inside", layout.Rect{W: 10, H: 10}, layout.Rect{X: 2, Y: 3, W: 4, H: 5}, layout.Rect{X: 2, Y: 3, W: 4, H: 5}},
		{"empty", layout.Rect{W: 10, H: 10}, layout.Rect{X: 2, Y: 2}, layout.Rect{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Intersect(tt.a, tt.b); got != tt.want {
				t.Errorf("Intersect() = %+v, want %+v", got, tt.want)
			}
			wantOverlap := tt.want != layout.Rect{}
			if got := Overlaps(tt.a, tt.b); got != wantOverlap {
				t.Errorf("Overlaps() = %v, want %v", got, wantOverlap)
			}
		})
	}
}
