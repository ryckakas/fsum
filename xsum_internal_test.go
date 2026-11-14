package fsum

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

const exactPrec = 2400

// exact is the accumulator's value computed from its chunks with arbitrary precision.
func (a *small) exact() *big.Float {
	sum := new(big.Float).SetPrec(exactPrec)
	term := new(big.Float).SetPrec(exactPrec)
	for i, c := range a.chunk {
		term.SetInt64(c)
		sum.Add(sum, term.SetMantExp(term, lowBits*i-1075))
	}
	return sum
}

func nearest(x *big.Float) float64 {
	f, _ := x.Float64()
	return f
}

func sameBits(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b)
}

// randomChunks fills a few chunks with values of random width and sign, anywhere in the accumulator,
// including the last chunk, which summing real float64 values cannot reach in a test.
func randomChunks(r *rand.Rand) *small {
	a := &small{used: ^uint64(0)}
	base := r.IntN(chunks)
	for range 1 + r.IntN(5) {
		i := min(base+r.IntN(6), lastChunk)
		c := int64(r.Uint64() >> (1 + r.IntN(63)))
		if r.IntN(2) == 0 {
			c = -c
		}
		a.chunk[i] = c
	}
	// Units below 2^-1074 never occur: every float64 is a multiple of it.
	a.chunk[0] &^= 1
	return a
}

func TestNormalizeKeepsValueAndRanges(t *testing.T) {
	r := rand.New(rand.NewPCG(67, 32))
	for range 20_000 {
		a := randomChunks(r)
		want := a.exact()
		top := a.normalize()
		if got := a.exact(); got.Cmp(want) != 0 {
			t.Fatalf("normalize changed the value: %v, want %v", got, want)
		}
		if top < 0 {
			continue
		}
		if a.chunk[top] == 0 || top > 0 && a.chunk[top] == -1 {
			t.Fatalf("top chunk %d is %d", top, a.chunk[top])
		}
		for i := range top {
			if a.chunk[i] < 0 || a.chunk[i] > lowMask {
				t.Fatalf("chunk %d of %d is %d, outside [0, 2^32)", i, top, a.chunk[i])
			}
		}
	}
}

func TestRoundMatchesExactValue(t *testing.T) {
	r := rand.New(rand.NewPCG(1075, 53))
	for range 50_000 {
		a := randomChunks(r)
		want := nearest(a.exact())
		if got := a.round(); !sameBits(got, want) {
			t.Fatalf("round = %v (%016x), want %v (%016x)", got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
}

type tieVariant struct{ sticky, odd, sign int64 }

func tieVariants() []tieVariant {
	vs := make([]tieVariant, 0, 12)
	for _, sticky := range []int64{0, 1, -1} {
		for _, odd := range []int64{0, 1} {
			vs = append(vs, tieVariant{sticky, odd, 1}, tieVariant{sticky, odd, -1})
		}
	}
	return vs
}

// TestRoundTiesWithDistantStickyBits puts a sum exactly on, just above and just below a rounding tie, with
// the deciding bit far below the mantissa, for both signs and for an even and an odd mantissa.
func TestRoundTiesWithDistantStickyBits(t *testing.T) {
	variants := tieVariants()
	for top := 2; top < lastChunk; top++ {
		for shift := range lowBits {
			for _, v := range variants {
				a := &small{used: ^uint64(0)}
				a.chunk[top] = v.sign << shift
				// The mantissa's last bit sits 52 bits below the leading one, the tie bit 53 below.
				a.chunk[top-1] += v.sign * v.odd << shift >> 20
				a.chunk[top-2] += v.sign << shift << 11
				a.chunk[0] += v.sticky * 2
				want := nearest(a.exact())
				if got := a.round(); !sameBits(got, want) {
					t.Fatalf("top %d shift %d %+v: round = %016x, want %016x",
						top, shift, v, math.Float64bits(got), math.Float64bits(want))
				}
			}
		}
	}
}

func TestLastChunkAbsorbsCarriesAndRoundsToInfinity(t *testing.T) {
	for _, sign := range []int64{1, -1} {
		var s state
		s.put(1.5)
		s.acc.chunk[lastChunk-1] += sign << 62
		s.acc.used = ^uint64(0)

		if top := s.acc.normalize(); top != lastChunk {
			t.Fatalf("sign %d: top chunk is %d, want the last chunk", sign, top)
		}
		probe := s
		if got, want := probe.sum(), math.Inf(int(sign)); got != want {
			t.Errorf("sign %d: sum = %v, want %v", sign, got, want)
		}

		s.acc.chunk[lastChunk] -= sign << 30
		if got := s.sum(); got != 1.5 {
			t.Errorf("sign %d: after canceling the overflow, sum = %v, want 1.5", sign, got)
		}
	}
}

func TestAccumulatorAddDoesNotAllocate(t *testing.T) {
	var acc Accumulator
	x := 0x1p-300
	if allocs := testing.AllocsPerRun(5000, func() { acc.Add(x); x *= 1.0000001 }); allocs != 0 {
		t.Errorf("Add allocates %v times per call, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(100, func() { acc.Sum() }); allocs != 0 {
		t.Errorf("Sum allocates %v times per call, want 0", allocs)
	}
}
