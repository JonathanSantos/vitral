package mock

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

// fakeCDP registra as chamadas de CDP feitas pelo interceptador.
type fakeCDP struct {
	mu    sync.Mutex
	calls []string
	last  map[string]json.RawMessage
}

func (f *fakeCDP) Call(_ context.Context, _, method string, params any) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method)
	if f.last == nil {
		f.last = map[string]json.RawMessage{}
	}
	data, _ := json.Marshal(params)
	f.last[method] = data
	return []byte("{}"), nil
}

func (f *fakeCDP) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func newTestInterceptor(reg *Registry, allowed []string) (*Interceptor, *fakeCDP, *[]proto.NetworkRequestID) {
	var mocked []proto.NetworkRequestID
	in := NewInterceptor(reg, allowed, slog.New(slog.NewTextHandler(io.Discard, nil)), func(id proto.NetworkRequestID, _ bool) {
		mocked = append(mocked, id)
	})
	f := &fakeCDP{}
	in.client = f
	return in, f, &mocked
}

func paused(url string, resource proto.NetworkResourceType) *proto.FetchRequestPaused {
	return &proto.FetchRequestPaused{
		RequestID: "r1", NetworkID: "n1", ResourceType: resource,
		Request: &proto.NetworkRequest{URL: url, Method: "GET"},
	}
}

func TestInterceptorHandle(t *testing.T) {
	reg := NewRegistry(nil)
	in, f, mocked := newTestInterceptor(reg, []string{"portal.local"})
	mustAdd(t, reg, Rule{URLPattern: "/api/ok", Status: 201, Body: `{"a":1}`, Times: 1})
	mustAdd(t, reg, Rule{URLPattern: "/api/down", Fail: "connection_refused"})

	tests := []struct {
		name   string
		req    *proto.FetchRequestPaused
		method string
	}{
		{"blocked navigation", paused("http://evil.test/", proto.NetworkResourceTypeDocument), "Fetch.failRequest"},
		{"no rule continues", paused("http://portal.local/other", proto.NetworkResourceTypeFetch), "Fetch.continueRequest"},
		{"fulfill mock", paused("http://portal.local/api/ok", proto.NetworkResourceTypeFetch), "Fetch.fulfillRequest"},
		{"fail mock", paused("http://portal.local/api/down", proto.NetworkResourceTypeFetch), "Fetch.failRequest"},
	}
	for _, tt := range tests {
		before := len(f.methods())
		in.handle(f, tt.req)
		calls := f.methods()[before:]
		if len(calls) == 0 || calls[0] != tt.method {
			t.Errorf("%s: calls %v, want %s", tt.name, calls, tt.method)
		}
	}
	if len(*mocked) != 2 {
		t.Fatalf("mocked requests reported = %d", len(*mocked))
	}
	// A regra com times=1 expirou: o interceptador religa o Fetch só com o que sobrou.
	if calls := f.methods(); calls[len(calls)-1] != "Fetch.failRequest" {
		t.Fatalf("calls %v", calls)
	}
}

func TestInterceptorSync(t *testing.T) {
	reg := NewRegistry(nil)
	in, f, _ := newTestInterceptor(reg, nil)
	in.sync()
	if calls := f.methods(); len(calls) != 1 || calls[0] != "Fetch.disable" {
		t.Fatalf("no rules: %v", calls)
	}
	mustAdd(t, reg, Rule{URLPattern: "/api"}) // OnChange → sync
	if calls := f.methods(); calls[len(calls)-1] != "Fetch.enable" {
		t.Fatalf("with a rule: %v", calls)
	}
	guard, g, _ := newTestInterceptor(nil, []string{"portal.local"})
	guard.sync()
	if calls := g.methods(); len(calls) != 1 || calls[0] != "Fetch.enable" {
		t.Fatalf("allowed hosts without mocks still needs Fetch: %v", calls)
	}
}

func TestRespondBodyAndHeaders(t *testing.T) {
	f := &fakeCDP{}
	if err := respond(f, "r1", Rule{Status: 503, Body: `{"x":1}`}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ResponseCode    int                       `json:"responseCode"`
		ResponseHeaders []*proto.FetchHeaderEntry `json:"responseHeaders"`
	}
	if err := json.Unmarshal(f.last["Fetch.fulfillRequest"], &got); err != nil {
		t.Fatal(err)
	}
	if got.ResponseCode != 503 || len(got.ResponseHeaders) != 1 || got.ResponseHeaders[0].Value != "application/json" {
		t.Fatalf("fulfill = %+v", got)
	}
	if err := respond(f, "r1", Rule{Fail: "explode"}); err == nil {
		t.Fatal("unknown fail must error")
	}
}
