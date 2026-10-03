package brand

import (
	"image/color"
	"strings"
	"testing"
)

func TestMarkColor(t *testing.T) {
	tests := []struct {
		ref        int
		fill, text color.RGBA
	}{
		{1, Sapphire, White},
		{2, Ruby, Ink},
		{3, Emerald, Ink},
		{4, Amber, Ink},
		{5, Amethyst, White},
		{6, Sapphire, White},
		{12, Ruby, Ink},
		{0, Sapphire, White},
	}
	for _, tt := range tests {
		fill, text := MarkColor(tt.ref)
		if fill != tt.fill || text != tt.text {
			t.Errorf("MarkColor(%d) = %v/%v, want %v/%v", tt.ref, fill, text, tt.fill, tt.text)
		}
	}
}

func TestMarkIndex(t *testing.T) {
	for ref, want := range map[int]int{1: 1, 5: 5, 6: 1, 12: 2, 0: 1} {
		if got := MarkIndex(ref); got != want {
			t.Errorf("MarkIndex(%d)=%d want %d", ref, got, want)
		}
	}
}

func TestRefNumberAndHex(t *testing.T) {
	cases := map[string]int{"e1": 1, "e42": 42, "x3": 0, "e": 0, "": 0}
	for in, want := range cases {
		if got := RefNumber(in); got != want {
			t.Errorf("RefNumber(%q)=%d want %d", in, got, want)
		}
	}
	if Hex(Ink) != "#1e1b2e" || Hex(Paper) != "#f7f5f0" {
		t.Fatal("hex mismatch")
	}
	css := CSSVars()
	for _, want := range []string{"--vitral-ink:#1e1b2e;", "--vitral-mark5:#8e4ec6;", "--vitral-paper:#f7f5f0;"} {
		if !strings.Contains(css, want) {
			t.Errorf("missing %s in %s", want, css)
		}
	}
}
