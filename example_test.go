package alp_test

import (
	"bytes"
	"fmt"

	"github.com/axiomhq/alp"
)

func Example() {
	// Create some sample data (monetary values)
	data := []float64{
		10.99, 20.50, 15.75, 99.99, 0.01, 1234.56, 7890.12, 45.67,
	}

	// Compress
	compressed := alp.CompressFloat64(data)

	// Decompress
	decompressed := alp.DecompressFloat64(compressed)

	// Verify
	fmt.Println("Original:", data)
	fmt.Println("Decompressed:", decompressed)
	fmt.Printf("Compression ratio: %.2f\n", compressed.CompressionRatio())
	// Output:
	// Original: [10.99 20.5 15.75 99.99 0.01 1234.56 7890.12 45.67]
	// Decompressed: [10.99 20.5 15.75 99.99 0.01 1234.56 7890.12 45.67]
	// Compression ratio: 1.23
}

func ExampleCompressor() {
	// For large datasets, reuse the compressor
	cfg := alp.DefaultConfig()
	compressor := alp.NewCompressor(cfg)

	// Sample data to find optimal parameters
	data := make([]float64, 10000)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	// Analyze once, compress many times
	scheme := compressor.Analyze(data)
	fmt.Printf("Selected scheme: %v\n", scheme)

	compressed := compressor.Compress(data)
	fmt.Printf("Compressed %d values to ratio %.2f\n", len(data), compressed.CompressionRatio())
	// Output:
	// Selected scheme: 1
	// Compressed 10000 values to ratio 0.16
}

func ExampleEncoder() {
	// Low-level API for fine-grained control
	cfg := alp.DefaultConfig()
	encoder := alp.NewEncoder(cfg)

	// Sample to find optimal encoding parameters
	data := []float64{1.5, 2.5, 3.5, 4.5, 5.5}
	encoder.Sample(data)

	// Encode a block
	block := encoder.Encode(data)
	fmt.Printf("Encoded %d values, bit width: %d\n", len(block.Encoded), block.BitWidth)
	fmt.Printf("Exceptions: %d\n", len(block.ExceptionPos))

	// Decode
	decoder := alp.NewDecoder()
	result := decoder.Decode(block)
	fmt.Println("Decoded:", result)
	// Output:
	// Encoded 5 values, bit width: 6
	// Exceptions: 0
	// Decoded: [1.5 2.5 3.5 4.5 5.5]
}

func ExampleRDEncoder() {
	// Use RD encoder for high-precision data
	cfg := alp.DefaultConfig()
	encoder := alp.NewRDEncoder(cfg)

	// High-precision values
	data := []float64{3.141592653589793, 2.718281828459045, 1.4142135623730951}
	encoder.Sample(data)

	// Encode
	block := encoder.Encode(data)
	fmt.Printf("Cutting position: %d\n", block.CuttingPos)
	fmt.Printf("Dictionary size: %d\n", len(block.Dictionary))

	// Decode
	decoder := alp.NewRDDecoder()
	result := decoder.Decode(block)

	// Verify lossless
	for i, v := range data {
		if result[i] != v {
			fmt.Println("Mismatch!")
		}
	}
	fmt.Println("Compression verified lossless")
	// Output:
	// Cutting position: 1
	// Dictionary size: 3
	// Compression verified lossless
}

func ExampleCompressedData_serialization() {
	// Serialize compressed data for storage/transmission
	data := make([]float64, 1000)
	for i := range data {
		data[i] = float64(i) * 0.01
	}

	compressed := alp.CompressFloat64(data)

	// Serialize to bytes
	var buf bytes.Buffer
	n, _ := compressed.WriteTo(&buf)
	fmt.Printf("Serialized to %d bytes\n", n)

	// Deserialize
	var loaded alp.CompressedData
	loaded.ReadFrom(&buf)

	// Decompress
	result := alp.DecompressFloat64(&loaded)
	fmt.Printf("Decompressed %d values\n", len(result))
	// Output:
	// Serialized to 1276 bytes
	// Decompressed 1000 values
}

func ExampleVerifyLossless() {
	// Verify that compression is truly lossless
	data := []float64{1.1, 2.2, 3.3, 4.4, 5.5}
	compressed := alp.CompressFloat64(data)

	if alp.VerifyLossless(data, compressed) {
		fmt.Println("Compression is lossless")
	} else {
		fmt.Println("Compression lost data!")
	}
	// Output:
	// Compression is lossless
}
