package main

import (
	"strings"
	"testing"
)

const gremlinsJSON = `{"files":[{"file_name":"state.go","mutations":[
 {"type":"CONDITIONALS_NEGATION","status":"KILLED","line":10},
 {"type":"CONDITIONALS_NEGATION","status":"KILLED","line":11},
 {"type":"ARITHMETIC_BASE","status":"LIVED","line":20},
 {"type":"INVERT_NEGATIVES","status":"LIVED","line":30},
 {"type":"CONDITIONALS_BOUNDARY","status":"TIMED OUT","line":40},
 {"type":"INCREMENT_DECREMENT","status":"NOT COVERED","line":50}
]}]}`

func TestParseMutation(t *testing.T) {
	ex := []Exclusion{{File: "internal/ux/state.go", Line: 30, Mutator: "invert_negatives", Reason: "equivalente"}}
	r, err := ParseMutation("internal/ux", []byte(gremlinsJSON), ex)
	if err != nil {
		t.Fatal(err)
	}
	if r.Killed != 2 || r.Lived != 1 || r.Excluded != 1 || r.Ignored != 2 {
		t.Fatalf("counts %+v", r)
	}
	if got := r.Score(); got < 66.6 || got > 66.7 {
		t.Fatalf("score = %.2f", got)
	}
	if r.LivedAt[0] != "internal/ux/state.go:20 ARITHMETIC_BASE" {
		t.Fatalf("lived = %v", r.LivedAt)
	}
	if (MutationResult{}).Score() != 100 {
		t.Fatal("no mutants counts as 100")
	}
	if _, err := ParseMutation("x", []byte("{"), nil); err == nil {
		t.Fatal("broken report must fail")
	}
	failures := EvaluateMutation([]MutationResult{r}, 75)
	if len(failures) != 1 || !strings.Contains(failures[0], "state.go:20") {
		t.Fatalf("failures %v", failures)
	}
	if out := FormatMutation([]MutationResult{r}, 75); !strings.Contains(out, "66.7%") || !strings.Contains(out, "FALHA") {
		t.Fatalf("format:\n%s", out)
	}
}

func TestParseExclusionsNeedsReason(t *testing.T) {
	if _, err := ParseExclusions([]byte("- {file: a.go, line: 1, mutator: X, reason: porque}")); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseExclusions([]byte("- {file: a.go, line: 1, mutator: X}")); err == nil {
		t.Fatal("exclusion without reason must fail")
	}
}

func TestChangedPackages(t *testing.T) {
	scope := []string{"internal/flow", "internal/ux"}
	files := []string{"internal/flow/flow.go", "internal/flow/flow_test.go", "internal/ux/state.go", "internal/session/a.go", "README.md", "internal/ux/state.go"}
	if got := ChangedPackages(files, scope); strings.Join(got, ",") != "internal/flow,internal/ux" {
		t.Fatalf("got %v", got)
	}
	if got := ChangedPackages([]string{"internal/flow/flow_test.go"}, scope); len(got) != 0 {
		t.Fatalf("only tests changed: %v", got)
	}
}
