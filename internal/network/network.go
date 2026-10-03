// Package network guarda o histórico de requests da página ativa num ring buffer.
package network

import (
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/ring"
)

// Entry é uma request observada. Corpos de resposta nunca são guardados aqui.
type Entry struct {
	ID          int
	RequestID   proto.NetworkRequestID
	Method      string
	URL         string
	Type        string
	Status      int
	StatusText  string
	Failed      string
	Mocked      bool
	Done        bool
	Start       float64
	End         float64
	Size        int64
	MIMEType    string
	ReqHeaders  map[string]string
	RespHeaders map[string]string
	PostData    string
	HasPostData bool
}

// DurationMS devolve a duração em milissegundos (0 se ainda pendente).
func (e Entry) DurationMS() int {
	if e.End == 0 || e.Start == 0 {
		return 0
	}
	return int((e.End - e.Start) * 1000)
}

// Tracker recebe eventos do domínio Network e mantém o histórico e as requests em voo.
type Tracker struct {
	mu           sync.Mutex
	buf          *ring.Buffer[*Entry]
	byReq        map[proto.NetworkRequestID]*Entry
	inflight     map[proto.NetworkRequestID]bool
	mocked       map[proto.NetworkRequestID]bool
	held         map[proto.NetworkRequestID]bool
	nextID       int
	lastActivity time.Time
	preserve     bool
	mainFrame    proto.PageFrameID
	now          func() time.Time
}

// NewTracker cria um tracker com capacidade fixa.
func NewTracker(capacity int, preserve bool) *Tracker {
	return &Tracker{
		buf:      ring.New[*Entry](capacity),
		byReq:    map[proto.NetworkRequestID]*Entry{},
		inflight: map[proto.NetworkRequestID]bool{},
		mocked:   map[proto.NetworkRequestID]bool{},
		held:     map[proto.NetworkRequestID]bool{},
		preserve: preserve,
		now:      time.Now,
	}
}

// Attach liga o tracker a uma página nova. Chamado uma vez por página.
func (t *Tracker) Attach(p *rod.Page) error {
	t.mu.Lock()
	t.mainFrame = p.FrameID
	clear(t.inflight)
	clear(t.held)
	t.mu.Unlock()
	if err := (proto.NetworkEnable{}).Call(p); err != nil {
		return err
	}
	go p.EachEvent(t.OnRequest, t.OnResponse, t.OnFinished, t.OnFailed)()
	return nil
}

// ignoredForIdle são tipos que ficam abertos indefinidamente e não indicam carregamento.
var ignoredForIdle = map[proto.NetworkResourceType]bool{
	proto.NetworkResourceTypeWebSocket:   true,
	proto.NetworkResourceTypeEventSource: true,
}

// OnRequest trata Network.requestWillBeSent.
func (t *Tracker) OnRequest(e *proto.NetworkRequestWillBeSent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActivity = t.now()

	// Redirect reaproveita o requestId: fecha a entrada anterior com o status 3xx.
	if prev, ok := t.byReq[e.RequestID]; ok && e.RedirectResponse != nil {
		prev.Status = e.RedirectResponse.Status
		prev.StatusText = e.RedirectResponse.StatusText
		prev.RespHeaders = flatHeaders(e.RedirectResponse.Headers)
		prev.End = float64(e.Timestamp)
		prev.Done = true
	}

	isNavigation := e.Type == proto.NetworkResourceTypeDocument && string(e.LoaderID) == string(e.RequestID) &&
		(t.mainFrame == "" || e.FrameID == t.mainFrame)
	if isNavigation && !t.preserve {
		t.buf.Clear()
		clear(t.byReq)
	}

	t.nextID++
	entry := &Entry{
		ID:          t.nextID,
		RequestID:   e.RequestID,
		Method:      e.Request.Method,
		URL:         e.Request.URL,
		Type:        string(e.Type),
		Start:       float64(e.Timestamp),
		ReqHeaders:  flatHeaders(e.Request.Headers),
		PostData:    e.Request.PostData,
		HasPostData: e.Request.HasPostData,
		Mocked:      t.mocked[e.RequestID],
	}
	if evicted, ok := t.buf.Push(entry); ok && t.byReq[evicted.RequestID] == evicted {
		delete(t.byReq, evicted.RequestID)
	}
	t.byReq[e.RequestID] = entry
	if !ignoredForIdle[e.Type] && !t.held[e.RequestID] {
		t.inflight[e.RequestID] = true
	}
}

// OnResponse trata Network.responseReceived.
func (t *Tracker) OnResponse(e *proto.NetworkResponseReceived) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActivity = t.now()
	entry, ok := t.byReq[e.RequestID]
	if !ok || e.Response == nil {
		return
	}
	entry.Status = e.Response.Status
	entry.StatusText = e.Response.StatusText
	entry.MIMEType = e.Response.MIMEType
	entry.RespHeaders = flatHeaders(e.Response.Headers)
	if len(e.Response.RequestHeaders) > 0 {
		entry.ReqHeaders = flatHeaders(e.Response.RequestHeaders)
	}
}

// OnFinished trata Network.loadingFinished.
func (t *Tracker) OnFinished(e *proto.NetworkLoadingFinished) {
	t.finish(e.RequestID, float64(e.Timestamp), func(entry *Entry) {
		entry.Size = int64(e.EncodedDataLength)
	})
}

// OnFailed trata Network.loadingFailed.
func (t *Tracker) OnFailed(e *proto.NetworkLoadingFailed) {
	t.finish(e.RequestID, float64(e.Timestamp), func(entry *Entry) {
		entry.Failed = e.ErrorText
		if e.Canceled {
			entry.Failed = "canceled"
		}
	})
}

func (t *Tracker) finish(id proto.NetworkRequestID, ts float64, update func(*Entry)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastActivity = t.now()
	delete(t.inflight, id)
	delete(t.mocked, id)
	delete(t.held, id)
	entry, ok := t.byReq[id]
	if !ok {
		return
	}
	entry.End = ts
	entry.Done = true
	update(entry)
}

// MarkMocked marca a request como atendida por mock (o evento pode chegar antes da request).
// Requests seguradas por delay saem da conta de "rede ocupada": assim a ação devolve o estado
// de carregamento em vez de esperar o delay acabar.
func (t *Tracker) MarkMocked(id proto.NetworkRequestID, delayed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if delayed {
		t.held[id] = true
		delete(t.inflight, id)
	}
	if entry, ok := t.byReq[id]; ok {
		entry.Mocked = true
		return
	}
	t.mocked[id] = true
}

// IdleSince informa se não há requests em voo e há quanto tempo a rede está parada,
// contando no mínimo a partir de since (início da ação).
func (t *Tracker) IdleSince(since time.Time) (pending int, idle time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.inflight) > 0 {
		return len(t.inflight), 0
	}
	from := t.lastActivity
	if since.After(from) {
		from = since
	}
	return 0, t.now().Sub(from)
}

// Mark devolve o id da última entrada, para consultar o que veio depois de uma ação.
func (t *Tracker) Mark() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.nextID
}

// Entries devolve cópias de todas as entradas, da mais antiga para a mais nova.
func (t *Tracker) Entries() []Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	items := t.buf.Items()
	out := make([]Entry, len(items))
	for i, e := range items {
		out[i] = *e
	}
	return out
}

// Get busca uma entrada pelo id curto.
func (t *Tracker) Get(id int) (Entry, bool) {
	for _, e := range t.Entries() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

func flatHeaders(h proto.NetworkHeaders) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v.Str()
	}
	return out
}
