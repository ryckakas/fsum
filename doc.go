// Package fsum computes the exact sum of float64 values, rounded once to the nearest float64.
//
// A plain loop rounds after every addition, so its error grows with the input: summing ten copies of 0.1 gives
// 0.9999999999999999, and 1e100 + 1 - 1e100 gives 0. Compensated sums (Kahan, Neumaier) shrink the error but
// do not remove it. fsum keeps the running sum exact and rounds only the final result, so Sum returns the
// float64 nearest to the true mathematical sum of its inputs (ties to even).
//
// # Semantics
//
// fsum follows JavaScript's Math.sumPrecise (ES2026), not Python's math.fsum, because it never fails:
//
//   - an empty input, or one where every value is -0, sums to -0;
//   - any NaN, or both +Inf and -Inf, gives NaN;
//   - otherwise an infinite input gives that infinity;
//   - otherwise the result is the exact sum rounded once, and ±Inf only when that rounding overflows. A running
//     total that overflows midway, as in [math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64], still returns
//     math.MaxFloat64.
//
// # Guarantees
//
//   - No function panics, whatever float64 values it receives.
//   - The result does not depend on the order of the inputs.
//   - The result is bit-identical to Math.sumPrecise for the same inputs, except that every NaN result is
//     math.NaN().
//
// # Cost
//
// The exact running sum is a short list of non-overlapping partial sums (Shewchuk's algorithm, as in Python's
// math.fsum). Each value costs time proportional to the number of partials, which stays at one to five for
// everyday data. Sum allocates nothing unless the input keeps more than 32 partials alive.
package fsum
