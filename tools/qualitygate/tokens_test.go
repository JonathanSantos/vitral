package main

import (
	"strings"
	"testing"
)

func TestEvaluateTokens(t *testing.T) {
	rules := []TokenRule{
		{Match: "snapshot/large.html", Max: 150},
		{Match: "action/*/*-open", Max: 280},
		{Match: "action/*", Max: 100}, // não casa com chaves de 3 níveis
		{Match: "action/*/*", Max: 300},
	}
	current := map[string]int{
		"snapshot/large.html":   160,
		"action/form/01-open":   229,
		"action/form/02-type":   15,
		"snapshot/unknown.html": 10,
	}
	base := map[string]int{"snapshot/large.html": 119, "action/form/02-type": 20}
	rows, failures := EvaluateTokens(rules, current, base)
	if len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
	joined := strings.Join(failures, "\n")
	for _, want := range []string{"snapshot/large.html: 160 > teto 150", "snapshot/unknown.html: sem teto"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing failure %q in %v", want, failures)
		}
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %v", failures)
	}
	out := FormatTokens(rows)
	for _, want := range []string{"+41", "-5", "280", "atual", "base"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
