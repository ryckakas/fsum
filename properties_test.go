package fsum_test

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/ryckakas/fsum"
)

func negated(xs []float64) []float64 {
	neg := make([]float64, len(xs))
	for i, x := range xs {
		neg[i] = -x
	}
	return neg
}

// checkNegated checks that negating every input negates the result. Zero and NaN results are left to the
// oracle: -0 and +0 do not mirror each other.
func checkNegated(t *testing.T, xs []float64, want float64) {
	t.Helper()
	if want == 0 || math.IsNaN(want) {
		return
	}
	for api, got := range entryPoints(negated(xs)) {
		if !sameFloat(got, -want) {
			t.Errorf("negated %s = %v (%016x), want %v (%016x)", api, got, math.Float64bits(got), -want, math.Float64bits(-want))
		}
	}
}

// carryTop has a biased exponent whose low five bits are all ones and a full mantissa, the largest
// contribution a value can make to one chunk of a superaccumulator.
var carryTop = math.Nextafter(2, 0)

var carryExponents = []int{-992, 0, 992}

func TestCarryBoundaries(t *testing.T) {
	r := rand.New(rand.NewPCG(2046, 31))
	for _, n := range []int{2045, 2046, 2047, 2048, 4091, 4092, 4093, 6137, 6138, 6139} {
		for _, exp := range carryExponents {
			same := slices.Repeat([]float64{math.Ldexp(carryTop, exp)}, n)
			checkAll(t, same, oracleSum(same))

			mixed := slices.Clone(same)
			for i := range mixed {
				if r.IntN(2) == 0 {
					mixed[i] = -mixed[i]
				}
			}
			checkAll(t, mixed, oracleSum(mixed))
		}
	}
}

func TestAccumulatorSumAtCarryBoundaries(t *testing.T) {
	readAt := []int{2045, 2046, 2047, 2048, 4091, 4092, 4093, 4094}
	for _, exp := range carryExponents {
		xs := slices.Repeat([]float64{math.Ldexp(carryTop, exp)}, 4100)
		var acc fsum.Accumulator
		for i, x := range xs {
			acc.Add(x)
			if !slices.Contains(readAt, i+1) {
				continue
			}
			if got, want := acc.Sum(), oracleSum(xs[:i+1]); !sameFloat(got, want) {
				t.Errorf("exponent %d, after %d values: Sum = %016x, want %016x", exp, i+1, math.Float64bits(got), math.Float64bits(want))
			}
		}
	}
}

const maxRepeatedValues = 20_000

// decodeRepeated reads 10-byte records, a float64 and a repeat count, so a short input can stand for
// thousands of values.
func decodeRepeated(data []byte) []float64 {
	var xs []float64
	for ; len(data) >= 10 && len(xs) < maxRepeatedValues; data = data[10:] {
		x := math.Float64frombits(binary.LittleEndian.Uint64(data))
		n := int(binary.LittleEndian.Uint16(data[8:]))%4097 + 1
		for range min(n, maxRepeatedValues-len(xs)) {
			xs = append(xs, x)
		}
	}
	return xs
}

func encodeRepeated(x float64, n int) []byte {
	b := binary.LittleEndian.AppendUint64(nil, math.Float64bits(x))
	return binary.LittleEndian.AppendUint16(b, uint16(n-1))
}

func FuzzSumRepeated(f *testing.F) {
	for _, n := range []int{2045, 2046, 2047, 2048, 4091, 4092, 4093, 4094, 4095, 4096} {
		f.Add(encodeRepeated(carryTop, n))
		f.Add(append(encodeRepeated(math.MaxFloat64, n), encodeRepeated(-math.MaxFloat64, n)...))
	}
	f.Add(append(encodeRepeated(1, 4096), encodeRepeated(math.Ldexp(1, -53), 3)...))
	f.Add(append(encodeRepeated(-1, 2047), encodeRepeated(math.SmallestNonzeroFloat64, 2047)...))

	f.Fuzz(func(t *testing.T, data []byte) {
		xs := decodeRepeated(data)
		checkAll(t, xs, oracleSum(xs))
	})
}

const maxEngineValues = 200_000

// FuzzEngines compares the engine with the v0.1 one. Neither needs the arbitrary-precision oracle, so inputs
// can be ten times longer than in FuzzSumRepeated.
func FuzzEngines(f *testing.F) {
	for _, n := range []int{2046, 2047, 4092, 4093} {
		f.Add(append(encodeRepeated(-carryTop, n), encodeRepeated(math.Ldexp(carryTop, -60), n)...))
		f.Add(append(encodeRepeated(math.MaxFloat64, n), encodeRepeated(-math.MaxFloat64, n)...))
	}
	f.Add(encodeRepeated(0.1, 4096))

	f.Fuzz(func(t *testing.T, data []byte) {
		var xs []float64
		for len(xs) < maxEngineValues && len(data) >= 10 {
			xs = append(xs, decodeRepeated(data)...)
			data = data[min(len(data), 10*maxRepeatedValues):]
		}
		if got, want := fsum.Sum(xs), fsum.ShewchukSum(xs); !sameFloat(got, want) {
			t.Errorf("Sum = %016x, v0.1 engine = %016x", math.Float64bits(got), math.Float64bits(want))
		}
		if got, want := fsum.Sum(negated(xs)), fsum.ShewchukSum(negated(xs)); !sameFloat(got, want) {
			t.Errorf("negated: Sum = %016x, v0.1 engine = %016x", math.Float64bits(got), math.Float64bits(want))
		}
	})
}

// A sum of exactly minus one unit of a chunk once lost that chunk: the carry pass folded the sign into the
// chunk below the tracked range.
func TestDifferenceOfNeighbors(t *testing.T) {
	for exp := -1022; exp <= 1023; exp++ {
		p := math.Ldexp(1, exp)
		for _, xs := range [][]float64{
			{p, -math.Nextafter(p, math.Inf(1))},
			{math.Nextafter(p, math.Inf(1)), -p},
			{p, -math.Nextafter(p, 0)},
			{2 * p, -p, -math.Nextafter(p, math.Inf(1))},
		} {
			checkAll(t, xs, oracleSum(xs))
		}
	}
}
