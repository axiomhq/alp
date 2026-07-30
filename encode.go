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
// It considers both exception count and estimated bit width.
// Reference: iterates exp from MAX to 0, factor from exp down to 0.
func (e *Encoder) findBestCombination(samples []float64) factorExponentPair {
	type combo struct {
		exp        uint8
		factor     uint8
		exceptions int
		bitWidth   int // estimated bit width
		score      int // lower is better
	}

	var best combo
	found := false

	// Try all valid (exp, factor) combinations where factor <= exp
	// This matches the reference implementation's search space
	for exp := uint8(0); exp <= float64MaxExponent; exp++ {
		for factor := uint8(0); factor <= exp && int(factor) < len(float64FactArr); factor++ {
			scoreLimit := -1
			if found {
				scoreLimit = best.score
			}
			exceptions, minVal, maxVal, complete := e.analyzeWithExpFactorBounded(
				samples, exp, factor, scoreLimit, true,
			)
			if !complete {
				continue
			}

			// Skip if too many exceptions
			if exceptions > len(samples)/2 {
				continue
			}

			// Estimate bit width from value range
			var bitWidth int
			if maxVal > minVal {
				rng := uint64(maxVal - minVal)
				bitWidth = bitsRequired(rng)
			}

			// Score: prioritize fewer exceptions, then lower bit width
			// Each exception adds ~10 bytes overhead, each bit adds ~n/8 bytes
			score := exceptions*80 + bitWidth*len(samples)/8

			candidate := combo{
				exp:        exp,
				factor:     factor,
				exceptions: exceptions,
				bitWidth:   bitWidth,
				score:      score,
			}
			if !found || candidate.score < best.score {
				best = candidate
				found = true
			}
		}
	}

	if !found {
		return factorExponentPair{0, 0, len(samples)}
	}

	return factorExponentPair{
		exp:        best.exp,
		factor:     best.factor,
		exceptions: best.exceptions,
	}
}

// analyzeWithExpFactor counts exceptions and finds min/max encoded values.
func (e *Encoder) analyzeWithExpFactor(samples []float64, exp, factor uint8) (exceptions int, minVal, maxVal int64) {
	exceptions, minVal, maxVal, _ = e.analyzeWithExpFactorBounded(samples, exp, factor, -1, false)
	return exceptions, minVal, maxVal
}

// analyzeWithExpFactorBounded stops once a candidate cannot beat scoreLimit.
// Exception count and encoded range are monotonic, so the partial score is a
// lower bound on the candidate's final score.
func (e *Encoder) analyzeWithExpFactorBounded(samples []float64, exp, factor uint8, scoreLimit int, rejectMajority bool) (exceptions int, minVal, maxVal int64, complete bool) {
	minVal = int64(1<<62 - 1)
	maxVal = int64(-1 << 62)
	expMult := float64ExpArr[exp]
	factorFrac := float64FracArr[factor]
	factMult := float64(float64FactArr[factor])
	expFrac := float64FracArr[exp]

	for i, v := range samples {
		bits := math.Float64bits(v)
		if bits&float64ExponentMask == float64ExponentMask || bits == float64NegZeroBits {
			exceptions++
		} else {
			scaled := v * expMult * factorFrac
			var encoded int64
			if scaled >= 0 {
				encoded = int64(scaled + 0.5)
			} else {
				encoded = int64(scaled - 0.5)
			}
			decoded := float64(encoded) * factMult * expFrac
			if encoded < encodingLowerLimit || encoded > encodingUpperLimit ||
				math.Float64bits(decoded) != bits {
				exceptions++
			} else {
				if encoded < minVal {
					minVal = encoded
				}
				if encoded > maxVal {
					maxVal = encoded
				}
			}
		}

		if rejectMajority && exceptions > len(samples)/2 {
			return exceptions, minVal, maxVal, false
		}
		if scoreLimit >= 0 && i&3 == 3 {
			lowerBound := exceptions * 80
			if maxVal > minVal {
				lowerBound += bitsRequired(uint64(maxVal-minVal)) * len(samples) / 8
			}
			if lowerBound >= scoreLimit {
				return exceptions, minVal, maxVal, false
			}
		}
	}

	return exceptions, minVal, maxVal, true
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

	// Decode and check if we get the original back (bit-exact comparison)
	decoded := decodeValue(encoded, exp, factor)
	return math.Float64bits(decoded) == math.Float64bits(v)
}

// encodeValue converts a float64 to an int64 using the given parameters.
// Reference formula: encoded = round(value × 10^exp × 10^(-factor)) = round(value × 10^(exp-factor))
func encodeValue(v float64, exp, factor uint8) int64 {
	// Multiply by 10^exp and divide by 10^factor (multiply by 10^-factor)
	scaled := v * float64ExpArr[exp] * float64FracArr[factor]

	// Fast rounding using magic number technique (Lemire-style)
	// Adding 0.5 and truncating is equivalent to rounding for positive numbers
	// For negative numbers, we subtract 0.5 instead
	if scaled >= 0 {
		return int64(scaled + 0.5)
	}
	return int64(scaled - 0.5)
}

// decodeValue converts an int64 back to a float64.
// Reference formula: decoded = encoded × 10^factor × 10^(-exp) = encoded × 10^(factor-exp)
func decodeValue(encoded int64, exp, factor uint8) float64 {
	return float64(encoded) * float64(float64FactArr[factor]) * float64FracArr[exp]
}

// Encode compresses a vector of float64 values.
func (e *Encoder) Encode(data []float64) *EncodedBlock {
	block := new(EncodedBlock)
	e.encodeTo(data, make([]int64, len(data)), block)
	return block
}

func (e *Encoder) encodeTo(data []float64, encoded []int64, block *EncodedBlock) {
	n := len(data)
	encoded = encoded[:n]
	var exceptionPos []uint16
	var exceptions []uint64
	if n == 0 {
		*block = EncodedBlock{}
		return
	}

	// First pass: encode all values and identify exceptions
	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)
	encodeMult := float64ExpArr[e.exp] * float64FracArr[e.factor]
	factMult := float64(float64FactArr[e.factor])
	fracMult := float64FracArr[e.exp]

	for i, v := range data {
		bits := math.Float64bits(v)
		scaled := v * encodeMult
		var value int64
		if scaled >= 0 {
			value = int64(scaled + 0.5)
		} else {
			value = int64(scaled - 0.5)
		}

		decoded := float64(value) * factMult * fracMult
		if math.Float64bits(decoded) != bits || value < encodingLowerLimit || value > encodingUpperLimit {
			exceptionPos = append(exceptionPos, uint16(i))
			exceptions = append(exceptions, bits)
			encoded[i] = 0
			continue
		}

		encoded[i] = value
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}

	// Handle edge case where all values are exceptions
	if len(exceptionPos) == n {
		*block = EncodedBlock{
			Encoded:      encoded,
			ExceptionPos: exceptionPos,
			Exceptions:   exceptions,
			Exp:          e.exp,
			Factor:       e.factor,
		}
		return
	}

	// Compute range and bit width
	rng := uint64(maxVal - minVal)
	bitWidth := uint8(bitsRequired(rng))

	// Subtract the frame in one linear pass, then restore exception slots to
	// their canonical zero placeholder.
	for i := range encoded {
		encoded[i] -= minVal
	}
	for _, pos := range exceptionPos {
		encoded[pos] = 0
	}

	*block = EncodedBlock{
		Encoded:      encoded,
		FrameOfRef:   minVal,
		BitWidth:     bitWidth,
		ExceptionPos: exceptionPos,
		Exceptions:   exceptions,
		Exp:          e.exp,
		Factor:       e.factor,
	}
}

// EncodeVector encodes a vector of exactly VectorSize elements.
// This is optimized for the common case of full vectors.
func (e *Encoder) EncodeVector(data []float64) *EncodedBlock {
	if len(data) != VectorSize {
		return e.Encode(data)
	}
	return e.encodeVectorInto(data, make([]int64, VectorSize))
}

// encodeVectorInto encodes a full vector using caller-owned scratch storage.
// The returned block is valid until encoded is reused.
func (e *Encoder) encodeVectorInto(data []float64, encoded []int64) *EncodedBlock {
	return e.encodeVector(data, encoded, true)
}

// encodeVectorAbsoluteInto leaves encoded values before frame subtraction so
// the compressor can fuse that operation into bit packing.
func (e *Encoder) encodeVectorAbsoluteInto(data []float64, encoded []int64) *EncodedBlock {
	return e.encodeVector(data, encoded, false)
}

func (e *Encoder) encodeVector(data []float64, encoded []int64, applyFrame bool) *EncodedBlock {
	block := new(EncodedBlock)
	e.encodeVectorTo(data, encoded, applyFrame, block)
	return block
}

func (e *Encoder) encodeVectorTo(data []float64, encoded []int64, applyFrame bool, block *EncodedBlock) {
	data = data[:VectorSize]
	encoded = encoded[:VectorSize]
	var exceptionPos []uint16
	var exceptions []uint64

	// Vectorized encoding pass
	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)

	// Pre-compute encode multiplier
	// Encode: value × EXP_ARR[exp] × FRAC_ARR[factor]
	encodeMult := float64ExpArr[e.exp] * float64FracArr[e.factor]
	factMult := float64(float64FactArr[e.factor])
	fracMult := float64FracArr[e.exp]

	for idx, v := range data {
		bits := math.Float64bits(v)
		scaled := v * encodeMult
		var value int64
		if scaled >= 0 {
			value = int64(scaled + 0.5)
		} else {
			value = int64(scaled - 0.5)
		}

		decoded := float64(value) * factMult * fracMult
		if math.Float64bits(decoded) != bits || value < encodingLowerLimit || value > encodingUpperLimit {
			if exceptionPos == nil {
				exceptionPos = make([]uint16, 0, 8)
				exceptions = make([]uint64, 0, 8)
			}
			exceptionPos = append(exceptionPos, uint16(idx))
			exceptions = append(exceptions, bits)
			encoded[idx] = 0
			continue
		}

		encoded[idx] = value
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}

	// Handle all-exceptions case
	if len(exceptionPos) == VectorSize {
		*block = EncodedBlock{
			Encoded:      encoded,
			ExceptionPos: exceptionPos,
			Exceptions:   exceptions,
			Exp:          e.exp,
			Factor:       e.factor,
		}
		return
	}

	rng := uint64(maxVal - minVal)
	bitWidth := uint8(bitsRequired(rng))

	if applyFrame {
		// Subtract frame of reference. Exception slots are reset afterwards;
		// their packed values are ignored by the decoder.
		for i := 0; i < VectorSize; i++ {
			encoded[i] -= minVal
		}
		for _, pos := range exceptionPos {
			encoded[pos] = 0
		}
	}

	*block = EncodedBlock{
		Encoded:      encoded,
		FrameOfRef:   minVal,
		BitWidth:     bitWidth,
		ExceptionPos: exceptionPos,
		Exceptions:   exceptions,
		Exp:          e.exp,
		Factor:       e.factor,
	}
}
