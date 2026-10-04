package cli

import (
	"encoding/json"
	"testing"

	"github.com/tailscale/hujson"
)

// FuzzMergeServer: o resultado é JSONC válido, tem o Vitral e preserva os servidores de terceiros.
func FuzzMergeServer(f *testing.F) {
	f.Add("")
	f.Add("{}")
	f.Add(`{"servers":{"github":{"type":"http","url":"https://x"}}}`)
	f.Add("{\n  // c\n  \"servers\": {\n    \"a\": {\"command\": \"x\"}, // fim\n  },\n  \"inputs\": [],\n}\n")
	f.Add(`{"servers":{"vitral":{"command":"/old"}}}`)
	f.Add(`{"servers": 3}`)
	entry := ServerEntry{Type: "stdio", Command: `C:\Users\A\.vitral\bin\vitral.exe`, Args: []string{}}
	f.Fuzz(func(t *testing.T, data string) {
		out, err := MergeServer([]byte(data), KeyVSCode, ServerName, entry)
		if err != nil {
			return
		}
		if _, err := hujson.Parse(out); err != nil {
			t.Fatalf("result is not valid JSONC: %v\n%s", err, out)
		}
		got, ok, err := ReadServer(out, KeyVSCode, ServerName)
		if err != nil || !ok || got.Command != entry.Command {
			t.Fatalf("vitral entry missing: %+v %v\n%s", got, err, out)
		}
		before := servers(t, []byte(data))
		after := servers(t, out)
		for name, v := range before {
			if name == ServerName {
				continue
			}
			if after[name] != v {
				t.Fatalf("third-party server %q changed: %s -> %s", name, v, after[name])
			}
		}
	})
}

// servers lê o objeto "servers" com a chave exata (como o VS Code), cada valor normalizado.
func servers(t *testing.T, data []byte) map[string]string {
	t.Helper()
	v, err := hujson.Parse(data)
	if err != nil {
		return nil
	}
	node := v.Find("/servers")
	if node == nil {
		return nil
	}
	obj, ok := node.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, m := range obj.Members {
		name, ok := m.Name.Value.(hujson.Literal)
		if !ok {
			continue
		}
		value := m.Value.Clone()
		value.Standardize()
		var compact any
		if json.Unmarshal(value.Pack(), &compact) == nil {
			b, _ := json.Marshal(compact)
			out[name.String()] = string(b)
		}
	}
	return out
}
