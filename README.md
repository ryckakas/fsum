# fsum

Exact, correctly rounded float64 summation for Go.

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
