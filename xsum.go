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
	// used has bit i set when chunk i may be non-zero. Bit 63 stands for chunks 63 and up. It lets sums of a
	// few values skip the chunks nothing touched.
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

	a.used |= 3 << i
	a.chunk[i] += (low ^ sign) - sign
	a.chunk[i+1] += (high ^ sign) - sign
}

// putSubnormal is put for a subnormal: no implicit bit, and the exponent of the smallest normal.
func (a *small) putSubnormal(b uint64) {
	mantissa := b & mantissaMask << 1
	sign := signed(b) >> 63
	low := signed(mantissa & lowMask)
	high := signed(mantissa >> lowBits)

	a.used |= 3
	a.chunk[0] += (low ^ sign) - sign
	a.chunk[1] += (high ^ sign) - sign
}

// normalize propagates carries upward and returns the index of the highest non-zero chunk, or -1 for a zero
// sum. Afterwards every chunk but the last is a signed digit in [-2^31, 2^31), so the highest non-zero chunk
// has the sign of the sum. Signed digits stop a carry at the first empty chunk, where unsigned ones would
// ripple a borrow through every empty chunk between two distant values.
//
// The last chunk has nowhere to carry to: it absorbs, and stays exact far beyond any reachable input.
func (a *small) normalize() int {
	a.adds = 0
	if a.used == 0 {
		return -1
	}
	end := min(bits.Len64(a.used), lastChunk)
	if a.used>>63 != 0 {
		end = lastChunk
	}
	nonZero, carry := uint64(0), int64(0)
	for i := bits.TrailingZeros64(a.used); i < end; {
		c := a.chunk[i] + carry
		digit := lowDigit(c)
		a.chunk[i] = digit
		carry = (c - digit) >> lowBits
		if digit != 0 {
			nonZero |= 1 << min(i, 63)
		}
		i++
		if carry == 0 {
			i = a.nextUsed(i, end)
		}
	}
	a.chunk[end] += carry
	if a.chunk[end] != 0 {
		nonZero |= 1 << min(end, 63)
	}
	a.used = nonZero
	return a.highestUpTo(lastChunk)
}

// nextUsed returns the first index from i on whose chunk may be non-zero, at most end.
func (a *small) nextUsed(i, end int) int {
	if i >= 63 {
		return min(i, end)
	}
	rest := a.used >> i
	if rest == 0 {
		return end
	}
	return min(i+bits.TrailingZeros64(rest), end)
}

// highestUpTo returns the index of the highest non-zero chunk at or below i, or -1. It needs the bitmap
// normalize leaves, which is exact for the chunks below 63.
func (a *small) highestUpTo(i int) int {
	if i < 0 {
		return -1
	}
	for ; i > 63; i-- {
		if a.chunk[i] != 0 {
			return i
		}
	}
	return bits.Len64(a.used&(^uint64(0)>>(63-i))) - 1
}

// round returns the sum rounded to the nearest float64, ties to even. Only the magnitude is rounded, so the
// two signs cannot disagree. It normalizes a.
func (a *small) round() float64 {
	top := a.normalize()
	if top < 0 {
		return 0
	}
	sign := int64(1)
	if a.chunk[top] < 0 {
		sign = -1
	}
	if top == lastChunk {
		return math.Inf(int(sign))
	}
	return float64(sign) * a.roundMagnitude(top, sign)
}

// window holds the five highest base-2^32 digits of the magnitude, unsigned, and whether anything below them
// is non-zero.
type window struct {
	digit  [5]uint64
	sticky bool
}

// magnitudeWindow converts the signed digits from top-4 to top into unsigned digits of sign * sum. What lies
// below matters only through its sign: a negative remainder borrows one from the lowest digit.
func (a *small) magnitudeWindow(top int, sign int64) window {
	var w window
	borrow := int64(0)
	if below := a.highestUpTo(top - 5); below >= 0 {
		w.sticky = true
		if sign*a.chunk[below] < 0 {
			borrow = 1
		}
	}
	for k := range w.digit {
		d := -borrow
		if i := top - 4 + k; i >= 0 {
			d += sign * a.chunk[i]
		}
		borrow = 0
		if d < 0 {
			d += 1 << lowBits
			borrow = 1
		}
		w.digit[k] = unsigned(d)
	}
	return w
}

// roundMagnitude rounds sign * sum, which is positive, where top is the highest non-zero chunk.
func (a *small) roundMagnitude(top int, sign int64) float64 {
	w := a.magnitudeWindow(top, sign)
	hi, lo, sticky := w.digit[4]<<lowBits|w.digit[3], w.digit[2]<<lowBits|w.digit[1], w.sticky || w.digit[0] != 0
	// A borrow can empty the top digit: the magnitude then starts one chunk lower.
	if w.digit[4] == 0 {
		top--
		hi, lo, sticky = w.digit[3]<<lowBits|w.digit[2], w.digit[1]<<lowBits|w.digit[0], w.sticky
	}
	// Inputs are multiples of 2^-1074, so a sum below 2^-1021 is a float64 already: no rounding.
	if top <= 1 {
		if units := hi >> (lowBits * (1 - top)); units < 1<<(mantissaBits+2) {
			return math.Float64frombits(units >> 1)
		}
	}

	shift := bits.LeadingZeros64(hi)
	frac := hi<<shift | lo>>(64-shift)
	sticky = sticky || lo<<shift != 0

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

// lowDigit returns the low 32 bits of c as a signed digit.
func lowDigit(c int64) int64 {
	return int64(int32(c)) //nolint:gosec // truncation is the point
}

// signed and unsigned reinterpret bits. Every use is a field of a float64 or a value known to be non-negative.
func signed(u uint64) int64 {
	return int64(u) //nolint:gosec // bit reinterpretation, see above
}

func unsigned(i int64) uint64 {
	return uint64(i) //nolint:gosec // bit reinterpretation, see above
}
