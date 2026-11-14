package fsum

import (
	"iter"
	"math"
)

// Sum returns the exact sum of xs, rounded once to the nearest float64 (ties to even).
//
// It follows JavaScript's Math.sumPrecise: an empty or all -0 input sums to -0, any NaN or a mix of +Inf and
// -Inf gives NaN, any other infinite input gives that infinity, and finite inputs give ±Inf only when their exact
// sum rounds beyond math.MaxFloat64.
func Sum[S ~[]E, E ~float64](xs S) E {
	// One or two values need no accumulator: a single IEEE addition is already the exact sum rounded once.
	switch len(xs) {
	case 0:
		return E(math.Copysign(0, -1))
	case 1:
		return E(canonical(float64(xs[0])))
	case 2:
		return E(canonical(float64(xs[0]) + float64(xs[1])))
	}
	var s state
	addSlice(&s, xs)
	return E(s.sum())
}

func canonical(x float64) float64 {
	if math.IsNaN(x) {
		return math.NaN()
	}
	return x
}

// SumSeq returns the exact sum of the values seq yields, rounded once to the nearest float64 (ties to even).
// It has the same semantics as Sum.
func SumSeq[E ~float64](seq iter.Seq[E]) E {
	var s state
	for x := range seq {
		s.add(float64(x))
	}
	return E(s.sum())
}

// Accumulator is an exact running sum of float64 values with the same semantics as Sum.
//
// The zero value is an empty sum, ready to use. It holds its whole state inline, about 560 bytes, and never
// allocates. An Accumulator should not be copied after first use: go vet reports such copies.
type Accumulator struct {
	_ noCopy
	s state
}

// Add adds x to the running sum.
func (a *Accumulator) Add(x float64) {
	a.s.add(x)
}

// Sum returns the exact sum of every value added so far, rounded once to the nearest float64. It does not
// change the accumulator, so Add can continue afterwards.
func (a *Accumulator) Sum() float64 {
	s := a.s
	return s.sum()
}

// Reset empties the accumulator, keeping its allocated capacity for reuse.
func (a *Accumulator) Reset() {
	a.s = state{}
}

// noCopy makes go vet's copylocks check flag copies of the struct that embeds it.
type noCopy struct{}

// Lock does nothing; it makes noCopy a sync.Locker, which is what copylocks looks for.
func (*noCopy) Lock() {}

// Unlock does nothing; see Lock.
func (*noCopy) Unlock() {}
