package cursor

import (
	"math"
	"math/rand/v2"
	"testing"
)

// FuzzPlan: posições, alvos e parâmetros arbitrários (inclusive NaN, infinito, negativos e enormes)
// nunca geram panic, NaN ou plano vazio, e mantêm os invariantes da seção 9.
func FuzzPlan(f *testing.F) {
	f.Add(10.0, 10.0, 500.0, 400.0, 80.0, 30.0, uint64(1), uint8(0), uint8(0), uint8(0))
	f.Add(0.0, 0.0, 0.0, 0.0, 0.0, 0.0, uint64(2), uint8(1), uint8(1), uint8(1))
	f.Add(-500.0, 3000.0, 2500.0, -40.0, 1.0, 1.0, uint64(3), uint8(2), uint8(2), uint8(2))
	f.Add(1.0, 1.0, 1e300, -1e300, 1e9, 0.5, uint64(4), uint8(0), uint8(3), uint8(0))
	f.Add(math.NaN(), math.Inf(1), 10.0, 10.0, math.Inf(-1), math.NaN(), uint64(5), uint8(1), uint8(0), uint8(1))
	f.Fuzz(func(t *testing.T, fx, fy, tx, ty, tw, th float64, seed uint64, kind, speed, motion uint8) {
		p := DefaultProfile()
		p.Speed = []Speed{Normal, Slow, Fast, Instant}[speed%4]
		rng := rand.New(rand.NewPCG(seed, 11))
		target := Rect{tx, ty, tw, th}
		k := Kind(kind % 3)
		cands := Candidates(target, k, p, rng)
		if len(cands) != p.Tries+1 {
			t.Fatalf("got %d candidates", len(cands))
		}
		for _, c := range cands {
			if math.IsNaN(c.X) || math.IsNaN(c.Y) || math.IsInf(c.X, 0) || math.IsInf(c.Y, 0) {
				t.Fatalf("bad candidate %v", c)
			}
		}
		in := Input{
			From: Point{fx, fy}, To: cands[seed%uint64(len(cands))], Target: target, Kind: k,
			Motion: Motion(motion % 3), Side: int(seed%3) - 1, Correct: seed%7 == 0,
		}
		plan := Compute(in, p, rng)
		if msg := checkPlan(in, p, plan); msg != "" {
			t.Fatalf("%s\ninput %+v", msg, in)
		}
		if _, ok := FreeArea(target, []Rect{{tx + tw/2, ty, tw, th}}); ok && (tw <= 0 || th <= 0) {
			t.Fatal("free area in an empty target")
		}
	})
}
