// Package world roda os scripts do Vitral num "isolated world" do CDP: o mesmo DOM da página,
// mas um JavaScript separado. A página não alcança window.__vitral, o overlay nem a binding,
// e não consegue interferir nos protótipos que o snapshot usa.
package world

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// Elem é um elemento do DOM como argumento de Call: chega à função JS como o próprio elemento. Com
// Backend, vem do DOM.resolveNode no mundo isolado (alcança até shadow root fechado); sem Backend,
// da ref no registro do script (refs que o próprio script criou, como as dos links no texto).
// Elemento que não existe mais chega como null.
type Elem struct {
	Backend int
	Ref     string
}

var groups atomic.Int64

// resolveWorkers limita as resoluções de nó em paralelo (o primeiro snapshot de uma página grande
// resolve centenas de uma vez).
const resolveWorkers = 16

// Call chama uma função JS no mundo isolado e devolve o resultado em JSON.
func (t *Tracker) Call(ctx context.Context, fn string, args ...any) ([]byte, error) {
	for {
		p, id, err := t.contextID(ctx)
		if err != nil {
			return nil, err
		}
		pc := p.Context(ctx)
		group := ""
		callArgs, err := t.callArgs(pc, id, &group, args)
		if err == nil {
			var res *proto.RuntimeCallFunctionOnResult
			res, err = proto.RuntimeCallFunctionOn{
				FunctionDeclaration: fn,
				Arguments:           callArgs,
				ExecutionContextID:  id,
				ReturnByValue:       true,
				AwaitPromise:        true,
			}.Call(pc)
			if err == nil && res.ExceptionDetails != nil {
				err = fmt.Errorf("evaluate in page: %s", exceptionText(res.ExceptionDetails))
			}
			if err == nil {
				release(pc, group)
				return []byte(res.Result.Value.JSON("", "")), nil
			}
		}
		release(pc, group)
		if staleContext(err) {
			t.forget(id)
			continue
		}
		return nil, err
	}
}

func release(p *rod.Page, group string) {
	if group != "" {
		_ = proto.RuntimeReleaseObjectGroup{ObjectGroup: group}.Call(p)
	}
}

// callArgs converte os argumentos; os Elem viram objetos do mundo isolado (resolvidos em paralelo,
// num grupo de objetos liberado depois da chamada).
func (t *Tracker) callArgs(p *rod.Page, id proto.RuntimeExecutionContextID, group *string, args []any) ([]*proto.RuntimeCallArgument, error) {
	out := make([]*proto.RuntimeCallArgument, len(args))
	var elems []int
	for i, a := range args {
		if _, ok := a.(Elem); ok {
			elems = append(elems, i)
			continue
		}
		out[i] = &proto.RuntimeCallArgument{Value: gson.New(a)}
	}
	if len(elems) == 0 {
		return out, nil
	}
	*group = fmt.Sprintf("vitral-%d", groups.Add(1))
	errs := make([]error, len(args))
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for _, i := range elems {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			obj, err := resolve(p, id, *group, args[i].(Elem))
			if obj == "" {
				out[i] = &proto.RuntimeCallArgument{Value: gson.New(nil)}
			} else {
				out[i] = &proto.RuntimeCallArgument{ObjectID: obj}
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if staleContext(err) {
			return nil, err
		}
	}
	return out, nil
}

// resolve devolve o objeto do elemento no mundo isolado ("" se o nó não existe mais).
func resolve(p *rod.Page, id proto.RuntimeExecutionContextID, group string, e Elem) (proto.RuntimeRemoteObjectID, error) {
	if e.Backend != 0 {
		res, err := proto.DOMResolveNode{BackendNodeID: proto.DOMBackendNodeID(e.Backend), ObjectGroup: group, ExecutionContextID: id}.Call(p)
		if err != nil || res.Object == nil {
			return "", err
		}
		return res.Object.ObjectID, nil
	}
	res, err := proto.RuntimeCallFunctionOn{
		FunctionDeclaration: `(r) => window.__vitral ? window.__vitral.find(r) : null`,
		Arguments:           []*proto.RuntimeCallArgument{{Value: gson.New(e.Ref)}},
		ExecutionContextID:  id, ObjectGroup: group,
	}.Call(p)
	if err != nil || res.ExceptionDetails != nil || res.Result.Subtype != proto.RuntimeRemoteObjectSubtypeNode {
		return "", err
	}
	return res.Result.ObjectID, nil
}

// Nodes chama uma função JS que devolve uma lista de elementos e devolve o backendNodeId de cada
// um (0 para o que não for elemento). É o caminho de volta das refs criadas pelo próprio script.
func (t *Tracker) Nodes(ctx context.Context, fn string) ([]int, error) {
	for {
		p, id, err := t.contextID(ctx)
		if err != nil {
			return nil, err
		}
		pc := p.Context(ctx)
		group := fmt.Sprintf("vitral-%d", groups.Add(1))
		out, err := nodes(pc, id, group, fn)
		release(pc, group)
		if staleContext(err) {
			t.forget(id)
			continue
		}
		return out, err
	}
}

func nodes(p *rod.Page, id proto.RuntimeExecutionContextID, group, fn string) ([]int, error) {
	res, err := proto.RuntimeCallFunctionOn{FunctionDeclaration: fn, ExecutionContextID: id, ObjectGroup: group}.Call(p)
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("evaluate in page: %s", exceptionText(res.ExceptionDetails))
	}
	if res.Result.ObjectID == "" {
		return nil, nil
	}
	props, err := proto.RuntimeGetProperties{ObjectID: res.Result.ObjectID, OwnProperties: true}.Call(p)
	if err != nil {
		return nil, err
	}
	var objs []proto.RuntimeRemoteObjectID
	for _, pr := range props.Result {
		i, err := strconv.Atoi(pr.Name)
		if err != nil || pr.Value == nil {
			continue
		}
		for len(objs) <= i {
			objs = append(objs, "")
		}
		if pr.Value.Subtype == proto.RuntimeRemoteObjectSubtypeNode {
			objs[i] = pr.Value.ObjectID
		}
	}
	out := make([]int, len(objs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for i, obj := range objs {
		if obj == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if d, err := (proto.DOMDescribeNode{ObjectID: obj}).Call(p); err == nil && d.Node != nil {
				out[i] = int(d.Node.BackendNodeID)
			}
		}()
	}
	wg.Wait()
	return out, nil
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
