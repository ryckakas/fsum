package fsum_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/ryckakas/fsum"
)

var sink float64

func naiveSum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func kahanSum(xs []float64) float64 {
	s, c := 0.0, 0.0
	for _, x := range xs {
		y := x - c
		t := s + y
		c = (t - s) - y
		s = t
	}
	return s
}

// neumaierSum is the algorithm behind gonum's floats.SumCompensated.
func neumaierSum(xs []float64) float64 {
	s, c := 0.0, 0.0
	for _, x := range xs {
		t := s + x
		if math.Abs(s) >= math.Abs(x) {
			c += (s - t) + x
		} else {
			c += (x - t) + s
		}
		s = t
	}
	return s + c
}

func accumulatorSum(xs []float64) float64 {
	var acc fsum.Accumulator
	for _, x := range xs {
		acc.Add(x)
	}
	return acc.Sum()
}

type benchData struct {
	name string
	gen  func(r *rand.Rand, n int) []float64
}

var benchClasses = []benchData{
	{"uniform", func(r *rand.Rand, n int) []float64 {
		xs := make([]float64, n)
		for i := range xs {
			xs[i] = r.Float64()
		}
		return xs
	}},
	{"wide-exponent", func(r *rand.Rand, n int) []float64 {
		xs := make([]float64, n)
		for i := range xs {
			xs[i] = randomFinite(r, -300, 300)
		}
		return xs
	}},
	{"canceling", func(r *rand.Rand, n int) []float64 {
		xs := make([]float64, n)
		for i := 0; i+1 < n; i += 2 {
			xs[i] = randomFinite(r, -60, 60)
			xs[i+1] = -xs[i]
		}
		return xs
	}},
	{"adversarial", func(r *rand.Rand, n int) []float64 {
		xs := make([]float64, n)
		for i := range xs {
			xs[i] = randomFinite(r, 1000-3*(i%690), 1000-3*(i%690))
		}
		return xs
	}},
}

var benchFuncs = []struct {
	name string
	sum  func([]float64) float64
}{
	{"naive", naiveSum},
	{"kahan", kahanSum},
	{"neumaier", neumaierSum},
	{"Sum", fsum.Sum[[]float64]},
	{"Accumulator", accumulatorSum},
}

func BenchmarkSum(b *testing.B) {
	for _, class := range benchClasses {
		for _, n := range []int{10, 1_000, 1_000_000} {
			xs := class.gen(rand.New(rand.NewPCG(uint64(n), 7)), n)
			for _, f := range benchFuncs {
				b.Run(fmt.Sprintf("%s/n=%d/%s", class.name, n, f.name), func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(8 * n))
					for range b.N {
						sink = f.sum(xs)
					}
				})
			}
		}
	}
}
