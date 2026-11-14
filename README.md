# fsum

[![CI](https://github.com/ryckakas/fsum/actions/workflows/ci.yml/badge.svg)](https://github.com/ryckakas/fsum/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ryckakas/fsum.svg)](https://pkg.go.dev/github.com/ryckakas/fsum)

Exact, correctly rounded float64 summation for Go: the equivalent of Python's `math.fsum` and JavaScript's
`Math.sumPrecise`. Zero dependencies.

```go
tenths := []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}

naive := 0.0
for _, x := range tenths {
	naive += x
}

fmt.Println(naive)            // 0.9999999999999999
fmt.Println(fsum.Sum(tenths)) // 1
```

## Why

A plain loop rounds after every addition, so its error grows with the input and can swallow values entirely:
`1e100 + 1 - 1e100` is `0`. Compensated sums (Kahan, or Neumaier as in gonum's `floats.SumCompensated`) shrink
the error but don't remove it. Neumaier returns `1e16` for `[1, 1e-100, 1e16]`, where the correctly rounded
answer is `1.0000000000000002e+16`.

`fsum` keeps the running sum exact and rounds once at the end. The result is the float64 nearest to the true
mathematical sum of the inputs, ties to even, whatever their order.

Go has had no importable exact sum. A standard-library `math.Sum` was declined in 2026
([golang/go#80011](https://github.com/golang/go/issues/80011)), and `x/exp/stats` points users who need accuracy
guarantees to "a more specialized implementation". This is one.

## Install

```sh
go get github.com/ryckakas/fsum
```

Requires Go 1.23 or newer.

## Usage

```go
// A slice, including named float types such as `type Meters float64`.
fsum.Sum([]float64{1e100, 1, -1e100}) // 1

// Any Go 1.23 iterator.
balances := map[string]float64{"alice": 0.1, "bob": 0.2, "carol": 0.3}
fsum.SumSeq(maps.Values(balances)) // 0.6

// A running sum. The zero value is ready to use; Sum doesn't consume it.
var acc fsum.Accumulator
for _, x := range []float64{1e16, 1, 1e-16} {
	acc.Add(x)
}
acc.Sum() // 1.0000000000000002e+16
```

Don't copy an `Accumulator` after first use: copies share state. `go vet` reports such copies.

## Semantics

`fsum` follows `Math.sumPrecise` rather than `math.fsum`, because Python's version raises exceptions where Go
would need an `error`, and one of them (intermediate overflow) reports a limitation of the algorithm rather than
of the input:

| Input | `fsum.Sum` | Python `math.fsum` |
|---|---|---|
| `[]`, or every value `-0` | `-0` | `0.0` |
| `[+Inf, -Inf]` | `NaN` | `ValueError` |
| `[MaxFloat64, MaxFloat64, -MaxFloat64]` | `MaxFloat64` | `OverflowError` (intermediate overflow) |
| `[1e308, 1e308]` | `+Inf` | `OverflowError` |
| any other input where Python returns a value | the same value, bit for bit (any NaN is `math.NaN()`) | that value |

In full: an empty or all `-0` input sums to `-0`; any NaN, or both infinities, gives NaN; otherwise an infinite
input gives that infinity; otherwise the exact sum is rounded once, overflowing to ±Inf only when that final
rounding does.

### Guarantees

1. No function panics, whatever float64 values it receives.
2. The result does not depend on the order of the inputs.
3. The result is bit-identical to `Math.sumPrecise`, except that every NaN result is `math.NaN()`.

## How it's tested

- **A golden corpus** of 1,033 cases (`testdata/golden.txt`), with expected values computed from exact rational
  arithmetic (`scripts/gen_golden.py`) and checked against V8's own `Math.sumPrecise`
  (`scripts/crosscheck.mjs`). It covers every edge case above, the rounding ties, and random data near overflow,
  in the subnormal range and with heavy cancellation.
- **A second, independent oracle**: `math/big` at 2,300 bits, used for inputs up to 100,000 values and by
  `FuzzSum`, which runs in CI on every change.
- Every case goes through `Sum`, `SumSeq`, `Accumulator` (with a `Sum` call midway) and `Sum` on the reversed
  input. CI runs on amd64 and arm64, on Go 1.23 and the current release.

## Performance

Exactness costs time. Each value is added into a short list of non-overlapping partial sums, so the cost per value
grows with how many partials the data keeps alive: one to five for everyday data, dozens for data spread over the
whole float64 range.

Time to sum 1,000,000 values (Go 1.27.4, Intel i7-6800K, `go test -bench .`):

| Data | Plain loop | Kahan | Neumaier | `fsum.Sum` | `Sum` vs plain loop |
|---|---|---|---|---|---|
| Uniform in [0, 1) | 0.89 ms | 3.68 ms | 1.56 ms | 14.5 ms | 16x |
| Canceling pairs, shuffled | 0.88 ms | 3.70 ms | 1.54 ms | 46.8 ms | 53x |
| Exponents spread over 2^-300 to 2^300 | 0.90 ms | 3.73 ms | 1.64 ms | 109 ms | 121x |
| Exponents spread over the whole float64 range | 0.89 ms | 3.65 ms | 1.46 ms | 245 ms | 276x |

Neumaier is the algorithm behind gonum's `floats.SumCompensated`; Kahan and Neumaier are faster but not exact.
`Sum` makes no heap allocation unless the input keeps more than 32 partials alive. A faster O(1)-per-value
algorithm (Neal's xsum, used by V8 for `Math.sumPrecise`) is planned for a later version, with the same results
bit for bit.

## Credits

- Jonathan Richard Shewchuk, "Adaptive Precision Floating-Point Arithmetic and Fast Robust Geometric
  Predicates" (1997): the non-overlapping partials.
- Python's [`math.fsum`](https://github.com/python/cpython/blob/main/Modules/mathmodule.c): the summation
  loop and the half-way rounding correction.
- The TC39 [`Math.sumPrecise` proposal](https://github.com/tc39/proposal-math-sum): the semantics and the
  overflow handling.

## License

[MIT](LICENSE)
