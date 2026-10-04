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
	"time"

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

// Update regrava um fluxo existente e guarda a versão anterior em .history/<nome>.<data>.yaml.
func (s Store) Update(f Flow) (path, history string, err error) {
	path = s.Path(f.Name)
	old, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("flow %q not found in %s. Call flow_list.", f.Name, s.Dir)
	}
	dir := filepath.Join(s.Dir, ".history")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", "", fmt.Errorf("create history dir: %w", err)
	}
	stamp := now().Format("20060102-150405")
	history = filepath.Join(dir, f.Name+"."+stamp+".yaml")
	for i := 2; fileExists(history); i++ {
		history = filepath.Join(dir, fmt.Sprintf("%s.%s-%d.yaml", f.Name, stamp, i))
	}
	if err := os.WriteFile(history, old, 0o644); err != nil {
		return "", "", fmt.Errorf("write flow history: %w", err)
	}
	if _, _, err := s.Save(f); err != nil {
		return "", "", err
	}
	return path, history, nil
}

// now é o relógio do histórico (trocado nos testes).
var now = time.Now

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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
	preview   *Selection
}

// Selection é a escolha de passos de uma prévia do flow_save.
type Selection struct {
	Name     string
	From, To int
	Exclude  []int
}

// RememberPreview guarda a escolha da última prévia: o save logo depois, com o mesmo nome e sem
// intervalo, usa a mesma escolha (um modelo pequeno confere a prévia e esquece de repetir o intervalo).
func (r *Recorder) RememberPreview(sel Selection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sel.Exclude = slices.Clone(sel.Exclude)
	r.preview = &sel
}

// LastPreview devolve a escolha da última prévia com esse nome.
func (r *Recorder) LastPreview(name string) (Selection, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.preview == nil || r.preview.Name != name {
		return Selection{}, false
	}
	return *r.preview, true
}

// Add grava um passo e devolve seu número.
func (r *Recorder) Add(s Step) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, s)
	return len(r.steps)
}

// Range devolve os passos de from até to (1-based, inclusive); from 0 usa "desde o último
// flow_save" e to 0 vai até o último passo.
func (r *Recorder) Range(from, to int) ([]Step, int, int, error) {
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
	if to == 0 {
		to = total
	}
	if to < from || to > total {
		return nil, 0, 0, fmt.Errorf("toStep must be between %d (fromStep) and %d", from, total)
	}
	return slices.Clone(r.steps[from-1 : to]), from, to, nil
}

// MarkSaved registra até onde o último flow_save foi (e esquece a prévia).
func (r *Recorder) MarkSaved(upTo int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.savedUpTo = upTo
	r.preview = nil
}

// Numbered é um passo com o número dele na sessão (o "flow step #N" de cada ação).
type Numbered struct {
	N    int
	Step Step
}

// Pick numera os passos from..from+len(steps)-1 da sessão e tira os de exclude. Um número fora do
// intervalo, repetido ou que tiraria todos os passos é erro (a IA errou o número: melhor avisar).
func Pick(steps []Step, from int, exclude []int) ([]Numbered, error) {
	to := from + len(steps) - 1
	drop := map[int]bool{}
	for _, n := range exclude {
		if n < from || n > to {
			return nil, fmt.Errorf("excludeSteps: step #%d is not in the steps being saved (#%d-#%d)", n, from, to)
		}
		if drop[n] {
			return nil, fmt.Errorf("excludeSteps: step #%d appears twice", n)
		}
		drop[n] = true
	}
	out := make([]Numbered, 0, len(steps))
	for i, s := range steps {
		if !drop[from+i] {
			out = append(out, Numbered{N: from + i, Step: s})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("excludeSteps removes every step (#%d-#%d): nothing to save", from, to)
	}
	return out, nil
}
