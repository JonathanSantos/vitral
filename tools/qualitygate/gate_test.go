package main

import (
	"strings"
	"testing"
	"time"
)

const mod = "example.com/m"

func TestParseProfilesDedupes(t *testing.T) {
	unit := `mode: atomic
example.com/m/internal/a/a.go:1.1,2.2 3 1
example.com/m/internal/a/a.go:3.1,4.2 2 0
example.com/m/internal/b/b.go:1.1,2.2 5 0
`
	// Integração com -coverpkg repete os blocos; um deles agora está coberto.
	integration := `mode: atomic
example.com/m/internal/a/a.go:1.1,2.2 3 0
example.com/m/internal/a/a.go:3.1,4.2 2 7
example.com/m/internal/a/a.go:3.1,4.2 2 0
`
	got, err := ParseProfiles(mod, strings.NewReader(unit), strings.NewReader(integration))
	if err != nil {
		t.Fatal(err)
	}
	if got["internal/a"] != (Coverage{Statements: 5, Covered: 5}) {
		t.Fatalf("internal/a = %+v", got["internal/a"])
	}
	if got["internal/b"] != (Coverage{Statements: 5, Covered: 0}) {
		t.Fatalf("internal/b = %+v", got["internal/b"])
	}
	if _, err := ParseProfiles(mod, strings.NewReader("lixo")); err == nil {
		t.Fatal("invalid line must fail")
	}
	if (Coverage{}).Percent() != 100 {
		t.Fatal("empty package counts as fully covered")
	}
}

func TestParseThresholds(t *testing.T) {
	ok := "module: x\ncoverage:\n  internal/a: {criterion: unit, min: 85}\n  cmd/x: {criterion: none}\n"
	if _, err := ParseThresholds([]byte(ok)); err != nil {
		t.Fatal(err)
	}
	bad := []string{
		"coverage: {}",
		"module: x\ncoverage:\n  internal/a: {criterion: half, min: 85}\n",
		"module: x\ncoverage:\n  internal/a: {criterion: unit}\n",
		"module: x\ncoverage:\n  internal/a: {criterion: unit, min: 80, target: 70}\n",
	}
	for _, b := range bad {
		if _, err := ParseThresholds([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
}

func TestEvaluate(t *testing.T) {
	th := Thresholds{Module: mod, Coverage: map[string]PackageThreshold{
		"internal/pure":    {Criterion: CriterionUnit, Min: 85},
		"internal/browser": {Criterion: CriterionCombined, Min: 70},
		"internal/glue":    {Criterion: CriterionNone},
		"cmd/x":            {Criterion: CriterionNone},
	}}
	unit := map[string]Coverage{
		"internal/pure":    {Statements: 100, Covered: 90},
		"internal/browser": {Statements: 100, Covered: 10},
	}
	combined := map[string]Coverage{
		"internal/pure":    {Statements: 100, Covered: 95},
		"internal/browser": {Statements: 100, Covered: 75},
	}
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	base := Input{Thresholds: th, Packages: []string{"internal/pure", "internal/browser", "internal/glue", "cmd/x"}, Unit: unit, Combined: combined, Now: now}

	tests := []struct {
		name    string
		mutate  func(in *Input)
		wantOK  bool
		failure string
	}{
		{"all good", func(*Input) {}, true, ""},
		{"pure package below min on unit (combined does not help)", func(in *Input) {
			in.Unit = map[string]Coverage{"internal/pure": {Statements: 100, Covered: 80}, "internal/browser": {Statements: 100, Covered: 10}}
		}, false, "internal/pure: 80.0% < mínimo 85.0%"},
		{"browser package judged by combined", func(in *Input) {
			in.Combined = map[string]Coverage{"internal/pure": {Statements: 100, Covered: 95}, "internal/browser": {Statements: 100, Covered: 60}}
		}, false, "internal/browser: 60.0% < mínimo 70.0%"},
		{"new internal package without entry", func(in *Input) {
			in.Packages = append(in.Packages, "internal/novo")
		}, false, "internal/novo: sem entrada no thresholds.yaml"},
		{"entry for a package that no longer exists", func(in *Input) {
			in.Packages = []string{"internal/pure", "internal/browser", "cmd/x"}
		}, false, "internal/glue: está no thresholds.yaml mas o pacote não existe"},
		{"check-fast: combined not measured is not a failure", func(in *Input) {
			in.Combined = nil
		}, true, ""},
		{"expired quarantine", func(in *Input) {
			in.Quarantine = []Quarantine{{File: "x_test.go", Line: 3, Reason: "issue-1", Until: now.AddDate(0, 0, -2)}}
		}, false, "quarentena vencida"},
		{"quarantine within deadline", func(in *Input) {
			in.Quarantine = []Quarantine{{File: "x_test.go", Line: 3, Reason: "issue-1", Until: now}}
		}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.mutate(&in)
			r := Evaluate(in)
			if r.OK() != tt.wantOK {
				t.Fatalf("OK = %v, failures %v", r.OK(), r.Failures)
			}
			if tt.failure != "" && !strings.Contains(strings.Join(r.Failures, "\n"), tt.failure) {
				t.Fatalf("failures %v, want %q", r.Failures, tt.failure)
			}
			out := Format(r)
			if !strings.Contains(out, "unitária") || !strings.Contains(out, "combinada") {
				t.Fatalf("table must show both columns:\n%s", out)
			}
		})
	}
}

func TestFormatShowsBothColumns(t *testing.T) {
	r := Evaluate(Input{
		Thresholds: Thresholds{Module: mod, Coverage: map[string]PackageThreshold{"internal/a": {Criterion: CriterionUnit, Min: 50, Target: 85}}},
		Packages:   []string{"internal/a"},
		Unit:       map[string]Coverage{"internal/a": {Statements: 4, Covered: 3}},
		Combined:   map[string]Coverage{"internal/a": {Statements: 4, Covered: 4}},
	})
	out := Format(r)
	for _, want := range []string{"internal/a", "75.0%", "100.0%", "unit", "50", "85", "gate: OK"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFindQuarantines(t *testing.T) {
	// A marcação é montada em partes para este próprio arquivo não ser detectado como quarentena.
	mark := `t.Skip("quarantine: https://x/issues/9 ` + `until 2026-11-01")`
	src := "func TestA(t *testing.T) {\n\t" + mark + "\n}\n"
	got := FindQuarantines("a_test.go", strings.NewReader(src))
	if len(got) != 1 || got[0].Line != 2 || got[0].Reason != "https://x/issues/9" || got[0].Until.Format("2006-01-02") != "2026-11-01" {
		t.Fatalf("got %+v", got)
	}
}
