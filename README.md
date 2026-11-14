# fsum

Exact, correctly rounded float64 summation for Go.

## Performance

Exactness costs time. Each value is added into a short list of non-overlapping partial sums, so the cost per value
grows with how many partials the data keeps alive: one to five for everyday data, hundreds for adversarial input.

Time to sum 1,000,000 values (Go 1.27.4, Intel i7-6800K, `go test -bench .`):

| Data | Plain loop | Kahan | Neumaier | `fsum.Sum` | `Sum` vs plain loop |
|---|---|---|---|---|---|
| Uniform in [0, 1) | 0.90 ms | 3.67 ms | 1.55 ms | 14.5 ms | 16x |
| Canceling pairs | 0.89 ms | 3.71 ms | 1.54 ms | 9.0 ms | 10x |
| Exponents spread over 2^-300 to 2^300 | 0.89 ms | 3.71 ms | 1.51 ms | 108 ms | 122x |
| Adversarial (many partials) | 0.89 ms | 3.65 ms | 1.57 ms | 147 ms | 165x |

Neumaier is the algorithm behind gonum's `floats.SumCompensated`; Kahan and Neumaier are faster but not exact.
`Sum` makes no heap allocation unless the input keeps more than 32 partials alive. A faster O(1)-per-value
algorithm (Neal's xsum, which browsers use for `Math.sumPrecise`) is planned for a later version, with the same
results bit for bit.
