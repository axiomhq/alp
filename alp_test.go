package alp

import (
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
	Unpack(&unpacked, packed)
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
	Unpack(&unpacked, packed)
	if got, want := unpacked.NumExceptions, uint16(VectorSize); got != want {
		t.Fatalf("unpacked exception count = %d, want %d", got, want)
	}

	decoded := make([]float64, VectorSize)
	Decode(decoded, &unpacked)
	assertEqualBits(t, source, decoded)
}

func TestUnpackTruncatedInput(t *testing.T) {
	var vector Vector
	vector.FrameOfRef = 42
	vector.NumExceptions = 7
	Unpack(&vector, []byte{1, 2, 3})
	if vector != (Vector{}) {
		t.Fatal("truncated input did not clear destination")
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
		Unpack(&unpacked, packed)
		decoded := make([]float64, VectorSize)
		Decode(decoded, &unpacked)
		assertEqualBits(t, source, decoded)
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
