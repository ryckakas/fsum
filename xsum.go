// The superaccumulator in this file is based on xsum by Radford M. Neal (https://gitlab.com/radfordneal/xsum):
// the chunk layout and the carry-free add are his. Carry propagation and rounding are rewritten for Go.
//
// Copyright 2015, 2018, 2021, 2024 Radford M. Neal
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated
// documentation files (the "Software"), to deal in the Software without restriction, including without
// limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the
// Software, and to permit persons to whom the Software is furnished to do so, subject to the following
// conditions:
//
// The above copyright notice and this permission notice shall be included in all copies or substantial portions
// of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED
// TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF
// CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package fsum

import (
	"math"
	"math/bits"
)

const (
	mantissaBits = 52
	mantissaMask = 1<<mantissaBits - 1
	exponentMask = 0x7ff

	chunks    = 67
	lastChunk = chunks - 1
	lowBits   = 32
	lowMask   = 1<<lowBits - 1

	// One add changes a chunk by less than 2^52 and a normalized chunk is below 2^32 in magnitude, so this
	// many adds cannot overflow an int64.
	addsPerNormalize = 2046
)

// small is an exact sum of finite float64 values: the sum of chunk[i] * 2^(32i-1075). Chunks overlap by 32
// bits, so an add touches two of them and never carries. The zero value is an empty sum.
type small struct {
	chunk [chunks]int64
	// used has bit i set when chunk i or i+1 may be non-zero. It lets tiny sums skip the untouched chunks.
	used uint64
	adds int32
}

// put adds the normal float64 with bit pattern b. The caller counts the adds and normalizes in time.
func (a *small) put(b uint64) {
	mantissa := b&mantissaMask | 1<<mantissaBits
	exponent := b >> mantissaBits
	shift := exponent & (lowBits - 1)
	i := exponent >> 5 & 63
	sign := signed(b) >> 63
	low := signed(mantissa << shift & lowMask)
	high := signed(mantissa >> (lowBits - shift))

	a.used |= 1 << i
	a.chunk[i] += (low ^ sign) - sign
	a.chunk[i+1] += (high ^ sign) - sign
}

// putSubnormal is put for a subnormal: no implicit bit, and the exponent of the smallest normal.
func (a *small) putSubnormal(b uint64) {
	mantissa := b & mantissaMask << 1
	sign := signed(b) >> 63
	low := signed(mantissa & lowMask)
	high := signed(mantissa >> lowBits)

	a.used |= 1
	a.chunk[0] += (low ^ sign) - sign
	a.chunk[1] += (high ^ sign) - sign
}

// normalize propagates carries upward and returns the index of the highest non-zero chunk, or -1 for a zero
// sum. Afterwards that chunk carries the sign, in [-2^32, 2^32), and every chunk below it is in [0, 2^32).
// The last chunk has nowhere to carry to: it absorbs, and stays exact far beyond any reachable input.
func (a *small) normalize() int {
	return a.propagate(0)
}

// propagate is normalize when sign is 0. When sign is -1 it negates the sum in the same pass.
func (a *small) propagate(sign int64) int {
	a.adds = 0
	if a.used == 0 {
		return -1
	}
	low := bits.TrailingZeros64(a.used)
	top := a.highest(min(bits.Len64(a.used)+2, lastChunk))
	if top < 0 {
		a.used = 0
		return -1
	}
	end := min(top+1, lastChunk)
	carry := int64(0)
	for i := low; i < end; i++ {
		c := (a.chunk[i] ^ sign) - sign + carry
		a.chunk[i] = c & lowMask
		carry = c >> lowBits
	}
	a.chunk[end] = (a.chunk[end] ^ sign) - sign + carry
	top = a.foldMinusOnes(a.highest(end))
	a.used = usedBetween(low, top)
	return top
}

func (a *small) highest(from int) int {
	for i := from; i >= 0; i-- {
		if a.chunk[i] != 0 {
			return i
		}
	}
	return -1
}

// foldMinusOnes merges a top chunk of -1 into the chunk below, so a small negative sum does not climb one
// chunk higher at every normalize.
func (a *small) foldMinusOnes(top int) int {
	for top > 0 && a.chunk[top] == -1 {
		a.chunk[top] = 0
		top--
		a.chunk[top] -= 1 << lowBits
	}
	return top
}

func usedBetween(low, top int) uint64 {
	if top < 0 {
		return 0
	}
	return ^uint64(0) << low & (^uint64(0) >> (63 - min(top, 63)))
}

// round returns the sum rounded to the nearest float64, ties to even. A negative sum is negated first, so
// only magnitudes are ever rounded and the two signs cannot disagree.
func (a *small) round() float64 {
	top := a.normalize()
	if top < 0 {
		return 0
	}
	if a.chunk[top] > 0 {
		return a.roundMagnitude(top)
	}
	return -a.roundMagnitude(a.propagate(-1))
}

// roundMagnitude rounds a positive, normalized sum whose highest non-zero chunk is top.
func (a *small) roundMagnitude(top int) float64 {
	if top == lastChunk {
		return math.Inf(1)
	}
	// Inputs are multiples of 2^-1074, so a sum below 2^-1021 is a float64 already: no rounding.
	if units := unsigned(a.chunk[1])<<lowBits | unsigned(a.chunk[0]); top <= 1 && units < 1<<(mantissaBits+2) {
		return math.Float64frombits(units >> 1)
	}

	hi := unsigned(a.chunk[top])<<lowBits | unsigned(a.chunk[top-1])
	lo := a.below(top-2)<<lowBits | a.below(top-3)
	shift := bits.LeadingZeros64(hi)
	frac := hi<<shift | lo>>(64-shift)
	sticky := lo<<shift != 0 || a.anyBetween(bits.TrailingZeros64(a.used), top-4)

	mantissa := frac >> 11
	rest := frac & 0x7ff
	const half = 0x400
	if rest > half || rest == half && (sticky || mantissa&1 == 1) {
		mantissa++
	}
	// A mantissa that rounded up to 2^53 carries into the exponent by itself.
	exponent := unsigned(int64(lowBits*(top-1) + 63 - shift - mantissaBits))
	b := exponent<<mantissaBits + (mantissa - 1<<mantissaBits)
	if b >= exponentMask<<mantissaBits {
		return math.Inf(1)
	}
	return math.Float64frombits(b)
}

func (a *small) anyBetween(low, high int) bool {
	for i := low; i <= high; i++ {
		if a.chunk[i] != 0 {
			return true
		}
	}
	return false
}

func (a *small) below(i int) uint64 {
	if i < 0 {
		return 0
	}
	return unsigned(a.chunk[i])
}

// signed and unsigned reinterpret bits. Every use is a field of a float64 or a chunk known to be non-negative.
func signed(u uint64) int64 {
	return int64(u) //nolint:gosec // bit reinterpretation, see above
}

func unsigned(i int64) uint64 {
	return uint64(i) //nolint:gosec // bit reinterpretation, see above
}
