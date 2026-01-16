# ALP: Adaptive Lossless floating-Point Compression

A Go implementation of the ALP compression algorithm for IEEE 754 floating-point data.

[![Go Reference](https://pkg.go.dev/badge/github.com/axiomhq/alp.svg)](https://pkg.go.dev/github.com/axiomhq/alp)

## Overview

ALP (Adaptive Lossless floating-Point compression) is a state-of-the-art lossless compression algorithm specifically designed for floating-point data. It exploits two common patterns in real-world datasets:

1. **Decimal Floating-Point Numbers**: Most floats in real-world data represent decimal values (like prices, measurements, etc.). ALP converts these to integers by multiplying by powers of 10, then applies Frame-of-Reference encoding.

2. **High-Precision Floating-Point Numbers**: For values that don't fit the decimal pattern, ALP RD (Real Doubles) splits the IEEE 754 representation and uses dictionary encoding.

## Installation

```bash
go get github.com/axiomhq/alp
```

## Usage

### Simple API

```go
package main

import (
    "fmt"
    "github.com/axiomhq/alp"
)

func main() {
    // Sample data
    data := []float64{10.99, 20.50, 15.75, 99.99, 0.01}

    // Compress
    compressed := alp.CompressFloat64(data)

    // Decompress
    decompressed := alp.DecompressFloat64(compressed)

    fmt.Printf("Compression ratio: %.2f\n", compressed.CompressionRatio())
    fmt.Printf("Lossless: %v\n", alp.VerifyLossless(data, compressed))
}
```

### Advanced API

For better performance with large datasets, reuse the compressor:

```go
cfg := alp.DefaultConfig()
compressor := alp.NewCompressor(cfg)

// Analyze once per row group
compressor.Analyze(data)

// Compress multiple vectors
compressed := compressor.Compress(data)
```

### Low-Level API

For fine-grained control:

```go
cfg := alp.DefaultConfig()
encoder := alp.NewEncoder(cfg)

// Sample to find optimal parameters
encoder.Sample(data)

// Encode blocks
block := encoder.Encode(data)

// Decode
decoder := alp.NewDecoder()
result := decoder.Decode(block)
```

### Serialization

```go
// Serialize
var buf bytes.Buffer
compressed.WriteTo(&buf)

// Deserialize
var loaded alp.CompressedData
loaded.ReadFrom(&buf)
```

## Algorithm Details

### ALP Encoding

1. **Sampling**: Analyze a subset of values to find optimal encoding parameters
2. **Parameter Selection**: Find the best exponent E such that `round(value × 10^E)` produces integers that roundtrip exactly
3. **Encoding**: Convert floats to integers: `encoded = round(value × 10^E)`
4. **Frame-of-Reference**: Subtract the minimum value and bit-pack the results
5. **Exception Handling**: Store values that don't roundtrip as raw bits

### ALP RD Encoding

For high-precision values that don't fit the decimal pattern:

1. **Bit Splitting**: Split IEEE 754 representation into left (upper) and right (lower) parts
2. **Dictionary Building**: Create a dictionary of common left parts
3. **Encoding**: Store dictionary indices for left parts, raw values for right parts
4. **Exception Handling**: Store uncommon left parts separately

## Performance

The implementation includes several optimizations:

- **Cache-friendly processing**: Data processed in chunks of 8 for better cache utilization
- **Vectorization-ready**: Code structure allows Go compiler to auto-vectorize hot loops
- **Minimal allocations**: Reusable encoders/decoders to reduce GC pressure
- **Fast bit manipulation**: Optimized leading zeros and bit packing operations

### Benchmarks

```
BenchmarkEncodeDecimal-16         148245     7861 ns/op    8368 B/op     4 allocs/op
BenchmarkDecodeDecimal-16        1217095     1012 ns/op       0 B/op     0 allocs/op
BenchmarkRDEncode-16               47428    24656 ns/op   40992 B/op    20 allocs/op
BenchmarkRDDecode-16              354829     3434 ns/op       0 B/op     0 allocs/op
```

## Configuration

```go
type Config struct {
    RowGroupSize     int // Number of values per row group (default: 102400)
    SamplesPerVector int // Samples per vector for analysis (default: 32)
    TopKCombinations int // Factor/exponent pairs to evaluate (default: 5)
}
```

## References

- [ALP: Adaptive Lossless floating-Point Compression](https://ir.cwi.nl/pub/33334) - SIGMOD 2024
- [Original C++ Implementation](https://github.com/cwida/ALP)

## License

MIT License - see [LICENSE](LICENSE) for details.
