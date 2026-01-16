package alp

import (
	"math"
)

// Encoder32 compresses float32 values using ALP encoding.
type Encoder32 struct {
	cfg Config

	// Encoding state determined by sampling
	exp    uint8 // Exponent index into float32ExpArr
	factor uint8 // Factor index into float32FactArr
}

// NewEncoder32 creates a new ALP encoder for float32 values.
func NewEncoder32(cfg Config) *Encoder32 {
	return &Encoder32{cfg: cfg}
}

// EncodedBlock32 holds the result of encoding a vector of float32s.
type EncodedBlock32 struct {
	// Encoded integers after ALP transformation
	Encoded []int32
	// Frame-of-Reference base value
	FrameOfRef int32
	// Bit width needed for each value after FOR subtraction
	BitWidth uint8
	// Exception positions
	ExceptionPos []uint16
	// Exception values (original float bits)
	Exceptions []uint32
	// Encoding parameters
	Exp    uint8
	Factor uint8
}

// Sample analyzes the data to find optimal encoding parameters.
func (e *Encoder32) Sample(data []float32) {
	if len(data) == 0 {
		return
	}

	samples := e.sampleData(data)
	if len(samples) == 0 {
		e.exp = 0
		e.factor = 0
		return
	}

	best := e.findBestCombination(samples)
	e.exp = best.exp
	e.factor = best.factor
}

func (e *Encoder32) sampleData(data []float32) []float32 {
	n := len(data)
	if n == 0 {
		return nil
	}

	numVectors := (n + VectorSize - 1) / VectorSize
	if numVectors == 0 {
		return nil
	}

	samplesPerVector := e.cfg.SamplesPerVector
	if samplesPerVector <= 0 {
		samplesPerVector = 32
	}

	samples := make([]float32, 0, numVectors*samplesPerVector)

	for v := 0; v < numVectors; v++ {
		vecStart := v * VectorSize
		vecEnd := vecStart + VectorSize
		if vecEnd > n {
			vecEnd = n
		}
		vecLen := vecEnd - vecStart

		if vecLen < samplesPerVector {
			continue
		}

		step := vecLen / samplesPerVector
		if step == 0 {
			step = 1
		}

		for i := 0; i < samplesPerVector && vecStart+i*step < vecEnd; i++ {
			samples = append(samples, data[vecStart+i*step])
		}
	}

	return samples
}

func (e *Encoder32) findBestCombination(samples []float32) factorExponentPair {
	type combo struct {
		exp        uint8
		factor     uint8
		exceptions int
	}

	var candidates []combo

	for exp := uint8(0); exp <= float32MaxExponent; exp++ {
		for factor := exp; factor <= float32MaxExponent && int(factor) < len(float32FactArr); factor++ {
			exceptions := e.countExceptions(samples, exp, factor)
			candidates = append(candidates, combo{exp, factor, exceptions})
		}
	}

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

func (e *Encoder32) countExceptions(samples []float32, exp, factor uint8) int {
	exceptions := 0
	for _, v := range samples {
		if !e.canEncode(v, exp, factor) {
			exceptions++
			if exceptions > samplingEarlyExitThreshold {
				return exceptions
			}
		}
	}
	return exceptions
}

func (e *Encoder32) canEncode(v float32, exp, factor uint8) bool {
	if isSpecialFloat32(v) {
		return false
	}

	encoded := encodeValue32(v, exp)
	if encoded < math.MinInt32 || encoded > math.MaxInt32 {
		return false
	}

	decoded := decodeValue32(int32(encoded), exp)
	return decoded == v
}

func encodeValue32(v float32, exp uint8) int64 {
	scaled := v * float32ExpArr[exp]
	if scaled >= 0 {
		return int64(scaled + 0.5)
	}
	return int64(scaled - 0.5)
}

func decodeValue32(encoded int32, exp uint8) float32 {
	return float32(encoded) * float32FracArr[exp]
}

// Encode compresses a vector of float32 values.
func (e *Encoder32) Encode(data []float32) *EncodedBlock32 {
	n := len(data)
	if n == 0 {
		return &EncodedBlock32{}
	}

	block := &EncodedBlock32{
		Encoded: make([]int32, n),
		Exp:     e.exp,
		Factor:  e.factor,
	}

	minVal := int32(math.MaxInt32)
	maxVal := int32(math.MinInt32)

	for i, v := range data {
		if isSpecialFloat32(v) || !e.canEncode(v, e.exp, e.factor) {
			block.ExceptionPos = append(block.ExceptionPos, uint16(i))
			block.Exceptions = append(block.Exceptions, math.Float32bits(v))
			block.Encoded[i] = 0
			continue
		}

		encoded := int32(encodeValue32(v, e.exp))
		block.Encoded[i] = encoded

		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	if len(block.ExceptionPos) == n {
		block.FrameOfRef = 0
		block.BitWidth = 0
		return block
	}

	block.FrameOfRef = minVal

	rng := uint32(maxVal - minVal)
	block.BitWidth = uint8(bitsRequired(uint64(rng)))

	isException := make([]bool, n)
	for _, pos := range block.ExceptionPos {
		isException[pos] = true
	}

	for i := range block.Encoded {
		if !isException[i] {
			block.Encoded[i] -= minVal
		}
	}

	return block
}

// Decoder32 decompresses ALP-encoded float32 data.
type Decoder32 struct{}

// NewDecoder32 creates a new ALP decoder for float32.
func NewDecoder32() *Decoder32 {
	return &Decoder32{}
}

// Decode decompresses an encoded block back to float32 values.
func (d *Decoder32) Decode(block *EncodedBlock32) []float32 {
	n := len(block.Encoded)
	if n == 0 {
		return nil
	}

	result := make([]float32, n)

	exceptionIdx := make(map[uint16]int, len(block.ExceptionPos))
	for i, pos := range block.ExceptionPos {
		exceptionIdx[pos] = i
	}

	fracMult := float32FracArr[block.Exp]
	frameOfRef := block.FrameOfRef

	for i := 0; i < n; i++ {
		if excIdx, ok := exceptionIdx[uint16(i)]; ok {
			result[i] = math.Float32frombits(block.Exceptions[excIdx])
			continue
		}

		encoded := block.Encoded[i] + frameOfRef
		result[i] = float32(encoded) * fracMult
	}

	return result
}
