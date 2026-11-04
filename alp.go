package alp

import "math/bits"

// VectorSize is the number of values per compression block; it is part of the wire format.
const VectorSize = 1024

// State holds the encoding parameters learned from sampling.
type State struct {
	Exp    uint8 // Exponent: multiply by 10^Exp before rounding
	Factor uint8 // Factor: divide by 10^Factor after rounding
}

// Vector is a compressed block of VectorSize float64 values.
type Vector struct {
	// Encoded contains the integers after ALP transformation.
	Encoded [VectorSize]int64

	// FrameOfRef is the minimum value (or delta, in delta mode) subtracted for bit-packing.
	FrameOfRef int64

	// Delta mode stores Base as the first value and later slots as differences.
	Delta bool
	Base  int64

	// BitWidth is the number of bits needed to represent max-min.
	BitWidth uint8

	// Exp and Factor are the encoding parameters used for this vector.
	Exp    uint8
	Factor uint8

	// ExceptionPos and ExceptionBits hold values that do not round-trip; the first NumExceptions are valid.
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
	return uint8(bits.Len64(v))
}
