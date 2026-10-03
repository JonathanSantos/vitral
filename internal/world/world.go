// Package world roda os scripts do Vitral num "isolated world" do CDP: o mesmo DOM da página,
// mas um JavaScript separado. A página não alcança window.__vitral, o overlay nem a binding,
// e não consegue interferir nos protótipos que o snapshot usa.
package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

// Name é o nome do mundo isolado do Vitral (também usado na binding do overlay).
const Name = "vitral"

// Tracker acompanha o contexto de execução do mundo isolado no frame principal da página ativa.
type Tracker struct {
	mu      sync.Mutex
	page    *rod.Page
	frame   proto.PageFrameID
	id      proto.RuntimeExecutionContextID
	changed chan struct{} // fechado e trocado a cada mudança de contexto
}

// New cria o rastreador.
func New() *Tracker {
	return &Tracker{changed: make(chan struct{})}
}

// Attach começa a rastrear uma página nova. Precisa vir antes dos scripts (AddScript),
// para não perder o evento de criação do contexto.
func (t *Tracker) Attach(p *rod.Page) error {
	t.mu.Lock()
	t.page, t.frame, t.id = p, p.FrameID, 0
	t.notifyLocked()
	t.mu.Unlock()
	go p.EachEvent(t.onCreated, t.onDestroyed, t.onCleared)()
	return (proto.RuntimeEnable{}).Call(p)
}

func (t *Tracker) notifyLocked() {
	close(t.changed)
	t.changed = make(chan struct{})
}

func (t *Tracker) onCreated(e *proto.RuntimeExecutionContextCreated) {
	c := e.Context
	if c == nil || c.Name != Name {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if frame, ok := c.AuxData["frameId"]; ok && proto.PageFrameID(frame.Str()) != t.frame {
		return // iframes têm o próprio mundo; o Vitral avalia sempre no frame principal
	}
	t.id = c.ID
	t.notifyLocked()
}

func (t *Tracker) onDestroyed(e *proto.RuntimeExecutionContextDestroyed) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e.ExecutionContextID == t.id {
		t.id = 0
		t.notifyLocked()
	}
}

func (t *Tracker) onCleared(*proto.RuntimeExecutionContextsCleared) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.id = 0
	t.notifyLocked()
}

// AddScript registra um script para rodar no mundo isolado de todo documento novo
// e também no documento atual (runImmediately).
func AddScript(p *rod.Page, source string) error {
	_, err := proto.PageAddScriptToEvaluateOnNewDocument{Source: source, WorldName: Name, RunImmediately: true}.Call(p)
	return err
}

// contextID espera o contexto do documento atual existir (durante uma navegação ele some por instantes).
func (t *Tracker) contextID(ctx context.Context) (*rod.Page, proto.RuntimeExecutionContextID, error) {
	for {
		t.mu.Lock()
		p, id, changed := t.page, t.id, t.changed
		t.mu.Unlock()
		if p == nil {
			return nil, 0, errors.New("no page attached")
		}
		if id != 0 {
			return p, id, nil
		}
		// Não criamos o mundo aqui: um mundo criado à mão viria sem os scripts do Vitral.
		// Quem cria é o AddScript (documento atual) e o próprio Chrome (documentos novos).
		select {
		case <-ctx.Done():
			return nil, 0, fmt.Errorf("isolated world not ready: %w", ctx.Err())
		case <-changed:
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// staleContext reconhece o erro de um contexto que acabou de ser destruído (navegação).
func staleContext(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Cannot find context") ||
		strings.Contains(err.Error(), "Execution context was destroyed"))
}

func (t *Tracker) forget(id proto.RuntimeExecutionContextID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.id == id {
		t.id = 0
		t.notifyLocked()
	}
}

// Call chama uma função JS no mundo isolado e devolve o resultado em JSON.
func (t *Tracker) Call(ctx context.Context, fn string, args ...any) ([]byte, error) {
	callArgs := make([]*proto.RuntimeCallArgument, len(args))
	for i, a := range args {
		callArgs[i] = &proto.RuntimeCallArgument{Value: gson.New(a)}
	}
	for {
		p, id, err := t.contextID(ctx)
		if err != nil {
			return nil, err
		}
		res, err := proto.RuntimeCallFunctionOn{
			FunctionDeclaration: fn,
			Arguments:           callArgs,
			ExecutionContextID:  id,
			ReturnByValue:       true,
			AwaitPromise:        true,
		}.Call(p.Context(ctx))
		if staleContext(err) {
			t.forget(id)
			continue
		}
		if err != nil {
			return nil, err
		}
		if res.ExceptionDetails != nil {
			return nil, fmt.Errorf("evaluate in page: %s", exceptionText(res.ExceptionDetails))
		}
		return []byte(res.Result.Value.JSON("", "")), nil
	}
}

// Eval avalia uma expressão no mundo isolado, ignorando o resultado.
func (t *Tracker) Eval(ctx context.Context, expr string) error {
	for {
		p, id, err := t.contextID(ctx)
		if err != nil {
			return err
		}
		res, err := proto.RuntimeEvaluate{Expression: expr, ContextID: id}.Call(p.Context(ctx))
		if staleContext(err) {
			t.forget(id)
			continue
		}
		if err != nil {
			return err
		}
		if res.ExceptionDetails != nil {
			return fmt.Errorf("evaluate in page: %s", exceptionText(res.ExceptionDetails))
		}
		return nil
	}
}

func exceptionText(d *proto.RuntimeExceptionDetails) string {
	if d.Exception != nil && d.Exception.Description != "" {
		line, _, _ := strings.Cut(d.Exception.Description, "\n")
		return line
	}
	return d.Text
}

// Unmarshal é um atalho para Call + json.Unmarshal.
func (t *Tracker) Unmarshal(ctx context.Context, out any, fn string, args ...any) error {
	data, err := t.Call(ctx, fn, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
