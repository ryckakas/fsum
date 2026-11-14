"""Generate testdata/golden.txt: float64 sums with Math.sumPrecise semantics, computed with exact rationals.

Each line is `<name>\t<expected>\t<inputs>`, every value a 16-digit big-endian hex bit pattern and inputs separated
by spaces. Lines starting with `#` are comments. The file is deterministic: rerunning reproduces it byte for byte.

Usage: python scripts/gen_golden.py [output]
"""

import math
import random
import struct
import sys
from fractions import Fraction

SEED = 20261114
MAX = 1.7976931348623157e308
TINY = 5e-324
NEG_ZERO = -0.0
INF = math.inf
NAN = math.nan


def bits(x):
    return struct.pack(">d", x).hex()


def is_neg_zero(x):
    return x == 0 and math.copysign(1, x) < 0


def expected(xs):
    """Math.sumPrecise, except that every NaN result is the canonical quiet NaN."""
    if any(math.isnan(x) for x in xs):
        return NAN
    has_pos_inf = INF in xs
    has_neg_inf = -INF in xs
    if has_pos_inf and has_neg_inf:
        return NAN
    if has_pos_inf:
        return INF
    if has_neg_inf:
        return -INF
    if all(is_neg_zero(x) for x in xs):
        return NEG_ZERO
    exact = sum((Fraction(x) for x in xs), Fraction(0))
    if exact == 0:
        return 0.0
    try:
        # int / int true division is correctly rounded (ties to even) in CPython.
        return exact.numerator / exact.denominator
    except OverflowError:
        return INF if exact > 0 else -INF


def canonical(x):
    return math.nan if math.isnan(x) else x


def hand_cases():
    p = math.ldexp
    nz = NEG_ZERO
    cases = {
        "empty": [],
        "one": [1.5],
        "neg-zero": [nz],
        "neg-zeros": [nz, nz],
        "pos-neg-zero": [0.0, nz],
        "neg-pos-zero": [nz, 0.0],
        "pos-zero": [0.0],
        "cancel-to-zero": [1.0, -1.0],
        "neg-zero-then-cancel": [nz, 1.0, -1.0],
        "nan": [NAN],
        "pos-inf": [INF],
        "neg-inf": [-INF],
        "inf-inf": [INF, INF],
        "inf-neg-inf": [INF, -INF],
        "nan-inf-neg-inf": [NAN, INF, -INF],
        "inf-nan": [INF, NAN],
        "inf-with-finite": [1.0, INF, -1e308],
        "intermediate-overflow": [1e308, 1e308, -1e308],
        "intermediate-overflow-neg": [-1e308, -1e308, 1e308],
        "max-max-neg-max": [MAX, MAX, -MAX],
        "true-overflow": [1e308, 1e308],
        "true-overflow-neg": [-1e308, -1e308],
        "max-below-tie": [MAX, p(1, 969)],
        "max-exact-tie": [MAX, p(1, 970)],
        "max-just-below-tie": [MAX, p(1, 970), -TINY],
        "max-just-above-tie": [MAX, p(1, 970), TINY],
        "neg-max-exact-tie": [-MAX, -p(1, 970)],
        "neg-max-just-below-tie": [-MAX, -p(1, 970), TINY],
        "inf-then-overflow": [INF, 1e308, 1e308],
        "overflow-then-inf": [1e308, 1e308, INF],
        "nan-then-overflow": [NAN, 1e308, 1e308],
        "tenth-times-ten": [0.1] * 10,
        "big-small-big": [1e100, 1.0, -1e100],
        "two-ones-hidden": [1.0, 1e100, 1.0, -1e100],
        "subnormal-cancel": [TINY, -TINY, TINY],
        "subnormal-pair": [TINY, TINY],
        "tie-to-even-down": [1.0, p(1, -53)],
        "tie-broken-up": [1.0, p(1, -53), p(1, -105)],
        "tie-broken-down": [1.0, p(1, -53), -p(1, -105)],
        "tie-odd-mantissa": [1.0 + p(1, -52), p(1, -53)],
        "halfway-fixup": [1e16, 1.0, 1e-16],
        "halfway-fixup-reversed": [1e-16, 1.0, 1e16],
        "max-plus-neg-max": [MAX, -MAX],
        "smallest-normal-split": [p(1, -1022), -TINY],
    }
    return list(cases.items())


def rand_float(rng, emin=-1074, emax=1023):
    m = rng.getrandbits(52) | (1 << 52)
    e = rng.randint(emin, emax - 52)
    x = math.ldexp(m, e)
    if math.isinf(x):
        x = MAX
    return x if rng.random() < 0.5 else -x


def gen_uniform(rng, n):
    return [rng.random() for _ in range(n)]


def gen_wide(rng, n):
    return [rand_float(rng) for _ in range(n)]


def gen_cancel(rng, n):
    """Large values that cancel exactly, hiding a residual far below their precision."""
    half = [rand_float(rng, -60, 60) for _ in range(n // 2)]
    residual = [rand_float(rng, -120, -60) for _ in range(max(1, n - 2 * len(half)))]
    xs = half + [-x for x in half] + residual
    rng.shuffle(xs)
    return xs


def gen_subnormal(rng, n):
    return [math.ldexp(rng.randint(1, 1 << 40), -1074) * rng.choice([1, -1]) for _ in range(n)]


def gen_near_overflow(rng, n):
    """Half the cases overflow for real; the other half overflow only in a naive running sum."""
    if rng.random() < 0.5:
        return [rng.choice([1, -1]) * MAX * rng.uniform(0.3, 1.0) for _ in range(n)]
    xs = []
    while len(xs) < n:
        x = rng.choice([1, -1]) * MAX * rng.uniform(0.5, 1.0)
        xs += [x, -x * rng.uniform(0.98, 1.0)]
    xs = xs[:n]
    xs.sort(key=lambda v: v < 0)
    return xs


def gen_ties(rng, n):
    base = rand_float(rng, -200, 200)
    ulp = math.ulp(base)
    xs = [base, math.copysign(ulp / 2, rng.choice([1, -1]))]
    while len(xs) < n:
        xs.append(math.ldexp(rng.choice([1, -1]), math.frexp(ulp)[1] - rng.randint(60, 200)))
    return xs


def gen_specials(rng, n):
    pool = [NAN, INF, -INF, NEG_ZERO, 0.0]
    xs = [rand_float(rng, -100, 100) for _ in range(n)]
    for _ in range(rng.randint(1, 3)):
        xs[rng.randrange(n)] = rng.choice(pool)
    return xs


def gen_many_partials(rng, limit):
    """Inputs spaced a few bits apart so the running sum keeps many non-overlapping partials."""
    xs = []
    e = 1000
    while e > -1074 and len(xs) < limit:
        m = rng.getrandbits(53) | (1 << 52) | 1
        xs.append(math.ldexp(m, max(e - 52, -1074)) * rng.choice([1, -1]))
        e -= rng.randint(1, 3)
    rng.shuffle(xs)
    return xs


RANDOM_CLASSES = [
    ("uniform", gen_uniform),
    ("wide", gen_wide),
    ("cancel", gen_cancel),
    ("subnormal", gen_subnormal),
    ("near-overflow", gen_near_overflow),
    ("ties", gen_ties),
    ("specials", gen_specials),
]


def random_cases(rng):
    cases = []
    for name, gen in RANDOM_CLASSES:
        for i in range(140):
            cases.append((f"{name}-{i:03d}", gen(rng, rng.randint(1, 64))))
    for name, gen in RANDOM_CLASSES[:3]:
        for i in range(2):
            cases.append((f"{name}-n1000-{i}", gen(rng, 1000)))
    for i in range(3):
        cases.append((f"many-partials-{i}", gen_many_partials(rng, 1000)))
    return cases


def render(cases):
    lines = [
        "# Generated by scripts/gen_golden.py. Do not edit by hand.",
        "# name<TAB>expected<TAB>inputs (float64 bit patterns, big-endian hex)",
    ]
    for name, xs in cases:
        xs = [canonical(x) for x in xs]
        lines.append(f"{name}\t{bits(expected(xs))}\t{' '.join(bits(x) for x in xs)}")
    return "\n".join(lines) + "\n"


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "testdata/golden.txt"
    rng = random.Random(SEED)
    text = render(hand_cases() + random_cases(rng))
    with open(out, "w", encoding="ascii", newline="\n") as f:
        f.write(text)


if __name__ == "__main__":
    main()
