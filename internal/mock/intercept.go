package mock

import (
	"log/slog"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Interceptor liga o Fetch na página só enquanto existem regras, para não pausar toda request à toa.
type Interceptor struct {
	reg *Registry
	log *slog.Logger
	// onMocked avisa o histórico de rede; delayed indica request segurada pelo delay.
	onMocked func(id proto.NetworkRequestID, delayed bool)

	mu   sync.Mutex
	page *rod.Page
}

// NewInterceptor cria o interceptador e passa a reagir a mudanças nas regras.
func NewInterceptor(reg *Registry, logger *slog.Logger, onMocked func(proto.NetworkRequestID, bool)) *Interceptor {
	in := &Interceptor{reg: reg, log: logger, onMocked: onMocked}
	reg.OnChange(in.sync)
	return in
}

// Attach prepara uma página nova.
func (in *Interceptor) Attach(p *rod.Page) error {
	in.mu.Lock()
	in.page = p
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
	p := in.page
	in.mu.Unlock()
	if p == nil {
		return
	}
	patterns := in.reg.Patterns()
	if len(patterns) == 0 {
		if err := (proto.FetchDisable{}).Call(p); err != nil {
			in.log.Debug("fetch disable", "err", err)
		}
		return
	}
	req := proto.FetchEnable{}
	for _, pattern := range patterns {
		req.Patterns = append(req.Patterns, &proto.FetchRequestPattern{URLPattern: pattern, RequestStage: proto.FetchRequestStageRequest})
	}
	if err := req.Call(p); err != nil {
		in.log.Warn("fetch enable failed; mocks will not apply", "err", err)
	}
}

func (in *Interceptor) handle(p *rod.Page, e *proto.FetchRequestPaused) {
	rule, ok := in.reg.Take(e.Request.URL, e.Request.Method)
	if !ok {
		if err := (proto.FetchContinueRequest{RequestID: e.RequestID}).Call(p); err != nil {
			in.log.Debug("fetch continue", "err", err)
		}
		return
	}
	in.onMocked(e.NetworkID, rule.DelayMs > 0)
	if rule.DelayMs > 0 {
		time.Sleep(time.Duration(rule.DelayMs) * time.Millisecond)
	}
	if err := respond(p, e.RequestID, rule); err != nil {
		in.log.Debug("mock respond", "rule", rule.ID, "err", err)
	}
	if Expired(rule) {
		in.reg.Notify()
	}
}

func respond(p *rod.Page, id proto.FetchRequestID, rule Rule) error {
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
