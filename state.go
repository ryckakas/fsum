// This file is a Go translation, with Math.sumPrecise semantics, of the polyfill in the TC39 proposal
// (https://github.com/tc39/proposal-math-sum), which adapts the fsum loop of CPython's Modules/mathmodule.c.
//
// CPython: Copyright (c) 2001 Python Software Foundation; All Rights Reserved. Used under the PSF License
// Agreement, https://docs.python.org/3/license.html
//
// The polyfill: BSD 3-Clause License
//
// Copyright (c) 2024 Kevin Gibbons
//
// Redistribution and use in source and binary forms, with or without modification, are permitted provided that
// the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of conditions and the
// following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the
// following disclaimer in the documentation and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its contributors may be used to endorse or
// promote products derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED
// WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A
// PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY
// DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR
// OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH
// DAMAGE.

package fsum

import "math"

const (
	two1023 = 0x1p1023
	// maxULP is the gap between math.MaxFloat64 and the next float64 below it.
	maxULP = 0x1p971
)

// state is Shewchuk's exact running sum with TC39 Math.sumPrecise overflow handling: the sum is
// overflow*2^1024 plus the partials (non-overlapping, increasing magnitude). The partials travel by value
// because a slice stored behind a pointer escapes, which would put Sum's stack buffer on the heap.
type state struct {
	overflow   int64
	special    float64
	hasSpecial bool
	// sawNonNegZero is false while every input so far was -0, which is when the sum itself is -0.
	sawNonNegZero bool
}

func (s *state) add(partials []float64, x float64) []float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		s.addSpecial(x)
		return partials
	}
	if x != 0 || !math.Signbit(x) {
		s.sawNonNegZero = true
	}
	if s.hasSpecial || x == 0 {
		return partials
	}
	return s.addFinite(partials, x)
}

func (s *state) addSpecial(x float64) {
	switch {
	case !s.hasSpecial:
		s.special, s.hasSpecial = x, true
	case s.special != x:
		s.special = math.NaN()
	}
}

func (s *state) addFinite(partials []float64, x float64) []float64 {
	n := 0
	for _, y := range partials {
		if math.Abs(x) < math.Abs(y) {
			x, y = y, x
		}
		hi, lo := twoSum(x, y)
		if math.IsInf(hi, 0) {
			sign := math.Copysign(1, hi)
			s.overflow += int64(sign)
			x = (x - sign*two1023) - sign*two1023
			if math.Abs(x) < math.Abs(y) {
				x, y = y, x
			}
			hi, lo = twoSum(x, y)
		}
		if lo != 0 {
			partials[n] = lo
			n++
		}
		x = hi
	}
	partials = partials[:n]
	if x != 0 {
		partials = append(partials, x)
	}
	return partials
}

func overflowSign(overflow int64) int {
	if overflow > 0 {
		return 1
	}
	return -1
}

// twoSum returns the rounded sum and its exact rounding error. It needs x == 0 or |x| >= |y|.
func twoSum(x, y float64) (hi, lo float64) {
	hi = x + y
	lo = y - (hi - x)
	return hi, lo
}

func (s *state) sum(partials []float64) float64 {
	switch {
	case s.hasSpecial:
		if math.IsNaN(s.special) {
			return math.NaN()
		}
		return s.special
	case !s.sawNonNegZero:
		return math.Copysign(0, -1)
	case s.overflow != 0:
		return s.sumWithOverflow(partials)
	}
	return roundPartials(0, partials)
}

// sumWithOverflow folds the overflow count into the largest partial. It works on halved values so the
// arithmetic itself cannot overflow.
func (s *state) sumWithOverflow(partials []float64) float64 {
	rest := partials
	top := 0.0
	if len(rest) > 0 {
		top, rest = rest[len(rest)-1], rest[:len(rest)-1]
	}
	if s.overflow > 1 || s.overflow < -1 || (top != 0 && (s.overflow > 0) == (top > 0)) {
		return math.Inf(overflowSign(s.overflow))
	}
	hi, lo := twoSum(float64(s.overflow)*two1023, top/2)
	lo *= 2
	if math.IsInf(2*hi, 0) {
		return roundAtMax(hi, lo, rest)
	}
	hi *= 2
	if lo == 0 {
		return roundPartials(hi, rest)
	}
	hi, lo = twoSum(hi, lo)
	if lo == 0 {
		return roundPartials(hi, rest)
	}
	return roundHalfway(hi, lo, rest)
}

// roundAtMax resolves a sum within half an ULP of 2^1024. Exactly half an ULP below rounds to even, which is
// infinity, unless a smaller partial pulls the sum back below the tie.
func roundAtMax(hi, lo float64, rest []float64) float64 {
	sign := math.Copysign(1, hi)
	if hi == sign*two1023 && lo == -sign*maxULP/2 && len(rest) > 0 && math.Signbit(rest[len(rest)-1]) != math.Signbit(sign) {
		return sign * math.MaxFloat64
	}
	return math.Inf(int(sign))
}

// roundPartials adds partials onto hi from the largest down, stopping at the first inexact addition.
func roundPartials(hi float64, partials []float64) float64 {
	lo := 0.0
	for len(partials) > 0 {
		y := partials[len(partials)-1]
		partials = partials[:len(partials)-1]
		hi, lo = twoSum(hi, y)
		if lo != 0 {
			break
		}
	}
	return roundHalfway(hi, lo, partials)
}

// roundHalfway corrects hi when its rounding error lo is exactly half an ULP: round-half-even may have
// rounded the wrong way if the next smaller partial has the same sign as lo.
func roundHalfway(hi, lo float64, partials []float64) float64 {
	if len(partials) == 0 || lo == 0 || math.Signbit(lo) != math.Signbit(partials[len(partials)-1]) {
		return hi
	}
	y := lo * 2
	x := hi + y
	if x-hi == y {
		return x
	}
	return hi
}
