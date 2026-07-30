package alp

import "math"

func isSpecial(bits uint64) bool {
	exponent := bits & 0x7FF0000000000000
	return exponent == 0x7FF0000000000000 || bits == 0x8000000000000000
}

func roundToInt64(value float64) int64 {
	if value >= 0 {
		return int64(value + 0.5)
	}
	return int64(value - 0.5)
}

const maxSamples = 1024

// sampleData extracts representative samples from data.
func sampleData(data []float64) []float64 {
	n := len(data)
	if n <= maxSamples {
		result := make([]float64, n)
		copy(result, data)
		return result
	}

	result := make([]float64, 0, maxSamples)
	step := n / maxSamples
	for i := 0; i < n && len(result) < maxSamples; i += step {
		result = append(result, data[i])
	}
	return result
}

// findBestParameters returns the exponent and factor that minimize encoded
// width plus exception cost for samples.
func findBestParameters(samples []float64) (exp, factor uint8) {
	if len(samples) == 0 {
		return 0, 0
	}

	bestExp := uint8(0)
	bestFactor := uint8(0)
	bestCost := uint64(1<<63 - 1)

	for candidateExp := uint8(0); candidateExp < numExp; candidateExp++ {
		for candidateFactor := uint8(0); candidateFactor < numFact; candidateFactor++ {
			cost := estimateCost(samples, candidateExp, candidateFactor)
			if cost < bestCost {
				bestCost = cost
				bestExp = candidateExp
				bestFactor = candidateFactor
			}
		}
	}

	return bestExp, bestFactor
}

func estimateCost(samples []float64, exp, factor uint8) uint64 {
	encodeMult := expArr[exp] * fracArr[factor]
	factMult := float64(factArr[factor])
	fracMult := fracArr[exp]

	minVal := int64(1<<63 - 1)
	maxVal := int64(-1 << 63)
	exceptions := 0

	for _, value := range samples {
		bits := math.Float64bits(value)
		if isSpecial(bits) {
			exceptions++
			continue
		}

		encoded := roundToInt64(value * encodeMult)
		if encoded < minEncodable || encoded > maxEncodable {
			exceptions++
			continue
		}

		decoded := float64(encoded) * factMult * fracMult
		if math.Float64bits(decoded) != bits {
			exceptions++
			continue
		}

		if encoded < minVal {
			minVal = encoded
		}
		if encoded > maxVal {
			maxVal = encoded
		}
	}

	if minVal > maxVal {
		return uint64(len(samples)) * 64
	}

	width := bitWidth(uint64(maxVal - minVal))
	normalCount := len(samples) - exceptions
	return uint64(normalCount)*uint64(width) + uint64(exceptions)*64
}
