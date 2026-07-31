# ALP: Adaptive Lossless floating-Point Compression

A small Go implementation of the ALP compression algorithm for IEEE 754
`float64` data.

[![Go Reference](https://pkg.go.dev/badge/github.com/axiomhq/alp/v3.svg)](https://pkg.go.dev/github.com/axiomhq/alp/v3)

## Install

```bash
go get github.com/axiomhq/alp/v3
```

## Use

ALP operates on fixed blocks of 1,024 values. Sample representative data once,
then reuse the returned state for blocks with similar values.

```go
package main

import (
	"fmt"

	"github.com/axiomhq/alp/v3"
)

func main() {
	values := make([]float64, alp.VectorSize)
	for i := range values {
		values[i] = float64(i) / 100
	}

	state := alp.Sample(values)

	var vector alp.Vector
	alp.Encode(&vector, values, state)

	packed := alp.Pack(&vector)
	fmt.Printf("compressed to %d bytes\n", len(packed))

	var restored alp.Vector
	if err := alp.Unpack(&restored, packed); err != nil {
		panic(err)
	}

	decoded := make([]float64, alp.VectorSize)
	alp.Decode(decoded, &restored)
}
```

`Encode` and `Decode` do not allocate. `Pack` returns the serialized form of a
vector, while `PackedSize` reports its size before allocation.

## Migrating to v3

Version 3 is the consolidated fixed-vector API at
`github.com/axiomhq/alp/v3`. Existing root-module and `/v2` users remain on
their prior contracts until they explicitly migrate; v3 does not replace those
module paths.

The arbitrary-length `CompressFloat64`, `DecompressFloat64`, `Compressor`,
`Encoder`, `Decoder`, and stream-serialization APIs are not part of v3. Replace
compression with `Sample`, `Encode`, and `Pack`, and replace decompression with
`Unpack` and `Decode`, processing input in blocks of exactly 1,024 values.

Packed v3 frames include a magic value and format version. `Unpack` rejects
legacy bytes and unknown format versions instead of silently interpreting them
as v3 data. The v3 frame format is distinct and must not be passed to legacy
readers.

## Algorithm

ALP finds an exponent and factor that transform decimal floating-point values
into integers without losing their exact IEEE 754 representation. Each block is
then frame-of-reference encoded and bit-packed. Values that do not round-trip
through the integer transform, including NaNs, infinities, and negative zero,
are stored as exceptions using their original bits.

See [ALP: Adaptive Lossless floating-Point Compression](https://dl.acm.org/doi/10.1145/3626717)
for the algorithm described at SIGMOD 2024.

## License

MIT License. See [LICENSE](LICENSE).
