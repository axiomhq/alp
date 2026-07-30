package alp

import (
	"encoding/binary"
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

// decodePackedInto fuses bit unpacking and ALP decoding, avoiding the
// temporary uint64, int64, and float64 slices otherwise created per block.
func (d *Decoder) decodePackedInto(block *PackedBlock, result []float64) int {
	n := min(block.Count, len(result))
	if n == 0 {
		return 0
	}

	frameOfRef := block.FrameOfRef
	factMult := float64(float64FactArr[block.Factor])
	fracMult := float64FracArr[block.Exp]
	width := uint(block.BitWidth)

	switch block.BitWidth {
	case 0:
		decoded := float64(frameOfRef) * factMult * fracMult
		for i := 0; i < n; i++ {
			result[i] = decoded
		}
	case 8:
		for i, value := range block.Data[:n] {
			result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
		}
	case 10:
		if block.Factor == 0 {
			decodePacked10FactorZero(block.Data, result[:n], frameOfRef, fracMult)
		} else {
			decodePacked10(block.Data, result[:n], frameOfRef, factMult, fracMult)
		}
	case 16:
		for i := 0; i < n; i++ {
			value := binary.LittleEndian.Uint16(block.Data[i*2:])
			result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
		}
	case 20:
		decodePacked20(block.Data, result[:n], frameOfRef, factMult, fracMult)
	case 32:
		for i := 0; i < n; i++ {
			value := binary.LittleEndian.Uint32(block.Data[i*4:])
			result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
		}
	case 64:
		for i := 0; i < n; i++ {
			value := binary.LittleEndian.Uint64(block.Data[i*8:])
			result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
		}
	default:
		if width <= 56 {
			mask := (uint64(1) << width) - 1
			bitPos := uint(0)
			for i := 0; i < n; i++ {
				bytePos := int(bitPos >> 3)
				var word uint64
				if bytePos+8 <= len(block.Data) {
					word = binary.LittleEndian.Uint64(block.Data[bytePos:])
				} else {
					for j, value := range block.Data[bytePos:] {
						word |= uint64(value) << (8 * j)
					}
				}
				value := (word >> (bitPos & 7)) & mask
				result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
				bitPos += width
			}
		} else {
			unpacker := BitUnpacker{buf: block.Data}
			for i := 0; i < n; i++ {
				value := unpacker.unpackValue(block.BitWidth)
				result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
			}
		}
	}

	for i, pos := range block.ExceptionPos {
		if int(pos) >= n || i >= len(block.Exceptions) {
			break
		}
		result[pos] = math.Float64frombits(block.Exceptions[i])
	}

	return n
}

func decodePacked10(data []byte, result []float64, frameOfRef int64, factMult, fracMult float64) {
	i := 0
	bytePos := 0
	for ; i+8 <= len(result); i += 8 {
		lo := binary.LittleEndian.Uint64(data[bytePos : bytePos+8])
		hi := uint64(binary.LittleEndian.Uint16(data[bytePos+8 : bytePos+10]))
		result[i] = float64(int64(lo&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+1] = float64(int64((lo>>10)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+2] = float64(int64((lo>>20)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+3] = float64(int64((lo>>30)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+4] = float64(int64((lo>>40)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+5] = float64(int64((lo>>50)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+6] = float64(int64((lo>>60|hi<<4)&0x3ff)+frameOfRef) * factMult * fracMult
		result[i+7] = float64(int64((hi>>6)&0x3ff)+frameOfRef) * factMult * fracMult
		bytePos += 10
	}
	decodePackedRemainder(data[bytePos:], result[i:], 10, frameOfRef, factMult, fracMult)
}

func decodePacked10FactorZero(data []byte, result []float64, frameOfRef int64, fracMult float64) {
	i := 0
	bytePos := 0
	for ; i+8 <= len(result); i += 8 {
		lo := binary.LittleEndian.Uint64(data[bytePos : bytePos+8])
		hi := uint64(binary.LittleEndian.Uint16(data[bytePos+8 : bytePos+10]))
		result[i] = float64(int64(lo&0x3ff)+frameOfRef) * fracMult
		result[i+1] = float64(int64((lo>>10)&0x3ff)+frameOfRef) * fracMult
		result[i+2] = float64(int64((lo>>20)&0x3ff)+frameOfRef) * fracMult
		result[i+3] = float64(int64((lo>>30)&0x3ff)+frameOfRef) * fracMult
		result[i+4] = float64(int64((lo>>40)&0x3ff)+frameOfRef) * fracMult
		result[i+5] = float64(int64((lo>>50)&0x3ff)+frameOfRef) * fracMult
		result[i+6] = float64(int64((lo>>60|hi<<4)&0x3ff)+frameOfRef) * fracMult
		result[i+7] = float64(int64((hi>>6)&0x3ff)+frameOfRef) * fracMult
		bytePos += 10
	}
	decodePackedRemainder(data[bytePos:], result[i:], 10, frameOfRef, 1, fracMult)
}

func decodePacked20(data []byte, result []float64, frameOfRef int64, factMult, fracMult float64) {
	i := 0
	bytePos := 0
	for ; i+4 <= len(result); i += 4 {
		lo := binary.LittleEndian.Uint64(data[bytePos : bytePos+8])
		hi := uint64(binary.LittleEndian.Uint16(data[bytePos+8 : bytePos+10]))
		result[i] = float64(int64(lo&0xfffff)+frameOfRef) * factMult * fracMult
		result[i+1] = float64(int64((lo>>20)&0xfffff)+frameOfRef) * factMult * fracMult
		result[i+2] = float64(int64((lo>>40)&0xfffff)+frameOfRef) * factMult * fracMult
		result[i+3] = float64(int64((lo>>60|hi<<4)&0xfffff)+frameOfRef) * factMult * fracMult
		bytePos += 10
	}
	decodePackedRemainder(data[bytePos:], result[i:], 20, frameOfRef, factMult, fracMult)
}

func decodePackedRemainder(data []byte, result []float64, width uint, frameOfRef int64, factMult, fracMult float64) {
	if len(result) == 0 {
		return
	}
	mask := (uint64(1) << width) - 1
	bitPos := uint(0)
	for i := range result {
		bytePos := int(bitPos >> 3)
		var word uint64
		for j, value := range data[bytePos:] {
			word |= uint64(value) << (8 * j)
		}
		value := (word >> (bitPos & 7)) & mask
		result[i] = float64(int64(value)+frameOfRef) * factMult * fracMult
		bitPos += width
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
