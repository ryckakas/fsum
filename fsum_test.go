package fsum_test

import (
	"bufio"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ryckakas/fsum"
)

type goldenCase struct {
	name string
	want float64
	xs   []float64
}

func parseBits(t *testing.T, hex string) float64 {
	t.Helper()
	u, err := strconv.ParseUint(hex, 16, 64)
	if err != nil {
		t.Fatalf("bad bit pattern %q: %v", hex, err)
	}
	return math.Float64frombits(u)
}

func loadGolden(t *testing.T) []goldenCase {
	t.Helper()
	f, err := os.Open("testdata/golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var cases []goldenCase
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("malformed corpus line: %q", line)
		}
		c := goldenCase{name: fields[0], want: parseBits(t, fields[1])}
		for _, hex := range strings.Fields(fields[2]) {
			c.xs = append(c.xs, parseBits(t, hex))
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

// entryPoints sums xs through every public API, in the given order and reversed. The Accumulator reads its
// sum halfway through, which must not disturb the final result.
func entryPoints(xs []float64) map[string]float64 {
	var acc fsum.Accumulator
	for i, x := range xs {
		if i == len(xs)/2 {
			acc.Sum()
		}
		acc.Add(x)
	}
	reversed := slices.Clone(xs)
	slices.Reverse(reversed)
	return map[string]float64{
		"Sum":             fsum.Sum(xs),
		"SumSeq":          fsum.SumSeq(slices.Values(xs)),
		"Accumulator.Sum": acc.Sum(),
		"Sum(reversed)":   fsum.Sum(reversed),
	}
}

func checkAll(t *testing.T, xs []float64, want float64) {
	t.Helper()
	for api, got := range entryPoints(xs) {
		if !sameFloat(got, want) {
			t.Errorf("%s = %v (%016x), want %v (%016x)", api, got, math.Float64bits(got), want, math.Float64bits(want))
		}
	}
}

func TestGolden(t *testing.T) {
	cases := loadGolden(t)
	if len(cases) < 1000 {
		t.Fatalf("corpus has %d cases, want at least 1000", len(cases))
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkAll(t, c.xs, c.want)
		})
	}
}

func TestGoldenAgreesWithOracle(t *testing.T) {
	for _, c := range loadGolden(t) {
		if got := oracleSum(c.xs); !sameFloat(got, c.want) {
			t.Errorf("%s: oracle %016x, corpus %016x", c.name, math.Float64bits(got), math.Float64bits(c.want))
		}
	}
}

func randomFinite(r *rand.Rand, minExp, maxExp int) float64 {
	mantissa := float64(r.Uint64N(1<<52) | 1<<52)
	x := math.Ldexp(mantissa, minExp+r.IntN(maxExp-minExp+1)-52)
	if math.IsInf(x, 0) {
		x = math.MaxFloat64
	}
	if r.IntN(2) == 0 {
		return -x
	}
	return x
}

func TestLargeInputsAgreeWithOracle(t *testing.T) {
	r := rand.New(rand.NewPCG(2026, 11))
	for _, n := range []int{1, 10, 1000, 100_000} {
		for _, exp := range [][2]int{{-1074, 1023}, {-30, 30}, {1000, 1023}} {
			xs := make([]float64, n)
			for i := range xs {
				xs[i] = randomFinite(r, exp[0], exp[1])
			}
			checkAll(t, xs, oracleSum(xs))
		}
	}
}

func TestNaNIsCanonical(t *testing.T) {
	payload := math.Float64frombits(0x7ff0_0000_0000_0001)
	for api, got := range entryPoints([]float64{1, payload}) {
		if bits := math.Float64bits(got); bits != math.Float64bits(math.NaN()) {
			t.Errorf("%s = %016x, want math.NaN() bits", api, bits)
		}
	}
}

func TestNamedFloatType(t *testing.T) {
	type meters float64
	tenths := slices.Repeat([]meters{0.1}, 10)
	want := oracleSum(slices.Repeat([]float64{0.1}, 10))
	if got := float64(fsum.Sum(tenths)); !sameFloat(got, want) {
		t.Errorf("Sum = %v, want %v", got, want)
	}
	if got := float64(fsum.SumSeq(slices.Values(tenths))); !sameFloat(got, want) {
		t.Errorf("SumSeq = %v, want %v", got, want)
	}
}

func TestAccumulatorZeroValueIsEmptySum(t *testing.T) {
	var acc fsum.Accumulator
	if got, want := acc.Sum(), oracleSum(nil); !sameFloat(got, want) {
		t.Errorf("zero value Sum = %v, want %v", got, want)
	}
}

func TestAccumulatorSumIsRepeatableAndAddContinues(t *testing.T) {
	xs := []float64{math.MaxFloat64, math.Ldexp(1, 970), -math.SmallestNonzeroFloat64, 1e16, -math.MaxFloat64}
	var acc fsum.Accumulator
	for i, x := range xs {
		acc.Add(x)
		want := oracleSum(xs[:i+1])
		first, second := acc.Sum(), acc.Sum()
		if !sameFloat(first, want) || !sameFloat(second, want) {
			t.Fatalf("after %d values: Sum twice = %v, %v; want %v both times", i+1, first, second, want)
		}
	}
}

func TestAccumulatorReset(t *testing.T) {
	var acc fsum.Accumulator
	acc.Add(math.Inf(1))
	acc.Add(3)
	acc.Reset()
	if got, want := acc.Sum(), oracleSum(nil); !sameFloat(got, want) {
		t.Fatalf("after Reset: Sum = %v, want %v", got, want)
	}
	half := 0.5
	acc.Add(half)
	if got, want := acc.Sum(), oracleSum([]float64{half}); !sameFloat(got, want) {
		t.Errorf("reused after Reset: Sum = %v, want %v", got, want)
	}
}

func TestSumDoesNotAllocate(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	xs := make([]float64, 1000)
	for i := range xs {
		xs[i] = r.Float64()
	}
	if allocs := testing.AllocsPerRun(100, func() { fsum.Sum(xs) }); allocs != 0 {
		t.Errorf("Sum allocates %v times per call, want 0", allocs)
	}
}

func encode(xs ...float64) []byte {
	b := make([]byte, 0, 8*len(xs))
	for _, x := range xs {
		b = binary.LittleEndian.AppendUint64(b, math.Float64bits(x))
	}
	return b
}

func FuzzSum(f *testing.F) {
	f.Add(encode())
	f.Add(encode(math.Copysign(0, -1)))
	f.Add(encode(math.NaN()))
	f.Add(encode(math.Inf(1), math.Inf(-1)))
	f.Add(encode(math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64))
	f.Add(encode(math.MaxFloat64, math.Ldexp(1, 970), -math.SmallestNonzeroFloat64))
	f.Add(encode(1e16, 1, 1e-16))
	f.Add(encode(1, math.Ldexp(1, -53), math.Ldexp(1, -105)))
	f.Add(encode(0.1, 0.2, 0.3))

	f.Fuzz(func(t *testing.T, data []byte) {
		xs := make([]float64, len(data)/8)
		for i := range xs {
			xs[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[8*i:]))
		}
		checkAll(t, xs, oracleSum(xs))
	})
}
