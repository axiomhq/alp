package alp

import (
	"math"
	"sort"
)

// RDEncoder implements ALP Real Doubles encoding for high-precision floats.
// It splits IEEE 754 representation into left and right parts, using
// dictionary encoding for the common left parts.
type RDEncoder struct {
	cfg Config

	// Encoding state
	cuttingPos   uint8            // Position to split left/right (0-63)
	dictionary   []uint64         // Common left parts
	dictLookup   map[uint64]uint8 // Left part -> dictionary index
	leftBitWidth uint8            // Bits needed to encode dictionary index
}

// NewRDEncoder creates a new ALP RD encoder.
func NewRDEncoder(cfg Config) *RDEncoder {
	return &RDEncoder{
		cfg:        cfg,
		dictLookup: make(map[uint64]uint8),
	}
}

// RDEncodedBlock holds the result of RD encoding.
type RDEncodedBlock struct {
	// Right parts (lower bits of each value)
	RightParts []uint64
	// Left parts encoded as dictionary indices
	LeftEncoded []uint8
	// Dictionary of unique left parts
	Dictionary []uint64
	// Cutting position (where to split left/right)
	CuttingPos uint8
	// Bit widths
	LeftBitWidth  uint8
	RightBitWidth uint8
	// Exception positions (values not in dictionary)
	ExceptionPos []uint16
	// Exception left parts (full left values for exceptions)
	Exceptions []uint64
}

// leftPartCount tracks frequency of left parts during sampling.
type leftPartCount struct {
	leftPart uint64
	count    int
}

// Sample analyzes data to find optimal cutting position and dictionary.
func (e *RDEncoder) Sample(data []float64) {
	if len(data) == 0 {
		return
	}

	// Sample data
	samples := e.sampleData(data)
	if len(samples) == 0 {
		samples = data
		if len(samples) > 1024 {
			samples = samples[:1024]
		}
	}

	// Find best cutting position
	bestCuttingPos := uint8(0)
	bestSize := uint64(math.MaxUint64)

	for cuttingPos := uint8(1); cuttingPos <= rdCuttingLimit; cuttingPos++ {
		size := e.estimateSize(samples, cuttingPos)
		if size < bestSize {
			bestSize = size
			bestCuttingPos = cuttingPos
		}
	}

	e.cuttingPos = bestCuttingPos

	// Build dictionary for the chosen cutting position
	e.buildDictionary(samples, bestCuttingPos)
}

// sampleData extracts samples for analysis.
func (e *RDEncoder) sampleData(data []float64) []float64 {
	n := len(data)
	samplesPerVector := e.cfg.SamplesPerVector
	if samplesPerVector <= 0 {
		samplesPerVector = 32
	}

	// Sample at most 1024 values
	maxSamples := 1024
	if n <= maxSamples {
		return append([]float64(nil), data...)
	}

	step := n / maxSamples
	samples := make([]float64, 0, maxSamples)
	for i := 0; i < n && len(samples) < maxSamples; i += step {
		samples = append(samples, data[i])
	}
	return samples
}

// estimateSize estimates compressed size for a given cutting position.
func (e *RDEncoder) estimateSize(samples []float64, cuttingPos uint8) uint64 {
	// Count frequency of left parts
	leftCounts := make(map[uint64]int)
	for _, v := range samples {
		bits := math.Float64bits(v)
		leftPart := bits >> cuttingPos
		leftCounts[leftPart]++
	}

	// Sort by frequency
	sorted := make([]leftPartCount, 0, len(leftCounts))
	for lp, count := range leftCounts {
		sorted = append(sorted, leftPartCount{lp, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Take top dictionary entries
	dictSize := maxRDDictionarySize
	if len(sorted) < dictSize {
		dictSize = len(sorted)
	}

	// Count exceptions (values not in dictionary)
	dictSet := make(map[uint64]bool)
	for i := 0; i < dictSize; i++ {
		dictSet[sorted[i].leftPart] = true
	}

	exceptions := 0
	for _, v := range samples {
		bits := math.Float64bits(v)
		leftPart := bits >> cuttingPos
		if !dictSet[leftPart] {
			exceptions++
		}
	}

	// Estimate size in bits per value
	rightBitWidth := uint64(cuttingPos)
	leftBitWidth := uint64(bitsRequired(uint64(dictSize - 1)))
	if leftBitWidth == 0 {
		leftBitWidth = 1
	}

	// Size = right bits + left bits + exception overhead
	sampleCount := uint64(len(samples))
	exceptionBits := uint64(exceptions) * (64 + 16) // full value + position
	normalBits := sampleCount * (rightBitWidth + leftBitWidth)

	return normalBits + exceptionBits
}

// buildDictionary creates the encoding dictionary for the given cutting position.
func (e *RDEncoder) buildDictionary(samples []float64, cuttingPos uint8) {
	// Count frequency of left parts
	leftCounts := make(map[uint64]int)
	for _, v := range samples {
		bits := math.Float64bits(v)
		leftPart := bits >> cuttingPos
		leftCounts[leftPart]++
	}

	// Sort by frequency (descending)
	sorted := make([]leftPartCount, 0, len(leftCounts))
	for lp, count := range leftCounts {
		sorted = append(sorted, leftPartCount{lp, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Build dictionary from top entries
	dictSize := maxRDDictionarySize
	if len(sorted) < dictSize {
		dictSize = len(sorted)
	}

	e.dictionary = make([]uint64, dictSize)
	e.dictLookup = make(map[uint64]uint8, dictSize)
	for i := 0; i < dictSize; i++ {
		e.dictionary[i] = sorted[i].leftPart
		e.dictLookup[sorted[i].leftPart] = uint8(i)
	}

	e.leftBitWidth = uint8(bitsRequired(uint64(dictSize - 1)))
	if e.leftBitWidth == 0 {
		e.leftBitWidth = 1
	}
}

// Encode compresses a vector of float64 values using RD encoding.
func (e *RDEncoder) Encode(data []float64) *RDEncodedBlock {
	n := len(data)
	if n == 0 {
		return &RDEncodedBlock{}
	}

	block := &RDEncodedBlock{
		RightParts:    make([]uint64, n),
		LeftEncoded:   make([]uint8, n),
		Dictionary:    append([]uint64(nil), e.dictionary...),
		CuttingPos:    e.cuttingPos,
		LeftBitWidth:  e.leftBitWidth,
		RightBitWidth: e.cuttingPos,
	}

	rightMask := uint64((1 << e.cuttingPos) - 1)

	for i, v := range data {
		bits := math.Float64bits(v)
		rightPart := bits & rightMask
		leftPart := bits >> e.cuttingPos

		block.RightParts[i] = rightPart

		// Look up left part in dictionary
		if dictIdx, ok := e.dictLookup[leftPart]; ok {
			block.LeftEncoded[i] = dictIdx
		} else {
			// Exception: left part not in dictionary
			block.ExceptionPos = append(block.ExceptionPos, uint16(i))
			block.Exceptions = append(block.Exceptions, leftPart)
			block.LeftEncoded[i] = 0 // Placeholder
		}
	}

	return block
}

// EncodeVector encodes exactly VectorSize elements.
func (e *RDEncoder) EncodeVector(data []float64) *RDEncodedBlock {
	if len(data) != VectorSize {
		return e.Encode(data)
	}

	block := &RDEncodedBlock{
		RightParts:    make([]uint64, VectorSize),
		LeftEncoded:   make([]uint8, VectorSize),
		Dictionary:    append([]uint64(nil), e.dictionary...),
		CuttingPos:    e.cuttingPos,
		LeftBitWidth:  e.leftBitWidth,
		RightBitWidth: e.cuttingPos,
		ExceptionPos:  make([]uint16, 0, 16),
		Exceptions:    make([]uint64, 0, 16),
	}

	rightMask := uint64((1 << e.cuttingPos) - 1)

	// Process in chunks of 8 for cache efficiency
	for i := 0; i < VectorSize; i += 8 {
		for j := 0; j < 8; j++ {
			idx := i + j
			bits := math.Float64bits(data[idx])

			rightPart := bits & rightMask
			leftPart := bits >> e.cuttingPos

			block.RightParts[idx] = rightPart

			if dictIdx, ok := e.dictLookup[leftPart]; ok {
				block.LeftEncoded[idx] = dictIdx
			} else {
				block.ExceptionPos = append(block.ExceptionPos, uint16(idx))
				block.Exceptions = append(block.Exceptions, leftPart)
				block.LeftEncoded[idx] = 0
			}
		}
	}

	return block
}

// RDDecoder decompresses ALP RD encoded data.
type RDDecoder struct{}

// NewRDDecoder creates a new RD decoder.
func NewRDDecoder() *RDDecoder {
	return &RDDecoder{}
}

// Decode decompresses an RD encoded block.
func (d *RDDecoder) Decode(block *RDEncodedBlock) []float64 {
	n := len(block.RightParts)
	if n == 0 {
		return nil
	}

	result := make([]float64, n)

	// Create exception lookup
	exceptionIdx := make(map[uint16]int, len(block.ExceptionPos))
	for i, pos := range block.ExceptionPos {
		exceptionIdx[pos] = i
	}

	for i := 0; i < n; i++ {
		var leftPart uint64

		// Check if this is an exception
		if excIdx, ok := exceptionIdx[uint16(i)]; ok {
			leftPart = block.Exceptions[excIdx]
		} else {
			// Look up in dictionary
			dictIdx := block.LeftEncoded[i]
			leftPart = block.Dictionary[dictIdx]
		}

		// Reconstruct the original bits
		bits := (leftPart << block.CuttingPos) | block.RightParts[i]
		result[i] = math.Float64frombits(bits)
	}

	return result
}

// DecodeVector decodes exactly VectorSize elements.
func (d *RDDecoder) DecodeVector(block *RDEncodedBlock, result []float64) {
	if len(result) != VectorSize || len(block.RightParts) != VectorSize {
		copy(result, d.Decode(block))
		return
	}

	// Create exception bitmap
	var exceptionBits [VectorSize / 64]uint64
	for _, pos := range block.ExceptionPos {
		exceptionBits[pos/64] |= 1 << (pos % 64)
	}

	exceptionIdx := 0
	cuttingPos := block.CuttingPos

	// Process in chunks of 64
	for chunk := 0; chunk < VectorSize/64; chunk++ {
		bits := exceptionBits[chunk]
		baseIdx := chunk * 64

		if bits == 0 {
			// No exceptions in this chunk - fast path
			for i := 0; i < 64; i++ {
				idx := baseIdx + i
				dictIdx := block.LeftEncoded[idx]
				leftPart := block.Dictionary[dictIdx]
				floatBits := (leftPart << cuttingPos) | block.RightParts[idx]
				result[idx] = math.Float64frombits(floatBits)
			}
		} else {
			// Some exceptions
			for i := 0; i < 64; i++ {
				idx := baseIdx + i
				var leftPart uint64

				if bits&(1<<i) != 0 {
					leftPart = block.Exceptions[exceptionIdx]
					exceptionIdx++
				} else {
					dictIdx := block.LeftEncoded[idx]
					leftPart = block.Dictionary[dictIdx]
				}

				floatBits := (leftPart << cuttingPos) | block.RightParts[idx]
				result[idx] = math.Float64frombits(floatBits)
			}
		}
	}
}

// DecodeSingle decodes a single value at the given index.
func (d *RDDecoder) DecodeSingle(block *RDEncodedBlock, idx int) float64 {
	// Check if this is an exception
	for i, pos := range block.ExceptionPos {
		if int(pos) == idx {
			leftPart := block.Exceptions[i]
			bits := (leftPart << block.CuttingPos) | block.RightParts[idx]
			return math.Float64frombits(bits)
		}
	}

	// Use dictionary
	dictIdx := block.LeftEncoded[idx]
	leftPart := block.Dictionary[dictIdx]
	bits := (leftPart << block.CuttingPos) | block.RightParts[idx]
	return math.Float64frombits(bits)
}
