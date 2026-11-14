package fsum_test

import (
	"fmt"
	"maps"
	"math"

	"github.com/ryckakas/fsum"
)

func ExampleSum() {
	tenths := []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}

	naive := 0.0
	for _, x := range tenths {
		naive += x
	}

	fmt.Println(naive)
	fmt.Println(fsum.Sum(tenths))
	// Output:
	// 0.9999999999999999
	// 1
}

func ExampleSum_cancellation() {
	fmt.Println(fsum.Sum([]float64{1e100, 1, -1e100}))
	fmt.Println(fsum.Sum([]float64{math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64}))
	// Output:
	// 1
	// 1.7976931348623157e+308
}

func ExampleSumSeq() {
	balances := map[string]float64{"alice": 0.1, "bob": 0.2, "carol": 0.3}
	fmt.Println(fsum.SumSeq(maps.Values(balances)))
	// Output: 0.6
}

func ExampleAccumulator() {
	var acc fsum.Accumulator
	for _, x := range []float64{1e16, 1, 1e-16} {
		acc.Add(x)
	}
	fmt.Println(acc.Sum())
	// Output: 1.0000000000000002e+16
}
