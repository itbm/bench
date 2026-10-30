package layout

import "testing"

// TestDropAt checks the drop rule that drag.js follows too: the header is
// a strip that never splits, and the body's outer quarters split.
func TestDropAt(t *testing.T) {
	// A pane with a 40-point header: its body is 2 to 802 across and 43 to
	// 642 down (inside the 1-point border, below the header and its
	// divider).
	frame := Rect{X: 1, Y: 1, W: 802, H: 642}
	body := Body(frame, 40)
	if body != (Rect{X: 2, Y: 43, W: 800, H: 599}) {
		t.Fatalf("body = %+v", body)
	}
	tests := []struct {
		name   string
		x, y   float64
		header float64
		splits bool
		want   Drop
	}{
		{"the header's top edge", 400, 2, 40, true, Drop{Strip: true, Zone: Center}},
		{"the header's left end", 3, 20, 40, true, Drop{Strip: true, Zone: Center}},
		{"the header's buttons", 790, 30, 40, true, Drop{Strip: true, Zone: Center}},
		{"the body's top edge, just under the header", 400, 50, 40, true, Drop{Zone: Top}},
		{"the body's middle", 400, 340, 40, true, Drop{Zone: Center}},
		{"the body's left quarter", 150, 340, 40, true, Drop{Zone: Left}},
		{"the body's right edge", 790, 340, 40, true, Drop{Zone: Right}},
		{"the body's bottom quarter", 400, 600, 40, true, Drop{Zone: Bottom}},
		{"nearest side wins in a corner", 10, 60, 40, true, Drop{Zone: Left}},
		{"a tie goes to the left, as in drag.js", 2 + 100, 43 + 100*599.0/800, 40, true, Drop{Zone: Left}},
		{"with splits off, the body joins", 790, 340, 40, false, Drop{Zone: Center}},
		{"with splits off, the header is still a strip", 400, 2, 40, false, Drop{Strip: true, Zone: Center}},
		{"a pane with no header has no strip", 400, 2, 0, true, Drop{Zone: Top}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DropAt(frame, tt.header, tt.x, tt.y, tt.splits); got != tt.want {
				t.Errorf("DropAt(%v, %v) = %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestDropAtEmptyBody checks that a pane too small for its chrome, whose
// body has no room, takes a drop without splitting.
func TestDropAtEmptyBody(t *testing.T) {
	frame := Rect{X: 0, Y: 0, W: 100, H: 30}
	if got := DropAt(frame, 40, 50, 29, true); got != (Drop{Strip: true, Zone: Center}) {
		t.Errorf("in the header: %+v", got)
	}
	if got := DropAt(Rect{X: 0, Y: 0, W: 1, H: 1}, 0, 0, 0, true); got != (Drop{Zone: Center}) {
		t.Errorf("no body: %+v", got)
	}
}
