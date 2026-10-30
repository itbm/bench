package mac

import (
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestGuideMarks(t *testing.T) {
	g := platform.DropGuide{
		Pane:   layout.Rect{X: 100, Y: 200, W: 800, H: 600},
		Body:   layout.Rect{X: 101, Y: 241, W: 798, H: 558},
		Target: layout.Rect{X: 101, Y: 241, W: 399, H: 558},
		Zone:   "left", Splits: true, Label: "New pane on the left",
	}
	marks := guideMarks(g)
	if len(marks) != 7 {
		t.Fatalf("%d marks, want the area, five targets, and the label: %+v", len(marks), marks)
	}
	// The panel's coordinates have their origin at its bottom left: the
	// body's bottom is 1 point above the pane's.
	if a := marks[0]; a.kind != guideArea || a.side != guideLeft || a.rect != (layout.Rect{X: 1, Y: 1, W: 399, H: 558}) {
		t.Errorf("area = %+v, want the left half of the body", a)
	}
	on := 0
	for _, m := range marks[1:6] {
		if m.kind == guideMarkOn {
			on++
			if m.side != guideLeft {
				t.Errorf("the highlighted mark is side %d, want the left one", m.side)
			}
		}
		if m.rect.W != guideMarkSize || m.rect.H != guideMarkSize {
			t.Errorf("mark %+v isn't %d points square", m, guideMarkSize)
		}
		if m.rect.X < 0 || m.rect.Y < 0 || m.rect.X+m.rect.W > g.Pane.W || m.rect.Y+m.rect.H > g.Pane.H {
			t.Errorf("mark %+v is outside the panel", m)
		}
	}
	if on != 1 {
		t.Errorf("%d marks are highlighted, want one", on)
	}
	middle, left, top := marks[1], marks[2], marks[4]
	if middle.side != guideMiddle || left.side != guideLeft || top.side != guideTop {
		t.Fatalf("marks in the wrong order: %+v", marks)
	}
	if left.rect.X != 1+guideEdgeInset || left.rect.Y != middle.rect.Y {
		t.Errorf("left mark %+v, want inset from the body's left edge, level with the middle one %+v", left.rect, middle.rect)
	}
	if top.rect.Y <= middle.rect.Y || top.rect.Y+top.rect.H != 600-41-guideEdgeInset {
		t.Errorf("top mark %+v, want inset below the header (y up)", top.rect)
	}
	label := marks[6]
	if label.kind != guideLabel || label.rect.W != guideLabelWidth || label.rect.Y+label.rect.H != middle.rect.Y-guideLabelGap {
		t.Errorf("label %+v, want %d points wide, below the middle mark", label, guideLabelWidth)
	}

	// A pane that can't be split has the middle mark alone, and a narrow
	// one a narrower label.
	g.Splits, g.Zone = false, "center"
	g.Body.W, g.Pane.W = 100, 102
	marks = guideMarks(g)
	if len(marks) != 3 || marks[1].kind != guideMarkOn || marks[1].side != guideMiddle {
		t.Errorf("marks without splits = %+v, want the area, the middle mark highlighted, and the label", marks)
	}
	if w := marks[2].rect.W; w != 100-2*guideEdgeInset {
		t.Errorf("label width %v in a narrow pane", w)
	}
}
