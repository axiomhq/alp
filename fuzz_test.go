package alp

import (
	"math"
	"math/rand"
	"testing"
)

// FuzzCompressDecompress tests roundtrip compression/decompression.
func FuzzCompressDecompress(f *testing.F) {
	// Seed corpus with various patterns
	f.Add(int64(42), 100, 0)    // decimal data
	f.Add(int64(123), 100, 1)   // integer data
	f.Add(int64(999), 100, 2)   // scientific data
	f.Add(int64(7), 1024, 0)    // full vector
	f.Add(int64(31337), 5000, 0) // multiple vectors
	f.Add(int64(1), 1, 0)       // single value
	f.Add(int64(0), 10, 3)      // mixed with special values

	f.Fuzz(func(t *testing.T, seed int64, count int, pattern int) {
		if count <= 0 || count > 100000 {
			return
		}

		rng := rand.New(rand.NewSource(seed))
		data := generateFuzzData(rng, count, pattern)

		compressed := CompressFloat64(data)
		decompressed := DecompressFloat64(compressed)

		if len(decompressed) != len(data) {
			t.Fatalf("length mismatch: got %d, want %d", len(decompressed), len(data))
		}

		for i := range data {
			if !floatEqual(data[i], decompressed[i]) {
				t.Fatalf("value mismatch at %d: got %v (bits=%x), want %v (bits=%x)",
					i, decompressed[i], math.Float64bits(decompressed[i]),
					data[i], math.Float64bits(data[i]))
			}
		}
	})
}

// FuzzEncodeDecode tests the low-level encoder/decoder roundtrip.
func FuzzEncodeDecode(f *testing.F) {
	f.Add(int64(42), 100)
	f.Add(int64(123), VectorSize)
	f.Add(int64(999), 50)

	f.Fuzz(func(t *testing.T, seed int64, count int) {
		if count <= 0 || count > VectorSize {
			return
		}

		rng := rand.New(rand.NewSource(seed))
		data := make([]float64, count)
		for i := range data {
			data[i] = float64(rng.Intn(1000000)) / 100.0
		}

		cfg := DefaultConfig()
		encoder := NewEncoder(cfg)
		encoder.Sample(data)

		block := encoder.Encode(data)
		decoder := NewDecoder()
		result := decoder.Decode(block)

		if len(result) != len(data) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(data))
		}

		for i := range data {
			if !floatEqual(data[i], result[i]) {
				t.Fatalf("value mismatch at %d: got %v, want %v", i, result[i], data[i])
			}
		}
	})
}

// FuzzRDEncodeDecode tests the RD encoder/decoder roundtrip.
func FuzzRDEncodeDecode(f *testing.F) {
	f.Add(int64(42), 100)
	f.Add(int64(123), VectorSize)
	f.Add(int64(999), 50)

	f.Fuzz(func(t *testing.T, seed int64, count int) {
		if count <= 0 || count > VectorSize {
			return
		}

		rng := rand.New(rand.NewSource(seed))
		data := make([]float64, count)
		for i := range data {
			// High-precision random floats
			data[i] = rng.Float64() * math.Pow(10, float64(rng.Intn(20)-10))
		}

		cfg := DefaultConfig()
		encoder := NewRDEncoder(cfg)
		encoder.Sample(data)

		block := encoder.Encode(data)
		decoder := NewRDDecoder()
		result := decoder.Decode(block)

		if len(result) != len(data) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(data))
		}

		for i := range data {
			if !floatEqual(data[i], result[i]) {
				t.Fatalf("value mismatch at %d: got %v, want %v", i, result[i], data[i])
			}
		}
	})
}

// FuzzBitPack tests bit-packing roundtrip.
func FuzzBitPack(f *testing.F) {
	f.Add(int64(42), 100, uint8(8))
	f.Add(int64(123), 50, uint8(16))
	f.Add(int64(999), 200, uint8(4))

	f.Fuzz(func(t *testing.T, seed int64, count int, bitWidth uint8) {
		if count <= 0 || count > 10000 {
			return
		}
		if bitWidth == 0 || bitWidth > 64 {
			return
		}

		rng := rand.New(rand.NewSource(seed))
		maxVal := uint64(1) << bitWidth
		if bitWidth == 64 {
			maxVal = math.MaxUint64
		}

		values := make([]uint64, count)
		for i := range values {
			values[i] = rng.Uint64() % maxVal
		}

		packer := NewBitPacker(count * 8)
		packer.Pack(values, bitWidth)

		unpacker := NewBitUnpacker(packer.Bytes())
		result := unpacker.Unpack(count, bitWidth)

		if len(result) != len(values) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(values))
		}

		for i := range values {
			if result[i] != values[i] {
				t.Fatalf("value mismatch at %d: got %d, want %d", i, result[i], values[i])
			}
		}
	})
}

// FuzzSerialization tests serialization roundtrip.
func FuzzSerialization(f *testing.F) {
	f.Add(int64(42), 100)
	f.Add(int64(123), 2048)
	f.Add(int64(999), 5000)

	f.Fuzz(func(t *testing.T, seed int64, count int) {
		if count <= 0 || count > 100000 {
			return
		}

		rng := rand.New(rand.NewSource(seed))
		data := make([]float64, count)
		for i := range data {
			data[i] = float64(rng.Intn(1000000)) / 100.0
		}

		compressed := CompressFloat64(data)

		// Serialize and deserialize
		serialized := compressed.Bytes()
		var loaded CompressedData
		if err := loaded.FromBytes(serialized); err != nil {
			t.Fatalf("FromBytes failed: %v", err)
		}

		result := DecompressFloat64(&loaded)

		if len(result) != len(data) {
			t.Fatalf("length mismatch: got %d, want %d", len(result), len(data))
		}

		for i := range data {
			if !floatEqual(data[i], result[i]) {
				t.Fatalf("value mismatch at %d: got %v, want %v", i, result[i], data[i])
			}
		}
	})
}

// generateFuzzData creates test data based on pattern type.
func generateFuzzData(rng *rand.Rand, count int, pattern int) []float64 {
	data := make([]float64, count)
	switch pattern % 4 {
	case 0: // Decimal/monetary values
		for i := range data {
			data[i] = float64(rng.Intn(1000000)) / 100.0
		}
	case 1: // Integer values
		for i := range data {
			data[i] = float64(rng.Intn(100000))
		}
	case 2: // Scientific notation
		for i := range data {
			data[i] = rng.Float64() * math.Pow(10, float64(rng.Intn(20)-10))
		}
	case 3: // Mixed with special values
		for i := range data {
			switch rng.Intn(20) {
			case 0:
				data[i] = 0.0
			case 1:
				data[i] = math.Copysign(0, -1) // -0.0
			case 2:
				data[i] = math.Inf(1)
			case 3:
				data[i] = math.Inf(-1)
			case 4:
				data[i] = math.NaN()
			default:
				data[i] = float64(rng.Intn(1000000)) / 100.0
			}
		}
	}
	return data
}

// floatEqual compares two float64 values, handling NaN and -0.
func floatEqual(a, b float64) bool {
	// Both NaN
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	// Check bit-exact equality (handles -0 vs +0)
	return math.Float64bits(a) == math.Float64bits(b)
}

// Test specific edge cases based on reference implementation patterns

func TestEdgeCaseNegativeZero(t *testing.T) {
	data := []float64{-0.0, 0.0, -0.0, 0.0}
	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if math.Float64bits(data[i]) != math.Float64bits(decompressed[i]) {
			t.Errorf("negative zero not preserved at %d: got bits %x, want %x",
				i, math.Float64bits(decompressed[i]), math.Float64bits(data[i]))
		}
	}
}

func TestEdgeCaseAllNaN(t *testing.T) {
	data := []float64{math.NaN(), math.NaN(), math.NaN()}
	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if !math.IsNaN(decompressed[i]) {
			t.Errorf("NaN not preserved at %d: got %v", i, decompressed[i])
		}
	}
}

func TestEdgeCaseAllInf(t *testing.T) {
	data := []float64{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if math.Float64bits(data[i]) != math.Float64bits(decompressed[i]) {
			t.Errorf("infinity not preserved at %d: got %v, want %v",
				i, decompressed[i], data[i])
		}
	}
}

func TestEdgeCaseExtremeValues(t *testing.T) {
	data := []float64{
		math.MaxFloat64,
		-math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		-math.SmallestNonzeroFloat64,
		1.7976931348623157e+308,  // near max
		-1.7976931348623157e+308, // near -max
		4.9406564584124654e-324,  // smallest denormal
	}

	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if !floatEqual(data[i], decompressed[i]) {
			t.Errorf("extreme value not preserved at %d: got %v (bits=%x), want %v (bits=%x)",
				i, decompressed[i], math.Float64bits(decompressed[i]),
				data[i], math.Float64bits(data[i]))
		}
	}
}

func TestEdgeCaseSingleValue(t *testing.T) {
	testCases := []float64{
		0.0, -0.0, 1.0, -1.0, 0.1, -0.1,
		math.Pi, math.E, math.Phi,
		math.Inf(1), math.Inf(-1), math.NaN(),
		math.MaxFloat64, math.SmallestNonzeroFloat64,
	}

	for _, val := range testCases {
		data := []float64{val}
		compressed := CompressFloat64(data)
		decompressed := DecompressFloat64(compressed)

		if len(decompressed) != 1 {
			t.Errorf("single value %v: wrong length %d", val, len(decompressed))
			continue
		}

		if !floatEqual(val, decompressed[0]) {
			t.Errorf("single value not preserved: got %v (bits=%x), want %v (bits=%x)",
				decompressed[0], math.Float64bits(decompressed[0]),
				val, math.Float64bits(val))
		}
	}
}

func TestEdgeCaseRepeatedValue(t *testing.T) {
	testCases := []struct {
		name  string
		value float64
		count int
	}{
		{"zeros", 0.0, 1000},
		{"ones", 1.0, 1000},
		{"pi", math.Pi, 1000},
		{"negative", -123.45, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]float64, tc.count)
			for i := range data {
				data[i] = tc.value
			}

			compressed := CompressFloat64(data)
			decompressed := DecompressFloat64(compressed)

			for i := range data {
				if !floatEqual(data[i], decompressed[i]) {
					t.Errorf("repeated value not preserved at %d: got %v, want %v",
						i, decompressed[i], data[i])
				}
			}

			// Repeated values should compress very well
			if tc.value != math.Pi { // Pi won't compress well with ALP
				ratio := compressed.CompressionRatio()
				if ratio > 0.5 {
					t.Logf("Warning: repeated %s compression ratio %.2f is high", tc.name, ratio)
				}
			}
		})
	}
}

func TestEdgeCaseMonotonicSequence(t *testing.T) {
	data := make([]float64, 1000)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if !floatEqual(data[i], decompressed[i]) {
			t.Errorf("monotonic value not preserved at %d: got %v, want %v",
				i, decompressed[i], data[i])
		}
	}
}

func TestEdgeCaseAlternatingSign(t *testing.T) {
	data := make([]float64, 1000)
	for i := range data {
		if i%2 == 0 {
			data[i] = float64(i) * 0.01
		} else {
			data[i] = -float64(i) * 0.01
		}
	}

	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	for i := range data {
		if !floatEqual(data[i], decompressed[i]) {
			t.Errorf("alternating value not preserved at %d: got %v, want %v",
				i, decompressed[i], data[i])
		}
	}
}

// TestDeterministicCompression ensures compression is deterministic.
func TestDeterministicCompression(t *testing.T) {
	data := make([]float64, 1000)
	rng := rand.New(rand.NewSource(42))
	for i := range data {
		data[i] = float64(rng.Intn(1000000)) / 100.0
	}

	// Compress twice
	c1 := CompressFloat64(data)
	c2 := CompressFloat64(data)

	// Compare serialized output
	b1 := c1.Bytes()
	b2 := c2.Bytes()

	if len(b1) != len(b2) {
		t.Fatalf("compressed lengths differ: %d vs %d", len(b1), len(b2))
	}

	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("compressed byte differs at %d: %x vs %x", i, b1[i], b2[i])
		}
	}
}

// Benchmark fuzz-like patterns
func BenchmarkFuzzPatterns(b *testing.B) {
	patterns := []struct {
		name    string
		pattern int
	}{
		{"decimal", 0},
		{"integer", 1},
		{"scientific", 2},
		{"mixed_special", 3},
	}

	for _, p := range patterns {
		b.Run(p.name, func(b *testing.B) {
			rng := rand.New(rand.NewSource(42))
			data := generateFuzzData(rng, 10000, p.pattern)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				compressed := CompressFloat64(data)
				_ = DecompressFloat64(compressed)
			}
		})
	}
}
