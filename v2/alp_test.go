package alp

import (
	"math"
	"math/rand"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	// Generate test data: monetary values
	src := make([]float64, VectorSize)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0 // 0.00 to 9999.99
	}

	// Sample and encode
	state := Sample(src)
	var v Vector
	Encode(&v, src, state)

	// Decode
	dst := make([]float64, VectorSize)
	Decode(dst, &v)

	// Verify
	for i := range src {
		if math.Float64bits(src[i]) != math.Float64bits(dst[i]) {
			t.Errorf("mismatch at %d: got %v, want %v", i, dst[i], src[i])
		}
	}
}

func TestRoundtripWithSpecials(t *testing.T) {
	src := make([]float64, VectorSize)
	for i := range src {
		switch i % 10 {
		case 0:
			src[i] = 0.0
		case 1:
			src[i] = math.Copysign(0, -1) // -0.0
		case 2:
			src[i] = math.Inf(1)
		case 3:
			src[i] = math.Inf(-1)
		case 4:
			src[i] = math.NaN()
		default:
			src[i] = float64(rand.Intn(1000000)) / 100.0
		}
	}

	state := Sample(src)
	var v Vector
	Encode(&v, src, state)

	dst := make([]float64, VectorSize)
	Decode(dst, &v)

	for i := range src {
		srcBits := math.Float64bits(src[i])
		dstBits := math.Float64bits(dst[i])

		// NaN: just check both are NaN
		if math.IsNaN(src[i]) {
			if !math.IsNaN(dst[i]) {
				t.Errorf("mismatch at %d: got %v, want NaN", i, dst[i])
			}
			continue
		}

		if srcBits != dstBits {
			t.Errorf("mismatch at %d: got bits %x, want %x", i, dstBits, srcBits)
		}
	}
}

func TestPackUnpack(t *testing.T) {
	src := make([]float64, VectorSize)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0
	}

	state := Sample(src)
	var v1 Vector
	Encode(&v1, src, state)

	// Pack and unpack
	packed := Pack(&v1)
	var v2 Vector
	Unpack(&v2, packed)

	// Decode both
	dst1 := make([]float64, VectorSize)
	dst2 := make([]float64, VectorSize)
	Decode(dst1, &v1)
	Decode(dst2, &v2)

	for i := range dst1 {
		if math.Float64bits(dst1[i]) != math.Float64bits(dst2[i]) {
			t.Errorf("mismatch at %d after pack/unpack", i)
		}
	}
}

func TestCompressionRatio(t *testing.T) {
	src := make([]float64, VectorSize)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0
	}

	state := Sample(src)
	var v Vector
	Encode(&v, src, state)

	packed := Pack(&v)
	originalSize := VectorSize * 8
	compressedSize := len(packed)
	ratio := float64(compressedSize) / float64(originalSize)

	t.Logf("Original: %d bytes", originalSize)
	t.Logf("Compressed: %d bytes", compressedSize)
	t.Logf("Ratio: %.2f%%", ratio*100)
	t.Logf("BitWidth: %d, Exceptions: %d", v.BitWidth, v.NumExceptions)

	if ratio > 0.6 {
		t.Errorf("compression ratio too high: %.2f%%", ratio*100)
	}
}

func BenchmarkEncode(b *testing.B) {
	src := make([]float64, VectorSize)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0
	}

	state := Sample(src)
	var v Vector

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Encode(&v, src, state)
	}
}

func BenchmarkDecode(b *testing.B) {
	src := make([]float64, VectorSize)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0
	}

	state := Sample(src)
	var v Vector
	Encode(&v, src, state)
	dst := make([]float64, VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Decode(dst, &v)
	}
}

func BenchmarkSample(b *testing.B) {
	src := make([]float64, 100000)
	for i := range src {
		src[i] = float64(rand.Intn(1000000)) / 100.0
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = Sample(src)
	}
}
