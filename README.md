# fsum

[![CI](https://github.com/ryckakas/fsum/actions/workflows/ci.yml/badge.svg)](https://github.com/ryckakas/fsum/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ryckakas/fsum.svg)](https://pkg.go.dev/github.com/ryckakas/fsum)

Exact, correctly rounded float64 summation for Go: the equivalent of Python's `math.fsum` and JavaScript's
`Math.sumPrecise`. Zero dependencies.

Also for PHP: [ryckakas/fsum-php](https://github.com/ryckakas/fsum-php) (`composer require ryckakas/fsum`), with
the same results bit for bit, checked against this repository's golden corpus.

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
3. The result is bit-identical to what the `Math.sumPrecise` specification requires, except that every NaN result
   is `math.NaN()`. Engines agree with it on any input you can hold in memory; by their source, they return NaN
   once more than 2^45 values push the running total past about 2^1069, where the specification and fsum keep the
   exact sum.

## How it's tested

- **A golden corpus** of 1,033 cases (`testdata/golden.txt`), with expected values computed from exact rational
  arithmetic (`scripts/gen_golden.py`) and checked against V8's own `Math.sumPrecise`
  (`scripts/crosscheck.mjs`). It covers every edge case above, the rounding ties, and random data near overflow,
  in the subnormal range and with heavy cancellation.
- **A second, independent oracle**: `math/big` at 2,300 bits, used for inputs up to 100,000 values and by the
  fuzz targets, which run in CI on every change.
- **A second exact engine**: the Shewchuk implementation from v0.1, kept in the tests and fuzzed against the
  current one on inputs up to 200,000 values.
- **The accumulator itself**: random internal states, including ones no real input reaches in a test, rounded
  against `math/big`; negating every input must negate the result.
- Every case goes through `Sum`, `SumSeq`, `Accumulator` (with a `Sum` call midway) and `Sum` on the reversed
  input. CI runs on amd64 and arm64, on Go 1.23 and the current release.

## Performance

Exactness costs time, but a constant amount per value. The running sum is a superaccumulator: 67 overlapping
64-bit integer chunks, where adding a value touches two chunks and never carries. The cost does not depend on the
data, and no function allocates.

Time to sum 1,000,000 values (Go 1.27.4, Apple M5, median of `go test -bench . -count 10`):

| Data | Plain loop | Kahan | Neumaier | `fsum.Sum` | `Sum` vs plain loop |
|---|---|---|---|---|---|
| Uniform in [0, 1) | 0.58 ms | 1.97 ms | 1.17 ms | 1.36 ms | 2.4x |
| Canceling pairs, shuffled | 0.58 ms | 2.01 ms | 1.18 ms | 1.54 ms | 2.6x |
| Exponents spread over 2^-300 to 2^300 | 0.58 ms | 1.99 ms | 1.17 ms | 1.42 ms | 2.4x |
| Exponents spread over the whole float64 range | 0.59 ms | 2.01 ms | 1.17 ms | 1.58 ms | 2.7x |

Neumaier is the algorithm behind gonum's `floats.SumCompensated`; Kahan and Neumaier are not exact.

Short inputs pay a fixed cost for rounding: `Sum` takes about 1.4 ns for one or two values, 16 to 32 ns for three
and 160 to 230 ns for a hundred. An `Accumulator` costs about 2.3 ns per value and holds its state inline,
about 560 bytes.

## Credits

- Radford M. Neal, "Fast exact summation using small and large superaccumulators" (2015) and his
  [xsum](https://gitlab.com/radfordneal/xsum): the superaccumulator. `xsum.go` carries its licence notice.
- Jonathan Richard Shewchuk, "Adaptive Precision Floating-Point Arithmetic and Fast Robust Geometric
  Predicates" (1997): the non-overlapping partials of the first engine, which the tests still compare against.
- Python's [`math.fsum`](https://github.com/python/cpython/blob/main/Modules/mathmodule.c): the summation
  loop and the half-way rounding correction of that first engine.
- The TC39 [`Math.sumPrecise` proposal](https://github.com/tc39/proposal-math-sum) and its polyfill by Kevin
  Gibbons: the semantics, and the overflow handling of the first engine. `shewchuk_test.go` carries the licence notices.

## License

[MIT](LICENSE)
