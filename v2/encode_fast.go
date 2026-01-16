package alp

import "math"

// Encode compresses a vector of float64 values using the given state.
// Lemire-style: single pass, cache-line aware, minimal branching.
func Encode(dst *Vector, src []float64, s State) {
	if len(src) != VectorSize {
		panic("alp: input must have exactly VectorSize elements")
	}

	dst.Exp = s.Exp
	dst.Factor = s.Factor
	dst.NumExceptions = 0

	encodeMult := expArr[s.Exp] * fracArr[s.Factor]
	factMult := float64(factArr[s.Factor])
	fracMult := fracArr[s.Exp]

	minVal := int64(math.MaxInt64)
	maxVal := int64(math.MinInt64)
	excCount := uint16(0)

	// Single pass: encode and track exceptions
	for i := 0; i < VectorSize; i++ {
		v := src[i]
		bits := math.Float64bits(v)

		// Check for special values (NaN, Inf, -0)
		exp := bits & 0x7FF0000000000000
		if exp == 0x7FF0000000000000 || bits == 0x8000000000000000 {
			dst.ExceptionPos[excCount] = uint16(i)
			dst.ExceptionBits[excCount] = bits
			excCount++
			dst.Encoded[i] = 0
			continue
		}

		// Encode: round(value × 10^exp × 10^-factor)
		scaled := v * encodeMult
		var encoded int64
		if scaled >= 0 {
			encoded = int64(scaled + 0.5)
		} else {
			encoded = int64(scaled - 0.5)
		}

		// Roundtrip check
		decoded := float64(encoded) * factMult * fracMult
		if math.Float64bits(decoded) != bits || encoded < minEncodable || encoded > maxEncodable {
			dst.ExceptionPos[excCount] = uint16(i)
			dst.ExceptionBits[excCount] = bits
			excCount++
			dst.Encoded[i] = 0
			continue
		}

		dst.Encoded[i] = encoded

		// Branchless min/max update (compiler optimizes these well)
		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	dst.NumExceptions = excCount

	// Handle all-exceptions case
	if minVal > maxVal {
		minVal, maxVal = 0, 0
	}

	dst.FrameOfRef = minVal
	dst.BitWidth = bitWidth(uint64(maxVal - minVal))

	// Apply FOR (this loop is auto-vectorizable)
	for i := 0; i < VectorSize; i++ {
		dst.Encoded[i] -= minVal
	}
}

// Decode decompresses a vector back to float64 values.
// Lemire-style: bitmap-based exception handling, cache-line chunks.
func Decode(dst []float64, src *Vector) {
	if len(dst) != VectorSize {
		panic("alp: output must have exactly VectorSize elements")
	}

	factMult := float64(factArr[src.Factor])
	fracMult := fracArr[src.Exp]
	frameOfRef := src.FrameOfRef

	// Fast path: no exceptions at all
	if src.NumExceptions == 0 {
		for i := 0; i < VectorSize; i++ {
			dst[i] = float64(src.Encoded[i]+frameOfRef) * factMult * fracMult
		}
		return
	}

	// Build exception bitmap for O(1) lookup
	var excBitmap [VectorSize / 64]uint64
	for i := uint16(0); i < src.NumExceptions; i++ {
		pos := src.ExceptionPos[i]
		excBitmap[pos/64] |= 1 << (pos % 64)
	}

	excIdx := uint16(0)

	// Process in 64-element chunks (one bitmap word each)
	for word := 0; word < VectorSize/64; word++ {
		bits := excBitmap[word]
		base := word * 64

		if bits == 0 {
			// Fast path: no exceptions in this chunk (auto-vectorizable)
			for i := 0; i < 64; i++ {
				dst[base+i] = float64(src.Encoded[base+i]+frameOfRef) * factMult * fracMult
			}
		} else {
			// Slow path: has exceptions
			for i := 0; i < 64; i++ {
				idx := base + i
				if bits&(1<<i) != 0 {
					dst[idx] = math.Float64frombits(src.ExceptionBits[excIdx])
					excIdx++
				} else {
					dst[idx] = float64(src.Encoded[idx]+frameOfRef) * factMult * fracMult
				}
			}
		}
	}
}
