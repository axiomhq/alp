# ALP: Adaptive Lossless floating-Point Compression

A small Go implementation of the ALP compression algorithm for IEEE 754
`float64` data.

[![Go Reference](https://pkg.go.dev/badge/github.com/axiomhq/alp.svg)](https://pkg.go.dev/github.com/axiomhq/alp)

## Install

```bash
go get github.com/axiomhq/alp
```

## Use

ALP operates on fixed blocks of 1,024 values. Sample representative data once,
then reuse the returned state for blocks with similar values.

```go
package main

import (
	"fmt"

	"github.com/axiomhq/alp"
)

func main() {
	values := make([]float64, alp.VectorSize)
	for i := range values {
		values[i] = float64(i) / 100
	}

	state := alp.Sample(values)

	var vector alp.Vector
	alp.Encode(&vector, values, state)

	packed := alp.Pack(nil, &vector)
	fmt.Printf("compressed to %d bytes\n", len(packed))

	var restored alp.Vector
	if _, err := alp.Unpack(&restored, packed); err != nil {
		panic(err)
	}

	decoded := make([]float64, alp.VectorSize)
	alp.Decode(decoded, &restored)
}
```

`Encode` and `Decode` do not allocate. `Pack` appends the serialized form of a
vector to a buffer, and `Unpack` returns an error on truncated or corrupt input.

## Algorithm

ALP finds an exponent and factor that transform decimal floating-point values
into integers without losing their exact IEEE 754 representation. Each block is
then frame-of-reference encoded and bit-packed, either as values or as
differences between neighbors, whichever packs narrower. Values that do not round-trip
through the integer transform, including NaNs, infinities, and negative zero,
are stored as exceptions using their original bits.

See [ALP: Adaptive Lossless floating-Point Compression](https://dl.acm.org/doi/10.1145/3626717)
for the algorithm described at SIGMOD 2024.

## License

MIT License. See [LICENSE](LICENSE).
