# ALP vs Zstd

Measured on 1,048,576 `float64` values per dataset (8 MiB raw), Go 1.25,
Intel Xeon @ 2.6 GHz, one core, best of 5 runs. Zstd is
[klauspost/compress](https://github.com/klauspost/compress) v1.20.3, applied to
the raw little-endian bytes. ALP encode includes `Encode` + `Pack`; ALP decode
includes `Unpack` + `Decode`. Throughput is raw (uncompressed) MB/s.

Reproduce: `cd doc/bench && go run .`

| Data | ALP size | Zstd default | Zstd best | ALP enc | ALP dec | Zstd enc | Zstd dec |
|---|---|---|---|---|---|---|---|
| Monetary, 2 decimals | **26.7%** | 43.1% | 37.4% | 759 | 1100 | 116 | 998 |
| Sensor random walk, 1 decimal | **8.1%** | 24.6% | 19.5% | 1216 | 2155 | 175 | 848 |
| Integer-valued | **15.8%** | 24.4% | 21.6% | 1069 | 1786 | 224 | 861 |
| Random doubles | 104.6% | 100.0% | **98.1%** | 369 | 562 | 882 | 5714 |

Size is a share of the raw size (lower is better); speeds are MB/s.

## What it shows

- **Decimal data:** ALP is 1.5x to 3x smaller than Zstd default, and encodes 5x to
  7x faster. Decode is about 1.1x to 2.5x faster, not orders of magnitude.
- **Random doubles:** ALP stores nearly everything as exceptions and grows the
  data by about 5%. Zstd is faster here and no worse in size. Fall back to raw
  or Zstd when `Pack` output exceeds `VectorSize*8` bytes.
- **Random access:** ALP decodes one 1,024-value vector independently once you know its
  offset (the format has no index; store offsets yourself). Zstd must decode a
  whole frame.

## Use ALP when

- Values are decimals: prices, measurements, coordinates.
- You need to decode single vectors or scan fast.

## Use Zstd when

- Values are arbitrary doubles, such as simulation output.
- Data is mixed and not float-only.

## Package benchmarks

`go test -bench=. -benchmem` on the same machine: `Encode` about 3.4 µs per
vector (about 2.4 GB/s), `Decode` about 0.46 µs per vector (about 18 GB/s),
both with zero allocations. These exclude `Pack` and `Unpack`.
