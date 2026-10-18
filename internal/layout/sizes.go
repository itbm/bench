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
