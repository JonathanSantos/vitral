package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// ServerName é a chave do Vitral em "servers" no mcp.json.
const ServerName = "vitral"

// ServerEntry é a configuração do servidor no mcp.json do VS Code.
type ServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// MergeServer grava a entrada em servers.<name> preservando o resto do arquivo, comentários
// e formatação (o mcp.json do VS Code é JSONC). A entrada nova segue a indentação dos vizinhos.
func MergeServer(data []byte, name string, entry ServerEntry) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		doc := map[string]map[string]ServerEntry{"servers": {name: entry}}
		out, err := json.MarshalIndent(doc, "", "  ")
		return append(out, '\n'), err
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	root, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("the file must contain a JSON object")
	}
	unit := indentOf(root, "")
	if unit == "" && bytes.Contains(data, []byte("\n")) {
		unit = "  "
	}

	servers := member(root, "servers")
	if servers == nil {
		value, err := render(map[string]ServerEntry{name: entry}, unit, unit)
		if err != nil {
			return nil, err
		}
		appendMember(root, "servers", value, unit)
		return v.Pack(), nil
	}
	obj, ok := servers.Value.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New(`"servers" must be an object`)
	}
	childIndent := indentOf(obj, unit+unit)
	value, err := render(entry, childIndent, unit)
	if err != nil {
		return nil, err
	}
	if existing := member(obj, name); existing != nil {
		existing.Value.Value = value.Value
		return v.Pack(), nil
	}
	appendMember(obj, name, value, childIndent)
	return v.Pack(), nil
}

func member(obj *hujson.Object, name string) *hujson.ObjectMember {
	for i := range obj.Members {
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.String() == name {
			return &obj.Members[i]
		}
	}
	return nil
}

// indentOf é a indentação do primeiro membro (o que vem depois da última quebra de linha).
func indentOf(obj *hujson.Object, fallback string) string {
	if len(obj.Members) == 0 {
		return fallback
	}
	before := obj.Members[0].Name.BeforeExtra
	i := bytes.LastIndexByte(before, '\n')
	if i < 0 {
		return ""
	}
	if ws := before[i+1:]; len(bytes.TrimSpace(ws)) == 0 {
		return string(ws)
	}
	return fallback
}

// render serializa o valor já indentado para a posição onde vai entrar.
func render(v any, indent, unit string) (hujson.Value, error) {
	var raw []byte
	var err error
	if unit == "" {
		raw, err = json.Marshal(v)
	} else {
		raw, err = json.MarshalIndent(v, indent, unit)
	}
	if err != nil {
		return hujson.Value{}, err
	}
	return hujson.Parse(raw)
}

// appendMember põe o membro no fim do objeto. Comentários que estavam antes do "}" (ex.: o
// comentário de fim de linha do último membro) continuam onde estavam.
func appendMember(obj *hujson.Object, name string, value hujson.Value, indent string) {
	before := hujson.Extra(nil)
	after := obj.AfterExtra
	if i := bytes.LastIndexByte(after, '\n'); i >= 0 {
		before = append(append(hujson.Extra{}, after[:i]...), []byte("\n"+indent)...)
		obj.AfterExtra = append(hujson.Extra{}, after[i:]...)
	}
	m := hujson.ObjectMember{
		Name:  hujson.Value{BeforeExtra: before, Value: hujson.String(name)},
		Value: hujson.Value{BeforeExtra: hujson.Extra(" "), Value: value.Value},
	}
	// Mantém a vírgula final se o arquivo já usava (o VS Code aceita).
	if n := len(obj.Members); n > 0 && obj.Members[n-1].Value.AfterExtra != nil {
		m.Value.AfterExtra = hujson.Extra{}
		obj.Members[n-1].Value.AfterExtra = nil
	}
	obj.Members = append(obj.Members, m)
}

// ReadServer lê servers.<name> de um mcp.json (JSONC). ok=false se não existir.
func ReadServer(data []byte, name string) (ServerEntry, bool, error) {
	std, err := hujson.Standardize(data)
	if err != nil {
		return ServerEntry{}, false, fmt.Errorf("invalid JSON: %w", err)
	}
	var doc struct {
		Servers map[string]ServerEntry `json:"servers"`
	}
	if err := json.Unmarshal(std, &doc); err != nil {
		return ServerEntry{}, false, err
	}
	entry, ok := doc.Servers[name]
	return entry, ok, nil
}
