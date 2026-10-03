package tokens

import "testing"

func TestEstimate(t *testing.T) {
	tests := map[string]int{"": 0, "a": 1, "abcd": 1, "abcde": 2, "çã": 1, "12345678": 2}
	for in, want := range tests {
		if got := Estimate(in); got != want {
			t.Errorf("Estimate(%q)=%d want %d", in, got, want)
		}
	}
}
