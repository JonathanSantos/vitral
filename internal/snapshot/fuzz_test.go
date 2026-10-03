package snapshot

import (
	"strings"
	"testing"
)

// FuzzParseAndFormat: o JSON vem do JS da página; nada pode derrubar o parse nem a formatação,
// e a saída respeita o limite de linhas.
func FuzzParseAndFormat(f *testing.F) {
	f.Add(`{"docId":"d","url":"http://x","title":"T","viewport":[1,1],"scroll":[0,0],"nodes":[{"ref":"e1","role":"button","name":"Ok","box":[0,0,1,1]}]}`)
	f.Add(`{"nodes":[{"ref":"e1","role":"table","count":3,"children":[{"ref":"e2","role":"row","name":"a"},{"ref":"e3","role":"row"}]}]}`)
	f.Add(`{"error":"ref_not_found","ref":"e9"}`)
	f.Add(`{"nodes":[{"ref":"e1","role":"list","count":-1,"value":"x","states":["invalid"],"invalidMessage":"m"}]}`)
	f.Add(`nope`)
	f.Fuzz(func(t *testing.T, data string) {
		p, err := Parse([]byte(data))
		if err != nil {
			return
		}
		opts := FormatOptions{MaxLines: 5, TextMax: 20, ListMax: 2}
		out := Format(p, opts)
		body := strings.Split(out, "\n")[2:]
		if len(body) > opts.MaxLines+1 {
			t.Fatalf("%d lines > max %d:\n%s", len(body), opts.MaxLines, out)
		}
		_ = Summarize(p, p, opts)
	})
}
