package alp

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(random.Intn(1_000_000)) / 100
	}

	state := Sample(source)
	var encoded Vector
	Encode(&encoded, source, state)

	decoded := make([]float64, VectorSize)
	Decode(decoded, &encoded)
	assertEqualBits(t, source, decoded)
}

func TestEncodeDecodeSpecialValues(t *testing.T) {
	source := make([]float64, VectorSize)
	for i := range source {
		switch i % 8 {
		case 0:
			source[i] = 0
		case 1:
			source[i] = math.Copysign(0, -1)
		case 2:
			source[i] = math.Inf(1)
		case 3:
			source[i] = math.Inf(-1)
		case 4:
			source[i] = math.Float64frombits(0x7ff8000000000042)
		default:
			source[i] = float64(i) / 100
		}
	}

	state := Sample(source)
	var encoded Vector
	Encode(&encoded, source, state)

	decoded := make([]float64, VectorSize)
	Decode(decoded, &encoded)
	assertEqualBits(t, source, decoded)
}

func TestPackUnpack(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(random.Intn(1_000_000)) / 100
	}

	state := Sample(source)
	var encoded Vector
	Encode(&encoded, source, state)

	packed := Pack(&encoded)
	if got, want := len(packed), PackedSize(&encoded); got != want {
		t.Fatalf("packed size = %d, want %d", got, want)
	}

	var unpacked Vector
	if err := Unpack(&unpacked, packed); err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	decoded := make([]float64, VectorSize)
	Decode(decoded, &unpacked)
	assertEqualBits(t, source, decoded)
}

func TestPackUnpackAllExceptions(t *testing.T) {
	// The packed format must retain an exception count larger than one byte.
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = math.Float64frombits(0x7ff8000000000000 + uint64(i))
	}

	var encoded Vector
	Encode(&encoded, source, Sample(source))
	if got, want := encoded.NumExceptions, uint16(VectorSize); got != want {
		t.Fatalf("exception count = %d, want %d", got, want)
	}

	packed := Pack(&encoded)
	var unpacked Vector
	if err := Unpack(&unpacked, packed); err != nil {
		t.Fatalf("Unpack() error = %v", err)
	}
	if got, want := unpacked.NumExceptions, uint16(VectorSize); got != want {
		t.Fatalf("unpacked exception count = %d, want %d", got, want)
	}

	decoded := make([]float64, VectorSize)
	Decode(decoded, &unpacked)
	assertEqualBits(t, source, decoded)
}

func TestUnpackTruncatedInput(t *testing.T) {
	packed := Pack(testPackedVector())
	original := testDestinationVector()

	for prefix := 0; prefix < len(packed); prefix++ {
		dst := original
		err := Unpack(&dst, packed[:prefix])
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("prefix %d: Unpack() error = %v, want io.ErrUnexpectedEOF", prefix, err)
		}
		if dst != original {
			t.Fatalf("prefix %d: Unpack() modified destination", prefix)
		}
	}
}

func TestUnpackRejectsInvalidEncoding(t *testing.T) {
	valid := Pack(testPackedVector())
	packedBytes := (VectorSize*int(valid[packedBitWidthOffset]) + 7) / 8
	firstException := packedHeaderSize + packedBytes
	secondException := firstException + packedExceptionSize

	tests := []struct {
		name   string
		change func([]byte) []byte
		want   error
	}{
		{
			name: "bad magic",
			change: func(frame []byte) []byte {
				frame[0] ^= 0xff
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "unknown version",
			change: func(frame []byte) []byte {
				frame[packedVersionOffset] = packedFormatVersion + 1
				return frame
			},
			want: ErrUnsupportedVersion,
		},
		{
			name: "invalid width",
			change: func(frame []byte) []byte {
				frame[packedBitWidthOffset] = 65
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "invalid exponent",
			change: func(frame []byte) []byte {
				frame[packedExpOffset] = numExp
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "invalid factor",
			change: func(frame []byte) []byte {
				frame[packedFactorOffset] = numFact
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "invalid exception count",
			change: func(frame []byte) []byte {
				binary.LittleEndian.PutUint16(
					frame[packedExceptionOffset:packedHeaderSize],
					VectorSize+1,
				)
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "out of range exception position",
			change: func(frame []byte) []byte {
				binary.LittleEndian.PutUint16(frame[firstException:], VectorSize)
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "duplicate exception position",
			change: func(frame []byte) []byte {
				position := binary.LittleEndian.Uint16(frame[firstException:])
				binary.LittleEndian.PutUint16(frame[secondException:], position)
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "non-increasing exception position",
			change: func(frame []byte) []byte {
				binary.LittleEndian.PutUint16(frame[firstException:], 2)
				binary.LittleEndian.PutUint16(frame[secondException:], 1)
				return frame
			},
			want: ErrInvalidEncoding,
		},
		{
			name: "trailing data",
			change: func(frame []byte) []byte {
				return append(frame, 0)
			},
			want: ErrInvalidEncoding,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			frame := append([]byte(nil), valid...)
			frame = test.change(frame)
			original := testDestinationVector()
			dst := original
			err := Unpack(&dst, frame)
			if !errors.Is(err, test.want) {
				t.Fatalf("Unpack() error = %v, want %v", err, test.want)
			}
			if dst != original {
				t.Fatal("Unpack() modified destination")
			}
		})
	}
}

func TestCompressionRatio(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(random.Intn(1_000_000)) / 100
	}

	var encoded Vector
	Encode(&encoded, source, Sample(source))
	ratio := float64(PackedSize(&encoded)) / float64(VectorSize*8)
	if ratio > 0.6 {
		t.Fatalf("compression ratio %.2f%% exceeds 60%%", ratio*100)
	}
}

func TestSampleEmpty(t *testing.T) {
	if state := Sample(nil); state != (State{}) {
		t.Fatalf("Sample(nil) = %+v, want zero state", state)
	}
}

func TestEncodeRequiresFullVector(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Encode did not panic for a partial vector")
		}
	}()
	Encode(&Vector{}, make([]float64, VectorSize-1), State{})
}

func TestDecodeRequiresFullVector(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Decode did not panic for a partial vector")
		}
	}()
	Decode(make([]float64, VectorSize-1), &Vector{})
}

func assertEqualBits(t *testing.T, want, got []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("decoded length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		wantBits := math.Float64bits(want[i])
		gotBits := math.Float64bits(got[i])
		if gotBits != wantBits {
			t.Fatalf("value %d bits = %x, want %x", i, gotBits, wantBits)
		}
	}
}

func testPackedVector() *Vector {
	vector := &Vector{
		FrameOfRef:    -42,
		BitWidth:      5,
		Exp:           2,
		Factor:        1,
		NumExceptions: 2,
	}
	for i := range vector.Encoded {
		vector.Encoded[i] = int64(i % 32)
	}
	vector.ExceptionPos[0] = 3
	vector.ExceptionBits[0] = math.Float64bits(math.Inf(1))
	vector.ExceptionPos[1] = VectorSize - 1
	vector.ExceptionBits[1] = math.Float64bits(math.Copysign(0, -1))
	return vector
}

func testDestinationVector() Vector {
	return Vector{
		Encoded:       [VectorSize]int64{0: 99, VectorSize - 1: -99},
		FrameOfRef:    123,
		BitWidth:      7,
		Exp:           4,
		Factor:        3,
		NumExceptions: 1,
		ExceptionPos:  [VectorSize]uint16{0: 17},
		ExceptionBits: [VectorSize]uint64{0: 0x1234},
	}
}

func FuzzEncodeDecode(f *testing.F) {
	f.Add(int64(42), uint8(0))
	f.Add(int64(7), uint8(1))
	f.Add(int64(99), uint8(2))

	f.Fuzz(func(t *testing.T, seed int64, pattern uint8) {
		random := rand.New(rand.NewSource(seed))
		source := make([]float64, VectorSize)
		for i := range source {
			switch pattern % 3 {
			case 0:
				source[i] = float64(random.Intn(1_000_000)) / 100
			case 1:
				source[i] = random.Float64() * math.Pow10(random.Intn(20)-10)
			case 2:
				source[i] = math.Float64frombits(random.Uint64())
			}
		}

		var encoded Vector
		Encode(&encoded, source, Sample(source))
		packed := Pack(&encoded)
		var unpacked Vector
		if err := Unpack(&unpacked, packed); err != nil {
			t.Fatalf("Unpack() error = %v", err)
		}
		decoded := make([]float64, VectorSize)
		Decode(decoded, &unpacked)
		assertEqualBits(t, source, decoded)
	})
}

func FuzzUnpack(f *testing.F) {
	f.Add(Pack(testPackedVector()))
	f.Add([]byte(nil))

	f.Fuzz(func(t *testing.T, packed []byte) {
		var vector Vector
		if err := Unpack(&vector, packed); err != nil {
			return
		}
		decoded := make([]float64, VectorSize)
		Decode(decoded, &vector)
	})
}

func BenchmarkEncode(b *testing.B) {
	random := rand.New(rand.NewSource(42))
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(random.Intn(1_000_000)) / 100
	}
	state := Sample(source)
	var vector Vector

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(&vector, source, state)
	}
}

func BenchmarkDecode(b *testing.B) {
	random := rand.New(rand.NewSource(42))
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(random.Intn(1_000_000)) / 100
	}
	var vector Vector
	Encode(&vector, source, Sample(source))
	destination := make([]float64, VectorSize)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(destination, &vector)
	}
}
