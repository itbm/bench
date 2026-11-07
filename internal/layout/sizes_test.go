package layout

import (
	"math"
	"testing"
)

func TestResize(t *testing.T) {
	tests := []struct {
		name  string
		sizes []float64
		i     int
		step  float64
		want  []float64
		ok    bool
	}{
		{"grow the first of two", []float64{50, 50}, 0, 5, []float64{55, 45}, true},
		{"shrink the second of two", []float64{65, 35}, 1, -5, []float64{70, 30}, true},
		{"sizes that don't add up to 100", []float64{2, 2}, 0, 5, []float64{55, 45}, true},
		// The others give in proportion to their room above MinSize.
		{"grow the middle of three", []float64{40, 30, 30}, 1, 5, []float64{37, 35, 28}, true},
		{"one at its minimum gives nothing", []float64{40, 10, 50}, 0, 5, []float64{45, 10, 45}, true},
		// The others take in proportion to their sizes.
		{"shrink the first of three", []float64{40, 30, 30}, 0, -5, []float64{35, 32.5, 32.5}, true},
		// Less than a step, when that's all the room.
		{"grow to the limit", []float64{88, 12}, 0, 5, []float64{90, 10}, true},
		{"shrink to the limit", []float64{12, 88}, 0, -5, []float64{10, 90}, true},
		{"at the largest", []float64{90, 10}, 0, 5, nil, false},
		{"at the smallest", []float64{10, 90}, 0, -5, nil, false},
		{"smaller than the smallest already", []float64{5, 95}, 0, -5, nil, false},
		{"one child", []float64{100}, 0, 5, nil, false},
		{"no such child", []float64{50, 50}, 2, 5, nil, false},
		{"no step", []float64{50, 50}, 0, 0, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Resize(tt.sizes, tt.i, tt.step)
			if ok != tt.ok || !near(got, tt.want) {
				t.Errorf("Resize(%v, %d, %v) = %v, %v; want %v, %v", tt.sizes, tt.i, tt.step, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestResizeKeepsTheMinimum grows and shrinks each child of several splits
// until it can't, and checks that the sizes always add up to 100 and that
// no child goes below MinSize.
func TestResizeKeepsTheMinimum(t *testing.T) {
	for _, start := range [][]float64{{50, 50}, {65, 35}, {40, 30, 30}, {25, 25, 25, 25}, {10, 80, 10}} {
		for i := range start {
			for _, step := range []float64{5, -5} {
				sizes := start
				for range 40 {
					next, ok := Resize(sizes, i, step)
					if !ok {
						break
					}
					var sum float64
					for j, s := range next {
						sum += s
						if s < MinSize-1e-9 {
							t.Fatalf("%v, child %d by %v: child %d is %v, below the minimum", start, i, step, j, s)
						}
					}
					if math.Abs(sum-100) > 1e-9 {
						t.Fatalf("%v, child %d by %v: sizes %v add up to %v", start, i, step, next, sum)
					}
					sizes = next
				}
				if step > 0 && math.Abs(sizes[i]-(100-MinSize*float64(len(start)-1))) > 1e-6 {
					t.Errorf("%v: child %d grew to %v, want it as large as the others' minimum allows", start, i, sizes[i])
				}
				if step < 0 && math.Abs(sizes[i]-MinSize) > 1e-6 {
					t.Errorf("%v: child %d shrank to %v, want %v", start, i, sizes[i], float64(MinSize))
				}
			}
		}
	}
}
