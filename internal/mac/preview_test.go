package mac

import (
	"math"
	"testing"

	"github.com/itbm/bench/internal/layout"
)

func TestPreviewSize(t *testing.T) {
	if previewWidth != 250 || previewHeight != 186 {
		t.Errorf("preview panel is %d by %d, want 250 by 186", previewWidth, previewHeight)
	}
}

func TestPreviewFrame(t *testing.T) {
	content := layout.Rect{X: 100, Y: 50, W: 1400, H: 900}
	tests := []struct {
		name    string
		anchor  layout.Rect
		content layout.Rect
		want    layout.Rect
	}{
		{
			name:    "under the anchor",
			anchor:  layout.Rect{X: 300, Y: 40, W: 160, H: 30},
			content: content,
			want:    layout.Rect{X: 400, Y: 126, W: 250, H: 186},
		},
		{
			name:    "kept off the left edge",
			anchor:  layout.Rect{X: 2, Y: 40, W: 160, H: 30},
			content: content,
			want:    layout.Rect{X: 108, Y: 126, W: 250, H: 186},
		},
		{
			name:    "kept inside the right edge",
			anchor:  layout.Rect{X: 1300, Y: 40, W: 90, H: 30},
			content: content,
			want:    layout.Rect{X: 100 + 1400 - 250 - 12, Y: 126, W: 250, H: 186},
		},
		{
			name:    "narrow window: the left edge wins",
			anchor:  layout.Rect{X: 100, Y: 0, W: 50, H: 20},
			content: layout.Rect{X: 0, Y: 0, W: 200, H: 300},
			want:    layout.Rect{X: 8, Y: 26, W: 250, H: 186},
		},
		{
			name:    "content on a display left of the primary",
			anchor:  layout.Rect{X: 20, Y: 40, W: 100, H: 30},
			content: layout.Rect{X: -1920, Y: -200, W: 1920, H: 1080},
			want:    layout.Rect{X: -1900, Y: -124, W: 250, H: 186},
		},
		{
			name:    "above an anchor near the bottom",
			anchor:  layout.Rect{X: 300, Y: 800, W: 160, H: 30},
			content: content,
			want:    layout.Rect{X: 400, Y: 50 + 800 - 6 - 186, W: 250, H: 186},
		},
		{
			name:    "just fits below",
			anchor:  layout.Rect{X: 300, Y: 678, W: 160, H: 30},
			content: content,
			want:    layout.Rect{X: 400, Y: 50 + 678 + 30 + 6, W: 250, H: 186},
		},
		{
			name:    "short window: below, with more room there",
			anchor:  layout.Rect{X: 20, Y: 40, W: 100, H: 30},
			content: layout.Rect{X: 0, Y: 0, W: 800, H: 200},
			want:    layout.Rect{X: 20, Y: 76, W: 250, H: 186},
		},
		{
			name:    "short window: above, with more room there",
			anchor:  layout.Rect{X: 20, Y: 150, W: 100, H: 30},
			content: layout.Rect{X: 0, Y: 0, W: 800, H: 200},
			want:    layout.Rect{X: 20, Y: 150 - 6 - 186, W: 250, H: 186},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := previewFrame(tt.anchor, tt.content); got != tt.want {
				t.Errorf("previewFrame(%v, %v) = %v, want %v", tt.anchor, tt.content, got, tt.want)
			}
		})
	}
}

func TestCaptureSize(t *testing.T) {
	tests := []struct {
		name         string
		w, h         float64
		wantW, wantH int
	}{
		{"same shape as the picture", 1600, 1000, 464, 290},
		{"wide window", 2000, 500, 464, 116},
		{"tall window", 500, 1000, 145, 290},
		{"small window isn't enlarged past 2x", 100, 50, 200, 100},
		{"tiny window", 0.5, 0.5, 2, 2},
		{"no size", 0, 0, 464, 290},
		{"negative size", -10, 100, 464, 290},
		{"not a number", math.NaN(), 100, 464, 290},
		{"infinite", math.Inf(1), 100, 464, 290},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := captureSize(tt.w, tt.h)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("captureSize(%v, %v) = %d, %d, want %d, %d", tt.w, tt.h, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}
