package main

import (
	"bufio"
	"io"
	"regexp"
	"time"
)

// Quarantine é um teste fora do gate, com issue e prazo (docs/TESTING.md, seção 10).
type Quarantine struct {
	File   string
	Line   int
	Reason string
	Until  time.Time
}

// Formato: t.Skip("quarantine: <issue> until AAAA-MM-DD")
var quarantineRe = regexp.MustCompile(`t\.Skip\("quarantine: (.+?) until (\d{4}-\d{2}-\d{2})"\)`)

// FindQuarantines procura marcações de quarentena num arquivo de teste.
func FindQuarantines(file string, r io.Reader) []Quarantine {
	var out []Quarantine
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		m := quarantineRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		until, err := time.Parse("2006-01-02", m[2])
		if err != nil {
			continue
		}
		out = append(out, Quarantine{File: file, Line: line, Reason: m[1], Until: until})
	}
	return out
}
