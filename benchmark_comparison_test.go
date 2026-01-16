//go:build comparison

// Run with: go test -tags=comparison -bench=Compare -benchmem

package alp_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	"github.com/axiomhq/alp"
	"github.com/klauspost/compress/zstd"
)

// float64ToBytes converts float64 slice to bytes for zstd comparison
func float64ToBytes(data []float64) []byte {
	buf := make([]byte, len(data)*8)
	for i, v := range data {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}
	return buf
}

// bytesToFloat64 converts bytes back to float64 slice
func bytesToFloat64(data []byte) []float64 {
	result := make([]float64, len(data)/8)
	for i := range result {
		result[i] = math.Float64frombits(binary.LittleEndian.Uint64(data[i*8:]))
	}
	return result
}

func BenchmarkCompareMonetary(b *testing.B) {
	// Monetary-style data (prices like 10.99, 20.50)
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, 10000)
	for i := range data {
		data[i] = float64(rng.Intn(1000000)) / 100.0
	}

	b.Run("ALP", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			compressed := alp.CompressFloat64(data)
			_ = alp.DecompressFloat64(compressed)
		}
	})

	b.Run("Zstd-Default", func(b *testing.B) {
		encoder, _ := zstd.NewWriter(nil)
		decoder, _ := zstd.NewReader(nil)
		raw := float64ToBytes(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compressed := encoder.EncodeAll(raw, nil)
			decompressed, _ := decoder.DecodeAll(compressed, nil)
			_ = bytesToFloat64(decompressed)
		}
	})

	b.Run("Zstd-BestSpeed", func(b *testing.B) {
		encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		decoder, _ := zstd.NewReader(nil)
		raw := float64ToBytes(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compressed := encoder.EncodeAll(raw, nil)
			decompressed, _ := decoder.DecodeAll(compressed, nil)
			_ = bytesToFloat64(decompressed)
		}
	})

	b.Run("Zstd-BestCompression", func(b *testing.B) {
		encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		decoder, _ := zstd.NewReader(nil)
		raw := float64ToBytes(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compressed := encoder.EncodeAll(raw, nil)
			decompressed, _ := decoder.DecodeAll(compressed, nil)
			_ = bytesToFloat64(decompressed)
		}
	})
}

func BenchmarkCompareSensor(b *testing.B) {
	// Sensor data (temperature readings with small variance)
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, 10000)
	base := 20.0
	for i := range data {
		data[i] = base + rng.Float64()*5.0 - 2.5
	}

	b.Run("ALP", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			compressed := alp.CompressFloat64(data)
			_ = alp.DecompressFloat64(compressed)
		}
	})

	b.Run("Zstd-Default", func(b *testing.B) {
		encoder, _ := zstd.NewWriter(nil)
		decoder, _ := zstd.NewReader(nil)
		raw := float64ToBytes(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compressed := encoder.EncodeAll(raw, nil)
			decompressed, _ := decoder.DecodeAll(compressed, nil)
			_ = bytesToFloat64(decompressed)
		}
	})
}

func BenchmarkCompareRandom(b *testing.B) {
	// Random high-precision floats (worst case for ALP)
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, 10000)
	for i := range data {
		data[i] = rng.Float64() * 1e10
	}

	b.Run("ALP", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			compressed := alp.CompressFloat64(data)
			_ = alp.DecompressFloat64(compressed)
		}
	})

	b.Run("Zstd-Default", func(b *testing.B) {
		encoder, _ := zstd.NewWriter(nil)
		decoder, _ := zstd.NewReader(nil)
		raw := float64ToBytes(data)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compressed := encoder.EncodeAll(raw, nil)
			decompressed, _ := decoder.DecodeAll(compressed, nil)
			_ = bytesToFloat64(decompressed)
		}
	})
}

// TestCompareCompressionRatio prints compression ratio comparison
func TestCompareCompressionRatio(t *testing.T) {
	encoder, _ := zstd.NewWriter(nil)
	encoderFast, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	encoderBest, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))

	tests := []struct {
		name string
		data []float64
	}{
		{
			name: "Monetary (prices like 10.99)",
			data: func() []float64 {
				rng := rand.New(rand.NewSource(42))
				d := make([]float64, 10000)
				for i := range d {
					d[i] = float64(rng.Intn(1000000)) / 100.0
				}
				return d
			}(),
		},
		{
			name: "Sensor (temp ~20.0 ± 2.5)",
			data: func() []float64 {
				rng := rand.New(rand.NewSource(42))
				d := make([]float64, 10000)
				for i := range d {
					d[i] = 20.0 + rng.Float64()*5.0 - 2.5
				}
				return d
			}(),
		},
		{
			name: "Integer-like (1, 2, 3, ...)",
			data: func() []float64 {
				d := make([]float64, 10000)
				for i := range d {
					d[i] = float64(i)
				}
				return d
			}(),
		},
		{
			name: "Random high-precision",
			data: func() []float64 {
				rng := rand.New(rand.NewSource(42))
				d := make([]float64, 10000)
				for i := range d {
					d[i] = rng.Float64() * 1e10
				}
				return d
			}(),
		},
	}

	t.Log("\nCompression Ratio Comparison (lower is better):")
	t.Log("================================================")
	t.Logf("%-30s %8s %8s %8s %8s", "Data Type", "ALP", "Zstd", "Zstd-F", "Zstd-B")
	t.Log("------------------------------------------------")

	for _, tt := range tests {
		raw := float64ToBytes(tt.data)
		originalSize := float64(len(raw))

		// ALP
		alpCompressed := alp.CompressFloat64(tt.data)
		var alpBuf bytes.Buffer
		alpCompressed.WriteTo(&alpBuf)
		alpRatio := float64(alpBuf.Len()) / originalSize

		// Zstd default
		zstdCompressed := encoder.EncodeAll(raw, nil)
		zstdRatio := float64(len(zstdCompressed)) / originalSize

		// Zstd fast
		zstdFastCompressed := encoderFast.EncodeAll(raw, nil)
		zstdFastRatio := float64(len(zstdFastCompressed)) / originalSize

		// Zstd best
		zstdBestCompressed := encoderBest.EncodeAll(raw, nil)
		zstdBestRatio := float64(len(zstdBestCompressed)) / originalSize

		t.Logf("%-30s %7.2f%% %7.2f%% %7.2f%% %7.2f%%",
			tt.name,
			alpRatio*100,
			zstdRatio*100,
			zstdFastRatio*100,
			zstdBestRatio*100)
	}
	t.Log("------------------------------------------------")
	t.Log("Zstd-F = SpeedFastest, Zstd-B = BestCompression")
}
