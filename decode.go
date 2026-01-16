package alp

import (
	"math"
)

// Decoder decompresses ALP-encoded data.
type Decoder struct{}

// NewDecoder creates a new ALP decoder.
func NewDecoder() *Decoder {
	return &Decoder{}
}

// Decode decompresses an encoded block back to float64 values.
// Reference formula: decoded = encoded × FACT_ARR[factor] × FRAC_ARR[exp]
func (d *Decoder) Decode(block *EncodedBlock) []float64 {
	n := len(block.Encoded)
	if n == 0 {
		return nil
	}

	result := make([]float64, n)

	// Create exception lookup for O(1) check
	exceptionIdx := make(map[uint16]int, len(block.ExceptionPos))
	for i, pos := range block.ExceptionPos {
		exceptionIdx[pos] = i
	}

	frameOfRef := block.FrameOfRef
	exp := block.Exp
	factor := block.Factor

	// Decode each value
	for i := 0; i < n; i++ {
		// Check if this position is an exception
		if excIdx, ok := exceptionIdx[uint16(i)]; ok {
			result[i] = math.Float64frombits(block.Exceptions[excIdx])
			continue
		}

		// Add back frame of reference and decode using canonical decodeValue
		encoded := block.Encoded[i] + frameOfRef
		result[i] = decodeValue(encoded, exp, factor)
	}

	return result
}

// DecodeVector decodes exactly VectorSize elements.
// This is optimized for the common case of full vectors.
func (d *Decoder) DecodeVector(block *EncodedBlock, result []float64) {
	if len(result) != VectorSize || len(block.Encoded) != VectorSize {
		copy(result, d.Decode(block))
		return
	}

	// Create exception bitmap for fast lookup
	var exceptionBits [VectorSize / 64]uint64
	for _, pos := range block.ExceptionPos {
		exceptionBits[pos/64] |= 1 << (pos % 64)
	}

	// Build exception value lookup
	exceptionVals := make([]uint64, len(block.ExceptionPos))
	copy(exceptionVals, block.Exceptions)
	exceptionIdx := 0

	frameOfRef := block.FrameOfRef
	exp := block.Exp
	factor := block.Factor

	// Process in chunks of 64 for cache efficiency
	for chunk := 0; chunk < VectorSize/64; chunk++ {
		bits := exceptionBits[chunk]
		baseIdx := chunk * 64

		if bits == 0 {
			// No exceptions in this chunk - fast path
			for i := 0; i < 64; i++ {
				idx := baseIdx + i
				encoded := block.Encoded[idx] + frameOfRef
				result[idx] = decodeValue(encoded, exp, factor)
			}
		} else {
			// Some exceptions - need to check each position
			for i := 0; i < 64; i++ {
				idx := baseIdx + i
				if bits&(1<<i) != 0 {
					// This is an exception
					result[idx] = math.Float64frombits(exceptionVals[exceptionIdx])
					exceptionIdx++
				} else {
					encoded := block.Encoded[idx] + frameOfRef
					result[idx] = decodeValue(encoded, exp, factor)
				}
			}
		}
	}
}

// DecodeInPlace decodes values directly into the encoded slice, converting int64 to float64.
// The caller must ensure the underlying memory can hold float64 values.
// This is useful when you want to avoid allocation.
func (d *Decoder) DecodeInPlace(block *EncodedBlock, result []float64) {
	n := len(block.Encoded)
	if n == 0 || len(result) < n {
		return
	}

	// Sort exception positions for sequential access
	excIdx := 0
	nextExcPos := int(-1)
	if len(block.ExceptionPos) > 0 {
		nextExcPos = int(block.ExceptionPos[0])
	}

	frameOfRef := block.FrameOfRef
	exp := block.Exp
	factor := block.Factor

	for i := 0; i < n; i++ {
		if i == nextExcPos {
			// Handle exception
			result[i] = math.Float64frombits(block.Exceptions[excIdx])
			excIdx++
			if excIdx < len(block.ExceptionPos) {
				nextExcPos = int(block.ExceptionPos[excIdx])
			} else {
				nextExcPos = -1
			}
		} else {
			// Regular decode using canonical decodeValue
			encoded := block.Encoded[i] + frameOfRef
			result[i] = decodeValue(encoded, exp, factor)
		}
	}
}

// DecodeSingle decodes a single value at the given index.
func (d *Decoder) DecodeSingle(block *EncodedBlock, idx int) float64 {
	// Check if this is an exception
	for i, pos := range block.ExceptionPos {
		if int(pos) == idx {
			return math.Float64frombits(block.Exceptions[i])
		}
	}

	// Regular decode using canonical decodeValue
	encoded := block.Encoded[idx] + block.FrameOfRef
	return decodeValue(encoded, block.Exp, block.Factor)
}
