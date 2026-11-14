package fsum

import "math"

// state is an exact running sum with Math.sumPrecise semantics: the superaccumulator holds the finite values
// and the flags hold what it cannot (NaN, infinities and the sign of an all -0 sum).
type state struct {
	acc        small
	special    float64
	hasSpecial bool
	// sawNonNegZero is false while every input so far was -0, which is when the sum itself is -0.
	sawNonNegZero bool
}

func (s *state) add(x float64) {
	if s.acc.adds == addsPerNormalize {
		s.acc.normalize()
	}
	s.acc.adds++
	s.put(x)
}

// addSlice is add for every value of xs, with the carry bookkeeping done once per block instead of per value.
func addSlice[E ~float64](s *state, xs []E) {
	for len(xs) > 0 {
		if s.acc.adds == addsPerNormalize {
			s.acc.normalize()
		}
		block := xs[:min(len(xs), addsPerNormalize-int(s.acc.adds))]
		xs = xs[len(block):]
		s.acc.adds += int32(len(block)) //nolint:gosec // a block is at most addsPerNormalize long
		for _, x := range block {
			b := math.Float64bits(float64(x))
			if isRare(b) {
				s.putRare(float64(x), b)
				continue
			}
			s.sawNonNegZero = true
			s.acc.put(b)
		}
	}
}

func (s *state) put(x float64) {
	b := math.Float64bits(x)
	if isRare(b) {
		s.putRare(x, b)
		return
	}
	s.sawNonNegZero = true
	s.acc.put(b)
}

// isRare reports whether b is a zero, a subnormal, an infinity or a NaN. Their exponents are 0 and 0x7ff,
// which one unsigned compare covers.
func isRare(b uint64) bool {
	return b>>mantissaBits&exponentMask-1 >= exponentMask-1
}

func (s *state) putRare(x float64, b uint64) {
	switch {
	case b == 0:
		s.sawNonNegZero = true
	case b<<1 == 0:
	case b>>mantissaBits&exponentMask == 0:
		s.sawNonNegZero = true
		s.acc.putSubnormal(b)
	case !s.hasSpecial:
		s.special, s.hasSpecial = x, true
	case s.special != x:
		s.special = math.NaN()
	}
}

// sum normalizes the accumulator in place, which changes its representation but not its value.
func (s *state) sum() float64 {
	switch {
	case s.hasSpecial:
		if math.IsNaN(s.special) {
			return math.NaN()
		}
		return s.special
	case !s.sawNonNegZero:
		return math.Copysign(0, -1)
	}
	return s.acc.round()
}
