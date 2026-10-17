package mac

import (
	"strconv"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// GlobalFromAppKit converts a rectangle from AppKit screen coordinates
// (origin at the bottom-left of the primary display, y upwards) to global
// coordinates (origin at the top-left of the primary display, y downwards),
// given the primary display's height in points:
//
//	y_global = primaryHeight - (y_appkit + height)
//
// The conversion is its own inverse, so it also turns global rectangles into
// AppKit ones.
func GlobalFromAppKit(r layout.Rect, primaryHeight float64) layout.Rect {
	return layout.Rect{X: r.X, Y: primaryHeight - (r.Y + r.H), W: r.W, H: r.H}
}

// rawScreen is a display as AppKit describes it, before conversion.
type rawScreen struct {
	DisplayID uint32
	Name      string
	Frame     layout.Rect // AppKit coordinates.
	Visible   layout.Rect // AppKit coordinates, without the menu bar and the Dock.
}

// screensFromRaw converts AppKit's displays to platform screens in global
// coordinates. The first display is the primary one, as in +[NSScreen
// screens]; its AppKit frame starts at (0, 0), so its height is the
// primaryHeight for the conversion.
func screensFromRaw(raw []rawScreen) []platform.Screen {
	screens := make([]platform.Screen, 0, len(raw))
	if len(raw) == 0 {
		return screens
	}
	primaryHeight := raw[0].Frame.H
	for i, r := range raw {
		screens = append(screens, platform.Screen{
			ID:      strconv.FormatUint(uint64(r.DisplayID), 10),
			Name:    r.Name,
			Frame:   GlobalFromAppKit(r.Frame, primaryHeight),
			Visible: GlobalFromAppKit(r.Visible, primaryHeight),
			Primary: i == 0,
		})
	}
	return screens
}

// primaryHeight returns the primary screen's height, or 0 if there's none.
func primaryHeight(screens []platform.Screen) float64 {
	for _, s := range screens {
		if s.Primary {
			return s.Frame.H
		}
	}
	return 0
}

// screenOfRect returns the screen holding most of r, or the primary screen
// if r is empty or on no screen. With no screens at all it returns a zero
// Screen.
func screenOfRect(screens []platform.Screen, r layout.Rect) platform.Screen {
	var best platform.Screen
	bestArea := 0.0
	for _, s := range screens {
		i := platform.Intersect(s.Frame, r)
		if area := i.W * i.H; area > bestArea {
			best, bestArea = s, area
		}
	}
	if bestArea > 0 {
		return best
	}
	for _, s := range screens {
		if s.Primary {
			return s
		}
	}
	if len(screens) > 0 {
		return screens[0]
	}
	return platform.Screen{}
}
