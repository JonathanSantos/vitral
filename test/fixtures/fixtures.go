// Package fixtures serve as páginas de testdata/pages e uma API fake para os testes com browser
// (integração e e2e). Só é usado por testes; o binário nunca importa este pacote.
package fixtures

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
)

// healPage é o nome de uma página de autocura (testdata/pages/heal/<nome>.v1.html).
var healPage = regexp.MustCompile(`^[a-z-]+$`)

// Server é o site de fixtures.
type Server struct {
	*httptest.Server
	// RenameSubmit simula uma mudança na tela: o botão "Enviar" do form.html vira "Confirmar".
	RenameSubmit atomic.Bool
	// OrdersStatus força o status da API /api/orders (0 = 200 normal).
	OrdersStatus atomic.Int32
	// HealV2 serve a versão 2 das páginas de autocura (/pages/heal/<caso>.html): a tela mudou.
	HealV2 atomic.Bool
}

// PagesDir é o caminho absoluto de testdata/pages, achado a partir deste arquivo.
func PagesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "pages")
}

// New sobe o servidor. Feche com Close.
func New() *Server {
	s := &Server{}
	s.Server = httptest.NewServer(s.mux())
	return s
}

// URL monta o endereço de uma página: s.Page("/pages/form.html").
func (s *Server) Page(path string) string { return s.URL + path }

func (s *Server) mux() *http.ServeMux {
	dir := PagesDir()
	mux := http.NewServeMux()
	mux.Handle("/pages/", http.StripPrefix("/pages/", http.FileServer(http.Dir(dir))))
	mux.HandleFunc("/pages/form.html", func(w http.ResponseWriter, r *http.Request) {
		html, err := os.ReadFile(filepath.Join(dir, "form.html"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.RenameSubmit.Load() {
			html = bytes.ReplaceAll(html, []byte(`<button type="submit">Enviar</button>`), []byte(`<button type="submit">Confirmar</button>`))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
	// Autocura: a mesma URL serve <caso>.v1.html ou <caso>.v2.html (testdata/pages/heal).
	mux.HandleFunc("/pages/heal/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSuffix(filepath.Base(r.URL.Path), ".html")
		if !healPage.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		version := ".v1.html"
		if s.HealV2.Load() {
			version = ".v2.html"
		}
		html, err := os.ReadFile(filepath.Join(dir, "heal", name+version))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html) //nolint:gosec // G705: só serve arquivos de testdata/pages/heal, com nome validado
	})
	// Página com link para outro host (o mesmo servidor por "localhost"), para o --allowed-hosts.
	mux.HandleFunc("/pages/external.html", func(w http.ResponseWriter, r *http.Request) {
		other := strings.Replace(s.URL, "127.0.0.1", "localhost", 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Externo</title></head>
<body><h1>Links</h1><a href="%s/pages/form.html">Site de fora</a></body></html>`, other)
	})
	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		if code := s.OrdersStatus.Load(); code != 0 {
			http.Error(w, `{"error":"forced"}`, int(code))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=secret")
		fmt.Fprint(w, `{"orders":[{"id":1023,"status":"Aprovado","total":450},{"id":1024,"status":"Pendente","total":99.9}]}`)
	})
	return mux
}
