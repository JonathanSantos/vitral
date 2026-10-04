package main

import (
	"strings"
	"testing"
)

func TestEvaluateTiming(t *testing.T) {
	rules := []TimingRule{{Match: "snapshot/large.html", MaxMs: 120}, {Match: "snapshot/*", MaxMs: 30}}
	rows, failures := EvaluateTiming(rules, map[string]float64{
		"snapshot/large.html": 119.9, "snapshot/form.html": 30, "snapshot/big-table.html": 30.1, "other": 1,
	})
	if len(rows) != 4 || len(failures) != 2 {
		t.Fatalf("rows %d failures %v", len(rows), failures)
	}
	want := map[string]bool{"other": false, "snapshot/big-table.html": false, "snapshot/form.html": true, "snapshot/large.html": true}
	for _, r := range rows {
		if r.OK != want[r.Key] {
			t.Errorf("%s ok=%v (%s)", r.Key, r.OK, r.Problem)
		}
	}
	if rows[0].Key != "other" || rows[0].Problem != "sem teto no thresholds.yaml" {
		t.Errorf("first row %+v", rows[0])
	}
	if rows[1].Max != 30 || !strings.Contains(rows[1].Problem, "30.1 ms > teto 30 ms") {
		t.Errorf("big-table row %+v", rows[1])
	}
	out := FormatTiming(rows)
	for _, s := range []string{"tempo (mediana)", "119.9 ms", "120 ms", "FALHA: 30.1 ms > teto 30 ms"} {
		if !strings.Contains(out, s) {
			t.Errorf("missing %q in:\n%s", s, out)
		}
	}
	if FormatTiming(nil) != "" {
		t.Error("no rows, no table")
	}
}

func TestParseTimingRules(t *testing.T) {
	_, err := ParseThresholds([]byte("module: m\ntiming:\n  - {match: \"snapshot/*\", maxMs: 0}\n"))
	if err == nil {
		t.Fatal("rule without maxMs must fail")
	}
	th, err := ParseThresholds([]byte("module: m\ntiming:\n  - {match: \"snapshot/*\", maxMs: 30}\n"))
	if err != nil || len(th.Timing) != 1 || th.Timing[0].MaxMs != 30 {
		t.Fatalf("%v %+v", err, th.Timing)
	}
}
