package alp

import (
	"errors"
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

	packed := Pack(nil, &encoded)
	if got, want := len(packed), PackedSize(&encoded); got != want {
		t.Fatalf("packed size = %d, want %d", got, want)
	}

	var unpacked Vector
	n, err := Unpack(&unpacked, packed)
	if err != nil || n != len(packed) {
		t.Fatalf("Unpack = %d, %v; want %d, nil", n, err, len(packed))
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

	packed := Pack(nil, &encoded)
	var unpacked Vector
	if _, err := Unpack(&unpacked, packed); err != nil {
		t.Fatal(err)
	}
	if got, want := unpacked.NumExceptions, uint16(VectorSize); got != want {
		t.Fatalf("unpacked exception count = %d, want %d", got, want)
	}

	decoded := make([]float64, VectorSize)
	Decode(decoded, &unpacked)
	assertEqualBits(t, source, decoded)
}

func TestUnpackRejectsCorruptInput(t *testing.T) {
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(i) / 100
	}
	source[3], source[700] = math.NaN(), math.Inf(1)
	var encoded Vector
	Encode(&encoded, source, Sample(source))
	if !encoded.Delta {
		t.Fatal("want a delta-mode vector so the base is covered")
	}
	packed := Pack(nil, &encoded)
	exceptions := len(packed) - 20 // two exceptions of 10 bytes each

	corrupt := func(edit func(b []byte) []byte) []byte {
		return edit(append([]byte(nil), packed...))
	}
	for name, src := range map[string][]byte{
		"short header":         packed[:packedHeaderSize-1],
		"truncated bits":       packed[:packedHeaderSize+1],
		"truncated exception":  packed[:len(packed)-1],
		"truncated delta base": packed[:packedHeaderSize+7],
		"bit width 65":         corrupt(func(b []byte) []byte { b[8] = 65; return b }),
		"exp out of range":     corrupt(func(b []byte) []byte { b[9] = numExp; return b }),
		"factor out of range":  corrupt(func(b []byte) []byte { b[10] = numFact; return b }),
		"too many exceptions":  corrupt(func(b []byte) []byte { b[11], b[12] = 0x01, 0x04; return b }),
		"position past block":  corrupt(func(b []byte) []byte { b[exceptions], b[exceptions+1] = 0x00, 0x04; return b }),
		"positions unordered":  corrupt(func(b []byte) []byte { b[exceptions+10], b[exceptions+11] = 3, 0; return b }),
	} {
		t.Run(name, func(t *testing.T) {
			var v Vector
			if _, err := Unpack(&v, src); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Unpack error = %v, want ErrCorrupt", err)
			}
		})
	}

	// Trailing bytes belong to the next vector and are not consumed.
	var v Vector
	if n, err := Unpack(&v, append(append([]byte(nil), packed...), 0xff)); err != nil || n != len(packed) {
		t.Fatalf("Unpack with trailing byte = %d, %v; want %d, nil", n, err, len(packed))
	}
}

func FuzzUnpack(f *testing.F) {
	source := make([]float64, VectorSize)
	for i := range source {
		source[i] = float64(i) / 100
	}
	source[5] = math.NaN()
	var encoded Vector
	Encode(&encoded, source, Sample(source))
	f.Add(Pack(nil, &encoded))
	f.Fuzz(func(t *testing.T, src []byte) {
		var v Vector
		if _, err := Unpack(&v, src); err == nil {
			Decode(make([]float64, VectorSize), &v)
		}
	})
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

func TestDeltaMode(t *testing.T) {
	ramp := make([]float64, VectorSize)
	for i := range ramp {
		ramp[i] = 1000 + float64(i)*0.25
	}
	noisy := make([]float64, VectorSize)
	random := rand.New(rand.NewSource(1))
	for i := range noisy {
		noisy[i] = float64(random.Intn(1_000_000)) / 100
	}
	// Exception and padding slots repeat the previous value: a ramp of step 25 gets deltas in {0, 25, 50}, 6 bits against 15 plain.
	withNaN := append([]float64(nil), ramp...)
	withNaN[0], withNaN[500], withNaN[VectorSize-1] = math.NaN(), math.NaN(), math.Inf(1)
	// Deltas that wrap int64 must still round-trip.
	wrapping := make([]float64, VectorSize)
	for i := range wrapping {
		wrapping[i] = []float64{9e15, -9e15}[i%2]
	}
	for _, tc := range []struct {
		name      string
		values    []float64
		wantDelta bool
		maxWidth  uint8
	}{
		{"ramp", ramp, true, 0},
		{"ramp_with_exceptions", withNaN, true, 6},
		{"noisy", noisy, false, 64},
		{"wrapping", wrapping, false, 64},
		{"short_ramp", ramp[:300], true, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var encoded Vector
			Encode(&encoded, tc.values, Sample(tc.values))
			if encoded.Delta != tc.wantDelta || encoded.BitWidth > tc.maxWidth {
				t.Fatalf("delta = %v, width = %d; want delta %v, width <= %d", encoded.Delta, encoded.BitWidth, tc.wantDelta, tc.maxWidth)
			}
			var unpacked Vector
			if _, err := Unpack(&unpacked, Pack(nil, &encoded)); err != nil {
				t.Fatal(err)
			}
			decoded := make([]float64, len(tc.values))
			Decode(decoded, &unpacked)
			assertEqualBits(t, tc.values, decoded)
		})
	}
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
	f.Add(int64(5), uint8(3))
	f.Add(int64(6), uint8(4))

	f.Fuzz(func(t *testing.T, seed int64, pattern uint8) {
		random := rand.New(rand.NewSource(seed))
		source := make([]float64, 1+random.Intn(VectorSize))
		step := float64(random.Intn(1000)) / 100
		for i := range source {
			switch pattern % 5 {
			case 3:
				source[i] = float64(i) * step
			case 4:
				source[i] = []float64{math.NaN(), 1e18, -1e18, 0.5}[random.Intn(4)]
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
		packed := Pack(nil, &encoded)
		var unpacked Vector
		if _, err := Unpack(&unpacked, packed); err != nil {
			t.Fatal(err)
		}
		decoded := make([]float64, len(source))
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
	for b.Loop() {
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
	for b.Loop() {
		Decode(destination, &vector)
	}
}
