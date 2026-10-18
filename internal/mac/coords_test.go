package mac

import (
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestGlobalFromAppKit(t *testing.T) {
	tests := []struct {
		name          string
		in            layout.Rect
		primaryHeight float64
		want          layout.Rect
	}{
		{"whole primary display", layout.Rect{X: 0, Y: 0, W: 1512, H: 982}, 982, layout.Rect{X: 0, Y: 0, W: 1512, H: 982}},
		{"visible area above a 70-point Dock", layout.Rect{X: 0, Y: 70, W: 1512, H: 887}, 982, layout.Rect{X: 0, Y: 25, W: 1512, H: 887}},
		{"window near the top", layout.Rect{X: 100, Y: 682, W: 800, H: 275}, 982, layout.Rect{X: 100, Y: 25, W: 800, H: 275}},
		{"display above the primary", layout.Rect{X: 0, Y: 982, W: 2560, H: 1440}, 982, layout.Rect{X: 0, Y: -1440, W: 2560, H: 1440}},
		{"display to the right, bottoms aligned", layout.Rect{X: 1512, Y: 0, W: 2560, H: 1440}, 982, layout.Rect{X: 1512, Y: -458, W: 2560, H: 1440}},
		{"display below the primary", layout.Rect{X: -200, Y: -1080, W: 1920, H: 1080}, 982, layout.Rect{X: -200, Y: 982, W: 1920, H: 1080}},
		{"fractional points", layout.Rect{X: 0.5, Y: 10.25, W: 100.5, H: 50.5}, 1000, layout.Rect{X: 0.5, Y: 939.25, W: 100.5, H: 50.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobalFromAppKit(tt.in, tt.primaryHeight)
			if got != tt.want {
				t.Errorf("GlobalFromAppKit(%v, %v) = %v, want %v", tt.in, tt.primaryHeight, got, tt.want)
			}
			if back := GlobalFromAppKit(got, tt.primaryHeight); back != tt.in {
				t.Errorf("converting back gives %v, want %v", back, tt.in)
			}
		})
	}
}

func TestScreensFromRaw(t *testing.T) {
	raw := []rawScreen{
		{
			DisplayID: 1,
			Name:      "Built-in Retina Display",
			Frame:     layout.Rect{X: 0, Y: 0, W: 1512, H: 982},
			Visible:   layout.Rect{X: 0, Y: 0, W: 1512, H: 957},
		},
		{
			DisplayID: 4280832,
			Name:      "LG UltraFine",
			Frame:     layout.Rect{X: -524, Y: 982, W: 2560, H: 1440},
			Visible:   layout.Rect{X: -524, Y: 982, W: 2560, H: 1415},
		},
	}
	want := []platform.Screen{
		{
			ID:      "1",
			Name:    "Built-in Retina Display",
			Frame:   layout.Rect{X: 0, Y: 0, W: 1512, H: 982},
			Visible: layout.Rect{X: 0, Y: 25, W: 1512, H: 957},
			Primary: true,
		},
		{
			ID:      "4280832",
			Name:    "LG UltraFine",
			Frame:   layout.Rect{X: -524, Y: -1440, W: 2560, H: 1440},
			Visible: layout.Rect{X: -524, Y: -1415, W: 2560, H: 1415},
		},
	}
	got := screensFromRaw(raw)
	if len(got) != len(want) {
		t.Fatalf("screensFromRaw returned %d screens, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("screen %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := screensFromRaw(nil); got == nil || len(got) != 0 {
		t.Errorf("screensFromRaw(nil) = %#v, want an empty, non-nil slice", got)
	}
}

func TestScreenOfRect(t *testing.T) {
	laptop := platform.Screen{ID: "1", Frame: layout.Rect{X: 0, Y: 0, W: 1512, H: 982}, Primary: true}
	monitor := platform.Screen{ID: "2", Frame: layout.Rect{X: 1512, Y: -458, W: 2560, H: 1440}}
	screens := []platform.Screen{laptop, monitor}

	tests := []struct {
		name    string
		screens []platform.Screen
		r       layout.Rect
		want    string
	}{
		{"on the laptop", screens, layout.Rect{X: 100, Y: 100, W: 800, H: 600}, "1"},
		{"on the monitor", screens, layout.Rect{X: 2000, Y: 0, W: 800, H: 600}, "2"},
		{"mostly on the monitor", screens, layout.Rect{X: 1400, Y: 100, W: 800, H: 600}, "2"},
		{"mostly on the laptop", screens, layout.Rect{X: 900, Y: 100, W: 800, H: 600}, "1"},
		{"no window", screens, layout.Rect{}, "1"},
		{"on no screen", screens, layout.Rect{X: -5000, Y: -5000, W: 100, H: 100}, "1"},
		{"no primary listed", []platform.Screen{monitor}, layout.Rect{}, "2"},
		{"no screens", nil, layout.Rect{X: 1, Y: 1, W: 1, H: 1}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := screenOfRect(tt.screens, tt.r); got.ID != tt.want {
				t.Errorf("screenOfRect(%v) = screen %q, want %q", tt.r, got.ID, tt.want)
			}
		})
	}
}

func TestPrimaryHeight(t *testing.T) {
	screens := []platform.Screen{
		{ID: "2", Frame: layout.Rect{X: 1512, Y: -458, W: 2560, H: 1440}},
		{ID: "1", Frame: layout.Rect{W: 1512, H: 982}, Primary: true},
	}
	if got := primaryHeight(screens); got != 982 {
		t.Errorf("primaryHeight = %v, want 982", got)
	}
	if got := primaryHeight(nil); got != 0 {
		t.Errorf("primaryHeight(nil) = %v, want 0", got)
	}
}
