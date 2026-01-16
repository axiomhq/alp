package alp

import "math"

// Encoding limits for int64 representation.
// These are slightly less than math.MaxInt64/MinInt64 to prevent overflow.
const (
	encodingUpperLimit int64 = 9223372036854774784
	encodingLowerLimit int64 = -9223372036854774784
)

// samplingEarlyExitThreshold is the max exceptions allowed during sampling
// before trying the next factor/exponent combination.
const samplingEarlyExitThreshold = 2

// Double-precision constants.
const (
	float64MaxExponent = 18 // Maximum exponent we try for doubles
	float64MaxFactor   = 23 // Maximum factor index
)

// float64ExpArr contains powers of 10 from 10^0 to 10^23.
// Used for multiplying values during encoding.
var float64ExpArr = [24]float64{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7,
	1e8, 1e9, 1e10, 1e11, 1e12, 1e13, 1e14, 1e15,
	1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22, 1e23,
}

// float64FracArr contains powers of 0.1 from 10^0 to 10^-20.
// Used for dividing values during decoding.
// Using multiplication by these values is faster than division.
var float64FracArr = [21]float64{
	1e0, 1e-1, 1e-2, 1e-3, 1e-4, 1e-5, 1e-6, 1e-7,
	1e-8, 1e-9, 1e-10, 1e-11, 1e-12, 1e-13, 1e-14, 1e-15,
	1e-16, 1e-17, 1e-18, 1e-19, 1e-20,
}

// float64FactArr contains integer powers of 10 from 10^0 to 10^18.
// Used for factor multiplication.
var float64FactArr = [19]uint64{
	1, 10, 100, 1000, 10000, 100000, 1000000, 10000000,
	100000000, 1000000000, 10000000000, 100000000000,
	1000000000000, 10000000000000, 100000000000000,
	1000000000000000, 10000000000000000, 100000000000000000,
	1000000000000000000,
}

// Single-precision constants.
const (
	float32MaxExponent = 10 // Maximum exponent we try for floats
	float32MaxFactor   = 10 // Maximum factor index
)

// float32ExpArr contains powers of 10 from 10^0 to 10^10.
var float32ExpArr = [11]float32{
	1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10,
}

// float32FracArr contains powers of 0.1 from 10^0 to 10^-10.
var float32FracArr = [11]float32{
	1e0, 1e-1, 1e-2, 1e-3, 1e-4, 1e-5, 1e-6, 1e-7, 1e-8, 1e-9, 1e-10,
}

// float32FactArr contains integer powers of 10 from 10^0 to 10^9.
var float32FactArr = [10]uint32{
	1, 10, 100, 1000, 10000, 100000, 1000000, 10000000,
	100000000, 1000000000,
}

// IEEE 754 bit manipulation constants for float64.
const (
	float64SignMask     = uint64(1) << 63
	float64ExponentMask = uint64(0x7FF) << 52
	float64MantissaMask = (uint64(1) << 52) - 1
	float64InfBits      = float64ExponentMask
	float64NegZeroBits  = float64SignMask
)

// IEEE 754 bit manipulation constants for float32.
const (
	float32SignMask     = uint32(1) << 31
	float32ExponentMask = uint32(0xFF) << 23
	float32MantissaMask = (uint32(1) << 23) - 1
	float32InfBits      = float32ExponentMask
	float32NegZeroBits  = float32SignMask
)

// RD (Real Doubles) constants.
const (
	maxRDDictionarySize = 8   // Maximum dictionary entries for RD encoding
	rdCuttingLimit      = 16  // Maximum cutting position to try
	rdExceptionSize     = 2   // Bytes per exception position
)

// bitsRequired returns the minimum bits needed to represent n.
// This is essentially ceil(log2(n+1)) but handles edge cases.
func bitsRequired(n uint64) int {
	if n == 0 {
		return 0
	}
	return 64 - leadingZeros64(n)
}

// leadingZeros64 returns the number of leading zero bits in x.
// Uses a de Bruijn sequence for efficiency (Lemire-style optimization).
func leadingZeros64(x uint64) int {
	if x == 0 {
		return 64
	}
	n := 0
	if x <= 0x00000000FFFFFFFF {
		n += 32
		x <<= 32
	}
	if x <= 0x0000FFFFFFFFFFFF {
		n += 16
		x <<= 16
	}
	if x <= 0x00FFFFFFFFFFFFFF {
		n += 8
		x <<= 8
	}
	if x <= 0x0FFFFFFFFFFFFFFF {
		n += 4
		x <<= 4
	}
	if x <= 0x3FFFFFFFFFFFFFFF {
		n += 2
		x <<= 2
	}
	if x <= 0x7FFFFFFFFFFFFFFF {
		n += 1
	}
	return n
}

// trailingZeros64 returns the number of trailing zero bits in x.
func trailingZeros64(x uint64) int {
	if x == 0 {
		return 64
	}
	n := 0
	if x&0xFFFFFFFF == 0 {
		n += 32
		x >>= 32
	}
	if x&0xFFFF == 0 {
		n += 16
		x >>= 16
	}
	if x&0xFF == 0 {
		n += 8
		x >>= 8
	}
	if x&0xF == 0 {
		n += 4
		x >>= 4
	}
	if x&0x3 == 0 {
		n += 2
		x >>= 2
	}
	if x&0x1 == 0 {
		n += 1
	}
	return n
}

// isSpecialFloat64 returns true if v is NaN, Inf, or -0.
func isSpecialFloat64(v float64) bool {
	bits := math.Float64bits(v)
	// Check for infinity or NaN (all exponent bits set)
	if bits&float64ExponentMask == float64ExponentMask {
		return true
	}
	// Check for negative zero
	if bits == float64NegZeroBits {
		return true
	}
	return false
}

// isSpecialFloat32 returns true if v is NaN, Inf, or -0.
func isSpecialFloat32(v float32) bool {
	bits := math.Float32bits(v)
	// Check for infinity or NaN (all exponent bits set)
	if bits&float32ExponentMask == float32ExponentMask {
		return true
	}
	// Check for negative zero
	if bits == float32NegZeroBits {
		return true
	}
	return false
}
