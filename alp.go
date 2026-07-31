// Package alp implements Adaptive Lossless floating-Point compression.
//
// ALP is a lossless compression scheme for floating-point data from
// SIGMOD 2024. It exploits the fact that most real-world floats are
// human-friendly decimals that can be represented as integers scaled by
// powers of 10.
//
// Reference: https://dl.acm.org/doi/10.1145/3626717
package alp

// VectorSize is the number of values per compression block.
// This matches the paper's recommendation for cache efficiency.
const VectorSize = 1024

// State holds the encoding parameters learned from sampling.
// A single State can encode many vectors of similar data. Exp and Factor must
// each be in the range 0..17. Callers should obtain a State from Sample rather
// than setting its representation fields directly.
type State struct {
	Exp    uint8 // Exponent: multiply by 10^Exp before rounding
	Factor uint8 // Factor: divide by 10^Factor after rounding
}

// Vector is a compressed block of VectorSize float64 values. BitWidth must be
// in the range 0..64, Exp and Factor must each be in the range 0..17, and
// NumExceptions must not exceed VectorSize. The first NumExceptions entries in
// ExceptionPos must be strictly increasing, unique positions less than
// VectorSize; their corresponding ExceptionBits entries hold the original
// float64 bits. Callers should construct vectors with Encode or a successful
// Unpack rather than modifying representation fields directly.
type Vector struct {
	// Encoded contains the integers after ALP transformation.
	Encoded [VectorSize]int64

	// FrameOfRef is the minimum encoded value subtracted for bit-packing.
	FrameOfRef int64

	// BitWidth is the number of bits needed to represent max-min.
	BitWidth uint8

	// Exp and Factor are the encoding parameters used for this vector.
	Exp    uint8
	Factor uint8

	// ExceptionPos and ExceptionBits contain the positions and raw float bits
	// for values that do not round-trip through the integer transform. Only the
	// first NumExceptions entries are valid.
	NumExceptions uint16
	ExceptionPos  [VectorSize]uint16
	ExceptionBits [VectorSize]uint64
}

// Sample analyzes data to find encoding parameters.
func Sample(data []float64) State {
	if len(data) == 0 {
		return State{}
	}

	samples := sampleData(data)
	exp, factor := findBestParameters(samples)
	return State{Exp: exp, Factor: factor}
}

// Encoding limits from the ALP paper.
const (
	minEncodable = -9223372036854775807 // -(2^63 - 1)
	maxEncodable = 9223372036854775807  // 2^63 - 1
)

// bitWidth returns the number of bits needed to represent v.
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
		n++
	}
	return n
}
