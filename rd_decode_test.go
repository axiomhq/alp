package alp

import (
	"encoding/binary"
	"math"
	"strconv"
	"testing"
)

func TestDecodeRD48(t *testing.T) {
	const (
		prefix0 = uint16(0x3ff0)
		prefix1 = uint16(0xbff0)
	)

	for _, n := range []int{0, 1, 7, 8, 9, 15, 16, 1023, 1024} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			leftCodes := make([]byte, (n+7)/8)
			rightParts := make([]byte, n*6)
			want := make([]float64, n)
			for i := 0; i < n; i++ {
				code := uint16(i>>1) & 1
				leftCodes[i>>3] |= byte(code) << (i & 7)
				right := uint64(i) * 0x9e3779b97f4a & (uint64(1)<<48 - 1)
				offset := i * 6
				binary.LittleEndian.PutUint32(rightParts[offset:], uint32(right))
				binary.LittleEndian.PutUint16(rightParts[offset+4:], uint16(right>>32))
				prefix := prefix0
				if code != 0 {
					prefix = prefix1
				}
				want[i] = math.Float64frombits(uint64(prefix)<<48 | right)
			}

			got := make([]float64, n)
			DecodeRD48(got, leftCodes, rightParts, [2]uint16{prefix0, prefix1})
			for i := range want {
				if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
					t.Fatalf("value %d: got %016x, want %016x",
						i, math.Float64bits(got[i]), math.Float64bits(want[i]))
				}
			}
		})
	}
}

func TestDecodeRD48ShortInput(t *testing.T) {
	tests := []struct {
		name        string
		left, right []byte
	}{
		{name: "left", right: make([]byte, 6)},
		{name: "right", left: make([]byte, 1), right: make([]byte, 5)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("DecodeRD48 did not panic")
				}
			}()
			DecodeRD48(make([]float64, 1), test.left, test.right, [2]uint16{})
		})
	}
}

func BenchmarkDecodeRD48(b *testing.B) {
	const count = 64 * 1024
	leftCodes := make([]byte, count/8)
	rightParts := make([]byte, count*6)
	for i := 0; i < count; i++ {
		leftCodes[i>>3] |= byte(i>>1&1) << (i & 7)
		right := uint64(i) * 0x9e3779b97f4a & (uint64(1)<<48 - 1)
		offset := i * 6
		binary.LittleEndian.PutUint32(rightParts[offset:], uint32(right))
		binary.LittleEndian.PutUint16(rightParts[offset+4:], uint16(right>>32))
	}
	dst := make([]float64, count)

	b.SetBytes(int64(len(dst) * 8))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecodeRD48(dst, leftCodes, rightParts, [2]uint16{0x3ff0, 0xbff0})
	}
}
