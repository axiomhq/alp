// Package alp implements Adaptive Lossless floating-Point compression.
//
// ALP is a state-of-the-art lossless compression algorithm for IEEE 754
// floating-point data. It exploits the fact that most real-world floats
// represent decimal values (like 3.14, 10.5, etc.) that can be exactly
// represented as integers multiplied by powers of ten.
//
// The algorithm works by:
//  1. Finding optimal exponent E and factor F to convert floats to integers
//  2. Encoding: encoded = round(value × 10^E)
//  3. Using Frame-of-Reference encoding with bit-packing
//  4. Handling exceptions for values that don't roundtrip correctly
//
// For high-precision floats that don't fit this pattern, ALP RD (Real Doubles)
// splits the IEEE 754 representation and uses dictionary encoding.
//
// Reference: "ALP: Adaptive Lossless floating-Point Compression"
// Afroozeh, Kuffó, Boncz - ACM SIGMOD 2024
package alp

// Scheme indicates which compression scheme to use.
type Scheme uint8

const (
	SchemeInvalid Scheme = iota
	SchemeALP            // Standard ALP for decimal floats
	SchemeALPRD          // ALP Real Doubles for high-precision floats
)

// VectorSize is the number of elements processed in one vector.
// Chosen to be SIMD-friendly (1024 = 16 × 64).
const VectorSize = 1024

// Config holds sampling and encoding parameters.
type Config struct {
	// RowGroupSize is the number of values in a row group.
	RowGroupSize int
	// SamplesPerVector is how many samples to take per vector.
	SamplesPerVector int
	// TopKCombinations is how many factor/exponent pairs to evaluate.
	TopKCombinations int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		RowGroupSize:     100 * VectorSize,
		SamplesPerVector: 32,
		TopKCombinations: 5,
	}
}
