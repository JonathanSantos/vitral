package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// ServerName é a chave do Vitral na lista de servidores.
const ServerName = "vitral"

// Chaves da lista de servidores: "servers" no formato do VS Code (.vscode/mcp.json e o mcp.json do
// perfil, deprecados) e "mcpServers" no formato portável (.mcp.json e ~/.copilot/mcp-config.json).
// KeyDisabled guarda a entrada desligada pelo "vitral disable": não existe campo oficial de
// "desligado" no arquivo, e uma chave de topo desconhecida mantém o JSON válido para todos.
const (
	KeyVSCode   = "servers"
	KeyPortable = "mcpServers"
	KeyDisabled = "_disabled"
)

// ServerEntry é a configuração do servidor no arquivo MCP.
type ServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// Tools é a lista de ferramentas liberadas no formato do Copilot CLI ("*" = todas).
	Tools []string `json:"tools,omitempty"`
}

// MergeServer grava a entrada em <key>.<name> preservando o resto do arquivo, comentários
// e formatação (JSONC). A entrada nova segue a indentação dos vizinhos.
func MergeServer(data []byte, key, name string, entry ServerEntry) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		doc := map[string]map[string]ServerEntry{key: {name: entry}}
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

	servers := member(root, key)
	if servers == nil {
		value, err := render(map[string]ServerEntry{name: entry}, unit, unit)
		if err != nil {
			return nil, err
		}
		appendMember(root, key, value, unit)
		return v.Pack(), nil
	}
	obj, ok := servers.Value.Value.(*hujson.Object)
	if !ok {
		return nil, fmt.Errorf("%q must be an object", key)
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

// ReadServer lê <key>.<name> de um arquivo MCP (JSONC). ok=false se não existir.
// As chaves são comparadas exatamente, como o VS Code faz (o encoding/json do Go ignoraria
// maiúsculas e poderia ler "Servers" no lugar de "servers").
func ReadServer(data []byte, key, name string) (ServerEntry, bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return ServerEntry{}, false, nil
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return ServerEntry{}, false, fmt.Errorf("invalid JSON: %w", err)
	}
	esc := strings.NewReplacer("~", "~0", "/", "~1")
	found := v.Find("/" + esc.Replace(key) + "/" + esc.Replace(name))
	if found == nil {
		return ServerEntry{}, false, nil
	}
	entryValue := found.Clone()
	entryValue.Standardize()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(entryValue.Pack(), &fields); err != nil {
		return ServerEntry{}, false, fmt.Errorf("%s.%s is not an object", key, name)
	}
	var e ServerEntry
	for field, dst := range map[string]any{"type": &e.Type, "command": &e.Command, "args": &e.Args, "tools": &e.Tools} {
		if raw, ok := fields[field]; ok {
			if err := json.Unmarshal(raw, dst); err != nil {
				return ServerEntry{}, false, fmt.Errorf("%s.%s.%s: %w", key, name, field, err)
			}
		}
	}
	return e, true, nil
}

// MoveServer tira <from>.<name> do arquivo e põe em <to>.<name>, com todos os campos e sem mexer
// no resto (é o "vitral disable"/"enable"; to vazio só remove). moved=false se não estava em from.
func MoveServer(data []byte, from, to, name string) (out []byte, moved bool, err error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return data, false, nil
	}
	v, err := hujson.Parse(data)
	if err != nil {
		return nil, false, fmt.Errorf("invalid JSON: %w", err)
	}
	root, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil, false, errors.New("the file must contain a JSON object")
	}
	src := member(root, from)
	if src == nil {
		return data, false, nil
	}
	obj, ok := src.Value.Value.(*hujson.Object)
	if !ok {
		return nil, false, fmt.Errorf("%q must be an object", from)
	}
	i := -1
	for k := range obj.Members {
		if lit, ok := obj.Members[k].Name.Value.(hujson.Literal); ok && lit.String() == name {
			i = k
			break
		}
	}
	if i < 0 {
		return data, false, nil
	}
	entry := obj.Members[i].Value.Clone()
	entry.Standardize()
	removeMember(obj, i)
	if to != "" {
		unit := indentOf(root, "")
		if unit == "" && bytes.Contains(data, []byte("\n")) {
			unit = "  "
		}
		var raw any
		if err := json.Unmarshal(entry.Pack(), &raw); err != nil {
			return nil, false, err
		}
		dst := member(root, to)
		if dst == nil {
			value, err := render(map[string]any{name: raw}, unit, unit)
			if err != nil {
				return nil, false, err
			}
			appendMember(root, to, value, unit)
		} else {
			dobj, ok := dst.Value.Value.(*hujson.Object)
			if !ok {
				return nil, false, fmt.Errorf("%q must be an object", to)
			}
			indent := indentOf(dobj, unit+unit)
			value, err := render(raw, indent, unit)
			if err != nil {
				return nil, false, err
			}
			if existing := member(dobj, name); existing != nil {
				existing.Value.Value = value.Value
			} else {
				appendMember(dobj, name, value, indent)
			}
		}
	}
	return v.Pack(), true, nil
}

// removeMember tira o membro i, mantendo a vírgula final se o arquivo usava.
func removeMember(obj *hujson.Object, i int) {
	last := i == len(obj.Members)-1
	trailing := obj.Members[i].Value.AfterExtra
	obj.Members = append(obj.Members[:i], obj.Members[i+1:]...)
	if last && len(obj.Members) > 0 {
		obj.Members[len(obj.Members)-1].Value.AfterExtra = trailing
	}
}
