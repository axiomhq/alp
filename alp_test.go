package alp

import (
	"bytes"
	"math"
	"math/rand"
	"testing"
)

func TestEncodeDecodeBasic(t *testing.T) {
	tests := []struct {
		name string
		data []float64
	}{
		{
			name: "simple decimals",
			data: []float64{1.0, 2.0, 3.0, 4.0, 5.0},
		},
		{
			name: "monetary values",
			data: []float64{10.99, 20.50, 15.75, 99.99, 0.01},
		},
		{
			name: "scientific notation",
			data: []float64{1.23e5, 4.56e-3, 7.89e10, 1.11e-7},
		},
		{
			name: "negative values",
			data: []float64{-1.5, -2.5, -100.25, -0.001},
		},
		{
			name: "mixed",
			data: []float64{0, 1, -1, 0.5, -0.5, 100, -100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := CompressFloat64(tt.data)
			decompressed := DecompressFloat64(compressed)

			if len(decompressed) != len(tt.data) {
				t.Errorf("length mismatch: got %d, want %d", len(decompressed), len(tt.data))
			}

			for i := range tt.data {
				if decompressed[i] != tt.data[i] {
					t.Errorf("value mismatch at %d: got %v, want %v", i, decompressed[i], tt.data[i])
				}
			}
		})
	}
}

func TestEncodeDecodeSpecialValues(t *testing.T) {
	data := []float64{
		0.0,
		-0.0,
		math.Inf(1),
		math.Inf(-1),
		math.NaN(),
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
	}

	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	if len(decompressed) != len(data) {
		t.Fatalf("length mismatch: got %d, want %d", len(decompressed), len(data))
	}

	// Check special values
	// +0.0
	if decompressed[0] != 0.0 {
		t.Errorf("expected 0.0, got %v", decompressed[0])
	}

	// -0.0 (check bit pattern)
	if math.Float64bits(decompressed[1]) != math.Float64bits(data[1]) {
		t.Errorf("expected -0.0 bit pattern, got different")
	}

	// +Inf
	if !math.IsInf(decompressed[2], 1) {
		t.Errorf("expected +Inf, got %v", decompressed[2])
	}

	// -Inf
	if !math.IsInf(decompressed[3], -1) {
		t.Errorf("expected -Inf, got %v", decompressed[3])
	}

	// NaN
	if !math.IsNaN(decompressed[4]) {
		t.Errorf("expected NaN, got %v", decompressed[4])
	}

	// MaxFloat64
	if decompressed[5] != math.MaxFloat64 {
		t.Errorf("expected MaxFloat64, got %v", decompressed[5])
	}

	// SmallestNonzeroFloat64
	if decompressed[6] != math.SmallestNonzeroFloat64 {
		t.Errorf("expected SmallestNonzeroFloat64, got %v", decompressed[6])
	}
}

func TestEncodeDecodeLargeVector(t *testing.T) {
	// Create a full vector of monetary-like values
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	compressed := CompressFloat64(data)
	decompressed := DecompressFloat64(compressed)

	if len(decompressed) != len(data) {
		t.Fatalf("length mismatch: got %d, want %d", len(decompressed), len(data))
	}

	for i := range data {
		if decompressed[i] != data[i] {
			t.Errorf("value mismatch at %d: got %v, want %v", i, decompressed[i], data[i])
		}
	}

	// Check compression ratio
	ratio := compressed.CompressionRatio()
	t.Logf("Compression ratio: %.2f", ratio)
	if ratio > 1.0 {
		t.Logf("Warning: compressed size larger than original")
	}
}

func TestVerifyLossless(t *testing.T) {
	// Generate random decimal data
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, 10000)
	for i := range data {
		// Generate values like 123.45
		data[i] = float64(rng.Intn(1000000)) / 100.0
	}

	compressed := CompressFloat64(data)

	if !VerifyLossless(data, compressed) {
		t.Error("compression was not lossless")
	}
}

func TestBitPacker(t *testing.T) {
	tests := []struct {
		name     string
		values   []uint64
		bitWidth uint8
	}{
		{"1-bit", []uint64{0, 1, 0, 1, 1, 0, 1, 0}, 1},
		{"2-bit", []uint64{0, 1, 2, 3, 2, 1, 0, 3}, 2},
		{"4-bit", []uint64{0, 5, 10, 15, 8, 3, 12, 7}, 4},
		{"8-bit", []uint64{0, 128, 255, 64, 192, 32, 96, 200}, 8},
		{"16-bit", []uint64{0, 1000, 30000, 65535, 12345}, 16},
		{"32-bit", []uint64{0, 100000, 4000000000, 2147483648}, 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packer := NewBitPacker(len(tt.values) * 8)
			packer.Pack(tt.values, tt.bitWidth)

			unpacker := NewBitUnpacker(packer.Bytes())
			result := unpacker.Unpack(len(tt.values), tt.bitWidth)

			for i := range tt.values {
				if result[i] != tt.values[i] {
					t.Errorf("mismatch at %d: got %d, want %d", i, result[i], tt.values[i])
				}
			}
		})
	}
}

func TestRDEncodeDecode(t *testing.T) {
	// High-precision values that don't compress well with standard ALP
	data := []float64{
		math.Pi,
		math.E,
		math.Phi,
		math.Sqrt2,
		math.SqrtE,
		1.4142135623730951,
		2.718281828459045,
		3.141592653589793,
	}

	cfg := DefaultConfig()
	encoder := NewRDEncoder(cfg)
	encoder.Sample(data)

	encoded := encoder.Encode(data)
	decoder := NewRDDecoder()
	decompressed := decoder.Decode(encoded)

	if len(decompressed) != len(data) {
		t.Fatalf("length mismatch: got %d, want %d", len(decompressed), len(data))
	}

	for i := range data {
		if decompressed[i] != data[i] {
			t.Errorf("value mismatch at %d: got %v, want %v", i, decompressed[i], data[i])
		}
	}
}

func TestSerializationRoundtrip(t *testing.T) {
	data := make([]float64, 2048)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	compressed := CompressFloat64(data)

	// Serialize
	var buf bytes.Buffer
	_, err := compressed.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	// Deserialize
	var loaded CompressedData
	_, err = loaded.ReadFrom(&buf)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}

	// Decompress
	decompressed := DecompressFloat64(&loaded)

	if len(decompressed) != len(data) {
		t.Fatalf("length mismatch: got %d, want %d", len(decompressed), len(data))
	}

	for i := range data {
		if decompressed[i] != data[i] {
			t.Errorf("value mismatch at %d: got %v, want %v", i, decompressed[i], data[i])
		}
	}
}

func TestEmptyInput(t *testing.T) {
	compressed := CompressFloat64(nil)
	if compressed.Scheme != SchemeInvalid {
		t.Error("expected invalid scheme for nil input")
	}

	compressed = CompressFloat64([]float64{})
	if compressed.Scheme != SchemeInvalid {
		t.Error("expected invalid scheme for empty input")
	}

	decompressed := DecompressFloat64(compressed)
	if len(decompressed) != 0 {
		t.Error("expected empty output for empty input")
	}
}

func TestBitsRequired(t *testing.T) {
	tests := []struct {
		n    uint64
		want int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{7, 3},
		{8, 4},
		{15, 4},
		{16, 5},
		{255, 8},
		{256, 9},
		{1<<16 - 1, 16},
		{1 << 16, 17},
		{1<<32 - 1, 32},
		{1 << 32, 33},
		{1<<63 - 1, 63},
		{1 << 63, 64},
	}

	for _, tt := range tests {
		got := bitsRequired(tt.n)
		if got != tt.want {
			t.Errorf("bitsRequired(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestLeadingZeros(t *testing.T) {
	tests := []struct {
		x    uint64
		want int
	}{
		{0, 64},
		{1, 63},
		{2, 62},
		{0x8000000000000000, 0},
		{0x4000000000000000, 1},
		{0x0000000100000000, 31},
		{0x00000000FFFFFFFF, 32},
	}

	for _, tt := range tests {
		got := leadingZeros64(tt.x)
		if got != tt.want {
			t.Errorf("leadingZeros64(0x%x) = %d, want %d", tt.x, got, tt.want)
		}
	}
}

func TestSchemeSelection(t *testing.T) {
	// Decimal-friendly data should use ALP
	decimalData := make([]float64, 1000)
	for i := range decimalData {
		decimalData[i] = float64(i) * 0.01
	}

	c1 := NewCompressor(DefaultConfig())
	scheme1 := c1.Analyze(decimalData)
	if scheme1 != SchemeALP {
		t.Errorf("expected SchemeALP for decimal data, got %v", scheme1)
	}

	// High-precision data should use ALP RD
	highPrecData := make([]float64, 1000)
	rng := rand.New(rand.NewSource(42))
	for i := range highPrecData {
		highPrecData[i] = rng.Float64() * 1e10
	}

	c2 := NewCompressor(DefaultConfig())
	scheme2 := c2.Analyze(highPrecData)
	t.Logf("Scheme for high-precision data: %v", scheme2)
	// Note: scheme selection depends on exception rate
}

// Benchmarks

func BenchmarkEncodeDecimal(b *testing.B) {
	// Monetary-style values
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	cfg := DefaultConfig()
	encoder := NewEncoder(cfg)
	encoder.Sample(data)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = encoder.EncodeVector(data)
	}
}

func BenchmarkDecodeDecimal(b *testing.B) {
	// Monetary-style values
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

func BenchmarkCompressDecompress(b *testing.B) {
	data := make([]float64, 10000)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		compressed := CompressFloat64(data)
		_ = DecompressFloat64(compressed)
	}
}

func BenchmarkBitPack(b *testing.B) {
	values := make([]uint64, VectorSize)
	for i := range values {
		values[i] = uint64(i % 256)
	}

	packer := NewBitPacker(VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		packer.Reset()
		packer.Pack(values, 8)
	}
}

func BenchmarkBitUnpack(b *testing.B) {
	values := make([]uint64, VectorSize)
	for i := range values {
		values[i] = uint64(i % 256)
	}

	packer := NewBitPacker(VectorSize)
	packer.Pack(values, 8)
	data := packer.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		unpacker := NewBitUnpacker(data)
		_ = unpacker.Unpack(VectorSize, 8)
	}
}

func BenchmarkRDEncode(b *testing.B) {
	// Random high-precision values
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = rng.Float64() * 1e10
	}

	cfg := DefaultConfig()
	encoder := NewRDEncoder(cfg)
	encoder.Sample(data)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = encoder.EncodeVector(data)
	}
}

func BenchmarkRDDecode(b *testing.B) {
	// Random high-precision values
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, VectorSize)
	for i := range data {
		data[i] = rng.Float64() * 1e10
	}

	cfg := DefaultConfig()
	encoder := NewRDEncoder(cfg)
	encoder.Sample(data)
	block := encoder.EncodeVector(data)

	decoder := NewRDDecoder()
	result := make([]float64, VectorSize)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		decoder.DecodeVector(block, result)
	}
}

// Test data generators for benchmarking different patterns

func generateMonetaryData(n int) []float64 {
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, n)
	for i := range data {
		data[i] = float64(rng.Intn(1000000)) / 100.0
	}
	return data
}

func generateSensorData(n int) []float64 {
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, n)
	base := 20.0 // base temperature
	for i := range data {
		data[i] = base + rng.Float64()*5.0 - 2.5
	}
	return data
}

func generateScientificData(n int) []float64 {
	rng := rand.New(rand.NewSource(42))
	data := make([]float64, n)
	for i := range data {
		data[i] = rng.Float64() * math.Pow(10, float64(rng.Intn(20)-10))
	}
	return data
}

func BenchmarkMonetaryData(b *testing.B) {
	data := generateMonetaryData(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed := CompressFloat64(data)
		_ = DecompressFloat64(compressed)
	}
}

func BenchmarkSensorData(b *testing.B) {
	data := generateSensorData(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed := CompressFloat64(data)
		_ = DecompressFloat64(compressed)
	}
}

func BenchmarkScientificData(b *testing.B) {
	data := generateScientificData(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compressed := CompressFloat64(data)
		_ = DecompressFloat64(compressed)
	}
}
