package fsum

import "iter"

// Sum returns the exact sum of xs, rounded once to the nearest float64 (ties to even).
func Sum[S ~[]E, E ~float64](xs S) E {
	return 0
}

// SumSeq returns the exact sum of the values seq yields, rounded once to the nearest float64 (ties to even).
func SumSeq[E ~float64](seq iter.Seq[E]) E {
	return 0
}

// Accumulator is an exact running sum of float64 values. The zero value is an empty sum, ready to use.
type Accumulator struct{}

// Add adds x to the running sum.
func (a *Accumulator) Add(x float64) {}

// Sum returns the exact sum of every value added so far, rounded once to the nearest float64.
func (a *Accumulator) Sum() float64 {
	return 0
}

// Reset empties the accumulator.
func (a *Accumulator) Reset() {}
