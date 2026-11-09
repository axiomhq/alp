package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/axiomhq/alp"
	"github.com/klauspost/compress/zstd"
)

const N = 1 << 20

func gen() map[string][]float64 {
	r := rand.New(rand.NewSource(42))
	m := map[string][]float64{}
	mon := make([]float64, N)
	sen := make([]float64, N)
	ints := make([]float64, N)
	rnd := make([]float64, N)
	t := 20.0
	for i := range N {
		mon[i] = float64(r.Intn(100000)) / 100
		t += float64(r.Intn(21)-10) / 10
		sen[i] = math.Round(t*10) / 10
		ints[i] = float64(r.Intn(1000))
		rnd[i] = math.Float64frombits(r.Uint64()&0x000fffffffffffff | 0x3ff0000000000000)
	}
	m["1 monetary (2 dp)"], m["2 sensor (random walk, 1 dp)"], m["3 integer-like"], m["4 random doubles"] = mon, sen, ints, rnd
	return m
}

func bench(f func()) time.Duration {
	best := time.Hour
	for range 5 {
		s := time.Now()
		f()
		if d := time.Since(s); d < best {
			best = d
		}
	}
	return best
}
func mbps(d time.Duration) float64 { return float64(N*8) / 1e6 / d.Seconds() }

func main() {
	zd, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderConcurrency(1))
	zb, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderConcurrency(1))
	zr, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	fmt.Println("| Data | ALP ratio | Zstd default | Zstd best | ALP enc MB/s | ALP dec MB/s | Zstd enc MB/s | Zstd dec MB/s |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	for _, name := range []string{"1 monetary (2 dp)", "2 sensor (random walk, 1 dp)", "3 integer-like", "4 random doubles"} {
		data := gen()[name]
		raw := make([]byte, N*8)
		for i, v := range data {
			binary.LittleEndian.PutUint64(raw[i*8:], math.Float64bits(v))
		}
		state := alp.Sample(data)
		var v alp.Vector
		var packed []byte
		encode := func() {
			packed = packed[:0]
			for i := 0; i < N; i += alp.VectorSize {
				alp.Encode(&v, data[i:i+alp.VectorSize], state)
				packed = alp.Pack(packed, &v)
			}
		}
		out := make([]float64, alp.VectorSize)
		decode := func() {
			for off := 0; off < len(packed); {
				n, err := alp.Unpack(&v, packed[off:])
				if err != nil {
					panic(err)
				}
				alp.Decode(out, &v)
				off += n
			}
		}
		encode()
		ae, ad := bench(encode), bench(decode)
		cd := zd.EncodeAll(raw, nil)
		cb := zb.EncodeAll(raw, nil)
		ze := bench(func() { zd.EncodeAll(raw, nil) })
		buf := make([]byte, 0, len(raw))
		zdec := bench(func() { buf, _ = zr.DecodeAll(cd, buf[:0]) })
		fmt.Printf("| %s | %.1f%% | %.1f%% | %.1f%% | %.0f | %.0f | %.0f | %.0f |\n", name,
			100*float64(len(packed))/float64(len(raw)), 100*float64(len(cd))/float64(len(raw)), 100*float64(len(cb))/float64(len(raw)),
			mbps(ae), mbps(ad), mbps(ze), mbps(zdec))
	}
}
