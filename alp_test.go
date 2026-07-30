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

func TestBitPackerAllWidths(t *testing.T) {
	lengths := []int{1, 2, 7, 8, 9, 63, 64, 65, VectorSize}
	for bitWidth := uint8(1); bitWidth <= 64; bitWidth++ {
		for _, n := range lengths {
			values := make([]uint64, n)
			mask := ^uint64(0)
			if bitWidth < 64 {
				mask = (uint64(1) << bitWidth) - 1
			}
			for i := range values {
				// Deterministic values that exercise both sparse and high bits.
				values[i] = (uint64(i)*0x9e3779b97f4a7c15 ^ uint64(n)) & mask
			}

			packer := NewBitPacker((n*int(bitWidth) + 7) / 8)
			packer.Pack(values, bitWidth)
			got := NewBitUnpacker(packer.Bytes()).Unpack(n, bitWidth)
			for i := range values {
				if got[i] != values[i] {
					t.Fatalf("width %d length %d index %d: got %d, want %d", bitWidth, n, i, got[i], values[i])
				}
			}
		}
	}
}

func TestDecodePackedAllWidths(t *testing.T) {
	decoder := NewDecoder()
	lengths := []int{1, 2, 3, 4, 7, 8, 9, 63, 64, 65}
	for bitWidth := uint8(0); bitWidth <= 64; bitWidth++ {
		for _, count := range lengths {
			mask := ^uint64(0)
			if bitWidth < 64 {
				mask = (uint64(1) << bitWidth) - 1
			}
			encoded := make([]int64, count)
			for i := range encoded {
				encoded[i] = int64((uint64(i) * 0x9e3779b97f4a7c15) & mask)
			}
			exceptionPos := count / 2
			block := &EncodedBlock{
				Encoded:      encoded,
				FrameOfRef:   -17,
				BitWidth:     bitWidth,
				ExceptionPos: []uint16{uint16(exceptionPos)},
				Exceptions:   []uint64{math.Float64bits(math.Pi)},
			}
			packed := PackBlock(block)
			result := make([]float64, count)
			if got := decoder.decodePackedInto(packed, result); got != count {
				t.Fatalf("width %d length %d: decoded %d values, want %d", bitWidth, count, got, count)
			}

			for i := range result {
				want := float64(encoded[i] - 17)
				if i == exceptionPos {
					want = math.Pi
				}
				if math.Float64bits(result[i]) != math.Float64bits(want) {
					t.Fatalf("width %d length %d index %d: got %v, want %v", bitWidth, count, i, result[i], want)
				}
			}
		}
	}
}

func TestAbsolutePackingMatchesCanonical(t *testing.T) {
	sequential := make([]float64, VectorSize)
	withExceptions := make([]float64, VectorSize)
	for i := range sequential {
		sequential[i] = float64(i) * 0.01
		withExceptions[i] = sequential[i]
		if i%100 == 0 {
			withExceptions[i] = math.Pi + float64(i)
		}
	}

	patterns := [][]float64{
		sequential,
		generateMonetaryData(VectorSize),
		withExceptions,
	}
	for patternIndex, data := range patterns {
		encoder := NewEncoder(DefaultConfig())
		encoder.Sample(data)
		canonical := PackBlock(encoder.EncodeVector(data))
		absoluteBlock := encoder.encodeVectorAbsoluteInto(data, make([]int64, VectorSize))
		fused := packBlockAbsolute(absoluteBlock)

		if canonical.Count != fused.Count || canonical.FrameOfRef != fused.FrameOfRef ||
			canonical.BitWidth != fused.BitWidth || canonical.Exp != fused.Exp || canonical.Factor != fused.Factor ||
			!bytes.Equal(canonical.Data, fused.Data) {
			t.Fatalf("pattern %d: fused block metadata or data differs from canonical block", patternIndex)
		}
		if len(canonical.ExceptionPos) != len(fused.ExceptionPos) || len(canonical.Exceptions) != len(fused.Exceptions) {
			t.Fatalf("pattern %d: fused exception lengths differ", patternIndex)
		}
		for i := range canonical.ExceptionPos {
			if canonical.ExceptionPos[i] != fused.ExceptionPos[i] || canonical.Exceptions[i] != fused.Exceptions[i] {
				t.Fatalf("pattern %d exception %d differs", patternIndex, i)
			}
		}
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

func TestDecompressIntoRaw(t *testing.T) {
	values := []float64{1.25, math.Copysign(0, -1), math.Inf(1), math.NaN()}
	raw := make([]uint64, len(values))
	for i, value := range values {
		raw[i] = math.Float64bits(value)
	}
	compressed := &CompressedData{Scheme: SchemeRaw, Count: len(values), RawData: raw}
	dst := make([]float64, len(values)-1)

	written := NewDecompressor().DecompressInto(compressed, dst)
	if written != len(dst) {
		t.Fatalf("wrote %d values, want %d", written, len(dst))
	}
	for i := range dst {
		if math.Float64bits(dst[i]) != raw[i] {
			t.Fatalf("value %d: got bits %x, want %x", i, math.Float64bits(dst[i]), raw[i])
		}
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

func TestFindBestCombinationPruningMatchesExhaustive(t *testing.T) {
	patterns := [][]float64{
		generateMonetaryData(VectorSize),
		generateSensorData(VectorSize),
		generateScientificData(VectorSize),
		{0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(), math.Pi, -123.45},
	}

	encoder := NewEncoder(DefaultConfig())
	for patternIndex, data := range patterns {
		samples := encoder.sampleData(data)
		got := encoder.findBestCombination(samples)

		var want factorExponentPair
		bestScore := 0
		found := false
		for exp := uint8(0); exp <= float64MaxExponent; exp++ {
			for factor := uint8(0); factor <= exp && int(factor) < len(float64FactArr); factor++ {
				exceptions, minVal, maxVal := encoder.analyzeWithExpFactor(samples, exp, factor)
				if exceptions > len(samples)/2 {
					continue
				}
				bitWidth := 0
				if maxVal > minVal {
					bitWidth = bitsRequired(uint64(maxVal - minVal))
				}
				score := exceptions*80 + bitWidth*len(samples)/8
				if !found || score < bestScore {
					want = factorExponentPair{exp: exp, factor: factor, exceptions: exceptions}
					bestScore = score
					found = true
				}
			}
		}
		if !found {
			want = factorExponentPair{exp: 0, factor: 0, exceptions: len(samples)}
		}

		if got != want {
			t.Fatalf("pattern %d: pruned result %+v, exhaustive result %+v", patternIndex, got, want)
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

var benchmarkBitUnpackResult []uint64

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
		benchmarkBitUnpackResult = unpacker.Unpack(VectorSize, 8)
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
