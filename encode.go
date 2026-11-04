package alp

import "math"

// Encode compresses 1 to VectorSize values of src into dst, using delta mode when it packs narrower.
func Encode(dst *Vector, src []float64, s State) {
	if len(src) == 0 || len(src) > VectorSize {
		panic("alp: input must have 1 to VectorSize elements")
	}

	dst.Exp = s.Exp
	dst.Factor = s.Factor

	encodeMult := expArr[s.Exp] * fracArr[s.Factor]
	factMult := float64(factArr[s.Factor])
	fracMult := fracArr[s.Exp]

	exceptionCount := uint16(0)
	var first int64
	haveFirst := false
	for i, value := range src {
		bits := math.Float64bits(value)
		encoded := int64(0)
		ok := !isSpecial(bits)
		if ok {
			encoded = roundToInt64(value * encodeMult)
			decoded := float64(encoded) * factMult * fracMult
			ok = math.Float64bits(decoded) == bits && encoded >= minEncodable && encoded <= maxEncodable
		}
		if !ok {
			dst.ExceptionPos[exceptionCount] = uint16(i)
			dst.ExceptionBits[exceptionCount] = bits
			exceptionCount++
		} else if !haveFirst {
			first, haveFirst = encoded, true
		}
		dst.Encoded[i] = encoded
	}
	dst.NumExceptions = exceptionCount

	// Exception and padding slots repeat the previous value so they widen nothing.
	prev := first
	next := uint16(0)
	for i := range dst.Encoded {
		if i >= len(src) || (next < exceptionCount && dst.ExceptionPos[next] == uint16(i)) {
			if i < len(src) {
				next++
			}
			dst.Encoded[i] = prev
			continue
		}
		prev = dst.Encoded[i]
	}

	minVal, maxVal := dst.Encoded[0], dst.Encoded[0]
	minDelta, maxDelta := dst.Encoded[1]-dst.Encoded[0], dst.Encoded[1]-dst.Encoded[0]
	for i := 1; i < VectorSize; i++ {
		v := dst.Encoded[i]
		minVal, maxVal = min(minVal, v), max(maxVal, v)
		d := v - dst.Encoded[i-1]
		minDelta, maxDelta = min(minDelta, d), max(maxDelta, d)
	}
	plainWidth := bitWidth(uint64(maxVal) - uint64(minVal))
	deltaWidth := bitWidth(uint64(maxDelta) - uint64(minDelta))

	dst.Delta = deltaWidth < plainWidth
	if !dst.Delta {
		dst.Base = 0
		dst.FrameOfRef = minVal
		dst.BitWidth = plainWidth
		for i := range dst.Encoded {
			dst.Encoded[i] -= minVal
		}
		return
	}
	// Deltas wrap on int64 overflow; Decode wraps the same way.
	dst.Base = dst.Encoded[0]
	dst.FrameOfRef = minDelta
	dst.BitWidth = deltaWidth
	prev = dst.Encoded[0]
	dst.Encoded[0] = 0
	for i := 1; i < VectorSize; i++ {
		v := dst.Encoded[i]
		dst.Encoded[i] = v - prev - minDelta
		prev = v
	}
}

// Decode decompresses the first len(dst) values of src; len(dst) must not exceed VectorSize.
func Decode(dst []float64, src *Vector) {
	if len(dst) > VectorSize {
		panic("alp: output must have at most VectorSize elements")
	}

	factMult := float64(factArr[src.Factor])
	fracMult := fracArr[src.Exp]
	frameOfRef := src.FrameOfRef

	if src.Delta {
		v := src.Base
		for i := range dst {
			if i > 0 {
				v += src.Encoded[i] + frameOfRef
			}
			dst[i] = float64(v) * factMult * fracMult
		}
	} else {
		for i := range dst {
			dst[i] = float64(src.Encoded[i]+frameOfRef) * factMult * fracMult
		}
	}

	for i := range src.NumExceptions {
		if position := int(src.ExceptionPos[i]); position < len(dst) {
			dst[position] = math.Float64frombits(src.ExceptionBits[i])
		}
	}
}
