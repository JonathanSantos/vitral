package mock

import (
	"log/slog"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/security"
)

// Interceptor usa o domínio Fetch para duas coisas: aplicar mocks e barrar navegações para hosts
// fora do --allowed-hosts. Só liga o Fetch quando há algo a fazer, para não pausar toda request.
type Interceptor struct {
	reg     *Registry // nil com --disable-mock
	allowed []string
	log     *slog.Logger
	// onMocked avisa o histórico de rede; delayed indica request segurada pelo delay.
	onMocked func(id proto.NetworkRequestID, delayed bool)

	mu     sync.Mutex
	client proto.Client // a página ativa (rod.Page implementa proto.Client)
}

// NewInterceptor cria o interceptador e passa a reagir a mudanças nas regras.
func NewInterceptor(reg *Registry, allowed []string, logger *slog.Logger, onMocked func(proto.NetworkRequestID, bool)) *Interceptor {
	in := &Interceptor{reg: reg, allowed: allowed, log: logger, onMocked: onMocked}
	if reg != nil {
		reg.OnChange(in.sync)
	}
	return in
}

// Patterns monta os padrões do Fetch.enable: os das regras de mock e, com allowed hosts, todo
// documento (navegação do frame principal ou de iframes).
func Patterns(mockPatterns []string, guardNavigation bool) []*proto.FetchRequestPattern {
	var out []*proto.FetchRequestPattern
	for _, p := range mockPatterns {
		out = append(out, &proto.FetchRequestPattern{URLPattern: p, RequestStage: proto.FetchRequestStageRequest})
	}
	if guardNavigation {
		out = append(out, &proto.FetchRequestPattern{URLPattern: "*", ResourceType: proto.NetworkResourceTypeDocument, RequestStage: proto.FetchRequestStageRequest})
	}
	return out
}

// Decision é o que fazer com uma request pausada.
type Decision struct {
	Block bool // navegação para host fora da lista
	Rule  *Rule
}

// Decide é a regra pura: navegação fora da lista é barrada; senão, a regra de mock mais recente vale.
func Decide(reg *Registry, allowed []string, url, method string, resource proto.NetworkResourceType) Decision {
	if resource == proto.NetworkResourceTypeDocument && !security.HostAllowed(allowed, url) {
		return Decision{Block: true}
	}
	if reg == nil {
		return Decision{}
	}
	if rule, ok := reg.Take(url, method); ok {
		return Decision{Rule: &rule}
	}
	return Decision{}
}

// Attach prepara uma página nova.
func (in *Interceptor) Attach(p *rod.Page) error {
	in.mu.Lock()
	in.client = p
	in.mu.Unlock()
	go p.EachEvent(func(e *proto.FetchRequestPaused) {
		go in.handle(p, e)
	})()
	in.sync()
	return nil
}

// sync liga o Fetch com os padrões atuais ou desliga quando não há regras.
func (in *Interceptor) sync() {
	in.mu.Lock()
	p := in.client
	in.mu.Unlock()
	if p == nil {
		return
	}
	var mockPatterns []string
	if in.reg != nil {
		mockPatterns = in.reg.Patterns()
	}
	patterns := Patterns(mockPatterns, len(in.allowed) > 0)
	if len(patterns) == 0 {
		if err := (proto.FetchDisable{}).Call(p); err != nil {
			in.log.Debug("fetch disable", "err", err)
		}
		return
	}
	if err := (proto.FetchEnable{Patterns: patterns}).Call(p); err != nil {
		in.log.Warn("fetch enable failed; mocks will not apply", "err", err)
	}
}

func (in *Interceptor) handle(p proto.Client, e *proto.FetchRequestPaused) {
	d := Decide(in.reg, in.allowed, e.Request.URL, e.Request.Method, e.ResourceType)
	if d.Block {
		if err := (proto.FetchFailRequest{RequestID: e.RequestID, ErrorReason: proto.NetworkErrorReasonBlockedByClient}).Call(p); err != nil {
			in.log.Debug("fetch block", "err", err)
		}
		return
	}
	if d.Rule == nil {
		if err := (proto.FetchContinueRequest{RequestID: e.RequestID}).Call(p); err != nil {
			in.log.Debug("fetch continue", "err", err)
		}
		return
	}
	rule := *d.Rule
	in.onMocked(e.NetworkID, rule.DelayMs > 0)
	if rule.DelayMs > 0 {
		time.Sleep(time.Duration(rule.DelayMs) * time.Millisecond)
	}
	if err := respond(p, e.RequestID, rule); err != nil {
		in.log.Debug("mock respond", "rule", rule.ID, "err", err)
	}
	if Expired(rule) && in.reg != nil {
		in.reg.Notify()
	}
}

func respond(p proto.Client, id proto.FetchRequestID, rule Rule) error {
	if rule.Fail != "" {
		reason, err := FailReason(rule.Fail)
		if err != nil {
			return err
		}
		return proto.FetchFailRequest{RequestID: id, ErrorReason: reason}.Call(p)
	}
	return proto.FetchFulfillRequest{
		RequestID:       id,
		ResponseCode:    rule.Status,
		ResponseHeaders: ResponseHeaders(rule),
		Body:            []byte(rule.Body),
	}.Call(p)
}
