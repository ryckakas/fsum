package fsum_test

import (
	"math"
	"math/big"
)

// Enough bits to hold any exact sum of up to 2^200 float64 values: they span 2^-1074 to 2^1024.
const oraclePrec = 2300

// oracleSum is Math.sumPrecise computed with arbitrary-precision arithmetic, independent of the library.
func oracleSum(xs []float64) float64 {
	posInf, negInf, allNegZero := false, false, true
	for _, x := range xs {
		switch {
		case math.IsNaN(x):
			return math.NaN()
		case math.IsInf(x, 1):
			posInf = true
		case math.IsInf(x, -1):
			negInf = true
		}
		if x != 0 || !math.Signbit(x) {
			allNegZero = false
		}
	}
	switch {
	case posInf && negInf:
		return math.NaN()
	case posInf:
		return math.Inf(1)
	case negInf:
		return math.Inf(-1)
	case allNegZero:
		return math.Copysign(0, -1)
	}
	sum := new(big.Float).SetPrec(oraclePrec)
	term := new(big.Float).SetPrec(oraclePrec)
	for _, x := range xs {
		sum.Add(sum, term.SetFloat64(x))
	}
	f, _ := sum.Float64()
	return f
}

func sameFloat(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return math.Float64bits(a) == math.Float64bits(b)
}
