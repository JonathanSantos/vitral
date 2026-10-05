package retouch

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed ext
var extFiles embed.FS

// ExtensionName é o nome da extensão gerada (aparece na barra do navegador).
const ExtensionName = "Vitral Retoques"

type contentScript struct {
	Matches []string `json:"matches"`
	JS      []string `json:"js"`
	RunAt   string   `json:"run_at"`
}

type manifest struct {
	ManifestVersion int               `json:"manifest_version"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Icons           map[string]string `json:"icons,omitempty"`
	Action          action            `json:"action"`
	Background      struct {
		ServiceWorker string `json:"service_worker"`
	} `json:"background"`
	Permissions    []string        `json:"permissions"`
	ContentScripts []contentScript `json:"content_scripts,omitempty"`
}

type action struct {
	DefaultTitle string            `json:"default_title"`
	DefaultPopup string            `json:"default_popup"`
	DefaultIcon  map[string]string `json:"default_icon,omitempty"`
}

// Compile gera a extensão MV3 dos retoques em dir. Escreve numa pasta ao lado e troca no fim, para o
// navegador nunca ver uma extensão pela metade. icons é tamanho → PNG (16, 32, 48, 128). Retoques
// inválidos não entram (o Store.List já os separa).
func Compile(dir string, list []Retouch, icons map[int][]byte) error {
	tmp := dir + ".new"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(tmp, "r"), 0o700); err != nil {
		return err
	}
	write := func(name string, data []byte) error {
		return os.WriteFile(filepath.Join(tmp, name), data, 0o600)
	}
	if err := fs.WalkDir(extFiles, "ext", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := extFiles.ReadFile(path)
		if err != nil {
			return err
		}
		return write(filepath.Base(path), data)
	}); err != nil {
		return err
	}

	m := manifest{
		ManifestVersion: 3,
		Name:            ExtensionName,
		Version:         "1.0",
		Description:     "Retoques do Vitral: mudanças de visual que valem só neste navegador.",
		Action:          action{DefaultTitle: "Retoques do Vitral", DefaultPopup: "popup.html"},
		Permissions:     []string{"storage"},
	}
	m.Background.ServiceWorker = "sw.js"
	if len(icons) > 0 {
		m.Icons, m.Action.DefaultIcon = map[string]string{}, map[string]string{}
		sizes := make([]int, 0, len(icons))
		for size := range icons {
			sizes = append(sizes, size)
		}
		sort.Ints(sizes)
		if err := os.MkdirAll(filepath.Join(tmp, "icons"), 0o700); err != nil {
			return err
		}
		for _, size := range sizes {
			name := fmt.Sprintf("icons/icon-%d.png", size)
			if err := write(name, icons[size]); err != nil {
				return err
			}
			m.Icons[fmt.Sprint(size)], m.Action.DefaultIcon[fmt.Sprint(size)] = name, name
		}
	}

	about := map[string]map[string]string{}
	for _, r := range list {
		if err := r.Validate(); err != nil {
			continue
		}
		name, _ := json.Marshal(r.Name)
		css, _ := json.Marshal(r.CSS)
		script := fmt.Sprintf("globalThis.__vitralRetoques.add(%s, %s);\n", name, css)
		file := "r/" + r.Name + ".js"
		if err := write(file, []byte(script)); err != nil {
			return err
		}
		m.ContentScripts = append(m.ContentScripts, contentScript{
			Matches: r.Matches, JS: []string{"runtime.js", file}, RunAt: "document_start",
		})
		about[r.Name] = map[string]string{"title": r.Name, "description": r.Description}
	}
	// Sem escapar < e >: o manifest fica legível ("<all_urls>") para quem abrir a pasta.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	if err := write("manifest.json", buf.Bytes()); err != nil {
		return err
	}
	data, _ := json.Marshal(about)
	if err := write("retoques.json", data); err != nil {
		return err
	}

	// Troca: a pasta antiga sai de cena só depois que a nova está completa.
	old := dir + ".old"
	_ = os.RemoveAll(old)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

// Ready diz se dir tem uma extensão compilada (o que pode ir para o --load-extension: uma pasta que
// não existe ou sem manifest faz o Chrome abrir um diálogo de erro).
func Ready(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return false
	}
	var m manifest
	return json.Unmarshal(data, &m) == nil && m.ManifestVersion == 3
}

// Builder compila os retoques de um Store na extensão gerada, que fica em .build dentro da pasta dos
// retoques (o ponto no nome a deixa fora do Store.List).
type Builder struct {
	Store Store
	Icons map[int][]byte
}

// Dir é a pasta da extensão gerada.
func (b Builder) Dir() string { return filepath.Join(b.Store.Dir, ".build") }

// Build compila e devolve quantos retoques entraram.
func (b Builder) Build() (int, error) {
	list, _, err := b.Store.List()
	if err != nil {
		return 0, err
	}
	return len(list), Compile(b.Dir(), list, b.Icons)
}

// Launch é o que vai para o --load-extension: a pasta da extensão, só se houver algum retoque (sem
// retoques, o navegador abre sem extensão nenhuma).
func (b Builder) Launch() ([]string, error) {
	n, err := b.Build()
	if err != nil || n == 0 || !Ready(b.Dir()) {
		return nil, err
	}
	return []string{b.Dir()}, nil
}
