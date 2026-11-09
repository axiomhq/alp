package alp_test

import (
	"fmt"

	"github.com/axiomhq/alp"
)

func Example() {
	values := make([]float64, alp.VectorSize)
	for i := range values {
		values[i] = float64(i) / 100
	}

	state := alp.Sample(values)

	var vector alp.Vector
	alp.Encode(&vector, values, state)
	packed := alp.Pack(nil, &vector)

	var restored alp.Vector
	if _, err := alp.Unpack(&restored, packed); err != nil {
		panic(err)
	}
	decoded := make([]float64, alp.VectorSize)
	alp.Decode(decoded, &restored)

	fmt.Println(len(packed) < alp.VectorSize*8, decoded[123] == values[123])
	// Output: true true
}
