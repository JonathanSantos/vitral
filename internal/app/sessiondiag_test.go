//go:build integration && sessiondiag

package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Medição para o session_save (pedido 4.2): ler e gravar o localStorage de uma origem que NÃO está
// aberta. (a) DOMStorage direto pelo CDP; (b) aba temporária na própria origem com a requisição
// interceptada (Fetch) devolvendo uma página vazia, sem rede e sem script da página.
// go test -tags 'integration sessiondiag' -run TestSessionDiag -v ./internal/app/
func TestSessionDiag(t *testing.T) {
	hh, cleanup := newHarness(nil)
	defer cleanup()
	a := hh.site.URL                                     // http://127.0.0.1:PORT
	b := strings.Replace(a, "127.0.0.1", "localhost", 1) // outra origem
	hh.ok(t, "open", map[string]any{"url": a + "/pages/form.html"})
	hh.mainWorld(t, `() => { localStorage.setItem('token', 'abc'); return {}; }`)
	hh.ok(t, "open", map[string]any{"url": b + "/pages/api.html"})
	p := hh.page(t)

	// (a) DOMStorage.
	start := time.Now()
	items, err := proto.DOMStorageGetDOMStorageItems{StorageID: &proto.DOMStorageStorageID{SecurityOrigin: a, IsLocalStorage: true}}.Call(p)
	t.Logf("(a) DOMStorage read of %s while on %s: %v items=%v err=%v", a, b, time.Since(start), items, err)
	err = proto.DOMStorageSetDOMStorageItem{StorageID: &proto.DOMStorageStorageID{SecurityOrigin: a, IsLocalStorage: true}, Key: "viaDOMStorage", Value: "1"}.Call(p)
	t.Logf("(a) DOMStorage write: err=%v", err)

	// (b) Aba temporária com a requisição interceptada.
	start = time.Now()
	br := p.Browser()
	tab, err := br.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		t.Fatal(err)
	}
	router := tab.HijackRequests()
	router.MustAdd(a+"/*", func(h *rod.Hijack) {
		h.Response.SetHeader("Content-Type", "text/html")
		h.Response.SetBody("<!doctype html><title></title>")
	})
	go router.Run()
	if err := tab.Navigate(a + "/__vitral_blank__"); err != nil {
		t.Fatal(err)
	}
	_ = tab.WaitLoad()
	res, err := tab.Eval(`() => { localStorage.setItem('viaTab', '1'); return JSON.stringify(Object.assign({}, localStorage)); }`)
	_ = router.Stop()
	_ = tab.Close()
	got := ""
	if err == nil {
		got = res.Value.String()
	}
	t.Logf("(b) temporary tab read+write of %s: %v -> %s err=%v", a, time.Since(start), got, err)

	// O que a página de A vê ao carregar (antes do script dela).
	hh.ok(t, "open", map[string]any{"url": a + "/pages/form.html"})
	seen := hh.mainWorld(t, `() => ({all: JSON.stringify(Object.assign({}, localStorage))})`)
	t.Logf("page %s sees: %v", a, seen["all"])
}
