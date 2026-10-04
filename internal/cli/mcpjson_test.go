package cli

import (
	"strings"
	"testing"

	"github.com/tailscale/hujson"
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
	out, err := MergeServer([]byte(in), KeyVSCode, ServerName, ServerEntry{Type: "stdio", Command: `C:\Users\Ana\.vitral\bin\vitral.exe`, Args: []string{}})
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
			out, err := MergeServer([]byte(tt.in), KeyVSCode, ServerName, entry)
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
			got, ok, err := ReadServer(out, KeyVSCode, ServerName)
			if err != nil || !ok || got.Command != entry.Command {
				t.Fatalf("ReadServer: %+v %v %v\n%s", got, ok, err, out)
			}
		})
	}
	if _, err := MergeServer([]byte(`[1, 2]`), KeyVSCode, ServerName, entry); err == nil {
		t.Fatal("array root must fail")
	}
	if _, err := MergeServer([]byte(`{"servers": `), KeyVSCode, ServerName, entry); err == nil {
		t.Fatal("broken JSON must fail")
	}
}

func TestMergeServerPortable(t *testing.T) {
	entry := ServerEntry{Type: "stdio", Command: "/v", Args: []string{}, Tools: []string{"*"}}
	out, err := MergeServer(nil, KeyPortable, ServerName, entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"mcpServers": {`) || !strings.Contains(string(out), `"tools": [`) {
		t.Fatalf("%s", out)
	}
	got, ok, err := ReadServer(out, KeyPortable, ServerName)
	if err != nil || !ok || got.Tools[0] != "*" {
		t.Fatalf("%+v %v %v", got, ok, err)
	}
	if _, ok, _ := ReadServer(out, KeyVSCode, ServerName); ok {
		t.Fatal("the key matters")
	}
	if _, ok, err := ReadServer([]byte("  "), KeyPortable, ServerName); ok || err != nil {
		t.Fatal("empty file has no entry")
	}
	if _, _, err := ReadServer([]byte(`{"mcpServers":{"vitral":{"tools": 3}}}`), KeyPortable, ServerName); err == nil || !strings.Contains(err.Error(), "mcpServers.vitral.tools") {
		t.Fatalf("bad field: %v", err)
	}
	if _, err := MergeServer([]byte(`{"mcpServers": []}`), KeyPortable, ServerName, entry); err == nil || !strings.Contains(err.Error(), `"mcpServers" must be an object`) {
		t.Fatalf("%v", err)
	}
}

func TestMoveServer(t *testing.T) {
	in := `{
  // equipe
  "mcpServers": {
    "github": { "type": "http", "url": "https://x" },
    "vitral": { "type": "stdio", "command": "/v", "args": [], "env": { "A": "1" } }, // nosso
  },
}
`
	off, moved, err := MoveServer([]byte(in), KeyPortable, KeyDisabled, ServerName)
	if err != nil || !moved {
		t.Fatal(err)
	}
	if _, ok, _ := ReadServer(off, KeyPortable, ServerName); ok {
		t.Fatalf("still active:\n%s", off)
	}
	got, ok, _ := ReadServer(off, KeyDisabled, ServerName)
	if !ok || got.Command != "/v" || !strings.Contains(string(off), `"A": "1"`) || !strings.Contains(string(off), "// equipe") || !strings.Contains(string(off), `"github"`) {
		t.Fatalf("disabled entry keeps every field and the rest of the file:\n%s", off)
	}
	if _, err := hujson.Parse(off); err != nil {
		t.Fatalf("invalid JSONC: %v\n%s", err, off)
	}
	// Idempotente: de novo não muda nada.
	if again, moved, err := MoveServer(off, KeyPortable, KeyDisabled, ServerName); moved || err != nil || string(again) != string(off) {
		t.Fatal("second disable must be a no-op")
	}
	on, moved, err := MoveServer(off, KeyDisabled, KeyPortable, ServerName)
	if err != nil || !moved {
		t.Fatal(err)
	}
	if got, ok, _ := ReadServer(on, KeyPortable, ServerName); !ok || got.Command != "/v" {
		t.Fatalf("enable:\n%s", on)
	}
	// Ligar de novo por cima de uma entrada existente troca a entrada.
	both := `{"mcpServers": {"vitral": {"command": "/novo"}}, "_disabled": {"vitral": {"command": "/velho"}}}`
	out, _, _ := MoveServer([]byte(both), KeyDisabled, KeyPortable, ServerName)
	if got, _, _ := ReadServer(out, KeyPortable, ServerName); got.Command != "/velho" || strings.Contains(string(out), "/novo") {
		t.Fatalf("%s", out)
	}
	// Só remover (to vazio), único membro e arquivo sem a chave.
	out, moved, _ = MoveServer([]byte(`{"servers": {"vitral": {"command": "/v"}}}`), KeyVSCode, "", ServerName)
	if !moved || strings.Contains(string(out), "vitral") {
		t.Fatalf("%s", out)
	}
	for _, data := range []string{"", `{"servers": {}}`, `{"inputs": []}`} {
		if out, moved, err := MoveServer([]byte(data), KeyVSCode, KeyDisabled, ServerName); moved || err != nil || string(out) != data {
			t.Errorf("%q: %s %v %v", data, out, moved, err)
		}
	}
	for _, bad := range []string{`[1]`, `{"servers": 3}`, `{ quebrado`} {
		if _, _, err := MoveServer([]byte(bad), KeyVSCode, KeyDisabled, ServerName); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
	if _, _, err := MoveServer([]byte(`{"servers": {"vitral": {}}, "_disabled": 3}`), KeyVSCode, KeyDisabled, ServerName); err == nil {
		t.Error("a non-object destination must fail")
	}
	// Arquivo de uma linha (sem indentação).
	out, _, _ = MoveServer([]byte(`{"servers":{"vitral":{"command":"/v"},"x":{}}}`), KeyVSCode, KeyDisabled, ServerName)
	if _, ok, _ := ReadServer(out, KeyDisabled, ServerName); !ok {
		t.Fatalf("%s", out)
	}
}
