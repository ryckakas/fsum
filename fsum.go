package fsum

import "iter"

// Typical inputs keep one to five partials alive, so this buffer keeps Sum off the heap; adversarial inputs
// spill over it into a growable slice.
const stackPartials = 32

// Sum returns the exact sum of xs, rounded once to the nearest float64 (ties to even).
//
// It follows JavaScript's Math.sumPrecise: an empty or all -0 input sums to -0, any NaN or a mix of +Inf and
// -Inf gives NaN, any other infinite input gives that infinity, and finite inputs give ±Inf only when their exact
// sum rounds beyond math.MaxFloat64.
func Sum[S ~[]E, E ~float64](xs S) E {
	var buf [stackPartials]float64
	var s state
	partials := buf[:0]
	for _, x := range xs {
		partials = s.add(partials, float64(x))
	}
	return E(s.sum(partials))
}

// SumSeq returns the exact sum of the values seq yields, rounded once to the nearest float64 (ties to even).
// It has the same semantics as Sum.
func SumSeq[E ~float64](seq iter.Seq[E]) E {
	var buf [stackPartials]float64
	var s state
	partials := buf[:0]
	for x := range seq {
		partials = s.add(partials, float64(x))
	}
	return E(s.sum(partials))
}

// Accumulator is an exact running sum of float64 values with the same semantics as Sum.
//
// The zero value is an empty sum, ready to use. An Accumulator must not be copied after first use: copies share
// internal state and corrupt each other's sums. go vet reports such copies.
type Accumulator struct {
	_        noCopy
	s        state
	partials []float64
}

// Add adds x to the running sum.
func (a *Accumulator) Add(x float64) {
	a.partials = a.s.add(a.partials, x)
}

// Sum returns the exact sum of every value added so far, rounded once to the nearest float64. It does not
// change the accumulator, so Add can continue afterwards.
func (a *Accumulator) Sum() float64 {
	return a.s.sum(a.partials)
}

// Reset empties the accumulator, keeping its allocated capacity for reuse.
func (a *Accumulator) Reset() {
	a.s = state{}
	a.partials = a.partials[:0]
}

// noCopy makes go vet's copylocks check flag copies of the struct that embeds it.
type noCopy struct{}

// Lock does nothing; it makes noCopy a sync.Locker, which is what copylocks looks for.
func (*noCopy) Lock() {}

// Unlock does nothing; see Lock.
func (*noCopy) Unlock() {}
