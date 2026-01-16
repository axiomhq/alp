// Package alp implements Adaptive Lossless floating-Point compression.
//
// ALP is a lossless compression scheme for floating-point data from
// SIGMOD 2024. It exploits the fact that most real-world floats are
// "human-friendly" decimals that can be represented as integers
// scaled by powers of 10.
//
// Reference: https://dl.acm.org/doi/10.1145/3626717
package alp

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

// Encoding limits from the paper
const (
	minEncodable = -9223372036854775807 // -(2^63 - 1)
	maxEncodable = 9223372036854775807  // 2^63 - 1
)

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
