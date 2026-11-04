package layout

import (
	"math"
	"slices"
)

// Normalise returns n sizes that add up to 100, in proportion to sizes. If
// sizes has the wrong length, or any size isn't a positive finite number, it
// returns equal sizes. It also returns equal sizes in the unlikely case that
// the proportions are too extreme to represent as positive numbers. It
// returns nil when n isn't positive.
func Normalise(sizes []float64, n int) []float64 {
	if n <= 0 {
		return nil
	}
	if len(sizes) == n && allPositive(sizes) {
		// Dividing by the largest size first only matters when the sizes
		// are so large that their sum overflows.
		if out, ok := proportions(sizes, 1); ok {
			return out
		}
		if out, ok := proportions(sizes, slices.Max(sizes)); ok {
			return out
		}
	}
	return equalSizes(n)
}

// proportions scales sizes (each divided by unit) to add up to 100. It
// reports false if any result isn't a positive finite number.
func proportions(sizes []float64, unit float64) ([]float64, bool) {
	var sum float64
	for _, s := range sizes {
		sum += s / unit
	}
	out := make([]float64, len(sizes))
	for i, s := range sizes {
		v := s / unit * 100 / sum
		if !isPositive(v) {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// equalSizes returns n equal sizes that add up to 100.
func equalSizes(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 100 / float64(n)
	}
	return out
}

// isPositive reports whether v is a positive finite number.
func isPositive(v float64) bool {
	return v > 0 && !math.IsInf(v, 1)
}

// allPositive reports whether every size is a positive finite number.
func allPositive(sizes []float64) bool {
	for _, s := range sizes {
		if !isPositive(s) {
			return false
		}
	}
	return true
}

// MinSize is the smallest that Resize makes a child of a split, in percent
// of the split, as dragging a divider does (MIN_SIZE in the frontend's
// drag.js).
const MinSize = 10

// Resize returns a split's sizes, normalised, with child i step percentage
// points larger, or smaller for a negative step: the Grow pane and Shrink
// pane keys. The room comes from the other children, or goes to them: when
// child i grows, each other child gives in proportion to its room above
// MinSize, so that none goes below it; when it shrinks, the others share
// the room in proportion to their sizes, which keeps their ratios. Child i
// stays at MinSize or more, and a child that's smaller already isn't made
// smaller. Resize changes child i by less than step if that's all the room
// there is, and reports false if there's none, or if i isn't a child.
func Resize(sizes []float64, i int, step float64) ([]float64, bool) {
	n := len(sizes)
	if i < 0 || i >= n || n < 2 || step == 0 || !isPositive(math.Abs(step)) {
		return nil, false
	}
	out := Normalise(sizes, n)
	// room is how much each child can give, and total all of it.
	room := make([]float64, n)
	var total float64
	if step > 0 {
		for j, s := range out {
			if j != i {
				room[j] = max(0, s-MinSize)
				total += room[j]
			}
		}
	} else {
		room[i] = max(0, out[i]-MinSize)
		total = room[i]
	}
	d := min(math.Abs(step), total)
	// A change too small to see is no change.
	if d < 0.05 {
		return nil, false
	}
	if step > 0 {
		for j := range out {
			if j != i {
				out[j] -= d * room[j] / total
			}
		}
		out[i] += d
	} else {
		rest := 100 - out[i]
		for j := range out {
			if j != i {
				out[j] += d * out[j] / rest
			}
		}
		out[i] -= d
	}
	return Normalise(out, n), true
}
