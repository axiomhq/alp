package alp

import "math"

// Encode compresses src into dst using s. src must contain exactly VectorSize
// values.
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
	exceptionCount := uint16(0)

	for i := 0; i < VectorSize; i++ {
		value := src[i]
		bits := math.Float64bits(value)

		// NaN, infinity, and negative zero are always exceptions.
		exponent := bits & 0x7FF0000000000000
		if exponent == 0x7FF0000000000000 || bits == 0x8000000000000000 {
			dst.ExceptionPos[exceptionCount] = uint16(i)
			dst.ExceptionBits[exceptionCount] = bits
			exceptionCount++
			dst.Encoded[i] = 0
			continue
		}

		scaled := value * encodeMult
		var encoded int64
		if scaled >= 0 {
			encoded = int64(scaled + 0.5)
		} else {
			encoded = int64(scaled - 0.5)
		}

		decoded := float64(encoded) * factMult * fracMult
		if math.Float64bits(decoded) != bits || encoded < minEncodable || encoded > maxEncodable {
			dst.ExceptionPos[exceptionCount] = uint16(i)
			dst.ExceptionBits[exceptionCount] = bits
			exceptionCount++
			dst.Encoded[i] = 0
			continue
		}

		dst.Encoded[i] = encoded
		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	dst.NumExceptions = exceptionCount
	if minVal > maxVal {
		minVal, maxVal = 0, 0
	}

	dst.FrameOfRef = minVal
	dst.BitWidth = bitWidth(uint64(maxVal - minVal))

	for i := 0; i < VectorSize; i++ {
		dst.Encoded[i] -= minVal
	}
}

// Decode decompresses src into dst. dst must contain exactly VectorSize
// elements.
func Decode(dst []float64, src *Vector) {
	if len(dst) != VectorSize {
		panic("alp: output must have exactly VectorSize elements")
	}

	factMult := float64(factArr[src.Factor])
	fracMult := fracArr[src.Exp]
	frameOfRef := src.FrameOfRef

	if src.NumExceptions == 0 {
		for i := 0; i < VectorSize; i++ {
			dst[i] = float64(src.Encoded[i]+frameOfRef) * factMult * fracMult
		}
		return
	}

	var exceptionBitmap [VectorSize / 64]uint64
	for i := uint16(0); i < src.NumExceptions; i++ {
		position := src.ExceptionPos[i]
		exceptionBitmap[position/64] |= 1 << (position % 64)
	}

	exceptionIndex := uint16(0)
	for word := 0; word < VectorSize/64; word++ {
		bits := exceptionBitmap[word]
		base := word * 64

		if bits == 0 {
			for i := 0; i < 64; i++ {
				dst[base+i] = float64(src.Encoded[base+i]+frameOfRef) * factMult * fracMult
			}
			continue
		}

		for i := 0; i < 64; i++ {
			index := base + i
			if bits&(1<<i) != 0 {
				dst[index] = math.Float64frombits(src.ExceptionBits[exceptionIndex])
				exceptionIndex++
			} else {
				dst[index] = float64(src.Encoded[index]+frameOfRef) * factMult * fracMult
			}
		}
	}
}
