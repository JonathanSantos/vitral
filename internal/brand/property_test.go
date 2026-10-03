package brand

import (
	"testing"

	"pgregory.net/rapid"
)

// MarkColor é estável (mesma ref, mesma cor) e cíclica de período 5.
func TestPropertyMarkColorCyclic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 1_000_000).Draw(t, "ref")
		fill, text := MarkColor(n)
		again, againText := MarkColor(n)
		next, nextText := MarkColor(n + 5)
		if fill != again || text != againText || fill != next || text != nextText {
			t.Fatalf("MarkColor(%d) not stable/cyclic", n)
		}
		if MarkIndex(n) < 1 || MarkIndex(n) > 5 {
			t.Fatalf("MarkIndex(%d) = %d", n, MarkIndex(n))
		}
	})
}
