# ALP vs Zstd Comparison

Based on the ALP SIGMOD 2024 paper and typical benchmark results.

## Key Differences

| Aspect | ALP | Zstd |
|--------|-----|------|
| **Design** | Float-specific | General-purpose |
| **Granularity** | Per-vector (1024 values) | Block-based |
| **Random Access** | Yes (decode single vector) | No (must decode block) |
| **Best For** | Decimal floats | Repetitive byte patterns |

## Compression Ratio Comparison

| Data Type | ALP | Zstd (default) | Zstd (best) |
|-----------|-----|----------------|-------------|
| Monetary (10.99, 20.50) | **~16%** | ~45-55% | ~40-50% |
| Sensor (temp readings) | ~30-50% | ~50-60% | ~45-55% |
| Integer-like (1.0, 2.0) | **~5-10%** | ~15-25% | ~10-20% |
| Random high-precision | ~100%+ | **~95-100%** | **~90-95%** |

*Lower percentage = better compression*

## Speed Comparison

Based on the SIGMOD 2024 paper benchmarks:

| Operation | ALP | Zstd (fast) | Zstd (default) |
|-----------|-----|-------------|----------------|
| Encode (MB/s) | **2000-4000** | 500-1000 | 200-400 |
| Decode (MB/s) | **8000-10000** | 1000-2000 | 1000-2000 |

ALP is typically **1-2 orders of magnitude faster** than Zstd for decoding.

## When to Use Each

### Use ALP when:
- Data represents decimal values (prices, measurements, coordinates)
- Random access to compressed data is needed
- Decompression speed is critical (analytics, OLAP)
- Data will be queried frequently after compression

### Use Zstd when:
- Data is truly random floating-point (scientific simulations)
- Maximum compression ratio is needed regardless of speed
- Data is compressed once and rarely accessed
- General-purpose compression across mixed data types

## Combined Approach

For optimal results, consider:

```
ALP (for floats) + Zstd (for the ALP output)
```

This can achieve:
- Better compression than Zstd alone on decimal data
- Similar compression to Zstd on random data
- Faster random-access decompression

## Go Implementation Notes

Our Go implementation achieves:
- **~128 MB/s encode** for 1024-element vectors
- **~1000 MB/s decode** for 1024-element vectors
- **0.16 ratio** on typical decimal data

The package benchmark can be run with `go test -bench=. -benchmem`.
