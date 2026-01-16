package alp

import (
	"testing"
)

func BenchmarkDecodeVectorOriginal(b *testing.B) {
	// Setup: encode some data
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	cfg := DefaultConfig()
	encoder := NewEncoder(cfg)
	encoder.Sample(data)
	block := encoder.EncodeVector(data)

	decoder := NewDecoder()
	result := make([]float64, VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		decoder.DecodeVector(block, result)
	}
}

func BenchmarkDecodeVectorFast(b *testing.B) {
	// Setup: encode some data
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	cfg := DefaultConfig()
	encoder := NewEncoder(cfg)
	encoder.Sample(data)
	block := encoder.EncodeVector(data)

	decoder := NewDecoder()
	result := make([]float64, VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		decoder.DecodeVectorFast(block, result)
	}
}

func BenchmarkDecodeChunkNoExceptions(b *testing.B) {
	encoded := make([]int64, 64)
	result := make([]float64, 64)
	for i := range encoded {
		encoded[i] = int64(i * 100)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		decodeChunkNoExceptions(encoded, result, 1000, 100.0, 0.01)
	}
}

func BenchmarkFindMinMax(b *testing.B) {
	data := make([]int64, VectorSize)
	for i := range data {
		data[i] = int64(i * 100)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		findMinMax(data)
	}
}

func BenchmarkForEncode(b *testing.B) {
	data := make([]int64, VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset data
		for j := range data {
			data[j] = int64(j * 100)
		}
		forEncode(data, 50000)
	}
}

func BenchmarkPopcount(b *testing.B) {
	x := uint64(0xAAAAAAAAAAAAAAAA)

	b.ResetTimer()

	var sum int
	for i := 0; i < b.N; i++ {
		sum += popcount64(x)
	}
	_ = sum
}

func TestDecodeVectorFastCorrectness(t *testing.T) {
	// Test with no exceptions
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = float64(i)
	}

	cfg := DefaultConfig()
	encoder := NewEncoder(cfg)
	encoder.Sample(data)
	block := encoder.EncodeVector(data)

	decoder := NewDecoder()
	result1 := make([]float64, VectorSize)
	result2 := make([]float64, VectorSize)

	decoder.DecodeVector(block, result1)
	decoder.DecodeVectorFast(block, result2)

	for i := range result1 {
		if result1[i] != result2[i] {
			t.Errorf("mismatch at %d: got %v, want %v", i, result2[i], result1[i])
		}
	}
}

func TestDecodeVectorFastWithExceptions(t *testing.T) {
	// Test with some exceptions (special values)
	data := make([]float64, VectorSize)
	for i := range data {
		if i%100 == 0 {
			data[i] = float64(i) + 0.123456789012345 // Will likely be exception
		} else {
			data[i] = float64(i)
		}
	}

	cfg := DefaultConfig()
	encoder := NewEncoder(cfg)
	encoder.Sample(data)
	block := encoder.EncodeVector(data)

	decoder := NewDecoder()
	result1 := make([]float64, VectorSize)
	result2 := make([]float64, VectorSize)

	decoder.DecodeVector(block, result1)
	decoder.DecodeVectorFast(block, result2)

	for i := range result1 {
		if result1[i] != result2[i] {
			t.Errorf("mismatch at %d: got %v, want %v", i, result2[i], result1[i])
		}
	}
}

func TestFindMinMax(t *testing.T) {
	tests := []struct {
		data     []int64
		wantMin  int64
		wantMax  int64
	}{
		{[]int64{1, 2, 3, 4, 5}, 1, 5},
		{[]int64{5, 4, 3, 2, 1}, 1, 5},
		{[]int64{-10, 0, 10}, -10, 10},
		{[]int64{42}, 42, 42},
		{[]int64{}, 0, 0},
	}

	for _, tt := range tests {
		min, max := findMinMax(tt.data)
		if min != tt.wantMin || max != tt.wantMax {
			t.Errorf("findMinMax(%v) = (%d, %d), want (%d, %d)",
				tt.data, min, max, tt.wantMin, tt.wantMax)
		}
	}
}

func TestPopcount(t *testing.T) {
	tests := []struct {
		x    uint64
		want int
	}{
		{0, 0},
		{1, 1},
		{0xFF, 8},
		{0xFFFF, 16},
		{0xFFFFFFFF, 32},
		{0xFFFFFFFFFFFFFFFF, 64},
		{0xAAAAAAAAAAAAAAAA, 32},
	}

	for _, tt := range tests {
		got := popcount64(tt.x)
		if got != tt.want {
			t.Errorf("popcount64(0x%x) = %d, want %d", tt.x, got, tt.want)
		}
	}
}
