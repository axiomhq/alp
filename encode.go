package alp

import (
	"math"
)

// Encoder compresses float64 values using ALP encoding.
type Encoder struct {
	cfg Config

	// Encoding state determined by sampling
	exp    uint8 // Exponent index into float64ExpArr
	factor uint8 // Factor index into float64FactArr
	scheme Scheme
}

// NewEncoder creates a new ALP encoder with the given configuration.
func NewEncoder(cfg Config) *Encoder {
	return &Encoder{cfg: cfg}
}

// EncodedBlock holds the result of encoding a vector of floats.
type EncodedBlock struct {
	// Encoded integers after ALP transformation
	Encoded []int64
	// Frame-of-Reference base value
	FrameOfRef int64
	// Bit width needed for each value after FOR subtraction
	BitWidth uint8
	// Exception positions (indices of values that didn't roundtrip)
	ExceptionPos []uint16
	// Exception values (original float bits for exceptions)
	Exceptions []uint64
	// Encoding parameters
	Exp    uint8
	Factor uint8
}

// factorExponentPair represents a candidate encoding configuration.
type factorExponentPair struct {
	exp        uint8
	factor     uint8
	exceptions int
}

// Sample analyzes the data to find optimal encoding parameters.
// This should be called once per row group before encoding vectors.
func (e *Encoder) Sample(data []float64) {
	if len(data) == 0 {
		e.scheme = SchemeInvalid
		return
	}

	// Sample data equidistantly
	samples := e.sampleData(data)
	if len(samples) == 0 {
		e.scheme = SchemeALP
		e.exp = 0
		e.factor = 0
		return
	}

	// Find best factor/exponent combinations
	best := e.findBestCombination(samples)
	e.exp = best.exp
	e.factor = best.factor
	e.scheme = SchemeALP
}

// sampleData extracts equidistant samples from the data.
func (e *Encoder) sampleData(data []float64) []float64 {
	n := len(data)
	if n == 0 {
		return nil
	}

	samplesPerVector := e.cfg.SamplesPerVector
	if samplesPerVector <= 0 {
		samplesPerVector = 32
	}

	// For small datasets, use all values as samples
	if n <= samplesPerVector {
		return append([]float64(nil), data...)
	}

	// Calculate how many vectors we have
	numVectors := (n + VectorSize - 1) / VectorSize
	if numVectors == 0 {
		return append([]float64(nil), data...)
	}

	samples := make([]float64, 0, numVectors*samplesPerVector)

	// Sample equidistantly from each vector
	for v := 0; v < numVectors; v++ {
		vecStart := v * VectorSize
		vecEnd := vecStart + VectorSize
		if vecEnd > n {
			vecEnd = n
		}
		vecLen := vecEnd - vecStart

		// For partial vectors, sample proportionally
		samplesToTake := samplesPerVector
		if vecLen < VectorSize {
			samplesToTake = vecLen
			if samplesToTake > samplesPerVector {
				samplesToTake = samplesPerVector
			}
		}

		// Sample equidistantly within this vector
		step := vecLen / samplesToTake
		if step == 0 {
			step = 1
		}

		for i := 0; i < samplesToTake && vecStart+i*step < vecEnd; i++ {
			samples = append(samples, data[vecStart+i*step])
		}
	}

	if len(samples) == 0 {
		return append([]float64(nil), data...)
	}

	return samples
}

// findBestCombination evaluates factor/exponent pairs to find the best one.
func (e *Encoder) findBestCombination(samples []float64) factorExponentPair {
	// Track exception counts for each combination
	type combo struct {
		exp        uint8
		factor     uint8
		exceptions int
	}

	var candidates []combo

	// Try all valid factor/exponent combinations
	// Factor must be >= Exponent for valid encoding
	for exp := uint8(0); exp <= float64MaxExponent; exp++ {
		for factor := exp; factor <= float64MaxExponent && int(factor) < len(float64FactArr); factor++ {
			exceptions := e.countExceptions(samples, exp, factor)
			candidates = append(candidates, combo{exp, factor, exceptions})
		}
	}

	// Find the combination with fewest exceptions
	best := combo{0, 0, len(samples)}
	for _, c := range candidates {
		if c.exceptions < best.exceptions {
			best = c
		}
	}

	return factorExponentPair{
		exp:        best.exp,
		factor:     best.factor,
		exceptions: best.exceptions,
	}
}

// countExceptions counts how many samples don't roundtrip correctly.
func (e *Encoder) countExceptions(samples []float64, exp, factor uint8) int {
	exceptions := 0
	for _, v := range samples {
		if !e.canEncode(v, exp, factor) {
			exceptions++
			// Early exit if too many exceptions
			if exceptions > samplingEarlyExitThreshold {
				return exceptions
			}
		}
	}
	return exceptions
}

// canEncode checks if a value can be encoded without loss.
func (e *Encoder) canEncode(v float64, exp, factor uint8) bool {
	if isSpecialFloat64(v) {
		return false
	}

	// Encode the value
	encoded := encodeValue(v, exp, factor)
	if encoded < encodingLowerLimit || encoded > encodingUpperLimit {
		return false
	}

	// Decode and check if we get the original back
	decoded := decodeValue(encoded, exp, factor)
	return decoded == v
}

// encodeValue converts a float64 to an int64 using the given parameters.
// encoded = round(value * 10^exp)
func encodeValue(v float64, exp, factor uint8) int64 {
	// Multiply by power of 10 and round to nearest integer
	scaled := v * float64ExpArr[exp]

	// Fast rounding using magic number technique (Lemire-style)
	// Adding 0.5 and truncating is equivalent to rounding for positive numbers
	// For negative numbers, we subtract 0.5 instead
	if scaled >= 0 {
		return int64(scaled + 0.5)
	}
	return int64(scaled - 0.5)
}

// decodeValue converts an int64 back to a float64.
// decoded = encoded * 10^(-exp) * factor
func decodeValue(encoded int64, exp, factor uint8) float64 {
	return float64(encoded) * float64FracArr[exp]
}

// Encode compresses a vector of float64 values.
func (e *Encoder) Encode(data []float64) *EncodedBlock {
	n := len(data)
	if n == 0 {
		return &EncodedBlock{}
	}

	block := &EncodedBlock{
		Encoded: make([]int64, n),
		Exp:     e.exp,
		Factor:  e.factor,
	}

	// First pass: encode all values and identify exceptions
	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)

	for i, v := range data {
		if isSpecialFloat64(v) || !e.canEncode(v, e.exp, e.factor) {
			// Mark as exception
			block.ExceptionPos = append(block.ExceptionPos, uint16(i))
			block.Exceptions = append(block.Exceptions, math.Float64bits(v))
			block.Encoded[i] = 0 // Placeholder
			continue
		}

		encoded := encodeValue(v, e.exp, e.factor)
		block.Encoded[i] = encoded

		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	// Handle edge case where all values are exceptions
	if len(block.ExceptionPos) == n {
		block.FrameOfRef = 0
		block.BitWidth = 0
		return block
	}

	// Set frame of reference to minimum value
	block.FrameOfRef = minVal

	// Compute range and bit width
	rng := uint64(maxVal - minVal)
	block.BitWidth = uint8(bitsRequired(rng))

	// Subtract frame of reference from all non-exception values
	for i := range block.Encoded {
		// Skip exceptions (they have placeholder values)
		isException := false
		for _, pos := range block.ExceptionPos {
			if int(pos) == i {
				isException = true
				break
			}
		}
		if !isException {
			block.Encoded[i] -= minVal
		}
	}

	return block
}

// EncodeVector encodes a vector of exactly VectorSize elements.
// This is optimized for the common case of full vectors.
func (e *Encoder) EncodeVector(data []float64) *EncodedBlock {
	if len(data) != VectorSize {
		return e.Encode(data)
	}

	block := &EncodedBlock{
		Encoded:      make([]int64, VectorSize),
		ExceptionPos: make([]uint16, 0, 8), // Preallocate for typical case
		Exceptions:   make([]uint64, 0, 8),
		Exp:          e.exp,
		Factor:       e.factor,
	}

	// Vectorized encoding pass
	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)
	expMult := float64ExpArr[e.exp]
	fracMult := float64FracArr[e.exp]

	// Process in cache-friendly chunks of 8
	for i := 0; i < VectorSize; i += 8 {
		for j := 0; j < 8; j++ {
			idx := i + j
			v := data[idx]

			// Fast path: check if value is special
			bits := math.Float64bits(v)
			if bits&float64ExponentMask == float64ExponentMask || bits == float64NegZeroBits {
				block.ExceptionPos = append(block.ExceptionPos, uint16(idx))
				block.Exceptions = append(block.Exceptions, bits)
				block.Encoded[idx] = 0
				continue
			}

			// Encode
			scaled := v * expMult
			var encoded int64
			if scaled >= 0 {
				encoded = int64(scaled + 0.5)
			} else {
				encoded = int64(scaled - 0.5)
			}

			// Verify roundtrip
			decoded := float64(encoded) * fracMult
			if decoded != v || encoded < encodingLowerLimit || encoded > encodingUpperLimit {
				block.ExceptionPos = append(block.ExceptionPos, uint16(idx))
				block.Exceptions = append(block.Exceptions, bits)
				block.Encoded[idx] = 0
				continue
			}

			block.Encoded[idx] = encoded
			if encoded < minVal {
				minVal = encoded
			}
			if encoded > maxVal {
				maxVal = encoded
			}
		}
	}

	// Handle all-exceptions case
	if len(block.ExceptionPos) == VectorSize {
		block.FrameOfRef = 0
		block.BitWidth = 0
		return block
	}

	block.FrameOfRef = minVal
	rng := uint64(maxVal - minVal)
	block.BitWidth = uint8(bitsRequired(rng))

	// Create exception lookup for O(1) check
	isException := make([]bool, VectorSize)
	for _, pos := range block.ExceptionPos {
		isException[pos] = true
	}

	// Subtract frame of reference
	for i := 0; i < VectorSize; i++ {
		if !isException[i] {
			block.Encoded[i] -= minVal
		}
	}

	return block
}
