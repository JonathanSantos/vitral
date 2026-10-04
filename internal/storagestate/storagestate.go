// Package storagestate guarda o estado de login de um perfil (cookies e localStorage das origens
// visitadas) em ~/.vitral/sessions/<nome>.json, com permissão 0600, para rodar fluxos sem janela ou
// em outra máquina (--session). O conteúdo nunca vai para a IA: só o resumo (Summary).
// IndexedDB e sessionStorage não entram; alguns sites guardam o login ali.
package storagestate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Cookie é um cookie do navegador (os campos do CDP que voltam no Storage.setCookies).
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires,omitempty"` // segundos desde 1970; 0 = cookie de sessão
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
}

// Origin é o localStorage de uma origem (https://portal.exemplo.com).
type Origin struct {
	Origin       string            `json:"origin"`
	LocalStorage map[string]string `json:"localStorage"`
}

// State é o arquivo de uma sessão salva.
type State struct {
	Version int       `json:"version"`
	Name    string    `json:"name"`
	Profile string    `json:"profile"`
	Created time.Time `json:"created"`
	Cookies []Cookie  `json:"cookies"`
	Origins []Origin  `json:"origins"`
}

// Summary é o que pode ser mostrado (à IA e à pessoa): nunca valores.
type Summary struct {
	Name    string
	Profile string
	Created time.Time
	Cookies int
	Origins []string
	Path    string
}

// Summary resume o estado.
func (s State) Summary(path string) Summary {
	out := Summary{Name: s.Name, Profile: s.Profile, Created: s.Created, Cookies: len(s.Cookies), Path: path}
	for _, o := range s.Origins {
		out.Origins = append(out.Origins, o.Origin)
	}
	return out
}

// Store lê e grava sessões num diretório (0700, arquivos 0600).
type Store struct{ Dir string }

// Path é o arquivo de uma sessão.
func (s Store) Path(name string) string { return filepath.Join(s.Dir, name+".json") }

// Save grava a sessão (substitui uma de mesmo nome).
func (s Store) Save(st State) (string, error) {
	st.Version = 1
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("create sessions dir: %w", err)
	}
	path := s.Path(st.Name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write session: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("write session: %w", err)
	}
	return path, os.Chmod(path, 0o600)
}

// ErrNotFound é uma sessão que não existe.
var ErrNotFound = errors.New("session not found")

// Load lê uma sessão.
func (s Store) Load(name string) (State, error) {
	data, err := os.ReadFile(s.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, fmt.Errorf("%w: %s (saved sessions are in %s)", ErrNotFound, name, s.Dir)
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("%s: invalid session file: %w", s.Path(name), err)
	}
	return st, nil
}

// List resume as sessões salvas, por nome.
func (s Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if e.IsDir() || !ok {
			continue
		}
		st, err := s.Load(name)
		if err != nil {
			out = append(out, Summary{Name: name, Path: s.Path(name)})
			continue
		}
		out = append(out, st.Summary(s.Path(name)))
	}
	slices.SortFunc(out, func(a, b Summary) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Delete apaga uma sessão.
func (s Store) Delete(name string) error {
	err := os.Remove(s.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return err
}

// OriginOf devolve a origem (esquema://host[:porta]) de uma URL http(s); "" para o resto.
func OriginOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// CookieOrigins sugere origens pelos domínios dos cookies (https://dominio), para achar o
// localStorage de sites visitados antes de o Vitral registrar as origens.
func CookieOrigins(cookies []Cookie) []string {
	var out []string
	for _, c := range cookies {
		host := strings.TrimPrefix(c.Domain, ".")
		if host == "" {
			continue
		}
		scheme := "https"
		if !c.Secure && (host == "localhost" || host == "127.0.0.1") {
			scheme = "http"
		}
		out = append(out, scheme+"://"+host)
	}
	return out
}

// Merge junta listas de origens sem repetir, na ordem em que aparecem.
func Merge(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, o := range l {
			if o != "" && !seen[o] {
				seen[o] = true
				out = append(out, o)
			}
		}
	}
	return out
}

// Visited guarda as origens visitadas num perfil (profileDir/vitral-origins.json), para o
// "vitral session save" achar o localStorage sem o navegador do Vitral aberto. Só origens.
type Visited struct{ Dir string }

const visitedFile = "vitral-origins.json"

// maxVisited limita a lista (as mais recentes ficam).
const maxVisited = 200

// List lê as origens registradas.
func (v Visited) List() []string {
	data, err := os.ReadFile(filepath.Join(v.Dir, visitedFile))
	if err != nil {
		return nil
	}
	var out []string
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

// Add registra uma origem (sem repetir) e devolve se mudou.
func (v Visited) Add(origin string) (bool, error) {
	if origin == "" {
		return false, nil
	}
	list := v.List()
	if slices.Contains(list, origin) {
		return false, nil
	}
	list = append(list, origin)
	if len(list) > maxVisited {
		list = list[len(list)-maxVisited:]
	}
	data, err := json.Marshal(list)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(v.Dir, 0o700); err != nil {
		return false, err
	}
	return true, os.WriteFile(filepath.Join(v.Dir, visitedFile), data, 0o600)
}
