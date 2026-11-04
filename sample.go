package alp

import (
	"math"
)

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

const (
	maxSamples = 1024
	// sampleWindow keeps neighbors together so the cost estimate sees deltas.
	sampleWindow = 32
)

// sampleData extracts up to maxSamples values as evenly spaced contiguous windows.
func sampleData(data []float64) []float64 {
	n := len(data)
	if n <= maxSamples {
		return data
	}

	windows := maxSamples / sampleWindow
	result := make([]float64, 0, maxSamples)
	for w := range windows {
		start := w * (n - sampleWindow) / (windows - 1)
		result = append(result, data[start:start+sampleWindow]...)
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

	for candidateExp := range uint8(numExp) {
		for candidateFactor := range uint8(numFact) {
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

// estimateCost returns the bits to encode samples with exp and factor, taking
// the narrower of plain and delta packing. Deltas only span values in one sample window.
func estimateCost(samples []float64, exp, factor uint8) uint64 {
	encodeMult := expArr[exp] * fracArr[factor]
	factMult := float64(factArr[factor])
	fracMult := fracArr[exp]

	minVal, maxVal := int64(math.MaxInt64), int64(math.MinInt64)
	minDelta, maxDelta := int64(math.MaxInt64), int64(math.MinInt64)
	exceptions := 0
	var prev int64
	havePrev := false

	for i, value := range samples {
		if i%sampleWindow == 0 {
			havePrev = false
		}
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

		minVal, maxVal = min(minVal, encoded), max(maxVal, encoded)
		if havePrev {
			d := encoded - prev
			minDelta, maxDelta = min(minDelta, d), max(maxDelta, d)
		}
		prev, havePrev = encoded, true
	}

	if minVal > maxVal {
		return uint64(len(samples)) * 64
	}

	width := bitWidth(uint64(maxVal) - uint64(minVal))
	if minDelta <= maxDelta {
		width = min(width, bitWidth(uint64(maxDelta)-uint64(minDelta)))
	}
	normalCount := len(samples) - exceptions
	return uint64(normalCount)*uint64(width) + uint64(exceptions)*64
}
