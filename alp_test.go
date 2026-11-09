package alp

import (
	"bytes"
	"errors"
	"math"
	"math/rand"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	source := randomDecimals(42, VectorSize)

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
	source := randomDecimals(42, VectorSize)

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
	source := randomDecimals(42, VectorSize)

	var encoded Vector
	Encode(&encoded, source, Sample(source))
	ratio := float64(PackedSize(&encoded)) / float64(VectorSize*8)
	if ratio > 0.35 {
		t.Fatalf("compression ratio %.2f%% exceeds 35%%", ratio*100)
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
	noisy := randomDecimals(1, VectorSize)
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

// randomDecimals returns n deterministic values with two decimal places.
func randomDecimals(seed int64, n int) []float64 {
	random := rand.New(rand.NewSource(seed))
	values := make([]float64, n)
	for i := range values {
		values[i] = float64(random.Intn(1_000_000)) / 100
	}
	return values
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
	for pattern := range uint8(5) {
		f.Add(int64(42), pattern)
	}

	f.Fuzz(func(t *testing.T, seed int64, pattern uint8) {
		random := rand.New(rand.NewSource(seed))
		source := make([]float64, 1+random.Intn(VectorSize))
		step := float64(random.Intn(1000)) / 100
		for i := range source {
			switch pattern % 5 {
			case 0: // two-decimal values
				source[i] = float64(random.Intn(1_000_000)) / 100
			case 1: // random magnitudes
				source[i] = random.Float64() * math.Pow10(random.Intn(20)-10)
			case 2: // arbitrary bit patterns
				source[i] = math.Float64frombits(random.Uint64())
			case 3: // ramp
				source[i] = float64(i) * step
			case 4: // NaN and out-of-range values
				source[i] = []float64{math.NaN(), 1e18, -1e18, 0.5}[random.Intn(4)]
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
	source := randomDecimals(42, VectorSize)
	state := Sample(source)
	var vector Vector

	b.ReportAllocs()
	for b.Loop() {
		Encode(&vector, source, state)
	}
}

func BenchmarkDecode(b *testing.B) {
	source := randomDecimals(42, VectorSize)
	var vector Vector
	Encode(&vector, source, Sample(source))
	destination := make([]float64, VectorSize)

	b.ReportAllocs()
	for b.Loop() {
		Decode(destination, &vector)
	}
}

// naivePack is the original bit-at-a-time layout, kept as the wire-format reference.
func naivePack(src []int64, width uint) []byte {
	out := make([]byte, (VectorSize*int(width)+7)/8)
	pos := uint(0)
	for _, s := range src {
		for b := range width {
			if uint64(s)>>b&1 != 0 {
				out[pos/8] |= 1 << (pos % 8)
			}
			pos++
		}
	}
	return out
}

func TestPackBitsAllWidths(t *testing.T) {
	random := rand.New(rand.NewSource(3))
	for width := uint(0); width <= 64; width++ {
		src := make([]int64, VectorSize)
		for i := range src {
			src[i] = int64(random.Uint64() & (^uint64(0) >> (64 - max(width, 1))))
			if width == 0 {
				src[i] = 0
			}
		}
		want := naivePack(src, width)
		got := make([]byte, len(want))
		packBits(got, src, uint8(width))
		if !bytes.Equal(got, want) {
			t.Fatalf("width %d: packed bytes differ from reference layout", width)
		}
		back := make([]int64, VectorSize)
		for i := range back {
			back[i] = -1
		}
		unpackBits(back, got, uint8(width))
		for i := range src {
			if back[i] != src[i] {
				t.Fatalf("width %d value %d = %d, want %d", width, i, back[i], src[i])
			}
		}
	}
}

func TestNegativeAndPartialDecode(t *testing.T) {
	source := []float64{-1.25, -3.5, -0.01, 7.75, math.NaN(), -0.5, math.Inf(-1), 2}
	var encoded Vector
	Encode(&encoded, source, Sample(source))
	decoded := make([]float64, len(source))
	Decode(decoded, &encoded)
	assertEqualBits(t, source, decoded)

	// Decoding a shorter prefix must ignore exceptions beyond it.
	short := make([]float64, 4)
	Decode(short, &encoded)
	assertEqualBits(t, source[:4], short)
}
