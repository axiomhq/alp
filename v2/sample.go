package alp

import "math"

// Sampling implements the paper's two-level sampling strategy:
// 1. First level: sample values from the dataset to find top-K (exp, factor) pairs
// 2. Second level: for each vector, test top-K pairs and pick the best

const (
	samplesPerVector = 32    // samples to take from each vector
	maxSamples       = 1024  // maximum total samples
	topK             = 5     // number of top combinations to track
)

// sampleData extracts representative samples from the input.
func sampleData(data []float64) []float64 {
	n := len(data)
	if n <= maxSamples {
		// Small data: use everything
		out := make([]float64, n)
		copy(out, data)
		return out
	}

	// Large data: sample evenly
	out := make([]float64, 0, maxSamples)
	step := n / maxSamples
	for i := 0; i < n && len(out) < maxSamples; i += step {
		out = append(out, data[i])
	}
	return out
}

// findBestParameters tests all (exp, factor) combinations against samples
// and returns the pair that minimizes encoded bit width + exceptions.
func findBestParameters(samples []float64) (exp, factor uint8) {
	if len(samples) == 0 {
		return 0, 0
	}

	bestExp := uint8(0)
	bestFactor := uint8(0)
	bestCost := uint64(1<<63 - 1)

	// Test all combinations (18 × 18 = 324 pairs)
	for e := uint8(0); e < numExp; e++ {
		for f := uint8(0); f < numFact; f++ {
			cost := estimateCost(samples, e, f)
			if cost < bestCost {
				bestCost = cost
				bestExp = e
				bestFactor = f
			}
		}
	}

	return bestExp, bestFactor
}

// estimateCost estimates compression cost for given parameters.
// Cost = (bit_width × num_values) + (exceptions × 64)
func estimateCost(samples []float64, exp, factor uint8) uint64 {
	encodeMult := expArr[exp] * fracArr[factor]
	factMult := float64(factArr[factor])
	fracMult := fracArr[exp]

	minVal := int64(1<<63 - 1)
	maxVal := int64(-1 << 63)
	exceptions := 0

	for _, v := range samples {
		bits := math.Float64bits(v)

		// Special values are always exceptions
		if isSpecial(bits) {
			exceptions++
			continue
		}

		encoded := roundToInt64(v * encodeMult)

		// Range check
		if encoded < minEncodable || encoded > maxEncodable {
			exceptions++
			continue
		}

		// Roundtrip check
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

	// All exceptions: high cost
	if minVal > maxVal {
		return uint64(len(samples)) * 64
	}

	width := bitWidth(uint64(maxVal - minVal))
	normalCount := len(samples) - exceptions

	// Cost model from paper
	return uint64(normalCount)*uint64(width) + uint64(exceptions)*64
}
