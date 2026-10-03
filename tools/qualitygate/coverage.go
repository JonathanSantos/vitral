package main

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// Coverage é a contagem de statements de um pacote.
type Coverage struct {
	Statements int
	Covered    int
}

// Percent devolve a cobertura em porcentagem (100 para pacote sem statements).
func (c Coverage) Percent() float64 {
	if c.Statements == 0 {
		return 100
	}
	return 100 * float64(c.Covered) / float64(c.Statements)
}

type block struct {
	stmts   int
	covered bool
}

// ParseProfiles lê um ou mais coverprofiles do go test e soma por pacote.
// Blocos repetidos (o mesmo pacote reportado por vários binários de teste com -coverpkg,
// ou por unitários e integração) contam uma vez só, coberto se algum perfil o cobriu.
func ParseProfiles(module string, profiles ...io.Reader) (map[string]Coverage, error) {
	blocks := map[string]block{}
	for _, r := range profiles {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "mode:") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) != 3 {
				return nil, fmt.Errorf("invalid coverprofile line: %q", line)
			}
			stmts, err1 := strconv.Atoi(fields[1])
			count, err2 := strconv.Atoi(fields[2])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid coverprofile line: %q", line)
			}
			prev := blocks[fields[0]]
			blocks[fields[0]] = block{stmts: stmts, covered: prev.covered || count > 0}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	out := map[string]Coverage{}
	for key, b := range blocks {
		file, _, _ := strings.Cut(key, ":")
		pkg := strings.TrimPrefix(path.Dir(file), module+"/")
		c := out[pkg]
		c.Statements += b.stmts
		if b.covered {
			c.Covered += b.stmts
		}
		out[pkg] = c
	}
	return out, nil
}
