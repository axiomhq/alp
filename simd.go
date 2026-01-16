package alp

import (
	"math"
	"unsafe"
)

// Optimized decoding functions using patterns that help Go compiler vectorize.
// These are used internally when conditions allow for faster execution.

// decodeChunkNoExceptions decodes a contiguous chunk with no exceptions.
// This is the hot path that benefits most from vectorization.
// The Go compiler can auto-vectorize this simple loop pattern.
// Note: uses same multiplication order as decodeValue for bit-exact results.
//
//go:noinline
func decodeChunkNoExceptions(encoded []int64, result []float64, frameOfRef int64, factMult float64, fracMult float64) {
	// Hint to compiler: we know these are the same length
	if len(encoded) != len(result) {
		return
	}
	n := len(encoded)

	// Simple loop that compiler can vectorize
	// Key: no bounds checks, no branches, contiguous access
	// Multiplication order matches decodeValue: (encoded * factMult) * fracMult
	for i := 0; i < n; i++ {
		result[i] = float64(encoded[i]+frameOfRef) * factMult * fracMult
	}
}

// decodeChunk8 decodes exactly 8 values - optimized for common unroll factor.
// Manual unrolling helps when auto-vectorization doesn't kick in.
// Note: uses same multiplication order as decodeValue for bit-exact results.
//
//go:noinline
func decodeChunk8(encoded *[8]int64, result *[8]float64, frameOfRef int64, factMult float64, fracMult float64) {
	result[0] = float64(encoded[0]+frameOfRef) * factMult * fracMult
	result[1] = float64(encoded[1]+frameOfRef) * factMult * fracMult
	result[2] = float64(encoded[2]+frameOfRef) * factMult * fracMult
	result[3] = float64(encoded[3]+frameOfRef) * factMult * fracMult
	result[4] = float64(encoded[4]+frameOfRef) * factMult * fracMult
	result[5] = float64(encoded[5]+frameOfRef) * factMult * fracMult
	result[6] = float64(encoded[6]+frameOfRef) * factMult * fracMult
	result[7] = float64(encoded[7]+frameOfRef) * factMult * fracMult
}

// DecodeVectorFast is an optimized decoder for full vectors with few exceptions.
// It processes data in large chunks and only falls back to scalar for exceptions.
func (d *Decoder) DecodeVectorFast(block *EncodedBlock, result []float64) {
	if len(result) < VectorSize || len(block.Encoded) != VectorSize {
		d.DecodeVector(block, result)
		return
	}

	// Use separate multipliers to match decodeValue computation order
	factMult := float64(float64FactArr[block.Factor])
	fracMult := float64FracArr[block.Exp]
	frameOfRef := block.FrameOfRef

	// Fast path: no exceptions at all
	if len(block.ExceptionPos) == 0 {
		decodeChunkNoExceptions(block.Encoded, result[:VectorSize], frameOfRef, factMult, fracMult)
		return
	}

	// Create exception bitmap
	var exceptionBits [VectorSize / 64]uint64
	for _, pos := range block.ExceptionPos {
		exceptionBits[pos/64] |= 1 << (pos % 64)
	}

	// Process in 64-element chunks
	excIdx := 0
	for chunk := 0; chunk < VectorSize/64; chunk++ {
		bits := exceptionBits[chunk]
		start := chunk * 64

		if bits == 0 {
			// No exceptions - use vectorizable path
			decodeChunkNoExceptions(
				block.Encoded[start:start+64],
				result[start:start+64],
				frameOfRef,
				factMult,
				fracMult,
			)
		} else {
			// Has exceptions - decode with patching
			// First decode all values
			decodeChunkNoExceptions(
				block.Encoded[start:start+64],
				result[start:start+64],
				frameOfRef,
				factMult,
				fracMult,
			)
			// Then patch exceptions
			for i := 0; i < 64; i++ {
				if bits&(1<<i) != 0 {
					result[start+i] = math.Float64frombits(block.Exceptions[excIdx])
					excIdx++
				}
			}
		}
	}
}

// Vectorized bit-packing helpers

// packBits8x8 packs 8 values of 8 bits each into 8 bytes.
// Optimized for the common case of 8-bit values.
func packBits8x8(dst []byte, src []uint64) {
	_ = dst[7] // bounds check elimination
	_ = src[7]
	dst[0] = byte(src[0])
	dst[1] = byte(src[1])
	dst[2] = byte(src[2])
	dst[3] = byte(src[3])
	dst[4] = byte(src[4])
	dst[5] = byte(src[5])
	dst[6] = byte(src[6])
	dst[7] = byte(src[7])
}

// unpackBits8x8 unpacks 8 bytes into 8 uint64 values.
func unpackBits8x8(dst []uint64, src []byte) {
	_ = dst[7] // bounds check elimination
	_ = src[7]
	dst[0] = uint64(src[0])
	dst[1] = uint64(src[1])
	dst[2] = uint64(src[2])
	dst[3] = uint64(src[3])
	dst[4] = uint64(src[4])
	dst[5] = uint64(src[5])
	dst[6] = uint64(src[6])
	dst[7] = uint64(src[7])
}

// packBits16x4 packs 4 values of 16 bits each.
func packBits16x4(dst []byte, src []uint64) {
	_ = dst[7] // bounds check elimination
	_ = src[3]
	// Use little-endian byte order
	dst[0] = byte(src[0])
	dst[1] = byte(src[0] >> 8)
	dst[2] = byte(src[1])
	dst[3] = byte(src[1] >> 8)
	dst[4] = byte(src[2])
	dst[5] = byte(src[2] >> 8)
	dst[6] = byte(src[3])
	dst[7] = byte(src[3] >> 8)
}

// unpackBits16x4 unpacks 8 bytes into 4 uint64 values (16 bits each).
func unpackBits16x4(dst []uint64, src []byte) {
	_ = dst[3]
	_ = src[7]
	dst[0] = uint64(src[0]) | uint64(src[1])<<8
	dst[1] = uint64(src[2]) | uint64(src[3])<<8
	dst[2] = uint64(src[4]) | uint64(src[5])<<8
	dst[3] = uint64(src[6]) | uint64(src[7])<<8
}

// packBits32x2 packs 2 values of 32 bits each.
func packBits32x2(dst []byte, src []uint64) {
	_ = dst[7]
	_ = src[1]
	dst[0] = byte(src[0])
	dst[1] = byte(src[0] >> 8)
	dst[2] = byte(src[0] >> 16)
	dst[3] = byte(src[0] >> 24)
	dst[4] = byte(src[1])
	dst[5] = byte(src[1] >> 8)
	dst[6] = byte(src[1] >> 16)
	dst[7] = byte(src[1] >> 24)
}

// unpackBits32x2 unpacks 8 bytes into 2 uint64 values (32 bits each).
func unpackBits32x2(dst []uint64, src []byte) {
	_ = dst[1]
	_ = src[7]
	dst[0] = uint64(src[0]) | uint64(src[1])<<8 | uint64(src[2])<<16 | uint64(src[3])<<24
	dst[1] = uint64(src[4]) | uint64(src[5])<<8 | uint64(src[6])<<16 | uint64(src[7])<<24
}

// Optimized FOR (Frame-of-Reference) encoding

// forEncode subtracts the minimum value from all elements.
// Simple loop that compiler can vectorize.
func forEncode(data []int64, minVal int64) {
	for i := range data {
		data[i] -= minVal
	}
}

// forDecode adds the frame of reference back to all elements.
func forDecode(data []int64, frameOfRef int64) {
	for i := range data {
		data[i] += frameOfRef
	}
}

// findMinMax finds minimum and maximum in a slice.
// Unrolled for better performance.
func findMinMax(data []int64) (min, max int64) {
	if len(data) == 0 {
		return 0, 0
	}

	min = data[0]
	max = data[0]

	// Process 4 at a time
	n := len(data)
	i := 1
	for ; i+3 < n; i += 4 {
		v0, v1, v2, v3 := data[i], data[i+1], data[i+2], data[i+3]

		if v0 < min {
			min = v0
		}
		if v0 > max {
			max = v0
		}
		if v1 < min {
			min = v1
		}
		if v1 > max {
			max = v1
		}
		if v2 < min {
			min = v2
		}
		if v2 > max {
			max = v2
		}
		if v3 < min {
			min = v3
		}
		if v3 > max {
			max = v3
		}
	}

	// Handle remainder
	for ; i < n; i++ {
		if data[i] < min {
			min = data[i]
		}
		if data[i] > max {
			max = data[i]
		}
	}

	return min, max
}

// Parallel exception detection using bitmask

// detectExceptions marks positions where roundtrip fails.
// Returns a bitmap and count of exceptions.
// Uses bit-exact comparison to match canEncode behavior.
func detectExceptions(data []float64, encodeMult float64, factMult float64, fracMult float64) ([]uint64, int) {
	n := len(data)
	numWords := (n + 63) / 64
	bitmap := make([]uint64, numWords)
	count := 0

	for i := 0; i < n; i++ {
		v := data[i]

		// Check for special values
		bits := math.Float64bits(v)
		if bits&float64ExponentMask == float64ExponentMask || bits == float64NegZeroBits {
			bitmap[i/64] |= 1 << (i % 64)
			count++
			continue
		}

		// Check roundtrip
		scaled := v * encodeMult
		var encoded int64
		if scaled >= 0 {
			encoded = int64(scaled + 0.5)
		} else {
			encoded = int64(scaled - 0.5)
		}

		if encoded < encodingLowerLimit || encoded > encodingUpperLimit {
			bitmap[i/64] |= 1 << (i % 64)
			count++
			continue
		}

		// Use same multiplication order as decodeValue for bit-exact comparison
		decoded := float64(encoded) * factMult * fracMult
		if math.Float64bits(decoded) != bits {
			bitmap[i/64] |= 1 << (i % 64)
			count++
		}
	}

	return bitmap, count
}

// popcount64 returns the number of set bits in x.
// Uses the parallel bit counting algorithm.
func popcount64(x uint64) int {
	// Parallel bit count (SWAR algorithm)
	x = x - ((x >> 1) & 0x5555555555555555)
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	return int((x * 0x0101010101010101) >> 56)
}

// countSetBits counts total set bits across a bitmap.
func countSetBits(bitmap []uint64) int {
	count := 0
	for _, word := range bitmap {
		count += popcount64(word)
	}
	return count
}

// Unsafe optimizations (use with caution)

// float64SliceToBytes reinterprets a float64 slice as bytes without copying.
// UNSAFE: The returned slice shares memory with the input.
func float64SliceToBytes(data []float64) []byte {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
}

// bytesToFloat64Slice reinterprets bytes as float64 slice without copying.
// UNSAFE: The input must be properly aligned and sized.
func bytesToFloat64Slice(data []byte) []float64 {
	if len(data) < 8 {
		return nil
	}
	return unsafe.Slice((*float64)(unsafe.Pointer(&data[0])), len(data)/8)
}

// int64SliceToBytes reinterprets an int64 slice as bytes without copying.
func int64SliceToBytes(data []int64) []byte {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*8)
}

// bytesToInt64Slice reinterprets bytes as int64 slice without copying.
func bytesToInt64Slice(data []byte) []int64 {
	if len(data) < 8 {
		return nil
	}
	return unsafe.Slice((*int64)(unsafe.Pointer(&data[0])), len(data)/8)
}
