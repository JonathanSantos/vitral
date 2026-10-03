package cli

import (
	"strings"
	"testing"
)

func TestMergeServerKeepsLayout(t *testing.T) {
	in := "{\n  // equipe\n  \"servers\": {\n    \"github\": { \"type\": \"http\", \"url\": \"https://x\" }, // remoto\n  },\n}\n"
	want := `{
  // equipe
  "servers": {
    "github": { "type": "http", "url": "https://x" }, // remoto
    "vitral": {
      "type": "stdio",
      "command": "C:\\Users\\Ana\\.vitral\\bin\\vitral.exe",
      "args": []
    },
  },
}
`
	out, err := MergeServer([]byte(in), ServerName, ServerEntry{Type: "stdio", Command: `C:\Users\Ana\.vitral\bin\vitral.exe`, Args: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestMergeServer(t *testing.T) {
	entry := ServerEntry{Type: "stdio", Command: `C:\Users\Ana\.vitral\bin\vitral.exe`, Args: []string{}}
	tests := []struct {
		name     string
		in       string
		contains []string
	}{
		{
			name:     "empty file",
			in:       "",
			contains: []string{`"servers": {`, `"vitral": {`, `"command": "C:\\Users\\Ana\\.vitral\\bin\\vitral.exe"`},
		},
		{
			name: "keeps other servers, comments and trailing commas",
			in: `{
  // meus servidores
  "servers": {
    "github": { "type": "http", "url": "https://x" }, // remoto
  },
  "inputs": [],
}`,
			contains: []string{"// meus servidores", "// remoto", `"github": { "type": "http", "url": "https://x" }`, `"inputs": []`, `"vitral"`},
		},
		{
			name:     "no servers key",
			in:       `{"inputs": []}`,
			contains: []string{`"inputs": []`, `"servers"`, `"vitral"`},
		},
		{
			name:     "replaces an old vitral entry",
			in:       `{"servers": {"vitral": {"type": "stdio", "command": "/old/vitral"}}}`,
			contains: []string{`vitral.exe`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := MergeServer([]byte(tt.in), ServerName, entry)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(out), want) {
					t.Errorf("missing %q in:\n%s", want, out)
				}
			}
			if strings.Contains(string(out), "/old/vitral") {
				t.Errorf("old entry kept:\n%s", out)
			}
			got, ok, err := ReadServer(out, ServerName)
			if err != nil || !ok || got.Command != entry.Command {
				t.Fatalf("ReadServer: %+v %v %v\n%s", got, ok, err, out)
			}
		})
	}
	if _, err := MergeServer([]byte(`[1, 2]`), ServerName, entry); err == nil {
		t.Fatal("array root must fail")
	}
	if _, err := MergeServer([]byte(`{"servers": `), ServerName, entry); err == nil {
		t.Fatal("broken JSON must fail")
	}
}
