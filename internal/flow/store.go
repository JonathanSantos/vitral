package flow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Marshal gera o YAML do fluxo.
func Marshal(f Flow) ([]byte, error) {
	return yaml.Marshal(f)
}

// Unmarshal lê e valida o YAML de um fluxo.
func Unmarshal(data []byte) (Flow, error) {
	var f Flow
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("invalid flow yaml: %w", err)
	}
	return f, Validate(f)
}

// Store lê e grava fluxos num diretório.
type Store struct {
	Dir string
}

// Path é o caminho do arquivo de um fluxo.
func (s Store) Path(name string) string {
	return filepath.Join(s.Dir, name+".yaml")
}

// Save grava o fluxo e informa se substituiu um arquivo existente.
func (s Store) Save(f Flow) (path string, replaced bool, err error) {
	if err := Validate(f); err != nil {
		return "", false, err
	}
	data, err := Marshal(f)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return "", false, fmt.Errorf("create flows dir: %w", err)
	}
	path = s.Path(f.Name)
	_, statErr := os.Stat(path)
	replaced = statErr == nil
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", false, fmt.Errorf("write flow: %w", err)
	}
	return path, replaced, nil
}

// Load lê um fluxo pelo nome.
func (s Store) Load(name string) (Flow, error) {
	if err := ValidateName(name); err != nil {
		return Flow{}, err
	}
	data, err := os.ReadFile(s.Path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return Flow{}, fmt.Errorf("flow %q not found in %s. Call flow_list.", name, s.Dir)
	}
	if err != nil {
		return Flow{}, err
	}
	f, err := Unmarshal(data)
	if err != nil {
		return f, fmt.Errorf("%s: %w", s.Path(name), err)
	}
	return f, nil
}

// List lê todos os fluxos do diretório; arquivos inválidos entram com o erro na descrição.
func (s Store) List() ([]Flow, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Flow
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		f, err := s.Load(name)
		if err != nil {
			f = Flow{Name: name, Description: "invalid: " + err.Error()}
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b Flow) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// Recorder guarda os passos bem-sucedidos da sessão. Os números começam em 1 e nunca se repetem.
type Recorder struct {
	mu        sync.Mutex
	steps     []Step
	savedUpTo int
}

// Add grava um passo e devolve seu número.
func (r *Recorder) Add(s Step) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, s)
	return len(r.steps)
}

// Range devolve os passos a partir de from (1-based); from 0 usa "desde o último flow_save".
func (r *Recorder) Range(from int) ([]Step, int, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(r.steps)
	if from == 0 {
		from = r.savedUpTo + 1
	}
	if from < 1 || from > total {
		if total == 0 {
			return nil, 0, 0, errors.New("no steps recorded yet: run some actions first")
		}
		if from == r.savedUpTo+1 {
			return nil, 0, 0, fmt.Errorf("no new steps since the last flow_save (steps #1-#%d). Use fromStep to save older steps", total)
		}
		return nil, 0, 0, fmt.Errorf("fromStep must be between 1 and %d", total)
	}
	return slices.Clone(r.steps[from-1:]), from, total, nil
}

// MarkSaved registra até onde o último flow_save foi.
func (r *Recorder) MarkSaved(upTo int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.savedUpTo = upTo
}
