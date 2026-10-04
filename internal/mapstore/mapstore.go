// Package mapstore guarda os mapas de telas (internal/sitemap) em ~/.vitral/maps/<host>.json: privados,
// por usuário, com escrita atômica e debounce. Separado do sitemap, que é puro.
package mapstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JonathanSantos/vitral/internal/sitemap"
)

// Store lê e grava mapas num diretório.
type Store struct {
	Dir string
	Now func() time.Time
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var unsafeChars = regexp.MustCompile(`[^a-z0-9._-]`)

// Path é o arquivo do mapa de um host ("127.0.0.1:8080" vira "127.0.0.1_8080.json").
func (s Store) Path(host string) string {
	return filepath.Join(s.Dir, unsafeChars.ReplaceAllString(strings.ToLower(host), "_")+".json")
}

// Load lê o mapa do host (vazio se não existe) e tira o que envelheceu (90 dias).
func (s Store) Load(host string) (*sitemap.Map, error) {
	data, err := os.ReadFile(s.Path(host))
	if errors.Is(err, fs.ErrNotExist) {
		return sitemap.New(host), nil
	}
	if err != nil {
		return nil, err
	}
	m, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.Path(host), err)
	}
	m.Prune(s.now())
	return m, nil
}

// Decode lê um mapa e confere a versão do formato.
func Decode(data []byte) (*sitemap.Map, error) {
	var m sitemap.Map
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid map file: %w", err)
	}
	if m.Version != sitemap.Version {
		return nil, fmt.Errorf("unsupported map version %d (this Vitral reads version %d)", m.Version, sitemap.Version)
	}
	if m.Host == "" {
		return nil, errors.New("invalid map file: no host")
	}
	return &m, nil
}

// Save grava o mapa (atômico: arquivo temporário e rename; 0600).
func (s Store) Save(m *sitemap.Map) error {
	return writeAtomic(s.Path(m.Host), m)
}

func writeAtomic(path string, m *sitemap.Map) error {
	data, err := encode(m)
	if err != nil {
		return err
	}
	return writeBytes(path, data)
}

func encode(m *sitemap.Map) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", " ")
	return append(data, '\n'), err
}

func writeBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".map-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Clear apaga o mapa de um host.
func (s Store) Clear(host string) error {
	err := os.Remove(s.Path(host))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Export copia o mapa do host para um arquivo (para compartilhar com a equipe).
func (s Store) Export(host, to string) error {
	m, err := s.Load(host)
	if err != nil {
		return err
	}
	if len(m.Screens) == 0 {
		return fmt.Errorf("there is no map for %s in %s", host, s.Dir)
	}
	return writeAtomic(to, m)
}

// Import lê um mapa exportado e o grava como o mapa do host dele (substitui o atual).
func (s Store) Import(from string) (*sitemap.Map, error) {
	data, err := os.ReadFile(from)
	if err != nil {
		return nil, err
	}
	m, err := Decode(data)
	if err != nil {
		return nil, err
	}
	m.Prune(s.now())
	return m, s.Save(m)
}

// Saver junta gravações: cada mapa alterado é gravado depois de Delay sem novas mudanças (debounce),
// e Flush grava o que estiver pendente (no fim da sessão). Schedule serializa na hora: quem chama
// continua mexendo no mapa sem disputar com a gravação.
type Saver struct {
	Store Store
	Delay time.Duration
	// OnError recebe erros de gravação (o mapa nunca derruba uma ação).
	OnError func(error)

	mu      sync.Mutex
	pending map[string][]byte
	timer   *time.Timer
}

// Schedule marca o mapa para gravar.
func (s *Saver) Schedule(m *sitemap.Map) {
	data, err := encode(m)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		s.pending = map[string][]byte{}
	}
	s.pending[m.Host] = data
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.Delay, s.Flush)
}

// Flush grava os mapas pendentes agora.
func (s *Saver) Flush() {
	s.mu.Lock()
	pending := s.pending
	s.pending = nil
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	for host, data := range pending {
		if err := writeBytes(s.Store.Path(host), data); err != nil && s.OnError != nil {
			s.OnError(err)
		}
	}
}
