// Package alp implements Adaptive Lossless floating-Point compression.
//
// ALP is a lossless compression scheme for floating-point data from
// SIGMOD 2024. It exploits the fact that most real-world floats are
// "human-friendly" decimals that can be represented as integers
// scaled by powers of 10.
//
// Reference: https://dl.acm.org/doi/10.1145/3626717
package alp

import "math"

// VectorSize is the number of values per compression block.
// This matches the paper's recommendation for cache efficiency.
const VectorSize = 1024

// State holds the encoding parameters learned from sampling.
// A single State can encode many vectors of similar data.
type State struct {
	Exp    uint8 // Exponent: multiply by 10^Exp before rounding
	Factor uint8 // Factor: divide by 10^Factor after rounding
}

// Vector is a compressed block of VectorSize float64 values.
type Vector struct {
	// Encoded integers after ALP transformation
	Encoded [VectorSize]int64

	// Frame-of-reference: minimum encoded value (subtracted for bit-packing)
	FrameOfRef int64

	// BitWidth needed to represent (max - min) of encoded values
	BitWidth uint8

	// Encoding parameters used
	Exp    uint8
	Factor uint8

	// Exceptions: values that don't roundtrip correctly
	NumExceptions uint16
	ExceptionPos  [VectorSize]uint16 // positions (only first NumExceptions valid)
	ExceptionBits [VectorSize]uint64 // raw float bits (only first NumExceptions valid)
}

// Sample analyzes data to find optimal encoding parameters.
// This implements the paper's two-level sampling strategy.
func Sample(data []float64) State {
	if len(data) == 0 {
		return State{}
	}

	samples := sampleData(data)
	exp, factor := findBestParameters(samples)
	return State{Exp: exp, Factor: factor}
}

// Encode compresses a vector of float64 values using the given state.
// The input must have exactly VectorSize elements.
func Encode(dst *Vector, src []float64, s State) {
	if len(src) != VectorSize {
		panic("alp: input must have exactly VectorSize elements")
	}

	dst.Exp = s.Exp
	dst.Factor = s.Factor
	dst.NumExceptions = 0

	encodeMult := expArr[s.Exp] * fracArr[s.Factor]
	factMult := float64(factArr[s.Factor])
	fracMult := fracArr[s.Exp]

	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)

	for i := 0; i < VectorSize; i++ {
		v := src[i]
		bits := math.Float64bits(v)

		// Check for special values (NaN, Inf, -0)
		if isSpecial(bits) {
			dst.ExceptionPos[dst.NumExceptions] = uint16(i)
			dst.ExceptionBits[dst.NumExceptions] = bits
			dst.NumExceptions++
			dst.Encoded[i] = 0
			continue
		}

		// Encode: round(value × 10^exp × 10^-factor)
		encoded := roundToInt64(v * encodeMult)

		// Check roundtrip
		decoded := float64(encoded) * factMult * fracMult
		if math.Float64bits(decoded) != bits || encoded < minEncodable || encoded > maxEncodable {
			dst.ExceptionPos[dst.NumExceptions] = uint16(i)
			dst.ExceptionBits[dst.NumExceptions] = bits
			dst.NumExceptions++
			dst.Encoded[i] = 0
			continue
		}

		dst.Encoded[i] = encoded
		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	// Handle all-exceptions case
	if minVal > maxVal {
		minVal, maxVal = 0, 0
	}

	dst.FrameOfRef = minVal
	dst.BitWidth = bitWidth(uint64(maxVal - minVal))

	// Apply frame-of-reference
	for i := 0; i < VectorSize; i++ {
		dst.Encoded[i] -= minVal
	}
}

// Decode decompresses a vector back to float64 values.
// The output slice must have exactly VectorSize elements.
func Decode(dst []float64, src *Vector) {
	if len(dst) != VectorSize {
		panic("alp: output must have exactly VectorSize elements")
	}

	factMult := float64(factArr[src.Factor])
	fracMult := fracArr[src.Exp]
	frameOfRef := src.FrameOfRef

	// Build exception bitmap for O(1) lookup
	var excBitmap [VectorSize / 64]uint64
	for i := uint16(0); i < src.NumExceptions; i++ {
		pos := src.ExceptionPos[i]
		excBitmap[pos/64] |= 1 << (pos % 64)
	}

	excIdx := uint16(0)
	for chunk := 0; chunk < VectorSize/64; chunk++ {
		bits := excBitmap[chunk]
		base := chunk * 64

		if bits == 0 {
			// Fast path: no exceptions in this chunk
			for i := 0; i < 64; i++ {
				encoded := src.Encoded[base+i] + frameOfRef
				dst[base+i] = float64(encoded) * factMult * fracMult
			}
		} else {
			// Slow path: check each position
			for i := 0; i < 64; i++ {
				if bits&(1<<i) != 0 {
					dst[base+i] = math.Float64frombits(src.ExceptionBits[excIdx])
					excIdx++
				} else {
					encoded := src.Encoded[base+i] + frameOfRef
					dst[base+i] = float64(encoded) * factMult * fracMult
				}
			}
		}
	}
}

// Encoding limits from the paper
const (
	minEncodable = -9223372036854775807 // -(2^63 - 1)
	maxEncodable = 9223372036854775807  // 2^63 - 1
)

// isSpecial returns true for NaN, Inf, or negative zero
func isSpecial(bits uint64) bool {
	exp := bits & 0x7FF0000000000000
	return exp == 0x7FF0000000000000 || bits == 0x8000000000000000
}

// roundToInt64 rounds a float64 to the nearest int64
func roundToInt64(v float64) int64 {
	if v >= 0 {
		return int64(v + 0.5)
	}
	return int64(v - 0.5)
}

// bitWidth returns the number of bits needed to represent v
func bitWidth(v uint64) uint8 {
	if v == 0 {
		return 0
	}
	return uint8(64 - leadingZeros64(v))
}

func leadingZeros64(x uint64) int {
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
