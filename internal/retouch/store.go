package retouch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	metaFile  = "retoque.yaml"
	cssFile   = "style.css"
	histDir   = ".history"
	stampTime = "20060102-150405"
)

// Store lê e grava retoques numa pasta (0700, arquivos 0600): ~/.vitral/retoques.
type Store struct {
	Dir string
	Now func() time.Time // relógio das versões em .history (testes)
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Path é a pasta de um retoque.
func (s Store) Path(name string) string { return filepath.Join(s.Dir, name) }

// Save grava o retoque. Se já existia, a versão anterior vai para .history/<data>/ e replaced é true.
func (s Store) Save(r Retouch) (replaced bool, err error) {
	if err := r.Validate(); err != nil {
		return false, err
	}
	dir := s.Path(r.Name)
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err == nil {
		if err := s.archive(dir); err != nil {
			return false, err
		}
		replaced = true
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	meta, err := yaml.Marshal(r)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(filepath.Join(dir, cssFile), []byte(r.CSS), 0o600); err != nil {
		return false, err
	}
	// O YAML por último: um retoque sem retoque.yaml não existe para o Load.
	return replaced, os.WriteFile(filepath.Join(dir, metaFile), meta, 0o600)
}

// archive copia a versão atual para .history/<data>[-N]/.
func (s Store) archive(dir string) error {
	base := filepath.Join(dir, histDir, s.now().Format(stampTime))
	dest := base
	for i := 2; ; i++ {
		if _, err := os.Stat(dest); errors.Is(err, fs.ErrNotExist) {
			break
		}
		dest = fmt.Sprintf("%s-%d", base, i)
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	for _, f := range []string{metaFile, cssFile} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dest, f), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Load lê um retoque.
func (s Store) Load(name string) (Retouch, error) {
	if !ValidName(name) {
		return Retouch{}, fmt.Errorf("invalid retouch name %q", name)
	}
	dir := s.Path(name)
	meta, err := os.ReadFile(filepath.Join(dir, metaFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Retouch{}, fmt.Errorf("no retouch named %q. Call retouch_list to see the saved ones", name)
	}
	if err != nil {
		return Retouch{}, err
	}
	var r Retouch
	if err := yaml.Unmarshal(meta, &r); err != nil {
		return Retouch{}, fmt.Errorf("retouch %q: %w", name, err)
	}
	css, err := os.ReadFile(filepath.Join(dir, cssFile))
	if err != nil {
		return Retouch{}, fmt.Errorf("retouch %q: %w", name, err)
	}
	r.Name, r.CSS = name, string(css)
	return r, r.Validate()
}

// List lê todos os retoques válidos, em ordem de nome. Pastas quebradas vão em bad (nome → erro) e
// nunca entram na extensão.
func (s Store) List() (list []Retouch, bad map[string]error, err error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		r, err := s.Load(e.Name())
		if err != nil {
			if bad == nil {
				bad = map[string]error{}
			}
			bad[e.Name()] = err
			continue
		}
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, bad, nil
}

// Delete apaga um retoque (com o histórico). Só pelo terminal ou pelo painel, nunca pela IA.
func (s Store) Delete(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid retouch name %q", name)
	}
	dir := s.Path(name)
	if _, err := os.Stat(filepath.Join(dir, metaFile)); err != nil {
		return fmt.Errorf("no retouch named %q", name)
	}
	return os.RemoveAll(dir)
}
